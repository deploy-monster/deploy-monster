package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// SelfUpdateHandler checks for platform updates.
type SelfUpdateHandler struct {
	core *core.Core
}

func NewSelfUpdateHandler(c *core.Core) *SelfUpdateHandler {
	return &SelfUpdateHandler{core: c}
}

// CheckUpdate handles GET /api/v1/admin/updates
func (h *SelfUpdateHandler) CheckUpdate(w http.ResponseWriter, _ *http.Request) {
	currentVersion := h.core.Build.Version

	latest, releaseURL, err := checkLatestRelease()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"current_version": currentVersion,
			"commit":          h.core.Build.Commit,
			"build_date":      h.core.Build.Date,
			"update_check":    "failed",
			"error":           err.Error(),
		})
		return
	}

	hasUpdate := latest != currentVersion && latest != ""

	writeJSON(w, http.StatusOK, map[string]any{
		"current_version":  currentVersion,
		"latest_version":   latest,
		"update_available": hasUpdate,
		"release_url":      releaseURL,
		"commit":           h.core.Build.Commit,
		"build_date":       h.core.Build.Date,
	})
}

func checkLatestRelease() (version, url string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.github.com/repos/deploy-monster/deploy-monster/releases/latest", nil)
	if err != nil {
		return "", "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var release struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	json.Unmarshal(body, &release)
	return release.TagName, release.HTMLURL, nil
}
