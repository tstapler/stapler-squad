package tmux

import "github.com/linkdata/deadlock"

import (
	"context"
	"sync/atomic"
	"time"
)

// ForkPressureLevel describes the current subprocess pressure state.
type ForkPressureLevel int

const (
	ForkPressureOK       ForkPressureLevel = iota
	ForkPressureWarning                    // spawn rate elevated
	ForkPressureCritical                   // spawn failures detected
)

func (l ForkPressureLevel) String() string {
	switch l {
	case ForkPressureWarning:
		return "warning"
	case ForkPressureCritical:
		return "critical"
	default:
		return "ok"
	}
}

const (
	forkPressureWindow         = 30 * time.Second
	spawnFailureAlertThreshold = 10  // failures/window → critical
	spawnRateWarnThreshold     = 120 // spawns/window → warning (4/s avg)
	zombieAlertThreshold       = 10  // zombie children/window → alert
)

// ForkPressureStats is a point-in-time snapshot of fork pressure metrics.
type ForkPressureStats struct {
	TotalSpawns      int64
	TotalFailures    int64
	TotalZombies     int64
	SpawnsInWindow   int64
	FailuresInWindow int64
	ZombiesInWindow  int64
	WindowDuration   time.Duration
	Level            ForkPressureLevel
	LastAlertAt      time.Time
}

// AlertFunc is called when fork pressure crosses a threshold.
type AlertFunc func(level ForkPressureLevel, stats ForkPressureStats)

// timestampRing is a fixed-size ring buffer for counting events in a sliding window.
type timestampRing struct {
	mu   deadlock.Mutex
	buf  []time.Time
	head int
}

func newTimestampRing(capacity int) *timestampRing {
	return &timestampRing{buf: make([]time.Time, capacity)}
}

func (r *timestampRing) record(now time.Time) {
	r.mu.Lock()
	r.buf[r.head] = now
	r.head = (r.head + 1) % len(r.buf)
	r.mu.Unlock()
}

func (r *timestampRing) countSince(cutoff time.Time) int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int64
	for _, t := range r.buf {
		if !t.IsZero() && t.After(cutoff) {
			n++
		}
	}
	return n
}

// spawnEntry records the origin description for a live child process.
type spawnEntry struct {
	Description string
	StartedAt   time.Time
}

// spawnRegistry tracks live child PIDs so zombie detection can log which component
// is responsible. Keyed by PID; entries added via TrackChildPID after cmd.Start()
// and removed via UntrackChildPID after cmd.Wait().
var spawnRegistry struct {
	mu      deadlock.Mutex
	entries map[int]spawnEntry
}

func init() {
	spawnRegistry.entries = make(map[int]spawnEntry)
}

// TrackChildPID registers a child PID with a human-readable description so zombie
// alerts can identify which component failed to call Wait(). Call after cmd.Start().
// description should identify the component and purpose, e.g.:
//
//	"tmux control-mode session=my-session"
//	"tmux registry control-mode socket=/tmp/tmux.sock"
func TrackChildPID(pid int, description string) {
	spawnRegistry.mu.Lock()
	spawnRegistry.entries[pid] = spawnEntry{Description: description, StartedAt: time.Now()}
	spawnRegistry.mu.Unlock()
}

// UntrackChildPID removes a PID from the registry. Call after cmd.Wait() returns.
func UntrackChildPID(pid int) {
	spawnRegistry.mu.Lock()
	delete(spawnRegistry.entries, pid)
	spawnRegistry.mu.Unlock()
}

// LookupChildPID returns the description and start time for a tracked PID.
// Returns ("unknown", zero, false) if the PID was not registered.
func LookupChildPID(pid int) (description string, startedAt time.Time, ok bool) {
	spawnRegistry.mu.Lock()
	e, ok := spawnRegistry.entries[pid]
	spawnRegistry.mu.Unlock()
	if !ok {
		return "unknown", time.Time{}, false
	}
	return e.Description, e.StartedAt, true
}

// forkMonitor is the process-wide fork pressure monitor.
var forkMonitor = struct {
	totalSpawns   atomic.Int64
	totalFailures atomic.Int64
	totalZombies  atomic.Int64
	spawnRing     *timestampRing
	failureRing   *timestampRing
	zombieRing    *timestampRing
	alertMu       deadlock.Mutex
	lastAlertAt   time.Time
	// Baseline fields for condition-change gating (FR-1, FR-2).
	// All three are read and written only under alertMu.
	lastAlertZombieCount  int64
	lastAlertFailureCount int64
	lastAlertLevel        ForkPressureLevel
	alertFns              []AlertFunc
}{
	spawnRing:   newTimestampRing(int(forkPressureWindow/time.Second) * 5),
	failureRing: newTimestampRing(256),
	zombieRing:  newTimestampRing(64),
}

// RegisterForkPressureAlert registers fn to be called when fork pressure crosses a threshold.
// Safe to call from multiple goroutines before any subprocess spawning begins.
func RegisterForkPressureAlert(fn AlertFunc) {
	forkMonitor.alertMu.Lock()
	defer forkMonitor.alertMu.Unlock()
	forkMonitor.alertFns = append(forkMonitor.alertFns, fn)
}

// ForkPressureSnapshot returns a point-in-time snapshot of fork pressure metrics.
func ForkPressureSnapshot() ForkPressureStats {
	return snapshotAt(time.Now())
}

