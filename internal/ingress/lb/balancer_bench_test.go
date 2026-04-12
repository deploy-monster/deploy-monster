package lb

import (
	"net/http"
	"testing"
)

func BenchmarkRoundRobin(b *testing.B) {
	rr := &RoundRobin{}
	backends := []string{"a:80", "b:80", "c:80", "d:80"}
	r, _ := http.NewRequest("GET", "/", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr.Next(backends, r)
	}
}

func BenchmarkIPHash(b *testing.B) {
	ih := &IPHash{}
	backends := []string{"a:80", "b:80", "c:80", "d:80"}
	r, _ := http.NewRequest("GET", "/", nil)
	r.RemoteAddr = "192.168.1.100:12345"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ih.Next(backends, r)
	}
}

func BenchmarkLeastConn(b *testing.B) {
	lc := NewLeastConn()
	backends := []string{"a:80", "b:80", "c:80", "d:80"}
	r, _ := http.NewRequest("GET", "/", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		backend := lc.Next(backends, r)
		lc.Release(backend)
	}
}
