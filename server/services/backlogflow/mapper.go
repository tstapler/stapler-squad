package backlogflow

import (
	"strings"
	"time"
	"unicode"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/flow"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// This file is the only place flow result types become sessionv1 messages. It
// translates; it never computes.

const (
	flowTitleMaxRunes = 200
	flowNoteMaxRunes  = 280
)

// FlowJSONOptions is the one protojson configuration for flow responses:
// snake_case names and every field present, so no lowerCamelCase form exists.
func FlowJSONOptions() protojson.MarshalOptions {
	return protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true}
}

// flowEstimatedLegend explains every code the aggregator can emit in
// FlowMeta.Estimated. A test pins it to that vocabulary.
var flowEstimatedLegend = map[string]string{
	flow.EstimatedDeadlockHeuristic:        "deadlock candidates are a heuristic ranking of review-fail streaks, not a verdict",
	"pr_fix_cause:unavailable":             "PR-fix trigger is not recorded for older events, so causes show as unknown",
	flow.EstimatedPRFixCauseSinceCapture:   "PR-fix trigger split covers only events after capture began; earlier events stay unknown",
	flow.EstimatedPRFixDwellSplit:          "PR-fix dwell is split by trigger only for cycles after capture began; earlier cycles stay combined",
	flow.EstimatedPRFixOutcomeSinceCapture: "PR-fix outcome counts cover only the period since outcome capture began",
	flow.EstimatedManualNudgeSinceCapture:  "manual nudge counts cover only the period since nudge capture began (no backfill)",
	"pr_fix_dwell:combined":                "PR-fix dwell is combined across triggers",
	flow.EstimatedDwellSeedMixedOffset:     "stage dwell near the window start is inexact: events before 2026-08-18 were stored with mixed UTC offsets, so some window-start states cannot be found",
	flow.EstimatedLeadTimeInWindowOnly:     "lead time uses in-window events only: items started before the window start are excluded or understated",
	flow.EstimatedFPYInWindowOnly:          "first-pass yield uses in-window events only: rework before the window start and merges that skip pr_pending are not seen, so yield can read high near the window start",
	"stuck_history:lower_bound":            "stuck history is a lower bound: intervals before detection began are not recorded",
}

// flowEstimatedLegendPrefixes explains codes that carry a variable suffix.
var flowEstimatedLegendPrefixes = map[string]string{
	flow.EstimatedStuckEpisodeDriftPrefix: "open stuck episodes disagree with open stuck rows by the number shown",
}

const flowUnknownLegend = "heuristic metric with no description"

func legendText(code string) string {
	if text, ok := flowEstimatedLegend[code]; ok {
		return text
	}
	for prefix, text := range flowEstimatedLegendPrefixes {
		if strings.HasPrefix(code, prefix) {
			return text
		}
	}
	return flowUnknownLegend
}

func estimatedLegend(codes []string) map[string]string {
	out := make(map[string]string, len(codes))
	for _, c := range codes {
		out[c] = legendText(c)
	}
	return out
}

// sanitizeFlowText flattens control characters and whitespace runs to single
// spaces and caps the length, so raw titles and notes are safe to emit.
func sanitizeFlowText(s string, maxRunes int) string {
	var b strings.Builder
	space := false
	for _, r := range s {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			space = true
			continue
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		b.WriteRune(r)
	}
	out := []rune(b.String())
	if len(out) > maxRunes {
		return string(out[:maxRunes-1]) + "…"
	}
	return string(out)
}

// SanitizeFlowText is sanitizeFlowText for transports outside this package
// (the MCP adapter), so both flatten text the same way.
func SanitizeFlowText(s string, maxRunes int) string { return sanitizeFlowText(s, maxRunes) }

func flowTime(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t.UTC())
}

func flowI32(n int) int32 { return int32(n) } //nolint:gosec // flow counts are bounded by row counts

func flowMetaToProto(m flow.FlowMeta) *sessionv1.FlowMeta {
	return &sessionv1.FlowMeta{
		AsOf:        flowTime(m.AsOf),
		WindowStart: flowTime(m.WindowStart),
		WindowEnd:   flowTime(m.WindowEnd),
		RowCounts: &sessionv1.FlowRowCounts{
			Events: flowI32(m.RowCounts.Events), Verdicts: flowI32(m.RowCounts.Verdicts),
			StuckIntervals: flowI32(m.RowCounts.StuckIntervals), Items: flowI32(m.RowCounts.Items),
		},
		QueryMs:         m.QueryMs,
		Estimated:       m.Estimated,
		SlowPath:        m.SlowPath,
		CaptureSince:    flowTime(m.CaptureSince),
		Clamps:          m.Clamps,
		EstimatedLegend: estimatedLegend(m.Estimated),
	}
}

