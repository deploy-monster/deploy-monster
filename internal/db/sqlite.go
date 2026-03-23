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
		tx.Rollback()
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
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		// Extract version number from filename: 0001_init.sql -> 1
		var version int
		fmt.Sscanf(entry.Name(), "%04d", &version)

		// Check if already applied
		var count int
		s.db.QueryRow("SELECT COUNT(*) FROM _migrations WHERE version = ?", version).Scan(&count)
		if count > 0 {
			continue
		}

		// Apply migration
		data, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}

		if _, err := s.db.Exec(string(data)); err != nil {
			return fmt.Errorf("apply migration %s: %w", entry.Name(), err)
		}

		s.db.Exec("INSERT INTO _migrations (version, name) VALUES (?, ?)", version, entry.Name())
	}

	return nil
}
