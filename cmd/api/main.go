package main

import (
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/joho/godotenv"

	"spotify-backend/internal/handler"
	"spotify-backend/internal/middleware"
	"spotify-backend/internal/model"
	"spotify-backend/internal/repository"
	"spotify-backend/internal/router"
	"spotify-backend/internal/service"
	ws "spotify-backend/internal/websocket"
	"spotify-backend/pkg/cache"
	"spotify-backend/pkg/database"
	jwtpkg "spotify-backend/pkg/jwt"
	"spotify-backend/pkg/mailer"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using system env")
	}

	// --- Infrastruktur ----------------------------------------------------
	db := database.ConnectPostgres()
	rdb := cache.ConnectRedis()

	// Album sebelumnya tidak disebut di sini dan tabelnya hanya kebetulan
	// terbuat karena ikut termigrasi lewat relasi Song.Album. Sekarang semua
	// model didaftarkan eksplisit, urut dari yang tidak punya dependensi.
	if err := db.AutoMigrate(
		&model.User{},
		&model.Artist{},
		&model.Album{},
		&model.Song{},
		// Playlist terakhir: ia bergantung pada User (pemilik) dan Song (lewat
		// tabel perantara playlist_songs, yang dibuat GORM di langkah ini juga).
		&model.Playlist{},
	); err != nil {
		log.Fatal("Failed to run migrations: ", err)
	}

	cacheStore := cache.NewStore(rdb)
	cacheTTL := envDuration("CACHE_TTL", 5*time.Minute)

	// --- WebSocket hub ----------------------------------------------------
	//
	// Hub harus jalan sebagai goroutine terpisah dan dibuat SEBELUM service,
	// karena SongService menerimanya sebagai dependency untuk broadcast
	// song:created.
	hub := ws.NewHub()
	go hub.Run()

	// --- JWT --------------------------------------------------------------
	jwtManager := jwtpkg.NewManager(
		mustEnv("JWT_SECRET"),
		mustEnv("JWT_REFRESH_SECRET"),
		envDuration("JWT_ACCESS_TTL", 15*time.Minute),
		envDuration("JWT_REFRESH_TTL", 7*24*time.Hour),
	)

	// --- Mailer -----------------------------------------------------------
	resetTokenTTL := envDuration("RESET_TOKEN_TTL", 15*time.Minute)
	mailService := service.NewMailService(buildMailer(), os.Getenv("FRONTEND_URL"), resetTokenTTL)

	// --- Repository → Service → Handler -----------------------------------
	//
	// Semua dependency dirangkai di sini dan hanya di sini. Tidak ada satu pun
	// variabel global di seluruh proyek — setiap komponen menerima yang ia
	// butuhkan lewat constructor. Itulah yang membuat tiap layer bisa dites
	// dengan dependency tiruan.
	userRepo := repository.NewUserRepository(db)
	tokenRepo := repository.NewTokenRepository(rdb)
	artistRepo := repository.NewArtistRepository(db)
	albumRepo := repository.NewAlbumRepository(db)
	songRepo := repository.NewSongRepository(db)
	playlistRepo := repository.NewPlaylistRepository(db)

	authService := service.NewAuthService(userRepo, tokenRepo, jwtManager, mailService, resetTokenTTL)
	artistService := service.NewArtistService(artistRepo, cacheStore, cacheTTL)
	albumService := service.NewAlbumService(albumRepo, artistRepo, cacheStore, cacheTTL)
	songService := service.NewSongService(songRepo, artistRepo, cacheStore, hub)
	playlistService := service.NewPlaylistService(playlistRepo, songRepo)

	h := &router.Handlers{
		Artist:   handler.NewArtistHandler(artistService),
		Song:     handler.NewSongHandler(songService),
		Album:    handler.NewAlbumHandler(albumService),
		Auth:     handler.NewAuthHandler(authService),
		Playlist: handler.NewPlaylistHandler(playlistService),
		WS:       handler.NewWSHandler(hub, songService),
	}

	mw := &router.Middlewares{
		Auth:      middleware.NewAuthMiddleware(jwtManager),
		RateLimit: middleware.NewRateLimiter(rdb),
	}

	app := fiber.New(fiber.Config{
		AppName: "Spotify Clone API",

		// Batasi ukuran body request. Default Fiber 4MB; endpoint di sini hanya
		// menerima JSON kecil, jadi 1MB sudah lebih dari cukup dan sekaligus
		// mengurangi permukaan serangan.
		BodyLimit: 1 * 1024 * 1024,

		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	})

	// CORS harus dipasang SEBELUM route didaftarkan.
	//
	// Fiber menjalankan middleware sesuai urutan pendaftaran. Kalau app.Use ini
	// ditaruh setelah SetupRoutes, request preflight OPTIONS akan lebih dulu
	// tertangkap oleh route matcher (dan dibalas 405) sebelum sempat sampai ke
	// middleware CORS.
	app.Use(middleware.CORS())

	router.SetupRoutes(app, h, mw)

	startWithGracefulShutdown(app, hub)
}

