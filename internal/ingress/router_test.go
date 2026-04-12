package ingress

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRouteTable_Match_ExactHost(t *testing.T) {
	rt := NewRouteTable()
	rt.Upsert(&RouteEntry{Host: "app.example.com", PathPrefix: "/", Backends: []string{"127.0.0.1:3000"}})

	got := rt.Match("app.example.com", "/anything")
	if got == nil {
		t.Fatal("expected match for exact host")
	}
	if got.Host != "app.example.com" {
		t.Errorf("expected host 'app.example.com', got %q", got.Host)
	}
}

func TestRouteTable_Match_WildcardHost(t *testing.T) {
	rt := NewRouteTable()
	rt.Upsert(&RouteEntry{Host: "*.example.com", PathPrefix: "/", Backends: []string{"127.0.0.1:3000"}})

	if rt.Match("app.example.com", "/") == nil {
		t.Error("wildcard *.example.com should match app.example.com")
	}
	if rt.Match("api.example.com", "/test") == nil {
		t.Error("wildcard *.example.com should match api.example.com")
	}
	if rt.Match("other.domain.com", "/") != nil {
		t.Error("wildcard *.example.com should NOT match other.domain.com")
	}
}

func TestRouteTable_Match_PathPrefix(t *testing.T) {
	rt := NewRouteTable()
	rt.Upsert(&RouteEntry{Host: "example.com", PathPrefix: "/api", Backends: []string{"127.0.0.1:8080"}, Priority: 200})
	rt.Upsert(&RouteEntry{Host: "example.com", PathPrefix: "/", Backends: []string{"127.0.0.1:3000"}, Priority: 100})

	// /api should match the /api route (higher priority)
	got := rt.Match("example.com", "/api/users")
	if got == nil || got.PathPrefix != "/api" {
		t.Error("expected /api route for /api/users")
	}

	// / should match the fallback route
	got = rt.Match("example.com", "/about")
	if got == nil || got.PathPrefix != "/" {
		t.Error("expected / route for /about")
	}
}

func TestRouteTable_Match_NoMatch(t *testing.T) {
	rt := NewRouteTable()
	rt.Upsert(&RouteEntry{Host: "app.example.com", PathPrefix: "/"})

	if rt.Match("unknown.com", "/") != nil {
		t.Error("should not match unknown host")
	}
}

func TestRouteTable_Upsert_Replace(t *testing.T) {
	rt := NewRouteTable()
	rt.Upsert(&RouteEntry{Host: "app.com", PathPrefix: "/", Backends: []string{"old:80"}})
	rt.Upsert(&RouteEntry{Host: "app.com", PathPrefix: "/", Backends: []string{"new:80"}})

	if rt.Count() != 1 {
		t.Errorf("expected 1 route after upsert, got %d", rt.Count())
	}

	got := rt.Match("app.com", "/")
	if got.Backends[0] != "new:80" {
		t.Error("upsert should replace existing route")
	}
}

func TestRouteTable_Remove(t *testing.T) {
	rt := NewRouteTable()
	rt.Upsert(&RouteEntry{Host: "app.com", PathPrefix: "/"})

	rt.Remove("app.com", "/")
	if rt.Count() != 0 {
		t.Error("expected 0 routes after remove")
	}
}

func TestRouteTable_RemoveByAppID(t *testing.T) {
	rt := NewRouteTable()
	rt.Upsert(&RouteEntry{Host: "a.com", PathPrefix: "/", AppID: "app-1"})
	rt.Upsert(&RouteEntry{Host: "b.com", PathPrefix: "/", AppID: "app-1"})
	rt.Upsert(&RouteEntry{Host: "c.com", PathPrefix: "/", AppID: "app-2"})

	rt.RemoveByAppID("app-1")

	if rt.Count() != 1 {
		t.Errorf("expected 1 route after RemoveByAppID, got %d", rt.Count())
	}
}

