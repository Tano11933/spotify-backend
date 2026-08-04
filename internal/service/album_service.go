package service

import (
	"errors"
	"spotify-backend/internal/model"
	"spotify-backend/internal/repository"
)

type AlbumService struct {
	repo       *repository.AlbumRepository
	artistRepo *repository.ArtistRepository
}

func NewAlbumService(repo *repository.AlbumRepository, artistRepo *repository.ArtistRepository) *AlbumService {
	return &AlbumService{repo: repo, artistRepo: artistRepo}
}

func (s *AlbumService) CreateAlbum(album *model.Album) error {
	_, err := s.artistRepo.FindByID(album.ArtistID)
	if err != nil {
		return errors.New("artist not found")
	}
	return s.repo.Create(album)
}

func (s *AlbumService) GetAllAlbums() ([]model.Album, error) {
	return s.repo.FindAll()
}

func (s *AlbumService) GetAlbumByID(id uint) (*model.Album, error) {
	return s.repo.FindByID(id)
}

func (s *AlbumService) UpdateAlbum(album *model.Album) error {
	_, err := s.artistRepo.FindByID(album.ArtistID)
	if err != nil {
		return errors.New("artist not found")
	}
	return s.repo.Update(album)
}

func (s *AlbumService) DeleteAlbum(id uint) error {
	return s.repo.Delete(id)
}