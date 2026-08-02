package service

import (
	"spotify-backend/internal/model"
	"spotify-backend/internal/repository"
)

type ArtistService struct {
	repo *repository.ArtistRepository
}

func NewArtistService(repo *repository.ArtistRepository) *ArtistService {
	return &ArtistService{repo: repo}
}

func (s *ArtistService) CreateArtist(artist *model.Artist) error {
	return s.repo.Create(artist)
}

func (s *ArtistService) GetAllArtists() ([]model.Artist, error) {
	return s.repo.FindAll()
}

func (s *ArtistService) GetArtistByID(id uint) (*model.Artist, error) {
	return s.repo.FindByID(id)
}

func (s *ArtistService) UpdateArtist(artist *model.Artist) error {
	return s.repo.Update(artist)
}

func (s *ArtistService) DeleteArtist(id uint) error {
	return s.repo.Delete(id)
}