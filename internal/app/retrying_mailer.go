package app

import (
	"context"
	"fmt"
	"log"

	"spotify-backend/internal/queue"
	"spotify-backend/internal/service"
	"spotify-backend/internal/worker"
)

// retryingMailer membungkus Mailer dengan percobaan ulang lewat antrean.
//
// Percobaan pertama tetap sinkron seperti sebelumnya: selama SMTP sehat,
// perilakunya tidak berubah sama sekali. Yang baru hanya jalur gagal: email
// yang gagal dikirim tidak hilang, melainkan diantrekan sebagai job
// email:send untuk dicoba worker dengan backoff.
//
// Dipasang di sini (bukan di service) supaya MailService dan AuthService tidak
// perlu tahu apa-apa soal antrean.
type retryingMailer struct {
	inner service.Mailer
	queue *queue.Queue
}

func newRetryingMailer(inner service.Mailer, jobQueue *queue.Queue) service.Mailer {
	return &retryingMailer{inner: inner, queue: jobQueue}
}

func (m *retryingMailer) Send(ctx context.Context, to, subject, textBody, htmlBody string) error {
	err := m.inner.Send(ctx, to, subject, textBody, htmlBody)
	if err == nil {
		return nil
	}

	payload := worker.EmailPayload{To: to, Subject: subject, Text: textBody, HTML: htmlBody}
	if enqueueErr := m.queue.Enqueue(ctx, worker.JobEmailSend, payload); enqueueErr != nil {
		// Gagal kirim DAN gagal mengantre: kembalikan error asli supaya
		// pemanggil tetap melihat kegagalannya, dengan konteks tambahan.
		return fmt.Errorf("%w (antre retry juga gagal: %v)", err, enqueueErr)
	}

	log.Printf("email ke %s gagal dikirim, diantrekan untuk dicoba ulang: %v", to, err)
	return nil
}
