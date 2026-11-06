package flow

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var stuckAsOf = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

func closedStuck(item, reason string, first time.Time, dur time.Duration) StuckInterval {
	resolved := first.Add(dur)
	return StuckInterval{ItemID: ItemID(item), Reason: reason, FirstDetectedAt: first, ResolvedAt: &resolved}
}

func openStuck(item, reason string, first time.Time) StuckInterval {
	return StuckInterval{ItemID: ItemID(item), Reason: reason, FirstDetectedAt: first}
}

func stuckByReason(t *testing.T, s StuckSummary, reason string) StuckReasonStats {
	t.Helper()
	for _, r := range s.Reasons {
		if r.Reason == reason {
			return r
		}
	}
	require.Failf(t, "reason missing", "no bucket for %q", reason)
	return StuckReasonStats{}
}

func TestStuckStats_should_CountDistinctItemsAndKeepMultipleReasonsSeparate_When_CompositeRowPresent(t *testing.T) {
	t.Parallel()
	base := stuckAsOf.Add(-48 * time.Hour)
	got := StuckStats([]StuckInterval{
		closedStuck("i1", "autonomous_stuck", base, time.Hour),
		closedStuck("i1", "multiple_reasons", base, time.Hour),
		closedStuck("i2", "autonomous_stuck", base, time.Hour),
		closedStuck("i1", "autonomous_stuck", base.Add(5*time.Hour), time.Hour),
	}, StuckOptions{AsOf: stuckAsOf, LowSampleN: 5})

	assert.Equal(t, 2, stuckByReason(t, got, "autonomous_stuck").DistinctItems)
	assert.Equal(t, 1, stuckByReason(t, got, "multiple_reasons").DistinctItems)
	assert.Len(t, got.Reasons, 2)
}

func TestStuckStats_should_FlagLowerBoundAndUseAsOfForOpenRows_When_NoEpisodeCapture(t *testing.T) {
	t.Parallel()
	base := stuckAsOf.Add(-1000 * time.Hour)
	hours := []float64{287.7, 287.7, 815.1, 887.7}
	in := make([]StuckInterval, 0, len(hours)+1)
	for i, h := range hours {
		in = append(in, closedStuck(string(rune('a'+i)), "bounce_cap_exhausted", base, time.Duration(h*float64(time.Hour))))
	}
	in = append(in, openStuck("p1", "pr_needs_fix", stuckAsOf.Add(-3*time.Hour)))

	got := StuckStats(in, StuckOptions{AsOf: stuckAsOf, LowSampleN: 3})

	b := stuckByReason(t, got, "bounce_cap_exhausted")
	assert.Equal(t, 4, b.Duration.N)
	assert.Equal(t, time.Duration(287.7*float64(time.Hour)), b.Duration.P50)
	assert.Equal(t, b.Duration.Max, b.Duration.P90, "nearest-rank P90 of 4 samples is rank 4")
	assert.Equal(t, time.Duration(887.7*float64(time.Hour)), b.Duration.Max)
	assert.True(t, b.LowerBound)
	assert.Contains(t, got.Estimated, "stuck_history:lower_bound")

	p := stuckByReason(t, got, "pr_needs_fix")
	assert.Equal(t, 1, p.Open)
	assert.Equal(t, 3*time.Hour, p.Duration.Max)
	assert.Equal(t, 1, got.Open)
}

func TestStuckStats_should_OmitLowerBound_When_EpisodeCaptureComplete(t *testing.T) {
	t.Parallel()
	got := StuckStats([]StuckInterval{closedStuck("i1", "bouncing", stuckAsOf.Add(-5*time.Hour), time.Hour)},
		StuckOptions{AsOf: stuckAsOf, LowSampleN: 5, EpisodeCapture: true})

	assert.False(t, stuckByReason(t, got, "bouncing").LowerBound)
	assert.Empty(t, got.Estimated)
}

func TestStuckStats_should_ReturnEmptyWithoutFlag_When_NoIntervals(t *testing.T) {
	t.Parallel()
	got := StuckStats(nil, StuckOptions{AsOf: stuckAsOf, LowSampleN: 5})

	assert.Empty(t, got.Reasons)
	assert.Empty(t, got.Estimated)
	assert.Zero(t, got.Open)
}

