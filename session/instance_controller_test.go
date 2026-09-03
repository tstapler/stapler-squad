package session

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStartController_DoesNotRaceWithDestroy is the regression guard for the
// TOCTOU-closing i.destroyed.Load() guards in StartController (see the doc
// comments at their call sites in instance_controller.go): CreateSession's
// async init goroutine can call StartController well after the RPC returned,
// racing a fast-following DeleteSession's Destroy(). Destroy() sets
// i.destroyed before doing anything else, so StartController's guards must
// observe that and refuse to register a controller no one is left to stop.
//
// Runs many iterations with fresh instances so -race and Go's scheduler have
// repeated chances to interleave StartController's pre-registration check
// with Destroy()'s i.destroyed.Store(true), rather than relying on a single
// lucky (or unlucky) execution to hit the narrow window.
func TestStartController_DoesNotRaceWithDestroy(t *testing.T) {
	t.Parallel()

	const iterations = 50
	for i := 0; i < iterations; i++ {
		inst := newControllerRaceTestInstance(t, "controller-destroy-race-test")

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = inst.StartController()
		}()
		go func() {
			defer wg.Done()
			_ = inst.Destroy()
		}()
		wg.Wait()

		// Whichever goroutine "won", the instance must end up destroyed with
		// no controller left registered -- a controller surviving a completed
		// Destroy() call is exactly the leak (nothing left to stop it) the
		// guards exist to prevent.
		assert.True(t, inst.destroyed.Load(), "instance must be marked destroyed after Destroy() returns")
		assert.False(t, inst.controllerManager.HasController(), "no controller should remain registered once Destroy() has completed")
	}
}

// newControllerRaceTestInstance builds a minimal Instance suitable for
// exercising StartController/Destroy concurrently: a status manager is wired
// so StartController's "no status manager" short-circuit doesn't mask the
// destroyed-guard check, and started is set so StartController proceeds past
// its "not started yet" short-circuit. All other subsystems (tmux session,
// git worktree, VNC/CDP) are left unset; the destroy chain's KillSession and
// CleanupWorktree calls no-op safely against a bare Instance with no real
// backing tmux session or worktree.
func newControllerRaceTestInstance(t *testing.T, title string) *Instance {
	t.Helper()
	inst := &Instance{Title: title}
	inst.SetStatusManager(NewInstanceStatusManager())
	inst.started.Store(true)
	return inst
}

// TestStartController_RefusesAfterDestroy is the sequential (non-racy)
// companion to TestStartController_DoesNotRaceWithDestroy: proves the guard's
// logic in isolation -- once destroyed is set, StartController must not
// register a controller, deterministically and without depending on
// goroutine interleaving.
func TestStartController_RefusesAfterDestroy(t *testing.T) {
	t.Parallel()

	inst := newControllerRaceTestInstance(t, "controller-refuses-after-destroy-test")
	inst.destroyed.Store(true)

	err := inst.StartController()

	require.NoError(t, err)
	assert.False(t, inst.controllerManager.HasController(), "StartController must not register a controller once the instance is destroyed")
}

// TestStopController_StopsLivePiStatusSourceRegardlessOfProgramOrFlag is the
// regression guard for Bug 2 (architecture + idiom review, BLOCKER): routing
// in StopController/stopControllerLocked used to re-evaluate
// piStatusSupported() -- isPi(i.Program) && the live pi-support feature
// flag -- instead of checking whether a PiStatusSource was actually
// registered. That meant a mid-flight flag disable (or, as exercised here, a
// program value that no longer resolves via isPi) could route to the
// Claude-controller branch and skip stopPiStatusSource entirely, leaking the
// still-running PiStatusSource's subprocess/goroutines and breaking
// SetPiSessionID's documented invariant that Restart's i.piSession access is
// safe because Stop() already joined the only writer goroutine.
//
// This test registers a real, running PiStatusSource directly on
// i.piStatusSrc (bypassing StartController/piStatusSupported entirely, so
// the test result cannot depend on the current value of isPi(i.Program) or
// the feature flag) and asserts StopController() still calls Stop() on it --
// observed via the subprocess actually being killed and the reader/wait
// goroutines joining (src.Stop() returning, which blocks on p.wg.Wait()).
func TestStopController_StopsLivePiStatusSourceRegardlessOfProgramOrFlag(t *testing.T) {
	t.Parallel()

	inst := &Instance{Title: "stop-controller-live-pi-source-test", Program: "not-pi-and-flag-irrelevant"}
	inst.SetStatusManager(NewInstanceStatusManager())

	src := NewPiStatusSource(inst.Title, sleeperCmd())
	require.NoError(t, src.Start())
	inst.piStatusSrc.Store(src)

	// Sanity check: piStatusSupported() is false here (Program doesn't
	// resolve via isPi), which is exactly the condition that used to make
	// StopController skip stopPiStatusSource.
	require.False(t, inst.piStatusSupported(), "test setup: piStatusSupported() must be false to exercise the Bug 2 routing gap")

	inst.StopController()

	assert.Nil(t, inst.piStatusSrc.Load(), "StopController must clear the live PiStatusSource even when piStatusSupported() is false")

	// src.Stop() is idempotent and blocks on p.wg.Wait(); calling it again
	// here should return immediately (already stopped) rather than hang,
	// confirming the reader/wait goroutines were actually joined by
	// StopController's call, not left running.
	done := make(chan struct{})
	go func() {
		src.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("src.Stop() did not return promptly -- PiStatusSource's goroutines were not actually stopped by StopController")
	}
}