// StartForkPressureLogger starts a background goroutine that logs fork pressure stats periodically.
func StartForkPressureLogger(ctx context.Context, interval time.Duration, logFn func(string, ...any)) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s := ForkPressureSnapshot()
				if s.SpawnsInWindow > 0 || s.FailuresInWindow > 0 || s.ZombiesInWindow > 0 {
					logFn("[ForkPressure] window=%ds spawns=%d failures=%d zombies=%d level=%s total(spawns=%d failures=%d zombies=%d)",
						int(forkPressureWindow.Seconds()),
						s.SpawnsInWindow, s.FailuresInWindow, s.ZombiesInWindow,
						s.Level,
						s.TotalSpawns, s.TotalFailures, s.TotalZombies)
				}
			}
		}
	}()
}

func recordSpawn(now time.Time) {
	forkMonitor.totalSpawns.Add(1)
	forkMonitor.spawnRing.record(now)
	checkPressure(now)
}

func recordFailure(now time.Time) {
	forkMonitor.totalFailures.Add(1)
	forkMonitor.failureRing.record(now)
	checkPressure(now)
}

// RecordZombieProcess records detection of a zombie child process (Z state in ps).
// sessionName is the comm field from ps (process name). The spawn registry is checked
// to include the originating component in the log message.
func RecordZombieProcess(pid int, sessionName string, warnFn func(string, ...any)) {
	now := time.Now()
	forkMonitor.totalZombies.Add(1)
	forkMonitor.zombieRing.record(now)
	if warnFn != nil {
		if desc, startedAt, ok := LookupChildPID(pid); ok {
			age := now.Sub(startedAt).Truncate(time.Millisecond)
			warnFn("[ForkPressure] zombie child detected: pid=%d comm=%q origin=%q age=%v (total=%d)",
				pid, sessionName, desc, age, forkMonitor.totalZombies.Load())
		} else {
			warnFn("[ForkPressure] zombie child detected: pid=%d comm=%q origin=unregistered (total=%d)",
				pid, sessionName, forkMonitor.totalZombies.Load())
		}
	}
	checkPressure(now)
}

func snapshotAt(now time.Time) ForkPressureStats {
	cutoff := now.Add(-forkPressureWindow)
	spawns := forkMonitor.spawnRing.countSince(cutoff)
	failures := forkMonitor.failureRing.countSince(cutoff)
	zombies := forkMonitor.zombieRing.countSince(cutoff)

	forkMonitor.alertMu.Lock()
	lastAlert := forkMonitor.lastAlertAt
	forkMonitor.alertMu.Unlock()

	level := ForkPressureOK
	if failures >= spawnFailureAlertThreshold || zombies >= zombieAlertThreshold {
		level = ForkPressureCritical
	} else if spawns >= spawnRateWarnThreshold {
		level = ForkPressureWarning
	}

	return ForkPressureStats{
		TotalSpawns:      forkMonitor.totalSpawns.Load(),
		TotalFailures:    forkMonitor.totalFailures.Load(),
		TotalZombies:     forkMonitor.totalZombies.Load(),
		SpawnsInWindow:   spawns,
		FailuresInWindow: failures,
		ZombiesInWindow:  zombies,
		WindowDuration:   forkPressureWindow,
		Level:            level,
		LastAlertAt:      lastAlert,
	}
}

func checkPressure(now time.Time) {
	stats := snapshotAt(now)
	if stats.Level == ForkPressureOK {
		// Only pay the mutex cost when we actually need to reset the baseline.
		// Read lastAlertLevel under the lock to avoid data races.
		forkMonitor.alertMu.Lock()
		if forkMonitor.lastAlertLevel != ForkPressureOK {
			// Condition cleared — reset baseline so next re-occurrence fires a fresh alert (FR-5).
			forkMonitor.lastAlertZombieCount = 0
			forkMonitor.lastAlertFailureCount = 0
			forkMonitor.lastAlertLevel = ForkPressureOK
		}
		forkMonitor.alertMu.Unlock()
		return
	}

	forkMonitor.alertMu.Lock()
	// Condition-change check: worsened means strictly higher counts OR level escalated.
	// We use strict > (not >=) intentionally: if the ring-buffer count drops due to
	// entry expiry and then rises again, we only re-alert when it exceeds the baseline
	// set at the last alert — not just when it equals it. This prevents re-alerts on
	// oscillation around the threshold boundary.
	worsened := stats.Level > forkMonitor.lastAlertLevel ||
		stats.ZombiesInWindow > forkMonitor.lastAlertZombieCount ||
		stats.FailuresInWindow > forkMonitor.lastAlertFailureCount

	if !worsened {
		// Conditions unchanged — suppress. Re-alerts only fire when the situation
		// genuinely worsens (higher count or escalated level), never on cooldown expiry alone.
		forkMonitor.alertMu.Unlock()
		return
	}
	// Conditions worsened — alert immediately.
	forkMonitor.lastAlertAt = now
	forkMonitor.lastAlertZombieCount = stats.ZombiesInWindow
	forkMonitor.lastAlertFailureCount = stats.FailuresInWindow
	forkMonitor.lastAlertLevel = stats.Level
	fns := forkMonitor.alertFns
	forkMonitor.alertMu.Unlock()

	go func() {
		for _, fn := range fns {
			fn(stats.Level, stats)
		}
	}()
}
