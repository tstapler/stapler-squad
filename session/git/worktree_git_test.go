package git

import (
	"os/exec"
	"testing"
	"time"
)

// raceSimulatorExecutor implements executor.Executor for testing the double-checked
// locking invariant in IsDirtyWithHint.  When CombinedOutput is called it runs
// raceSetup first (simulating a concurrent goroutine updating the cache), then
// returns the configured output.
type raceSimulatorExecutor struct {
	output    []byte
	raceSetup func()
}

func (e *raceSimulatorExecutor) Run(_ *exec.Cmd) error              { return nil }
func (e *raceSimulatorExecutor) Output(_ *exec.Cmd) ([]byte, error) { return e.output, nil }
func (e *raceSimulatorExecutor) CombinedOutput(_ *exec.Cmd) ([]byte, error) {
	if e.raceSetup != nil {
		e.raceSetup()
	}
	return e.output, nil
}

// TestIsDirtyWithHint_ReturnsLocallyComputedValue_WhenCacheIsWrittenByRacingGoroutine
// verifies the return-own-observation invariant: IsDirtyWithHint must return the
// locally-computed value, not a re-read of the cache slot.
//
// With atomic.Value the write is unconditional, so a race can't suppress our Store.
// The test simulates a racing goroutine that stores false into the cache WHILE our
// git subprocess is running; our code must still return true (its own observation).
func TestIsDirtyWithHint_ReturnsLocallyComputedValue_WhenCacheIsWrittenByRacingGoroutine(t *testing.T) {
	mock := &raceSimulatorExecutor{
		output: []byte("M file.txt\n"), // our goroutine sees the worktree as dirty
	}

	g := NewGitWorktreeFromStorageWithExecutor(
		"/fake/repo", "/fake/worktree", "test-session", "test-branch", "", mock,
	)

	// The raceSetup closure runs inside CombinedOutput, simulating a concurrent
	// goroutine that stores dirty=false while our git call is "in flight".
	mock.raceSetup = func() {
		g.isDirtyCache.Store(dirtyCacheState{dirty: false, time: time.Now()})
	}

	// Start with an invalid cache so IsDirtyWithHint takes the slow (git) path.
	g.isDirtyCache.Store(dirtyCacheState{}) // zero time = cache invalid

	got, err := g.IsDirtyWithHint(false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// We computed dirty=true. The racing goroutine stored false. The invariant:
	// we must return our own observation (true), overwriting the racing store.
	if !got {
		t.Errorf("IsDirtyWithHint = false; want true (locally computed value)")
	}
}
