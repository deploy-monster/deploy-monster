package ingress

import (
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// RouteEntry represents a single routing rule mapping host+path to a backend pool.
type RouteEntry struct {
	Host        string   // Exact host or *.example.com wildcard
	PathPrefix  string   // Path prefix to match (default: "/")
	Priority    int      // Higher = matched first
	ServiceName string   // Logical service name
	Backends    []string // Backend addresses (ip:port)
	LBStrategy  string   // round-robin, least-conn, ip-hash
	Middlewares []string // Middleware names to apply
	TLS         bool     // Whether TLS is required
	StripPrefix bool     // Strip the path prefix before forwarding
	HealthPath  string   // Health check path
	AppID       string   // DeployMonster application ID
}

// routesSnapshot is the immutable entries slice that backs the routing
// table at a point in time. Readers load the whole struct pointer with
// one atomic read and then iterate without locks. Writers build a fresh
// snapshot and atomic-store the replacement — the old snapshot is only
// GC'd once every in-flight reader has released its pointer.
type routesSnapshot struct {
	entries []*RouteEntry
}

// RouteTable manages the routing rules using an atomic snapshot so the
// request hot path (Match) never contends on a lock.
//
// Reads (Match / All / Count): one atomic.Pointer.Load + range loop.
// Writes (Upsert / Remove / RemoveByAppID): serialize on writeMu, clone
// the current snapshot, apply the mutation, re-sort, atomic.Store. The
// writer mutex exists only to keep two concurrent writers from
// clobbering each other's snapshot — readers never block on it.
//
// Routes are pre-sorted inside each snapshot by priority (desc) then
// path specificity (desc), so Match is a linear scan with first-match
// wins.
type RouteTable struct {
	writeMu  sync.Mutex
	snapshot atomic.Pointer[routesSnapshot]
}

// NewRouteTable creates a new empty route table.
func NewRouteTable() *RouteTable {
	rt := &RouteTable{}
	rt.snapshot.Store(&routesSnapshot{})
	return rt
}

// load returns the current snapshot. Always safe to call concurrently
// with writers — the atomic.Pointer gives a consistent view.
func (rt *RouteTable) load() *routesSnapshot {
	return rt.snapshot.Load()
}

// replace clones the current snapshot, applies mutate() to the cloned
// slice, sorts the result, and atomic-stores it. The writer mutex is
// held by the caller. Readers continue to see the old snapshot until
// the Store completes.
func (rt *RouteTable) replace(mutate func([]*RouteEntry) []*RouteEntry) {
	current := rt.load().entries
	next := make([]*RouteEntry, len(current))
	copy(next, current)
	next = mutate(next)
	sortRoutes(next)
	rt.snapshot.Store(&routesSnapshot{entries: next})
}

// Upsert adds or updates a route entry. If a route with the same
// host+pathPrefix already exists, it is replaced. Readers racing with
// this call will either see the pre- or post-upsert snapshot, never
// a partial write.
func (rt *RouteTable) Upsert(entry *RouteEntry) {
	rt.writeMu.Lock()
	defer rt.writeMu.Unlock()

	if entry.PathPrefix == "" {
		entry.PathPrefix = "/"
	}

	rt.replace(func(next []*RouteEntry) []*RouteEntry {
		for i, r := range next {
			if r.Host == entry.Host && r.PathPrefix == entry.PathPrefix {
				next[i] = entry
				return next
			}
		}
		return append(next, entry)
	})
}

// Remove deletes a route by host and path prefix.
func (rt *RouteTable) Remove(host, pathPrefix string) {
	rt.writeMu.Lock()
	defer rt.writeMu.Unlock()

	if pathPrefix == "" {
		pathPrefix = "/"
	}

	rt.replace(func(next []*RouteEntry) []*RouteEntry {
		for i, r := range next {
			if r.Host == host && r.PathPrefix == pathPrefix {
				return append(next[:i], next[i+1:]...)
			}
		}
		return next
	})
}

// RemoveByAppID removes all routes for a given application.
func (rt *RouteTable) RemoveByAppID(appID string) {
	rt.writeMu.Lock()
	defer rt.writeMu.Unlock()

	rt.replace(func(next []*RouteEntry) []*RouteEntry {
		filtered := next[:0]
		for _, r := range next {
			if r.AppID != appID {
				filtered = append(filtered, r)
			}
		}
		return filtered
	})
}

// Match finds the best matching route for a given host and path.
// Matching order: exact host > wildcard host, then longest path prefix.
// Reads are lock-free: one atomic load plus a linear scan over the
// snapshot, which is immutable for the lifetime of the read.
func (rt *RouteTable) Match(host, path string) *RouteEntry {
	for _, r := range rt.load().entries {
		if matchHost(r.Host, host) && matchPath(r.PathPrefix, path) {
			return r
		}
	}
	return nil
}

// All returns a copy of all route entries. Lock-free: the snapshot
// slice is immutable, so returning a clone protects the caller from
// a future writer shrinking it underfoot.
func (rt *RouteTable) All() []*RouteEntry {
	current := rt.load().entries
	out := make([]*RouteEntry, len(current))
	copy(out, current)
	return out
}

// Count returns the number of routes. Lock-free.
func (rt *RouteTable) Count() int {
	return len(rt.load().entries)
}

// sortRoutes sorts the given slice in place by priority (desc), then
// path length (desc). Called on every write from inside replace().
func sortRoutes(routes []*RouteEntry) {
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Priority != routes[j].Priority {
			return routes[i].Priority > routes[j].Priority
		}
		return len(routes[i].PathPrefix) > len(routes[j].PathPrefix)
	})
}

// matchHost checks if a pattern matches the given host.
// Supports exact match and *.example.com wildcard matching.
func matchHost(pattern, host string) bool {
	if pattern == host {
		return true
	}

	// Wildcard: *.example.com matches sub.example.com
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[1:] // .example.com
		if strings.HasSuffix(host, suffix) && strings.Count(host, ".") == strings.Count(pattern, ".") {
			return true
		}
	}

	return false
}

// matchPath checks if a request path matches the given prefix.
func matchPath(prefix, path string) bool {
	if prefix == "/" {
		return true
	}
	return strings.HasPrefix(path, prefix)
}
