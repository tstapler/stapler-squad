package flow

import (
	"slices"
	"time"

	"github.com/tstapler/stapler-squad/session/domain"
)

// ReworkCause partitions real rework transitions. ReviewFail and Bounce
// together are exactly the real review>in_progress events.
type ReworkCause int

// ReworkCause values.
const (
	// ReworkReviewFail is a review>in_progress whose note classifies as ReviewReopen.
	ReworkReviewFail ReworkCause = iota
	// ReworkPRFix is a real (post-collapse) pr_pending>in_progress.
	ReworkPRFix
	// ReworkBounce is every other real review>in_progress (agent recoveries, manual moves).
	ReworkBounce
	// ReworkOther is a reopen out of done (done>anything but archived).
	ReworkOther
)

var reworkCauseNames = [...]string{"review_fail", "pr_fix", "bounce", "other"}

// AllReworkCauses lists every ReworkCause.
func AllReworkCauses() []ReworkCause {
	return []ReworkCause{ReworkReviewFail, ReworkPRFix, ReworkBounce, ReworkOther}
}

// String returns the wire name, or "unknown" for an out-of-range value.
func (c ReworkCause) String() string {
	if c < 0 || int(c) >= len(reworkCauseNames) {
		return "unknown"
	}
	return reworkCauseNames[c]
}

// PRFixTrigger says why a PR-fix reopen fired. Events do not record it, so
// until trigger capture exists every reopen is PRFixTriggerUnknown.
type PRFixTrigger string

// PRFixTrigger values.
const (
	PRFixTriggerCI       PRFixTrigger = "ci"
	PRFixTriggerConflict PRFixTrigger = "conflict"
	PRFixTriggerMixed    PRFixTrigger = "mixed"
	PRFixTriggerReview   PRFixTrigger = "review"
	PRFixTriggerFeedback PRFixTrigger = "feedback"
	PRFixTriggerUnknown  PRFixTrigger = "unknown"
)

// ValidPRFixTrigger reports whether t is one of the accepted values.
func ValidPRFixTrigger(t PRFixTrigger) bool {
	switch t {
	case PRFixTriggerCI, PRFixTriggerConflict, PRFixTriggerMixed, PRFixTriggerReview, PRFixTriggerFeedback, PRFixTriggerUnknown:
		return true
	}
	return false
}

// BounceOverflowBucket is the index of the "10 or more" bounce bucket.
const BounceOverflowBucket = 10

// ItemRework is one item's rework. The PreMerge fields count only events
// before the item's first merge (zero MergedAt means never merged, so all
// events count); the other fields count everything in the window.
type ItemRework struct {
	Item           ItemID
	MergedAt       time.Time
	ReviewFail     int
	Bounce         int
	PRFix          int
	ThrashEpisodes int
	ThrashEvents   int

	PreMergeBounces int // real review>in_progress (ReviewFail + Bounce) before the first merge
	PreMergePRFix   int
	PreMergeThrash  int
}

// Merged reports whether the item has a first merge in the supplied spans.
func (r ItemRework) Merged() bool { return !r.MergedAt.IsZero() }

// firstPass is true when the item had no review reopen, real PR-fix reopen or
// thrash episode of any cause before its first merge.
func (r ItemRework) firstPass() bool {
	return r.PreMergeBounces == 0 && r.PreMergePRFix == 0 && r.PreMergeThrash == 0
}

// FirstPassYield is Passed/Total over merged items. Thrash-only items do not
// pass; ThrashAffected counts the merged items that had thrash before merge.
type FirstPassYield struct {
	Passed         int
	Total          int
	ThrashAffected int
	LowSample      bool
}

// Ratio is Passed/Total, 0 for an empty cohort.
func (f FirstPassYield) Ratio() float64 {
	if f.Total == 0 {
		return 0
	}
	return float64(f.Passed) / float64(f.Total)
}

// ReworkResult is the rework section: cause totals, the per-item bounce
// distribution over the merged cohort, and first-pass yield.
type ReworkResult struct {
	// Causes totals every real rework event in the spans.
	Causes map[ReworkCause]int
	// PRFixTriggers splits the PRFix total by trigger; events before capture are unknown.
	PRFixTriggers map[PRFixTrigger]int
	Items         []ItemRework
	MergedCount   int
	// BounceBuckets[n] counts merged items with n pre-merge review reopens;
	// the final bucket holds ten or more.
	BounceBuckets [BounceOverflowBucket + 1]int
	ZeroBounce    int
	MedianBounces int
	LowSample     bool
	FPY           FirstPassYield
	Estimated     []string
	Drill         DrillSets

	join *TriggerJoin
}

// Rework computes cause totals, the merged-cohort bounce distribution and
// first-pass yield from Collapse output. MergedItem membership is the item's
// first pr_pending>done among the supplied spans (done is not monotonic).
// Spans carry only in-window events, so an item first merged before the window
// is judged on its in-window merge. Thrash episodes count as episodes, never
// as individual reopens.
func Rework(spans []Span, cfg Config) ReworkResult {
	return ReworkWith(spans, cfg, nil)
}

