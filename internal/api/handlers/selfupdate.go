package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

// SelfUpdateHandler checks for platform updates.
type SelfUpdateHandler struct {
	currentVersion string
}

func NewSelfUpdateHandler(version string) *SelfUpdateHandler {
	return &SelfUpdateHandler{currentVersion: version}
}

// CheckUpdate handles GET /api/v1/admin/updates
func (h *SelfUpdateHandler) CheckUpdate(w http.ResponseWriter, _ *http.Request) {
	latest, releaseURL, err := checkLatestRelease()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"current_version": h.currentVersion,
			"update_check":    "failed",
			"error":           err.Error(),
		})
		return
	}

	hasUpdate := latest != h.currentVersion && latest != ""

	writeJSON(w, http.StatusOK, map[string]any{
		"current_version": h.currentVersion,
		"latest_version":  latest,
		"update_available": hasUpdate,
		"release_url":     releaseURL,
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