func sectionStatusToProto(m flow.SectionMeta) *sessionv1.FlowSectionStatus {
	return &sessionv1.FlowSectionStatus{Status: m.Status.String(), Error: m.Error, Estimated: m.Estimated}
}

func countMetric(m flow.CountMetric) *sessionv1.FlowCountMetric {
	return &sessionv1.FlowCountMetric{Value: flowI32(m.Value), DrillKey: string(m.Drill)}
}

func ratioMetric(m flow.RatioMetric) *sessionv1.FlowRatioMetric {
	return &sessionv1.FlowRatioMetric{
		Num: flowI32(m.Num), Den: flowI32(m.Den), Ratio: m.Ratio, LowSample: m.LowSample, DrillKey: string(m.Drill),
	}
}

func durationMetric(m flow.DurationMetric) *sessionv1.FlowDurationMetric {
	return &sessionv1.FlowDurationMetric{
		N: flowI32(m.Stats.N), P50Ms: m.Stats.P50.Milliseconds(), P90Ms: m.Stats.P90.Milliseconds(),
		MaxMs: m.Stats.Max.Milliseconds(), LowSample: m.Stats.LowSample, DrillKey: string(m.Drill),
	}
}

func mapSlice[S, D any](in []S, f func(S) D) []D {
	if in == nil {
		return nil
	}
	out := make([]D, len(in))
	for i, v := range in {
		out[i] = f(v)
	}
	return out
}

func flowSummaryToProto(s *flow.FlowSummary) *sessionv1.FlowSummary {
	return &sessionv1.FlowSummary{
		Meta:      flowMetaToProto(s.Meta),
		Tiles:     tilesToProto(s.Tiles),
		Funnel:    funnelToProto(s.Funnel),
		Causes:    causesToProto(s.Causes),
		Review:    reviewToProto(s.Review),
		Pr:        prToProto(s.PR),
		Stuck:     stuckToProto(s.Stuck),
		Anomalies: anomaliesToProto(s.Anomalies),
		Trends:    trendsToProto(s.Trends),
		Notes:     notesToProto(s.Notes),
	}
}

func tilesToProto(s flow.TilesSection) *sessionv1.FlowTilesSection {
	return &sessionv1.FlowTilesSection{
		Meta: sectionStatusToProto(s.Meta), Merged: countMetric(s.Merged), LeadTime: durationMetric(s.LeadTime),
		FirstPassYield: ratioMetric(s.FirstPassYield), ThrashAffected: countMetric(s.ThrashAffected),
		LeadTimeExcluded: flowI32(s.LeadTimeExcluded), MedianBounces: flowI32(s.MedianBounces),
	}
}

func funnelToProto(s flow.FunnelSection) *sessionv1.FlowFunnelSection {
	return &sessionv1.FlowFunnelSection{
		Meta: sectionStatusToProto(s.Meta),
		Stages: mapSlice(s.Stages, func(r flow.FunnelStageRow) *sessionv1.FlowFunnelStageRow {
			return &sessionv1.FlowFunnelStageRow{
				Status: r.Status, Entered: countMetric(r.Entered), Advanced: flowI32(r.Advanced),
				BackEdges: flowI32(r.BackEdges), Dwell: durationMetric(r.Dwell),
				AdvancedDrillKey: string(r.AdvancedDrill),
				BackEdgeTargets: mapSlice(r.BackEdgeTargets, func(b flow.BackEdgeRow) *sessionv1.FlowBackEdge {
					return &sessionv1.FlowBackEdge{ToStage: b.To, Count: flowI32(b.Count), DrillKey: string(b.Drill)}
				}),
			}
		}),
		Other: flowI32(s.Other), Archived: flowI32(s.Archived), Thrash: durationMetric(s.Thrash),
	}
}

func namedCount(n flow.NamedCount) *sessionv1.FlowNamedCount {
	return &sessionv1.FlowNamedCount{Name: n.Name, Count: flowI32(n.Count), DrillKey: string(n.Drill)}
}

