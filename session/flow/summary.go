package flow

import (
	"fmt"
	"slices"
	"strings"
)

// String returns the wire name, or "unknown" for an out-of-range value.
func (s SectionStatus) String() string {
	switch s {
	case SectionOK:
		return "ok"
	case SectionPartial:
		return "partial"
	case SectionFailed:
		return "failed"
	case SectionSkipped:
		return "skipped"
	case SectionEmpty:
		return "empty"
	}
	return "unknown"
}

func ratioMetric(num, den int, lowSampleN int, drill DrillKey) RatioMetric {
	return RatioMetric{Num: num, Den: den, Ratio: ratio(num, den), LowSample: den < lowSampleN, Drill: drill}
}

func (r *run) tiles(s *TilesSection) error {
	ev, err := r.loadEvents()
	if err != nil {
		return err
	}
	rw, lead := r.reworkResult(ev), r.leadResult(ev)
	s.Merged = CountMetric{Value: rw.MergedCount, Drill: DrillMerged}
	s.LeadTime = DurationMetric{Stats: lead.Stats, Drill: DrillLeadTime}
	s.LeadTimeExcluded = lead.Excluded
	s.FirstPassYield = ratioMetric(rw.FPY.Passed, rw.FPY.Total, r.cfg.LowSampleN, DrillFPY)
	s.ThrashAffected = CountMetric{Value: rw.FPY.ThrashAffected, Drill: DrillFPYThrashAffected}
	s.MedianBounces = rw.MedianBounces
	if ev.rawInWindow == 0 {
		s.Meta.Status = SectionEmpty
	}
	return nil
}

func (r *run) funnel(s *FunnelSection) error {
	ev, err := r.loadEvents()
	if err != nil {
		return err
	}
	stages, err := r.loadStages()
	if err != nil {
		return err
	}
	dwell, err := r.loadDwell()
	if err != nil {
		return err
	}
	f := Funnel(ev.spans, stages, dwell, r.cfg)
	for _, st := range f.Stages {
		key := DrillFunnelStage(string(st.Status))
		s.Stages = append(s.Stages, FunnelStageRow{
			Status:    string(st.Status),
			Entered:   CountMetric{Value: st.Entered, Drill: key},
			Advanced:  st.Advanced,
			BackEdges: st.BackEdges,
			Dwell:     DurationMetric{Stats: st.Dwell, Drill: key},
		})
	}
	s.Other, s.Archived = f.Other, f.Archived
	s.Thrash = DurationMetric{Stats: dwell.Thrash.Percentiles(r.cfg.LowSampleN), Drill: DrillThrashEpisode}
	if ev.rawInWindow == 0 {
		s.Meta.Status = SectionEmpty
	}
	return nil
}

var reworkDrillKeys = map[ReworkCause]DrillKey{
	ReworkReviewFail: DrillReworkReviewFail,
	ReworkBounce:     DrillReworkBounce,
	ReworkPRFix:      DrillReworkPRFix,
	ReworkOther:      DrillReworkOther,
}

func (r *run) causes(s *CausesSection) error {
	ev, err := r.loadEvents()
	if err != nil {
		return err
	}
	rw := r.reworkResult(ev)
	total := 0
	for _, c := range AllReworkCauses() {
		total += rw.Causes[c]
		s.Rework = append(s.Rework, CauseRow{Cause: c.String(), Count: CountMetric{Value: rw.Causes[c], Drill: reworkDrillKeys[c]}})
	}
	for trig, n := range rw.PRFixTriggers {
		s.PRFixTriggers = append(s.PRFixTriggers, TriggerRow{Trigger: string(trig), Count: n})
	}
	slices.SortFunc(s.PRFixTriggers, func(a, b TriggerRow) int { return strings.Compare(a.Trigger, b.Trigger) })
	s.BounceBuckets = slices.Clone(rw.BounceBuckets[:])
	s.ZeroBounce, s.MergedCount = rw.ZeroBounce, rw.MergedCount
	s.Notes, s.Unclassified = reworkNoteTally(ev.spans)
	s.Meta.Estimated = slices.Clone(rw.Estimated)
	if total == 0 && len(s.Notes) == 0 {
		s.Meta.Status = SectionEmpty
	}
	return nil
}

// reworkNoteTally classifies the notes of real rework events and counts the
// events collapsed into thrash by cause; Unclassified is always reported.
func reworkNoteTally(spans []Span) ([]NamedCount, int) {
	var notes []string
	thrash := map[Cause]int{}
	for _, sp := range spans {
		switch v := sp.(type) {
		case RealSpan:
			if _, ok := classifyReworkSpan(v); ok {
				notes = append(notes, v.Note())
			}
		case ThrashSpan:
			thrash[v.Cause()] += v.Count()
		}
	}
	tally := ClassifyNotes(notes)
	var out []NamedCount
	for _, c := range AllCauses() {
		if n := tally.ByCause[c]; n > 0 {
			out = append(out, NamedCount{Name: c.String(), Count: n})
		}
	}
	for _, c := range AllCauses() {
		if n := thrash[c]; n > 0 {
			out = append(out, NamedCount{Name: "thrash:" + c.String(), Count: n})
		}
	}
	return out, tally.Unclassified
}

func (r *run) pr(s *PRSection) error {
	ev, err := r.loadEvents()
	if err != nil {
		return err
	}
	res := r.prfixResult(ev)
	s.FixDwell = DurationMetric{Stats: res.Stats, Drill: DrillPRFixDwell}
	s.Cycles = CountMetric{Value: len(res.Cycles), Drill: DrillPRFixDwell}
	s.OpenCount = res.OpenCount
	s.Notes = slices.Clone(res.Notes)
	s.Meta.Estimated = slices.Clone(res.Estimated)
	if len(res.Cycles) == 0 {
		s.Meta.Status = SectionEmpty
	}
	return nil
}

