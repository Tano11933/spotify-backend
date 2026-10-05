// Package queue menyediakan antrean job berbasis Redis untuk pekerjaan yang
// tidak boleh membebani request: kirim ulang email, refresh chart terjadwal,
// dan pekerjaan latar lain.
//
// Bentuknya sengaja sederhana: list Redis untuk job siap proses, sorted set
// untuk job yang sedang menunggu backoff, dan list terpisah untuk dead-letter.
// Semantiknya AT-MOST-ONCE: job yang sedang diproses saat worker mati tidak
// dikembalikan otomatis. Untuk kebutuhan proyek ini retry difokuskan pada
// kegagalan handler (SMTP mati, database sibuk), bukan crash recovery. Kalau
// nanti butuh at-least-once, tambahkan processing list + reaper.
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
	// MaxAttempts adalah jumlah percobaan maksimum sebelum job masuk
	// dead-letter. Percobaan pertama tidak dihitung sebagai retry.
	MaxAttempts = 5

	// BaseRetryDelay dikalikan dua setiap percobaan: 2s, 4s, 8s, 16s.
	BaseRetryDelay = 2 * time.Second

	// MaxRetryDelay membatasi backoff supaya job tidak tertunda berjam-jam.
	MaxRetryDelay = time.Minute
)

// ErrEmpty dikembalikan Reserve kalau tidak ada job dalam batas waktu tunggu.
var ErrEmpty = errors.New("queue is empty")

// Job adalah satu unit pekerjaan. Payload disimpan sebagai JSON mentah supaya
// queue tidak perlu tahu tipe payload masing-masing handler.
type Job struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	Payload    json.RawMessage `json:"payload"`
	Attempt    int             `json:"attempt"`
	EnqueuedAt time.Time       `json:"enqueued_at"`
	LastError  string          `json:"last_error,omitempty"`
}

type Queue struct {
	rdb  *redis.Client
	name string
}

func New(rdb *redis.Client, name string) *Queue {
	if name == "" {
		name = "jobs"
	}
	return &Queue{rdb: rdb, name: name}
}

func (q *Queue) readyKey() string   { return "queue:" + q.name }
func (q *Queue) delayedKey() string { return "queue:" + q.name + ":delayed" }
func (q *Queue) deadKey() string    { return "queue:" + q.name + ":dead" }

// Enqueue menambahkan job baru ke ujung antrean.
func (q *Queue) Enqueue(ctx context.Context, jobType string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("queue: marshal payload %s: %w", jobType, err)
	}

	job := Job{
		ID:         uuid.NewString(),
		Type:       jobType,
		Payload:    raw,
		EnqueuedAt: time.Now().UTC(),
	}
	return q.push(ctx, job)
}

func (q *Queue) push(ctx context.Context, job Job) error {
	raw, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("queue: marshal job %s: %w", job.Type, err)
	}
	return q.rdb.RPush(ctx, q.readyKey(), raw).Err()
}

// Reserve mengambil satu job siap proses. Job yang retry-nya belum jatuh tempo
// dipindahkan dulu dari sorted set ke list siap.
//
// BLPOP memblokir sampai ada job atau timeout habis; timeout pendek dipakai
// worker supaya ctx pembatalan (shutdown) tetap responsif.
func (q *Queue) Reserve(ctx context.Context, timeout time.Duration) (*Job, error) {
	if err := q.promoteDue(ctx); err != nil {
		return nil, err
	}

	result, err := q.rdb.BLPop(ctx, timeout, q.readyKey()).Result()
	if errors.Is(err, redis.Nil) {
		return nil, ErrEmpty
	}
	if err != nil {
		return nil, err
	}

	// Hasil BLPOP berbentuk [key, value].
	var job Job
	if err := json.Unmarshal([]byte(result[1]), &job); err != nil {
		return nil, fmt.Errorf("queue: job tidak bisa diurai: %w", err)
	}
	return &job, nil
}

// promoteDue memindahkan job yang waktu retry-nya sudah lewat ke list siap.
// ZREM dilakukan lebih dulu supaya dua worker tidak memproses job yang sama.
func (q *Queue) promoteDue(ctx context.Context) error {
	now := time.Now().UTC().UnixMilli()

	raws, err := q.rdb.ZRangeByScore(ctx, q.delayedKey(), &redis.ZRangeBy{
		Min:   "-inf",
		Max:   fmt.Sprintf("%d", now),
		Count: 10,
	}).Result()
	if err != nil {
		return err
	}

	for _, raw := range raws {
		removed, err := q.rdb.ZRem(ctx, q.delayedKey(), raw).Result()
		if err != nil {
			return err
		}
		if removed == 0 {
			continue // worker lain sudah mengambilnya
		}
		if err := q.rdb.RPush(ctx, q.readyKey(), raw).Err(); err != nil {
			return err
		}
	}
	return nil
}

// Retry menaikkan Attempt lalu menjadwalkan ulang job dengan backoff
// eksponensial. Kalau percobaannya sudah habis, job dipindahkan ke dead-letter
// supaya bisa diperiksa manual alih-alih berputar selamanya.
func (q *Queue) Retry(ctx context.Context, job *Job, cause error) error {
	job.Attempt++
	job.LastError = cause.Error()

	if job.Attempt >= MaxAttempts {
		return q.DeadLetter(ctx, job, cause)
	}

	raw, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("queue: marshal retry %s: %w", job.Type, err)
	}

	readyAt := time.Now().UTC().Add(retryDelay(job.Attempt)).UnixMilli()
	return q.rdb.ZAdd(ctx, q.delayedKey(), redis.Z{
		Score:  float64(readyAt),
		Member: raw,
	}).Err()
}

// DeadLetter menyimpan job yang menyerah ke list terpisah. Isinya sengaja
// tidak pernah dibaca otomatis; pemeriksaannya manual (mis. lewat redis-cli)
// atau lewat tooling yang menyusul.
func (q *Queue) DeadLetter(ctx context.Context, job *Job, cause error) error {
	job.LastError = cause.Error()

	raw, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("queue: marshal dead-letter %s: %w", job.Type, err)
	}
	return q.rdb.RPush(ctx, q.deadKey(), raw).Err()
}

// DeadLen mengembalikan jumlah job di dead-letter. Dipakai test dan nanti
// metrik operasional.
func (q *Queue) DeadLen(ctx context.Context) (int64, error) {
	return q.rdb.LLen(ctx, q.deadKey()).Result()
}

// retryDelay menghitung backoff: 2s, 4s, 8s, 16s, dibatasi MaxRetryDelay.
func retryDelay(attempt int) time.Duration {
	delay := BaseRetryDelay << (attempt - 1)
	if delay > MaxRetryDelay {
		return MaxRetryDelay
	}
	return delay
}
