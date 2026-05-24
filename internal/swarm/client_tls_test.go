package swarm

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestAgentClient_Dial_UsesTLSConfigRootCA(t *testing.T) {
	certFile, keyFile := writeSelfSignedAgentCert(t)
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		t.Fatalf("load test cert: %v", err)
	}

	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
	})
	if err != nil {
		t.Fatalf("tls listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		reader := bufio.NewReader(conn)
		if _, err := http.ReadRequest(reader); err != nil {
			return
		}
		_, _ = conn.Write([]byte("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: deploymonster-agent/1\r\n\r\n"))
	}()

	client := NewAgentClient("https://"+ln.Addr().String(), "agent-tls", "token", "1.0.0", &mockRuntime{}, discardLogger(), certFile, keyFile, certFile)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.dial(ctx); err != nil {
		t.Fatalf("dial with configured root CA: %v", err)
	}
	defer func() { _ = client.conn.Close() }()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("TLS test server did not handle the upgrade request")
	}
}

func TestAgentClient_HandleContainerCreate_NilRuntimeReturnsError(t *testing.T) {
	client := NewAgentClient("http://master", "agent-nil", "token", "1.0.0", nil, discardLogger(), "", "", "")
	_, err := client.handleContainerCreate(context.Background(), core.AgentMessage{
		Payload: core.ContainerOpts{Image: "alpine:latest"},
	})
	if err == nil {
		t.Fatal("expected nil runtime error")
	}
	if !strings.Contains(err.Error(), "container runtime not configured") {
		t.Fatalf("error = %q, want runtime-not-configured message", err.Error())
	}
}

func writeSelfSignedAgentCert(t *testing.T) (certFile, keyFile string) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}

	dir := t.TempDir()
	certFile = filepath.Join(dir, "agent.pem")
	keyFile = filepath.Join(dir, "agent-key.pem")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	keyBytes := x509.MarshalPKCS1PrivateKey(key)
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyBytes}), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	return certFile, keyFile
}
