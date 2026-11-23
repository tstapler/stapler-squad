package session

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/internal/fixturetest"
	"github.com/tstapler/stapler-squad/session/internal/procbackendtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/launchcmd"
	"github.com/tstapler/stapler-squad/session/tymux"
)

// stubEnsureDaemonRunning substitutes backend_tymux.go's ensureDaemonRunningFn
// injection seam with a deterministic fake, mirroring
// session/tymux/supervise_test.go's save/restore-via-defer stub helpers for
// checkDaemonHealthyFn/startDaemonAttemptFn. Restores the original on the
// returned func, meant to be deferred.
func stubEnsureDaemonRunning(fn func(context.Context, tymux.DaemonConfig) (tymux.TymuxdReady, error)) func() {
	orig := ensureDaemonRunningFn
	ensureDaemonRunningFn = fn
	return func() { ensureDaemonRunningFn = orig }
}

// TestTymuxBackendStart_should_CallEnsureDaemonRunning_BeforeDelegatingToManager
// is Task 2.1.3b: Start must call tymux.EnsureDaemonRunning (via its
// injection seam) before delegating to the wrapped TymuxManager's Start —
// the whole point of Story 2.1.3's lazy call-before-use wiring.
func TestTymuxBackendStart_should_CallEnsureDaemonRunning_BeforeDelegatingToManager(t *testing.T) {
	var order []string
	fake := &procbackendtest.FakeTymuxManagerForStartRestore{
		OnStart: func(dir string) { order = append(order, "start:"+dir) },
	}
	defer stubEnsureDaemonRunning(func(context.Context, tymux.DaemonConfig) (tymux.TymuxdReady, error) {
		order = append(order, "ensure")
		return tymux.TymuxdReady{}, nil
	})()

	backend := NewTymuxBackend(fake)
	err := backend.Start("/some/dir")

	require.NoError(t, err)
	assert.Equal(t, []string{"ensure", "start:/some/dir"}, order,
		"EnsureDaemonRunning must run before the wrapped manager's Start")
}

// TestTymuxBackendRestoreWithWorkDir_should_CallEnsureDaemonRunning_BeforeDelegatingToManager
// mirrors the Start test above for the restore path.
func TestTymuxBackendRestoreWithWorkDir_should_CallEnsureDaemonRunning_BeforeDelegatingToManager(t *testing.T) {
	var order []string
	fake := &procbackendtest.FakeTymuxManagerForStartRestore{
		OnRestore: func(w string) { order = append(order, "restore:"+w) },
	}
	defer stubEnsureDaemonRunning(func(context.Context, tymux.DaemonConfig) (tymux.TymuxdReady, error) {
		order = append(order, "ensure")
		return tymux.TymuxdReady{}, nil
	})()

	backend := NewTymuxBackend(fake)
	err := backend.RestoreWithWorkDir("/some/workdir")

	require.NoError(t, err)
	assert.Equal(t, []string{"ensure", "restore:/some/workdir"}, order,
		"EnsureDaemonRunning must run before the wrapped manager's RestoreWithWorkDir")
}

// TestTymuxBackendStart_should_ReturnWrappedError_When_DaemonUnavailable is
// Task 2.1.3b's error path: when EnsureDaemonRunning fails, Start must
// return a wrapped, non-nil error and must never call through to the
// wrapped TymuxManager's own Start — no partial/silent proceed with an
// unverified daemon (mirrors Task 2.1.2d's port-squat "fail loudly" posture).
func TestTymuxBackendStart_should_ReturnWrappedError_When_DaemonUnavailable(t *testing.T) {
	sentinelErr := errors.New("boom: tymuxd did not become healthy")
	startCalled := false
	fake := &procbackendtest.FakeTymuxManagerForStartRestore{
		OnStart: func(string) { startCalled = true },
	}
	defer stubEnsureDaemonRunning(func(context.Context, tymux.DaemonConfig) (tymux.TymuxdReady, error) {
		return tymux.TymuxdReady{}, sentinelErr
	})()

	backend := NewTymuxBackend(fake)
	err := backend.Start("/some/dir")

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinelErr)
	assert.Contains(t, err.Error(), "tymux backend requested but daemon unavailable")
	assert.False(t, startCalled, "the wrapped manager's Start must not be called when the daemon is unavailable")
}

