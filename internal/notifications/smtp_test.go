package notifications

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ---------------------------------------------------------------------------
// Unit tests that don't hit the network
// ---------------------------------------------------------------------------

func TestSMTPProvider_Name(t *testing.T) {
	p := NewSMTPProvider(core.SMTPConfig{}, discardLogger())
	if p.Name() != "email" {
		t.Errorf("Name = %q, want email", p.Name())
	}
}

func TestSMTPProvider_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     core.SMTPConfig
		wantErr string
	}{
		{
			name:    "missing host",
			cfg:     core.SMTPConfig{From: "noreply@example.com"},
			wantErr: "host is required",
		},
		{
			name:    "missing from",
			cfg:     core.SMTPConfig{Host: "mail.example.com"},
			wantErr: "from address is required",
		},
		{
			name:    "bad from address",
			cfg:     core.SMTPConfig{Host: "mail.example.com", From: "not-an-email"},
			wantErr: "invalid",
		},
		{
			name: "username without password",
			cfg: core.SMTPConfig{
				Host: "mail.example.com", From: "noreply@example.com",
				Username: "bob",
			},
			wantErr: "password is empty",
		},
		{
			name: "minimal valid",
			cfg: core.SMTPConfig{
				Host: "mail.example.com", From: "noreply@example.com",
			},
		},
		{
			name: "authenticated valid",
			cfg: core.SMTPConfig{
				Host: "mail.example.com", From: "noreply@example.com",
				Username: "bob", Password: "secret",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewSMTPProvider(tt.cfg, discardLogger()).Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("Validate = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Validate = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestSMTPProvider_DefaultPort(t *testing.T) {
	cases := []struct {
		useTLS bool
		port   int
		want   int
	}{
		{false, 0, 587},
		{true, 0, 465},
		{false, 2525, 2525},
		{true, 2525, 2525},
	}
	for _, c := range cases {
		p := NewSMTPProvider(core.SMTPConfig{UseTLS: c.useTLS, Port: c.port}, discardLogger())
		if got := p.defaultPort(); got != c.want {
			t.Errorf("useTLS=%v port=%d → %d, want %d", c.useTLS, c.port, got, c.want)
		}
	}
}

func TestSMTPProvider_BuildMessage_HeadersAndBody(t *testing.T) {
	p := NewSMTPProvider(core.SMTPConfig{
		From:     "noreply@example.com",
		FromName: "Deploy Monster",
	}, discardLogger())
	msg := string(p.buildMessage("admin@example.com", "Alert!", "server down", "text"))

	for _, must := range []string{
		"From: \"Deploy Monster\" <noreply@example.com>\r\n",
		"To: admin@example.com\r\n",
		"Subject: Alert!\r\n",
		"MIME-Version: 1.0\r\n",
		"Content-Type: text/plain; charset=UTF-8\r\n",
		"\r\n\r\nserver down",
	} {
		if !strings.Contains(msg, must) {
			t.Errorf("message missing header/body fragment %q\nfull message:\n%s", must, msg)
		}
	}

	// html format should yield text/html MIME type
	html := string(p.buildMessage("admin@example.com", "Alert!", "<b>down</b>", "html"))
	if !strings.Contains(html, "Content-Type: text/html") {
		t.Error("html format did not produce text/html Content-Type")
	}
}

func TestSMTPProvider_Send_RejectsInvalidRecipient(t *testing.T) {
	p := NewSMTPProvider(core.SMTPConfig{
		Host: "mail.example.com", From: "noreply@example.com",
	}, discardLogger())
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := p.Send(ctx, "not-an-email", "s", "b", "text")
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Errorf("Send with invalid recipient = %v, want invalid error", err)
	}
}

func TestSMTPProvider_Send_RejectsEmptyRecipient(t *testing.T) {
	p := NewSMTPProvider(core.SMTPConfig{
		Host: "mail.example.com", From: "noreply@example.com",
	}, discardLogger())
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := p.Send(ctx, "", "s", "b", "text"); err == nil {
		t.Error("Send with empty recipient = nil, want error")
	}
}

// ---------------------------------------------------------------------------
// In-process SMTP fake server for end-to-end deliver() coverage
// ---------------------------------------------------------------------------

// fakeSMTPServer is a minimal SMTP conversation partner. It runs on
// an in-memory pipe and speaks just enough of RFC 5321 (EHLO, MAIL,
// RCPT, DATA, QUIT) for smtp.Client to complete a transaction.
type fakeSMTPServer struct {
	mu       sync.Mutex
	mailFrom string
	rcptTo   []string
	data     string
	closed   bool
}

func (f *fakeSMTPServer) serve(conn net.Conn) {
	defer conn.Close()
	br := bufio.NewReader(conn)
	writeLine := func(line string) {
		_, _ = io.WriteString(conn, line+"\r\n")
	}

	writeLine("220 fake.local ESMTP")

	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		cmd := strings.ToUpper(line)

		switch {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
			writeLine("250-fake.local")
			// Include auth and (optionally) STARTTLS as extensions so
			// smtp.Client picks them up.
			writeLine("250 AUTH PLAIN")
		case strings.HasPrefix(cmd, "MAIL FROM:"):
			f.mu.Lock()
			f.mailFrom = strings.TrimPrefix(line, "MAIL FROM:")
			f.mu.Unlock()
			writeLine("250 OK")
		case strings.HasPrefix(cmd, "RCPT TO:"):
			f.mu.Lock()
			f.rcptTo = append(f.rcptTo, strings.TrimPrefix(line, "RCPT TO:"))
			f.mu.Unlock()
			writeLine("250 OK")
		case cmd == "DATA":
			writeLine("354 start")
			var body strings.Builder
			for {
				l, err := br.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" || l == ".\n" {
					break
				}
				body.WriteString(l)
			}
			f.mu.Lock()
			f.data = body.String()
			f.mu.Unlock()
			writeLine("250 OK")
		case cmd == "QUIT":
			writeLine("221 bye")
			f.mu.Lock()
			f.closed = true
			f.mu.Unlock()
			return
		case strings.HasPrefix(cmd, "AUTH"):
			writeLine("235 OK")
		case cmd == "RSET":
			writeLine("250 OK")
		case cmd == "NOOP":
			writeLine("250 OK")
		default:
			writeLine("500 unknown command")
		}
	}
}

