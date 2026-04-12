package backup

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/awsauth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// Compile-time check.
var _ core.BackupStorage = (*S3Storage)(nil)

// S3Storage stores backups in S3-compatible storage (AWS S3, MinIO, R2, etc.).
// Uses raw HTTP with AWS Signature V4 to avoid the heavy AWS SDK dependency.
type S3Storage struct {
	endpoint     string
	bucket       string
	region       string
	accessKey    string
	secretKey    string
	pathStyle    bool
	client       *http.Client
	maxRetries   int
	initialDelay time.Duration
	maxDelay     time.Duration
	logger       *slog.Logger
}

// S3Config holds S3 storage configuration.
type S3Config struct {
	Endpoint     string        `json:"endpoint"` // Empty = AWS default
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
		endpoint = fmt.Sprintf("s3.%s.amazonaws.com", cfg.Region)
	} else {
		// Strip scheme for consistent URL building
		endpoint = stripScheme(endpoint)
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

// stripScheme removes http:// or https:// prefix from a URL.
func stripScheme(endpoint string) string {
	if strings.HasPrefix(endpoint, "https://") {
		return strings.TrimPrefix(endpoint, "https://")
	}
	if strings.HasPrefix(endpoint, "http://") {
		return strings.TrimPrefix(endpoint, "http://")
	}
	return endpoint
}

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
	// SigV4 requires the full payload for the signature, so the
	// request body has to be fully buffered before signing. This
	// trades memory for correctness — streaming uploads would need
	// S3's UNSIGNED-PAYLOAD chunk encoding, which we don't need for
	// the backup sizes DeployMonster produces.
	body, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("read upload body: %w", err)
	}
	if size > 0 && int64(len(body)) != size {
		// Be permissive — callers that pass size=0 use len(body);
		// mismatched sizes almost always mean the caller is wrong,
		// but S3 will reject it anyway, so let it through.
	}

	return s.retry(ctx, func() error {
		endpoint := s.objectURL(key)

		req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("create S3 request: %w", err)
		}
		req.ContentLength = int64(len(body))
		req.Header.Set("Content-Type", "application/octet-stream")
		s.sign(req, body)

		resp, err := s.client.Do(req)
		if err != nil {
			return fmt.Errorf("S3 upload: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			return fmt.Errorf("S3 upload failed: HTTP %d", resp.StatusCode)
		}

		if s.logger != nil {
			s.logger.Debug("S3 upload complete", "key", key, "size", len(body))
		}
		return nil
	})
}

// sign stamps the request with AWS SigV4 using the configured
// credentials. Called by every S3 request path — if credentials are
// empty we still call it so any pre-existing X-Amz-Date / Authorization
// headers are overwritten deterministically.
func (s *S3Storage) sign(req *http.Request, body []byte) {
	region := s.region
	if region == "" {
		region = "us-east-1"
	}
	awsauth.SignV4(req, body, s.accessKey, s.secretKey, region, "s3", time.Now())
}

func (s *S3Storage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	var result io.ReadCloser
	err := s.retry(ctx, func() error {
		url := s.objectURL(key)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		s.sign(req, nil)

		resp, err := s.client.Do(req)
		if err != nil {
			return fmt.Errorf("S3 download: %w", err)
		}
		if resp.StatusCode >= 400 {
			io.Copy(io.Discard, resp.Body)
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
		s.sign(req, nil)

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

func (s *S3Storage) List(ctx context.Context, prefix string) ([]core.BackupEntry, error) {
	var entries []core.BackupEntry

	err := s.retry(ctx, func() error {
		// Build ListObjects URL with query parameters
		var listURL string
		if prefix != "" {
			listURL = fmt.Sprintf("%s?prefix=%s&list-type=2", s.bucketURL(), url.QueryEscape(prefix))
		} else {
			listURL = fmt.Sprintf("%s?list-type=2", s.bucketURL())
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
		if err != nil {
			return fmt.Errorf("create list request: %w", err)
		}
		s.sign(req, nil)

		resp, err := s.client.Do(req)
		if err != nil {
			return fmt.Errorf("S3 list: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			return fmt.Errorf("S3 list failed: HTTP %d", resp.StatusCode)
		}

		// Parse S3 ListObjectsV2 XML response
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("read list response: %w", err)
		}

		var listResult s3ListResult
		if err := xml.Unmarshal(body, &listResult); err != nil {
			return fmt.Errorf("parse list response: %w", err)
		}

		for _, content := range listResult.Contents {
			// Parse timestamp
			modTime, _ := time.Parse(time.RFC3339, content.LastModified)

			entries = append(entries, core.BackupEntry{
				Key:       content.Key,
				Size:      content.Size,
				CreatedAt: modTime.Unix(),
			})
		}

		if s.logger != nil {
			s.logger.Debug("S3 list complete", "count", len(entries), "prefix", prefix)
		}
		return nil
	})

	return entries, err
}

// s3ListResult represents the S3 ListObjectsV2 XML response.
type s3ListResult struct {
	Name     string `xml:"Name"`
	Prefix   string `xml:"Prefix"`
	KeyCount int    `xml:"KeyCount"`
	Contents []struct {
		Key          string `xml:"Key"`
		LastModified string `xml:"LastModified"`
		Size         int64  `xml:"Size"`
		ETag         string `xml:"ETag"`
	} `xml:"Contents"`
}

// bucketURL returns the base URL for bucket operations (listing).
func (s *S3Storage) bucketURL() string {
	host := stripScheme(s.endpoint)
	scheme := "https"
	if strings.HasPrefix(s.endpoint, "http://") {
		scheme = "http"
	}
	if s.pathStyle {
		return fmt.Sprintf("%s://%s/%s", scheme, host, s.bucket)
	}
	return fmt.Sprintf("%s://%s.%s", scheme, s.bucket, host)
}

func (s *S3Storage) objectURL(key string) string {
	host := stripScheme(s.endpoint)
	scheme := "https"
	if strings.HasPrefix(s.endpoint, "http://") {
		scheme = "http"
	}
	if s.pathStyle {
		return fmt.Sprintf("%s://%s/%s/%s", scheme, host, s.bucket, key)
	}
	return fmt.Sprintf("%s://%s.%s/%s", scheme, s.bucket, host, key)
}
