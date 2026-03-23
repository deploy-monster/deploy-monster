package models

import "time"

// Subscription represents a billing subscription.
type Subscription struct {
	ID                    string     `json:"id"`
	TenantID              string     `json:"tenant_id"`
	PlanID                string     `json:"plan_id"`
	Status                string     `json:"status"`
	StripeSubscriptionID  string     `json:"stripe_subscription_id,omitempty"`
	CurrentPeriodStart    *time.Time `json:"current_period_start,omitempty"`
	CurrentPeriodEnd      *time.Time `json:"current_period_end,omitempty"`
	TrialEnd              *time.Time `json:"trial_end,omitempty"`
	CancelAt              *time.Time `json:"cancel_at,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
}

// UsageRecord tracks resource usage for billing.
type UsageRecord struct {
	ID         int64     `json:"id"`
	TenantID   string    `json:"tenant_id"`
	AppID      string    `json:"app_id,omitempty"`
	MetricType string    `json:"metric_type"`
	Value      float64   `json:"value"`
	HourBucket time.Time `json:"hour_bucket"`
	CreatedAt  time.Time `json:"created_at"`
}

// Invoice represents a billing invoice.
type Invoice struct {
	ID               string     `json:"id"`
	TenantID         string     `json:"tenant_id"`
	SubscriptionID   string     `json:"subscription_id,omitempty"`
	PeriodStart      time.Time  `json:"period_start"`
	PeriodEnd        time.Time  `json:"period_end"`
	SubtotalCents    int        `json:"subtotal_cents"`
	TaxCents         int        `json:"tax_cents"`
	TotalCents       int        `json:"total_cents"`
	Currency         string     `json:"currency"`
	Status           string     `json:"status"`
	StripeInvoiceID  string     `json:"stripe_invoice_id,omitempty"`
	PDFURL           string     `json:"pdf_url,omitempty"`
	PaidAt           *time.Time `json:"paid_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}
