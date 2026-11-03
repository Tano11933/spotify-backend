//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"spotify-backend/internal/model"
	"spotify-backend/internal/queue"
	"spotify-backend/internal/repository"
	"spotify-backend/internal/worker"
)

func TestWorkerKeepsLongRunningJobReserved(t *testing.T) {
	q := queue.NewWithLease(testApp.Redis, "test:"+t.Name(), 3*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := q.Enqueue(ctx, "slow", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	w := worker.New(q)
	w.Handle("slow", func(ctx context.Context, _ json.RawMessage) error {
		leases, err := testApp.Redis.ZRangeWithScores(ctx, queuePrefix(t)+":leases", 0, -1).Result()
		if err != nil {
			return err
		}
		if len(leases) != 1 {
			return fmt.Errorf("leases=%d", len(leases))
		}
		originalDeadline := leases[0].Score
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for {
			now, err := testApp.Redis.Time(ctx).Result()
			if err != nil {
				return err
			}
			if float64(now.UnixMilli()) > originalDeadline {
				n, err := q.RecoverExpired(ctx)
				if err != nil {
					return err
				}
				if n != 0 {
					return fmt.Errorf("live worker recovered %d jobs", n)
				}
				// A second worker cannot reserve the live handler's job.
				if _, err := q.Reserve(ctx, 10*time.Millisecond); !errors.Is(err, queue.ErrEmpty) {
					return fmt.Errorf("competing reserve: %v", err)
				}
				return nil
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
			}
		}
	})
	if done, err := w.RunOnce(ctx); err != nil || !done {
		t.Fatalf("long job=%v %v", done, err)
	}
	assertNoReservations(t)
}

func TestWorkerRecoversCrashAndAcknowledgesDelivery(t *testing.T) {
	q := testQueue(t)
	ctx := context.Background()
	old := enqueueReserve(t, q)
	expireReservation(t, old)
	// RunOnce itself must recover the expired reservation, without an external reaper.
	w := worker.New(q)
	calls := 0
	w.Handle("demo", func(context.Context, json.RawMessage) error { calls++; return nil })
	shortCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	if done, err := w.RunOnce(shortCtx); !errors.Is(err, context.DeadlineExceeded) || done {
		t.Fatalf("backoff RunOnce=%v %v", done, err)
	}
	promoteRetryNow(t)
	if done, err := w.RunOnce(ctx); err != nil || !done {
		t.Fatalf("recovered RunOnce=%v %v", done, err)
	}
	if calls != 1 {
		t.Fatalf("handler calls=%d", calls)
	}
	assertNoReservations(t)
}

func TestRepeatedChartJobsDoNotAccumulatePlays(t *testing.T) {
	q := testQueue(t)
	ctx := context.Background()
	history := model.PlayHistory{UserID: fixtureUserID, SongID: fixtureSongID, PlayedAt: time.Now().UTC()}
	if err := db.Create(&history).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Delete(&history).Error; err != nil {
			t.Error(err)
		}
	})
	charts := repository.NewChartRepository(db)
	w := worker.New(q)
	w.Handle(worker.JobChartRefresh, func(ctx context.Context, _ json.RawMessage) error { return charts.Refresh(ctx) })
	var baseline []repository.SongPlays
	for i := 0; i < 2; i++ {
		if err := q.Enqueue(ctx, worker.JobChartRefresh, map[string]any{}); err != nil {
			t.Fatal(err)
		}
		if done, err := w.RunOnce(ctx); err != nil || !done {
			t.Fatalf("chart=%v %v", done, err)
		}
		rows, _, err := charts.TopSongPlays(ctx, 100, 0)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			found := false
			for _, row := range rows {
				if row.SongID == fixtureSongID && row.Plays > 0 {
					found = true
				}
			}
			if !found {
				t.Fatal("chart did not include the test play")
			}
			baseline = rows
		} else if fmt.Sprint(rows) != fmt.Sprint(baseline) {
			t.Fatalf("refresh changed counts: %v -> %v", baseline, rows)
		}
	}
	assertNoReservations(t)
}
