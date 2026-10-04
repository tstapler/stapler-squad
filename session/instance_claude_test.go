package session

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestInstance_SetHistoryInfo_FiresCallback_When_UUIDChanges guards the fix
// making SetHistoryInfo (the HistoryLinker's setter) fire the same
// claudeSessionIDSavedCallback SetClaudeConversationUUID uses. Without it, a
// HistoryLinker-detected conversation UUID only reached durable storage on
// the next incidental full SaveInstances sweep — a tmux pane killed before
// that sweep ran would resume with no conversation UUID to pass to --resume.
func TestInstance_SetHistoryInfo_FiresCallback_When_UUIDChanges(t *testing.T) {
	t.Parallel()
	inst := makeTestInstance("history-info-callback")
	fired := 0
	inst.SetClaudeSessionIDSavedCallback(func() { fired++ })

	inst.SetHistoryInfo("conv-uuid-1", "/path/to/history.jsonl")

	assert.Equal(t, 1, fired, "expected callback to fire once when the UUID changes")
	assert.Equal(t, "conv-uuid-1", inst.claudeSession.ConversationUUID)
	assert.Equal(t, "/path/to/history.jsonl", inst.HistoryFilePath)
}

// TestInstance_SetHistoryInfo_SkipsCallback_When_UUIDUnchanged verifies the
// no-op path (same UUID and history path) does not re-fire the persistence
// callback — mirrors SetClaudeConversationUUID's existing no-op guard.
func TestInstance_SetHistoryInfo_SkipsCallback_When_UUIDUnchanged(t *testing.T) {
	t.Parallel()
	inst := makeTestInstance("history-info-noop")
	fired := 0
	inst.SetClaudeSessionIDSavedCallback(func() { fired++ })

	inst.SetHistoryInfo("conv-uuid-1", "/path/to/history.jsonl")
	inst.SetHistoryInfo("conv-uuid-1", "/path/to/history.jsonl")

	assert.Equal(t, 1, fired, "expected callback to fire only once across two identical calls")
}

// TestInstance_SetHistoryInfo_SkipsCallback_When_OnlyHistoryPathChanges
// verifies the callback is scoped to UUID changes specifically: a history
// file path update alone (UUID unchanged) must not trigger a redundant save.
func TestInstance_SetHistoryInfo_SkipsCallback_When_OnlyHistoryPathChanges(t *testing.T) {
	t.Parallel()
	inst := makeTestInstance("history-info-path-only")
	fired := 0
	inst.SetClaudeSessionIDSavedCallback(func() { fired++ })

	inst.SetHistoryInfo("conv-uuid-1", "/path/to/history-a.jsonl")
	inst.SetHistoryInfo("conv-uuid-1", "/path/to/history-b.jsonl")

	assert.Equal(t, 1, fired, "expected callback to fire only for the UUID-changing call")
	assert.Equal(t, "/path/to/history-b.jsonl", inst.HistoryFilePath)
}

// TestRecoverFromStaleResume_should_NotRestart_When_InstanceArchived pins
// guard 6 of ADR-001 (superseded-rework-session-retirement) — the seventh
// revival path, found in round 3 of adversarial review. The PTY-EOF callback
// fires recoverFromStaleResume whenever an exiting session's tail contains
// Claude's "No conversation found with session ID", and it calls
// RecoverFromStopped() + Start(false) directly, bypassing every other guard
// (it is not a fromInstanceData copy, not boot-time, not reconcileSessions,
// not healthCheckSkipReason, and not restartForRetry).
//
// It is reachable for archived sessions because KillTmuxPaneOnly closes the
// pane without stopping the controller — and a restart here is worse than the
// one being fixed elsewhere: it starts a brand-new, un-resumed conversation.
func TestRecoverFromStaleResume_should_NotRestart_When_InstanceArchived(t *testing.T) {
	t.Parallel()

	t.Run("archived_is_not_restarted", func(t *testing.T) {
		t.Parallel()
		archivedAt := time.Now()
		mock := &mockTmuxManager{}
		inst := &Instance{Title: "archived-stale-resume", Status: Stopped, ArchivedAt: &archivedAt}
		inst.processManager = NewTmuxBackend(mock)

		inst.recoverFromStaleResume()

		assert.Equal(t, 0, mock.startCalls, "an archived session must not be restarted on a stale --resume exit")
		assert.Equal(t, Stopped, inst.Snapshot().Status, "the archived session's status must be left alone")
	})

	t.Run("not_archived_is_restarted_control", func(t *testing.T) {
		t.Parallel()
		mock := &mockTmuxManager{}
		inst := &Instance{Title: "live-stale-resume", Status: Stopped}
		inst.processManager = NewTmuxBackend(mock)

		inst.recoverFromStaleResume()

		assert.Positive(t, mock.startCalls, "control: a live session must still recover from a stale --resume uuid")
	})
}
