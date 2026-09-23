package handler

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"spotify-backend/internal/model"
	"spotify-backend/internal/service"
	"spotify-backend/pkg/pagination"
	"spotify-backend/pkg/response"
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
		return response.BadRequest(c, "invalid request body")
	}

	// Sanitasi mass assignment — lihat catatan di album_handler.go.
	song.ID = 0
	song.Artist = nil
	song.Album = nil
	song.CreatedAt = time.Time{}
	song.UpdatedAt = time.Time{}

	if err := validator.ValidateStruct(song); err != nil {
		return response.Validation(c, err)
	}

	if err := h.service.CreateSong(c.UserContext(), &song); err != nil {
		return respondError(c, "song", err)
	}

	return c.Status(fiber.StatusCreated).JSON(song)
}

func (h *SongHandler) GetAll(c *fiber.Ctx) error {
	params, err := pagination.Parse(c.Query("limit"), c.Query("offset"))
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	page, err := h.service.GetAllSongs(c.UserContext(), params)
	if err != nil {
		return respondError(c, "song", err)
	}
	return c.JSON(page)
}

func (h *SongHandler) GetByID(c *fiber.Ctx) error {
	id, err := parseUintParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid id")
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
		return response.BadRequest(c, "invalid artist id")
	}

	params, err := pagination.Parse(c.Query("limit"), c.Query("offset"))
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	page, err := h.service.GetSongsByArtist(c.UserContext(), artistID, params)
	if err != nil {
		if errors.Is(err, service.ErrArtistNotFound) {
			return response.NotFound(c, err.Error())
		}
		return respondError(c, "song", err)
	}
	return c.JSON(page)
}

func (h *SongHandler) Update(c *fiber.Ctx) error {
	id, err := parseUintParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid id")
	}

	song, err := h.service.GetSongByID(c.UserContext(), id)
	if err != nil {
		return respondError(c, "song", err)
	}

	// BodyParser menimpa field yang ada di body — termasuk created_at.
	// Simpan nilai aslinya dulu, lalu kembalikan setelah parse.
	createdAt := song.CreatedAt

	if err := c.BodyParser(song); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	song.ID = id
	song.CreatedAt = createdAt

	if err := validator.ValidateStruct(song); err != nil {
		return response.Validation(c, err)
	}

	if err := h.service.UpdateSong(c.UserContext(), song); err != nil {
		return respondError(c, "song", err)
	}

	return c.JSON(song)
}

func (h *SongHandler) Delete(c *fiber.Ctx) error {
	id, err := parseUintParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid id")
	}

	if err := h.service.DeleteSong(c.UserContext(), id); err != nil {
		return respondError(c, "song", err)
	}

	return c.JSON(model.MessageResponse{Message: "song deleted"})
}
