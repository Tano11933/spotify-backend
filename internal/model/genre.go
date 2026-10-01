package model

import "time"

// Genre adalah kategori musik. Artist punya banyak genre lewat tabel
// perantara artist_genres, dan genre inilah yang dipakai fitur rekomendasi
// ("Made for you") untuk menebak selera user dari riwayat putarnya.
type Genre struct {
	ID   uint   `json:"id" gorm:"primaryKey"`
	Name string `json:"name" gorm:"not null;uniqueIndex" validate:"required,min=2,max=50"`

	// Slug adalah bentuk nama yang aman untuk URL (mis. "indie-folk").
	// Dihitung server dari Name, jadi client tidak pernah mengirimnya.
	Slug string `json:"slug" gorm:"not null;uniqueIndex" validate:"-"`

	// Artists tidak ikut JSON genre: daftar artist di genre ini punya endpoint
	// sendiri. constraint:OnDelete:CASCADE membuat baris artist_genres ikut
	// terhapus saat genre dihapus.
	Artists []Artist `json:"-" gorm:"many2many:artist_genres;constraint:OnDelete:CASCADE" validate:"-"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
