package services

// Inspect-and-nudge before the stale-work restart: every observed state at
// the InspectStaleWork entry point, and RemediateStaleWorkSession's handling of
// each disposition. The nudger is a fake; the stuck row and nudge claim are real.

import (
	"context"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

func (f *stallNudgeFixture) inspect(state domain.ObservedState) StaleWorkDisposition {
	f.svc.autonomousSvc.staleSnapshot = func(*session.Instance) domain.StopSnapshot { return snapshotFor(state) }
	item, err := f.storage.GetBacklogItem(context.Background(), f.itemID)
	if err != nil {
		panic(err)
	}
	return f.svc.autonomousSvc.InspectStaleWork(context.Background(), f.inst.UUID, item)
}

func TestInspectStaleWork_should_nudgeOnlyIdlePrompt_When_AnyObservedState(t *testing.T) {
	for _, state := range domain.AllObservedStates {
		t.Run(string(state), func(t *testing.T) {
			f := newStallNudgeFixture(t, "stale-"+string(state), session.BacklogStatusInProgress, true, true)

			got := f.inspect(state)

			if state == domain.ObservedIdlePrompt {
				assert.Equal(t, StaleWorkNudged, got)
				assert.Equal(t, 1, f.nudger.callCount())
				row := f.rowContext(t)
				assert.Contains(t, row, "stop_reason=stale_work state=idle_prompt action=nudged")
				return
			}
			assert.Equal(t, StaleWorkRespawn, got)
			assert.Zero(t, f.nudger.callCount(), "only an idle prompt is ever typed into")
			assert.Contains(t, f.rowContext(t), "stop_reason=stale_work state="+string(state)+" action=respawned")
		})
	}
}

func TestInspectStaleWork_should_respawn_When_NudgeNotAllowed(t *testing.T) {
	cases := []struct {
		name            string
		diagnose, stall bool
		autonomous      bool
	}{
		{"flags off", false, false, true},
		{"interactive session", true, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newStallNudgeFixture(t, "stale-no-nudge", session.BacklogStatusInProgress, tc.diagnose, tc.stall)
			if !tc.autonomous {
				f.inst.SetAutonomousMode(false, "")
			}

			assert.Equal(t, StaleWorkRespawn, f.inspect(domain.ObservedIdlePrompt))
			assert.Zero(t, f.nudger.callCount())
		})
	}
}

func TestInspectStaleWork_should_skip_When_StallRecoveryLive(t *testing.T) {
	f := newStallNudgeFixture(t, "stale-recovery-live", session.BacklogStatusInProgress, true, true)
	_, release := f.svc.autonomousSvc.beginRecovery(f.title)
	defer release()
	captured := false
	f.svc.autonomousSvc.staleSnapshot = func(*session.Instance) domain.StopSnapshot {
		captured = true
		return snapshotFor(domain.ObservedIdlePrompt)
	}
	item, err := f.storage.GetBacklogItem(context.Background(), f.itemID)
	require.NoError(t, err)

	got := f.svc.autonomousSvc.InspectStaleWork(context.Background(), f.inst.UUID, item)

	assert.Equal(t, StaleWorkSkipped, got)
	assert.Zero(t, f.nudger.callCount())
	assert.False(t, captured, "a live recovery owns the pane; do not even capture")
}

type fakeStaleInspector struct{ result StaleWorkDisposition }

func (f fakeStaleInspector) InspectStaleWork(context.Context, string, *session.BacklogItemData) StaleWorkDisposition {
	return f.result
}

func TestRemediateStaleWorkSession_should_leaveSessionOpen_When_InspectorNudgesOrSkips(t *testing.T) {
	for name, disp := range map[string]StaleWorkDisposition{"nudged": StaleWorkNudged, "skipped": StaleWorkSkipped, "respawn": StaleWorkRespawn} {
		t.Run(name, func(t *testing.T) {
			storage := createTestStorage(t)
			creator := &mockSessionCreator{}
			svc := NewBacklogService(storage, creator, nil, nil, nil, nil)
			stopper := &mockSessionStopper{liveUUIDs: map[string]bool{"stale-uuid": true}}
			svc.SetSessionStopper(stopper)
			svc.SetStaleWorkInspector(fakeStaleInspector{result: disp})
			repoPath := t.TempDir()
			initGitRepoWithCommit(t, repoPath)
			itemID := createReadyItemForSpawn(t, svc, repoPath, "stale item "+name)
			_, err := storage.CreateItemSession(t.Context(), session.ItemSessionData{ItemID: itemID, SessionUUID: "stale-uuid", SessionRole: session.SessionRoleWork})
			require.NoError(t, err)
			_, err = storage.TransitionBacklogItemStatus(t.Context(), itemID, session.BacklogStatusInProgress, nil, session.TriggeredBySystem)
			require.NoError(t, err)

			require.NoError(t, svc.RemediateStaleWorkSession(t.Context(), itemID))

			sessions, err := storage.ListItemSessions(t.Context(), itemID)
			require.NoError(t, err)
			restarted := disp == StaleWorkRespawn
			assert.Equal(t, restarted, slices.Contains(stopper.killedPaneUUIDs, "stale-uuid"))
			assert.Equal(t, restarted, len(creator.calls) == 1)
			for _, is := range sessions {
				if is.SessionUUID == "stale-uuid" {
					assert.Equal(t, restarted, is.EndedAt != nil)
				}
			}
		})
	}
}

func TestTryBeginRecovery_should_refuseAndKeepLiveEntry_When_RecoveryAlreadyRegistered(t *testing.T) {
	a := &AutonomousOrchestrationService{}
	_, release, ok := a.tryBeginRecovery("s")
	require.True(t, ok)

	_, _, ok2 := a.tryBeginRecovery("s")
	assert.False(t, ok2, "second registration must not overwrite the live recovery")
	assert.True(t, a.recoveryLive("s"))

	release()
	assert.False(t, a.recoveryLive("s"))
}