func TestRouteTable_Priority_Order(t *testing.T) {
	rt := NewRouteTable()
	rt.Upsert(&RouteEntry{Host: "app.com", PathPrefix: "/", Priority: 10, ServiceName: "low"})
	rt.Upsert(&RouteEntry{Host: "app.com", PathPrefix: "/", Priority: 100, ServiceName: "high"})

	// After upsert with same host+path, should keep last one
	// But if different paths with same host, priority matters
	rt2 := NewRouteTable()
	rt2.Upsert(&RouteEntry{Host: "app.com", PathPrefix: "/api", Priority: 200, ServiceName: "api"})
	rt2.Upsert(&RouteEntry{Host: "app.com", PathPrefix: "/", Priority: 100, ServiceName: "web"})

	got := rt2.Match("app.com", "/api/test")
	if got == nil || got.ServiceName != "api" {
		t.Error("higher priority route should match first")
	}
}

// TestRouteTable_LockFree_ConcurrentReadsWrites runs many concurrent
// readers against a single writer that is continuously churning routes.
// The atomic-snapshot implementation guarantees:
//
//   - readers never see a partially-applied write (always a whole
//     pre-write or post-write snapshot),
//   - readers never block on a mutex while the writer holds it,
//   - iteration is safe without copying since the snapshot's entries
//     slice is never mutated after publication.
//
// A race-detector run under -race catches any data race on the entries
// slice. A non-race run still exercises the functional invariants: for
// a stable baseline route, Match must return a non-nil entry from every
// reader call regardless of concurrent churn.
func TestRouteTable_LockFree_ConcurrentReadsWrites(t *testing.T) {
	rt := NewRouteTable()

	// Baseline route that must remain matchable throughout the run.
	baseline := &RouteEntry{
		Host:     "stable.example.com",
		Backends: []string{"10.0.0.1:80"},
		Priority: 100,
		AppID:    "stable",
	}
	rt.Upsert(baseline)

	var stop atomic.Bool
	defer stop.Store(true)

	// Writer: churn 16 different hosts for the life of the test.
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		var rev int
		for !stop.Load() {
			rev++
			rt.Upsert(&RouteEntry{
				Host:     fmt.Sprintf("churn-%d.example.com", rev%16),
				Backends: []string{"10.0.0.2:80"},
				Priority: 50,
				AppID:    "churn",
			})
			if rev%32 == 0 {
				rt.RemoveByAppID("churn")
			}
		}
	}()

	// 8 reader goroutines each doing 10k Match calls.
	var readerWG sync.WaitGroup
	readerMisses := atomic.Int64{}
	for i := 0; i < 8; i++ {
		readerWG.Add(1)
		go func() {
			defer readerWG.Done()
			for j := 0; j < 10000; j++ {
				if rt.Match("stable.example.com", "/") == nil {
					readerMisses.Add(1)
				}
			}
		}()
	}

	// Let the readers drive the test length. Writer runs until they finish.
	readerWG.Wait()
	stop.Store(true)
	select {
	case <-writerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("writer goroutine did not stop in time")
	}

	if misses := readerMisses.Load(); misses != 0 {
		t.Errorf("stable route was missed %d times during concurrent churn — atomic snapshot broke invariant", misses)
	}
}

func Test_matchHost(t *testing.T) {
	tests := []struct {
		pattern, host string
		want          bool
	}{
		{"example.com", "example.com", true},
		{"example.com", "other.com", false},
		{"*.example.com", "app.example.com", true},
		{"*.example.com", "api.example.com", true},
		{"*.example.com", "example.com", false},
		{"*.example.com", "sub.app.example.com", false}, // Only one level
	}

	for _, tt := range tests {
		got := matchHost(tt.pattern, tt.host)
		if got != tt.want {
			t.Errorf("matchHost(%q, %q) = %v, want %v", tt.pattern, tt.host, got, tt.want)
		}
	}
}
