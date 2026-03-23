package lb

import (
	"hash/fnv"
	"math"
	"net/http"
	"sync"
	"sync/atomic"
)

// Strategy picks the next backend from a pool.
type Strategy interface {
	Next(backends []string, r *http.Request) string
}

// New creates a load balancer strategy by name.
func New(name string) Strategy {
	switch name {
	case "least-conn", "leastconn":
		return NewLeastConn()
	case "ip-hash", "iphash":
		return &IPHash{}
	case "random":
		return &Random{}
	default: // round-robin
		return &RoundRobin{}
	}
}

// RoundRobin distributes requests sequentially across backends.
type RoundRobin struct {
	counter atomic.Uint64
}

func (rr *RoundRobin) Next(backends []string, _ *http.Request) string {
	idx := rr.counter.Add(1) - 1
	return backends[idx%uint64(len(backends))]
}

// LeastConn routes to the backend with the fewest active connections.
type LeastConn struct {
	mu    sync.Mutex
	conns map[string]int64
}

func NewLeastConn() *LeastConn {
	return &LeastConn{conns: make(map[string]int64)}
}

func (lc *LeastConn) Next(backends []string, _ *http.Request) string {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	var best string
	var bestCount int64 = math.MaxInt64

	for _, b := range backends {
		count := lc.conns[b]
		if count < bestCount {
			bestCount = count
			best = b
		}
	}

	lc.conns[best]++
	return best
}

// Release decrements the connection count for a backend.
func (lc *LeastConn) Release(backend string) {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	if lc.conns[backend] > 0 {
		lc.conns[backend]--
	}
}

// IPHash consistently routes requests from the same IP to the same backend.
type IPHash struct{}

func (ih *IPHash) Next(backends []string, r *http.Request) string {
	ip := r.RemoteAddr
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ip = xff
	}
	h := fnv.New32a()
	h.Write([]byte(ip))
	idx := h.Sum32() % uint32(len(backends))
	return backends[idx]
}

// Random selects a random backend.
type Random struct {
	counter atomic.Uint64 // pseudo-random via counter + FNV
}

func (rn *Random) Next(backends []string, r *http.Request) string {
	// Mix counter with remote addr for better distribution
	rn.counter.Add(1)
	h := fnv.New32a()
	h.Write([]byte(r.RemoteAddr))
	idx := (h.Sum32() + uint32(rn.counter.Load())) % uint32(len(backends))
	return backends[idx]
}

// Weighted distributes traffic based on backend weights.
// Used for canary deployments.
type Weighted struct {
	mu      sync.RWMutex
	weights map[string]int // backend -> weight (1-100)
	counter atomic.Uint64
}

func NewWeighted(weights map[string]int) *Weighted {
	return &Weighted{weights: weights}
}

func (w *Weighted) Next(backends []string, _ *http.Request) string {
	w.mu.RLock()
	defer w.mu.RUnlock()

	// Build weighted pool
	var pool []string
	for _, b := range backends {
		weight := w.weights[b]
		if weight <= 0 {
			weight = 1
		}
		for range weight {
			pool = append(pool, b)
		}
	}

	if len(pool) == 0 {
		return backends[0]
	}

	idx := w.counter.Add(1) - 1
	return pool[idx%uint64(len(pool))]
}

// SetWeight updates the weight for a backend.
func (w *Weighted) SetWeight(backend string, weight int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.weights[backend] = weight
}
