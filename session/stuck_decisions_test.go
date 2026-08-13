package session

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tstapler/stapler-squad/github"
)

func TestStuckPRReady_should_returnTrue_When_GreenMergeablePast30Min(t *testing.T) {
	now := time.Now()
	assert.True(t, stuckPRReady(now.Add(-31*time.Minute), now))
}

func TestStuckPRReady_should_returnFalse_When_WithinThreshold(t *testing.T) {
	now := time.Now()
	assert.False(t, stuckPRReady(now.Add(-30*time.Minute), now), "exactly 30m must not flag")
	assert.False(t, stuckPRReady(now.Add(-29*time.Minute), now), "29m must not flag")
}

func TestPrReadyToMergeSolo_should_returnTrue_When_GreenMergeableUnapproved(t *testing.T) {
	info := &github.PRInfo{
		State:                 "open",
		IsDraft:               false,
		ChangesRequestedCount: 0,
		CheckConclusion:       "success",
		Mergeable:             "MERGEABLE",
		ApprovedCount:         0, // flagship single-user case: no approval, still ready
	}
	assert.True(t, prReadyToMergeSolo(info), "unapproved but green+mergeable PR must be ready-to-merge-solo")
}

func TestPrReadyToMergeSolo_should_returnFalse_When_BlockedOrConflictingOrFailing(t *testing.T) {
	base := func() *github.PRInfo {
		return &github.PRInfo{
			State:                 "open",
			IsDraft:               false,
			ChangesRequestedCount: 0,
			CheckConclusion:       "success",
			Mergeable:             "MERGEABLE",
			ApprovedCount:         0,
		}
	}

	changesRequested := base()
	changesRequested.ChangesRequestedCount = 1
	assert.False(t, prReadyToMergeSolo(changesRequested), "changes requested must block")

	ciFailing := base()
	ciFailing.CheckConclusion = "failure"
	assert.False(t, prReadyToMergeSolo(ciFailing), "failing CI must block")

	draft := base()
	draft.IsDraft = true
	assert.False(t, prReadyToMergeSolo(draft), "draft must block")

	conflicting := base()
	conflicting.Mergeable = "CONFLICTING"
	assert.False(t, prReadyToMergeSolo(conflicting), "conflicting must block")

	assert.False(t, prReadyToMergeSolo(nil), "nil info must not be ready")

	merged := base()
	merged.State = "MERGED"
	assert.False(t, prReadyToMergeSolo(merged), "merged state must not be ready (handled elsewhere)")

	closed := base()
	closed.State = "CLOSED"
	assert.False(t, prReadyToMergeSolo(closed), "closed state must not be ready")
}

func TestAbandonedReview_should_returnTrue_When_LastReviewOlderThan15Min(t *testing.T) {
	now := time.Now()
	assert.True(t, abandonedReview(now.Add(-16*time.Minute), now))
}

func TestAbandonedReview_should_returnFalse_When_WithinGrace(t *testing.T) {
	now := time.Now()
	assert.False(t, abandonedReview(now.Add(-15*time.Minute), now), "exactly 15m must not flag")
	assert.False(t, abandonedReview(now.Add(-5*time.Minute), now), "5m must not flag")
}

func TestStaleWork_should_returnTrue_When_LastProgressOlderThan2h(t *testing.T) {
	now := time.Now()
	assert.True(t, staleWork(now.Add(-3*time.Hour), now))
}

func TestStaleWork_should_returnFalse_When_ProgressWithin2h(t *testing.T) {
	now := time.Now()
	assert.False(t, staleWork(now.Add(-1*time.Hour), now))
}

func TestIsBouncing_should_returnTrue_When_ThreeCyclesNoPass(t *testing.T) {
	assert.True(t, isBouncing(3, false))
}

func TestIsBouncing_should_returnFalse_When_TwoCyclesOrHasPass(t *testing.T) {
	assert.False(t, isBouncing(2, false), "below threshold must not flag")
	assert.False(t, isBouncing(3, true), "a recorded PASS must not flag even at/above threshold")
}

func TestIsMultiReasonEscalated_should_returnTrue_When_CountAtOrAboveThreshold(t *testing.T) {
	assert.True(t, isMultiReasonEscalated(2), "exactly at threshold must flag")
	assert.True(t, isMultiReasonEscalated(3), "above threshold must flag")
}

func TestIsMultiReasonEscalated_should_returnFalse_When_CountBelowThreshold(t *testing.T) {
	assert.False(t, isMultiReasonEscalated(1), "below threshold must not flag")
	assert.False(t, isMultiReasonEscalated(0), "zero must not flag")
}

func TestMultiReasonEscalationNotifyReady_should_returnTrue_When_DwellElapsed(t *testing.T) {
	now := time.Now()
	assert.True(t, multiReasonEscalationNotifyReady(now.Add(-61*time.Second), now))
	assert.True(t, multiReasonEscalationNotifyReady(now.Add(-60*time.Second), now), "exactly 60s must flag (>=)")
}

