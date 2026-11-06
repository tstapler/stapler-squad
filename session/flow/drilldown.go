package flow

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/session/domain"
)

// Drill keys registered by the summary assembly (the others are in drill.go,
// review.go and funnel/stuck key builders).
const (
	DrillMerged       DrillKey = "tiles.merged"
	DrillReworkOther  DrillKey = "rework.other"
	DrillDeadlock     DrillKey = "review.deadlock"
	DrillVerdicts     DrillKey = "review.verdicts"
	DrillRawFail      DrillKey = "review.raw_fail"
	DrillAdjustedFail DrillKey = "review.adjusted_fail"
	DrillOverride     DrillKey = "review.override"
)

// staticDrillKeys are registered whether or not their set is empty, so a quiet
// window still answers "which items?" with an empty page, not an unknown key.
var staticDrillKeys = []DrillKey{
	DrillReworkReviewFail, DrillReworkBounce, DrillReworkPRFix, DrillReworkOther, DrillThrashEpisode,
	DrillFPY, DrillFPYThrashAffected, DrillLeadTime, DrillPRFixDwell, DrillMerged,
	DrillDeadlock, DrillVerdicts, DrillRawFail, DrillAdjustedFail, DrillOverride,
}

var drillReasons = map[DrillKey]string{
	DrillReworkReviewFail:  "review failed, item reopened",
	DrillReworkBounce:      "review bounce (recovery or manual reopen)",
	DrillReworkPRFix:       "reopened for PR fix",
	DrillReworkOther:       "reopened after done",
	DrillThrashEpisode:     "thrash episode",
	DrillFPY:               "merged on first pass",
	DrillFPYThrashAffected: "merged after thrash",
	DrillLeadTime:          "merged (lead time)",
	DrillPRFixDwell:        "PR-fix cycle",
	DrillMerged:            "merged in window",
	DrillDeadlock:          "deadlock candidate",
	DrillVerdicts:          "has a review verdict",
	DrillRawFail:           "FAIL verdict",
	DrillAdjustedFail:      "judgment FAIL verdict",
	DrillOverride:          "verdict overridden by a human",
}

func drillReason(key DrillKey) string {
	if r, ok := drillReasons[key]; ok {
		return r
	}
	k := string(key)
	switch {
	case strings.HasPrefix(k, "funnel.stage."):
		return "entered " + strings.TrimPrefix(k, "funnel.stage.")
	case strings.HasPrefix(k, "review.class."):
		return "verdict class " + strings.TrimPrefix(k, "review.class.")
	case strings.HasPrefix(k, "stuck."):
		return "stuck: " + strings.TrimPrefix(k, "stuck.")
	}
	return k
}

// drillFamily is the source group a key needs; a failing group fails its keys only.
type drillFamily int

const (
	familyEvents drillFamily = iota
	familyFunnel
	familyReview
	familyStuck
	familyUnknown
)

func familyOf(key DrillKey) drillFamily {
	k := string(key)
	switch {
	case strings.HasPrefix(k, "funnel."):
		return familyFunnel
	case strings.HasPrefix(k, "review."):
		return familyReview
	case strings.HasPrefix(k, "stuck."):
		return familyStuck
	case strings.HasPrefix(k, "rework."), strings.HasPrefix(k, "thrash."), strings.HasPrefix(k, "fpy."),
		strings.HasPrefix(k, "tiles."), strings.HasPrefix(k, "pr."):
		return familyEvents
	}
	return familyUnknown
}

// eventDrill fills the sets that come from the collapsed events.
func (r *run) eventDrill(sets DrillSets) error {
	ev, err := r.loadEvents()
	if err != nil {
		return err
	}
	for _, src := range []DrillSets{r.reworkResult(ev).Drill, r.leadResult(ev).Drill, r.prfixResult(ev).Drill} {
		for k, ids := range src {
			sets[k] = ids
		}
	}
	merged := map[ItemID]struct{}{}
	other := map[ItemID]struct{}{}
	for _, it := range r.reworkResult(ev).Items {
		if it.Merged() {
			merged[it.Item] = struct{}{}
		}
	}
	for _, sp := range ev.spans {
		if rs, ok := sp.(RealSpan); ok {
			if c, isRework := classifyReworkSpan(rs); isRework && c == ReworkOther {
				other[rs.Item()] = struct{}{}
			}
		}
	}
	sets[DrillMerged], sets[DrillReworkOther] = sortedItems(merged), sortedItems(other)
	return nil
}

// funnelDrill fills the per-stage entered sets (membership needs no dwell).
func (r *run) funnelDrill(sets DrillSets) error {
	ev, err := r.loadEvents()
	if err != nil {
		return err
	}
	stages, err := r.loadStages()
	if err != nil {
		return err
	}
	for k, ids := range Funnel(ev.spans, stages, DwellResult{}, r.cfg).Drill {
		sets[k] = ids
	}
	for _, st := range stages {
		if _, ok := sets[DrillFunnelStage(string(st))]; !ok && st != domain.BacklogStatusArchived {
			sets[DrillFunnelStage(string(st))] = nil
		}
	}
	return nil
}