// startWithGracefulShutdown menjalankan server dan menunggu sinyal berhenti.
//
// Kenapa perlu? Kalau proses langsung dibunuh, request yang sedang diproses
// terputus di tengah jalan dan koneksi WebSocket mati tanpa frame close yang
// benar. Graceful shutdown memberi waktu semuanya beres dulu.
//
// Bagian ini juga contoh bagus pemakaian channel di luar konteks WebSocket.
func startWithGracefulShutdown(app *fiber.App, hub *ws.Hub) {
	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "9000"
	}

	// Channel untuk menerima error dari goroutine server. Tanpa ini, kegagalan
	// Listen (misal port sudah dipakai) akan hilang tanpa jejak karena
	// terjadi di goroutine lain.
	serverErr := make(chan error, 1)

	go func() {
		// app.Listen memblokir, jadi harus di goroutine — kalau tidak, kode di
		// bawahnya tidak akan pernah tereksekusi.
		serverErr <- app.Listen(":" + port)
	}()

	// signal.Notify menyambungkan sinyal OS ke sebuah channel. SIGINT adalah
	// Ctrl+C, SIGTERM adalah yang dikirim Docker/Kubernetes saat menghentikan
	// container.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	// select di sini menunggu MANA SAJA yang datang lebih dulu: server gagal,
	// atau ada sinyal berhenti.
	select {
	case err := <-serverErr:
		log.Fatal("Server failed: ", err)

	case sig := <-quit:
		log.Printf("Received %s, shutting down gracefully...", sig)

		// Hentikan hub lebih dulu supaya semua koneksi WebSocket menerima frame
		// close yang benar, bukan sekadar terputus.
		hub.Stop()

		if err := app.ShutdownWithTimeout(10 * time.Second); err != nil {
			log.Printf("Forced shutdown: %v", err)
		}
		log.Println("Server stopped")
	}
}

// buildMailer memilih implementasi mailer berdasarkan konfigurasi.
//
// Kalau SMTP_HOST kosong, aplikasi TIDAK gagal start — ia memakai LogMailer yang
// menulis email ke terminal. Ini disengaja: kamu bisa menguji seluruh alur reset
// password tanpa kredensial Mailtrap, tinggal copy link dari log.
func buildMailer() service.Mailer {
	host := os.Getenv("SMTP_HOST")
	if host == "" {
		log.Println("⚠️  SMTP_HOST is empty — using LogMailer (emails will be printed to this terminal)")
		return mailer.NewLogMailer()
	}

	port, err := strconv.Atoi(os.Getenv("SMTP_PORT"))
	if err != nil {
		log.Fatalf("Invalid SMTP_PORT %q: %v", os.Getenv("SMTP_PORT"), err)
	}

	smtpMailer, err := mailer.NewSMTPMailer(mailer.Config{
		Host:       host,
		Port:       port,
		Username:   os.Getenv("SMTP_USERNAME"),
		Password:   os.Getenv("SMTP_PASSWORD"),
		FromEmail:  os.Getenv("SMTP_FROM_EMAIL"),
		FromName:   os.Getenv("SMTP_FROM_NAME"),
		Encryption: mailer.Encryption(os.Getenv("SMTP_ENCRYPTION")),
	})
	if err != nil {
		log.Fatal("Failed to configure SMTP mailer: ", err)
	}

	log.Printf("✅ SMTP mailer configured (%s:%d)", host, port)
	return smtpMailer
}

// mustEnv membaca env yang WAJIB ada, dan mematikan aplikasi kalau tidak.
//
// Ini penting khusus untuk JWT secret. Kalau nilai kosong dibiarkan lolos,
// aplikasi tetap jalan tapi menandatangani semua token dengan secret kosong —
// artinya siapa pun bisa membuat token palsu yang valid. Lebih baik crash saat
// startup dengan pesan jelas daripada berjalan dalam keadaan tidak aman.
func mustEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("Required environment variable %s is not set", key)
	}
	return value
}

// envDuration membaca durasi bergaya Go ("15m", "168h", "5m30s") dengan
// nilai default kalau kosong atau tidak valid.
func envDuration(key string, fallback time.Duration) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}

	d, err := time.ParseDuration(raw)
	if err != nil {
		log.Printf("Invalid duration %s=%q, using default %s", key, raw, fallback)
		return fallback
	}
	return d
}
