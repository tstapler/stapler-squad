package services

// Inspect-and-nudge before the stale-work restart: every observed state at
// the InspectStaleWork entry point, and RemediateStaleWorkSession's handling of
// each disposition. The nudger is a fake; the stuck row and nudge claim are real.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

func (f *stallNudgeFixture) inspect(state domain.ObservedState) StaleWorkDisposition {
	f.svc.AutonomousSvc.staleSnapshot = func(*instance.Instance) domain.StopSnapshot { return snapshotFor(state) }
	item, err := f.storage.GetBacklogItem(context.Background(), f.itemID)
	if err != nil {
		panic(err)
	}
	return f.svc.AutonomousSvc.InspectStaleWork(context.Background(), f.inst.UUID, item)
}

func TestInspectStaleWork_should_nudgeOnlyIdlePrompt_When_AnyObservedState(t *testing.T) {
	for _, state := range domain.AllObservedStates {
		t.Run(string(state), func(t *testing.T) {
			f := newStallNudgeFixture(t, "stale-"+string(state), domain.BacklogStatusInProgress, true, true)

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
			f := newStallNudgeFixture(t, "stale-no-nudge", domain.BacklogStatusInProgress, tc.diagnose, tc.stall)
			if !tc.autonomous {
				f.inst.SetAutonomousMode(false, "")
			}

			assert.Equal(t, StaleWorkRespawn, f.inspect(domain.ObservedIdlePrompt))
			assert.Zero(t, f.nudger.callCount())
		})
	}
}

func TestInspectStaleWork_should_skip_When_StallRecoveryLive(t *testing.T) {
	f := newStallNudgeFixture(t, "stale-recovery-live", domain.BacklogStatusInProgress, true, true)
	_, release := f.svc.AutonomousSvc.beginRecovery(f.title)
	defer release()
	captured := false
	f.svc.AutonomousSvc.staleSnapshot = func(*instance.Instance) domain.StopSnapshot {
		captured = true
		return snapshotFor(domain.ObservedIdlePrompt)
	}
	item, err := f.storage.GetBacklogItem(context.Background(), f.itemID)
	require.NoError(t, err)

	got := f.svc.AutonomousSvc.InspectStaleWork(context.Background(), f.inst.UUID, item)

	assert.Equal(t, StaleWorkSkipped, got)
	assert.Zero(t, f.nudger.callCount())
	assert.False(t, captured, "a live recovery owns the pane; do not even capture")
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

func TestRecordDeadPane_should_writeStuckRow_When_WorkSessionLive(t *testing.T) {
	f := newStallNudgeFixture(t, "dead-pane-row", domain.BacklogStatusInProgress, true, true)

	f.svc.AutonomousSvc.RecordDeadPane(f.inst.UUID, f.title, snapshotFor(domain.ObservedCrashed), domain.ObservedCrashed)

	assert.Contains(t, f.rowContext(t), "stop_reason=dead_pane state=crashed action=respawned")
	assert.Zero(t, f.nudger.callCount(), "a dead pane is never nudged")
}

func TestRecordDeadPane_should_storeSanitizedTail_When_SnapshotCarriesPaneTail(t *testing.T) {
	cases := map[string]struct {
		tail string
		want string
	}{
		"plain tail folded onto one line": {"building\npanic: boom", "building ⏎ panic: boom"},
		"secret tail withheld":            {"ok\nAPI_TOKEN=abcdef123456\nok", stopTailWithheldMarker},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newStallNudgeFixture(t, "dead-pane-tail", domain.BacklogStatusInProgress, true, true)
			snap := snapshotFor(domain.ObservedCrashed)
			snap.PaneTail = tc.tail

			f.svc.AutonomousSvc.RecordDeadPane(f.inst.UUID, f.title, snap, domain.ObservedCrashed)

			got := f.rowContext(t)
			assert.Contains(t, got, tc.want)
			if tc.want == stopTailWithheldMarker {
				assert.NotContains(t, got, "abcdef123456")
			}
		})
	}
}

func TestRecordDeadPane_should_beNoop_When_SessionHasNoLiveWorkItemSession(t *testing.T) {
	f := newStallNudgeFixture(t, "dead-pane-noop", domain.BacklogStatusInProgress, true, true)

	f.svc.AutonomousSvc.RecordDeadPane("unlinked-uuid", f.title, snapshotFor(domain.ObservedCrashed), domain.ObservedCrashed)

	open, err := f.storage.FindOpenStuckStates(context.Background())
	require.NoError(t, err)
	assert.Empty(t, open)
}
