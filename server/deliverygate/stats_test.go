package deliverygate

import (
	"math"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/pkg/events"
)

// statsEnv drives a Gate's Stats on a fake clock: every step advances the clock
// and ticks the accumulator the way the writer goroutine would.
type statsEnv struct {
	t     *testing.T
	g     *Gate
	clk   *fakeClock
	flags *staticFlags
	f     func(*events.Event) bool
}

func newStatsEnv(t *testing.T, flagOn bool) *statsEnv {
	t.Helper()
	g, clk, _, flags := newTestGate(flagOn, hiddenReview, visibleSess)
	// Start on an hour boundary so bucket arithmetic in the tests is exact.
	clk.Advance(time.Hour - clk.Now().Sub(clk.Now().Truncate(time.Hour)))
	g.stats = NewStats(clk.Now)
	g.flags.swapHook = func(s FlagSettings) { g.stats.onSwap(s) }
	g.Flags().Reload() // first swap: primes the accumulator, not a flip
	return &statsEnv{t: t, g: g, clk: clk, flags: flags, f: g.PublishFilter()}
}

func (e *statsEnv) advance(d time.Duration) {
	e.clk.Advance(d)
	e.g.Stats().Tick(e.clk.Now())
}

func (e *statsEnv) setFlag(on bool) {
	e.flags.set(on)
	e.g.Flags().Reload()
}

func (e *statsEnv) hour(typ eventKind) {
	e.advance(30 * time.Minute)
	e.emit(typ)
	e.advance(30 * time.Minute)
}

type eventKind int

const (
	routineEvent eventKind = iota
	failureEvent
	needsHumanEvent
)

func (e *statsEnv) emit(k eventKind) {
	switch k {
	case routineEvent:
		e.f(notif("review:abc", tTaskComplete, nil))
	case failureEvent:
		e.f(notif("review:abc", tError, nil))
	case needsHumanEvent:
		e.f(notif("review:abc", tApproval, nil))
	}
}

func (e *statsEnv) snap() StatsSnapshot { return e.g.Stats().Snapshot(e.clk.Now()) }

