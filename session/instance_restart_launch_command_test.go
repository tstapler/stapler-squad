package session

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestKillSessionThenStart_DoesNotRebuildLaunchCommand is the Epic 1.0
// verification spike from project_plans/cold-restart-uuid-recovery/implementation/plan.md
// (Task 1.0.1a). It traces whether Instance.KillSession() + Instance.Start(false) —
// the exact sequence session/health.go's dead-pane recovery loop uses — rebuilds the
// tmux launch command from the current i.claudeSession.ConversationUUID, or reuses
// whatever program string was baked in at the last SetSession() call.
//
// Result: it does NOT rebuild. TmuxProcessManager.Close() (called by KillSession())
// never nils the underlying atomic.Pointer[tmux.TmuxSession], so initTmuxSession()'s
// `if i.pm().HasSession() { ...; return }` early-return skips buildLaunchCommand()
// entirely on this path. Contrast with Instance.Restart(), which explicitly calls
// buildLaunchCommand() and SetSession() every time.
//
// Consequence for this plan: Epic 1.1's reorder (moving tryExtractConversationUUID()
// before initTmuxSession()) fixes the true process-boot cold-start case (a fresh
// Instance whose TmuxSession has never been constructed — exactly what
// TestColdRestore_WithoutUUID_RecoversFromJSONL exercises), but does NOT by itself
// close the in-process restart-churn case captured in requirements.md's timeline,
// where health.go's KillSession()+Start(false) reuses an already-constructed
// TmuxSession. That gap is tracked as a named follow-up in plan.md's Risk Control
// (item 5 references pre-mortem #4; the HasSession()-reuse gap itself is a distinct,
// separately-scoped follow-up — see ADR-001) rather than fixed in this plan.
func TestKillSessionThenStart_DoesNotRebuildLaunchCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test that starts real tmux sessions")
	}
	checkTmuxAvailable(t)

	title := fmt.Sprintf("test-spike-%d", time.Now().UnixNano())
	inst, cleanup, err := NewInstanceWithCleanup(InstanceOptions{
		Title:            title,
		Path:             t.TempDir(),
		Program:          "claude",
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
	require.Eventually(t, inst.TmuxAlive, 10*time.Second, 50*time.Millisecond, "tmux session must be alive after first start")
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
	require.Eventually(t, inst.TmuxAlive, 10*time.Second, 50*time.Millisecond, "tmux session must be alive after KillSession()+Start(false)")

	t.Logf("LaunchCommand after KillSession()+Start(false): %q", inst.LaunchCommand)
	assert.NotContains(t, inst.LaunchCommand, "--resume",
		"KillSession()+Start(false) is expected NOT to rebuild the launch command from "+
			"the newly-set ConversationUUID (see this test's doc comment) — if this now "+
			"fails, the premise behind Epic 1.0/ADR-001 has changed and plan.md's scope "+
			"note needs to be revisited")

	// Contrast: Restart() DOES explicitly rebuild the launch command every call.
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
	require.Eventually(t, inst.TmuxAlive, 10*time.Second, 50*time.Millisecond, "tmux session must be alive after Restart")
	assert.Contains(t, inst.LaunchCommand, "--resume", "Restart() is expected to rebuild the launch command with --resume")
	assert.Contains(t, inst.LaunchCommand, "550e8400-e29b-41d4-a716-446655440000")
}
