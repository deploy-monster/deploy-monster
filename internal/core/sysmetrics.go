package core

import "time"

// SysMetrics is a lightweight snapshot of host-level resource usage gathered
// from the operating system. Fields that can't be read on the current OS are
// returned as zero so callers can safely surface them without branching.
type SysMetrics struct {
	CPUPercent  float64    // 0..100, busy fraction across all CPUs since the last sample
	RAMUsedMB   int64      // MemTotal - MemAvailable
	RAMTotalMB  int64      // MemTotal
	DiskUsedMB  int64      // best-effort root-fs used
	DiskTotalMB int64      // best-effort root-fs total
	LoadAvg     [3]float64 // 1/5/15 min load averages on Linux; zeros elsewhere
	SampledAt   time.Time
}

// SysMetricsReader reads a point-in-time SysMetrics snapshot. Implementations
// must be cheap enough to call on a short interval (<= 1s) — the CPU sample
// path retains a tiny bit of state so it can compute a delta over successive
// calls without sleeping internally.
type SysMetricsReader interface {
	Read() (SysMetrics, error)
}

// NewSysMetricsReader returns the platform-appropriate reader. On Linux it
// parses /proc; on other operating systems it returns a reader whose Read
// method produces a best-effort SysMetrics with the fields it can fill in and
// zeros for everything else. It never returns nil.
func NewSysMetricsReader() SysMetricsReader { return newSysMetricsReader() }
