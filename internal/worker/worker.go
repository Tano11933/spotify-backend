// Package worker menjalankan job dari antrean: mengambil satu job, memanggil
// handler sesuai tipenya, lalu mengembalikan hasilnya ke antrean (retry atau
// dead-letter). Semua kebijakan retry ada di package queue; worker hanya
// menerjemahkan sukses/gagal.
package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"spotify-backend/internal/queue"
)

// Jenis job yang dikenal. Konstanta ini dipakai dua sisi: pengirim (mis.
// retrying mailer di internal/app dan penjadwal chart di cmd/worker) dan
// handler yang didaftarkan di cmd/worker.
const (
	JobEmailSend    = "email:send"
	JobChartRefresh = "chart:refresh"
)

// EmailPayload adalah isi email yang SUDAH dirender. Worker tidak menyusun
// ulang templat: yang gagal dikirim adalah email yang sama persis.
type EmailPayload struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Text    string `json:"text"`
	HTML    string `json:"html"`
}

// Handler memproses satu job. Error yang dikembalikan memicu retry (atau
// dead-letter kalau percobaannya habis).
type Handler func(ctx context.Context, payload json.RawMessage) error

// JobSource adalah sisi antrean yang dibutuhkan worker. Dipisah sebagai
// interface supaya logika worker bisa diuji tanpa Redis.
type JobSource interface {
	Reserve(ctx context.Context, timeout time.Duration) (*queue.Job, error)
	Retry(ctx context.Context, job *queue.Job, cause error) error
	DeadLetter(ctx context.Context, job *queue.Job, cause error) error
}

// reserveTimeout pendek supaya shutdown tidak menunggu lama: BLPOP dibatalkan
// ctx, dan timeout ini hanya menentukan seberapa sering loop berputar saat
// antrean kosong.
const reserveTimeout = 2 * time.Second

type Worker struct {
	source   JobSource
	handlers map[string]Handler
}

func New(source JobSource) *Worker {
	return &Worker{source: source, handlers: make(map[string]Handler)}
}

// Handle mendaftarkan handler untuk satu jenis job. Jenis yang tidak punya
// handler langsung masuk dead-letter saat diproses, karena retry tidak akan
// pernah membuatnya berhasil.
func (w *Worker) Handle(jobType string, handler Handler) {
	w.handlers[jobType] = handler
}

// Run memproses job sampai ctx dibatalkan (mis. sinyal shutdown).
func (w *Worker) Run(ctx context.Context) {
	log.Println("worker: mulai memproses job")

	for {
		if _, err := w.RunOnce(ctx); err != nil {
			if ctx.Err() != nil {
				log.Println("worker: berhenti")
				return
			}

			log.Printf("worker: reserve gagal: %v", err)
			// Jeda kecil supaya Redis yang sedang bermasalah tidak dibombardir
			// percobaan bertubi-tubi.
			time.Sleep(time.Second)
		}
	}
}

// RunOnce mengambil dan memproses satu job. Mengembalikan false tanpa error
// kalau antrean sedang kosong; itulah yang dipakai unit test untuk memproses
// satu job tanpa menjalankan loop.
func (w *Worker) RunOnce(ctx context.Context) (bool, error) {
	job, err := w.source.Reserve(ctx, reserveTimeout)
	if errors.Is(err, queue.ErrEmpty) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	w.process(ctx, job)
	return true, nil
}

func (w *Worker) process(ctx context.Context, job *queue.Job) {
	handler, ok := w.handlers[job.Type]
	if !ok {
		cause := fmt.Errorf("tidak ada handler untuk job %q", job.Type)
		log.Printf("worker: %v (job=%s), dipindah ke dead-letter", cause, job.ID)

		if err := w.source.DeadLetter(ctx, job, cause); err != nil {
			log.Printf("worker: gagal memindahkan job %s ke dead-letter: %v", job.ID, err)
		}
		return
	}

	if err := handler(ctx, job.Payload); err != nil {
		log.Printf("worker: job %s (%s) gagal: %v", job.ID, job.Type, err)

		if err := w.source.Retry(ctx, job, err); err != nil {
			log.Printf("worker: gagal menjadwalkan retry job %s: %v", job.ID, err)
		}
		return
	}

	log.Printf("worker: job %s (%s) selesai", job.ID, job.Type)
}
