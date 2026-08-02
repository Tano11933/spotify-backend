package repository

import (
	"gorm.io/gorm"
	"spotify-backend/internal/model"
)

type ArtistRepository struct {
	db *gorm.DB
}

func NewArtistRepository(db *gorm.DB) *ArtistRepository {
	return &ArtistRepository{db: db}
}

func (r *ArtistRepository) Create(artist *model.Artist) error {
	return r.db.Create(artist).Error
}

func (r *ArtistRepository) FindAll() ([]model.Artist, error) {
	var artists []model.Artist
	err := r.db.Find(&artists).Error
	return artists, err
}

func (r *ArtistRepository) FindByID(id uint) (*model.Artist, error) {
	var artist model.Artist
	err := r.db.First(&artist, id).Error
	if err != nil {
		return nil, err
	}
	return &artist, nil
}

func (r *ArtistRepository) Update(artist *model.Artist) error {
	return r.db.Save(artist).Error
}

func (r *ArtistRepository) Delete(id uint) error {
	return r.db.Delete(&model.Artist{}, id).Error
}