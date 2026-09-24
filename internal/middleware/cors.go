package middleware

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

const DefaultAllowedOrigins = "http://localhost:5173,http://localhost:3000"

// AllowedOriginsFromEnv membaca CORS_ALLOWED_ORIGINS dari environment.
//
// Wildcard ditolak saat startup, bukan saat request: dengan AllowCredentials
// aktif, "*" berarti situs mana pun boleh mengirim request ber-kredensial dan
// membaca hasilnya. Lebih baik gagal start dengan pesan jelas.
func AllowedOriginsFromEnv() (string, error) {
	origins := strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGINS"))
	if origins == "" {
		return DefaultAllowedOrigins, nil
	}

	if strings.Contains(origins, "*") {
		return "", fmt.Errorf(
			"CORS_ALLOWED_ORIGINS tidak boleh mengandung '*' selama AllowCredentials aktif. " +
				"Tulis daftar origin secara eksplisit, dipisah koma, " +
				"contoh: " + DefaultAllowedOrigins,
		)
	}

	return origins, nil
}

// CORS menerima daftar origin yang sudah divalidasi — pembacaannya dari
// environment dilakukan AllowedOriginsFromEnv, supaya test bisa menentukan
// originnya sendiri tanpa menyentuh env.
func CORS(origins string) fiber.Handler {
	log.Printf("CORS allowed origins: %s", origins)

	return cors.New(cors.Config{
		AllowOrigins: origins,

		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
		AllowMethods: "GET,POST,PUT,DELETE,OPTIONS",

		AllowCredentials: true,
	})
}
