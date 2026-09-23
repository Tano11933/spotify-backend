package handler

import (
	"errors"
	"log"

	"github.com/gofiber/fiber/v2"

	"spotify-backend/internal/repository"
	"spotify-backend/internal/service"
	"spotify-backend/pkg/apperr"
	"spotify-backend/pkg/response"
)

func respondError(c *fiber.Ctx, resource string, err error) error {
	switch {
	case errors.Is(err, repository.ErrNotFound),
		errors.Is(err, service.ErrSongNotFound):
		return response.NotFound(c, resource+" not found")

	case errors.Is(err, service.ErrArtistNotFound):
		return response.Error(c, fiber.StatusUnprocessableEntity, apperr.CodeValidation, err.Error())

	case errors.Is(err, repository.ErrDuplicate):
		return response.Conflict(c, err.Error())

	case errors.Is(err, service.ErrArtistNotEmpty),
		errors.Is(err, service.ErrAlbumNotEmpty):
		return response.Conflict(c, err.Error())

	default:
		log.Printf("handler error on %s %s: %v", c.Method(), c.Path(), err)
		return response.Internal(c)
	}
}
