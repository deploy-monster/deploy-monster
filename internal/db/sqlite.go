package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"strings"

	"github.com/deploy-monster/deploy-monster/internal/core"
	_ "modernc.org/sqlite"
)

// Compile-time check: SQLiteDB must implement core.Store.
var _ core.Store = (*SQLiteDB)(nil)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// SQLiteDB wraps a sql.DB connection to SQLite with migrations and helpers.
type SQLiteDB struct {
	db *sql.DB
}

// NewSQLite opens a SQLite database with WAL mode and performance pragmas.
func NewSQLite(path string) (*SQLiteDB, error) {
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on&_synchronous=NORMAL", path)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// Performance pragmas
	pragmas := []string{
		"PRAGMA cache_size = -64000",   // 64MB cache
		"PRAGMA mmap_size = 268435456", // 256MB mmap
		"PRAGMA temp_store = MEMORY",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return nil, fmt.Errorf("pragma: %w", err)
		}
	}

	// SQLite single-writer connection pool
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(2)

	s := &SQLiteDB{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migration: %w", err)
	}

	return s, nil
}

// DB returns the underlying *sql.DB.
func (s *SQLiteDB) DB() *sql.DB {
	return s.db
}

// Ping verifies the database connection.
func (s *SQLiteDB) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// Close closes the database connection.
func (s *SQLiteDB) Close() error {
	return s.db.Close()
}

// Tx runs a function within a database transaction.
// The transaction is automatically rolled back on error, committed on success.
func (s *SQLiteDB) Tx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// migrate applies all pending SQL migrations from the embedded filesystem.
func (s *SQLiteDB) migrate() error {
	// Create migrations tracking table
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS _migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("create _migrations table: %w", err)
	}

	// Read migration files
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		name := entry.Name()
		// Skip down migrations and non-SQL files
		if !strings.HasSuffix(name, ".sql") || strings.HasSuffix(name, ".down.sql") {
			continue
		}

		// Extract version number from filename: 0001_init.sql -> 1
		var version int
		if _, err := fmt.Sscanf(name, "%04d", &version); err != nil {
			continue
		}

		// Check if already applied
		var count int
		s.db.QueryRow("SELECT COUNT(*) FROM _migrations WHERE version = ?", version).Scan(&count)
		if count > 0 {
			continue
		}

		// Apply migration
		data, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		if _, err := s.db.Exec(string(data)); err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}

		s.db.Exec("INSERT INTO _migrations (version, name) VALUES (?, ?)", version, name)
	}

	return nil
}

// Rollback reverts the last n applied migrations by executing their .down.sql counterparts.
// If steps <= 0, it rolls back all migrations.
func (s *SQLiteDB) Rollback(steps int) error {
	// Get applied migrations in reverse order
	rows, err := s.db.Query("SELECT version, name FROM _migrations ORDER BY version DESC")
	if err != nil {
		return fmt.Errorf("list applied migrations: %w", err)
	}
	defer rows.Close()

	type migration struct {
		version int
		name    string
	}
	var applied []migration
	for rows.Next() {
		var m migration
		if err := rows.Scan(&m.version, &m.name); err != nil {
			return fmt.Errorf("scan migration: %w", err)
		}
		applied = append(applied, m)
	}

	if len(applied) == 0 {
		return nil
	}

	if steps <= 0 || steps > len(applied) {
		steps = len(applied)
	}

	for i := 0; i < steps; i++ {
		m := applied[i]
		// Derive down filename: 0001_init.sql -> 0001_init.down.sql
		downName := strings.TrimSuffix(m.name, ".sql") + ".down.sql"

		data, err := migrationsFS.ReadFile("migrations/" + downName)
		if err != nil {
			return fmt.Errorf("down migration %s not found: %w", downName, err)
		}

		if _, err := s.db.Exec(string(data)); err != nil {
			return fmt.Errorf("rollback migration %s: %w", downName, err)
		}

		if _, err := s.db.Exec("DELETE FROM _migrations WHERE version = ?", m.version); err != nil {
			return fmt.Errorf("remove migration record %d: %w", m.version, err)
		}
	}

	return nil
}
