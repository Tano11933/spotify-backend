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
	if job.Attempt != 1 || job.ReservationID == "" || job.LeaseDuration != queue.DefaultLease {
		t.Fatalf("missing reservation: %+v", job)
	}
	if err := q.Ack(ctx, job); err != nil {
		t.Fatal(err)
	}

	// Antrean sudah kosong lagi setelah job diambil.
	if _, err := q.Reserve(ctx, 100*time.Millisecond); !errors.Is(err, queue.ErrEmpty) {
		t.Fatalf("reserve kedua = %v, mau ErrEmpty", err)
	}
}
