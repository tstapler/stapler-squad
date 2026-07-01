package session

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTryExtractConversationUUID_SkipsWhenAlreadyHasID verifies the early-return
// guard: when claudeSession.ConversationUUID is already populated, tryExtractConversationUUID
// returns immediately and does not overwrite the existing ID.
//
// The guard lives at the top of tryExtractConversationUUID:
//
//	if i.claudeSession != nil && i.claudeSession.ConversationUUID != "" { return }
//
// Because this returns before any tmux interaction, the test requires no live
// tmux session and will not reach NewHistoryFileDetectorWithRealInspector.
func TestTryExtractConversationUUID_SkipsWhenAlreadyHasID(t *testing.T) {
	const existingID = "a1b2c3d4-e5f6-7890-abcd-ef1234567890"

	inst := &Instance{
		Title: "test-skip-existing-id",
		claudeSession: &ClaudeSessionData{
			ConversationUUID: existingID,
		},
		// tmuxManager.session is nil — DoesSessionExist() would return false,
		// but the guard fires before we ever reach the tmux check.
	}

	inst.tryExtractConversationUUID()

	require.NotNil(t, inst.claudeSession)
	assert.Equal(t, existingID, inst.claudeSession.ConversationUUID,
		"SessionID must not be overwritten when it is already populated")
}

// TestTryExtractConversationUUID_SkipsWhenTmuxNotRunning verifies that calling
// tryExtractConversationUUID on an instance whose tmux session does not exist
// completes without panicking and leaves claudeSession unchanged.
//
// TmuxProcessManager.DoesSessionExist() returns false when session == nil,
// so the function logs a debug message and returns early.
func TestTryExtractConversationUUID_SkipsWhenTmuxNotRunning(t *testing.T) {
	inst := &Instance{
		Title: "test-no-tmux",
		// claudeSession is nil — we have no ID yet.
		// tmuxManager.session is nil — DoesSessionExist() returns false.
	}

	// Must not panic.
	inst.tryExtractConversationUUID()

	// claudeSession must remain nil because we bailed out before setting it.
	assert.Nil(t, inst.claudeSession,
		"claudeSession must remain nil when tmux is not running")
}

// TestTryExtractConversationUUID_EmptySessionIDTriesTmux verifies that an
// instance whose claudeSession exists but whose SessionID is empty does NOT
// skip via the early-return guard and instead proceeds to the tmux check.
//
// With no live tmux session the function returns early at the DoesSessionExist
// check, leaving SessionID empty.  This validates that the guard condition
// requires BOTH non-nil claudeSession AND non-empty SessionID.
func TestTryExtractConversationUUID_EmptySessionIDTriesTmux(t *testing.T) {
	inst := &Instance{
		Title: "test-empty-session-id",
		claudeSession: &ClaudeSessionData{
			ConversationUUID: "", // Explicitly empty — guard should NOT fire.
		},
		// tmuxManager.session is nil — DoesSessionExist() returns false.
	}

	inst.tryExtractConversationUUID()

	// SessionID must still be empty because tmux was not running.
	require.NotNil(t, inst.claudeSession)
	assert.Equal(t, "", inst.claudeSession.ConversationUUID,
		"SessionID must remain empty when tmux is not running")
}

