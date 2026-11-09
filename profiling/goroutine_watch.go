package profiling

import (
	"context"
	"runtime"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// Goroutine budget. A healthy process runs about goroutinesPerSession per live session on top of a
// fixed base (pprof dump: 615 goroutines with ~30 sessions, bounded 400-700 over a day). The
// per-session figure is deliberately above the measured ~20 so only a leak, not a busy day, trips
// it; BUG-034 was a real leak caught at 310-320 goroutines with only a few sessions.
const (
	goroutineBase        = 100
	goroutinesPerSession = 25

	// goroutineGrowthFactor is how far above the expected count a process may run before it is
	// "elevated"; goroutineSustainFor is how long it must stay elevated to count as a leak.
	goroutineGrowthFactor = 1.5
	goroutineSustainFor   = 30 * time.Minute
	// goroutineClearAfter is how long the count must stay under the limit before the sustain clock
	// resets, so a leak hovering around the limit is not forgiven by one low sample.
	goroutineClearAfter = 5 * time.Minute

	// HighGoroutineThreshold is the absolute floor of the hard ceiling: above it, or above twice
	// the session-scaled expectation when that is larger, the count is reported immediately
	// without waiting out goroutineSustainFor. ~95 sessions at the measured 20 goroutines each.
	HighGoroutineThreshold = 2000

	// GoroutineSampleInterval is how often the always-on watch samples the process.
	GoroutineSampleInterval = 30 * time.Second
)

// GoroutineState is the watch's verdict on the latest sample.
type GoroutineState string

const (
	// GoroutinesOK: at or below the session-scaled budget.
	GoroutinesOK GoroutineState = "ok"
	// GoroutinesElevated: above budget, not yet for goroutineSustainFor. Logged, not a health fault.
	GoroutinesElevated GoroutineState = "elevated"
	// GoroutinesSustained: above budget continuously for goroutineSustainFor. Likely a leak.
	GoroutinesSustained GoroutineState = "sustained"
	// GoroutinesOverCeiling: above the hard ceiling right now.
	GoroutinesOverCeiling GoroutineState = "over_ceiling"
)

// GoroutineVerdict is one sample's result.
type GoroutineVerdict struct {
	State       GoroutineState
	Count       int
	Sessions    int
	Expected    int           // base + per-session budget
	Limit       int           // Expected scaled by the growth factor
	Ceiling     int           // hard ceiling for this session count
	ElevatedFor time.Duration // how long Count has stayed above Limit
}

// Degraded reports whether /actuator/health should flag this verdict.
func (v GoroutineVerdict) Degraded() bool {
	return v.State == GoroutinesSustained || v.State == GoroutinesOverCeiling
}

// ExpectedGoroutines is the budget for a process with the given number of live sessions.
func ExpectedGoroutines(sessions int) int {
	return goroutineBase + goroutinesPerSession*max(sessions, 0)
}

func goroutineLimit(expected int) int { return int(float64(expected) * goroutineGrowthFactor) }

func goroutineCeiling(expected int) int { return max(HighGoroutineThreshold, 2*expected) }

// GoroutineWatch detects goroutine leaks by comparing the count against a budget derived from the
// live session count, so a large but healthy fleet does not alarm while a small process that keeps
// growing does. Safe for concurrent use; time is passed in, so tests need no clock.
type GoroutineWatch struct {
	mu           sync.Mutex
	elevatedFrom time.Time // zero when not elevated
	belowSince   time.Time // zero while above the limit
	last         GoroutineVerdict
}

// DefaultGoroutineWatch is the process-wide watch the server samples and /actuator/health reads.
var DefaultGoroutineWatch = &GoroutineWatch{}

// Observe records one sample taken at now and returns its verdict.
func (w *GoroutineWatch) Observe(now time.Time, goroutines, sessions int) GoroutineVerdict {
	expected := ExpectedGoroutines(sessions)
	v := GoroutineVerdict{
		State: GoroutinesOK, Count: goroutines, Sessions: sessions,
		Expected: expected, Limit: goroutineLimit(expected), Ceiling: goroutineCeiling(expected),
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if goroutines > v.Limit {
		w.belowSince = time.Time{}
		if w.elevatedFrom.IsZero() {
			w.elevatedFrom = now
		}
	} else if !w.elevatedFrom.IsZero() {
		if w.belowSince.IsZero() {
			w.belowSince = now
		}
		if now.Sub(w.belowSince) >= goroutineClearAfter {
			w.elevatedFrom, w.belowSince = time.Time{}, time.Time{}
		}
	}
	if !w.elevatedFrom.IsZero() {
		v.ElevatedFor = now.Sub(w.elevatedFrom)
		v.State = GoroutinesElevated
		if v.ElevatedFor >= goroutineSustainFor {
			v.State = GoroutinesSustained
		}
	}
	if goroutines > v.Ceiling {
		v.State = GoroutinesOverCeiling
	}
	w.last = v
	return v
}

// Known blind spot: the limit is 1.5x the budget (~37 goroutines per session against ~20 measured),
// so a slow leak that stays inside that headroom is not detected; the watch catches small-fleet
// leaks and gross growth, not drift.
//
// Last returns the most recent verdict, or an OK zero-sample before the first Observe.
func (w *GoroutineWatch) Last() GoroutineVerdict {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.last
}

// RunGoroutineWatch samples the process every interval until ctx ends, feeding w and logging
// state changes (and every goroutineSustainFor while a leak persists). sessions reports the live
// session count; nil means unknown and is treated as zero.
func RunGoroutineWatch(ctx context.Context, w *GoroutineWatch, interval time.Duration, sessions func() int) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	runGoroutineWatch(ctx, w, ticker.C, time.Now, runtime.NumGoroutine, sessions)
}

func runGoroutineWatch(ctx context.Context, w *GoroutineWatch, ticks <-chan time.Time, now func() time.Time, goroutines, sessions func() int) {
	prev := GoroutinesOK
	var lastLogged time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
		}
		n := 0
		if sessions != nil {
			n = sessions()
		}
		t := now()
		v := w.Observe(t, goroutines(), n)
		if v.State != prev || (v.Degraded() && t.Sub(lastLogged) >= goroutineSustainFor) {
			logGoroutineVerdict(v)
			lastLogged = t
		}
		prev = v.State
	}
}

func logGoroutineVerdict(v GoroutineVerdict) {
	fields := []any{"count", v.Count, "sessions", v.Sessions, "expected", v.Expected, "limit", v.Limit, "elevated_for", v.ElevatedFor.Round(time.Second)}
	switch v.State {
	case GoroutinesElevated:
		log.Warn("goroutine count above session-scaled budget", fields...)
	case GoroutinesSustained:
		log.Error("goroutine count above budget for a sustained period; likely a leak", fields...)
	case GoroutinesOverCeiling:
		log.Error("goroutine count over hard ceiling", append(fields, "ceiling", v.Ceiling)...)
	default:
		log.Info("goroutine count back within budget", fields...)
	}
}
