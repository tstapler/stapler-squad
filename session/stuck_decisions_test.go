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
