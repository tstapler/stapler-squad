package profiling

import (
	"context"
	"runtime"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// Goroutine budget, measured 2026-11-09 on this codebase: a freshly started idle instance runs
// 149-156 goroutines; the live service ran 575-579 with 19 ACTIVE sessions (22 hibernated, 4 paused,
// 2 stopped and 2 failed hold none), about 23 per active session. The budget leaves real headroom
// over both: idle is ~75% of goroutineBase, and the live service ~67% of the 19-session budget.
const (
	goroutineBase        = 200
	goroutinesPerSession = 35

	// goroutinesPerSessionTypical is the measured per-session cost the drift check subtracts, so
	// opening or closing sessions does not look like growth.
	goroutinesPerSessionTypical = 25

	// goroutineGrowthFactor is how far above the budget a process may run before it is "elevated";
	// goroutineSustainFor is how long it must stay elevated to count as a leak.
	goroutineGrowthFactor = 1.5
	goroutineSustainFor   = 30 * time.Minute
	// goroutineClearAfter is how long the count must stay under the limit before the sustain clock
	// resets, so a leak hovering around the limit is not forgiven by one low sample.
	goroutineClearAfter = 5 * time.Minute

	// Drift check: least-squares slope of (count - typical per-session cost) over a trailing window,
	// sampled once a minute. It catches slow leaks that stay inside the 1.5x headroom.
	driftWindow      = 2 * time.Hour
	driftMinSpan     = time.Hour // history needed before judging
	driftMinSamples  = 30
	driftSampleEvery = time.Minute
	driftPerHour     = 40.0 // goroutines/hour of sustained rise
	driftMinRise     = 100.0
	driftMinRSquared = 0.8 // a steady climb, not a noisy plateau

	// HighGoroutineThreshold is the absolute floor of the hard ceiling: above it, or above twice
	// the session-scaled budget when that is larger, the count is reported immediately.
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
	// GoroutinesDrifting: a steady upward trend over the last hour or more, even though the count is
	// still within its limit. Likely a slow leak.
	GoroutinesDrifting GoroutineState = "drifting"
	// GoroutinesOverCeiling: above the hard ceiling right now.
	GoroutinesOverCeiling GoroutineState = "over_ceiling"
)

// GoroutineVerdict is one sample's result.
type GoroutineVerdict struct {
	State        GoroutineState
	Count        int
	Sessions     int
	Expected     int           // base + per-session budget
	Limit        int           // Expected scaled by the growth factor
	Ceiling      int           // hard ceiling for this session count
	ElevatedFor  time.Duration // how long Count has stayed above Limit
	DriftPerHour float64       // fitted slope of the session-adjusted count, goroutines/hour
}

// Degraded reports whether /actuator/health should flag this verdict.
func (v GoroutineVerdict) Degraded() bool {
	return v.State == GoroutinesSustained || v.State == GoroutinesDrifting || v.State == GoroutinesOverCeiling
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
	trend        []trendPoint
	last         GoroutineVerdict
}

type trendPoint struct {
	at       time.Time
	adjusted float64
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
	slope, drifting := w.observeTrend(now, float64(goroutines-goroutinesPerSessionTypical*max(sessions, 0)))
	v.DriftPerHour = slope
	if drifting && v.State != GoroutinesSustained {
		v.State = GoroutinesDrifting
	}
	if goroutines > v.Ceiling {
		v.State = GoroutinesOverCeiling
	}
	w.last = v
	return v
}

// observeTrend appends a once-a-minute sample, trims to driftWindow and reports the fitted slope
// (goroutines/hour) and whether it is a sustained, steady rise. Caller holds w.mu.
func (w *GoroutineWatch) observeTrend(now time.Time, adjusted float64) (slopePerHour float64, drifting bool) {
	if n := len(w.trend); n == 0 || now.Sub(w.trend[n-1].at) >= driftSampleEvery {
		w.trend = append(w.trend, trendPoint{now, adjusted})
	}
	cut := 0
	for cut < len(w.trend) && now.Sub(w.trend[cut].at) > driftWindow {
		cut++
	}
	w.trend = w.trend[cut:]
	n := len(w.trend)
	if n < driftMinSamples || w.trend[n-1].at.Sub(w.trend[0].at) < driftMinSpan {
		return 0, false
	}
	var sx, sy, sxx, sxy, syy float64
	t0 := w.trend[0].at
	for _, p := range w.trend {
		x, y := p.at.Sub(t0).Hours(), p.adjusted
		sx, sy, sxx, sxy, syy = sx+x, sy+y, sxx+x*x, sxy+x*y, syy+y*y
	}
	fn := float64(n)
	varX, varY, cov := sxx-sx*sx/fn, syy-sy*sy/fn, sxy-sx*sy/fn
	if varX <= 0 || varY <= 0 {
		return 0, false
	}
	slopePerHour = cov / varX
	r2 := cov * cov / (varX * varY)
	rise := slopePerHour * w.trend[n-1].at.Sub(t0).Hours()
	return slopePerHour, slopePerHour >= driftPerHour && rise >= driftMinRise && r2 >= driftMinRSquared
}

// Last returns the most recent verdict, or an OK zero-sample before the first Observe.
func (w *GoroutineWatch) Last() GoroutineVerdict {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.last
}

// RunGoroutineWatch samples the process every interval until ctx ends, feeding w and logging
// state changes (and every goroutineSustainFor while a leak persists). sessions reports how many
// sessions own goroutines and whether that is known yet; while it is not (the session poller is
// not populated), the tick is skipped rather than judged against a zero budget.
func RunGoroutineWatch(ctx context.Context, w *GoroutineWatch, interval time.Duration, sessions func() (int, bool)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	runGoroutineWatch(ctx, w, ticker.C, time.Now, runtime.NumGoroutine, sessions)
}

func runGoroutineWatch(ctx context.Context, w *GoroutineWatch, ticks <-chan time.Time, now func() time.Time, goroutines func() int, sessions func() (int, bool)) {
	prev := GoroutinesOK
	var lastLogged time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
		}
		n, known := sessions()
		if !known {
			continue
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
	fields := []any{"count", v.Count, "sessions", v.Sessions, "expected", v.Expected, "limit", v.Limit, "elevated_for", v.ElevatedFor.Round(time.Second), "drift_per_hour", int(v.DriftPerHour)}
	switch v.State {
	case GoroutinesElevated:
		log.Warn("goroutine count above session-scaled budget", fields...)
	case GoroutinesDrifting:
		log.Error("goroutine count rising steadily; likely a slow leak", fields...)
	case GoroutinesSustained:
		log.Error("goroutine count above budget for a sustained period; likely a leak", fields...)
	case GoroutinesOverCeiling:
		log.Error("goroutine count over hard ceiling", append(fields, "ceiling", v.Ceiling)...)
	default:
		log.Info("goroutine count back within budget", fields...)
	}
}
