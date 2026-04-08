package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
)

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

func writeError(w http.ResponseWriter, status int, message string) {
	resp := map[string]string{"error": message}
	if rid := w.Header().Get("X-Request-ID"); rid != "" {
		resp["request_id"] = rid
	}
	writeJSON(w, status, resp)
}
