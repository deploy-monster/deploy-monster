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
