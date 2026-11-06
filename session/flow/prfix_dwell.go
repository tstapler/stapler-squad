package flow

import (
	"slices"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/session/domain"
)

// prFixGapNote states the recorded requirement gap (plan Story 1.2.3).
const prFixGapNote = "requirement asks for CI-failure and merge-conflict dwell separately; " +
	"the split exists only for cycles after <capture date> (Story 5.2.3)"

// PRFixCycle is one PR-fix cycle: a real pr_pending>in_progress to its exit.
type PRFixCycle struct {
	Item     ItemID
	Start    time.Time
	End      time.Time
	Duration time.Duration
	// Open means no exit event was seen, so End is as-of.
	Open bool
	// Trigger is the joined trigger for cycles that start after capture; unknown otherwise.
	Trigger PRFixTrigger
}

// PRFixDwellResult is the combined PR-fix dwell section.
type PRFixDwellResult struct {
	Stats     Percentiles
	Cycles    []PRFixCycle
	OpenCount int
	// Estimated lists the labels that mark this metric as combined across triggers.
	Estimated []string
	Notes     []string
	Drill     DrillSets
	Quality   DataQuality
	// TriggerDwell groups cycles that start after capture by joined trigger
	// (sorted by trigger name); empty when no capture exists.
	TriggerDwell []TriggerDwell
}

// TriggerDwell is the dwell of the post-capture cycles joined to one trigger.
type TriggerDwell struct {
	Trigger PRFixTrigger
	Cycles  int
	Stats   Percentiles
}

// PRFixDwell measures how long items sit in PR-fix cycles, combined across
// triggers. A cycle starts at a real (non-collapsed) pr_pending>in_progress and
// ends at the item's next real in_progress>pr_pending, in_progress>review or
// transition into done; an unfinished cycle ends at as-of (capped at the
// window end) and is flagged Open. Thrash reopens are episodes, never cycles,
// because spans from Collapse contain no real spans for them.
func PRFixDwell(spans []Span, window FlowWindow, asOf time.Time, cfg Config) PRFixDwellResult {
	return PRFixDwellWith(spans, window, asOf, cfg, nil)
}

// PRFixDwellWith is PRFixDwell plus a per-trigger split: cycles whose reopen
// is at or after the join's capture date are also grouped by joined trigger
// (TriggerDwell); earlier cycles stay only in the combined dwell. The combined
// Stats always cover every cycle.
func PRFixDwellWith(spans []Span, window FlowWindow, asOf time.Time, cfg Config, join *TriggerJoin) PRFixDwellResult {
	end := asOf
	if window.End().Before(end) {
		end = window.End()
	}
	byItem, ids := groupByItem(spans)

	res := PRFixDwellResult{
		Estimated: []string{"pr_fix_dwell:combined", EstimatedPRFixCauseUnavailable},
		Notes:     []string{prFixGapNote},
		Drill:     DrillSets{},
	}
	if join.Active() {
		res.Estimated = []string{"pr_fix_dwell:combined", EstimatedPRFixDwellSplit, EstimatedPRFixCauseSinceCapture}
		res.Notes = []string{"CI-failure and merge-conflict dwell are split only for cycles after " +
			join.capture.Format("2006-01-02") + "; earlier cycles stay in the combined dwell"}
	}
	var samples []time.Duration
	for _, id := range ids {
		reals := realSpansOf(byItem[id])
		for _, c := range itemCycles(reals, end, join, &res.Quality) {
			res.Cycles = append(res.Cycles, c)
			samples = append(samples, c.Duration)
			res.Drill.add(DrillPRFixDwell, id)
			if c.Open {
				res.OpenCount++
			}
		}
	}
	res.Stats = NewPercentiles(samples, cfg.LowSampleN)
	res.TriggerDwell = triggerDwell(res.Cycles, join, cfg)
	return res
}

// triggerDwell groups the cycles that start after capture by trigger.
func triggerDwell(cycles []PRFixCycle, join *TriggerJoin, cfg Config) []TriggerDwell {
	if !join.Active() {
		return nil
	}
	by := map[PRFixTrigger][]time.Duration{}
	for _, c := range cycles {
		if join.After(c.Start) {
			by[c.Trigger] = append(by[c.Trigger], c.Duration)
		}
	}
	out := make([]TriggerDwell, 0, len(by))
	for trig, ds := range by {
		out = append(out, TriggerDwell{Trigger: trig, Cycles: len(ds), Stats: NewPercentiles(ds, cfg.LowSampleN)})
	}
	slices.SortFunc(out, func(a, b TriggerDwell) int { return strings.Compare(string(a.Trigger), string(b.Trigger)) })
	return out
}

func itemCycles(reals []RealSpan, end time.Time, join *TriggerJoin, dq *DataQuality) []PRFixCycle {
	var cycles []PRFixCycle
	for i, rs := range reals {
		if rs.From() != domain.BacklogStatusPRPending || rs.To() != domain.BacklogStatusInProgress {
			continue
		}
		c := PRFixCycle{Item: rs.Item(), Start: rs.At(), End: end, Open: true, Trigger: join.TriggerFor(rs.Item(), rs.At())}
		for _, next := range reals[i+1:] {
			if endsPRFixCycle(next) {
				c.End, c.Open = next.At(), false
				break
			}
		}
		c.Duration = c.End.Sub(c.Start)
		if c.Duration < 0 {
			dq.NegativeGaps++
			c.Duration = 0
		}
		cycles = append(cycles, c)
	}
	return cycles
}

func endsPRFixCycle(rs RealSpan) bool {
	if rs.To() == domain.BacklogStatusDone {
		return true
	}
	return rs.From() == domain.BacklogStatusInProgress &&
		(rs.To() == domain.BacklogStatusPRPending || rs.To() == domain.BacklogStatusReview)
}

// realSpansOf keeps the RealSpans of spans, in time order.
func realSpansOf(spans []Span) []RealSpan {
	var out []RealSpan
	for _, s := range spans {
		if rs, ok := s.(RealSpan); ok {
			out = append(out, rs)
		}
	}
	slices.SortStableFunc(out, func(a, b RealSpan) int { return a.At().Compare(b.At()) })
	return out
}
