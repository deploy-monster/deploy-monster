package secrets

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// rotateStoreWrapper composes the existing mockSecretStore so we can
// override the two methods RotateEncryptionKey actually calls and
// inject error scenarios that the success-path test does not exercise.
type rotateStoreWrapper struct {
	*mockSecretStore
	listErr   error
	updateErr error
}

func (w *rotateStoreWrapper) ListAllSecretVersions(ctx context.Context) ([]core.SecretVersion, error) {
	if w.listErr != nil {
		return nil, w.listErr
	}
	return w.mockSecretStore.ListAllSecretVersions(ctx)
}

func (w *rotateStoreWrapper) UpdateSecretVersionValue(ctx context.Context, id, valueEnc string) error {
	if w.updateErr != nil {
		return w.updateErr
	}
	return w.mockSecretStore.UpdateSecretVersionValue(ctx, id, valueEnc)
}

func TestRotateEncryptionKey_ListVersionsError(t *testing.T) {
	store := &rotateStoreWrapper{
		mockSecretStore: newMockSecretStore(),
		listErr:         errors.New("boom: db unavailable"),
	}
	m := &Module{store: store, vault: NewVault("any-32-bytes-or-more-master-key!")}

	rotated, err := m.RotateEncryptionKey(context.Background(), "new-master-key-32-bytes-7654321")
	if err == nil {
		t.Fatal("expected error when ListAllSecretVersions fails")
	}
	if !strings.Contains(err.Error(), "list secret versions") {
		t.Errorf("err = %v, want wrapped 'list secret versions'", err)
	}
	if rotated != 0 {
		t.Errorf("rotated = %d, want 0 on early failure", rotated)
	}
}

func TestRotateEncryptionKey_DecryptFailureMidway(t *testing.T) {
	old := NewVault("old-master-key-32-bytes-1234567")
	store := newMockSecretStore()

	// First version is properly encrypted; second carries a corrupted
	// ciphertext so the rotation loop fails on the second iteration.
	good, _ := old.Encrypt("ok-secret")
	store.versions["s1"] = &core.SecretVersion{ID: "v1", SecretID: "s1", ValueEnc: good, Version: 1}
	store.versions["s2"] = &core.SecretVersion{ID: "v2", SecretID: "s2", ValueEnc: "not-base64!!", Version: 1}

	m := &Module{store: store, vault: old}

	rotated, err := m.RotateEncryptionKey(context.Background(), "new-master-key-32-bytes-7654321")
	if err == nil {
		t.Fatal("expected decrypt error on corrupted ciphertext")
	}
	if !strings.Contains(err.Error(), "decrypt version") {
		t.Errorf("err = %v, want wrapped 'decrypt version'", err)
	}
	// Map iteration order is unstable; the failure may interrupt before
	// the good version was rotated, so allow rotated >= 0.
	if rotated < 0 || rotated > 1 {
		t.Errorf("rotated = %d, want 0 or 1 (depends on map iteration order)", rotated)
	}
}

func TestRotateEncryptionKey_UpdateStoreError(t *testing.T) {
	old := NewVault("old-master-key-32-bytes-1234567")
	enc, _ := old.Encrypt("password")

	wrapper := &rotateStoreWrapper{mockSecretStore: newMockSecretStore()}
	wrapper.versions["s1"] = &core.SecretVersion{ID: "v1", SecretID: "s1", ValueEnc: enc, Version: 1}
	wrapper.updateErr = errors.New("write disk full")

	m := &Module{store: wrapper, vault: old}

	rotated, err := m.RotateEncryptionKey(context.Background(), "new-master-key-32-bytes-7654321")
	if err == nil {
		t.Fatal("expected error when UpdateSecretVersionValue fails")
	}
	if !strings.Contains(err.Error(), "update version") {
		t.Errorf("err = %v, want wrapped 'update version'", err)
	}
	if rotated != 0 {
		t.Errorf("rotated = %d, want 0 (failure on first iteration)", rotated)
	}
}
