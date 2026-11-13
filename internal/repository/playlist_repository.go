package repository

import (
	"context"
	"database/sql"

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

func (r *PlaylistRepository) FindSongPage(ctx context.Context, playlistID uint, viewerID uuid.UUID, limit, offset int) ([]model.Song, int64, error) {
	var songs []model.Song
	var total int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var playlist model.Playlist
		if err := tx.Select("id").
			Where("user_id = ? OR is_public = ?", viewerID, true).
			First(&playlist, playlistID).Error; err != nil {
			return translateNotFound(err)
		}

		membership := tx.Table(playlistSongsTable).Where("playlist_id = ?", playlist.ID)
		if err := membership.Session(&gorm.Session{}).Count(&total).Error; err != nil {
			return err
		}
		return tx.Preload("Artist").Preload("Album").
			Where("id IN (?)", membership.Select("song_id")).
			Order("id ASC").Limit(limit).Offset(offset).Find(&songs).Error
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return songs, total, err
}

func (r *PlaylistRepository) FindPageByUserID(ctx context.Context, userID uuid.UUID, limit, offset int, includeSongs bool) ([]model.Playlist, int64, error) {
	var playlists []model.Playlist
	var total int64

	if err := r.db.WithContext(ctx).
		Model(&model.Playlist{}).
		Where("user_id = ?", userID).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query := r.db.WithContext(ctx).Where("user_id = ?", userID)
	if includeSongs {
		query = query.Preload("Songs.Artist").Preload("Songs.Album")
	}
	err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&playlists).Error
	return playlists, total, err
}

func (r *PlaylistRepository) FindPagePublic(ctx context.Context, limit, offset int, includeSongs bool) ([]model.Playlist, int64, error) {
	var playlists []model.Playlist
	var total int64

	if err := r.db.WithContext(ctx).
		Model(&model.Playlist{}).
		Where("is_public = ?", true).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query := r.db.WithContext(ctx).Preload("User").Where("is_public = ?", true)
	if includeSongs {
		query = query.Preload("Songs.Artist").Preload("Songs.Album")
	}
	err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&playlists).Error
	return playlists, total, err
}

// FindPagePublicByUser mengembalikan playlist publik MILIK satu user, dipakai
// halaman profil publik. Songs sengaja tidak di-preload: daftar playlist di
// profil cukup menampilkan nama & pemiliknya.
func (r *PlaylistRepository) FindPagePublicByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]model.Playlist, int64, error) {
	var playlists []model.Playlist
	var total int64

	if err := r.db.WithContext(ctx).
		Model(&model.Playlist{}).
		Where("user_id = ? AND is_public = ?", userID, true).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := r.db.WithContext(ctx).
		Preload("User").
		Where("user_id = ? AND is_public = ?", userID, true).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&playlists).Error
	return playlists, total, err
}

// CountPublicByUser menghitung playlist publik milik user, dipakai statistik
// pada profil publik.
func (r *PlaylistRepository) CountPublicByUser(ctx context.Context, userID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.Playlist{}).
		Where("user_id = ? AND is_public = ?", userID, true).
		Count(&count).Error
	return count, err
}

// FindByIDs memuat sekumpulan playlist sekaligus, dipakai feed aktivitas.
func (r *PlaylistRepository) FindByIDs(ctx context.Context, ids []uint) ([]model.Playlist, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	var playlists []model.Playlist
	err := r.db.WithContext(ctx).
		Preload("User").
		Where("id IN ?", ids).
		Find(&playlists).Error
	return playlists, err
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

func (r *PlaylistRepository) FindMemberSongIDs(ctx context.Context, playlistID uint, ownerID uuid.UUID, songIDs []uint) ([]uint, error) {
	var playlist model.Playlist
	tx := r.db.WithContext(ctx)
	if err := tx.Select("id").Where("user_id = ?", ownerID).First(&playlist, playlistID).Error; err != nil {
		return nil, translateNotFound(err)
	}

	ids := make([]uint, 0)
	err := tx.Table(playlistSongsTable).Where("playlist_id = ? AND song_id IN ?", playlist.ID, songIDs).
		Pluck("song_id", &ids).Error
	return ids, err
}
