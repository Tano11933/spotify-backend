package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"spotify-backend/internal/queue"
)

// recordSpans memasang tracer provider global yang merekam span, lalu
// memulihkan provider sebelumnya setelah test selesai.
func recordSpans(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()

	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})

	return recorder
}

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

func TestWorkerRecordsJobSpan(t *testing.T) {
	recorder := recordSpans(t)

	source := &fakeSource{jobs: []*queue.Job{{ID: "span-1", Type: "demo"}}}
	w := New(source)
	w.Handle("demo", func(context.Context, json.RawMessage) error { return nil })

	if _, err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("span tercatat %d, mau 1", len(spans))
	}
	if spans[0].Name() != "job demo" {
		t.Fatalf("nama span = %q, mau %q", spans[0].Name(), "job demo")
	}

	attrs := map[string]string{}
	for _, attr := range spans[0].Attributes() {
		attrs[string(attr.Key)] = attr.Value.Emit()
	}
	if attrs["job.id"] != "span-1" {
		t.Fatalf("atribut job.id = %q, mau span-1", attrs["job.id"])
	}
	if spans[0].Status().Code == codes.Error {
		t.Fatal("job sukses tidak boleh berstatus Error")
	}
}

func TestWorkerMarksFailedJobSpan(t *testing.T) {
	recorder := recordSpans(t)

	source := &fakeSource{jobs: []*queue.Job{{ID: "span-2", Type: "demo"}}}
	w := New(source)
	w.Handle("demo", func(context.Context, json.RawMessage) error { return errors.New("handler gagal") })

	if _, err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("span tercatat %d, mau 1", len(spans))
	}
	if spans[0].Status().Code != codes.Error {
		t.Fatalf("status span = %v, mau Error", spans[0].Status().Code)
	}
}
