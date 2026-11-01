package model

import (
	"github.com/google/uuid"
	"time"
)

type LoginSession struct {
	ID         uuid.UUID `json:"id"`
	UserAgent  string    `json:"user_agent"`
	IPAddress  string    `json:"ip_address"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsedAt time.Time `json:"last_used_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	Current    bool      `json:"current"`
}

type StoredLoginSession struct {
	Session        LoginSession
	RefreshHash    string `json:"-"`
	CredentialHash string `json:"-"`
}

type LoginDevice struct {
	UserAgent string
	IPAddress string
}
