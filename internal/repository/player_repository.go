package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"spotify-backend/internal/model"
)

// PlayerRepository menyimpan state pemutaran, antrean, dan riwayat putar user.
type PlayerRepository struct {
	db *gorm.DB
}

func NewPlayerRepository(db *gorm.DB) *PlayerRepository {
	return &PlayerRepository{db: db}
}

/* -------------------------------------------------------------------------
 * State pemutaran
 * ---------------------------------------------------------------------- */

// UpsertState menulis state pemutaran user — satu baris per user, jadi
// konflik pada user_id diperbarui, bukan error.
func (r *PlayerRepository) UpsertState(ctx context.Context, userID uuid.UUID, songID uint, position int) error {
	state := model.PlayerState{
		UserID:          userID,
		SongID:          songID,
		PositionSeconds: position,
		UpdatedAt:       time.Now().UTC(),
	}

	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"song_id", "position_seconds", "updated_at"}),
		}).
		Create(&state).Error
}

// GetState mengembalikan nil (tanpa error) kalau user belum pernah memutar
// apa pun — "belum ada state" bukan keadaan gagal.
func (r *PlayerRepository) GetState(ctx context.Context, userID uuid.UUID) (*model.PlayerState, error) {
	var state model.PlayerState

	err := r.db.WithContext(ctx).First(&state, "user_id = ?", userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &state, nil
}

/* -------------------------------------------------------------------------
 * Antrean
 * ---------------------------------------------------------------------- */

// AppendToQueue menambahkan lagu ke akhir antrean. Posisi dihitung di dalam
// transaksi supaya dua request bersamaan tidak menghasilkan posisi yang sama.
func (r *PlayerRepository) AppendToQueue(ctx context.Context, userID uuid.UUID, songID uint) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var maxPosition int
		if err := tx.Model(&model.QueueItem{}).
			Where("user_id = ?", userID).
			Select("COALESCE(MAX(position), 0)").
			Scan(&maxPosition).Error; err != nil {
			return err
		}

		item := model.QueueItem{
			UserID:   userID,
			SongID:   songID,
			Position: maxPosition + 1,
			AddedAt:  time.Now().UTC(),
		}
		return tx.Create(&item).Error
	})
}

func (r *PlayerRepository) FindQueue(ctx context.Context, userID uuid.UUID, limit, offset int) ([]model.Song, int64, error) {
	var songs []model.Song
	var total int64

	if err := r.queueQuery(ctx, userID).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := r.queueQuery(ctx, userID).
		Preload("Artist").
		Preload("Album").
		Order("queue_items.position ASC").
		Limit(limit).
		Offset(offset).
		Find(&songs).Error
	return songs, total, err
}

func (r *PlayerRepository) queueQuery(ctx context.Context, userID uuid.UUID) *gorm.DB {
	return r.db.WithContext(ctx).
		Model(&model.Song{}).
		Joins("JOIN queue_items ON queue_items.song_id = songs.id AND queue_items.user_id = ?", userID)
}

// RemoveFromQueue menghapus kemunculan PERTAMA lagu di antrean. Lagu yang sama
// boleh ada lebih dari sekali (seperti Spotify), dan menghapus yang tidak ada
// bukan error — idempoten.
func (r *PlayerRepository) RemoveFromQueue(ctx context.Context, userID uuid.UUID, songID uint) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var item model.QueueItem

		err := tx.Where("user_id = ? AND song_id = ?", userID, songID).
			Order("position ASC").
			First(&item).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}

		return tx.Delete(&item).Error
	})
}

/* -------------------------------------------------------------------------
 * Riwayat & hitungan putar
 * ---------------------------------------------------------------------- */

func (r *PlayerRepository) AddHistory(ctx context.Context, userID uuid.UUID, songID uint) error {
	entry := model.PlayHistory{
		UserID:   userID,
		SongID:   songID,
		PlayedAt: time.Now().UTC(),
	}
	return r.db.WithContext(ctx).Create(&entry).Error
}

func (r *PlayerRepository) FindHistory(ctx context.Context, userID uuid.UUID, limit, offset int) ([]model.PlayHistory, int64, error) {
	var entries []model.PlayHistory
	var total int64

	query := r.db.WithContext(ctx).Model(&model.PlayHistory{}).Where("user_id = ?", userID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := r.db.WithContext(ctx).
		Model(&model.PlayHistory{}).
		Where("user_id = ?", userID).
		Order("played_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&entries).Error
	return entries, total, err
}

// IncrementPlayCount menaikkan penghitung di database secara atomik —
// `play_count + 1` dikerjakan Postgres, bukan read-modify-write di aplikasi.
func (r *PlayerRepository) IncrementPlayCount(ctx context.Context, songID uint) error {
	return r.db.WithContext(ctx).
		Model(&model.Song{}).
		Where("id = ?", songID).
		UpdateColumn("play_count", gorm.Expr("play_count + 1")).Error
}
