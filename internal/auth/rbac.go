package auth

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/db/models"
)

// Permission constants.
const (
	PermAppView        = "app.view"
	PermAppCreate      = "app.create"
	PermAppDeploy      = "app.deploy"
	PermAppDelete      = "app.delete"
	PermAppRestart     = "app.restart"
	PermAppStop        = "app.stop"
	PermAppLogs        = "app.logs"
	PermAppMetrics     = "app.metrics"
	PermAppEnvEdit     = "app.env.edit"
	PermProjectView    = "project.view"
	PermProjectCreate  = "project.create"
	PermProjectDelete  = "project.delete"
	PermMemberView     = "member.view"
	PermMemberInvite   = "member.invite"
	PermMemberRemove   = "member.remove"
	PermMemberManage   = "member.manage"
	PermSecretView     = "secret.view"
	PermSecretCreate   = "secret.create"
	PermSecretDelete   = "secret.delete"
	PermServerView     = "server.view"
	PermServerManage   = "server.manage"
	PermDomainView     = "domain.view"
	PermDomainManage   = "domain.manage"
	PermBillingView    = "billing.view"
	PermBillingManage  = "billing.manage"
	PermDatabaseView   = "db.view"
	PermDatabaseManage = "db.manage"
	PermBackupView     = "backup.view"
	PermBackupManage   = "backup.manage"
	PermAdminAll       = "*"
)

// contextKey is a type-safe key for context values.
type contextKey string

const claimsKey contextKey = "claims"

// ClaimsFromContext extracts Claims from a context.
func ClaimsFromContext(ctx context.Context) *Claims {
	claims, _ := ctx.Value(claimsKey).(*Claims)
	return claims
}

// ContextWithClaims adds Claims to a context.
func ContextWithClaims(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, claimsKey, claims)
}

// roleCache caches loaded roles to avoid repeated DB queries.
type roleCache struct {
	mu    sync.RWMutex
	roles map[string]*cachedRole
}

type cachedRole struct {
	role      *models.Role
	expiresAt time.Time
}

var cache = &roleCache{
	roles: make(map[string]*cachedRole),
}

const roleCacheTTL = 5 * time.Minute

// SetRoleInCache stores a role in the in-memory cache.
func SetRoleInCache(role *models.Role) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.roles[role.ID] = &cachedRole{
		role:      role,
		expiresAt: time.Now().Add(roleCacheTTL),
	}
}

// GetRoleCached retrieves a role from cache, or nil if not found/expired.
func GetRoleCached(roleID string) *models.Role {
	cache.mu.RLock()
	defer cache.mu.RUnlock()
	cr, ok := cache.roles[roleID]
	if !ok || time.Now().After(cr.expiresAt) {
		return nil
	}
	return cr.role
}

// HasPermission checks if the given role has a specific permission.
// Supports wildcard matching: "app.*" matches "app.deploy".
func HasPermission(roleID string, permission string) bool {
	// Super admin has all permissions
	if roleID == "role_super_admin" {
		return true
	}

	role := GetRoleCached(roleID)
	if role == nil {
		return false
	}

	var perms []string
	json.Unmarshal([]byte(role.PermissionsJSON), &perms)

	for _, p := range perms {
		if p == "*" || p == permission {
			return true
		}
		// Wildcard matching: "app.*" matches "app.deploy"
		if strings.HasSuffix(p, ".*") {
			prefix := strings.TrimSuffix(p, ".*")
			if strings.HasPrefix(permission, prefix+".") {
				return true
			}
		}
	}
	return false
}
