package jwt

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type TokenType string

const (
	TokenTypeAccess  TokenType = "access"
	TokenTypeRefresh TokenType = "refresh"
)

const signingMethod = "HS256"

const issuer = "spotify-backend"

var ErrInvalidToken = errors.New("invalid or expired token")

type Claims struct {
	Role      string    `json:"role"`
	TokenType TokenType `json:"typ"`

	jwt.RegisteredClaims
}

func (c *Claims) UserID() (uuid.UUID, error) {
	return uuid.Parse(c.Subject)
}

type Manager struct {
	accessSecret  []byte
	refreshSecret []byte
	accessTTL     time.Duration
	refreshTTL    time.Duration
}

func NewManager(accessSecret, refreshSecret string, accessTTL, refreshTTL time.Duration) *Manager {
	return &Manager{
		accessSecret:  []byte(accessSecret),
		refreshSecret: []byte(refreshSecret),
		accessTTL:     accessTTL,
		refreshTTL:    refreshTTL,
	}
}

func (m *Manager) AccessTTL() time.Duration  { return m.accessTTL }
func (m *Manager) RefreshTTL() time.Duration { return m.refreshTTL }

func (m *Manager) GenerateAccessToken(userID uuid.UUID, role string) (string, error) {
	return m.generate(m.accessSecret, m.accessTTL, TokenTypeAccess, userID, role)
}

func (m *Manager) GenerateRefreshToken(userID uuid.UUID, role string) (string, error) {
	return m.generate(m.refreshSecret, m.refreshTTL, TokenTypeRefresh, userID, role)
}

func (m *Manager) ParseAccessToken(raw string) (*Claims, error) {
	return m.parse(m.accessSecret, TokenTypeAccess, raw)
}

func (m *Manager) ParseRefreshToken(raw string) (*Claims, error) {
	return m.parse(m.refreshSecret, TokenTypeRefresh, raw)
}

func (m *Manager) generate(
	secret []byte,
	ttl time.Duration,
	typ TokenType,
	userID uuid.UUID,
	role string,
) (string, error) {
	jti, err := uuid.NewRandom()
	if err != nil {
		return "", fmt.Errorf("generate token id: %w", err)
	}

	now := time.Now()
	claims := Claims{
		Role:      role,
		TokenType: typ,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			ID:        jti.String(),
			Issuer:    issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}

	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
}

func (m *Manager) parse(secret []byte, want TokenType, raw string) (*Claims, error) {
	claims := &Claims{}

	_, err := jwt.ParseWithClaims(
		raw,
		claims,
		func(*jwt.Token) (any, error) { return secret, nil },
		jwt.WithValidMethods([]string{signingMethod}),
		jwt.WithIssuer(issuer),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, ErrInvalidToken
	}

	if claims.TokenType != want {
		return nil, ErrInvalidToken
	}

	if _, err := claims.UserID(); err != nil {
		return nil, ErrInvalidToken
	}

	return claims, nil
}
