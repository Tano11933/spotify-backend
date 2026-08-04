package repository

import (
	"gorm.io/gorm"
	"spotify-backend/internal/model"
)

type SongRepository struct {
	db *gorm.DB
}

func NewSongRepository(db *gorm.DB) *SongRepository {
	return &SongRepository{db: db}
}

func (r *SongRepository) Create(song *model.Song) error {
	if err := r.db.Create(song).Error; err != nil {
		return err
	}
	return r.db.Preload("Artist").Preload("Album").First(song, song.ID).Error
}

func (r *SongRepository) FindAll() ([]model.Song, error) {
	var songs []model.Song
	err := r.db.Preload("Artist").Find(&songs).Error
	return songs, err
}

func (r *SongRepository) FindByID(id uint) (*model.Song, error) {
	var song model.Song
	err := r.db.Preload("Artist").First(&song, id).Error
	if err != nil {
		return nil, err
	}
	return &song, nil
}

func (r *SongRepository) FindByArtistID(artistID uint) ([]model.Song, error) {
	var songs []model.Song
	err := r.db.Where("artist_id = ?", artistID).Find(&songs).Error
	return songs, err
}

func (r *SongRepository) Update(song *model.Song) error {
	return r.db.Save(song).Error
}

func (r *SongRepository) Delete(id uint) error {
	return r.db.Delete(&model.Song{}, id).Error
}