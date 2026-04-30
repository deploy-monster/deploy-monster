package webhooks

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// InboundLog records an inbound webhook received by the receiver.
// Used for debugging, deduplication, and tracking processing status.
type InboundLog struct {
	ID          string `json:"id"`
	WebhookID   string `json:"webhook_id"`
	Event       string `json:"event_type"`   // push, tag, pull_request
	PayloadHash string `json:"payload_hash"` // SHA-256 of raw body for dedup
	CommitSHA   string `json:"commit_sha,omitempty"`
	Branch      string `json:"branch,omitempty"`
	Status      string `json:"status"` // received, processing, approval_pending, processed, failed, approved, rejected
	DeployID    string `json:"deployment_id,omitempty"`
	AppID       string `json:"app_id,omitempty"`
	Error       string `json:"error,omitempty"`
	ReceivedAt  int64  `json:"received_at"`
	ProcessedAt int64  `json:"processed_at,omitempty"`
}

// LogAndEmit receives a webhook, records it, and emits the event.
func (l *InboundLog) Save(bolt core.BoltStorer) error {
	return bolt.Set("webhook_logs", l.ID, l, 0)
}

// ComputePayloadHash computes SHA-256 hash of the webhook payload for deduplication.
func ComputePayloadHash(body []byte) string {
	h := sha256.Sum256(body)
	return hex.EncodeToString(h[:])
}

// NewInboundLog creates an inbound webhook log entry.
func NewInboundLog(webhookID, eventType string, body []byte) *InboundLog {
	return &InboundLog{
		ID:          core.GenerateID(),
		WebhookID:   webhookID,
		Event:       eventType,
		PayloadHash: ComputePayloadHash(body),
		Status:      "received",
		ReceivedAt:  time.Now().Unix(),
	}
}

// MarkProcessed updates the log as processed (successful deploy or error).
func (l *InboundLog) MarkProcessed(bolt core.BoltStorer, appID, deployID, errMsg string) error {
	l.AppID = appID
	l.DeployID = deployID
	l.ProcessedAt = time.Now().Unix()
	if errMsg != "" {
		l.Status = "failed"
		l.Error = errMsg
	} else {
		l.Status = "processed"
	}
	return bolt.Set("webhook_logs", l.ID, l, 0)
}

// ListLogs retrieves all inbound webhook logs for a webhook ID.
func ListLogs(bolt core.BoltStorer, webhookID string) ([]InboundLog, error) {
	var logs []InboundLog
	keys, err := bolt.List("webhook_logs")
	if err != nil {
		return nil, err
	}
	for _, key := range keys {
		var log InboundLog
		if err := bolt.Get("webhook_logs", key, &log); err == nil {
			if log.WebhookID == webhookID {
				logs = append(logs, log)
			}
		}
	}
	return logs, nil
}

// GetLatestLog gets the most recent log for a webhook with matching payload hash.
// Used for deduplication — if same payload was already processed, skip it.
func GetLatestLog(bolt core.BoltStorer, webhookID, payloadHash string) (*InboundLog, error) {
	logs, err := ListLogs(bolt, webhookID)
	if err != nil {
		return nil, err
	}
	var latest *InboundLog
	for i := range logs {
		if logs[i].PayloadHash == payloadHash {
			if latest == nil || logs[i].ReceivedAt > latest.ReceivedAt {
				latest = &logs[i]
			}
		}
	}
	return latest, nil
}

// MarshalJSON serializes the log to JSON.
func (l *InboundLog) MarshalJSON() ([]byte, error) {
	return json.Marshal(l)
}

// UpdateLogStatus updates an existing webhook log entry by ID.
func UpdateLogStatus(bolt core.BoltStorer, logID, status, appID, deployID, errMsg string) error {
	var log InboundLog
	if err := bolt.Get("webhook_logs", logID, &log); err != nil {
		return err
	}
	log.Status = status
	if appID != "" {
		log.AppID = appID
	}
	if deployID != "" {
		log.DeployID = deployID
	}
	if errMsg != "" {
		log.Error = errMsg
	}
	log.ProcessedAt = time.Now().Unix()
	return bolt.Set("webhook_logs", logID, log, 0)
}

// MarkLogProcessing marks a webhook log as processing (started deploy).
func MarkLogProcessing(bolt core.BoltStorer, logID string) error {
	var log InboundLog
	if err := bolt.Get("webhook_logs", logID, &log); err != nil {
		return err
	}
	log.Status = "processing"
	log.ProcessedAt = time.Now().Unix()
	return bolt.Set("webhook_logs", logID, log, 0)
}

// FormatTimestamp converts Unix timestamp to ISO8601 string.
func FormatTimestamp(ts int64) string {
	if ts == 0 {
		return ""
	}
	return time.Unix(ts, 0).UTC().Format("2006-01-02T15:04:05Z")
}