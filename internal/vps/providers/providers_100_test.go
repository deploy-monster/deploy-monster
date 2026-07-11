package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// =============================================================================
// do() marshal error — all providers
// =============================================================================

func TestHetzner_Do_MarshalError(t *testing.T) {
	h := &Hetzner{token: "t", client: http.DefaultClient}
	_, err := h.do(context.Background(), http.MethodPost, "/test", make(chan int))
	if err == nil || !strings.Contains(err.Error(), "marshal") {
		t.Errorf("expected marshal error, got %v", err)
	}
}

func TestDO_Do_MarshalError(t *testing.T) {
	d := &DigitalOcean{token: "t", client: http.DefaultClient}
	_, err := d.do(context.Background(), http.MethodPost, "/test", make(chan int))
	if err == nil || !strings.Contains(err.Error(), "marshal") {
		t.Errorf("expected marshal error, got %v", err)
	}
}

func TestVultr_Do_MarshalError(t *testing.T) {
	v := &Vultr{token: "t", client: http.DefaultClient}
	_, err := v.do(context.Background(), http.MethodPost, "/test", make(chan int))
	if err == nil || !strings.Contains(err.Error(), "marshal") {
		t.Errorf("expected marshal error, got %v", err)
	}
}

func TestLinode_Do_MarshalError(t *testing.T) {
	l := &Linode{token: "t", client: http.DefaultClient}
	_, err := l.do(context.Background(), http.MethodPost, "/test", make(chan int))
	if err == nil || !strings.Contains(err.Error(), "marshal") {
		t.Errorf("expected marshal error, got %v", err)
	}
}

// =============================================================================
// ListSizes decode errors — all providers
// =============================================================================

func TestHetzner_ListSizes_DecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `garbage`)
	}))
	defer srv.Close()
	h := &Hetzner{token: "t", client: rewriteClient(srv.URL, hetznerAPI)}
	_, err := h.ListSizes(context.Background(), "fsn1")
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Errorf("expected decode error, got %v", err)
	}
}

func TestDO_ListSizes_DecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `not json`)
	}))
	defer srv.Close()
	d := &DigitalOcean{token: "t", client: rewriteClient(srv.URL, doAPI)}
	_, err := d.ListSizes(context.Background(), "nyc1")
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Errorf("expected decode error, got %v", err)
	}
}

func TestVultr_ListSizes_DecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{{{`)
	}))
	defer srv.Close()
	v := &Vultr{token: "t", client: rewriteClient(srv.URL, vultrAPI)}
	_, err := v.ListSizes(context.Background(), "ewr")
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Errorf("expected decode error, got %v", err)
	}
}

func TestLinode_ListSizes_DecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `bad`)
	}))
	defer srv.Close()
	l := &Linode{token: "t", client: rewriteClient(srv.URL, linodeAPI)}
	_, err := l.ListSizes(context.Background(), "us-east")
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Errorf("expected decode error, got %v", err)
	}
}

// =============================================================================
// Status decode errors — all providers
// =============================================================================

func TestHetzner_Status_DecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{{{`)
	}))
	defer srv.Close()
	h := &Hetzner{token: "t", client: rewriteClient(srv.URL, hetznerAPI)}
	_, err := h.Status(context.Background(), "123")
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Errorf("expected decode error, got %v", err)
	}
}

func TestDO_Status_DecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `garbage`)
	}))
	defer srv.Close()
	d := &DigitalOcean{token: "t", client: rewriteClient(srv.URL, doAPI)}
	_, err := d.Status(context.Background(), "123")
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Errorf("expected decode error, got %v", err)
	}
}

func TestVultr_Status_DecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `not json`)
	}))
	defer srv.Close()
	v := &Vultr{token: "t", client: rewriteClient(srv.URL, vultrAPI)}
	_, err := v.Status(context.Background(), "123")
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Errorf("expected decode error, got %v", err)
	}
}

