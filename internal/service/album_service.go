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

var ErrArtistNotFound = errors.New("artist not found")

// ErrAlbumNotEmpty dikembalikan saat delete ditolak karena album masih
// memiliki lagu. Handler memetakannya ke 409 — lihat ErrArtistNotEmpty.
var ErrAlbumNotEmpty = errors.New("album still has songs")

type AlbumService struct {
	repo       *repository.AlbumRepository
	artistRepo *repository.ArtistRepository
	cache      *cache.Store
	cacheTTL   time.Duration
}

func NewAlbumService(
	repo *repository.AlbumRepository,
	artistRepo *repository.ArtistRepository,
	cacheStore *cache.Store,
	cacheTTL time.Duration,
) *AlbumService {
	return &AlbumService{
		repo:       repo,
		artistRepo: artistRepo,
		cache:      cacheStore,
		cacheTTL:   cacheTTL,
	}
}

func (s *AlbumService) CreateAlbum(ctx context.Context, album *model.Album) error {
	if err := s.ensureArtistExists(ctx, album.ArtistID); err != nil {
		return err
	}

	if err := s.repo.Create(ctx, album); err != nil {
		return err
	}

	s.invalidateList(ctx)
	return nil
}

func (s *AlbumService) GetAllAlbums(ctx context.Context) ([]model.Album, error) {
	var albums []model.Album

	hit, err := s.cache.GetJSON(ctx, cacheKeyAlbumList, &albums)
	warnCache("get "+cacheKeyAlbumList, err)
	if hit {
		return albums, nil
	}

	albums, err = s.repo.FindAll(ctx)
	if err != nil {
		return nil, err
	}

	warnCache("set "+cacheKeyAlbumList, s.cache.SetJSON(ctx, cacheKeyAlbumList, albums, s.cacheTTL))
	return albums, nil
}

func (s *AlbumService) GetAlbumByID(ctx context.Context, id uint) (*model.Album, error) {
	key := cacheKeyAlbum(id)

	var album model.Album
	hit, err := s.cache.GetJSON(ctx, key, &album)
	warnCache("get "+key, err)
	if hit {
		return &album, nil
	}

	found, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	warnCache("set "+key, s.cache.SetJSON(ctx, key, found, s.cacheTTL))
	return found, nil
}

func (s *AlbumService) UpdateAlbum(ctx context.Context, album *model.Album) error {
	if err := s.ensureArtistExists(ctx, album.ArtistID); err != nil {
		return err
	}

	if err := s.repo.Update(ctx, album); err != nil {
		return err
	}

	s.invalidateOne(ctx, album.ID)
	return nil
}

func (s *AlbumService) DeleteAlbum(ctx context.Context, id uint) error {
	songs, err := s.repo.CountSongs(ctx, id)
	if err != nil {
		return err
	}

	if songs > 0 {
		return fmt.Errorf("%w: %d song(s)", ErrAlbumNotEmpty, songs)
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}

	s.invalidateOne(ctx, id)
	return nil
}

func (s *AlbumService) ensureArtistExists(ctx context.Context, artistID uint) error {
	if _, err := s.artistRepo.FindByID(ctx, artistID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrArtistNotFound
		}
		return err
	}
	return nil
}

func (s *AlbumService) invalidateList(ctx context.Context) {
	warnCache("invalidate "+cacheKeyAlbumList, s.cache.Delete(ctx, cacheKeyAlbumList))
}

func (s *AlbumService) invalidateOne(ctx context.Context, id uint) {
	warnCache("invalidate album", s.cache.Delete(ctx, cacheKeyAlbumList, cacheKeyAlbum(id)))
}
