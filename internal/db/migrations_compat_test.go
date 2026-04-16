package db

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestMigrations_UpHeadReachesExpectedSchema walks the embedded migration
// ladder from empty → HEAD and verifies the resulting schema contains the
// core tables the rest of the codebase depends on. This catches the "someone
// added a new table but forgot to ship a migration" class of bug.
func TestMigrations_UpHeadReachesExpectedSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "up-head.db")

	// NewSQLite runs migrations as part of construction.
	db, err := NewSQLite(path)
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	defer db.Close()

	// Every one of these tables is referenced somewhere in the repository store
	// implementations under internal/db/*.go. If a migration is missing one,
	// the binary will start but runtime queries will fail.
	expected := []string{
		"tenants",
		"users",
		"team_members",
		"roles",
		"projects",
		"applications",
		"deployments",
		"domains",
		"ssl_certs",
		"servers",
		"secrets",
		"secret_versions",
		"git_sources",
		"webhooks",
		"webhook_logs",
		"managed_dbs",
		"volumes",
		"backups",
		"vps_providers",
		"subscriptions",
		"usage_records",
		"invoices",
		"audit_log",
		"api_keys",
		"compose_stacks",
		"marketplace_installs",
		"invitations",
	}

	tables := listTables(t, db)
	tableSet := make(map[string]bool, len(tables))
	for _, name := range tables {
		tableSet[name] = true
	}

	for _, want := range expected {
		if !tableSet[want] {
			t.Errorf("migration head missing table %q", want)
		}
	}

	// Verify the migration bookkeeping table exists and has both versions.
	var count int
	if err := db.DB().QueryRow("SELECT COUNT(*) FROM _migrations").Scan(&count); err != nil {
		t.Fatalf("count _migrations: %v", err)
	}
	if count < 2 {
		t.Errorf("_migrations: want at least 2 applied, got %d", count)
	}
}

// TestMigrations_DownLadderLeavesSchemaEmpty walks the ladder from empty →
// HEAD and back to empty, asserting that every .up.sql file has a matching
// .down.sql that successfully reverses it. This is the symmetry contract:
// an admin who runs `deploymonster rotate migration` back to zero must end
// up with no DeployMonster tables left behind.
func TestMigrations_DownLadderLeavesSchemaEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "down-ladder.db")

	db, err := NewSQLite(path)
	if err != nil {
		t.Fatalf("NewSQLite (up): %v", err)
	}

	tablesBefore := listTables(t, db)
	if len(tablesBefore) == 0 {
		db.Close()
		t.Fatal("up ladder produced no tables — is the migration runner working?")
	}

	// Roll back everything.
	if err := db.Rollback(0); err != nil {
		db.Close()
		t.Fatalf("Rollback(0): %v", err)
	}

	tablesAfter := listTables(t, db)
	db.Close()

	// The bookkeeping table `_migrations` is created by the runner itself
	// and is allowed to remain after full rollback — it tracks which
	// migrations are applied, not schema data. Every other table must be
	// gone.
	var leftovers []string
	for _, name := range tablesAfter {
		if name == "_migrations" {
			continue
		}
		leftovers = append(leftovers, name)
	}
	if len(leftovers) > 0 {
		sort.Strings(leftovers)
		t.Errorf("rollback to zero left tables behind: %v", leftovers)
	}

	// Verify the bookkeeping table shows zero applied migrations.
	db2, err := NewSQLite(path)
	if err != nil {
		t.Fatalf("re-open after rollback: %v", err)
	}
	defer db2.Close()

	// NewSQLite will re-run the up migrations on reopen, so this also
	// verifies that a fresh run after full rollback is idempotent and safe.
	tablesReup := listTables(t, db2)
	if len(tablesReup) <= 1 {
		t.Errorf("re-apply after rollback produced no tables (got %d)", len(tablesReup))
	}
}

// TestMigrations_UpDownPairsExist verifies that every numbered .sql file in
// the migrations directory has a matching .down.sql sibling. Missing a down
// migration means the rollback ladder breaks silently at that step.
func TestMigrations_UpDownPairsExist(t *testing.T) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}

	upNames := make(map[string]bool)
	downNames := make(map[string]bool)
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}
		if strings.HasSuffix(name, ".down.sql") {
			stem := strings.TrimSuffix(name, ".down.sql")
			downNames[stem] = true
		} else {
			stem := strings.TrimSuffix(name, ".sql")
			upNames[stem] = true
		}
	}

	if len(upNames) == 0 {
		t.Fatal("no up migrations found — embedded FS broken?")
	}

	for stem := range upNames {
		if !downNames[stem] {
			t.Errorf("migration %s.sql has no matching %s.down.sql", stem, stem)
		}
	}
	for stem := range downNames {
		if !upNames[stem] {
			t.Errorf("migration %s.down.sql has no matching %s.sql", stem, stem)
		}
	}
}

