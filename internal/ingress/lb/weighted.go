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
//   - Canary rollouts (stable at weight 95, canary at weight 5 → 5 %
//     of traffic hits the new version; adjust to 25, 50, 100 as
//     confidence grows).
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
// Exposed for tests and for Canary which layers on top.
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

// Canary wraps a Weighted balancer with a two-pool model: a stable pool
// that receives (100 - percent)% of traffic and a canary pool that
// receives percent%. The split is implemented on top of Weighted so the
// same smooth-WRR algorithm picks which pool a given request lands on;
// inside each pool, backends are distributed using Weighted as well (or
// plain round-robin if weights are not set).
//
// Typical lifecycle:
//
//	c := NewCanary([]string{"v1a", "v1b"}, []string{"v2a"}, 5)
//	// ... route traffic via c.Next(...); 5 % hits v2a ...
//	c.SetPercent(25)   // ramp up
//	c.SetPercent(100)  // fully cut over; stable pool now idle
//
// The canary pool is allowed to be empty; in that case Next always picks
// from the stable pool regardless of percent.
type Canary struct {
	mu         sync.Mutex
	stable     []string
	canary     []string
	percent    int // 0..100
	stableLB   *Weighted
	canaryLB   *Weighted
	router     *Weighted // picks "stable" vs "canary" virtual backend
	stableKey  string
	canaryKey  string
	routerPool []string
}

// NewCanary constructs a Canary balancer. percent is clamped to [0,100].
// When percent is 0 or canary is empty, the Canary behaves as a plain
// Weighted over the stable pool. When percent is 100, all traffic goes
// to canary (subject to canary being non-empty).
func NewCanary(stable, canary []string, percent int) *Canary {
	c := &Canary{
		stable:     append([]string(nil), stable...),
		canary:     append([]string(nil), canary...),
		percent:    clampPercent(percent),
		stableLB:   NewWeighted(),
		canaryLB:   NewWeighted(),
		router:     NewWeighted(),
		stableKey:  "__stable__",
		canaryKey:  "__canary__",
		routerPool: []string{"__stable__", "__canary__"},
	}
	c.syncRouter()
	return c
}

// SetPercent updates the traffic-percent going to the canary pool. The
// change takes effect on the next Next() call.
func (c *Canary) SetPercent(percent int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.percent = clampPercent(percent)
	c.syncRouter()
}

// Percent returns the current canary traffic percentage.
func (c *Canary) Percent() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.percent
}

// StableWeights and CanaryWeights expose the inner Weighted balancers so
// callers can configure uneven capacity within each pool. The underlying
// maps are copied under lock by the Weighted methods themselves.
func (c *Canary) StableWeights() *Weighted { return c.stableLB }
func (c *Canary) CanaryWeights() *Weighted { return c.canaryLB }

// Next picks a backend using the smooth-WRR router to pick the pool,
// then delegates to the per-pool Weighted balancer.
func (c *Canary) Next(_ []string, r *http.Request) string {
	c.mu.Lock()
	// If the canary pool is empty, route to stable unconditionally.
	if len(c.canary) == 0 || c.percent == 0 {
		stable := append([]string(nil), c.stable...)
		c.mu.Unlock()
		return c.stableLB.Next(stable, r)
	}
	if c.percent >= 100 {
		canary := append([]string(nil), c.canary...)
		c.mu.Unlock()
		return c.canaryLB.Next(canary, r)
	}
	pool := append([]string(nil), c.routerPool...)
	stable := append([]string(nil), c.stable...)
	canary := append([]string(nil), c.canary...)
	c.mu.Unlock()

	pick := c.router.Next(pool, r)
	if pick == c.canaryKey && len(canary) > 0 {
		return c.canaryLB.Next(canary, r)
	}
	return c.stableLB.Next(stable, r)
}

// syncRouter updates the router's weight table to reflect the current
// percent. Caller must hold c.mu.
func (c *Canary) syncRouter() {
	c.router.SetWeights(map[string]int{
		c.stableKey: 100 - c.percent,
		c.canaryKey: c.percent,
	})
}

func clampPercent(p int) int {
	switch {
	case p < 0:
		return 0
	case p > 100:
		return 100
	}
	return p
}
