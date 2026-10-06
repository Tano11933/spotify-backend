package middleware

import (
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
)

// Logger mencatat satu baris terstruktur per request SETELAH selesai, jadi
// status dan durasinya sudah final. Dipasang setelah RequestID supaya id-nya
// selalu ada, dan sebelum route supaya semua request (termasuk 404) tercatat.
//
// Level mengikuti status: 5xx error, 4xx warn, sisanya info. Ini yang membuat
// agregator log bisa membedakan "client salah" dari "server rusak".
func Logger() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()

		// c.Next() menjalankan handler berikutnya sampai selesai; error tidak
		// ditelan, hanya dicatat, supaya Fiber tetap menanganinya.
		err := c.Next()

		attrs := []any{
			"method", c.Method(),
			"path", c.Path(),
			"status", c.Response().StatusCode(),
			"duration_ms", time.Since(start).Milliseconds(),
			"ip", c.IP(),
		}

		if requestID, ok := RequestIDFromContext(c); ok {
			attrs = append(attrs, "request_id", requestID)
		}
		if userID, ok := UserIDFromContext(c); ok {
			attrs = append(attrs, "user_id", userID.String())
		}

		level := slog.LevelInfo
		switch status := c.Response().StatusCode(); {
		case status >= 500:
			level = slog.LevelError
		case status >= 400:
			level = slog.LevelWarn
		}

		slog.Log(c.UserContext(), level, "http request", attrs...)
		return err
	}
}
