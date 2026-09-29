package handler

import (
	"errors"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"spotify-backend/internal/middleware"
	"spotify-backend/internal/model"
	"spotify-backend/internal/service"
	"spotify-backend/pkg/apperr"
	"spotify-backend/pkg/pagination"
	"spotify-backend/pkg/response"
)

type SocialHandler struct {
	service *service.SocialService
}

func NewSocialHandler(service *service.SocialService) *SocialHandler {
	return &SocialHandler{service: service}
}

// GetProfile menampilkan profil publik. Endpointnya publik, tapi kalau request
// membawa token yang valid, `is_following` ikut terisi.
func (h *SocialHandler) GetProfile(c *fiber.Ctx) error {
	userID, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid user id")
	}

	var viewerID *uuid.UUID
	if id, ok := middleware.UserIDFromContext(c); ok {
		viewerID = &id
	}

	profile, err := h.service.GetProfile(c.UserContext(), userID, viewerID)
	if err != nil {
		return socialError(c, err)
	}
	return c.JSON(profile)
}

func (h *SocialHandler) Follow(c *fiber.Ctx) error {
	followerID, ok := middleware.UserIDFromContext(c)
	if !ok {
		return response.Unauthorized(c, "unauthorized")
	}

	followeeID, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid user id")
	}

	if err := h.service.Follow(c.UserContext(), followerID, followeeID); err != nil {
		return socialError(c, err)
	}

	return c.JSON(model.MessageResponse{Message: "user followed"})
}

func (h *SocialHandler) Unfollow(c *fiber.Ctx) error {
	followerID, ok := middleware.UserIDFromContext(c)
	if !ok {
		return response.Unauthorized(c, "unauthorized")
	}

	followeeID, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid user id")
	}

	if err := h.service.Unfollow(c.UserContext(), followerID, followeeID); err != nil {
		return socialError(c, err)
	}

	return c.JSON(model.MessageResponse{Message: "user unfollowed"})
}

func (h *SocialHandler) GetFollowers(c *fiber.Ctx) error {
	userID, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid user id")
	}

	params, err := pagination.Parse(c.Query("limit"), c.Query("offset"))
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	page, err := h.service.GetFollowers(c.UserContext(), userID, params)
	if err != nil {
		return socialError(c, err)
	}
	return c.JSON(page)
}

func (h *SocialHandler) GetFollowing(c *fiber.Ctx) error {
	userID, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid user id")
	}

	params, err := pagination.Parse(c.Query("limit"), c.Query("offset"))
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	page, err := h.service.GetFollowing(c.UserContext(), userID, params)
	if err != nil {
		return socialError(c, err)
	}
	return c.JSON(page)
}

func (h *SocialHandler) GetPlaylists(c *fiber.Ctx) error {
	userID, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid user id")
	}

	params, err := pagination.Parse(c.Query("limit"), c.Query("offset"))
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	page, err := h.service.GetPublicPlaylists(c.UserContext(), userID, params)
	if err != nil {
		return socialError(c, err)
	}
	return c.JSON(page)
}

func socialError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrUserNotFound):
		return response.NotFound(c, err.Error())

	case errors.Is(err, service.ErrCannotFollowSelf):
		return response.Error(c, fiber.StatusUnprocessableEntity, apperr.CodeValidation, err.Error())

	default:
		log.Printf("social handler error on %s %s: %v", c.Method(), c.Path(), err)
		return response.Internal(c)
	}
}

// parseUUIDParam membaca path parameter sebagai UUID.
func parseUUIDParam(c *fiber.Ctx, name string) (uuid.UUID, error) {
	return uuid.Parse(c.Params(name))
}