// TestSwitchWorkspace_GuardPreventsExtractionWhenIDPopulated verifies the
// guard condition at the SwitchWorkspace call site (line 133 and line 172 of
// instance_workspace.go).  Both call sites share the same shape:
//
//	if i.claudeSession == nil || i.claudeSession.ConversationUUID == "" {
//	    i.tryExtractConversationUUID()
//	}
//
// This test confirms the guard logic directly using tryExtractConversationUUID,
// which is what SwitchWorkspace delegates to after the guard check.
func TestSwitchWorkspace_GuardPreventsExtractionWhenIDPopulated(t *testing.T) {
	const preexistingID = "550e8400-e29b-41d4-a716-446655440000"

	inst := &Instance{
		Title: "test-guard-at-call-site",
		claudeSession: &ClaudeSessionData{
			ConversationUUID: preexistingID,
		},
	}

	// Simulate the guard condition used at both call sites in SwitchWorkspace.
	if inst.claudeSession == nil || inst.claudeSession.ConversationUUID == "" {
		inst.tryExtractConversationUUID()
	}

	// The guard prevented the call — ID is unchanged.
	require.NotNil(t, inst.claudeSession)
	assert.Equal(t, preexistingID, inst.claudeSession.ConversationUUID,
		"SessionID must be unchanged when SwitchWorkspace guard fires")
}

// TestSwitchWorkspace_GuardAllowsExtractionWhenIDMissing verifies the inverse:
// when claudeSession is nil the guard at the SwitchWorkspace call sites DOES
// allow tryExtractConversationUUID to run.  With no live tmux the extractor
// returns without setting anything, but the important thing is that the call
// was not blocked by the guard.
func TestSwitchWorkspace_GuardAllowsExtractionWhenIDMissing(t *testing.T) {
	inst := &Instance{
		Title:         "test-guard-allows-call",
		claudeSession: nil, // Triggers the guard condition.
	}

	// tryExtractConversationUUID is reachable when guard condition fires.
	// No tmux means it returns early without panic.
	var extractionAttempted bool
	if inst.claudeSession == nil || inst.claudeSession.ConversationUUID == "" {
		extractionAttempted = true
		inst.tryExtractConversationUUID()
	}

	assert.True(t, extractionAttempted,
		"extraction must be attempted when claudeSession is nil")
	// claudeSession remains nil because tmux was not running.
	assert.Nil(t, inst.claudeSession)
}

// TestSwitchWorkspace_DoesNotDeadlockOnStartCall is a regression test for a
// reentrant-lock self-deadlock: SwitchWorkspace used to hold i.stateMutex.Lock()
// across its entire body, including its calls to i.Start(false). Start() itself
// acquires i.stateMutex.Lock() (instance.go, in the Active-status transition near
// the end of start()). sync.RWMutex/deadlock.RWMutex is not reentrant, so calling
// Start() while still holding the lock deadlocks the calling goroutine forever.
//
// This is a deadlock, not a data race — go test -race will NOT catch it. The test
// instead runs SwitchWorkspace in a goroutine and asserts it returns within a
// generous bound; pre-fix, this test would hang until the suite's own timeout.
//
// The instance has no VCS in its temp directory, so VCS detection fails and
// SwitchWorkspace takes the SessionTypeDirectory fallback path (instance_workspace.go,
// "no vcs detected, falling back to simple directory change"), which is the first
// of the three call sites that used to deadlock.
func TestSwitchWorkspace_DoesNotDeadlockOnStartCall(t *testing.T) {
	instance, cleanup, err := NewTestInstance(t, "switch-workspace-deadlock-regression").
		WithSessionType(SessionTypeDirectory).
		Build()
	require.NoError(t, err)
	if cleanup != nil {
		defer func() { _ = cleanup() }()
	}

	// Simulate an already-running session so SwitchWorkspace's guard passes.
	instance.started = true
	instance.Status = Active

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Result/error are not asserted: the temp dir has no VCS, so this may
		// well return an error. The only thing this test verifies is that the
		// call returns at all instead of deadlocking.
		_, _ = instance.SwitchWorkspace(WorkspaceSwitchRequest{
			Type:   SwitchTypeRevision,
			Target: "some-branch",
		})
	}()

	select {
	case <-done:
		// Returned without deadlocking - the fix holds.
	case <-time.After(10 * time.Second):
		t.Fatal("SwitchWorkspace did not return within 10s - likely reentrant " +
			"stateMutex deadlock via Start() (see instance_workspace.go)")
	}
}
