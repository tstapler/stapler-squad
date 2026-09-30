package session

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// reviveOutcomeRecorder is a test LifecycleListener that records the reason
// string of every EventStarted it observes.
type reviveOutcomeRecorder struct {
	reasons []string
}

func (r *reviveOutcomeRecorder) OnLifecycleEvent(event LifecycleEvent, reason string) {
	if event == EventStarted {
		r.reasons = append(r.reasons, reason)
	}
}

// TestReviveOutcomeForColdRestore covers all four ReviveOutcome branches of
// the pure decision function directly, without the tmux/actor machinery the
// integration tests below require.
func TestReviveOutcomeForColdRestore(t *testing.T) {
	tests := []struct {
		name                  string
		hadUUIDBeforeRecovery bool
		hasClaudeSession      bool
		everHadHistory        bool
		want                  ReviveOutcome
	}{
		{"already had UUID, no recovery needed", true, true, false, ReviveOutcomeResumeLive},
		{"no UUID, recovery found one", false, true, false, ReviveOutcomeResumeRecovered},
		{"no UUID, recovery found nothing, never had history", false, false, false, ReviveOutcomeFreshExpected},
		{"no UUID, recovery found nothing, had history before", false, false, true, ReviveOutcomeFreshLostHistory},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reviveOutcomeForColdRestore(tt.hadUUIDBeforeRecovery, tt.hasClaudeSession, tt.everHadHistory)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestColdRestore_SignalsFreshLostHistory_When_RecoveryFailsButEverHadHistory
// verifies session-revive-uuid-loss AC3/AC4: when a cold restore's recovery
// attempt finds nothing (no live pane, no recoverable JSONL) but
// EverHadConversationHistory is true, both start entry points
// (startLocked via Start(), and the legacy start() via StartWithCleanup())
// set LastReviveOutcome to ReviveOutcomeFreshLostHistory and fire
// EventStarted with ReasonColdRestoreLostHistory.
func TestColdRestore_SignalsFreshLostHistory_When_RecoveryFailsButEverHadHistory(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test that starts real tmux sessions")
	}
	checkTmuxAvailable(t)

	tests := []struct {
		name    string
		startFn func(inst *Instance) error
	}{
		{
			name: "startLocked (actor-routed Start)",
			startFn: func(inst *Instance) error {
				return inst.Start(false)
			},
		},
		{
			name: "legacy start (StartWithCleanup)",
			startFn: func(inst *Instance) error {
				startCleanup, err := inst.StartWithCleanup(false)
				if startCleanup != nil {
					t.Cleanup(func() { _ = startCleanup() })
				}
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			title := fmt.Sprintf("test-cold-lost-history-%d", time.Now().UnixNano())
			inst, cleanup, err := NewInstanceWithCleanup(InstanceOptions{
				Title:            title,
				Path:             t.TempDir(),
				Program:          "sleep 300",
				SessionType:      SessionTypeDirectory,
				AutoYes:          false,
				TmuxPrefix:       fmt.Sprintf("test_coldrestore_%d_", time.Now().UnixNano()),
				TmuxServerSocket: coldRestoreSocket(t),
			})
			require.NoError(t, err)
			defer func() {
				if cleanupErr := cleanup(); cleanupErr != nil {
					t.Logf("cleanup warning: %v", cleanupErr)
				}
			}()

			// Simulate a session that had a captured conversation before, but the
			// in-memory UUID and any recoverable JSONL are both gone by the time of
			// this restart (e.g. the JSONL was deleted, or it lives under a home
			// dir this test's real inspector cannot see — no historyDetector
			// override, so DetectByPath finds nothing for this fresh t.TempDir()
			// path, matching TestColdRestore_WithoutUUID's assumption).
			inst.EverHadConversationHistory = true

			recorder := &reviveOutcomeRecorder{}
			inst.RegisterLifecycleListener(recorder)

			assert.False(t, inst.TmuxAlive(), "tmux session must be dead before cold restore")

			require.NoError(t, tt.startFn(inst), "cold start with lost history should not error")

			assert.Equal(t, ReviveOutcomeFreshLostHistory, inst.LastReviveOutcome)
			require.Len(t, recorder.reasons, 1, "expected exactly one EventStarted")
			assert.Equal(t, ReasonColdRestoreLostHistory, recorder.reasons[0])
		})
	}
}

// TestColdRestore_NoSignal_When_GenuinelyNeverHadHistory is the negative case
// for session-revive-uuid-loss AC3/AC6: a session that never captured a
// conversation (EverHadConversationHistory stays false, the zero value) must
// start fresh with ReviveOutcomeFreshExpected and no ReasonColdRestoreLostHistory
// signal — a legitimate first-time-equivalent fresh start is not lost history.
func TestColdRestore_NoSignal_When_GenuinelyNeverHadHistory(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test that starts real tmux sessions")
	}
	checkTmuxAvailable(t)

	title := fmt.Sprintf("test-cold-no-history-%d", time.Now().UnixNano())
	inst, cleanup, err := NewInstanceWithCleanup(InstanceOptions{
		Title:            title,
		Path:             t.TempDir(),
		Program:          "sleep 300",
		SessionType:      SessionTypeDirectory,
		AutoYes:          false,
		TmuxPrefix:       fmt.Sprintf("test_coldrestore_%d_", time.Now().UnixNano()),
		TmuxServerSocket: coldRestoreSocket(t),
	})
	require.NoError(t, err)
	defer func() {
		if cleanupErr := cleanup(); cleanupErr != nil {
			t.Logf("cleanup warning: %v", cleanupErr)
		}
	}()

	// inst.EverHadConversationHistory left at its zero value (false).
	recorder := &reviveOutcomeRecorder{}
	inst.RegisterLifecycleListener(recorder)

	assert.False(t, inst.TmuxAlive(), "tmux session must be dead before cold start")

	require.NoError(t, inst.Start(false), "cold start with no prior history should not error")

	assert.Equal(t, ReviveOutcomeFreshExpected, inst.LastReviveOutcome)
	require.Len(t, recorder.reasons, 1, "expected exactly one EventStarted")
	assert.Equal(t, "", recorder.reasons[0], "a genuinely fresh start must not carry the lost-history reason")
}

