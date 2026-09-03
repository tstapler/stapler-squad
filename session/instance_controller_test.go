package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
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

// writeFakePiBinary writes an executable script named exactly "pi" (so
// isPi(i.Program) resolves true) that records its own PID to pidLogFile
// (via the inherited PID_LOG_FILE env var, set by the caller) before
// sleeping, so a test can count how many real OS subprocesses a
// StartController race actually spawned.
func writeFakePiBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "pi")
	script := "#!/bin/sh\necho $$ >> \"$PID_LOG_FILE\"\nexec sleep 100\n"
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	return path
}

// TestStartController_PiStatusSourceNoDoubleStart is the regression guard
// for Blocker 2 (independent review pass on PR #685): startPiStatusSource's
// check-then-act on i.piStatusSrc.Load() used to have no lock, so two
// concurrent StartController() calls on the same pi-backed instance could
// both observe a nil piStatusSrc, both spawn a subprocess + reader/wait
// goroutine pair via src.Start(), and have the loser's *PiStatusSource
// silently discarded by the winner's Store() -- its subprocess and
// goroutines never Stop()-ed, leaking for the life of the process.
//
// This proves the fix (piStatusStartMu held across the whole check-then-act
// sequence) by actually counting real OS subprocesses spawned by a fake
// "pi" binary that records its own PID on launch: with the fix in place,
// racing two StartController() calls can only ever result in exactly one
// subprocess -- there is no "second, unlinked" subprocess to separately
// Stop(), which is a stronger guarantee than merely stopping a leak after
// the fact. Runs many iterations so the goroutine scheduler gets repeated
// chances at both possible orderings.
func TestStartController_PiStatusSourceNoDoubleStart(t *testing.T) {
	origFlag := config.LoadConfig().GetFeatureFlag(config.FeaturePiSupport)
	require.NoError(t, config.LoadConfig().SetFeatureFlag(config.FeaturePiSupport, true))
	defer func() {
		_ = config.LoadConfig().SetFeatureFlag(config.FeaturePiSupport, origFlag)
	}()

	const iterations = 20
	for iter := 0; iter < iterations; iter++ {
		iter := iter
		t.Run(fmt.Sprintf("iter-%d", iter), func(t *testing.T) {
			fakePi := writeFakePiBinary(t)
			pidLogFile := filepath.Join(t.TempDir(), "pids.log")
			require.NoError(t, os.Setenv("PID_LOG_FILE", pidLogFile))
			defer func() { _ = os.Unsetenv("PID_LOG_FILE") }()

			inst := &Instance{Title: fmt.Sprintf("pi-start-race-%d", iter), Program: fakePi}
			inst.SetStatusManager(NewInstanceStatusManager())
			inst.started.Store(true)
			require.True(t, inst.piStatusSupported(), "test setup: piStatusSupported() must be true to exercise the race")

			var wg sync.WaitGroup
			wg.Add(2)
			errs := make(chan error, 2)
			for range 2 {
				go func() {
					defer wg.Done()
					errs <- inst.StartController()
				}()
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				assert.NoError(t, err, "StartController() should not error")
			}

			// Give the (at most one) spawned subprocess a moment to record its
			// PID before counting. 5s (not 2s) margin for shared-machine load
			// in this repo's dev/CI environments (see
			// docs/how-to's timing-budget precedent for the same reasoning).
			deadline := time.Now().Add(5 * time.Second)
			var pidLines []string
			for time.Now().Before(deadline) {
				data, readErr := os.ReadFile(pidLogFile)
				if readErr == nil && len(strings.TrimSpace(string(data))) > 0 {
					pidLines = strings.Fields(strings.TrimSpace(string(data)))
					break
				}
				time.Sleep(20 * time.Millisecond)
			}

			assert.Len(t, pidLines, 1, "exactly one real subprocess should have been spawned across both concurrent StartController() calls, got PIDs: %v", pidLines)
			assert.NotNil(t, inst.piStatusSrc.Load(), "a PiStatusSource must be registered after StartController()")

			// Cleanup: stop the registered source, then forcibly kill any PID
			// this iteration recorded (belt-and-suspenders in case the
			// assertion above already failed and left an extra process
			// running).
			inst.StopController()
			for _, pidStr := range pidLines {
				var pid int
				if _, scanErr := fmt.Sscanf(pidStr, "%d", &pid); scanErr == nil {
					if proc, findErr := os.FindProcess(pid); findErr == nil {
						_ = proc.Kill()
					}
				}
			}
		})
	}
}
