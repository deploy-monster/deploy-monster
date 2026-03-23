package core

import (
	"context"
	"fmt"
	"sync"
)

// Registry manages module registration, dependency resolution, and lifecycle.
type Registry struct {
	mu      sync.RWMutex
	modules map[string]Module
	order   []string // topologically sorted module IDs
}

// NewRegistry creates a new module registry.
func NewRegistry() *Registry {
	return &Registry{
		modules: make(map[string]Module),
	}
}

// Register adds a module to the registry.
// Returns an error if a module with the same ID is already registered.
func (r *Registry) Register(m Module) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := m.ID()
	if _, exists := r.modules[id]; exists {
		return fmt.Errorf("module %q already registered", id)
	}
	r.modules[id] = m
	return nil
}

// Resolve performs topological sort based on Dependencies().
// Returns error if a circular dependency or unknown dependency is detected.
func (r *Registry) Resolve() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	visited := make(map[string]bool)
	visiting := make(map[string]bool)
	var sorted []string

	var visit func(id string) error
	visit = func(id string) error {
		if visited[id] {
			return nil
		}
		if visiting[id] {
			return fmt.Errorf("circular dependency detected at %q", id)
		}
		visiting[id] = true

		m, ok := r.modules[id]
		if !ok {
			return fmt.Errorf("unknown module dependency: %q", id)
		}

		for _, dep := range m.Dependencies() {
			if err := visit(dep); err != nil {
				return err
			}
		}

		visiting[id] = false
		visited[id] = true
		sorted = append(sorted, id)
		return nil
	}

	for id := range r.modules {
		if err := visit(id); err != nil {
			return err
		}
	}

	r.order = sorted
	return nil
}

// InitAll initializes modules in dependency order.
func (r *Registry) InitAll(ctx context.Context, core *Core) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, id := range r.order {
		m := r.modules[id]
		if err := m.Init(ctx, core); err != nil {
			return fmt.Errorf("init %s: %w", id, err)
		}
	}
	return nil
}

// StartAll starts modules in dependency order.
func (r *Registry) StartAll(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, id := range r.order {
		m := r.modules[id]
		if err := m.Start(ctx); err != nil {
			return fmt.Errorf("start %s: %w", id, err)
		}
	}
	return nil
}

// StopAll stops modules in reverse dependency order for graceful shutdown.
func (r *Registry) StopAll(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var firstErr error
	for i := len(r.order) - 1; i >= 0; i-- {
		m := r.modules[r.order[i]]
		if err := m.Stop(ctx); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("stop %s: %w", r.order[i], err)
			}
		}
	}
	return firstErr
}

// Get retrieves a module by ID.
func (r *Registry) Get(id string) Module {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.modules[id]
}

// All returns all registered module IDs in dependency order.
func (r *Registry) All() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}

// HealthAll returns health status for all modules.
func (r *Registry) HealthAll() map[string]HealthStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]HealthStatus, len(r.modules))
	for id, m := range r.modules {
		result[id] = m.Health()
	}
	return result
}
