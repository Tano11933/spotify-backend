package service

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"net/url"
	"strings"
	"time"
)

type Mailer interface {
	Send(ctx context.Context, to, subject, textBody, htmlBody string) error
}

// MailService menyusun isi email transaksional aplikasi.
type MailService struct {
	mailer      Mailer
	frontendURL string
	tokenTTL    time.Duration
}

func NewMailService(mailer Mailer, frontendURL string, tokenTTL time.Duration) *MailService {
	return &MailService{
		mailer:      mailer,
		frontendURL: strings.TrimRight(frontendURL, "/"),
		tokenTTL:    tokenTTL,
	}
}

var resetPasswordHTML = template.Must(template.New("reset_password").Parse(`
<!doctype html>
<html>
  <body style="margin:0;padding:24px;background:#121212;font-family:Arial,Helvetica,sans-serif;color:#ffffff;">
    <div style="max-width:480px;margin:0 auto;background:#181818;border-radius:8px;padding:32px;">
      <h1 style="margin:0 0 16px;font-size:20px;">Reset password</h1>
      <p style="margin:0 0 16px;color:#b3b3b3;font-size:14px;line-height:1.6;">
        Hai {{.Name}}, kami menerima permintaan untuk mengatur ulang password akunmu.
        Klik tombol di bawah untuk membuat password baru.
      </p>
      <p style="margin:0 0 24px;">
        <a href="{{.ResetURL}}"
           style="display:inline-block;background:#1db954;color:#000000;text-decoration:none;
                  font-weight:bold;padding:12px 28px;border-radius:9999px;font-size:14px;">
          Buat password baru
        </a>
      </p>
      <p style="margin:0 0 8px;color:#b3b3b3;font-size:12px;line-height:1.6;">
        Link ini hanya berlaku {{.ExpiresIn}} dan hanya bisa dipakai satu kali.
      </p>
      <p style="margin:0;color:#b3b3b3;font-size:12px;line-height:1.6;">
        Kalau kamu tidak meminta ini, abaikan saja email ini — password kamu tidak berubah.
      </p>
    </div>
  </body>
</html>
`))

func (s *MailService) SendPasswordReset(ctx context.Context, to, name, token string) error {
	resetURL, err := s.buildResetURL(token)
	if err != nil {
		return err
	}

	expiresIn := humanizeDuration(s.tokenTTL)

	data := struct {
		Name      string
		ResetURL  string
		ExpiresIn string
	}{
		Name:      name,
		ResetURL:  resetURL,
		ExpiresIn: expiresIn,
	}

	var html bytes.Buffer
	if err := resetPasswordHTML.Execute(&html, data); err != nil {
		return fmt.Errorf("render reset password email: %w", err)
	}

	text := fmt.Sprintf(
		"Hai %s,\n\n"+
			"Kami menerima permintaan untuk mengatur ulang password akunmu.\n"+
			"Buka link berikut untuk membuat password baru:\n\n%s\n\n"+
			"Link ini hanya berlaku %s dan hanya bisa dipakai satu kali.\n\n"+
			"Kalau kamu tidak meminta ini, abaikan saja email ini — password kamu tidak berubah.\n",
		name, resetURL, expiresIn,
	)

	return s.mailer.Send(ctx, to, "Reset password akun Spotify Clone", text, html.String())
}

func (s *MailService) buildResetURL(token string) (string, error) {
	u, err := url.Parse(s.frontendURL)
	if err != nil {
		return "", fmt.Errorf("invalid frontend url %q: %w", s.frontendURL, err)
	}

	u.Path = "/reset-password"
	u.RawQuery = url.Values{"token": {token}}.Encode()

	return u.String(), nil
}

func humanizeDuration(d time.Duration) string {
	if d >= time.Hour {
		hours := int(d.Hours())
		if d == time.Duration(hours)*time.Hour {
			return fmt.Sprintf("%d jam", hours)
		}
	}
	return fmt.Sprintf("%d menit", int(d.Minutes()))
}
