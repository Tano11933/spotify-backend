//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestQueueDeadLetterReplayScript(t *testing.T) {
	rawScript, err := os.ReadFile("../../scripts/replay-dead-job.lua")
	if err != nil {
		t.Fatal(err)
	}
	script := redis.NewScript(string(rawScript))
	ctx := context.Background()
	q := testQueue(t)
	job := enqueueReserve(t, q)
	if err := q.DeadLetter(ctx, job, errors.New("operator must fix handler")); err != nil {
		t.Fatal(err)
	}
	keys := []string{queuePrefix(t) + ":dead", queuePrefix(t)}
	id, err := script.Run(ctx, testApp.Redis, keys).Text()
	if err != nil || id != job.ID {
		t.Fatalf("replay=%q %v", id, err)
	}
	replayed, err := q.Reserve(ctx, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.ID != job.ID || replayed.ReservationID == job.ReservationID || replayed.Attempt != 1 || replayed.LastError != "" || string(replayed.Payload) != string(job.Payload) {
		t.Fatalf("replayed=%+v", replayed)
	}
	if err := q.Ack(ctx, replayed); err != nil {
		t.Fatal(err)
	}
	if _, err := script.Run(ctx, testApp.Redis, keys).Result(); !errors.Is(err, redis.Nil) {
		t.Fatalf("empty replay=%v", err)
	}
	// A legacy dead-letter record is replayable without changing its payload.
	legacy := json.RawMessage(`{"id":"old-dead","type":"demo","attempt":5,"payload":[],"enqueued_at":"2026-10-01T00:00:00Z"}`)
	if err := testApp.Redis.RPush(ctx, keys[0], string(legacy)).Err(); err != nil {
		t.Fatal(err)
	}
	if _, err := script.Run(ctx, testApp.Redis, keys).Result(); err != nil {
		t.Fatal(err)
	}
	replayed, err = q.Reserve(ctx, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Attempt != 1 || string(replayed.Payload) != "[]" {
		t.Fatalf("legacy replay=%+v", replayed)
	}
	if err := q.Ack(ctx, replayed); err != nil {
		t.Fatal(err)
	}
	if err := testApp.Redis.RPush(ctx, keys[0], "broken JSON").Err(); err != nil {
		t.Fatal(err)
	}
	if _, err := script.Run(ctx, testApp.Redis, keys).Result(); err == nil {
		t.Fatal("invalid replay succeeded")
	}
	if count, err := q.DeadLen(ctx); err != nil || count != 1 {
		t.Fatalf("invalid dead job lost: %d %v", count, err)
	}
}