func (r *run) loadStuck() ([]StuckInterval, error) {
	return r.stuck.get(func() ([]StuckInterval, error) {
		all, err := r.a.src.Stuck.StuckIntervals(r.ctx)
		if err != nil {
			return nil, wrapRead("stuck intervals", err)
		}
		r.rows.StuckIntervals = len(all)
		return all, nil
	})
}

// stuckInWindow keeps intervals that overlap the window.
func (r *run) stuckInWindow(all []StuckInterval) []StuckInterval {
	var out []StuckInterval
	for _, iv := range all {
		if iv.FirstDetectedAt.Before(r.w.End()) && (iv.ResolvedAt == nil || !iv.ResolvedAt.Before(r.w.Start())) {
			out = append(out, iv)
		}
	}
	return out
}

func (r *run) stuckSection(s *StuckSection) error {
	all, err := r.loadStuck()
	if err != nil {
		return err
	}
	ivs := r.stuckInWindow(all)
	sum := StuckStats(ivs, StuckOptions{AsOf: r.asOf, LowSampleN: r.cfg.LowSampleN})
	s.Open = sum.Open
	for _, st := range sum.Reasons {
		key := DrillStuckReason(st.Reason)
		s.Reasons = append(s.Reasons, StuckReasonRow{
			Reason:        st.Reason,
			DistinctItems: CountMetric{Value: st.DistinctItems, Drill: key},
			Open:          st.Open,
			Duration:      DurationMetric{Stats: st.Duration, Drill: key},
			LowerBound:    st.LowerBound,
		})
	}
	s.Meta.Estimated = slices.Clone(sum.Estimated)
	r.noteStuckDrift(s)
	if len(ivs) == 0 {
		s.Meta.Status = SectionEmpty
	}
	return nil
}

// noteStuckDrift surfaces open episodes disagreeing with open stuck rows. A
// source that cannot report capture, or a failed read, adds nothing.
func (r *run) noteStuckDrift(s *StuckSection) {
	src, ok := r.a.src.Stuck.(StuckCaptureSource)
	if !ok {
		return
	}
	c, err := src.StuckCapture(r.ctx)
	if err != nil {
		r.a.logger.Warn("flow stuck capture read failed", "error", err.Error())
		return
	}
	if d := c.Drift(); d > 0 {
		s.Meta.Estimated = append(s.Meta.Estimated, fmt.Sprintf("%s%d", EstimatedStuckEpisodeDriftPrefix, d))
		s.Notes = append(s.Notes, fmt.Sprintf("open episodes differ from open stuck rows by %d", d))
	}
}

func (r *run) anomalies(s *AnomaliesSection) error {
	ev, err := r.loadEvents()
	if err != nil {
		return err
	}
	rows := thrashRows(ev.spans, r.reworkResult(ev))
	s.ThrashItems = CountMetric{Value: len(rows), Drill: DrillThrashEpisode}
	limit, truncated := listLimitFor(r.q.ListLimit, r.q.Compact)
	s.Items = pageOf(rows, limit, truncated)
	if len(rows) == 0 {
		s.Meta.Status = SectionEmpty
		return nil
	}
	ids := make([]ItemID, len(s.Items.Items))
	for i, row := range s.Items.Items {
		ids[i] = row.ItemID
	}
	meta, err := r.itemMeta(ids)
	for i := range s.Items.Items {
		s.Items.Items[i].Title = meta[s.Items.Items[i].ItemID].Title
	}
	if err != nil {
		s.Meta.Status, s.Meta.Error = SectionPartial, err.Error()
		r.failure(SectionAnomalies, err)
	}
	return nil
}

// thrashRows lists thrashing items ranked by collapsed events, then item id.
func thrashRows(spans []Span, rw *ReworkResult) []ThrashItemRow {
	byItem := map[ItemID]*ThrashItemRow{}
	biggest := map[ItemID]int{}
	for _, sp := range spans {
		ts, ok := sp.(ThrashSpan)
		if !ok {
			continue
		}
		row := byItem[ts.Item()]
		if row == nil {
			row = &ThrashItemRow{ItemID: ts.Item(), FirstAt: ts.First(), LastAt: ts.Last()}
			byItem[ts.Item()] = row
		}
		row.FirstAt, row.LastAt = minTime(row.FirstAt, ts.First()), maxTime(row.LastAt, ts.Last())
		if ts.Count() > biggest[ts.Item()] {
			biggest[ts.Item()] = ts.Count()
			row.Pair = string(ts.Pair().A) + "<>" + string(ts.Pair().B)
			row.Cause = ts.Cause().String()
		}
	}
	for _, it := range rw.Items {
		if row := byItem[it.Item]; row != nil {
			row.Episodes, row.Events = it.ThrashEpisodes, it.ThrashEvents
		}
	}
	rows := make([]ThrashItemRow, 0, len(byItem))
	for _, row := range byItem {
		rows = append(rows, *row)
	}
	slices.SortFunc(rows, func(a, b ThrashItemRow) int {
		if a.Events != b.Events {
			return b.Events - a.Events
		}
		return strings.Compare(string(a.ItemID), string(b.ItemID))
	})
	return rows
}

func (r *run) notes(s *NotesSection) error {
	ev, err := r.loadEvents()
	if err != nil {
		return err
	}
	s.RawEvents = ev.rawInWindow
	s.RealSpans, s.CollapsedEvents = SpanTotals(ev.spans)
	s.Quality = ev.quality
	if ev.quality.SelfLoops > 0 {
		s.Notes = append(s.Notes, "self-loop events were dropped before aggregation")
	}
	if ev.rawInWindow == 0 {
		s.Meta.Status = SectionEmpty
	}
	return nil
}
