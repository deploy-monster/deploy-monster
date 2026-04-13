//go:build !linux

package resource

import (
	"math"
	"runtime"
)

// hostStats is the non-Linux fallback metrics provider. The real
// platform-level numbers live in host_linux.go — on macOS, Windows,
// and BSD we return a "best-effort zero" for host metrics because
// the master node is only supported on Linux in production. Dev
// builds on other platforms still need to compile, and the agent
// collector has to have *something* to return.
type hostStats struct{}

// newHostStats constructs the non-Linux fallback.
func newHostStats() *hostStats { return &hostStats{} }

// CPUPercent always returns 0 on non-Linux. The Go runtime does not
// expose a portable per-core busy counter, and wiring a third
// dependency like gopsutil just for dev builds would violate the
// "minimum dependencies" project rule.
func (h *hostStats) CPUPercent() (float64, error) { return 0, nil }

// MemoryMB returns the Go process's estimate of virtual memory as
// both the used and total figures. This is wildly approximate but
// keeps the dashboard from rendering a blank tile on macOS and
// Windows development machines.
func (h *hostStats) MemoryMB() (used, total int64, err error) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	sys := m.Sys / (1024 * 1024)
	// Defensive: clamp uint64->int64 overflow
	if sys > (1<<63)-1 {
		return math.MaxInt64, math.MaxInt64, nil
	}
	return int64(sys), int64(sys), nil
}

// DiskMB is unimplemented off-Linux — returning zeros keeps the JSON
// shape consistent without lying about the host filesystem.
func (h *hostStats) DiskMB() (used, total int64, err error) { return 0, 0, nil }

// NetworkMB is unimplemented off-Linux.
func (h *hostStats) NetworkMB() (rx, tx int64, err error) { return 0, 0, nil }

// LoadAvg is unimplemented off-Linux; the zero array matches what a
// just-booted Linux box would report.
func (h *hostStats) LoadAvg() ([3]float64, error) { return [3]float64{}, nil }
