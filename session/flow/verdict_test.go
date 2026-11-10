package flow

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	sumNoCommits  = "Review blocked: no committed changes were found for this session."
	sumSelfCheck  = "Review UNVERIFIABLE: codebase-read capability self-check failed."
	sumNoDiff     = "Review blocked: no diff could be computed and the fallback failed."
	sumDegraded   = "Degraded to UNVERIFIABLE: reviewer output was unusable."
	sumNoCriteria = "Review UNVERIFIABLE: no acceptance criteria were provided."
	sumAuto       = "Auto-verified by reconcileBouncingItems: PR #1 is confirmed merged."
	sumJudgment   = "Criteria not met by the diff."
)

var verdictEpoch = time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC) // a Monday

func mkVerdict(outcome Outcome, summary string, n int) Verdict {
	return Verdict{
		ItemID:    "item",
		SessionID: SessionID(fmt.Sprintf("s-%d", n)),
		CreatedAt: verdictEpoch.Add(time.Duration(n) * time.Minute),
		Outcome:   outcome,
		Summary:   summary,
	}
}

func TestVerdictClass_should_ClassifyAndComputeAdjustedRate_When_FiveSummaryShapes(t *testing.T) {
	t.Parallel()
	vs := []Verdict{
		mkVerdict(OutcomeFail, sumNoCommits, 1),
		mkVerdict(OutcomeUnverifiable, sumSelfCheck, 2),
		mkVerdict(OutcomeUnverifiable, sumNoDiff, 3),
		mkVerdict(OutcomePass, sumAuto, 4),
		mkVerdict(OutcomeFail, sumJudgment, 5),
	}
	want := []VerdictClass{ClassNoCommits, ClassSelfCheckFailed, ClassReviewerDegraded, ClassAutoVerified, ClassJudgment}
	for i, v := range vs {
		got, _ := ClassifyVerdict(v)
		assert.Equal(t, want[i], got, "verdict %d", i)
	}

	s := SummarizeVerdicts(vs)
	assert.Equal(t, 5, s.Total)
	assert.Equal(t, 2, s.RawFail)
	assert.InDelta(t, 2.0/5.0, s.RawFailRate, 1e-9)
	assert.Equal(t, 1, s.AdjustedTotal, "only the real FAIL is a judgment")
	assert.Equal(t, 1, s.AdjustedFail)
	assert.InDelta(t, 1.0, s.AdjustedFailRate, 1e-9)
	assert.Zero(t, s.Unrecognized)
}

func TestVerdictClass_should_MatchEveryReviewerDegradedShape_And_FlagUnknownSystemSummaries(t *testing.T) {
	t.Parallel()
	for _, sum := range []string{sumNoDiff, sumDegraded, sumNoCriteria} {
		c, ok := ClassifyVerdict(mkVerdict(OutcomeUnverifiable, sum, 1))
		assert.Equal(t, ClassReviewerDegraded, c, sum)
		assert.True(t, ok, sum)
	}
	// A system-prefixed summary matching no table row stays Judgment but is counted.
	v := mkVerdict(OutcomeFail, "Review blocked: a brand new failure shape", 1)
	c, ok := ClassifyVerdict(v)
	assert.Equal(t, ClassJudgment, c)
	assert.False(t, ok)
	assert.Equal(t, 1, SummarizeVerdicts([]Verdict{v}).Unrecognized)
	// Class strings are stable wire names.
	assert.Equal(t, "reviewer_degraded", ClassReviewerDegraded.String())
	assert.Equal(t, "unknown", VerdictClass(99).String())
}

// liveMix builds 528 verdicts: FAIL 186 (98 no-commits), PARTIAL 82, PASS 150
// (23 auto-verified), UNVERIFIABLE 110 (34 self-check, 18 degraded).
func liveMix() []Verdict {
	var vs []Verdict
	add := func(n int, o Outcome, sum string) {
		for range n {
			vs = append(vs, mkVerdict(o, sum, len(vs)))
		}
	}
	add(98, OutcomeFail, sumNoCommits)
	add(88, OutcomeFail, sumJudgment)
	add(82, OutcomePartial, sumJudgment)
	add(23, OutcomePass, sumAuto)
	add(127, OutcomePass, "Criteria met.")
	add(34, OutcomeUnverifiable, sumSelfCheck)
	add(18, OutcomeUnverifiable, sumNoDiff)
	add(58, OutcomeUnverifiable, "Reviewer could not confirm criteria.")
	return vs
}

func TestVerdictSummary_should_ReconcileLiveTotals_When_528VerdictFixture(t *testing.T) {
	t.Parallel()
	vs := liveMix()
	require.Len(t, vs, 528)
	s := SummarizeVerdicts(vs)

	assert.Equal(t, 528, s.Total)
	assert.Equal(t, 186, s.RawFail)
	assert.InDelta(t, 0.352, s.RawFailRate, 0.0005)
	assert.Equal(t, 110, s.ByOutcome[OutcomeUnverifiable])
	assert.InDelta(t, 0.208, float64(s.ByOutcome[OutcomeUnverifiable])/float64(s.Total), 0.0005)

	assert.Equal(t, 378, s.NonPass)
	assert.Equal(t, 132, s.NonJudgmentNonPass(), "98 no-commits + 34 self-check")
	assert.Equal(t, 98, s.ByClass[ClassNoCommits])
	assert.Equal(t, 34, s.ByClass[ClassSelfCheckFailed])
	assert.Equal(t, 18, s.ByClass[ClassReviewerDegraded])
	assert.Equal(t, 150, s.ReviewerAndSystemNonPass(), "132 plus 18 degraded")
	assert.Equal(t, 23, s.ByClass[ClassAutoVerified])

	assert.Equal(t, 355, s.AdjustedTotal)
	assert.Equal(t, 88, s.AdjustedFail)
	assert.InDelta(t, 0.248, s.AdjustedFailRate, 0.0005)
	assert.Zero(t, s.Unrecognized)
}

