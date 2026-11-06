package flow

import "time"

var verdictOutcomes = []Outcome{OutcomePass, OutcomePartial, OutcomeFail, OutcomeUnverifiable}

func minTime(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}

func maxTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

// weekStarts lists the ISO week starts (Monday 00:00 UTC) from the week of the
// window start to the week of its last instant. A window starting over a year
// before the earliest datum (all-time) starts at that datum instead.
func weekStarts(w FlowWindow, earliest time.Time) []time.Time {
	first := w.Start()
	if !earliest.IsZero() && earliest.Sub(first) > allTimeSpan {
		first = earliest
	}
	var weeks []time.Time
	for ws, last := isoWeekStart(first), isoWeekStart(w.End().Add(-time.Nanosecond)); !ws.After(last); ws = ws.AddDate(0, 0, 7) {
		weeks = append(weeks, ws)
	}
	return weeks
}

// earliestSpanTime is the first in-window span instant, or zero.
func earliestSpanTime(spans []Span, w FlowWindow) time.Time {
	var first time.Time
	for _, s := range spans {
		if t := SpanTime(s); w.Contains(t) && (first.IsZero() || t.Before(first)) {
			first = t
		}
	}
	return first
}

func (r *run) trends(s *TrendsSection) error {
	ev, err := r.loadEvents()
	if err != nil {
		return err
	}
	vd, err := r.loadVerdicts()
	if err != nil {
		return err
	}
	weeks := r.trendWeeks(ev)
	throughput := throughputSeries(r.reworkResult(ev), weeks)
	rework := reworkSeries(ev.spans, weeks)
	mix := verdictMixRows(VerdictMixSeries(vd.vs, r.w))
	for _, b := range throughput {
		s.ThroughputTotal += b.N
	}
	for _, b := range rework {
		s.ReworkTotal += b.N
	}
	for _, b := range mix {
		s.VerdictMixTotal += b.N
	}
	if !r.q.Compact {
		s.Throughput, s.Rework, s.VerdictMix = throughput, rework, mix
	}
	if s.ThroughputTotal+s.ReworkTotal+s.VerdictMixTotal == 0 {
		s.Meta.Status = SectionEmpty
	}
	return nil
}

func (r *run) trendWeeks(ev *eventData) []time.Time {
	return weekStarts(r.w, earliestSpanTime(ev.spans, r.w))
}

// bucketDrill registers the item sets behind the throughput and rework trend
// buckets in sets: every week gets its keys (empty when quiet) so a bucket
// drill never reads as an unknown key. Sets are distinct items, whereas a
// rework bucket's n counts events.
func bucketDrill(sets DrillSets, spans []Span, rw *ReworkResult, weeks []time.Time) {
	for _, wk := range weeks {
		sets[DrillBucket(DrillMerged, wk)] = nil
		sets[DrillBucket(DrillReworkAny, wk)] = nil
		for _, c := range AllReworkCauses() {
			sets[DrillBucket(reworkDrillKeys[c], wk)] = nil
		}
	}
	in := map[DrillKey]map[ItemID]struct{}{}
	mark := func(k DrillKey, id ItemID) {
		if _, ok := sets[k]; !ok {
			return
		}
		if in[k] == nil {
			in[k] = map[ItemID]struct{}{}
		}
		in[k][id] = struct{}{}
	}
	for _, it := range rw.Items {
		if it.Merged() {
			mark(DrillBucket(DrillMerged, isoWeekStart(it.MergedAt)), it.Item)
		}
	}
	for _, sp := range spans {
		rs, ok := sp.(RealSpan)
		if !ok {
			continue
		}
		if cause, isRework := classifyReworkSpan(rs); isRework {
			wk := isoWeekStart(rs.At())
			mark(DrillBucket(DrillReworkAny, wk), rs.Item())
			mark(DrillBucket(reworkDrillKeys[cause], wk), rs.Item())
		}
	}
	for k, set := range in {
		sets[k] = sortedItems(set)
	}
}

func weekIndex(weeks []time.Time, t time.Time) (int, bool) {
	ws := isoWeekStart(t)
	for i, w := range weeks {
		if w.Equal(ws) {
			return i, true
		}
	}
	return 0, false
}

// throughputSeries counts first merges per week.
func throughputSeries(rw *ReworkResult, weeks []time.Time) []ThroughputBucket {
	out := make([]ThroughputBucket, len(weeks))
	for i, w := range weeks {
		out[i].WeekStart = w
		out[i].Drill = DrillBucket(DrillMerged, w)
	}
	for _, it := range rw.Items {
		if !it.Merged() {
			continue
		}
		if i, ok := weekIndex(weeks, it.MergedAt); ok {
			out[i].N++
		}
	}
	return out
}

// reworkSeries counts real rework events per week and cause. Every cause is
// present in every bucket so the shape is stable.
func reworkSeries(spans []Span, weeks []time.Time) []ReworkBucket {
	causes := AllReworkCauses()
	out := make([]ReworkBucket, len(weeks))
	for i, w := range weeks {
		out[i] = ReworkBucket{WeekStart: w, Drill: DrillBucket(DrillReworkAny, w), ByCause: make([]NamedCount, len(causes))}
		for j, c := range causes {
			out[i].ByCause[j] = NamedCount{Name: c.String(), Drill: DrillBucket(reworkDrillKeys[c], w)}
		}
	}
	for _, sp := range spans {
		rs, ok := sp.(RealSpan)
		if !ok {
			continue
		}
		cause, isRework := classifyReworkSpan(rs)
		i, inRange := weekIndex(weeks, rs.At())
		if !isRework || !inRange {
			continue
		}
		out[i].ByCause[int(cause)].Count++
		out[i].N++
	}
	return out
}

func verdictMixRows(buckets []VerdictMixBucket) []VerdictMixBucketRow {
	rows := make([]VerdictMixBucketRow, len(buckets))
	for i, b := range buckets {
		rows[i] = VerdictMixBucketRow{WeekStart: b.WeekStart, N: b.Total, Drill: DrillBucket(DrillVerdicts, b.WeekStart)}
		for _, class := range AllVerdictClasses() {
			for _, o := range verdictOutcomes {
				if n := b.Count(class, o); n > 0 {
					rows[i].Cells = append(rows[i].Cells, VerdictMixCell{Class: class.String(), Outcome: string(o), Count: n})
				}
			}
		}
	}
	return rows
}
