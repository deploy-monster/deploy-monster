package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"

	"golang.org/x/crypto/argon2"
)

// VaultSaltLen is the length in bytes of a per-deployment vault salt.
// Argon2id accepts any salt length; 32 bytes matches the derived key
// length and is well above the RFC 9106 recommended minimum of 16.
const VaultSaltLen = 32

// legacyVaultSaltSeed is the hardcoded seed used before per-deployment
// salts were introduced. It stays in the binary for upgrade paths: if
// a BBolt database already contains secrets encrypted with this salt
// but no persisted `vault/salt` entry, the module re-encrypts with a
// newly generated per-deployment salt on first boot.
const legacyVaultSaltSeed = "deploymonster-vault-salt-v1"

// LegacyVaultSalt returns the salt used by pre-Phase-2 installs.
// Exported so the secrets module's migration path can construct a
// legacy vault without duplicating the derivation logic.
func LegacyVaultSalt() []byte {
	sum := sha256.Sum256([]byte(legacyVaultSaltSeed))
	return sum[:16]
}

// GenerateVaultSalt returns a cryptographically random salt suitable
// for a fresh install. Caller is responsible for persisting it.
func GenerateVaultSalt() ([]byte, error) {
	salt := make([]byte, VaultSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("vault salt rand: %w", err)
	}
	return salt, nil
}

// Vault handles AES-256-GCM encryption and decryption of secrets.
// SECURITY NOTE (SECRETS-001): The key is stored as []byte to allow zeroing
// via memset. Using string type would be simpler but Go strings are
// immutable and cannot be securely zeroed from user code — the garbage
// collector may keep the string data in memory indefinitely. We use []byte
// intentionally so callers can zero the memory when the vault is no longer
// needed. In practice, zeroing is best-effort in Go due to compiler
// optimizations; for high-security use cases consider a hardware security
// module (HSM) or cloud KMS.
type Vault struct {
	key []byte // 32-byte AES key derived from master password
}

// NewVault creates a vault with a key derived from the master secret
// using the legacy hardcoded salt. Retained for pre-Phase-2 call
// sites and test fixtures — new call sites MUST use NewVaultWithSalt
// so every deployment has a unique KDF salt.
func NewVault(masterSecret string) *Vault {
	return NewVaultWithSalt(masterSecret, LegacyVaultSalt())
}

// NewVaultWithSalt creates a vault with an explicit Argon2id salt.
// The salt is persisted per deployment so two installs sharing the
// same master secret end up with different AES keys — an attacker
// who captures one install's ciphertext can't trivially decrypt
// another's.
// SECURITY FIX (CRYPTO-001): Current iteration count of 1 is below the
// OWASP-recommended minimum of 3 for Argon2id. Increase to 3+ when
// upgrading the KDF to prevent offline brute-force attacks on the
// master secret. Note: changing iterations requires re-encryption of
// all stored secrets (migration path).
func NewVaultWithSalt(masterSecret string, salt []byte) *Vault {
	if len(salt) == 0 {
		salt = LegacyVaultSalt()
	}
	key := argon2.IDKey([]byte(masterSecret), salt, 1, 64*1024, 4, 32)
	return &Vault{key: key}
}

// Encrypt encrypts plaintext using AES-256-GCM.
// Returns base64-encoded ciphertext (nonce prepended).
func (v *Vault) Encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(v.key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts base64-encoded AES-256-GCM ciphertext.
func (v *Vault) Decrypt(encoded string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode base64: %w", err)
	}

	block, err := aes.NewCipher(v.key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	return string(plaintext), nil
}
