package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"spotify-backend/internal/model"
	"spotify-backend/internal/service"
	"spotify-backend/pkg/pagination"
	"spotify-backend/pkg/response"
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
		return response.BadRequest(c, "invalid request body")
	}

	// Sanitasi mass assignment: primary key, timestamp, dan relasi tidak boleh
	// datang dari body. Relasi hanya ditentukan lewat artist_id (dan album_id
	// di endpoint song) yang sudah divalidasi service.
	album.ID = 0
	album.Artist = nil
	album.Songs = nil
	album.CreatedAt = time.Time{}
	album.UpdatedAt = time.Time{}

	if err := validator.ValidateStruct(album); err != nil {
		return response.Validation(c, err)
	}

	if err := h.service.CreateAlbum(c.UserContext(), &album); err != nil {
		return respondError(c, "album", err)
	}

	return c.Status(fiber.StatusCreated).JSON(album)
}

func (h *AlbumHandler) GetAll(c *fiber.Ctx) error {
	params, err := pagination.Parse(c.Query("limit"), c.Query("offset"))
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	page, err := h.service.GetAllAlbums(c.UserContext(), params)
	if err != nil {
		return respondError(c, "album", err)
	}
	return c.JSON(page)
}

func (h *AlbumHandler) GetByID(c *fiber.Ctx) error {
	id, err := parseUintParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid id")
	}

	album, err := h.service.GetAlbumByID(c.UserContext(), id)
	if err != nil {
		return respondError(c, "album", err)
	}
	return c.JSON(album)
}

func (h *AlbumHandler) Update(c *fiber.Ctx) error {
	id, err := parseUintParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid id")
	}

	album, err := h.service.GetAlbumByID(c.UserContext(), id)
	if err != nil {
		return respondError(c, "album", err)
	}

	// BodyParser menimpa field yang ada di body — termasuk created_at.
	// Simpan nilai aslinya dulu, lalu kembalikan setelah parse.
	createdAt := album.CreatedAt

	if err := c.BodyParser(album); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	album.ID = id
	album.CreatedAt = createdAt

	if err := validator.ValidateStruct(album); err != nil {
		return response.Validation(c, err)
	}

	if err := h.service.UpdateAlbum(c.UserContext(), album); err != nil {
		return respondError(c, "album", err)
	}

	return c.JSON(album)
}

func (h *AlbumHandler) Delete(c *fiber.Ctx) error {
	id, err := parseUintParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid id")
	}

	if err := h.service.DeleteAlbum(c.UserContext(), id); err != nil {
		return respondError(c, "album", err)
	}

	return c.JSON(model.MessageResponse{Message: "album deleted"})
}
