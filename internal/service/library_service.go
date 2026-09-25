package service

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"spotify-backend/internal/model"
	"spotify-backend/internal/repository"
	"spotify-backend/pkg/pagination"
)

// ErrAlbumNotFound dikembalikan saat album yang diminta tidak ada. Lagu dan
// artist memakai sentinel yang sudah ada (ErrSongNotFound, ErrArtistNotFound).
var ErrAlbumNotFound = errors.New("album not found")

// LibraryService mengurus pustaka pribadi user: lagu disimpan, album disimpan,
// dan artist yang diikuti.
//
// Menyimpan item yang tidak ada ditolak lebih awal (404) alih-alih membiarkan
// foreign key Postgres gagal — pesannya jauh lebih berguna. Menghapus item
// yang tidak tersimpan sengaja tidak divalidasi: hasilnya sama saja (idempoten).
type LibraryService struct {
	libraryRepo *repository.LibraryRepository
	songRepo    *repository.SongRepository
	albumRepo   *repository.AlbumRepository
	artistRepo  *repository.ArtistRepository
}

func NewLibraryService(
	libraryRepo *repository.LibraryRepository,
	songRepo *repository.SongRepository,
	albumRepo *repository.AlbumRepository,
	artistRepo *repository.ArtistRepository,
) *LibraryService {
	return &LibraryService{
		libraryRepo: libraryRepo,
		songRepo:    songRepo,
		albumRepo:   albumRepo,
		artistRepo:  artistRepo,
	}
}

/* -------------------------------------------------------------------------
 * Liked songs
 * ---------------------------------------------------------------------- */

func (s *LibraryService) SaveTrack(ctx context.Context, userID uuid.UUID, songID uint) error {
	exists, err := s.songRepo.Exists(ctx, songID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrSongNotFound
	}

	return s.libraryRepo.SaveTrack(ctx, userID, songID)
}

func (s *LibraryService) RemoveTrack(ctx context.Context, userID uuid.UUID, songID uint) error {
	return s.libraryRepo.RemoveTrack(ctx, userID, songID)
}

func (s *LibraryService) GetTracks(ctx context.Context, userID uuid.UUID, params pagination.Params) (pagination.Page[model.Song], error) {
	songs, total, err := s.libraryRepo.FindTracks(ctx, userID, params.Limit, params.Offset)
	if err != nil {
		return pagination.Page[model.Song]{}, err
	}
	return pagination.NewPage(songs, total, params), nil
}

// TracksContain mengembalikan status simpan untuk SETIAP id yang diminta —
// id yang tidak tersimpan tetap muncul dengan nilai false, supaya frontend
// bisa langsung memetakannya ke ikon tanpa menebak-nebak.
func (s *LibraryService) TracksContain(ctx context.Context, userID uuid.UUID, songIDs []uint) (map[uint]bool, error) {
	saved, err := s.libraryRepo.FindSavedTrackIDs(ctx, userID, songIDs)
	if err != nil {
		return nil, err
	}

	result := make(map[uint]bool, len(songIDs))
	for _, id := range songIDs {
		result[id] = false
	}
	for _, id := range saved {
		result[id] = true
	}
	return result, nil
}

/* -------------------------------------------------------------------------
 * Saved albums
 * ---------------------------------------------------------------------- */

func (s *LibraryService) SaveAlbum(ctx context.Context, userID uuid.UUID, albumID uint) error {
	exists, err := s.albumRepo.Exists(ctx, albumID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrAlbumNotFound
	}

	return s.libraryRepo.SaveAlbum(ctx, userID, albumID)
}

func (s *LibraryService) RemoveAlbum(ctx context.Context, userID uuid.UUID, albumID uint) error {
	return s.libraryRepo.RemoveAlbum(ctx, userID, albumID)
}

func (s *LibraryService) GetAlbums(ctx context.Context, userID uuid.UUID, params pagination.Params) (pagination.Page[model.Album], error) {
	albums, total, err := s.libraryRepo.FindAlbums(ctx, userID, params.Limit, params.Offset)
	if err != nil {
		return pagination.Page[model.Album]{}, err
	}
	return pagination.NewPage(albums, total, params), nil
}

func (s *LibraryService) AlbumsContain(ctx context.Context, userID uuid.UUID, albumIDs []uint) (map[uint]bool, error) {
	saved, err := s.libraryRepo.FindSavedAlbumIDs(ctx, userID, albumIDs)
	if err != nil {
		return nil, err
	}

	result := make(map[uint]bool, len(albumIDs))
	for _, id := range albumIDs {
		result[id] = false
	}
	for _, id := range saved {
		result[id] = true
	}
	return result, nil
}

/* -------------------------------------------------------------------------
 * Followed artists
 * ---------------------------------------------------------------------- */

func (s *LibraryService) FollowArtist(ctx context.Context, userID uuid.UUID, artistID uint) error {
	exists, err := s.artistRepo.Exists(ctx, artistID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrArtistNotFound
	}

	return s.libraryRepo.FollowArtist(ctx, userID, artistID)
}

func (s *LibraryService) UnfollowArtist(ctx context.Context, userID uuid.UUID, artistID uint) error {
	return s.libraryRepo.UnfollowArtist(ctx, userID, artistID)
}

func (s *LibraryService) GetFollowing(ctx context.Context, userID uuid.UUID, params pagination.Params) (pagination.Page[model.Artist], error) {
	artists, total, err := s.libraryRepo.FindFollowing(ctx, userID, params.Limit, params.Offset)
	if err != nil {
		return pagination.Page[model.Artist]{}, err
	}
	return pagination.NewPage(artists, total, params), nil
}

func (s *LibraryService) FollowingContain(ctx context.Context, userID uuid.UUID, artistIDs []uint) (map[uint]bool, error) {
	followed, err := s.libraryRepo.FindFollowedArtistIDs(ctx, userID, artistIDs)
	if err != nil {
		return nil, err
	}

	result := make(map[uint]bool, len(artistIDs))
	for _, id := range artistIDs {
		result[id] = false
	}
	for _, id := range followed {
		result[id] = true
	}
	return result, nil
}