func TestSMTPProvider_Deliver_FullHandshake(t *testing.T) {
	// Spin up a real TCP listener so we can point a real smtp.Client at it.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	srv := &fakeSMTPServer{}
	// Accept in a loop so core.Retry's redials don't deadlock if the
	// first attempt fails midway. Errors from Accept are silent
	// because ln.Close() during cleanup is the expected exit path.
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go srv.serve(conn)
		}
	}()

	p := NewSMTPProvider(core.SMTPConfig{
		Host: "127.0.0.1",
		Port: ln.Addr().(*net.TCPAddr).Port,
		From: "from@fake.local",
		// No TLS, no auth — the fake server doesn't advertise STARTTLS
		// so the provider should skip the TLS upgrade and proceed
		// plain because Username is also empty.
	}, discardLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := p.Send(ctx, "to@fake.local", "Hi", "hello", "text"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	srv.mu.Lock()
	defer srv.mu.Unlock()
	if !strings.Contains(srv.mailFrom, "from@fake.local") {
		t.Errorf("MAIL FROM captured = %q, want from@fake.local", srv.mailFrom)
	}
	if len(srv.rcptTo) != 1 || !strings.Contains(srv.rcptTo[0], "to@fake.local") {
		t.Errorf("RCPT TO captured = %v, want [to@fake.local]", srv.rcptTo)
	}
	if !strings.Contains(srv.data, "Subject: Hi\r\n") {
		t.Errorf("DATA body missing Subject header:\n%s", srv.data)
	}
	if !strings.Contains(srv.data, "hello") {
		t.Errorf("DATA body missing payload:\n%s", srv.data)
	}
}

func TestSMTPProvider_Deliver_RefusesPlaintextAuth(t *testing.T) {
	// When the server does not advertise STARTTLS and the provider
	// has credentials, deliver() must refuse rather than send the
	// password in the clear.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	handle := func(conn net.Conn) {
		defer conn.Close()
		// Simplest possible fake: announce ESMTP but no STARTTLS
		// extension, so smtp.Client.Extension("STARTTLS") returns
		// false and the provider bails out rather than attempt auth.
		_, _ = io.WriteString(conn, "220 fake.local ESMTP\r\n")
		br := bufio.NewReader(conn)
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				return
			}
			cmd := strings.ToUpper(line)
			switch {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
				_, _ = io.WriteString(conn, "250-fake.local\r\n250 PIPELINING\r\n")
			case strings.HasPrefix(cmd, "QUIT"):
				_, _ = io.WriteString(conn, "221 bye\r\n")
				return
			default:
				_, _ = io.WriteString(conn, "500 unexpected\r\n")
				return
			}
		}
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handle(conn)
		}
	}()

	p := NewSMTPProvider(core.SMTPConfig{
		Host:     "127.0.0.1",
		Port:     ln.Addr().(*net.TCPAddr).Port,
		From:     "from@fake.local",
		Username: "bob",
		Password: "secret",
	}, discardLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err = p.Send(ctx, "to@fake.local", "Hi", "body", "text")
	if err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Errorf("Send = %v, want error mentioning STARTTLS refusal", err)
	}
}

