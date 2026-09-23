// Package response menyatukan bentuk response error aplikasi.
//
// Semua error keluar sebagai {"error": "...", "code": "..."} — bentuk lama
// `{"error"}` tetap ada, `code` bersifat aditif. Helper ini dipakai handler
// DAN middleware; kalau ditaruh di internal/handler, middleware tidak bisa
// memakainya tanpa import cycle.
package response

import (
	"github.com/gofiber/fiber/v2"

	"spotify-backend/pkg/apperr"
	"spotify-backend/pkg/validator"
)

// Error mengirim error dengan kode machine-readable.
func Error(c *fiber.Ctx, status int, code apperr.Code, message string) error {
	return c.Status(status).JSON(fiber.Map{
		"error": message,
		"code":  code,
	})
}

// Validation mengirim 400 beserta `details` berisi field yang gagal divalidasi
// (kalau error-nya memang berasal dari validator).
func Validation(c *fiber.Ctx, err error) error {
	payload := fiber.Map{
		"error": err.Error(),
		"code":  apperr.CodeValidation,
	}

	if details := validator.Details(err); len(details) > 0 {
		payload["details"] = details
	}

	return c.Status(fiber.StatusBadRequest).JSON(payload)
}

func BadRequest(c *fiber.Ctx, message string) error {
	return Error(c, fiber.StatusBadRequest, apperr.CodeValidation, message)
}

func Unauthorized(c *fiber.Ctx, message string) error {
	return Error(c, fiber.StatusUnauthorized, apperr.CodeUnauthorized, message)
}

func Forbidden(c *fiber.Ctx, message string) error {
	return Error(c, fiber.StatusForbidden, apperr.CodeForbidden, message)
}

func NotFound(c *fiber.Ctx, message string) error {
	return Error(c, fiber.StatusNotFound, apperr.CodeNotFound, message)
}

func Conflict(c *fiber.Ctx, message string) error {
	return Error(c, fiber.StatusConflict, apperr.CodeConflict, message)
}

func Internal(c *fiber.Ctx) error {
	return Error(c, fiber.StatusInternalServerError, apperr.CodeInternal, "internal server error")
}
