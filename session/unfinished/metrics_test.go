package unfinished

import (
	"context"
	"testing"
)

// TestRegisterMetrics_NoError is a smoke check that instrument registration
// against the OTel global (no-op, in tests) meter succeeds — the one thing
// that would break silently if an instrument name were invalid or an
// observable gauge were registered with a description/unit OTel rejects.
func TestRegisterMetrics_NoError(t *testing.T) {
	t.Parallel()
	if err := RegisterMetrics(); err != nil {
		t.Fatalf("RegisterMetrics: %v", err)
	}
}

// The worker must count one cycle per task, including a task whose repo has
// no scannable worktrees (the common idle case): the counter is a denominator
// for "scanner CPU per scan", so skipped-empty cycles still count.
func TestScannerWorker_ShouldCountOneCyclePerTask(t *testing.T) {
	// Not parallel: asserts a delta on a process-global counter other
	// scanner tests also bump, so it checks ">=" rather than "==".
	s := NewScannerWithReader(nil, nil, &GoGitVCSReader{})
	repo := t.TempDir()
	s.inFlight.Store(repo, struct{}{})
	s.scanQueue <- scanTask{repoPath: repo}
	close(s.scanQueue) // worker drains the one task, then returns: no goroutine, no sleep

	before := ScanCyclesTotal()
	s.worker(context.Background())

	if got := ScanCyclesTotal(); got < before+1 {
		t.Fatalf("ScanCyclesTotal = %d, want >= %d after one worker task", got, before+1)
	}
	if _, stillInFlight := s.inFlight.Load(repo); stillInFlight {
		t.Fatal("worker left the repo marked in-flight")
	}
}
