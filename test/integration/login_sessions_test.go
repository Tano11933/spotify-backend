//go:build integration

package integration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"spotify-backend/internal/model"
	"spotify-backend/internal/repository"
	jwtpkg "spotify-backend/pkg/jwt"
)

func sessionDigest(raw string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(raw))) }
func storedSessionFixture(t *testing.T, userID uuid.UUID, ttl time.Duration) (string, string, uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	repo := repository.NewTokenRepository(testApp.Redis)
	manager := jwtpkg.NewManager("integration-access-secret", "integration-refresh-secret", time.Minute, time.Hour)
	sid := uuid.New()
	access, err := manager.GenerateAccessToken(userID, "user", sid)
	if err != nil {
		t.Fatal(err)
	}
	refresh, err := manager.GenerateRefreshToken(userID, "user", sid)
	if err != nil {
		t.Fatal(err)
	}
	var user model.User
	if err := db.First(&user, "id = ?", userID).Error; err != nil {
		t.Fatal(err)
	}
	generation, err := repo.SessionGeneration(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	ok, err := repo.CreateSession(ctx, userID, generation, model.StoredLoginSession{Session: model.LoginSession{ID: sid, UserAgent: "Fixture browser", CreatedAt: now, LastUsedAt: now, ExpiresAt: now.Add(ttl)}, RefreshHash: sessionDigest(refresh), CredentialHash: sessionDigest(user.PasswordHash)})
	if err != nil || !ok {
		t.Fatalf("create session: %v %v", ok, err)
	}
	t.Cleanup(func() { _ = repo.RevokeSession(ctx, userID, uuid.Nil, sid) })
	return access, refresh, sid
}
func loginDevice(t *testing.T, email, agent string) model.AuthResponse {
	t.Helper()
	raw, _ := json.Marshal(map[string]string{"email": email, "password": "Password123!"})
	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", agent)
	resp, err := testApp.Fiber.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("device login = %d", resp.StatusCode)
	}
	var result model.AuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}
