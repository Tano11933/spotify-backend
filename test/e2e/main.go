package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"spotify-backend/internal/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		// The browser runner closes this pipe even when Playwright fails.
		_, _ = io.Copy(io.Discard, os.Stdin)
		stop()
	}()
	if err := serve(ctx); err != nil {
		log.Fatal(err)
	}
}

func serve(ctx context.Context) (result error) {
	setup, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	pg, err := tcpostgres.Run(setup, "postgres:16",
		tcpostgres.WithDatabase("melodia_e2e"),
		tcpostgres.WithUsername("e2e"),
		tcpostgres.WithPassword(uuid.NewString()),
		tcpostgres.BasicWaitStrategies(),
		loopbackPort("5432/tcp"),
	)
	defer func() { result = errors.Join(result, testcontainers.TerminateContainer(pg)) }()
	if err != nil {
		return fmt.Errorf("start E2E PostgreSQL: %w", err)
	}
	dsn, err := pg.ConnectionString(setup, "sslmode=disable")
	if err != nil {
		return err
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{TranslateError: true})
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	cache, err := tcredis.Run(setup, "redis:7-alpine", loopbackPort("6379/tcp"))
	defer func() { result = errors.Join(result, testcontainers.TerminateContainer(cache)) }()
	if err != nil {
		return fmt.Errorf("start E2E Redis: %w", err)
	}
	addr, err := cache.Endpoint(setup, "")
	if err != nil {
		return err
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	defer rdb.Close()

	media, err := os.MkdirTemp("", "melodia-e2e-media-")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, os.RemoveAll(media)) }()

	mail := &mailbox{messages: make(map[string]string)}
	const frontend = "http://127.0.0.1:15173"
	api, err := app.New(app.Config{
		DB: db, Redis: rdb, Mailer: mail, StorageDir: media,
		JWTSecret: uuid.NewString(), JWTRefreshSecret: uuid.NewString(),
		AccessTTL: 20 * time.Second, RefreshTTL: time.Hour,
		CacheTTL: time.Minute, ResetTokenTTL: time.Minute,
		FrontendURL: frontend, CORSOrigins: frontend,
	})
	if err != nil {
		return err
	}
	stopHub := sync.OnceFunc(api.Shutdown)
	defer stopHub()
	if err := api.Migrate(); err != nil {
		return err
	}
	if err := seed(setup, db, media); err != nil {
		return err
	}
	api.Fiber.Get("/__e2e/mail", mail.read)

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer listener.Close()
	done := make(chan error, 1)
	go func() { done <- api.Fiber.Listener(listener) }()
	fmt.Printf("E2E_READY http://%s\n", listener.Addr())

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		stopHub()
		return api.Fiber.ShutdownWithTimeout(5 * time.Second)
	}
}

func loopbackPort(port string) testcontainers.CustomizeRequestOption {
	return testcontainers.WithHostConfigModifier(func(config *container.HostConfig) {
		config.PortBindings = network.PortMap{
			network.MustParsePort(port): {{HostIP: netip.MustParseAddr("127.0.0.1")}},
		}
	})
}

type mailbox struct {
	mu       sync.Mutex
	messages map[string]string
}

func (m *mailbox) Send(_ context.Context, to, _, text, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages[to] = text
	return nil
}

func (m *mailbox) read(c *fiber.Ctx) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	text, ok := m.messages[c.Query("to")]
	if !ok {
		return c.SendStatus(fiber.StatusNotFound)
	}
	return c.Type("txt").SendString(text)
}