func TestMultiReasonEscalationNotifyReady_should_returnFalse_When_WithinDwell(t *testing.T) {
	now := time.Now()
	assert.False(t, multiReasonEscalationNotifyReady(now.Add(-30*time.Second), now), "30s must not flag")
	assert.False(t, multiReasonEscalationNotifyReady(now, now), "0s must not flag")
}

func TestIsRepeatedFailure_should_returnTrue_When_LastTwoVerdictsIdentical(t *testing.T) {
	recent := []ReviewVerdictSummary{
		{OverallOutcome: string(ReviewOutcomeFail), Summary: "diff computation failed"},
		{OverallOutcome: string(ReviewOutcomeFail), Summary: "diff computation failed"},
	}
	assert.True(t, IsRepeatedFailure(recent))
}

func TestIsRepeatedFailure_should_returnFalse_When_FewerThanTwoVerdicts(t *testing.T) {
	assert.False(t, IsRepeatedFailure(nil))
	assert.False(t, IsRepeatedFailure([]ReviewVerdictSummary{{OverallOutcome: string(ReviewOutcomeFail), Summary: "x"}}))
}

func TestIsRepeatedFailure_should_returnFalse_When_LatestIsPass(t *testing.T) {
	recent := []ReviewVerdictSummary{
		{OverallOutcome: string(ReviewOutcomePass), Summary: "diff computation failed"},
		{OverallOutcome: string(ReviewOutcomeFail), Summary: "diff computation failed"},
	}
	assert.False(t, IsRepeatedFailure(recent), "a PASS must never be treated as a repeated failure")
}

func TestIsRepeatedFailure_should_returnFalse_When_SummariesDiffer(t *testing.T) {
	recent := []ReviewVerdictSummary{
		{OverallOutcome: string(ReviewOutcomeFail), Summary: "missing acceptance criterion 2"},
		{OverallOutcome: string(ReviewOutcomeFail), Summary: "diff computation failed"},
	}
	assert.False(t, IsRepeatedFailure(recent), "different failure reasons must not trip the breaker")
}

func TestIsRepeatedFailure_should_returnFalse_When_SummariesAreBothEmpty(t *testing.T) {
	recent := []ReviewVerdictSummary{
		{OverallOutcome: string(ReviewOutcomeFail), Summary: ""},
		{OverallOutcome: string(ReviewOutcomeFail), Summary: ""},
	}
	assert.False(t, IsRepeatedFailure(recent), "two empty summaries carry no signal and must not trip the breaker")
}

func TestIsRepeatedNoVerdictFailure_should_returnTrue_When_LastTwoReviewsHadNoVerdict(t *testing.T) {
	assert.True(t, IsRepeatedNoVerdictFailure([]bool{false, false}),
		"two consecutive review sessions with no verdict at all must trip the breaker")
}

func TestIsRepeatedNoVerdictFailure_should_returnFalse_When_FewerThanTwoReviews(t *testing.T) {
	assert.False(t, IsRepeatedNoVerdictFailure(nil))
	assert.False(t, IsRepeatedNoVerdictFailure([]bool{false}), "a single no-verdict review must not trip the breaker")
}

func TestIsRepeatedNoVerdictFailure_should_returnFalse_When_EitherReviewHadAVerdict(t *testing.T) {
	assert.False(t, IsRepeatedNoVerdictFailure([]bool{true, false}), "latest had a verdict — not a repeat of nothing")
	assert.False(t, IsRepeatedNoVerdictFailure([]bool{false, true}), "prior had a verdict — not a repeat of nothing")
	assert.False(t, IsRepeatedNoVerdictFailure([]bool{true, true}), "both had verdicts — IsRepeatedFailure's job, not this one")
}

func TestIsFlakyVerdictFlipFlop_should_returnTrue_When_SameDiffHashDifferentOutcome(t *testing.T) {
	recent := []ReviewVerdictSummary{
		{DiffHash: "abc123", OverallOutcome: string(ReviewOutcomePass)},
		{DiffHash: "abc123", OverallOutcome: string(ReviewOutcomeFail)},
	}
	assert.True(t, IsFlakyVerdictFlipFlop(recent), "identical diff, different outcome is the definition of a flip-flop")
}

func TestIsFlakyVerdictFlipFlop_should_returnFalse_When_FewerThanTwoVerdicts(t *testing.T) {
	assert.False(t, IsFlakyVerdictFlipFlop(nil))
	assert.False(t, IsFlakyVerdictFlipFlop([]ReviewVerdictSummary{{DiffHash: "abc123", OverallOutcome: string(ReviewOutcomeFail)}}))
}

