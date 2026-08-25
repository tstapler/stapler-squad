//go:build linux

package telemetry

import (
	"bufio"
	"context"
	"os"
	"strconv"
	"strings"
	"sync"

	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/log"
)

// This file exists to confirm/refute, with real dashboard visibility instead
// of a one-off manual `cat /sys/fs/cgroup/.../memory.*` check, whether the
// process's cgroup memory.high throttling (2026-08-25 investigation: usage
// sitting at the MemoryHigh ceiling set in scripts/install-service.sh,
// 630M+ cumulative reclaim, 14 cumulative OOM kills) correlates with reports
// of broken real-time terminal type-ahead on the live instance. Follows
// session/streamhub/observability.go's pattern: sync.Once-guarded
// registration in init(), Observable instruments read fresh at each
// collection callback — there's no in-process running value to back with an
// atomic here, since the kernel's cgroup files already are the source of
// truth, read fresh every callback instead of cached.
var cgroupRegisterOnce sync.Once

func init() {
	cgroupRegisterOnce.Do(func() {
		if err := registerCgroupMemoryMetrics(); err != nil {
			log.Warn("telemetry: failed to register cgroup memory metrics (expected outside a cgroup v2 environment)", "error", err)
		}
	})
}

// cgroupMemoryDir resolves this process's own cgroup v2 memory-controller
// directory from /proc/self/cgroup + the unified mount point, rather than
// hardcoding a path — the actual path is specific to the systemd unit/user
// slice this happens to run under and differs machine to machine.
func cgroupMemoryDir() (string, error) {
	data, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return "", err
	}
	// cgroup v2 (unified hierarchy) reports exactly one line: "0::<path>".
	line := strings.TrimSpace(string(data))
	parts := strings.SplitN(line, ":", 3)
	if len(parts) != 3 {
		return "", os.ErrInvalid
	}
	return "/sys/fs/cgroup" + parts[2], nil
}

func readCgroupInt64(path string) (int64, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	s := strings.TrimSpace(string(data))
	if s == "max" {
		return 0, false // unlimited — nothing meaningful to report as a byte ceiling
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// parseCgroupEvents parses memory.events' "key value" lines into a map, e.g.
// "high 42\nmax 0\noom 1\noom_kill 1\n".
func parseCgroupEvents(data []byte) map[string]int64 {
	out := make(map[string]int64, 8)
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			continue
		}
		if v, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
			out[fields[0]] = v
		}
	}
	return out
}

// parseCgroupPSIAvg10 extracts the avg10 field from one memory.pressure line
// (e.g. "some avg10=0.00 avg60=0.00 avg300=0.00 total=12345") — the only PSI
// field this file's gauges report; avg60/avg300/total aren't wired to a
// metric, so there's no reason to parse them. Returns ok=false if the line
// has no avg10 field.
func parseCgroupPSIAvg10(line string) (avg10 float64, ok bool) {
	for _, f := range strings.Fields(line) {
		k, v, hasEq := strings.Cut(f, "=")
		if hasEq && k == "avg10" {
			if p, err := strconv.ParseFloat(v, 64); err == nil {
				return p, true
			}
		}
	}
	return 0, false
}

// CgroupMemoryUsageRatio returns this process's cgroup memory.current as a
// fraction of memory.high (e.g. 0.9 = 90% of the soft throttle ceiling) —
// the same figures registerCgroupMemoryMetrics exports as
// cgroup_memory_current_bytes/cgroup_memory_high_bytes, exposed directly for
// in-process consumers (server/services.MemoryPressureNotifier) that need a
// live reading without querying back through the metrics pipeline. ok is
// false if memory.high is unset ("max") or either file can't be read.
func CgroupMemoryUsageRatio() (ratio float64, ok bool) {
	dir, err := cgroupMemoryDir()
	if err != nil {
		return 0, false
	}
	current, ok := readCgroupInt64(dir + "/memory.current")
	if !ok {
		return 0, false
	}
	high, ok := readCgroupInt64(dir + "/memory.high")
	if !ok || high == 0 {
		return 0, false
	}
	return float64(current) / float64(high), true
}

