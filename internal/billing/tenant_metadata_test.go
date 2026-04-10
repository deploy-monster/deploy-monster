package billing

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestStripeMetadata_IsZero(t *testing.T) {
	tests := []struct {
		name string
		md   StripeMetadata
		want bool
	}{
		{"empty", StripeMetadata{}, true},
		{"customer id set", StripeMetadata{CustomerID: "cus_1"}, false},
		{"subscription id set", StripeMetadata{SubscriptionID: "sub_1"}, false},
		{"subscription item id set", StripeMetadata{SubscriptionItemID: "si_1"}, false},
		{"price id set", StripeMetadata{PriceID: "price_1"}, false},
		{"status set", StripeMetadata{Status: "active"}, false},
		{"last succeeded set", StripeMetadata{PaymentLastSucceededAt: time.Now()}, false},
		{"last failed set", StripeMetadata{PaymentLastFailedAt: time.Now()}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.md.IsZero(); got != tt.want {
				t.Errorf("IsZero() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetStripeMetadata_EmptyTenant(t *testing.T) {
	md, err := GetStripeMetadata(nil)
	if err != nil {
		t.Fatalf("nil tenant returned error: %v", err)
	}
	if !md.IsZero() {
		t.Error("expected zero metadata for nil tenant")
	}

	tenant := &core.Tenant{}
	md, err = GetStripeMetadata(tenant)
	if err != nil {
		t.Fatalf("empty tenant returned error: %v", err)
	}
	if !md.IsZero() {
		t.Error("expected zero metadata for empty tenant")
	}
}

func TestGetStripeMetadata_NoStripeKey(t *testing.T) {
	tenant := &core.Tenant{MetadataJSON: `{"other":{"key":"value"}}`}
	md, err := GetStripeMetadata(tenant)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !md.IsZero() {
		t.Error("expected zero metadata when no stripe key present")
	}
}

func TestGetStripeMetadata_MalformedJSON(t *testing.T) {
	tenant := &core.Tenant{MetadataJSON: `{not valid json`}
	_, err := GetStripeMetadata(tenant)
	if err == nil {
		t.Fatal("expected error on malformed metadata json")
	}
	if !strings.Contains(err.Error(), "parse tenant metadata") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestGetStripeMetadata_MalformedStripeBlob(t *testing.T) {
	tenant := &core.Tenant{MetadataJSON: `{"stripe":"not an object"}`}
	_, err := GetStripeMetadata(tenant)
	if err == nil {
		t.Fatal("expected error on malformed stripe blob")
	}
	if !strings.Contains(err.Error(), "parse stripe metadata") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSetStripeMetadata_NilTenant(t *testing.T) {
	err := SetStripeMetadata(nil, StripeMetadata{CustomerID: "cus_1"})
	if err == nil {
		t.Fatal("expected error on nil tenant")
	}
}

func TestSetStripeMetadata_RoundTrip(t *testing.T) {
	tenant := &core.Tenant{}
	want := StripeMetadata{
		CustomerID:         "cus_abc",
		SubscriptionID:     "sub_abc",
		SubscriptionItemID: "si_abc",
		PriceID:            "price_abc",
		Status:             "active",
		UpdatedAt:          time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}

	if err := SetStripeMetadata(tenant, want); err != nil {
		t.Fatalf("SetStripeMetadata: %v", err)
	}

	got, err := GetStripeMetadata(tenant)
	if err != nil {
		t.Fatalf("GetStripeMetadata: %v", err)
	}
	if got.CustomerID != want.CustomerID ||
		got.SubscriptionID != want.SubscriptionID ||
		got.SubscriptionItemID != want.SubscriptionItemID ||
		got.PriceID != want.PriceID ||
		got.Status != want.Status ||
		!got.UpdatedAt.Equal(want.UpdatedAt) {
		t.Errorf("round trip mismatch:\n got = %+v\nwant = %+v", got, want)
	}
}

func TestSetStripeMetadata_PreservesOtherKeys(t *testing.T) {
	tenant := &core.Tenant{
		MetadataJSON: `{"branding":{"logo":"/x.png"},"quota":{"overflow":true}}`,
	}
	if err := SetStripeMetadata(tenant, StripeMetadata{CustomerID: "cus_1"}); err != nil {
		t.Fatalf("SetStripeMetadata: %v", err)
	}

	var blob map[string]json.RawMessage
	if err := json.Unmarshal([]byte(tenant.MetadataJSON), &blob); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if _, ok := blob["branding"]; !ok {
		t.Error("branding key was dropped")
	}
	if _, ok := blob["quota"]; !ok {
		t.Error("quota key was dropped")
	}
	if _, ok := blob["stripe"]; !ok {
		t.Error("stripe key was not written")
	}
}

func TestSetStripeMetadata_ZeroRemovesKey(t *testing.T) {
	tenant := &core.Tenant{
		MetadataJSON: `{"branding":{"logo":"/x.png"},"stripe":{"customer_id":"cus_1"}}`,
	}
	if err := SetStripeMetadata(tenant, StripeMetadata{}); err != nil {
		t.Fatalf("SetStripeMetadata: %v", err)
	}

	var blob map[string]json.RawMessage
	if err := json.Unmarshal([]byte(tenant.MetadataJSON), &blob); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if _, ok := blob["stripe"]; ok {
		t.Error("stripe key should have been removed")
	}
	if _, ok := blob["branding"]; !ok {
		t.Error("branding key should still be present")
	}
}

func TestSetStripeMetadata_ZeroEmptiesJSONWhenNoOtherKeys(t *testing.T) {
	tenant := &core.Tenant{MetadataJSON: `{"stripe":{"customer_id":"cus_1"}}`}
	if err := SetStripeMetadata(tenant, StripeMetadata{}); err != nil {
		t.Fatalf("SetStripeMetadata: %v", err)
	}
	if tenant.MetadataJSON != "" {
		t.Errorf("expected empty MetadataJSON, got %q", tenant.MetadataJSON)
	}
}

func TestSetStripeMetadata_MalformedExistingMetadata(t *testing.T) {
	tenant := &core.Tenant{MetadataJSON: `{not json`}
	err := SetStripeMetadata(tenant, StripeMetadata{CustomerID: "cus_1"})
	if err == nil {
		t.Fatal("expected error on malformed existing metadata")
	}
}

func TestPlanByStripePriceID(t *testing.T) {
	plans := []Plan{
		{ID: "free"},
		{ID: "pro", StripePriceID: "price_pro_monthly"},
		{ID: "biz", StripePriceID: "price_biz_monthly"},
	}

	tests := []struct {
		name    string
		priceID string
		wantID  string
		wantNil bool
	}{
		{"empty", "", "", true},
		{"unknown", "price_xyz", "", true},
		{"pro match", "price_pro_monthly", "pro", false},
		{"biz match", "price_biz_monthly", "biz", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PlanByStripePriceID(plans, tt.priceID)
			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil plan")
			}
			if got.ID != tt.wantID {
				t.Errorf("ID = %q, want %q", got.ID, tt.wantID)
			}
		})
	}
}
