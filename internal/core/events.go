package core

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"
)

// Event represents something that happened in the system.
// Events are the primary communication channel between modules.
type Event struct {
	ID            string    // Unique event ID for tracing
	Type          string    // Dot-namespaced event type (e.g., "app.deployed")
	Source        string    // Module ID that emitted this event
	Timestamp     time.Time // When the event was created
	TenantID      string    // Tenant context (empty for system events)
	UserID        string    // User who triggered the action (empty for system events)
	CorrelationID string    // Links related events across modules (e.g., request ID)
	Data          any       // Event-specific payload
}

// EventHandler binds an event type to a named handler function.
type EventHandler struct {
	EventType string
	Name      string // Handler name for logging/debugging
	Handler   func(ctx context.Context, event Event) error
}

// Subscription represents a registered event subscription.
type Subscription struct {
	ID        string
	EventType string // Event type pattern (exact or "*" for all)
	Name      string // Subscriber name for debugging
	Async     bool   // If true, handler runs in a separate goroutine
	handler   func(ctx context.Context, event Event) error
}

// DefaultAsyncWorkers is the default max concurrent async event handlers.
const DefaultAsyncWorkers = 64

// EventBus is the central event system for inter-module communication.
// It supports synchronous and asynchronous handlers, wildcard subscriptions,
// prefix matching, error callbacks, and event history for debugging.
// Async handlers are bounded by a semaphore to prevent goroutine explosion.
type EventBus struct {
	mu            sync.RWMutex
	subscriptions []*Subscription
	logger        *slog.Logger
	onError       func(event Event, sub *Subscription, err error)
	asyncSem      chan struct{}  // semaphore bounding concurrent async handlers
	asyncWG       sync.WaitGroup // tracks in-flight async handlers for graceful drain

	// Metrics
	publishCount int64
	errorCount   int64
}

// NewEventBus creates a new EventBus with a bounded async worker pool.
func NewEventBus(logger *slog.Logger) *EventBus {
	if logger == nil {
		logger = slog.Default()
	}
	return &EventBus{
		logger:   logger,
		asyncSem: make(chan struct{}, DefaultAsyncWorkers),
		onError: func(event Event, sub *Subscription, err error) {
			logger.Error("event handler failed",
				"event", event.Type,
				"handler", sub.Name,
				"error", err,
			)
		},
	}
}

// Subscribe registers a synchronous handler for the given event type.
// Use "*" to subscribe to all events.
// Use "app.*" to subscribe to all events starting with "app.".
func (eb *EventBus) Subscribe(eventType string, handler func(ctx context.Context, event Event) error) {
	eb.SubscribeNamed(eventType, "", false, handler)
}

// SubscribeAsync registers an asynchronous handler that runs in its own goroutine.
// Errors from async handlers are logged but do not propagate to the publisher.
func (eb *EventBus) SubscribeAsync(eventType string, handler func(ctx context.Context, event Event) error) {
	eb.SubscribeNamed(eventType, "", true, handler)
}

// SubscribeNamed registers a named handler with explicit sync/async mode.
func (eb *EventBus) SubscribeNamed(eventType, name string, async bool, handler func(ctx context.Context, event Event) error) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	sub := &Subscription{
		ID:        GenerateID(),
		EventType: eventType,
		Name:      name,
		Async:     async,
		handler:   handler,
	}
	eb.subscriptions = append(eb.subscriptions, sub)
}

// OnError sets a custom error callback for handler failures.
func (eb *EventBus) OnError(fn func(event Event, sub *Subscription, err error)) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	eb.onError = fn
}

