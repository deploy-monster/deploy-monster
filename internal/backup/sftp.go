package backup

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// Compile-time check.
var _ core.BackupStorage = (*SFTPStorage)(nil)

// SFTPStorage stores backups on a remote SFTP server.
type SFTPStorage struct {
	host      string
	port      int
	user      string
	password  string
	basePath  string
	knownHost string
}

// SFTPConfig holds SFTP storage configuration.
type SFTPConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"` // Default: 22
	User     string `json:"user"`
	Password string `json:"password"`
	BasePath string `json:"base_path"` // Remote directory for backups
	KnownHost string `json:"known_host"` // Path to known_hosts file (optional)
}

// NewSFTPStorage creates an SFTP backup storage target.
func NewSFTPStorage(cfg SFTPConfig) *SFTPStorage {
	port := cfg.Port
	if port == 0 {
		port = 22
	}
	path := cfg.BasePath
	if path == "" {
		path = "/backups"
	}
	return &SFTPStorage{
		host:     cfg.Host,
		port:     port,
		user:     cfg.User,
		password: cfg.Password,
		basePath: path,
		knownHost: cfg.KnownHost,
	}
}

func (s *SFTPStorage) Name() string { return "sftp" }

// Upload stores data at the given key path on the SFTP server.
func (s *SFTPStorage) Upload(ctx context.Context, key string, reader io.Reader, _ int64) error {
	// Reject absolute paths for security
	if strings.HasPrefix(key, "/") {
		return fmt.Errorf("invalid backup key: absolute paths not allowed")
	}

	fullPath := filepath.Join(s.basePath, key)
	dirPath := filepath.Dir(fullPath)

	conn, err := s.dial(ctx)
	if err != nil {
		return fmt.Errorf("SFTP dial: %w", err)
	}
	defer func() { _ = conn.Close() }()

	// Ensure directory exists
	if err := conn.MkdirAll(dirPath); err != nil {
		return fmt.Errorf("create remote directory: %w", err)
	}

	// Upload file
	f, err := conn.Create(fullPath)
	if err != nil {
		return fmt.Errorf("create remote file: %w", err)
	}
	defer func() { _ = f.Close() }()

	if _, err := io.Copy(f, reader); err != nil {
		return fmt.Errorf("upload to SFTP: %w", err)
	}

	return nil
}

// Download retrieves data from the given key path on the SFTP server.
func (s *SFTPStorage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	if strings.HasPrefix(key, "/") {
		return nil, fmt.Errorf("invalid backup key: absolute paths not allowed")
	}

	fullPath := filepath.Join(s.basePath, key)

	conn, err := s.dial(ctx)
	if err != nil {
		return nil, fmt.Errorf("SFTP dial: %w", err)
	}

	f, err := conn.Open(fullPath)
	if err != nil {
		return nil, fmt.Errorf("open remote file: %w", err)
	}

	// Return a ReadCloser that also closes the SFTP connection
	return &sftpReadCloser{ReadCloser: f, conn: conn}, nil
}

// Delete removes a file at the given key path from the SFTP server.
func (s *SFTPStorage) Delete(ctx context.Context, key string) error {
	if strings.HasPrefix(key, "/") {
		return fmt.Errorf("invalid backup key: absolute paths not allowed")
	}

	fullPath := filepath.Join(s.basePath, key)

	conn, err := s.dial(ctx)
	if err != nil {
		return fmt.Errorf("SFTP dial: %w", err)
	}
	defer func() { _ = conn.Close() }()

	return conn.Remove(fullPath)
}

// List returns all backup entries under the given prefix.
func (s *SFTPStorage) List(ctx context.Context, prefix string) ([]core.BackupEntry, error) {
	if strings.HasPrefix(prefix, "/") {
		return nil, fmt.Errorf("invalid backup prefix: absolute paths not allowed")
	}

	searchPath := s.basePath
	if prefix != "" {
		searchPath = filepath.Join(s.basePath, prefix)
	}

	conn, err := s.dial(ctx)
	if err != nil {
		return nil, fmt.Errorf("SFTP dial: %w", err)
	}
	defer func() { _ = conn.Close() }()

	entries, err := conn.ReadDir(searchPath)
	if err != nil {
		// Directory might not exist yet
		if os.IsNotExist(err) {
			return []core.BackupEntry{}, nil
		}
		return nil, fmt.Errorf("read remote directory: %w", err)
	}

	result := make([]core.BackupEntry, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			key := entry.Name()
			if prefix != "" {
				key = filepath.Join(prefix, entry.Name())
			}
			result = append(result, core.BackupEntry{
				Key:       key,
				Size:      entry.Size(),
				CreatedAt: entry.ModTime().Unix(),
			})
		}
	}

	return result, nil
}

// sftpReadCloser wraps an io.ReadCloser to also close the SFTP connection.
type sftpReadCloser struct {
	io.ReadCloser
	conn interface{ Close() error }
}

func (r *sftpReadCloser) Close() error {
	_ = r.ReadCloser.Close()
	return r.conn.Close()
}

// dial establishes an SFTP connection. This is a simplified implementation
// that would use golang.org/x/crypto/sftp in production.
func (s *SFTPStorage) dial(ctx context.Context) (sftpConn, error) {
	addr := fmt.Sprintf("%s:%d", s.host, s.port)

	// Use dialer with context support
	dialer := &net.Dialer{Timeout: 30 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("TCP dial to %s: %w", addr, err)
	}

	// Note: Full SFTP implementation would use golang.org/x/crypto/ssh
	// and golang.org/x/crypto/sftp packages. This is a stub that returns
	// an error indicating SFTP is not yet fully implemented.
	_ = conn.Close()
	return nil, fmt.Errorf("SFTP storage not fully implemented: requires golang.org/x/crypto/ssh and sftp packages")
}

// sftpConn is a stub for the SFTP connection interface.
type sftpConn interface {
	MkdirAll(path string) error
	Create(path string) (io.WriteCloser, error)
	Open(path string) (io.ReadCloser, error)
	Remove(path string) error
	ReadDir(path string) ([]os.FileInfo, error)
	Close() error
}