// Package tracing menyiapkan OpenTelemetry untuk aplikasi.
//
// Default-nya MATI (`TRACING_ENABLED` tidak diisi): proyek ini harus bisa
// jalan tanpa collector, dan tracing yang tidak dikirim ke mana-mana hanya
// menambah overhead. Saat dinyalakan, ada dua exporter:
//
//   - `otlp` (default): kirim ke collector lewat OTLP/HTTP. Endpoint dibaca
//     dari `OTEL_EXPORTER_OTLP_ENDPOINT` (default http://localhost:4318).
//   - `stdout`: cetak span sebagai JSON ke stdout. Dipakai untuk demo tanpa
//     collector, atau saat memeriksa isi span secara manual.
package tracing

import (
	"context"
	"fmt"
	"os"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
)

// enabled membaca TRACING_ENABLED dengan nilai yang lazim dianggap benar.
func enabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("TRACING_ENABLED"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// Setup mengonfigurasi tracer provider global dan mengembalikan fungsi
// shutdown. Fungsi itu WAJIB dipanggil saat aplikasi berhenti: span yang masih
// ada di buffer batch harus dikirim dulu, kalau tidak jejaknya hilang.
//
// Nama service bisa ditimpa lewat OTEL_SERVICE_NAME.
func Setup(ctx context.Context, serviceName string) (func(context.Context) error, error) {
	if !enabled() {
		return func(context.Context) error { return nil }, nil
	}

	if override := os.Getenv("OTEL_SERVICE_NAME"); override != "" {
		serviceName = override
	}

	exporter, err := newExporter(ctx)
	if err != nil {
		return nil, err
	}

	res, err := resource.Merge(resource.Default(), resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceNameKey.String(serviceName),
	))
	if err != nil {
		return nil, fmt.Errorf("tracing: resource: %w", err)
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(provider)

	// TraceContext + Baggage: request id kita tetap header sendiri, tetapi
	// W3C traceparent ikut diteruskan kalau ada gateway/proxy di depan.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return provider.Shutdown, nil
}

func newExporter(ctx context.Context) (sdktrace.SpanExporter, error) {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("OTEL_TRACES_EXPORTER"))) {
	case "stdout":
		return stdouttrace.New(stdouttrace.WithPrettyPrint())

	case "", "otlp":
		endpoint := strings.TrimRight(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"), "/")
		if endpoint == "" {
			return otlptracehttp.New(ctx)
		}
		return otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint+"/v1/traces"))

	default:
		return nil, fmt.Errorf(
			"tracing: exporter %q tidak dikenal (pakai otlp atau stdout)",
			os.Getenv("OTEL_TRACES_EXPORTER"),
		)
	}
}
