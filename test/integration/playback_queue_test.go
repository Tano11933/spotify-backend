//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"spotify-backend/internal/model"
	"spotify-backend/internal/repository"
	jwtpkg "spotify-backend/pkg/jwt"
)

func queueUser(t *testing.T) (uuid.UUID, string) {
	t.Helper()
	user := model.User{ID: uuid.New(), Name: "Queue fixture", Email: uuid.NewString() + "@queue.test", PasswordHash: "unused", Role: model.RoleUser}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Delete(&user) })
	manager := jwtpkg.NewManager("integration-access-secret", "integration-refresh-secret", time.Minute, time.Hour)
	token, err := manager.GenerateAccessToken(user.ID, string(user.Role))
	if err != nil {
		t.Fatal(err)
	}
	return user.ID, token
}

func TestPlaybackQueueReservations(t *testing.T) {
	userID, token := queueUser(t)
	otherID, other := queueUser(t)
	repo := repository.NewPlayerRepository(db)
	ctx := context.Background()
	for range 2 {
		if err := repo.AppendToQueue(ctx, userID, fixtureSongID); err != nil {
			t.Fatal(err)
		}
	}
	status, page := do(t, "GET", "/api/me/player/queue/entries", token, nil)
	if status != 200 || len(items(t, page)) != 2 {
		t.Fatalf("entries: %d %v", status, page)
	}
	firstID := uint64(items(t, page)[0]["id"].(float64))
	secondID := uint64(items(t, page)[1]["id"].(float64))
	if firstID == secondID {
		t.Fatal("duplicate songs need distinct queue entry ids")
	}
	first, err := repo.ReserveQueueEntry(ctx, userID, firstID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ReserveQueueEntry(ctx, userID, firstID); !errors.Is(err, repository.ErrQueueUnavailable) {
		t.Fatalf("double reservation: %v", err)
	}
	if err := repo.ConfirmQueueEntry(ctx, otherID, first.ID); !errors.Is(err, repository.ErrQueueUnavailable) {
		t.Fatalf("cross-user confirmation: %v", err)
	}
	status, _ = do(t, "POST", "/api/me/player/queue/reservations", other, map[string]any{"entry_id": firstID})
	if status != 409 {
		t.Fatalf("cross-user reserve: %d", status)
	}
	status, before := do(t, "GET", "/api/me/player/queue", token, nil)
	if status != 200 || before["total"].(float64) != 2 {
		t.Fatal("reservation must not consume an entry")
	}
	if err := repo.ReleaseQueueEntry(ctx, userID, first.ID); err != nil {
		t.Fatal(err)
	}
	first, err = repo.ReserveQueueEntry(ctx, userID, firstID)
	if err != nil {
		t.Fatal(err)
	}
	status, _ = do(t, "POST", fmt.Sprintf("/api/me/player/queue/reservations/%s/confirm", first.ID), token, nil)
	if status != 200 {
		t.Fatalf("confirm: %d", status)
	}
	if err := repo.ConfirmQueueEntry(ctx, userID, first.ID); !errors.Is(err, repository.ErrQueueUnavailable) {
		t.Fatalf("double consumption: %v", err)
	}
	_, after := do(t, "GET", "/api/me/player/queue/entries", token, nil)
	if len(items(t, after)) != 1 || uint64(items(t, after)[0]["id"].(float64)) != secondID {
		t.Fatal("confirmation removed the wrong occurrence")
	}
	status, _ = do(t, "DELETE", fmt.Sprintf("/api/me/player/queue/entries/%d", secondID), other, nil)
	if status != 200 {
		t.Fatalf("idempotent foreign deletion: %d", status)
	}
	_, after = do(t, "GET", "/api/me/player/queue/entries", token, nil)
	if after["total"].(float64) != 1 {
		t.Fatal("foreign deletion changed owner queue")
	}
	status, _ = do(t, "DELETE", fmt.Sprintf("/api/me/player/queue/entries/%d", secondID), token, nil)
	if status != 200 {
		t.Fatalf("remove own entry: %d", status)
	}
	status, empty := do(t, "POST", "/api/me/player/queue/reservations", token, map[string]any{})
	if status != 200 || empty != nil {
		t.Fatalf("empty reservation: %d %v", status, empty)
	}
}

func TestPlaybackQueueConcurrentReservations(t *testing.T) {
	userID, _ := queueUser(t)
	repo := repository.NewPlayerRepository(db)
	ctx := context.Background()
	const count = 8
	var group sync.WaitGroup
	errs := make(chan error, count)
	for range count {
		group.Go(func() { errs <- repo.AppendToQueue(ctx, userID, fixtureSongID) })
	}
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var rows []model.QueueItem
	if err := db.Where("user_id = ?", userID).Order("position ASC").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	for index, row := range rows {
		if row.Position != index+1 {
			t.Fatalf("positions are not serialized: %+v", rows)
		}
	}
	reservations := make(chan *model.QueueReservation, count)
	errs = make(chan error, count)
	for range count {
		group.Go(func() {
			reservation, err := repo.ReserveQueueEntry(ctx, userID, 0)
			reservations <- reservation
			errs <- err
		})
	}
	group.Wait()
	close(reservations)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	seen := make(map[uint64]bool)
	for reservation := range reservations {
		if reservation == nil || seen[reservation.EntryID] {
			t.Fatal("concurrent consumers selected the same entry")
		}
		seen[reservation.EntryID] = true
		if err := repo.ConfirmQueueEntry(ctx, userID, reservation.ID); err != nil {
			t.Fatal(err)
		}
	}
	if len(seen) != count {
		t.Fatalf("reserved %d, want %d", len(seen), count)
	}
}

func TestPlaybackQueueExpiredReservation(t *testing.T) {
	userID, token := queueUser(t)
	repo := repository.NewPlayerRepository(db)
	ctx := context.Background()
	if err := repo.AppendToQueue(ctx, userID, fixtureSongID); err != nil {
		t.Fatal(err)
	}
	old, err := repo.ReserveQueueEntry(ctx, userID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.QueueItem{}).Where("id = ?", old.EntryID).Update("reserved_until", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.ConfirmQueueEntry(ctx, userID, old.ID); !errors.Is(err, repository.ErrQueueUnavailable) {
		t.Fatalf("expired confirm: %v", err)
	}
	fresh, err := repo.ReserveQueueEntry(ctx, userID, 0)
	if err != nil || fresh == nil || fresh.ID == old.ID {
		t.Fatalf("expired reservation not recovered: %v", err)
	}
	if err := repo.ReleaseQueueEntry(ctx, userID, old.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.ConfirmQueueEntry(ctx, userID, fresh.ID); err != nil {
		t.Fatal("stale release cleared the new reservation", err)
	}
	for _, path := range []string{"/api/me/player/queue/entries/nope", "/api/me/player/queue/entries/0"} {
		status, _ := do(t, "DELETE", path, token, nil)
		if status != 400 {
			t.Fatalf("invalid id: %s %d", path, status)
		}
	}
	status, _ := do(t, "GET", "/api/me/player/queue/entries", "", nil)
	if status != 401 {
		t.Fatalf("unauthenticated queue entries: %d", status)
	}
}
