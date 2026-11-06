package flow

import (
	"time"

	"github.com/tstapler/stapler-squad/session/domain"
)

// LeadTimeResult is the lead-time percentiles over merged items.
type LeadTimeResult struct {
	Stats Percentiles
	// Excluded counts merged items with no in_progress entry before the merge.
	Excluded int
	Drill    DrillSets
}

// LeadTime measures, per merged item, the time from its first real
// `* > in_progress` event to its first pr_pending>done. An item with no such
// start before the merge is excluded and counted, never zero-filled. Starts
// inside thrash episodes are not real transitions, so an item that only
// thrashed into in_progress before merging is excluded.
func LeadTime(spans []Span, cfg Config) LeadTimeResult {
	byItem, ids := groupByItem(spans)

	res := LeadTimeResult{Drill: DrillSets{}}
	var samples []time.Duration
	for _, id := range ids {
		merged := firstMerge(byItem[id])
		if merged.IsZero() {
			continue
		}
		start, ok := firstInProgress(byItem[id], merged)
		if !ok {
			res.Excluded++
			continue
		}
		samples = append(samples, merged.Sub(start))
		res.Drill.add(DrillLeadTime, id)
	}
	res.Stats = NewPercentiles(samples, cfg.LowSampleN)
	return res
}

// firstInProgress is the earliest real transition into in_progress at or before limit.
func firstInProgress(spans []Span, limit time.Time) (time.Time, bool) {
	var first time.Time
	for _, s := range spans {
		rs, ok := s.(RealSpan)
		if !ok || rs.To() != domain.BacklogStatusInProgress || rs.At().After(limit) {
			continue
		}
		if first.IsZero() || rs.At().Before(first) {
			first = rs.At()
		}
	}
	return first, !first.IsZero()
}
