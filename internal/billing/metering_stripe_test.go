package billing

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// meterStore is an in-memory store with the handful of tenant/usage methods
// the meter touches. It composes mockStore so the existing metering tests
// keep working with their slimmer fixture.
type meterStore struct {
	mockStore
	mu      sync.Mutex
	tenants map[string]*core.Tenant
}

func newMeterStore(tenants ...*core.Tenant) *meterStore {
	s := &meterStore{tenants: map[string]*core.Tenant{}}
	for _, t := range tenants {
		s.tenants[t.ID] = t
	}
	return s
}

func (s *meterStore) GetTenant(_ context.Context, id string) (*core.Tenant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tenants[id]
	if !ok {
		return nil, io.EOF // an arbitrary sentinel; reportUsageToStripe only logs
	}
	cp := *t
	return &cp, nil
}

func TestMeter_ReportUsageToStripe_Reports(t *testing.T) {
	// Seed one tenant with a subscription item id, one without.
	metered := &core.Tenant{ID: "tenant-metered"}
	_ = SetStripeMetadata(metered, StripeMetadata{
		SubscriptionItemID: "si_metered_1",
	})
	bare := &core.Tenant{ID: "tenant-bare"}

	store := newMeterStore(metered, bare)

	var calls int32
	var gotPath string
	var gotQty string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		vals, _ := url.ParseQuery(string(body))
		gotQty = vals.Get("quantity")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewStripeClient("sk_test", "whsec_test")
	client.baseURL = srv.URL

	meter := NewMeter(store, nil, slog.Default())
	meter.SetStripe(client, core.NewEventBus(slog.Default()))

	usage := map[string]*TenantUsage{
		"tenant-metered": {Containers: 7},
		"tenant-bare":    {Containers: 99}, // skipped — no subscription item
	}
	meter.reportUsageToStripe(context.Background(), usage, time.Now().UTC())

	// Only the metered tenant should have triggered a Stripe call.
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("stripe calls = %d, want 1", got)
	}
	if gotPath != "/subscription_items/si_metered_1/usage_records" {
		t.Errorf("path = %q", gotPath)
	}
	if gotQty != "7" {
		t.Errorf("quantity = %q, want 7", gotQty)
	}
}

func TestMeter_ReportUsageToStripe_NoStripeClient(t *testing.T) {
	// When Stripe is not configured the meter must still complete collect()
	// without panicking.
	store := newMeterStore()
	meter := NewMeter(store, nil, slog.Default())
	// Explicitly leave stripe nil.
	meter.collect()
}

func TestMeter_SetStripe(t *testing.T) {
	meter := NewMeter(newMeterStore(), nil, slog.Default())
	client := NewStripeClient("sk_test", "whsec_test")
	events := core.NewEventBus(slog.Default())
	meter.SetStripe(client, events)
	if meter.stripe != client {
		t.Error("stripe client not set")
	}
	if meter.events != events {
		t.Error("events not set")
	}
}

func TestMeter_ReportUsageToStripe_StripeAPIError(t *testing.T) {
	tenant := &core.Tenant{ID: "tenant-1"}
	_ = SetStripeMetadata(tenant, StripeMetadata{SubscriptionItemID: "si_1"})
	store := newMeterStore(tenant)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"boom"}}`))
	}))
	defer srv.Close()

	client := NewStripeClient("sk_test", "whsec_test")
	client.baseURL = srv.URL

	meter := NewMeter(store, nil, slog.Default())
	meter.SetStripe(client, nil)

	// Should log and continue, not panic.
	meter.reportUsageToStripe(context.Background(),
		map[string]*TenantUsage{"tenant-1": {Containers: 1}}, time.Now())
}
