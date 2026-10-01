package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"spotify-backend/internal/model"
)

// GenreRepository menyimpan katalog genre dan relasi many2many ke artist.
type GenreRepository struct {
	db *gorm.DB
}

func NewGenreRepository(db *gorm.DB) *GenreRepository {
	return &GenreRepository{db: db}
}

func (r *GenreRepository) Create(ctx context.Context, genre *model.Genre) error {
	err := r.db.WithContext(ctx).Create(genre).Error
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return ErrDuplicate
	}
	return err
}

// FindAll mengembalikan seluruh genre, urut nama. List ini kecil dan dipakai
// halaman Browse, jadi service menyimpannya utuh di cache seperti artist/album.
func (r *GenreRepository) FindAll(ctx context.Context) ([]model.Genre, error) {
	var genres []model.Genre
	err := r.db.WithContext(ctx).Order("name ASC").Find(&genres).Error
	return genres, err
}

func (r *GenreRepository) FindByID(ctx context.Context, id uint) (*model.Genre, error) {
	var genre model.Genre
	if err := r.db.WithContext(ctx).First(&genre, id).Error; err != nil {
		return nil, translateNotFound(err)
	}
	return &genre, nil
}

func (r *GenreRepository) Update(ctx context.Context, genre *model.Genre) error {
	// Omit(clause.Associations) sama alasannya dengan AlbumRepository: struct
	// yang masuk bisa membawa relasi ter-Preload, dan Save() tanpa Omit ikut
	// menulis ulang baris relasi itu.
	err := r.db.WithContext(ctx).Omit(clause.Associations).Save(genre).Error
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return ErrDuplicate
	}
	return err
}

func (r *GenreRepository) Delete(ctx context.Context, id uint) error {
	result := r.db.WithContext(ctx).Delete(&model.Genre{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// FindPageArtists mengembalikan artist yang terhubung ke genre, urut nama.
func (r *GenreRepository) FindPageArtists(ctx context.Context, genreID uint, limit, offset int) ([]model.Artist, int64, error) {
	var artists []model.Artist
	var total int64

	if err := r.db.WithContext(ctx).
		Model(&model.Artist{}).
		Joins("JOIN artist_genres ON artist_genres.artist_id = artists.id AND artist_genres.genre_id = ?", genreID).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := r.db.WithContext(ctx).
		Model(&model.Artist{}).
		Joins("JOIN artist_genres ON artist_genres.artist_id = artists.id AND artist_genres.genre_id = ?", genreID).
		Order("artists.name ASC").
		Limit(limit).
		Offset(offset).
		Find(&artists).Error
	return artists, total, err
}

// FindArtistIDsByGenre dipakai service untuk membersihkan cache artist yang
// response-nya meng-embed genre, baik saat genre diganti nama maupun dihapus.
func (r *GenreRepository) FindArtistIDsByGenre(ctx context.Context, genreID uint) ([]uint, error) {
	var ids []uint
	err := r.db.WithContext(ctx).
		Table("artist_genres").
		Where("genre_id = ?", genreID).
		Pluck("artist_id", &ids).Error
	return ids, err
}

// CountByIDs menghitung berapa id yang benar-benar ada. Service membandingkan
// hasilnya dengan daftar id yang diminta untuk menolak id asing sebelum
// menulis relasi.
func (r *GenreRepository) CountByIDs(ctx context.Context, ids []uint) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}

	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.Genre{}).
		Where("id IN ?", ids).
		Count(&count).Error
	return count, err
}

// ReplaceArtistGenres menukar seluruh genre milik artist dalam satu transaksi.
//
// "Tukar", bukan "tambah": mengirim request yang sama dua kali menghasilkan
// keadaan yang sama, dan melepas semua genre cukup dengan daftar kosong.
func (r *GenreRepository) ReplaceArtistGenres(ctx context.Context, artistID uint, genreIDs []uint) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Table("artist_genres").
			Where("artist_id = ?", artistID).
			Delete(nil).Error; err != nil {
			return err
		}

		for _, genreID := range genreIDs {
			if err := tx.Exec(
				"INSERT INTO artist_genres (artist_id, genre_id) VALUES (?, ?) ON CONFLICT DO NOTHING",
				artistID, genreID,
			).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
