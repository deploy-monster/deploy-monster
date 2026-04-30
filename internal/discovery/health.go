package discovery

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"
)

// HealthChecker performs periodic health checks on registered backends
// and removes unhealthy ones from the load balancer pool.
//
// The checker is designed so that the hot path (IsHealthy from ingress
// request routing) never blocks on I/O: the periodic sweep snapshots
// the check list under a brief lock, performs the network dials outside
// of any lock, then re-acquires the lock only to record the results.
// This keeps the ingress hot path latency-bounded even when a backend
// is hanging on a TCP dial.
type HealthChecker struct {
	mu       sync.RWMutex
	checks   map[string]*HealthCheck
	client   *http.Client
	logger   *slog.Logger
	interval time.Duration

	// Shutdown plumbing. stopOnce guards against double-close of stopCh
	// (a real risk when Stop is called from both Module.Stop and a test
	// teardown). wg lets Stop wait for the loop goroutine to fully exit.
	stopCh    chan struct{}
	stopOnce  sync.Once
	startOnce sync.Once
	wg        sync.WaitGroup
}

// HealthCheck defines how to verify a backend is healthy.
type HealthCheck struct {
	Backend     string // host:port
	Type        string // http, tcp
	Path        string // HTTP path (for http type)
	Interval    time.Duration
	Timeout     time.Duration
	Healthy     bool
	LastChecked time.Time
	LastError   string
	Failures    int
	Threshold   int // Failures before marking unhealthy
}

// NewHealthChecker creates a new backend health checker.
func NewHealthChecker(logger *slog.Logger) *HealthChecker {
	if logger == nil {
		logger = slog.Default()
	}
	return &HealthChecker{
		checks:   make(map[string]*HealthCheck),
		client:   &http.Client{Timeout: 5 * time.Second},
		logger:   logger,
		interval: 10 * time.Second,
		stopCh:   make(chan struct{}),
	}
}

// Register adds a backend for health checking.
func (hc *HealthChecker) Register(backend, checkType, path string) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	hc.checks[backend] = &HealthCheck{
		Backend:   backend,
		Type:      checkType,
		Path:      path,
		Interval:  10 * time.Second,
		Timeout:   5 * time.Second,
		Healthy:   true, // Assume healthy until proven otherwise
		Threshold: 3,
	}
}

// Deregister removes a backend from health checking.
func (hc *HealthChecker) Deregister(backend string) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	delete(hc.checks, backend)
}

// IsHealthy returns whether a backend is currently healthy.
func (hc *HealthChecker) IsHealthy(backend string) bool {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	check, ok := hc.checks[backend]
	if !ok {
		return true // Unknown backends assumed healthy
	}
	return check.Healthy
}

// Start begins the periodic health check loop. Calling Start more than
// once is a no-op — subsequent calls do not spawn additional goroutines.
func (hc *HealthChecker) Start() {
	hc.startOnce.Do(func() {
		hc.wg.Add(1)
		go hc.loop()
	})
}

// Stop halts the health checker. Safe to call multiple times; the
// second and subsequent calls are no-ops. Stop waits for the loop
// goroutine to exit before returning so callers can rely on "after
// Stop, no more sweeps run."
func (hc *HealthChecker) Stop() {
	hc.stopOnce.Do(func() {
		close(hc.stopCh)
	})
	hc.wg.Wait()
}

func (hc *HealthChecker) loop() {
	defer hc.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			hc.logger.Error("panic in health checker", "error", r)
		}
	}()
	ticker := time.NewTicker(hc.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			hc.checkAll()
		case <-hc.stopCh:
			return
		}
	}
}

// checkResult carries the outcome of a single network probe back to
// the post-probe commit phase. Keeping the probe result separate from
// the mutable HealthCheck struct lets us perform all network I/O with
// no locks held and then apply the results in a second, lock-held pass.
type checkResult struct {
	backend string
	err     error
	now     time.Time
}

// checkAll sweeps every registered backend. Network I/O is performed
// without any lock held — see the snapshot/commit phases below — so a
// slow backend cannot block IsHealthy readers on the hot ingress path.
func (hc *HealthChecker) checkAll() {
	// Phase 1: snapshot the probe work under a brief lock. We copy just
	// enough to run the probe (backend, type, path, timeout) so the
	// outer lock can be released immediately.
	type probe struct {
		backend   string
		checkType string
		path      string
		timeout   time.Duration
	}
	hc.mu.RLock()
	probes := make([]probe, 0, len(hc.checks))
	for _, c := range hc.checks {
		probes = append(probes, probe{
			backend:   c.Backend,
			checkType: c.Type,
			path:      c.Path,
			timeout:   c.Timeout,
		})
	}
	hc.mu.RUnlock()

	// Phase 2: run every probe with no locks held. In a future
	// iteration these could fan out in parallel; for now serial is
	// sufficient because each probe is capped by its own Timeout and
	// the default Interval (10s) is much larger than the typical probe
	// duration.
	results := make([]checkResult, 0, len(probes))
	for _, p := range probes {
		var err error
		switch p.checkType {
		case "http":
			err = hc.probeHTTP(p.backend, p.path, p.timeout)
		case "tcp":
			err = hc.probeTCP(p.backend, p.timeout)
		default:
			err = hc.probeTCP(p.backend, p.timeout)
		}
		results = append(results, checkResult{
			backend: p.backend,
			err:     err,
			now:     time.Now(),
		})
	}

	// Phase 3: commit results under the write lock. We re-check
	// existence because a Deregister may have happened while probes
	// were running — in that case we silently drop the result.
	hc.mu.Lock()
	defer hc.mu.Unlock()
	for _, res := range results {
		check, ok := hc.checks[res.backend]
		if !ok {
			continue
		}
		check.LastChecked = res.now
		if res.err != nil {
			check.Failures++
			check.LastError = res.err.Error()
			if check.Failures >= check.Threshold && check.Healthy {
				check.Healthy = false
				hc.logger.Warn("backend unhealthy",
					"backend", check.Backend,
					"failures", check.Failures,
					"error", res.err,
				)
			}
			continue
		}
		if !check.Healthy {
			hc.logger.Info("backend recovered", "backend", check.Backend)
		}
		check.Healthy = true
		check.Failures = 0
		check.LastError = ""
	}
}

func (hc *HealthChecker) probeHTTP(backend, path string, timeout time.Duration) error {
	url := fmt.Sprintf("http://%s%s", backend, path)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := hc.client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

func (hc *HealthChecker) probeTCP(backend string, timeout time.Duration) error {
	conn, err := net.DialTimeout("tcp", backend, timeout)
	if err != nil {
		return err
	}
	_ = conn.Close()
	return nil
}

// Status returns health status for all checked backends.
func (hc *HealthChecker) Status() map[string]*HealthCheck {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	result := make(map[string]*HealthCheck, len(hc.checks))
	for k, v := range hc.checks {
		snapshot := *v
		result[k] = &snapshot
	}
	return result
}
