package discovery

import (
	"testing"
)

func TestParseRule(t *testing.T) {
	tests := []struct {
		rule       string
		wantHost   string
		wantPath   string
	}{
		{"Host(`example.com`)", "example.com", ""},
		{"Host(`app.example.com`) && PathPrefix(`/api`)", "app.example.com", "/api"},
		{"Host(`*.example.com`)", "*.example.com", ""},
		{"PathPrefix(`/v2`)", "", "/v2"},
		{"", "", ""},
	}

	for _, tt := range tests {
		host, path := parseRule(tt.rule)
		if host != tt.wantHost {
			t.Errorf("parseRule(%q) host = %q, want %q", tt.rule, host, tt.wantHost)
		}
		if path != tt.wantPath {
			t.Errorf("parseRule(%q) path = %q, want %q", tt.rule, path, tt.wantPath)
		}
	}
}

func TestParseLabelsToRoute(t *testing.T) {
	labels := map[string]string{
		"monster.app.id":                                       "app-123",
		"monster.app.name":                                     "myapp",
		"monster.http.routers.myapp.rule":                      "Host(`myapp.example.com`)",
		"monster.http.services.myapp.loadbalancer.server.port": "3000",
		"monster.http.routers.myapp.middlewares":               "ratelimit, cors",
	}

	route := ParseLabelsToRoute(labels, "abc123def456789")
	if route == nil {
		t.Fatal("expected route, got nil")
	}

	if route.Host != "myapp.example.com" {
		t.Errorf("host = %q", route.Host)
	}
	if route.AppID != "app-123" {
		t.Errorf("app_id = %q", route.AppID)
	}
	if len(route.Backends) != 1 {
		t.Errorf("backends = %d", len(route.Backends))
	}
	if len(route.Middlewares) != 2 {
		t.Errorf("middlewares = %d, want 2", len(route.Middlewares))
	}
}

func TestParseLabelsToRoute_NoHost(t *testing.T) {
	labels := map[string]string{
		"monster.app.id":   "app-123",
		"monster.app.name": "myapp",
	}

	route := ParseLabelsToRoute(labels, "abc123")
	if route != nil {
		t.Error("should return nil when no host rule")
	}
}
