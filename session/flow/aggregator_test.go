package flow

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

func (f *fakeSrc) sources() Sources {
	return Sources{Events: f, Verdicts: f, Stuck: f, Items: f}
}

func summarize30(t *testing.T, f *fakeSrc, opts ...AggregatorOption) *FlowSummary {
	t.Helper()
	agg := NewAggregator(f.sources(), fixtureClock, opts...)
	s, err := agg.Summary(context.Background(), FlowQuery{Window: WindowSpec{Days: 30}})
	require.NoError(t, err)
	return s
}

func causeCount(t *testing.T, s *FlowSummary, cause string) int {
	t.Helper()
	for _, r := range s.Causes.Rework {
		if r.Cause == cause {
			return r.Count.Value
		}
	}
	t.Fatalf("cause %q not in causes section", cause)
	return 0
}

func TestAggregator_should_ReturnSectionedSummary_When_ThrashFixtureAndFixedClock(t *testing.T) {
	t.Parallel()
	s := summarize30(t, fixtureSrc(t))

	for _, sec := range []SectionMeta{s.Tiles.Meta, s.Funnel.Meta, s.Causes.Meta, s.Review.Meta, s.Stuck.Meta, s.Anomalies.Meta, s.Trends.Meta, s.Notes.Meta} {
		assert.Equal(t, SectionOK, sec.Status)
	}
	assert.Equal(t, SectionEmpty, s.PR.Meta.Status, "the fixture has no real PR-fix cycle (thrash-b collapses)")
	assert.Equal(t, fixtureClock.T, s.Meta.AsOf)
	assert.Equal(t, fixtureClock.T.AddDate(0, 0, -30), s.Meta.WindowStart)
	assert.Equal(t, fixtureClock.T, s.Meta.WindowEnd)

	assert.Equal(t, 2, s.Anomalies.ThrashItems.Value, "thrash-a and thrash-b")
	byID := map[ItemID]ThrashItemRow{}
	for _, r := range s.Anomalies.Items.Items {
		byID[r.ItemID] = r
	}
	require.Contains(t, byID, ItemID("thrash-a"))
	assert.Equal(t, 2, byID["thrash-a"].Episodes)
	assert.Equal(t, 279, byID["thrash-a"].Events)
	assert.Equal(t, 5, byID["thrash-b"].Episodes)
	assert.NotEmpty(t, byID["thrash-a"].Title)

	assert.Equal(t, 2, s.Tiles.Merged.Value, "tz-a and bounce-a merge")
	assert.Equal(t, s.Notes.RawEvents, s.Notes.RealSpans+s.Notes.CollapsedEvents, "raw reconciles with collapsed counts")
	assert.Equal(t, 462, s.Notes.RawEvents, "464 fixture rows minus 2 self-loops")
	assert.Equal(t, 17, causeCount(t, s, "review_fail")+causeCount(t, s, "bounce"))
	assert.Contains(t, s.Meta.Estimated, "pr_fix_cause:unavailable")
	assert.Contains(t, s.Meta.Estimated, "stuck_history:lower_bound")
	assert.Contains(t, s.Meta.Estimated, "deadlock_candidates:heuristic")
	assert.Greater(t, s.Meta.RowCounts.Events, 0)
}

func TestAggregator_should_FailOnlyReviewSection_When_VerdictSourceErrors(t *testing.T) {
	t.Parallel()
	f := fixtureSrc(t)
	f.verdictsErr = errors.New("verdict table locked")
	s := summarize30(t, f)

	assert.Equal(t, SectionFailed, s.Review.Meta.Status)
	assert.Contains(t, s.Review.Meta.Error, "verdict table locked")
	for _, sec := range []SectionMeta{s.Tiles.Meta, s.Funnel.Meta, s.Causes.Meta, s.Stuck.Meta, s.Anomalies.Meta, s.Notes.Meta} {
		assert.Equal(t, SectionOK, sec.Status)
	}
	assert.NotEmpty(t, s.Meta.Estimated, "heuristic metrics are still listed")
	assert.Equal(t, SectionFailed, s.Trends.Meta.Status, "verdict mix needs verdicts")
}

