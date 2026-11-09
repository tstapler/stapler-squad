package flow

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

const noteReviewFail = "auto-reopened after failed review verdict"

var s12MergeAt = s12Base.Add(20 * 24 * time.Hour)

// s12Cohort builds the 105-item merged cohort of Story 1.2.2: bounce counts
// 0:39 1:13 2:17 3:5 4:10 5:7 6:4 7:3 8:1 9:1 10+:5 (the 10+ items bounce 12 times).
// The last 7 zero-bounce items carry one real PR-fix reopen, so 32 are first-pass.
func s12Cohort(t *testing.T) []Span {
	t.Helper()
	counts := []struct{ bounces, items int }{{0, 39}, {1, 13}, {2, 17}, {3, 5}, {4, 10}, {5, 7}, {6, 4}, {7, 3}, {8, 1}, {9, 1}, {12, 5}}
	var spans []Span
	n := 0
	for _, c := range counts {
		for i := 0; i < c.items; i++ {
			item := s12Item(n)
			spans = append(spans, s12Merged(t, item, s12MergeAt)...)
			for k := 0; k < c.bounces; k++ {
				at := s12MergeAt.Add(-47*time.Hour + time.Duration(k)*time.Minute)
				spans = append(spans, s12RealNote(t, item, stReview, stProg, at, noteReviewFail))
			}
			if c.bounces == 0 && i >= 32 {
				spans = append(spans, s12Real(t, item, stPRPend, stProg, s12MergeAt.Add(-12*time.Hour)))
			}
			n++
		}
	}
	require.Equal(t, 105, n)
	return spans
}

func TestRework_should_CountZeroBounceAndMedian_When_105ItemMergedCohort(t *testing.T) {
	t.Parallel()
	spans := s12Cohort(t)

	// thrash-b: thousands of PR-fix flips collapse into episodes, counted as episodes.
	thrashB := fixtureNormalized(t, "thrash-b")
	w := wideWindow(t)
	collapsed := Collapse(thrashB, w, DefaultConfig())
	last := thrashB[len(thrashB)-1].At
	spans = append(spans, collapsed...)
	spans = append(spans, s12Real(t, "thrash-b", stPRPend, stDoneStat, last.Add(time.Hour)))

	res := Rework(spans, DefaultConfig())

	assert.Equal(t, 106, res.MergedCount)
	assert.Equal(t, 40, res.BounceBuckets[0], "39 cohort items plus thrash-b, which has no review bounce")
	assert.Equal(t, 13, res.BounceBuckets[1])
	assert.Equal(t, 5, res.BounceBuckets[BounceOverflowBucket], "10+ bucket")
	var tb ItemRework
	for _, it := range res.Items {
		if it.Item == "thrash-b" {
			tb = it
		}
	}
	assert.Equal(t, 5, tb.ThrashEpisodes, "thrash-b counts as its 5 episodes")
	realPR := 0
	for _, rs := range realSpans(collapsed) {
		if rs.From() == stPRPend && rs.To() == stProg {
			realPR++
		}
	}
	assert.Equal(t, realPR, tb.PRFix, "only real reopens, not thousands of flips")
	assert.Less(t, tb.PRFix, 20)
}

func TestRework_should_ReportZeroBounceAndMedian_When_OnlyTheCohortIsMerged(t *testing.T) {
	t.Parallel()

	res := Rework(s12Cohort(t), DefaultConfig())

	assert.Equal(t, 105, res.MergedCount)
	assert.Equal(t, 39, res.ZeroBounce)
	assert.Equal(t, 2, res.MedianBounces)
	assert.False(t, res.LowSample)
	assert.Equal(t, []string{"pr_fix_cause:unavailable"}, res.Estimated)
	assert.Equal(t, 7, res.Causes[ReworkPRFix])
	assert.Equal(t, 7, res.PRFixTriggers[PRFixTriggerUnknown])
	assert.Contains(t, res.Drill, DrillReworkPRFix)
}

func TestRework_should_UseFirstPrPendingDoneOnly_When_DoneIsReopenedAndRepeated(t *testing.T) {
	t.Parallel()
	first := s12Base
	spans := []Span{
		s12Real(t, "n", stPRPend, stDoneStat, first),
		s12Real(t, "n", stDoneStat, domain.BacklogStatusArchived, first.Add(time.Hour)),
		s12Real(t, "n", domain.BacklogStatusArchived, stDoneStat, first.Add(2*time.Hour)),
		s12Real(t, "n", stDoneStat, stReady, first.Add(3*time.Hour)),
		s12Real(t, "n", stReview, stProg, first.Add(4*time.Hour)), // after the first merge
		s12Real(t, "n", stPRPend, stDoneStat, first.Add(5*time.Hour)),
	}

	res := Rework(spans, DefaultConfig())

	assert.Equal(t, 1, res.MergedCount)
	assert.Equal(t, first, res.Items[0].MergedAt, "membership keyed on the first pr_pending>done")
	assert.Equal(t, 1, res.BounceBuckets[0], "rework after the first merge is not before-merge rework")
	assert.Equal(t, 1, res.Causes[ReworkBounce], "but it still counts toward cause totals")
}