func TestVerdictSummary_should_ComputeOverrideRateAndHandleEmptyInput(t *testing.T) {
	t.Parallel()
	assert.Zero(t, SummarizeVerdicts(nil).RawFailRate, "empty input must not divide by zero")

	vs := []Verdict{mkVerdict(OutcomePass, "ok", 1), mkVerdict(OutcomePass, "ok", 2), mkVerdict(OutcomeFail, sumJudgment, 3), mkVerdict(OutcomePass, "ok", 4)}
	vs[1].OverrideBy = "user"
	s := SummarizeVerdicts(vs)
	assert.Equal(t, 1, s.Overridden)
	assert.InDelta(t, 0.25, s.OverrideRate, 1e-9)
}

func TestVerdictJoin_should_CountFourNotThirtySix_When_ItemHasNineSessionsFourVerdicts(t *testing.T) {
	t.Parallel()
	rows := make([]SessionVerdictRow, 0, 36)
	for s := range 9 {
		for range 4 { // join inflation: every session row repeated per verdict row
			row := SessionVerdictRow{ItemID: "item", SessionID: SessionID(fmt.Sprintf("sess-%d", s))}
			if s < 4 {
				v := mkVerdict(OutcomeFail, sumJudgment, s)
				row.Verdict = &v
			}
			rows = append(rows, row)
		}
	}
	require.Len(t, rows, 36)

	got := JoinVerdictsBySession(rows)
	require.Len(t, got["item"], 4)
	for i := 1; i < len(got["item"]); i++ {
		assert.False(t, got["item"][i].CreatedAt.Before(got["item"][i-1].CreatedAt), "ordered by time")
	}
	assert.Equal(t, 4, SummarizeVerdicts(got["item"]).Total)
}

func TestVerdictMixSeries_should_ReturnZeroFilledWeeksAndMatchTotal_When_FiveIsoWeeks(t *testing.T) {
	t.Parallel()
	week := func(n int) time.Time { return verdictEpoch.AddDate(0, 0, 7*n) }
	at := func(w int, d time.Duration) time.Time { return week(w).Add(d) }
	vs := []Verdict{
		{SessionID: "a", CreatedAt: at(0, time.Hour), Outcome: OutcomeFail, Summary: sumJudgment},
		{SessionID: "b", CreatedAt: at(0, 24*time.Hour*6), Outcome: OutcomeFail, Summary: sumNoCommits},
		{SessionID: "c", CreatedAt: at(1, time.Hour), Outcome: OutcomePass, Summary: sumAuto},
		{SessionID: "d", CreatedAt: at(3, time.Hour), Outcome: OutcomeUnverifiable, Summary: sumSelfCheck},
		{SessionID: "e", CreatedAt: at(4, time.Hour), Outcome: OutcomePass, Summary: "ok"},
		{SessionID: "late", CreatedAt: at(5, time.Hour), Outcome: OutcomePass, Summary: "outside window"},
	}
	w, err := NewFlowWindowBounds(week(0), week(5))
	require.NoError(t, err)

	got := VerdictMixSeries(vs, w)
	require.Len(t, got, 5)
	total := 0
	for i, b := range got {
		assert.Equal(t, week(i), b.WeekStart)
		total += b.Total
	}
	assert.Equal(t, 5, total, "bucket totals equal the in-window total")
	assert.Equal(t, 2, got[0].Total)
	assert.Equal(t, 1, got[0].Count(ClassJudgment, OutcomeFail))
	assert.Equal(t, 1, got[0].Count(ClassNoCommits, OutcomeFail))
	assert.Zero(t, got[2].Total, "empty week is a real zero")
	assert.Zero(t, got[2].Count(ClassJudgment, OutcomePass))
	assert.Equal(t, 1, got[1].Count(ClassAutoVerified, OutcomePass))
}

func TestVerdictMixSeries_should_StartAtFirstVerdict_When_WindowIsAllTime(t *testing.T) {
	t.Parallel()
	now := verdictEpoch.AddDate(0, 0, 15)
	w, err := NewAllTimeWindow(FixedClock{T: now})
	require.NoError(t, err)
	vs := []Verdict{{SessionID: "a", CreatedAt: verdictEpoch.Add(time.Hour), Outcome: OutcomePass, Summary: "ok"}}

	got := VerdictMixSeries(vs, w)
	require.Len(t, got, 3, "weeks of Oct 5, 12 and 19, not 26 years of empty buckets")
	assert.Equal(t, verdictEpoch, got[0].WeekStart)
	assert.Nil(t, VerdictMixSeries(nil, w))
}
