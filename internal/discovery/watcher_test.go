package discovery

import (
	"testing"
)

// --- parseRule ---

func TestParseRule(t *testing.T) {
	tests := []struct {
		rule     string
		wantHost string
		wantPath string
	}{
		{"Host(`example.com`)", "example.com", ""},
		{"Host(`app.example.com`) && PathPrefix(`/api`)", "app.example.com", "/api"},
		{"Host(`*.example.com`)", "*.example.com", ""},
		{"PathPrefix(`/v2`)", "", "/v2"},
		{"", "", ""},
		{"Host(`a.b.c.d.example.com`)", "a.b.c.d.example.com", ""},
		{"Host(`localhost`)", "localhost", ""},
		{"Host(`example.com`) && PathPrefix(`/api/v2/users`)", "example.com", "/api/v2/users"},
		{"PathPrefix(`/`) && Host(`app.com`)", "app.com", "/"},
		{"Host(`app.com`) && PathPrefix(`/`)", "app.com", "/"},
		{"Host(`app.com`)&&PathPrefix(`/tight`)", "app.com", "/tight"},
	}

	for _, tt := range tests {
		t.Run(tt.rule, func(t *testing.T) {
			host, path := parseRule(tt.rule)
			if host != tt.wantHost {
				t.Errorf("parseRule(%q) host = %q, want %q", tt.rule, host, tt.wantHost)
			}
			if path != tt.wantPath {
				t.Errorf("parseRule(%q) path = %q, want %q", tt.rule, path, tt.wantPath)
			}
		})
	}
}

func TestParseRule_OnlyHost(t *testing.T) {
	host, path := parseRule("Host(`single.com`)")
	if host != "single.com" {
		t.Errorf("host = %q, want single.com", host)
	}
	if path != "" {
		t.Errorf("path = %q, want empty", path)
	}
}

func TestParseRule_OnlyPathPrefix(t *testing.T) {
	host, path := parseRule("PathPrefix(`/health`)")
	if host != "" {
		t.Errorf("host = %q, want empty", host)
	}
	if path != "/health" {
		t.Errorf("path = %q, want /health", path)
	}
}

func TestParseRule_MalformedHost(t *testing.T) {
	// Missing closing backtick+paren.
	host, path := parseRule("Host(`broken")
	if host != "" {
		t.Errorf("host = %q, want empty for malformed rule", host)
	}
	if path != "" {
		t.Errorf("path = %q, want empty for malformed rule", path)
	}
}

func TestParseRule_MalformedPathPrefix(t *testing.T) {
	host, path := parseRule("Host(`ok.com`) && PathPrefix(`/broken")
	if host != "ok.com" {
		t.Errorf("host = %q, want ok.com", host)
	}
	if path != "" {
		t.Errorf("path = %q, want empty for malformed PathPrefix", path)
	}
}

// --- ParseLabelsToRoute ---

