package model

import "time"

type Song struct {
	ID       uint   `json:"id" gorm:"primaryKey"`
	Title    string `json:"title" gorm:"not null" validate:"required,min=1,max=200"`
	Duration int    `json:"duration" validate:"required,min=1"`
	FileURL  string `json:"file_url" validate:"omitempty,url"`

	ArtistID uint   `json:"artist_id" gorm:"not null" validate:"required"`
	Artist   Artist `json:"artist,omitempty" gorm:"foreignKey:ArtistID" validate:"-"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}