package lb

import (
	"net/http"
	"sync"
)

// Weighted distributes requests across backends in proportion to a
// configured per-backend weight. It implements Nginx's smooth weighted
// round-robin algorithm (O'Reilly, "Nginx internals"), which gives a
// smoother interleave than naive weighted-random and avoids the thundering
// bias of large-weight backends over short burst windows.
//
// Use cases:
//   - Capacity-tiered backends (e.g. one 8-core box at weight 8,
//     two 2-core boxes at weight 2 each → the big box gets ~66 % of
//     traffic, the small ones ~17 % each).
//   - Blue/green cutovers (blue at 100 / green at 0, flip to 0 / 100
//     in one atomic SetWeights call at cutover time).
//
// Backends with an unset weight default to 1 — so Weighted behaves like
// RoundRobin out of the box and only diverges once SetWeight is called.
type Weighted struct {
	mu      sync.Mutex
	weights map[string]int // static weight per backend
	current map[string]int // running "current weight" per backend (smooth WRR)
}

// NewWeighted creates a weighted load balancer. All backends default to
// weight 1 until SetWeight or SetWeights is called.
func NewWeighted() *Weighted {
	return &Weighted{
		weights: make(map[string]int),
		current: make(map[string]int),
	}
}

// SetWeight sets the static weight for a single backend. Weights must be
// non-negative; a zero weight disables the backend without removing it
// from the pool, which is useful for a graceful drain before
// decommissioning. Negative values are clamped to 0.
func (w *Weighted) SetWeight(backend string, weight int) {
	if weight < 0 {
		weight = 0
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.weights[backend] = weight
	// Reset the running current weight so a weight change takes effect
	// immediately and is not skewed by history.
	w.current[backend] = 0
}

// SetWeights replaces the entire weight table atomically. Backends not
// present in the map fall back to their default weight of 1 on the next
// Next() call. Any weights < 0 are clamped to 0.
func (w *Weighted) SetWeights(weights map[string]int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.weights = make(map[string]int, len(weights))
	w.current = make(map[string]int, len(weights))
	for b, wt := range weights {
		if wt < 0 {
			wt = 0
		}
		w.weights[b] = wt
	}
}

// Weight returns the configured weight for a backend, or 1 if unset.
func (w *Weighted) Weight(backend string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	if wt, ok := w.weights[backend]; ok {
		return wt
	}
	return 1
}

// Next picks a backend from the pool using smooth weighted round-robin.
// The algorithm:
//  1. For every backend, add its static weight to its running current weight.
//  2. Pick the backend with the largest current weight.
//  3. Subtract the total static weight from the picked backend's current.
//
// Over a full cycle each backend is selected exactly `weight_i` times per
// `sum(weights)` picks, interleaved rather than clustered. Backends with
// weight 0 are skipped entirely — if every backend has weight 0, this
// method falls back to returning the first backend (caller is responsible
// for not routing traffic into an all-zero pool).
func (w *Weighted) Next(backends []string, _ *http.Request) string {
	if len(backends) == 0 {
		return ""
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	total := 0
	for _, b := range backends {
		wt, ok := w.weights[b]
		if !ok {
			wt = 1
		}
		total += wt
	}
	if total == 0 {
		return backends[0]
	}

	best := ""
	bestCur := 0
	first := true
	for _, b := range backends {
		wt, ok := w.weights[b]
		if !ok {
			wt = 1
		}
		if wt == 0 {
			continue
		}
		w.current[b] += wt
		if first || w.current[b] > bestCur {
			best = b
			bestCur = w.current[b]
			first = false
		}
	}
	if best == "" {
		return backends[0]
	}
	w.current[best] -= total
	return best
}
