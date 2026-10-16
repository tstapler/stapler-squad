package session

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/testutil/wait"
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

// stubClaudeBinary writes an executable "claude" stub (a plain `sleep 300`
// script) to a temp dir and returns its path. Using this as Instance.Program
// (instead of the literal string "claude") keeps isClaude()'s basename match
// — so buildLaunchCommand embeds --resume — while guaranteeing the tmux pane
// can actually exec something that stays alive: the real claude CLI is not
// installed on CI runners, only on a developer machine that happens to have
// it in PATH, and a pane whose command exits almost instantly makes
// TmuxAlive() a race against remain-on-exit instead of a reliable check.
func stubClaudeBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "claude")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\nsleep 300\n"), 0o755))
	return path
}

// TestColdRestore_WithUUID verifies that when the tmux session is dead and a
// Claude conversation UUID is present, Start(false) performs a cold restore by
// launching a new tmux session. Note: --resume flag injection is verified at the
// unit level in claude_command_builder_test.go; this test verifies the lifecycle
// (dead tmux → HasClaudeSession=true → Running).
func TestColdRestore_WithUUID(t *testing.T) {
	t.Parallel()
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
	wait.RequireEventually(t, inst.TmuxAlive, 10*time.Second, 50*time.Millisecond, "tmux session must be alive after cold restore")
}

// TestColdRestore_WithoutUUID verifies that when the tmux session is dead and
// there is no Claude conversation UUID, Start(false) still creates a fresh tmux
// session and the instance transitions to Running.
func TestColdRestore_WithoutUUID(t *testing.T) {
	t.Parallel()
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
	wait.RequireEventually(t, inst.TmuxAlive, 10*time.Second, 50*time.Millisecond, "tmux session must be alive after cold start")
}

// writeJSONLFixture writes a fake conversation JSONL fixture under
// <homeDir>/.claude/projects/<encoded-projectPath>/<uuid>.jsonl and returns its
// path. If modTime is non-zero, the file's mtime is set explicitly via
// os.Chtimes (matching history_detector_test.go's convention for deterministic
// ordering); otherwise the file keeps its natural write-time mtime.
func writeJSONLFixture(t *testing.T, homeDir, projectPath, uuid string, modTime time.Time) string {
	t.Helper()
	dir := filepath.Join(homeDir, ".claude", "projects", ClaudeProjectDirName(projectPath))
	require.NoError(t, os.MkdirAll(dir, 0o755))
	path := filepath.Join(dir, uuid+".jsonl")
	require.NoError(t, os.WriteFile(path, []byte("{}"), 0o644))
	if !modTime.IsZero() {
		require.NoError(t, os.Chtimes(path, modTime, modTime))
	}
	return path
}

