package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// requireTenantApp fetches an app by ID and verifies it belongs to the
// requesting user's tenant. Returns the app on success or writes an error
// response and returns nil on failure. Every handler that operates on
// an app by ID must use this instead of calling store.GetApp directly.
func requireTenantApp(w http.ResponseWriter, r *http.Request, store core.Store) *core.Application {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return nil
	}

	id := r.PathValue("id")
	app, err := store.GetApp(r.Context(), id)
	if err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "application not found")
		} else {
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return nil
	}

	if app.TenantID != claims.TenantID {
		writeError(w, http.StatusNotFound, "application not found")
		return nil
	}

	return app
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("failed to write JSON response", "error", err)
	}
}

var validAppName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9 ._-]{0,99}$`)

// validateAppName checks that an app name is safe and well-formed.
func validateAppName(name string) error {
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if len(name) > 100 {
		return fmt.Errorf("name must be 100 characters or fewer")
	}
	if !validAppName.MatchString(name) {
		return fmt.Errorf("name must start with a letter or digit and contain only letters, digits, spaces, dots, hyphens, or underscores")
	}
	return nil
}

// statusCodeMap maps HTTP status codes to machine-readable error codes.
var statusCodeMap = map[int]string{
	http.StatusBadRequest:          "bad_request",
	http.StatusUnauthorized:        "unauthorized",
	http.StatusForbidden:           "forbidden",
	http.StatusNotFound:            "not_found",
	http.StatusConflict:            "conflict",
	http.StatusTooManyRequests:     "rate_limited",
	http.StatusInternalServerError: "internal_error",
	http.StatusServiceUnavailable:  "unavailable",
}

func writeError(w http.ResponseWriter, status int, message string) {
	code := statusCodeMap[status]
	if code == "" {
		code = "error"
	}
	resp := map[string]any{
		"success": false,
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	}
	if rid := w.Header().Get("X-Request-ID"); rid != "" {
		resp["request_id"] = rid
	}
	writeJSON(w, status, resp)
}
