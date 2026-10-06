package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// RequestIDHeader adalah header yang dibaca dan dikembalikan middleware.
const RequestIDHeader = "X-Request-ID"

// maxRequestIDLength membatasi id dari client supaya header raksasa tidak
// masuk ke log. Yang lebih panjang dianggap tidak masuk akal dan diganti.
const maxRequestIDLength = 64

const requestIDKey = "request_id"

// RequestID memastikan setiap request punya id yang bisa dilacak:
//
//   - id dari client (X-Request-ID) dipakai ulang kalau masuk akal, sehingga
//     korelasi dengan sistem lain (gateway, frontend) tetap terjaga;
//   - kalau tidak ada, server membuat UUID sendiri;
//   - id selalu dikembalikan di response header supaya client bisa menyebutnya
//     saat melaporkan masalah.
func RequestID() fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := c.Get(RequestIDHeader)
		if id == "" || len(id) > maxRequestIDLength {
			id = uuid.NewString()
		}

		c.Locals(requestIDKey, id)
		c.Set(RequestIDHeader, id)

		return c.Next()
	}
}

// RequestIDFromContext membaca id yang dipasang RequestID.
func RequestIDFromContext(c *fiber.Ctx) (string, bool) {
	id, ok := c.Locals(requestIDKey).(string)
	return id, ok
}
