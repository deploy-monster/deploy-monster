package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// EnvCompareHandler compares environment variables between apps or projects.
type EnvCompareHandler struct {
	store core.Store
}

func NewEnvCompareHandler(store core.Store) *EnvCompareHandler {
	return &EnvCompareHandler{store: store}
}

// EnvDiff represents a difference between two env sets.
type EnvDiff struct {
	Key    string `json:"key"`
	Left   string `json:"left,omitempty"`
	Right  string `json:"right,omitempty"`
	Status string `json:"status"` // added, removed, changed, same
}

// Compare handles POST /api/v1/apps/env/compare
func (h *EnvCompareHandler) Compare(w http.ResponseWriter, r *http.Request) {
	var req struct {
		LeftAppID  string `json:"left_app_id"`
		RightAppID string `json:"right_app_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	leftApp, err := h.store.GetApp(r.Context(), req.LeftAppID)
	if err != nil || leftApp.TenantID != claims.TenantID {
		writeError(w, http.StatusNotFound, "left app not found")
		return
	}
	rightApp, err := h.store.GetApp(r.Context(), req.RightAppID)
	if err != nil || rightApp.TenantID != claims.TenantID {
		writeError(w, http.StatusNotFound, "right app not found")
		return
	}

	leftVars := parseEnvJSON(leftApp.EnvVarsEnc)
	rightVars := parseEnvJSON(rightApp.EnvVarsEnc)

	var diffs []EnvDiff
	seen := make(map[string]bool)

	for k, v := range leftVars {
		seen[k] = true
		rv, exists := rightVars[k]
		if !exists {
			diffs = append(diffs, EnvDiff{Key: k, Left: maskShort(v), Status: "removed"})
		} else if v != rv {
			diffs = append(diffs, EnvDiff{Key: k, Left: maskShort(v), Right: maskShort(rv), Status: "changed"})
		}
	}
	for k, v := range rightVars {
		if !seen[k] {
			diffs = append(diffs, EnvDiff{Key: k, Right: maskShort(v), Status: "added"})
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"left":  req.LeftAppID,
		"right": req.RightAppID,
		"diffs": diffs,
		"total": len(diffs),
	})
}

func parseEnvJSON(enc string) map[string]string {
	result := make(map[string]string)
	if enc == "" {
		return result
	}
	var vars []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	json.Unmarshal([]byte(enc), &vars)
	for _, v := range vars {
		result[v.Key] = v.Value
	}
	return result
}

func maskShort(v string) string {
	if len(v) <= 4 {
		return "****"
	}
	return v[:2] + "**" + v[len(v)-2:]
}
