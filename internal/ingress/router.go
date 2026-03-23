package ingress

import (
	"sort"
	"strings"
	"sync"
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

// RouteTable manages the routing rules with thread-safe access.
// Routes are sorted by priority (highest first), then by path specificity.
type RouteTable struct {
	mu     sync.RWMutex
	routes []*RouteEntry
}

// NewRouteTable creates a new empty route table.
func NewRouteTable() *RouteTable {
	return &RouteTable{}
}

// Upsert adds or updates a route entry. If a route with the same host+pathPrefix
// already exists, it is replaced.
func (rt *RouteTable) Upsert(entry *RouteEntry) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if entry.PathPrefix == "" {
		entry.PathPrefix = "/"
	}

	// Replace existing route with same host+path
	for i, r := range rt.routes {
		if r.Host == entry.Host && r.PathPrefix == entry.PathPrefix {
			rt.routes[i] = entry
			rt.sortLocked()
			return
		}
	}

	// Add new route
	rt.routes = append(rt.routes, entry)
	rt.sortLocked()
}

// Remove deletes a route by host and path prefix.
func (rt *RouteTable) Remove(host, pathPrefix string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if pathPrefix == "" {
		pathPrefix = "/"
	}

	for i, r := range rt.routes {
		if r.Host == host && r.PathPrefix == pathPrefix {
			rt.routes = append(rt.routes[:i], rt.routes[i+1:]...)
			return
		}
	}
}

// RemoveByAppID removes all routes for a given application.
func (rt *RouteTable) RemoveByAppID(appID string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	filtered := rt.routes[:0]
	for _, r := range rt.routes {
		if r.AppID != appID {
			filtered = append(filtered, r)
		}
	}
	rt.routes = filtered
}

// Match finds the best matching route for a given host and path.
// Matching order: exact host > wildcard host, then longest path prefix.
func (rt *RouteTable) Match(host, path string) *RouteEntry {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	// Routes are pre-sorted by priority then path length (descending).
	// First match wins.
	for _, r := range rt.routes {
		if matchHost(r.Host, host) && matchPath(r.PathPrefix, path) {
			return r
		}
	}
	return nil
}

// All returns a copy of all route entries.
func (rt *RouteTable) All() []*RouteEntry {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	out := make([]*RouteEntry, len(rt.routes))
	copy(out, rt.routes)
	return out
}

// Count returns the number of routes.
func (rt *RouteTable) Count() int {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return len(rt.routes)
}

// sortLocked sorts routes by priority (desc), then path length (desc).
// Must be called while holding the write lock.
func (rt *RouteTable) sortLocked() {
	sort.Slice(rt.routes, func(i, j int) bool {
		if rt.routes[i].Priority != rt.routes[j].Priority {
			return rt.routes[i].Priority > rt.routes[j].Priority
		}
		return len(rt.routes[i].PathPrefix) > len(rt.routes[j].PathPrefix)
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
