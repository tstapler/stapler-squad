package flow

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

var gapBase = time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC) // a Monday inside the 30-day fixture window

// gapSrc has item "a" (merged after one review bounce and a PR-fix reopen),
// item "b" (merged on first pass), an archived thrasher and two verdicts.
func gapSrc() *fakeSrc {
	f := newFakeSrc()
	ev := func(id string, from, to domain.BacklogStatus, at time.Time, note string) {
		f.events = append(f.events, Event{ItemID: ItemID(id), From: from, To: to, At: at, Note: note, Seq: int64(len(f.events) + 1)})
	}
	h := func(n int) time.Time { return gapBase.Add(time.Duration(n) * time.Hour) }
	ev("a", stReady, stProg, h(0), "")
	ev("a", stProg, stReview, h(24), "")
	ev("a", stReview, stProg, h(48), "review failed: criterion 2")
	ev("a", stProg, stReview, h(72), "")
	ev("a", stReview, stPRPend, h(96), "")
	ev("a", stPRPend, stProg, h(120), "")
	ev("a", stProg, stPRPend, h(144), "")
	ev("a", stPRPend, stDoneStat, h(168), "")
	ev("b", stReady, stProg, h(200), "")
	ev("b", stProg, stReview, h(210), "")
	ev("b", stReview, stPRPend, h(220), "")
	ev("b", stPRPend, stDoneStat, h(230), "")
	f.items = append(f.items,
		ItemMeta{ID: "a", Title: "item a", Status: stDoneStat}, ItemMeta{ID: "b", Title: "item b", Status: stDoneStat})
	f.addThrashItems(2, gapBase.Add(10*24*time.Hour))
	f.items[len(f.items)-1].Status = domain.BacklogStatusArchived // flip-001
	f.verdicts = []Verdict{
		{ItemID: "a", CreatedAt: h(30), Outcome: OutcomeUnverifiable, Summary: "could not verify"},
		{ItemID: "a", CreatedAt: h(60), Outcome: OutcomeFail, Summary: "criterion 2 failed"},
		{ItemID: "b", CreatedAt: h(215), Outcome: OutcomePass, Summary: "ok"},
	}
	return f
}

func gapSummary(t *testing.T, f *fakeSrc) (*Aggregator, *FlowSummary) {
	t.Helper()
	agg := NewAggregator(f.sources(), fixtureClock)
	s, err := agg.Summary(context.Background(), FlowQuery{Window: WindowSpec{Days: 30}})
	require.NoError(t, err)
	return agg, s
}

func drillIDs(t *testing.T, agg *Aggregator, key DrillKey) []ItemID {
	t.Helper()
	page, err := agg.Drilldown(context.Background(), DrilldownQuery{Key: key, Query: FlowQuery{Window: WindowSpec{Days: 30}}, Limit: 50})
	require.NoError(t, err, "drill key %s", key)
	ids := make([]ItemID, len(page.Items.Items))
	for i, it := range page.Items.Items {
		ids[i] = it.ItemID
	}
	return ids
}

func TestFunnel_should_SplitBackEdgesByTargetStage_When_ItemsReturnToDifferentStages(t *testing.T) {
	t.Parallel()
	order := []domain.BacklogStatus{stReady, stProg, stReview, stPRPend, stDoneStat}
	h := func(n int) time.Time { return s12Base.Add(time.Duration(n) * time.Hour) }
	spans := []Span{
		s12Real(t, "a", stReview, stProg, h(0)),
		s12Real(t, "a", stReview, stProg, h(1)),
		s12Real(t, "b", stReview, stProg, h(2)),
		s12Real(t, "b", stReview, stReady, h(3)),
		s12Real(t, "c", stReview, stPRPend, h(4)),
	}

	res := Funnel(spans, order, DwellResult{}, DefaultConfig())

	review := funnelStage(t, res, stReview)
	assert.Equal(t, 4, review.BackEdges)
	assert.Equal(t, []BackEdgeTarget{
		{To: stReady, Count: 1, Items: 1},
		{To: stProg, Count: 3, Items: 2},
	}, review.BackEdgeTargets, "targets in stage order; count is transitions, items is distinct")
	assert.Equal(t, []ItemID{"a", "b"}, res.Drill[DrillFunnelBackEdge("review", "in_progress")])
	assert.Equal(t, []ItemID{"c"}, res.Drill[DrillFunnelAdvanced("review")])
	assert.Contains(t, res.Drill, DrillFunnelAdvanced("ready"), "advanced key present even when empty")
}

