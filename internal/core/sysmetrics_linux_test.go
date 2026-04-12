//go:build linux

package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadMeminfo(t *testing.T) {
	// Happy path — /proc/meminfo exists and parses. We don't assert exact
	// values (CI host varies) but do assert the invariant that used <= total.
	total, used, ok := readMeminfo()
	if !ok {
		t.Skip("/proc/meminfo unavailable")
	}
	if total == 0 {
		t.Fatal("total is zero")
	}
	if used > total {
		t.Errorf("used (%d) > total (%d)", used, total)
	}
}

func TestReadLoadavg(t *testing.T) {
	load, ok := readLoadavg()
	if !ok {
		t.Skip("/proc/loadavg unavailable")
	}
	for i, v := range load {
		if v < 0 {
			t.Errorf("load[%d] = %f, want >= 0", i, v)
		}
	}
}

func TestReadCPUTimes(t *testing.T) {
	busy, idle, err := readCPUTimes()
	if err != nil {
		t.Skipf("cannot read /proc/stat: %v", err)
	}
	if busy == 0 && idle == 0 {
		t.Error("both busy and idle are zero")
	}
}

func TestReadDiskUsage(t *testing.T) {
	// Happy path against the test binary's own directory.
	dir := t.TempDir()
	total, used, ok := readDiskUsage(dir)
	if !ok {
		t.Skip("statfs unavailable")
	}
	if total == 0 {
		t.Error("total disk is zero")
	}
	if used > total {
		t.Errorf("used (%d) > total (%d)", used, total)
	}
}

func TestReadDiskUsage_BadPath(t *testing.T) {
	// A path that can't be statfs'd must return ok=false, not crash.
	bogus := filepath.Join(os.TempDir(), "does-not-exist-for-test-9a7f2c3e")
	_, _, ok := readDiskUsage(bogus)
	if ok {
		t.Error("expected ok=false for missing path")
	}
}

func TestParseMeminfoKB(t *testing.T) {
	tests := []struct {
		line string
		want uint64
	}{
		{"MemTotal:       16265136 kB", 16265136},
		{"MemAvailable:    1234 kB", 1234},
		{"MemTotal: 0 kB", 0},
		{"bogus", 0},
		{"", 0},
		{"MemTotal:       notanumber kB", 0},
	}
	for _, tt := range tests {
		if got := parseMeminfoKB(tt.line); got != tt.want {
			t.Errorf("parseMeminfoKB(%q) = %d, want %d", tt.line, got, tt.want)
		}
	}
}

func TestBytesToMB(t *testing.T) {
	tests := []struct {
		in   uint64
		want int64
	}{
		{0, 0},
		{1024 * 1024, 1},
		{1024*1024*1024 + 512*1024, 1024}, // 1GB + 512KB rounds down to 1024MB
	}
	for _, tt := range tests {
		if got := bytesToMB(tt.in); got != tt.want {
			t.Errorf("bytesToMB(%d) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestLinuxSysMetricsReader_CPUDelta(t *testing.T) {
	// After two reads the CPU percent should be a legal value (0..100). We
	// can't pin the exact value — it depends on host load — but we can make
	// sure the delta math didn't under/overflow.
	r := &linuxSysMetricsReader{}
	if _, err := r.Read(); err != nil {
		t.Fatalf("first read: %v", err)
	}
	// Do some work so the delta is non-trivial on a quiet CI host.
	sum := 0
	for i := 0; i < 1_000_000; i++ {
		sum += i
	}
	_ = sum
	snap, err := r.Read()
	if err != nil {
		t.Fatalf("second read: %v", err)
	}
	if snap.CPUPercent < 0 || snap.CPUPercent > 100 {
		t.Errorf("CPUPercent out of range: %f", snap.CPUPercent)
	}
}
