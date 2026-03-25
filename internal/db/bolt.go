package db

import (
	"encoding/json"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"
)

// Standard bucket names.
var (
	bucketSessions    = []byte("sessions")
	bucketRateLimit   = []byte("ratelimit")
	bucketBuildCache  = []byte("buildcache")
	bucketMetricsRing = []byte("metrics_ring")
	bucketCronJobs    = []byte("cronjobs")
	bucketAppPins     = []byte("app_pins")
	bucketAutoscale   = []byte("autoscale")
	bucketBasicAuth   = []byte("basic_auth")
	bucketAPIKeys     = []byte("api_keys")
	bucketSchedule    = []byte("deploy_schedule")
	bucketFreeze      = []byte("deploy_freeze")
	bucketNotify      = []byte("deploy_notify")
	bucketApproval    = []byte("deploy_approval")
	bucketMaintenance = []byte("maintenance")
	bucketMiddleware  = []byte("app_middleware")
	bucketMetrics     = []byte("container_metrics")
)

// BoltStore wraps a BBolt database for key-value operations with TTL support.
type BoltStore struct {
	db *bolt.DB
}

// NewBoltStore opens a BBolt database and creates the default buckets.
func NewBoltStore(path string) (*BoltStore, error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open bbolt: %w", err)
	}

	// Create default buckets
	err = db.Update(func(tx *bolt.Tx) error {
		for _, b := range [][]byte{
		bucketSessions, bucketRateLimit, bucketBuildCache, bucketMetricsRing,
		bucketCronJobs, bucketAppPins, bucketAutoscale, bucketBasicAuth,
		bucketAPIKeys, bucketSchedule, bucketFreeze, bucketNotify,
		bucketApproval, bucketMaintenance, bucketMiddleware, bucketMetrics,
	} {
			if _, err := tx.CreateBucketIfNotExists(b); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("create buckets: %w", err)
	}

	return &BoltStore{db: db}, nil
}

// boltEntry wraps data with an optional expiration timestamp.
type boltEntry struct {
	Data      json.RawMessage `json:"d"`
	ExpiresAt int64           `json:"e"` // Unix timestamp, 0 = no expiry
}

// Set stores a value in the given bucket with an optional TTL (in seconds, 0 = no expiry).
func (b *BoltStore) Set(bucket, key string, value any, ttlSeconds int64) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal value: %w", err)
	}

	entry := boltEntry{Data: data}
	if ttlSeconds > 0 {
		entry.ExpiresAt = time.Now().Unix() + ttlSeconds
	}

	raw, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal entry: %w", err)
	}

	return b.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket([]byte(bucket))
		if bkt == nil {
			return fmt.Errorf("bucket %q not found", bucket)
		}
		return bkt.Put([]byte(key), raw)
	})
}

// Get retrieves a value from the given bucket and unmarshals it into dest.
// Returns an error if the key is not found or has expired.
func (b *BoltStore) Get(bucket, key string, dest any) error {
	return b.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket([]byte(bucket))
		if bkt == nil {
			return fmt.Errorf("bucket %q not found", bucket)
		}

		raw := bkt.Get([]byte(key))
		if raw == nil {
			return fmt.Errorf("key not found")
		}

		var entry boltEntry
		if err := json.Unmarshal(raw, &entry); err != nil {
			return fmt.Errorf("unmarshal entry: %w", err)
		}

		if entry.ExpiresAt > 0 && time.Now().Unix() >= entry.ExpiresAt {
			return fmt.Errorf("expired")
		}

		return json.Unmarshal(entry.Data, dest)
	})
}

// Delete removes a key from the given bucket.
func (b *BoltStore) Delete(bucket, key string) error {
	return b.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket([]byte(bucket))
		if bkt == nil {
			return fmt.Errorf("bucket %q not found", bucket)
		}
		return bkt.Delete([]byte(key))
	})
}

// Close closes the BBolt database.
func (b *BoltStore) Close() error {
	return b.db.Close()
}
