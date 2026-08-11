package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestInstance_SetHistoryInfo_FiresCallback_When_UUIDChanges guards the fix
// making SetHistoryInfo (the HistoryLinker's setter) fire the same
// claudeSessionIDSavedCallback SetClaudeConversationUUID uses. Without it, a
// HistoryLinker-detected conversation UUID only reached durable storage on
// the next incidental full SaveInstances sweep — a tmux pane killed before
// that sweep ran would resume with no conversation UUID to pass to --resume.
func TestInstance_SetHistoryInfo_FiresCallback_When_UUIDChanges(t *testing.T) {
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
	inst := makeTestInstance("history-info-path-only")
	fired := 0
	inst.SetClaudeSessionIDSavedCallback(func() { fired++ })

	inst.SetHistoryInfo("conv-uuid-1", "/path/to/history-a.jsonl")
	inst.SetHistoryInfo("conv-uuid-1", "/path/to/history-b.jsonl")

	assert.Equal(t, 1, fired, "expected callback to fire only for the UUID-changing call")
	assert.Equal(t, "/path/to/history-b.jsonl", inst.HistoryFilePath)
}
