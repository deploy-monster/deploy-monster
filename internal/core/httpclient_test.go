package core

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewHTTPClient_ReusesTransport(t *testing.T) {
	c1 := NewHTTPClient(5 * time.Second)
	c2 := NewHTTPClient(10 * time.Second)

	if c1.Transport != c2.Transport {
		t.Error("NewHTTPClient should reuse the same transport across calls")
	}

	if c1.Timeout != 5*time.Second {
		t.Errorf("c1.Timeout = %v, want 5s", c1.Timeout)
	}
	if c2.Timeout != 10*time.Second {
		t.Errorf("c2.Timeout = %v, want 10s", c2.Timeout)
	}
}

func TestNewHTTPClient_TransportConfig(t *testing.T) {
	c := NewHTTPClient(5 * time.Second)
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatal("Transport is not *http.Transport")
	}

	if tr.MaxIdleConnsPerHost < 10 {
		t.Errorf("MaxIdleConnsPerHost = %d, want >= 10", tr.MaxIdleConnsPerHost)
	}
	if tr.MaxIdleConns < 100 {
		t.Errorf("MaxIdleConns = %d, want >= 100", tr.MaxIdleConns)
	}
	if tr.IdleConnTimeout < 30*time.Second {
		t.Errorf("IdleConnTimeout = %v, want >= 30s", tr.IdleConnTimeout)
	}
	if !tr.ForceAttemptHTTP2 {
		t.Error("ForceAttemptHTTP2 should be true")
	}
}

func TestNewHTTPClient_Works(t *testing.T) {
	// Sanity test that the client actually makes requests
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := NewHTTPClient(5 * time.Second)
	resp, err := c.Get(srv.URL)
	if err != nil {
		t.Fatalf("Get returned %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestNewHTTPClient_NoTimeout(t *testing.T) {
	c := NewHTTPClient(0)
	if c.Timeout != 0 {
		t.Errorf("Timeout = %v, want 0", c.Timeout)
	}
	if c.Transport == nil {
		t.Error("Transport should not be nil")
	}
}
