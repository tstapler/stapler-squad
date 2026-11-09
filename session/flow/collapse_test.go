package flow

import (
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

const (
	stReady    = domain.BacklogStatusReady
	stProg     = domain.BacklogStatusInProgress
	stPRPend   = domain.BacklogStatusPRPending
	stQueued   = domain.BacklogStatusQueued
	stReview   = domain.BacklogStatusReview
	stDoneStat = domain.BacklogStatusDone
)

var collapseT0 = time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)

// flipRun builds n alternating events a>b, b>a, ... for item, step apart from start.
func flipRun(item ItemID, a, b domain.BacklogStatus, start time.Time, n int, step time.Duration, seq *int64) []Event {
	out := make([]Event, 0, n)
	for i := 0; i < n; i++ {
		from, to := a, b
		if i%2 == 1 {
			from, to = b, a
		}
		*seq++
		out = append(out, Event{ItemID: item, From: from, To: to, At: start.Add(time.Duration(i) * step), Seq: *seq})
	}
	return out
}

func wideWindow(t *testing.T) FlowWindow {
	t.Helper()
	w, err := NewFlowWindowBounds(collapseT0.Add(-24*time.Hour), collapseT0.Add(30*24*time.Hour))
	require.NoError(t, err)
	return w
}

func thrashSpans(spans []Span) []ThrashSpan {
	var out []ThrashSpan
	for _, s := range spans {
		if ts, ok := s.(ThrashSpan); ok {
			out = append(out, ts)
		}
	}
	return out
}

func realSpans(spans []Span) []RealSpan {
	var out []RealSpan
	for _, s := range spans {
		if rs, ok := s.(RealSpan); ok {
			out = append(out, rs)
		}
	}
	return out
}

func fixtureNormalized(t *testing.T, item string) []Event {
	t.Helper()
	events, _ := Normalize(rawFromFixture(loadFixture[fixtureEvents](t, "events_thrash.json")))
	var out []Event
	for _, e := range events {
		if string(e.ItemID) == item {
			out = append(out, e)
		}
	}
	return out
}

func TestCollapse_should_EmitOneThrashSpan_When_217AlternatingFlipsOneMinuteApart(t *testing.T) {
	t.Parallel()
	var seq int64
	events := flipRun("a", stReady, stProg, collapseT0, 217, time.Minute, &seq)

	spans := Collapse(events, wideWindow(t), DefaultConfig())

	require.Len(t, spans, 1)
	ts := spans[0].(ThrashSpan)
	assert.Equal(t, 217, ts.Count())
	assert.Equal(t, CauseDequeueSpawnFailed, ts.Cause())
	assert.Equal(t, FlipPair{A: stReady, B: stProg}, ts.Pair())
	assert.Equal(t, stProg, ts.NetStatus(), "217 events ends on ready>in_progress")
}

func TestCollapse_should_ClassifyPrFixReopenByShape_When_NotesAreEmpty(t *testing.T) {
	t.Parallel()
	var seq int64
	events := flipRun("b", stPRPend, stProg, collapseT0, 10, 2*time.Minute, &seq)
	for _, e := range events {
		require.Empty(t, e.Note)
	}

	spans := Collapse(events, wideWindow(t), DefaultConfig())

	require.Len(t, spans, 1)
	assert.Equal(t, CausePRFixReopen, spans[0].(ThrashSpan).Cause())

	events = flipRun("q", stQueued, stProg, collapseT0, 8, time.Minute, &seq)
	spans = Collapse(events, wideWindow(t), DefaultConfig())
	require.Len(t, spans, 1)
	assert.Equal(t, CauseDequeueClaim, spans[0].(ThrashSpan).Cause())
}

func TestCollapse_should_MergeEpisodesAtOneHourGap_When_ThrashBFixture17To5(t *testing.T) {
	t.Parallel()
	w := wideWindow(t)

	b := fixtureNormalized(t, "thrash-b")
	cfg := DefaultConfig()
	merged := thrashSpans(Collapse(b, w, cfg))
	assert.Len(t, merged, 5)
	total := 0
	for _, s := range merged {
		total += s.Count()
	}
	assert.Equal(t, len(b)-len(realSpans(Collapse(b, w, cfg))), total, "episode events are exactly the non-real events")

	cfg.ThrashMergeGap = 0
	assert.Len(t, thrashSpans(Collapse(b, w, cfg)), 17)

	a := fixtureNormalized(t, "thrash-a")
	assert.Len(t, thrashSpans(Collapse(a, w, DefaultConfig())), 2)
}

