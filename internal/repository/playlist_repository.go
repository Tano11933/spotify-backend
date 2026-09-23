package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"spotify-backend/internal/model"
)

const playlistSongsTable = "playlist_songs"

type PlaylistRepository struct {
	db *gorm.DB
}

func NewPlaylistRepository(db *gorm.DB) *PlaylistRepository {
	return &PlaylistRepository{db: db}
}

func (r *PlaylistRepository) Create(ctx context.Context, playlist *model.Playlist) error {
	return r.db.WithContext(ctx).Create(playlist).Error
}

func (r *PlaylistRepository) FindByID(ctx context.Context, id uint) (*model.Playlist, error) {
	var playlist model.Playlist
	err := r.db.WithContext(ctx).
		Preload("Songs.Artist").
		Preload("Songs.Album").
		First(&playlist, id).Error
	if err != nil {
		return nil, translateNotFound(err)
	}
	return &playlist, nil
}

func (r *PlaylistRepository) FindPageByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]model.Playlist, int64, error) {
	var playlists []model.Playlist
	var total int64

	if err := r.db.WithContext(ctx).
		Model(&model.Playlist{}).
		Where("user_id = ?", userID).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := r.db.WithContext(ctx).
		Preload("Songs.Artist").
		Preload("Songs.Album").
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&playlists).Error
	return playlists, total, err
}

func (r *PlaylistRepository) FindPagePublic(ctx context.Context, limit, offset int) ([]model.Playlist, int64, error) {
	var playlists []model.Playlist
	var total int64

	if err := r.db.WithContext(ctx).
		Model(&model.Playlist{}).
		Where("is_public = ?", true).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := r.db.WithContext(ctx).
		Preload("User").
		Preload("Songs.Artist").
		Preload("Songs.Album").
		Where("is_public = ?", true).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&playlists).Error
	return playlists, total, err
}

func (r *PlaylistRepository) Update(ctx context.Context, playlist *model.Playlist) error {
	return r.db.WithContext(ctx).Omit(clause.Associations).Save(playlist).Error
}

func (r *PlaylistRepository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Table(playlistSongsTable).Where("playlist_id = ?", id).Delete(nil).Error; err != nil {
			return err
		}

		result := tx.Delete(&model.Playlist{}, id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

func (r *PlaylistRepository) AddSong(ctx context.Context, playlistID, songID uint) error {
	return r.db.WithContext(ctx).Exec(
		`INSERT INTO `+playlistSongsTable+` (playlist_id, song_id)
		 VALUES (?, ?) ON CONFLICT DO NOTHING`,
		playlistID, songID,
	).Error
}

func (r *PlaylistRepository) RemoveSong(ctx context.Context, playlistID, songID uint) error {
	result := r.db.WithContext(ctx).
		Table(playlistSongsTable).
		Where("playlist_id = ? AND song_id = ?", playlistID, songID).
		Delete(nil)

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PlaylistRepository) HasSong(ctx context.Context, playlistID, songID uint) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Table(playlistSongsTable).
		Where("playlist_id = ? AND song_id = ?", playlistID, songID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
