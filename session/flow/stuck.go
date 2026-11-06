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
}

// StuckOptions parameterizes StuckStats. EpisodeCapture is true when every
// interval comes from recorded episodes; otherwise history is a lower bound.
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
	for reason, a := range byReason {
		out.Reasons = append(out.Reasons, StuckReasonStats{
			Reason:        reason,
			DistinctItems: len(a.items),
			Open:          a.open,
			Duration:      NewPercentiles(a.durations, opts.LowSampleN),
			LowerBound:    !opts.EpisodeCapture,
		})
	}
	slices.SortFunc(out.Reasons, func(x, y StuckReasonStats) int { return strings.Compare(x.Reason, y.Reason) })
	if len(out.Reasons) > 0 && !opts.EpisodeCapture {
		out.Estimated = []string{estimatedStuckHistoryLowerBound}
	}
	return out
}
