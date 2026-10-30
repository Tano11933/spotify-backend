//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"spotify-backend/internal/repository"
	jwtpkg "spotify-backend/pkg/jwt"
)

func TestConcurrentRefreshHasOneSuccessor(t *testing.T) {
	id, _ := queueUser(t)
	ctx := context.Background()
	manager := jwtpkg.NewManager("integration-access-secret", "integration-refresh-secret", time.Minute, time.Hour)
	previous, err := manager.GenerateRefreshToken(id, "user")
	if err != nil {
		t.Fatal(err)
	}
	repo := repository.NewTokenRepository(testApp.Redis)
	if err := repo.StoreRefreshToken(ctx, id, previous, time.Hour); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.DeleteRefreshToken(ctx, id) })
	body, _ := json.Marshal(map[string]string{"refresh_token": previous})
	type result struct {
		status int
		token  string
		err    error
	}
	results := make(chan result, 8)
	start := make(chan struct{})
	var workers sync.WaitGroup
	for range 8 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			req := httptest.NewRequest("POST", "/api/auth/refresh", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			resp, err := testApp.Fiber.Test(req, -1)
			if err != nil {
				results <- result{err: err}
				return
			}
			defer resp.Body.Close()
			var payload struct {
				RefreshToken string `json:"refresh_token"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&payload)
			results <- result{status: resp.StatusCode, token: payload.RefreshToken}
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	successes := 0
	winner := ""
	for result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		switch result.status {
		case 200:
			successes++
			winner = result.token
		case 401:
		default:
			t.Fatalf("unexpected refresh status %d", result.status)
		}
	}
	if successes != 1 || winner == previous || winner == "" {
		t.Fatalf("successes=%d, successor missing or reused", successes)
	}
	stored, err := repo.GetRefreshToken(ctx, id)
	if err != nil || stored != winner {
		t.Fatal("winner was overwritten", err)
	}
	ttl, err := testApp.Redis.PTTL(ctx, fmt.Sprintf("refresh_token:%s", id)).Result()
	if err != nil || ttl < 59*time.Minute || ttl > time.Hour {
		t.Fatalf("rotation TTL = %v (%v)", ttl, err)
	}
	status, _ := do(t, "POST", "/api/auth/refresh", "", map[string]string{"refresh_token": previous})
	if status != 401 {
		t.Fatalf("previous refresh accepted: %d", status)
	}
	status, _ = do(t, "POST", "/api/auth/refresh", "", map[string]string{"refresh_token": winner})
	if status != 200 {
		t.Fatalf("winner rejected: %d", status)
	}
}

func TestRefreshCASDoesNotResurrectRevokedOrReplaceNewLogin(t *testing.T) {
	id, _ := queueUser(t)
	ctx := context.Background()
	repo := repository.NewTokenRepository(testApp.Redis)
	t.Cleanup(func() { _ = repo.DeleteRefreshToken(ctx, id) })
	if err := repo.StoreRefreshToken(ctx, id, "old", time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteRefreshToken(ctx, id); err != nil {
		t.Fatal(err)
	}
	ok, err := repo.RotateRefreshToken(ctx, id, "old", "resurrected", time.Hour)
	if err != nil || ok {
		t.Fatalf("revoked token rotated: %v %v", ok, err)
	}
	if err := repo.StoreRefreshToken(ctx, id, "new-login", time.Hour); err != nil {
		t.Fatal(err)
	}
	ok, err = repo.RotateRefreshToken(ctx, id, "old", "overwrite", time.Hour)
	if err != nil || ok {
		t.Fatalf("old token replaced new login: %v %v", ok, err)
	}
	got, err := repo.GetRefreshToken(ctx, id)
	if err != nil || got != "new-login" {
		t.Fatalf("new login lost: %v", err)
	}
	testApp.Redis.PExpire(ctx, fmt.Sprintf("refresh_token:%s", id), time.Millisecond)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if testApp.Redis.Exists(ctx, fmt.Sprintf("refresh_token:%s", id)).Val() == 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	ok, err = repo.RotateRefreshToken(ctx, id, "new-login", "expired", time.Hour)
	if err != nil || ok {
		t.Fatalf("expired token rotated: %v %v", ok, err)
	}
}
