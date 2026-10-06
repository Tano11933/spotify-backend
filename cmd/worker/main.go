// Command worker memproses job latar di belakang API:
//
//   - email:send    : kirim ulang email yang gagal (mis. SMTP sempat down)
//   - chart:refresh : hitung ulang materialized view chart secara berkala
//
// Dijalankan sebagai proses terpisah dari API, dengan Redis dan Postgres yang
// sama. API tetap bisa melayani request walau worker mati; yang hilang hanya
// retry email dan refresh terjadwal (chart kembali memakai jalur lazy).
//
// Pemakaian:
//
//	go run ./cmd/worker
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"spotify-backend/internal/app"
	"spotify-backend/internal/queue"
	"spotify-backend/internal/repository"
	"spotify-backend/internal/worker"
	"spotify-backend/pkg/cache"
	"spotify-backend/pkg/database"
	"spotify-backend/pkg/logging"
)

const defaultChartRefreshInterval = 5 * time.Minute

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using system env")
	}

	// Logger disiapkan paling awal supaya log startup ikut terstruktur,
	// seragam dengan proses API.
	logging.Setup(os.Getenv("LOG_FORMAT"))

	db := database.ConnectPostgres()
	rdb := cache.ConnectRedis()

	mail, err := app.MailerFromEnv()
	if err != nil {
		log.Fatal("Failed to configure mailer: ", err)
	}

	jobs := queue.New(rdb, os.Getenv("WORKER_QUEUE"))
	chartRepo := repository.NewChartRepository(db)

	w := worker.New(jobs)

	w.Handle(worker.JobEmailSend, func(ctx context.Context, payload json.RawMessage) error {
		var email worker.EmailPayload
		if err := json.Unmarshal(payload, &email); err != nil {
			return fmt.Errorf("payload email tidak valid: %w", err)
		}
		return mail.Send(ctx, email.To, email.Subject, email.Text, email.HTML)
	})

	w.Handle(worker.JobChartRefresh, func(ctx context.Context, _ json.RawMessage) error {
		return chartRepo.Refresh(ctx)
	})

	// NotifyContext mengubah sinyal OS menjadi pembatalan ctx; worker
	// menyelesaikan job yang sedang berjalan lalu berhenti dengan rapi.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Refresh berkala lewat antrean (bukan langsung di goroutine ini) supaya
	// kegagalannya ikut kena retry seperti job lain. Endpoint chart tetap
	// punya jalur lazy sebagai cadangan saat worker tidak berjalan.
	interval := app.EnvDuration("CHART_REFRESH_INTERVAL", defaultChartRefreshInterval)
	go scheduleChartRefresh(ctx, jobs, interval)

	w.Run(ctx)
	log.Println("worker: shutdown selesai")
}

// scheduleChartRefresh mengantrekan satu job refresh saat start lalu setiap
// interval. Job pertama sengaja langsung dikirim supaya chart yang baru dibuka
// tidak menunggu interval penuh.
func scheduleChartRefresh(ctx context.Context, jobs *queue.Queue, interval time.Duration) {
	enqueue := func() {
		if err := jobs.Enqueue(ctx, worker.JobChartRefresh, map[string]any{}); err != nil {
			log.Printf("worker: gagal menjadwalkan refresh chart: %v", err)
		}
	}

	enqueue()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			enqueue()
		}
	}
}