// drillSets builds the sets of family fam (every family for familyUnknown, so
// the unknown-key error can list all valid keys). A family that fails to load
// is reported so its keys return that error instead of "unknown key".
func (r *run) drillSets(fam drillFamily) (DrillSets, map[drillFamily]error) {
	sets := DrillSets{}
	for _, k := range staticDrillKeys {
		sets[k] = nil
	}
	for _, c := range AllVerdictClasses() {
		sets[DrillVerdictClass(c)] = nil
	}
	failed := map[drillFamily]error{}
	builders := map[drillFamily]func(DrillSets) error{
		familyEvents: r.eventDrill, familyFunnel: r.funnelDrill, familyReview: r.reviewDrill, familyStuck: r.stuckDrill,
	}
	for f, build := range builders {
		if fam != familyUnknown && f != fam {
			continue
		}
		if err := build(sets); err != nil {
			failed[f] = err
		}
	}
	return sets, failed
}

func validKeys(sets DrillSets) string {
	keys := make([]string, 0, len(sets))
	for k := range sets {
		keys = append(keys, string(k))
	}
	slices.Sort(keys)
	return strings.Join(keys, ", ")
}

// Drilldown returns the items behind q.Key under q.Query: title, status,
// reason and (where the metric has one) duration. An unknown key returns an
// error listing the valid keys; the page carries the usual totals.
func (a *Aggregator) Drilldown(ctx context.Context, q DrilldownQuery) (*DrilldownPage, error) {
	w, err := q.Query.Window.Window(a.clock)
	if err != nil {
		return nil, err
	}
	limit, truncated := listLimitFor(q.Limit, false)
	// The page identity rides in the cache key via Sections and ListLimit.
	cq := q.Query
	cq.Sections = []SectionName{SectionName(fmt.Sprintf("drill:%s:%d", q.Key, q.Offset))}
	cq.ListLimit = limit
	return a.drillCache.Do(ctx, cq, func(ctx context.Context) (*DrilldownPage, error) {
		return a.computeDrilldown(ctx, q, w, limit, truncated)
	})
}

func (a *Aggregator) computeDrilldown(ctx context.Context, q DrilldownQuery, w FlowWindow, limit int, truncated bool) (*DrilldownPage, error) {
	r := newRun(ctx, a, q.Query, w)
	fam := familyOf(q.Key)
	sets, failed := r.drillSets(fam)
	if err, bad := failed[fam]; bad {
		return nil, fmt.Errorf("drill %q: %w", q.Key, err)
	}
	ids, ok := sets[q.Key]
	if !ok {
		return nil, fmt.Errorf("unknown drill key %q; valid keys: %s", q.Key, validKeys(sets))
	}
	page := pageRange(ids, q.Offset, limit, truncated)
	meta, err := r.itemMeta(page.Items)
	if err != nil {
		return nil, err
	}
	durations := r.drillDurations(q.Key)
	items := ListPage[DrillItem]{TotalCount: page.TotalCount, ReturnedCount: page.ReturnedCount, HasMore: page.HasMore, Truncated: page.Truncated}
	for _, id := range page.Items {
		m := meta[id]
		items.Items = append(items.Items, DrillItem{
			ItemID: id, Title: m.Title, Status: string(m.Status), Reason: drillReason(q.Key), Duration: durations[id],
		})
	}
	return &DrilldownPage{Key: q.Key, Meta: r.meta(map[SectionName]bool{}), Items: items}, nil
}

// drillDurations returns per-item durations for the keys that have one.
func (r *run) drillDurations(key DrillKey) map[ItemID]time.Duration {
	switch {
	case key == DrillLeadTime:
		return r.leadDurations()
	case key == DrillPRFixDwell:
		return r.prfixDurations()
	case strings.HasPrefix(string(key), "stuck."):
		return r.stuckDurations(key)
	}
	return map[ItemID]time.Duration{}
}

func (r *run) leadDurations() map[ItemID]time.Duration {
	out := map[ItemID]time.Duration{}
	ev, err := r.loadEvents()
	if err != nil {
		return out
	}
	byItem, ids := groupByItem(ev.spans)
	for _, id := range ids {
		merged := firstMerge(byItem[id])
		if merged.IsZero() {
			continue
		}
		if start, ok := firstInProgress(byItem[id], merged); ok {
			out[id] = merged.Sub(start)
		}
	}
	return out
}

func (r *run) prfixDurations() map[ItemID]time.Duration {
	out := map[ItemID]time.Duration{}
	ev, err := r.loadEvents()
	if err != nil {
		return out
	}
	for _, c := range r.prfixResult(ev).Cycles {
		out[c.Item] += c.Duration
	}
	return out
}

func (r *run) stuckDurations(key DrillKey) map[ItemID]time.Duration {
	out := map[ItemID]time.Duration{}
	all, err := r.loadStuck()
	if err != nil {
		return out
	}
	for _, iv := range r.stuckInWindow(all) {
		if DrillStuckReason(iv.Reason) != key {
			continue
		}
		end := r.asOf
		if iv.ResolvedAt != nil {
			end = *iv.ResolvedAt
		}
		out[iv.ItemID] += max(end.Sub(iv.FirstDetectedAt), 0)
	}
	return out
}