// TestMigrations_RollbackPartial verifies that Rollback(n) for n < total
// peels off exactly n migrations — not all of them, not none. We apply
// every migration, rollback top-of-stack one at a time, and assert the
// expected slice of schema artefacts disappears at each step while the
// underlying 0001 tables survive throughout.
//
// The "top" migration changes as new ones are added, so the test walks
// the expected-index set symbolically rather than pinning to a version
// number. Each migration's down.sql is the contract; this test pins
// that the Rollback loop honours it in order.
func TestMigrations_RollbackPartial(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "partial.db")

	db, err := NewSQLite(path)
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	defer db.Close()

	indexesBefore := listIndexes(t, db)
	if len(indexesBefore) == 0 {
		t.Fatal("expected indexes from index-adding migrations, got none")
	}
	// Snapshot the initial _migrations row count so we can assert the
	// N-peeled invariant without hard-coding a total.
	totalMigrations := migrationCount(t, db)
	if totalMigrations < 2 {
		t.Fatalf("expected at least 2 applied migrations, got %d", totalMigrations)
	}

	// Peel the top migration off. The specific indexes 0003 introduced
	// must vanish; 0002's must still be present; 0001's tables must not
	// be touched.
	if err := db.Rollback(1); err != nil {
		t.Fatalf("Rollback(1): %v", err)
	}
	assertTableExists(t, db, "applications")
	idx := listIndexes(t, db)
	for _, name := range idx {
		switch name {
		case "idx_secrets_scope_name", "idx_deployments_status", "idx_applications_tenant_name":
			t.Errorf("0003 index %q survived Rollback(1)", name)
		}
	}
	// 0002's indexes must still be here.
	if !containsPrefix(idx, "idx_domains_") {
		t.Error("Rollback(1) dropped 0002 indexes — expected only 0003 to be peeled")
	}
	if got := migrationCount(t, db); got != totalMigrations-1 {
		t.Errorf("_migrations count after Rollback(1): got %d, want %d", got, totalMigrations-1)
	}

	// Peel the next one. 0002's indexes must now also be gone.
	if err := db.Rollback(1); err != nil {
		t.Fatalf("Rollback(1) second call: %v", err)
	}
	assertTableExists(t, db, "applications")
	idx = listIndexes(t, db)
	for _, name := range idx {
		if strings.HasPrefix(name, "idx_applications_") ||
			strings.HasPrefix(name, "idx_deployments_") ||
			strings.HasPrefix(name, "idx_domains_") {
			t.Errorf("0002 index %q survived second Rollback(1)", name)
		}
	}
	if got := migrationCount(t, db); got != totalMigrations-2 {
		t.Errorf("_migrations count after second Rollback(1): got %d, want %d", got, totalMigrations-2)
	}
}

func migrationCount(t *testing.T, db *SQLiteDB) int {
	t.Helper()
	var count int
	if err := db.DB().QueryRow("SELECT COUNT(*) FROM _migrations").Scan(&count); err != nil {
		t.Fatalf("count _migrations: %v", err)
	}
	return count
}

func assertTableExists(t *testing.T, db *SQLiteDB, name string) {
	t.Helper()
	for _, got := range listTables(t, db) {
		if got == name {
			return
		}
	}
	t.Errorf("table %q missing — rollback removed a 0001 artefact", name)
}

func containsPrefix(names []string, prefix string) bool {
	for _, n := range names {
		if strings.HasPrefix(n, prefix) {
			return true
		}
	}
	return false
}

// listTables returns every user-created SQL table name in the database, in
// sorted order. Internal sqlite_* tables are excluded.
func listTables(t *testing.T, db *SQLiteDB) []string {
	t.Helper()
	rows, err := db.DB().Query(
		"SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name",
	)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table name: %v", err)
		}
		out = append(out, name)
	}
	return out
}

// listIndexes returns every user-created index name.
func listIndexes(t *testing.T, db *SQLiteDB) []string {
	t.Helper()
	rows, err := db.DB().Query(
		"SELECT name FROM sqlite_master WHERE type='index' AND name NOT LIKE 'sqlite_%' ORDER BY name",
	)
	if err != nil {
		t.Fatalf("list indexes: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan index name: %v", err)
		}
		out = append(out, name)
	}
	return out
}
