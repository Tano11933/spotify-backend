package model

import (
	"time"

	"github.com/google/uuid"
)

// PlayerState menyimpan posisi pemutaran terakhir user — satu baris per user
// (user_id sebagai primary key) supaya sinkronisasi lintas device cukup
// memakai upsert, tanpa khawatir duplikat.
type PlayerState struct {
	UserID          uuid.UUID `json:"user_id" gorm:"type:uuid;primaryKey"`
	SongID          uint      `json:"song_id" gorm:"not null"`
	PositionSeconds int       `json:"position_seconds" gorm:"not null;default:0"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// QueueItem adalah satu lagu dalam antrean "next up" milik user.
//
// Position dipakai untuk urutan; menyisipkan di tengah berarti menggeser
// posisi item sesudahnya (belum dibutuhkan sekarang — append sudah cukup).
type QueueItem struct {
	ID       uint64    `json:"id" gorm:"primaryKey"`
	UserID   uuid.UUID `json:"user_id" gorm:"type:uuid;not null;index"`
	SongID   uint      `json:"song_id" gorm:"not null"`
	Position int       `json:"position" gorm:"not null"`
	AddedAt  time.Time `json:"added_at" gorm:"not null"`
}

// PlayHistory mencatat setiap kali user MULAI memutar sebuah lagu.
//
// Tabel ini append-only dan bisa tumbuh cepat — untuk production, kebijakan
// retensi (mis. simpan 90 hari) perlu ditambahkan; di skala demo biarkan dulu.
type PlayHistory struct {
	ID       uint64    `json:"id" gorm:"primaryKey"`
	UserID   uuid.UUID `json:"user_id" gorm:"type:uuid;not null;index"`
	SongID   uint      `json:"song_id" gorm:"not null"`
	PlayedAt time.Time `json:"played_at" gorm:"not null;index"`
}
