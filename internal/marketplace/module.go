package marketplace

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func init() {
	core.RegisterModule(func() core.Module { return New() })
}

// Module implements the marketplace — one-click deploy of popular apps
// from a curated template registry.
type Module struct {
	core     *core.Core
	store    core.Store
	registry *TemplateRegistry
	logger   *slog.Logger

	// Optional remote template sources. When non-empty, Start() spawns
	// a background goroutine that refreshes the registry every
	// updateInterval. Populated via AddSource before Start.
	sourcesMu      sync.Mutex
	sources        []TemplateSource
	updateInterval time.Duration
	stopCh         chan struct{}
	stopOnce       sync.Once
	wg             sync.WaitGroup
}

func New() *Module { return &Module{} }

// AddSource registers an upstream template source. Sources are consulted
// in registration order during each refresh tick. Safe to call before
// Start; calling after Start takes effect on the next tick.
func (m *Module) AddSource(source TemplateSource) {
	if source == nil {
		return
	}
	m.sourcesMu.Lock()
	defer m.sourcesMu.Unlock()
	m.sources = append(m.sources, source)
}

// SetUpdateInterval configures how often Start's background loop pulls
// from registered sources. A zero or negative interval disables the
// loop — call UpdateTemplates manually instead.
func (m *Module) SetUpdateInterval(d time.Duration) {
	m.sourcesMu.Lock()
	defer m.sourcesMu.Unlock()
	m.updateInterval = d
}

// UpdateTemplates refreshes the registry from all registered sources in
// a single pass. Failures on individual sources are logged and do not
// abort the remaining sources — one flaky upstream cannot prevent the
// others from contributing.
func (m *Module) UpdateTemplates(ctx context.Context) []*UpdateResult {
	m.sourcesMu.Lock()
	sources := make([]TemplateSource, len(m.sources))
	copy(sources, m.sources)
	m.sourcesMu.Unlock()

	results := make([]*UpdateResult, 0, len(sources))
	for _, src := range sources {
		res, err := m.registry.Update(ctx, src)
		if err != nil {
			if m.logger != nil {
				m.logger.Warn("template source update failed",
					"source", src.Name(), "error", err)
			}
			continue
		}
		if m.logger != nil {
			m.logger.Info("template source refreshed",
				"source", res.Source,
				"added", res.Added,
				"updated", res.Updated,
				"rejected", res.Rejected,
				"total", res.Total)
		}
		results = append(results, res)
	}
	return results
}

func (m *Module) ID() string                  { return "marketplace" }
func (m *Module) Name() string                { return "Marketplace" }
func (m *Module) Version() string             { return "1.0.0" }
func (m *Module) Dependencies() []string      { return []string{"core.db", "deploy"} }
func (m *Module) Routes() []core.Route        { return nil }
func (m *Module) Events() []core.EventHandler { return nil }

func (m *Module) Init(_ context.Context, c *core.Core) error {
	m.core = c
	m.store = c.Store
	m.logger = c.Logger.With("module", m.ID())

	m.registry = NewTemplateRegistry()
	m.registry.LoadBuiltins()

	// Load additional templates for 100+ total
	for _, t := range GetMoreTemplates100() {
		m.registry.Add(t)
	}

	return nil
}

func (m *Module) Start(_ context.Context) error {
	m.logger.Info("marketplace started", "templates", m.registry.Count())

	// Kick off the background update loop only if sources + interval are
	// configured. Most deployments run with builtins only and never need
	// this goroutine at all.
	m.sourcesMu.Lock()
	interval := m.updateInterval
	haveSources := len(m.sources) > 0
	m.sourcesMu.Unlock()

	if haveSources && interval > 0 {
		m.stopCh = make(chan struct{})
		m.wg.Add(1)
		go m.updateLoop(interval)
	}
	return nil
}

func (m *Module) Stop(_ context.Context) error {
	m.stopOnce.Do(func() {
		if m.stopCh != nil {
			close(m.stopCh)
		}
	})
	m.wg.Wait()
	return nil
}

// updateLoop polls the configured sources on a fixed interval until
// Stop closes stopCh. Each tick runs UpdateTemplates with a bounded
// context so a stuck HTTP call cannot block shutdown forever.
func (m *Module) updateLoop(interval time.Duration) {
	defer m.wg.Done()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			m.UpdateTemplates(ctx)
			cancel()
		}
	}
}

func (m *Module) Health() core.HealthStatus {
	// Before Init, registry is nil — report OK (not yet started)
	if m.registry == nil {
		return core.HealthOK
	}
	if m.registry.Count() == 0 {
		return core.HealthDegraded
	}
	return core.HealthOK
}

// Registry returns the template registry for API handlers.
func (m *Module) Registry() *TemplateRegistry { return m.registry }