// TestColdRestore_WithoutUUID_RecoversFromJSONL verifies AC1/AC4 of
// project_plans/cold-restart-uuid-recovery: when the tmux session is dead, the
// in-memory conversation UUID is empty, but a same-path conversation JSONL exists
// on disk, Start(false) recovers that UUID via the DetectByPath fallback BEFORE
// building the launch command, so the revived session launches with --resume
// instead of silently starting fresh. This is the exact regression
// TestColdRestore_WithoutUUID left uncovered (see requirements.md's "Existing
// related work").
func TestColdRestore_WithoutUUID_RecoversFromJSONL(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration test that starts real tmux sessions")
	}
	checkTmuxAvailable(t)

	title := fmt.Sprintf("test-cold-%d", time.Now().UnixNano())
	fakeHome := t.TempDir()

	inst, cleanup, err := NewInstanceWithCleanup(InstanceOptions{
		Title:            title,
		Path:             t.TempDir(),
		Program:          stubClaudeBinary(t),
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

	// Inject a fake home dir and pre-write a conversation JSONL for this
	// instance's project path, as if a previous run had captured one. Uses a
	// no-open-files mock inspector (not nil) because Start's post-launch
	// tryExtractConversationUUID call re-runs once tmux is alive, hitting the
	// PID fast path — a nil inspector would panic there. Same convention as
	// history_linker_test.go's "no open files → always falls through to
	// DetectByPath" mocks.
	inst.historyDetector = NewHistoryFileDetectorWithHomeDir(&mockProcessInspector{files: []string{}}, fakeHome)
	const fixtureUUID = "550e8400-e29b-41d4-a716-446655440000"
	writeJSONLFixture(t, fakeHome, inst.Path, fixtureUUID, time.Time{})

	// No claudeSession set — instance.claudeSession remains nil, tmux dead.
	assert.False(t, inst.TmuxAlive(), "tmux session must be dead before cold restore")

	startCleanup, err := inst.StartWithCleanup(false)
	require.NoError(t, err, "cold restore with recoverable JSONL should not error")
	defer func() {
		if startCleanup != nil {
			if cleanupErr := startCleanup(); cleanupErr != nil {
				t.Logf("startCleanup warning: %v", cleanupErr)
			}
		}
	}()

	assert.True(t, inst.Started(), "instance must be marked as started after cold restore")
	assert.Equal(t, Running, inst.Status, "instance status must be Running after cold restore")
	wait.RequireEventually(t, inst.TmuxAlive, 10*time.Second, 50*time.Millisecond, "tmux session must be alive after cold restore")

	assert.Contains(t, inst.LaunchCommand, "--resume", "launch command must embed --resume when a same-path JSONL was recoverable")
	assert.Contains(t, inst.LaunchCommand, fixtureUUID)
	assert.Equal(t, fixtureUUID, inst.GetConversationUUID())
}

// TestHotRestore_ExistingSession verifies that when the tmux session is already
// alive, Start(false) attaches to it (hot restore) rather than creating a new one.
// sleepInstanceOptions builds the InstanceOptions for a long-lived `sleep
// 300` tmux session on an isolated prefix/socket — the fixture shared by
// every real-tmux restore test below (hot restore and the one-shot revive
// regression test), so the live-pane session shape is defined once.
func sleepInstanceOptions(title, tmpDir, prefix, socket string) InstanceOptions {
	return InstanceOptions{
		Title:            title,
		Path:             tmpDir,
		Program:          "sleep 300",
		SessionType:      SessionTypeDirectory,
		AutoYes:          false,
		TmuxPrefix:       prefix,
		TmuxServerSocket: socket,
	}
}

func TestHotRestore_ExistingSession(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration test that starts real tmux sessions")
	}
	checkTmuxAvailable(t)

	title := fmt.Sprintf("test-hot-%d", time.Now().UnixNano())
	tmpDir := t.TempDir()
	socket := coldRestoreSocket(t)
	prefix := fmt.Sprintf("test_coldrestore_%d_", time.Now().UnixNano())

	// First instance: create and start normally to put a live tmux session in place.
	inst1, cleanup1, err := NewInstanceWithCleanup(sleepInstanceOptions(title, tmpDir, prefix, socket))
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

	wait.RequireEventually(t, inst1.TmuxAlive, 10*time.Second, 50*time.Millisecond, "inst1 tmux session must be alive before hot restore")

	// Second instance: same title/socket — simulates an instance reloaded from storage
	// while the original tmux session is still alive.
	inst2, cleanup2, err := NewInstanceWithCleanup(sleepInstanceOptions(title, tmpDir, prefix, socket))
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

// TestFromInstanceData_should_NotReviveStoppedToActive_When_OneShotSessionHasLiveTmuxPane
// is a regression test for the production respawn loop first seen on backlog
// item ce71ad1a-a6a5-485f-8245-c5a502754a8b: a `backlog:review` one-shot
// session, already archived at the item level but with its own ArchivedAt
// left nil (an orphaned pre-item_sessions-linkage row), was recreated roughly
// every 60s for hours. The Stopped-branch "secretly still alive" probe in
// fromInstanceData (session/instance_serialization.go) saw the tmux pane
// still running (remain-on-exit keeps a dead pane around, see that probe's
// doc comment) and flipped Stopped -> Active -> Start(false), contradicting
// session_driver.go's handleStoppedStatus, which already treats a one-shot
// session's Stopped status as terminal ("driver exits cleanly"). Uses a real
// tmux session (not a mock) so the test exercises the actual IsAlive()/
// PaneExitStatus() probe the production bug hit, not a stand-in for it.
func TestFromInstanceData_should_NotReviveStoppedToActive_When_OneShotSessionHasLiveTmuxPane(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration test that starts real tmux sessions")
	}
	checkTmuxAvailable(t)

	t.Run("oneshot_review_tag_stays_stopped", func(t *testing.T) {
		t.Parallel()
		assertStoppedReloadOutcome(t, []string{"backlog:review"}, Stopped, true)
	})

	t.Run("non_oneshot_control_revives_to_active", func(t *testing.T) {
		t.Parallel()
		assertStoppedReloadOutcome(t, nil, Active, false)
	})
}