func TestFirstPassYield_should_ReturnRatioWithDenominator_When_32Of105FirstPass(t *testing.T) {
	t.Parallel()

	res := Rework(s12Cohort(t), DefaultConfig())

	assert.Equal(t, 32, res.FPY.Passed)
	assert.Equal(t, 105, res.FPY.Total)
	assert.InDelta(t, 0.305, res.FPY.Ratio(), 0.001)
	assert.False(t, res.FPY.LowSample)
	assert.Len(t, res.Drill[DrillFPY], 32)
}

func TestFirstPassYield_should_FlagLowSample_When_ThreeMergedItems(t *testing.T) {
	t.Parallel()
	var spans []Span
	for i := 0; i < 3; i++ {
		spans = append(spans, s12Merged(t, s12Item(i), s12MergeAt)...)
	}

	res := Rework(spans, DefaultConfig())

	assert.True(t, res.FPY.LowSample)
	assert.Equal(t, 3, res.FPY.Total)
	assert.Equal(t, 3, res.FPY.Passed, "percentage still carried with N=3")
	assert.True(t, res.LowSample)
}

func TestFirstPassYield_should_ExcludeThrashOnlyItemAndReportThrashAffected_When_AllReopensCollapsed(t *testing.T) {
	t.Parallel()
	var seq int64
	flips := flipRun("t", stPRPend, stProg, s12MergeAt.Add(-30*time.Hour), 40, time.Minute, &seq)
	spans := Collapse(flips, wideWindowAround(t, s12MergeAt), DefaultConfig())
	require.Empty(t, realSpans(spans), "every reopen collapsed")
	spans = append(spans, s12Merged(t, "t", s12MergeAt)...)
	spans = append(spans, s12Merged(t, "clean", s12MergeAt)...)

	res := Rework(spans, DefaultConfig())

	assert.Equal(t, 1, res.FPY.Passed, "only the clean item")
	assert.Equal(t, 2, res.FPY.Total)
	assert.Equal(t, 1, res.FPY.ThrashAffected)
	assert.Equal(t, []ItemID{"t"}, res.Drill[DrillFPYThrashAffected])
}

func wideWindowAround(t *testing.T, at time.Time) FlowWindow {
	t.Helper()
	return dwellWindow(t, at.Add(-60*24*time.Hour), at.Add(24*time.Hour))
}

func TestRework_should_PartitionReviewFailAndBounce_When_398ReopenNoteAnd28Others(t *testing.T) {
	t.Parallel()
	var spans []Span
	for i := 0; i < 426; i++ {
		note := noteReviewFail
		if i >= 398 {
			note = []string{"request_review recovering stale/blocked review", ""}[i%2]
		}
		spans = append(spans, s12RealNote(t, s12Item(i%40), stReview, stProg, s12Base.Add(time.Duration(i)*time.Minute), note).WithTriggeredBy("system"))
	}
	order := []domain.BacklogStatus{stReady, stProg, stReview, stPRPend, stDoneStat}

	res := Rework(spans, DefaultConfig())
	fun := Funnel(spans, order, DwellResult{}, DefaultConfig())

	assert.Equal(t, 398, res.Causes[ReworkReviewFail])
	assert.Equal(t, 28, res.Causes[ReworkBounce])
	assert.Equal(t, 426, funnelStage(t, fun, stReview).BackEdges)
	assert.Equal(t, funnelStage(t, fun, stReview).BackEdges, res.Causes[ReworkReviewFail]+res.Causes[ReworkBounce])
}

func TestReworkCause_should_NameEveryValue_When_Stringified(t *testing.T) {
	t.Parallel()
	for _, c := range AllReworkCauses() {
		assert.NotEqual(t, "unknown", c.String())
	}
	assert.Equal(t, "unknown", ReworkCause(99).String())
}

func TestClassifyReworkSpan_should_RequireSystemActor_When_NoteLooksLikeReviewFail(t *testing.T) {
	t.Parallel()
	note := "auto-reopened after failed review verdict (attempt 2)"
	span := s12RealNote(t, "i", stReview, stProg, s12Base, note)

	forged, ok := classifyReworkSpan(span.WithTriggeredBy("agent"))
	assert.True(t, ok)
	assert.Equal(t, ReworkBounce, forged, "an agent-authored note must not forge a review-fail reopen")

	genuine, ok := classifyReworkSpan(span.WithTriggeredBy("system"))
	assert.True(t, ok)
	assert.Equal(t, ReworkReviewFail, genuine)
}
