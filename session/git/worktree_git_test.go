package git

import (
	"os/exec"
	"strings"
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

// TestParsePRStatusPayload_ConflictDetection is a table-driven test over the
// documented mergeable/mergeStateStatus enum combinations (plan.md Story 1.3.1),
// proving the HasConflicts OR condition is correct for both the trigger cases
// and the near-miss cases that must NOT trigger.
func TestParsePRStatusPayload_ConflictDetection(t *testing.T) {
	tests := []struct {
		name             string
		mergeable        string
		mergeStateStatus string
		wantHasConflicts bool
	}{
		{
			name:             "MERGEABLE/CLEAN is healthy, no conflict",
			mergeable:        "MERGEABLE",
			mergeStateStatus: "CLEAN",
			wantHasConflicts: false,
		},
		{
			name:             "CONFLICTING/DIRTY is a conflict",
			mergeable:        "CONFLICTING",
			mergeStateStatus: "DIRTY",
			wantHasConflicts: true,
		},
		{
			name:             "CONFLICTING/BLOCKED is a conflict (mergeable is authoritative)",
			mergeable:        "CONFLICTING",
			mergeStateStatus: "BLOCKED",
			wantHasConflicts: true,
		},
		{
			name:             "UNKNOWN/UNKNOWN is transient, no signal",
			mergeable:        "UNKNOWN",
			mergeStateStatus: "UNKNOWN",
			wantHasConflicts: false,
		},
		{
			name:             "MERGEABLE/BLOCKED is a review/check gate, not a conflict",
			mergeable:        "MERGEABLE",
			mergeStateStatus: "BLOCKED",
			wantHasConflicts: false,
		},
		{
			name:             "MERGEABLE/BEHIND is behind base, not a conflict",
			mergeable:        "MERGEABLE",
			mergeStateStatus: "BEHIND",
			wantHasConflicts: false,
		},
		{
			name:             "MERGEABLE/DIRTY is a conflict (cli/cli#9583 stale-mergeable scenario)",
			mergeable:        "MERGEABLE",
			mergeStateStatus: "DIRTY",
			wantHasConflicts: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := []byte(`{"statusCheckRollup":[],"reviews":[],"comments":[],"mergeable":"` +
				tt.mergeable + `","mergeStateStatus":"` + tt.mergeStateStatus + `"}`)

			status, err := parsePRStatusPayload(raw)
			if err != nil {
				t.Fatalf("parsePRStatusPayload() error = %v", err)
			}

			if status.HasConflicts != tt.wantHasConflicts {
				t.Errorf("HasConflicts = %v; want %v", status.HasConflicts, tt.wantHasConflicts)
			}

			if tt.wantHasConflicts {
				if !strings.Contains(status.FeedbackText, "## Merge conflict") {
					t.Errorf("FeedbackText = %q; want it to contain %q", status.FeedbackText, "## Merge conflict")
				}
			} else {
				if strings.Contains(status.FeedbackText, "## Merge conflict") {
					t.Errorf("FeedbackText = %q; want no %q section", status.FeedbackText, "## Merge conflict")
				}
			}
		})
	}
}

// TestParsePRStatusPayload_ConflictGuidanceText proves Story 1.2.1's guidance
// text (--force-with-lease, .gitignore suspicion, leave-markers-and-stop,
// mandatory git diff --stat) renders into FeedbackText's conflict section, and
// that it renders identically regardless of which field (mergeable vs.
// mergeStateStatus) tripped the HasConflicts OR condition.
func TestParsePRStatusPayload_ConflictGuidanceText(t *testing.T) {
	wantSubstrings := []string{
		"--force-with-lease",
		".gitignore",
		"leave the conflict markers",
		"git diff --stat",
	}

	t.Run("CONFLICTING/DIRTY renders guidance text", func(t *testing.T) {
		raw := []byte(`{"statusCheckRollup":[],"reviews":[],"comments":[],"mergeable":"CONFLICTING","mergeStateStatus":"DIRTY"}`)

		status, err := parsePRStatusPayload(raw)
		if err != nil {
			t.Fatalf("parsePRStatusPayload() error = %v", err)
		}

		for _, want := range wantSubstrings {
			if !strings.Contains(status.FeedbackText, want) {
				t.Errorf("FeedbackText missing %q; got %q", want, status.FeedbackText)
			}
		}
	})

	t.Run("MERGEABLE/DIRTY (cli/cli#9583 stale-mergeable case) renders identical guidance text", func(t *testing.T) {
		raw := []byte(`{"statusCheckRollup":[],"reviews":[],"comments":[],"mergeable":"MERGEABLE","mergeStateStatus":"DIRTY"}`)

		status, err := parsePRStatusPayload(raw)
		if err != nil {
			t.Fatalf("parsePRStatusPayload() error = %v", err)
		}

		for _, want := range wantSubstrings {
			if !strings.Contains(status.FeedbackText, want) {
				t.Errorf("FeedbackText missing %q; got %q", want, status.FeedbackText)
			}
		}
	})
}

