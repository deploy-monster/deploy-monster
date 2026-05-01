package lb

import (
	"hash/fnv"
	"math"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
)

// Strategy picks the next backend from a pool.
type Strategy interface {
	Next(backends []string, r *http.Request) string
}

// New creates a load balancer strategy by name.
//
// "weighted" returns a Weighted balancer with all backends at default
// weight 1; caller is expected to call SetWeight/SetWeights to diverge
// from plain round-robin behavior.
func New(name string) Strategy {
	switch name {
	case "least-conn", "leastconn":
		return NewLeastConn()
	case "ip-hash", "iphash":
		return &IPHash{}
	case "random":
		return &Random{}
	case "weighted":
		return NewWeighted()
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

// parseXFF takes a raw X-Forwarded-For header value and returns the
// first valid IP address or empty string if none is found.
// This prevents XFF injection attacks where arbitrary bytes could be
// passed through to hash functions.
func parseXFF(raw string) string {
	if raw == "" {
		return ""
	}
	// XFF can be comma-separated: "203.0.113.1, 10.0.0.1, client"
	// Take only the first element (closest proxy to client)
	first := strings.TrimSpace(strings.SplitN(raw, ",", 2)[0])
	first = strings.TrimSpace(first)
	if ip := net.ParseIP(first); ip != nil {
		return ip.String()
	}
	return ""
}

// IPHash consistently routes requests from the same IP to the same backend.
type IPHash struct{}

func (ih *IPHash) Next(backends []string, r *http.Request) string {
	ip := r.RemoteAddr
	if raw := r.Header.Get("X-Forwarded-For"); raw != "" {
		if sanitized := parseXFF(raw); sanitized != "" {
			ip = sanitized
		}
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
	// Use uint64 to prevent overflow when counter exceeds MaxUint32
	sum := uint64(h.Sum32()) + rn.counter.Load()
	idx := sum % uint64(len(backends))
	return backends[idx]
}
