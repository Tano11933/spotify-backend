package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"spotify-backend/internal/model"
	"spotify-backend/internal/repository"
	"spotify-backend/pkg/cache"
	"spotify-backend/pkg/pagination"
)

var (
	// ErrGenreNotFound dikembalikan saat genre tidak ada. Dipetakan handler ke 404.
	ErrGenreNotFound = errors.New("genre not found")

	// ErrUnknownGenres muncul saat request menyebut id genre yang tidak ada.
	// Handler membalas 422 karena ini kesalahan referensi di body, bukan URL.
	ErrUnknownGenres = errors.New("one or more genres do not exist")

	// ErrInvalidGenreName muncul kalau nama genre tidak menghasilkan slug sama
	// sekali (mis. hanya tanda baca), sehingga URL-nya tidak mungkin dibuat.
	ErrInvalidGenreName = errors.New("genre name must contain at least one letter or digit")
)

type GenreService struct {
	repo       *repository.GenreRepository
	artistRepo *repository.ArtistRepository
	cache      *cache.Store
	cacheTTL   time.Duration
}

func NewGenreService(
	repo *repository.GenreRepository,
	artistRepo *repository.ArtistRepository,
	cacheStore *cache.Store,
	cacheTTL time.Duration,
) *GenreService {
	return &GenreService{repo: repo, artistRepo: artistRepo, cache: cacheStore, cacheTTL: cacheTTL}
}

func (s *GenreService) CreateGenre(ctx context.Context, genre *model.Genre) error {
	slug := slugify(genre.Name)
	if slug == "" {
		return ErrInvalidGenreName
	}
	genre.Slug = slug

	if err := s.repo.Create(ctx, genre); err != nil {
		return err
	}

	s.invalidateList(ctx)
	return nil
}

// GetAllGenres memakai pola cache yang sama dengan artist/album: list utuh
// disimpan di Redis dan pemotongan halaman dilakukan di sini.
func (s *GenreService) GetAllGenres(ctx context.Context, params pagination.Params) (pagination.Page[model.Genre], error) {
	var genres []model.Genre

	hit, err := s.cache.GetJSON(ctx, cacheKeyGenreList, &genres)
	warnCache("get "+cacheKeyGenreList, err)

	if !hit {
		genres, err = s.repo.FindAll(ctx)
		if err != nil {
			return pagination.Page[model.Genre]{}, err
		}

		warnCache("set "+cacheKeyGenreList, s.cache.SetJSON(ctx, cacheKeyGenreList, genres, s.cacheTTL))
	}

	return pagination.NewPage(pagination.Slice(genres, params), int64(len(genres)), params), nil
}

func (s *GenreService) GetGenreByID(ctx context.Context, id uint) (*model.Genre, error) {
	key := cacheKeyGenre(id)

	var genre model.Genre
	hit, err := s.cache.GetJSON(ctx, key, &genre)
	warnCache("get "+key, err)
	if hit {
		return &genre, nil
	}

	found, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrGenreNotFound
		}
		return nil, err
	}

	warnCache("set "+key, s.cache.SetJSON(ctx, key, found, s.cacheTTL))
	return found, nil
}

// UpdateGenre selalu menghitung ulang slug dari nama, supaya rename tidak
// meninggalkan slug lama yang menyesatkan.
func (s *GenreService) UpdateGenre(ctx context.Context, genre *model.Genre) error {
	slug := slugify(genre.Name)
	if slug == "" {
		return ErrInvalidGenreName
	}
	genre.Slug = slug

	if err := s.repo.Update(ctx, genre); err != nil {
		return err
	}

	// Response detail artist meng-embed genre, jadi rename juga membuat
	// salinan cache artist basi.
	s.invalidateOne(ctx, genre.ID)

	artistIDs, err := s.repo.FindArtistIDsByGenre(ctx, genre.ID)
	if err != nil {
		warnCache("find artists of genre", err)
		return nil
	}
	s.invalidateArtistDetails(ctx, artistIDs)
	return nil
}

