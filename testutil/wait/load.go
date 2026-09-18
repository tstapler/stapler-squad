package wait

import (
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/load"
)

// maxLoadFactor caps how much a single wait can be stretched by observed
// system load, so a genuinely wedged condition still fails in bounded time
// instead of waiting indefinitely just because the machine is busy.
const maxLoadFactor = 8.0

// loadFactorCacheTTL bounds how often LoadFactor actually reads the system
// load average. The read itself is cheap (one syscall on Darwin/BSD, one
// /proc file on Linux) but this is called from every WaitForCondition/
// Eventually invocation across a large parallel test binary, so a short
// cache avoids redundant reads without meaningfully staling the value.
const loadFactorCacheTTL = 250 * time.Millisecond

var loadFactorCache struct { //nolint:gochecknoglobals
	mu        sync.Mutex
	value     float64
	expiresAt time.Time
}

// LoadFactor returns a multiplier for scaling test wait timeouts to current
// system load: 1.0 when the machine has full headroom (1-minute load average
// per core at or below 1), rising linearly with load average per core above
// that, capped at maxLoadFactor. Returns 1.0 -- never shrinks a timeout below
// what the test author chose -- if load average can't be read on this
// platform, so a read failure fails open rather than tightening a bound.
//
// This exists because a fixed wall-clock require.Eventually/WaitForCondition
// bound assumes the goroutines it's waiting on get scheduled promptly. That
// assumption breaks on a machine running other CPU-heavy work concurrently
// (a live production instance of this same service, another test run, a
// loaded CI runner) -- see BUG-103 and docs/explanation/test-io-storage-isolation.md.
func LoadFactor() float64 {
	loadFactorCache.mu.Lock()
	defer loadFactorCache.mu.Unlock()
	if time.Now().Before(loadFactorCache.expiresAt) {
		return loadFactorCache.value
	}

	factor := 1.0
	if avg, err := load.Avg(); err == nil {
		if perCore := avg.Load1 / float64(runtime.NumCPU()); perCore > factor {
			factor = perCore
		}
		if factor > maxLoadFactor {
			factor = maxLoadFactor
		}
	}
	loadFactorCache.value = factor
	loadFactorCache.expiresAt = time.Now().Add(loadFactorCacheTTL)
	return factor
}

// envScale reads STAPLER_SQUAD_TEST_TIMEOUT_SCALE, an explicit manual
// override multiplied on top of LoadFactor -- e.g. for a known-slow CI
// runner, or a developer who wants generous bounds regardless of measured
// load. Unset, empty, non-numeric, or non-positive values default to 1.0
// (no additional scaling).
func envScale() float64 {
	raw := os.Getenv("STAPLER_SQUAD_TEST_TIMEOUT_SCALE")
	if raw == "" {
		return 1.0
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v <= 0 {
		return 1.0
	}
	return v
}

// ScaleTimeout multiplies base by the current LoadFactor and any explicit
// STAPLER_SQUAD_TEST_TIMEOUT_SCALE override. Use this wherever a test would
// otherwise hardcode a wall-clock wait bound.
func ScaleTimeout(base time.Duration) time.Duration {
	return time.Duration(float64(base) * LoadFactor() * envScale())
}
