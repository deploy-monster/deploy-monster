#!/usr/bin/env bash
set -euo pipefail

usage() {
    cat >&2 <<'USAGE'
Usage:
  scripts/transfer-bbolt-to-sqlite-kv.sh OLD_BOLT_FILE SQLITE_DB_FILE

Copies legacy BoltStore data from OLD_BOLT_FILE into the SQLite-backed KV
tables in SQLITE_DB_FILE. Run with deploymonster stopped.

This is a one-time transfer helper. It keeps go.etcd.io/bbolt out of the
DeployMonster module by building the reader in a temporary Go module.
USAGE
}

if [ "$#" -ne 2 ]; then
    usage
    exit 2
fi

SRC="$1"
DST="$2"

if [ ! -f "$SRC" ]; then
    echo "source file not found: $SRC" >&2
    exit 1
fi

if ! command -v go >/dev/null 2>&1; then
    echo "go is required for this one-time transfer helper" >&2
    exit 1
fi

WORKDIR="$(mktemp -d)"
cleanup() {
    rm -rf "$WORKDIR"
}
trap cleanup EXIT

cat > "$WORKDIR/go.mod" <<'EOF'
module deploymonster-kv-transfer

go 1.23

require (
	go.etcd.io/bbolt v1.4.3
	modernc.org/sqlite v1.50.0
)
EOF

cat > "$WORKDIR/main.go" <<'EOF'
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"
	_ "modernc.org/sqlite"
)

var defaultBuckets = []string{
	"sessions",
	"ratelimit",
	"buildcache",
	"metrics_ring",
	"cronjobs",
	"app_pins",
	"autoscale",
	"basic_auth",
	"api_keys",
	"deploy_freeze",
	"deploy_notify",
	"deploy_approval",
	"maintenance",
	"app_middleware",
	"container_metrics",
	"announcements",
	"certificates",
	"ssh_keys",
	"log_retention",
	"event_webhooks",
	"webhook_logs",
	"webhooks",
	"revoked_tokens",
	"vault",
	"git_provider_connections",
	"build_queue",
}

type legacyEntry struct {
	Data      json.RawMessage `json:"d"`
	ExpiresAt int64           `json:"e"`
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: transfer OLD_BOLT_FILE SQLITE_DB_FILE")
		os.Exit(2)
	}
	src := os.Args[1]
	dst := os.Args[2]

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		fatalf("create destination directory: %v", err)
	}

	bdb, err := bolt.Open(src, 0o400, &bolt.Options{
		ReadOnly: true,
		Timeout:  5 * time.Second,
	})
	if err != nil {
		fatalf("open source: %v", err)
	}
	defer bdb.Close()

	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on&_synchronous=NORMAL", dst)
	sdb, err := sql.Open("sqlite", dsn)
	if err != nil {
		fatalf("open destination: %v", err)
	}
	defer sdb.Close()
	sdb.SetMaxOpenConns(1)
	sdb.SetMaxIdleConns(1)

	if err := initSchema(sdb); err != nil {
		fatalf("init destination schema: %v", err)
	}

	tx, err := sdb.Begin()
	if err != nil {
		fatalf("begin destination transaction: %v", err)
	}
	defer tx.Rollback()

	for _, bucket := range defaultBuckets {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO kv_buckets(name) VALUES (?)`, bucket); err != nil {
			fatalf("create default bucket %s: %v", bucket, err)
		}
	}

	var buckets, records int
	err = bdb.View(func(btx *bolt.Tx) error {
		return btx.ForEach(func(name []byte, b *bolt.Bucket) error {
			bucket := string(name)
			buckets++
			if _, err := tx.Exec(`INSERT OR IGNORE INTO kv_buckets(name) VALUES (?)`, bucket); err != nil {
				return fmt.Errorf("create bucket %s: %w", bucket, err)
			}
			return b.ForEach(func(k, v []byte) error {
				if v == nil {
					return nil
				}
				data, expiresAt := unwrap(v)
				if _, err := tx.Exec(`
					INSERT INTO kv_store(bucket, key, data, expires_at)
					VALUES (?, ?, ?, ?)
					ON CONFLICT(bucket, key) DO UPDATE SET
						data = excluded.data,
						expires_at = excluded.expires_at
				`, bucket, string(k), data, expiresAt); err != nil {
					return fmt.Errorf("copy %s/%s: %w", bucket, string(k), err)
				}
				records++
				return nil
			})
		})
	})
	if err != nil {
		fatalf("copy source data: %v", err)
	}

	if err := tx.Commit(); err != nil {
		fatalf("commit destination transaction: %v", err)
	}

	fmt.Printf("transferred %d records across %d buckets into %s\n", records, buckets, dst)
}

func initSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS kv_store (
			bucket TEXT NOT NULL,
			key TEXT NOT NULL,
			data BLOB NOT NULL,
			expires_at INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (bucket, key)
		);
		CREATE INDEX IF NOT EXISTS idx_kv_store_bucket ON kv_store(bucket);
		CREATE TABLE IF NOT EXISTS kv_buckets (
			name TEXT PRIMARY KEY
		);
	`)
	return err
}

func unwrap(raw []byte) ([]byte, int64) {
	var entry legacyEntry
	if err := json.Unmarshal(raw, &entry); err == nil && len(entry.Data) > 0 {
		return []byte(entry.Data), entry.ExpiresAt
	}
	return raw, 0
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
EOF

(cd "$WORKDIR" && go mod tidy >/dev/null && go run . "$SRC" "$DST")
