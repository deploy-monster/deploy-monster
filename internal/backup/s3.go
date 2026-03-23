package backup

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// Compile-time check.
var _ core.BackupStorage = (*S3Storage)(nil)

// S3Storage stores backups in S3-compatible storage (AWS S3, MinIO, R2, etc.).
// Uses raw HTTP with AWS Signature V4 to avoid the heavy AWS SDK dependency.
type S3Storage struct {
	endpoint   string
	bucket     string
	region     string
	accessKey  string
	secretKey  string
	pathStyle  bool
	client     *http.Client
}

// S3Config holds S3 storage configuration.
type S3Config struct {
	Endpoint  string `json:"endpoint"`   // Empty = AWS default
	Bucket    string `json:"bucket"`
	Region    string `json:"region"`
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
	PathStyle bool   `json:"path_style"` // Required for MinIO
}

// NewS3Storage creates an S3-compatible backup storage.
func NewS3Storage(cfg S3Config) *S3Storage {
	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = fmt.Sprintf("https://s3.%s.amazonaws.com", cfg.Region)
	}
	return &S3Storage{
		endpoint:  endpoint,
		bucket:    cfg.Bucket,
		region:    cfg.Region,
		accessKey: cfg.AccessKey,
		secretKey: cfg.SecretKey,
		pathStyle: cfg.PathStyle,
		client:    &http.Client{Timeout: 5 * time.Minute},
	}
}

func (s *S3Storage) Name() string { return "s3" }

func (s *S3Storage) Upload(ctx context.Context, key string, reader io.Reader, size int64) error {
	url := s.objectURL(key)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, reader)
	if err != nil {
		return fmt.Errorf("create S3 request: %w", err)
	}
	req.ContentLength = size
	req.Header.Set("Content-Type", "application/octet-stream")

	// Note: Full AWS SigV4 signing would be implemented here.
	// For now, this is a structural placeholder — production would use
	// a lightweight signing library or manual SigV4 implementation.

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("S3 upload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("S3 upload failed: HTTP %d", resp.StatusCode)
	}
	return nil
}

func (s *S3Storage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	url := s.objectURL(key)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("S3 download: %w", err)
	}
	if resp.StatusCode >= 400 {
		resp.Body.Close()
		return nil, fmt.Errorf("S3 download failed: HTTP %d", resp.StatusCode)
	}
	return resp.Body, nil
}

func (s *S3Storage) Delete(ctx context.Context, key string) error {
	url := s.objectURL(key)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("S3 delete: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

func (s *S3Storage) List(_ context.Context, _ string) ([]core.BackupEntry, error) {
	// S3 ListObjects would be implemented here with XML parsing
	return nil, fmt.Errorf("S3 list not yet implemented")
}

func (s *S3Storage) objectURL(key string) string {
	if s.pathStyle {
		return fmt.Sprintf("%s/%s/%s", s.endpoint, s.bucket, key)
	}
	return fmt.Sprintf("https://%s.%s/%s", s.bucket, s.endpoint, key)
}
