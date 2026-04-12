package billing

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// stripeMetadataKey is the JSON object key under which Stripe-related state
// is stashed inside a tenant's free-form MetadataJSON column. Namespacing keeps
// the column usable for other integrations.
const stripeMetadataKey = "stripe"

// StripeMetadata captures the Stripe-specific state persisted per tenant so
// the billing pipeline can look up the customer, subscription, and current
// price without calling Stripe on every request.
type StripeMetadata struct {
	CustomerID             string    `json:"customer_id,omitempty"`
	SubscriptionID         string    `json:"subscription_id,omitempty"`
	SubscriptionItemID     string    `json:"subscription_item_id,omitempty"`
	PriceID                string    `json:"price_id,omitempty"`
	Status                 string    `json:"status,omitempty"`
	PaymentLastSucceededAt time.Time `json:"payment_last_succeeded_at,omitempty"`
	PaymentLastFailedAt    time.Time `json:"payment_last_failed_at,omitempty"`
	UpdatedAt              time.Time `json:"updated_at,omitempty"`
}

// IsZero reports whether the metadata carries no Stripe state at all.
func (m StripeMetadata) IsZero() bool {
	return m.CustomerID == "" &&
		m.SubscriptionID == "" &&
		m.SubscriptionItemID == "" &&
		m.PriceID == "" &&
		m.Status == "" &&
		m.PaymentLastSucceededAt.IsZero() &&
		m.PaymentLastFailedAt.IsZero()
}

// GetStripeMetadata extracts Stripe state from a tenant's MetadataJSON blob.
// Returns a zero value (not an error) when the tenant has never been linked to
// Stripe — callers treat absence and "free plan" identically.
func GetStripeMetadata(tenant *core.Tenant) (StripeMetadata, error) {
	var zero StripeMetadata
	if tenant == nil || tenant.MetadataJSON == "" {
		return zero, nil
	}
	var blob map[string]json.RawMessage
	if err := json.Unmarshal([]byte(tenant.MetadataJSON), &blob); err != nil {
		return zero, fmt.Errorf("parse tenant metadata: %w", err)
	}
	raw, ok := blob[stripeMetadataKey]
	if !ok || len(raw) == 0 {
		return zero, nil
	}
	var md StripeMetadata
	if err := json.Unmarshal(raw, &md); err != nil {
		return zero, fmt.Errorf("parse stripe metadata: %w", err)
	}
	return md, nil
}

// SetStripeMetadata writes Stripe state into a tenant's MetadataJSON blob,
// preserving any unrelated keys that are already present.
func SetStripeMetadata(tenant *core.Tenant, md StripeMetadata) error {
	if tenant == nil {
		return fmt.Errorf("nil tenant")
	}
	blob := map[string]json.RawMessage{}
	if tenant.MetadataJSON != "" {
		if err := json.Unmarshal([]byte(tenant.MetadataJSON), &blob); err != nil {
			return fmt.Errorf("parse tenant metadata: %w", err)
		}
	}
	if md.IsZero() {
		delete(blob, stripeMetadataKey)
	} else {
		encoded, err := json.Marshal(md)
		if err != nil {
			return fmt.Errorf("encode stripe metadata: %w", err)
		}
		blob[stripeMetadataKey] = encoded
	}
	if len(blob) == 0 {
		tenant.MetadataJSON = ""
		return nil
	}
	out, err := json.Marshal(blob)
	if err != nil {
		return fmt.Errorf("encode tenant metadata: %w", err)
	}
	tenant.MetadataJSON = string(out)
	return nil
}

// PlanByStripePriceID returns the first plan whose StripePriceID matches the
// given Stripe price id, or nil when no plan is wired to that price.
func PlanByStripePriceID(plans []Plan, priceID string) *Plan {
	if priceID == "" {
		return nil
	}
	for i := range plans {
		if plans[i].StripePriceID == priceID {
			return &plans[i]
		}
	}
	return nil
}
