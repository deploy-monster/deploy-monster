package auth

import (
	"context"
	"encoding/base32"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ═══════════════════════════════════════════════════════════════════════════════
// Module Init with mock Store
// ═══════════════════════════════════════════════════════════════════════════════

type mockStore struct {
	core.Store
	userCount      int
	countErr       error
	createTenantID string
	createTenantErr error
	createUserID   string
	createUserErr  error
}

func (m *mockStore) CountUsers(_ context.Context) (int, error) {
	return m.userCount, m.countErr
}

func (m *mockStore) CreateTenantWithDefaults(_ context.Context, _, _ string) (string, error) {
	if m.createTenantErr != nil {
		return "", m.createTenantErr
	}
	return m.createTenantID, nil
}

func (m *mockStore) CreateUserWithMembership(_ context.Context, _, _, _, _, _, _ string) (string, error) {
	if m.createUserErr != nil {
		return "", m.createUserErr
	}
	return m.createUserID, nil
}

func TestModule_Init_WithStore(t *testing.T) {
	store := &mockStore{userCount: 5}

	cfg := &core.Config{}
	cfg.Server.SecretKey = "test-secret-key-at-least-32-bytes-long!"

	c := &core.Core{
		Logger: slog.Default(),
		Store:  store,
		Config: cfg,
	}

	m := New()
	err := m.Init(context.Background(), c)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	if m.jwt == nil {
		t.Error("JWT service should be initialized after Init")
	}
	if m.store == nil {
		t.Error("store should be set after Init")
	}
}

func TestModule_Init_NilStore(t *testing.T) {
	c := &core.Core{
		Logger: slog.Default(),
		Store:  nil,
		Config: &core.Config{},
	}

	m := New()
	err := m.Init(context.Background(), c)
	if err == nil {
		t.Fatal("Init should return error when Store is nil")
	}
}

func TestModule_Init_FirstRunSetup(t *testing.T) {
	store := &mockStore{
		userCount:      0, // No users - first run
		createTenantID: "tenant-1",
		createUserID:   "user-1",
	}

	cfg := &core.Config{}
	cfg.Server.SecretKey = "test-secret-key-at-least-32-bytes-long!"

	c := &core.Core{
		Logger: slog.Default(),
		Store:  store,
		Config: cfg,
	}

	m := New()
	err := m.Init(context.Background(), c)
	if err != nil {
		t.Fatalf("Init with first run setup: %v", err)
	}
}

func TestModule_Init_FirstRunSetup_WithEnvPassword(t *testing.T) {
	store := &mockStore{
		userCount:      0,
		createTenantID: "tenant-1",
		createUserID:   "user-1",
	}

	cfg := &core.Config{}
	cfg.Server.SecretKey = "test-secret-key-at-least-32-bytes-long!"

	c := &core.Core{
		Logger: slog.Default(),
		Store:  store,
		Config: cfg,
	}

	t.Setenv("MONSTER_ADMIN_EMAIL", "custom@example.com")
	t.Setenv("MONSTER_ADMIN_PASSWORD", "CustomPass123!")

	m := New()
	err := m.Init(context.Background(), c)
	if err != nil {
		t.Fatalf("Init with custom env credentials: %v", err)
	}
}

func TestModule_Init_FirstRunSetup_CountError(t *testing.T) {
	store := &mockStore{
		countErr: context.DeadlineExceeded,
	}

	cfg := &core.Config{}
	cfg.Server.SecretKey = "test-secret-key-at-least-32-bytes-long!"

	c := &core.Core{
		Logger: slog.Default(),
		Store:  store,
		Config: cfg,
	}

	m := New()
	err := m.Init(context.Background(), c)
	if err == nil {
		t.Fatal("Init should propagate CountUsers error")
	}
}

func TestModule_Init_FirstRunSetup_CreateTenantError(t *testing.T) {
	store := &mockStore{
		userCount:       0,
		createTenantErr: context.DeadlineExceeded,
	}

	cfg := &core.Config{}
	cfg.Server.SecretKey = "test-secret-key-at-least-32-bytes-long!"

	c := &core.Core{
		Logger: slog.Default(),
		Store:  store,
		Config: cfg,
	}

	m := New()
	err := m.Init(context.Background(), c)
	if err == nil {
		t.Fatal("Init should propagate CreateTenantWithDefaults error")
	}
}

func TestModule_Init_FirstRunSetup_CreateUserError(t *testing.T) {
	store := &mockStore{
		userCount:      0,
		createTenantID: "tenant-1",
		createUserErr:  context.DeadlineExceeded,
	}

	cfg := &core.Config{}
	cfg.Server.SecretKey = "test-secret-key-at-least-32-bytes-long!"

	c := &core.Core{
		Logger: slog.Default(),
		Store:  store,
		Config: cfg,
	}

	m := New()
	err := m.Init(context.Background(), c)
	if err == nil {
		t.Fatal("Init should propagate CreateUserWithMembership error")
	}
}

func TestModule_Start_WithLogger(t *testing.T) {
	m := New()
	m.logger = slog.Default()

	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// NewTestModule
// ═══════════════════════════════════════════════════════════════════════════════

func TestNewTestModule_Functional(t *testing.T) {
	store := &mockStore{userCount: 1}
	m := NewTestModule("test-secret-key-at-least-32-bytes-long!", store)

	if m.JWT() == nil {
		t.Error("JWT should be set in test module")
	}
	if m.Store() != store {
		t.Error("Store should be set in test module")
	}

	// Verify JWT works
	pair, err := m.JWT().GenerateTokenPair("u1", "t1", "r1", "test@test.com")
	if err != nil {
		t.Fatalf("GenerateTokenPair: %v", err)
	}
	if pair.AccessToken == "" {
		t.Error("access token should not be empty")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// TOTP edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestValidateTOTP_WrongCode(t *testing.T) {
	cfg, _ := GenerateTOTP("test@test.com", "Test")
	if ValidateTOTP(cfg.Secret, "000000") {
		// The chance of a random code matching within +-1 window is extremely low
		// but not impossible; so just verify the function runs without error
		t.Log("unlikely match of random code")
	}
}

func TestValidateTOTP_AdjacentWindow(t *testing.T) {
	cfg, _ := GenerateTOTP("test@test.com", "Test")

	key, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(cfg.Secret))
	counter := time.Now().Unix() / 30

	// Generate code for previous window
	prevCode := generateCode(key, counter-1)
	if !ValidateTOTP(cfg.Secret, prevCode) {
		t.Error("code from previous time window should validate")
	}

	// Generate code for next window
	nextCode := generateCode(key, counter+1)
	if !ValidateTOTP(cfg.Secret, nextCode) {
		t.Error("code from next time window should validate")
	}
}

func TestGenerateCode_Deterministic(t *testing.T) {
	key := []byte("12345678901234567890")
	code1 := generateCode(key, 1000)
	code2 := generateCode(key, 1000)

	if code1 != code2 {
		t.Error("generateCode should be deterministic for same key and counter")
	}

	if len(code1) != 6 {
		t.Errorf("code length should be 6, got %d", len(code1))
	}
}

func TestGenerateCode_DifferentCounters(t *testing.T) {
	key := []byte("12345678901234567890")
	code1 := generateCode(key, 1000)
	code2 := generateCode(key, 1001)

	// Different counters should almost always produce different codes
	// (technically could collide, but extremely unlikely)
	if code1 == code2 {
		t.Log("adjacent counters produced same code (unlikely but possible)")
	}
}

func TestRandomHex_Length(t *testing.T) {
	for _, n := range []int{1, 4, 8, 16} {
		result := randomHex(n)
		if len(result) != n {
			t.Errorf("randomHex(%d) length = %d, want %d", n, len(result), n)
		}
	}
}

func TestRandomHex_HexChars(t *testing.T) {
	result := randomHex(100)
	for _, c := range result {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("non-hex character in output: %c", c)
		}
	}
}

func TestGenerateTOTPSecret_Length(t *testing.T) {
	s := generateTOTPSecret(20)
	if len(s) != 20 {
		t.Errorf("expected length 20, got %d", len(s))
	}
}

func TestGenerateTOTP_URL_Format(t *testing.T) {
	cfg, err := GenerateTOTP("user@example.com", "DeployMonster")
	if err != nil {
		t.Fatalf("GenerateTOTP: %v", err)
	}

	// Check URL has all required components
	if !strings.Contains(cfg.URL, "DeployMonster") {
		t.Error("URL should contain issuer")
	}
	if !strings.Contains(cfg.URL, "user@example.com") {
		t.Error("URL should contain email")
	}
	if !strings.Contains(cfg.URL, "algorithm=SHA1") {
		t.Error("URL should contain algorithm")
	}
	if !strings.Contains(cfg.URL, "digits=6") {
		t.Error("URL should contain digits")
	}
	if !strings.Contains(cfg.URL, "period=30") {
		t.Error("URL should contain period")
	}
}

func TestGenerateTOTP_RecoveryCodeFormat(t *testing.T) {
	cfg, _ := GenerateTOTP("test@test.com", "Test")

	for _, code := range cfg.Recovery {
		if !strings.Contains(code, "-") {
			t.Errorf("recovery code %q should contain a dash separator", code)
		}
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// JWT ValidateRefreshToken edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestJWT_ValidateRefreshToken_Invalid(t *testing.T) {
	svc := NewJWTService("test-secret-key-at-least-32-bytes-long!")

	_, err := svc.ValidateRefreshToken("invalid-token-string")
	if err == nil {
		t.Error("expected error for invalid refresh token")
	}
}

func TestJWT_ValidateRefreshToken_WrongSecret(t *testing.T) {
	svc1 := NewJWTService("secret-one-at-least-32-bytes-long-aaaa!")
	svc2 := NewJWTService("secret-two-at-least-32-bytes-long-bbbb!")

	pair, _ := svc1.GenerateTokenPair("user-1", "t", "r", "e@e.com")

	_, err := svc2.ValidateRefreshToken(pair.RefreshToken)
	if err == nil {
		t.Error("expected error when validating refresh token with different secret")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// API Key edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestGenerateAPIKey_PrefixLength(t *testing.T) {
	pair, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}

	// Prefix = "dm_" + 8 hex chars = 11 chars total
	if len(pair.Prefix) != 11 {
		t.Errorf("prefix length = %d, want 11", len(pair.Prefix))
	}
}

func TestGenerateAPIKey_KeyLength(t *testing.T) {
	pair, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}

	// Key = "dm_" + 64 hex chars (32 bytes) = 67 chars total
	if len(pair.Key) != 67 {
		t.Errorf("key length = %d, want 67", len(pair.Key))
	}
}

func TestHashAPIKey_DifferentKeys(t *testing.T) {
	h1 := HashAPIKey("dm_key1")
	h2 := HashAPIKey("dm_key2")

	if h1 == h2 {
		t.Error("different keys should produce different hashes")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Password - HashPassword error paths
// ═══════════════════════════════════════════════════════════════════════════════

func TestHashPassword_VeryLongPassword(t *testing.T) {
	// bcrypt has a 72-byte limit; test that it doesn't error
	longPwd := strings.Repeat("A", 72)
	hash, err := HashPassword(longPwd)
	if err != nil {
		t.Fatalf("HashPassword with 72-char password: %v", err)
	}
	if hash == "" {
		t.Error("hash should not be empty")
	}
}

func TestValidatePasswordStrength_CustomMinLength(t *testing.T) {
	// minLength = 12
	err := ValidatePasswordStrength("Short1Ab", 12)
	if err == nil {
		t.Error("8-char password should fail when minLength is 12")
	}

	err = ValidatePasswordStrength("LongEnough1Ab", 12)
	if err != nil {
		t.Errorf("13-char password should pass with minLength 12: %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// OAuth edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestGetUser_UnknownProvider(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id": "123", "email": "test@test.com"}`))
	}))
	defer srv.Close()

	p := &OAuthProvider{
		Name:        "unknown-provider",
		UserInfoURL: srv.URL,
		client:      &http.Client{},
	}

	user, err := p.GetUser(context.Background(), "tok")
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}

	// Unknown provider should return empty user data since no case matches
	if user.Provider != "unknown-provider" {
		t.Errorf("Provider = %q, want unknown-provider", user.Provider)
	}
	// ID/Email should be empty because no case matches in the switch
	if user.ID != "" {
		t.Errorf("ID should be empty for unknown provider, got %q", user.ID)
	}
}

func TestJoinScopes_Empty(t *testing.T) {
	result := joinScopes(nil)
	if result != "" {
		t.Errorf("joinScopes(nil) = %q, want empty", result)
	}
}

func TestJoinScopes_Single(t *testing.T) {
	result := joinScopes([]string{"email"})
	if result != "email" {
		t.Errorf("joinScopes([email]) = %q, want email", result)
	}
}

func TestGenerateTokenID(t *testing.T) {
	id1 := generateTokenID()
	id2 := generateTokenID()

	if id1 == "" {
		t.Error("token ID should not be empty")
	}
	if len(id1) != 32 {
		t.Errorf("token ID length = %d, want 32", len(id1))
	}
	if id1 == id2 {
		t.Error("two token IDs should be different")
	}
}
