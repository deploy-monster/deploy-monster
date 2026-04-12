package middleware

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// QuotaEnforcement checks tenant resource limits before allowing resource creation.
// Applied to POST endpoints that create apps, databases, domains, etc.
func QuotaEnforcement(store core.Store, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only check on resource creation
			if r.Method != http.MethodPost {
				next.ServeHTTP(w, r)
				return
			}

			// Only check specific creation endpoints
			path := r.URL.Path
			if !isQuotaPath(path) {
				next.ServeHTTP(w, r)
				return
			}

			claims := auth.ClaimsFromContext(r.Context())
			if claims == nil {
				next.ServeHTTP(w, r)
				return
			}

			// Check app count limit
			if strings.HasSuffix(path, "/apps") {
				_, total, _ := store.ListAppsByTenant(r.Context(), claims.TenantID, 1, 0)
				// Default limit: 100 (from free plan)
				if total >= 100 {
					logger.Warn("quota exceeded", "tenant", claims.TenantID, "resource", "apps", "count", total)
					writeErrorJSON(w, http.StatusForbidden, "app quota exceeded — upgrade your plan")
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

func isQuotaPath(path string) bool {
	quotaPaths := []string{"/api/v1/apps", "/api/v1/databases", "/api/v1/domains"}
	for _, qp := range quotaPaths {
		if strings.HasSuffix(path, qp) {
			return true
		}
	}
	return false
}
