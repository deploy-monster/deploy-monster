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
