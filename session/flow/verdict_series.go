package flow

import (
	"slices"
	"time"
)

// allTimeSpan is the window length beyond which the series starts at the first
// verdict instead of the window start, so an all-time window does not emit
// decades of empty weeks.
const allTimeSpan = 365 * 24 * time.Hour

// VerdictMixBucket is one ISO week (Monday 00:00 UTC) of verdict counts by
// class and outcome. An empty week is a real zero, not missing data.
type VerdictMixBucket struct {
	WeekStart time.Time
	Total     int
	counts    map[VerdictClass]map[Outcome]int
}

// Count returns the verdicts of the given class and outcome in the week.
func (b VerdictMixBucket) Count(class VerdictClass, outcome Outcome) int {
	return b.counts[class][outcome]
}

// isoWeekStart truncates t to the Monday 00:00 UTC of its ISO week.
func isoWeekStart(t time.Time) time.Time {
	t = t.UTC()
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return day.AddDate(0, 0, -((int(day.Weekday()) + 6) % 7))
}

// VerdictMixSeries buckets in-window verdicts into consecutive weeks from the
// week of the window start to the week of its last instant. When the window
// starts more than a year before the first verdict (all-time) the series starts
// at that verdict. No in-window verdicts yields nil.
func VerdictMixSeries(vs []Verdict, w FlowWindow) []VerdictMixBucket {
	var in []Verdict
	for _, v := range vs {
		if w.Contains(v.CreatedAt) {
			in = append(in, v)
		}
	}
	if len(in) == 0 {
		return nil
	}
	first := w.Start()
	earliest := slices.MinFunc(in, func(a, b Verdict) int { return a.CreatedAt.Compare(b.CreatedAt) }).CreatedAt
	if earliest.Sub(first) > allTimeSpan {
		first = earliest
	}
	start, last := isoWeekStart(first), isoWeekStart(w.End().Add(-time.Nanosecond))

	var buckets []VerdictMixBucket
	index := make(map[time.Time]int)
	for ws := start; !ws.After(last); ws = ws.AddDate(0, 0, 7) {
		index[ws] = len(buckets)
		buckets = append(buckets, VerdictMixBucket{WeekStart: ws, counts: make(map[VerdictClass]map[Outcome]int)})
	}
	for _, v := range in {
		b := &buckets[index[isoWeekStart(v.CreatedAt)]]
		class, _ := ClassifyVerdict(v)
		if b.counts[class] == nil {
			b.counts[class] = make(map[Outcome]int)
		}
		b.counts[class][v.Outcome]++
		b.Total++
	}
	return buckets
}
