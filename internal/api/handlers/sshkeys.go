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
	bolt  core.BoltStorer
}

func NewSSHKeyHandler(store core.Store, bolt core.BoltStorer) *SSHKeyHandler {
	return &SSHKeyHandler{store: store, bolt: bolt}
}

// SSHKeyInfo represents an SSH key.
type SSHKeyInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Fingerprint string `json:"fingerprint"`
	PublicKey   string `json:"public_key"`
}

// sshKeyList wraps the persisted list of SSH keys for a user.
type sshKeyList struct {
	Keys []SSHKeyInfo `json:"keys"`
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

	keyID := core.GenerateID()
	info := SSHKeyInfo{
		ID:          keyID,
		Name:        req.Name,
		Fingerprint: fp,
		PublicKey:   pubKeyStr,
	}

	// Store key metadata in BBolt (private key is only returned once)
	var list sshKeyList
	_ = h.bolt.Get("ssh_keys", claims.UserID, &list)

	if len(list.Keys) >= 50 {
		writeError(w, http.StatusConflict, "SSH key limit reached (50)")
		return
	}
	list.Keys = append(list.Keys, info)

	if err := h.bolt.Set("ssh_keys", claims.UserID, list, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store SSH key")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":          keyID,
		"name":        req.Name,
		"public_key":  pubKeyStr,
		"private_key": string(privPEM), // Only returned once at generation
		"fingerprint": fp,
	})
}

// List handles GET /api/v1/ssh-keys
func (h *SSHKeyHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var list sshKeyList
	if err := h.bolt.Get("ssh_keys", claims.UserID, &list); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "total": 0})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": list.Keys, "total": len(list.Keys)})
}
