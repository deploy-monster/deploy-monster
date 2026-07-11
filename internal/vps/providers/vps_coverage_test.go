package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// =============================================================================
// vpsDoRequest: Retry-After > vpsMaxRetryAfter gets clamped (vps_http.go:92-94)
// =============================================================================

func TestVPSDoRequest_RetryAfterClamped(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls == 0 {
			w.Header().Set("Retry-After", "120") // exceeds vpsMaxRetryAfter (30s)
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"error":"rate limited"}`)
			calls++
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
}

// =============================================================================
// vpsDoRequest: ctx.Done() during 429 Retry-After wait (vps_http.go:97-98)
// =============================================================================

func TestVPSDoRequest_429ContextCanceledDuringRetryWait(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "5")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":"rate limited"}`)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()

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
		t.Fatal("expected error for canceled context during 429 wait")
	}
}

// =============================================================================
// Pagination: max pages exceeded for ALL providers' ListRegions
// =============================================================================

// TestDO_ListRegions_MaxPages covers digitalocean.go:92 (return all, nil).
func TestDO_Cov_ListRegions_MaxPages(t *testing.T) {
	pageCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		pageCount++
		nextURL := fmt.Sprintf("https://api.digitalocean.com/v2/regions?page=%d&per_page=100", pageCount+1)
		json.NewEncoder(w).Encode(map[string]any{
			"regions": []map[string]any{{"slug": fmt.Sprintf("reg-%d", pageCount), "name": fmt.Sprintf("Region %d", pageCount)}},
			"links":   map[string]any{"pages": map[string]any{"next": nextURL}},
		})
	}))
	defer srv.Close()

	d := &DigitalOcean{token: "t", client: rewriteClient(srv.URL, doAPI)}
	regions, err := d.ListRegions(context.Background())
	if err != nil {
		t.Fatalf("ListRegions: %v", err)
	}
	if len(regions) < vpsMaxPages {
		t.Errorf("expected at least %d regions (max pages), got %d", vpsMaxPages, len(regions))
	}
}

// TestHetzner_ListRegions_MaxPages covers hetzner.go:96 (return all, nil).
func TestHetzner_ListRegions_MaxPages(t *testing.T) {
	pageCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		pageCount++
		json.NewEncoder(w).Encode(map[string]any{
			"locations": []map[string]any{{"name": fmt.Sprintf("loc-%d", pageCount), "description": fmt.Sprintf("Loc %d", pageCount), "city": "City"}},
			"meta":      map[string]any{"pagination": map[string]any{"page": pageCount, "next_page": pageCount + 1}},
		})
	}))
	defer srv.Close()

	h := &Hetzner{token: "t", client: rewriteClient(srv.URL, hetznerAPI)}
	regions, err := h.ListRegions(context.Background())
	if err != nil {
		t.Fatalf("ListRegions: %v", err)
	}
	if len(regions) < vpsMaxPages {
		t.Errorf("expected at least %d regions, got %d", vpsMaxPages, len(regions))
	}
}

// TestLinode_ListRegions_MaxPages covers linode.go:89 (return all, nil).
func TestLinode_ListRegions_MaxPages(t *testing.T) {
	pageCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		pageCount++
		totalPages := vpsMaxPages + 5
		json.NewEncoder(w).Encode(map[string]any{
			"data":  []map[string]any{{"id": fmt.Sprintf("reg-%d", pageCount), "label": fmt.Sprintf("Reg %d", pageCount)}},
			"page":  pageCount,
			"pages": totalPages,
		})
	}))
	defer srv.Close()

	l := &Linode{token: "t", client: rewriteClient(srv.URL, linodeAPI)}
	regions, err := l.ListRegions(context.Background())
	if err != nil {
		t.Fatalf("ListRegions: %v", err)
	}
	if len(regions) < vpsMaxPages {
		t.Errorf("expected at least %d regions, got %d", vpsMaxPages, len(regions))
	}
}

// TestVultr_ListRegions_MaxPages covers vultr.go:89 (return all, nil).
func TestVultr_ListRegions_MaxPages(t *testing.T) {
	pageCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		pageCount++
		json.NewEncoder(w).Encode(map[string]any{
			"regions": []map[string]any{{"id": fmt.Sprintf("reg-%d", pageCount), "city": fmt.Sprintf("City %d", pageCount)}},
			"meta":    map[string]any{"links": map[string]any{"next": fmt.Sprintf("cursor-%d", pageCount+1)}},
		})
	}))
	defer srv.Close()

	v := &Vultr{token: "t", client: rewriteClient(srv.URL, vultrAPI)}
	regions, err := v.ListRegions(context.Background())
	if err != nil {
		t.Fatalf("ListRegions: %v", err)
	}
	if len(regions) < vpsMaxPages {
		t.Errorf("expected at least %d regions, got %d", vpsMaxPages, len(regions))
	}
}

