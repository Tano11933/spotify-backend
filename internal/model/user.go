package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Role string

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

type User struct {
	ID uuid.UUID `json:"id" gorm:"type:uuid;primaryKey"`

	Name  string `json:"name" gorm:"not null"`
	Email string `json:"email" gorm:"not null;uniqueIndex"`

	PasswordHash string `json:"-" gorm:"not null"`

	Role Role `json:"role" gorm:"type:varchar(20);not null;default:user"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (u *User) BeforeCreate(tx *gorm.DB) error {

	if u.ID != uuid.Nil {
		return nil
	}

	id, err := uuid.NewRandom()
	if err != nil {
		return err
	}

	u.ID = id
	return nil
}

func (u *User) IsAdmin() bool {
	return u.Role == RoleAdmin
}
