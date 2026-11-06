package mcp

import (
	"context"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/flow"
)

const (
	liveThrashItems = 35
	liveDeadlocks   = 41
	liveWeeks       = 5
)

func okMeta(estimated ...string) flow.SectionMeta {
	return flow.SectionMeta{Status: flow.SectionOK, Estimated: estimated}
}

func durMetric(key flow.DrillKey, n int, p50 time.Duration) flow.DurationMetric {
	return flow.DurationMetric{Stats: flow.Percentiles{N: n, P50: p50, P90: 3 * p50, Max: 9 * p50}, Drill: key}
}

// fixtureListLimit mimics the aggregator's top-N sizing: default 10, max 50,
// compact 3. The fake summarizer applies it so size tests see realistic lists.
func fixtureListLimit(q flow.FlowQuery) int {
	switch {
	case q.Compact:
		return 3
	case q.ListLimit <= 0:
		return 10
	}
	return min(q.ListLimit, 50)
}

// liveShapedSummary is a FlowSummary shaped like the live workspace: 35 thrash
// items, 41 deadlock candidates and 5 weekly trend buckets. Every metric
// carries a drill key from flow.DrillKeys().
func liveShapedSummary(q flow.FlowQuery) *flow.FlowSummary {
	limit := fixtureListLimit(q)
	week0 := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)

	deadlocks := make([]flow.DeadlockRow, 0, liveDeadlocks)
	for i := 0; i < liveDeadlocks; i++ {
		deadlocks = append(deadlocks, flow.DeadlockRow{
			ItemID: flow.ItemID(fmt.Sprintf("dl-%03d", i)), Title: fmt.Sprintf("Deadlocked item number %d with a typical title", i),
			Resolution: "open", Sequence: "FRFRF", RunLength: 3 + i%5, RepeatedCriterion: 3, HasRepeatedCriterion: true,
			DiffUnchangedPairs: 1, AgentClaimedDone: i%2 == 0, LastVerdictAt: week0.Add(time.Duration(i) * time.Hour),
		})
	}
	thrash := make([]flow.ThrashItemRow, 0, liveThrashItems)
	for i := 0; i < liveThrashItems; i++ {
		thrash = append(thrash, flow.ThrashItemRow{
			ItemID: flow.ItemID(fmt.Sprintf("th-%03d", i)), Title: fmt.Sprintf("Thrashing item number %d with a typical title", i),
			Episodes: 1 + i%3, Events: 20 + i, FirstAt: week0, LastAt: week0.Add(48 * time.Hour),
			Pair: "in_progress<->review", Cause: "dequeue_spawn_failed",
		})
	}

	s := &flow.FlowSummary{
		Meta: flow.FlowMeta{
			AsOf: flowToolNow, WindowStart: flowToolNow.AddDate(0, 0, -30), WindowEnd: flowToolNow,
			RowCounts: flow.RowCounts{Events: 8000, Verdicts: 528, StuckIntervals: 717, Items: 300}, QueryMs: 412,
			Estimated: []string{flow.EstimatedDeadlockHeuristic, "pr_fix_cause:unavailable", "stuck_history:lower_bound"},
		},
		Tiles: flow.TilesSection{
			Meta: okMeta(), Merged: flow.CountMetric{Value: 105, Drill: flow.DrillMerged},
			LeadTime:       durMetric(flow.DrillLeadTime, 105, 6*time.Hour),
			FirstPassYield: flow.RatioMetric{Num: 32, Den: 105, Ratio: 0.305, Drill: flow.DrillFPY},
			ThrashAffected: flow.CountMetric{Value: 2, Drill: flow.DrillFPYThrashAffected}, MedianBounces: 2,
		},
		Funnel: flow.FunnelSection{Meta: okMeta(), Thrash: durMetric(flow.DrillThrashEpisode, 35, time.Hour)},
		Causes: flow.CausesSection{
			Meta: okMeta("pr_fix_cause:unavailable"),
			Rework: []flow.CauseRow{
				{Cause: "review_fail", Count: flow.CountMetric{Value: 98, Drill: flow.DrillReworkReviewFail}},
				{Cause: "pr_fix", Count: flow.CountMetric{Value: 20, Drill: flow.DrillReworkPRFix}},
				{Cause: "other", Count: flow.CountMetric{Value: 4, Drill: flow.DrillReworkOther}},
			},
			PRFixTriggers: []flow.TriggerRow{{Trigger: "unknown", Count: 20}},
			BounceBuckets: []int{39, 20, 10, 5, 2, 1, 0, 0, 0, 0, 0}, ZeroBounce: 39, MergedCount: 105,
			Notes: []flow.NamedCount{{Name: "unclassified", Count: 7}},
		},
		Review: flow.ReviewSection{
			Meta: okMeta(flow.EstimatedDeadlockHeuristic), Total: flow.CountMetric{Value: 528, Drill: flow.DrillVerdicts},
			RawFail:      flow.RatioMetric{Num: 186, Den: 528, Ratio: 0.352, Drill: flow.DrillRawFail},
			AdjustedFail: flow.RatioMetric{Num: 132, Den: 400, Ratio: 0.33, Drill: flow.DrillAdjustedFail},
			Override:     flow.RatioMetric{Num: 12, Den: 528, Ratio: 0.022, Drill: flow.DrillOverride},
			Deadlocks:    pageOf(deadlocks, limit), WeakFilter: "Heuristic: ranked review-fail streaks, not confirmed deadlocks.",
		},
		PR: flow.PRSection{Meta: okMeta(), FixDwell: durMetric(flow.DrillPRFixDwell, 20, 2*time.Hour), Cycles: flow.CountMetric{Value: 20, Drill: flow.DrillReworkPRFix}},
		Stuck: flow.StuckSection{Meta: okMeta("stuck_history:lower_bound"), Open: 7, Reasons: []flow.StuckReasonRow{
			{Reason: string(domain.StuckReasonAutonomousStuck), DistinctItems: flow.CountMetric{Value: 4, Drill: flow.DrillStuckReason(string(domain.StuckReasonAutonomousStuck))}, Duration: durMetric(flow.DrillStuckReason(string(domain.StuckReasonAutonomousStuck)), 4, time.Hour)},
		}},
		Anomalies: flow.AnomaliesSection{Meta: okMeta(), ThrashItems: flow.CountMetric{Value: liveThrashItems, Drill: flow.DrillThrashEpisode}, Items: pageOf(thrash, limit)},
		Notes:     flow.NotesSection{Meta: okMeta(), RawEvents: 8000, RealSpans: 7000, CollapsedEvents: 900, Notes: []string{"35 items thrash; collapsed to episodes."}},
	}
	for _, c := range flow.AllVerdictClasses() {
		s.Review.Classes = append(s.Review.Classes, flow.VerdictClassRow{
			Class: c.String(), Total: flow.CountMetric{Value: 100, Drill: flow.DrillVerdictClass(c)}, NonPass: 30,
		})
	}
	for _, st := range []domain.BacklogStatus{domain.BacklogStatusIdea, domain.BacklogStatusInProgress, domain.BacklogStatusReview, domain.BacklogStatusPRPending, domain.BacklogStatusDone} {
		s.Funnel.Stages = append(s.Funnel.Stages, flow.FunnelStageRow{
			Status: string(st), Entered: flow.CountMetric{Value: 200, Drill: flow.DrillFunnelStage(string(st))},
			Advanced: 150, BackEdges: 20, Dwell: durMetric(flow.DrillFunnelStage(string(st)), 150, 2*time.Hour),
		})
	}
	if !q.Compact {
		for w := 0; w < liveWeeks; w++ {
			ws := week0.AddDate(0, 0, 7*w)
			s.Trends.Throughput = append(s.Trends.Throughput, flow.ThroughputBucket{WeekStart: ws, N: 20 + w})
			s.Trends.VerdictMix = append(s.Trends.VerdictMix, flow.VerdictMixBucketRow{WeekStart: ws, N: 100, Cells: []flow.VerdictMixCell{
				{Class: "judgment", Outcome: "pass", Count: 60}, {Class: "judgment", Outcome: "fail", Count: 30}, {Class: "no_commits", Outcome: "fail", Count: 10},
			}})
			s.Trends.Rework = append(s.Trends.Rework, flow.ReworkBucket{WeekStart: ws, N: 25, ByCause: []flow.NamedCount{{Name: "review_fail", Count: 20}, {Name: "pr_fix", Count: 5}}})
		}
	}
	s.Trends.Meta = okMeta()
	s.Trends.ThroughputTotal, s.Trends.VerdictMixTotal, s.Trends.ReworkTotal = 110, 500, 125
	return s
}

// pageOf returns the first limit entries with honest list totals.
func pageOf[T any](all []T, limit int) flow.ListPage[T] {
	n := min(limit, len(all))
	return flow.ListPage[T]{
		Items: all[:n], TotalCount: len(all), ReturnedCount: n, HasMore: len(all) > n, Truncated: false,
	}
}

// liveShapedAgg is a fake summarizer whose Summary honors list limits and Compact.
type liveShapedAgg struct{ fakeFlowAgg }

func (a *liveShapedAgg) Summary(_ context.Context, q flow.FlowQuery) (*flow.FlowSummary, error) {
	a.gotSummary = q
	return liveShapedSummary(q), nil
}
