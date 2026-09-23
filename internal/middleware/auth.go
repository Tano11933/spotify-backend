package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"spotify-backend/internal/model"
	jwtpkg "spotify-backend/pkg/jwt"
	"spotify-backend/pkg/response"
)

const (
	ContextUserID   = "userID"
	ContextUserRole = "userRole"
)

type AuthMiddleware struct {
	jwt *jwtpkg.Manager
}

func NewAuthMiddleware(jwtManager *jwtpkg.Manager) *AuthMiddleware {
	return &AuthMiddleware{jwt: jwtManager}
}

func (m *AuthMiddleware) Protected() fiber.Handler {
	return m.protect(false)
}

func (m *AuthMiddleware) ProtectedAllowQueryToken() fiber.Handler {
	return m.protect(true)
}

func (m *AuthMiddleware) protect(allowQueryToken bool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		raw := bearerToken(c.Get(fiber.HeaderAuthorization))

		if raw == "" && allowQueryToken {
			raw = strings.TrimSpace(c.Query("token"))
		}

		if raw == "" {
			return response.Unauthorized(c, "missing authorization token")
		}

		claims, err := m.jwt.ParseAccessToken(raw)
		if err != nil {
			return response.Unauthorized(c, "invalid or expired token")
		}

		userID, err := claims.UserID()
		if err != nil {
			return response.Unauthorized(c, "invalid or expired token")
		}

		c.Locals(ContextUserID, userID)
		c.Locals(ContextUserRole, claims.Role)

		return c.Next()
	}
}

func (m *AuthMiddleware) RequireAdmin() fiber.Handler {
	return func(c *fiber.Ctx) error {
		role, ok := c.Locals(ContextUserRole).(string)
		if !ok || role != string(model.RoleAdmin) {
			return response.Forbidden(c, "admin access required")
		}
		return c.Next()
	}
}

func UserIDFromContext(c *fiber.Ctx) (uuid.UUID, bool) {
	id, ok := c.Locals(ContextUserID).(uuid.UUID)
	if !ok || id == uuid.Nil {
		return uuid.Nil, false
	}
	return id, true
}

// bearerToken mengambil bagian token dari header "Bearer <token>".
func bearerToken(header string) string {
	const prefix = "Bearer "

	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}
