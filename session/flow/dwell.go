package flow

import (
	"slices"
	"time"

	"github.com/tstapler/stapler-squad/session/domain"
)

// DwellBucket is the per-visit dwell sample set of one stage (or of thrash).
type DwellBucket struct {
	Samples []time.Duration
	// Open counts samples whose interval was still running at as-of.
	Open int
}

// Percentiles summarizes the bucket with the package's nearest-rank definition.
func (b DwellBucket) Percentiles(lowSampleN int) Percentiles {
	return NewPercentiles(b.Samples, lowSampleN)
}

// Total is the summed dwell.
func (b DwellBucket) Total() time.Duration {
	var sum time.Duration
	for _, s := range b.Samples {
		sum += s
	}
	return sum
}

// DwellInput is the parameter object of Dwell.
type DwellInput struct {
	// Spans come from Collapse (in-window only).
	Spans []Span
	// Seeds are each item's last event before Window.Start; Seed.To is the
	// status the item is in when the window opens.
	Seeds  []Event
	Window FlowWindow
	// AsOf closes open intervals; it is capped at Window.End.
	AsOf time.Time
}

// DwellResult holds dwell per stage plus the separate thrash bucket.
type DwellResult struct {
	Stages  map[domain.BacklogStatus]DwellBucket
	Thrash  DwellBucket
	Quality DataQuality
}

// dwellTracker walks one item's timeline, adding one sample per visit.
type dwellTracker struct {
	res    *DwellResult
	status domain.BacklogStatus // empty until the item's status is known
	since  time.Time
}

// Dwell computes per-stage dwell clipped to the window. One sample is one
// visit to a stage. Thrash episodes go to the thrash bucket and never to the
// stages they flip between. Terminal statuses (done, archived) are not dwell
// stages. An item with no seed has unknown status until its first span, so no
// dwell is invented before that. Negative intervals clamp to zero and are
// counted in Quality.NegativeGaps.
func Dwell(in DwellInput) DwellResult {
	res := DwellResult{Stages: make(map[domain.BacklogStatus]DwellBucket)}
	spansByItem := make(map[ItemID][]Span)
	for _, s := range in.Spans {
		spansByItem[s.Item()] = append(spansByItem[s.Item()], s)
	}
	seeds := make(map[ItemID]domain.BacklogStatus)
	for _, e := range in.Seeds {
		seeds[e.ItemID] = e.To
		if _, ok := spansByItem[e.ItemID]; !ok {
			spansByItem[e.ItemID] = nil
		}
	}
	end := in.AsOf
	if in.Window.End().Before(end) {
		end = in.Window.End()
	}
	for item, spans := range spansByItem {
		slices.SortStableFunc(spans, func(a, b Span) int { return SpanTime(a).Compare(SpanTime(b)) })
		tr := &dwellTracker{res: &res, status: seeds[item], since: in.Window.Start()}
		for _, s := range spans {
			tr.apply(s, in.Window)
		}
		tr.closeOpen(end)
	}
	return res
}

func (tr *dwellTracker) apply(s Span, window FlowWindow) {
	switch v := s.(type) {
	case RealSpan:
		tr.leave(v.At())
		tr.status, tr.since = v.To(), v.At()
	case ThrashSpan:
		start := v.First()
		if v.StartsBeforeWindow() {
			start = window.Start()
		}
		// A seeded stage whose visit began at the window start and is cut
		// there by pre-window thrash has no in-window time: no zero sample.
		if !v.StartsBeforeWindow() || tr.since.Before(start) {
			tr.leave(start)
		}
		tr.record(&tr.res.Thrash, start, v.Last())
		tr.status, tr.since = v.NetStatus(), v.Last()
	default:
		tr.res.Quality.UnknownSpans++
	}
}

// leave closes the current stage visit at at (no-op while status is unknown).
func (tr *dwellTracker) leave(at time.Time) {
	if tr.status == "" {
		return
	}
	tr.recordStage(at)
}

func (tr *dwellTracker) closeOpen(end time.Time) {
	if tr.status == "" {
		return
	}
	tr.recordStage(end)
	if !isTerminalStatus(tr.status) {
		b := tr.res.Stages[tr.status]
		b.Open++
		tr.res.Stages[tr.status] = b
	}
}

func (tr *dwellTracker) recordStage(until time.Time) {
	if isTerminalStatus(tr.status) {
		return
	}
	b := tr.res.Stages[tr.status]
	tr.record(&b, tr.since, until)
	tr.res.Stages[tr.status] = b
}

func (tr *dwellTracker) record(b *DwellBucket, from, until time.Time) {
	d := until.Sub(from)
	if d < 0 {
		tr.res.Quality.NegativeGaps++
		d = 0
	}
	b.Samples = append(b.Samples, d)
}

func isTerminalStatus(s domain.BacklogStatus) bool {
	return s == domain.BacklogStatusDone || s == domain.BacklogStatusArchived
}
