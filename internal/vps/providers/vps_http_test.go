package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// TestMain shrinks the shared VPS HTTP retry timings so retry-exercising tests
// complete in milliseconds instead of seconds. The production defaults remain
// in place for real runs; only the test binary sees these values.
func TestMain(m *testing.M) {
	vpsHTTPRetryConfig = core.RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     5 * time.Millisecond,
	}
	os.Exit(m.Run())
}

// =============================================================================
// parseRetryAfter unit tests
// =============================================================================

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  time.Duration
	}{
		{"empty", "", 0},
		{"whitespace", "   ", 0},
		{"delta seconds", "10", 10 * time.Second},
		{"zero seconds", "0", 0},
		{"negative seconds", "-5", 0},
		{"large delta", "3600", 3600 * time.Second},
		{"http date in future", time.Now().Add(30 * time.Second).UTC().Format(http.TimeFormat), 0}, // approx
		{"http date in past", "Mon, 01 Jan 1990 00:00:00 GMT", 0},
		{"garbage", "not-a-value", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseRetryAfter(tt.input)
			// Past/garbage/empty should be exactly zero. For future HTTP-dates
			// the value depends on wall clock jitter, so accept any positive.
			if tt.name == "http date in future" {
				if got <= 0 || got > 31*time.Second {
					t.Errorf("parseRetryAfter future = %v, want (0, 31s]", got)
				}
				return
			}
			if got != tt.want {
				t.Errorf("parseRetryAfter(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// =============================================================================
// truncateBody unit tests
// =============================================================================

func TestTruncateBody(t *testing.T) {
	if got := truncateBody([]byte("hi"), 10); got != "hi" {
		t.Errorf("short body: got %q", got)
	}
	if got := truncateBody([]byte("0123456789abcdef"), 5); got != "01234..." {
		t.Errorf("long body: got %q", got)
	}
	if got := truncateBody([]byte{}, 5); got != "" {
		t.Errorf("empty body: got %q", got)
	}
}

// =============================================================================
// vpsDoRequest: 429 with Retry-After is retried after the delay
// =============================================================================

func TestVPSDoRequest_429RetryAfter(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.Header().Set("Retry-After", "0") // minimize test duration
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"error":"rate limited"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	body, err := vpsDoRequest(
		context.Background(),
		srv.Client(),
		nil,
		"test",
		http.MethodGet,
		srv.URL+"/x",
		"token",
		nil,
	)
	if err != nil {
		t.Fatalf("vpsDoRequest: %v", err)
	}
	if !strings.Contains(string(body), "ok") {
		t.Errorf("body = %q, want to contain 'ok'", string(body))
	}
	if atomic.LoadInt32(&calls) < 2 {
		t.Errorf("calls = %d, want >= 2 (429 should have been retried)", calls)
	}
}

// =============================================================================
// vpsDoRequest: 5xx is retried
// =============================================================================

func TestVPSDoRequest_5xxRetried(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 2 {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	body, err := vpsDoRequest(
		context.Background(),
		srv.Client(),
		nil,
		"test",
		http.MethodGet,
		srv.URL+"/x",
		"token",
		nil,
	)
	if err != nil {
		t.Fatalf("vpsDoRequest: %v", err)
	}
	if !strings.Contains(string(body), "ok") {
		t.Errorf("body = %q, want to contain 'ok'", string(body))
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
}

// =============================================================================
// vpsDoRequest: non-429 4xx is NOT retried
// =============================================================================

func TestVPSDoRequest_4xxNotRetried(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		http.Error(w, `{"error":"validation failed"}`, http.StatusUnprocessableEntity)
	}))
	defer srv.Close()

	_, err := vpsDoRequest(
		context.Background(),
		srv.Client(),
		nil,
		"test",
		http.MethodGet,
		srv.URL+"/x",
		"token",
		nil,
	)
	if err == nil {
		t.Fatal("expected error for 422")
	}
	if !strings.Contains(err.Error(), "422") {
		t.Errorf("error = %q, want to contain '422'", err.Error())
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("calls = %d, want exactly 1 (4xx must not be retried)", calls)
	}
}

// =============================================================================
// vpsDoRequest: exhausted retries on persistent 5xx
// =============================================================================

func TestVPSDoRequest_5xxExhausted(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		http.Error(w, "boom", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	_, err := vpsDoRequest(
		context.Background(),
		srv.Client(),
		nil,
		"test",
		http.MethodGet,
		srv.URL+"/x",
		"token",
		nil,
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "503") {
		t.Errorf("error = %q, want to contain '503'", err.Error())
	}
	if atomic.LoadInt32(&calls) != int32(vpsHTTPRetryConfig.MaxAttempts) {
		t.Errorf("calls = %d, want %d", calls, vpsHTTPRetryConfig.MaxAttempts)
	}
}

// =============================================================================
// vpsDoRequest: invalid method → ErrNoRetry → one call only
// =============================================================================

func TestVPSDoRequest_InvalidMethod(t *testing.T) {
	_, err := vpsDoRequest(
		context.Background(),
		http.DefaultClient,
		nil,
		"test",
		"BAD METHOD",
		"http://127.0.0.1:0/x",
		"token",
		nil,
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "build request") {
		t.Errorf("error = %q, want to contain 'build request'", err.Error())
	}
}

// =============================================================================
// vpsDoRequest: context cancellation is terminal
// =============================================================================

func TestVPSDoRequest_ContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block to let the context cancellation fire mid-request.
		select {
		case <-time.After(500 * time.Millisecond):
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancelled

	_, err := vpsDoRequest(
		ctx,
		srv.Client(),
		nil,
		"test",
		http.MethodGet,
		srv.URL+"/x",
		"token",
		nil,
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
}

// =============================================================================
// Decode errors are now propagated (previously silently ignored)
// =============================================================================

func TestHetzner_ListRegions_DecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `not valid json`)
	}))
	defer srv.Close()
	h := &Hetzner{token: "t", client: rewriteClient(srv.URL, hetznerAPI)}
	_, err := h.ListRegions(context.Background())
	if err == nil {
		t.Fatal("expected decode error")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Errorf("error = %q, want to contain 'decode'", err.Error())
	}
}

