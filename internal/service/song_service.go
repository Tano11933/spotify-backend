package service

import (
	"context"
	"errors"

	"spotify-backend/internal/model"
	"spotify-backend/internal/repository"
	"spotify-backend/pkg/cache"
	"spotify-backend/pkg/pagination"
)

var ErrSongNotFound = errors.New("song not found")

// Event type yang dikirim ke client WebSocket. Dijadikan konstanta supaya
// backend dan frontend punya satu daftar rujukan yang sama.
const (
	EventSongCreated = "song:created"
	EventSongPlaying = "song:playing"
)

type EventBroadcaster interface {
	BroadcastEvent(eventType string, payload any)
}

type SongService struct {
	repo        *repository.SongRepository
	artistRepo  *repository.ArtistRepository
	cache       *cache.Store
	broadcaster EventBroadcaster
}

func NewSongService(
	repo *repository.SongRepository,
	artistRepo *repository.ArtistRepository,
	cacheStore *cache.Store,
	broadcaster EventBroadcaster,
) *SongService {
	return &SongService{
		repo:        repo,
		artistRepo:  artistRepo,
		cache:       cacheStore,
		broadcaster: broadcaster,
	}
}

func (s *SongService) CreateSong(ctx context.Context, song *model.Song) error {
	if err := s.ensureArtistExists(ctx, song.ArtistID); err != nil {
		return err
	}

	if err := s.repo.Create(ctx, song); err != nil {
		return err
	}

	s.invalidateAlbum(ctx, song.AlbumID)

	s.broadcast(EventSongCreated, song)
	return nil
}

func (s *SongService) GetAllSongs(ctx context.Context, params pagination.Params) (pagination.Page[model.Song], error) {
	songs, total, err := s.repo.FindPage(ctx, params.Limit, params.Offset)
	if err != nil {
		return pagination.Page[model.Song]{}, err
	}
	return pagination.NewPage(songs, total, params), nil
}

func (s *SongService) GetSongByID(ctx context.Context, id uint) (*model.Song, error) {
	song, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrSongNotFound
		}
		return nil, err
	}
	return song, nil
}

func (s *SongService) GetSongsByArtist(ctx context.Context, artistID uint, params pagination.Params) (pagination.Page[model.Song], error) {
	if err := s.ensureArtistExists(ctx, artistID); err != nil {
		return pagination.Page[model.Song]{}, err
	}

	songs, total, err := s.repo.FindPageByArtistID(ctx, artistID, params.Limit, params.Offset)
	if err != nil {
		return pagination.Page[model.Song]{}, err
	}
	return pagination.NewPage(songs, total, params), nil
}

func (s *SongService) UpdateSong(ctx context.Context, song *model.Song) error {
	if err := s.ensureArtistExists(ctx, song.ArtistID); err != nil {
		return err
	}

	previous, err := s.repo.FindByID(ctx, song.ID)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return err
	}

	if err := s.repo.Update(ctx, song); err != nil {
		return err
	}

	if previous != nil {
		s.invalidateAlbum(ctx, previous.AlbumID)
	}
	s.invalidateAlbum(ctx, song.AlbumID)
	return nil
}

func (s *SongService) DeleteSong(ctx context.Context, id uint) error {
	song, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrSongNotFound
		}
		return err
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}

	s.invalidateAlbum(ctx, song.AlbumID)
	return nil
}

func (s *SongService) ensureArtistExists(ctx context.Context, artistID uint) error {
	if _, err := s.artistRepo.FindByID(ctx, artistID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrArtistNotFound
		}
		return err
	}
	return nil
}

func (s *SongService) invalidateAlbum(ctx context.Context, albumID *uint) {
	if albumID == nil {
		return
	}

	warnCache("invalidate album", s.cache.Delete(ctx, cacheKeyAlbumList, cacheKeyAlbum(*albumID)))
}

func (s *SongService) broadcast(eventType string, payload any) {
	if s.broadcaster == nil {
		return
	}
	s.broadcaster.BroadcastEvent(eventType, payload)
}
