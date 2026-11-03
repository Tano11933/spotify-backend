//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"spotify-backend/internal/queue"
)

func queuePrefix(t *testing.T) string { return "queue:test:" + t.Name() }

func enqueueReserve(t *testing.T, q *queue.Queue) *queue.Job {
	t.Helper()
	if err := q.Enqueue(context.Background(), "demo", map[string]int{"n": 7}); err != nil {
		t.Fatal(err)
	}
	job, err := q.Reserve(context.Background(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return job
}

// Change only this test's Redis deadline; no sleeps or production clock hooks.
func expireReservation(t *testing.T, job *queue.Job) {
	t.Helper()
	if err := testApp.Redis.ZAdd(context.Background(), queuePrefix(t)+":leases", redis.Z{Score: 0, Member: job.ReservationID}).Err(); err != nil {
		t.Fatal(err)
	}
}

func promoteRetryNow(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	key := queuePrefix(t) + ":delayed"
	raws, err := testApp.Redis.ZRange(ctx, key, 0, -1).Result()
	if err != nil {
		t.Fatal(err)
	}
	if len(raws) != 1 {
		t.Fatalf("delayed jobs = %d, want 1", len(raws))
	}
	if err := testApp.Redis.ZAdd(ctx, key, redis.Z{Score: 0, Member: raws[0]}).Err(); err != nil {
		t.Fatal(err)
	}
}

func assertNoReservations(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	processing, err := testApp.Redis.HLen(ctx, queuePrefix(t)+":processing").Result()
	if err != nil {
		t.Fatal(err)
	}
	leases, err := testApp.Redis.ZCard(ctx, queuePrefix(t)+":leases").Result()
	if err != nil {
		t.Fatal(err)
	}
	if processing != 0 || leases != 0 {
		t.Fatalf("processing=%d leases=%d", processing, leases)
	}
}

func TestQueueCrashRecoveryAndStaleWorker(t *testing.T) {
	q := testQueue(t)
	ctx := context.Background()
	old := enqueueReserve(t, q)
	expireReservation(t, old)
	// Expiry alone is enough to reject a stale owner, before the reaper runs.
	if err := q.Ack(ctx, old); !errors.Is(err, queue.ErrReservationLost) {
		t.Fatalf("expired ack = %v", err)
	}
	n, err := q.RecoverExpired(ctx)
	if err != nil || n != 1 {
		t.Fatalf("recover=%d, %v", n, err)
	}
	if n, err := q.RecoverExpired(ctx); err != nil || n != 0 {
		t.Fatalf("second recovery=%d, %v", n, err)
	}
	assertNoReservations(t)
	promoteRetryNow(t)
	current, err := q.Reserve(ctx, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if current.ID != old.ID || current.ReservationID == old.ReservationID || current.Attempt != 2 || current.LastError != "processing lease expired" {
		t.Fatalf("recovered=%+v", current)
	}
	for name, action := range map[string]func(context.Context, *queue.Job) error{
		"ack": q.Ack, "renew": q.Renew,
		"retry": func(ctx context.Context, j *queue.Job) error { return q.Retry(ctx, j, errors.New("late")) },
		"dead":  func(ctx context.Context, j *queue.Job) error { return q.DeadLetter(ctx, j, errors.New("late")) },
	} {
		if err := action(ctx, old); !errors.Is(err, queue.ErrReservationLost) {
			t.Fatalf("stale %s=%v", name, err)
		}
	}
	if err := q.Ack(ctx, current); err != nil {
		t.Fatal(err)
	}
	if err := q.Ack(ctx, current); !errors.Is(err, queue.ErrReservationLost) {
		t.Fatalf("duplicate ack=%v", err)
	}
	assertNoReservations(t)
}

func TestQueueRetryBackoffAndDeadLetter(t *testing.T) {
	q := testQueue(t)
	ctx := context.Background()
	job := enqueueReserve(t, q)
	for attempt := 1; attempt <= queue.MaxAttempts; attempt++ {
		if job.Attempt != attempt {
			t.Fatalf("attempt=%d want %d", job.Attempt, attempt)
		}
		if err := q.Retry(ctx, job, errors.New("smtp down")); err != nil {
			t.Fatal(err)
		}
		assertNoReservations(t)
		if attempt == queue.MaxAttempts {
			break
		}
		if _, err := q.Reserve(ctx, 10*time.Millisecond); !errors.Is(err, queue.ErrEmpty) {
			t.Fatalf("backoff reserve=%v", err)
		}
		entries, err := testApp.Redis.ZRangeWithScores(ctx, queuePrefix(t)+":delayed", 0, -1).Result()
		if err != nil || len(entries) != 1 {
			t.Fatalf("delayed=%v err=%v", entries, err)
		}
		now, err := testApp.Redis.Time(ctx).Result()
		if err != nil {
			t.Fatal(err)
		}
		want := queue.BaseRetryDelay * time.Duration(1<<(attempt-1))
		remaining := time.UnixMilli(int64(entries[0].Score)).Sub(now)
		if remaining > want || remaining < want-time.Second {
			t.Fatalf("delay=%v want near %v", remaining, want)
		}
		promoteRetryNow(t)
		job, err = q.Reserve(ctx, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if job.LastError != "smtp down" {
			t.Fatalf("last_error=%q", job.LastError)
		}
	}
	if dead, err := q.DeadLen(ctx); err != nil || dead != 1 {
		t.Fatalf("dead=%d err=%v", dead, err)
	}
	if _, err := q.Reserve(ctx, 10*time.Millisecond); !errors.Is(err, queue.ErrEmpty) {
		t.Fatalf("dead-letter returned=%v", err)
	}
}

func TestQueueRepeatedCrashesReachDeadLetter(t *testing.T) {
	q := testQueue(t)
	ctx := context.Background()
	job := enqueueReserve(t, q)
	for attempt := 1; attempt <= queue.MaxAttempts; attempt++ {
		expireReservation(t, job)
		if n, err := q.RecoverExpired(ctx); err != nil || n != 1 {
			t.Fatalf("recover=%d %v", n, err)
		}
		if attempt == queue.MaxAttempts {
			break
		}
		promoteRetryNow(t)
		var err error
		job, err = q.Reserve(ctx, time.Second)
		if err != nil {
			t.Fatal(err)
		}
	}
	if dead, err := q.DeadLen(ctx); err != nil || dead != 1 {
		t.Fatalf("dead=%d %v", dead, err)
	}
	assertNoReservations(t)
}

func TestQueueConcurrentReservationsAndRecovery(t *testing.T) {
	q := testQueue(t)
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		if err := q.Enqueue(ctx, "demo", i); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	jobs := make(chan *queue.Job, 20)
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			j, err := q.Reserve(ctx, time.Second)
			if err != nil {
				errs <- err
				return
			}
			jobs <- j
		}()
	}
	wg.Wait()
	close(jobs)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for j := range jobs {
		if ids[j.ID] {
			t.Fatal("same job reserved twice")
		}
		ids[j.ID] = true
		expireReservation(t, j)
	}
	if len(ids) != 20 {
		t.Fatalf("reserved=%d", len(ids))
	}
	counts := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n, err := q.RecoverExpired(ctx)
			if err != nil {
				counts <- -100
				return
			}
			counts <- n
		}()
	}
	wg.Wait()
	close(counts)
	total := 0
	for n := range counts {
		total += n
	}
	if total != 20 {
		t.Fatalf("recovered=%d", total)
	}
	if n, err := testApp.Redis.ZCard(ctx, queuePrefix(t)+":delayed").Result(); err != nil || n != 20 {
		t.Fatalf("delayed=%d %v", n, err)
	}
	assertNoReservations(t)
}