func TestIsFlakyVerdictFlipFlop_should_returnFalse_When_EitherDiffHashEmpty(t *testing.T) {
	latestEmpty := []ReviewVerdictSummary{
		{DiffHash: "", OverallOutcome: string(ReviewOutcomePass)},
		{DiffHash: "abc123", OverallOutcome: string(ReviewOutcomeFail)},
	}
	assert.False(t, IsFlakyVerdictFlipFlop(latestEmpty), "an unknown (empty) diff hash must never be treated as a match")

	priorEmpty := []ReviewVerdictSummary{
		{DiffHash: "abc123", OverallOutcome: string(ReviewOutcomePass)},
		{DiffHash: "", OverallOutcome: string(ReviewOutcomeFail)},
	}
	assert.False(t, IsFlakyVerdictFlipFlop(priorEmpty), "an unknown (empty) diff hash must never be treated as a match")

	bothEmpty := []ReviewVerdictSummary{
		{DiffHash: "", OverallOutcome: string(ReviewOutcomePass)},
		{DiffHash: "", OverallOutcome: string(ReviewOutcomeFail)},
	}
	assert.False(t, IsFlakyVerdictFlipFlop(bothEmpty), "two unknowns carry no signal")
}

func TestIsFlakyVerdictFlipFlop_should_returnFalse_When_OutcomesAgree(t *testing.T) {
	recent := []ReviewVerdictSummary{
		{DiffHash: "abc123", OverallOutcome: string(ReviewOutcomeFail)},
		{DiffHash: "abc123", OverallOutcome: string(ReviewOutcomeFail)},
	}
	assert.False(t, IsFlakyVerdictFlipFlop(recent), "matching outcomes is IsRepeatedFailure's job, not this one")
}

func TestIsFlakyVerdictFlipFlop_should_returnFalse_When_DiffHashesDiffer(t *testing.T) {
	recent := []ReviewVerdictSummary{
		{DiffHash: "abc123", OverallOutcome: string(ReviewOutcomePass)},
		{DiffHash: "def456", OverallOutcome: string(ReviewOutcomeFail)},
	}
	assert.False(t, IsFlakyVerdictFlipFlop(recent), "a genuinely different diff explains a different outcome — not flaky")
}

func TestIsTestOnlyReworkCycle_should_returnTrue_When_AllAttemptsTouchOnlyTestFiles(t *testing.T) {
	attempts := [][]string{
		{"session/stuck_decisions_test.go"},
		{"web-app/src/lib/omnibar/detector.test.ts", "session/stuck_decisions_test.go"},
	}
	assert.True(t, IsTestOnlyReworkCycle(attempts))
}

func TestIsTestOnlyReworkCycle_should_returnFalse_When_FewerThanMinAttempts(t *testing.T) {
	assert.False(t, IsTestOnlyReworkCycle(nil))
	assert.False(t, IsTestOnlyReworkCycle([][]string{{"session/stuck_decisions_test.go"}}), "only one attempt of history — not enough to call it a cycle")
}

func TestIsTestOnlyReworkCycle_should_returnFalse_When_AnyCycleHasNoFileData(t *testing.T) {
	attempts := [][]string{
		{"session/stuck_decisions_test.go"},
		{},
	}
	assert.False(t, IsTestOnlyReworkCycle(attempts), "no file data for an attempt is no signal, not a match")
}

func TestIsTestOnlyReworkCycle_should_returnFalse_When_AnyCycleTouchesNonTestFile(t *testing.T) {
	attempts := [][]string{
		{"session/stuck_decisions_test.go"},
		{"session/stuck_decisions.go", "session/stuck_decisions_test.go"},
	}
	assert.False(t, IsTestOnlyReworkCycle(attempts), "a production file in any attempt breaks the test-only cycle")
}

func TestMostRecentCompletedWorkSession_should_returnLatestCompletedWorkSession_When_Present(t *testing.T) {
	endedAt := time.Now()
	sessions := []ItemSessionSummary{
		{ID: "s1", Role: SessionRoleWork, EndedAt: &endedAt},
		{ID: "s2", Role: SessionRoleReview, EndedAt: &endedAt},
		{ID: "s3", Role: SessionRoleWork, EndedAt: &endedAt},
	}
	got := MostRecentCompletedWorkSession(sessions)
	assert.NotNil(t, got)
	assert.Equal(t, "s3", got.ID, "must pick the most recent (last in oldest-first order) completed work session")
}

func TestMostRecentCompletedWorkSession_should_returnNil_When_NoCompletedWorkSession(t *testing.T) {
	assert.Nil(t, MostRecentCompletedWorkSession(nil))

	stillRunning := []ItemSessionSummary{{ID: "s1", Role: SessionRoleWork, EndedAt: nil}}
	assert.Nil(t, MostRecentCompletedWorkSession(stillRunning), "an in-progress session must not be picked — see validation.md's async-race edge case")

	reviewOnly := []ItemSessionSummary{{ID: "s1", Role: SessionRoleReview, EndedAt: &time.Time{}}}
	assert.Nil(t, MostRecentCompletedWorkSession(reviewOnly))
}
