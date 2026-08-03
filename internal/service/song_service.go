package service

import (
	"errors"
	"spotify-backend/internal/model"
	"spotify-backend/internal/repository"
)

type SongService struct {
	repo       *repository.SongRepository
	artistRepo *repository.ArtistRepository
}

func NewSongService(repo *repository.SongRepository, artistRepo *repository.ArtistRepository) *SongService {
	return &SongService{repo: repo, artistRepo: artistRepo}
}

func (s *SongService) CreateSong(song *model.Song) error {
	// cek dulu artist-nya beneran ada
	_, err := s.artistRepo.FindByID(song.ArtistID)
	if err != nil {
		return errors.New("artist not found")
	}
	return s.repo.Create(song)
}

func (s *SongService) GetAllSongs() ([]model.Song, error) {
	return s.repo.FindAll()
}

func (s *SongService) GetSongByID(id uint) (*model.Song, error) {
	return s.repo.FindByID(id)
}

func (s *SongService) GetSongsByArtist(artistID uint) ([]model.Song, error) {
	_, err := s.artistRepo.FindByID(artistID)
	if err != nil {
		return nil, errors.New("artist not found")
	}
	return s.repo.FindByArtistID(artistID)
}

func (s *SongService) UpdateSong(song *model.Song) error {
	_, err := s.artistRepo.FindByID(song.ArtistID)
	if err != nil {
		return errors.New("artist not found")
	}
	return s.repo.Update(song)
}

func (s *SongService) DeleteSong(id uint) error {
	return s.repo.Delete(id)
}