func near(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.001 {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
}

func TestGateOnSeconds_ShouldCreditElapsedUsingTheSnapshotInForce_WhenFlagFlipsMidInterval(t *testing.T) {
	t.Parallel()
	e := newStatsEnv(t, true)
	e.advance(10 * time.Minute)
	e.advance(5 * time.Minute) // credited while on; the flip lands at the end of this interval
	e.setFlag(false)
	e.advance(10 * time.Minute)

	b := e.snap().Buckets
	if len(b) != 1 {
		t.Fatalf("buckets = %d, want 1", len(b))
	}
	if b[0].GateOnSeconds != 900 || b[0].UptimeSeconds != 1500 {
		t.Fatalf("gate_on=%d uptime=%d, want 900 and 1500", b[0].GateOnSeconds, b[0].UptimeSeconds)
	}
	if got := b[0].GateOnByKind["review"]; got != 900 {
		t.Fatalf("review gate_on = %d, want 900", got)
	}
}

func TestGateOnSeconds_ShouldSplitAnIntervalAtTheBucketBoundary_WhenTickSpansTwoHours(t *testing.T) {
	t.Parallel()
	e := newStatsEnv(t, true)
	e.advance(50 * time.Minute)
	e.advance(20 * time.Minute)

	b := e.snap().Buckets
	if len(b) != 2 || b[0].GateOnSeconds != 3000+600 || b[1].GateOnSeconds != 600 {
		t.Fatalf("buckets = %+v", b)
	}
}

func TestStatsSnapshot_ShouldCountOnlyGateOnTimeInRoutineBucketsSinceTheLastOffFlip_WhenOffFlipThenOnAgain(t *testing.T) {
	t.Parallel()
	e := newStatsEnv(t, true)
	for i := 0; i < 5; i++ {
		e.hour(routineEvent)
	}
	near(t, "streak before flip", e.snap().Soak.SoakStreakHours, 5)

	e.setFlag(false)
	e.advance(time.Minute)
	e.setFlag(true)
	// The bucket holding the flip and any routine events in it add nothing.
	e.emit(routineEvent)
	near(t, "streak right after the flip", e.snap().Soak.SoakStreakHours, 0)

	// The first hour after the flip is partly inside the flip bucket, so 25
	// loop hours give 24 whole routine buckets.
	for i := 0; i < 25; i++ {
		e.hour(routineEvent)
	}
	s := e.snap().Soak
	near(t, "streak after 24 routine gate-on hours", s.SoakStreakHours, 24)
	if s.LastOffFlipAt.IsZero() {
		t.Fatal("last_off_flip_at not recorded")
	}
}

func TestStatsSnapshot_ShouldNotCountFailureOnlyZeroTrafficOrGateOffHours_WhenComputingTheStreak(t *testing.T) {
	t.Parallel()
	e := newStatsEnv(t, true)
	e.hour(failureEvent)     // failure-only: exercises no suppression
	e.advance(1 * time.Hour) // zero traffic
	e.setFlag(false)
	e.hour(routineEvent) // routine traffic with the gate off: would_suppress, not suppressed
	e.setFlag(true)
	e.hour(routineEvent)

	s := e.snap().Soak
	if s.SoakStreakHours >= 1.01 {
		t.Fatalf("streak = %v, want only the one routine gate-on hour (or less)", s.SoakStreakHours)
	}
	if s.FailureDeliveredWhileOn != 0 || s.LastOffFlipAt.IsZero() {
		t.Fatalf("failure_delivered_while_on = %d (want 0: it predates the off flip), last_off_flip_at zero=%v",
			s.FailureDeliveredWhileOn, s.LastOffFlipAt.IsZero())
	}
}

func TestSoak_ShouldRequireAFailureDeliveryWhileOn_WhenTwentyFourRoutineGateOnHoursHaveNoFailure(t *testing.T) {
	t.Parallel()
	e := newStatsEnv(t, true)
	for i := 0; i < 24; i++ {
		e.hour(routineEvent)
	}
	s := e.snap().Soak
	near(t, "streak", s.SoakStreakHours, 24)
	if s.FailureDeliveredWhileOn != 0 || s.NeedsHumanDeliveredWhileOn != 0 {
		t.Fatalf("failure=%d needs_human=%d, want 0 and 0", s.FailureDeliveredWhileOn, s.NeedsHumanDeliveredWhileOn)
	}
}

func TestSoak_ShouldCountFailureAndNeedsHumanSeparatelyAndTheProbeApart_WhenOnlyOneClassOrOnlyProbesWereDelivered(t *testing.T) {
	t.Parallel()
	e := newStatsEnv(t, true)
	e.emit(failureEvent)
	s := e.snap().Soak
	if s.FailureDeliveredWhileOn != 1 || s.NeedsHumanDeliveredWhileOn != 0 {
		t.Fatalf("one error: failure=%d needs_human=%d", s.FailureDeliveredWhileOn, s.NeedsHumanDeliveredWhileOn)
	}
	probe := events.NewNotificationEvent("review:abc", "review:abc", "p1", int32(tApproval), 2,
		ProbeTitlePrefix+" gate check", "ignore", nil)
	if !e.f(probe) {
		t.Fatal("probe must still be delivered")
	}
	s = e.snap().Soak
	if s.NeedsHumanDeliveredWhileOn != 1 || s.ProbeDeliveredWhileOn != 1 {
		t.Fatalf("after probe: needs_human=%d probe=%d, want 1 and 1", s.NeedsHumanDeliveredWhileOn, s.ProbeDeliveredWhileOn)
	}
}

func TestStatsSnapshot_ShouldNotCountAnUnresolvedRoutineEventAsRoutineWhileOn_WhenSessionIsUnknown(t *testing.T) {
	t.Parallel()
	e := newStatsEnv(t, true)
	if !e.f(notif("no-such-session", tTaskComplete, nil)) {
		t.Fatal("unresolved must fail open")
	}
	b := e.snap().Buckets[0]
	if b.RoutineWhileOn != 0 || b.Seen != 0 {
		t.Fatalf("seen=%d routine_while_on=%d, want 0 and 0", b.Seen, b.RoutineWhileOn)
	}
	var unresolved int64
	for _, c := range b.Counters {
		if c.Counter == StatUnresolved {
			unresolved += c.Count
		}
	}
	if unresolved != 1 {
		t.Fatalf("unresolved counter = %d, want 1", unresolved)
	}
}

func TestStatsResponse_ShouldCarryKindEventsFlagHistoryAndProcessStart_WhenQueried(t *testing.T) {
	t.Parallel()
	e := newStatsEnv(t, true)
	e.emit(routineEvent)
	e.emit(failureEvent)
	e.setFlag(false)
	e.setFlag(true)

	s := e.snap()
	if s.EventsByKind24h["review"] != 2 {
		t.Fatalf("events_by_kind_24h = %v", s.EventsByKind24h)
	}
	if len(s.FlagHistory) != 2 || s.FlagHistory[0].Value || !s.FlagHistory[1].Value || s.FlagHistory[0].Scope != "global" {
		t.Fatalf("flag_history = %+v", s.FlagHistory)
	}
	if s.FlagHistory[0].Mutation != "SET_DISABLED" {
		t.Fatalf("mutation = %q", s.FlagHistory[0].Mutation)
	}
	b := s.Buckets[0]
	if b.Seen != 2 || b.WhileOn != 2 || b.RoutineWhileOn != 1 {
		t.Fatalf("seen=%d while_on=%d routine_while_on=%d, want 2 2 1", b.Seen, b.WhileOn, b.RoutineWhileOn)
	}
}

func TestStats_ShouldKeepOnly72BucketsAndBoundKeys_WhenRunForAWeek(t *testing.T) {
	t.Parallel()
	e := newStatsEnv(t, true)
	for i := 0; i < 24*7; i++ {
		e.hour(routineEvent)
		e.emit(failureEvent)
		e.emit(needsHumanEvent)
	}
	p := e.g.Stats().Persist()
	if len(p.Buckets) != BucketCount {
		t.Fatalf("buckets = %d, want %d", len(p.Buckets), BucketCount)
	}
	for _, b := range p.Buckets {
		if len(b.Counters) > PersistedKeyCeiling {
			t.Fatalf("bucket holds %d keys, ceiling %d", len(b.Counters), PersistedKeyCeiling)
		}
	}
}

func TestStats_ShouldMergeByHourStartSummingCountersAndClampingSecondsTo3600_WhenReloadedIntoALiveStats(t *testing.T) {
	t.Parallel()
	e := newStatsEnv(t, true)
	e.hour(routineEvent)
	p := e.g.Stats().Persist()

	e.g.Stats().Merge(p) // double the same document
	b := e.g.Stats().Persist().Buckets[0]
	if b.UptimeMs != 3600*1000 || b.GateOnMs != 3600*1000 {
		t.Fatalf("uptime=%d gate_on=%d ms, want clamped to one hour", b.UptimeMs, b.GateOnMs)
	}
	var suppressed int64
	for _, c := range b.Counters {
		if c.Counter == StatSuppressed {
			suppressed += c.Count
		}
	}
	if suppressed != 2 {
		t.Fatalf("suppressed after merge = %d, want 2", suppressed)
	}
}

func TestGate_ShouldWarnEnabledWithoutStats_WhenHandEditedOnWhileNoWriterRuns(t *testing.T) {
	t.Parallel()
	g, _, recs, flags := newTestGate(false, hiddenReview)
	flags.set(true)
	g.Flags().Reload() // a ticker read picks up a hand edit
	if got := countMsg(recs(), "delivery_gate_enabled_without_stats"); got != 1 {
		t.Fatalf("WARN count = %d, want 1", got)
	}

	g2, _, recs2, flags2 := newTestGate(false, hiddenReview)
	g2.Stats().SetWriterRunning(true)
	flags2.set(true)
	g2.Flags().Reload()
	if got := countMsg(recs2(), "delivery_gate_enabled_without_stats"); got != 0 {
		t.Fatalf("WARN with a running writer = %d, want 0", got)
	}
}