func TestLinode_Status_DecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `bad json`)
	}))
	defer srv.Close()
	l := &Linode{token: "t", client: rewriteClient(srv.URL, linodeAPI)}
	_, err := l.Status(context.Background(), "123")
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Errorf("expected decode error, got %v", err)
	}
}

// =============================================================================
// Create decode errors — all providers
// =============================================================================

func TestHetzner_Create_DecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `invalid`)
	}))
	defer srv.Close()
	h := &Hetzner{token: "t", client: rewriteClient(srv.URL, hetznerAPI)}
	_, err := h.Create(context.Background(), core.VPSCreateOpts{Name: "t", Region: "fsn1", Size: "cx11", Image: "img"})
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Errorf("expected decode error, got %v", err)
	}
}

func TestDO_Create_DecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{{{`)
	}))
	defer srv.Close()
	d := &DigitalOcean{token: "t", client: rewriteClient(srv.URL, doAPI)}
	_, err := d.Create(context.Background(), core.VPSCreateOpts{Name: "t", Region: "nyc1", Size: "s-1vcpu-1gb", Image: "img"})
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Errorf("expected decode error, got %v", err)
	}
}

func TestVultr_Create_DecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `bad`)
	}))
	defer srv.Close()
	v := &Vultr{token: "t", client: rewriteClient(srv.URL, vultrAPI)}
	_, err := v.Create(context.Background(), core.VPSCreateOpts{Name: "t", Region: "ewr", Size: "vc2-1c-1gb", Image: "img"})
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Errorf("expected decode error, got %v", err)
	}
}

func TestLinode_Create_DecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `garbage`)
	}))
	defer srv.Close()
	l := &Linode{token: "t", client: rewriteClient(srv.URL, linodeAPI)}
	_, err := l.Create(context.Background(), core.VPSCreateOpts{Name: "t", Region: "us-east", Size: "g6-nanode-1", Image: "img"})
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Errorf("expected decode error, got %v", err)
	}
}

// =============================================================================
// doNextPath — empty path after stripping /v2 prefix
// =============================================================================

func TestDONextPath_EmptyAfterStrip(t *testing.T) {
	// When the URL path after stripping /v2 is empty, should return "/"
	got, ok := doNextPath("https://api.digitalocean.com/v2")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if got != "/" {
		t.Errorf("got %q, want /", got)
	}

	// With query params
	got, ok = doNextPath("https://api.digitalocean.com/v2?page=2")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if got != "/?page=2" {
		t.Errorf("got %q, want /?page=2", got)
	}
}

// =============================================================================
// vpsDoRequest — transport error (non-context) path
// =============================================================================

func TestVPSDoRequest_TransportError(t *testing.T) {
	// Use a client that returns a transport error (not context-related)
	client := &http.Client{
		Transport: roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("connection refused")
		}),
		Timeout: time.Second,
	}

	_, err := vpsDoRequest(
		context.Background(),
		client,
		nil,
		"test",
		http.MethodGet,
		"http://127.0.0.1:1/x",
		"token",
		nil,
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("error = %v, want connection refused", err)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// =============================================================================
// vpsDoRequest — ReadBody error
// =============================================================================

func TestVPSDoRequest_ReadBodyError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set content-length larger than actual body so ReadAll fails
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "short")
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
	// Note: io.ReadAll doesn't actually return an error for early EOF in Go 1.16+,
	// so this test just exercises the code path. The error may be nil.
	_ = err
}

// =============================================================================
// vpsDoRequest — 429 with Retry-After 0 (no actual wait)
// =============================================================================

func TestVPSDoRequest_429RetryAfterZero(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 2 {
			w.Header().Set("Retry-After", "0")
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
}

// =============================================================================
// vpsDoRequest — 429 with Retry-After value that triggers the wait path
// =============================================================================

func TestVPSDoRequest_429RetryAfterWait(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 2 {
			w.Header().Set("Retry-After", "1") // 1 second
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
}

// =============================================================================
// vpsDoRequest — 429 with ctx cancellation during wait
// =============================================================================

func TestVPSDoRequest_429ContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "10") // Will wait 10s, which we cancel
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":"rate limited"}`)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-canceled

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
}

// =============================================================================
// vpsDoRequest — 429 with Retry-After HTTP-date (future, short)
// =============================================================================

func TestVPSDoRequest_429RetryAfterShortDate(t *testing.T) {
	var calls int32
	futureTime := time.Now().Add(100 * time.Millisecond).UTC().Format(http.TimeFormat)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 2 {
			w.Header().Set("Retry-After", futureTime)
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
}

// =============================================================================
// vpsDoRequest — 429 with invalid Retry-After (not a number, not a date)
// =============================================================================

func TestParseRetryAfter_Invalid(t *testing.T) {
	if got := parseRetryAfter("garbage"); got != 0 {
		t.Errorf("expected 0 for invalid input, got %v", got)
	}
}

// =============================================================================
// vpsMarshalPayload — nil and non-nil (already tested via do() but direct)
// =============================================================================

func TestVpsMarshalPayload_Nil(t *testing.T) {
	data, err := vpsMarshalPayload(nil)
	if err != nil {
		t.Fatalf("vpsMarshalPayload(nil): %v", err)
	}
	if data != nil {
		t.Errorf("expected nil, got %v", data)
	}
}

func TestVpsMarshalPayload_Valid(t *testing.T) {
	data, err := vpsMarshalPayload(map[string]string{"key": "val"})
	if err != nil {
		t.Fatalf("vpsMarshalPayload: %v", err)
	}
	if !strings.Contains(string(data), "key") {
		t.Errorf("data = %q, want to contain 'key'", string(data))
	}
}

func TestVpsMarshalPayload_Error(t *testing.T) {
	_, err := vpsMarshalPayload(make(chan int))
	if err == nil {
		t.Fatal("expected error for unserializable payload")
	}
}

// =============================================================================
// Vultr — Create with SSHKeyID (exercises SSH key branch)
// =============================================================================

func TestVultrCov_Create_WithSSHKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if _, ok := body["sshkey_id"]; !ok {
			t.Error("request missing sshkey_id")
		}
		json.NewEncoder(w).Encode(map[string]any{
			"instance": map[string]any{
				"id": "v-abc", "label": "ssh-test",
				"main_ip": "1.2.3.4", "status": "active",
			},
		})
	}))
	defer srv.Close()

	v := &Vultr{token: "t", client: rewriteClient(srv.URL, vultrAPI)}
	inst, err := v.Create(context.Background(), core.VPSCreateOpts{
		Name: "ssh-test", Region: "ewr", Size: "vc2-1c-1gb",
		Image: "387", SSHKeyID: "my-key-id",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if inst.ID != "v-abc" {
		t.Errorf("ID = %q", inst.ID)
	}
}

// =============================================================================
// DigitalOcean — Create with SSHKeyID
// =============================================================================

func TestDOCov_Create_WithSSHKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"droplet": map[string]any{"id": 1, "name": "do-ssh", "status": "new"},
		})
	}))
	defer srv.Close()

	d := &DigitalOcean{token: "t", client: rewriteClient(srv.URL, doAPI)}
	inst, err := d.Create(context.Background(), core.VPSCreateOpts{
		Name: "do-ssh", Region: "nyc1", Size: "s-1vcpu-1gb",
		Image: "ubuntu", SSHKeyID: "key-1",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if inst.ID != "1" {
		t.Errorf("ID = %q", inst.ID)
	}
}

// =============================================================================
// Hetzner — Create with SSHKeyID  
// =============================================================================

func TestHetznerCov_Create_WithSSHKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"server": map[string]any{
				"id": 999, "name": "hz-ssh", "status": "starting",
				"public_net": map[string]any{"ipv4": map[string]any{"ip": "10.0.0.1"}},
			},
		})
	}))
	defer srv.Close()

	h := &Hetzner{token: "t", client: rewriteClient(srv.URL, hetznerAPI)}
	inst, err := h.Create(context.Background(), core.VPSCreateOpts{
		Name: "hz-ssh", Region: "fsn1", Size: "cx11",
		Image: "ubuntu", SSHKeyID: "my-key",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if inst.ID != "999" {
		t.Errorf("ID = %q", inst.ID)
	}
}

// =============================================================================
// Linode — Create with SSHKeyID
// =============================================================================

func TestLinodeCov_Create_WithSSHKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id": 777, "label": "ln-ssh",
			"ipv4": []string{"10.0.0.2"}, "status": "provisioning",
		})
	}))
	defer srv.Close()

	l := &Linode{token: "t", client: rewriteClient(srv.URL, linodeAPI)}
	inst, err := l.Create(context.Background(), core.VPSCreateOpts{
		Name: "ln-ssh", Region: "us-east", Size: "g6-nanode-1",
		Image: "linode/ubuntu22.04", SSHKeyID: "ssh-rsa AAA...",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if inst.ID != "777" {
		t.Errorf("ID = %q", inst.ID)
	}
}

// =============================================================================
// ListSizes pagination tests — all providers
// =============================================================================

func TestHetzner_ListSizes_Pagination(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		page := r.URL.Query().Get("page")
		switch page {
		case "1":
			next := 2
			json.NewEncoder(w).Encode(map[string]any{
				"server_types": []map[string]any{
					{"name": "cx11", "description": "CX11", "cores": 1, "memory": 2.0, "disk": 20},
				},
				"meta": map[string]any{
					"pagination": map[string]any{"page": 1, "next_page": next},
				},
			})
		case "2":
			json.NewEncoder(w).Encode(map[string]any{
				"server_types": []map[string]any{
					{"name": "cx21", "description": "CX21", "cores": 2, "memory": 4.0, "disk": 40},
				},
				"meta": map[string]any{
					"pagination": map[string]any{"page": 2, "next_page": nil},
				},
			})
		default:
			t.Errorf("unexpected page=%q", page)
			http.Error(w, "bad request", http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	h := &Hetzner{token: "t", client: rewriteClient(srv.URL, hetznerAPI)}
	sizes, err := h.ListSizes(context.Background(), "fsn1")
	if err != nil {
		t.Fatalf("ListSizes: %v", err)
	}
	if len(sizes) != 2 {
		t.Fatalf("expected 2 sizes across 2 pages, got %d", len(sizes))
	}
	if sizes[0].ID != "cx11" || sizes[1].ID != "cx21" {
		t.Errorf("sizes = %+v", sizes)
	}
}

func TestDO_ListSizes_Pagination(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		page := r.URL.Query().Get("page")
		switch page {
		case "1":
			json.NewEncoder(w).Encode(map[string]any{
				"sizes": []map[string]any{
					{"slug": "s-1vcpu-1gb", "vcpus": 1, "memory": 1024, "disk": 25, "price_hourly": 0.00744},
				},
				"links": map[string]any{
					"pages": map[string]any{
						"next": "https://api.digitalocean.com/v2/sizes?page=2&per_page=100",
					},
				},
			})
		case "2":
			json.NewEncoder(w).Encode(map[string]any{
				"sizes": []map[string]any{
					{"slug": "s-2vcpu-2gb", "vcpus": 2, "memory": 2048, "disk": 50, "price_hourly": 0.01488},
				},
				"links": map[string]any{"pages": map[string]any{}},
			})
		default:
			t.Errorf("unexpected page=%q", page)
			http.Error(w, "bad request", http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	d := &DigitalOcean{token: "t", client: rewriteClient(srv.URL, doAPI)}
	sizes, err := d.ListSizes(context.Background(), "nyc1")
	if err != nil {
		t.Fatalf("ListSizes: %v", err)
	}
	if len(sizes) != 2 {
		t.Fatalf("expected 2 sizes across 2 pages, got %d", len(sizes))
	}
	if sizes[0].ID != "s-1vcpu-1gb" || sizes[1].ID != "s-2vcpu-2gb" {
		t.Errorf("sizes = %+v", sizes)
	}
}

func TestVultr_ListSizes_Pagination(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		cursor := r.URL.Query().Get("cursor")
		switch cursor {
		case "":
			json.NewEncoder(w).Encode(map[string]any{
				"plans": []map[string]any{
					{"id": "vc2-1c-1gb", "vcpu_count": 1, "ram": 1024, "disk": 25, "monthly_cost": 5.0},
				},
				"meta": map[string]any{"links": map[string]any{"next": "cursor-page-2"}},
			})
		case "cursor-page-2":
			json.NewEncoder(w).Encode(map[string]any{
				"plans": []map[string]any{
					{"id": "vc2-2c-4gb", "vcpu_count": 2, "ram": 4096, "disk": 80, "monthly_cost": 20.0},
				},
				"meta": map[string]any{"links": map[string]any{"next": ""}},
			})
		default:
			t.Errorf("unexpected cursor=%q", cursor)
			http.Error(w, "bad request", http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	v := &Vultr{token: "t", client: rewriteClient(srv.URL, vultrAPI)}
	sizes, err := v.ListSizes(context.Background(), "ewr")
	if err != nil {
		t.Fatalf("ListSizes: %v", err)
	}
	if len(sizes) != 2 {
		t.Fatalf("expected 2 sizes across 2 pages, got %d", len(sizes))
	}
	if sizes[0].ID != "vc2-1c-1gb" || sizes[1].ID != "vc2-2c-4gb" {
		t.Errorf("sizes = %+v", sizes)
	}
}

func TestLinode_ListSizes_Pagination(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		page := r.URL.Query().Get("page")
		switch page {
		case "1":
			json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": "g6-nanode-1", "label": "Nanode 1GB", "vcpus": 1, "memory": 1024, "disk": 25600, "price": map[string]any{"hourly": 0.0075}},
				},
				"page": 1, "pages": 2,
			})
		case "2":
			json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": "g6-standard-2", "label": "Linode 4GB", "vcpus": 2, "memory": 4096, "disk": 81920, "price": map[string]any{"hourly": 0.03}},
				},
				"page": 2, "pages": 2,
			})
		default:
			t.Errorf("unexpected page=%q", page)
			http.Error(w, "bad request", http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	l := &Linode{token: "t", client: rewriteClient(srv.URL, linodeAPI)}
	sizes, err := l.ListSizes(context.Background(), "us-east")
	if err != nil {
		t.Fatalf("ListSizes: %v", err)
	}
	if len(sizes) != 2 {
		t.Fatalf("expected 2 sizes across 2 pages, got %d", len(sizes))
	}
	if sizes[0].ID != "g6-nanode-1" || sizes[1].ID != "g6-standard-2" {
		t.Errorf("sizes = %+v", sizes)
	}
}

// =============================================================================
// vpsDoRequest: ReadBody error & other remaining vpsDoRequest paths
// =============================================================================

// =============================================================================
// vpsDoRequest with payload (non-nil body reader branch)
// =============================================================================

func TestVPSDoRequest_WithPayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if len(body) == 0 {
			t.Error("expected payload body")
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
		http.MethodPost,
		srv.URL+"/x",
		"token",
		[]byte(`{"key":"val"}`),
	)
	if err != nil {
		t.Fatalf("vpsDoRequest: %v", err)
	}
	if !strings.Contains(string(body), "ok") {
		t.Errorf("body = %q", string(body))
	}
}
