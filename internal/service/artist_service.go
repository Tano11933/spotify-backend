package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"spotify-backend/internal/model"
	"spotify-backend/internal/repository"
	"spotify-backend/pkg/cache"
)

// ErrArtistNotEmpty dikembalikan saat delete ditolak karena artist masih
// memiliki relasi. Handler memetakannya ke 409 supaya pesannya berguna —
// tanpa ini, Postgres melempar error foreign key yang menjadi 500.
var ErrArtistNotEmpty = errors.New("artist still has albums or songs")

type ArtistService struct {
	repo     *repository.ArtistRepository
	cache    *cache.Store
	cacheTTL time.Duration
}

func NewArtistService(
	repo *repository.ArtistRepository,
	cacheStore *cache.Store,
	cacheTTL time.Duration,
) *ArtistService {
	return &ArtistService{repo: repo, cache: cacheStore, cacheTTL: cacheTTL}
}

func (s *ArtistService) CreateArtist(ctx context.Context, artist *model.Artist) error {
	if err := s.repo.Create(ctx, artist); err != nil {
		return err
	}

	s.invalidateList(ctx)
	return nil
}

func (s *ArtistService) GetAllArtists(ctx context.Context) ([]model.Artist, error) {
	var artists []model.Artist

	hit, err := s.cache.GetJSON(ctx, cacheKeyArtistList, &artists)

	warnCache("get "+cacheKeyArtistList, err)
	if hit {
		return artists, nil
	}

	artists, err = s.repo.FindAll(ctx)
	if err != nil {
		return nil, err
	}

	warnCache("set "+cacheKeyArtistList, s.cache.SetJSON(ctx, cacheKeyArtistList, artists, s.cacheTTL))
	return artists, nil
}

func (s *ArtistService) GetArtistByID(ctx context.Context, id uint) (*model.Artist, error) {
	key := cacheKeyArtist(id)

	var artist model.Artist
	hit, err := s.cache.GetJSON(ctx, key, &artist)
	warnCache("get "+key, err)
	if hit {
		return &artist, nil
	}

	found, err := s.repo.FindByID(ctx, id)
	if err != nil {

		return nil, err
	}

	warnCache("set "+key, s.cache.SetJSON(ctx, key, found, s.cacheTTL))
	return found, nil
}

func (s *ArtistService) UpdateArtist(ctx context.Context, artist *model.Artist) error {
	if err := s.repo.Update(ctx, artist); err != nil {
		return err
	}

	s.invalidateOne(ctx, artist.ID)
	s.invalidateArtistAlbums(ctx, artist.ID)
	return nil
}

func (s *ArtistService) DeleteArtist(ctx context.Context, id uint) error {
	albums, songs, err := s.repo.CountDependents(ctx, id)
	if err != nil {
		return err
	}

	if albums > 0 || songs > 0 {
		return fmt.Errorf("%w: %d album(s), %d song(s)", ErrArtistNotEmpty, albums, songs)
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}

	s.invalidateOne(ctx, id)
	return nil
}

func (s *ArtistService) invalidateList(ctx context.Context) {
	warnCache("invalidate "+cacheKeyArtistList, s.cache.Delete(ctx, cacheKeyArtistList))
}

func (s *ArtistService) invalidateOne(ctx context.Context, id uint) {
	warnCache("invalidate artist", s.cache.Delete(ctx, cacheKeyArtistList, cacheKeyArtist(id)))
}

// invalidateArtistAlbums membersihkan cache album milik artist ini.
//
// Response album meng-embed data artist (Preload "Artist" di AlbumRepository),
// jadi mengubah artist membuat salinan di cache album ikut basi — tanpa ini,
// daftar/detail album menampilkan nama artist lama sampai TTL 5 menit habis.
func (s *ArtistService) invalidateArtistAlbums(ctx context.Context, artistID uint) {
	ids, err := s.repo.FindAlbumIDsByArtist(ctx, artistID)
	if err != nil {
		warnCache("find albums of artist", err)
		return
	}

	keys := make([]string, 0, len(ids)+1)
	keys = append(keys, cacheKeyAlbumList)
	for _, id := range ids {
		keys = append(keys, cacheKeyAlbum(id))
	}

	warnCache("invalidate artist albums", s.cache.Delete(ctx, keys...))
}
