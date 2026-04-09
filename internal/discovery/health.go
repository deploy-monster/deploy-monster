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
type HealthChecker struct {
	mu       sync.RWMutex
	checks   map[string]*HealthCheck
	client   *http.Client
	logger   *slog.Logger
	interval time.Duration
	stopCh   chan struct{}
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

// Start begins the periodic health check loop.
func (hc *HealthChecker) Start() {
	go func() {
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
	}()
}

// Stop halts the health checker.
func (hc *HealthChecker) Stop() {
	close(hc.stopCh)
}

func (hc *HealthChecker) checkAll() {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	for _, check := range hc.checks {
		var err error
		switch check.Type {
		case "http":
			err = hc.checkHTTP(check)
		case "tcp":
			err = hc.checkTCP(check)
		default:
			err = hc.checkTCP(check)
		}

		check.LastChecked = time.Now()

		if err != nil {
			check.Failures++
			check.LastError = err.Error()
			if check.Failures >= check.Threshold && check.Healthy {
				check.Healthy = false
				hc.logger.Warn("backend unhealthy",
					"backend", check.Backend,
					"failures", check.Failures,
					"error", err,
				)
			}
		} else {
			if !check.Healthy {
				hc.logger.Info("backend recovered", "backend", check.Backend)
			}
			check.Healthy = true
			check.Failures = 0
			check.LastError = ""
		}
	}
}

func (hc *HealthChecker) checkHTTP(check *HealthCheck) error {
	url := fmt.Sprintf("http://%s%s", check.Backend, check.Path)
	ctx, cancel := context.WithTimeout(context.Background(), check.Timeout)
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
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

func (hc *HealthChecker) checkTCP(check *HealthCheck) error {
	conn, err := net.DialTimeout("tcp", check.Backend, check.Timeout)
	if err != nil {
		return err
	}
	conn.Close()
	return nil
}

// Status returns health status for all checked backends.
func (hc *HealthChecker) Status() map[string]*HealthCheck {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	result := make(map[string]*HealthCheck, len(hc.checks))
	for k, v := range hc.checks {
		copy := *v
		result[k] = &copy
	}
	return result
}