// Smoke test the dialer override path used by the full handshake
// test above. Without the override, tests couldn't exercise
// dial() in isolation.
func TestSMTPProvider_CustomDialer(t *testing.T) {
	p := NewSMTPProvider(core.SMTPConfig{
		Host: "mail.example.com", From: "noreply@example.com",
	}, discardLogger())
	called := false
	p.dialer = func(ctx context.Context, addr string) (net.Conn, error) {
		called = true
		return nil, errors.New("injected")
	}
	_, err := p.dial(context.Background(), "mail.example.com:587")
	if !called {
		t.Error("custom dialer was not invoked")
	}
	if err == nil {
		t.Error("dial err = nil, want injected error")
	}
}

func TestNotification_Registration_SMTP(t *testing.T) {
	// Integration: Module.Init registers the SMTP provider when the
	// config has Host+From set, and reports healthy afterwards.
	cfg := &core.Config{
		Notifications: core.NotificationConfig{
			SMTP: core.SMTPConfig{
				Host: "mail.example.com",
				From: "noreply@example.com",
			},
		},
	}
	c := newTestCore(t, cfg)
	m := New()
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, ok := m.dispatcher.GetProvider("email"); !ok {
		t.Error("email provider not registered")
	}
	if m.Health() != core.HealthOK {
		t.Errorf("Health = %v, want HealthOK", m.Health())
	}
}

// newTestCore builds the minimum Core surface needed by Module.Init.
// Notifications only touches Config, Logger, Services, and Events.
func newTestCore(t *testing.T, cfg *core.Config) *core.Core {
	t.Helper()
	return &core.Core{
		Config:   cfg,
		Logger:   discardLogger(),
		Services: core.NewServices(),
		Events:   core.NewEventBus(discardLogger()),
	}
}

func TestSMTPProvider_Dial_UseTLS(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	p := NewSMTPProvider(core.SMTPConfig{
		Host:   "127.0.0.1",
		Port:   ln.Addr().(*net.TCPAddr).Port,
		From:   "test@example.com",
		UseTLS: true,
	}, discardLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = p.dial(ctx, "127.0.0.1:"+strconv.Itoa(ln.Addr().(*net.TCPAddr).Port))
	if err == nil {
		t.Error("expected TLS handshake error against plain TCP listener")
	}
}

func TestSMTPProvider_Deliver_STARTTLSError(t *testing.T) {
	// Server advertises STARTTLS but the handshake fails.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	handle := func(conn net.Conn) {
		defer conn.Close()
		_, _ = io.WriteString(conn, "220 fake.local ESMTP\r\n")
		br := bufio.NewReader(conn)
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				return
			}
			cmd := strings.ToUpper(strings.TrimRight(line, "\r\n"))
			switch {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
				// Advertise STARTTLS so the client attempts it
				_, _ = io.WriteString(conn, "250-fake.local\r\n250 STARTTLS\r\n250 PIPELINING\r\n")
			case strings.HasPrefix(cmd, "STARTTLS"):
				// Return error code 454 (TLS not available)
				_, _ = io.WriteString(conn, "454 TLS not available\r\n")
			case strings.HasPrefix(cmd, "QUIT"):
				_, _ = io.WriteString(conn, "221 bye\r\n")
				return
			default:
				_, _ = io.WriteString(conn, "500 unexpected\r\n")
			}
		}
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handle(conn)
		}
	}()

	p := NewSMTPProvider(core.SMTPConfig{
		Host: "127.0.0.1",
		Port: ln.Addr().(*net.TCPAddr).Port,
		From: "from@fake.local",
		// No TLS, no auth — but server advertises STARTTLS and it fails
	}, discardLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err = p.deliver(ctx, "127.0.0.1:"+strconv.Itoa(ln.Addr().(*net.TCPAddr).Port), "to@fake.local", []byte("test"))
	if err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Errorf("deliver = %v, want STARTTLS error", err)
	}
}

