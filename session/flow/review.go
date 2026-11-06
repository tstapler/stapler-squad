package flow

import (
	"errors"
	"fmt"
	"slices"
)

// capExhaustedReasons are the stuck reasons that mean a bounce or rework cap is spent.
var capExhaustedReasons = []string{"bounce_cap_exhausted", "rework_cap"}

func wrapRead(what string, err error) error { return fmt.Errorf("read %s: %w", what, err) }

// verdictData is the window's verdicts plus the informational agent claims.
type verdictData struct {
	vs        []Verdict
	claims    map[ItemID]bool
	claimsErr error
}

func (r *run) loadVerdicts() (*verdictData, error) {
	return r.vdata.get(func() (*verdictData, error) {
		all, err := r.a.src.Verdicts.Verdicts(r.ctx, r.w)
		if err != nil {
			return nil, wrapRead("verdicts", err)
		}
		d := &verdictData{}
		for _, v := range all {
			if r.w.Contains(v.CreatedAt) {
				d.vs = append(d.vs, v)
			}
		}
		r.rows.Verdicts = len(d.vs)
		claims, cerr := r.a.src.Verdicts.AgentDoneClaims(r.ctx, r.w)
		d.claims, d.claimsErr = claims, cerr
		return d, nil
	})
}

// verdictItemIDs lists the distinct items that have a verdict, sorted.
func verdictItemIDs(vs []Verdict) []ItemID {
	set := map[ItemID]struct{}{}
	for _, v := range vs {
		set[v.ItemID] = struct{}{}
	}
	return sortedItems(set)
}

// capExhaustedItems is the set of items with a cap-exhaustion stuck interval.
func capExhaustedItems(ivs []StuckInterval) map[ItemID]bool {
	out := map[ItemID]bool{}
	for _, iv := range ivs {
		if slices.Contains(capExhaustedReasons, iv.Reason) {
			out[iv.ItemID] = true
		}
	}
	return out
}

// reviewHistories groups verdicts per item for deadlock detection.
func reviewHistories(vd *verdictData, meta map[ItemID]ItemMeta, caps map[ItemID]bool) []ItemReviewHistory {
	byItem := map[ItemID][]Verdict{}
	for _, v := range vd.vs {
		byItem[v.ItemID] = append(byItem[v.ItemID], v)
	}
	ids := verdictItemIDs(vd.vs)
	out := make([]ItemReviewHistory, 0, len(ids))
	for _, id := range ids {
		out = append(out, ItemReviewHistory{
			ItemID: id, Status: meta[id].Status, Verdicts: byItem[id],
			AgentClaimedDone: vd.claims[id], CapExhausted: caps[id],
		})
	}
	return out
}

// deadlockInputs loads item statuses and cap exhaustion; failures are returned
// as non-fatal errors so the review section can degrade to Partial.
func (r *run) deadlockInputs(vd *verdictData) (meta map[ItemID]ItemMeta, caps map[ItemID]bool, soft []error) {
	if vd.claimsErr != nil {
		soft = append(soft, wrapRead("agent done claims", vd.claimsErr))
	}
	meta, err := r.itemMeta(verdictItemIDs(vd.vs))
	if err != nil {
		soft = append(soft, err)
	}
	all, err := r.loadStuck()
	if err != nil {
		soft = append(soft, err)
	}
	return meta, capExhaustedItems(all), soft
}

func (r *run) review(s *ReviewSection) error {
	vd, err := r.loadVerdicts()
	if err != nil {
		return err
	}
	sum := SummarizeVerdicts(vd.vs)
	n := r.cfg.LowSampleN
	s.Total = CountMetric{Value: sum.Total, Drill: DrillVerdicts}
	s.RawFail = ratioMetric(sum.RawFail, sum.Total, n, DrillRawFail)
	s.AdjustedFail = ratioMetric(sum.AdjustedFail, sum.AdjustedTotal, n, DrillAdjustedFail)
	s.Override = ratioMetric(sum.Overridden, sum.Total, n, DrillOverride)
	s.Unverifiable = ratioMetric(sum.ByOutcome[OutcomeUnverifiable], sum.Total, n, DrillUnverifiable)
	s.Unrecognized = sum.Unrecognized
	for _, c := range AllVerdictClasses() {
		s.Classes = append(s.Classes, VerdictClassRow{
			Class: c.String(), Total: CountMetric{Value: sum.ByClass[c], Drill: DrillVerdictClass(c)}, NonPass: sum.nonPassByClass[c],
			NonPassDrill: DrillNonPass(c),
		})
	}
	s.WeakFilter = WeakFilterNote
	meta, caps, soft := r.deadlockInputs(vd)
	report := DetectDeadlocks(reviewHistories(vd, meta, caps), r.cfg, r.a.clock)
	s.StalledCount, s.StalledDrill = report.StalledCount, DrillStalled
	limit, truncated := listLimitFor(r.q.ListLimit, r.q.Compact)
	s.Deadlocks = pageOf(deadlockRows(report.Candidates, meta), limit, truncated)
	s.ReviewerStreaks = pageOf(streakRows(report.ReviewerStreaks, meta), limit, truncated)
	s.Meta.Estimated = slices.Clone(report.Estimated)
	switch {
	case len(soft) > 0:
		err := errors.Join(soft...)
		s.Meta.Status, s.Meta.Error = SectionPartial, err.Error()
		r.failure(SectionReview, err)
	case sum.Total == 0:
		s.Meta.Status = SectionEmpty
	}
	return nil
}

