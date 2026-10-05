//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"spotify-backend/internal/queue"
)

// testQueue memakai nama antrean unik per test supaya tidak bercampur dengan
// job milik test lain atau milik aplikasi yang berjalan.
func testQueue(t *testing.T) *queue.Queue {
	t.Helper()
	return queue.New(testApp.Redis, "test:"+t.Name())
}

func TestQueueEnqueueAndReserve(t *testing.T) {
	q := testQueue(t)
	ctx := context.Background()

	if err := q.Enqueue(ctx, "demo", map[string]any{"n": 7}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	job, err := q.Reserve(ctx, time.Second)
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if job.Type != "demo" {
		t.Fatalf("type = %q, mau demo", job.Type)
	}
	if job.ID == "" || job.EnqueuedAt.IsZero() {
		t.Fatalf("job tidak lengkap: %+v", job)
	}

	var payload map[string]int
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		t.Fatalf("payload tidak bisa diurai: %v", err)
	}
	if payload["n"] != 7 {
		t.Fatalf("payload = %v, mau n=7", payload)
	}

	// Antrean sudah kosong lagi setelah job diambil.
	if _, err := q.Reserve(ctx, 100*time.Millisecond); !errors.Is(err, queue.ErrEmpty) {
		t.Fatalf("reserve kedua = %v, mau ErrEmpty", err)
	}
}

func TestQueueRetryBackoffAndDeadLetter(t *testing.T) {
	q := testQueue(t)
	ctx := context.Background()

	// Retry pertama dijadwalkan dengan backoff, jadi belum bisa diambil.
	job := &queue.Job{ID: "retry-1", Type: "demo"}
	if err := q.Retry(ctx, job, errors.New("smtp down")); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if job.Attempt != 1 {
		t.Fatalf("attempt = %d, mau 1", job.Attempt)
	}
	if _, err := q.Reserve(ctx, 100*time.Millisecond); !errors.Is(err, queue.ErrEmpty) {
		t.Fatalf("job yang masih backoff ikut terambil: %v", err)
	}

	// Setelah backoff lewat, job muncul lagi dengan Attempt dan LastError.
	deadline := time.Now().Add(queue.BaseRetryDelay + 3*time.Second)
	var reserved *queue.Job
	var err error

	for time.Now().Before(deadline) {
		reserved, err = q.Reserve(ctx, 200*time.Millisecond)
		if err == nil {
			break
		}
		if !errors.Is(err, queue.ErrEmpty) {
			t.Fatalf("reserve: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}

	if reserved == nil {
		t.Fatal("job retry tidak pernah muncul setelah backoff")
	}
	if reserved.Attempt != 1 || reserved.LastError == "" {
		t.Fatalf("job retry = %+v, mau Attempt=1 dan LastError terisi", reserved)
	}

	// Percobaan terakhir langsung masuk dead-letter, bukan delayed.
	lastChance := &queue.Job{ID: "retry-2", Type: "demo", Attempt: queue.MaxAttempts - 1}
	if err := q.Retry(ctx, lastChance, errors.New("smtp masih down")); err != nil {
		t.Fatalf("retry terakhir: %v", err)
	}

	dead, err := q.DeadLen(ctx)
	if err != nil {
		t.Fatalf("dead len: %v", err)
	}
	if dead != 1 {
		t.Fatalf("dead-letter = %d job, mau 1", dead)
	}
	if _, err := q.Reserve(ctx, 100*time.Millisecond); !errors.Is(err, queue.ErrEmpty) {
		t.Fatalf("job dead-letter ikut kembali ke antrean: %v", err)
	}
}
