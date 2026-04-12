//go:build linux

package resource

import (
	"os"
	"path/filepath"
	"testing"
)

// withSyntheticProc writes a fake /proc tree under t.TempDir() and
// points procBase at it for the duration of the test. The returned
// cleanup restores procBase so parallel sub-tests don't trample
// each other's overrides.
func withSyntheticProc(t *testing.T, files map[string]string) {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", full, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
	orig := procBase
	procBase = dir
	t.Cleanup(func() { procBase = orig })
}

func TestHostStats_CPUPercent_DeltaBased(t *testing.T) {
	// First sample: 100 user, 0 nice, 0 system, 900 idle, 0 iowait
	// → total=1000, idle=900
	withSyntheticProc(t, map[string]string{
		"stat": "cpu  100 0 0 900 0 0 0 0 0 0\n",
	})
	h := newHostStats()
	first, err := h.CPUPercent()
	if err != nil {
		t.Fatalf("first CPUPercent: %v", err)
	}
	if first != 0 {
		t.Errorf("first sample returned %.2f%%, want 0 (baseline seed)", first)
	}

	// Second sample: 200 user, 0 nice, 0 system, 1000 idle, 0 iowait
	// delta: total=200, idle=100, busy=100 → 50%
	withSyntheticProc(t, map[string]string{
		"stat": "cpu  200 0 0 1000 0 0 0 0 0 0\n",
	})
	second, err := h.CPUPercent()
	if err != nil {
		t.Fatalf("second CPUPercent: %v", err)
	}
	if second < 49.9 || second > 50.1 {
		t.Errorf("second sample = %.2f%%, want ~50%%", second)
	}
}

func TestHostStats_CPUPercent_ZeroDelta(t *testing.T) {
	withSyntheticProc(t, map[string]string{
		"stat": "cpu  100 0 0 900 0 0 0 0 0 0\n",
	})
	h := newHostStats()
	if _, err := h.CPUPercent(); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Identical sample → zero busy, zero total → returns 0 cleanly.
	got, err := h.CPUPercent()
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if got != 0 {
		t.Errorf("zero-delta CPUPercent = %.2f, want 0", got)
	}
}

func TestHostStats_CPUPercent_BadFile(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		orig := procBase
		procBase = filepath.Join(t.TempDir(), "does-not-exist")
		t.Cleanup(func() { procBase = orig })
		if _, err := newHostStats().CPUPercent(); err == nil {
			t.Error("missing /proc/stat returned nil error, want failure")
		}
	})
	t.Run("garbage first line", func(t *testing.T) {
		withSyntheticProc(t, map[string]string{"stat": "not a cpu line\n"})
		if _, err := newHostStats().CPUPercent(); err == nil {
			t.Error("garbage first line returned nil error")
		}
	})
	t.Run("non-numeric field", func(t *testing.T) {
		withSyntheticProc(t, map[string]string{"stat": "cpu  abc 0 0 0 0 0 0 0 0 0\n"})
		if _, err := newHostStats().CPUPercent(); err == nil {
			t.Error("non-numeric field returned nil error")
		}
	})
}

func TestHostStats_MemoryMB(t *testing.T) {
	// 8 GB total, 4 GB available → 4 GB used.
	meminfo := "MemTotal:       8388608 kB\n" +
		"MemFree:         1048576 kB\n" +
		"MemAvailable:   4194304 kB\n" +
		"Buffers:         262144 kB\n"
	withSyntheticProc(t, map[string]string{"meminfo": meminfo})

	used, total, err := newHostStats().MemoryMB()
	if err != nil {
		t.Fatalf("MemoryMB: %v", err)
	}
	if total != 8192 {
		t.Errorf("total = %d MB, want 8192", total)
	}
	if used != 4096 {
		t.Errorf("used = %d MB, want 4096", used)
	}
}

func TestHostStats_MemoryMB_MissingTotal(t *testing.T) {
	withSyntheticProc(t, map[string]string{
		"meminfo": "MemAvailable:   4194304 kB\n",
	})
	if _, _, err := newHostStats().MemoryMB(); err == nil {
		t.Error("missing MemTotal returned nil error")
	}
}