func deadlockRows(cs []DeadlockCandidate, meta map[ItemID]ItemMeta) []DeadlockRow {
	rows := make([]DeadlockRow, len(cs))
	for i, c := range cs {
		rows[i] = DeadlockRow{
			ItemID: c.ItemID, Title: meta[c.ItemID].Title, Resolution: string(c.Resolution), Sequence: c.Sequence,
			RunLength: c.RunLength, RepeatedCriterion: c.RepeatedCriterion, HasRepeatedCriterion: c.HasRepeatedCriterion,
			DiffUnchangedPairs: c.DiffUnchangedPairs, AgentClaimedDone: c.AgentClaimedDone, CapExhausted: c.CapExhausted,
			InFlight: c.InFlight, LastVerdictAt: c.LastVerdictAt,
		}
	}
	return rows
}

func streakRows(ss []ReviewerStreak, meta map[ItemID]ItemMeta) []StreakRow {
	rows := make([]StreakRow, len(ss))
	for i, st := range ss {
		rows[i] = StreakRow{ItemID: st.ItemID, Title: meta[st.ItemID].Title, Length: st.Length, LastVerdictAt: st.LastVerdictAt}
	}
	return rows
}

// DrillVerdictClass is the key for the items with a verdict of the given class.
func DrillVerdictClass(c VerdictClass) DrillKey { return DrillKey("review.class." + c.String()) }

// DrillStuckReason is the key for the items stuck for the given reason.
func DrillStuckReason(reason string) DrillKey { return DrillKey("stuck." + reason) }

// reviewDrill computes the item sets behind the review-section numbers.
func (r *run) reviewDrill(sets DrillSets) error {
	vd, err := r.loadVerdicts()
	if err != nil {
		return err
	}
	byKey := map[DrillKey]map[ItemID]struct{}{}
	mark := func(k DrillKey, id ItemID) {
		if byKey[k] == nil {
			byKey[k] = map[ItemID]struct{}{}
		}
		byKey[k][id] = struct{}{}
	}
	for _, v := range vd.vs {
		class, _ := ClassifyVerdict(v)
		mark(DrillVerdicts, v.ItemID)
		mark(DrillVerdictClass(class), v.ItemID)
		if v.Outcome != OutcomePass {
			mark(DrillNonPass(class), v.ItemID)
		}
		if v.Outcome == OutcomeUnverifiable {
			mark(DrillUnverifiable, v.ItemID)
		}
		if v.Outcome == OutcomeFail {
			mark(DrillRawFail, v.ItemID)
			if class == ClassJudgment {
				mark(DrillAdjustedFail, v.ItemID)
			}
		}
		if v.OverrideBy != "" {
			mark(DrillOverride, v.ItemID)
		}
	}
	meta, caps, _ := r.deadlockInputs(vd)
	report := DetectDeadlocks(reviewHistories(vd, meta, caps), r.cfg, r.a.clock)
	for _, c := range report.Candidates {
		mark(DrillDeadlock, c.ItemID)
		if c.Resolution == ResolutionOpen && !c.InFlight {
			mark(DrillStalled, c.ItemID)
		}
	}
	for _, b := range VerdictMixSeries(vd.vs, r.w) {
		sets[DrillBucket(DrillVerdicts, b.WeekStart)] = nil
	}
	for _, k := range []DrillKey{DrillUnverifiable, DrillStalled} {
		sets[k] = nil
	}
	for _, c := range AllVerdictClasses() {
		sets[DrillNonPass(c)] = nil
	}
	for _, v := range vd.vs {
		if w := DrillBucket(DrillVerdicts, isoWeekStart(v.CreatedAt)); r.w.Contains(v.CreatedAt) {
			mark(w, v.ItemID)
		}
	}
	for k, set := range byKey {
		sets[k] = sortedItems(set)
	}
	return nil
}

// stuckDrill computes the item sets per stuck reason.
func (r *run) stuckDrill(sets DrillSets) error {
	all, err := r.loadStuck()
	if err != nil {
		return err
	}
	byKey := map[DrillKey]map[ItemID]struct{}{}
	for _, iv := range r.stuckInWindow(all) {
		k := DrillStuckReason(iv.Reason)
		if byKey[k] == nil {
			byKey[k] = map[ItemID]struct{}{}
		}
		byKey[k][iv.ItemID] = struct{}{}
	}
	for k, set := range byKey {
		sets[k] = sortedItems(set)
	}
	return nil
}
