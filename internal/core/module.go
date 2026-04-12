package core

import "context"

// HealthStatus represents the health state of a module.
type HealthStatus int

const (
	HealthOK       HealthStatus = iota // Module is fully operational
	HealthDegraded                     // Module is operational but impaired
	HealthDown                         // Module is not operational
)

// String returns a human-readable health status.
func (h HealthStatus) String() string {
	switch h {
	case HealthOK:
		return "ok"
	case HealthDegraded:
		return "degraded"
	case HealthDown:
		return "down"
	default:
		return "unknown"
	}
}

// Module is the contract every subsystem implements.
// Each feature of DeployMonster is a module that registers itself
// with the core engine and participates in the lifecycle.
type Module interface {
	// Identity
	ID() string
	Name() string
	Version() string
	Dependencies() []string // Module IDs this depends on

	// Lifecycle
	Init(ctx context.Context, core *Core) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error

	// Observability
	Health() HealthStatus

	// HTTP integration
	Routes() []Route
	Events() []EventHandler
}

// AuthLevel defines the required authentication level for a route.
type AuthLevel int

const (
	AuthNone       AuthLevel = iota // No authentication required
	AuthAPIKey                      // Valid API key required
	AuthJWT                         // Valid JWT required
	AuthAdmin                       // Admin role required
	AuthSuperAdmin                  // Super admin role required
)

// Route represents an HTTP endpoint a module registers.
type Route struct {
	Method  string
	Path    string
	Handler HandlerFunc
	Auth    AuthLevel
}

// HandlerFunc is the handler signature for module routes.
type HandlerFunc func(ctx *RequestContext) error

// RequestContext wraps an HTTP request with parsed authentication claims
// and tenant context for use by module handlers.
type RequestContext struct {
	UserID   string
	TenantID string
	RoleID   string
	Email    string
}
