package flow

import (
	"slices"
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
	// Trigger is unknown until trigger capture exists (Story 5.2.3).
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
}

// PRFixDwell measures how long items sit in PR-fix cycles, combined across
// triggers. A cycle starts at a real (non-collapsed) pr_pending>in_progress and
// ends at the item's next real in_progress>pr_pending, in_progress>review or
// transition into done; an unfinished cycle ends at as-of (capped at the
// window end) and is flagged Open. Thrash reopens are episodes, never cycles,
// because spans from Collapse contain no real spans for them.
func PRFixDwell(spans []Span, window FlowWindow, asOf time.Time, cfg Config) PRFixDwellResult {
	end := asOf
	if window.End().Before(end) {
		end = window.End()
	}
	byItem, ids := groupByItem(spans)

	res := PRFixDwellResult{
		Estimated: []string{"pr_fix_dwell:combined", "pr_fix_cause:unavailable"},
		Notes:     []string{prFixGapNote},
		Drill:     DrillSets{},
	}
	var samples []time.Duration
	for _, id := range ids {
		reals := realSpansOf(byItem[id])
		for _, c := range itemCycles(reals, end, &res.Quality) {
			res.Cycles = append(res.Cycles, c)
			samples = append(samples, c.Duration)
			res.Drill.add(DrillPRFixDwell, id)
			if c.Open {
				res.OpenCount++
			}
		}
	}
	res.Stats = NewPercentiles(samples, cfg.LowSampleN)
	return res
}

func itemCycles(reals []RealSpan, end time.Time, dq *DataQuality) []PRFixCycle {
	var cycles []PRFixCycle
	for i, rs := range reals {
		if rs.From() != domain.BacklogStatusPRPending || rs.To() != domain.BacklogStatusInProgress {
			continue
		}
		c := PRFixCycle{Item: rs.Item(), Start: rs.At(), End: end, Open: true, Trigger: PRFixTriggerUnknown}
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
