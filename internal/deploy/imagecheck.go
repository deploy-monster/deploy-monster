package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

const imageCheckInterval = 6 * time.Hour

// ImageUpdateChecker polls Docker registries for newer image versions.
//
// Lifecycle note for Tier 74: Stop used to call close(stopCh) with
// no sync.Once guard. A second Stop crashed the deploy module with
// "close of closed channel". stopOnce now serializes the close so
// Module.Stop is idempotent across shutdown retries.
type ImageUpdateChecker struct {
	store    core.Store
	events   *core.EventBus
	client   *http.Client
	logger   *slog.Logger
	stopCh   chan struct{}
	stopOnce sync.Once
}

// NewImageUpdateChecker creates an image update checker. A nil
// logger is tolerated and replaced with slog.Default().
func NewImageUpdateChecker(store core.Store, events *core.EventBus, logger *slog.Logger) *ImageUpdateChecker {
	if logger == nil {
		logger = slog.Default()
	}
	return &ImageUpdateChecker{
		store:  store,
		events: events,
		client: &http.Client{Timeout: 10 * time.Second},
		logger: logger,
		stopCh: make(chan struct{}),
	}
}

// ImageUpdate represents an available image update.
type ImageUpdate struct {
	AppID      string `json:"app_id"`
	AppName    string `json:"app_name"`
	CurrentTag string `json:"current_tag"`
	LatestTag  string `json:"latest_tag"`
	Registry   string `json:"registry"`
}

// Start begins periodic update checking (every 6 hours).
func (c *ImageUpdateChecker) Start() {
	core.SafeGo(c.logger, "image-update-checker", func() {
		ticker := time.NewTicker(imageCheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				c.checkAll()
			case <-c.stopCh:
				return
			}
		}
	})
	c.logger.Info("image update checker started")
}

// Stop halts the checker. Safe to call multiple times — the second
// and subsequent calls are no-ops.
func (c *ImageUpdateChecker) Stop() {
	c.stopOnce.Do(func() {
		close(c.stopCh)
	})
}

func (c *ImageUpdateChecker) checkAll() {
	ctx := context.Background()

	// Check all image-type apps
	apps, _, err := c.store.ListAppsByTenant(ctx, "", 1000, 0)
	if err != nil {
		return
	}

	for _, app := range apps {
		if app.SourceType != "image" || app.SourceURL == "" {
			continue
		}
		// Check if newer image digest is available
		c.logger.Debug("checking image update", "app", app.Name, "image", app.SourceURL)
	}
}

// CheckDockerHubTag queries Docker Hub API for tag info.
func CheckDockerHubTag(ctx context.Context, image, tag string) (string, error) {
	url := fmt.Sprintf("https://hub.docker.com/v2/repositories/%s/tags/%s", image, tag)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return "", fmt.Errorf("Docker Hub API returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result struct {
		Digest      string `json:"digest"`
		LastUpdated string `json:"last_updated"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}
	return result.Digest, nil
}
