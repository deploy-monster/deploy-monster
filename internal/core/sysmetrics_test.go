package core

import (
	"testing"
	"time"
)

func TestNewSysMetricsReader_ReturnsNonNil(t *testing.T) {
	r := NewSysMetricsReader()
	if r == nil {
		t.Fatal("NewSysMetricsReader returned nil")
	}
}

func TestSysMetricsReader_Read_NeverErrors(t *testing.T) {
	// Contract: Read should return a usable snapshot on every platform,
	// even if some fields are zero. It must not return an error — any
	// platform-specific read failure is surfaced as zero fields so callers
	// can treat the value as "unknown".
	r := NewSysMetricsReader()
	snap, err := r.Read()
	if err != nil {
		t.Fatalf("Read returned error: %v", err)
	}
	if snap.SampledAt.IsZero() {
		t.Error("SampledAt was not populated")
	}
	if time.Since(snap.SampledAt) > 5*time.Second {
		t.Errorf("SampledAt is suspiciously old: %v", snap.SampledAt)
	}
}

func TestSysMetricsReader_Read_SucceedingCallsAreConsistent(t *testing.T) {
	// Two back-to-back reads should both succeed. On Linux the second read
	// may produce a non-zero CPUPercent once a delta is available; on other
	// platforms both reads return zero-value fields except SampledAt.
	r := NewSysMetricsReader()

	if _, err := r.Read(); err != nil {
		t.Fatalf("first Read: %v", err)
	}
	second, err := r.Read()
	if err != nil {
		t.Fatalf("second Read: %v", err)
	}
	if second.CPUPercent < 0 || second.CPUPercent > 100 {
		t.Errorf("CPUPercent out of bounds: %f", second.CPUPercent)
	}
	if second.RAMUsedMB < 0 || second.RAMTotalMB < 0 {
		t.Errorf("negative RAM values: used=%d total=%d", second.RAMUsedMB, second.RAMTotalMB)
	}
	if second.RAMUsedMB > second.RAMTotalMB && second.RAMTotalMB > 0 {
		t.Errorf("RAMUsedMB > RAMTotalMB: %d > %d", second.RAMUsedMB, second.RAMTotalMB)
	}
}
