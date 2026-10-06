package middleware

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"
)

// HTTPMetrics menyimpan kolektor metrik HTTP dan mendaftarkannya ke registry
// milik aplikasi.
type HTTPMetrics struct {
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

// NewHTTPMetrics membuat kolektor lalu mendaftarkannya ke registry yang
// diberikan. Registry per aplikasi (bukan global) dipakai supaya beberapa
// instance aplikasi dalam satu proses tidak panik karena mendaftarkan
// kolektor yang sama dua kali.
func NewHTTPMetrics(registry prometheus.Registerer) *HTTPMetrics {
	metrics := &HTTPMetrics{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "melodia",
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "Jumlah request HTTP berdasarkan method, route, dan status.",
		}, []string{"method", "route", "status"}),

		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "melodia",
			Subsystem: "http",
			Name:      "request_duration_seconds",
			Help:      "Durasi request HTTP dalam detik.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"method", "route"}),
	}

	registry.MustRegister(metrics.requests, metrics.duration)
	return metrics
}

// Middleware mencatat setiap request setelah selesai.
//
// Label route memakai TEMPLATE route (mis. /api/songs/:id), bukan path mentah.
// Bedanya penting: path mentah berisi id yang tak terbatas, dan setiap id
// membuat deret metrik baru yang tidak pernah dibaca siapa pun.
func (m *HTTPMetrics) Middleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()

		err := c.Next()

		route := c.Route().Path
		if route == "" {
			route = "unmatched"
		}

		m.requests.WithLabelValues(c.Method(), route, strconv.Itoa(c.Response().StatusCode())).Inc()
		m.duration.WithLabelValues(c.Method(), route).Observe(time.Since(start).Seconds())

		return err
	}
}
