package model

import "time"

type Song struct {
	ID       uint   `json:"id" gorm:"primaryKey"`
	Title    string `json:"title" gorm:"not null" validate:"required,min=1,max=200"`
	Duration int    `json:"duration" validate:"required,min=1"`
	FileURL  string `json:"file_url" validate:"omitempty,url"`

	// AudioKey menunjuk berkas hasil unggahan di storage. Sengaja TIDAK
	// diekspos ke JSON: client selalu memakai GET /api/stream/songs/:id,
	// yang menerjemahkan key ini (atau fallback ke FileURL) di server.
	AudioKey string `json:"-" gorm:"column:audio_key"`

	// PlayCount bertambah setiap kali lagu mulai diputar. Nilainya bisa
	// tertinggal sedikit di halaman album karena cache detail album hidup
	// 5 menit — angka ini untuk analitik, bukan data kritikal.
	PlayCount int64 `json:"play_count" gorm:"not null;default:0"`

	ArtistID uint    `json:"artist_id" gorm:"not null" validate:"required"`
	Artist   *Artist `json:"artist,omitempty" gorm:"foreignKey:ArtistID" validate:"-"`

	AlbumID *uint  `json:"album_id" validate:"omitempty"`
	Album   *Album `json:"album,omitempty" gorm:"foreignKey:AlbumID" validate:"-"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
