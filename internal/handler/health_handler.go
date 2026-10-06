package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gofiber/adaptor/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// readinessTimeout membatasi pemeriksaan dependency. Kalau Postgres/Redis
// menggantung, /ready harus cepat menjawab "not ready", bukan ikut menggantung
// sampai orchestrator menyerah.
const readinessTimeout = 2 * time.Second

// HealthHandler menyajikan endpoint operasional: kesiapan dependency dan
// metrik Prometheus. Keduanya sengaja di luar prefix /api karena bukan bagian
// dari domain produk.
type HealthHandler struct {
	db      *gorm.DB
	redis   *redis.Client
	metrics http.Handler
}

func NewHealthHandler(db *gorm.DB, rdb *redis.Client, gatherer prometheus.Gatherer) *HealthHandler {
	return &HealthHandler{
		db:      db,
		redis:   rdb,
		metrics: promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{}),
	}
}

// Ready memeriksa Postgres dan Redis, dan membalas 503 kalau salah satu tidak
// bisa dijangkau. Berbeda dari /health yang hanya menandakan proses hidup,
// /ready menandakan proses SIAP melayani request.
func (h *HealthHandler) Ready(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.UserContext(), readinessTimeout)
	defer cancel()

	checks := fiber.Map{}
	ready := true

	if sqlDB, err := h.db.DB(); err != nil {
		checks["database"] = "error: " + err.Error()
		ready = false
	} else if err := sqlDB.PingContext(ctx); err != nil {
		checks["database"] = "error: " + err.Error()
		ready = false
	} else {
		checks["database"] = "ok"
	}

	if err := h.redis.Ping(ctx).Err(); err != nil {
		checks["redis"] = "error: " + err.Error()
		ready = false
	} else {
		checks["redis"] = "ok"
	}

	status := fiber.StatusOK
	state := "ready"
	if !ready {
		status = fiber.StatusServiceUnavailable
		state = "not ready"
	}

	return c.Status(status).JSON(fiber.Map{"status": state, "checks": checks})
}

// Metrics menyajikan format eksposisi Prometheus lewat adaptor net/http,
// karena promhttp adalah handler http.Handler sementara Fiber bukan.
func (h *HealthHandler) Metrics(c *fiber.Ctx) error {
	return adaptor.HTTPHandler(h.metrics)(c)
}