// TestParsePRStatusPayload_CIFailing covers the pre-existing, previously
// untested CIFailing detection logic: a terminal FAILURE conclusion must set
// CIFailing=true, while a non-terminal IN_PROGRESS check must not.
func TestParsePRStatusPayload_CIFailing(t *testing.T) {
	t.Run("terminal FAILURE conclusion sets CIFailing true", func(t *testing.T) {
		raw := []byte(`{"statusCheckRollup":[{"name":"build","conclusion":"FAILURE"}],"reviews":[],"comments":[],"mergeable":"MERGEABLE","mergeStateStatus":"CLEAN"}`)

		status, err := parsePRStatusPayload(raw)
		if err != nil {
			t.Fatalf("parsePRStatusPayload() error = %v", err)
		}

		if !status.CIFailing {
			t.Errorf("CIFailing = false; want true")
		}
		if !strings.Contains(status.FeedbackText, "## Failing CI checks") {
			t.Errorf("FeedbackText missing %q; got %q", "## Failing CI checks", status.FeedbackText)
		}
		if !strings.Contains(status.FeedbackText, "build FAILED") {
			t.Errorf("FeedbackText missing %q; got %q", "build FAILED", status.FeedbackText)
		}
	})

	t.Run("non-terminal IN_PROGRESS leaves CIFailing false", func(t *testing.T) {
		raw := []byte(`{"statusCheckRollup":[{"name":"build","status":"IN_PROGRESS"}],"reviews":[],"comments":[],"mergeable":"MERGEABLE","mergeStateStatus":"CLEAN"}`)

		status, err := parsePRStatusPayload(raw)
		if err != nil {
			t.Fatalf("parsePRStatusPayload() error = %v", err)
		}

		if status.CIFailing {
			t.Errorf("CIFailing = true; want false")
		}
		if strings.Contains(status.FeedbackText, "## Failing CI checks") {
			t.Errorf("FeedbackText = %q; want no %q section", status.FeedbackText, "## Failing CI checks")
		}
	})
}

// TestParsePRStatusPayload_HasBlockingReviews covers the pre-existing,
// previously untested HasBlockingReviews detection logic: a CHANGES_REQUESTED
// review must set HasBlockingReviews=true, while an APPROVED-only review set
// must not.
func TestParsePRStatusPayload_HasBlockingReviews(t *testing.T) {
	t.Run("CHANGES_REQUESTED review sets HasBlockingReviews true", func(t *testing.T) {
		raw := []byte(`{"statusCheckRollup":[],"reviews":[{"state":"CHANGES_REQUESTED","body":"Fix the null check","author":{"login":"reviewer1"}}],"comments":[],"mergeable":"MERGEABLE","mergeStateStatus":"CLEAN"}`)

		status, err := parsePRStatusPayload(raw)
		if err != nil {
			t.Fatalf("parsePRStatusPayload() error = %v", err)
		}

		if !status.HasBlockingReviews {
			t.Errorf("HasBlockingReviews = false; want true")
		}
		if !strings.Contains(status.FeedbackText, "## Review: changes requested by @reviewer1") {
			t.Errorf("FeedbackText missing %q; got %q", "## Review: changes requested by @reviewer1", status.FeedbackText)
		}
		if !strings.Contains(status.FeedbackText, "Fix the null check") {
			t.Errorf("FeedbackText missing %q; got %q", "Fix the null check", status.FeedbackText)
		}
	})

	t.Run("APPROVED-only reviews leave HasBlockingReviews false", func(t *testing.T) {
		raw := []byte(`{"statusCheckRollup":[],"reviews":[{"state":"APPROVED","body":"LGTM","author":{"login":"reviewer1"}}],"comments":[],"mergeable":"MERGEABLE","mergeStateStatus":"CLEAN"}`)

		status, err := parsePRStatusPayload(raw)
		if err != nil {
			t.Fatalf("parsePRStatusPayload() error = %v", err)
		}

		if status.HasBlockingReviews {
			t.Errorf("HasBlockingReviews = true; want false")
		}
		if strings.Contains(status.FeedbackText, "## Review:") {
			t.Errorf("FeedbackText = %q; want no %q section", status.FeedbackText, "## Review:")
		}
	})
}

// TestParsePRStatusPayload_ConflictSectionOrderedFirst proves the "conflict
// section is always ordered first" design decision (features.md §2A): when a
// PR has both HasConflicts and CIFailing true, the conflict section must
// precede the CI section in FeedbackText.
func TestParsePRStatusPayload_ConflictSectionOrderedFirst(t *testing.T) {
	raw := []byte(`{"statusCheckRollup":[{"name":"build","conclusion":"FAILURE"}],"reviews":[],"comments":[],"mergeable":"CONFLICTING","mergeStateStatus":"DIRTY"}`)

	status, err := parsePRStatusPayload(raw)
	if err != nil {
		t.Fatalf("parsePRStatusPayload() error = %v", err)
	}

	conflictIdx := strings.Index(status.FeedbackText, "## Merge conflict")
	ciIdx := strings.Index(status.FeedbackText, "## Failing CI checks")

	if conflictIdx == -1 {
		t.Fatalf("FeedbackText missing %q section; got %q", "## Merge conflict", status.FeedbackText)
	}
	if ciIdx == -1 {
		t.Fatalf("FeedbackText missing %q section; got %q", "## Failing CI checks", status.FeedbackText)
	}
	if conflictIdx >= ciIdx {
		t.Errorf("conflict section index %d not before CI section index %d in FeedbackText: %q", conflictIdx, ciIdx, status.FeedbackText)
	}
}
