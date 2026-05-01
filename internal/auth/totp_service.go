package auth

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// TOTPService handles TOTP MFA operations.
// It provides enrollment, verification, and backup code management.
// The TOTP secret is stored encrypted using the secrets vault for
// secure storage and retrieval during validation.
type TOTPService struct {
	store  core.Store
	vault  interface {
		Encrypt(string) (string, error)
		Decrypt(string) (string, error)
	}
	logger *slog.Logger
}

// NewTOTPService creates a new TOTP service.
func NewTOTPService(store core.Store) *TOTPService {
	return &TOTPService{store: store}
}

// SetVault sets the encryption vault for TOTP secrets.
// This should be called during module initialization once the vault is available.
func (s *TOTPService) SetVault(vault interface {
	Encrypt(string) (string, error)
	Decrypt(string) (string, error)
}) {
	s.vault = vault
}

// Enroll generates a new TOTP secret for a user and returns the provisioning URI.
// The secret is stored encrypted in the user's totp_secret_enc field.
// The plain text secret is only returned once and should be shown to the user
// (e.g., via QR code or manual entry).
func (s *TOTPService) Enroll(ctx context.Context, userID string) (provisioningURI string, err error) {
	if s.vault == nil {
		return "", fmt.Errorf("TOTP vault not configured")
	}

	// Get user
	user, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("get user: %w", err)
	}

	if user.TOTPEnabled {
		return "", fmt.Errorf("TOTP is already enabled")
	}

	// Generate a new TOTP secret
	secret, uri, err := GenerateTOTPSecret(userID, user.Email)
	if err != nil {
		return "", fmt.Errorf("generate secret: %w", err)
	}

	// Encrypt the secret using the vault before storing
	encryptedSecret, err := s.vault.Encrypt(secret)
	if err != nil {
		return "", fmt.Errorf("encrypt secret: %w", err)
	}

	// Enable TOTP for the user with the encrypted secret
	if err := s.store.UpdateTOTPEnabled(ctx, userID, true, encryptedSecret); err != nil {
		return "", fmt.Errorf("enable totp: %w", err)
	}

	s.logger.Info("TOTP enrolled for user", "user_id", userID)

	return uri, nil
}

// Validate validates a TOTP code against the user's stored secret.
// The stored secret is encrypted using the secrets vault; we decrypt it
// and then generate the expected TOTP code for comparison.
func (s *TOTPService) Validate(userID, code string) bool {
	if s.vault == nil {
		return false
	}

	user, err := s.store.GetUser(nil, userID)
	if err != nil || !user.TOTPEnabled || user.TOTPSecret == "" {
		return false
	}

	// Decrypt the stored secret
	secret, err := s.vault.Decrypt(user.TOTPSecret)
	if err != nil {
		s.logger.Warn("failed to decrypt TOTP secret", "user_id", userID, "error", err)
		return false
	}

	// Validate the TOTP code against the decrypted secret
	return ValidateTOTP(code, secret)
}

// Disable disables TOTP for a user after validating the provided code.
func (s *TOTPService) Disable(ctx context.Context, userID, code string) error {
	user, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	if !user.TOTPEnabled {
		return fmt.Errorf("TOTP is not enabled")
	}

	// Validate the TOTP code before disabling
	if !s.Validate(userID, code) {
		return fmt.Errorf("invalid TOTP code")
	}

	// Disable TOTP and clear the stored secret
	if err := s.store.UpdateTOTPEnabled(ctx, userID, false, ""); err != nil {
		return fmt.Errorf("disable totp: %w", err)
	}

	s.logger.Info("TOTP disabled for user", "user_id", userID)

	return nil
}

// Status returns whether TOTP is enabled for a user.
func (s *TOTPService) Status(userID string) (enabled bool, err error) {
	user, err := s.store.GetUser(nil, userID)
	if err != nil {
		return false, fmt.Errorf("get user: %w", err)
	}
	return user.TOTPEnabled, nil
}

// GenerateBackupCodes generates a new set of backup codes for a user.
// The plain text codes are returned once for display to the user.
func (s *TOTPService) GenerateBackupCodes(ctx context.Context, userID string) (*BackupCodes, error) {
	codes, err := GenerateBackupCodes()
	if err != nil {
		return nil, err
	}

	// Store the hashed codes in the user's record or a separate table
	// For now, return both hashes and plain codes
	return codes, nil
}