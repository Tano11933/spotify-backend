package repository

import (
	"gorm.io/gorm"
	"spotify-backend/internal/model"
)

type AlbumRepository struct {
	db *gorm.DB
}

func NewAlbumRepository(db *gorm.DB) *AlbumRepository {
	return &AlbumRepository{db: db}
}

func (r *AlbumRepository) Create(album *model.Album) error {
	if err := r.db.Create(album).Error; err != nil {
		return err
	}
	return r.db.Preload("Artist").First(album, album.ID).Error
}

func (r *AlbumRepository) FindAll() ([]model.Album, error) {
	var albums []model.Album
	err := r.db.Preload("Artist").Find(&albums).Error
	return albums, err
}

func (r *AlbumRepository) FindByID(id uint) (*model.Album, error) {
	var album model.Album
	err := r.db.Preload("Artist").Preload("Songs").First(&album, id).Error
	if err != nil {
		return nil, err
	}
	return &album, nil
}

func (r *AlbumRepository) Update(album *model.Album) error {
	return r.db.Save(album).Error
}

func (r *AlbumRepository) Delete(id uint) error {
	return r.db.Delete(&model.Album{}, id).Error
}