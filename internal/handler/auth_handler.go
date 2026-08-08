package handler

import (
	"errors"
	"log"

	"github.com/gofiber/fiber/v2"

	"spotify-backend/internal/middleware"
	"spotify-backend/internal/model"
	"spotify-backend/internal/service"
	"spotify-backend/pkg/validator"
)

type AuthHandler struct {
	service *service.AuthService
}

func NewAuthHandler(service *service.AuthService) *AuthHandler {
	return &AuthHandler{service: service}
}

func (h *AuthHandler) Register(c *fiber.Ctx) error {
	var req model.RegisterRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	req.Normalize()

	if err := validator.ValidateStruct(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	user, err := h.service.Register(c.UserContext(), req)
	if err != nil {
		return authError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(user)
}

func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req model.LoginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	req.Normalize()

	if err := validator.ValidateStruct(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	res, err := h.service.Login(c.UserContext(), req)
	if err != nil {
		return authError(c, err)
	}

	return c.JSON(res)
}

func (h *AuthHandler) Refresh(c *fiber.Ctx) error {
	var req model.RefreshRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if err := validator.ValidateStruct(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	tokens, err := h.service.Refresh(c.UserContext(), req.RefreshToken)
	if err != nil {
		return authError(c, err)
	}

	return c.JSON(tokens)
}

func (h *AuthHandler) Logout(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	if err := h.service.Logout(c.UserContext(), userID); err != nil {
		return authError(c, err)
	}

	return c.JSON(model.MessageResponse{Message: "logged out successfully"})
}

func (h *AuthHandler) Me(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	user, err := h.service.GetProfile(c.UserContext(), userID)
	if err != nil {
		return authError(c, err)
	}

	return c.JSON(user)
}

func (h *AuthHandler) ForgotPassword(c *fiber.Ctx) error {
	var req model.ForgotPasswordRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	req.Normalize()

	if err := validator.ValidateStruct(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if err := h.service.ForgotPassword(c.UserContext(), req); err != nil {
		return authError(c, err)
	}

	return c.JSON(model.MessageResponse{
		Message: "if the email is registered, a password reset link has been sent",
	})
}

func (h *AuthHandler) ResetPassword(c *fiber.Ctx) error {
	var req model.ResetPasswordRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if err := validator.ValidateStruct(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if err := h.service.ResetPassword(c.UserContext(), req); err != nil {
		return authError(c, err)
	}

	return c.JSON(model.MessageResponse{Message: "password has been reset, please log in again"})
}

func authError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrInvalidCredentials),
		errors.Is(err, service.ErrInvalidToken):
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})

	case errors.Is(err, service.ErrEmailAlreadyUsed):
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})

	case errors.Is(err, service.ErrWeakPassword),
		errors.Is(err, service.ErrPasswordTooLong):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})

	case errors.Is(err, service.ErrUserNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})

	default:
		log.Printf("auth handler error on %s %s: %v", c.Method(), c.Path(), err)
		return c.Status(fiber.StatusInternalServerError).
			JSON(fiber.Map{"error": "internal server error"})
	}
}
