package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"spotify-backend/internal/queue"
)

// fakeSource adalah JobSource in-memory: cukup untuk menguji logika worker
// tanpa Redis. Antreannya sendiri diuji dengan Redis sungguhan di test
// integrasi.
type fakeSource struct {
	jobs    []*queue.Job
	retried []retryCall
	dead    []*queue.Job
}

type retryCall struct {
	job   *queue.Job
	cause error
}

func (f *fakeSource) Reserve(context.Context, time.Duration) (*queue.Job, error) {
	if len(f.jobs) == 0 {
		return nil, queue.ErrEmpty
	}

	job := f.jobs[0]
	f.jobs = f.jobs[1:]
	return job, nil
}

func (f *fakeSource) Retry(_ context.Context, job *queue.Job, cause error) error {
	f.retried = append(f.retried, retryCall{job: job, cause: cause})
	return nil
}

func (f *fakeSource) DeadLetter(_ context.Context, job *queue.Job, cause error) error {
	f.dead = append(f.dead, job)
	return nil
}

func TestWorkerRunsRegisteredHandler(t *testing.T) {
	source := &fakeSource{jobs: []*queue.Job{
		{ID: "1", Type: "demo", Payload: json.RawMessage(`{"n":7}`)},
	}}

	w := New(source)
	var got map[string]int
	w.Handle("demo", func(_ context.Context, payload json.RawMessage) error {
		return json.Unmarshal(payload, &got)
	})

	processed, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if !processed {
		t.Fatal("RunOnce melaporkan tidak ada job, padahal ada")
	}
	if got["n"] != 7 {
		t.Fatalf("payload handler = %v, mau n=7", got)
	}
	if len(source.retried) != 0 || len(source.dead) != 0 {
		t.Fatalf("job sukses tidak boleh di-retry/dead-letter: %+v", source)
	}
}

func TestWorkerRetriesFailedHandler(t *testing.T) {
	source := &fakeSource{jobs: []*queue.Job{{ID: "2", Type: "demo"}}}

	w := New(source)
	handlerErr := errors.New("smtp down")
	w.Handle("demo", func(context.Context, json.RawMessage) error { return handlerErr })

	if _, err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if len(source.retried) != 1 {
		t.Fatalf("retry tercatat %d kali, mau 1", len(source.retried))
	}
	if !errors.Is(source.retried[0].cause, handlerErr) {
		t.Fatalf("cause retry = %v, mau %v", source.retried[0].cause, handlerErr)
	}
	if len(source.dead) != 0 {
		t.Fatalf("job gagal tidak boleh langsung dead-letter: %+v", source.dead)
	}
}

func TestWorkerDeadLettersUnknownJob(t *testing.T) {
	source := &fakeSource{jobs: []*queue.Job{{ID: "3", Type: "tidak-dikenal"}}}

	w := New(source)

	if _, err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if len(source.dead) != 1 {
		t.Fatalf("job tanpa handler harus dead-letter, tercatat %d", len(source.dead))
	}
	if len(source.retried) != 0 {
		t.Fatal("job tanpa handler tidak boleh di-retry")
	}
}

func TestWorkerRunOnceReturnsFalseWhenEmpty(t *testing.T) {
	w := New(&fakeSource{})

	processed, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if processed {
		t.Fatal("antrean kosong seharusnya menghasilkan processed=false")
	}
}
