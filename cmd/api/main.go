package main

import (
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/joho/godotenv"

	"spotify-backend/internal/handler"
	"spotify-backend/internal/model"
	"spotify-backend/internal/repository"
	"spotify-backend/internal/router"
	"spotify-backend/internal/service"
	"spotify-backend/pkg/cache"
	"spotify-backend/pkg/database"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using system env")
	}

	db := database.ConnectPostgres()
	rdb := cache.ConnectRedis()
	_ = rdb

	db.AutoMigrate(&model.Artist{}, &model.Song{})

	artistRepo := repository.NewArtistRepository(db)
	artistService := service.NewArtistService(artistRepo)
	artistHandler := handler.NewArtistHandler(artistService)

	albumRepo := repository.NewAlbumRepository(db)
	albumService := service.NewAlbumService(albumRepo, artistRepo)
	albumHandler := handler.NewAlbumHandler(albumService)

	songRepo := repository.NewSongRepository(db)
	songService := service.NewSongService(songRepo, artistRepo)
	songHandler := handler.NewSongHandler(songService)

	h := &router.Handlers{
		Artist: artistHandler,
		Song:   songHandler,
		Album:  albumHandler,
	}

	app := fiber.New()
	router.SetupRoutes(app, h)

	log.Fatal(app.Listen(":" + os.Getenv("APP_PORT")))
}