func TestHostStats_NetworkMB(t *testing.T) {
	// Two header rows, loopback (skipped), eth0, eth1.
	// Column layout (after the `:`):
	// bytes packets errs drop fifo frame compressed multicast \
	//   bytes packets errs drop fifo colls carrier compressed
	netdev := "Inter-|   Receive                                                |  Transmit\n" +
		" face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed\n" +
		"    lo: 99999999       0    0    0    0     0          0         0  99999999       0    0    0    0     0       0          0\n" +
		"  eth0: 1048576       10    0    0    0     0          0         0   524288       5    0    0    0     0       0          0\n" +
		"  eth1: 2097152       20    0    0    0     0          0         0  1048576       8    0    0    0     0       0          0\n"
	withSyntheticProc(t, map[string]string{"net/dev": netdev})

	rx, tx, err := newHostStats().NetworkMB()
	if err != nil {
		t.Fatalf("NetworkMB: %v", err)
	}
	// eth0+eth1 rx: 1048576 + 2097152 = 3145728 → 3 MB
	if rx != 3 {
		t.Errorf("rx = %d MB, want 3", rx)
	}
	// eth0+eth1 tx: 524288 + 1048576 = 1572864 → 1 MB (integer division)
	if tx != 1 {
		t.Errorf("tx = %d MB, want 1", tx)
	}
}

func TestHostStats_LoadAvg(t *testing.T) {
	withSyntheticProc(t, map[string]string{
		"loadavg": "0.42 1.50 2.75 3/456 12345\n",
	})
	load, err := newHostStats().LoadAvg()
	if err != nil {
		t.Fatalf("LoadAvg: %v", err)
	}
	want := [3]float64{0.42, 1.50, 2.75}
	for i := range want {
		if load[i] < want[i]-0.001 || load[i] > want[i]+0.001 {
			t.Errorf("load[%d] = %f, want %f", i, load[i], want[i])
		}
	}
}

func TestHostStats_LoadAvg_BadFile(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		orig := procBase
		procBase = filepath.Join(t.TempDir(), "none")
		t.Cleanup(func() { procBase = orig })
		if _, err := newHostStats().LoadAvg(); err == nil {
			t.Error("missing file returned nil error")
		}
	})
	t.Run("short", func(t *testing.T) {
		withSyntheticProc(t, map[string]string{"loadavg": "0.1\n"})
		if _, err := newHostStats().LoadAvg(); err == nil {
			t.Error("short file returned nil error")
		}
	})
	t.Run("non-numeric", func(t *testing.T) {
		withSyntheticProc(t, map[string]string{"loadavg": "x y z 1/2 3\n"})
		if _, err := newHostStats().LoadAvg(); err == nil {
			t.Error("non-numeric returned nil error")
		}
	})
}

func TestHostStats_DiskMB(t *testing.T) {
	// DiskMB calls statfs on rootFS — point it at a real tempdir so
	// the syscall succeeds against a filesystem we know exists. The
	// numbers are whatever the host reports; we just assert sanity.
	orig := rootFS
	rootFS = t.TempDir()
	t.Cleanup(func() { rootFS = orig })

	used, total, err := newHostStats().DiskMB()
	if err != nil {
		t.Fatalf("DiskMB: %v", err)
	}
	if total <= 0 {
		t.Errorf("total = %d, want positive", total)
	}
	if used < 0 || used > total {
		t.Errorf("used = %d (total = %d), want 0 ≤ used ≤ total", used, total)
	}
}

func TestParseMeminfoKB(t *testing.T) {
	cases := []struct {
		line string
		want int64
	}{
		{"MemTotal:       8388608 kB", 8388608},
		{"MemAvailable:   4194304 kB", 4194304},
		{"MemTotal:", 0},
		{"garbage", 0},
		{"MemTotal:       notanumber kB", 0},
	}
	for _, c := range cases {
		got := parseMeminfoKB(c.line)
		if got != c.want {
			t.Errorf("parseMeminfoKB(%q) = %d, want %d", c.line, got, c.want)
		}
	}
}
