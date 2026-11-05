package backlogsvc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

// TestAutonomousOrchestrationService_OnAutonomousDriverComplete_WorkStuck_RespawnsInsteadOfSelfBlocking
// exercises a stuck (outcome.Done=false) SessionRoleWork completion against a
// *real* BacklogService, with the driver's own ItemSession still reported
// live by the session stopper — AutonomousDriver.run stopping never kills the
// tmux pane, so a respawn that doesn't close the stuck session out first
// finds that same still-open session via findActiveWorkSession and
// self-blocks with RespawnBlockedActive instead of spawning a fresh one.
// Neither TestAutonomousOrchestrationService_OnAutonomousDriverComplete_NotifiesOperator_When_RespawnAttemptFails
// (a fake respawner, never reaches BacklogService) nor
// TestAutoRespawnAutonomousWork_ActiveWorkSession_RecordsRespawnBlockedActive
// (calls AutoRespawnAutonomousWork directly, skipping the driver-complete
// entrypoint) covers this path.
func TestAutonomousOrchestrationService_OnAutonomousDriverComplete_WorkStuck_RespawnsInsteadOfSelfBlocking(t *testing.T) {
	t.Parallel()
	storage := svctest.CreateTestStorage(t)
	ctx, lifecycleCancel := context.WithCancel(context.Background())
	t.Cleanup(lifecycleCancel)
	eventBus := events.NewEventBus(4)
	svc := services.NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })
	svc.SetLifecycleContext(ctx)

	repoPath := t.TempDir()
	svctest.InitGitRepoWithCommit(t, repoPath)

	const title = "autonomous-work-respawn-test"
	inst := &session.Instance{
		Title: title, UUID: title + "-uuid", Path: repoPath,
		Status: session.Paused, Program: "claude", AutonomousMode: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, storage.AddInstance(inst))
	svc.AutonomousSvc.SetInstanceFinder(func(_ string) *session.Instance { return inst })

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:    "Autonomous work respawn test item",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: inst.UUID,
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	creator := &mockSessionCreator{}
	stopper := &svctest.MockSessionStopper{LiveUUIDs: map[string]bool{inst.UUID: true}}
	backlogSvc := NewBacklogService(storage, creator, nil, nil, nil, nil)
	backlogSvc.SetSessionStopper(stopper)
	backlogSvc.SetEventBus(eventBus)
	svc.SetAutonomousStuckRespawner(backlogSvc)

	outcome := session.AutonomousDriverOutcome{Done: false, Reason: "no DONE signal", Turns: 20, Stuck: true}
	svc.AutonomousSvc.OnAutonomousDriverComplete(title, outcome)

	// The respawn runs in a goroutine and takes ~1s under -race when idle; the bound is a hang guard, not a latency assertion.
	wait.RequireEventually(t, func() bool {
		return creator.callCount() == 1
	}, 30*time.Second, 10*time.Millisecond, "the stuck work session must be closed out before the respawn dispatch, so AutoRespawnAutonomousWork spawns a fresh session instead of self-blocking on the very session that just reported stuck")

	open, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	for _, s := range open {
		assert.NotEqual(t, domain.StuckReasonRespawnBlockedActive, s.Reason, "must not self-block on the stuck session it is trying to replace")
	}
}

func TestOnAutonomousDriverComplete_should_LeaveIdeaAndRaiseOneDenial_When_SystemGateBlocksIdeaToReady(t *testing.T) {
	f := newCharFixture(t, "autonomous-denied", session.SessionRoleTriage, session.BacklogStatusIdea)
	bs := NewBacklogService(f.storage, &mockSessionCreator{}, nil, nil, nil, nil)
	h := wireEnforcedGate(t, bs)
	h.blockWithGate(t, session.BacklogStatusIdea, session.BacklogStatusReady)

	f.complete(session.AutonomousDriverOutcome{Done: true, Reason: "done", Turns: 3})

	item, err := f.storage.GetBacklogItem(context.Background(), f.itemID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusIdea), item.Status)
	assert.Equal(t, []string{string(session.CallerAutonomousComplete)}, deniedCallers(t, bs, f.itemID))
	assert.Equal(t, []string{"Autonomous fix complete — status update failed"}, svctest.NotificationTitles(f.drain()), "the existing status-update-failed notification fires once")
}