func TestAggregator_should_FillGapFields_When_ScenarioSummarized(t *testing.T) {
	t.Parallel()
	agg, s := gapSummary(t, gapSrc())

	// (a) archived thrash flag and whole-set count
	assert.Equal(t, 2, s.Anomalies.ThrashItems.Value)
	assert.Equal(t, 1, s.Anomalies.ArchivedItems)
	archived := map[ItemID]bool{}
	for _, r := range s.Anomalies.Items.Items {
		archived[r.ItemID] = r.Archived
	}
	assert.Equal(t, map[ItemID]bool{"flip-000": false, "flip-001": true}, archived)

	// (b) back-edge targets
	var prog FunnelStageRow
	for _, st := range s.Funnel.Stages {
		if st.Status == "review" {
			prog = st
		}
	}
	require.Len(t, prog.BackEdgeTargets, 1)
	assert.Equal(t, BackEdgeRow{To: "in_progress", Count: 1, Drill: "funnel.back_edge.review.in_progress"}, prog.BackEdgeTargets[0])
	assert.Equal(t, []ItemID{"a"}, drillIDs(t, agg, prog.BackEdgeTargets[0].Drill))
	assert.Equal(t, []ItemID{"a", "b"}, drillIDs(t, agg, prog.AdvancedDrill))

	// (d) items affected per cause is the drill set size
	for _, r := range s.Causes.Rework {
		assert.Len(t, drillIDs(t, agg, r.Count.Drill), r.ItemsAffected, r.Cause)
	}
	assert.Equal(t, []ItemID{"b"}, drillIDs(t, agg, s.Causes.ZeroBounceDrill))

	// (e) raw UNVERIFIABLE over the raw total
	assert.Equal(t, 1, s.Review.Unverifiable.Num)
	assert.Equal(t, 3, s.Review.Unverifiable.Den)
	assert.Equal(t, []ItemID{"a"}, drillIDs(t, agg, s.Review.Unverifiable.Drill))
	for _, c := range s.Review.Classes {
		drillIDs(t, agg, c.NonPassDrill)
	}
	drillIDs(t, agg, s.Review.StalledDrill)
	drillIDs(t, agg, s.PR.OpenDrill)
}

func TestAggregator_should_ResolveEveryTrendBucketDrill_When_ScenarioSummarized(t *testing.T) {
	t.Parallel()
	agg, s := gapSummary(t, gapSrc())
	week := isoWeekStart(gapBase.Add(168 * time.Hour)) // item a merges here

	var merged *ThroughputBucket
	for i, b := range s.Trends.Throughput {
		assert.NotEmpty(t, b.Drill, "every throughput bucket names its items")
		assert.Equal(t, DrillBucket(DrillMerged, b.WeekStart), b.Drill)
		if b.WeekStart.Equal(week) {
			merged = &s.Trends.Throughput[i]
		}
		ids := drillIDs(t, agg, b.Drill)
		assert.Len(t, ids, b.N, "throughput n is distinct merged items")
	}
	require.NotNil(t, merged)
	assert.Positive(t, merged.N)

	for _, b := range s.Trends.Rework {
		require.NotEmpty(t, b.Drill)
		drillIDs(t, agg, b.Drill)
		for _, c := range b.ByCause {
			assert.LessOrEqual(t, len(drillIDs(t, agg, c.Drill)), c.Count, "distinct items never exceed events: %s", c.Name)
		}
	}
	for _, b := range s.Trends.VerdictMix {
		assert.LessOrEqual(t, len(drillIDs(t, agg, b.Drill)), b.N, b.Drill)
	}
}

func TestIsKnownDrillKey_should_AcceptBucketKeysOnlyWithWeekSuffix_When_Checked(t *testing.T) {
	t.Parallel()
	wk := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	assert.True(t, IsKnownDrillKey(DrillBucket(DrillMerged, wk)))
	assert.True(t, IsKnownDrillKey(DrillBucket(DrillVerdicts, wk)))
	assert.False(t, IsKnownDrillKey("tiles.merged@yesterday"))
	assert.False(t, IsKnownDrillKey("fpy.first_pass@2026-10-05T00:00:00Z"), "only bucketed series accept a week")
	assert.True(t, IsKnownDrillKey(DrillFunnelBackEdge("review", "in_progress")))
}

func TestTiles_should_LabelInWindowOnly_When_WindowIsNotAllTime(t *testing.T) {
	t.Parallel()
	_, s := gapSummary(t, gapSrc())
	assert.ElementsMatch(t, []string{EstimatedLeadTimeInWindowOnly, EstimatedFPYInWindowOnly}, s.Tiles.Meta.Estimated)
	assert.Contains(t, s.Meta.Estimated, EstimatedLeadTimeInWindowOnly)

	agg := NewAggregator(gapSrc().sources(), fixtureClock)
	all, err := agg.Summary(context.Background(), FlowQuery{Window: WindowSpec{AllTime: true}})
	require.NoError(t, err)
	assert.Empty(t, all.Tiles.Meta.Estimated, "an all-time window sees every event, so nothing is edge-biased")
}

func TestFunnel_should_LabelSeedInexact_When_WindowStartsBeforeUTCCutover(t *testing.T) {
	t.Parallel()
	agg := NewAggregator(gapSrc().sources(), fixtureClock)
	for name, tc := range map[string]struct {
		q    FlowQuery
		want bool
	}{
		"365d window reaches before 2026-08-18": {FlowQuery{Window: WindowSpec{Days: 365}}, true},
		"30d window starts after the cutover":   {FlowQuery{Window: WindowSpec{Days: 30}}, false},
		"all-time has no seeds to miss":         {FlowQuery{Window: WindowSpec{AllTime: true}}, false},
	} {
		s, err := agg.Summary(context.Background(), tc.q)
		require.NoError(t, err, name)
		assert.Equal(t, tc.want, slices.Contains(s.Funnel.Meta.Estimated, EstimatedDwellSeedMixedOffset), name)
		assert.Equal(t, tc.want, slices.Contains(s.Meta.Estimated, EstimatedDwellSeedMixedOffset), name)
	}
}
