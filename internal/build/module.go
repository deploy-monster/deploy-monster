package build

import (
	"context"
	"log/slog"
	"sync"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func init() {
	core.RegisterModule(func() core.Module { return New() })
}

// Module implements the build engine — detects project types,
// generates Dockerfiles, and builds container images.
type Module struct {
	core   *core.Core
	store  core.Store
	pool   *WorkerPool
	logger *slog.Logger
}

func New() *Module {
	return &Module{}
}

func (m *Module) ID() string                  { return "build" }
func (m *Module) Name() string                { return "Build Engine" }
func (m *Module) Version() string             { return "1.0.0" }
func (m *Module) Dependencies() []string      { return []string{"core.db", "deploy"} }
func (m *Module) Routes() []core.Route        { return nil }
func (m *Module) Events() []core.EventHandler { return nil }

func (m *Module) Init(_ context.Context, c *core.Core) error {
	m.core = c
	m.store = c.Store
	m.logger = c.Logger.With("module", m.ID())

	maxConcurrent := c.Config.Limits.MaxConcurrentBuilds
	if maxConcurrent <= 0 {
		maxConcurrent = 5
	}
	m.pool = NewWorkerPool(maxConcurrent)

	return nil
}

func (m *Module) Start(_ context.Context) error {
	m.logger.Info("build engine started", "max_concurrent", m.pool.maxWorkers)
	return nil
}

func (m *Module) Stop(_ context.Context) error {
	m.pool.Wait()
	return nil
}

func (m *Module) Health() core.HealthStatus {
	return core.HealthOK
}

// WorkerPool limits concurrent builds.
type WorkerPool struct {
	maxWorkers int
	sem        chan struct{}
	wg         sync.WaitGroup
}

func NewWorkerPool(max int) *WorkerPool {
	return &WorkerPool{
		maxWorkers: max,
		sem:        make(chan struct{}, max),
	}
}

// Submit adds a build job to the pool. Blocks if at capacity.
func (wp *WorkerPool) Submit(fn func()) {
	wp.wg.Add(1)
	wp.sem <- struct{}{}
	go func() {
		defer func() {
			<-wp.sem
			wp.wg.Done()
		}()
		fn()
	}()
}

// Wait blocks until all submitted jobs complete.
func (wp *WorkerPool) Wait() {
	wp.wg.Wait()
}
