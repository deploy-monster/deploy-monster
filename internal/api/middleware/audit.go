package middleware

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// AuditLog returns middleware that records state-changing requests to the audit log.
// Only logs POST, PUT, PATCH, DELETE — not GET requests.
func AuditLog(store core.Store, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only audit state-changing methods
			if r.Method == http.MethodGet || r.Method == http.MethodOptions || r.Method == http.MethodHead {
				next.ServeHTTP(w, r)
				return
			}

			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)

			// Only log successful operations
			if sw.status >= 400 {
				return
			}

			claims := auth.ClaimsFromContext(r.Context())
			if claims == nil {
				return
			}

			// Extract resource info from path
			resourceType, resourceID, action := parseAuditPath(r.Method, r.URL.Path)

			entry := &core.AuditEntry{
				TenantID:     claims.TenantID,
				UserID:       claims.UserID,
				Action:       action,
				ResourceType: resourceType,
				ResourceID:   resourceID,
				IPAddress:    realIP(r),
				UserAgent:    r.UserAgent(),
			}

			if err := store.CreateAuditLog(r.Context(), entry); err != nil {
				logger.Error("audit log failed", "error", err)
			}
		})
	}
}

// parseAuditPath extracts resource type, ID, and action from an API path.
// e.g., POST /api/v1/apps → (apps, "", create)
// e.g., DELETE /api/v1/apps/abc123 → (apps, abc123, delete)
func parseAuditPath(method, path string) (resourceType, resourceID, action string) {
	// Strip /api/v1/ prefix
	path = strings.TrimPrefix(path, "/api/v1/")
	parts := strings.Split(path, "/")

	if len(parts) >= 1 {
		resourceType = parts[0]
	}
	if len(parts) >= 2 {
		resourceID = parts[1]
	}

	switch method {
	case http.MethodPost:
		if len(parts) >= 3 {
			action = parts[2] // e.g., "restart", "stop"
		} else {
			action = "create"
		}
	case http.MethodPut, http.MethodPatch:
		action = "update"
	case http.MethodDelete:
		action = "delete"
	default:
		action = method
	}

	return
}
