package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"spotify-backend/internal/app"
	"spotify-backend/internal/middleware"
	"spotify-backend/pkg/cache"
	"spotify-backend/pkg/database"
	"spotify-backend/pkg/logging"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using system env")
	}

	// Logger disiapkan paling awal supaya log startup (koneksi DB/Redis, dsb)
	// ikut terstruktur. LOG_FORMAT=json untuk produksi; default text.
	logging.Setup(os.Getenv("LOG_FORMAT"))

	// --- Infrastruktur ----------------------------------------------------
	db := database.ConnectPostgres()
	rdb := cache.ConnectRedis()

	origins, err := middleware.AllowedOriginsFromEnv()
	if err != nil {
		log.Fatal(err)
	}

	mail, err := app.MailerFromEnv()
	if err != nil {
		log.Fatal("Failed to configure mailer: ", err)
	}

	// --- Rakit aplikasi ---------------------------------------------------
	//
	// Semua dependency dirangkai di internal/app — main hanya membaca
	// environment dan menyalakan server. Itu yang membuat test integrasi bisa
	// membangun aplikasi yang sama di atas container Postgres/Redis.
	application, err := app.New(app.Config{
		DB:    db,
		Redis: rdb,

		JWTSecret:        mustEnv("JWT_SECRET"),
		JWTRefreshSecret: mustEnv("JWT_REFRESH_SECRET"),
		AccessTTL:        app.EnvDuration("JWT_ACCESS_TTL", 15*time.Minute),
		RefreshTTL:       app.EnvDuration("JWT_REFRESH_TTL", 7*24*time.Hour),

		CacheTTL:      app.EnvDuration("CACHE_TTL", 5*time.Minute),
		ResetTokenTTL: app.EnvDuration("RESET_TOKEN_TTL", 15*time.Minute),

		FrontendURL: os.Getenv("FRONTEND_URL"),
		CORSOrigins: origins,
		Mailer:      mail,
		StorageDir:  os.Getenv("UPLOAD_DIR"),
	})
	if err != nil {
		log.Fatal("Failed to build app: ", err)
	}

	// Migrasi tabel berjalan otomatis via GORM AutoMigrate saat startup.
	if err := application.Migrate(); err != nil {
		log.Fatal("Failed to run migrations: ", err)
	}

	startWithGracefulShutdown(application)
}

// startWithGracefulShutdown menjalankan server dan menunggu sinyal berhenti.
//
// Kenapa perlu? Kalau proses langsung dibunuh, request yang sedang diproses
// terputus di tengah jalan dan koneksi WebSocket mati tanpa frame close yang
// benar. Graceful shutdown memberi waktu semuanya beres dulu.
func startWithGracefulShutdown(application *app.App) {
	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "9000"
	}

	// Channel untuk menerima error dari goroutine server. Tanpa ini, kegagalan
	// Listen (misal port sudah dipakai) akan hilang tanpa jejak karena
	// terjadi di goroutine lain.
	serverErr := make(chan error, 1)

	go func() {
		// Listen memblokir, jadi harus di goroutine — kalau tidak, kode di
		// bawahnya tidak akan pernah tereksekusi.
		serverErr <- application.Fiber.Listen(":" + port)
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
		application.Shutdown()

		if err := application.Fiber.ShutdownWithTimeout(10 * time.Second); err != nil {
			log.Printf("Forced shutdown: %v", err)
		}
		log.Println("Server stopped")
	}
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
