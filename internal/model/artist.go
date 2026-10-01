package model

import "time"

type Artist struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Name      string    `json:"name" gorm:"not null" validate:"required,min=2,max=100"`
	Bio       string    `json:"bio" validate:"max=1000"`
	ImageURL  string    `json:"image_url" validate:"omitempty,url"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Genre hanya terisi kalau pemanggil memuatnya (Preload); nil dihilangkan
	// dari JSON oleh omitempty. constraint:OnDelete:CASCADE membuat baris
	// artist_genres ikut terhapus saat artist dihapus.
	Genres []Genre `json:"genres,omitempty" gorm:"many2many:artist_genres;constraint:OnDelete:CASCADE" validate:"-"`
}
