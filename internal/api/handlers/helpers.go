package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"runtime/debug"
	"strconv"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// pagination holds parsed page and per_page query parameters.
type pagination struct {
	Page    int
	PerPage int
	Offset  int
}

// parsePagination extracts page and per_page from query params.
// Defaults: page=1, per_page=20. PerPage is capped at 100.
func parsePagination(r *http.Request) pagination {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	return pagination{
		Page:    page,
		PerPage: perPage,
		Offset:  (page - 1) * perPage,
	}
}

// writePaginatedJSON writes a standard paginated JSON response.
func writePaginatedJSON(w http.ResponseWriter, data any, total int, pg pagination) {
	totalPages := (total + pg.PerPage - 1) / pg.PerPage
	writeJSON(w, http.StatusOK, map[string]any{
		"data":        data,
		"total":       total,
		"page":        pg.Page,
		"per_page":    pg.PerPage,
		"total_pages": totalPages,
	})
}

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

// FieldError describes a single field validation failure.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// writeValidationErrors writes a 400 response with per-field error details.
// Use this when multiple fields may fail validation simultaneously, so the
// client can display all issues at once rather than one-at-a-time.
func writeValidationErrors(w http.ResponseWriter, message string, fields []FieldError) {
	resp := map[string]any{
		"success": false,
		"error": map[string]any{
			"code":    "validation_error",
			"message": message,
			"details": fields,
		},
	}
	if rid := w.Header().Get("X-Request-ID"); rid != "" {
		resp["request_id"] = rid
	}
	writeJSON(w, http.StatusBadRequest, resp)
}

// safeGo launches a goroutine with panic recovery. If the goroutine panics,
// it logs the error with stack trace and calls onPanic (if non-nil).
func safeGo(fn func(), onPanic func(recovered any)) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("goroutine panic recovered",
					"error", r,
					"stack", string(debug.Stack()),
				)
				if onPanic != nil {
					onPanic(r)
				}
			}
		}()
		fn()
	}()
}
