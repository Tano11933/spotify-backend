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

type PlaylistHandler struct {
	service *service.PlaylistService
}

func NewPlaylistHandler(service *service.PlaylistService) *PlaylistHandler {
	return &PlaylistHandler{service: service}
}

func (h *PlaylistHandler) Create(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		return response.Unauthorized(c, "unauthorized")
	}

	var req model.CreatePlaylistRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}

	if err := validator.ValidateStruct(req); err != nil {
		return response.Validation(c, err)
	}

	playlist, err := h.service.Create(c.UserContext(), userID, req)
	if err != nil {
		return playlistError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(playlist)
}

// GetMine mengembalikan playlist milik user yang sedang login.
func (h *PlaylistHandler) GetMine(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		return response.Unauthorized(c, "unauthorized")
	}

	params, err := pagination.Parse(c.Query("limit"), c.Query("offset"))
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	page, err := h.service.GetOwned(c.UserContext(), userID, params)
	if err != nil {
		return playlistError(c, err)
	}
	return c.JSON(page)
}

// GetPublic mengembalikan halaman playlist yang ditandai publik.
func (h *PlaylistHandler) GetPublic(c *fiber.Ctx) error {
	params, err := pagination.Parse(c.Query("limit"), c.Query("offset"))
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	page, err := h.service.GetPublic(c.UserContext(), params)
	if err != nil {
		return playlistError(c, err)
	}
	return c.JSON(page)
}

func (h *PlaylistHandler) GetByID(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		return response.Unauthorized(c, "unauthorized")
	}

	id, err := parseUintParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid id")
	}

	playlist, err := h.service.GetByID(c.UserContext(), id, userID)
	if err != nil {
		return playlistError(c, err)
	}
	return c.JSON(playlist)
}

func (h *PlaylistHandler) Update(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		return response.Unauthorized(c, "unauthorized")
	}

	id, err := parseUintParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid id")
	}

	var req model.UpdatePlaylistRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}

	if err := validator.ValidateStruct(req); err != nil {
		return response.Validation(c, err)
	}

	playlist, err := h.service.Update(c.UserContext(), id, userID, req)
	if err != nil {
		return playlistError(c, err)
	}
	return c.JSON(playlist)
}

func (h *PlaylistHandler) Delete(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		return response.Unauthorized(c, "unauthorized")
	}

	id, err := parseUintParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid id")
	}

	if err := h.service.Delete(c.UserContext(), id, userID); err != nil {
		return playlistError(c, err)
	}
	return c.JSON(model.MessageResponse{Message: "playlist deleted"})
}

func (h *PlaylistHandler) AddSong(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		return response.Unauthorized(c, "unauthorized")
	}

	id, err := parseUintParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid id")
	}

	var req model.AddSongRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}

	if err := validator.ValidateStruct(req); err != nil {
		return response.Validation(c, err)
	}

	if err := h.service.AddSong(c.UserContext(), id, userID, req.SongID); err != nil {
		return playlistError(c, err)
	}
	return c.JSON(model.MessageResponse{Message: "song added to playlist"})
}

func (h *PlaylistHandler) RemoveSong(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		return response.Unauthorized(c, "unauthorized")
	}

	id, err := parseUintParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid id")
	}

	songID, err := parseUintParam(c, "songId")
	if err != nil {
		return response.BadRequest(c, "invalid song id")
	}

	if err := h.service.RemoveSong(c.UserContext(), id, userID, songID); err != nil {
		return playlistError(c, err)
	}
	return c.JSON(model.MessageResponse{Message: "song removed from playlist"})
}

func playlistError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrPlaylistNotFound),
		errors.Is(err, service.ErrSongNotFound),
		errors.Is(err, service.ErrSongNotInPlaylist):
		return response.NotFound(c, err.Error())

	case errors.Is(err, service.ErrPlaylistForbidden):
		return response.Forbidden(c, err.Error())

	case errors.Is(err, service.ErrSongAlreadyInPlaylist):
		return response.Conflict(c, err.Error())

	default:
		log.Printf("playlist handler error on %s %s: %v", c.Method(), c.Path(), err)
		return response.Internal(c)
	}
}