// TestTymuxBackend_BackendRestarted_ObservableThroughTymuxManagerInterface
// closes the Phase 5 sweep's Gap 1: Story 2.5.3's acceptance criterion says
// BackendTymux "surfaces a distinct...condition to its caller," but
// BackendRestarted() previously existed only on the unexported concrete
// tymuxGRPCSession type, so no real caller holding a
// ProcessManager/TymuxManager reference could ever observe it — only tests
// doing an unexported-type assertion from inside the package could. This
// test drives BackendRestarted() through a fake TymuxManager (typed as the
// interface, never the concrete type) wrapped by *TymuxBackend, proving a
// caller with only the interface/TymuxBackend surface can observe the
// state.
func TestTymuxBackend_BackendRestarted_ObservableThroughTymuxManagerInterface(t *testing.T) {
	since := time.Now()
	fake := &procbackendtest.FakeTymuxManagerForBackendRestarted{Restarted: true, Since: since}

	// mgr is typed as the interface, not *tymuxGRPCSession — the whole
	// point of the fix.
	var mgr tymux.TymuxManager = fake
	backend := NewTymuxBackend(mgr)

	restarted, gotSince := backend.BackendRestarted()

	assert.True(t, restarted, "BackendRestarted() must be reachable through *TymuxBackend, not just the unexported concrete type")
	assert.Equal(t, since, gotSince)
}

// TestInstanceRestart_MigratesBackendWhenOverrideDiffersFromCurrent proves
// Instance.Restart's migrating branch actually swaps i.processManager and
// persists i.Backend to the requested target, rather than respawning the
// instance's current backend in place.
//
// newTymuxBackendFromOpts dials tymux.ResolveDaemonConfig().Addr for real --
// with no TYMUXD_ADDR/STAPLER_SQUAD_INSTANCE override that resolves to the
// *shared default* address (defaultTymuxdAddr, 127.0.0.1:7419), which a real
// developer machine running this suite can easily have a real tymuxd
// listening on right now. Hitting that would create a real session against
// live infra, not a test fixture. TYMUXD_ADDR is pinned here to a genuinely
// unused loopback port (bind-then-immediately-close) specifically so the
// Start() call below fails fast on connection-refused against nothing, never
// against something real.
func TestInstanceRestart_MigratesBackendWhenOverrideDiffersFromCurrent(t *testing.T) {
	t.Setenv("TYMUXD_ADDR", "http://"+unusedLoopbackAddr(t))
	defer stubEnsureDaemonRunning(func(context.Context, tymux.DaemonConfig) (tymux.TymuxdReady, error) {
		return tymux.TymuxdReady{}, nil
	})()

	inst := &instance.Instance{
		Title:   "migrate-test",
		Path:    t.TempDir(),
		Program: "bash",
		Status:  domain.Active,
		Backend: domain.BackendTmux,
	}
	inst.MarkStarted()
	require.NoError(t, initializeBaselineForRestartTest(inst))

	err := inst.Restart(false, domain.BackendTymux)

	require.Error(t, err, "Start() against an unreachable tymuxd must fail in this sandboxed test environment")
	assert.Equal(t, domain.BackendTymux, inst.Backend,
		"Backend must be updated to the migration target even though the subsequent Start() failed -- the migration decision happens before Start() is attempted")
	_, ok := inst.ProcessManagerForTest().(*TymuxBackend)
	assert.True(t, ok, "processManager must be swapped to *TymuxBackend, not left as the original *TmuxBackend")
}

// unusedLoopbackAddr returns a loopback host:port that was free at the
// moment of the call (bound then immediately closed) -- used to point a
// test at an address guaranteed to refuse the connection, instead of the
// package's shared default tymuxd address, which a real developer machine
// can have a live daemon listening on.
func unusedLoopbackAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	return addr
}