func causesToProto(s flow.CausesSection) *sessionv1.FlowCausesSection {
	return &sessionv1.FlowCausesSection{
		Meta: sectionStatusToProto(s.Meta),
		Rework: mapSlice(s.Rework, func(r flow.CauseRow) *sessionv1.FlowCauseRow {
			return &sessionv1.FlowCauseRow{Cause: r.Cause, Count: countMetric(r.Count), ItemsAffected: flowI32(r.ItemsAffected)}
		}),
		PrFixTriggers: mapSlice(s.PRFixTriggers, func(r flow.TriggerRow) *sessionv1.FlowTriggerRow {
			return &sessionv1.FlowTriggerRow{Trigger: r.Trigger, Count: flowI32(r.Count), DrillKey: string(flow.DrillReworkPRFix)}
		}),
		BounceBuckets: mapSlice(s.BounceBuckets, flowI32),
		ZeroBounce:    flowI32(s.ZeroBounce), MergedCount: flowI32(s.MergedCount),
		ZeroBounceDrillKey: string(s.ZeroBounceDrill),
		Notes:              mapSlice(s.Notes, namedCount),
		Unclassified:       flowI32(s.Unclassified),
	}
}

func deadlockPage(p flow.ListPage[flow.DeadlockRow]) *sessionv1.FlowDeadlockPage {
	return &sessionv1.FlowDeadlockPage{
		Items: mapSlice(p.Items, func(r flow.DeadlockRow) *sessionv1.FlowDeadlockRow {
			return &sessionv1.FlowDeadlockRow{
				ItemId: string(r.ItemID), Title: sanitizeFlowText(r.Title, flowTitleMaxRunes),
				Resolution: r.Resolution, Sequence: r.Sequence, RunLength: flowI32(r.RunLength),
				RepeatedCriterion: flowI32(r.RepeatedCriterion), HasRepeatedCriterion: r.HasRepeatedCriterion,
				DiffUnchangedPairs: flowI32(r.DiffUnchangedPairs), AgentClaimedDone: r.AgentClaimedDone,
				CapExhausted: r.CapExhausted, InFlight: r.InFlight, LastVerdictAt: flowTime(r.LastVerdictAt),
			}
		}),
		TotalCount: flowI32(p.TotalCount), ReturnedCount: flowI32(p.ReturnedCount), HasMore: p.HasMore, Truncated: p.Truncated,
	}
}

func streakPage(p flow.ListPage[flow.StreakRow]) *sessionv1.FlowStreakPage {
	return &sessionv1.FlowStreakPage{
		Items: mapSlice(p.Items, func(r flow.StreakRow) *sessionv1.FlowStreakRow {
			return &sessionv1.FlowStreakRow{
				ItemId: string(r.ItemID), Title: sanitizeFlowText(r.Title, flowTitleMaxRunes),
				Length: flowI32(r.Length), LastVerdictAt: flowTime(r.LastVerdictAt),
			}
		}),
		TotalCount: flowI32(p.TotalCount), ReturnedCount: flowI32(p.ReturnedCount), HasMore: p.HasMore, Truncated: p.Truncated,
	}
}

func reviewToProto(s flow.ReviewSection) *sessionv1.FlowReviewSection {
	return &sessionv1.FlowReviewSection{
		Meta: sectionStatusToProto(s.Meta), Total: countMetric(s.Total),
		Classes: mapSlice(s.Classes, func(r flow.VerdictClassRow) *sessionv1.FlowVerdictClassRow {
			return &sessionv1.FlowVerdictClassRow{
				Class: r.Class, Total: countMetric(r.Total), NonPass: flowI32(r.NonPass), NonPassDrillKey: string(r.NonPassDrill),
			}
		}),
		RawFail: ratioMetric(s.RawFail), AdjustedFail: ratioMetric(s.AdjustedFail), Override: ratioMetric(s.Override),
		Unrecognized: flowI32(s.Unrecognized), Deadlocks: deadlockPage(s.Deadlocks), ReviewerStreaks: streakPage(s.ReviewerStreaks),
		StalledCount: flowI32(s.StalledCount), StalledDrillKey: string(s.StalledDrill), WeakFilter: s.WeakFilter,
		Unverifiable: ratioMetric(s.Unverifiable),
	}
}

