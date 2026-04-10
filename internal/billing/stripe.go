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
	"strconv"
	"strings"
	"time"
)

const stripeAPI = "https://api.stripe.com/v1"

// stripeRequestMaxBody bounds how much of a Stripe error body we include in
// an error message to keep logs readable.
const stripeRequestMaxBody = 512

// StripeClient handles Stripe API operations.
// Uses raw HTTP to avoid the heavy Stripe SDK dependency.
type StripeClient struct {
	secretKey  string
	webhookKey string
	client     *http.Client
	// baseURL is the API base, overridable for tests. Empty means production.
	baseURL string
}

// NewStripeClient creates a Stripe API client.
func NewStripeClient(secretKey, webhookKey string) *StripeClient {
	return &StripeClient{
		secretKey:  secretKey,
		webhookKey: webhookKey,
		client:     &http.Client{Timeout: 30 * time.Second},
	}
}

// apiBase returns the effective API base URL.
func (s *StripeClient) apiBase() string {
	if s.baseURL != "" {
		return s.baseURL
	}
	return stripeAPI
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

// ReportUsage posts a metered usage record against a subscription item.
// `quantity` is the incremental value for this period when action=increment
// (the default Stripe behavior for metered billing).
func (s *StripeClient) ReportUsage(ctx context.Context, subscriptionItemID string, quantity int64, ts time.Time) error {
	if subscriptionItemID == "" {
		return fmt.Errorf("stripe: subscription_item_id is required")
	}
	if quantity < 0 {
		return fmt.Errorf("stripe: quantity must be >= 0")
	}
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	params := url.Values{
		"quantity":  {strconv.FormatInt(quantity, 10)},
		"timestamp": {strconv.FormatInt(ts.Unix(), 10)},
		"action":    {"increment"},
	}
	return s.post(ctx, "/subscription_items/"+subscriptionItemID+"/usage_records", params, nil)
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
		s.apiBase()+path, bytes.NewReader([]byte(params.Encode())))
	if err != nil {
		return fmt.Errorf("stripe %s: build request: %w", path, err)
	}
	req.Header.Set("Authorization", "Bearer "+s.secretKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("stripe API: %w", err)
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return fmt.Errorf("stripe %s: read body: %w", path, readErr)
	}

	if resp.StatusCode >= 400 {
		var errResp struct {
			Error struct {
				Message string `json:"message"`
				Type    string `json:"type"`
				Code    string `json:"code"`
			} `json:"error"`
		}
		_ = json.Unmarshal(body, &errResp) // best-effort: fall back to raw body
		msg := errResp.Error.Message
		if msg == "" {
			snippet := body
			if len(snippet) > stripeRequestMaxBody {
				snippet = snippet[:stripeRequestMaxBody]
			}
			msg = string(snippet)
		}
		return fmt.Errorf("stripe %s: HTTP %d: %s", path, resp.StatusCode, msg)
	}

	if dest != nil {
		if err := json.Unmarshal(body, dest); err != nil {
			return fmt.Errorf("stripe %s: decode response: %w", path, err)
		}
	}
	return nil
}
