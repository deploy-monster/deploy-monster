package lb

import (
	"net/http"
	"testing"
)

func TestRoundRobin(t *testing.T) {
	rr := &RoundRobin{}
	backends := []string{"a:80", "b:80", "c:80"}

	// Should cycle through backends
	results := make(map[string]int)
	for range 9 {
		b := rr.Next(backends, nil)
		results[b]++
	}

	for _, b := range backends {
		if results[b] != 3 {
			t.Errorf("expected 3 requests to %s, got %d", b, results[b])
		}
	}
}

func TestIPHash_Consistent(t *testing.T) {
	ih := &IPHash{}
	backends := []string{"a:80", "b:80", "c:80"}

	r, _ := http.NewRequest("GET", "/", nil)
	r.RemoteAddr = "192.168.1.100:12345"

	// Same IP should always get the same backend
	first := ih.Next(backends, r)
	for range 10 {
		got := ih.Next(backends, r)
		if got != first {
			t.Errorf("IP hash should be consistent: got %s, expected %s", got, first)
		}
	}
}

func TestIPHash_Distributes(t *testing.T) {
	ih := &IPHash{}
	backends := []string{"a:80", "b:80", "c:80", "d:80"}

	results := make(map[string]bool)
	for i := range 100 {
		r, _ := http.NewRequest("GET", "/", nil)
		r.RemoteAddr = "192.168.1." + string(rune('0'+i%10)) + ":80"
		b := ih.Next(backends, r)
		results[b] = true
	}

	// With 100 different IPs, we should hit multiple backends
	if len(results) < 2 {
		t.Error("IP hash should distribute across multiple backends")
	}
}

func TestLeastConn(t *testing.T) {
	lc := NewLeastConn()
	backends := []string{"a:80", "b:80"}

	// First request should go to first backend (both at 0)
	b1 := lc.Next(backends, nil)
	// Second should go to the other one (first now at 1)
	b2 := lc.Next(backends, nil)

	if b1 == b2 {
		t.Error("least-conn should distribute to different backends when counts are equal")
	}

	// Release one, next should go there
	lc.Release(b1)
	b3 := lc.Next(backends, nil)
	if b3 != b1 {
		t.Errorf("after release, should route to %s, got %s", b1, b3)
	}
}

func TestNew(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"round-robin", "*lb.RoundRobin"},
		{"least-conn", "*lb.LeastConn"},
		{"ip-hash", "*lb.IPHash"},
		{"random", "*lb.Random"},
		{"unknown", "*lb.RoundRobin"}, // default
	}

	for _, tt := range tests {
		s := New(tt.name)
		if s == nil {
			t.Errorf("New(%q) returned nil", tt.name)
		}
	}
}
