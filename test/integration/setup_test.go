//go:build integration

// Package integration menjalankan test terhadap aplikasi lengkap di atas
// Postgres dan Redis sungguhan — container sementara yang dinyalakan
// testcontainers, bukan tiruan.
//
// Jalankan dengan:
//
//	go test -tags=integration ./test/integration/...
//
// Butuh Docker yang berjalan. Unit test biasa (`go test ./...`) tidak
// menyentuh package ini karena build tag.
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"spotify-backend/internal/app"
	"spotify-backend/internal/model"
)

var (
	testApp *app.App
	db      *gorm.DB

	// ID fixture — dipakai test supaya tidak perlu mencari lewat API.
	fixtureArtistID   uint
	fixtureAlbumID    uint
	fixturePlaylistID uint
)

// silentMailer menelan email supaya output test tidak penuh isi reset password.
type silentMailer struct{}

func (silentMailer) Send(context.Context, string, string, string, string) error { return nil }

func TestMain(m *testing.M) {
	ctx := context.Background()

	pgContainer, err := tcpostgres.Run(ctx, "postgres:16",
		tcpostgres.WithDatabase("spotify_test"),
		tcpostgres.WithUsername("spotify"),
		tcpostgres.WithPassword("spotify123"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gagal menyalakan container Postgres (Docker berjalan?):", err)
		os.Exit(1)
	}
	defer func() { _ = pgContainer.Terminate(ctx) }()

	dsn, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintln(os.Stderr, "gagal membaca DSN container:", err)
		os.Exit(1)
	}

	db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
		TranslateError: true,
		Logger:         logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "gagal konek ke Postgres test:", err)
		os.Exit(1)
	}

	redisContainer, err := tcredis.Run(ctx, "redis:7-alpine")
	if err != nil {
		fmt.Fprintln(os.Stderr, "gagal menyalakan container Redis:", err)
		os.Exit(1)
	}
	defer func() { _ = redisContainer.Terminate(ctx) }()

	redisAddr, err := redisContainer.Endpoint(ctx, "")
	if err != nil {
		fmt.Fprintln(os.Stderr, "gagal membaca endpoint Redis:", err)
		os.Exit(1)
	}
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})

	testApp, err = app.New(app.Config{
		DB:    db,
		Redis: rdb,

		JWTSecret:        "integration-access-secret",
		JWTRefreshSecret: "integration-refresh-secret",
		AccessTTL:        15 * time.Minute,
		RefreshTTL:       time.Hour,

		CacheTTL:      time.Minute,
		ResetTokenTTL: 15 * time.Minute,

		FrontendURL: "http://localhost:5173",
		CORSOrigins: "http://localhost:5173",
		Mailer:      silentMailer{},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "gagal merakit aplikasi:", err)
		os.Exit(1)
	}

	if err := testApp.Migrate(); err != nil {
		fmt.Fprintln(os.Stderr, "gagal menjalankan migrasi:", err)
		os.Exit(1)
	}

	if err := seedFixture(); err != nil {
		fmt.Fprintln(os.Stderr, "gagal menyiapkan fixture:", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// seedFixture mengisi data minimal: dua user (admin & biasa), dua artist,
// satu album berisi satu lagu, dan satu playlist publik.
func seedFixture() error {
	hash, err := bcrypt.GenerateFromPassword([]byte("Password123!"), bcrypt.MinCost)
	if err != nil {
		return err
	}

	admin := model.User{Name: "Admin Test", Email: "admin@test.local", PasswordHash: string(hash), Role: model.RoleAdmin}
	user := model.User{Name: "User Test", Email: "user@test.local", PasswordHash: string(hash), Role: model.RoleUser}
	if err := db.Create(&admin).Error; err != nil {
		return err
	}
	if err := db.Create(&user).Error; err != nil {
		return err
	}

	alpha := model.Artist{Name: "Alpha", Bio: "artist fixture"}
	beta := model.Artist{Name: "Beta"}
	if err := db.Create(&alpha).Error; err != nil {
		return err
	}
	if err := db.Create(&beta).Error; err != nil {
		return err
	}
	fixtureArtistID = alpha.ID

	album := model.Album{
		Title:       "First Album",
		ArtistID:    alpha.ID,
		ReleaseDate: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	if err := db.Create(&album).Error; err != nil {
		return err
	}
	fixtureAlbumID = album.ID

	song := model.Song{Title: "Alpha Song", Duration: 200, ArtistID: alpha.ID, AlbumID: &album.ID}
	if err := db.Create(&song).Error; err != nil {
		return err
	}

	playlist := model.Playlist{Name: "Public Mix", Description: "fixture", IsPublic: true, UserID: user.ID}
	if err := db.Create(&playlist).Error; err != nil {
		return err
	}
	if err := db.Model(&playlist).Association("Songs").Append(&song); err != nil {
		return err
	}
	fixturePlaylistID = playlist.ID

	return nil
}

// do mengirim request ke aplikasi Fiber tanpa membuka port (in-memory).
func do(t *testing.T, method, path, token string, body any) (int, map[string]any) {
	t.Helper()

	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	}

	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := testApp.Fiber.Test(req, -1)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	var payload map[string]any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &payload)
	}

	return resp.StatusCode, payload
}

// login mengembalikan access token untuk kredensial yang diberikan.
func login(t *testing.T, email, password string) string {
	t.Helper()

	status, payload := do(t, "POST", "/api/auth/login", "", map[string]string{
		"email":    email,
		"password": password,
	})
	if status != fiber.StatusOK {
		t.Fatalf("login %s gagal: %d %v", email, status, payload)
	}

	tokens, ok := payload["tokens"].(map[string]any)
	if !ok {
		t.Fatalf("response login tidak berisi tokens: %v", payload)
	}

	access, ok := tokens["access_token"].(string)
	if !ok || access == "" {
		t.Fatalf("access token kosong: %v", tokens)
	}
	return access
}

// items mengambil array items dari response envelope.
func items(t *testing.T, payload map[string]any) []map[string]any {
	t.Helper()

	raw, ok := payload["items"].([]any)
	if !ok {
		t.Fatalf("payload bukan envelope: %v", payload)
	}

	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		obj, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("item bukan objek: %v", item)
		}
		out = append(out, obj)
	}
	return out
}
