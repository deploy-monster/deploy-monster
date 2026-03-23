package ingress

import "testing"

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
