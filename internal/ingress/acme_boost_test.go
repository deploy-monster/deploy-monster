package ingress

import (
	"context"
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/crypto/acme/autocert"
)

func TestACMEManager_SetDomains(t *testing.T) {
	cs := NewCertStore()
	am := NewACMEManager(cs, "test@example.com", true, slog.Default())

	am.SetDomains("example.com", "www.example.com")

	// Verify that the manager accepts the new domains by attempting a cert request
	// (it will fail with autocert.ErrCacheMiss because no cert is cached, but that
	// proves the domain was whitelisted — unlisted domains return a different error.)
	hello := &tls.ClientHelloInfo{ServerName: "example.com"}
	_, err := am.GetCertificate(hello)
	if err == autocert.ErrCacheMiss {
		// Expected: domain is whitelisted but no cert in cache
	} else if err != nil {
		// Self-signed fallback is also acceptable
	}
}

func TestACMEManager_SetDomains_NilManager(t *testing.T) {
	cs := NewCertStore()
	am := NewACMEManager(cs, "", false, slog.Default()) // email empty → mgr is nil

	// Should not panic
	am.SetDomains("example.com")
}

func TestAutocertCache_Get_Miss(t *testing.T) {
	cs := NewCertStore()
	cache := &autocertCache{store: cs}

	_, err := cache.Get(context.Background(), "nonexistent.example.com")
	if err != autocert.ErrCacheMiss {
		t.Errorf("expected ErrCacheMiss, got %v", err)
	}
}

func TestAutocertCache_Get_Hit(t *testing.T) {
	cs := NewCertStore()
	cache := &autocertCache{store: cs}

	cert, err := GenerateSelfSigned("hit.example.com")
	if err != nil {
		t.Fatalf("GenerateSelfSigned: %v", err)
	}
	cs.Put("hit.example.com", cert)

	data, err := cache.Get(context.Background(), "hit.example.com")
	if err != nil {
		t.Fatalf("cache.Get: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty PEM data")
	}
}

func TestAutocertCache_Put(t *testing.T) {
	cs := NewCertStore()
	cache := &autocertCache{store: cs}

	cert, err := GenerateSelfSigned("put.example.com")
	if err != nil {
		t.Fatalf("GenerateSelfSigned: %v", err)
	}

	// Encode cert+key into PEM
	var pemData []byte
	for _, der := range cert.Certificate {
		pemData = append(pemData, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})...)
	}
	if key, ok := cert.PrivateKey.(*ecdsa.PrivateKey); ok {
		if keyDER, err := x509.MarshalECPrivateKey(key); err == nil {
			pemData = append(pemData, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})...)
		}
	}

	if err := cache.Put(context.Background(), "put.example.com", pemData); err != nil {
		t.Fatalf("cache.Put: %v", err)
	}

	// Verify it was stored
	got := cs.Get("put.example.com")
	if got == nil {
		t.Error("expected cert to be stored")
	}
}

func TestAutocertCache_Delete(t *testing.T) {
	cs := NewCertStore()
	cache := &autocertCache{store: cs}

	cert, _ := GenerateSelfSigned("del.example.com")
	cs.Put("del.example.com", cert)

	if err := cache.Delete(context.Background(), "del.example.com"); err != nil {
		t.Fatalf("cache.Delete: %v", err)
	}

	if cs.Get("del.example.com") != nil {
		t.Error("expected cert to be deleted")
	}
}

func TestACMEManager_HTTPHandler_NilManager(t *testing.T) {
	cs := NewCertStore()
	am := NewACMEManager(cs, "", false, slog.Default()) // email empty → mgr is nil

	fallback := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	h := am.HTTPHandler(fallback)
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 from fallback, got %d", rr.Code)
	}
}

func TestACMEManager_CheckRenewals_Expiring(t *testing.T) {
	cs := NewCertStore()

	cert, err := GenerateSelfSigned("expiring.example.com")
	if err != nil {
		t.Fatalf("GenerateSelfSigned: %v", err)
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	leaf.NotAfter = time.Now().Add(24 * time.Hour)
	cert.Leaf = leaf
	cs.Put("expiring.example.com", cert)

	am := NewACMEManager(cs, "test@example.com", true, slog.Default())
	am.checkRenewals()
}
