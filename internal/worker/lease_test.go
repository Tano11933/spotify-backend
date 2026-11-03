package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"spotify-backend/internal/queue"
)

func TestWorkerRenewsLeaseWhileHandlerRuns(t *testing.T) {
	renewed := make(chan struct{}, 1)
	source := &fakeSource{jobs: []*queue.Job{{ID: "slow", Type: "demo", LeaseDuration: 30 * time.Millisecond}}}
	source.renew = func(context.Context, *queue.Job) error {
		select {
		case renewed <- struct{}{}:
		default:
		}
		return nil
	}
	w := New(source)
	w.Handle("demo", func(ctx context.Context, _ json.RawMessage) error {
		select {
		case <-renewed:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if processed, err := w.RunOnce(ctx); !processed || err != nil {
		t.Fatalf("RunOnce = %v, %v", processed, err)
	}
	if len(source.acked) != 1 {
		t.Fatal("renewed job was not acknowledged")
	}
}

func TestWorkerCancelsHandlerWhenLeaseCannotBeRenewed(t *testing.T) {
	for _, renewErr := range []error{queue.ErrReservationLost, errors.New("redis unavailable")} {
		t.Run(renewErr.Error(), func(t *testing.T) {
			source := &fakeSource{jobs: []*queue.Job{{ID: "lost", Type: "demo", LeaseDuration: 30 * time.Millisecond}}}
			source.renew = func(context.Context, *queue.Job) error { return renewErr }
			w := New(source)
			w.Handle("demo", func(ctx context.Context, _ json.RawMessage) error { <-ctx.Done(); return ctx.Err() })
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, err := w.RunOnce(ctx)
			if !errors.Is(err, renewErr) {
				t.Fatalf("RunOnce = %v, want %v", err, renewErr)
			}
			if len(source.acked)+len(source.retried)+len(source.dead) != 0 {
				t.Fatal("lost reservation must be left for recovery")
			}
		})
	}
}

func TestWorkerReturnsSettlementFailures(t *testing.T) {
	for _, mode := range []string{"ack", "retry", "dead"} {
		t.Run(mode, func(t *testing.T) {
			want := errors.New("redis settlement failed")
			source := &fakeSource{jobs: []*queue.Job{{ID: "settle", Type: "demo"}}, settleErr: want}
			w := New(source)
			if mode != "dead" {
				w.Handle("demo", func(context.Context, json.RawMessage) error {
					if mode == "retry" {
						return errors.New("smtp failed")
					}
					return nil
				})
			}
			if _, err := w.RunOnce(context.Background()); !errors.Is(err, want) {
				t.Fatalf("RunOnce = %v", err)
			}
		})
	}
}

func TestWorkerCancellationLeavesJobForRecovery(t *testing.T) {
	source := &fakeSource{jobs: []*queue.Job{{ID: "shutdown", Type: "demo"}}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := New(source)
	w.Handle("demo", func(context.Context, json.RawMessage) error { cancel(); return nil })
	if _, err := w.RunOnce(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("RunOnce = %v", err)
	}
	if len(source.acked)+len(source.retried)+len(source.dead) != 0 {
		t.Fatal("shutdown should leave the reservation recoverable")
	}
}