func TestLoginSessionsIndependentAndOwned(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("Password123!"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	user := model.User{ID: uuid.New(), Name: "Sessions fixture", Email: uuid.NewString() + "@sessions.test", PasswordHash: string(hash), Role: model.RoleUser}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		repository.NewTokenRepository(testApp.Redis).RevokeAllSessions(context.Background(), user.ID, uuid.Nil)
		db.Delete(&user)
	})
	a := loginDevice(t, user.Email, "Mozilla Windows Chrome/130.0")
	b := loginDevice(t, user.Email, "Mozilla Android Firefox/130.0")
	manager := jwtpkg.NewManager("integration-access-secret", "integration-refresh-secret", time.Minute, time.Hour)
	ca, _ := manager.ParseRefreshToken(a.Tokens.RefreshToken)
	cb, _ := manager.ParseRefreshToken(b.Tokens.RefreshToken)
	if ca.SessionID == "" || ca.SessionID == cb.SessionID {
		t.Fatal("logins did not get independent session IDs")
	}
	status, rotatedA := do(t, "POST", "/api/auth/refresh", "", map[string]string{"refresh_token": a.Tokens.RefreshToken})
	if status != 200 {
		t.Fatalf("A refresh = %d", status)
	}
	status, rotatedB := do(t, "POST", "/api/auth/refresh", "", map[string]string{"refresh_token": b.Tokens.RefreshToken})
	if status != 200 {
		t.Fatalf("B refresh = %d", status)
	}
	status, list := do(t, "GET", "/api/auth/sessions", a.Tokens.AccessToken, nil)
	if status != 200 || len(items(t, list)) != 2 {
		t.Fatalf("sessions = %d %v", status, list)
	}
	current := 0
	for _, row := range items(t, list) {
		if row["current"] == true {
			current++
			if row["id"] != ca.SessionID {
				t.Fatal("wrong current marker")
			}
		}
		if row["created_at"] == "" || row["expires_at"] == "" {
			t.Fatal("missing session metadata")
		}
	}
	if current != 1 {
		t.Fatalf("current markers = %d", current)
	}
	raw, _ := json.Marshal(list)
	for _, secret := range []string{"refresh_hash", "credential_hash", a.Tokens.RefreshToken, b.Tokens.RefreshToken} {
		if bytes.Contains(raw, []byte(secret)) {
			t.Fatal("session response leaked secret")
		}
	}
	for _, path := range []string{"/api/auth/sessions/not-uuid", "/api/auth/sessions/00000000-0000-0000-0000-000000000000"} {
		status, _ := do(t, "DELETE", path, a.Tokens.AccessToken, nil)
		if status != 400 {
			t.Fatalf("invalid id = %d", status)
		}
	}
	status, _ = do(t, "GET", "/api/auth/sessions", "", nil)
	if status != 401 {
		t.Fatalf("anonymous sessions = %d", status)
	}
	otherID, _ := queueUser(t)
	other, otherRefresh, otherSID := storedSessionFixture(t, otherID, time.Hour)
	status, _ = do(t, "DELETE", "/api/auth/sessions/"+otherSID.String(), a.Tokens.AccessToken, nil)
	if status != 404 {
		t.Fatalf("foreign revoke = %d", status)
	}
	status, _ = do(t, "GET", "/api/auth/sessions", other, nil)
	if status != 200 {
		t.Fatal("foreign session affected")
	}
	status, _ = do(t, "DELETE", "/api/auth/sessions/"+cb.SessionID, a.Tokens.AccessToken, nil)
	if status != 200 {
		t.Fatalf("revoke B = %d", status)
	}
	status, _ = do(t, "POST", "/api/auth/refresh", "", map[string]string{"refresh_token": rotatedB["refresh_token"].(string)})
	if status != 401 {
		t.Fatal("revoked B still refreshes")
	}
	status, _ = do(t, "GET", "/api/auth/sessions", b.Tokens.AccessToken, nil)
	if status != 401 {
		t.Fatal("revoked B can manage sessions")
	}
	status, _ = do(t, "GET", "/api/auth/me", b.Tokens.AccessToken, nil)
	if status != 200 {
		t.Fatal("access-token expiry contract changed")
	}
	status, _ = do(t, "POST", "/api/auth/logout", a.Tokens.AccessToken, nil)
	if status != 200 {
		t.Fatalf("logout A = %d", status)
	}
	status, _ = do(t, "POST", "/api/auth/refresh", "", map[string]string{"refresh_token": rotatedA["refresh_token"].(string)})
	if status != 401 {
		t.Fatal("logout A did not revoke A")
	}
	status, _ = do(t, "POST", "/api/auth/refresh", "", map[string]string{"refresh_token": otherRefresh})
	if status != 200 {
		t.Fatal("another user lost session")
	}
}
func TestRevokeAllAndPasswordResetSessions(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewTokenRepository(testApp.Redis)
	id, _ := queueUser(t)
	a, refreshA, sidA := storedSessionFixture(t, id, time.Hour)
	_, refreshB, _ := storedSessionFixture(t, id, time.Hour)
	staleGeneration, err := repo.SessionGeneration(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := repo.GetSession(ctx, id, sidA)
	if err != nil {
		t.Fatal(err)
	}
	status, _ := do(t, "DELETE", "/api/auth/sessions", a, nil)
	if status != 200 {
		t.Fatalf("revoke all = %d", status)
	}
	for _, token := range []string{refreshA, refreshB} {
		status, _ := do(t, "POST", "/api/auth/refresh", "", map[string]string{"refresh_token": token})
		if status != 401 {
			t.Fatal("revoked-all session refreshed")
		}
	}
	ok, err := repo.CreateSession(ctx, id, staleGeneration, *stored)
	if err != nil || ok {
		t.Fatalf("stale login survived revoke-all = %v %v", ok, err)
	}
	_, freshA, _ := storedSessionFixture(t, id, time.Hour)
	_, freshB, _ := storedSessionFixture(t, id, time.Hour)
	reset := uuid.NewString()
	if err := repo.StoreResetToken(ctx, reset, id, time.Minute); err != nil {
		t.Fatal(err)
	}
	status, _ = do(t, "POST", "/api/auth/reset-password", "", map[string]string{"token": reset, "new_password": "Replacement123!"})
	if status != 200 {
		t.Fatalf("password reset = %d", status)
	}
	for _, token := range []string{freshA, freshB} {
		status, _ := do(t, "POST", "/api/auth/refresh", "", map[string]string{"refresh_token": token})
		if status != 401 {
			t.Fatal("reset-password session refreshed")
		}
	}
}
func TestLegacyRefreshRequiresLoginAndSessionExpiry(t *testing.T) {
	id, legacyAccess := queueUser(t)
	ctx := context.Background()
	manager := jwtpkg.NewManager("integration-access-secret", "integration-refresh-secret", time.Minute, time.Hour)
	legacy, err := manager.GenerateRefreshToken(id, "user")
	if err != nil {
		t.Fatal(err)
	}
	legacyKey := fmt.Sprintf("refresh_token:%s", id)
	testApp.Redis.Set(ctx, legacyKey, legacy, time.Hour)
	t.Cleanup(func() { testApp.Redis.Del(ctx, legacyKey) })
	status, _ := do(t, "POST", "/api/auth/refresh", "", map[string]string{"refresh_token": legacy})
	if status != 401 {
		t.Fatalf("legacy refresh = %d", status)
	}
	status, _ = do(t, "GET", "/api/auth/sessions", legacyAccess, nil)
	if status != 401 {
		t.Fatalf("legacy management = %d", status)
	}
	a, refreshA, _ := storedSessionFixture(t, id, 10*time.Millisecond)
	_, refreshB, sidB := storedSessionFixture(t, id, time.Hour)
	repo := repository.NewTokenRepository(testApp.Redis)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		status, _ = do(t, "GET", "/api/auth/sessions", a, nil)
		if status == 401 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if status != 401 {
		t.Fatal("expired session can manage account")
	}
	status, _ = do(t, "POST", "/api/auth/refresh", "", map[string]string{"refresh_token": refreshA})
	if status != 401 {
		t.Fatal("expired session refreshed")
	}
	stored, err := repo.GetSession(ctx, id, sidB)
	if err != nil || stored.RefreshHash != sessionDigest(refreshB) {
		t.Fatal("short session expired longer session", err)
	}
	raw, err := testApp.Redis.HGetAll(ctx, fmt.Sprintf("auth:{%s}:sessions", id)).Result()
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range raw {
		if strings.Contains(value, refreshB) {
			t.Fatal("plaintext refresh stored")
		}
	}
}
