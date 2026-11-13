package session

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tstapler/stapler-squad/session/domain"
)

// TestIsBackendProcessAlive_UsesNoCacheCheck_UnlikeTmuxAlive is the
// regression test for a bug caught reviewing this exact refactor: the
// generic replacement for connectrpc_websocket.go's GetTmuxSession()+
// DoesSessionExistNoCache() reach-in must call the ProcessManager's no-cache
// check, not the cached IsAlive() TmuxAlive() itself uses — a stale cached
// positive there causes control mode to attach to a dead session and
// immediately receive %exit (see the two callers' own comments). Also
// confirms IsBackendProcessAlive() bypasses Status/started gating, unlike
// TmuxAlive(), since streamViaHub/streamViaControlMode call it before that
// gating would even apply.
func TestIsBackendProcessAlive_UsesNoCacheCheck_UnlikeTmuxAlive(t *testing.T) {
	// isAliveReturn (cached) says dead; existsNoCacheReturn (fresh) says
	// alive — a stale-cache-vs-fresh-check divergence. If
	// IsBackendProcessAlive() used the cached path, it would wrongly
	// report false here.
	mock := &mockTmuxManager{hasSessionReturn: true, isAliveReturn: false, existsNoCacheReturn: true}
	inst := (&Instance{
		Title:  "t",
		Status: domain.Creating,
	}).WithProcessManagerForTest(NewTmuxBackend(mock))
	// started deliberately left false: TmuxAlive() would report false here,
	// but IsBackendProcessAlive() must not gate on it.

	if !inst.IsBackendProcessAlive() {
		t.Error("IsBackendProcessAlive() = false, want true (must use the no-cache check, and must not gate on Status/started like TmuxAlive())")
	}
	if inst.TmuxAlive() {
		t.Error("test setup invalid: TmuxAlive() should be false here (started==false) to prove the two diverge")
	}
}

// TestIsBackendProcessAlive_ColdStart_HasSessionFalse covers the other half
// of IsBackendProcessAlive's short-circuit (HasSession() && HasLiveSessionNoCache()):
// a freshly created instance whose backend has never had a session attached
// at all (hasSessionReturn: false, the cold-start case — e.g. before the
// first Start()/RestoreProcess() call ever runs). IsBackendProcessAlive must
// report false via the HasSession() short-circuit alone, without even
// reaching HasLiveSessionNoCache() — the caller's own tmux-alive check must
// never need a live session to answer this safely.
func TestIsBackendProcessAlive_ColdStart_HasSessionFalse(t *testing.T) {
	mock := &mockTmuxManager{hasSessionReturn: false, existsNoCacheReturn: true}
	inst := (&Instance{
		Title:  "t",
		Status: domain.Creating,
	}).WithProcessManagerForTest(NewTmuxBackend(mock))

	if inst.IsBackendProcessAlive() {
		t.Error("IsBackendProcessAlive() = true, want false when HasSession() is false (no backend session object exists yet)")
	}
}

// TestInitTmuxSession_StalePointerAliveFalse_RebuildsWithResume is the
// regression test for the 2026-09-12 mass tmux-kill-server incident: after a
// tmux server crash, every instance's in-process TmuxSession pointer is still
// non-nil (HasSession() true) even though the OS-level session it points to
// is gone (IsAlive() false) -- exactly what a fake TmuxManager reports here
// without needing a real tmux server. Before the fix, initTmuxSession()'s
// guard checked HasSession() alone and returned early, skipping
// buildLaunchCommand() entirely and leaving i.LaunchCommand as whatever
// resume-less string was baked in at construction -- while the caller's own
// "cold restoring with --resume" log line (session/instance.go) claimed
// otherwise. Asserting on i.LaunchCommand (not a log line) is what makes this
// a real regression check: it fails against the pre-fix guard and passes once
// initTmuxSession() also requires IsAlive().
func TestInitTmuxSession_StalePointerAliveFalse_RebuildsWithResume(t *testing.T) {
	const uuid = "550e8400-e29b-41d4-a716-446655440000"
	mock := &mockTmuxManager{hasSessionReturn: true, isAliveReturn: false}
	inst := (&Instance{
		Title:         "t",
		Program:       "claude",
		LaunchCommand: "claude", // stale command baked in before the crash, with no --resume
	}).WithProcessManagerForTest(NewTmuxBackend(mock))
	inst.SetClaudeSession(&domain.ClaudeSessionData{ConversationUUID: uuid})

	inst.InitTmuxSessionForTest()

	if !strings.Contains(inst.LaunchCommand, "--resume") || !strings.Contains(inst.LaunchCommand, uuid) {
		t.Errorf("LaunchCommand = %q, want it rebuilt with \"--resume %s\" (pointer stale but IsAlive()==false must not short-circuit the rebuild)", inst.LaunchCommand, uuid)
	}
}

