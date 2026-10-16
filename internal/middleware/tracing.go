package middleware

import (
	"errors"
	"fmt"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// tracerScope adalah nama instrumentasi untuk span HTTP.
const tracerScope = "spotify-backend/http"

// Tracing memulai satu span server per request dan menutupnya setelah handler
// selesai. Kalau tracing tidak dikonfigurasi, otel.Tracer mengembalikan no-op
// tracer dan biayanya nyaris nol; middleware ini tidak perlu tahu.
//
// Span membawa atribut mengikuti semantic conventions OTel
// (`http.request.method`, `http.route`, `http.response.status_code`) plus
// `request.id` dan `user.id` supaya jejaknya bisa disambung ke log terstruktur.
//
// Dipasang SETELAH RequestID dan SEBELUM Logger: request id sudah tersedia,
// dan logger bisa membaca trace id dari context yang di-set di sini.
func Tracing() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Nama sementara: di middleware, c.Route() masih menunjuk route
		// middleware ("/"), bukan route handler. Nama final di-set setelah
		// c.Next() ketika route yang cocok sudah diketahui.
		ctx, span := otel.Tracer(tracerScope).Start(
			c.UserContext(),
			c.Method(),
			trace.WithSpanKind(trace.SpanKindServer),
		)
		defer span.End()

		// Handler dan logger di bawahnya membaca context ini.
		c.SetUserContext(ctx)

		err := c.Next()

		// Route handler sudah diketahui di titik ini; template-nya dipakai
		// sebagai nama span ("GET /api/songs/:id"), bukan path mentah yang
		// kardinalitasnya tak terbatas.
		route := c.Route().Path
		if route == "" {
			route = "unmatched"
		}
		span.SetName(c.Method() + " " + route)

		// Response final belum tentu ditulis: error yang dikembalikan handler
		// baru diterjemahkan error handler Fiber SETELAH middleware ini
		// selesai. Karena itu status dihitung dari error bila ada.
		status := c.Response().StatusCode()
		if err != nil {
			status = fiber.StatusInternalServerError
			var fiberErr *fiber.Error
			if errors.As(err, &fiberErr) {
				status = fiberErr.Code
			}
		}

		span.SetAttributes(
			attribute.String("http.request.method", c.Method()),
			attribute.String("http.route", route),
			attribute.Int("http.response.status_code", status),
		)

		if requestID, ok := RequestIDFromContext(c); ok {
			span.SetAttributes(attribute.String("request.id", requestID))
		}
		if userID, ok := UserIDFromContext(c); ok {
			span.SetAttributes(attribute.String("user.id", userID.String()))
		}

		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else if status >= 500 {
			span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", status))
		}

		return err
	}
}
