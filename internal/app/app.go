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
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"spotify-backend/internal/handler"
	"spotify-backend/internal/middleware"
	"spotify-backend/internal/migrations"
	"spotify-backend/internal/model"
	"spotify-backend/internal/queue"
	"spotify-backend/internal/repository"
	"spotify-backend/internal/router"
	"spotify-backend/internal/service"
	ws "spotify-backend/internal/websocket"
	"spotify-backend/pkg/cache"
	jwtpkg "spotify-backend/pkg/jwt"
	"spotify-backend/pkg/storage"
)

const (
	// BodyLimit lebih besar dari ukuran berkas maksimum karena body multipart
	// menambahkan boundary + header di sekitar berkas. BodyLimit berlaku
	// server-wide (Fiber meneruskannya ke MaxRequestBodySize fasthttp) — batas
	// per endpoint JSON tetap dijaga validasi ukuran muatan.
	defaultBodyLimit     = 21 * 1024 * 1024
	defaultMaxUploadSize = 20 * 1024 * 1024

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

	// QueueName adalah nama antrean job Redis. Kosong berarti "jobs".
	QueueName string

	FrontendURL string
	CORSOrigins string
	Mailer      service.Mailer

	// StorageDir adalah direktori penyimpanan berkas unggahan (audio).
	// Kosong berarti "./uploads".
	StorageDir string

	// MaxUploadBytes membatasi ukuran satu berkas audio. Kosong berarti 20MB.
	MaxUploadBytes int64

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
	if cfg.StorageDir == "" {
		cfg.StorageDir = "./uploads"
	}
	if cfg.MaxUploadBytes == 0 {
		cfg.MaxUploadBytes = defaultMaxUploadSize
	}

	// Hub harus jalan sebagai goroutine terpisah dan dibuat SEBELUM service,
	// karena SongService menerimanya sebagai dependency untuk broadcast.
	//
	// Broker Redis membuat event hub sampai ke client yang terhubung ke
	// instance lain; instanceID menandai asal pesan supaya instance pengirim
	// tidak memproses ulang event-nya sendiri.
	hub := ws.NewHub()
	hub.SetBroker(ws.NewRedisBroker(cfg.Redis, ws.DefaultBrokerChannel), uuid.NewString())
	go hub.Run()

	// Storage menulis ke disk lokal. Ganti implementasinya (mis. S3/MinIO)
	// tanpa menyentuh service — semuanya lewat interface storage.Storage.
	fileStorage, err := storage.NewLocal(cfg.StorageDir)
	if err != nil {
		return nil, fmt.Errorf("app: %w", err)
	}

	jwtManager := jwtpkg.NewManager(
		cfg.JWTSecret,
		cfg.JWTRefreshSecret,
		cfg.AccessTTL,
		cfg.RefreshTTL,
	)

	cacheStore := cache.NewStore(cfg.Redis)

	// Mailer dibungkus: kegagalan kirim pertama diantrekan untuk dicoba ulang
	// worker, sementara SMTP yang sehat tidak berubah perilakunya.
	jobQueue := queue.New(cfg.Redis, cfg.QueueName)
	mailService := service.NewMailService(
		newRetryingMailer(cfg.Mailer, jobQueue),
		cfg.FrontendURL,
		cfg.ResetTokenTTL,
	)

	userRepo := repository.NewUserRepository(cfg.DB)
	tokenRepo := repository.NewTokenRepository(cfg.Redis)
	artistRepo := repository.NewArtistRepository(cfg.DB)
	albumRepo := repository.NewAlbumRepository(cfg.DB)
	songRepo := repository.NewSongRepository(cfg.DB)
	playlistRepo := repository.NewPlaylistRepository(cfg.DB)
	searchRepo := repository.NewSearchRepository(cfg.DB)
	libraryRepo := repository.NewLibraryRepository(cfg.DB)
	playerRepo := repository.NewPlayerRepository(cfg.DB)
	followRepo := repository.NewFollowRepository(cfg.DB)
	activityRepo := repository.NewActivityRepository(cfg.DB)
	notificationRepo := repository.NewNotificationRepository(cfg.DB)
	genreRepo := repository.NewGenreRepository(cfg.DB)
	recommendationRepo := repository.NewRecommendationRepository(cfg.DB)
	chartRepo := repository.NewChartRepository(cfg.DB)

	authService := service.NewAuthService(userRepo, tokenRepo, jwtManager, mailService, cfg.ResetTokenTTL)
	artistService := service.NewArtistService(artistRepo, cacheStore, cfg.CacheTTL)
	albumService := service.NewAlbumService(albumRepo, artistRepo, cacheStore, cfg.CacheTTL)
	songService := service.NewSongService(songRepo, artistRepo, cacheStore, hub)
	feedService := service.NewFeedService(activityRepo, userRepo, songRepo, playlistRepo)
	notificationService := service.NewNotificationService(notificationRepo, userRepo, hub)

	playlistService := service.NewPlaylistService(playlistRepo, songRepo, feedService)
	searchService := service.NewSearchService(searchRepo)
	libraryService := service.NewLibraryService(libraryRepo, songRepo, albumRepo, artistRepo)
	playerService := service.NewPlayerService(playerRepo, songRepo, hub, feedService)
	mediaService := service.NewMediaService(songRepo, fileStorage, cfg.MaxUploadBytes)
	socialService := service.NewSocialService(followRepo, userRepo, playlistRepo, notificationService)
	genreService := service.NewGenreService(genreRepo, artistRepo, cacheStore, cfg.CacheTTL)
	recommendationService := service.NewRecommendationService(recommendationRepo, artistRepo)
	// TTL penanda refresh chart memakai CacheTTL yang sama dengan cache katalog;
	// di test integrasi nilainya 1 menit sehingga data test cepat terlihat.
	chartService := service.NewChartService(chartRepo, songRepo, cfg.Redis, cfg.CacheTTL)

	h := &router.Handlers{
		Artist:         handler.NewArtistHandler(artistService),
		Song:           handler.NewSongHandler(songService),
		Album:          handler.NewAlbumHandler(albumService),
		Auth:           handler.NewAuthHandler(authService),
		Playlist:       handler.NewPlaylistHandler(playlistService),
		Search:         handler.NewSearchHandler(searchService),
		Library:        handler.NewLibraryHandler(libraryService),
		Player:         handler.NewPlayerHandler(playerService),
		Media:          handler.NewMediaHandler(mediaService),
		Social:         handler.NewSocialHandler(socialService),
		Feed:           handler.NewFeedHandler(feedService),
		Notification:   handler.NewNotificationHandler(notificationService),
		Genre:          handler.NewGenreHandler(genreService),
		Recommendation: handler.NewRecommendationHandler(recommendationService),
		Chart:          handler.NewChartHandler(chartService),
		WS:             handler.NewWSHandler(hub, songService),
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
		// Genre didaftarkan sebelum Artist karena tabel artist_genres
		// (many2many) membutuhkan tabel genres lebih dulu.
		&model.Genre{},
		&model.Artist{},
		&model.Album{},
		&model.Song{},
		&model.Playlist{},
		// Tabel library: bergantung pada User, Song, Album, dan Artist.
		&model.SavedTrack{},
		&model.SavedAlbum{},
		&model.FollowedArtist{},
		// Tabel playback: bergantung pada User dan Song.
		&model.PlayerState{},
		&model.QueueItem{},
		&model.PlayHistory{},
		// Relasi sosial antar user.
		&model.UserFollow{},
		// Feed aktivitas & notifikasi.
		&model.Activity{},
		&model.Notification{},
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
