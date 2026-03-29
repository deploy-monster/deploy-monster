package ingress

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthHandler_Healthy(t *testing.T) {
	m := &Module{
		router: NewRouteTable(),
		proxy:  NewReverseProxy(NewRouteTable(), nil),
	}

	req := httptest.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()

	m.healthHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	var health HealthStatus
	if err := json.NewDecoder(rr.Body).Decode(&health); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if health.Status != "healthy" {
		t.Errorf("expected status healthy, got %s", health.Status)
	}
}

func TestHealthHandler_Unhealthy(t *testing.T) {
	m := &Module{
		router: nil, // nil router = unhealthy
	}

	req := httptest.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()

	m.healthHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", rr.Code)
	}

	var health HealthStatus
	if err := json.NewDecoder(rr.Body).Decode(&health); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if health.Status != "unhealthy" {
		t.Errorf("expected status unhealthy, got %s", health.Status)
	}
}

func TestHealthHandler_RouteCount(t *testing.T) {
	rt := NewRouteTable()
	rt.Upsert(&RouteEntry{Host: "app1.example.com", Backends: []string{"127.0.0.1:3000"}})
	rt.Upsert(&RouteEntry{Host: "app2.example.com", Backends: []string{"127.0.0.1:3001"}})

	m := &Module{
		router: rt,
		proxy:  NewReverseProxy(rt, nil),
	}

	req := httptest.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()

	m.healthHandler().ServeHTTP(rr, req)

	var health HealthStatus
	if err := json.NewDecoder(rr.Body).Decode(&health); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if health.Routes != 2 {
		t.Errorf("expected 2 routes, got %d", health.Routes)
	}
}

func TestReadyHandler_Ready(t *testing.T) {
	m := &Module{
		router: NewRouteTable(),
		proxy:  NewReverseProxy(NewRouteTable(), nil),
	}

	req := httptest.NewRequest("GET", "/ready", nil)
	rr := httptest.NewRecorder()

	m.readyHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	if rr.Body.String() != "OK" {
		t.Errorf("expected body OK, got %s", rr.Body.String())
	}
}

func TestReadyHandler_NotReady_NilRouter(t *testing.T) {
	m := &Module{
		router: nil,
		proxy:  NewReverseProxy(NewRouteTable(), nil),
	}

	req := httptest.NewRequest("GET", "/ready", nil)
	rr := httptest.NewRecorder()

	m.readyHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", rr.Code)
	}
}

func TestReadyHandler_NotReady_NilProxy(t *testing.T) {
	m := &Module{
		router: NewRouteTable(),
		proxy:  nil,
	}

	req := httptest.NewRequest("GET", "/ready", nil)
	rr := httptest.NewRecorder()

	m.readyHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", rr.Code)
	}
}

func TestLiveHandler_AlwaysLive(t *testing.T) {
	m := &Module{}

	req := httptest.NewRequest("GET", "/live", nil)
	rr := httptest.NewRecorder()

	m.liveHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	if rr.Body.String() != "OK" {
		t.Errorf("expected body OK, got %s", rr.Body.String())
	}
}

func TestHealthHandler_ContentType(t *testing.T) {
	m := &Module{
		router: NewRouteTable(),
		proxy:  NewReverseProxy(NewRouteTable(), nil),
	}

	req := httptest.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()

	m.healthHandler().ServeHTTP(rr, req)

	contentType := rr.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}
}

func TestHealthHandler_CircuitBreakerStatus(t *testing.T) {
	rt := NewRouteTable()
	rt.Upsert(&RouteEntry{Host: "app.example.com", Backends: []string{"127.0.0.1:3000"}})

	m := &Module{
		router: rt,
		proxy:  NewReverseProxy(rt, nil),
	}

	// Record a failure to create circuit breaker
	m.proxy.circuit.RecordFailure("127.0.0.1:3000")

	req := httptest.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()

	m.healthHandler().ServeHTTP(rr, req)

	var health HealthStatus
	if err := json.NewDecoder(rr.Body).Decode(&health); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(health.Circuits) != 1 {
		t.Errorf("expected 1 circuit, got %d", len(health.Circuits))
	}

	if health.Circuits["127.0.0.1:3000"] != "closed" {
		t.Errorf("expected circuit state closed, got %s", health.Circuits["127.0.0.1:3000"])
	}
}
