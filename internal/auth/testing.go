package auth

import (
	"log/slog"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// NewTestModule creates a minimal auth Module suitable for handler integration
// tests. Only the JWT() method is functional — Init/Start/Stop are no-ops.
func NewTestModule(jwtSecret string, store core.Store) *Module {
	return &Module{
		jwt:    MustNewJWTService(jwtSecret),
		store:  store,
		logger: slog.Default(),
	}
}
