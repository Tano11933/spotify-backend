package model

import (
	"time"

	"github.com/google/uuid"
)

// Jenis notifikasi in-app.
const (
	NotificationUserFollowed = "user_followed"
)

// Notification adalah notifikasi milik satu user. `ActorID` menunjuk siapa
// yang memicunya (mis. user yang mulai mengikuti), supaya penerima bisa
// menampilkan nama tanpa menyimpan teks yang bisa basi saat nama berubah.
type Notification struct {
	ID     uint64    `gorm:"primaryKey"`
	UserID uuid.UUID `gorm:"type:uuid;not null;index"`

	Type    string     `gorm:"not null;size:40"`
	ActorID *uuid.UUID `gorm:"type:uuid"`

	// NULL berarti belum dibaca; timestamp berarti sudah.
	ReadAt *time.Time `gorm:"index"`

	CreatedAt time.Time `gorm:"not null;index"`
}
