package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

// fakeTymuxManagerForStartRestore is a minimal tymux.TymuxManager fake for
// Story 2.1.3's Start/RestoreWithWorkDir tests: it embeds the interface (so
// any unused method panics if called — none should be for these tests) and
// overrides only Start/RestoreWithWorkDir, with optional hooks so a test can
// observe call order relative to the injected ensureDaemonRunningFn.
type fakeTymuxManagerForStartRestore struct {
	tymux.TymuxManager // deliberately nil; unused methods must not be called
	startErr           error
	restoreErr         error
	onStart            func(dir string)
	onRestore          func(workDir string)
}

func (f *fakeTymuxManagerForStartRestore) Start(dir string) error {
	if f.onStart != nil {
		f.onStart(dir)
	}
	return f.startErr
}

func (f *fakeTymuxManagerForStartRestore) RestoreWithWorkDir(w string) error {
	if f.onRestore != nil {
		f.onRestore(w)
	}
	return f.restoreErr
}

// TestTymuxBackendStart_should_CallEnsureDaemonRunning_BeforeDelegatingToManager
// is Task 2.1.3b: Start must call tymux.EnsureDaemonRunning (via its
// injection seam) before delegating to the wrapped TymuxManager's Start —
// the whole point of Story 2.1.3's lazy call-before-use wiring.
func TestTymuxBackendStart_should_CallEnsureDaemonRunning_BeforeDelegatingToManager(t *testing.T) {
	var order []string
	fake := &fakeTymuxManagerForStartRestore{
		onStart: func(dir string) { order = append(order, "start:"+dir) },
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
	fake := &fakeTymuxManagerForStartRestore{
		onRestore: func(w string) { order = append(order, "restore:"+w) },
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
	fake := &fakeTymuxManagerForStartRestore{
		onStart: func(string) { startCalled = true },
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

// fakeTymuxManagerForBackendRestarted is a minimal tymux.TymuxManager fake
// used only to prove BackendRestarted() is reachable through the interface
// (Phase 5 spec-compliance sweep, Story 2.5.3 gap): it embeds the interface
// (so any unused method panics if called — none should be for this test)
// and overrides only BackendRestarted with a canned value.
type fakeTymuxManagerForBackendRestarted struct {
	tymux.TymuxManager // deliberately nil; unused methods must not be called
	restarted          bool
	since              time.Time
}

func (f *fakeTymuxManagerForBackendRestarted) BackendRestarted() (bool, time.Time) {
	return f.restarted, f.since
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
	fake := &fakeTymuxManagerForBackendRestarted{restarted: true, since: since}

	// mgr is typed as the interface, not *tymuxGRPCSession — the whole
	// point of the fix.
	var mgr tymux.TymuxManager = fake
	backend := NewTymuxBackend(mgr)

	restarted, gotSince := backend.BackendRestarted()

	assert.True(t, restarted, "BackendRestarted() must be reachable through *TymuxBackend, not just the unexported concrete type")
	assert.Equal(t, since, gotSince)
}
