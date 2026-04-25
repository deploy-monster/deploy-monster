package auth

import "context"

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
	PermDomainView     = "domain.view"
	PermDomainManage   = "domain.manage"
	PermBillingView    = "billing.view"
	PermBillingManage  = "billing.manage"
	PermDatabaseView   = "db.view"
	PermDatabaseManage = "db.manage"
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

// Role levels define the built-in hierarchy. Higher number = more powerful.
const (
	LevelViewer      = 1
	LevelOperator    = 2
	LevelDeveloper   = 3
	LevelAdmin       = 4
	LevelOwner       = 5
	LevelSuperAdmin  = 6
)

// RoleLevel returns the numeric level for a built-in role ID.
// Custom roles default to LevelDeveloper (middle of the range).
func RoleLevel(roleID string) int {
	switch roleID {
	case "role_super_admin":
		return LevelSuperAdmin
	case "role_owner":
		return LevelOwner
	case "role_admin":
		return LevelAdmin
	case "role_developer":
		return LevelDeveloper
	case "role_operator":
		return LevelOperator
	case "role_viewer":
		return LevelViewer
	default:
		return LevelDeveloper
	}
}

// CanAssignRole returns true if an inviter with inviterRoleID may invite
// someone to targetRoleID. Users can only assign roles at or below their own
// level (prevents privilege escalation via invites).
func CanAssignRole(inviterRoleID, targetRoleID string) bool {
	return RoleLevel(targetRoleID) <= RoleLevel(inviterRoleID)
}
