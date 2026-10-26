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

var ErrQueueUnavailable = errors.New("queue entry is unavailable or reserved")

func (r *PlayerRepository) FindQueueEntries(ctx context.Context, userID uuid.UUID, limit, offset int) ([]model.QueueEntry, int64, error) {
	var rows []model.QueueItem
	var total int64
	query := r.db.WithContext(ctx).Model(&model.QueueItem{}).Where("user_id = ?", userID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := query.Order("position ASC, id ASC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	entries := make([]model.QueueEntry, 0, len(rows))
	ids := make([]uint, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.SongID)
	}
	var songs []model.Song
	if len(ids) > 0 {
		if err := r.db.WithContext(ctx).Preload("Artist").Preload("Album").Find(&songs, ids).Error; err != nil {
			return nil, 0, err
		}
	}
	byID := make(map[uint]model.Song, len(songs))
	for _, song := range songs {
		byID[song.ID] = song
	}
	for _, row := range rows {
		if song, ok := byID[row.SongID]; ok {
			entries = append(entries, model.QueueEntry{ID: row.ID, Position: row.Position, AddedAt: row.AddedAt, Song: song})
		}
	}
	return entries, total, nil
}

func (r *PlayerRepository) ReserveQueueEntry(ctx context.Context, userID uuid.UUID, entryID uint64) (*model.QueueReservation, error) {
	var reservation *model.QueueReservation
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		var row model.QueueItem
		query := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("user_id = ? AND (reserved_until IS NULL OR reserved_until <= ?)", userID, now)
		if entryID != 0 {
			query = query.Where("id = ?", entryID)
		}
		err := query.Order("position ASC, id ASC").First(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if entryID != 0 {
				return ErrQueueUnavailable
			}
			return nil
		}
		if err != nil {
			return err
		}
		token := uuid.New()
		expires := now.Add(time.Minute)
		if err := tx.Model(&row).Updates(map[string]any{"reservation_id": token, "reserved_until": expires}).Error; err != nil {
			return err
		}
		var song model.Song
		if err := tx.Preload("Artist").Preload("Album").First(&song, row.SongID).Error; err != nil {
			return err
		}
		reservation = &model.QueueReservation{ID: token, EntryID: row.ID, ExpiresAt: expires, Song: song}
		return nil
	})
	return reservation, err
}

func (r *PlayerRepository) ConfirmQueueEntry(ctx context.Context, userID uuid.UUID, reservationID uuid.UUID) error {
	result := r.db.WithContext(ctx).Where("user_id = ? AND reservation_id = ? AND reserved_until > ?", userID, reservationID, time.Now().UTC()).Delete(&model.QueueItem{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrQueueUnavailable
	}
	return nil
}

func (r *PlayerRepository) ReleaseQueueEntry(ctx context.Context, userID uuid.UUID, reservationID uuid.UUID) error {
	return r.db.WithContext(ctx).Model(&model.QueueItem{}).Where("user_id = ? AND reservation_id = ?", userID, reservationID).
		Updates(map[string]any{"reservation_id": nil, "reserved_until": nil}).Error
}

func (r *PlayerRepository) RemoveQueueEntry(ctx context.Context, userID uuid.UUID, entryID uint64) error {
	return r.db.WithContext(ctx).Where("user_id = ? AND id = ?", userID, entryID).Delete(&model.QueueItem{}).Error
}