// assertStoppedReloadOutcome starts a real, long-running tmux session, then
// reloads it from a Stopped/unarchived InstanceData carrying the given tags
// — the same shape LoadInstances feeds fromInstanceData's "secretly still
// alive" probe — and asserts the resulting status/started flags. A live pane
// (not a mock) so the assertion exercises the actual IsAlive()/
// PaneExitStatus() probe the production respawn loop hit.
func assertStoppedReloadOutcome(t *testing.T, tags []string, wantStatus Status, wantStarted bool) {
	t.Helper()
	inst, title, tmpDir, prefix, socket := newLiveSleepInstance(t)

	wait.RequireEventually(t, inst.TmuxAlive, 10*time.Second, 50*time.Millisecond, "tmux session must be alive before restore")

	data := InstanceData{
		Title:            title,
		Path:             tmpDir,
		Status:           Stopped,
		Program:          "sleep 300",
		TmuxPrefix:       prefix,
		TmuxServerSocket: socket,
		Tags:             tags,
	}
	restored, err := FromInstanceDataDeferred(data)
	require.NoError(t, err)

	assert.Equal(t, wantStatus, restored.Snapshot().Status)
	assert.Equal(t, wantStarted, restored.Started())
}

// newLiveSleepInstance starts a real tmux session running a long-lived
// program on an isolated socket/prefix and registers its teardown, returning
// the live Instance plus the identifiers needed to reload it via
// InstanceData (title, path, tmux prefix, tmux socket).
func newLiveSleepInstance(t *testing.T) (inst *Instance, title, tmpDir, prefix, socket string) {
	t.Helper()
	title = fmt.Sprintf("test-oneshot-revive-%d", time.Now().UnixNano())
	tmpDir = t.TempDir()
	socket = coldRestoreSocket(t)
	prefix = fmt.Sprintf("test_coldrestore_%d_", time.Now().UnixNano())

	inst, cleanup, err := NewInstanceWithCleanup(sleepInstanceOptions(title, tmpDir, prefix, socket))
	require.NoError(t, err)
	t.Cleanup(func() { logTeardownErr(t, "cleanup", cleanup()) })

	startCleanup, err := inst.StartWithCleanup(true)
	require.NoError(t, err)
	if startCleanup != nil {
		t.Cleanup(func() { logTeardownErr(t, "startCleanup", startCleanup()) })
	}
	return inst, title, tmpDir, prefix, socket
}

// logTeardownErr logs a non-nil test-cleanup error without failing the test
// — teardown failures are informational, matching this file's existing
// cleanup-callback convention.
func logTeardownErr(t *testing.T, label string, err error) {
	t.Helper()
	if err != nil {
		t.Logf("%s warning: %v", label, err)
	}
}

// TestIsStaleResumeExit verifies the detection function used by the auto-recovery path.
func TestIsStaleResumeExit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		content []byte
		want    bool
	}{
		{
			name:    "plain text match",
			content: []byte("No conversation found with session ID: 550e8400-e29b-41d4-a716-446655440000\n"),
			want:    true,
		},
		{
			name:    "ANSI colour codes around message",
			content: []byte("\x1b[31mNo conversation found with session ID: 02c8a5f5-6604-4bcb-957c-be98ec8db4f3\x1b[0m\n"),
			want:    true,
		},
		{
			name:    "normal session output",
			content: []byte("> Hello, how can I help you?\n"),
			want:    false,
		},
		{
			name:    "empty content",
			content: nil,
			want:    false,
		},
		{
			name:    "rate limit error (should not match)",
			content: []byte("Usage limit reached for claude-opus-4-5\n"),
			want:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, isStaleResumeExit("claude", tt.content))
		})
	}
}