func TestSMTPProvider_Deliver_AuthError(t *testing.T) {
	// Use implicit TLS (UseTLS=true) with a self-signed cert so we can
	// reach the AUTH stage and reject it.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	cert, err := generateTestCert("127.0.0.1")
	if err != nil {
		t.Fatalf("generate cert: %v", err)
	}
	tlsLn := tls.NewListener(ln, &tls.Config{Certificates: []tls.Certificate{cert}})

	handle := func(conn net.Conn) {
		defer conn.Close()
		_, _ = io.WriteString(conn, "220 fake.local ESMTP\r\n")
		br := bufio.NewReader(conn)
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				return
			}
			cmd := strings.ToUpper(strings.TrimRight(line, "\r\n"))
			switch {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
				// On an implicit TLS connection, STARTTLS is not needed
				// but we advertise it anyway; client won't try it since
				// the connection is already TLS.
				_, _ = io.WriteString(conn, "250-fake.local\r\n250 AUTH PLAIN\r\n")
			case strings.HasPrefix(cmd, "AUTH"):
				_, _ = io.WriteString(conn, "535 Authentication failed\r\n")
			case strings.HasPrefix(cmd, "QUIT"):
				_, _ = io.WriteString(conn, "221 bye\r\n")
				return
			default:
				_, _ = io.WriteString(conn, "500 unexpected\r\n")
			}
		}
	}
	go func() {
		for {
			conn, err := tlsLn.Accept()
			if err != nil {
				return
			}
			go handle(conn)
		}
	}()

	p := NewSMTPProvider(core.SMTPConfig{
		Host:               "127.0.0.1",
		Port:               ln.Addr().(*net.TCPAddr).Port,
		From:               "from@fake.local",
		Username:           "bob",
		Password:           "wrong",
		UseTLS:             true,
		InsecureSkipVerify: true,
	}, discardLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err = p.deliver(ctx, "127.0.0.1:"+strconv.Itoa(ln.Addr().(*net.TCPAddr).Port), "to@fake.local", []byte("test"))
	if err == nil || !strings.Contains(err.Error(), "auth") {
		t.Errorf("deliver = %v, want auth error", err)
	}
}

func TestSMTPProvider_Deliver_MailFromError(t *testing.T) {
	// Do NOT advertise STARTTLS — since there's no username either,
	// the deliver() skips the STARTTLS block and reaches MAIL FROM.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	handle := func(conn net.Conn) {
		defer conn.Close()
		_, _ = io.WriteString(conn, "220 fake.local ESMTP\r\n")
		br := bufio.NewReader(conn)
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				return
			}
			cmd := strings.ToUpper(strings.TrimRight(line, "\r\n"))
			switch {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
				_, _ = io.WriteString(conn, "250-fake.local\r\n250 PIPELINING\r\n")
			case strings.HasPrefix(cmd, "MAIL FROM:"):
				_, _ = io.WriteString(conn, "550 Sender rejected\r\n")
			case strings.HasPrefix(cmd, "QUIT"):
				_, _ = io.WriteString(conn, "221 bye\r\n")
				return
			default:
				_, _ = io.WriteString(conn, "500 unexpected\r\n")
			}
		}
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handle(conn)
		}
	}()

	p := NewSMTPProvider(core.SMTPConfig{
		Host: "127.0.0.1",
		Port: ln.Addr().(*net.TCPAddr).Port,
		From: "from@fake.local",
	}, discardLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err = p.deliver(ctx, "127.0.0.1:"+strconv.Itoa(ln.Addr().(*net.TCPAddr).Port), "to@fake.local", []byte("test"))
	if err == nil || !strings.Contains(err.Error(), "MAIL FROM") {
		t.Errorf("deliver = %v, want MAIL FROM error", err)
	}
}

