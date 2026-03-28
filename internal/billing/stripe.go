package billing

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const stripeAPI = "https://api.stripe.com/v1"

// StripeClient handles Stripe API operations.
// Uses raw HTTP to avoid the heavy Stripe SDK dependency.
type StripeClient struct {
	secretKey  string
	webhookKey string
	client     *http.Client
}

// NewStripeClient creates a Stripe API client.
func NewStripeClient(secretKey, webhookKey string) *StripeClient {
	return &StripeClient{
		secretKey:  secretKey,
		webhookKey: webhookKey,
		client:     &http.Client{Timeout: 30 * time.Second},
	}
}

// CreateCustomer creates a Stripe customer for a tenant.
func (s *StripeClient) CreateCustomer(ctx context.Context, email, name, tenantID string) (string, error) {
	params := url.Values{
		"email":               {email},
		"name":                {name},
		"metadata[tenant_id]": {tenantID},
	}

	var resp struct {
		ID string `json:"id"`
	}
	if err := s.post(ctx, "/customers", params, &resp); err != nil {
		return "", err
	}
	return resp.ID, nil
}

// CreateSubscription creates a subscription for a customer.
func (s *StripeClient) CreateSubscription(ctx context.Context, customerID, priceID string) (string, error) {
	params := url.Values{
		"customer":        {customerID},
		"items[0][price]": {priceID},
	}

	var resp struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := s.post(ctx, "/subscriptions", params, &resp); err != nil {
		return "", err
	}
	return resp.ID, nil
}

// CancelSubscription cancels a subscription at period end.
func (s *StripeClient) CancelSubscription(ctx context.Context, subscriptionID string) error {
	params := url.Values{
		"cancel_at_period_end": {"true"},
	}
	return s.post(ctx, "/subscriptions/"+subscriptionID, params, nil)
}

// CreatePortalSession creates a Stripe Customer Portal session.
func (s *StripeClient) CreatePortalSession(ctx context.Context, customerID, returnURL string) (string, error) {
	params := url.Values{
		"customer":   {customerID},
		"return_url": {returnURL},
	}

	var resp struct {
		URL string `json:"url"`
	}
	if err := s.post(ctx, "/billing_portal/sessions", params, &resp); err != nil {
		return "", err
	}
	return resp.URL, nil
}

// VerifyWebhookSignature validates a Stripe webhook signature.
func (s *StripeClient) VerifyWebhookSignature(payload []byte, sigHeader string) bool {
	if s.webhookKey == "" {
		return false
	}

	// Parse Stripe signature header: t=timestamp,v1=signature
	parts := strings.Split(sigHeader, ",")
	var timestamp, signature string
	for _, part := range parts {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			timestamp = kv[1]
		case "v1":
			signature = kv[1]
		}
	}

	if timestamp == "" || signature == "" {
		return false
	}

	// Compute expected signature
	signedPayload := timestamp + "." + string(payload)
	mac := hmac.New(sha256.New, []byte(s.webhookKey))
	mac.Write([]byte(signedPayload))
	expected := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(expected), []byte(signature))
}

func (s *StripeClient) post(ctx context.Context, path string, params url.Values, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		stripeAPI+path, bytes.NewReader([]byte(params.Encode())))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.secretKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("stripe API: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		var errResp struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(body, &errResp)
		return fmt.Errorf("stripe %s: %s", path, errResp.Error.Message)
	}

	if dest != nil {
		return json.Unmarshal(body, dest)
	}
	return nil
}
