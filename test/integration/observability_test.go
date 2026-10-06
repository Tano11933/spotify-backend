//go:build integration

package integration

import (
	"io"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// doRaw mengirim request tanpa mengurai body, dan mengembalikan response utuh
// supaya header (X-Request-ID) dan body teks (/metrics) bisa diperiksa.
func doRaw(t *testing.T, method, path string, headers map[string]string) (int, string, map[string]string) {
	t.Helper()

	req := httptest.NewRequest(method, path, nil)
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := testApp.Fiber.Test(req, -1)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	responseHeaders := map[string]string{}
	for key := range resp.Header {
		responseHeaders[key] = resp.Header.Get(key)
	}

	return resp.StatusCode, string(raw), responseHeaders
}

// TestRequestIDMiddleware mengunci kontrak request id: selalu ada di response,
// id dari client dipakai ulang, dan id yang tidak masuk akal diganti.
func TestRequestIDMiddleware(t *testing.T) {
	status, _, headers := doRaw(t, "GET", "/health", nil)
	if status != 200 {
		t.Fatalf("/health = %d, mau 200", status)
	}
	if headers["X-Request-Id"] == "" {
		t.Fatal("response tidak membawa X-Request-ID")
	}

	status, _, headers = doRaw(t, "GET", "/health", map[string]string{"X-Request-ID": "trace-123"})
	if status != 200 || headers["X-Request-Id"] != "trace-123" {
		t.Fatalf("request id dari client tidak dipakai ulang: %v", headers["X-Request-Id"])
	}

	tooLong := strings.Repeat("a", 100)
	_, _, headers = doRaw(t, "GET", "/health", map[string]string{"X-Request-ID": tooLong})
	if headers["X-Request-Id"] == tooLong || headers["X-Request-Id"] == "" {
		t.Fatalf("request id terlalu panjang tidak diganti: %v", headers["X-Request-Id"])
	}
}

// TestReadinessEndpoint mengunci /ready: dependency sehat -> 200 ready, dan
// body memuat status tiap dependency.
func TestReadinessEndpoint(t *testing.T) {
	status, body := do(t, "GET", "/ready", "", nil)
	if status != 200 {
		t.Fatalf("/ready = %d %v, mau 200", status, body)
	}
	if body["status"] != "ready" {
		t.Fatalf("status = %v, mau ready", body["status"])
	}

	checks, ok := body["checks"].(map[string]any)
	if !ok {
		t.Fatalf("body tidak memuat checks: %v", body)
	}
	if checks["database"] != "ok" || checks["redis"] != "ok" {
		t.Fatalf("checks = %v, mau database & redis ok", checks)
	}
}

// TestMetricsEndpoint mengunci /metrics: format Prometheus, memuat metrik Go,
// dan counter request bertambah mengikuti template route (bukan path mentah).
func TestMetricsEndpoint(t *testing.T) {
	// Tiga request supaya counter /health pasti bertambah, berapa pun nilai
	// awalnya dari request test lain.
	for i := 0; i < 3; i++ {
		do(t, "GET", "/health", "", nil)
	}

	status, body, _ := doRaw(t, "GET", "/metrics", nil)
	if status != 200 {
		t.Fatalf("/metrics = %d, mau 200", status)
	}
	if !strings.Contains(body, "go_goroutines") {
		t.Fatal("metrik runtime Go tidak ikut terekspos")
	}

	pattern := regexp.MustCompile(`melodia_http_requests_total\{method="GET",route="/health",status="200"\} (\d+)`)
	match := pattern.FindStringSubmatch(body)
	if match == nil {
		t.Fatalf("counter /health tidak ditemukan di /metrics:\n%s", body)
	}

	count, err := strconv.Atoi(match[1])
	if err != nil {
		t.Fatalf("nilai counter tidak valid: %v", match[1])
	}
	if count < 3 {
		t.Fatalf("counter /health = %d, mau minimal 3", count)
	}

	if !strings.Contains(body, "melodia_http_request_duration_seconds") {
		t.Fatal("histogram durasi request tidak terekspos")
	}
}
