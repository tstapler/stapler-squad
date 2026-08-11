package session

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSwitchProgram_SameValue_NoOp verifies that requesting the program the instance
// already has is a no-op: no persist callback, no restart, changed=false.
func TestSwitchProgram_SameValue_NoOp(t *testing.T) {
	inst := minimalInstance(t)
	inst.Program = "claude"

	persistCalled := false
	changed, resolved, err := inst.SwitchProgram(context.Background(), "claude", func() error {
		persistCalled = true
		return nil
	})

	require.NoError(t, err)
	assert.False(t, changed, "same-value switch should be a no-op")
	assert.Equal(t, "claude", resolved)
	assert.False(t, persistCalled, "persist should not run on a no-op switch")
}

// TestSwitchProgram_EmptyString_ResolvesToConfigDefault verifies that an empty program
// string ("System default") resolves to config.LoadConfig().DefaultProgram rather than
// being stored as "" or silently dropped.
func TestSwitchProgram_EmptyString_ResolvesToConfigDefault(t *testing.T) {
	inst := minimalInstance(t)
	inst.Program = "aider"

	changed, resolved, err := inst.SwitchProgram(context.Background(), "", nil)

	require.NoError(t, err)
	assert.True(t, changed)
	assert.NotEmpty(t, resolved, "empty program must resolve to a non-empty default")
	assert.Equal(t, resolved, inst.Program)
}

// TestSwitchProgram_EmptyString_SameAsCurrentDefault_NoOp verifies that resolving an
// empty string to a default that already matches the current program is correctly
// treated as a no-op (regression guard for the capacity-monitor path, which previously
// compared the raw "" against inst.Program instead of the resolved default first).
func TestSwitchProgram_EmptyString_SameAsCurrentDefault_NoOp(t *testing.T) {
	inst := minimalInstance(t)
	_, defaultProgram, err := inst.SwitchProgram(context.Background(), "", nil)
	require.NoError(t, err)

	persistCalled := false
	changed, resolved, err := inst.SwitchProgram(context.Background(), "", func() error {
		persistCalled = true
		return nil
	})

	require.NoError(t, err)
	assert.False(t, changed, "resolving to the already-current default must be a no-op")
	assert.Equal(t, defaultProgram, resolved)
	assert.False(t, persistCalled)
}

// TestSwitchProgram_Stopped_PersistsNoRestart verifies that a non-Active instance
// persists the new program via the persist callback and never attempts a restart.
func TestSwitchProgram_Stopped_PersistsNoRestart(t *testing.T) {
	inst := minimalInstance(t)
	inst.Program = "claude"
	inst.Status = Stopped

	persistCalled := false
	changed, resolved, err := inst.SwitchProgram(context.Background(), "aider", func() error {
		persistCalled = true
		return nil
	})

	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, "aider", resolved)
	assert.Equal(t, "aider", inst.Program)
	assert.True(t, persistCalled, "persist callback must run even when not Active")
}

// TestSwitchProgram_RestartError_PersistAlreadyRan verifies the durability property the
// shipped fix depends on: the persist callback runs (and the new Program value is set)
// before an Active-session restart is attempted, so a restart failure never rolls back
// the already-applied program change. An empty Path deterministically fails Restart with
// "no working directory configured" without touching a real tmux backend.
func TestSwitchProgram_RestartError_PersistAlreadyRan(t *testing.T) {
	inst := minimalInstance(t)
	inst.Program = "claude"
	inst.Status = Active
	inst.started.Store(true)
	inst.Path = "" // forces Restart() to fail deterministically, no real tmux involved

	persistedProgram := ""
	changed, resolved, err := inst.SwitchProgram(context.Background(), "aider", func() error {
		persistedProgram = inst.Program
		return nil
	})

	require.Error(t, err, "Restart should fail with no working directory configured")
	assert.True(t, changed)
	assert.Equal(t, "aider", resolved)
	assert.Equal(t, "aider", inst.Program, "Program field must reflect the switch even though restart failed")
	assert.Equal(t, "aider", persistedProgram, "persist callback must observe the new program, i.e. run before Restart")
}

