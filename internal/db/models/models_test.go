package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestAPIKey_StructDefaults(t *testing.T) {
	// Verify a zero-value APIKey has expected defaults
	var key APIKey
	if key.ID != "" {
		t.Errorf("ID should be empty, got %q", key.ID)
	}
	if key.UserID != "" {
		t.Errorf("UserID should be empty, got %q", key.UserID)
	}
	if key.TenantID != "" {
		t.Errorf("TenantID should be empty, got %q", key.TenantID)
	}
	if key.KeyHash != "" {
		t.Errorf("KeyHash should be empty, got %q", key.KeyHash)
	}
	if key.KeyPrefix != "" {
		t.Errorf("KeyPrefix should be empty, got %q", key.KeyPrefix)
	}
	if key.ScopesJSON != "" {
		t.Errorf("ScopesJSON should be empty, got %q", key.ScopesJSON)
	}
	if key.ExpiresAt != nil {
		t.Error("ExpiresAt should be nil")
	}
	if key.LastUsedAt != nil {
		t.Error("LastUsedAt should be nil")
	}
	if !key.CreatedAt.IsZero() {
		t.Error("CreatedAt should be zero")
	}
}

func TestAPIKey_FullConstruction(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	expiresAt := now.Add(24 * time.Hour)
	lastUsedAt := now.Add(-1 * time.Hour)

	key := APIKey{
		ID:         "ak_abc123",
		UserID:     "user_42",
		TenantID:   "tenant_7",
		Name:       "My API Key",
		KeyHash:    "$2a$10$hashedvalue",
		KeyPrefix:  "dmk_abc",
		ScopesJSON: `["read","write"]`,
		ExpiresAt:  &expiresAt,
		LastUsedAt: &lastUsedAt,
		CreatedAt:  now,
	}
	if key.ID != "ak_abc123" {
		t.Errorf("ID = %q", key.ID)
	}
	if key.UserID != "user_42" {
		t.Errorf("UserID = %q", key.UserID)
	}
	if key.TenantID != "tenant_7" {
		t.Errorf("TenantID = %q", key.TenantID)
	}
	if key.Name != "My API Key" {
		t.Errorf("Name = %q", key.Name)
	}
	if key.KeyHash != "$2a$10$hashedvalue" {
		t.Errorf("KeyHash = %q", key.KeyHash)
	}
	if key.KeyPrefix != "dmk_abc" {
		t.Errorf("KeyPrefix = %q", key.KeyPrefix)
	}
	if key.ScopesJSON != `["read","write"]` {
		t.Errorf("ScopesJSON = %q", key.ScopesJSON)
	}
	if key.ExpiresAt != &expiresAt {
		t.Errorf("ExpiresAt pointer mismatch")
	}
	if key.LastUsedAt != &lastUsedAt {
		t.Errorf("LastUsedAt pointer mismatch")
	}
	if !key.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt = %v, want %v", key.CreatedAt, now)
	}
}

func TestAPIKey_JSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	key := APIKey{
		ID:         "ak_json_1",
		UserID:     "user_99",
		TenantID:   "tenant_3",
		Name:       "JSON Key",
		KeyPrefix:  "dmk_xyz",
		ScopesJSON: `["admin"]`,
		CreatedAt:  now,
	}

	data, err := json.Marshal(key)
	if err != nil {
		t.Fatalf("Marshal APIKey: %v", err)
	}

	var decoded APIKey
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal APIKey: %v", err)
	}

	if decoded.ID != key.ID {
		t.Errorf("ID = %q, want %q", decoded.ID, key.ID)
	}
	if decoded.Name != key.Name {
		t.Errorf("Name = %q, want %q", decoded.Name, key.Name)
	}
	if decoded.KeyPrefix != key.KeyPrefix {
		t.Errorf("KeyPrefix = %q, want %q", decoded.KeyPrefix, key.KeyPrefix)
	}
}

func TestAPIKey_KeyHashNotExported(t *testing.T) {
	// KeyHash has json:"-" tag, so it should not appear in JSON output
	key := APIKey{
		ID:      "ak_secret",
		KeyHash: "super-secret-hash",
		Name:    "Secret Key",
	}
	data, err := json.Marshal(key)
	if err != nil {
		t.Fatalf("Marshal APIKey: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal raw: %v", err)
	}
	if _, ok := raw["key_hash"]; ok {
		t.Error("KeyHash should not be in JSON output (json:\"-\")")
	}
}

func TestAPIKey_ExpiresAt_Nil(t *testing.T) {
	key := APIKey{
		ID:    "ak_no_expiry",
		Name:  "No Expiry",
		CreatedAt: time.Now(),
	}
	if key.ExpiresAt != nil {
		t.Error("ExpiresAt should be nil for unset key")
	}
}

