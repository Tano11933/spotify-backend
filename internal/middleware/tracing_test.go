package middleware

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
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

func spanAttributes(span sdktrace.ReadOnlySpan) map[string]string {
	attrs := map[string]string{}
	for _, attr := range span.Attributes() {
		attrs[string(attr.Key)] = attr.Value.Emit()
	}
	return attrs
}

func TestTracingRecordsRequestSpan(t *testing.T) {
	recorder := recordSpans(t)

	app := fiber.New()
	app.Use(RequestID())
	app.Use(Tracing())
	app.Get("/ping", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	req := httptest.NewRequest("GET", "/ping", nil)
	req.Header.Set(RequestIDHeader, "trace-me")

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	_ = resp.Body.Close()

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("span tercatat %d, mau 1", len(spans))
	}

	span := spans[0]
	if span.Name() != "GET /ping" {
		t.Fatalf("nama span = %q, mau %q", span.Name(), "GET /ping")
	}
	if span.SpanKind() != trace.SpanKindServer {
		t.Fatalf("span kind = %v, mau server", span.SpanKind())
	}

	attrs := spanAttributes(span)
	expected := map[string]string{
		"http.request.method":       "GET",
		"http.route":                "/ping",
		"http.response.status_code": "200",
		"request.id":                "trace-me",
	}
	for key, want := range expected {
		if attrs[key] != want {
			t.Errorf("atribut %s = %q, mau %q", key, attrs[key], want)
		}
	}
}

func TestTracingMarksServerErrors(t *testing.T) {
	recorder := recordSpans(t)

	app := fiber.New()
	app.Use(Tracing())
	app.Get("/boom", func(c *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusInternalServerError, "boom")
	})

	req := httptest.NewRequest("GET", "/boom", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	_ = resp.Body.Close()

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("span tercatat %d, mau 1", len(spans))
	}
	if spans[0].Status().Code != codes.Error {
		t.Fatalf("status span = %v, mau Error", spans[0].Status().Code)
	}
}
