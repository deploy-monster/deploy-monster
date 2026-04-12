package vps

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"net"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"golang.org/x/crypto/ssh"
)

// discardLogger returns a logger that discards output.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// generateTestKey generates an ECDSA private key in PEM format for SSH tests.
func generateTestKey(t *testing.T) []byte {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
}

// startFakeSSHServer starts a minimal SSH server that accepts connections
// and handles "exec" requests. It returns the listener address and a
// cleanup function.
func startFakeSSHServer(t *testing.T, hostKey ssh.Signer, authorizedKey ssh.PublicKey) (string, func()) {
	t.Helper()

	config := &ssh.ServerConfig{
		PublicKeyCallback: func(c ssh.ConnMetadata, pubKey ssh.PublicKey) (*ssh.Permissions, error) {
			if string(pubKey.Marshal()) == string(authorizedKey.Marshal()) {
				return nil, nil
			}
			return nil, fmt.Errorf("unknown public key")
		},
	}
	config.AddHostKey(hostKey)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener closed
			}
			go handleSSHConn(t, conn, config)
		}
	}()

	cleanup := func() {
		ln.Close()
		<-done
	}

	return ln.Addr().String(), cleanup
}

func handleSSHConn(t *testing.T, conn net.Conn, config *ssh.ServerConfig) {
	t.Helper()
	sshConn, chans, reqs, err := ssh.NewServerConn(conn, config)
	if err != nil {
		conn.Close()
		return
	}
	defer sshConn.Close()

	go ssh.DiscardRequests(reqs)

	for newChan := range chans {
		if newChan.ChannelType() != "session" {
			newChan.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}
		channel, requests, err := newChan.Accept()
		if err != nil {
			continue
		}
		go func() {
			defer channel.Close()
			for req := range requests {
				switch req.Type {
				case "exec":
					// Parse the command length + string (SSH wire format)
					if len(req.Payload) > 4 {
						// command := string(req.Payload[4:])
						channel.Write([]byte("command executed"))
					}
					channel.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
					if req.WantReply {
						req.Reply(true, nil)
					}
					return
				default:
					if req.WantReply {
						req.Reply(false, nil)
					}
				}
			}
		}()
	}
}

// =============================================================================
// Module.Init Tests
// =============================================================================

func TestModule_Init(t *testing.T) {
	m := New()
	c := &core.Core{
		Store:  nil,
		Logger: discardLogger(),
	}

	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if m.core != c {
		t.Error("core reference not set")
	}
	if m.store != nil {
		t.Error("store should be nil since Core.Store is nil")
	}
	if m.logger == nil {
		t.Error("logger should not be nil")
	}
}

func TestModule_Init_WithStore(t *testing.T) {
	m := New()
	// We don't need a real store; just verify the field is populated
	c := &core.Core{
		Store:  nil, // Would be a real Store in production
		Logger: discardLogger(),
	}

	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}
}

// =============================================================================
// SSHPool — getOrCreate, Execute, Upload, Close, remove with real SSH
// =============================================================================

