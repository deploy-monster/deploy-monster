package db

import (
	"context"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestSQLite_GetDomain(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	app := createTestApp(t, db, ctx)

	domain := &core.Domain{
		AppID:       app.ID,
		FQDN:        "test.example.com",
		Type:        "custom",
		DNSProvider: "manual",
	}
	if err := db.CreateDomain(ctx, domain); err != nil {
		t.Fatalf("CreateDomain: %v", err)
	}

	// Get by ID
	got, err := db.GetDomain(ctx, domain.ID)
	if err != nil {
		t.Fatalf("GetDomain: %v", err)
	}
	if got.ID != domain.ID {
		t.Errorf("id = %q, want %q", got.ID, domain.ID)
	}
	if got.FQDN != "test.example.com" {
		t.Errorf("fqdn = %q, want test.example.com", got.FQDN)
	}

	// Not found
	_, err = db.GetDomain(ctx, "nonexistent-id")
	if err != core.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
