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

func (e *raceSimulatorExecutor) Run(_ *exec.Cmd) error                    { return nil }
func (e *raceSimulatorExecutor) Output(_ *exec.Cmd) ([]byte, error)       { return e.output, nil }
func (e *raceSimulatorExecutor) CombinedOutput(_ *exec.Cmd) ([]byte, error) {
	if e.raceSetup != nil {
		e.raceSetup()
	}
	return e.output, nil
}

// TestIsDirtyWithHint_ReturnsLocallyComputedValue_WhenCacheIsWrittenByRacingGoroutine
// verifies the double-checked locking invariant: IsDirtyWithHint must return the
// locally-computed value (dirty), not the shared cache slot (g.isDirtyCache).
//
// Reproduces the pre-fix bug: if a concurrent goroutine wins the write lock and
// stores a different value between our git call and our write-lock acquisition,
// the pre-fix code returned the racing goroutine's value instead of ours.
//
// Pre-fix behaviour: returns false (racing goroutine's cached value) — test FAILS.
// Post-fix behaviour: returns true (our locally-computed value)      — test PASSES.
func TestIsDirtyWithHint_ReturnsLocallyComputedValue_WhenCacheIsWrittenByRacingGoroutine(t *testing.T) {
	mock := &raceSimulatorExecutor{
		output: []byte("M file.txt\n"), // our goroutine sees the worktree as dirty
	}

	g := NewGitWorktreeFromStorageWithExecutor(
		"/fake/repo", "/fake/worktree", "test-session", "test-branch", "", mock,
	)

	// The raceSetup closure runs inside CombinedOutput, simulating a concurrent
	// goroutine that wins the write lock while our call is "in flight".
	// It stores dirty=false with a fresh timestamp, making the cache appear valid.
	mock.raceSetup = func() {
		g.isDirtyCacheMu.Lock()
		g.isDirtyCache = false         // racing goroutine observed: not dirty
		g.isDirtyCacheTime = time.Now() // marks cache fresh — causes our write to be skipped
		g.isDirtyCacheMu.Unlock()
	}

	// Start with a stale cache so IsDirtyWithHint takes the slow (git) path.
	g.isDirtyCacheMu.Lock()
	g.isDirtyCacheTime = time.Time{}
	g.isDirtyCacheMu.Unlock()

	got, err := g.IsDirtyWithHint(false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// We computed dirty=true from the git output.  The racing goroutine stored
	// false. The invariant: we must return our own observation (true).
	if !got {
		t.Errorf("IsDirtyWithHint = false; want true (locally computed value, not racing goroutine's cached value)")
	}
}
