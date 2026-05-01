package api

import (
	"context"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

type testAuthServices struct {
	jwt *auth.JWTService
}

func newTestAuthServices(t testing.TB, secret string) *testAuthServices {
	t.Helper()
	jwt, err := auth.NewJWTService(secret)
	if err != nil {
		t.Fatalf("NewJWTService: %v", err)
	}
	return &testAuthServices{jwt: jwt}
}

func (s *testAuthServices) JWT() *auth.JWTService {
	return s.jwt
}

func (s *testAuthServices) TOTP() *auth.TOTPService {
	return nil
}

func (s *testAuthServices) ID() string                             { return "core.auth" }
func (s *testAuthServices) Name() string                           { return "Auth Test Services" }
func (s *testAuthServices) Version() string                        { return "test" }
func (s *testAuthServices) Dependencies() []string                 { return nil }
func (s *testAuthServices) Init(context.Context, *core.Core) error { return nil }
func (s *testAuthServices) Start(context.Context) error            { return nil }
func (s *testAuthServices) Stop(context.Context) error             { return nil }
func (s *testAuthServices) Health() core.HealthStatus              { return core.HealthOK }
func (s *testAuthServices) Routes() []core.Route                   { return nil }
func (s *testAuthServices) Events() []core.EventHandler            { return nil }
