package notifications

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// SMTPProvider delivers email notifications via an external SMTP
// server. It supports two transport modes:
//
//   - Implicit TLS (port 465): the connection is TLS from byte 0.
//     Older mail servers and some corporate relays still speak this.
//
//   - STARTTLS (port 587, default submission port): the connection
//     starts plain and is upgraded with STARTTLS before any
//     credentials cross the wire. This is the modern recommendation
//     and is what RFC 8314 prefers.
//
// Authentication uses PLAIN auth when Username is set. Servers that
// accept unauthenticated submission (localhost relays) are supported
// by leaving Username empty.
type SMTPProvider struct {
	Host               string
	Port               int
	Username           string
	Password           string
	From               string
	FromName           string
	UseTLS             bool
	InsecureSkipVerify bool

	// dialer overrides the dialing function in tests. Nil uses the
	// real network.
	dialer func(ctx context.Context, addr string) (net.Conn, error)
	// logger for structured security warnings
	logger *slog.Logger
}

// NewSMTPProvider constructs an SMTPProvider from the validated
// config. The returned provider is NOT validated — call Validate()
// before registering it with the dispatcher so misconfiguration
// surfaces at startup rather than on first send.
func NewSMTPProvider(cfg core.SMTPConfig, logger *slog.Logger) *SMTPProvider {
	return &SMTPProvider{
		Host:               cfg.Host,
		Port:               cfg.Port,
		Username:           cfg.Username,
		Password:           cfg.Password,
		From:               cfg.From,
		FromName:           cfg.FromName,
		UseTLS:             cfg.UseTLS,
		InsecureSkipVerify: cfg.InsecureSkipVerify,
		logger:             logger,
	}
}

func (s *SMTPProvider) Name() string { return "email" }

// Validate enforces the non-negotiable SMTP fields. We deliberately
// don't validate Port — defaultPort() supplies a sane fallback — so
// operators can set only Host + From for localhost relay setups.
func (s *SMTPProvider) Validate() error {
	if s.Host == "" {
		return fmt.Errorf("smtp host is required")
	}
	if s.From == "" {
		return fmt.Errorf("smtp from address is required")
	}
	if _, err := mail.ParseAddress(s.From); err != nil {
		return fmt.Errorf("smtp from address %q is invalid: %w", s.From, err)
	}
	if s.Username != "" && s.Password == "" {
		return fmt.Errorf("smtp username set but password is empty")
	}

	// SECURITY WARNING: InsecureSkipVerify disables TLS certificate verification
	// This should only be used in development environments or with trusted internal relays
	if s.InsecureSkipVerify {
		if s.logger != nil {
			s.logger.Warn("SMTP InsecureSkipVerify is enabled – TLS certificate verification disabled",
				"host", s.Host,
			)
		}
	}

	return nil
}

// defaultPort returns the SMTP port, falling back to the
// submission-port defaults: 465 for implicit TLS, 587 for STARTTLS.
func (s *SMTPProvider) defaultPort() int {
	if s.Port > 0 {
		return s.Port
	}
	if s.UseTLS {
		return 465
	}
	return 587
}

// Send delivers a single message. Retry happens at the core.Retry
// layer so transient network errors (connection reset, TLS
// handshake timeout) are retried with backoff, but permanent errors
// (authentication failure, bad recipient) surface on the first try.
func (s *SMTPProvider) Send(ctx context.Context, recipient, subject, body, format string) error {
	if recipient == "" {
		return fmt.Errorf("smtp: recipient is required")
	}
	if _, err := mail.ParseAddress(recipient); err != nil {
		return fmt.Errorf("smtp: recipient %q is invalid: %w", recipient, err)
	}

	msg := s.buildMessage(recipient, subject, body, format)
	addr := net.JoinHostPort(s.Host, fmt.Sprintf("%d", s.defaultPort()))

	return core.Retry(ctx, core.DefaultRetryConfig(), func() error {
		return s.deliver(ctx, addr, recipient, msg)
	})
}

// buildMessage assembles RFC 5322 headers + body. It picks a
// Content-Type based on the provided format: "html" → text/html,
// anything else → text/plain. Subject lines are not MIME-encoded —
// callers should keep them ASCII. Non-ASCII subjects would need
// `=?UTF-8?B?...?=` framing which we skip until a real need surfaces.
func (s *SMTPProvider) buildMessage(to, subject, body, format string) []byte {
	fromHeader := s.From
	if s.FromName != "" {
		addr := mail.Address{Name: s.FromName, Address: s.From}
		fromHeader = addr.String()
	}

	contentType := "text/plain; charset=UTF-8"
	if strings.EqualFold(format, "html") {
		contentType = "text/html; charset=UTF-8"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", fromHeader)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	fmt.Fprintf(&b, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&b, "Content-Type: %s\r\n", contentType)
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().UTC().Format(time.RFC1123Z))
	fmt.Fprintf(&b, "\r\n")
	b.WriteString(body)
	return []byte(b.String())
}

// deliver performs one attempt of the full SMTP handshake. Separated
// from Send so retry logic wraps it.
func (s *SMTPProvider) deliver(ctx context.Context, addr, recipient string, msg []byte) error {
	conn, err := s.dial(ctx, addr)
	if err != nil {
		return fmt.Errorf("smtp dial: %w", err)
	}

	client, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("smtp client: %w", err)
	}
	defer client.Close()

	// STARTTLS path: upgrade the plain connection before we send
	// credentials. If the server doesn't advertise STARTTLS, bail
	// out rather than leak the password in the clear.
	if !s.UseTLS {
		tlsCfg := &tls.Config{
			ServerName:         s.Host,
			InsecureSkipVerify: s.InsecureSkipVerify, //nolint:gosec // config opt-in for self-signed relays
			MinVersion:         tls.VersionTLS12,
		}
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(tlsCfg); err != nil {
				return fmt.Errorf("smtp STARTTLS: %w", err)
			}
		} else if s.Username != "" {
			return fmt.Errorf("smtp: server does not advertise STARTTLS; refusing to send credentials in plaintext")
		}
	}

	if s.Username != "" {
		auth := smtp.PlainAuth("", s.Username, s.Password, s.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}

	if err := client.Mail(s.From); err != nil {
		return fmt.Errorf("smtp MAIL FROM: %w", err)
	}
	if err := client.Rcpt(recipient); err != nil {
		return fmt.Errorf("smtp RCPT TO: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp DATA: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		_ = w.Close()
		return fmt.Errorf("smtp body write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp body close: %w", err)
	}
	return client.Quit()
}

// dial opens a connection in the mode the provider is configured
// for: an implicit-TLS dial when UseTLS is true, a plain TCP dial
// otherwise. A custom dialer installed by tests overrides both.
func (s *SMTPProvider) dial(ctx context.Context, addr string) (net.Conn, error) {
	if s.dialer != nil {
		return s.dialer(ctx, addr)
	}

	if s.UseTLS {
		tlsCfg := &tls.Config{
			ServerName:         s.Host,
			InsecureSkipVerify: s.InsecureSkipVerify, //nolint:gosec // config opt-in for self-signed relays
			MinVersion:         tls.VersionTLS12,
		}
		d := &tls.Dialer{Config: tlsCfg, NetDialer: &net.Dialer{Timeout: 10 * time.Second}}
		return d.DialContext(ctx, "tcp", addr)
	}

	d := &net.Dialer{Timeout: 10 * time.Second}
	return d.DialContext(ctx, "tcp", addr)
}
