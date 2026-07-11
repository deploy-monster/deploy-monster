package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/db/models"
	"github.com/golang-jwt/jwt/v5"

	"golang.org/x/crypto/bcrypt"
)

// =============================================================================
// Module.init — covers the init() registration path (module.go:15, 50.0%)
// =============================================================================

func TestModuleInit_Registered(t *testing.T) {
	// The init() function in module.go registers via core.RegisterModule.
	// This test verifies that the factory function works.
	m := New()
	if m.ID() != "core.auth" {
		t.Errorf("ID() = %q, want %q", m.ID(), "core.auth")
	}
	if m.Name() != "Authentication" {
		t.Errorf("Name() = %q, want %q", m.Name(), "Authentication")
	}
	if m.Version() != "1.0.0" {
		t.Errorf("Version() = %q, want %q", m.Version(), "1.0.0")
	}
}

// =============================================================================
// ValidatePasswordStrength — covers the remaining edge cases (password.go:79)
// Missing lowercase detection + common password reject
// =============================================================================

func TestValidatePasswordStrength_MissingLower(t *testing.T) {
	err := ValidatePasswordStrength("UPPERCASE1!", 8)
	if err == nil {
		t.Fatal("expected error for missing lowercase")
	}
}

func TestValidatePasswordStrength_CommonPassword(t *testing.T) {
	err := ValidatePasswordStrength("Password1!", 8)
	if err == nil {
		t.Fatal("expected error for common password 'Password1!' — contains 'password'")
	}
}

func TestValidatePasswordStrength_CommonPasswordExact(t *testing.T) {
	err := ValidatePasswordStrength("password", 8)
	if err == nil {
		t.Fatal("expected error for common password 'password'")
	}
}

func TestValidatePasswordStrength_MinLengthZero(t *testing.T) {
	// minLength=0 triggers the default of 12 in the function.
	err := ValidatePasswordStrength("Short1A!", 0)
	if err == nil {
		t.Fatal("expected error when minLength=0 and password is <12 chars")
	}
	// With minLength=0, a 12-char password should pass
	err2 := ValidatePasswordStrength("LongEnough1Ab!", 0)
	if err2 != nil {
		t.Fatalf("unexpected error: %v", err2)
	}
}

// =============================================================================
// validateStoredSecret — anti-replay path (totp_service.go:167-178)
// =============================================================================

// replayBoltStorer implements core.BoltStorer with a controlled lastStep.
type replayBoltStorer struct {
	lastStep int64
	getErr   error
	setErr   error
	getCalls int
	setCalls int
}

func (r *replayBoltStorer) Set(_ string, _ string, _ any, _ int64) error {
	r.setCalls++
	if r.setErr != nil {
		return r.setErr
	}
	return nil
}

func (r *replayBoltStorer) Get(_ string, _ string, dest any) error {
	r.getCalls++
	if r.getErr != nil {
		return r.getErr
	}
	if d, ok := dest.(*int64); ok {
		*d = r.lastStep
	}
	return nil
}

func (r *replayBoltStorer) Delete(_, _ string) error                { return nil }
func (r *replayBoltStorer) List(_ string) ([]string, error)         { return nil, nil }
func (r *replayBoltStorer) Close() error                            { return nil }
func (r *replayBoltStorer) BatchSet(_ []core.BoltBatchItem) error   { return nil }
func (r *replayBoltStorer) GetAPIKeyByPrefix(_ context.Context, _ string) (*models.APIKey, error) {
	return nil, errors.New("not found")
}
func (r *replayBoltStorer) GetWebhookSecret(_ string) (string, error) {
	return "", errors.New("not found")
}

type replayTestStore struct {
	core.Store
	user *core.User
}

func (s *replayTestStore) GetUser(_ context.Context, _ string) (*core.User, error) {
	return s.user, nil
}

func TestTOTPValidateStoredSecret_AntiReplay_RejectsReplayedStep(t *testing.T) {
	secret, _, err := GenerateTOTPSecret("u1", "alice@example.com")
	if err != nil {
		t.Fatalf("GenerateTOTPSecret: %v", err)
	}
	replay := &replayBoltStorer{lastStep: 999999999999} // Set lastStep far in the future

	store := &replayTestStore{
		user: &core.User{
			ID:          "u1",
			TOTPEnabled: true,
			TOTPSecret:  "enc:" + secret,
		},
	}
	svc := NewTOTPService(store)
	svc.SetVault(testTOTPVault{})
	svc.SetReplayStore(replay)

	// The current TOTP step will be <= lastStep (which is huge), so this should be false
	if svc.validateStoredSecret(context.Background(), "u1", currentTOTPCode(t, secret), true) {
		t.Fatal("validateStoredSecret should return false when step <= lastStep (anti-replay)")
	}
	if replay.getCalls == 0 {
		t.Error("expected Get to be called on replay store")
	}
}

