//go:build linux

package core

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// linuxSysMetricsReader reads metrics from /proc and the root filesystem.
// Successive calls compute a CPU delta against the last sample.
type linuxSysMetricsReader struct {
	mu       sync.Mutex
	lastBusy uint64
	lastIdle uint64
	haveLast bool
}

func newSysMetricsReader() SysMetricsReader { return &linuxSysMetricsReader{} }

func (r *linuxSysMetricsReader) Read() (SysMetrics, error) {
	now := time.Now()
	out := SysMetrics{SampledAt: now}

	busy, idle, err := readCPUTimes()
	if err == nil {
		r.mu.Lock()
		if r.haveLast {
			dBusy := float64(busy - r.lastBusy)
			dTotal := float64((busy + idle) - (r.lastBusy + r.lastIdle))
			if dTotal > 0 {
				pct := (dBusy / dTotal) * 100.0
				if pct < 0 {
					pct = 0
				} else if pct > 100 {
					pct = 100
				}
				out.CPUPercent = pct
			}
		}
		r.lastBusy = busy
		r.lastIdle = idle
		r.haveLast = true
		r.mu.Unlock()
	}

	if total, used, ok := readMeminfo(); ok {
		out.RAMTotalMB = bytesToMB(total)
		out.RAMUsedMB = bytesToMB(used)
	}

	if load, ok := readLoadavg(); ok {
		out.LoadAvg = load
	}

	if total, used, ok := readDiskUsage("/"); ok {
		out.DiskTotalMB = bytesToMB(total)
		out.DiskUsedMB = bytesToMB(used)
	}

	if rx, tx, ok := readNetCounters(); ok {
		out.NetRxBytes = rx
		out.NetTxBytes = tx
	}

	return out, nil
}

// readNetCounters parses /proc/net/dev and returns the sum of received and
// transmitted bytes across every non-loopback interface. Loopback (`lo`)
// would inflate the totals with intra-host traffic and isn't useful as a
// platform-level signal, so it's skipped.
func readNetCounters() (int64, int64, bool) {
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return 0, 0, false
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	// Skip the two header lines.
	for i := 0; i < 2 && scanner.Scan(); i++ {
	}

	var rx, tx int64
	for scanner.Scan() {
		line := scanner.Text()
		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			continue
		}
		iface := strings.TrimSpace(line[:colon])
		if iface == "lo" {
			continue
		}
		fields := strings.Fields(line[colon+1:])
		if len(fields) < 9 {
			continue
		}
		// /proc/net/dev columns: rx_bytes rx_packets ... (8 rx) tx_bytes tx_packets ... (8 tx)
		rxv, err1 := strconv.ParseInt(fields[0], 10, 64)
		txv, err2 := strconv.ParseInt(fields[8], 10, 64)
		if err1 == nil {
			rx += rxv
		}
		if err2 == nil {
			tx += txv
		}
	}
	return rx, tx, true
}

// readCPUTimes parses /proc/stat's first "cpu" aggregate line and returns
// (busy, idle) in jiffies. Busy = user+nice+system+irq+softirq+steal.
// Idle  = idle+iowait.
func readCPUTimes() (uint64, uint64, error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		return 0, 0, fmt.Errorf("empty /proc/stat")
	}
	line := scanner.Text()
	if !strings.HasPrefix(line, "cpu ") {
		return 0, 0, fmt.Errorf("unexpected /proc/stat prefix")
	}

	fields := strings.Fields(line)[1:]
	vals := make([]uint64, 0, len(fields))
	for _, f := range fields {
		v, err := strconv.ParseUint(f, 10, 64)
		if err != nil {
			return 0, 0, err
		}
		vals = append(vals, v)
	}
	if len(vals) < 4 {
		return 0, 0, fmt.Errorf("truncated cpu line")
	}

	// user, nice, system, idle, iowait, irq, softirq, steal, guest, guest_nice
	var user, nice, system, idle, iowait, irq, softirq, steal uint64
	user = vals[0]
	nice = vals[1]
	system = vals[2]
	idle = vals[3]
	if len(vals) >= 5 {
		iowait = vals[4]
	}
	if len(vals) >= 6 {
		irq = vals[5]
	}
	if len(vals) >= 7 {
		softirq = vals[6]
	}
	if len(vals) >= 8 {
		steal = vals[7]
	}

	busy := user + nice + system + irq + softirq + steal
	idleAll := idle + iowait
	return busy, idleAll, nil
}

// readMeminfo returns (total, used) in bytes from /proc/meminfo.
func readMeminfo() (uint64, uint64, bool) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, false
	}
	defer func() { _ = f.Close() }()

	var memTotal, memAvailable uint64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			memTotal = parseMeminfoKB(line)
		case strings.HasPrefix(line, "MemAvailable:"):
			memAvailable = parseMeminfoKB(line)
		}
		if memTotal > 0 && memAvailable > 0 {
			break
		}
	}
	if memTotal == 0 {
		return 0, 0, false
	}
	used := uint64(0)
	if memAvailable < memTotal {
		used = memTotal - memAvailable
	}
	return memTotal * 1024, used * 1024, true
}

// parseMeminfoKB extracts the kilobyte value from a /proc/meminfo line.
// Example: "MemTotal:       16265136 kB" -> 16265136
func parseMeminfoKB(line string) uint64 {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	v, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// readLoadavg parses /proc/loadavg for the 1/5/15 minute averages.
func readLoadavg() ([3]float64, bool) {
	var out [3]float64
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return out, false
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return out, false
	}
	for i := 0; i < 3; i++ {
		v, err := strconv.ParseFloat(fields[i], 64)
		if err != nil {
			return [3]float64{}, false
		}
		out[i] = v
	}
	return out, true
}

// readDiskUsage returns (total, used) bytes for the filesystem containing path.
func readDiskUsage(path string) (uint64, uint64, bool) {
	var s syscall.Statfs_t
	if err := syscall.Statfs(path, &s); err != nil {
		return 0, 0, false
	}
	bsize := uint64(s.Bsize)
	total := s.Blocks * bsize
	free := s.Bfree * bsize
	if free > total {
		return total, 0, true
	}
	return total, total - free, true
}

func bytesToMB(b uint64) int64 {
	return int64(b / (1024 * 1024))
}
