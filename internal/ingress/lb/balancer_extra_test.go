package lb

import (
	"net/http"
	"testing"
)

func TestRandom_Next_ReturnsValidBackend(t *testing.T) {
	rn := &Random{}
	backends := []string{"a:80", "b:80", "c:80"}

	r, _ := http.NewRequest("GET", "/", nil)
	r.RemoteAddr = "192.168.1.1:12345"

	for range 50 {
		got := rn.Next(backends, r)

		found := false
		for _, b := range backends {
			if b == got {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Random.Next returned %q which is not in backends", got)
		}
	}
}

func TestRandom_Next_SingleBackend(t *testing.T) {
	rn := &Random{}
	backends := []string{"only:80"}

	r, _ := http.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.1:1234"

	for range 10 {
		got := rn.Next(backends, r)
		if got != "only:80" {
			t.Errorf("expected 'only:80', got %q", got)
		}
	}
}

func TestRandom_Next_DifferentRemoteAddrs(t *testing.T) {
	rn := &Random{}
	backends := []string{"a:80", "b:80", "c:80", "d:80"}

	results := make(map[string]bool)
	for i := range 100 {
		r, _ := http.NewRequest("GET", "/", nil)
		r.RemoteAddr = "10.0.0." + itoa(i%256) + ":1234"
		got := rn.Next(backends, r)
		results[got] = true
	}

	// With different remote addresses and counter mixing, we should hit multiple backends
	if len(results) < 2 {
		t.Error("Random should distribute across multiple backends with different addresses")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := make([]byte, 0, 3)
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