func TestDO_ListRegions_DecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{}`) // malformed
	}))
	defer srv.Close()
	d := &DigitalOcean{token: "t", client: rewriteClient(srv.URL, doAPI)}
	_, err := d.ListRegions(context.Background())
	if err == nil {
		t.Fatal("expected decode error")
	}
}

func TestVultr_ListRegions_DecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `garbage`)
	}))
	defer srv.Close()
	v := &Vultr{token: "t", client: rewriteClient(srv.URL, vultrAPI)}
	_, err := v.ListRegions(context.Background())
	if err == nil {
		t.Fatal("expected decode error")
	}
}

func TestLinode_ListRegions_DecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{{`)
	}))
	defer srv.Close()
	l := &Linode{token: "t", client: rewriteClient(srv.URL, linodeAPI)}
	_, err := l.ListRegions(context.Background())
	if err == nil {
		t.Fatal("expected decode error")
	}
}

// =============================================================================
// Pagination tests — each provider's list endpoints accumulate multiple pages
// =============================================================================

func TestHetzner_ListRegions_Pagination(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		page := r.URL.Query().Get("page")
		switch page {
		case "1":
			next := 2
			json.NewEncoder(w).Encode(map[string]any{
				"locations": []map[string]any{
					{"name": "fsn1", "description": "Falkenstein", "city": "Falkenstein"},
				},
				"meta": map[string]any{
					"pagination": map[string]any{"page": 1, "next_page": next},
				},
			})
		case "2":
			json.NewEncoder(w).Encode(map[string]any{
				"locations": []map[string]any{
					{"name": "nbg1", "description": "Nuremberg", "city": "Nuremberg"},
				},
				"meta": map[string]any{
					"pagination": map[string]any{"page": 2, "next_page": nil},
				},
			})
		default:
			t.Errorf("unexpected page=%q", page)
			http.Error(w, "unexpected page", http.StatusBadRequest)
		}
	}))
	defer srv.Close()
	h := &Hetzner{token: "t", client: rewriteClient(srv.URL, hetznerAPI)}
	regions, err := h.ListRegions(context.Background())
	if err != nil {
		t.Fatalf("ListRegions: %v", err)
	}
	if len(regions) != 2 {
		t.Fatalf("expected 2 regions across 2 pages, got %d", len(regions))
	}
	if regions[0].ID != "fsn1" || regions[1].ID != "nbg1" {
		t.Errorf("regions = %+v", regions)
	}
}