func prToProto(s flow.PRSection) *sessionv1.FlowPRSection {
	return &sessionv1.FlowPRSection{
		Meta: sectionStatusToProto(s.Meta), FixDwell: durationMetric(s.FixDwell), Cycles: countMetric(s.Cycles),
		OpenCount: flowI32(s.OpenCount), OpenDrillKey: string(s.OpenDrill), Notes: s.Notes,
		Outcomes: &sessionv1.FlowPROutcomes{
			Recording: s.Outcomes.Recording, Steered: flowI32(s.Outcomes.Steered), Respawned: flowI32(s.Outcomes.Respawned),
			SkippedActive: flowI32(s.Outcomes.SkippedActive), ResolvedWithin_24H: flowI32(s.Outcomes.ResolvedWithin24h),
		},
		TriggerDwell: mapSlice(s.TriggerDwell, func(r flow.TriggerDwellRow) *sessionv1.FlowTriggerDwellRow {
			return &sessionv1.FlowTriggerDwellRow{Trigger: r.Trigger, Cycles: countMetric(r.Cycles), Dwell: durationMetric(r.Dwell)}
		}),
		Nudges: &sessionv1.FlowManualNudges{
			Recording: s.Nudges.Recording, CaptureSince: flowTime(s.Nudges.CaptureSince), Total: flowI32(s.Nudges.Total),
			DrillKey: string(s.Nudges.Drill),
			Weekly: mapSlice(s.Nudges.Weekly, func(b flow.ThroughputBucket) *sessionv1.FlowThroughputBucket {
				return &sessionv1.FlowThroughputBucket{WeekStart: flowTime(b.WeekStart), N: flowI32(b.N), DrillKey: string(b.Drill)}
			}),
		},
	}
}

func stuckToProto(s flow.StuckSection) *sessionv1.FlowStuckSection {
	return &sessionv1.FlowStuckSection{
		Meta: sectionStatusToProto(s.Meta), Open: flowI32(s.Open), Notes: s.Notes,
		Reasons: mapSlice(s.Reasons, func(r flow.StuckReasonRow) *sessionv1.FlowStuckReasonRow {
			return &sessionv1.FlowStuckReasonRow{
				Reason: r.Reason, DistinctItems: countMetric(r.DistinctItems), Open: flowI32(r.Open),
				Duration: durationMetric(r.Duration), LowerBound: r.LowerBound,
			}
		}),
	}
}

func thrashPage(p flow.ListPage[flow.ThrashItemRow]) *sessionv1.FlowThrashItemPage {
	return &sessionv1.FlowThrashItemPage{
		Items: mapSlice(p.Items, func(r flow.ThrashItemRow) *sessionv1.FlowThrashItemRow {
			return &sessionv1.FlowThrashItemRow{
				ItemId: string(r.ItemID), Title: sanitizeFlowText(r.Title, flowTitleMaxRunes),
				Episodes: flowI32(r.Episodes), Events: flowI32(r.Events), FirstAt: flowTime(r.FirstAt),
				LastAt: flowTime(r.LastAt), Pair: r.Pair, Cause: r.Cause, Archived: r.Archived,
			}
		}),
		TotalCount: flowI32(p.TotalCount), ReturnedCount: flowI32(p.ReturnedCount), HasMore: p.HasMore, Truncated: p.Truncated,
	}
}

func anomaliesToProto(s flow.AnomaliesSection) *sessionv1.FlowAnomaliesSection {
	return &sessionv1.FlowAnomaliesSection{
		Meta: sectionStatusToProto(s.Meta), ThrashItems: countMetric(s.ThrashItems), Items: thrashPage(s.Items),
		ArchivedItems: flowI32(s.ArchivedItems),
	}
}

func trendsToProto(s flow.TrendsSection) *sessionv1.FlowTrendsSection {
	return &sessionv1.FlowTrendsSection{
		Meta: sectionStatusToProto(s.Meta),
		Throughput: mapSlice(s.Throughput, func(b flow.ThroughputBucket) *sessionv1.FlowThroughputBucket {
			return &sessionv1.FlowThroughputBucket{WeekStart: flowTime(b.WeekStart), N: flowI32(b.N), DrillKey: string(b.Drill)}
		}),
		ThroughputTotal: flowI32(s.ThroughputTotal),
		VerdictMix: mapSlice(s.VerdictMix, func(b flow.VerdictMixBucketRow) *sessionv1.FlowVerdictMixBucket {
			return &sessionv1.FlowVerdictMixBucket{
				WeekStart: flowTime(b.WeekStart), N: flowI32(b.N), DrillKey: string(b.Drill),
				Cells: mapSlice(b.Cells, func(c flow.VerdictMixCell) *sessionv1.FlowVerdictMixCell {
					return &sessionv1.FlowVerdictMixCell{Class: c.Class, Outcome: c.Outcome, N: flowI32(c.Count)}
				}),
			}
		}),
		VerdictMixTotal: flowI32(s.VerdictMixTotal),
		Rework: mapSlice(s.Rework, func(b flow.ReworkBucket) *sessionv1.FlowReworkBucket {
			return &sessionv1.FlowReworkBucket{
				WeekStart: flowTime(b.WeekStart), N: flowI32(b.N), DrillKey: string(b.Drill),
				ByCause: mapSlice(b.ByCause, func(c flow.NamedCount) *sessionv1.FlowTrendCell {
					return &sessionv1.FlowTrendCell{Name: c.Name, N: flowI32(c.Count), DrillKey: string(c.Drill)}
				}),
			}
		}),
		ReworkTotal: flowI32(s.ReworkTotal),
	}
}

