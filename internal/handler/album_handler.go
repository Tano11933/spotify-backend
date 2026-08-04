package handler

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"spotify-backend/internal/model"
	"spotify-backend/internal/service"
	"spotify-backend/pkg/validator"
)

type AlbumHandler struct {
	service *service.AlbumService
}

func NewAlbumHandler(service *service.AlbumService) *AlbumHandler {
	return &AlbumHandler{service: service}
}

func (h *AlbumHandler) Create(c *fiber.Ctx) error {
	var album model.Album
	if err := c.BodyParser(&album); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if err := validator.ValidateStruct(album); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	if err := h.service.CreateAlbum(&album); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(album)
}

func (h *AlbumHandler) GetAll(c *fiber.Ctx) error {
	albums, err := h.service.GetAllAlbums()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(albums)
}

func (h *AlbumHandler) GetByID(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	album, err := h.service.GetAlbumByID(uint(id))
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "album not found"})
	}
	return c.JSON(album)
}

func (h *AlbumHandler) Update(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	album, err := h.service.GetAlbumByID(uint(id))
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "album not found"})
	}

	if err := c.BodyParser(album); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if err := validator.ValidateStruct(album); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	if err := h.service.UpdateAlbum(album); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(album)
}

func (h *AlbumHandler) Delete(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	if err := h.service.DeleteAlbum(uint(id)); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "album deleted"})
}