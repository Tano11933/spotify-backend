package handler

import (
	"errors"
	"log"

	"github.com/gofiber/fiber/v2"

	"spotify-backend/internal/middleware"
	"spotify-backend/internal/model"
	"spotify-backend/internal/service"
	"spotify-backend/pkg/pagination"
	"spotify-backend/pkg/response"
	"spotify-backend/pkg/validator"
)

// DTO request player — kecil dan hanya dipakai di sini, jadi tidak perlu
// dinaikkan ke package model.
type updatePlayerRequest struct {
	SongID          uint `json:"song_id" validate:"required,min=1"`
	PositionSeconds int  `json:"position_seconds" validate:"min=0"`
}

type playerSongRequest struct {
	SongID uint `json:"song_id" validate:"required,min=1"`
}

type PlayerHandler struct {
	service *service.PlayerService
}

func NewPlayerHandler(service *service.PlayerService) *PlayerHandler {
	return &PlayerHandler{service: service}
}

func (h *PlayerHandler) GetState(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		return response.Unauthorized(c, "unauthorized")
	}

	state, err := h.service.GetState(c.UserContext(), userID)
	if err != nil {
		return playerError(c, err)
	}
	return c.JSON(state)
}

// UpdateState menyinkronkan posisi (resume lintas device) tanpa mencatat riwayat.
func (h *PlayerHandler) UpdateState(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		return response.Unauthorized(c, "unauthorized")
	}

	var req updatePlayerRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	if err := validator.ValidateStruct(req); err != nil {
		return response.Validation(c, err)
	}

	state, err := h.service.UpdateState(c.UserContext(), userID, req.SongID, req.PositionSeconds)
	if err != nil {
		return playerError(c, err)
	}
	return c.JSON(state)
}

// Play memulai pemutaran: state + riwayat + penghitung putar + broadcast WS.
func (h *PlayerHandler) Play(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		return response.Unauthorized(c, "unauthorized")
	}

	var req playerSongRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	if err := validator.ValidateStruct(req); err != nil {
		return response.Validation(c, err)
	}

	state, err := h.service.Play(c.UserContext(), userID, req.SongID)
	if err != nil {
		return playerError(c, err)
	}
	return c.JSON(state)
}

func (h *PlayerHandler) GetQueue(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		return response.Unauthorized(c, "unauthorized")
	}

	params, err := pagination.Parse(c.Query("limit"), c.Query("offset"))
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	page, err := h.service.GetQueue(c.UserContext(), userID, params)
	if err != nil {
		return playerError(c, err)
	}
	return c.JSON(page)
}

func (h *PlayerHandler) AddToQueue(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		return response.Unauthorized(c, "unauthorized")
	}

	var req playerSongRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	if err := validator.ValidateStruct(req); err != nil {
		return response.Validation(c, err)
	}

	if err := h.service.AddToQueue(c.UserContext(), userID, req.SongID); err != nil {
		return playerError(c, err)
	}

	return c.JSON(model.MessageResponse{Message: "song added to queue"})
}

func (h *PlayerHandler) RemoveFromQueue(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		return response.Unauthorized(c, "unauthorized")
	}

	songID, err := parseUintParam(c, "songId")
	if err != nil {
		return response.BadRequest(c, "invalid song id")
	}

	if err := h.service.RemoveFromQueue(c.UserContext(), userID, songID); err != nil {
		return playerError(c, err)
	}

	return c.JSON(model.MessageResponse{Message: "song removed from queue"})
}

func (h *PlayerHandler) GetHistory(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		return response.Unauthorized(c, "unauthorized")
	}

	params, err := pagination.Parse(c.Query("limit"), c.Query("offset"))
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	page, err := h.service.GetHistory(c.UserContext(), userID, params)
	if err != nil {
		return playerError(c, err)
	}
	return c.JSON(page)
}

func playerError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrSongNotFound):
		return response.NotFound(c, err.Error())

	default:
		log.Printf("player handler error on %s %s: %v", c.Method(), c.Path(), err)
		return response.Internal(c)
	}
}