// TestTryExtractConversationUUID_ClearedAtGuard verifies AC3 of
// project_plans/cold-restart-uuid-recovery: a DetectByPath candidate JSONL that
// predates an explicit ClearConversationState() call must not be resurrected —
// the guard leaves claudeSession nil / ConversationUUID empty rather than
// resuming a conversation the user explicitly discarded — while a JSONL written
// AFTER the clear (e.g. a new conversation, later interrupted again) is still
// recovered normally, proving the guard is one-sided rather than a permanent
// recovery kill switch. No live tmux needed: tryExtractConversationUUID's PID
// fast path is skipped (bare Instance, pm() has no session), so it goes
// straight to the DetectByPath fallback.
func TestTryExtractConversationUUID_ClearedAtGuard(t *testing.T) {
	t.Parallel()
	const fixtureUUID = "550e8400-e29b-41d4-a716-446655440000"

	tests := []struct {
		name       string
		offset     time.Duration
		wantUUID   string
		wantReason string
	}{
		{name: "predates clear is not resurrected", offset: -1 * time.Hour, wantUUID: "", wantReason: "a JSONL predating the explicit clear must not be resurrected"},
		{name: "postdates clear is still recovered", offset: 1 * time.Hour, wantUUID: fixtureUUID, wantReason: "a JSONL postdating the explicit clear must still be recovered"},
		// The guard is `!info.ModTime.After(clearedAt)`, an inclusive comparison:
		// a JSONL with the exact same mtime as the clear is treated as predating
		// it (safe default — do not resurrect on a tie) rather than recovered.
		{name: "exactly at clear time is not resurrected", offset: 0, wantUUID: "", wantReason: "a JSONL with the same mtime as the clear must not be resurrected (inclusive boundary)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tmpDir := t.TempDir()
			fakeHome := t.TempDir()
			clearedAt := time.Now()

			inst := &Instance{
				Title:           "test-clearedat-guard",
				Path:            tmpDir,
				SessionType:     SessionTypeDirectory,
				claudeExtension: claudeExtension{conversationClearedAt: clearedAt},
				historyDetector: NewHistoryFileDetectorWithHomeDir(&mockProcessInspector{files: []string{}}, fakeHome),
			}
			writeJSONLFixture(t, fakeHome, tmpDir, fixtureUUID, clearedAt.Add(tt.offset))

			inst.tryExtractConversationUUID()

			gotUUID := ""
			if inst.claudeSession != nil {
				gotUUID = inst.claudeSession.ConversationUUID
			}
			assert.Equal(t, tt.wantUUID, gotUUID, tt.wantReason)
		})
	}
}

// simulatedLiveInstance is a minimal stand-in for the (UUID, conversation
// UUID, path, liveness) tuple server/services.SessionService.
// ConversationOwnedByOtherLiveSession reads from its reviewQueuePoller.
// session package tests cannot construct a real SessionService (that type,
// and its own regression test for ConversationOwnedByOtherLiveSession's own
// matching logic, live in server/services -- out of this package's scope),
// so newSimulatedConversationOwnershipGuard below builds a
// conversationOwnershipGuard closure with the same "skip self, skip dead,
// require both UUID and path to match" contract the real implementation
// documents (worktree-envvars-hijack plan.md, Story 1.4.2). These tests exist
// to prove tryExtractConversationUUID's own calling contract -- that it
// invokes the guard with the detected candidate UUID and effective path, and
// correctly no-ops when told the conversation is owned by another live
// session -- not to re-verify ConversationOwnedByOtherLiveSession's own
// matching logic a second time.
type simulatedLiveInstance struct {
	uuid             string
	conversationUUID string
	path             string
	alive            bool
}

