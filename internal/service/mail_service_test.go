package service

import (
	"context"
	"strings"
	"testing"
	"time"
)

// recordingMailer memenuhi interface Mailer dan hanya menyimpan apa yang dikirim.
type recordingMailer struct {
	to        string
	subject   string
	textBody  string
	htmlBody  string
	callCount int
}

func (m *recordingMailer) Send(_ context.Context, to, subject, textBody, htmlBody string) error {
	m.callCount++
	m.to, m.subject, m.textBody, m.htmlBody = to, subject, textBody, htmlBody
	return nil
}

func TestSendPasswordResetBuildsCorrectLink(t *testing.T) {
	mailer := &recordingMailer{}
	svc := NewMailService(mailer, "http://localhost:5173", 15*time.Minute)

	token := "abc-123_XYZ"
	if err := svc.SendPasswordReset(context.Background(), "gabriel@example.com", "Gabriel", token); err != nil {
		t.Fatalf("SendPasswordReset: %v", err)
	}

	if mailer.callCount != 1 {
		t.Fatalf("mailer dipanggil %d kali, ingin 1", mailer.callCount)
	}
	if mailer.to != "gabriel@example.com" {
		t.Errorf("penerima = %q", mailer.to)
	}

	want := "http://localhost:5173/reset-password?token=abc-123_XYZ"
	if !strings.Contains(mailer.textBody, want) {
		t.Errorf("body teks tidak memuat link yang benar.\ningin memuat: %s\nbody:\n%s", want, mailer.textBody)
	}
	if !strings.Contains(mailer.htmlBody, want) {
		t.Errorf("body HTML tidak memuat link yang benar")
	}
}

// TestSendPasswordResetTrimsTrailingSlash mencegah URL berisi dua garis miring
// (http://host//reset-password) kalau FRONTEND_URL di .env ditulis dengan slash
// di akhir — kesalahan konfigurasi yang sangat mudah terjadi.
func TestSendPasswordResetTrimsTrailingSlash(t *testing.T) {
	mailer := &recordingMailer{}
	svc := NewMailService(mailer, "http://localhost:5173/", 15*time.Minute)

	if err := svc.SendPasswordReset(context.Background(), "a@b.com", "A", "tok"); err != nil {
		t.Fatalf("SendPasswordReset: %v", err)
	}

	if strings.Contains(mailer.textBody, "5173//") {
		t.Errorf("URL punya slash ganda:\n%s", mailer.textBody)
	}
}

// TestSendPasswordResetEscapesUserName adalah test keamanan.
//
// Nama user berasal dari input saat register dan disisipkan ke dalam HTML email.
// Kalau template-nya text/template alih-alih html/template, tag <script> di
// bawah akan masuk ke email apa adanya. Test ini yang menahan seseorang
// mengganti import itu tanpa sadar akibatnya.
func TestSendPasswordResetEscapesUserName(t *testing.T) {
	mailer := &recordingMailer{}
	svc := NewMailService(mailer, "http://localhost:5173", 15*time.Minute)

	evil := `<script>alert("xss")</script>`
	if err := svc.SendPasswordReset(context.Background(), "a@b.com", evil, "tok"); err != nil {
		t.Fatalf("SendPasswordReset: %v", err)
	}

	if strings.Contains(mailer.htmlBody, "<script>") {
		t.Errorf("nama user tidak di-escape, tag <script> masuk ke HTML:\n%s", mailer.htmlBody)
	}
	if !strings.Contains(mailer.htmlBody, "&lt;script&gt;") {
		t.Errorf("tidak menemukan bentuk ter-escape yang diharapkan di HTML")
	}
}

// TestSendPasswordResetEscapesTokenInURL memastikan token yang mengandung
// karakter khusus tetap aman di query string. Token kita base64url sehingga tidak
// pernah begitu, tapi test ini mengunci pemakaian net/url alih-alih penggabungan
// string manual.
func TestSendPasswordResetEscapesTokenInURL(t *testing.T) {
	mailer := &recordingMailer{}
	svc := NewMailService(mailer, "http://localhost:5173", 15*time.Minute)

	if err := svc.SendPasswordReset(context.Background(), "a@b.com", "A", "a b&c=d"); err != nil {
		t.Fatalf("SendPasswordReset: %v", err)
	}

	if !strings.Contains(mailer.textBody, "token=a+b%26c%3Dd") {
		t.Errorf("token tidak di-encode dengan benar di URL:\n%s", mailer.textBody)
	}
}

func TestSendPasswordResetIncludesBothBodyTypes(t *testing.T) {
	mailer := &recordingMailer{}
	svc := NewMailService(mailer, "http://localhost:5173", 15*time.Minute)

	if err := svc.SendPasswordReset(context.Background(), "a@b.com", "A", "tok"); err != nil {
		t.Fatalf("SendPasswordReset: %v", err)
	}

	if mailer.textBody == "" {
		t.Error("body plaintext kosong — email HTML tanpa padanan teks lebih mudah ditandai spam")
	}
	if !strings.Contains(mailer.htmlBody, "<html") {
		t.Error("body HTML tidak berisi HTML")
	}
	if mailer.subject == "" {
		t.Error("subject kosong")
	}
}

func TestHumanizeDuration(t *testing.T) {
	cases := map[time.Duration]string{
		15 * time.Minute:               "15 menit",
		30 * time.Minute:               "30 menit",
		time.Hour:                      "1 jam",
		2 * time.Hour:                  "2 jam",
		90 * time.Minute:               "90 menit",
		5*time.Minute + 30*time.Second: "5 menit",
	}

	for input, want := range cases {
		if got := humanizeDuration(input); got != want {
			t.Errorf("humanizeDuration(%s) = %q, ingin %q", input, got, want)
		}
	}
}
