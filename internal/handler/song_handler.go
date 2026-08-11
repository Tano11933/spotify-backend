package handler

import (
	"errors"

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
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if err := validator.ValidateStruct(song); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if err := h.service.CreateSong(c.UserContext(), &song); err != nil {
		return respondError(c, "song", err)
	}

	return c.Status(fiber.StatusCreated).JSON(song)
}

func (h *SongHandler) GetAll(c *fiber.Ctx) error {
	songs, err := h.service.GetAllSongs(c.UserContext())
	if err != nil {
		return respondError(c, "song", err)
	}
	return c.JSON(songs)
}

func (h *SongHandler) GetByID(c *fiber.Ctx) error {
	id, err := parseUintParam(c, "id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	song, err := h.service.GetSongByID(c.UserContext(), id)
	if err != nil {
		return respondError(c, "song", err)
	}
	return c.JSON(song)
}

func (h *SongHandler) GetByArtist(c *fiber.Ctx) error {
	artistID, err := parseUintParam(c, "artistId")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid artist id"})
	}

	songs, err := h.service.GetSongsByArtist(c.UserContext(), artistID)
	if err != nil {
		if errors.Is(err, service.ErrArtistNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return respondError(c, "song", err)
	}
	return c.JSON(songs)
}

func (h *SongHandler) Update(c *fiber.Ctx) error {
	id, err := parseUintParam(c, "id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	song, err := h.service.GetSongByID(c.UserContext(), id)
	if err != nil {
		return respondError(c, "song", err)
	}

	if err := c.BodyParser(song); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	song.ID = id

	if err := validator.ValidateStruct(song); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if err := h.service.UpdateSong(c.UserContext(), song); err != nil {
		return respondError(c, "song", err)
	}

	return c.JSON(song)
}

func (h *SongHandler) Delete(c *fiber.Ctx) error {
	id, err := parseUintParam(c, "id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	if err := h.service.DeleteSong(c.UserContext(), id); err != nil {
		return respondError(c, "song", err)
	}

	return c.JSON(model.MessageResponse{Message: "song deleted"})
}