// =============================================================================
// Pagination: max pages exceeded for ListSizes
// =============================================================================

// TestDO_ListSizes_MaxPages covers digitalocean.go:118,120.
func TestDO_Cov_ListSizes_MaxPages(t *testing.T) {
	pageCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		pageCount++
		nextURL := fmt.Sprintf("https://api.digitalocean.com/v2/sizes?page=%d&per_page=100", pageCount+1)
		json.NewEncoder(w).Encode(map[string]any{
			"sizes": []map[string]any{{"slug": fmt.Sprintf("s-%d", pageCount), "vcpus": 1, "memory": 1024, "disk": 25, "price_hourly": 0.007}},
			"links": map[string]any{"pages": map[string]any{"next": nextURL}},
		})
	}))
	defer srv.Close()

	d := &DigitalOcean{token: "t", client: rewriteClient(srv.URL, doAPI)}
	sizes, err := d.ListSizes(context.Background(), "")
	if err != nil {
		t.Fatalf("ListSizes: %v", err)
	}
	if len(sizes) < vpsMaxPages {
		t.Errorf("expected at least %d sizes, got %d", vpsMaxPages, len(sizes))
	}
}

// TestHetzner_ListSizes_MaxPages covers hetzner.go:123,125.
func TestHetzner_ListSizes_MaxPages(t *testing.T) {
	pageCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		pageCount++
		json.NewEncoder(w).Encode(map[string]any{
			"server_types": []map[string]any{{"name": fmt.Sprintf("cx%d", pageCount), "description": fmt.Sprintf("CX%d", pageCount), "cores": 1, "memory": 2.0, "disk": 20}},
			"meta":         map[string]any{"pagination": map[string]any{"page": pageCount, "next_page": pageCount + 1}},
		})
	}))
	defer srv.Close()

	h := &Hetzner{token: "t", client: rewriteClient(srv.URL, hetznerAPI)}
	sizes, err := h.ListSizes(context.Background(), "")
	if err != nil {
		t.Fatalf("ListSizes: %v", err)
	}
	if len(sizes) < vpsMaxPages {
		t.Errorf("expected at least %d sizes, got %d", vpsMaxPages, len(sizes))
	}
}

// TestLinode_ListSizes_MaxPages covers linode.go:114,116.
func TestLinode_ListSizes_MaxPages(t *testing.T) {
	pageCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		pageCount++
		totalPages := vpsMaxPages + 5
		json.NewEncoder(w).Encode(map[string]any{
			"data":  []map[string]any{{"id": fmt.Sprintf("type-%d", pageCount), "label": fmt.Sprintf("Type %d", pageCount), "vcpus": 1, "memory": 1024, "disk": 25600, "price": map[string]any{"hourly": 0.005}}},
			"page":  pageCount,
			"pages": totalPages,
		})
	}))
	defer srv.Close()

	l := &Linode{token: "t", client: rewriteClient(srv.URL, linodeAPI)}
	sizes, err := l.ListSizes(context.Background(), "")
	if err != nil {
		t.Fatalf("ListSizes: %v", err)
	}
	if len(sizes) < vpsMaxPages {
		t.Errorf("expected at least %d sizes, got %d", vpsMaxPages, len(sizes))
	}
}

// TestVultr_ListSizes_MaxPages covers vultr.go:116.
func TestVultr_ListSizes_MaxPages(t *testing.T) {
	pageCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		pageCount++
		json.NewEncoder(w).Encode(map[string]any{
			"plans": []map[string]any{{"id": fmt.Sprintf("plan-%d", pageCount), "vcpu_count": 1, "ram": 1024, "disk": 25, "monthly_cost": 5.0}},
			"meta":  map[string]any{"links": map[string]any{"next": fmt.Sprintf("cursor-%d", pageCount+1)}},
		})
	}))
	defer srv.Close()

	v := &Vultr{token: "t", client: rewriteClient(srv.URL, vultrAPI)}
	sizes, err := v.ListSizes(context.Background(), "")
	if err != nil {
		t.Fatalf("ListSizes: %v", err)
	}
	if len(sizes) < vpsMaxPages {
		t.Errorf("expected at least %d sizes, got %d", vpsMaxPages, len(sizes))
	}
}