// newSimulatedConversationOwnershipGuard returns a conversationOwnershipGuard
// closure over siblings, mirroring ConversationOwnedByOtherLiveSession's own
// selfUUID exclusion and liveness/UUID/path matching.
func newSimulatedConversationOwnershipGuard(selfUUID string, siblings ...simulatedLiveInstance) func(candidateUUID, path string) (string, bool) {
	return func(candidateUUID, path string) (string, bool) {
		for _, sib := range siblings {
			if sib.uuid == selfUUID || !sib.alive {
				continue
			}
			if sib.conversationUUID != candidateUUID || sib.path != path {
				continue
			}
			return sib.uuid, true
		}
		return "", false
	}
}

// TestTryExtractConversationUUID_should_NotAdoptUUID_When_OtherLiveSessionOwnsConversation
// is worktree-envvars-hijack Task 1.4.2b's cross-session non-adoption
// regression test. Session A's real, fake JSONL is the only conversation file
// under the shared directory's encoded ~/.claude/projects/ path -- the
// legitimate SessionTypeDirectory path-sharing case Story 1.4.1's audit
// confirmed can still occur even after Epic 3.3's collision guard lands.
// Without the ownership guard, session B's DetectByPath fallback would find
// that same JSONL and silently adopt A's still-live conversation UUID; with
// the guard wired (as production's wireCallbacks does via
// SetConversationOwnershipGuard), B must find nothing.
func TestTryExtractConversationUUID_should_NotAdoptUUID_When_OtherLiveSessionOwnsConversation(t *testing.T) {
	t.Parallel()
	sharedPath := t.TempDir()
	fakeHome := t.TempDir()
	const sessionAUUID = "session-a-uuid"
	const conversationUUID = "550e8400-e29b-41d4-a716-446655440000"

	writeJSONLFixture(t, fakeHome, sharedPath, conversationUUID, time.Time{})

	instB := &Instance{
		Title:           "session-b",
		UUID:            "session-b-uuid",
		Path:            sharedPath,
		SessionType:     SessionTypeDirectory,
		historyDetector: NewHistoryFileDetectorWithHomeDir(&mockProcessInspector{files: []string{}}, fakeHome),
	}
	instB.SetConversationOwnershipGuard(newSimulatedConversationOwnershipGuard(instB.UUID,
		simulatedLiveInstance{uuid: sessionAUUID, conversationUUID: conversationUUID, path: sharedPath, alive: true},
	))

	instB.tryExtractConversationUUID()

	assert.Nil(t, instB.claudeSession, "B must not silently adopt A's still-live conversation UUID via the path-shared DetectByPath fallback")
}

// TestTryExtractConversationUUID_should_AdoptOwnUUID_When_ColdRestoreSelfRecovery
// is worktree-envvars-hijack Task 1.4.2b's ColdRestore self-recovery
// regression test (pre-mortem Failure #4, P2). A session undergoing
// ColdRestore reuses its own, already-registered *Instance object to call
// tryExtractConversationUUID with no ClaudeConversationUUID set yet -- the
// guard's sibling registry here contains only this same instance's own
// record, proving the inst.UUID == selfUUID exclusion actually lets it
// re-adopt its own conversation rather than treating its own record as
// "owned by another session."
func TestTryExtractConversationUUID_should_AdoptOwnUUID_When_ColdRestoreSelfRecovery(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	fakeHome := t.TempDir()
	const fixtureUUID = "550e8400-e29b-41d4-a716-446655440000"

	writeJSONLFixture(t, fakeHome, tmpDir, fixtureUUID, time.Time{})

	inst := &Instance{
		Title:           "cold-restore-self-recovery",
		UUID:            "cold-restore-self-uuid",
		Path:            tmpDir,
		SessionType:     SessionTypeDirectory,
		historyDetector: NewHistoryFileDetectorWithHomeDir(&mockProcessInspector{files: []string{}}, fakeHome),
	}
	inst.SetConversationOwnershipGuard(newSimulatedConversationOwnershipGuard(inst.UUID,
		simulatedLiveInstance{uuid: inst.UUID, conversationUUID: fixtureUUID, path: tmpDir, alive: true},
	))

	inst.tryExtractConversationUUID()

	require.NotNil(t, inst.claudeSession, "ColdRestore self-recovery must not be blocked by the ownership guard")
	assert.Equal(t, fixtureUUID, inst.claudeSession.ConversationUUID, "must successfully re-adopt its own JSONL's conversation UUID")
}
