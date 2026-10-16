package services

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// T-RO-39 (Task 5.1d-d): the PR-fix steer and the verdict steer, the two
// interface-dispatched callers of SteerActiveSession/SteerSessionGuarded, never
// write to a hidden session; they degrade to their notify-only branch.
func TestPrFixSteerAndVerdictSteer_ShouldRefuseAHiddenTargetAndDegradeToNotifyOnly_WhenTheSessionIsHidden(t *testing.T) {
	t.Run("PR-fix steer", func(t *testing.T) {
		svc, steerer, _ := newTestBacklogServiceForSteerIntegration(t, activeSessionUUIDPrimary, "claude")
		steerer.hidden = map[string]bool{activeSessionUUIDPrimary: true}
		itemID := createPRPendingItemWithActiveSession(t, svc.storage, activeSessionUUIDPrimary)

		require.NoError(t, svc.AutoReopenForPRFix(context.Background(), itemID, ciOnlyFixContext))

		assert.Empty(t, steerer.calls(), "no write to a hidden session")
		assert.Empty(t, steerer.guardedSigs, "not even the guarded steer is attempted")
		assert.True(t, openStuckReasons(t, svc)[domain.StuckReasonRespawnBlockedActive], "degraded to notify-only")
	})

	t.Run("verdict steer", func(t *testing.T) {
		svc, steerer, bus, item := newVerdictSteerFixture(t, true)
		steerer.hidden = map[string]bool{verdictSteerSessionUUID: true}
		svc.StartVerdictSteering()

		publishVerdict(bus, item, session.ReviewOutcomePass, "all criteria met")

		assert.Never(t, func() bool { return len(steerer.calls()) > 0 }, 300*time.Millisecond, 20*time.Millisecond,
			"no write to a hidden session")
	})

	t.Run("both still steer a visible session", func(t *testing.T) {
		svc, steerer, bus, item := newVerdictSteerFixture(t, true)
		svc.StartVerdictSteering()
		publishVerdict(bus, item, session.ReviewOutcomePass, "ok")
		require.Eventually(t, func() bool { return len(steerer.calls()) == 1 }, 3*time.Second, 10*time.Millisecond)
	})
}