// TestRestoreProcess_DelegatesToProcessManager confirms RestoreProcess is a
// thin, backend-agnostic pass-through to ProcessManager.RestoreWithWorkDir —
// the replacement for reaching into a concrete *tmux.TmuxSession via
// GetTmuxSession().RestoreWithWorkDir().
func TestRestoreProcess_DelegatesToProcessManager(t *testing.T) {
	mock := &mockTmuxManager{restoreReturn: errRestoreFailedForTest}
	inst := (&Instance{Title: "t"}).WithProcessManagerForTest(NewTmuxBackend(mock))

	err := inst.RestoreProcess("/some/dir")

	if mock.restoreCalls != 1 {
		t.Errorf("RestoreWithWorkDir calls = %d, want 1", mock.restoreCalls)
	}
	if !errors.Is(err, errRestoreFailedForTest) {
		t.Errorf("RestoreProcess() error = %v, want the ProcessManager's own error propagated", err)
	}
}

var errRestoreFailedForTest = errors.New("restore failed (test)")

// TestInitTmuxSession_ReuseRequiresAliveNotJustConstructed is the fast, mocked
// counterpart to TestKillSessionThenStart_DoesNotRebuildLaunchCommand: it
// exercises initTmuxSession()'s reuse guard directly against the three
// HasSession()/IsAlive() combinations, without spinning up a real tmux binary.
// HasSession()=true alone must NOT be enough to reuse — the gate must also
// require IsAlive(), or a stale *tmux.TmuxSession pointer left behind by a
// crashed tmux server silently skips buildLaunchCommand() (and its --resume
// rebuild) forever. See instance_tmux.go's initTmuxSession doc comment.
func TestInitTmuxSession_ReuseRequiresAliveNotJustConstructed(t *testing.T) {
	t.Parallel()

	const sentinel = "sentinel-unchanged-launch-command"
	cases := []struct {
		name        string
		hasSession  bool
		isAlive     bool
		wantRebuild bool
	}{
		{"stale pointer, dead session: rebuilds", true, false, true},
		{"live session: reuses, no rebuild", true, true, false},
		{"cold start, never constructed: rebuilds", false, false, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mock := &mockTmuxManager{hasSessionReturn: tc.hasSession, isAliveReturn: tc.isAlive}
			inst := (&Instance{
				Title:         "t",
				Program:       "some-other-program",
				LaunchCommand: sentinel,
			}).WithProcessManagerForTest(NewTmuxBackend(mock))

			inst.InitTmuxSessionForTest()

			rebuilt := inst.LaunchCommand != sentinel
			if rebuilt != tc.wantRebuild {
				t.Errorf("HasSession=%v IsAlive=%v: LaunchCommand rebuilt = %v, want %v (LaunchCommand=%q)",
					tc.hasSession, tc.isAlive, rebuilt, tc.wantRebuild, inst.LaunchCommand)
			}
		})
	}
}

func TestInstance_CanFallBackToTmuxSendKeys_OnlyForATmuxBackend(t *testing.T) {
	tmuxInst := (&Instance{Title: "t"}).WithProcessManagerForTest(NewTmuxBackend(&TmuxProcessManager{}))
	tymuxInst := (&Instance{Title: "t"}).WithProcessManagerForTest(NewTymuxBackend(nil))

	assert.True(t, tmuxInst.CanFallBackToTmuxSendKeys())
	assert.False(t, tymuxInst.CanFallBackToTmuxSendKeys())
}
