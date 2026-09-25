package model

import (
	"time"

	"github.com/google/uuid"
)

// SavedTrack adalah lagu yang disimpan user (Liked Songs).
//
// Composite primary key (user_id, song_id) membuat operasi simpan idempoten
// secara struktural: tidak mungkin ada dua baris untuk pasangan yang sama,
// dan menyimpan dua kali cukup diabaikan (ON CONFLICT DO NOTHING).
type SavedTrack struct {
	UserID  uuid.UUID `json:"user_id" gorm:"type:uuid;primaryKey"`
	SongID  uint      `json:"song_id" gorm:"primaryKey"`
	SavedAt time.Time `json:"saved_at" gorm:"not null"`
}

// SavedAlbum adalah album yang disimpan user.
type SavedAlbum struct {
	UserID  uuid.UUID `json:"user_id" gorm:"type:uuid;primaryKey"`
	AlbumID uint      `json:"album_id" gorm:"primaryKey"`
	SavedAt time.Time `json:"saved_at" gorm:"not null"`
}

// FollowedArtist adalah artist yang diikuti user.
type FollowedArtist struct {
	UserID     uuid.UUID `json:"user_id" gorm:"type:uuid;primaryKey"`
	ArtistID   uint      `json:"artist_id" gorm:"primaryKey"`
	FollowedAt time.Time `json:"followed_at" gorm:"not null"`
}
