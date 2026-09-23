package handler

import (
	"errors"
	"log"

	"github.com/gofiber/fiber/v2"

	"spotify-backend/internal/repository"
	"spotify-backend/internal/service"
)

func respondError(c *fiber.Ctx, resource string, err error) error {
	switch {
	case errors.Is(err, repository.ErrNotFound),
		errors.Is(err, service.ErrSongNotFound):
		return c.Status(fiber.StatusNotFound).
			JSON(fiber.Map{"error": resource + " not found"})

	case errors.Is(err, service.ErrArtistNotFound):
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": err.Error()})

	case errors.Is(err, repository.ErrDuplicate):
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})

	case errors.Is(err, service.ErrArtistNotEmpty),
		errors.Is(err, service.ErrAlbumNotEmpty):
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})

	default:
		log.Printf("handler error on %s %s: %v", c.Method(), c.Path(), err)
		return c.Status(fiber.StatusInternalServerError).
			JSON(fiber.Map{"error": "internal server error"})
	}
}
