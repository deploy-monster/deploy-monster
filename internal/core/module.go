package core

import (
	"context"
	"net/http"
)

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

// Route describes a single HTTP route exposed by a module.
// Deprecated: no module ever provides custom HTTP routes via the Module
// interface — routes are registered directly on the API router in each
// module's Init. This type is retained for compatibility with existing
// module definitions that embed Routes() returning []Route(nil).
type Route struct {
	Method  string
	Path    string
	Handler http.Handler
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
}