// TestSwitchProgram_ActiveStartedFalse_RestartErrorsCannotRestart verifies the
// !i.started guard on Restart surfaces as a SwitchProgram error for an Active instance
// that was loaded without being started (fallback load path).
func TestSwitchProgram_ActiveStartedFalse_RestartErrorsCannotRestart(t *testing.T) {
	inst := minimalInstance(t)
	inst.Program = "claude"
	inst.Status = Active
	inst.Path = "/tmp/somewhere" // valid path; started=false is what should trip the error
	// inst.started left false (zero value)

	changed, _, err := inst.SwitchProgram(context.Background(), "aider", nil)

	require.ErrorIs(t, err, ErrCannotRestart)
	assert.True(t, changed, "program field change and persist still happen even though restart is rejected")
	assert.Equal(t, "aider", inst.Program)
}

// TestSwitchProgram_LeavingClaudeFamily_ClearsConversationState verifies that switching
// away from the Claude/Antigravity family entirely clears the stored conversation UUID
// and history file path, so a later switch back doesn't attempt --resume with a stale
// UUID captured under the old program.
func TestSwitchProgram_LeavingClaudeFamily_ClearsConversationState(t *testing.T) {
	inst := minimalInstance(t)
	inst.Program = "claude"
	inst.SetClaudeSession(&ClaudeSessionData{ConversationUUID: "abc-123"})
	inst.HistoryFilePath = "/fake/history.jsonl"

	changed, _, err := inst.SwitchProgram(context.Background(), "aider", nil)

	require.NoError(t, err)
	assert.True(t, changed)
	assert.Empty(t, inst.GetClaudeConversationUUID(), "UUID must be cleared when leaving the claude/antigravity family")
	assert.Empty(t, inst.HistoryFilePath, "history file path must be cleared alongside the UUID")
}

// TestSwitchProgram_RoundTrip_NoStaleResume verifies claude -> aider -> claude does not
// resurrect the UUID captured under the first claude session: leaving the family clears
// it, and returning to claude from a non-family program never restores it.
func TestSwitchProgram_RoundTrip_NoStaleResume(t *testing.T) {
	inst := minimalInstance(t)
	inst.Program = "claude"
	inst.SetClaudeSession(&ClaudeSessionData{ConversationUUID: "stale-uuid"})

	_, _, err := inst.SwitchProgram(context.Background(), "aider", nil)
	require.NoError(t, err)
	require.Empty(t, inst.GetClaudeConversationUUID())

	_, _, err = inst.SwitchProgram(context.Background(), "claude", nil)
	require.NoError(t, err)
	assert.Empty(t, inst.GetClaudeConversationUUID(), "switching back to claude must not resurrect the stale UUID")
}

// TestSwitchProgram_WithinClaudeAntigravityFamily_PreservesUUID verifies that a switch
// that stays within the claude/antigravity family (recognized as history-portable) does
// NOT take the "leaving the family" clear-state branch. This only asserts the decision
// (no clear), not PortSessionHistory's own Import/Export behavior, which performs real
// file I/O against the adapters' backing stores and is out of scope here.
func TestSwitchProgram_WithinClaudeAntigravityFamily_PreservesUUID(t *testing.T) {
	assert.True(t, isClaudeAntigravityCrossSwitch("claude", "antigravity"))
	assert.True(t, isClaudeAntigravityCrossSwitch("antigravity", "claude"))
	assert.True(t, isClaudeAntigravityCrossSwitch("claude", "agy"))
	assert.False(t, isClaudeAntigravityCrossSwitch("claude", "aider"))
	assert.False(t, isClaudeAntigravityCrossSwitch("aider", "opencode"))

	assert.True(t, isClaudeAntigravityFamily("claude"))
	assert.True(t, isClaudeAntigravityFamily("antigravity"))
	assert.True(t, isClaudeAntigravityFamily("agy"))
	assert.False(t, isClaudeAntigravityFamily("aider"))
}

