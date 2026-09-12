package tmux

import "github.com/linkdata/deadlock"

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
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

// spawnRateClearThreshold/spawnFailureClearThreshold/zombieClearThreshold are the
// falling-edge counterparts to the entry thresholds above. The gap between enter and
// clear (hysteresis) exists so a count oscillating around the old single threshold
// (e.g. 118/122/119/121 spawns/window) doesn't flap the alert on every crossing —
// mirrors memoryPressureWarnRatio/ClearRatio's pattern (memory_pressure_notifier.go).
const (
	spawnRateClearThreshold    = 90 // spawns/window → warning clears
	spawnFailureClearThreshold = 5  // failures/window → critical clears
	zombieClearThreshold       = 5  // zombie children/window → critical clears
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
	// AlertID is a stable identifier for the current pressure episode (minted when
	// pressure first rises above OK, reused for every escalation within that episode,
	// and carried on the final "cleared" callback too) — only populated on the
	// ForkPressureStats passed to an AlertFunc callback, never on ForkPressureSnapshot's
	// point-in-time reads. Lets a notification consumer (server.go) update one record
	// in place across an episode instead of minting a fresh ID per call.
	AlertID string
	// Cleared is true only on the single callback fired when pressure returns to OK
	// after an active episode — distinguishes "episode ended" from "episode
	// entered/escalated" for consumers building a persistent status signal.
	Cleared bool
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
	// Hysteresis state for alert gating. All fields are read and written only
	// under alertMu.
	//
	// currentLevel is the latched (hysteretic) pressure level — distinct from a raw
	// snapshotAt() Level, which reflects only the entry thresholds. currentLevel only
	// rises when a raw level's entry threshold is crossed and only falls to OK when
	// warningActive and criticalActive have both cleared (dropped below their own,
	// lower clear thresholds) — see nextHystereticLevel.
	currentLevel ForkPressureLevel
	// warningActive/criticalActive are independent latches (Critical is driven by
	// failures/zombies, Warning by spawn rate — see nextHystereticLevel) so each can
	// clear on its own falling edge without resetting the other.
	warningActive  bool
	criticalActive bool
	// episodeID is the stable AlertFunc/notification ID for the current non-OK
	// episode, minted on entry and reused for every escalation within it. Empty
	// while currentLevel == ForkPressureOK.
	episodeID string
	alertFns  []AlertFunc
	// alertWG tracks in-flight alert-dispatch goroutines spawned by checkPressure,
	// so tests can deterministically wait for them to finish (see
	// waitForPendingAlerts in fork_metrics_test.go) instead of sleeping.
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

// nextHystereticLevel updates forkMonitor's warningActive/criticalActive latches from
// the raw window counts in stats and returns the resulting hysteretic level. Must be
// called with alertMu held.
//
// Each latch only rises on its entry threshold and only falls on its own, lower clear
// threshold — a count sitting in the gap between the two (the old flapping zone) leaves
// the latch exactly as it was. This is the fix for the trickle-flapping bug: the old
// code compared each new count to the count recorded at the last alert, which a slow,
// never-worsening trickle could still creep past on every window slide.
func nextHystereticLevel(stats ForkPressureStats) ForkPressureLevel {
	switch {
	case stats.FailuresInWindow >= spawnFailureAlertThreshold || stats.ZombiesInWindow >= zombieAlertThreshold:
		forkMonitor.criticalActive = true
	case stats.FailuresInWindow < spawnFailureClearThreshold && stats.ZombiesInWindow < zombieClearThreshold:
		forkMonitor.criticalActive = false
	}

	switch {
	case stats.SpawnsInWindow >= spawnRateWarnThreshold:
		forkMonitor.warningActive = true
	case stats.SpawnsInWindow < spawnRateClearThreshold:
		forkMonitor.warningActive = false
	}

	switch {
	case forkMonitor.criticalActive:
		return ForkPressureCritical
	case forkMonitor.warningActive:
		return ForkPressureWarning
	default:
		return ForkPressureOK
	}
}

// episodeTransition decides whether the level change from current to next is
// user-visible (a genuine worsening, or a full clear back to OK) and, if so, the
// episode ID to report and whether it clears. Must be called with alertMu held;
// mutates forkMonitor.episodeID for entry/clear transitions.
func episodeTransition(current, next ForkPressureLevel) (alertID string, fire, cleared bool) {
	switch {
	case next == current:
		return "", false, false
	case next == ForkPressureOK:
		// Full clear, regardless of which level it fell from.
		id := forkMonitor.episodeID
		forkMonitor.episodeID = ""
		return id, true, true
	case current == ForkPressureOK:
		id := uuid.New().String()
		forkMonitor.episodeID = id
		return id, true, false
	case next < current:
		// Partial de-escalation that's still elevated (e.g. Critical -> Warning
		// without fully clearing) — update the latched level silently, no alert.
		return "", false, false
	default:
		// Escalation within the active episode — reuse its ID so the notification
		// consumer updates the same record instead of minting a new one.
		return forkMonitor.episodeID, true, false
	}
}

func checkPressure(now time.Time) {
	stats := snapshotAt(now)

	forkMonitor.alertMu.Lock()
	current := forkMonitor.currentLevel
	next := nextHystereticLevel(stats)
	forkMonitor.currentLevel = next
	alertID, fire, cleared := episodeTransition(current, next)
	if !fire {
		forkMonitor.alertMu.Unlock()
		return
	}
	forkMonitor.lastAlertAt = now
	fns := forkMonitor.alertFns
	forkMonitor.alertMu.Unlock()

	stats.Level = next
	stats.AlertID = alertID
	stats.Cleared = cleared

	forkMonitor.alertWG.Add(1)
	go func() {
		defer forkMonitor.alertWG.Done()
		for _, fn := range fns {
			fn(stats.Level, stats)
		}
	}()
}
