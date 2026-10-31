package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/session/tmux"
	"github.com/tstapler/stapler-squad/session/tymux"
)

func restoredPausedData(t *testing.T, title string) InstanceData {
	t.Helper()
	return InstanceData{Title: title, Path: t.TempDir(), Program: "claude", Status: Paused}
}

// A session persisted before its per-session tymux override existed has an empty
// Backend; restore must still honor the override instead of falling to the global tmux default.
func TestFromInstanceData_EmptyBackend_HonorsTymuxSessionOverride(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	force := true
	require.NoError(t, config.LoadConfig().SetTymuxSessionOverride(tmux.NewSessionName("dotfiles", tmux.TmuxPrefix).String(), &force))

	inst, err := FromInstanceDataDeferred(restoredPausedData(t, "dotfiles"))

	require.NoError(t, err)
	assert.Equal(t, BackendTymux, inst.Backend)
	_, isTymux := inst.processManager.(*TymuxBackend)
	assert.True(t, isTymux, "process manager must be a TymuxBackend, got %T", inst.processManager)
}

func TestFromInstanceData_EmptyBackend_NoOverride_StaysOnDefault(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())

	inst, err := FromInstanceDataDeferred(restoredPausedData(t, "no-override"))

	require.NoError(t, err)
	assert.Empty(t, inst.Backend, "no override means the backend stays unpinned")
	_, isTmux := inst.processManager.(*TmuxBackend)
	assert.True(t, isTmux)
}

// BUG-113: a restored tymux session whose tymuxd-side session is gone falls back to
// Start, which must carry the program rather than let tymuxd spawn a bare $SHELL.
func TestFromInstanceData_TymuxBackend_SetsBaselineCommand(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	fake := &fakeTymuxManagerForSetCommand{}
	orig := newTymuxSessionFn
	newTymuxSessionFn = func() tymux.TymuxManager { return fake }
	defer func() { newTymuxSessionFn = orig }()
	data := restoredPausedData(t, "tymux-restore")
	data.Backend = BackendTymux

	_, err := FromInstanceDataDeferred(data)

	require.NoError(t, err)
	assert.Equal(t, "claude", fake.gotCommand)
}

// wireTmuxSession must reach every backend through ProcessManager.WireLaunch; before it
// did, Resume skipped non-tmux backends and tymux relaunched without --resume.
func TestWireTmuxSession_DeliversLaunchCommandToTymuxBackend(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	fake := &fakeTymuxManagerForSetCommand{}
	inst := &Instance{Title: "wire-tymux", Path: t.TempDir(), Program: "claude", Backend: BackendTymux}
	inst.processManager = NewTymuxBackend(fake)

	inst.wireTmuxSession(shellLaunch("claude --resume abc"))

	assert.Equal(t, "claude --resume abc", fake.gotCommand)
}

func TestWireTmuxSession_InstallsTmuxSessionOnTmuxBackend(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	mgr := &TmuxProcessManager{}
	inst := &Instance{Title: "wire-tmux", Path: t.TempDir(), Program: "claude"}
	inst.processManager = NewTmuxBackend(mgr)

	inst.wireTmuxSession(shellLaunch("claude"))

	assert.NotNil(t, mgr.Session())
}

// deadTymuxManager reports no live session so restore branches that probe liveness don't panic.
type deadTymuxManager struct{ fakeTymuxManagerForSetCommand }

func (*deadTymuxManager) IsAlive() bool { return false }

// Every persisted status takes its own branch in fromInstanceData; each must hand the
// program to a tymux backend, or that branch regresses to a bare $SHELL (BUG-113).
func TestFromInstanceData_EveryStatusBranch_WiresTymuxCommand(t *testing.T) {
	for _, status := range []Status{Paused, Stopped, Hibernated, Crashed, Active} {
		t.Run(status.String(), func(t *testing.T) {
			t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
			fake := &deadTymuxManager{}
			orig := newTymuxSessionFn
			newTymuxSessionFn = func() tymux.TymuxManager { return fake }
			defer func() { newTymuxSessionFn = orig }()
			data := restoredPausedData(t, "branch-"+status.String())
			data.Status = status
			data.Backend = BackendTymux

			_, err := FromInstanceDataDeferred(data)

			require.NoError(t, err)
			assert.Equal(t, "claude", fake.gotCommand)
		})
	}
}

// A legacy session persisted with no Backend was created as tmux. Flipping the global tymux
// flag later must not silently re-route it (and its stream) to tymux.
func TestFromInstanceData_EmptyBackend_GlobalTymux_StaysTmux(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	on := true
	require.NoError(t, config.LoadConfig().SetTymuxGlobalOverride(&on))

	inst, err := FromInstanceDataDeferred(restoredPausedData(t, "legacy-tmux"))

	require.NoError(t, err)
	assert.Equal(t, BackendTmux, inst.Backend, "legacy sessions are pinned to tmux")
	_, isTmux := inst.processManager.(*TmuxBackend)
	assert.True(t, isTmux, "process manager must stay a TmuxBackend, got %T", inst.processManager)
}
