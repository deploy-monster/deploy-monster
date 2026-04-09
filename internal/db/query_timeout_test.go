package db

import (
	"context"
	"testing"
	"time"
)

func TestSetQueryTimeout(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/timeout-test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if db.queryTimeout != 0 {
		t.Errorf("expected default timeout=0, got %v", db.queryTimeout)
	}

	db.SetQueryTimeout(3 * time.Second)
	if db.queryTimeout != 3*time.Second {
		t.Errorf("expected 3s, got %v", db.queryTimeout)
	}
}

func TestWithTimeout_AddsDeadline(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/timeout-ctx.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	db.SetQueryTimeout(2 * time.Second)

	ctx := context.Background()
	tctx, cancel := db.withTimeout(ctx)
	defer cancel()

	deadline, ok := tctx.Deadline()
	if !ok {
		t.Fatal("expected context to have deadline")
	}
	remaining := time.Until(deadline)
	if remaining < 1*time.Second || remaining > 3*time.Second {
		t.Errorf("deadline remaining %v not in expected range", remaining)
	}
}

func TestWithTimeout_NoTimeout_NoDeadline(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/timeout-none.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Default: no timeout
	ctx := context.Background()
	tctx, cancel := db.withTimeout(ctx)
	defer cancel()

	if _, ok := tctx.Deadline(); ok {
		t.Error("expected no deadline with zero timeout")
	}
}

func TestQueryTimeout_QueriesStillWork(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/timeout-query.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	db.SetQueryTimeout(5 * time.Second)

	// Simple query should work fine within timeout
	var count int
	row := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM applications")
	if err := row.Scan(&count); err != nil {
		t.Fatalf("expected query to work, got: %v", err)
	}
}
