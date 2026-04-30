package backup

import (
	"context"
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
type LocalStorage struct {
	basePath string
}

// NewLocalStorage creates a local backup storage target.
func NewLocalStorage(basePath string) *LocalStorage {
	_ = os.MkdirAll(basePath, 0750)
	return &LocalStorage{basePath: basePath}
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

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create backup file: %w", err)
	}
	defer func() { _ = f.Close() }()

	if _, err := io.Copy(f, reader); err != nil {
		return fmt.Errorf("write backup: %w", err)
	}
	return nil
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
	// Sanitize prefix to prevent path traversal.
	cleanPrefix := filepath.Clean(l.basePath + "/" + prefix)
	if !strings.HasPrefix(cleanPrefix, l.basePath) {
		return nil, fmt.Errorf("invalid backup prefix: path outside storage root")
	}
	pattern := cleanPrefix + "*"
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	entries := make([]core.BackupEntry, 0, len(matches))
	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		rel, _ := filepath.Rel(l.basePath, path)
		entries = append(entries, core.BackupEntry{
			Key:       rel,
			Size:      info.Size(),
			CreatedAt: info.ModTime().Unix(),
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].CreatedAt > entries[j].CreatedAt
	})

	return entries, nil
}
