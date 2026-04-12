//go:build pgintegration
// +build pgintegration

// End-to-end CRUD smoke test against a real PostgreSQL instance.
//
// Gated behind the `pgintegration` build tag so it does not run during
// the default `go test ./...` pass (which has no database). The GitHub
// Actions `test-integration` job starts a `postgres:16` service
// container and runs:
//
//	go test -tags pgintegration -run TestPostgresIntegration ./internal/db/...
//
// with TEST_POSTGRES_DSN pointed at the service. If the env var is
// unset the test simply skips — that makes it safe to run the tag
// locally on a machine without Postgres.
//
// Almost everything in the flow lives in store_contract_test.go and is
// shared with TestSQLiteIntegration. Only the connection-pool check is
// Postgres-specific.

package db

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestPostgresIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set; skipping real-Postgres integration test")
	}

	pg, err := NewPostgres(dsn)
	if err != nil {
		t.Fatalf("NewPostgres: %v", err)
	}
	t.Cleanup(func() { _ = pg.Close() })

	pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer pingCancel()
	if err := pg.Ping(pingCtx); err != nil {
		t.Fatalf("Ping: %v", err)
	}

	runStoreContract(t, pg, storeContractOpts{
		backend:     "postgres",
		rawDB:       pg.DB(),
		placeholder: "$1",
	})

	// ---- Postgres-specific: connection pool validation ----------------
	//
	// NewPostgres sets MaxOpenConns=25, MaxIdleConns=5. Confirm those
	// stuck through driver initialization — if the pool config is
	// dropped we lose concurrency headroom silently. SQLite uses a
	// single-writer pool (MaxOpenConns=1), so this assertion is
	// Postgres-only and does not belong in the shared contract.
	stats := pg.DB().Stats()
	if stats.MaxOpenConnections != 25 {
		t.Errorf("pool MaxOpenConnections: got %d, want 25", stats.MaxOpenConnections)
	}
}
