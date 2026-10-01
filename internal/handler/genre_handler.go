package handler

import (
	"errors"
	"log"

	"github.com/gofiber/fiber/v2"

	"spotify-backend/internal/model"
	"spotify-backend/internal/repository"
	"spotify-backend/internal/service"
	"spotify-backend/pkg/apperr"
	"spotify-backend/pkg/pagination"
	"spotify-backend/pkg/response"
	"spotify-backend/pkg/validator"
)

type GenreHandler struct {
	service *service.GenreService
}

func NewGenreHandler(service *service.GenreService) *GenreHandler {
	return &GenreHandler{service: service}
}

type genreRequest struct {
	Name string `json:"name" validate:"required,min=2,max=50"`
}

type setGenresRequest struct {
	// Dibatasi 10 supaya satu artist tidak menumpuk puluhan genre; "dive"
	// memvalidasi tiap elemen, bukan slice-nya secara keseluruhan.
	GenreIDs []uint `json:"genre_ids" validate:"required,max=10,dive,min=1"`
}

func (h *GenreHandler) GetAll(c *fiber.Ctx) error {
	params, err := pagination.Parse(c.Query("limit"), c.Query("offset"))
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	page, err := h.service.GetAllGenres(c.UserContext(), params)
	if err != nil {
		return genreError(c, err)
	}
	return c.JSON(page)
}

func (h *GenreHandler) GetByID(c *fiber.Ctx) error {
	id, err := parseUintParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid id")
	}

	genre, err := h.service.GetGenreByID(c.UserContext(), id)
	if err != nil {
		return genreError(c, err)
	}
	return c.JSON(genre)
}

// GetArtists menampilkan artist yang terhubung ke genre ini, dipakai halaman
// Browse: klik kategori, keluar daftar artist-nya.
func (h *GenreHandler) GetArtists(c *fiber.Ctx) error {
	id, err := parseUintParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid id")
	}

	params, err := pagination.Parse(c.Query("limit"), c.Query("offset"))
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	page, err := h.service.GetArtistsByGenre(c.UserContext(), id, params)
	if err != nil {
		return genreError(c, err)
	}
	return c.JSON(page)
}

func (h *GenreHandler) Create(c *fiber.Ctx) error {
	var req genreRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}

	if err := validator.ValidateStruct(req); err != nil {
		return response.Validation(c, err)
	}

	genre := &model.Genre{Name: req.Name}
	if err := h.service.CreateGenre(c.UserContext(), genre); err != nil {
		return genreError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(genre)
}

func (h *GenreHandler) Update(c *fiber.Ctx) error {
	id, err := parseUintParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid id")
	}

	// Ambil dulu data lama, lalu timpa dengan body. Efeknya PUT di sini
	// berperilaku seperti PATCH: field yang tidak dikirim memakai nilai lama.
	genre, err := h.service.GetGenreByID(c.UserContext(), id)
	if err != nil {
		return genreError(c, err)
	}

	createdAt := genre.CreatedAt

	var req genreRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	if err := validator.ValidateStruct(req); err != nil {
		return response.Validation(c, err)
	}

	genre.Name = req.Name
	genre.ID = id
	genre.CreatedAt = createdAt

	if err := h.service.UpdateGenre(c.UserContext(), genre); err != nil {
		return genreError(c, err)
	}
	return c.JSON(genre)
}

func (h *GenreHandler) Delete(c *fiber.Ctx) error {
	id, err := parseUintParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid id")
	}

	if err := h.service.DeleteGenre(c.UserContext(), id); err != nil {
		return genreError(c, err)
	}
	return c.JSON(model.MessageResponse{Message: "genre deleted"})
}

// SetArtistGenres mengganti seluruh genre milik artist. Body berisi daftar
// genre_ids; daftar kosong berarti melepas semua genre artist tersebut.
func (h *GenreHandler) SetArtistGenres(c *fiber.Ctx) error {
	artistID, err := parseUintParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid artist id")
	}

	var req setGenresRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	if err := validator.ValidateStruct(req); err != nil {
		return response.Validation(c, err)
	}

	artist, err := h.service.SetArtistGenres(c.UserContext(), artistID, req.GenreIDs)
	if err != nil {
		return genreError(c, err)
	}
	return c.JSON(artist)
}

func genreError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrGenreNotFound),
		errors.Is(err, repository.ErrNotFound):
		return response.NotFound(c, "genre not found")

	case errors.Is(err, service.ErrArtistNotFound):
		return response.Error(c, fiber.StatusUnprocessableEntity, apperr.CodeValidation, err.Error())

	case errors.Is(err, service.ErrUnknownGenres):
		return response.Error(c, fiber.StatusUnprocessableEntity, apperr.CodeValidation, err.Error())

	case errors.Is(err, service.ErrInvalidGenreName):
		return response.BadRequest(c, err.Error())

	case errors.Is(err, repository.ErrDuplicate):
		return response.Conflict(c, "genre name or slug already exists")

	default:
		log.Printf("genre handler error on %s %s: %v", c.Method(), c.Path(), err)
		return response.Internal(c)
	}
}
