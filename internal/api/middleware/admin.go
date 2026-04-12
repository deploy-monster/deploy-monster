package middleware

import (
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/auth"
)

// Role identifiers. These match the seed values written by 0001_init.sql
// and the role_id stored in JWT claims.
const (
	RoleSuperAdmin = "role_super_admin"
	RoleOwner      = "role_owner"
	RoleAdmin      = "role_admin"
	RoleDeveloper  = "role_developer"
	RoleViewer     = "role_viewer"
	RoleBilling    = "role_billing"
)

// requireRole wraps next with a role-set check. Requests without claims
// return 401; requests whose claim role is not in allowed return 403.
// The response body is the standard writeErrorJSON envelope so callers
// always see {success:false, error:{code, message}}.
func requireRole(allowed map[string]struct{}) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := auth.ClaimsFromContext(r.Context())
			if claims == nil {
				writeErrorJSON(w, http.StatusUnauthorized, "authentication required")
				return
			}
			if _, ok := allowed[claims.RoleID]; !ok {
				writeErrorJSON(w, http.StatusForbidden, "insufficient privileges")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAdmin permits role_super_admin and role_admin. Use on endpoints
// that should be reachable by any administrator but not by developers,
// viewers, or billing users.
func RequireAdmin(next http.Handler) http.Handler {
	return requireRole(map[string]struct{}{
		RoleSuperAdmin: {},
		RoleAdmin:      {},
	})(next)
}

// RequireSuperAdmin permits only role_super_admin. Use on platform-level
// endpoints (tenant management, system config) that must never be
// reachable by a tenant-level admin.
func RequireSuperAdmin(next http.Handler) http.Handler {
	return requireRole(map[string]struct{}{
		RoleSuperAdmin: {},
	})(next)
}

// RequireOwnerOrAbove permits role_super_admin, role_owner, and
// role_admin. Use on tenant-owner actions such as provisioning servers
// or deleting projects — actions that are destructive enough to warrant
// more than a regular admin, but that a super admin should still be
// able to perform on behalf of a tenant.
func RequireOwnerOrAbove(next http.Handler) http.Handler {
	return requireRole(map[string]struct{}{
		RoleSuperAdmin: {},
		RoleOwner:      {},
		RoleAdmin:      {},
	})(next)
}
