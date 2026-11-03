// Package queue stores ready, reserved, delayed, and dead-letter jobs in Redis.
// Delivery is at-least-once while Redis retains the queue data.
package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	MaxAttempts    = 5
	BaseRetryDelay = 2 * time.Second
	MaxRetryDelay  = time.Minute
	DefaultLease   = time.Minute
	pollInterval   = 50 * time.Millisecond
)

var (
	ErrEmpty           = errors.New("queue is empty")
	ErrReservationLost = errors.New("queue reservation expired or no longer owned")
	ErrInvalidJob      = errors.New("queue job is invalid; moved to dead-letter")
)

type Job struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
	// Attempt counts deliveries, including reservations abandoned by a crash.
	Attempt       int           `json:"attempt"`
	EnqueuedAt    time.Time     `json:"enqueued_at"`
	LastError     string        `json:"last_error,omitempty"`
	ReservationID string        `json:"-"`
	LeaseDuration time.Duration `json:"-"`
}

type Queue struct {
	rdb   *redis.Client
	name  string
	lease time.Duration
}

func New(rdb *redis.Client, name string) *Queue {
	return NewWithLease(rdb, name, DefaultLease)
}

// NewWithLease permits shorter leases for tests and bounded job runtimes.
// Workers must renew their lease while a handler is still running.
func NewWithLease(rdb *redis.Client, name string, lease time.Duration) *Queue {
	if name == "" {
		name = "jobs"
	}
	if lease < 3*time.Millisecond {
		lease = DefaultLease
	}
	return &Queue{rdb: rdb, name: name, lease: lease}
}

func (q *Queue) readyKey() string      { return "queue:" + q.name }
func (q *Queue) delayedKey() string    { return q.readyKey() + ":delayed" }
func (q *Queue) deadKey() string       { return q.readyKey() + ":dead" }
func (q *Queue) processingKey() string { return q.readyKey() + ":processing" }
func (q *Queue) leasesKey() string     { return q.readyKey() + ":leases" }
func (q *Queue) keys() []string {
	return []string{q.readyKey(), q.processingKey(), q.leasesKey(), q.delayedKey(), q.deadKey()}
}

func (q *Queue) Enqueue(ctx context.Context, jobType string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("queue: marshal payload %s: %w", jobType, err)
	}
	job := Job{ID: uuid.NewString(), Type: jobType, Payload: raw, EnqueuedAt: time.Now().UTC()}
	raw, err = json.Marshal(job)
	if err != nil {
		return err
	}
	return q.rdb.RPush(ctx, q.readyKey(), raw).Err()
}

// Reserve polls so the ready -> processing transfer and lease assignment can
// share one Redis script. A blocking pop followed by HSET would lose crashed jobs.
// A nonpositive timeout waits until a job arrives or ctx is canceled.
func (q *Queue) Reserve(ctx context.Context, timeout time.Duration) (*Job, error) {
	var deadline time.Time
	if timeout > 0 {
		deadline = time.Now().Add(timeout)
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if _, err := q.RecoverExpired(ctx); err != nil {
			return nil, err
		}
		if err := promoteScript.Run(ctx, q.rdb, q.keys()).Err(); err != nil {
			return nil, err
		}
		reservationID := uuid.NewString()
		raw, err := reserveScript.Run(ctx, q.rdb, q.keys(), reservationID, q.lease.Milliseconds()).Text()
		if err == nil {
			if raw == "invalid" {
				return nil, ErrInvalidJob
			}
			var delivery struct {
				RawJob    string `json:"raw_job"`
				Attempt   int    `json:"attempt"`
				LastError string `json:"last_error"`
			}
			if err := json.Unmarshal([]byte(raw), &delivery); err != nil {
				return nil, err
			}
			var job Job
			if err := json.Unmarshal([]byte(delivery.RawJob), &job); err != nil {
				return nil, fmt.Errorf("queue: decode reserved job: %w", err)
			}
			job.Attempt = delivery.Attempt
			job.LastError = delivery.LastError
			job.ReservationID = reservationID
			job.LeaseDuration = q.lease
			return &job, nil
		}
		if !errors.Is(err, redis.Nil) {
			return nil, err
		}
		wait := pollInterval
		if !deadline.IsZero() {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				return nil, ErrEmpty
			}
			if remaining < wait {
				wait = remaining
			}
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

// Ack removes only the currently owned, unexpired reservation.
func (q *Queue) Ack(ctx context.Context, job *Job) error {
	return q.finish(ctx, job, "ack", "")
}

func (q *Queue) Renew(ctx context.Context, job *Job) error {
	return q.finish(ctx, job, "renew", "")
}

// Retry and DeadLetter read the stored job instead of trusting a mutated copy.
func (q *Queue) Retry(ctx context.Context, job *Job, cause error) error {
	return q.finish(ctx, job, "retry", cause.Error())
}

func (q *Queue) DeadLetter(ctx context.Context, job *Job, cause error) error {
	return q.finish(ctx, job, "dead", cause.Error())
}

func (q *Queue) finish(ctx context.Context, job *Job, action, message string) error {
	if job == nil || job.ReservationID == "" {
		return ErrReservationLost
	}
	n, err := finishScript.Run(ctx, q.rdb, q.keys(), job.ReservationID, action, message,
		q.lease.Milliseconds(), MaxAttempts, BaseRetryDelay.Milliseconds(), MaxRetryDelay.Milliseconds()).Int()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrReservationLost
	}
	return nil
}

func (q *Queue) DeadLen(ctx context.Context) (int64, error) {
	return q.rdb.LLen(ctx, q.deadKey()).Result()
}
