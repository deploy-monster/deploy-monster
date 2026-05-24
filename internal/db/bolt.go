package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/db/models"
)

// BoltStore is the legacy name for DeployMonster's KV store. It is backed by
// SQLite. The name is kept to avoid churn in the large handler surface that
// consumes core.BoltStorer.
type BoltStore struct {
	db      *sql.DB
	closeDB bool
}

var defaultKVBuckets = []string{
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

// NewSQLiteKVStore opens a standalone SQLite-backed KV store.
func NewSQLiteKVStore(path string) (*BoltStore, error) {
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on&_synchronous=NORMAL", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite kv: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &BoltStore{db: db, closeDB: true}
	if err := store.initSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

// NewSQLiteKVStoreFromDB uses an existing SQLite connection for KV storage.
func NewSQLiteKVStoreFromDB(db *sql.DB) (*BoltStore, error) {
	store := &BoltStore{db: db}
	if err := store.initSchema(); err != nil {
		return nil, err
	}
	return store, nil
}

// NewBoltStore is retained as a compatibility constructor. It returns a
// SQLite-backed KV store.
func NewBoltStore(path string) (*BoltStore, error) {
	return NewSQLiteKVStore(path)
}

type kvEntry struct {
	Data      json.RawMessage `json:"d"`
	ExpiresAt int64           `json:"e"`
}

func (b *BoltStore) initSchema() error {
	if b == nil || b.db == nil {
		return fmt.Errorf("sqlite kv: nil database")
	}
	_, err := b.db.Exec(`
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
	if err != nil {
		return fmt.Errorf("create sqlite kv schema: %w", err)
	}
	for _, bucket := range defaultKVBuckets {
		if _, err := b.db.Exec(`INSERT OR IGNORE INTO kv_buckets(name) VALUES (?)`, bucket); err != nil {
			return fmt.Errorf("create default sqlite kv bucket %s: %w", bucket, err)
		}
	}
	if _, err := b.db.Exec(`INSERT OR IGNORE INTO kv_buckets(name) SELECT DISTINCT bucket FROM kv_store`); err != nil {
		return fmt.Errorf("backfill sqlite kv buckets: %w", err)
	}
	return nil
}

// Set stores a value in the given bucket with an optional TTL in seconds.
func (b *BoltStore) Set(bucket, key string, value any, ttlSeconds int64) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal value: %w", err)
	}

	expiresAt := int64(0)
	if ttlSeconds > 0 {
		expiresAt = time.Now().Unix() + ttlSeconds
	}

	tx, err := b.db.Begin()
	if err != nil {
		return fmt.Errorf("begin sqlite kv set: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`INSERT OR IGNORE INTO kv_buckets(name) VALUES (?)`, bucket); err != nil {
		return fmt.Errorf("create sqlite kv bucket %s: %w", bucket, err)
	}
	_, err = tx.Exec(`
		INSERT INTO kv_store(bucket, key, data, expires_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(bucket, key) DO UPDATE SET
			data = excluded.data,
			expires_at = excluded.expires_at
	`, bucket, key, data, expiresAt)
	if err != nil {
		return fmt.Errorf("sqlite kv set %s/%s: %w", bucket, key, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sqlite kv set: %w", err)
	}
	return nil
}

// BatchSet writes multiple key-value pairs in one SQLite transaction.
func (b *BoltStore) BatchSet(items []core.BoltBatchItem) error {
	if len(items) == 0 {
		return nil
	}

	tx, err := b.db.Begin()
	if err != nil {
		return fmt.Errorf("begin sqlite kv batch: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	bucketStmt, err := tx.Prepare(`INSERT OR IGNORE INTO kv_buckets(name) VALUES (?)`)
	if err != nil {
		return fmt.Errorf("prepare sqlite kv bucket batch: %w", err)
	}
	defer bucketStmt.Close()

	stmt, err := tx.Prepare(`
		INSERT INTO kv_store(bucket, key, data, expires_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(bucket, key) DO UPDATE SET
			data = excluded.data,
			expires_at = excluded.expires_at
	`)
	if err != nil {
		return fmt.Errorf("prepare sqlite kv batch: %w", err)
	}
	defer stmt.Close()

	now := time.Now().Unix()
	for _, item := range items {
		data, err := json.Marshal(item.Value)
		if err != nil {
			return fmt.Errorf("marshal value for %s/%s: %w", item.Bucket, item.Key, err)
		}
		expiresAt := int64(0)
		if item.TTL > 0 {
			expiresAt = now + item.TTL
		}
		if _, err := bucketStmt.Exec(item.Bucket); err != nil {
			return fmt.Errorf("create sqlite kv bucket %s: %w", item.Bucket, err)
		}
		if _, err := stmt.Exec(item.Bucket, item.Key, data, expiresAt); err != nil {
			return fmt.Errorf("put %s/%s: %w", item.Bucket, item.Key, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sqlite kv batch: %w", err)
	}
	return nil
}

// Mutate loads, modifies, and writes a single key inside one SQLite transaction.
func (b *BoltStore) Mutate(bucket, key string, dest any, ttlSeconds int64, mutate func(exists bool) error) error {
	if mutate == nil {
		return fmt.Errorf("mutate callback is required")
	}

	tx, err := b.db.Begin()
	if err != nil {
		return fmt.Errorf("begin sqlite kv mutate: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	exists := false
	row := tx.QueryRow(`SELECT data, expires_at FROM kv_store WHERE bucket = ? AND key = ?`, bucket, key)
	var data []byte
	var expiresAt int64
	switch err := row.Scan(&data, &expiresAt); {
	case err == nil:
		if expiresAt == 0 || time.Now().Unix() < expiresAt {
			if err := json.Unmarshal(data, dest); err != nil {
				return fmt.Errorf("unmarshal value: %w", err)
			}
			exists = true
		}
	case err == sql.ErrNoRows:
	default:
		return fmt.Errorf("read sqlite kv %s/%s: %w", bucket, key, err)
	}

	if err := mutate(exists); err != nil {
		return err
	}

	data, err = json.Marshal(dest)
	if err != nil {
		return fmt.Errorf("marshal value: %w", err)
	}
	expiresAt = 0
	if ttlSeconds > 0 {
		expiresAt = time.Now().Unix() + ttlSeconds
	}
	if _, err := tx.Exec(`INSERT OR IGNORE INTO kv_buckets(name) VALUES (?)`, bucket); err != nil {
		return fmt.Errorf("create sqlite kv bucket %s: %w", bucket, err)
	}
	if _, err := tx.Exec(`
		INSERT INTO kv_store(bucket, key, data, expires_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(bucket, key) DO UPDATE SET
			data = excluded.data,
			expires_at = excluded.expires_at
	`, bucket, key, data, expiresAt); err != nil {
		return fmt.Errorf("write sqlite kv %s/%s: %w", bucket, key, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sqlite kv mutate: %w", err)
	}
	return nil
}

// Get retrieves a value from the given bucket and unmarshals it into dest.
func (b *BoltStore) Get(bucket, key string, dest any) error {
	var data []byte
	var expiresAt int64
	err := b.db.QueryRow(`SELECT data, expires_at FROM kv_store WHERE bucket = ? AND key = ?`, bucket, key).Scan(&data, &expiresAt)
	if err == sql.ErrNoRows {
		return fmt.Errorf("key %q: %w", key, core.ErrBoltNotFound)
	}
	if err != nil {
		return fmt.Errorf("sqlite kv get %s/%s: %w", bucket, key, err)
	}
	if expiresAt > 0 && time.Now().Unix() >= expiresAt {
		return fmt.Errorf("key %q: %w", key, core.ErrBoltNotFound)
	}
	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("unmarshal value: %w", err)
	}
	return nil
}

// Delete removes a key from the given bucket.
func (b *BoltStore) Delete(bucket, key string) error {
	exists, err := b.bucketExists(bucket)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("bucket %q: %w", bucket, core.ErrBoltNotFound)
	}
	if _, err := b.db.Exec(`DELETE FROM kv_store WHERE bucket = ? AND key = ?`, bucket, key); err != nil {
		return fmt.Errorf("sqlite kv delete %s/%s: %w", bucket, key, err)
	}
	return nil
}

// List returns all non-expired keys in the given bucket.
func (b *BoltStore) List(bucket string) ([]string, error) {
	rows, err := b.db.Query(`SELECT key, data FROM kv_store WHERE bucket = ? AND (expires_at = 0 OR expires_at > ?)`, bucket, time.Now().Unix())
	if err != nil {
		return nil, fmt.Errorf("sqlite kv list %s: %w", bucket, err)
	}
	defer rows.Close()

	keys := make([]string, 0)
	for rows.Next() {
		var key string
		var data []byte
		if err := rows.Scan(&key, &data); err != nil {
			return nil, fmt.Errorf("scan sqlite kv key: %w", err)
		}
		if !json.Valid(data) {
			continue
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sqlite kv keys: %w", err)
	}
	if len(keys) == 0 {
		exists, err := b.bucketExists(bucket)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, fmt.Errorf("bucket %q: %w", bucket, core.ErrBoltNotFound)
		}
	}
	return keys, nil
}

func (b *BoltStore) bucketExists(bucket string) (bool, error) {
	var one int
	err := b.db.QueryRow(`SELECT 1 FROM kv_buckets WHERE name = ?`, bucket).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("sqlite kv bucket lookup %s: %w", bucket, err)
	}
	return true, nil
}

// Close closes the standalone SQLite KV database. Stores created from the main
// SQLite connection are closed by SQLiteDB.
func (b *BoltStore) Close() error {
	if b == nil || b.db == nil || !b.closeDB {
		return nil
	}
	return b.db.Close()
}

// GetAPIKeyByPrefix retrieves an API key by its stored key prefix.
func (b *BoltStore) GetAPIKeyByPrefix(ctx context.Context, prefix string) (*models.APIKey, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	rows, err := b.db.QueryContext(ctx, `SELECT data FROM kv_store WHERE bucket = ?`, "api_keys")
	if err != nil {
		return nil, fmt.Errorf("query api keys: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan api key: %w", err)
		}
		apiKey, err := decodeAPIKeyRecord(raw)
		if err != nil {
			continue
		}
		if apiKey.KeyPrefix == prefix {
			return apiKey, nil
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate api keys: %w", err)
	}
	return nil, fmt.Errorf("api key not found")
}

type apiKeyKVRecord struct {
	ID        string     `json:"id"`
	UserID    string     `json:"user_id"`
	TenantID  string     `json:"tenant_id"`
	Name      string     `json:"name"`
	KeyHash   string     `json:"key_hash"`
	KeyPrefix string     `json:"key_prefix"`
	Scopes    string     `json:"scopes_json"`
	Prefix    string     `json:"prefix"`
	Hash      string     `json:"hash"`
	CreatedBy string     `json:"created_by"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

func decodeAPIKeyRecord(raw []byte) (*models.APIKey, error) {
	var legacy kvEntry
	if err := json.Unmarshal(raw, &legacy); err == nil && len(legacy.Data) > 0 {
		raw = legacy.Data
	}

	var rec apiKeyKVRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		return nil, err
	}

	keyHash := rec.KeyHash
	if keyHash == "" {
		keyHash = rec.Hash
	}
	keyPrefix := rec.KeyPrefix
	if keyPrefix == "" {
		keyPrefix = rec.Prefix
	}
	userID := rec.UserID
	if userID == "" {
		userID = rec.CreatedBy
	}
	if keyPrefix == "" || userID == "" {
		return nil, fmt.Errorf("invalid api key record")
	}

	id := rec.ID
	if id == "" {
		id = keyPrefix
	}
	name := rec.Name
	if name == "" {
		name = keyPrefix
	}

	return &models.APIKey{
		ID:         id,
		UserID:     userID,
		TenantID:   rec.TenantID,
		Name:       name,
		KeyHash:    keyHash,
		KeyPrefix:  keyPrefix,
		ScopesJSON: rec.Scopes,
		ExpiresAt:  rec.ExpiresAt,
		CreatedAt:  rec.CreatedAt,
	}, nil
}

// GetWebhookSecret retrieves the webhook secret hash for signature verification.
func (b *BoltStore) GetWebhookSecret(webhookID string) (string, error) {
	var raw []byte
	err := b.db.QueryRow(`SELECT data FROM kv_store WHERE bucket = ? AND key = ?`, "webhooks", webhookID).Scan(&raw)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("webhook secret not found")
	}
	if err != nil {
		return "", fmt.Errorf("query webhook secret: %w", err)
	}

	var legacy kvEntry
	if err := json.Unmarshal(raw, &legacy); err == nil && len(legacy.Data) > 0 {
		raw = legacy.Data
	}

	var rec struct {
		SecretHash string `json:"secret_hash"`
	}
	if err := json.Unmarshal(raw, &rec); err != nil {
		return "", fmt.Errorf("unmarshal webhook: %w", err)
	}
	if rec.SecretHash == "" {
		return "", fmt.Errorf("webhook secret not found")
	}
	return rec.SecretHash, nil
}
