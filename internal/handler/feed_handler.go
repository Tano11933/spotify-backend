package handler

import (
	"log"

	"github.com/gofiber/fiber/v2"

	"spotify-backend/internal/middleware"
	"spotify-backend/internal/service"
	"spotify-backend/pkg/pagination"
	"spotify-backend/pkg/response"
)

type FeedHandler struct {
	service *service.FeedService
}

func NewFeedHandler(service *service.FeedService) *FeedHandler {
	return &FeedHandler{service: service}
}

// GetFeed mengembalikan aktivitas user-user yang diikuti viewer.
func (h *FeedHandler) GetFeed(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		return response.Unauthorized(c, "unauthorized")
	}

	params, err := pagination.Parse(c.Query("limit"), c.Query("offset"))
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	page, err := h.service.GetFeed(c.UserContext(), userID, params)
	if err != nil {
		log.Printf("feed handler error on %s %s: %v", c.Method(), c.Path(), err)
		return response.Internal(c)
	}
	return c.JSON(page)
}