func TestStuckStats_should_MatchFixtureCounts_When_StuckFixtureLoaded(t *testing.T) {
	t.Parallel()
	f := loadFixture[fixtureStuck](t, "stuck.json")
	in := make([]StuckInterval, 0, len(f.Intervals))
	for _, iv := range f.Intervals {
		first, err := time.Parse(fixtureTimeLayout, iv.FirstDetectedAt)
		require.NoError(t, err)
		si := StuckInterval{ItemID: ItemID(iv.ItemID), Reason: iv.Reason, FirstDetectedAt: first.UTC()}
		if iv.ResolvedAt != nil {
			r, err := time.Parse(fixtureTimeLayout, *iv.ResolvedAt)
			require.NoError(t, err)
			r = r.UTC()
			si.ResolvedAt = &r
		}
		in = append(in, si)
	}
	asOf := time.Date(2026, 10, 25, 22, 0, 0, 0, time.UTC) // 15:00 -07:00 is 22:00Z

	got := StuckStats(in, StuckOptions{AsOf: asOf, LowSampleN: 5})

	assert.Equal(t, 2, got.Open)
	pr := stuckByReason(t, got, "pr_needs_fix")
	assert.Equal(t, 1, pr.DistinctItems)
	assert.Equal(t, 2, pr.Duration.N)
	assert.Equal(t, 2*time.Hour, pr.Duration.Max)
	assert.True(t, pr.Duration.LowSample)
}

func TestStuckStats_should_ClampNegativeDuration_When_OpenRowStartsAfterAsOf(t *testing.T) {
	t.Parallel()
	got := StuckStats([]StuckInterval{openStuck("i1", "bouncing", stuckAsOf.Add(time.Hour))},
		StuckOptions{AsOf: stuckAsOf, LowSampleN: 5})

	assert.Zero(t, stuckByReason(t, got, "bouncing").Duration.Max)
}

func episodeOf(iv StuckInterval) StuckInterval {
	iv.Episode = true
	return iv
}

func TestStuckStats_should_FlagLowerBoundOnlyForReasonsWithLegacyRows_When_EpisodesMixed(t *testing.T) {
	t.Parallel()
	base := stuckAsOf.Add(-48 * time.Hour)
	got := StuckStats([]StuckInterval{
		episodeOf(closedStuck("i1", "bouncing", base, time.Hour)),
		closedStuck("i2", "autonomous_stuck", base, time.Hour),
		episodeOf(closedStuck("i3", "autonomous_stuck", base, time.Hour)),
	}, StuckOptions{AsOf: stuckAsOf, LowSampleN: 5})

	assert.False(t, stuckByReason(t, got, "bouncing").LowerBound)
	assert.True(t, stuckByReason(t, got, "autonomous_stuck").LowerBound)
	assert.Equal(t, []string{"stuck_history:lower_bound"}, got.Estimated)
}

func TestStuckStats_should_OmitLowerBoundEstimate_When_AllIntervalsAreEpisodes(t *testing.T) {
	t.Parallel()
	got := StuckStats([]StuckInterval{
		episodeOf(closedStuck("i1", "bouncing", stuckAsOf.Add(-5*time.Hour), time.Hour)),
	}, StuckOptions{AsOf: stuckAsOf, LowSampleN: 5})

	assert.False(t, stuckByReason(t, got, "bouncing").LowerBound)
	assert.Empty(t, got.Estimated)
}

type captureStuckSrc struct {
	*fakeSrc
	capture StuckCapture
	err     error
}

func (c captureStuckSrc) StuckCapture(context.Context) (StuckCapture, error) { return c.capture, c.err }

func stuckSectionFor(t *testing.T, c captureStuckSrc) StuckSection {
	t.Helper()
	f := c.fakeSrc
	src := f.sources()
	src.Stuck = c
	agg := NewAggregator(src, fixtureClock)
	s, err := agg.Summary(context.Background(), FlowQuery{Window: WindowSpec{Days: 30}, Sections: []SectionName{SectionStuck}})
	require.NoError(t, err)
	return s.Stuck
}

func TestStuckReader_should_ReportEpisodeDrift_When_OpenEpisodesDifferFromOpenStuckRows(t *testing.T) {
	t.Parallel()
	f := newFakeSrc()
	f.stuck = []StuckInterval{episodeOf(openStuck("i1", "bouncing", fixtureClock.T.Add(-time.Hour)))}
	capture := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)

	drifted := stuckSectionFor(t, captureStuckSrc{fakeSrc: f, capture: StuckCapture{CaptureSince: capture, OpenEpisodes: 3, OpenStateRows: 4}})
	assert.Contains(t, drifted.Meta.Estimated, "stuck_episode_drift:1")
	assert.Equal(t, []string{"open episodes differ from open stuck rows by 1"}, drifted.Notes)

	equal := stuckSectionFor(t, captureStuckSrc{fakeSrc: f, capture: StuckCapture{CaptureSince: capture, OpenEpisodes: 4, OpenStateRows: 4}})
	assert.Empty(t, equal.Notes)
	for _, e := range equal.Meta.Estimated {
		assert.NotContains(t, e, "stuck_episode_drift")
	}

	failed := stuckSectionFor(t, captureStuckSrc{fakeSrc: f, err: assert.AnError})
	assert.Empty(t, failed.Notes, "a failed capture read adds nothing and does not fail the section")
	assert.Equal(t, SectionOK, failed.Meta.Status)
}
