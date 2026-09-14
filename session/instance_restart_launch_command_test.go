package session

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

// TestKillSessionThenStart_RebuildsLaunchCommand traces whether
// Instance.KillSession() + Instance.Start(false) — the exact sequence
// session/health.go's dead-pane recovery loop uses — rebuilds the tmux launch
// command from the current i.claudeSession.ConversationUUID, or reuses
// whatever program string was baked in at the last SetSession() call.
//
// It must rebuild. Before the fix, initTmuxSession()'s
// `if i.pm().HasSession() { ...; return }` early-return skipped
// buildLaunchCommand() whenever an in-process TmuxSession pointer already
// existed, regardless of whether the OS-level tmux session it pointed to was
// still alive — the root cause of the 2026-09-12 mass tmux-kill-server
// incident (every session's recovery hit this early-return and silently
// relaunched without --resume). See initTmuxSession()'s doc comment for the
// fix (HasSession() && IsAlive()).
func TestKillSessionThenStart_RebuildsLaunchCommand(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration test that starts real tmux sessions")
	}
	checkTmuxAvailable(t)

	title := fmt.Sprintf("test-spike-%d", time.Now().UnixNano())
	inst, cleanup, err := NewInstanceWithCleanup(InstanceOptions{
		Title:            title,
		Path:             t.TempDir(),
		Program:          stubClaudeBinary(t),
		SessionType:      SessionTypeDirectory,
		AutoYes:          false,
		TmuxPrefix:       fmt.Sprintf("test_spike_%d_", time.Now().UnixNano()),
		TmuxServerSocket: coldRestoreSocket(t),
	})
	require.NoError(t, err)
	defer func() {
		if cleanupErr := cleanup(); cleanupErr != nil {
			t.Logf("cleanup warning: %v", cleanupErr)
		}
	}()

	// First start with no UUID set: original launch command has no --resume, and
	// this construction is what registers the TmuxSession object via SetSession().
	startCleanup, err := inst.StartWithCleanup(true)
	require.NoError(t, err, "first start should succeed")
	defer func() {
		if startCleanup != nil {
			if cleanupErr := startCleanup(); cleanupErr != nil {
				t.Logf("startCleanup warning: %v", cleanupErr)
			}
		}
	}()
	wait.RequireEventually(t, inst.TmuxAlive, 10*time.Second, 50*time.Millisecond, "tmux session must be alive after first start")
	require.NotContains(t, inst.LaunchCommand, "--resume", "sanity: first launch had no UUID set yet")

	// Simulate the UUID being captured while the session was running (exactly what
	// tryExtractConversationUUID does mid-session), then the pane dying.
	inst.SetClaudeSession(&ClaudeSessionData{
		ConversationUUID: "550e8400-e29b-41d4-a716-446655440000",
		LastAttached:     time.Now(),
	})

	require.NoError(t, inst.KillSession(), "KillSession should succeed")

	// health.go's dead-pane recovery path: KillSession() then Start(false).
	require.NoError(t, inst.Start(false), "Start(false) after KillSession should succeed")
	wait.RequireEventually(t, inst.TmuxAlive, 10*time.Second, 50*time.Millisecond, "tmux session must be alive after KillSession()+Start(false)")

	t.Logf("LaunchCommand after KillSession()+Start(false): %q", inst.LaunchCommand)
	assert.Contains(t, inst.LaunchCommand, "--resume",
		"KillSession()+Start(false) must rebuild the launch command from the newly-set "+
			"ConversationUUID — regression check for the 2026-09-12 mass tmux-kill-server "+
			"incident (see this test's doc comment)")
	assert.Contains(t, inst.LaunchCommand, "550e8400-e29b-41d4-a716-446655440000")

	// Contrast: Restart() also explicitly rebuilds the launch command every call.
	// Re-set the UUID first: the cold-restore branch just exercised above clears
	// i.claudeSession.ConversationUUID by design (see startLocked's comment on
	// "Clear the stored session ID so HistoryLinker re-detects...") before
	// re-deriving it via tryExtractConversationUUID, which found nothing in this
	// fake, JSONL-less test environment and left it empty.
	inst.SetClaudeSession(&ClaudeSessionData{
		ConversationUUID: "550e8400-e29b-41d4-a716-446655440000",
		LastAttached:     time.Now(),
	})
	require.NoError(t, inst.Restart(false), "Restart should succeed")
	wait.RequireEventually(t, inst.TmuxAlive, 10*time.Second, 50*time.Millisecond, "tmux session must be alive after Restart")
	assert.Contains(t, inst.LaunchCommand, "--resume", "Restart() is expected to rebuild the launch command with --resume")
	assert.Contains(t, inst.LaunchCommand, "550e8400-e29b-41d4-a716-446655440000")
}