func (s *GenreService) DeleteGenre(ctx context.Context, id uint) error {
	// Daftar artist diambil SEBELUM delete: setelah barisnya hilang, relasi
	// cascade-nya sudah tidak bisa dibaca lagi, padahal cache artist itulah
	// yang perlu dibersihkan.
	artistIDs, err := s.repo.FindArtistIDsByGenre(ctx, id)
	if err != nil {
		return err
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrGenreNotFound
		}
		return err
	}

	s.invalidateOne(ctx, id)
	s.invalidateArtistDetails(ctx, artistIDs)
	return nil
}

// SetArtistGenres menukar seluruh genre milik artist. Operasinya idempoten:
// request yang sama boleh dikirim berulang kali.
func (s *GenreService) SetArtistGenres(ctx context.Context, artistID uint, genreIDs []uint) (*model.Artist, error) {
	exists, err := s.artistRepo.Exists(ctx, artistID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrArtistNotFound
	}

	unique := dedupeIDs(genreIDs)
	count, err := s.repo.CountByIDs(ctx, unique)
	if err != nil {
		return nil, err
	}
	if count != int64(len(unique)) {
		return nil, ErrUnknownGenres
	}

	if err := s.repo.ReplaceArtistGenres(ctx, artistID, unique); err != nil {
		return nil, err
	}

	// Response detail artist meng-embed genre, jadi salinan di cache harus
	// dibuang. Daftar artist tidak memuat genre, jadi tidak perlu ikut.
	warnCache("invalidate artist", s.cache.Delete(ctx, cacheKeyArtist(artistID)))

	return s.artistRepo.FindByID(ctx, artistID)
}

func (s *GenreService) GetArtistsByGenre(ctx context.Context, genreID uint, params pagination.Params) (pagination.Page[model.Artist], error) {
	if _, err := s.GetGenreByID(ctx, genreID); err != nil {
		return pagination.Page[model.Artist]{}, err
	}

	artists, total, err := s.repo.FindPageArtists(ctx, genreID, params.Limit, params.Offset)
	if err != nil {
		return pagination.Page[model.Artist]{}, err
	}
	return pagination.NewPage(artists, total, params), nil
}

// invalidateOne membuang cache list dan detail genre. Cache artist TIDAK ikut
// di sini karena pemanggilnya yang tahu id artist terdampak: saat update
// relasinya masih bisa dibaca, saat delete harus diambil sebelum barisnya
// hilang. Keduanya memanggil invalidateArtistDetails secara eksplisit.
func (s *GenreService) invalidateOne(ctx context.Context, genreID uint) {
	warnCache("invalidate genre", s.cache.Delete(ctx, cacheKeyGenreList, cacheKeyGenre(genreID)))
}

func (s *GenreService) invalidateList(ctx context.Context) {
	warnCache("invalidate "+cacheKeyGenreList, s.cache.Delete(ctx, cacheKeyGenreList))
}

func (s *GenreService) invalidateArtistDetails(ctx context.Context, artistIDs []uint) {
	if len(artistIDs) == 0 {
		return
	}

	keys := make([]string, 0, len(artistIDs))
	for _, id := range artistIDs {
		keys = append(keys, cacheKeyArtist(id))
	}
	warnCache("invalidate artists after genre delete", s.cache.Delete(ctx, keys...))
}

func dedupeIDs(ids []uint) []uint {
	seen := make(map[uint]bool, len(ids))
	result := make([]uint, 0, len(ids))

	for _, id := range ids {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		result = append(result, id)
	}
	return result
}

// slugify mengubah "Indie Folk" menjadi "indie-folk".
//
// Salinan dari cmd/seed/main.go dengan sengaja: seeder butuh versi yang sama
// untuk membuat slug data demo, sementara service memakainya untuk request
// admin. Kalau algoritmanya berubah, ubah keduanya supaya data lama dan baru
// tetap konsisten.
func slugify(value string) string {
	var builder strings.Builder
	pendingDash := false

	for _, r := range strings.ToLower(value) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			if pendingDash && builder.Len() > 0 {
				builder.WriteByte('-')
			}
			pendingDash = false
			builder.WriteRune(r)
		default:
			pendingDash = true
		}
	}

	return builder.String()
}
