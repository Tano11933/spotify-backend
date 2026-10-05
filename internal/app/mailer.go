package app

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"spotify-backend/internal/service"
	"spotify-backend/pkg/mailer"
)

// MailerFromEnv memilih implementasi mailer berdasarkan environment.
//
// Dipakai DUA command: API (untuk mengirim email reset) dan worker (untuk
// mengirim ulang email yang gagal). Karena itu pemilihannya tinggal di sini,
// bukan di salah satu cmd, supaya keduanya tidak bisa berbeda konfigurasi.
//
// Kalau SMTP_HOST kosong, aplikasi TIDAK gagal start: ia memakai LogMailer
// yang menulis email ke terminal. Ini disengaja supaya seluruh alur reset
// password bisa diuji tanpa kredensial SMTP.
func MailerFromEnv() (service.Mailer, error) {
	host := os.Getenv("SMTP_HOST")
	if host == "" {
		log.Println("⚠️  SMTP_HOST is empty, using LogMailer (emails will be printed to this terminal)")
		return mailer.NewLogMailer(), nil
	}

	port, err := strconv.Atoi(os.Getenv("SMTP_PORT"))
	if err != nil {
		return nil, fmt.Errorf("invalid SMTP_PORT %q: %w", os.Getenv("SMTP_PORT"), err)
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
		return nil, fmt.Errorf("configure SMTP mailer: %w", err)
	}

	log.Printf("✅ SMTP mailer configured (%s:%d)", host, port)
	return smtpMailer, nil
}
