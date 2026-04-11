//go:build linux

package resource

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

// procBase is the mount point of procfs. Tests override this to
// point at a synthetic tree under t.TempDir() so the parsing logic
// can be exercised without depending on the host kernel state.
var procBase = "/proc"

// rootFS is the path used for disk space statistics. Real processes
// stat the root filesystem; tests point this at a tempdir.
var rootFS = "/"

// cpuSample holds a snapshot of the /proc/stat aggregate row so we
// can compute a delta-based CPU percentage across collection cycles.
type cpuSample struct {
	total uint64
	idle  uint64
}

// hostStats is the Linux host metrics provider. It holds the
// previous CPU sample so consecutive calls can produce a real
// percentage. A zero-value is usable and thread-safe.
type hostStats struct {
	mu       sync.Mutex
	lastCPU  cpuSample
	haveLast bool
}

// newHostStats constructs the Linux host metrics provider.
func newHostStats() *hostStats { return &hostStats{} }

// CPUPercent returns the current CPU utilization as a percentage
// averaged across all cores. The first call seeds the baseline and
// returns 0 — subsequent calls return the delta between samples.
func (h *hostStats) CPUPercent() (float64, error) {
	cur, err := readCPUSample()
	if err != nil {
		return 0, err
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.haveLast {
		h.lastCPU = cur
		h.haveLast = true
		return 0, nil
	}

	totalDelta := cur.total - h.lastCPU.total
	idleDelta := cur.idle - h.lastCPU.idle
	h.lastCPU = cur

	if totalDelta == 0 {
		return 0, nil
	}
	busy := totalDelta - idleDelta
	return float64(busy) * 100.0 / float64(totalDelta), nil
}

// readCPUSample parses the aggregate `cpu` row from /proc/stat. The
// row format is: `cpu user nice system idle iowait irq softirq
// steal guest guest_nice`. We sum every field for the total and
// treat idle+iowait as the idle portion so iowait-heavy processes
// don't inflate the reported CPU.
func readCPUSample() (cpuSample, error) {
	f, err := os.Open(procBase + "/stat")
	if err != nil {
		return cpuSample{}, fmt.Errorf("open /proc/stat: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		return cpuSample{}, fmt.Errorf("empty /proc/stat")
	}
	line := scanner.Text()
	if !strings.HasPrefix(line, "cpu ") && !strings.HasPrefix(line, "cpu\t") {
		return cpuSample{}, fmt.Errorf("unexpected /proc/stat first line: %q", line)
	}

	fields := strings.Fields(line)[1:] // drop the "cpu" label
	var total uint64
	var idle uint64
	for i, f := range fields {
		n, err := strconv.ParseUint(f, 10, 64)
		if err != nil {
			return cpuSample{}, fmt.Errorf("parse field %d %q: %w", i, f, err)
		}
		total += n
		if i == 3 || i == 4 {
			// fields[3] = idle, fields[4] = iowait
			idle += n
		}
	}
	return cpuSample{total: total, idle: idle}, nil
}

// MemoryMB returns (used, total) physical memory in MB. It parses
// /proc/meminfo using the MemAvailable field (kernel 3.14+) which
// is the kernel's own accurate "free for new workloads" estimate —
// subtracting MemFree alone under-counts because reclaimable page
// cache is technically "used" from a top-level view.
func (h *hostStats) MemoryMB() (used, total int64, err error) {
	f, err := os.Open(procBase + "/meminfo")
	if err != nil {
		return 0, 0, fmt.Errorf("open /proc/meminfo: %w", err)
	}
	defer f.Close()

	var memTotalKB, memAvailKB int64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			memTotalKB = parseMeminfoKB(line)
		case strings.HasPrefix(line, "MemAvailable:"):
			memAvailKB = parseMeminfoKB(line)
		}
		if memTotalKB > 0 && memAvailKB > 0 {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, err
	}
	if memTotalKB == 0 {
		return 0, 0, fmt.Errorf("MemTotal not found in /proc/meminfo")
	}
	totalMB := memTotalKB / 1024
	usedMB := (memTotalKB - memAvailKB) / 1024
	return usedMB, totalMB, nil
}

// parseMeminfoKB extracts the KB value from a /proc/meminfo line
// of the form `MemTotal:       32857428 kB`. Returns 0 on any
// parse failure — callers check the field counter for completeness.
func parseMeminfoKB(line string) int64 {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	n, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// DiskMB returns (used, total) bytes on the root filesystem in MB
// via statfs(2). Used counts "blocks allocated by the filesystem" —
// same as `df -h /` without the reserved superuser headroom.
func (h *hostStats) DiskMB() (used, total int64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(rootFS, &stat); err != nil {
		return 0, 0, fmt.Errorf("statfs %s: %w", rootFS, err)
	}
	blockSize := int64(stat.Bsize)
	totalBytes := int64(stat.Blocks) * blockSize
	freeBytes := int64(stat.Bavail) * blockSize
	usedBytes := totalBytes - freeBytes
	return usedBytes / (1024 * 1024), totalBytes / (1024 * 1024), nil
}

// NetworkMB returns the cumulative RX and TX for every non-loopback
// interface in /proc/net/dev. The values are monotonic since boot;
// callers that want rate metrics should diff across two calls.
func (h *hostStats) NetworkMB() (rx, tx int64, err error) {
	f, err := os.Open(procBase + "/net/dev")
	if err != nil {
		return 0, 0, fmt.Errorf("open /proc/net/dev: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// Skip the two header rows.
	for i := 0; i < 2 && scanner.Scan(); i++ {
	}

	var rxBytes, txBytes int64
	for scanner.Scan() {
		line := scanner.Text()
		// Expected format:
		//   "  eth0: 12345  100  ...  67890 200 ..."
		// The byte counters are fields[1] (RX) and fields[9] (TX)
		// after splitting on whitespace with the interface label
		// trimmed.
		idx := strings.IndexByte(line, ':')
		if idx < 0 {
			continue
		}
		iface := strings.TrimSpace(line[:idx])
		if iface == "" || iface == "lo" {
			continue
		}
		fields := strings.Fields(line[idx+1:])
		if len(fields) < 10 {
			continue
		}
		if v, err := strconv.ParseInt(fields[0], 10, 64); err == nil {
			rxBytes += v
		}
		if v, err := strconv.ParseInt(fields[8], 10, 64); err == nil {
			txBytes += v
		}
	}
	return rxBytes / (1024 * 1024), txBytes / (1024 * 1024), nil
}

// LoadAvg returns the 1/5/15 minute load average from /proc/loadavg.
func (h *hostStats) LoadAvg() ([3]float64, error) {
	var result [3]float64
	data, err := os.ReadFile(procBase + "/loadavg")
	if err != nil {
		return result, fmt.Errorf("read /proc/loadavg: %w", err)
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return result, fmt.Errorf("unexpected /proc/loadavg format: %q", string(data))
	}
	for i := 0; i < 3; i++ {
		v, err := strconv.ParseFloat(fields[i], 64)
		if err != nil {
			return result, fmt.Errorf("parse loadavg field %d: %w", i, err)
		}
		result[i] = v
	}
	return result, nil
}
