package session

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/tymux"
)

// Epic 2.1 Story 2.1.3 / Task 2.1.3d: ProcessManagerOptions.Backend is an explicit
// per-call override, checked ahead of the process-wide global (getSelectedBackend),
// so one session can select BackendTymux without affecting any other concurrent
// session that leaves opts.Backend unset. See backend_factory.go's NewProcessManager
// doc comment for the full precedence chain.

// TestNewProcessManager_ShouldReturnBackendTymux_WhenOptsBackendIsTymux is REQ-1's
// happy path (validation.md): an explicit per-call override selects *BackendTymux
// even while the process-wide global default remains BackendTmux.
func TestNewProcessManager_ShouldReturnBackendTymux_WhenOptsBackendIsTymux(t *testing.T) {
	RegisterBackendProvider(BackendTmux)
	defer RegisterBackendProvider(BackendTmux) // restore default for other tests

	pm, err := NewProcessManager(context.Background(), BackendTmux, ProcessManagerOptions{Backend: BackendTymux})
	require.NoError(t, err)
	_, ok := pm.(*TymuxBackend)
	assert.True(t, ok, "expected *TymuxBackend when opts.Backend is BackendTymux, got %T", pm)
}

// TestNewProcessManager_ShouldReturnBackendTmux_WhenOptsBackendIsUnset_MatchingTodaysDefault
// is REQ-7's happy path (validation.md, UX-9.1): an omitted opts.Backend continues to
// resolve exactly as it did before this field existed — via the process-wide global,
// falling back to defaultBackend/BackendTmux.
func TestNewProcessManager_ShouldReturnBackendTmux_WhenOptsBackendIsUnset_MatchingTodaysDefault(t *testing.T) {
	RegisterBackendProvider(BackendTmux)
	defer RegisterBackendProvider(BackendTmux)

	pm, err := NewProcessManager(context.Background(), BackendTmux, ProcessManagerOptions{})
	require.NoError(t, err)
	_, ok := pm.(*TmuxBackend)
	assert.True(t, ok, "expected *TmuxBackend when opts.Backend is unset and global is BackendTmux, got %T", pm)
}

// TestNewProcessManager_ShouldNotLeakTymuxOverride_IntoConcurrentBackendTmuxCall is
// REQ-7's regression path (validation.md, UX-9.3): two concurrent NewProcessManager
// calls with different Backend values never interfere — a BackendTymux override on
// one call must never leak into a sibling call that left opts.Backend unset.
func TestNewProcessManager_ShouldNotLeakTymuxOverride_IntoConcurrentBackendTmuxCall(t *testing.T) {
	RegisterBackendProvider(BackendTmux)
	defer RegisterBackendProvider(BackendTmux)

	const iterations = 50
	var wg sync.WaitGroup
	wg.Add(iterations * 2)

	for i := 0; i < iterations; i++ {
		go func() {
			defer wg.Done()
			pm, err := NewProcessManager(context.Background(), BackendTmux, ProcessManagerOptions{Backend: BackendTymux})
			if err != nil {
				t.Errorf("unexpected error for opts.Backend=BackendTymux: %v", err)
				return
			}
			if _, ok := pm.(*TymuxBackend); !ok {
				t.Errorf("expected *TymuxBackend for opts.Backend=BackendTymux, got %T", pm)
			}
		}()
		go func() {
			defer wg.Done()
			pm, err := NewProcessManager(context.Background(), BackendTmux, ProcessManagerOptions{})
			if err != nil {
				t.Errorf("unexpected error for unset opts.Backend: %v", err)
				return
			}
			if _, ok := pm.(*TmuxBackend); !ok {
				t.Errorf("expected *TmuxBackend for unset opts.Backend (global default), got %T", pm)
			}
		}()
	}
	wg.Wait()
}

// TestNewProcessManager_ShouldReturnConstructionError_WhenBackendConstantUnrecognized
// is REQ-1's error path (validation.md, UX-9.2): an unrecognized/garbage
// ProcessManagerBackend value must fail at construction with a returned
// error, not silently downgrade to *TmuxBackend and not panic. Exercised via
// opts.Backend (the highest-precedence input) so the process-wide global and
// defaultBackend never get a chance to mask the bad value.
func TestNewProcessManager_ShouldReturnConstructionError_WhenBackendConstantUnrecognized(t *testing.T) {
	RegisterBackendProvider(BackendTmux)
	defer RegisterBackendProvider(BackendTmux)

	garbage := ProcessManagerBackend("not-a-real-backend")

	pm, err := NewProcessManager(context.Background(), BackendTmux, ProcessManagerOptions{Backend: garbage})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnrecognizedBackend)
	assert.Nil(t, pm)
}

// TestNewProcessManager_should_ReturnImmediately_When_BackendIsTymuxRegardlessOfDaemonState
// is Task 2.1.3b's regression guard for ADR-004: NewProcessManager's
// BackendTymux case must never call (or block on) tymux.EnsureDaemonRunning —
// that check moved into TymuxBackend.Start()/RestoreWithWorkDir(), which only
// ever runs inside the async goroutine session_service.go's CreateSession
// already uses, never on the synchronous construction path. Proven here by
// stubbing backend_tymux.go's ensureDaemonRunningFn seam to block
// indefinitely (simulating a wedged/cold tymuxd) and asserting
// NewProcessManager still returns promptly and successfully — if
// NewProcessManager regressed to calling that seam at construction time,
// this test would hang until its own timeout and fail.
func TestNewProcessManager_should_ReturnImmediately_When_BackendIsTymuxRegardlessOfDaemonState(t *testing.T) {
	RegisterBackendProvider(BackendTmux)
	defer RegisterBackendProvider(BackendTmux)

	blockForever := make(chan struct{})
	restore := stubEnsureDaemonRunning(func(context.Context, tymux.DaemonConfig) (tymux.TymuxdReady, error) {
		<-blockForever
		return tymux.TymuxdReady{}, nil
	})
	defer restore()

	done := make(chan struct{})
	var pm ProcessManager
	var err error
	go func() {
		pm, err = NewProcessManager(context.Background(), BackendTmux, ProcessManagerOptions{Backend: BackendTymux})
		close(done)
	}()

	timedOut := false
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		timedOut = true
	}

	// Unblock the stub and join the spawned goroutine unconditionally,
	// before this test function returns via any path (including the
	// t.Fatal below): a select-timeout branch that returned without
	// joining first left the goroutine racing later tests over shared
	// package globals (ensureDaemonRunningFn, the backend registry) -- a
	// genuine, reproduced -race failure caught in code review. Root-caused
	// and fixed here rather than re-excused as a rare flake (see
	// .claude/rules/fix-flaky-tests-dont-defer.md).
	close(blockForever)
	<-done

	if timedOut {
		t.Fatal("NewProcessManager blocked for BackendTymux -- daemon check must not run at construction time (ADR-004)")
	}

	require.NoError(t, err)
	_, ok := pm.(*TymuxBackend)
	assert.True(t, ok, "expected *TymuxBackend, got %T", pm)
}
