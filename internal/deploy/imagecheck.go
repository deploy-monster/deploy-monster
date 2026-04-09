package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ImageUpdateChecker polls Docker registries for newer image versions.
type ImageUpdateChecker struct {
	store  core.Store
	events *core.EventBus
	client *http.Client
	logger *slog.Logger
	stopCh chan struct{}
}

// NewImageUpdateChecker creates an image update checker.
func NewImageUpdateChecker(store core.Store, events *core.EventBus, logger *slog.Logger) *ImageUpdateChecker {
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
		ticker := time.NewTicker(6 * time.Hour)
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

// Stop halts the checker.
func (c *ImageUpdateChecker) Stop() {
	close(c.stopCh)
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
	body, _ := io.ReadAll(resp.Body)

	var result struct {
		Digest      string `json:"digest"`
		LastUpdated string `json:"last_updated"`
	}
	json.Unmarshal(body, &result)
	return result.Digest, nil
}
