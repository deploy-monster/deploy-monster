// Package apierr is the single source of truth for the platform's JSON error
// envelope. Both the HTTP handlers and the middleware chain emit errors through
// it, so the wire format and the status→code mapping cannot drift apart.
package apierr

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// codeMap maps HTTP status codes to machine-readable error codes returned in
// the JSON envelope. Statuses not listed fall back to "error".
var codeMap = map[int]string{
	http.StatusBadRequest:          "bad_request",
	http.StatusUnauthorized:        "unauthorized",
	http.StatusForbidden:           "forbidden",
	http.StatusNotFound:            "not_found",
	http.StatusConflict:            "conflict",
	http.StatusTooManyRequests:     "rate_limited",
	http.StatusInternalServerError: "internal_error",
	http.StatusServiceUnavailable:  "unavailable",
}

// Code returns the machine-readable error code for an HTTP status.
func Code(status int) string {
	if c, ok := codeMap[status]; ok {
		return c
	}
	return "error"
}

// Write emits the canonical structured error envelope:
//
//	{"success": false, "error": {"code": "...", "message": "..."}, "request_id": "..."}
//
// request_id is included when the X-Request-ID response header is set.
func Write(w http.ResponseWriter, status int, message string) {
	resp := map[string]any{
		"success": false,
		"error": map[string]string{
			"code":    Code(status),
			"message": message,
		},
	}
	if rid := w.Header().Get("X-Request-ID"); rid != "" {
		resp["request_id"] = rid
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Error("failed to write JSON error response", "error", err)
	}
}
