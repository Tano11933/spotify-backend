package repository

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"spotify-backend/internal/model"
)

type AlbumRepository struct {
	db *gorm.DB
}

func NewAlbumRepository(db *gorm.DB) *AlbumRepository {
	return &AlbumRepository{db: db}
}

func (r *AlbumRepository) Create(ctx context.Context, album *model.Album) error {
	db := r.db.WithContext(ctx)

	if err := db.Create(album).Error; err != nil {
		return err
	}
	// Baca ulang dengan Preload supaya response menyertakan data Artist,
	// bukan cuma artist_id.
	return db.Preload("Artist").First(album, album.ID).Error
}

func (r *AlbumRepository) FindAll(ctx context.Context) ([]model.Album, error) {
	var albums []model.Album
	err := r.db.WithContext(ctx).Preload("Artist").Find(&albums).Error
	return albums, err
}

func (r *AlbumRepository) FindByID(ctx context.Context, id uint) (*model.Album, error) {
	var album model.Album
	err := r.db.WithContext(ctx).
		Preload("Artist").
		Preload("Songs").
		Preload("Songs.Artist").
		First(&album, id).Error
	if err != nil {
		return nil, translateNotFound(err)
	}
	return &album, nil
}

func (r *AlbumRepository) Update(ctx context.Context, album *model.Album) error {
	// Omit(clause.Associations) penting di sini. Secara default, Save() milik
	// GORM ikut meng-upsert seluruh relasi yang terisi di struct — dan struct
	// yang masuk ke sini berasal dari FindByID yang mem-Preload Artist dan Songs.
	// Tanpa Omit, update judul album akan sekaligus menulis ulang baris artist
	// dan SEMUA baris lagu di album itu, termasuk menimpa perubahan yang mungkin
	// dilakukan request lain di antaranya.
	return r.db.WithContext(ctx).Omit(clause.Associations).Save(album).Error
}

func (r *AlbumRepository) Delete(ctx context.Context, id uint) error {
	result := r.db.WithContext(ctx).Delete(&model.Album{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// CountSongs menghitung lagu yang masih menunjuk ke album ini. Dipakai service
// untuk menolak delete dengan pesan jelas, alih-alih membiarkan Postgres
// melempar error foreign key yang berakhir menjadi 500.
func (r *AlbumRepository) CountSongs(ctx context.Context, id uint) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.Song{}).
		Where("album_id = ?", id).
		Count(&count).Error
	return count, err
}
