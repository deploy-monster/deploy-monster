package backup

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

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
	os.MkdirAll(basePath, 0750)
	return &LocalStorage{basePath: basePath}
}

func (l *LocalStorage) Name() string { return "local" }

func (l *LocalStorage) Upload(_ context.Context, key string, reader io.Reader, _ int64) error {
	path := filepath.Join(l.basePath, key)
	os.MkdirAll(filepath.Dir(path), 0750)

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create backup file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, reader); err != nil {
		return fmt.Errorf("write backup: %w", err)
	}
	return nil
}

func (l *LocalStorage) Download(_ context.Context, key string) (io.ReadCloser, error) {
	path := filepath.Join(l.basePath, key)
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open backup: %w", err)
	}
	return f, nil
}

func (l *LocalStorage) Delete(_ context.Context, key string) error {
	path := filepath.Join(l.basePath, key)
	return os.Remove(path)
}

func (l *LocalStorage) List(_ context.Context, prefix string) ([]core.BackupEntry, error) {
	pattern := filepath.Join(l.basePath, prefix+"*")
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