// TestGeminiFamilyGate_ConsistentWithAdapterResolution verifies the fix for the reported
// claude<->gemini asymmetry: gemini (the standalone Gemini CLI) is excluded from both the
// family gate here AND from AgyAdapter.CanHandle (session/agy_adapter.go), since Antigravity's
// adapter only reads/writes its own ~/.gemini/antigravity-cli/... storage, not the real
// Gemini CLI's format. If either side of this check is ever widened to include gemini without
// the other, this test catches the drift.
func TestGeminiFamilyGate_ConsistentWithAdapterResolution(t *testing.T) {
	assert.False(t, isClaudeAntigravityFamily("gemini"))
	assert.False(t, isClaudeAntigravityCrossSwitch("claude", "gemini"))
	assert.False(t, isClaudeAntigravityCrossSwitch("gemini", "claude"))
	assert.False(t, NewAgyAdapter().CanHandle("gemini"))
}

// TestSwitchProgram_ClaudeToGemini_CleanlyClearsConversationState verifies AC0: a
// claude->gemini switch never silently falls into an unhandled state. Since gemini isn't
// history-portable via AgyAdapter, it takes the leaving-the-family ClearConversationState()
// branch — same as any other non-family program — rather than being dropped on the floor.
func TestSwitchProgram_ClaudeToGemini_CleanlyClearsConversationState(t *testing.T) {
	inst := minimalInstance(t)
	inst.Program = "claude"
	inst.SetClaudeSession(&ClaudeSessionData{ConversationUUID: "abc-123"})

	changed, resolved, err := inst.SwitchProgram(context.Background(), "gemini", nil)

	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, "gemini", resolved)
	assert.Empty(t, inst.GetClaudeConversationUUID(), "switching to gemini must clear the stale claude UUID, not silently keep it")
}

// TestPortHistoryFailureIsExpected directly covers the Warn-vs-Error log-level selection used
// by SwitchProgram, independent of whether isClaudeAntigravityCrossSwitch/CanHandle parity
// currently makes the ErrNoHistoryAdapter branch reachable in production — this keeps the
// selection logic itself verified as defense-in-depth against future drift.
func TestPortHistoryFailureIsExpected(t *testing.T) {
	assert.True(t, portHistoryFailureIsExpected(ErrNoHistoryAdapter))
	assert.True(t, portHistoryFailureIsExpected(fmt.Errorf("wrapped: %w", ErrNoHistoryAdapter)))
	assert.False(t, portHistoryFailureIsExpected(errors.New("disk full")))
}

// TestSwitchProgram_ConcurrentCalls_Serialize is a race-detector regression guard (run
// under `go test -race`, part of `make ci`'s test-race target) for AC3: two goroutines
// calling SwitchProgram on the same instance concurrently — mimicking a manual
// UpdateSession request racing an automatic capacity-monitor fallback — must serialize
// through programSwitchMu rather than both reading the pre-change Program and both
// deciding independently, which is what would let them double-restart/double-port.
func TestSwitchProgram_ConcurrentCalls_Serialize(t *testing.T) {
	inst := minimalInstance(t)
	inst.Program = "claude"

	var wg sync.WaitGroup
	var mu sync.Mutex
	var changedCount int

	targets := []string{"aider", "opencode"}
	for _, target := range targets {
		wg.Add(1)
		go func(program string) {
			defer wg.Done()
			changed, _, err := inst.SwitchProgram(context.Background(), program, nil)
			if err != nil {
				t.Errorf("SwitchProgram(%q) returned unexpected error: %v", program, err)
			}
			if changed {
				mu.Lock()
				changedCount++
				mu.Unlock()
			}
		}(target)
	}
	wg.Wait()

	// Whichever goroutine ran first changes the program; the second observes the
	// already-updated value. Both are only "unchanged" if they raced to the exact
	// same target, which isn't the case here, so exactly one or two changes are
	// possible depending on scheduling — the invariant under test is the absence of
	// a data race (enforced by `-race`), not a specific count.
	assert.Contains(t, targets, inst.Program)
}