func TestAggregator_should_MarkReviewPartial_When_StuckSourceErrors(t *testing.T) {
	t.Parallel()
	f := fixtureSrc(t)
	f.stuckErr = errors.New("stuck read failed")
	s := summarize30(t, f)

	assert.Equal(t, SectionPartial, s.Review.Meta.Status, "cap exhaustion unknown but verdicts are fine")
	assert.Equal(t, SectionFailed, s.Stuck.Meta.Status)
	assert.Equal(t, SectionOK, s.Tiles.Meta.Status)
}

func TestAggregator_should_RecomputeInsteadOfServingCache_When_PreviousSummaryHadFailedSection(t *testing.T) {
	t.Parallel()
	f := fixtureSrc(t)
	f.eventsErr = errors.New("db down")
	agg := NewAggregator(f.sources(), fixtureClock)
	q := FlowQuery{Window: WindowSpec{Days: 30}}

	first, err := agg.Summary(context.Background(), q)
	require.NoError(t, err)
	require.Equal(t, SectionFailed, first.Tiles.Meta.Status)
	f.eventsErr = nil
	second, err := agg.Summary(context.Background(), q)

	require.NoError(t, err)
	assert.Equal(t, SectionOK, second.Tiles.Meta.Status, "a degraded result must not be served from cache")
	third, err := agg.Summary(context.Background(), q)
	require.NoError(t, err)
	assert.Same(t, second, third, "a healthy result is still cached")
}

func TestAggregator_should_FailEventSections_When_EventSourceErrors(t *testing.T) {
	t.Parallel()
	f := fixtureSrc(t)
	f.eventsErr = errors.New("db down")
	s := summarize30(t, f)

	for _, sec := range []SectionMeta{s.Tiles.Meta, s.Funnel.Meta, s.Causes.Meta, s.PR.Meta, s.Anomalies.Meta, s.Notes.Meta} {
		assert.Equal(t, SectionFailed, sec.Status)
		assert.Contains(t, sec.Error, "db down")
	}
	assert.Equal(t, SectionOK, s.Review.Meta.Status)
	assert.Equal(t, SectionOK, s.Stuck.Meta.Status)
}

func TestAggregator_should_EmitTrendsMatchingTileTotals_When_30DayWindow(t *testing.T) {
	t.Parallel()
	s := summarize30(t, fixtureSrc(t))

	require.Len(t, s.Trends.Throughput, 5, "five ISO weeks cover the 30-day window")
	var merged, verdicts, rework int
	for _, b := range s.Trends.Throughput {
		assert.Equal(t, time.Monday, b.WeekStart.Weekday())
		merged += b.N
	}
	assert.Equal(t, s.Tiles.Merged.Value, merged)
	assert.Equal(t, merged, s.Trends.ThroughputTotal)

	require.Len(t, s.Trends.VerdictMix, 5)
	for _, b := range s.Trends.VerdictMix {
		cells := 0
		for _, c := range b.Cells {
			cells += c.Count
		}
		assert.Equal(t, b.N, cells, "bucket N equals the sum of its cells")
		verdicts += b.N
	}
	assert.Equal(t, s.Review.Total.Value, verdicts)

	var causeTotal int
	for _, r := range s.Causes.Rework {
		causeTotal += r.Count.Value
	}
	for _, b := range s.Trends.Rework {
		rework += b.N
		var by int
		for _, c := range b.ByCause {
			by += c.Count
		}
		assert.Equal(t, b.N, by)
	}
	assert.Equal(t, causeTotal, rework)
	assert.Equal(t, rework, s.Trends.ReworkTotal)
	assert.Positive(t, rework)
}

func TestAggregator_should_KeepThrashOutOfDwellTotals_When_ThrashBPresent(t *testing.T) {
	t.Parallel()
	f := fixtureSrc(t)
	s := summarize30(t, f)

	naivePRFix := 0
	for _, e := range f.events {
		if e.ItemID == "thrash-b" && e.From == domain.BacklogStatusPRPending && e.To == domain.BacklogStatusInProgress {
			naivePRFix++
		}
	}
	require.Greater(t, naivePRFix, 20)
	assert.Less(t, causeCount(t, s, "pr_fix"), naivePRFix/2, "collapsed flips are not PR-fix reopens")
	assert.Positive(t, s.Funnel.Thrash.Stats.N, "thrash dwell lives in its own bucket")
	for _, st := range s.Funnel.Stages {
		if st.Status == string(domain.BacklogStatusPRPending) {
			assert.Less(t, st.Dwell.Stats.N, naivePRFix/2)
		}
	}
	assert.Equal(t, s.Anomalies.ThrashItems.Value, 2)
}

