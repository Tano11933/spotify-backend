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
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if err := validator.ValidateStruct(artist); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if err := h.service.CreateArtist(c.UserContext(), &artist); err != nil {
		return respondError(c, "artist", err)
	}

	return c.Status(fiber.StatusCreated).JSON(artist)
}

func (h *ArtistHandler) GetAll(c *fiber.Ctx) error {
	artists, err := h.service.GetAllArtists(c.UserContext())
	if err != nil {
		return respondError(c, "artist", err)
	}
	return c.JSON(artists)
}

func (h *ArtistHandler) GetByID(c *fiber.Ctx) error {
	id, err := parseUintParam(c, "id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	artist, err := h.service.GetArtistByID(c.UserContext(), id)
	if err != nil {
		return respondError(c, "artist", err)
	}
	return c.JSON(artist)
}

func (h *ArtistHandler) Update(c *fiber.Ctx) error {
	id, err := parseUintParam(c, "id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	// Ambil data existing lebih dulu, lalu timpa dengan field dari body. Efeknya
	// PUT di sini berperilaku seperti PATCH: field yang tidak dikirim tetap
	// memakai nilai lamanya.
	artist, err := h.service.GetArtistByID(c.UserContext(), id)
	if err != nil {
		return respondError(c, "artist", err)
	}

	if err := c.BodyParser(artist); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	// ID dipaksa kembali ke nilai dari URL. Tanpa ini, body berisi {"id": 99}
	// akan membuat request PUT /api/artists/1 justru menimpa artist 99.
	artist.ID = id

	if err := validator.ValidateStruct(artist); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if err := h.service.UpdateArtist(c.UserContext(), artist); err != nil {
		return respondError(c, "artist", err)
	}

	return c.JSON(artist)
}

func (h *ArtistHandler) Delete(c *fiber.Ctx) error {
	id, err := parseUintParam(c, "id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	if err := h.service.DeleteArtist(c.UserContext(), id); err != nil {
		return respondError(c, "artist", err)
	}

	return c.JSON(model.MessageResponse{Message: "artist deleted"})
}

// parseUintParam membaca path parameter sebagai uint.
//
// ParseUint dengan bitSize 32 dipakai, bukan strconv.Atoi lalu uint(id) seperti
// sebelumnya. Bedanya penting: Atoi menerima angka negatif, dan uint(-1) di Go
// tidak error — ia melipat jadi 4294967295. Jadi DELETE /api/artists/-1 dulu
// diterjemahkan menjadi id 4294967295, bukan ditolak.
func parseUintParam(c *fiber.Ctx, name string) (uint, error) {
	value, err := strconv.ParseUint(c.Params(name), 10, 32)
	if err != nil {
		return 0, err
	}
	return uint(value), nil
}
