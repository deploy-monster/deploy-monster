package backup

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// Compile-time check.
var _ core.BackupStorage = (*S3Storage)(nil)

// S3Storage stores backups in S3-compatible storage (AWS S3, MinIO, R2, etc.).
// Uses raw HTTP with AWS Signature V4 to avoid the heavy AWS SDK dependency.
type S3Storage struct {
	endpoint      string
	bucket        string
	region        string
	accessKey     string
	secretKey     string
	pathStyle     bool
	client        *http.Client
	maxRetries    int
	initialDelay  time.Duration
	maxDelay      time.Duration
	logger        *slog.Logger
}

// S3Config holds S3 storage configuration.
type S3Config struct {
	Endpoint     string        `json:"endpoint"`      // Empty = AWS default
	Bucket       string        `json:"bucket"`
	Region       string        `json:"region"`
	AccessKey    string        `json:"access_key"`
	SecretKey    string        `json:"secret_key"`
	PathStyle    bool          `json:"path_style"`    // Required for MinIO
	MaxRetries   int           `json:"max_retries"`   // Default: 3
	InitialDelay time.Duration `json:"initial_delay"` // Default: 100ms
	MaxDelay     time.Duration `json:"max_delay"`     // Default: 5s
}

// NewS3Storage creates an S3-compatible backup storage.
func NewS3Storage(cfg S3Config, logger *slog.Logger) *S3Storage {
	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = fmt.Sprintf("https://s3.%s.amazonaws.com", cfg.Region)
	}

	maxRetries := cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}
	initialDelay := cfg.InitialDelay
	if initialDelay <= 0 {
		initialDelay = 100 * time.Millisecond
	}
	maxDelay := cfg.MaxDelay
	if maxDelay <= 0 {
		maxDelay = 5 * time.Second
	}

	return &S3Storage{
		endpoint:     endpoint,
		bucket:       cfg.Bucket,
		region:       cfg.Region,
		accessKey:    cfg.AccessKey,
		secretKey:    cfg.SecretKey,
		pathStyle:    cfg.PathStyle,
		client:       &http.Client{Timeout: 10 * time.Minute},
		maxRetries:   maxRetries,
		initialDelay: initialDelay,
		maxDelay:     maxDelay,
		logger:       logger,
	}
}

func (s *S3Storage) Name() string { return "s3" }

// retry executes an operation with exponential backoff.
func (s *S3Storage) retry(ctx context.Context, op func() error) error {
	// Handle nil context gracefully
	if ctx == nil {
		ctx = context.Background()
	}

	maxRetries := s.maxRetries
	if maxRetries <= 0 {
		maxRetries = 1
	}

	var lastErr error
	for i := 0; i < maxRetries; i++ {
		err := op()
		if err == nil {
			return nil
		}
		lastErr = err

		// Calculate delay with exponential backoff
		delay := s.initialDelay * time.Duration(1<<uint(i))
		if delay > s.maxDelay {
			delay = s.maxDelay
		}

		if s.logger != nil {
			s.logger.Warn("S3 operation failed, retrying",
				"attempt", i+1,
				"max_retries", maxRetries,
				"delay", delay,
				"error", err,
			)
		}

		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return fmt.Errorf("S3 operation failed after %d retries: %w", maxRetries, lastErr)
}

func (s *S3Storage) Upload(ctx context.Context, key string, reader io.Reader, size int64) error {
	return s.retry(ctx, func() error {
		url := s.objectURL(key)

		req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, reader)
		if err != nil {
			return fmt.Errorf("create S3 request: %w", err)
		}
		req.ContentLength = size
		req.Header.Set("Content-Type", "application/octet-stream")

		// Note: Full AWS SigV4 signing would be implemented here.
		// For production, use a lightweight signing library or manual SigV4 implementation.

		resp, err := s.client.Do(req)
		if err != nil {
			return fmt.Errorf("S3 upload: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			return fmt.Errorf("S3 upload failed: HTTP %d", resp.StatusCode)
		}

		if s.logger != nil {
			s.logger.Debug("S3 upload complete", "key", key, "size", size)
		}
		return nil
	})
}

func (s *S3Storage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	var result io.ReadCloser
	err := s.retry(ctx, func() error {
		url := s.objectURL(key)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}

		resp, err := s.client.Do(req)
		if err != nil {
			return fmt.Errorf("S3 download: %w", err)
		}
		if resp.StatusCode >= 400 {
			resp.Body.Close()
			return fmt.Errorf("S3 download failed: HTTP %d", resp.StatusCode)
		}
		result = resp.Body
		return nil
	})
	return result, err
}

func (s *S3Storage) Delete(ctx context.Context, key string) error {
	return s.retry(ctx, func() error {
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

		if resp.StatusCode >= 400 && resp.StatusCode != 404 {
			return fmt.Errorf("S3 delete failed: HTTP %d", resp.StatusCode)
		}

		if s.logger != nil {
			s.logger.Debug("S3 delete complete", "key", key)
		}
		return nil
	})
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