func TestAggregator_should_ReturnListPageTotals_When_35ThrashItemsAndLimitClamped(t *testing.T) {
	t.Parallel()
	f := newFakeSrc()
	f.addThrashItems(35, fixtureClock.T.AddDate(0, 0, -10))
	agg := NewAggregator(f.sources(), fixtureClock)
	ctx := context.Background()

	s, err := agg.Summary(ctx, FlowQuery{Window: WindowSpec{Days: 30}, Sections: []SectionName{SectionAnomalies}})
	require.NoError(t, err)
	p := s.Anomalies.Items
	assert.Equal(t, 35, p.TotalCount)
	assert.Equal(t, 10, p.ReturnedCount)
	assert.Len(t, p.Items, 10)
	assert.True(t, p.HasMore)
	assert.False(t, p.Truncated)
	assert.Equal(t, 35, s.Anomalies.ThrashItems.Value)

	s, err = agg.Summary(ctx, FlowQuery{Window: WindowSpec{Days: 30}, Sections: []SectionName{SectionAnomalies}, ListLimit: 200})
	require.NoError(t, err)
	p = s.Anomalies.Items
	assert.Equal(t, 35, p.ReturnedCount)
	assert.False(t, p.HasMore)
	assert.True(t, p.Truncated, "requested 200 was clamped to 50")
}

func TestAggregator_should_SkipUnselectedSectionsAndCompact_When_SectionsAndCompactSet(t *testing.T) {
	t.Parallel()
	f := fixtureSrc(t)
	agg := NewAggregator(f.sources(), fixtureClock)
	ctx := context.Background()

	s, err := agg.Summary(ctx, FlowQuery{Window: WindowSpec{Days: 30}, Sections: []SectionName{SectionTiles}})
	require.NoError(t, err)
	assert.Equal(t, SectionOK, s.Tiles.Meta.Status)
	for _, sec := range []SectionMeta{s.Funnel.Meta, s.Causes.Meta, s.Review.Meta, s.PR.Meta, s.Stuck.Meta, s.Anomalies.Meta, s.Trends.Meta, s.Notes.Meta} {
		assert.Equal(t, SectionSkipped, sec.Status)
	}
	assert.Equal(t, 1, f.called("Events"))
	for _, name := range []string{"Verdicts", "AgentDoneClaims", "StuckIntervals", "Items", "Stages", "LastEventsBefore"} {
		assert.Zero(t, f.called(name), name)
	}

	_, err = agg.Summary(ctx, FlowQuery{Window: WindowSpec{Days: 30}, Sections: []SectionName{"bogus"}})
	require.Error(t, err)
	for _, name := range []string{"tiles", "funnel", "causes", "review", "pr", "stuck", "anomalies", "trends", "notes"} {
		assert.Contains(t, err.Error(), name)
	}

	full, err := agg.Summary(ctx, FlowQuery{Window: WindowSpec{Days: 30}})
	require.NoError(t, err)
	compact, err := agg.Summary(ctx, FlowQuery{Window: WindowSpec{Days: 30}, Compact: true})
	require.NoError(t, err)
	assert.NotEmpty(t, full.Trends.Throughput)
	assert.Empty(t, compact.Trends.Throughput)
	assert.Empty(t, compact.Trends.VerdictMix)
	assert.Empty(t, compact.Trends.Rework)
	assert.Equal(t, full.Trends.ThroughputTotal, compact.Trends.ThroughputTotal, "totals are kept")
	assert.Equal(t, full.Trends.VerdictMixTotal, compact.Trends.VerdictMixTotal)
	assert.LessOrEqual(t, compact.Review.Deadlocks.ReturnedCount, 3)
	assert.Equal(t, full.Review.Deadlocks.TotalCount, compact.Review.Deadlocks.TotalCount)
}

