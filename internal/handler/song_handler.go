package handler

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"spotify-backend/internal/model"
	"spotify-backend/internal/service"
	"spotify-backend/pkg/validator"
)

type SongHandler struct {
	service *service.SongService
}

func NewSongHandler(service *service.SongService) *SongHandler {
	return &SongHandler{service: service}
}

func (h *SongHandler) Create(c *fiber.Ctx) error {
	var song model.Song
	if err := c.BodyParser(&song); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if err := validator.ValidateStruct(song); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	if err := h.service.CreateSong(&song); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(song)
}

func (h *SongHandler) GetAll(c *fiber.Ctx) error {
	songs, err := h.service.GetAllSongs()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(songs)
}

func (h *SongHandler) GetByID(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	song, err := h.service.GetSongByID(uint(id))
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "song not found"})
	}
	return c.JSON(song)
}

func (h *SongHandler) GetByArtist(c *fiber.Ctx) error {
	artistID, err := strconv.Atoi(c.Params("artistId"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid artist id"})
	}

	songs, err := h.service.GetSongsByArtist(uint(artistID))
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(songs)
}

func (h *SongHandler) Update(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	song, err := h.service.GetSongByID(uint(id))
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "song not found"})
	}

	if err := c.BodyParser(song); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if err := validator.ValidateStruct(song); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	if err := h.service.UpdateSong(song); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(song)
}

func (h *SongHandler) Delete(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	if err := h.service.DeleteSong(uint(id)); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "song deleted"})
}