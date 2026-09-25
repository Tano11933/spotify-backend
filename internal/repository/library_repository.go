package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"spotify-backend/internal/model"
)

// LibraryRepository menangani pustaka pribadi user: lagu disimpan, album
// disimpan, dan artist yang diikuti.
//
// Ketiganya digabung dalam satu repository karena polanya identik (composite
// key + urutan waktu) dan masing-masing hanya butuh beberapa query pendek —
// memecahnya jadi tiga file hanya menambah wiring tanpa manfaat.
type LibraryRepository struct {
	db *gorm.DB
}

func NewLibraryRepository(db *gorm.DB) *LibraryRepository {
	return &LibraryRepository{db: db}
}

/* -------------------------------------------------------------------------
 * Liked songs
 * ---------------------------------------------------------------------- */

// SaveTrack menyimpan lagu. ON CONFLICT DO NOTHING membuat pemanggilan kedua
// tidak error — idempoten, aman untuk retry UI.
func (r *LibraryRepository) SaveTrack(ctx context.Context, userID uuid.UUID, songID uint) error {
	entry := model.SavedTrack{UserID: userID, SongID: songID, SavedAt: time.Now().UTC()}

	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&entry).Error
}

func (r *LibraryRepository) RemoveTrack(ctx context.Context, userID uuid.UUID, songID uint) error {
	return r.db.WithContext(ctx).
		Where("user_id = ? AND song_id = ?", userID, songID).
		Delete(&model.SavedTrack{}).Error
}

func (r *LibraryRepository) FindSavedTrackIDs(ctx context.Context, userID uuid.UUID, songIDs []uint) ([]uint, error) {
	if len(songIDs) == 0 {
		return nil, nil
	}

	var ids []uint
	err := r.db.WithContext(ctx).
		Model(&model.SavedTrack{}).
		Where("user_id = ? AND song_id IN ?", userID, songIDs).
		Pluck("song_id", &ids).Error
	return ids, err
}

// FindTracks mengembalikan halaman lagu tersimpan, terbaru disimpan lebih dulu.
// Lagu di-JOIN ke saved_tracks supaya urutannya berasal dari saved_at milik
// user, bukan dari urutan tabel songs.
func (r *LibraryRepository) FindTracks(ctx context.Context, userID uuid.UUID, limit, offset int) ([]model.Song, int64, error) {
	var songs []model.Song
	var total int64

	if err := r.savedTracksQuery(ctx, userID).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := r.savedTracksQuery(ctx, userID).
		Preload("Artist").
		Preload("Album").
		Order("saved_tracks.saved_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&songs).Error
	return songs, total, err
}

func (r *LibraryRepository) savedTracksQuery(ctx context.Context, userID uuid.UUID) *gorm.DB {
	return r.db.WithContext(ctx).
		Model(&model.Song{}).
		Joins("JOIN saved_tracks ON saved_tracks.song_id = songs.id AND saved_tracks.user_id = ?", userID)
}

/* -------------------------------------------------------------------------
 * Saved albums
 * ---------------------------------------------------------------------- */

func (r *LibraryRepository) SaveAlbum(ctx context.Context, userID uuid.UUID, albumID uint) error {
	entry := model.SavedAlbum{UserID: userID, AlbumID: albumID, SavedAt: time.Now().UTC()}

	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&entry).Error
}

func (r *LibraryRepository) RemoveAlbum(ctx context.Context, userID uuid.UUID, albumID uint) error {
	return r.db.WithContext(ctx).
		Where("user_id = ? AND album_id = ?", userID, albumID).
		Delete(&model.SavedAlbum{}).Error
}

func (r *LibraryRepository) FindSavedAlbumIDs(ctx context.Context, userID uuid.UUID, albumIDs []uint) ([]uint, error) {
	if len(albumIDs) == 0 {
		return nil, nil
	}

	var ids []uint
	err := r.db.WithContext(ctx).
		Model(&model.SavedAlbum{}).
		Where("user_id = ? AND album_id IN ?", userID, albumIDs).
		Pluck("album_id", &ids).Error
	return ids, err
}

func (r *LibraryRepository) FindAlbums(ctx context.Context, userID uuid.UUID, limit, offset int) ([]model.Album, int64, error) {
	var albums []model.Album
	var total int64

	if err := r.savedAlbumsQuery(ctx, userID).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := r.savedAlbumsQuery(ctx, userID).
		Preload("Artist").
		Order("saved_albums.saved_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&albums).Error
	return albums, total, err
}

func (r *LibraryRepository) savedAlbumsQuery(ctx context.Context, userID uuid.UUID) *gorm.DB {
	return r.db.WithContext(ctx).
		Model(&model.Album{}).
		Joins("JOIN saved_albums ON saved_albums.album_id = albums.id AND saved_albums.user_id = ?", userID)
}

/* -------------------------------------------------------------------------
 * Followed artists
 * ---------------------------------------------------------------------- */

func (r *LibraryRepository) FollowArtist(ctx context.Context, userID uuid.UUID, artistID uint) error {
	entry := model.FollowedArtist{UserID: userID, ArtistID: artistID, FollowedAt: time.Now().UTC()}

	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&entry).Error
}

func (r *LibraryRepository) UnfollowArtist(ctx context.Context, userID uuid.UUID, artistID uint) error {
	return r.db.WithContext(ctx).
		Where("user_id = ? AND artist_id = ?", userID, artistID).
		Delete(&model.FollowedArtist{}).Error
}

func (r *LibraryRepository) FindFollowedArtistIDs(ctx context.Context, userID uuid.UUID, artistIDs []uint) ([]uint, error) {
	if len(artistIDs) == 0 {
		return nil, nil
	}

	var ids []uint
	err := r.db.WithContext(ctx).
		Model(&model.FollowedArtist{}).
		Where("user_id = ? AND artist_id IN ?", userID, artistIDs).
		Pluck("artist_id", &ids).Error
	return ids, err
}

func (r *LibraryRepository) FindFollowing(ctx context.Context, userID uuid.UUID, limit, offset int) ([]model.Artist, int64, error) {
	var artists []model.Artist
	var total int64

	if err := r.followedArtistsQuery(ctx, userID).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := r.followedArtistsQuery(ctx, userID).
		Order("followed_artists.followed_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&artists).Error
	return artists, total, err
}

func (r *LibraryRepository) followedArtistsQuery(ctx context.Context, userID uuid.UUID) *gorm.DB {
	return r.db.WithContext(ctx).
		Model(&model.Artist{}).
		Joins("JOIN followed_artists ON followed_artists.artist_id = artists.id AND followed_artists.user_id = ?", userID)
}