func TestDO_ListRegions_Pagination(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		page := r.URL.Query().Get("page")
		switch page {
		case "1":
			// Return next link pointing at /v2/regions?page=2
			json.NewEncoder(w).Encode(map[string]any{
				"regions": []map[string]any{{"slug": "nyc1", "name": "NYC 1"}},
				"links": map[string]any{
					"pages": map[string]any{
						"next": "https://api.digitalocean.com/v2/regions?page=2&per_page=100",
					},
				},
			})
		case "2":
			json.NewEncoder(w).Encode(map[string]any{
				"regions": []map[string]any{{"slug": "sfo1", "name": "SFO 1"}},
				"links":   map[string]any{"pages": map[string]any{}},
			})
		default:
			t.Errorf("unexpected page=%q", page)
			http.Error(w, "unexpected page", http.StatusBadRequest)
		}
	}))
	defer srv.Close()
	d := &DigitalOcean{token: "t", client: rewriteClient(srv.URL, doAPI)}
	regions, err := d.ListRegions(context.Background())
	if err != nil {
		t.Fatalf("ListRegions: %v", err)
	}
	if len(regions) != 2 {
		t.Fatalf("expected 2 regions across 2 pages, got %d", len(regions))
	}
	if regions[0].ID != "nyc1" || regions[1].ID != "sfo1" {
		t.Errorf("regions = %+v", regions)
	}
}

func TestVultr_ListRegions_Pagination(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		cursor := r.URL.Query().Get("cursor")
		switch cursor {
		case "":
			json.NewEncoder(w).Encode(map[string]any{
				"regions": []map[string]any{{"id": "ewr", "city": "New Jersey"}},
				"meta":    map[string]any{"links": map[string]any{"next": "cursor-page-2"}},
			})
		case "cursor-page-2":
			json.NewEncoder(w).Encode(map[string]any{
				"regions": []map[string]any{{"id": "ord", "city": "Chicago"}},
				"meta":    map[string]any{"links": map[string]any{"next": ""}},
			})
		default:
			t.Errorf("unexpected cursor=%q", cursor)
			http.Error(w, "unexpected cursor", http.StatusBadRequest)
		}
	}))
	defer srv.Close()
	v := &Vultr{token: "t", client: rewriteClient(srv.URL, vultrAPI)}
	regions, err := v.ListRegions(context.Background())
	if err != nil {
		t.Fatalf("ListRegions: %v", err)
	}
	if len(regions) != 2 {
		t.Fatalf("expected 2 regions across 2 pages, got %d", len(regions))
	}
	if regions[0].ID != "ewr" || regions[1].ID != "ord" {
		t.Errorf("regions = %+v", regions)
	}
}

func TestLinode_ListRegions_Pagination(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		page := r.URL.Query().Get("page")
		switch page {
		case "1":
			json.NewEncoder(w).Encode(map[string]any{
				"data":  []map[string]any{{"id": "us-east", "label": "Newark"}},
				"page":  1,
				"pages": 2,
			})
		case "2":
			json.NewEncoder(w).Encode(map[string]any{
				"data":  []map[string]any{{"id": "us-west", "label": "Fremont"}},
				"page":  2,
				"pages": 2,
			})
		default:
			t.Errorf("unexpected page=%q", page)
			http.Error(w, "unexpected page", http.StatusBadRequest)
		}
	}))
	defer srv.Close()
	l := &Linode{token: "t", client: rewriteClient(srv.URL, linodeAPI)}
	regions, err := l.ListRegions(context.Background())
	if err != nil {
		t.Fatalf("ListRegions: %v", err)
	}
	if len(regions) != 2 {
		t.Fatalf("expected 2 regions across 2 pages, got %d", len(regions))
	}
	if regions[0].ID != "us-east" || regions[1].ID != "us-west" {
		t.Errorf("regions = %+v", regions)
	}
}

// =============================================================================
// doNextPath unit tests
// =============================================================================

func TestDONextPath(t *testing.T) {
	tests := []struct {
		in     string
		want   string
		wantOK bool
	}{
		{"", "", false},
		{"   ", "", false},
		{"https://api.digitalocean.com/v2/regions?page=2", "/regions?page=2", true},
		{"https://api.digitalocean.com/v2/sizes", "/sizes", true},
		{"::bad-url::", "", false},
	}
	for _, tt := range tests {
		got, ok := doNextPath(tt.in)
		if ok != tt.wantOK || got != tt.want {
			t.Errorf("doNextPath(%q) = (%q, %v), want (%q, %v)", tt.in, got, ok, tt.want, tt.wantOK)
		}
	}
}