func TestAggregator_should_ComputeUnderBudget_When_10kSyntheticEventsFakeSources(t *testing.T) {
	t.Parallel()
	f := newFakeSrc()
	start := fixtureClock.T.AddDate(0, 0, -20)
	for i := 0; i < 1000; i++ {
		id := ItemID(fmt.Sprintf("it-%04d", i))
		f.items = append(f.items, ItemMeta{ID: id, Title: string(id), Status: domain.BacklogStatusDone})
		path := []domain.BacklogStatus{
			domain.BacklogStatusIdea, domain.BacklogStatusReady, domain.BacklogStatusInProgress, domain.BacklogStatusReview,
			domain.BacklogStatusPRPending, domain.BacklogStatusDone, domain.BacklogStatusInProgress, domain.BacklogStatusReview,
			domain.BacklogStatusPRPending, domain.BacklogStatusDone,
		}
		for k := 1; k < len(path); k++ {
			f.events = append(f.events, Event{ItemID: id, From: path[k-1], To: path[k], At: start.Add(time.Duration(i*10+k) * time.Minute), Seq: int64(len(f.events))})
		}
		f.events = append(f.events, Event{ItemID: id, From: domain.BacklogStatusDone, To: domain.BacklogStatusArchived, At: start.Add(time.Duration(i*10+9) * time.Minute), Seq: int64(len(f.events))})
	}
	require.Len(t, f.events, 10000)

	s := summarize30(t, f)
	assert.Equal(t, 10000, s.Meta.RowCounts.Events)
	assert.Equal(t, 1, f.called("Events"), "one event read for the whole window")
	assert.Equal(t, 1, f.called("LastEventsBefore"))
	assert.Equal(t, 1, f.called("Stages"))
	assert.Equal(t, 1000, s.Tiles.Merged.Value)
}

func TestAggregator_should_InvokeObserverWithRowsAndMs_When_SummaryComputes(t *testing.T) {
	t.Parallel()
	var got []ComputeStats
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	f := fixtureSrc(t)
	summarize30(t, f, WithObserver(func(cs ComputeStats) { got = append(got, cs) }), WithLogger(logger))

	require.Len(t, got, 1)
	assert.Equal(t, "miss", got[0].Cache)
	assert.Positive(t, got[0].Rows)
	assert.GreaterOrEqual(t, got[0].QueryMs, int64(0))
	line := buf.String()
	for _, want := range []string{`"rows"`, `"query_ms"`, `"cache":"miss"`} {
		assert.Contains(t, line, want)
	}
	assert.NotContains(t, line, "Synthetic", "item titles never reach the log")
}

func TestAggregator_should_LogWarnWithError_When_SectionFails(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	f := fixtureSrc(t)
	f.verdictsErr = errors.New("verdicts exploded")
	var got []ComputeStats
	summarize30(t, f, WithLogger(logger), WithObserver(func(cs ComputeStats) { got = append(got, cs) }))

	assert.Contains(t, buf.String(), `"level":"WARN"`)
	assert.Contains(t, buf.String(), "verdicts exploded")
	assert.Contains(t, buf.String(), `"section":"review"`)
	require.Len(t, got, 1)
	assert.NotEmpty(t, got[0].Failures)
}

func TestAggregator_should_CoalesceAndServeFromCache_When_SameWindowRepeated(t *testing.T) {
	t.Parallel()
	f := fixtureSrc(t)
	var got []ComputeStats
	agg := NewAggregator(f.sources(), fixtureClock, WithObserver(func(cs ComputeStats) { got = append(got, cs) }))
	q := FlowQuery{Window: WindowSpec{Days: 30}}

	first, err := agg.Summary(context.Background(), q)
	require.NoError(t, err)
	second, err := agg.Summary(context.Background(), q)
	require.NoError(t, err)
	assert.Equal(t, first.Meta, second.Meta, "hit reports the original AsOf and QueryMs")
	assert.Equal(t, 1, f.called("Events"))
	require.Len(t, got, 1, "the observer fires on computes only")

	_, err = agg.Summary(context.Background(), FlowQuery{Window: WindowSpec{Days: 30}, Compact: true})
	require.NoError(t, err)
	assert.Equal(t, 2, f.called("Events"), "compact is part of the cache key")
}

func TestAggregator_should_RejectInvalidWindow_When_DaysZeroWithoutAllTime(t *testing.T) {
	t.Parallel()
	agg := NewAggregator(fixtureSrc(t).sources(), fixtureClock)
	_, err := agg.Summary(context.Background(), FlowQuery{})
	require.Error(t, err)
}