func TestParseLabelsToRoute(t *testing.T) {
	labels := map[string]string{
		"monster.app.id":                  "app-123",
		"monster.app.name":                "myapp",
		"monster.http.routers.myapp.rule": "Host(`myapp.example.com`)",
		"monster.http.services.myapp.loadbalancer.server.port": "3000",
		"monster.http.routers.myapp.middlewares":               "ratelimit, cors",
	}

	route := ParseLabelsToRoute(labels, "abc123def456789")
	if route == nil {
		t.Fatal("expected route, got nil")
	}

	if route.Host != "myapp.example.com" {
		t.Errorf("host = %q, want myapp.example.com", route.Host)
	}
	if route.AppID != "app-123" {
		t.Errorf("app_id = %q, want app-123", route.AppID)
	}
	if route.ServiceName != "myapp" {
		t.Errorf("service_name = %q, want myapp", route.ServiceName)
	}
	if len(route.Backends) != 1 {
		t.Fatalf("backends count = %d, want 1", len(route.Backends))
	}
	if route.Backends[0] != "abc123def456:3000" {
		t.Errorf("backend = %q, want abc123def456:3000", route.Backends[0])
	}
	if len(route.Middlewares) != 2 {
		t.Fatalf("middlewares count = %d, want 2", len(route.Middlewares))
	}
	if route.Middlewares[0] != "ratelimit" {
		t.Errorf("middleware[0] = %q, want ratelimit", route.Middlewares[0])
	}
	if route.Middlewares[1] != "cors" {
		t.Errorf("middleware[1] = %q, want cors", route.Middlewares[1])
	}
	if route.PathPrefix != "/" {
		t.Errorf("path_prefix = %q, want /", route.PathPrefix)
	}
	if route.Priority != 100 {
		t.Errorf("priority = %d, want 100", route.Priority)
	}
	if route.LBStrategy != "round-robin" {
		t.Errorf("lb_strategy = %q, want round-robin", route.LBStrategy)
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

func TestParseLabelsToRoute_DefaultPort(t *testing.T) {
	labels := map[string]string{
		"monster.app.id":                   "app-456",
		"monster.app.name":                 "webapp",
		"monster.http.routers.webapp.rule": "Host(`webapp.com`)",
		// No port label — should default to 80.
	}

	route := ParseLabelsToRoute(labels, "container123456789")
	if route == nil {
		t.Fatal("expected route, got nil")
	}

	if route.Backends[0] != "container123:80" {
		t.Errorf("backend = %q, want container123:80 (default port)", route.Backends[0])
	}
}

func TestParseLabelsToRoute_WithPathPrefix(t *testing.T) {
	labels := map[string]string{
		"monster.app.id":                "app-789",
		"monster.app.name":              "api",
		"monster.http.routers.api.rule": "Host(`api.example.com`) && PathPrefix(`/v1`)",
		"monster.http.services.api.loadbalancer.server.port": "8080",
	}

	route := ParseLabelsToRoute(labels, "container789012345")
	if route == nil {
		t.Fatal("expected route, got nil")
	}

	if route.Host != "api.example.com" {
		t.Errorf("host = %q, want api.example.com", route.Host)
	}
	if route.PathPrefix != "/v1" {
		t.Errorf("path_prefix = %q, want /v1", route.PathPrefix)
	}
}

func TestParseLabelsToRoute_CustomLBStrategy(t *testing.T) {
	labels := map[string]string{
		"monster.app.id":                  "app-lb",
		"monster.app.name":                "lbapp",
		"monster.http.routers.lbapp.rule": "Host(`lb.example.com`)",
		"monster.http.services.lbapp.loadbalancer.server.port": "9090",
		"monster.http.services.lbapp.loadbalancer.strategy":    "least-conn",
	}

	route := ParseLabelsToRoute(labels, "container_lb_123456")
	if route == nil {
		t.Fatal("expected route, got nil")
	}

	if route.LBStrategy != "least-conn" {
		t.Errorf("lb_strategy = %q, want least-conn", route.LBStrategy)
	}
}

func TestParseLabelsToRoute_NoMiddlewares(t *testing.T) {
	labels := map[string]string{
		"monster.app.id":                   "app-nomw",
		"monster.app.name":                 "simple",
		"monster.http.routers.simple.rule": "Host(`simple.com`)",
		"monster.http.services.simple.loadbalancer.server.port": "3000",
	}

	route := ParseLabelsToRoute(labels, "container_simple_1234567890")
	if route == nil {
		t.Fatal("expected route, got nil")
	}

	if len(route.Middlewares) != 0 {
		t.Errorf("middlewares = %v, want empty", route.Middlewares)
	}
}

func TestParseLabelsToRoute_MultipleMiddlewares(t *testing.T) {
	labels := map[string]string{
		"monster.app.id":                    "app-mw",
		"monster.app.name":                  "secured",
		"monster.http.routers.secured.rule": "Host(`secure.com`)",
		"monster.http.services.secured.loadbalancer.server.port": "443",
		"monster.http.routers.secured.middlewares":               "auth, ratelimit, cors, compress",
	}

	route := ParseLabelsToRoute(labels, "container_secured_1234567890")
	if route == nil {
		t.Fatal("expected route, got nil")
	}

	if len(route.Middlewares) != 4 {
		t.Fatalf("middlewares count = %d, want 4", len(route.Middlewares))
	}

	expected := []string{"auth", "ratelimit", "cors", "compress"}
	for i, want := range expected {
		if route.Middlewares[i] != want {
			t.Errorf("middleware[%d] = %q, want %q", i, route.Middlewares[i], want)
		}
	}
}

func TestParseLabelsToRoute_ShortContainerID(t *testing.T) {
	labels := map[string]string{
		"monster.app.id":                    "app-short",
		"monster.app.name":                  "shortid",
		"monster.http.routers.shortid.rule": "Host(`short.com`)",
	}

	// Container ID of exactly 12 characters.
	route := ParseLabelsToRoute(labels, "abc123def456")
	if route == nil {
		t.Fatal("expected route, got nil")
	}

	if route.Backends[0] != "abc123def456:80" {
		t.Errorf("backend = %q, want abc123def456:80", route.Backends[0])
	}
}

func TestParseLabelsToRoute_WildcardHost(t *testing.T) {
	labels := map[string]string{
		"monster.app.id":                     "app-wildcard",
		"monster.app.name":                   "wildcard",
		"monster.http.routers.wildcard.rule": "Host(`*.example.com`)",
	}

	route := ParseLabelsToRoute(labels, "container_wildcard_1234567890")
	if route == nil {
		t.Fatal("expected route, got nil")
	}

	if route.Host != "*.example.com" {
		t.Errorf("host = %q, want *.example.com", route.Host)
	}
}

// --- Watcher creation ---

func TestNewWatcher(t *testing.T) {
	// Verify NewWatcher returns a properly initialized watcher.
	// We cannot easily test Start/Stop without a real ContainerRuntime,
	// but we can verify the struct is created correctly.
	w := NewWatcher(nil, nil, nil, nil)
	if w == nil {
		t.Fatal("NewWatcher returned nil")
	}
	if w.stopCh == nil {
		t.Error("stopCh should be initialized")
	}
}

func TestWatcher_StopClosesChannel(t *testing.T) {
	w := NewWatcher(nil, nil, nil, nil)

	// Stop should close the channel without panic.
	w.Stop()

	// Verify channel is closed.
	select {
	case <-w.stopCh:
		// Good, channel is closed.
	default:
		t.Error("stopCh should be closed after Stop()")
	}
}