func TestCollapse_should_RespectExactBoundaries_When_FiveVsSixEventsAnd1459vs1501Gaps(t *testing.T) {
	t.Parallel()
	w := wideWindow(t)
	cfg := DefaultConfig()
	var seq int64

	five := flipRun("i", stReady, stProg, collapseT0, 5, time.Minute, &seq)
	spans := Collapse(five, w, cfg)
	assert.Empty(t, thrashSpans(spans), "5 events is normal flow")
	assert.Len(t, realSpans(spans), 5)

	six := flipRun("i", stReady, stProg, collapseT0, 6, time.Minute, &seq)
	assert.Len(t, thrashSpans(Collapse(six, w, cfg)), 1, "6 events is one episode")

	// A 6-event run whose 3rd gap is 14m59s stays whole; 15m01s splits it into two 3-event runs.
	for _, c := range []struct {
		gap       time.Duration
		wantThras int
		wantReal  int
	}{{14*time.Minute + 59*time.Second, 1, 0}, {15*time.Minute + 1*time.Second, 0, 6}} {
		evs := flipRun("g", stReady, stProg, collapseT0, 6, time.Minute, &seq)
		for i := 3; i < 6; i++ {
			evs[i].At = evs[i].At.Add(c.gap - time.Minute) // gap between events 2 and 3 is exactly c.gap
		}
		spans := Collapse(evs, w, cfg)
		assert.Len(t, thrashSpans(spans), c.wantThras, "gap %s", c.gap)
		assert.Len(t, realSpans(spans), c.wantReal, "gap %s", c.gap)
	}

	// Two 6-event episodes: inter-episode gap 59m59s merges, 1h01m does not.
	for _, c := range []struct {
		gap  time.Duration
		want int
	}{{59*time.Minute + 59*time.Second, 1}, {time.Hour + time.Minute, 2}} {
		first := flipRun("m", stReady, stProg, collapseT0, 6, time.Minute, &seq)
		secondStart := first[5].At.Add(c.gap)
		second := flipRun("m", stReady, stProg, secondStart, 6, time.Minute, &seq)
		got := thrashSpans(Collapse(append(first, second...), w, cfg))
		assert.Len(t, got, c.want, "inter-episode gap %s", c.gap)
	}
}

func straddlingRun(seq *int64, windowStart time.Time) []Event {
	// 50 one-minute flips: first 40 minutes before windowStart, last 10 inside it.
	return flipRun("s", stPRPend, stProg, windowStart.Add(-40*time.Minute), 50, time.Minute, seq)
}

func TestCollapse_should_KeepRunWhole_When_EpisodeStraddlesWindowStartWithPad(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	start := collapseT0
	w, err := NewFlowWindowBounds(start, start.Add(24*time.Hour))
	require.NoError(t, err)
	var seq int64
	events := straddlingRun(&seq, start) // pad events included, as the reader supplies them

	spans := Collapse(events, w, cfg)

	require.Len(t, spans, 1)
	ts := spans[0].(ThrashSpan)
	assert.True(t, ts.StartsBeforeWindow())
	assert.Equal(t, 10, ts.Count(), "count clipped to in-window events")
	assert.Equal(t, 50, ts.UnclippedCount())
	assert.Empty(t, realSpans(spans), "no in-window flip counted as a real PR fix")
}

func TestCollapse_should_CountRemainderAsReal_When_PadEventsNotSupplied(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	start := collapseT0
	w, err := NewFlowWindowBounds(start, start.Add(24*time.Hour))
	require.NoError(t, err)
	var seq int64
	var inWindowOnly []Event
	for _, e := range straddlingRun(&seq, start) {
		if w.Contains(e.At) {
			inWindowOnly = append(inWindowOnly, e)
		}
	}
	require.Len(t, inWindowOnly, 10)

	// Negative test: with the remainder alone, an episode of 10 still classifies;
	// shrink it below ThrashMinFlips to show the truncation the pad prevents.
	short := inWindowOnly[len(inWindowOnly)-5:]
	spans := Collapse(short, w, cfg)

	assert.Empty(t, thrashSpans(spans))
	assert.Len(t, realSpans(spans), 5, "remainder under ThrashMinFlips would be counted as real rework")
}

func TestCollapse_should_KeepRealTransitions_When_ReviewBouncesPresent(t *testing.T) {
	t.Parallel()
	events := fixtureNormalized(t, "bounce-a")

	spans := Collapse(events, wideWindow(t), DefaultConfig())

	bounces := 0
	for _, s := range realSpans(spans) {
		if s.From() == stReview && s.To() == stProg {
			bounces++
		}
	}
	assert.Equal(t, 17, bounces)
	assert.Empty(t, thrashSpans(spans))
}

func TestCollapse_should_CarryNoteOnRealSpan_When_EventHasNote(t *testing.T) {
	t.Parallel()
	events := []Event{{ItemID: "n", From: stReview, To: stProg, At: collapseT0, Note: "auto-reopened after failed review verdict", Seq: 1}}

	spans := Collapse(events, wideWindow(t), DefaultConfig())

	require.Len(t, spans, 1)
	assert.Equal(t, "auto-reopened after failed review verdict", spans[0].(RealSpan).Note())
}