// Publish emits an event to all matching subscribers.
// Synchronous handlers run in order; async handlers are dispatched to goroutines.
// Returns the first error from synchronous handlers (async errors are logged only).
func (eb *EventBus) Publish(ctx context.Context, event Event) error {
	eb.mu.RLock()
	subs := eb.matchSubscriptions(event.Type)
	eb.mu.RUnlock()

	// Set defaults
	if event.ID == "" {
		event.ID = GenerateID()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	eb.mu.Lock()
	eb.publishCount++
	eb.mu.Unlock()

	for _, sub := range subs {
		if sub.Async {
			// Fire and forget with bounded concurrency — errors logged via onError
			eb.asyncSem <- struct{}{} // acquire semaphore slot
			eb.asyncWG.Add(1)
			go func(s *Subscription) {
				defer eb.asyncWG.Done()
				defer func() { <-eb.asyncSem }() // release slot
				defer func() {
					if r := recover(); r != nil {
						eb.mu.Lock()
						eb.errorCount++
						eb.mu.Unlock()
						eb.logger.Error("async event handler panicked",
							"event", event.Type, "subscriber", s.ID,
							"panic", r, "stack", string(debug.Stack()))
					}
				}()
				if err := s.handler(ctx, event); err != nil {
					eb.mu.Lock()
					eb.errorCount++
					eb.mu.Unlock()
					if eb.onError != nil {
						eb.onError(event, s, err)
					}
				}
			}(sub)
		} else {
			if err := sub.handler(ctx, event); err != nil {
				eb.mu.Lock()
				eb.errorCount++
				eb.mu.Unlock()
				if eb.onError != nil {
					eb.onError(event, sub, err)
				}
				return err
			}
		}
	}
	return nil
}

// PublishAsync emits an event asynchronously. All handlers run in goroutines.
// Useful when the publisher doesn't care about handler results.
func (eb *EventBus) PublishAsync(ctx context.Context, event Event) {
	go func() {
		if err := eb.Publish(ctx, event); err != nil {
			eb.mu.RLock()
			logger := eb.logger
			eb.mu.RUnlock()
			if logger != nil {
				logger.Error("async publish failed", "error", err, "event", event.Type)
			}
		}
	}()
}

// matchSubscriptions returns all subscriptions matching the given event type.
// Supports exact match, wildcard "*", and prefix match "app.*".
func (eb *EventBus) matchSubscriptions(eventType string) []*Subscription {
	var matched []*Subscription
	for _, sub := range eb.subscriptions {
		if sub.EventType == "*" {
			matched = append(matched, sub)
			continue
		}
		if sub.EventType == eventType {
			matched = append(matched, sub)
			continue
		}
		// Prefix matching: "app.*" matches "app.created", "app.deployed", etc.
		if prefix, ok := cutSuffix(sub.EventType, ".*"); ok {
			if len(eventType) > len(prefix) && eventType[:len(prefix)] == prefix && eventType[len(prefix)] == '.' {
				matched = append(matched, sub)
			}
		}
	}
	return matched
}

// Stats returns event bus metrics.
func (eb *EventBus) Stats() EventBusStats {
	eb.mu.RLock()
	defer eb.mu.RUnlock()
	return EventBusStats{
		SubscriptionCount: len(eb.subscriptions),
		PublishCount:      eb.publishCount,
		ErrorCount:        eb.errorCount,
		AsyncPoolSize:     cap(eb.asyncSem),
		AsyncPoolActive:   len(eb.asyncSem),
	}
}

// Drain waits for all in-flight async handlers to complete.
// Call this during graceful shutdown to ensure no events are lost.
func (eb *EventBus) Drain() {
	eb.asyncWG.Wait()
}

// EventBusStats holds event bus metrics.
type EventBusStats struct {
	SubscriptionCount int   `json:"subscription_count"`
	PublishCount      int64 `json:"publish_count"`
	ErrorCount        int64 `json:"error_count"`
	AsyncPoolSize     int   `json:"async_pool_size"`
	AsyncPoolActive   int   `json:"async_pool_active"`
}

func cutSuffix(s, suffix string) (string, bool) {
	if len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix {
		return s[:len(s)-len(suffix)], true
	}
	return s, false
}

// Emit is a convenience method that builds and publishes an event.
func (eb *EventBus) Emit(ctx context.Context, eventType, source string, data any) error {
	return eb.Publish(ctx, Event{
		Type:   eventType,
		Source: source,
		Data:   data,
	})
}

// EmitWithTenant is like Emit but includes tenant/user context.
func (eb *EventBus) EmitWithTenant(ctx context.Context, eventType, source, tenantID, userID string, data any) error {
	return eb.Publish(ctx, Event{
		Type:     eventType,
		Source:   source,
		TenantID: tenantID,
		UserID:   userID,
		Data:     data,
	})
}

// Standard event type constants.
const (
	// Application lifecycle
	EventAppCreated  = "app.created"
	EventAppUpdated  = "app.updated"
	EventAppDeployed = "app.deployed"
	EventAppStopped  = "app.stopped"
	EventAppStarted  = "app.started"
	EventAppDeleted  = "app.deleted"
	EventAppCrashed  = "app.crashed"
	EventAppScaled   = "app.scaled"

	// Build pipeline
	EventBuildQueued    = "build.queued"
	EventBuildStarted   = "build.started"
	EventBuildCompleted = "build.completed"
	EventBuildFailed    = "build.failed"

	// Domain & SSL
	EventDomainAdded    = "domain.added"
	EventDomainRemoved  = "domain.removed"
	EventDomainVerified = "domain.verified"
	EventSSLIssued      = "ssl.issued"
	EventSSLExpiring    = "ssl.expiring"
	EventSSLRenewed     = "ssl.renewed"
	EventSSLFailed      = "ssl.failed"

	// Container
	EventContainerStarted = "container.started"
	EventContainerStopped = "container.stopped"
	EventContainerDied    = "container.died"
	EventContainerHealthy = "container.healthy"

	// Server / Infrastructure
	EventServerAdded     = "server.added"
	EventServerRemoved   = "server.removed"
	EventServerDown      = "server.down"
	EventServerRecovered = "server.recovered"

	// Webhook (inbound)
	EventWebhookReceived  = "webhook.received"
	EventWebhookProcessed = "webhook.processed"
	EventWebhookFailed    = "webhook.failed"

	// Webhook (outbound)
	EventOutboundSent   = "outbound.sent"
	EventOutboundFailed = "outbound.failed"

	// Backup
	EventBackupStarted   = "backup.started"
	EventBackupCompleted = "backup.completed"
	EventBackupFailed    = "backup.failed"

	// Alert
	EventAlertTriggered = "alert.triggered"
	EventAlertResolved  = "alert.resolved"

	// User / Auth
	EventUserCreated   = "user.created"
	EventUserLoggedIn  = "user.logged_in"
	EventUserLoggedOut = "user.logged_out"
	EventUserInvited   = "user.invited"

	// Tenant
	EventTenantCreated   = "tenant.created"
	EventTenantUpdated   = "tenant.updated"
	EventTenantSuspended = "tenant.suspended"

	// Secret
	EventSecretCreated = "secret.created"
	EventSecretUpdated = "secret.updated"
	EventSecretRotated = "secret.rotated"
	EventSecretDeleted = "secret.deleted"

	// Billing
	EventQuotaExceeded    = "quota.exceeded"
	EventQuotaWarning     = "quota.warning"
	EventInvoiceGenerated = "invoice.generated"
	EventPaymentReceived  = "payment.received"
	EventPaymentFailed    = "payment.failed"

	// Database
	EventDatabaseCreated = "database.created"
	EventDatabaseDeleted = "database.deleted"
	EventDatabaseBackup  = "database.backup"

	// Deployment
	EventDeployStarted  = "deploy.started"
	EventDeployFinished = "deploy.finished"
	EventDeployFailed   = "deploy.failed"
	EventRollbackDone   = "deploy.rollback"

	// Notification (sent by notification module)
	EventNotificationSent   = "notification.sent"
	EventNotificationFailed = "notification.failed"

	// Project
	EventProjectCreated = "project.created"
	EventProjectDeleted = "project.deleted"

	// CronJob
	EventCronJobCreated = "cronjob.created"
	EventCronJobDeleted = "cronjob.deleted"

	// DNS Record
	EventDNSRecordDeleted = "dns_record.deleted"

	// Event Webhook
	EventEventWebhookDeleted = "event_webhook.deleted"

	// Redirect
	EventRedirectCreated = "redirect.created"
	EventRedirectDeleted = "redirect.deleted"

	// Service Mesh
	EventServiceMeshDeleted = "service_mesh.deleted"

	// Config changes (security/operational)
	EventAutoscaleUpdated = "autoscale.updated"
	EventBasicAuthUpdated = "basicauth.updated"
	EventGPUConfigUpdated = "gpu.updated"

	// System
	EventSystemStarted       = "system.started"
	EventSystemStopping      = "system.stopping"
	EventConfigReloaded      = "system.config_reloaded"
	EventModuleHealthChanged = "module.health_changed"
)

// EventData types provide typed payloads for common events.
// Using these instead of map[string]any gives compile-time safety.

// AppEventData is the payload for app lifecycle events.
type AppEventData struct {
	AppID     string `json:"app_id"`
	AppName   string `json:"app_name"`
	ProjectID string `json:"project_id"`
	TenantID  string `json:"tenant_id"`
	Status    string `json:"status,omitempty"`
	Image     string `json:"image,omitempty"`
}

// DeployEventData is the payload for deployment events.
type DeployEventData struct {
	AppID        string `json:"app_id"`
	DeploymentID string `json:"deployment_id"`
	Version      int    `json:"version"`
	Image        string `json:"image"`
	ContainerID  string `json:"container_id,omitempty"`
	Strategy     string `json:"strategy"`
	CommitSHA    string `json:"commit_sha,omitempty"`
	Error        string `json:"error,omitempty"`
}

// BuildEventData is the payload for build events.
type BuildEventData struct {
	AppID     string        `json:"app_id"`
	BuildID   string        `json:"build_id"`
	CommitSHA string        `json:"commit_sha,omitempty"`
	Duration  time.Duration `json:"duration,omitempty"`
	Error     string        `json:"error,omitempty"`
}

// DomainEventData is the payload for domain events.
type DomainEventData struct {
	DomainID string `json:"domain_id"`
	FQDN     string `json:"fqdn"`
	AppID    string `json:"app_id"`
	SSLState string `json:"ssl_state,omitempty"`
}

// ServerEventData is the payload for server events.
type ServerEventData struct {
	ServerID string `json:"server_id"`
	Hostname string `json:"hostname"`
	IP       string `json:"ip"`
	Role     string `json:"role,omitempty"`
	Error    string `json:"error,omitempty"`
}

// AlertEventData is the payload for alert events.
type AlertEventData struct {
	Name     string            `json:"name"`
	Severity string            `json:"severity"` // info, warning, critical
	Message  string            `json:"message"`
	Resource string            `json:"resource"`
	Labels   map[string]string `json:"labels,omitempty"`
}

// WebhookEventData is the payload for inbound webhook events.
type WebhookEventData struct {
	WebhookID string `json:"webhook_id"`
	Provider  string `json:"provider"`
	EventType string `json:"event_type"`
	Branch    string `json:"branch,omitempty"`
	CommitSHA string `json:"commit_sha,omitempty"`
}

// NotificationEventData is the payload for notification events.
type NotificationEventData struct {
	Channel   string `json:"channel"` // email, slack, discord, telegram, webhook
	Recipient string `json:"recipient"`
	Subject   string `json:"subject"`
	Error     string `json:"error,omitempty"`
}

// UserEventData is the payload for user events.
type UserEventData struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Action string `json:"action"`
}