func TestQueueRenewAndCancellation(t *testing.T) {
	q := testQueue(t)
	ctx := context.Background()
	job := enqueueReserve(t, q)
	// Bring expiry close enough that a renewal must increase the deadline.
	now, err := testApp.Redis.Time(ctx).Result()
	if err != nil {
		t.Fatal(err)
	}
	if err := testApp.Redis.ZAdd(ctx, queuePrefix(t)+":leases", redis.Z{Score: float64(now.Add(10 * time.Second).UnixMilli()), Member: job.ReservationID}).Err(); err != nil {
		t.Fatal(err)
	}
	if err := q.Renew(ctx, job); err != nil {
		t.Fatal(err)
	}
	score, err := testApp.Redis.ZScore(ctx, queuePrefix(t)+":leases", job.ReservationID).Result()
	if err != nil {
		t.Fatal(err)
	}
	if score < float64(now.Add(50*time.Second).UnixMilli()) {
		t.Fatal("lease not extended")
	}
	if err := q.Ack(ctx, job); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := q.Reserve(canceled, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("reserve canceled=%v", err)
	}
	assertNoReservations(t)
}

func TestQueuePreservesPayloadAcrossRedelivery(t *testing.T) {
	q := testQueue(t)
	ctx := context.Background()
	payload := json.RawMessage(`{"large":9007199254740993,"empty":[],"object":{},"text":"café"}`)
	if err := q.Enqueue(ctx, "demo", payload); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		job, err := q.Reserve(ctx, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if string(job.Payload) != string(payload) {
			t.Fatalf("payload changed: %s", job.Payload)
		}
		if i == 0 {
			if err := q.Retry(ctx, job, errors.New("retry")); err != nil {
				t.Fatal(err)
			}
			promoteRetryNow(t)
		} else {
			if err := q.Ack(ctx, job); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestQueueQuarantinesMalformedJobs(t *testing.T) {
	q := testQueue(t)
	ctx := context.Background()
	if err := testApp.Redis.RPush(ctx, queuePrefix(t), "broken JSON").Err(); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Reserve(ctx, time.Second); !errors.Is(err, queue.ErrInvalidJob) {
		t.Fatalf("invalid=%v", err)
	}
	if n, err := q.DeadLen(ctx); err != nil || n != 1 {
		t.Fatalf("dead=%d %v", n, err)
	}
	job := enqueueReserve(t, q)
	if err := q.Ack(ctx, job); err != nil {
		t.Fatal(err)
	}
}

func TestQueueLegacyReadyAndDelayedJobs(t *testing.T) {
	q := testQueue(t)
	ctx := context.Background()
	for _, state := range []string{"ready", "delayed"} {
		raw := fmt.Sprintf(`{"id":"legacy-%s","type":"demo","payload":{"old":true},"attempt":1,"enqueued_at":"2026-10-01T00:00:00Z","last_error":"old retry"}`, state)
		var err error
		if state == "ready" {
			err = testApp.Redis.RPush(ctx, queuePrefix(t), raw).Err()
		} else {
			err = testApp.Redis.ZAdd(ctx, queuePrefix(t)+":delayed", redis.Z{Score: 0, Member: raw}).Err()
		}
		if err != nil {
			t.Fatal(err)
		}
		j, err := q.Reserve(ctx, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(j.ID, "legacy-") || j.Attempt != 2 || j.LastError != "old retry" {
			t.Fatalf("legacy=%+v", j)
		}
		if err := q.Ack(ctx, j); err != nil {
			t.Fatal(err)
		}
	}
}
