package db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/db/models"
	bolt "go.etcd.io/bbolt"
)

// Standard bucket names.
var (
	bucketSessions      = []byte("sessions")
	bucketRateLimit     = []byte("ratelimit")
	bucketBuildCache    = []byte("buildcache")
	bucketMetricsRing   = []byte("metrics_ring")
	bucketCronJobs      = []byte("cronjobs")
	bucketAppPins       = []byte("app_pins")
	bucketAutoscale     = []byte("autoscale")
	bucketBasicAuth     = []byte("basic_auth")
	bucketAPIKeys       = []byte("api_keys")
	bucketSchedule      = []byte("deploy_schedule")
	bucketFreeze        = []byte("deploy_freeze")
	bucketNotify        = []byte("deploy_notify")
	bucketApproval      = []byte("deploy_approval")
	bucketMaintenance   = []byte("maintenance")
	bucketMiddleware    = []byte("app_middleware")
	bucketMetrics       = []byte("container_metrics")
	bucketAnnouncements = []byte("announcements")
	bucketCertificates  = []byte("certificates")
	bucketSSHKeys       = []byte("ssh_keys")
	bucketLogRetention  = []byte("log_retention")
	bucketEventWebhooks = []byte("event_webhooks")
	bucketWebhookLogs   = []byte("webhook_logs")
	bucketWebhooks      = []byte("webhooks")
	bucketRevokedTokens = []byte("revoked_tokens")
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
			bucketAnnouncements, bucketCertificates, bucketSSHKeys,
			bucketLogRetention, bucketEventWebhooks, bucketWebhookLogs, bucketWebhooks,
			bucketRevokedTokens,
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

// BatchSet writes multiple key-value pairs in a single transaction.
// All items are committed atomically — if any write fails, the entire batch is rolled back.
func (b *BoltStore) BatchSet(items []core.BoltBatchItem) error {
	if len(items) == 0 {
		return nil
	}

	return b.db.Update(func(tx *bolt.Tx) error {
		for _, item := range items {
			bkt := tx.Bucket([]byte(item.Bucket))
			if bkt == nil {
				return fmt.Errorf("bucket %q not found", item.Bucket)
			}

			data, err := json.Marshal(item.Value)
			if err != nil {
				return fmt.Errorf("marshal value for %s/%s: %w", item.Bucket, item.Key, err)
			}

			entry := boltEntry{Data: data}
			if item.TTL > 0 {
				entry.ExpiresAt = time.Now().Unix() + item.TTL
			}

			raw, err := json.Marshal(entry)
			if err != nil {
				return fmt.Errorf("marshal entry for %s/%s: %w", item.Bucket, item.Key, err)
			}

			if err := bkt.Put([]byte(item.Key), raw); err != nil {
				return err
			}
		}
		return nil
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

// List returns all non-expired keys in the given bucket.
func (b *BoltStore) List(bucket string) ([]string, error) {
	var keys []string
	err := b.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket([]byte(bucket))
		if bkt == nil {
			return fmt.Errorf("bucket %q not found", bucket)
		}
		now := time.Now().Unix()
		return bkt.ForEach(func(k, v []byte) error {
			var entry boltEntry
			if err := json.Unmarshal(v, &entry); err != nil {
				return nil // skip corrupt entries
			}
			if entry.ExpiresAt > 0 && now >= entry.ExpiresAt {
				return nil // skip expired
			}
			keys = append(keys, string(k))
			return nil
		})
	})
	return keys, err
}

// Close closes the BBolt database.
func (b *BoltStore) Close() error {
	return b.db.Close()
}

// GetAPIKeyByPrefix retrieves an API key by its key prefix (first 8 chars).
// Used for API key validation in middleware.
func (b *BoltStore) GetAPIKeyByPrefix(ctx context.Context, prefix string) (*models.APIKey, error) {
	_ = ctx // context not used directly but kept for interface consistency
	var key models.APIKey
	err := b.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(bucketAPIKeys)
		if bkt == nil {
			return fmt.Errorf("bucket api_keys not found")
		}

		// Iterate to find key with matching prefix
		c := bkt.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var apiKey models.APIKey
			if err := json.Unmarshal(v, &apiKey); err != nil {
				continue
			}
			if apiKey.KeyPrefix == prefix {
				key = apiKey
				return nil
			}
		}
		return fmt.Errorf("api key not found")
	})
	if err != nil {
		return nil, err
	}
	return &key, nil
}

// GetWebhookSecret retrieves the webhook secret hash for signature verification.
// Returns the secret hash stored for the given webhook ID.
func (b *BoltStore) GetWebhookSecret(webhookID string) (string, error) {
	var secret string
	err := b.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(bucketWebhooks)
		if bkt == nil {
			return fmt.Errorf("webhooks bucket not found")
		}

		// Look up webhook by ID
		data := bkt.Get([]byte(webhookID))
		if data == nil {
			return fmt.Errorf("webhook not found")
		}

		var wh models.Webhook
		if err := json.Unmarshal(data, &wh); err != nil {
			return fmt.Errorf("unmarshal webhook: %w", err)
		}

		secret = wh.SecretHash
		return nil
	})
	if err != nil {
		return "", err
	}
	return secret, nil
}