type correlationKeyType struct{}

var correlationKey correlationKeyType

// WithCorrelationID returns a context carrying the given correlation ID.
// Middleware sets this from the request's X-Request-ID header so that all
// events emitted during a request share the same correlation ID.
func WithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, correlationKey, id)
}

// CorrelationIDFromContext extracts the correlation ID, or "" if not set.
func CorrelationIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(correlationKey).(string); ok {
		return v
	}
	return ""
}

// NewEvent creates an event, pulling correlation ID from context if available.
func NewEvent(eventType, source string, data any) Event {
	return Event{
		ID:        GenerateID(),
		Type:      eventType,
		Source:    source,
		Timestamp: time.Now(),
		Data:      data,
	}
}

// NewEventFromCtx creates an event with correlation ID from the context.
func NewEventFromCtx(ctx context.Context, eventType, source string, data any) Event {
	return Event{
		ID:            GenerateID(),
		Type:          eventType,
		Source:        source,
		Timestamp:     time.Now(),
		CorrelationID: CorrelationIDFromContext(ctx),
		Data:          data,
	}
}

// NewTenantEvent creates an event with tenant context.
func NewTenantEvent(eventType, source, tenantID, userID string, data any) Event {
	return Event{
		ID:        GenerateID(),
		Type:      eventType,
		Source:    source,
		Timestamp: time.Now(),
		TenantID:  tenantID,
		UserID:    userID,
		Data:      data,
	}
}

// NewTenantEventFromCtx creates a tenant event with correlation ID from the context.
func NewTenantEventFromCtx(ctx context.Context, eventType, source, tenantID, userID string, data any) Event {
	return Event{
		ID:            GenerateID(),
		Type:          eventType,
		Source:        source,
		Timestamp:     time.Now(),
		TenantID:      tenantID,
		UserID:        userID,
		CorrelationID: CorrelationIDFromContext(ctx),
		Data:          data,
	}
}

// DebugString returns a human-readable representation of the event.
func (e Event) DebugString() string {
	corr := ""
	if e.CorrelationID != "" {
		corr = " corr=" + e.CorrelationID
	}
	return fmt.Sprintf("[%s] %s from %s (tenant=%s user=%s%s)",
		e.ID[:8], e.Type, e.Source, e.TenantID, e.UserID, corr)
}
