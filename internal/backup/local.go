package backup

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// Compile-time check.
var _ core.BackupStorage = (*LocalStorage)(nil)

// LocalStorage stores backups on the local filesystem.
// Optionally encrypts payloads using AES-256-GCM when an encryption key is set.
type LocalStorage struct {
	basePath    string
	encryptionKey []byte // 32 bytes; nil means no encryption
}

// NewLocalStorage creates a local backup storage target. The base path is
// canonicalized via filepath.Clean so subsequent path-traversal checks
// (which compare against l.basePath via filepath.Clean) compare like-with-
// like. Without this, a "./backups" config produced clean paths that did
// not start with the literal "./backups" string and every List call —
// including the retention sweep — failed with "path outside storage root".
func NewLocalStorage(basePath string, encryptionKey []byte) *LocalStorage {
	if abs, err := filepath.Abs(basePath); err == nil {
		basePath = abs
	} else {
		basePath = filepath.Clean(basePath)
	}
	_ = os.MkdirAll(basePath, 0750)
	return &LocalStorage{basePath: basePath, encryptionKey: encryptionKey}
}

func (l *LocalStorage) Name() string { return "local" }

func (l *LocalStorage) Upload(_ context.Context, key string, reader io.Reader, _ int64) error {
	// Reject absolute paths to prevent bypassing the join with an absolute key
	if filepath.IsAbs(key) {
		return fmt.Errorf("invalid backup key: absolute paths not allowed")
	}
	// Join and clean ensures the key is resolved within basePath (not current dir)
	fullPath := filepath.Join(l.basePath, key)
	cleanPath := filepath.Clean(fullPath)
	rel, err := filepath.Rel(l.basePath, cleanPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("invalid backup key: path outside storage root")
	}
	path := cleanPath
	_ = os.MkdirAll(filepath.Dir(path), 0750)

	// Read the full payload before we know whether we need to encrypt —
	// SigV4 (S3) and path traversal checks also require full reads, so
	// this is consistent with the existing pattern.
	data, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("read backup payload: %w", err)
	}

	// Encrypt in-place if a key is configured.
	if l.encryptionKey != nil {
		encrypted, err := encryptAES256GCM(data, l.encryptionKey)
		if err != nil {
			return fmt.Errorf("encrypt backup: %w", err)
		}
		data = encrypted
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create backup file: %w", err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write backup: %w", err)
	}
	return nil
}

// encryptAES256GCM encrypts data using AES-256-GCM with a randomly generated nonce.
// The nonce is prepended to the ciphertext, so the total output is nonce+ciphertext.
func encryptAES256GCM(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// decryptAES256GCM decrypts AES-256-GCM ciphertext produced by encryptAES256GCM.
// The first GCM nonceSize bytes are the nonce; the rest is the ciphertext.
func decryptAES256GCM(ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short for GCM nonce")
	}
	nonce, data := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, data, nil)
	if err != nil {
		return nil, fmt.Errorf("GCM decryption failed: %w", err)
	}
	return plaintext, nil
}

func (l *LocalStorage) Download(_ context.Context, key string) (io.ReadCloser, error) {
	// Reject absolute paths to prevent bypassing the join with an absolute key
	if filepath.IsAbs(key) {
		return nil, fmt.Errorf("invalid backup key: absolute paths not allowed")
	}
	// Join and clean ensures the key is resolved within basePath (not current dir)
	fullPath := filepath.Join(l.basePath, key)
	cleanPath := filepath.Clean(fullPath)
	// Use filepath.Rel to ensure the resolved path is within basePath
	rel, err := filepath.Rel(l.basePath, cleanPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return nil, fmt.Errorf("invalid backup key: path outside storage root")
	}
	f, err := os.Open(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("open backup: %w", err)
	}

	// If encryption is enabled, we need to read and decrypt the full file.
	// io.ReadAll is acceptable here since backup files are not streamed.
	if l.encryptionKey != nil {
		data, err := io.ReadAll(f)
		_ = f.Close()
		if err != nil {
			return nil, fmt.Errorf("read encrypted backup: %w", err)
		}
		plaintext, err := decryptAES256GCM(data, l.encryptionKey)
		if err != nil {
			return nil, fmt.Errorf("decrypt backup: %w", err)
		}
		return io.NopCloser(strings.NewReader(string(plaintext))), nil
	}

	return f, nil
}

func (l *LocalStorage) Delete(_ context.Context, key string) error {
	// Reject absolute paths to prevent bypassing the join with an absolute key
	if filepath.IsAbs(key) {
		return fmt.Errorf("invalid backup key: absolute paths not allowed")
	}
	// Join and clean ensures the key is resolved within basePath (not current dir)
	fullPath := filepath.Join(l.basePath, key)
	cleanPath := filepath.Clean(fullPath)
	// Use filepath.Rel to ensure the resolved path is within basePath
	rel, err := filepath.Rel(l.basePath, cleanPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("invalid backup key: path outside storage root")
	}
	return os.Remove(cleanPath)
}

func (l *LocalStorage) List(_ context.Context, prefix string) ([]core.BackupEntry, error) {
	// Sanitize prefix to prevent path traversal. Both basePath (set in
	// NewLocalStorage) and target are absolute paths so the rel-based
	// containment check is comparing canonical strings.
	target := filepath.Clean(filepath.Join(l.basePath, prefix))
	rel, err := filepath.Rel(l.basePath, target)
	if err != nil || strings.HasPrefix(rel, "..") {
		return nil, fmt.Errorf("invalid backup prefix: path outside storage root")
	}

	// Two prefix flavours are supported:
	//   1. Directory prefix (e.g. "tenant-id/app-id"): recursively walk that
	//      subtree and surface every file. The retention sweep and the API
	//      List endpoint both use this form.
	//   2. Filename prefix within a directory (e.g. "test-"): glob siblings
	//      whose name starts with the prefix. Used by older callers and a
	//      few unit tests that assert on filename matching semantics.
	walkRoot := target
	filenameFilter := ""
	if info, statErr := os.Stat(target); statErr != nil || !info.IsDir() {
		walkRoot = filepath.Dir(target)
		filenameFilter = filepath.Base(target)
	}

	entries := make([]core.BackupEntry, 0)
	walkErr := filepath.Walk(walkRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsDir() {
			return nil
		}
		if filenameFilter != "" && !strings.HasPrefix(filepath.Base(path), filenameFilter) {
			return nil
		}
		// Walk reports symlinks via Lstat so a broken link wouldn't fail
		// the callback. Stat the target explicitly so we can drop entries
		// whose underlying file no longer exists — a backup pointing at a
		// missing target shouldn't appear in the list.
		st, statErr := os.Stat(path)
		if statErr != nil {
			return nil
		}
		rel, relErr := filepath.Rel(l.basePath, path)
		if relErr != nil {
			return nil
		}
		entries = append(entries, core.BackupEntry{
			Key:       filepath.ToSlash(rel),
			Size:      st.Size(),
			CreatedAt: st.ModTime().Unix(),
		})
		return nil
	})
	if walkErr != nil && !os.IsNotExist(walkErr) {
		return nil, walkErr
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].CreatedAt > entries[j].CreatedAt
	})

	return entries, nil
}
