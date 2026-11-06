package flow

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"
)

// ErrCompareAllTime is returned for an all-time compare: there is no previous
// period to compare it with, and guessing one would mislead.
var ErrCompareAllTime = errors.New("compare needs a bounded window; pick 7, 30 or 90 days")

// compareSections are the sections the compare metrics read.
var compareSections = []SectionName{SectionTiles, SectionCauses, SectionReview, SectionPR, SectionStuck, SectionAnomalies}

type sampleGate int

const (
	gateNone sampleGate = iota
	gateItems
	gateVerdicts
)

// metricValue is one metric's raw value and sample size in one window.
type metricValue struct {
	v float64
	n int
}

// metricDef describes one compared metric. Raw counts (gateNone) are always
// comparable; ratios and percentiles need the minimum sample in both windows.
// captureTied metrics depend on trigger or episode capture and are refused when
// a window straddles the capture date.
type metricDef struct {
	name        string
	unit        string
	section     SectionName
	gate        sampleGate
	captureTied bool
	drill       DrillKey
	value       func(*FlowSummary) metricValue
}

func toHours(d time.Duration) float64 { return d.Hours() }

var staticMetrics = []metricDef{
	{name: "merged_items", unit: "items", section: SectionTiles, drill: DrillMerged, value: func(s *FlowSummary) metricValue {
		return metricValue{float64(s.Tiles.Merged.Value), s.Tiles.Merged.Value}
	}},
	{name: "first_pass_yield", unit: "ratio", section: SectionTiles, gate: gateItems, drill: DrillFPY, value: func(s *FlowSummary) metricValue {
		return metricValue{s.Tiles.FirstPassYield.Ratio, s.Tiles.FirstPassYield.Den}
	}},
	{name: "lead_time_p50_hours", unit: "hours", section: SectionTiles, gate: gateItems, drill: DrillLeadTime, value: func(s *FlowSummary) metricValue {
		return metricValue{toHours(s.Tiles.LeadTime.Stats.P50), s.Tiles.LeadTime.Stats.N}
	}},
	{name: "median_bounces", unit: "reopens", section: SectionTiles, gate: gateItems, drill: DrillMerged, value: func(s *FlowSummary) metricValue {
		return metricValue{float64(s.Tiles.MedianBounces), s.Tiles.Merged.Value}
	}},
	{name: "thrash_items", unit: "items", section: SectionAnomalies, drill: DrillThrashEpisode, value: func(s *FlowSummary) metricValue {
		return metricValue{float64(s.Anomalies.ThrashItems.Value), s.Anomalies.ThrashItems.Value}
	}},
	{name: "rework_review_fail", unit: "events", section: SectionCauses, drill: DrillReworkReviewFail, value: causeValue("review_fail")},
	{name: "rework_pr_fix", unit: "events", section: SectionCauses, drill: DrillReworkPRFix, value: causeValue("pr_fix")},
	{name: "review_raw_fail_rate", unit: "ratio", section: SectionReview, gate: gateVerdicts, drill: DrillRawFail, value: func(s *FlowSummary) metricValue {
		return metricValue{s.Review.RawFail.Ratio, s.Review.RawFail.Den}
	}},
	{name: "review_adjusted_fail_rate", unit: "ratio", section: SectionReview, gate: gateVerdicts, drill: DrillAdjustedFail, value: func(s *FlowSummary) metricValue {
		return metricValue{s.Review.AdjustedFail.Ratio, s.Review.AdjustedFail.Den}
	}},
	{name: "review_override_rate", unit: "ratio", section: SectionReview, gate: gateVerdicts, drill: DrillOverride, value: func(s *FlowSummary) metricValue {
		return metricValue{s.Review.Override.Ratio, s.Review.Override.Den}
	}},
	{name: "pr_fix_dwell_p50_hours", unit: "hours", section: SectionPR, gate: gateItems, drill: DrillPRFixDwell, value: func(s *FlowSummary) metricValue {
		return metricValue{toHours(s.PR.FixDwell.Stats.P50), s.PR.FixDwell.Stats.N}
	}},
}

func causeValue(cause string) func(*FlowSummary) metricValue {
	return func(s *FlowSummary) metricValue {
		for _, r := range s.Causes.Rework {
			if r.Cause == cause {
				return metricValue{float64(r.Count.Value), r.Count.Value}
			}
		}
		return metricValue{}
	}
}

// dynamicMetrics adds the per-stuck-reason and per-PR-fix-trigger metrics for
// every reason or trigger seen in either window.
func dynamicMetrics(a, b *FlowSummary) []metricDef {
	reasons, triggers := map[string]struct{}{}, map[string]struct{}{}
	for _, s := range []*FlowSummary{a, b} {
		for _, r := range s.Stuck.Reasons {
			reasons[r.Reason] = struct{}{}
		}
		for _, t := range s.Causes.PRFixTriggers {
			triggers[t.Trigger] = struct{}{}
		}
	}
	defs := make([]metricDef, 0, len(reasons)+len(triggers))
	for _, reason := range sortedKeys(reasons) {
		defs = append(defs, metricDef{
			name: "stuck_p50_hours." + reason, unit: "hours", section: SectionStuck, gate: gateItems, captureTied: true,
			drill: DrillStuckReason(reason), value: stuckValue(reason),
		})
	}
	for _, trig := range sortedKeys(triggers) {
		defs = append(defs, metricDef{
			name: "pr_fix_split." + trig, unit: "events", section: SectionCauses, captureTied: true,
			drill: DrillReworkPRFix, value: triggerValue(trig),
		})
	}
	return defs
}

