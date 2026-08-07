package mailer

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/wneessen/go-mail"
)

type Encryption string

const (

	EncryptionStartTLS Encryption = "starttls"
	EncryptionSSLTLS Encryption = "ssltls"
	EncryptionNone Encryption = "none"
)

type Config struct {
	Host       string
	Port       int
	Username   string
	Password   string
	FromEmail  string
	FromName   string
	Encryption Encryption
	Timeout    time.Duration
}

type Mailer interface {
	Send(ctx context.Context, to, subject, textBody, htmlBody string) error
}

type SMTPMailer struct {
	cfg Config
}

func NewSMTPMailer(cfg Config) (*SMTPMailer, error) {
	if cfg.Host == "" {
		return nil, fmt.Errorf("smtp host is required")
	}
	if cfg.Port == 0 {
		return nil, fmt.Errorf("smtp port is required")
	}
	if cfg.FromEmail == "" {
		return nil, fmt.Errorf("smtp from email is required")
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 15 * time.Second
	}
	if cfg.Encryption == "" {
		cfg.Encryption = EncryptionStartTLS
	}

	if _, err := cfg.newClient(); err != nil {
		return nil, fmt.Errorf("invalid smtp configuration: %w", err)
	}

	return &SMTPMailer{cfg: cfg}, nil
}

func (c Config) newClient() (*mail.Client, error) {
	opts := []mail.Option{
		mail.WithPort(c.Port),
		mail.WithTimeout(c.Timeout),
	}

	switch c.Encryption {
	case EncryptionSSLTLS:
		opts = append(opts, mail.WithSSL())
	case EncryptionNone:
		opts = append(opts, mail.WithTLSPolicy(mail.NoTLS))
	default: // starttls
		opts = append(opts, mail.WithTLSPolicy(mail.TLSMandatory))
	}

	switch {
	case c.Username == "":
		opts = append(opts, mail.WithSMTPAuth(mail.SMTPAuthNoAuth))
	case c.Encryption == EncryptionNone:
		opts = append(opts,
			mail.WithSMTPAuth(mail.SMTPAuthPlainNoEnc),
			mail.WithUsername(c.Username),
			mail.WithPassword(c.Password),
		)
	default:
		opts = append(opts,
			mail.WithSMTPAuth(mail.SMTPAuthPlain),
			mail.WithUsername(c.Username),
			mail.WithPassword(c.Password),
		)
	}

	return mail.NewClient(c.Host, opts...)
}

func (m *SMTPMailer) Send(ctx context.Context, to, subject, textBody, htmlBody string) error {
	msg := mail.NewMsg()

	if err := msg.FromFormat(m.cfg.FromName, m.cfg.FromEmail); err != nil {
		return fmt.Errorf("set from address: %w", err)
	}
	if err := msg.To(to); err != nil {
		return fmt.Errorf("set to address: %w", err)
	}

	msg.Subject(subject)

	msg.SetBodyString(mail.TypeTextPlain, textBody)
	if htmlBody != "" {
		msg.AddAlternativeString(mail.TypeTextHTML, htmlBody)
	}

	client, err := m.cfg.newClient()
	if err != nil {
		return fmt.Errorf("create smtp client: %w", err)
	}

	if err := client.DialAndSendWithContext(ctx, msg); err != nil {
		return fmt.Errorf("send email: %w", err)
	}
	return nil
}

type LogMailer struct{}

func NewLogMailer() *LogMailer {
	return &LogMailer{}
}

func (l *LogMailer) Send(_ context.Context, to, subject, textBody, _ string) error {
	var b strings.Builder
	b.WriteString("\n──────── EMAIL (LogMailer, tidak benar-benar dikirim) ────────\n")
	b.WriteString("To      : " + to + "\n")
	b.WriteString("Subject : " + subject + "\n")
	b.WriteString("─────────────────────────────────────────────────────────────\n")
	b.WriteString(textBody)
	b.WriteString("\n─────────────────────────────────────────────────────────────\n")

	log.Print(b.String())
	return nil
}
