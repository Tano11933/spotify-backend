package handler

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"spotify-backend/internal/model"
	"spotify-backend/internal/service"
	"spotify-backend/pkg/validator"
)

type ArtistHandler struct {
	service *service.ArtistService
}

func NewArtistHandler(service *service.ArtistService) *ArtistHandler {
	return &ArtistHandler{service: service}
}

func (h *ArtistHandler) Create(c *fiber.Ctx) error {
	var artist model.Artist
	if err := c.BodyParser(&artist); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if err := validator.ValidateStruct(artist); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	if err := h.service.CreateArtist(&artist); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(artist)
}

func (h *ArtistHandler) GetAll(c *fiber.Ctx) error {
	artists, err := h.service.GetAllArtists()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(artists)
}

func (h *ArtistHandler) GetByID(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	artist, err := h.service.GetArtistByID(uint(id))
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "artist not found"})
	}
	return c.JSON(artist)
}

func (h *ArtistHandler) Update(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	artist, err := h.service.GetArtistByID(uint(id))
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "artist not found"})
	}

	if err := c.BodyParser(artist); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if err := validator.ValidateStruct(artist); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	if err := h.service.UpdateArtist(artist); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(artist)
}

func (h *ArtistHandler) Delete(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	if err := h.service.DeleteArtist(uint(id)); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "artist deleted"})
}