func sortedKeys(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func stuckValue(reason string) func(*FlowSummary) metricValue {
	return func(s *FlowSummary) metricValue {
		for _, r := range s.Stuck.Reasons {
			if r.Reason == reason {
				return metricValue{toHours(r.Duration.Stats.P50), r.Duration.Stats.N}
			}
		}
		return metricValue{}
	}
}

func triggerValue(trigger string) func(*FlowSummary) metricValue {
	return func(s *FlowSummary) metricValue {
		for _, t := range s.Causes.PRFixTriggers {
			if t.Trigger == trigger {
				return metricValue{float64(t.Count), t.Count}
			}
		}
		return metricValue{}
	}
}

func sectionStatusOf(s *FlowSummary, name SectionName) SectionStatus {
	switch name {
	case SectionTiles:
		return s.Tiles.Meta.Status
	case SectionCauses:
		return s.Causes.Meta.Status
	case SectionReview:
		return s.Review.Meta.Status
	case SectionPR:
		return s.PR.Meta.Status
	case SectionStuck:
		return s.Stuck.Meta.Status
	case SectionAnomalies:
		return s.Anomalies.Meta.Status
	}
	return SectionOK
}

// compareCtx carries what metric comparison needs besides the two summaries.
type compareCtx struct {
	a, b    *FlowSummary
	cfg     Config
	capture time.Time
	winA    FlowWindow
	winB    FlowWindow
}

func (c compareCtx) straddles() bool {
	if c.capture.IsZero() {
		return false
	}
	return c.winA.Contains(c.capture) || c.winB.Contains(c.capture)
}

func (c compareCtx) minSample(g sampleGate) int {
	switch g {
	case gateItems:
		return c.cfg.MinCompareItems
	case gateVerdicts:
		return c.cfg.MinCompareVerdicts
	}
	return 0
}

// compareMetric builds one CompareMetric. It always carries the raw values and
// sample sizes; Delta is B-A only when the metric is comparable and A is not
// empty, so no infinity or NaN can appear. No significance is computed.
func (c compareCtx) compareMetric(d metricDef) CompareMetric {
	va, vb := d.value(c.a), d.value(c.b)
	m := CompareMetric{Name: d.name, Unit: d.unit, State: CompareOK, A: va.v, B: vb.v, NA: va.n, NB: vb.n, Drill: d.drill}
	switch need := c.minSample(d.gate); {
	case sectionStatusOf(c.a, d.section) == SectionFailed || sectionStatusOf(c.b, d.section) == SectionFailed:
		m.State, m.Reason = CompareIncomparable, fmt.Sprintf("section %s failed in a window", d.section)
	case d.captureTied && c.straddles():
		m.State, m.Reason = CompareIncomparable, "window straddles capture_since "+c.capture.Format("2006-01-02")
	case d.gate != gateNone && (va.n < need || vb.n < need):
		m.State, m.Reason = CompareIncomparable, fmt.Sprintf("low sample (N_A=%d, N_B=%d, need %d)", va.n, vb.n, need)
	}
	if m.State == CompareOK && va.n > 0 {
		delta := vb.v - va.v
		m.Delta = &delta
	}
	return m
}

// Compare computes both windows and a per-metric comparison. The query is
// either day offsets or explicit half-open bounds, never both; an all-time
// query returns ErrCompareAllTime.
func (a *Aggregator) Compare(ctx context.Context, q CompareQuery) (*CompareResult, error) {
	started := a.clock.Now()
	winA, winB, clamps, err := a.resolveCompare(q)
	if err != nil {
		return nil, err
	}
	sel, _ := selectSections(compareSections)
	specA, specB := compareSpecs(q, winA, winB)
	fq := FlowQuery{Cohort: q.Cohort, Config: q.Config, Sections: compareSections}
	fqA, fqB := fq, fq
	fqA.Window, fqB.Window = specA, specB
	sumA, err := a.cachedSummary(ctx, fqA, winA, sel)
	if err != nil {
		return nil, err
	}
	sumB, err := a.cachedSummary(ctx, fqB, winB, sel)
	if err != nil {
		return nil, err
	}
	cc := compareCtx{a: sumA, b: sumB, cfg: effectiveConfig(q.Config), winA: winA, winB: winB}
	meta := FlowMeta{
		AsOf: a.clock.Now(), WindowStart: winA.Start(), WindowEnd: winB.End(), Clamps: clamps,
		Estimated: unionSorted(sumA.Meta.Estimated, sumB.Meta.Estimated),
		RowCounts: RowCounts{
			Events: sumA.Meta.RowCounts.Events + sumB.Meta.RowCounts.Events, Verdicts: sumA.Meta.RowCounts.Verdicts + sumB.Meta.RowCounts.Verdicts,
			StuckIntervals: sumA.Meta.RowCounts.StuckIntervals + sumB.Meta.RowCounts.StuckIntervals,
			Items:          sumA.Meta.RowCounts.Items + sumB.Meta.RowCounts.Items,
		},
	}
	if a.captureSince != nil {
		if t, err := a.captureSince(ctx); err == nil {
			cc.capture, meta.CaptureSince = t.UTC(), t.UTC()
		}
	}
	meta.QueryMs = a.clock.Now().Sub(started).Milliseconds()
	res := &CompareResult{Meta: meta, A: sumA, B: sumB}
	for _, d := range append(slices.Clone(staticMetrics), dynamicMetrics(sumA, sumB)...) {
		res.Metrics = append(res.Metrics, cc.compareMetric(d))
	}
	return res, nil
}

// compareSpecs names each resolved window as a clock-free spec so equal compares
// share cache entries: day offsets stay offsets (their bounds drift with now),
// explicit bounds are pinned.
func compareSpecs(q CompareQuery, winA, winB FlowWindow) (specA, specB WindowSpec) {
	if q.hasBounds() {
		return WindowSpec{Start: winA.Start(), End: winA.End()}, WindowSpec{Start: winB.Start(), End: winB.End()}
	}
	return WindowSpec{Days: q.ADays, OffsetDays: q.BOffsetDays + q.BDays}, WindowSpec{Days: q.BDays, OffsetDays: q.BOffsetDays}
}

func unionSorted(lists ...[]string) []string {
	set := map[string]struct{}{}
	for _, l := range lists {
		for _, s := range l {
			set[s] = struct{}{}
		}
	}
	return sortedKeys(set)
}

func (q CompareQuery) hasOffsets() bool { return q.ADays != 0 || q.BDays != 0 || q.BOffsetDays != 0 }

func (q CompareQuery) hasBounds() bool {
	return !q.AStart.IsZero() || !q.AEnd.IsZero() || !q.BStart.IsZero() || !q.BEnd.IsZero()
}

// resolveCompare turns the query into two windows, reporting any clamp.
func (a *Aggregator) resolveCompare(q CompareQuery) (winA, winB FlowWindow, clamps []string, err error) {
	switch {
	case q.AllTime:
		return winA, winB, nil, ErrCompareAllTime
	case q.hasOffsets() && q.hasBounds():
		return winA, winB, nil, errors.New("compare: give either day offsets (a_days, b_days, b_offset_days) or explicit bounds (a_start, a_end, b_start, b_end), not both")
	case q.hasBounds():
		return a.explicitWindows(q)
	case q.hasOffsets():
		winA, winB, err = a.offsetWindows(q)
		return winA, winB, nil, err
	}
	return winA, winB, nil, errors.New("compare: give a_days and b_days, or explicit bounds a_start, a_end, b_start, b_end")
}

func (a *Aggregator) offsetWindows(q CompareQuery) (winA, winB FlowWindow, err error) {
	if q.ADays < 1 || q.BDays < 1 || q.BOffsetDays < 0 {
		return winA, winB, errors.New("compare: a_days and b_days must be >= 1 and b_offset_days >= 0")
	}
	end := a.clock.Now().UTC().AddDate(0, 0, -q.BOffsetDays)
	bStart := end.AddDate(0, 0, -q.BDays)
	if winB, err = NewFlowWindowBounds(bStart, end); err != nil {
		return winA, winB, err
	}
	winA, err = NewFlowWindowBounds(bStart.AddDate(0, 0, -q.ADays), bStart)
	return winA, winB, err
}

func (a *Aggregator) explicitWindows(q CompareQuery) (winA, winB FlowWindow, clamps []string, err error) {
	fields := []struct {
		name string
		t    time.Time
	}{{"a_start", q.AStart}, {"a_end", q.AEnd}, {"b_start", q.BStart}, {"b_end", q.BEnd}}
	for _, f := range fields {
		if f.t.IsZero() {
			return winA, winB, nil, fmt.Errorf("compare: explicit bounds need all of a_start, a_end, b_start, b_end; %s is missing", f.name)
		}
	}
	now := a.clock.Now().UTC()
	bEnd := q.BEnd.UTC()
	if bEnd.After(now) {
		bEnd = now
		clamps = append(clamps, "b_end clamped to as-of "+now.Format(time.RFC3339))
	}
	if winA, err = NewFlowWindowBounds(q.AStart, q.AEnd); err != nil {
		return winA, winB, nil, fmt.Errorf("compare: a_start must be before a_end: %w", err)
	}
	if winB, err = NewFlowWindowBounds(q.BStart, bEnd); err != nil {
		return winA, winB, nil, fmt.Errorf("compare: b_start must be before b_end: %w", err)
	}
	return winA, winB, clamps, nil
}
