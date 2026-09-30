package model

import (
	"time"

	"github.com/google/uuid"
)

// Jenis aktivitas yang boleh muncul di feed. Hanya peristiwa PUBLIK yang
// dicatat; aktivitas privat (mis. playlist pribadi) tidak pernah masuk sini.
const (
	ActivitySongPlayed      = "song_played"
	ActivityPlaylistCreated = "playlist_created"
)

// Activity adalah satu peristiwa publik milik user untuk feed teman.
//
// Feed dibaca dengan cara JOIN ke user_follows saat request (read-time join),
// bukan disalin ke feed tiap pengikut saat peristiwa terjadi. Untuk skala
// demo, satu tabel + JOIN jauh lebih sederhana daripada fan-out on write, dan
// tidak ada data ganda yang bisa basi.
type Activity struct {
	ID     uint64    `json:"id" gorm:"primaryKey"`
	UserID uuid.UUID `json:"user_id" gorm:"type:uuid;not null;index"`

	Type string `json:"type" gorm:"not null;size:40"`

	// Salah satu dari dua ini terisi, tergantung jenis aktivitasnya.
	SongID     *uint `json:"song_id,omitempty"`
	PlaylistID *uint `json:"playlist_id,omitempty"`

	CreatedAt time.Time `json:"created_at" gorm:"not null;index"`
}