func TestTOTPValidateStoredSecret_AntiReplay_GetErrorAccepted(t *testing.T) {
	secret, _, err := GenerateTOTPSecret("u1", "alice@example.com")
	if err != nil {
		t.Fatalf("GenerateTOTPSecret: %v", err)
	}
	replay := &replayBoltStorer{
		getErr: errors.New("key not found"),
	}

	store := &replayTestStore{
		user: &core.User{
			ID:          "u1",
			TOTPEnabled: true,
			TOTPSecret:  "enc:" + secret,
		},
	}
	svc := NewTOTPService(store)
	svc.SetVault(testTOTPVault{})
	svc.SetReplayStore(replay)

	// Get returns error (step not found), so anti-replay check passes
	if !svc.validateStoredSecret(context.Background(), "u1", currentTOTPCode(t, secret), true) {
		t.Fatal("validateStoredSecret should return true when Get returns error (first use)")
	}
	if replay.setCalls == 0 {
		t.Error("expected Set to be called on replay store after successful validation")
	}
}

func TestTOTPValidateStoredSecret_AntiReplay_SetErrorLogs(t *testing.T) {
	secret, _, err := GenerateTOTPSecret("u1", "alice@example.com")
	if err != nil {
		t.Fatalf("GenerateTOTPSecret: %v", err)
	}
	replay := &replayBoltStorer{
		getErr: errors.New("key not found"),
		setErr: errors.New("store full"),
	}

	store := &replayTestStore{
		user: &core.User{
			ID:          "u1",
			TOTPEnabled: true,
			TOTPSecret:  "enc:" + secret,
		},
	}
	svc := NewTOTPService(store)
	svc.SetVault(testTOTPVault{})
	svc.SetReplayStore(replay)

	// Set error should be tolerated (logged, not returned)
	if !svc.validateStoredSecret(context.Background(), "u1", currentTOTPCode(t, secret), true) {
		t.Fatal("validateStoredSecret should still return true even if Set fails")
	}
}

// =============================================================================
// consumeBackupCode — match + update success and error paths (totp_service.go:183-208)
// =============================================================================

type backupCodeTestStore struct {
	core.Store
	user            *core.User
	updateBackupErr error
	updateCalled    bool
}

func (s *backupCodeTestStore) GetUser(_ context.Context, _ string) (*core.User, error) {
	return s.user, nil
}

func (s *backupCodeTestStore) UpdateTOTPBackupCodes(_ context.Context, _ string, _ []string) error {
	s.updateCalled = true
	return s.updateBackupErr
}

func TestTOTPConsumeBackupCode_Success(t *testing.T) {
	// First generate valid backup codes
	codes, err := GenerateBackupCodes()
	if err != nil {
		t.Fatalf("GenerateBackupCodes: %v", err)
	}

	store := &backupCodeTestStore{
		user: &core.User{
			ID:              "u1",
			TOTPEnabled:     true,
			TOTPBackupCodes: codes.Hashes,
		},
	}
	svc := NewTOTPService(store)

	// Consume the first backup code
	if !svc.consumeBackupCode(context.Background(), "u1", codes.Plain[0]) {
		t.Fatal("consumeBackupCode should return true for valid code")
	}
	if !store.updateCalled {
		t.Error("expected UpdateTOTPBackupCodes to be called")
	}
}

func TestTOTPConsumeBackupCode_WrongCode(t *testing.T) {
	codes, err := GenerateBackupCodes()
	if err != nil {
		t.Fatalf("GenerateBackupCodes: %v", err)
	}

	store := &backupCodeTestStore{
		user: &core.User{
			ID:              "u1",
			TOTPEnabled:     true,
			TOTPBackupCodes: codes.Hashes,
		},
	}
	svc := NewTOTPService(store)

	if svc.consumeBackupCode(context.Background(), "u1", "ZZZZZZZZ") {
		t.Fatal("consumeBackupCode should return false for wrong code")
	}
}

func TestTOTPConsumeBackupCode_UpdateError(t *testing.T) {
	codes, err := GenerateBackupCodes()
	if err != nil {
		t.Fatalf("GenerateBackupCodes: %v", err)
	}

	store := &backupCodeTestStore{
		user: &core.User{
			ID:              "u1",
			TOTPEnabled:     true,
			TOTPBackupCodes: codes.Hashes,
		},
		updateBackupErr: errors.New("db write failed"),
	}
	svc := NewTOTPService(store)

	// The backup code should match but the update error should cause false return
	if svc.consumeBackupCode(context.Background(), "u1", codes.Plain[0]) {
		t.Fatal("consumeBackupCode should return false when UpdateTOTPBackupCodes fails")
	}
}

func TestTOTPConsumeBackupCode_UserNotEnabled(t *testing.T) {
	store := &backupCodeTestStore{
		user: &core.User{
			ID:          "u1",
			TOTPEnabled: false,
		},
	}
	svc := NewTOTPService(store)

	if svc.consumeBackupCode(context.Background(), "u1", "ABCDEF") {
		t.Fatal("consumeBackupCode should return false when TOTP is not enabled")
	}
}

func TestTOTPConsumeBackupCode_NoBackupCodes(t *testing.T) {
	store := &backupCodeTestStore{
		user: &core.User{
			ID:              "u1",
			TOTPEnabled:     true,
			TOTPBackupCodes: []string{},
		},
	}
	svc := NewTOTPService(store)

	if svc.consumeBackupCode(context.Background(), "u1", "ABCDEF") {
		t.Fatal("consumeBackupCode should return false when no backup codes exist")
	}
}

