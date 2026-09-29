package model

import (
	"time"

	"github.com/google/uuid"
)

// UserFollow adalah relasi follow antar user: FollowerID mengikuti FolloweeID.
//
// Composite primary key membuat operasi follow idempoten secara struktural,
// dan CHECK di migrasi menolak follow ke diri sendiri di level database.
type UserFollow struct {
	FollowerID uuid.UUID `json:"follower_id" gorm:"type:uuid;primaryKey"`
	FolloweeID uuid.UUID `json:"followee_id" gorm:"type:uuid;primaryKey"`
	CreatedAt  time.Time `json:"created_at" gorm:"not null"`
}
