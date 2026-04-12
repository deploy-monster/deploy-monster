package webhooks

import (
	"context"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// DeliveryLog records webhook delivery attempts for debugging and DLQ tracking.
type DeliveryLog struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	Status    string `json:"status"` // "sent" or "failed"
	Error     string `json:"error,omitempty"`
	Timestamp int64  `json:"timestamp"`
}

// DeliveryTracker subscribes to webhook events and persists delivery logs to BBolt.
type DeliveryTracker struct {
	bolt   core.BoltStorer
	events *core.EventBus
}

const deliveryLogBucket = "webhook_delivery_log"

// NewDeliveryTracker creates a tracker that logs webhook deliveries to BBolt.
func NewDeliveryTracker(bolt core.BoltStorer, events *core.EventBus) *DeliveryTracker {
	return &DeliveryTracker{bolt: bolt, events: events}
}

// Start subscribes to webhook delivery events.
func (t *DeliveryTracker) Start() {
	t.events.SubscribeAsync(core.EventOutboundSent, func(_ context.Context, e core.Event) error {
		data, ok := e.Data.(core.NotificationEventData)
		if !ok {
			return nil
		}
		return t.record(DeliveryLog{
			ID:        core.GenerateID(),
			URL:       data.Recipient,
			Status:    "sent",
			Timestamp: time.Now().Unix(),
		})
	})

	t.events.SubscribeAsync(core.EventOutboundFailed, func(_ context.Context, e core.Event) error {
		data, ok := e.Data.(core.NotificationEventData)
		if !ok {
			return nil
		}
		return t.record(DeliveryLog{
			ID:        core.GenerateID(),
			URL:       data.Recipient,
			Status:    "failed",
			Error:     data.Error,
			Timestamp: time.Now().Unix(),
		})
	})
}

func (t *DeliveryTracker) record(log DeliveryLog) error {
	return t.bolt.Set(deliveryLogBucket, log.ID, log, 0)
}