// =============================================================================
// JWT: belt-and-suspenders method check (jwt.go:233-234, 322-324)
// These are guarded by WithValidMethods in ParseWithClaims, making them
// unreachable in normal conditions. We test them by constructing a token
// where the claims type assertion succeeds despite a mismatched method.
// =============================================================================

func TestValidateAccessToken_ClaimsTypeAssertionFails(t *testing.T) {
	svc := MustNewJWTService("test-secret-key-at-least-32-bytes!")

	// Create a token with the wrong claims type (refreshTokenWithSession instead of Claims)
	now := time.Now()
	refreshClaims := refreshTokenWithSession{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
			Subject:   "user1",
			ID:        "test-jti",
			Issuer:    tokenIssuer,
			Audience:  jwt.ClaimStrings{tokenAudience},
		},
		FirstIssuedAt: now.Unix(),
	}
	tokenStr, err := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims).SignedString([]byte("test-secret-key-at-least-32-bytes!"))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	// ParseWithClaims expects *Claims but the token was signed with refreshTokenWithSession.
	// The JWT library will parse it but the type assertion token.Claims.(*Claims) will fail.
	_, err = svc.ValidateAccessToken(tokenStr)
	if err == nil {
		t.Error("expected error when claims type assertion fails")
	}
}

func TestValidateRefreshToken_WrongClaimsType(t *testing.T) {
	svc := MustNewJWTService("test-secret-key-at-least-32-bytes!")

	// Create an access token (Claims type) and try to validate it as a refresh token
	pair, err := svc.GenerateTokenPair("u", "t", "r", "e@e.com")
	if err != nil {
		t.Fatalf("GenerateTokenPair: %v", err)
	}

	// ValidateRefreshToken expects refreshTokenWithSession claims.
	// The access token uses Claims, so the type assertion will fail.
	_, err = svc.ValidateRefreshToken(pair.AccessToken)
	if err == nil {
		t.Error("expected error when validating access token as refresh token (wrong claims type)")
	}
}

// =============================================================================
// GenerateTOTPSecret — verify the provisioning URI is well-formed (totp.go:34)
// =============================================================================

func TestGenerateTOTPSecret_URIContainsExpectedParts(t *testing.T) {
	secret, uri, err := GenerateTOTPSecret("user-1", "test@example.com")
	if err != nil {
		t.Fatalf("GenerateTOTPSecret: %v", err)
	}
	if secret == "" {
		t.Error("secret should not be empty")
	}
	if !strings.HasPrefix(uri, "otpauth://totp/") {
		t.Errorf("uri = %q, want otpauth://totp/ prefix", uri)
	}
	if !strings.Contains(uri, "secret=") {
		t.Error("uri should contain secret=")
	}
	if !strings.Contains(uri, "issuer=DeployMonster") {
		t.Error("uri should contain issuer=DeployMonster")
	}
	if !strings.Contains(uri, "digits=6") {
		t.Error("uri should contain digits=6")
	}
	if !strings.Contains(uri, "period=30") {
		t.Error("uri should contain period=30")
	}
}

// =============================================================================
// GenerateBackupCodes — verify count and that hashes can be verified (totp.go:156)
// =============================================================================

func TestGenerateBackupCodes_CountAndVerify(t *testing.T) {
	codes, err := GenerateBackupCodes()
	if err != nil {
		t.Fatalf("GenerateBackupCodes: %v", err)
	}
	if len(codes.Plain) != 10 {
		t.Errorf("plain codes count = %d, want 10", len(codes.Plain))
	}
	if len(codes.Hashes) != 10 {
		t.Errorf("hashes count = %d, want 10", len(codes.Hashes))
	}
	for i, plain := range codes.Plain {
		if len(plain) != 8 {
			t.Errorf("code %d length = %d, want 8", i, len(plain))
		}
		// Verify the hash matches the plain code
		if err := bcrypt.CompareHashAndPassword([]byte(codes.Hashes[i]), []byte(plain)); err != nil {
			t.Errorf("code %d hash does not match plain text: %v", i, err)
		}
	}
}

// =============================================================================
// ValidateTOTPStep — edge cases (totp.go:73)
// =============================================================================

func TestValidateTOTPStep_InvalidTokenLength(t *testing.T) {
	_, ok := ValidateTOTPStep("12345", "SECRETBASE32")
	if ok {
		t.Error("expected false for 5-digit token")
	}
}

func TestValidateTOTPStep_InvalidSecret(t *testing.T) {
	_, ok := ValidateTOTPStep("123456", "!!!INVALID!!!")
	if ok {
		t.Error("expected false for invalid base32 secret")
	}
}

// =============================================================================
// ValidateTOTP — top-level validation function (totp.go:64)
// =============================================================================

func TestValidateTOTP_InvalidCode(t *testing.T) {
	if ValidateTOTP("000000", "SECRET") {
		t.Error("expected false for invalid code")
	}
}