func TestCollapse_should_ReconcileTotals_When_RandomEventStreams(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewSource(42))
	cfg := DefaultConfig()
	pairs := []FlipPair{{stReady, stProg}, {stQueued, stProg}, {stPRPend, stProg}}
	w, err := NewFlowWindowBounds(collapseT0, collapseT0.Add(48*time.Hour))
	require.NoError(t, err)

	for round := 0; round < 200; round++ {
		var raw []RawEvent
		var seq int64
		cursor := collapseT0.Add(-3 * time.Hour)
		items := []ItemID{"x", "y", "z"}
		for _, item := range items {
			at := cursor.Add(time.Duration(rng.Intn(120)) * time.Minute)
			for n := rng.Intn(60) + 1; n > 0; n-- {
				p := pairs[rng.Intn(len(pairs))]
				from, to := p.A, p.B
				if rng.Intn(2) == 0 {
					from, to = to, from
				}
				if rng.Intn(25) == 0 {
					to = from // self-loop
				}
				if rng.Intn(10) == 0 {
					from, to = stProg, stReview
				}
				at = at.Add(time.Duration(rng.Intn(20)+1) * time.Minute * time.Duration(1+rng.Intn(3)/2))
				seq++
				raw = append(raw, RawEvent{ItemID: item, From: from, To: to, At: at.Format("2006-01-02 15:04:05.000000+00:00"), Seq: seq})
			}
		}
		events, dq := Normalize(raw)
		inWindowRaw, inWindowSelf := 0, 0
		for _, r := range raw {
			at, _ := ParseEventTime(r.At)
			if w.Contains(at) {
				inWindowRaw++
				if r.From == r.To {
					inWindowSelf++
				}
			}
		}
		require.GreaterOrEqual(t, dq.SelfLoops, inWindowSelf)

		spans := Collapse(events, w, cfg)

		real, collapsed := SpanTotals(spans)
		require.Equal(t, inWindowRaw, real+collapsed+inWindowSelf, "round %d", round)
	}
}

func TestCollapsePadDerivedFromConfig_should_Fail_When_PadNotThrashGapTimesMinFlipsPlusMerge(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	assert.Equal(t, 2*time.Hour+30*time.Minute, cfg.CollapsePad())
	assert.Equal(t, cfg.ThrashGap*time.Duration(cfg.ThrashMinFlips)+cfg.ThrashMergeGap, cfg.CollapsePad())

	cfg.ThrashGap = 30 * time.Minute
	assert.Equal(t, 4*time.Hour, cfg.CollapsePad())
	cfg.ThrashMinFlips, cfg.ThrashMergeGap = 4, 2*time.Hour
	assert.Equal(t, 4*time.Hour, cfg.CollapsePad())
}

func TestCollapse_should_ReconcileFixtureTotals_When_FixtureLoaded(t *testing.T) {
	t.Parallel()
	f := loadFixture[fixtureEvents](t, "events_thrash.json")
	events, dq := Normalize(rawFromFixture(f))
	w, err := NewFlowWindowBounds(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)

	real, collapsed := SpanTotals(Collapse(events, w, DefaultConfig()))

	assert.Equal(t, len(f.Events), real+collapsed+dq.SelfLoops)
}

// Both comparisons are strict (gap < limit), so a gap of exactly the limit is
// on the splitting side: 15:00 breaks a run, 1:00:00 keeps episodes apart.
func TestCollapse_should_SplitAtExactLimit_When_GapEqualsThrashGapOrMergeGap(t *testing.T) {
	t.Parallel()
	w := wideWindow(t)
	cfg := DefaultConfig()
	var seq int64

	run := flipRun("g", stReady, stProg, collapseT0, 6, time.Minute, &seq)
	for i := 3; i < 6; i++ {
		run[i].At = run[i].At.Add(cfg.ThrashGap - time.Minute) // gap between events 2 and 3 is exactly 15:00
	}
	spans := Collapse(run, w, cfg)
	assert.Empty(t, thrashSpans(spans), "an exact 15:00 gap splits the run into two 3-event runs, below the minimum")
	assert.Len(t, realSpans(spans), 6)

	first := flipRun("m", stReady, stProg, collapseT0, 6, time.Minute, &seq)
	second := flipRun("m", stReady, stProg, first[5].At.Add(cfg.ThrashMergeGap), 6, time.Minute, &seq)
	assert.Len(t, thrashSpans(Collapse(append(first, second...), w, cfg)), 2, "an exact 1h gap does not merge episodes")
}