func TestWebhook_StructDefaults(t *testing.T) {
	var wh Webhook
	if wh.ID != "" {
		t.Errorf("ID should be empty, got %q", wh.ID)
	}
	if wh.AppID != "" {
		t.Errorf("AppID should be empty, got %q", wh.AppID)
	}
	if wh.SecretHash != "" {
		t.Errorf("SecretHash should be empty, got %q", wh.SecretHash)
	}
	if wh.BranchFilter != "" {
		t.Errorf("BranchFilter should be empty, got %q", wh.BranchFilter)
	}
	if wh.Status != "" {
		t.Errorf("Status should be empty, got %q", wh.Status)
	}
	if wh.AutoDeploy {
		t.Error("AutoDeploy should be false")
	}
	if wh.LastTriggeredAt != nil {
		t.Error("LastTriggeredAt should be nil")
	}
	if !wh.CreatedAt.IsZero() {
		t.Error("CreatedAt should be zero")
	}
}

func TestWebhook_FullConstruction(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	lastTriggered := now.Add(-30 * time.Minute)

	wh := Webhook{
		ID:              "wh_001",
		AppID:           "app_42",
		GitSourceID:     "git_7",
		SecretHash:      "sha256hash",
		EventsJSON:      `["push","pull_request"]`,
		BranchFilter:    "main",
		AutoDeploy:      true,
		Status:          "active",
		LastTriggeredAt: &lastTriggered,
		CreatedAt:       now,
	}
	if wh.ID != "wh_001" {
		t.Errorf("ID = %q", wh.ID)
	}
	if wh.AppID != "app_42" {
		t.Errorf("AppID = %q", wh.AppID)
	}
	if wh.GitSourceID != "git_7" {
		t.Errorf("GitSourceID = %q", wh.GitSourceID)
	}
	if wh.SecretHash != "sha256hash" {
		t.Errorf("SecretHash = %q", wh.SecretHash)
	}
	if wh.EventsJSON != `["push","pull_request"]` {
		t.Errorf("EventsJSON = %q", wh.EventsJSON)
	}
	if wh.BranchFilter != "main" {
		t.Errorf("BranchFilter = %q", wh.BranchFilter)
	}
	if !wh.AutoDeploy {
		t.Error("AutoDeploy should be true")
	}
	if wh.Status != "active" {
		t.Errorf("Status = %q", wh.Status)
	}
	if wh.LastTriggeredAt != &lastTriggered {
		t.Errorf("LastTriggeredAt pointer mismatch")
	}
	if !wh.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt = %v, want %v", wh.CreatedAt, now)
	}
}

func TestWebhook_JSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	wh := Webhook{
		ID:           "wh_json_1",
		AppID:        "app_7",
		EventsJSON:   `["push"]`,
		BranchFilter: "develop",
		AutoDeploy:   false,
		Status:       "inactive",
		CreatedAt:    now,
	}

	data, err := json.Marshal(wh)
	if err != nil {
		t.Fatalf("Marshal Webhook: %v", err)
	}

	var decoded Webhook
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal Webhook: %v", err)
	}

	if decoded.ID != wh.ID {
		t.Errorf("ID = %q, want %q", decoded.ID, wh.ID)
	}
	if decoded.AppID != wh.AppID {
		t.Errorf("AppID = %q, want %q", decoded.AppID, wh.AppID)
	}
}

func TestWebhook_SecretHashNotExported(t *testing.T) {
	wh := Webhook{
		ID:         "wh_secret",
		AppID:      "app_1",
		SecretHash: "my-secret-hash",
	}
	data, err := json.Marshal(wh)
	if err != nil {
		t.Fatalf("Marshal Webhook: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal raw: %v", err)
	}
	if _, ok := raw["secret_hash"]; ok {
		t.Error("SecretHash should not be in JSON output (json:\"-\")")
	}
}

func TestWebhook_OptionalFields(t *testing.T) {
	wh := Webhook{
		ID:     "wh_optional",
		AppID:  "app_0",
		Status: "active",
	}
	if wh.GitSourceID != "" {
		t.Errorf("GitSourceID should be empty, got %q", wh.GitSourceID)
	}
	if wh.LastTriggeredAt != nil {
		t.Error("LastTriggeredAt should be nil")
	}
}

func TestWebhook_LastTriggeredAt_RoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	wh := Webhook{
		ID:              "wh_triggered",
		AppID:           "app_5",
		Status:          "active",
		LastTriggeredAt: &now,
	}

	data, err := json.Marshal(wh)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded Webhook
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.LastTriggeredAt == nil {
		t.Fatal("LastTriggeredAt should not be nil")
	}
	if !decoded.LastTriggeredAt.Equal(now) {
		t.Errorf("LastTriggeredAt = %v, want %v", decoded.LastTriggeredAt, now)
	}
}

func TestAPIKey_NilTimeFields(t *testing.T) {
	key := APIKey{
		ID:         "ak_niltime",
		Name:       "Nil Time Fields",
		ExpiresAt:  nil,
		LastUsedAt: nil,
	}

	data, err := json.Marshal(key)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded APIKey
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.ExpiresAt != nil {
		t.Error("ExpiresAt should be nil")
	}
	if decoded.LastUsedAt != nil {
		t.Error("LastUsedAt should be nil")
	}
}

func TestWebhook_NilTriggeredAt(t *testing.T) {
	wh := Webhook{
		ID:              "wh_notriggered",
		AppID:           "app_x",
		Status:          "active",
		LastTriggeredAt: nil,
	}

	data, err := json.Marshal(wh)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded Webhook
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.LastTriggeredAt != nil {
		t.Error("LastTriggeredAt should be nil")
	}
}
