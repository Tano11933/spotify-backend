package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"
)

func TestMetricsKeepLabelsAfterRequestBuffersAreReused(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewHTTPMetrics(registry)
	app := fiber.New()
	app.Use(metrics.Middleware())
	app.All("/items/:id", func(c *fiber.Ctx) error { return c.SendStatus(200) })
	for range 40 {
		for _, method := range []string{"GET", "POST", "DELETE", "PATCH"} {
			response, err := app.Test(httptest.NewRequest(method, "/items/1", nil))
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
		}
	}
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() != "melodia_http_requests_total" {
			continue
		}
		if len(family.Metric) != 4 {
			t.Fatalf("method labels corrupted: %v", family)
		}
		for _, metric := range family.Metric {
			if metric.GetCounter().GetValue() != 40 {
				t.Fatalf("counter labels mutated: %v", metric)
			}
		}
	}
}