// ReworkWith is Rework with PR-fix reopens attributed to a trigger by join.
// A nil or inactive join leaves every reopen unknown and labels the split
// unavailable; an active one splits events at or after capture and labels it
// since_capture (earlier events stay unknown).
func ReworkWith(spans []Span, cfg Config, join *TriggerJoin) ReworkResult {
	byItem, ids := groupByItem(spans)

	res := ReworkResult{
		Causes:        make(map[ReworkCause]int),
		PRFixTriggers: make(map[PRFixTrigger]int),
		Estimated:     []string{EstimatedPRFixCauseUnavailable},
		Drill:         DrillSets{},
		join:          join,
	}
	if join.Active() {
		res.Estimated = []string{EstimatedPRFixCauseSinceCapture}
	}
	for _, id := range ids {
		item := reworkItem(id, byItem[id], &res)
		res.Items = append(res.Items, item)
	}
	res.summarize(cfg)
	return res
}

func reworkItem(id ItemID, spans []Span, res *ReworkResult) ItemRework {
	slices.SortStableFunc(spans, func(a, b Span) int { return SpanTime(a).Compare(SpanTime(b)) })
	item := ItemRework{Item: id, MergedAt: firstMerge(spans)}
	pre := func(at time.Time) bool { return !item.Merged() || at.Before(item.MergedAt) }
	for _, s := range spans {
		switch v := s.(type) {
		case RealSpan:
			countRealRework(&item, v, pre(v.At()), res)
		case ThrashSpan:
			item.ThrashEpisodes++
			item.ThrashEvents += v.Count()
			if pre(v.First()) {
				item.PreMergeThrash++
			}
		}
	}
	return item
}

func countRealRework(item *ItemRework, rs RealSpan, preMerge bool, res *ReworkResult) {
	cause, ok := classifyReworkSpan(rs)
	if !ok {
		return
	}
	res.Causes[cause]++
	switch cause {
	case ReworkReviewFail, ReworkBounce:
		if cause == ReworkReviewFail {
			item.ReviewFail++
		} else {
			item.Bounce++
		}
		res.Drill.add(reviewDrillKey(cause), item.Item)
		if preMerge {
			item.PreMergeBounces++
		}
	case ReworkPRFix:
		item.PRFix++
		res.PRFixTriggers[res.join.TriggerFor(item.Item, rs.At())]++
		res.Drill.add(DrillReworkPRFix, item.Item)
		if preMerge {
			item.PreMergePRFix++
		}
	case ReworkOther:
	}
}

// classifyReworkSpan says whether a real transition is rework and of which
// cause: a review reopen (partitioned by note), a PR-fix reopen, or a reopen
// out of done.
func classifyReworkSpan(rs RealSpan) (ReworkCause, bool) {
	switch {
	case rs.From() == domain.BacklogStatusReview && rs.To() == domain.BacklogStatusInProgress:
		if ClassifyNote(rs.Note()) == CauseReviewReopen {
			return ReworkReviewFail, true
		}
		return ReworkBounce, true
	case rs.From() == domain.BacklogStatusPRPending && rs.To() == domain.BacklogStatusInProgress:
		return ReworkPRFix, true
	case rs.From() == domain.BacklogStatusDone && rs.To() != domain.BacklogStatusArchived:
		return ReworkOther, true
	}
	return 0, false
}

func reviewDrillKey(c ReworkCause) DrillKey {
	if c == ReworkReviewFail {
		return DrillReworkReviewFail
	}
	return DrillReworkBounce
}

// firstMerge is the time of the item's first real pr_pending>done, or zero.
func firstMerge(spans []Span) time.Time {
	var first time.Time
	for _, s := range spans {
		rs, ok := s.(RealSpan)
		if !ok || rs.From() != domain.BacklogStatusPRPending || rs.To() != domain.BacklogStatusDone {
			continue
		}
		if first.IsZero() || rs.At().Before(first) {
			first = rs.At()
		}
	}
	return first
}

// summarize fills the merged-cohort statistics and FPY from res.Items.
func (res *ReworkResult) summarize(cfg Config) {
	var bounceCounts []int
	for _, it := range res.Items {
		if it.ThrashEpisodes > 0 {
			res.Drill.add(DrillThrashEpisode, it.Item)
		}
		if !it.Merged() {
			continue
		}
		res.MergedCount++
		bounceCounts = append(bounceCounts, it.PreMergeBounces)
		res.BounceBuckets[min(it.PreMergeBounces, BounceOverflowBucket)]++
		if it.PreMergeBounces == 0 {
			res.ZeroBounce++
		}
		if it.PreMergeThrash > 0 {
			res.FPY.ThrashAffected++
			res.Drill.add(DrillFPYThrashAffected, it.Item)
		}
		if it.firstPass() {
			res.FPY.Passed++
			res.Drill.add(DrillFPY, it.Item)
		}
	}
	res.FPY.Total = res.MergedCount
	res.LowSample = res.MergedCount < cfg.LowSampleN
	res.FPY.LowSample = res.LowSample
	res.MedianBounces = medianInt(bounceCounts)
}

// medianInt is the nearest-rank median (rank ceil(N/2)); 0 for an empty sample.
func medianInt(xs []int) int {
	if len(xs) == 0 {
		return 0
	}
	sorted := slices.Clone(xs)
	slices.Sort(sorted)
	return sorted[(len(sorted)+1)/2-1]
}
