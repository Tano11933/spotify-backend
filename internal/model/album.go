package model

import "time"

type Album struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Title       string    `json:"title" gorm:"not null" validate:"required,min=1,max=200"`
	CoverURL    string    `json:"cover_url" validate:"omitempty,url"`
	ReleaseDate time.Time `json:"release_date" validate:"required"`

	ArtistID uint    `json:"artist_id" gorm:"not null" validate:"required"`
	Artist   *Artist `json:"artist,omitempty" gorm:"foreignKey:ArtistID" validate:"-"`

	Songs []Song `json:"songs,omitempty" gorm:"foreignKey:AlbumID" validate:"-"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
