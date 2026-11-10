package flow

import (
	"slices"
	"strings"
	"time"
)

// estimatedStuckHistoryLowerBound is the FlowMeta.Estimated code emitted when
// closed stuck durations come from state rows rather than recorded episodes.
const estimatedStuckHistoryLowerBound = "stuck_history:lower_bound"

// StuckInterval is one stuck episode for one reason. A nil ResolvedAt means
// the item is still stuck. Phase 5 episode rows and legacy state rows both
// map onto this type, so callers do not change when capture lands.
type StuckInterval struct {
	ItemID          ItemID
	Reason          string
	FirstDetectedAt time.Time
	ResolvedAt      *time.Time
	// Episode is true for a recorded episode row; false for a legacy state row
	// whose closed duration is only a lower bound.
	Episode bool
}

// StuckOptions parameterizes StuckStats. EpisodeCapture forces exact history;
// otherwise a reason is a lower bound while any of its intervals is a legacy row.
type StuckOptions struct {
	AsOf           time.Time
	LowSampleN     int
	EpisodeCapture bool
}

// StuckReasonStats summarizes one stuck reason. DistinctItems counts items,
// not intervals; Open counts intervals still unresolved at AsOf.
type StuckReasonStats struct {
	Reason        string
	DistinctItems int
	Open          int
	Duration      Percentiles
	LowerBound    bool
}

// StuckSummary is the per-reason stuck breakdown. multiple_reasons is its own
// bucket and is never folded into the single-reason buckets.
type StuckSummary struct {
	Reasons   []StuckReasonStats
	Open      int
	Estimated []string
}

type stuckAcc struct {
	items     map[ItemID]struct{}
	open      int
	durations []time.Duration
	legacy    bool
}

// groupStuck buckets intervals by reason and returns the total open count.
func groupStuck(intervals []StuckInterval, asOf time.Time) (map[string]*stuckAcc, int) {
	byReason := map[string]*stuckAcc{}
	var open int
	for _, iv := range intervals {
		a := byReason[iv.Reason]
		if a == nil {
			a = &stuckAcc{items: map[ItemID]struct{}{}}
			byReason[iv.Reason] = a
		}
		a.items[iv.ItemID] = struct{}{}
		a.legacy = a.legacy || !iv.Episode
		end := asOf
		if iv.ResolvedAt == nil {
			a.open++
			open++
		} else {
			end = *iv.ResolvedAt
		}
		a.durations = append(a.durations, max(end.Sub(iv.FirstDetectedAt), 0))
	}
	return byReason, open
}

// StuckStats aggregates intervals per reason, sorted by reason name. Open
// intervals measure time-in-stuck up to opts.AsOf (clamped at zero).
func StuckStats(intervals []StuckInterval, opts StuckOptions) StuckSummary {
	byReason, total := groupStuck(intervals, opts.AsOf)
	out := StuckSummary{Open: total}
	anyLower := false
	for reason, a := range byReason {
		lower := !opts.EpisodeCapture && a.legacy
		anyLower = anyLower || lower
		out.Reasons = append(out.Reasons, StuckReasonStats{
			Reason:        reason,
			DistinctItems: len(a.items),
			Open:          a.open,
			Duration:      NewPercentiles(a.durations, opts.LowSampleN),
			LowerBound:    lower,
		})
	}
	slices.SortFunc(out.Reasons, func(x, y StuckReasonStats) int { return strings.Compare(x.Reason, y.Reason) })
	if anyLower {
		out.Estimated = []string{estimatedStuckHistoryLowerBound}
	}
	return out
}

// EstimatedStuckEpisodeDriftPrefix prefixes the Estimated code that reports
// open episodes disagreeing with open stuck rows: "stuck_episode_drift:<d>".
const EstimatedStuckEpisodeDriftPrefix = "stuck_episode_drift:"

// StuckCapture is what a StuckSource that records episodes reports beyond the
// intervals: when capture began and the open counts to cross-check.
type StuckCapture struct {
	// CaptureSince is the first episode's open time; zero when none exist.
	CaptureSince time.Time
	// OpenEpisodes counts unresolved episodes.
	OpenEpisodes int
	// OpenStateRows counts unresolved legacy rows first detected at or after CaptureSince.
	OpenStateRows int
}

// Drift is the absolute difference between open episodes and open state rows.
func (c StuckCapture) Drift() int {
	if c.CaptureSince.IsZero() {
		return 0
	}
	d := c.OpenEpisodes - c.OpenStateRows
	if d < 0 {
		d = -d
	}
	return d
}
