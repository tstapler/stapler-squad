package tmux

import "github.com/linkdata/deadlock"

import (
	"context"
	"sync"
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

	// Clear thresholds sit strictly below their alert-threshold counterparts,
	// giving checkPressure hysteresis: an episode that fires at the alert
	// threshold doesn't re-clear the instant a single event ages out of the
	// window, only once the metric drops meaningfully below where it fired.
	// Halving the alert threshold is a simple, tunable starting gap.
	spawnFailureClearThreshold = spawnFailureAlertThreshold / 2
	spawnRateClearThreshold    = spawnRateWarnThreshold / 2
	zombieClearThreshold       = zombieAlertThreshold / 2
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
	// Hysteresis state for checkPressure (fields below are read and written
	// only under alertMu):
	//   - episodeActive is true from the moment an episode's first alert fires
	//     until every metric drops below its clear threshold.
	//   - peakLevel is the highest ForkPressureLevel reached so far in the
	//     current episode; a re-alert only fires when the current level
	//     exceeds it (a genuine escalation), never on a same-level trickle.
	episodeActive bool
	peakLevel     ForkPressureLevel
	alertFns      []AlertFunc
	// alertWG tracks in-flight alert-dispatch goroutines spawned by checkPressure,
	// so tests can deterministically wait for them to finish (see
	// resetForkMonitor's forkMonitor.alertWG.Wait() in fork_metrics_test.go)
	// instead of sleeping.
	alertWG sync.WaitGroup
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
//
// wg is joined by server.Server.Shutdown() (backlog item
// 81e82fee-9528-4dc9-a513-1040b4dee2ec) so shutdown blocks until this
// goroutine has fully exited, not just been signaled via ctx.Done() — see
// the join at server/server.go's Shutdown().
func StartForkPressureLogger(ctx context.Context, interval time.Duration, logFn func(string, ...any), wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()
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

// forkPressureTransition is the pure result of nextForkPressureTransition:
// what checkPressure should fire (if anything) and the episode state to store.
type forkPressureTransition struct {
	fire      bool
	fireLevel ForkPressureLevel
	active    bool
	peak      ForkPressureLevel
}

// nextForkPressureTransition is the pure decision function behind checkPressure's
// hysteresis, mirroring server/services/memory_pressure_notifier.go's
// notified-flag shape: fire once entering an elevated state, fire again only
// on a genuine escalation (Warning -> Critical) within the same episode, fire
// an explicit ForkPressureOK clear once every metric drops below its separate,
// lower clear threshold (FR-4/AC4), and otherwise stay silent even as counts
// drift up and down above the alert threshold or in the clear/alert gap — the
// sustained-trickle case a per-count ratchet used to mis-fire on (FR-1/AC1).
// The notification record this feeds (buildForkPressureNotification) keeps a
// stable ID and NotificationType for the whole episode, so an escalation fire
// updates that record in place rather than creating a second one (AC2).
func nextForkPressureTransition(wasActive bool, peak ForkPressureLevel, stats ForkPressureStats, belowClearThresholds bool) forkPressureTransition {
	switch {
	case wasActive && belowClearThresholds:
		return forkPressureTransition{fire: true, fireLevel: ForkPressureOK, active: false, peak: ForkPressureOK}
	case !wasActive && stats.Level != ForkPressureOK:
		return forkPressureTransition{fire: true, fireLevel: stats.Level, active: true, peak: stats.Level}
	case wasActive && stats.Level > peak:
		return forkPressureTransition{fire: true, fireLevel: stats.Level, active: true, peak: stats.Level}
	default:
		return forkPressureTransition{active: wasActive, peak: peak}
	}
}

func checkPressure(now time.Time) {
	stats := snapshotAt(now)
	belowClearThresholds := stats.FailuresInWindow < spawnFailureClearThreshold &&
		stats.ZombiesInWindow < zombieClearThreshold &&
		stats.SpawnsInWindow < spawnRateClearThreshold

	forkMonitor.alertMu.Lock()
	t := nextForkPressureTransition(forkMonitor.episodeActive, forkMonitor.peakLevel, stats, belowClearThresholds)
	forkMonitor.episodeActive = t.active
	forkMonitor.peakLevel = t.peak
	var fns []AlertFunc
	if t.fire {
		forkMonitor.lastAlertAt = now
		fns = forkMonitor.alertFns
	}
	forkMonitor.alertMu.Unlock()

	if t.fire {
		dispatchAlert(t.fireLevel, stats, fns)
	}
}

// dispatchAlert spawns the alert-fns dispatch goroutine, tracked via alertWG so
// tests can deterministically wait for it (see waitAlertCount/resetForkMonitor
// in fork_metrics_test.go) instead of sleeping. Must be called with alertMu
// already released (fns is captured under the lock by the caller).
func dispatchAlert(level ForkPressureLevel, stats ForkPressureStats, fns []AlertFunc) {
	forkMonitor.alertWG.Add(1)
	go func() {
		defer forkMonitor.alertWG.Done()
		for _, fn := range fns {
			fn(level, stats)
		}
	}()
}