// TestColdRestore_LastReviveOutcomeClears_When_LaterCycleIsHotRestore is a
// regression test for a bug caught in code review: LastReviveOutcome was
// originally written only inside the ColdRestore branch, so a
// FreshLostHistory value from one cycle survived unchanged into a later
// HotRestore cycle (tmux still alive, nothing lost) — re-firing the
// lost-history notification and leaving RevivedContextBadge stuck
// indefinitely, contradicting ux.md's "reflects only the most recent
// restart's outcome" requirement. LastReviveOutcome must now be set in every
// branch (ColdRestore, HotRestore, firstTimeSetup) of every start cycle.
func TestColdRestore_LastReviveOutcomeClears_When_LaterCycleIsHotRestore(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test that starts real tmux sessions")
	}
	checkTmuxAvailable(t)

	title := fmt.Sprintf("test-cold-then-hot-%d", time.Now().UnixNano())
	inst, cleanup, err := NewInstanceWithCleanup(InstanceOptions{
		Title:            title,
		Path:             t.TempDir(),
		Program:          "sleep 300",
		SessionType:      SessionTypeDirectory,
		AutoYes:          false,
		TmuxPrefix:       fmt.Sprintf("test_coldrestore_%d_", time.Now().UnixNano()),
		TmuxServerSocket: coldRestoreSocket(t),
	})
	require.NoError(t, err)
	defer func() {
		if cleanupErr := cleanup(); cleanupErr != nil {
			t.Logf("cleanup warning: %v", cleanupErr)
		}
	}()

	// First cycle: cold restore with lost history — sets LastReviveOutcome to
	// FreshLostHistory and, as a side effect of pm().Start(), leaves tmux alive.
	inst.EverHadConversationHistory = true
	require.NoError(t, inst.Start(false), "cold start with lost history should not error")
	require.Equal(t, ReviveOutcomeFreshLostHistory, inst.LastReviveOutcome, "sanity: first cycle must record lost history")
	require.True(t, inst.TmuxAlive(), "sanity: tmux must be alive after the first cycle's pm().Start()")

	recorder := &reviveOutcomeRecorder{}
	inst.RegisterLifecycleListener(recorder)

	// Second cycle: tmux is still alive, so this takes the HotRestore branch —
	// nothing was lost this time, and the stale FreshLostHistory value must not survive.
	require.NoError(t, inst.Start(false), "hot restore should not error")

	assert.Equal(t, ReviveOutcomeResumeLive, inst.LastReviveOutcome, "hot restore must overwrite the stale FreshLostHistory value")
	require.Len(t, recorder.reasons, 1, "expected exactly one EventStarted for the second cycle")
	assert.Equal(t, "", recorder.reasons[0], "hot restore must not re-fire the lost-history reason")
}
