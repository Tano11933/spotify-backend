package model

import (
	"time"

	"github.com/google/uuid"
)

type Playlist struct {
	ID          uint   `json:"id" gorm:"primaryKey"`
	Name        string `json:"name" gorm:"not null"`
	Description string `json:"description"`

	IsPublic bool `json:"is_public" gorm:"not null;default:false"`

	UserID uuid.UUID `json:"user_id" gorm:"type:uuid;not null;index"`
	User   *User     `json:"user,omitempty" gorm:"foreignKey:UserID" validate:"-"`

	Songs []Song `json:"songs,omitempty" gorm:"many2many:playlist_songs" validate:"-"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreatePlaylistRequest struct {
	Name        string `json:"name" validate:"required,min=1,max=200"`
	Description string `json:"description" validate:"max=1000"`
	IsPublic    bool   `json:"is_public"`
}

type UpdatePlaylistRequest struct {
	Name        *string `json:"name" validate:"omitempty,min=1,max=200"`
	Description *string `json:"description" validate:"omitempty,max=1000"`
	IsPublic    *bool   `json:"is_public"`
}

type AddSongRequest struct {
	SongID uint `json:"song_id" validate:"required,min=1"`
}