func registerCgroupMemoryMetrics() error {
	dir, err := cgroupMemoryDir()
	if err != nil {
		return err
	}

	meter := GetMeter()

	currentGauge, err := meter.Int64ObservableGauge("cgroup_memory_current_bytes",
		metric.WithDescription("memory.current: this process's cgroup's current memory usage"))
	if err != nil {
		return err
	}
	highGauge, err := meter.Int64ObservableGauge("cgroup_memory_high_bytes",
		metric.WithDescription("memory.high: soft throttle ceiling (0 reported when unset/\"max\")"))
	if err != nil {
		return err
	}
	maxGauge, err := meter.Int64ObservableGauge("cgroup_memory_max_bytes",
		metric.WithDescription("memory.max: hard OOM-kill ceiling (0 reported when unset/\"max\")"))
	if err != nil {
		return err
	}
	eventsHighCounter, err := meter.Int64ObservableCounter("cgroup_memory_events_high_total",
		metric.WithDescription("memory.events 'high': cumulative count of memory.high breaches triggering reclaim"))
	if err != nil {
		return err
	}
	eventsMaxCounter, err := meter.Int64ObservableCounter("cgroup_memory_events_max_total",
		metric.WithDescription("memory.events 'max': cumulative count of memory.max breaches"))
	if err != nil {
		return err
	}
	eventsOOMCounter, err := meter.Int64ObservableCounter("cgroup_memory_events_oom_total",
		metric.WithDescription("memory.events 'oom': cumulative count of OOM conditions in this cgroup"))
	if err != nil {
		return err
	}
	eventsOOMKillCounter, err := meter.Int64ObservableCounter("cgroup_memory_events_oom_kill_total",
		metric.WithDescription("memory.events 'oom_kill': cumulative count of processes killed by the OOM killer in this cgroup"))
	if err != nil {
		return err
	}
	pressureSomeAvg10, err := meter.Float64ObservableGauge("cgroup_memory_pressure_some_avg10",
		metric.WithDescription("memory.pressure 'some' avg10: %% of time in the last 10s at least one task stalled on memory"))
	if err != nil {
		return err
	}
	pressureFullAvg10, err := meter.Float64ObservableGauge("cgroup_memory_pressure_full_avg10",
		metric.WithDescription("memory.pressure 'full' avg10: %% of time in the last 10s every task stalled on memory simultaneously"))
	if err != nil {
		return err
	}

	_, err = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		if v, ok := readCgroupInt64(dir + "/memory.current"); ok {
			o.ObserveInt64(currentGauge, v)
		}
		if v, ok := readCgroupInt64(dir + "/memory.high"); ok {
			o.ObserveInt64(highGauge, v)
		}
		if v, ok := readCgroupInt64(dir + "/memory.max"); ok {
			o.ObserveInt64(maxGauge, v)
		}
		if data, err := os.ReadFile(dir + "/memory.events"); err == nil {
			events := parseCgroupEvents(data)
			o.ObserveInt64(eventsHighCounter, events["high"])
			o.ObserveInt64(eventsMaxCounter, events["max"])
			o.ObserveInt64(eventsOOMCounter, events["oom"])
			o.ObserveInt64(eventsOOMKillCounter, events["oom_kill"])
		}
		if data, err := os.ReadFile(dir + "/memory.pressure"); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if avg10, ok := parseCgroupPSIAvg10(line); ok {
					switch {
					case strings.HasPrefix(line, "some "):
						o.ObserveFloat64(pressureSomeAvg10, avg10)
					case strings.HasPrefix(line, "full "):
						o.ObserveFloat64(pressureFullAvg10, avg10)
					}
				}
			}
		}
		return nil
	}, currentGauge, highGauge, maxGauge, eventsHighCounter, eventsMaxCounter, eventsOOMCounter, eventsOOMKillCounter, pressureSomeAvg10, pressureFullAvg10)
	return err
}
