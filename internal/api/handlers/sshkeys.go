package handlers

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
	"golang.org/x/crypto/ssh"
)

// SSHKeyHandler manages SSH keys for server access.
type SSHKeyHandler struct {
	store core.Store
}

func NewSSHKeyHandler(store core.Store) *SSHKeyHandler {
	return &SSHKeyHandler{store: store}
}

// SSHKeyInfo represents an SSH key.
type SSHKeyInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Fingerprint string `json:"fingerprint"`
	PublicKey   string `json:"public_key"`
}

// Generate handles POST /api/v1/ssh-keys/generate
// Generates a new Ed25519 SSH key pair.
func (h *SSHKeyHandler) Generate(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	// Generate Ed25519 key pair
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "key generation failed")
		return
	}

	sshPub, err := ssh.NewPublicKey(pubKey)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SSH key conversion failed")
		return
	}

	pubKeyStr := string(ssh.MarshalAuthorizedKey(sshPub))

	// PEM encode private key
	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "OPENSSH PRIVATE KEY",
		Bytes: privKey,
	})

	// Fingerprint
	fp := ssh.FingerprintSHA256(sshPub)

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":          core.GenerateID(),
		"name":        req.Name,
		"public_key":  pubKeyStr,
		"private_key": string(privPEM), // Only returned once at generation
		"fingerprint": fp,
	})
}

// List handles GET /api/v1/ssh-keys
func (h *SSHKeyHandler) List(w http.ResponseWriter, _ *http.Request) {
	// Would query SSH keys from DB
	writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "total": 0})
}
