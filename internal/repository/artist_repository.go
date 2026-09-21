package repository

import (
	"context"

	"gorm.io/gorm"

	"spotify-backend/internal/model"
)

type ArtistRepository struct {
	db *gorm.DB
}

func NewArtistRepository(db *gorm.DB) *ArtistRepository {
	return &ArtistRepository{db: db}
}

func (r *ArtistRepository) Create(ctx context.Context, artist *model.Artist) error {
	return r.db.WithContext(ctx).Create(artist).Error
}

func (r *ArtistRepository) FindAll(ctx context.Context) ([]model.Artist, error) {
	var artists []model.Artist
	err := r.db.WithContext(ctx).Find(&artists).Error
	return artists, err
}

func (r *ArtistRepository) FindByID(ctx context.Context, id uint) (*model.Artist, error) {
	var artist model.Artist
	if err := r.db.WithContext(ctx).First(&artist, id).Error; err != nil {
		return nil, translateNotFound(err)
	}
	return &artist, nil
}

func (r *ArtistRepository) Update(ctx context.Context, artist *model.Artist) error {
	return r.db.WithContext(ctx).Save(artist).Error
}

func (r *ArtistRepository) Delete(ctx context.Context, id uint) error {
	result := r.db.WithContext(ctx).Delete(&model.Artist{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// CountDependents menghitung album dan lagu yang masih menunjuk ke artist ini.
// Dipakai service untuk menolak delete dengan pesan jelas, alih-alih
// membiarkan Postgres melempar error foreign key yang berakhir menjadi 500.
func (r *ArtistRepository) CountDependents(ctx context.Context, id uint) (albums, songs int64, err error) {
	if err = r.db.WithContext(ctx).
		Model(&model.Album{}).
		Where("artist_id = ?", id).
		Count(&albums).Error; err != nil {
		return 0, 0, err
	}

	if err = r.db.WithContext(ctx).
		Model(&model.Song{}).
		Where("artist_id = ?", id).
		Count(&songs).Error; err != nil {
		return 0, 0, err
	}

	return albums, songs, nil
}

// FindAlbumIDsByArtist mengembalikan ID album milik artist. Album meng-embed
// data artist (Preload "Artist"), jadi cache album:<id> ikut basi kalau artist
// berubah — service memakai daftar ini untuk membersihkannya.
func (r *ArtistRepository) FindAlbumIDsByArtist(ctx context.Context, id uint) ([]uint, error) {
	var ids []uint
	err := r.db.WithContext(ctx).
		Model(&model.Album{}).
		Where("artist_id = ?", id).
		Pluck("id", &ids).Error
	return ids, err
}
