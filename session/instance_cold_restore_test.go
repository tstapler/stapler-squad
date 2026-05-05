package session

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// checkTmuxAvailable skips the test if tmux is not installed.
func checkTmuxAvailable(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available on this system")
	}
}

// coldRestoreSocket returns a unique tmux server socket name for cold-restore
// tests and registers a t.Cleanup that kills the isolated server on test exit.
// The PID is embedded in the name so TestMain's PID-aware sweep can reap this
// socket on the next run if the current run was killed with SIGKILL.
func coldRestoreSocket(t *testing.T) string {
	t.Helper()
	name := fmt.Sprintf("test_coldrestore_%d_%d", os.Getpid(), time.Now().UnixNano())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = safeexec.CommandContext(ctx, "tmux", "-L", name, "kill-server").Run()
	})
	return name
}

// TestColdRestore_WithUUID verifies that when the tmux session is dead and a
// Claude conversation UUID is present, Start(false) performs a cold restore by
// launching a new tmux session. Note: --resume flag injection is verified at the
// unit level in claude_command_builder_test.go; this test verifies the lifecycle
// (dead tmux → HasClaudeSession=true → Running).
func TestColdRestore_WithUUID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test that starts real tmux sessions")
	}
	checkTmuxAvailable(t)

	title := fmt.Sprintf("test-cold-%d", time.Now().UnixNano())

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

	// Attach a valid Claude session UUID — no live tmux session exists yet.
	// Uses a valid UUID-v4 so HasClaudeSession() returns true and the cold-restore
	// branch logs "Cold restoring with --resume". The --resume flag itself is only
	// appended by ClaudeCommandBuilder for Program="claude"; that is unit-tested
	// separately in claude_command_builder_test.go.
	inst.SetClaudeSession(&ClaudeSessionData{
		ConversationUUID: "550e8400-e29b-41d4-a716-446655440000",
		LastAttached:     time.Now(),
	})

	// tmux session does NOT exist at this point (simulates post-reboot state).
	assert.False(t, inst.TmuxAlive(), "tmux session must be dead before cold restore")

	// Cold restore: Start(false) must not error.
	startCleanup, err := inst.StartWithCleanup(false)
	require.NoError(t, err, "cold restore with UUID should not error")
	defer func() {
		if startCleanup != nil {
			if cleanupErr := startCleanup(); cleanupErr != nil {
				t.Logf("startCleanup warning: %v", cleanupErr)
			}
		}
	}()

	assert.True(t, inst.Started(), "instance must be marked as started after cold restore")
	assert.Equal(t, Running, inst.Status, "instance status must be Running after cold restore")
	// DoesSessionExist slow path has a 3s timeout; deadline must exceed that to guarantee
	// at least one successful check before the Eventually deadline fires.
	require.Eventually(t, inst.TmuxAlive, 10*time.Second, 50*time.Millisecond, "tmux session must be alive after cold restore")
}

// TestColdRestore_WithoutUUID verifies that when the tmux session is dead and
// there is no Claude conversation UUID, Start(false) still creates a fresh tmux
// session and the instance transitions to Running.
func TestColdRestore_WithoutUUID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test that starts real tmux sessions")
	}
	checkTmuxAvailable(t)

	title := fmt.Sprintf("test-cold-%d", time.Now().UnixNano())

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

	// No claudeSession set — instance.claudeSession remains nil.
	assert.False(t, inst.TmuxAlive(), "tmux session must be dead before cold start")

	startCleanup, err := inst.StartWithCleanup(false)
	require.NoError(t, err, "cold start without UUID should not error")
	defer func() {
		if startCleanup != nil {
			if cleanupErr := startCleanup(); cleanupErr != nil {
				t.Logf("startCleanup warning: %v", cleanupErr)
			}
		}
	}()

	assert.True(t, inst.Started(), "instance must be marked as started after cold start")
	assert.Equal(t, Running, inst.Status, "instance status must be Running after cold start")
	require.Eventually(t, inst.TmuxAlive, 10*time.Second, 50*time.Millisecond, "tmux session must be alive after cold start")
}

// TestHotRestore_ExistingSession verifies that when the tmux session is already
// alive, Start(false) attaches to it (hot restore) rather than creating a new one.
func TestHotRestore_ExistingSession(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test that starts real tmux sessions")
	}
	checkTmuxAvailable(t)

	title := fmt.Sprintf("test-hot-%d", time.Now().UnixNano())
	tmpDir := t.TempDir()
	socket := coldRestoreSocket(t)
	prefix := fmt.Sprintf("test_coldrestore_%d_", time.Now().UnixNano())

	// First instance: create and start normally to put a live tmux session in place.
	inst1, cleanup1, err := NewInstanceWithCleanup(InstanceOptions{
		Title:            title,
		Path:             tmpDir,
		Program:          "sleep 300",
		SessionType:      SessionTypeDirectory,
		AutoYes:          false,
		TmuxPrefix:       prefix,
		TmuxServerSocket: socket,
	})
	require.NoError(t, err)
	defer func() {
		if cleanupErr := cleanup1(); cleanupErr != nil {
			t.Logf("cleanup1 warning: %v", cleanupErr)
		}
	}()

	startCleanup1, err := inst1.StartWithCleanup(true)
	require.NoError(t, err, "first start should succeed")
	defer func() {
		if startCleanup1 != nil {
			if cleanupErr := startCleanup1(); cleanupErr != nil {
				t.Logf("startCleanup1 warning: %v", cleanupErr)
			}
		}
	}()

	require.Eventually(t, inst1.TmuxAlive, 10*time.Second, 50*time.Millisecond, "inst1 tmux session must be alive before hot restore")

	// Second instance: same title/socket — simulates an instance reloaded from storage
	// while the original tmux session is still alive.
	inst2, cleanup2, err := NewInstanceWithCleanup(InstanceOptions{
		Title:            title,
		Path:             tmpDir,
		Program:          "sleep 300",
		SessionType:      SessionTypeDirectory,
		AutoYes:          false,
		TmuxPrefix:       prefix,
		TmuxServerSocket: socket,
	})
	require.NoError(t, err)
	defer func() {
		if cleanupErr := cleanup2(); cleanupErr != nil {
			t.Logf("cleanup2 warning: %v", cleanupErr)
		}
	}()

	// Hot restore: tmux session exists, so Start(false) must reuse it.
	startCleanup2, err := inst2.StartWithCleanup(false)
	require.NoError(t, err, "hot restore should not error")
	defer func() {
		if startCleanup2 != nil {
			if cleanupErr := startCleanup2(); cleanupErr != nil {
				t.Logf("startCleanup2 warning: %v", cleanupErr)
			}
		}
	}()

	assert.True(t, inst2.Started(), "inst2 must be marked as started after hot restore")
	assert.Equal(t, Running, inst2.Status, "inst2 status must be Running after hot restore")
}
