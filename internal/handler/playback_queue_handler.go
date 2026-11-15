package handler

import (
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"spotify-backend/internal/middleware"
	"spotify-backend/internal/model"
	"spotify-backend/internal/repository"
	"spotify-backend/pkg/pagination"
	"spotify-backend/pkg/response"
)

func (h *PlayerHandler) GetQueueEntries(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		return response.Unauthorized(c, "unauthorized")
	}
	params, err := pagination.Parse(c.Query("limit"), c.Query("offset"))
	if err != nil {
		return response.BadRequest(c, err.Error())
	}
	page, err := h.service.GetQueueEntries(c.UserContext(), userID, params)
	if err != nil {
		return playerError(c, err)
	}
	return c.JSON(page)
}

func (h *PlayerHandler) ReserveQueueEntry(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		return response.Unauthorized(c, "unauthorized")
	}
	var req struct {
		EntryID uint64 `json:"entry_id"`
	}
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	reservation, err := h.service.ReserveQueueEntry(c.UserContext(), userID, req.EntryID)
	if err != nil {
		return playerError(c, err)
	}
	return c.JSON(reservation)
}

func (h *PlayerHandler) ConfirmQueueEntry(c *fiber.Ctx) error {
	return h.finishQueueReservation(c, true)
}

func (h *PlayerHandler) ReleaseQueueEntry(c *fiber.Ctx) error {
	return h.finishQueueReservation(c, false)
}

func (h *PlayerHandler) finishQueueReservation(c *fiber.Ctx, confirm bool) error {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := uuid.Parse(c.Params("reservationId"))
	if err != nil || id == uuid.Nil {
		return response.BadRequest(c, "invalid reservation id")
	}
	if confirm {
		err = h.service.ConfirmQueueEntry(c.UserContext(), userID, id)
	} else {
		err = h.service.ReleaseQueueEntry(c.UserContext(), userID, id)
	}
	if err != nil {
		return playerError(c, err)
	}
	return c.JSON(model.MessageResponse{Message: "queue reservation updated"})
}

func (h *PlayerHandler) RemoveQueueEntry(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := strconv.ParseUint(c.Params("entryId"), 10, 64)
	if err != nil || id == 0 {
		return response.BadRequest(c, "invalid queue entry id")
	}
	if err := h.service.RemoveQueueEntry(c.UserContext(), userID, id); err != nil {
		return playerError(c, err)
	}
	return c.JSON(model.MessageResponse{Message: "queue entry removed"})
}

func (h *PlayerHandler) MoveQueueEntry(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := strconv.ParseUint(c.Params("entryId"), 10, 63)
	if err != nil || id == 0 {
		return response.BadRequest(c, "invalid queue entry id")
	}
	var req struct {
		Direction string `json:"direction"`
	}
	if err := c.BodyParser(&req); err != nil || (req.Direction != "up" && req.Direction != "down") {
		return response.BadRequest(c, "direction must be up or down")
	}
	moved, err := h.service.MoveQueueEntry(c.UserContext(), userID, id, req.Direction == "up")
	switch {
	case errors.Is(err, repository.ErrNotFound):
		return response.NotFound(c, "queue entry not found")
	case errors.Is(err, repository.ErrQueueMoveReserved):
		return response.Conflict(c, err.Error())
	case err != nil:
		return playerError(c, err)
	default:
		return c.JSON(fiber.Map{"moved": moved})
	}
}
