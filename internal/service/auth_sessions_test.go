package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"spotify-backend/internal/model"
)

func TestIndependentLoginsAndSessionLogout(t *testing.T) {
	f := newAuthFixture(t)
	user := f.register(t, "sessions@example.com", "rahasia123")
	ctx := context.Background()
	req := model.LoginRequest{Email: user.Email, Password: "rahasia123"}
	first, err := f.svc.Login(ctx, req, model.LoginDevice{UserAgent: "Browser\n" + strings.Repeat("a", 300), IPAddress: "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.svc.Login(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	firstClaims, _ := f.jwt.ParseRefreshToken(first.Tokens.RefreshToken)
	secondClaims, _ := f.jwt.ParseRefreshToken(second.Tokens.RefreshToken)
	a, _ := uuid.Parse(firstClaims.SessionID)
	b, _ := uuid.Parse(secondClaims.SessionID)
	if a == uuid.Nil || a == b {
		t.Fatal("logins share session")
	}
	stored, err := f.tokens.GetSession(ctx, user.ID, a)
	if err != nil {
		t.Fatal(err)
	}
	if len([]rune(stored.Session.UserAgent)) != 256 || strings.Contains(stored.Session.UserAgent, "\n") || stored.Session.IPAddress != "127.0.0.1" {
		t.Fatal("device hints not bounded")
	}
	if _, err := f.svc.Refresh(ctx, first.Tokens.RefreshToken); err != nil {
		t.Fatal("second login killed first", err)
	}
	if err := f.svc.Logout(ctx, user.ID, a); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Refresh(ctx, second.Tokens.RefreshToken); err != nil {
		t.Fatal("logout killed another session", err)
	}
}
func TestLegacyRefreshRequiresLogin(t *testing.T) {
	f := newAuthFixture(t)
	user := f.register(t, "legacy@example.com", "rahasia123")
	legacy, err := f.jwt.GenerateRefreshToken(user.ID, "user")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Refresh(context.Background(), legacy); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("legacy token accepted: %v", err)
	}
}
func TestResetFailureStillInvalidatesOldCredentialSessions(t *testing.T) {
	f := newAuthFixture(t)
	user := f.register(t, "reset-sessions@example.com", "rahasia123")
	ctx := context.Background()
	login, err := f.svc.Login(ctx, model.LoginRequest{Email: user.Email, Password: "rahasia123"})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.tokens.StoreResetToken(ctx, "reset", user.ID, 0); err != nil {
		t.Fatal(err)
	}
	f.tokens.revokeError = errors.New("redis unavailable")
	if err := f.svc.ResetPassword(ctx, model.ResetPasswordRequest{Token: "reset", NewPassword: "changed-password"}); err == nil {
		t.Fatal("reset reported successful revocation")
	}
	if _, err := f.svc.Refresh(ctx, login.Tokens.RefreshToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("old credential session accepted: %v", err)
	}
}
