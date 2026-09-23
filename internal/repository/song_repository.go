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

// FindPage mengembalikan satu halaman lagu beserta totalnya.
//
// Lagu tidak di-cache (jarang dibaca berulang), jadi pagination-nya langsung
// di SQL — berbeda dari artist/album yang memotong list hasil cache.
func (r *SongRepository) FindPage(ctx context.Context, limit, offset int) ([]model.Song, int64, error) {
	var songs []model.Song
	var total int64

	if err := r.db.WithContext(ctx).Model(&model.Song{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := r.db.WithContext(ctx).
		Preload("Artist").
		Preload("Album").
		Limit(limit).
		Offset(offset).
		Find(&songs).Error
	return songs, total, err
}

func (r *SongRepository) FindByID(ctx context.Context, id uint) (*model.Song, error) {
	var song model.Song
	if err := r.db.WithContext(ctx).Preload("Artist").Preload("Album").First(&song, id).Error; err != nil {
		return nil, translateNotFound(err)
	}
	return &song, nil
}

func (r *SongRepository) FindPageByArtistID(ctx context.Context, artistID uint, limit, offset int) ([]model.Song, int64, error) {
	var songs []model.Song
	var total int64

	if err := r.db.WithContext(ctx).
		Model(&model.Song{}).
		Where("artist_id = ?", artistID).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := r.db.WithContext(ctx).
		Preload("Artist").
		Preload("Album").
		Where("artist_id = ?", artistID).
		Limit(limit).
		Offset(offset).
		Find(&songs).Error
	return songs, total, err
}

func (r *SongRepository) Update(ctx context.Context, song *model.Song) error {
	// Sama seperti di AlbumRepository: cegah GORM ikut meng-upsert Artist dan
	// Album yang ter-Preload di struct ini. Yang ingin diubah hanya baris lagunya.
	return r.db.WithContext(ctx).Omit(clause.Associations).Save(song).Error
}

func (r *SongRepository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Hapus relasi lebih dulu karena playlist_songs memiliki foreign key ke
		// songs. Playlist dan lagu lain tetap dipertahankan.
		if err := tx.Table("playlist_songs").Where("song_id = ?", id).Delete(nil).Error; err != nil {
			return err
		}

		result := tx.Delete(&model.Song{}, id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}
