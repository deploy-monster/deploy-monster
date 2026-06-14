package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"runtime/debug"
	"strconv"
	"sync"

	"github.com/deploy-monster/deploy-monster/internal/api/apierr"
	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// maxBodySize is the maximum request body size accepted by decodeJSONInto (1 MB).
const maxBodySize = 1 << 20 // 1 MB

func decodeJSONInto(w http.ResponseWriter, r *http.Request, target any) bool {
	body := io.LimitReader(r.Body, maxBodySize+1)
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

func decodeJSONAllowUnknownInto(w http.ResponseWriter, r *http.Request, target any) bool {
	body := io.LimitReader(r.Body, maxBodySize+1)
	dec := json.NewDecoder(body)
	if err := dec.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

func decodeOptionalJSONInto(w http.ResponseWriter, r *http.Request, target any) bool {
	body := io.LimitReader(r.Body, maxBodySize+1)
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		if errors.Is(err, io.EOF) {
			return true
		}
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

// pagination holds parsed page and per_page query parameters.
type pagination struct {
	Page    int
	PerPage int
	Offset  int
}

// parsePagination extracts page and per_page from query params.
// Defaults: page=1, per_page=20. PerPage is capped at 100.
const maxPage = 10000 // cap page to prevent integer overflow in offset calc

func parsePagination(r *http.Request) pagination {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if page < 1 {
		page = 1
	}
	if page > maxPage {
		page = maxPage
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

// paginateSlice applies in-memory pagination to a slice, returning the page and total.
func paginateSlice[T any](items []T, pg pagination) ([]T, int) {
	total := len(items)
	start := pg.Offset
	if start > total {
		start = total
	}
	end := start + pg.PerPage
	if end > total {
		end = total
	}
	return items[start:end], total
}

func shortResourceID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
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
	if id == "" {
		writeError(w, http.StatusBadRequest, "application id is required")
		return nil
	}
	app, err := store.GetApp(r.Context(), id)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
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

// writeError emits the canonical JSON error envelope. It delegates to the
// shared apierr package so the handler and middleware layers cannot drift on
// the wire format or the status→code mapping.
func writeError(w http.ResponseWriter, status int, message string) {
	apierr.Write(w, status, message)
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

// internalError logs the full error details and returns a sanitized message to the client.
// Use this instead of writeError(w, 500, "..."+err.Error()) to avoid leaking internal details.
// ctxLogger returns an slog.Logger enriched with request_id from context.
// Use this in handlers instead of bare slog calls.
func ctxLogger(ctx context.Context) *slog.Logger {
	l := slog.Default()
	if rid := core.CorrelationIDFromContext(ctx); rid != "" {
		l = l.With("request_id", rid)
	}
	return l
}

func internalError(w http.ResponseWriter, userMsg string, err error) {
	slog.Error(userMsg, "error", err)
	writeError(w, http.StatusInternalServerError, userMsg)
}

// internalErrorCtx logs with request_id context and returns a sanitized error.
func internalErrorCtx(ctx context.Context, w http.ResponseWriter, userMsg string, err error) {
	ctxLogger(ctx).Error(userMsg, "error", err)
	writeError(w, http.StatusInternalServerError, userMsg)
}

func publishEvent(ctx context.Context, events *core.EventBus, event core.Event) {
	if events != nil {
		_ = events.Publish(ctx, event)
	}
}

func publishEventAsync(ctx context.Context, events *core.EventBus, event core.Event) {
	if events != nil {
		events.PublishAsync(ctx, event)
	}
}

func eventBusStats(events *core.EventBus) core.EventBusStats {
	if events == nil {
		return core.EventBusStats{}
	}
	return events.Stats()
}

// backgroundWG tracks goroutines launched via safeGo.
// Call WaitForBackground during graceful shutdown to wait for them.
var backgroundWG sync.WaitGroup

// WaitForBackground blocks until all safeGo goroutines have completed.
// Should be called during graceful shutdown.
func WaitForBackground() {
	backgroundWG.Wait()
}

// safeFilename strips any character that is not alphanumeric, dot, hyphen, or
// underscore from a filename. This prevents header injection via
// Content-Disposition and path-traversal via crafted names.
var safeFilenameRe = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

func safeFilename(name string) string {
	return safeFilenameRe.ReplaceAllString(name, "_")
}

// requirePathParam extracts a path parameter and writes a 400 error if it is
// empty. Returns the value and true on success, or ("", false) on failure.
func requirePathParam(w http.ResponseWriter, r *http.Request, name string) (string, bool) {
	v := r.PathValue(name)
	if v == "" {
		writeError(w, http.StatusBadRequest, name+" is required")
		return "", false
	}
	return v, true
}

// safeGo launches a goroutine with panic recovery. If the goroutine panics,
// it logs the error with stack trace and calls onPanic (if non-nil).
// The goroutine is tracked by backgroundWG for graceful shutdown.
func safeGo(fn func(), onPanic func(recovered any)) {
	backgroundWG.Add(1)
	go func() {
		defer backgroundWG.Done()
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
