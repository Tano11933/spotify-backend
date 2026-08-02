package router

import (
	"github.com/gofiber/fiber/v2"
	"spotify-backend/internal/handler"
)

type Handlers struct {
	Artist *handler.ArtistHandler
}

func SetupRoutes(app *fiber.App, h *Handlers) {
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "message": "Spotify backend is running 🎵"})
	})

	api := app.Group("/api")

	artists := api.Group("/artists")
	artists.Post("/", h.Artist.Create)
	artists.Get("/", h.Artist.GetAll)
	artists.Get("/:id", h.Artist.GetByID)
	artists.Put("/:id", h.Artist.Update)
	artists.Delete("/:id", h.Artist.Delete)

	// nanti fitur baru (song, album, dll) tinggal ditambah di sini
}