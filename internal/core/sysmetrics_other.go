//go:build !linux

package core

import "time"

// fallbackSysMetricsReader is used on non-Linux operating systems where we
// don't parse /proc. It returns a best-effort snapshot with zeros for the
// fields it can't gather; tests and UIs should treat zeros as "unknown".
type fallbackSysMetricsReader struct{}

func newSysMetricsReader() SysMetricsReader { return fallbackSysMetricsReader{} }

func (fallbackSysMetricsReader) Read() (SysMetrics, error) {
	return SysMetrics{SampledAt: time.Now()}, nil
}
