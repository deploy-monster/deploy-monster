package graceful

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// ConnectionTracker tracks active connections per container.
type ConnectionTracker struct {
	mu    sync.RWMutex
	conns map[string]*int64 // containerID -> active connections
}

// NewConnectionTracker creates a new connection tracker.
func NewConnectionTracker() *ConnectionTracker {
	return &ConnectionTracker{
		conns: make(map[string]*int64),
	}
}

// Increment adds a connection to the counter.
func (ct *ConnectionTracker) Increment(containerID string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	if ct.conns == nil {
		ct.conns = make(map[string]*int64)
	}
	if _, ok := ct.conns[containerID]; !ok {
		ct.conns[containerID] = new(int64)
	}
	atomic.AddInt64(ct.conns[containerID], 1)
}

// Decrement removes a connection from the counter.
func (ct *ConnectionTracker) Decrement(containerID string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	if ptr, ok := ct.conns[containerID]; ok && *ptr > 0 {
		atomic.AddInt64(ptr, -1)
	}
}

// Active returns the number of active connections for a container.
func (ct *ConnectionTracker) Active(containerID string) int64 {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	if ptr, ok := ct.conns[containerID]; ok {
		return atomic.LoadInt64(ptr)
	}
	return 0
}

// Total returns the total number of active connections across all containers.
func (ct *ConnectionTracker) Total() int64 {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	var total int64
	for _, ptr := range ct.conns {
		total += atomic.LoadInt64(ptr)
	}
	return total
}

// DrainState tracks the draining state of a container.
type DrainState struct {
	ContainerID string
	StartTime   time.Time
	ActiveConns int64
	Done        chan struct{}
}

// DrainManager manages draining containers.
type DrainManager struct {
	mu       sync.RWMutex
	draining map[string]*DrainState
	tracker  *ConnectionTracker
}

// NewDrainManager creates a new drain manager.
func NewDrainManager(tracker *ConnectionTracker) *DrainManager {
	return &DrainManager{
		draining: make(map[string]*DrainState),
		tracker:  tracker,
	}
}

// StartDrain marks a container as draining and returns a channel that will be closed when draining is complete.
func (dm *DrainManager) StartDrain(containerID string) <-chan struct{} {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if _, ok := dm.draining[containerID]; ok {
		return nil // Already draining
	}

	state := &DrainState{
		ContainerID: containerID,
		StartTime:   time.Now(),
		ActiveConns: dm.tracker.Active(containerID),
		Done:        make(chan struct{}),
	}
	dm.draining[containerID] = state
	return state.Done
}

// CompleteDrain signals that draining is complete for a container.
func (dm *DrainManager) CompleteDrain(containerID string) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if state, ok := dm.draining[containerID]; ok {
		close(state.Done)
		delete(dm.draining, containerID)
	}
}

// IsDraining returns true if a container is being drained.
func (dm *DrainManager) IsDraining(containerID string) bool {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	_, ok := dm.draining[containerID]
	return ok
}

// WaitForDrain blocks until all connections are drained or timeout.
func (dm *DrainManager) WaitForDrain(containerID string, timeout time.Duration) error {
	done := dm.StartDrain(containerID)
	if done == nil {
		return nil // Not draining
	}
	defer dm.CompleteDrain(containerID)

	// Wait for active connections to reach zero
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	deadline := time.After(timeout)

	for {
		select {
		case <-ticker.C:
			if dm.tracker.Active(containerID) == 0 {
				return nil
			}
		case <-deadline:
			return ErrDrainTimeout
		}
	}
}

var ErrDrainTimeout = errors.New("drain timeout: connections still active")
