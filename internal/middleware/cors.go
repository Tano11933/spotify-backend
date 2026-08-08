package middleware

import (
	"log"
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

const defaultAllowedOrigins = "http://localhost:5173,http://localhost:3000"

func CORS() fiber.Handler {
	origins := allowedOrigins()
	log.Printf("CORS allowed origins: %s", origins)

	return cors.New(cors.Config{
		AllowOrigins: origins,

		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
		AllowMethods: "GET,POST,PUT,DELETE,OPTIONS",

		AllowCredentials: true,
	})
}

func allowedOrigins() string {
	origins := strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGINS"))
	if origins == "" {
		return defaultAllowedOrigins
	}

	if strings.Contains(origins, "*") {
		log.Fatal(
			"CORS_ALLOWED_ORIGINS tidak boleh mengandung '*' selama AllowCredentials aktif. " +
				"Tulis daftar origin secara eksplisit, dipisah koma, " +
				"contoh: " + defaultAllowedOrigins,
		)
	}

	return origins
}