// TestInstanceRestart_NoMigration_KeepsExistingBackend is
// MigratesBackendWhenOverrideDiffersFromCurrent's control case: an empty
// backendOverride must preserve today's respawn-in-place behavior exactly,
// never touching i.Backend or replacing i.processManager's type.
func TestInstanceRestart_NoMigration_KeepsExistingBackend(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test that starts real tmux sessions")
	}
	fixturetest.CheckTmuxAvailable(t)
	// A private server socket means this respawn hits a real but
	// test-isolated tmux server rather than the host's default one.
	inst := &instance.Instance{
		Title:            "no-migrate-test",
		Path:             t.TempDir(),
		Program:          "bash",
		Status:           domain.Active,
		Backend:          domain.BackendTmux,
		TmuxServerSocket: testTmuxSocket(t),
	}
	inst.MarkStarted()
	require.NoError(t, initializeBaselineForRestartTest(inst))
	defer func() { _ = inst.KillSession() }()

	require.NoError(t, inst.Restart(false, ""))

	assert.Equal(t, domain.BackendTmux, inst.Backend)
	_, ok := inst.ProcessManagerForTest().(*TmuxBackend)
	assert.True(t, ok, "processManager must remain a *TmuxBackend when no migration was requested")
}

// initializeBaselineForRestartTest constructs the instance's process manager the same way
// NewInstance does (via the factory, so it's a real *TmuxBackend matching
// i.Backend) without going through the rest of NewInstance's worktree/VNC/
// CDP machinery this test has no use for.
func initializeBaselineForRestartTest(i *instance.Instance) error {
	pm, err := NewProcessManager(context.Background(), domain.BackendTmux, instance.ProcessManagerOptions{Backend: i.Backend})
	if err != nil {
		return err
	}
	i.SetProcessManagerForTest(pm)
	return nil
}

// TestNewTymuxBackendFromOpts_AppliesProgramViaSetCommand closes the gap
// where newTymuxBackendFromOpts's opts.Program->SetCommand wiring had no
// test coverage and, before the newTymuxSessionFn seam existed, no way to
// get any either.
func TestNewTymuxBackendFromOpts_AppliesProgramViaSetCommand(t *testing.T) {
	fake := &procbackendtest.FakeTymuxManagerForSetCommand{}
	orig := newTymuxSessionFn
	newTymuxSessionFn = func() tymux.TymuxManager { return fake }
	defer func() { newTymuxSessionFn = orig }()

	newTymuxBackendFromOpts(instance.ProcessManagerOptions{Program: "claude --resume abc"})

	assert.Equal(t, "claude --resume abc", fake.GotCommand)
	exec, ok := fake.GotLaunch.(launchcmd.ExecCommand)
	require.True(t, ok, "a plain program must reach the backend as argv, got %T", fake.GotLaunch)
	args, err := exec.Args(nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"--resume", "abc"}, args)
}

// TestInstanceRestart_NoMigration_ExistingTymuxBackend_AppliesProgramViaSetCommand
// covers the in-place (non-migrating) tymux-respawn branch of Restart --
// every tymux-backed session's ordinary restart hits this path, and it had
// zero coverage (the two existing Restart tests cover only tmux->tymux
// migration and tmux->tmux no-migration).
func TestInstanceRestart_NoMigration_ExistingTymuxBackend_AppliesProgramViaSetCommand(t *testing.T) {
	defer stubEnsureDaemonRunning(func(context.Context, tymux.DaemonConfig) (tymux.TymuxdReady, error) {
		return tymux.TymuxdReady{}, nil
	})()
	fake := &procbackendtest.FakeTymuxManagerForSetCommand{}
	inst := &instance.Instance{
		Title:   "already-tymux-test",
		Path:    t.TempDir(),
		Program: "bash",
		Status:  domain.Active,
		Backend: domain.BackendTymux,
	}
	inst.MarkStarted()
	inst.SetProcessManagerForTest(NewTymuxBackend(fake))

	require.NoError(t, inst.Restart(false, ""))

	assert.Equal(t, domain.BackendTymux, inst.Backend)
	assert.NotEmpty(t, fake.GotCommand, "SetCommand must be called with the rebuilt launch command on an in-place tymux respawn")
}

func TestNewTymuxBackendFromOpts_AppliesSessionNameAsLabel(t *testing.T) {
	fake := &procbackendtest.FakeTymuxManagerForSetCommand{}
	orig := newTymuxSessionFn
	newTymuxSessionFn = func() tymux.TymuxManager { return fake }
	defer func() { newTymuxSessionFn = orig }()

	newTymuxBackendFromOpts(instance.ProcessManagerOptions{SessionName: "stapler-squad-2"})

	assert.Equal(t, "stapler-squad-2", fake.GotLabel)
}
