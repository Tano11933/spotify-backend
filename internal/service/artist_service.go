package service

import (
	"context"
	"time"

	"spotify-backend/internal/model"
	"spotify-backend/internal/repository"
	"spotify-backend/pkg/cache"
)

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
	return nil
}

func (s *ArtistService) DeleteArtist(ctx context.Context, id uint) error {
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
