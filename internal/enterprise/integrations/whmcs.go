package integrations

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// WHMCSBridge connects DeployMonster to WHMCS for automated provisioning.
// When WHMCS creates/suspends/terminates a hosting account, it calls
// DeployMonster's provisioning API, and this bridge handles the mapping.
type WHMCSBridge struct {
	apiURL    string
	apiID     string
	apiSecret string
	client    *http.Client
}

// NewWHMCSBridge creates a WHMCS API bridge.
func NewWHMCSBridge(apiURL, apiID, apiSecret string) *WHMCSBridge {
	return &WHMCSBridge{
		apiURL:    apiURL,
		apiID:     apiID,
		apiSecret: apiSecret,
		client:    &http.Client{Timeout: 30 * time.Second},
	}
}

// ProvisionRequest is what WHMCS sends when a new service is ordered.
type ProvisionRequest struct {
	ServiceID int    `json:"service_id"`
	ClientID  int    `json:"client_id"`
	Email     string `json:"email"`
	Package   string `json:"package"` // Maps to DeployMonster plan
	Domain    string `json:"domain"`
	Username  string `json:"username"`
}

// ProvisionResponse is sent back to WHMCS.
type ProvisionResponse struct {
	Success  bool   `json:"success"`
	TenantID string `json:"tenant_id"`
	LoginURL string `json:"login_url"`
	Message  string `json:"message,omitempty"`
}

// SyncModuleCommand calls a WHMCS module command.
func (w *WHMCSBridge) SyncModuleCommand(ctx context.Context, action string, serviceID int) error {
	params := url.Values{
		"action":     {"ModuleCustom"},
		"id":         {fmt.Sprintf("%d", serviceID)},
		"custom":     {action},
		"identifier": {w.apiID},
		"secret":     {w.apiSecret},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.apiURL+"/includes/api.php",
		bytes.NewReader([]byte(params.Encode())))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("WHMCS API: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("WHMCS error: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Result  string `json:"result"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &result)

	if result.Result != "success" {
		return fmt.Errorf("WHMCS: %s", result.Message)
	}

	return nil
}

// GetClientDetails fetches client info from WHMCS.
func (w *WHMCSBridge) GetClientDetails(ctx context.Context, clientID int) (map[string]any, error) {
	params := url.Values{
		"action":       {"GetClientsDetails"},
		"clientid":     {fmt.Sprintf("%d", clientID)},
		"identifier":   {w.apiID},
		"secret":       {w.apiSecret},
		"responsetype": {"json"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.apiURL+"/includes/api.php",
		bytes.NewReader([]byte(params.Encode())))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := w.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	_ = json.Unmarshal(body, &result)
	return result, nil
}
