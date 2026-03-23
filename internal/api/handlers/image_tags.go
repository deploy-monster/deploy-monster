package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ImageTagHandler lists available tags for Docker images.
type ImageTagHandler struct{}

func NewImageTagHandler() *ImageTagHandler {
	return &ImageTagHandler{}
}

// TagInfo represents a Docker image tag.
type TagInfo struct {
	Name        string `json:"name"`
	Digest      string `json:"digest,omitempty"`
	Size        int64  `json:"size,omitempty"`
	LastUpdated string `json:"last_updated,omitempty"`
}

// List handles GET /api/v1/images/{image}/tags
// Queries Docker Hub (or configured registry) for available tags.
func (h *ImageTagHandler) List(w http.ResponseWriter, r *http.Request) {
	image := r.URL.Query().Get("image")
	if image == "" {
		writeError(w, http.StatusBadRequest, "image query param required")
		return
	}

	tags, err := fetchDockerHubTags(r.Context(), image)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch tags: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"image": image,
		"tags":  tags,
		"total": len(tags),
	})
}

func fetchDockerHubTags(ctx context.Context, image string) ([]TagInfo, error) {
	url := fmt.Sprintf("https://hub.docker.com/v2/repositories/%s/tags?page_size=25", image)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var result struct {
		Results []struct {
			Name        string `json:"name"`
			Digest      string `json:"digest"`
			FullSize    int64  `json:"full_size"`
			LastUpdated string `json:"last_updated"`
		} `json:"results"`
	}
	json.Unmarshal(body, &result)

	tags := make([]TagInfo, len(result.Results))
	for i, t := range result.Results {
		tags[i] = TagInfo{
			Name:        t.Name,
			Digest:      t.Digest,
			Size:        t.FullSize,
			LastUpdated: t.LastUpdated,
		}
	}
	return tags, nil
}