func notesToProto(s flow.NotesSection) *sessionv1.FlowNotesSection {
	return &sessionv1.FlowNotesSection{
		Meta: sectionStatusToProto(s.Meta), RawEvents: flowI32(s.RawEvents), RealSpans: flowI32(s.RealSpans),
		CollapsedEvents: flowI32(s.CollapsedEvents),
		Quality: &sessionv1.FlowDataQuality{
			SelfLoops: flowI32(s.Quality.SelfLoops), Unparseable: flowI32(s.Quality.Unparseable),
			NegativeGaps: flowI32(s.Quality.NegativeGaps), UnknownSpans: flowI32(s.Quality.UnknownSpans),
		},
		Notes: s.Notes,
	}
}

func compareResultToProto(r *flow.CompareResult) *sessionv1.CompareFlowWindowsResponse {
	resp := &sessionv1.CompareFlowWindowsResponse{
		Meta: flowMetaToProto(r.Meta),
		Metrics: mapSlice(r.Metrics, func(m flow.CompareMetric) *sessionv1.FlowCompareMetric {
			return &sessionv1.FlowCompareMetric{
				Name: m.Name, Unit: m.Unit, State: string(m.State), Reason: m.Reason, A: m.A, B: m.B,
				Delta: m.Delta, NA: flowI32(m.NA), NB: flowI32(m.NB), DrillKey: string(m.Drill),
			}
		}),
	}
	if r.A != nil {
		resp.A = flowSummaryToProto(r.A)
	}
	if r.B != nil {
		resp.B = flowSummaryToProto(r.B)
	}
	return resp
}

func drilldownToProto(p *flow.DrilldownPage) *sessionv1.ListFlowDrilldownResponse {
	return &sessionv1.ListFlowDrilldownResponse{
		Key:  string(p.Key),
		Meta: flowMetaToProto(p.Meta),
		Items: &sessionv1.FlowDrillItemPage{
			Items: mapSlice(p.Items.Items, func(d flow.DrillItem) *sessionv1.FlowDrillItem {
				return &sessionv1.FlowDrillItem{
					ItemId: string(d.ItemID), Title: sanitizeFlowText(d.Title, flowTitleMaxRunes),
					Status: d.Status, Reason: d.Reason, DurationMs: d.Duration.Milliseconds(),
				}
			}),
			TotalCount: flowI32(p.Items.TotalCount), ReturnedCount: flowI32(p.Items.ReturnedCount),
			HasMore: p.Items.HasMore, Truncated: p.Items.Truncated,
		},
	}
}

func timelineToProto(t *flow.ItemTimeline) *sessionv1.GetItemFlowTimelineResponse {
	return &sessionv1.GetItemFlowTimelineResponse{
		Meta: flowMetaToProto(t.Meta), ItemId: string(t.ItemID), Title: sanitizeFlowText(t.Title, flowTitleMaxRunes),
		Status: t.Status,
		Entries: mapSlice(t.Entries, func(e flow.TimelineEntry) *sessionv1.FlowTimelineEntry {
			return &sessionv1.FlowTimelineEntry{
				Kind: e.Kind, From: e.From, To: e.To, At: flowTime(e.At), Last: flowTime(e.Last),
				Events: flowI32(e.Count), Cause: e.Cause, Note: sanitizeFlowText(e.Note, flowNoteMaxRunes),
			}
		}),
		TotalEntries: flowI32(t.TotalEntries), Truncated: t.Truncated, ThrashEpisodes: flowI32(t.ThrashEpisodes),
	}
}
