package repository

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"spotify-backend/internal/model"
)

type SongRepository struct {
	db *gorm.DB
}

func NewSongRepository(db *gorm.DB) *SongRepository {
	return &SongRepository{db: db}
}

func (r *SongRepository) Create(ctx context.Context, song *model.Song) error {
	db := r.db.WithContext(ctx)

	if err := db.Create(song).Error; err != nil {
		return err
	}
	return db.Preload("Artist").Preload("Album").First(song, song.ID).Error
}

func (r *SongRepository) FindAll(ctx context.Context) ([]model.Song, error) {
	var songs []model.Song
	err := r.db.WithContext(ctx).Preload("Artist").Find(&songs).Error
	return songs, err
}

func (r *SongRepository) FindByID(ctx context.Context, id uint) (*model.Song, error) {
	var song model.Song
	if err := r.db.WithContext(ctx).Preload("Artist").First(&song, id).Error; err != nil {
		return nil, translateNotFound(err)
	}
	return &song, nil
}

func (r *SongRepository) FindByArtistID(ctx context.Context, artistID uint) ([]model.Song, error) {
	var songs []model.Song
	err := r.db.WithContext(ctx).Where("artist_id = ?", artistID).Find(&songs).Error
	return songs, err
}

func (r *SongRepository) Update(ctx context.Context, song *model.Song) error {
	// Sama seperti di AlbumRepository: cegah GORM ikut meng-upsert Artist dan
	// Album yang ter-Preload di struct ini. Yang ingin diubah hanya baris lagunya.
	return r.db.WithContext(ctx).Omit(clause.Associations).Save(song).Error
}

func (r *SongRepository) Delete(ctx context.Context, id uint) error {
	result := r.db.WithContext(ctx).Delete(&model.Song{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
