package jwt

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)


const (
	accessSecret  = "access-secret-for-testing-only"
	refreshSecret = "refresh-secret-for-testing-only"
)

func newManager() *Manager {
	return NewManager(accessSecret, refreshSecret, 15*time.Minute, 7*24*time.Hour)
}

func TestAccessTokenRoundTrip(t *testing.T) {
	m := newManager()
	userID := uuid.New()

	token, err := m.GenerateAccessToken(userID, "admin")
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	claims, err := m.ParseAccessToken(token)
	if err != nil {
		t.Fatalf("ParseAccessToken: %v", err)
	}

	gotID, err := claims.UserID()
	if err != nil {
		t.Fatalf("claims.UserID: %v", err)
	}
	if gotID != userID {
		t.Errorf("user id = %s, ingin %s", gotID, userID)
	}
	if claims.Role != "admin" {
		t.Errorf("role = %q, ingin %q", claims.Role, "admin")
	}
	if claims.TokenType != TokenTypeAccess {
		t.Errorf("token type = %q, ingin %q", claims.TokenType, TokenTypeAccess)
	}
}

func TestRefreshTokenRejectedAsAccessToken(t *testing.T) {
	m := newManager()

	refresh, err := m.GenerateRefreshToken(uuid.New(), "user")
	if err != nil {
		t.Fatalf("GenerateRefreshToken: %v", err)
	}

	if _, err := m.ParseAccessToken(refresh); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("refresh token diterima sebagai access token, err = %v", err)
	}
}

func TestAccessTokenRejectedAsRefreshToken(t *testing.T) {
	m := newManager()

	access, err := m.GenerateAccessToken(uuid.New(), "user")
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	if _, err := m.ParseRefreshToken(access); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("access token diterima sebagai refresh token, err = %v", err)
	}
}

func TestTokenFromDifferentSecretRejected(t *testing.T) {
	attacker := NewManager("secret-yang-salah", "secret-yang-salah", time.Minute, time.Minute)

	token, err := attacker.GenerateAccessToken(uuid.New(), "admin")
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	if _, err := newManager().ParseAccessToken(token); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("token dari secret berbeda diterima, err = %v", err)
	}
}

func TestExpiredTokenRejected(t *testing.T) {
	// TTL negatif menghasilkan token yang sudah kedaluwarsa saat dibuat.
	m := NewManager(accessSecret, refreshSecret, -time.Minute, time.Hour)

	token, err := m.GenerateAccessToken(uuid.New(), "user")
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	if _, err := m.ParseAccessToken(token); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("token kedaluwarsa diterima, err = %v", err)
	}
}

func TestTamperedTokenRejected(t *testing.T) {
	m := newManager()

	token, err := m.GenerateAccessToken(uuid.New(), "user")
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token bukan 3 bagian: %d", len(parts))
	}
	payload := []byte(parts[1])
	if payload[0] == 'A' {
		payload[0] = 'B'
	} else {
		payload[0] = 'A'
	}
	tampered := parts[0] + "." + string(payload) + "." + parts[2]

	if _, err := m.ParseAccessToken(tampered); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("token yang diubah isinya diterima, err = %v", err)
	}
}

func TestUnsignedTokenRejected(t *testing.T) {
	claims := Claims{
		Role:      "admin",
		TokenType: TokenTypeAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   uuid.New().String(),
			Issuer:    issuer,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}

	unsigned, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims).
		SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("membuat token unsigned: %v", err)
	}

	if _, err := newManager().ParseAccessToken(unsigned); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("token tanpa signature (alg=none) diterima, err = %v", err)
	}
}

func TestGarbageTokenRejected(t *testing.T) {
	m := newManager()

	for _, raw := range []string{"", "bukan-jwt", "a.b.c", "....", "eyJhbGciOiJIUzI1NiJ9"} {
		if _, err := m.ParseAccessToken(raw); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("token sampah %q diterima, err = %v", raw, err)
		}
	}
}

func TestTokensAreUnique(t *testing.T) {
	m := newManager()
	userID := uuid.New()

	first, err := m.GenerateRefreshToken(userID, "user")
	if err != nil {
		t.Fatalf("GenerateRefreshToken: %v", err)
	}
	second, err := m.GenerateRefreshToken(userID, "user")
	if err != nil {
		t.Fatalf("GenerateRefreshToken: %v", err)
	}

	if first == second {
		t.Error("dua token untuk user yang sama identik — jti tidak berfungsi")
	}
}

func TestManagerExposesConfiguredTTL(t *testing.T) {
	m := NewManager(accessSecret, refreshSecret, 15*time.Minute, 168*time.Hour)

	if got := m.AccessTTL(); got != 15*time.Minute {
		t.Errorf("AccessTTL = %s, ingin 15m", got)
	}
	if got := m.RefreshTTL(); got != 168*time.Hour {
		t.Errorf("RefreshTTL = %s, ingin 168h", got)
	}
}