func TestSectionStatus_should_RenderWireStrings_When_Stringified(t *testing.T) {
	t.Parallel()
	want := map[SectionStatus]string{SectionOK: "ok", SectionPartial: "partial", SectionFailed: "failed", SectionSkipped: "skipped", SectionEmpty: "empty"}
	for st, s := range want {
		assert.Equal(t, s, st.String())
	}
	assert.Equal(t, "unknown", SectionStatus(99).String())
}

// ctxBlockingSrc blocks the first CountEvents call until its context ends, so a
// test can cancel the singleflight leader mid-compute.
type ctxBlockingSrc struct {
	*fakeSrc
	entered chan struct{}
	once    sync.Once
}

func (c *ctxBlockingSrc) CountEvents(ctx context.Context, rng EventRange) (int, error) {
	first := false
	c.once.Do(func() { first = true })
	if first {
		close(c.entered)
		<-ctx.Done()
		return 0, ctx.Err()
	}
	return c.fakeSrc.CountEvents(ctx, rng)
}

func TestAggregator_should_RetryFollower_When_CoalescedLeaderIsCancelled(t *testing.T) {
	t.Parallel()
	f := fixtureSrc(t)
	blocking := &ctxBlockingSrc{fakeSrc: f, entered: make(chan struct{})}
	agg := NewAggregator(Sources{Events: blocking, Verdicts: f, Stuck: f, Items: f}, fixtureClock)
	q := FlowQuery{Window: WindowSpec{Days: 30}}

	leaderCtx, cancel := context.WithCancel(context.Background())
	leaderDone := make(chan error, 1)
	go func() {
		_, err := agg.Summary(leaderCtx, q)
		leaderDone <- err
	}()
	<-blocking.entered

	type result struct {
		s   *FlowSummary
		err error
	}
	followerDone := make(chan result, 1)
	go func() {
		s, err := agg.Summary(context.Background(), q)
		followerDone <- result{s, err}
	}()
	// Cancel only once the follower has had time to attach to the flight.
	time.Sleep(50 * time.Millisecond)
	cancel()

	require.Error(t, <-leaderDone)
	got := <-followerDone
	require.NoError(t, got.err)
	assert.Equal(t, SectionOK, got.s.Tiles.Meta.Status, "a live follower must retry, not inherit the cancelled leader's failed sections")
}

func TestAggregator_should_CacheSummary_When_SectionIsStablyPartial(t *testing.T) {
	t.Parallel()
	src := prFixFake("x").sources()
	src.Markers = fakeMarkers{err: errors.New("marker table locked")}
	capture := func(context.Context) (time.Time, error) { return prCapture, nil }
	agg := NewAggregator(src, fixtureClock, WithCaptureSince(capture))
	q := FlowQuery{Window: WindowSpec{Days: 30}}

	first, err := agg.Summary(context.Background(), q)
	require.NoError(t, err)
	require.Equal(t, SectionPartial, first.PR.Meta.Status)
	second, err := agg.Summary(context.Background(), q)

	require.NoError(t, err)
	assert.Same(t, first, second, "a stable Partial section must not force a recompute on every poll")
}

func inProgressDwell(t *testing.T, s *FlowSummary) Percentiles {
	t.Helper()
	for _, st := range s.Funnel.Stages {
		if st.Status == string(domain.BacklogStatusInProgress) {
			return st.Dwell.Stats
		}
	}
	require.Fail(t, "in_progress stage missing")
	return Percentiles{}
}

func TestAggregator_should_IncludeParkedItemInDwell_When_ItHasNoEventsInWindow(t *testing.T) {
	t.Parallel()
	base := inProgressDwell(t, summarize30(t, fixtureSrc(t)))

	f := fixtureSrc(t)
	parked := ItemID("00000000-0000-0000-0000-0000000000aa")
	f.events = append(f.events, Event{ItemID: parked, From: domain.BacklogStatusQueued, To: domain.BacklogStatusInProgress, At: fixtureClock.T.Add(-110 * 24 * time.Hour), Seq: 1})
	f.items = append(f.items, ItemMeta{ID: parked, Title: "parked", Status: domain.BacklogStatusInProgress})
	got := inProgressDwell(t, summarize30(t, f))

	assert.Equal(t, base.N+1, got.N, "a parked item contributes one open in_progress sample")
	assert.Equal(t, 30*24*time.Hour, got.Max, "parked for the whole 30-day window")
}
