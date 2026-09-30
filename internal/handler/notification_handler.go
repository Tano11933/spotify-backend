package handler

import (
	"log"

	"github.com/gofiber/fiber/v2"

	"spotify-backend/internal/middleware"
	"spotify-backend/internal/service"
	"spotify-backend/pkg/pagination"
	"spotify-backend/pkg/response"
)

type NotificationHandler struct {
	service *service.NotificationService
}

func NewNotificationHandler(service *service.NotificationService) *NotificationHandler {
	return &NotificationHandler{service: service}
}

func (h *NotificationHandler) GetMine(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		return response.Unauthorized(c, "unauthorized")
	}

	params, err := pagination.Parse(c.Query("limit"), c.Query("offset"))
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	page, err := h.service.GetMine(c.UserContext(), userID, params)
	if err != nil {
		log.Printf("notification handler error on %s %s: %v", c.Method(), c.Path(), err)
		return response.Internal(c)
	}
	return c.JSON(page)
}

// MarkAllRead menandai semua notifikasi user sudah dibaca. Idempoten.
func (h *NotificationHandler) MarkAllRead(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		return response.Unauthorized(c, "unauthorized")
	}

	if err := h.service.MarkAllRead(c.UserContext(), userID); err != nil {
		log.Printf("notification handler error on %s %s: %v", c.Method(), c.Path(), err)
		return response.Internal(c)
	}

	return c.JSON(map[string]string{"message": "notifications marked as read"})
}
