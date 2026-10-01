package handler

import (
	"errors"
	"log"

	"github.com/gofiber/fiber/v2"

	"spotify-backend/internal/middleware"
	"spotify-backend/internal/service"
	"spotify-backend/pkg/pagination"
	"spotify-backend/pkg/response"
)

type RecommendationHandler struct {
	service *service.RecommendationService
}

func NewRecommendationHandler(service *service.RecommendationService) *RecommendationHandler {
	return &RecommendationHandler{service: service}
}

// RelatedArtists menangani "Fans also like" untuk satu artist. Publik, sama
// seperti detail artist yang ditautkannya.
func (h *RecommendationHandler) RelatedArtists(c *fiber.Ctx) error {
	artistID, err := parseUintParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid artist id")
	}

	params, err := pagination.Parse(c.Query("limit"), c.Query("offset"))
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	page, err := h.service.RelatedArtists(c.UserContext(), artistID, params)
	if err != nil {
		if errors.Is(err, service.ErrArtistNotFound) {
			return response.NotFound(c, "artist not found")
		}

		log.Printf("recommendation handler error on %s %s: %v", c.Method(), c.Path(), err)
		return response.Internal(c)
	}
	return c.JSON(page)
}

// MadeForYou mengembalikan rekomendasi pribadi untuk user yang sedang login.
func (h *RecommendationHandler) MadeForYou(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		return response.Unauthorized(c, "unauthorized")
	}

	params, err := pagination.Parse(c.Query("limit"), c.Query("offset"))
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	page, err := h.service.MadeForYou(c.UserContext(), userID, params)
	if err != nil {
		log.Printf("recommendation handler error on %s %s: %v", c.Method(), c.Path(), err)
		return response.Internal(c)
	}
	return c.JSON(page)
}