func TestSMTPProvider_Deliver_RcptToError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	handle := func(conn net.Conn) {
		defer conn.Close()
		_, _ = io.WriteString(conn, "220 fake.local ESMTP\r\n")
		br := bufio.NewReader(conn)
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				return
			}
			cmd := strings.ToUpper(strings.TrimRight(line, "\r\n"))
			switch {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
				_, _ = io.WriteString(conn, "250-fake.local\r\n250 PIPELINING\r\n")
			case strings.HasPrefix(cmd, "MAIL FROM:"):
				_, _ = io.WriteString(conn, "250 OK\r\n")
			case strings.HasPrefix(cmd, "RCPT TO:"):
				_, _ = io.WriteString(conn, "550 Recipient rejected\r\n")
			case strings.HasPrefix(cmd, "QUIT"):
				_, _ = io.WriteString(conn, "221 bye\r\n")
				return
			default:
				_, _ = io.WriteString(conn, "500 unexpected\r\n")
			}
		}
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handle(conn)
		}
	}()

	p := NewSMTPProvider(core.SMTPConfig{
		Host: "127.0.0.1",
		Port: ln.Addr().(*net.TCPAddr).Port,
		From: "from@fake.local",
	}, discardLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err = p.deliver(ctx, "127.0.0.1:"+strconv.Itoa(ln.Addr().(*net.TCPAddr).Port), "to@fake.local", []byte("test"))
	if err == nil || !strings.Contains(err.Error(), "RCPT TO") {
		t.Errorf("deliver = %v, want RCPT TO error", err)
	}
}

func TestSMTPProvider_Deliver_DataError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	handle := func(conn net.Conn) {
		defer conn.Close()
		_, _ = io.WriteString(conn, "220 fake.local ESMTP\r\n")
		br := bufio.NewReader(conn)
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				return
			}
			cmd := strings.ToUpper(strings.TrimRight(line, "\r\n"))
			switch {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
				_, _ = io.WriteString(conn, "250-fake.local\r\n250 PIPELINING\r\n")
			case strings.HasPrefix(cmd, "MAIL FROM:"):
				_, _ = io.WriteString(conn, "250 OK\r\n")
			case strings.HasPrefix(cmd, "RCPT TO:"):
				_, _ = io.WriteString(conn, "250 OK\r\n")
			case cmd == "DATA":
				_, _ = io.WriteString(conn, "452 Too many recipients, no DATA\r\n")
			case strings.HasPrefix(cmd, "QUIT"):
				_, _ = io.WriteString(conn, "221 bye\r\n")
				return
			default:
				_, _ = io.WriteString(conn, "500 unexpected\r\n")
			}
		}
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handle(conn)
		}
	}()

	p := NewSMTPProvider(core.SMTPConfig{
		Host: "127.0.0.1",
		Port: ln.Addr().(*net.TCPAddr).Port,
		From: "from@fake.local",
	}, discardLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err = p.deliver(ctx, "127.0.0.1:"+strconv.Itoa(ln.Addr().(*net.TCPAddr).Port), "to@fake.local", []byte("test"))
	if err == nil || !strings.Contains(err.Error(), "DATA") {
		t.Errorf("deliver = %v, want DATA error", err)
	}
}

// generateTestCert creates a self-signed certificate for the given host.
func generateTestCert(host string) (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, err
	}
	template := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: host},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	template.DNSNames = append(template.DNSNames, host)
	template.IPAddresses = append(template.IPAddresses, net.ParseIP(host))

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}),
	)
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
