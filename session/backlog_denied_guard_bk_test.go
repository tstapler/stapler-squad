package session

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/session/domain"
)

func TestSelfHealStuck_should_ResolveATransitionDeniedRowOnlyOnceTheItemLeftItsFromStatus(t *testing.T) {
	t.Parallel()
	h := newGateHarness(t, ModeEnforce)
	ctx := context.Background()
	item := h.newItem(t, BacklogStatusIdea)
	denOpenDenial(t, h, item.ID, CallerAutonomousComplete, BacklogStatusIdea, BacklogStatusReady, DeniedOperatorGate)
	l := NewBacklogLifecycleListener(h.storage)

	l.selfHealStuck(ctx, h.repo)
	assert.Len(t, denOpenRows(t, h, item.ID, domain.StuckReasonTransitionDenied), 1, "still anchored at idea")

	h.moveRaw(t, item.ID, BacklogStatusReady)
	l.selfHealStuck(ctx, h.repo)
	assert.Empty(t, denOpenRows(t, h, item.ID, domain.StuckReasonTransitionDenied), "the item moved on")
}

func TestReconcileOrphanedTriageItems_should_NotMarkOrphaned_When_IdeaHasAnOpenDenial(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		denied  bool
		wantRow int
	}{{"open denial", true, 0}, {"no denial", false, 1}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newGateHarness(t, ModeEnforce)
			ctx := context.Background()
			denBlock(t, h, BacklogStatusIdea, BacklogStatusReady)
			item := newEndedTriageTestItem(t, h.storage, h.repo)
			if tc.denied {
				denOpenDenial(t, h, item.ID, CallerAutonomousComplete, BacklogStatusIdea, BacklogStatusReady, DeniedOperatorGate)
			}

			NewBacklogLifecycleListener(h.storage).reconcileOrphanedTriageItems(ctx, h.repo)

			assert.Len(t, denOpenRows(t, h, item.ID, domain.StuckReasonOrphanedTriage), tc.wantRow)
		})
	}
}

func TestReconcileOrphanedTriageRemediation_should_SpendNoAttempt_When_IdeaHasAnOpenDenial(t *testing.T) {
	t.Parallel()
	h := newGateHarness(t, ModeEnforce)
	ctx := context.Background()
	denBlock(t, h, BacklogStatusIdea, BacklogStatusReady)
	item := newEndedTriageTestItem(t, h.storage, h.repo)
	_, err := h.storage.MarkStuck(ctx, item.ID, domain.StuckReasonOrphanedTriage, BacklogStatusIdea, "pre-existing orphan row")
	require.NoError(t, err)
	denOpenDenial(t, h, item.ID, CallerAutonomousComplete, BacklogStatusIdea, BacklogStatusReady, DeniedOperatorGate)

	NewBacklogLifecycleListener(h.storage).reconcileOrphanedTriageRemediation(ctx, h.repo)

	rows := denOpenRows(t, h, item.ID, domain.StuckReasonOrphanedTriage)
	require.Len(t, rows, 1)
	assert.Zero(t, rows[0].RemediationAttempts)
}

func TestReconcileOrphanedAgentPRs_should_SkipTheGitHubLookup_When_ReviewHasAnOpenDenial(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		denied    bool
		wantCalls int
	}{{"open denial", true, 0}, {"no denial", false, 1}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newGateHarness(t, ModeEnforce)
			denBlock(t, h, BacklogStatusReview, BacklogStatusPRPending)
			item := newOrphanedAgentPRTestItem(t, h.storage, "backlog/den-orphan")
			if tc.denied {
				denOpenDenial(t, h, item.ID, CallerOrphanedAgentPR, BacklogStatusReview, BacklogStatusPRPending, DeniedOperatorGate)
			}
			lookups := 0
			l := NewBacklogLifecycleListener(h.storage)
			l.SetOrphanedPRFinder(func(context.Context, string, string) (*github.PRInfo, error) {
				lookups++
				return nil, github.ErrNoPR
			})

			l.reconcileOrphanedAgentPRs(context.Background(), h.repo)

			assert.Equal(t, tc.wantCalls, lookups)
		})
	}
}

func TestReconcilePushFailedItems_should_SpendNoAttempt_When_ReviewHasAnOpenDenial(t *testing.T) {
	t.Parallel()
	h := newGateHarness(t, ModeEnforce)
	ctx := context.Background()
	denBlock(t, h, BacklogStatusReview, BacklogStatusPRPending)
	item := h.newItem(t, BacklogStatusReview)
	_, err := h.storage.MarkStuck(ctx, item.ID, domain.StuckReasonPushFailed, BacklogStatusReview, "push rejected")
	require.NoError(t, err)
	denOpenDenial(t, h, item.ID, CallerPushCreatePRFallback, BacklogStatusReview, BacklogStatusPRPending, DeniedOperatorGate)

	NewBacklogLifecycleListener(h.storage).reconcilePushFailedItems(ctx, h.repo)

	rows := denOpenRows(t, h, item.ID, domain.StuckReasonPushFailed)
	require.Len(t, rows, 1)
	assert.Zero(t, rows[0].RemediationAttempts, "a held item spends no backoff attempt")
}

func TestReconcileUnprocessedReviewVerdicts_should_NotApplyTheVerdict_When_ReviewHasAnOpenDenial(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		denied     bool
		wantStatus BacklogStatus
	}{{"open denial", true, BacklogStatusReview}, {"no denial", false, BacklogStatusDone}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newGateHarness(t, ModeEnforce)
			ctx := context.Background()
			item := newStuckReviewTestItem(t, h.storage, ReviewVerdictPass, false, true)
			if tc.denied {
				denBlock(t, h, BacklogStatusReview, BacklogStatusDone)
				denOpenDenial(t, h, item.ID, CallerPushCreatePRFallback, BacklogStatusReview, BacklogStatusDone, DeniedOperatorGate)
			}
			l := NewBacklogLifecycleListener(h.storage)
			l.SetSessionLivenessChecker(func(sessionUUID string) bool { return !strings.HasPrefix(sessionUUID, "headless-review-") })

			l.reconcileUnprocessedReviewVerdicts(ctx, h.repo)

			assert.Equal(t, string(tc.wantStatus), h.status(t, item.ID))
		})
	}
}

func TestReconcileMultiReasonEscalation_should_TreatALoneTransitionDeniedRowAsNonEscalating(t *testing.T) {
	t.Parallel()
	h := newGateHarness(t, ModeEnforce)
	ctx := context.Background()
	denBlock(t, h, BacklogStatusIdea, BacklogStatusReady)
	item := h.newItem(t, BacklogStatusIdea)
	denOpenDenial(t, h, item.ID, CallerAutonomousComplete, BacklogStatusIdea, BacklogStatusReady, DeniedOperatorGate)

	NewBacklogLifecycleListener(h.storage).reconcileMultiReasonEscalation(ctx, h.repo)

	assert.Empty(t, denOpenRows(t, h, item.ID, domain.StuckReasonMultipleReasons))
}
