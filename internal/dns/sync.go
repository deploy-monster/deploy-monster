package dns

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// SyncJob represents a pending DNS record operation.
type SyncJob struct {
	ID        string
	Action    string // create, update, delete
	Provider  string
	Record    core.DNSRecord
	DomainID  string
	Retries   int
	LastError string
	CreatedAt time.Time
}

// SyncQueue processes DNS operations asynchronously with retry logic.
type SyncQueue struct {
	mu       sync.Mutex
	jobs     chan *SyncJob
	services *core.Services
	store    core.Store
	events   *core.EventBus
	logger   *slog.Logger
	stopCh   chan struct{}
}

// NewSyncQueue creates a DNS sync queue.
func NewSyncQueue(services *core.Services, store core.Store, events *core.EventBus, logger *slog.Logger) *SyncQueue {
	return &SyncQueue{
		jobs:     make(chan *SyncJob, 100),
		services: services,
		store:    store,
		events:   events,
		logger:   logger,
		stopCh:   make(chan struct{}),
	}
}

// Enqueue adds a DNS operation to the queue.
func (q *SyncQueue) Enqueue(job *SyncJob) {
	if job.ID == "" {
		job.ID = core.GenerateID()
	}
	job.CreatedAt = time.Now()
	select {
	case q.jobs <- job:
		q.logger.Info("DNS job queued", "action", job.Action, "record", job.Record.Name)
	default:
		q.logger.Warn("DNS queue full, dropping job", "record", job.Record.Name)
	}
}

// Start begins processing the queue.
func (q *SyncQueue) Start() {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				q.logger.Error("panic in DNS sync queue", "error", r)
			}
		}()
		for {
			select {
			case job := <-q.jobs:
				q.process(job)
			case <-q.stopCh:
				return
			}
		}
	}()
}

// Stop signals the queue to stop.
func (q *SyncQueue) Stop() {
	close(q.stopCh)
}

func (q *SyncQueue) process(job *SyncJob) {
	provider := q.services.DNSProvider(job.Provider)
	if provider == nil {
		q.logger.Error("DNS provider not found", "provider", job.Provider)
		return
	}

	var err error
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	switch job.Action {
	case "create":
		err = provider.CreateRecord(ctx, job.Record)
	case "update":
		err = provider.UpdateRecord(ctx, job.Record)
	case "delete":
		err = provider.DeleteRecord(ctx, job.Record)
	default:
		q.logger.Error("unknown DNS action", "action", job.Action)
		return
	}

	if err != nil {
		job.Retries++
		job.LastError = err.Error()
		q.logger.Error("DNS sync failed",
			"action", job.Action,
			"record", job.Record.Name,
			"error", err,
			"retries", job.Retries,
		)

		// Retry up to 3 times with backoff
		if job.Retries < 3 {
			time.Sleep(time.Duration(job.Retries*5) * time.Second)
			q.Enqueue(job)
		}
		return
	}

	q.logger.Info("DNS sync complete",
		"action", job.Action,
		"record", job.Record.Name,
		"provider", job.Provider,
	)

	// Verify propagation
	go func() {
		time.Sleep(5 * time.Second)
		verified, _ := provider.Verify(context.Background(), job.Record.Name)
		if verified {
			q.logger.Info("DNS verified", "record", job.Record.Name)
		}
	}()
}

// SyncDomainRecords creates DNS A records for a domain pointing to the server IP.
func SyncDomainRecords(q *SyncQueue, fqdn, serverIP, provider string) {
	q.Enqueue(&SyncJob{
		Action:   "create",
		Provider: provider,
		Record: core.DNSRecord{
			Type:  "A",
			Name:  fqdn,
			Value: serverIP,
			TTL:   300,
		},
	})

	// Also create www subdomain
	if fqdn[0] != '*' {
		q.Enqueue(&SyncJob{
			Action:   "create",
			Provider: provider,
			Record: core.DNSRecord{
				Type:  "CNAME",
				Name:  fmt.Sprintf("www.%s", fqdn),
				Value: fqdn,
				TTL:   300,
			},
		})
	}
}
