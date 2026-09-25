// Package app merangkai seluruh dependency aplikasi di satu tempat.
//
// Sebelumnya wiring ini tinggal di cmd/api/main.go — tidak bisa dipakai ulang
// oleh test integrasi karena berada di package main. Dengan dipindah ke sini,
// main hanya bertugas membaca environment dan menyalakan server, sementara
// test bisa membangun aplikasi lengkap di atas Postgres/Redis container.
package app

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"spotify-backend/internal/handler"
	"spotify-backend/internal/middleware"
	"spotify-backend/internal/migrations"
	"spotify-backend/internal/model"
	"spotify-backend/internal/repository"
	"spotify-backend/internal/router"
	"spotify-backend/internal/service"
	ws "spotify-backend/internal/websocket"
	"spotify-backend/pkg/cache"
	jwtpkg "spotify-backend/pkg/jwt"
)

const (
	defaultBodyLimit    = 1 * 1024 * 1024
	defaultReadTimeout  = 15 * time.Second
	defaultWriteTimeout = 15 * time.Second
)

// Config berisi seluruh nilai yang biasanya dibaca dari environment.
// DB dan Redis di-inject supaya test bisa memakai container-nya sendiri.
type Config struct {
	DB    *gorm.DB
	Redis *redis.Client

	JWTSecret        string
	JWTRefreshSecret string
	AccessTTL        time.Duration
	RefreshTTL       time.Duration

	CacheTTL      time.Duration
	ResetTokenTTL time.Duration

	FrontendURL string
	CORSOrigins string
	Mailer      service.Mailer

	BodyLimit    int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// App adalah komponen yang sudah terangkai dan siap dipakai.
type App struct {
	Fiber *fiber.App
	Hub   *ws.Hub
	DB    *gorm.DB
	Redis *redis.Client
}

// New merangkai repository → service → handler → router di atas Fiber.
func New(cfg Config) (*App, error) {
	if cfg.DB == nil {
		return nil, fmt.Errorf("app: DB is required")
	}
	if cfg.Redis == nil {
		return nil, fmt.Errorf("app: Redis is required")
	}
	if cfg.Mailer == nil {
		return nil, fmt.Errorf("app: Mailer is required")
	}
	if cfg.BodyLimit == 0 {
		cfg.BodyLimit = defaultBodyLimit
	}
	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = defaultReadTimeout
	}
	if cfg.WriteTimeout == 0 {
		cfg.WriteTimeout = defaultWriteTimeout
	}

	// Hub harus jalan sebagai goroutine terpisah dan dibuat SEBELUM service,
	// karena SongService menerimanya sebagai dependency untuk broadcast.
	hub := ws.NewHub()
	go hub.Run()

	jwtManager := jwtpkg.NewManager(
		cfg.JWTSecret,
		cfg.JWTRefreshSecret,
		cfg.AccessTTL,
		cfg.RefreshTTL,
	)

	cacheStore := cache.NewStore(cfg.Redis)
	mailService := service.NewMailService(cfg.Mailer, cfg.FrontendURL, cfg.ResetTokenTTL)

	userRepo := repository.NewUserRepository(cfg.DB)
	tokenRepo := repository.NewTokenRepository(cfg.Redis)
	artistRepo := repository.NewArtistRepository(cfg.DB)
	albumRepo := repository.NewAlbumRepository(cfg.DB)
	songRepo := repository.NewSongRepository(cfg.DB)
	playlistRepo := repository.NewPlaylistRepository(cfg.DB)
	searchRepo := repository.NewSearchRepository(cfg.DB)
	libraryRepo := repository.NewLibraryRepository(cfg.DB)

	authService := service.NewAuthService(userRepo, tokenRepo, jwtManager, mailService, cfg.ResetTokenTTL)
	artistService := service.NewArtistService(artistRepo, cacheStore, cfg.CacheTTL)
	albumService := service.NewAlbumService(albumRepo, artistRepo, cacheStore, cfg.CacheTTL)
	songService := service.NewSongService(songRepo, artistRepo, cacheStore, hub)
	playlistService := service.NewPlaylistService(playlistRepo, songRepo)
	searchService := service.NewSearchService(searchRepo)
	libraryService := service.NewLibraryService(libraryRepo, songRepo, albumRepo, artistRepo)

	h := &router.Handlers{
		Artist:   handler.NewArtistHandler(artistService),
		Song:     handler.NewSongHandler(songService),
		Album:    handler.NewAlbumHandler(albumService),
		Auth:     handler.NewAuthHandler(authService),
		Playlist: handler.NewPlaylistHandler(playlistService),
		Search:   handler.NewSearchHandler(searchService),
		Library:  handler.NewLibraryHandler(libraryService),
		WS:       handler.NewWSHandler(hub, songService),
	}

	mw := &router.Middlewares{
		Auth:      middleware.NewAuthMiddleware(jwtManager),
		RateLimit: middleware.NewRateLimiter(cfg.Redis),
	}

	fiberApp := fiber.New(fiber.Config{
		AppName: "Spotify Clone API",

		// Batasi ukuran body request. Default Fiber 4MB; endpoint di sini hanya
		// menerima JSON kecil, jadi 1MB sudah lebih dari cukup.
		BodyLimit: cfg.BodyLimit,

		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	})

	// CORS harus dipasang SEBELUM route didaftarkan. Fiber menjalankan
	// middleware sesuai urutan pendaftaran; kalau dipasang belakangan, request
	// preflight OPTIONS akan lebih dulu tertangkap route matcher (405).
	fiberApp.Use(middleware.CORS(cfg.CORSOrigins))

	router.SetupRoutes(fiberApp, h, mw)

	return &App{Fiber: fiberApp, Hub: hub, DB: cfg.DB, Redis: cfg.Redis}, nil
}

// Migrate menjalankan AutoMigrate untuk seluruh model, lalu migrasi SQL yang
// tidak bisa diungkapkan AutoMigrate (extension & index search).
//
// Urut dari yang tidak punya dependensi: Playlist terakhir karena bergantung
// pada User dan Song.
func (a *App) Migrate() error {
	if err := a.DB.AutoMigrate(
		&model.User{},
		&model.Artist{},
		&model.Album{},
		&model.Song{},
		&model.Playlist{},
		// Tabel library: bergantung pada User, Song, Album, dan Artist.
		&model.SavedTrack{},
		&model.SavedAlbum{},
		&model.FollowedArtist{},
	); err != nil {
		return err
	}

	return migrations.Run(a.DB)
}

// Shutdown menghentikan hub lebih dulu supaya semua koneksi WebSocket menerima
// frame close yang benar, bukan sekadar terputus.
func (a *App) Shutdown() {
	a.Hub.Stop()
}
