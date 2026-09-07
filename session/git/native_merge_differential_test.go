package git

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// DifferentialMergeHarness runs identical merge inputs through real `git merge` (the
// oracle, via subprocess) and this package's native pipeline
// (nativeMergeMainIntoWorktree), then diffs the results at the tree-hash and
// index-stage level — the hard correctness gate build-vs-buy.md §3 requires before
// either native feature flag's global default can flip on (Story 5.1.1).
//
// Run covers the clean/fast-forward comparison (its only conflict-handling is detecting
// a *disagreement* about whether a conflict occurred at all). Byte-level conflict-marker
// comparison is exercised separately via compareConflictMarkerBytes: the real pipeline
// (materializeConflictAndAbort, native_merge.go) always reverts a conflicted worktree
// back to clean immediately after rendering markers ("materialize, then abort" — see its
// doc comment), so there is no persisted on-disk conflict state for Run's two-repo
// subprocess comparison to inspect. assembleConflictedFileContent (also in
// native_merge.go) is the exact same unexported function that pipeline calls to render
// that transient content, so calling it directly here exercises real production
// rendering logic, not a re-implementation.
type DifferentialMergeHarness struct {
	t *testing.T
}

// NewDifferentialMergeHarness constructs a harness bound to t.
func NewDifferentialMergeHarness(t *testing.T) *DifferentialMergeHarness {
	t.Helper()
	return &DifferentialMergeHarness{t: t}
}

// DifferentialScenario describes one merge scenario, applied identically to both the
// real-git oracle and the native pipeline: Base is the common ancestor's file content by
// path, Ours is the checked-out branch's content (on top of Base), Theirs is the
// incoming branch's content (on top of Base, matching origin/mainBranch in
// nativeMergeMainIntoWorktree's own terms). An empty map for Ours or Theirs means that
// side makes no additional commit past Base.
type DifferentialScenario struct {
	Base, Ours, Theirs map[string]string
}

// DifferentialResult reports whether the native pipeline's output matched the real-git
// oracle's, and carries a human-readable diff when it did not.
type DifferentialResult struct {
	TreeMatch           bool
	IndexStageMatch     bool
	ConflictMarkerMatch bool
	Diff                string
}

// scenarioRepoPair is one scenario materialized as a real origin repo (Base then Theirs
// commits on "main") plus a clone forked at Base with an additional Ours commit on
// branch "feature" — the exact repo shape nativeMergeMainIntoWorktree's own tests
// already build by hand (TestNativeMergeMainIntoWorktree_CleanThreeWayMerge/
// _FastForward), built here via real git subprocess commands so an oracle pair and a
// native pair are byte-identical in content even though they're separate directories.
type scenarioRepoPair struct {
	originDir string
	workDir   string
}

// buildScenarioRepoPair implements Task 5.1.1a's real-git side: `git init`/commit
// sequence via safeexec.CommandContext (this package's existing test convention, see
// runGit in ops_test.go and runRealGit in native_worktree_add_test.go).
func buildScenarioRepoPair(t *testing.T, scenario DifferentialScenario) scenarioRepoPair {
	t.Helper()

	originDir := t.TempDir()
	runGit(t, originDir, "init", "-b", "main")
	runGit(t, originDir, "config", "user.email", "test@example.com")
	runGit(t, originDir, "config", "user.name", "Test User")
	writeScenarioFiles(t, originDir, scenario.Base)
	runGit(t, originDir, "add", ".")
	runGit(t, originDir, "commit", "-m", "base")

	workParent := t.TempDir()
	workDir := filepath.Join(workParent, "work")
	runGit(t, workParent, "clone", originDir, workDir)
	runGit(t, workDir, "config", "user.email", "test@example.com")
	runGit(t, workDir, "config", "user.name", "Test User")
	runGit(t, workDir, "checkout", "-b", "feature")
	if len(scenario.Ours) > 0 {
		writeScenarioFiles(t, workDir, scenario.Ours)
		runGit(t, workDir, "add", ".")
		runGit(t, workDir, "commit", "-m", "ours")
	}

	if len(scenario.Theirs) > 0 {
		writeScenarioFiles(t, originDir, scenario.Theirs)
		runGit(t, originDir, "add", ".")
		runGit(t, originDir, "commit", "-m", "theirs")
	}

	return scenarioRepoPair{originDir: originDir, workDir: workDir}
}

// writeScenarioFiles writes files (relative-path -> content) into dir, creating parent
// directories as needed.
func writeScenarioFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for relPath, content := range files {
		full := filepath.Join(dir, relPath)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	}
}

// Run executes scenario through the real-git oracle (a fresh scenarioRepoPair, merged
// via a real `git merge --no-edit origin/main` subprocess) and the native pipeline
// (a second, independently-built scenarioRepoPair, merged via
// nativeMergeMainIntoWorktree), then compares tree hash and index-stage output — Task
// 5.1.1b's comparison logic, for the clean/fast-forward case.
func (h *DifferentialMergeHarness) Run(scenario DifferentialScenario) DifferentialResult {
	t := h.t
	t.Helper()

	oracle := buildScenarioRepoPair(t, scenario)
	oracleConflicted := runOracleMerge(t, oracle)

	native := buildScenarioRepoPair(t, scenario)
	nativeResult, err := nativeMergeMainIntoWorktree(native.workDir, "main")
	require.NoError(t, err)
	nativeConflicted := nativeResult.Conflicted

	if oracleConflicted != nativeConflicted {
		return DifferentialResult{
			Diff: fmt.Sprintf("conflict-detection mismatch: oracle conflicted=%v, native conflicted=%v", oracleConflicted, nativeConflicted),
		}
	}
	if oracleConflicted {
		// Both sides agree a conflict occurred, but Run's two-repo subprocess comparison
		// can't inspect native's transient marker content (see this type's doc comment) —
		// use compareConflictMarkerBytes directly for that.
		return DifferentialResult{
			ConflictMarkerMatch: false,
			Diff:                "Run does not compare conflicted scenarios byte-for-byte; use compareConflictMarkerBytes",
		}
	}

	return compareCleanMergeOutcome(t, oracle, native)
}

// runOracleMerge runs `git merge --no-edit origin/main` against pair.workDir (already
// fetched from its origin), reporting whether the merge left conflicts rather than
// failing the test on a non-zero exit — a real conflict is an expected outcome here, not
// a harness error.
func runOracleMerge(t *testing.T, pair scenarioRepoPair) (conflicted bool) {
	t.Helper()
	runGit(t, pair.workDir, "fetch", "origin")
	mergeCmd := safeexec.CommandContext(context.Background(), "git", "merge", "--no-edit", "origin/main")
	mergeCmd.Dir = pair.workDir
	_, mergeErr := mergeCmd.CombinedOutput()
	return mergeErr != nil
}

// compareCleanMergeOutcome compares oracle's and native's resulting tree hash and
// `git ls-files --stage` output — Task 5.1.1b's comparison logic for the no-conflict
// case, factored out of Run to keep it under this repo's function-length lint.
func compareCleanMergeOutcome(t *testing.T, oracle, native scenarioRepoPair) DifferentialResult {
	t.Helper()
	oracleTree := strings.TrimSpace(runGit(t, oracle.workDir, "rev-parse", "HEAD^{tree}"))
	nativeTree := strings.TrimSpace(runGit(t, native.workDir, "rev-parse", "HEAD^{tree}"))
	oracleStage := runGit(t, oracle.workDir, "ls-files", "--stage")
	nativeStage := runGit(t, native.workDir, "ls-files", "--stage")

	result := DifferentialResult{
		TreeMatch:           oracleTree == nativeTree,
		IndexStageMatch:     oracleStage == nativeStage,
		ConflictMarkerMatch: true, // trivially true: nothing conflicted
	}
	if !result.TreeMatch || !result.IndexStageMatch {
		result.Diff = fmt.Sprintf(
			"oracle tree=%s native tree=%s\n--- oracle ls-files --stage ---\n%s--- native ls-files --stage ---\n%s",
			oracleTree, nativeTree, oracleStage, nativeStage,
		)
	}
	return result
}

// compareConflictMarkerBytes is the harness's byte-level conflict-marker comparison
// (Task 5.1.1b's third axis). It exists as a standalone function, not a method reachable
// only via Run, precisely so TestDifferentialMergeHarness_DetectsInjectedMarkerMismatch
// below can feed it a real production marker rendering alongside a deliberately
// corrupted one and prove the comparison itself is sound.
func compareConflictMarkerBytes(oracleContent, nativeContent []byte) DifferentialResult {
	match := bytes.Equal(oracleContent, nativeContent)
	result := DifferentialResult{ConflictMarkerMatch: match}
	if !match {
		result.Diff = fmt.Sprintf("conflict marker byte mismatch:\n--- oracle ---\n%s\n--- native ---\n%s", oracleContent, nativeContent)
	}
	return result
}

// TestDifferentialMergeHarness_ReportsMatch_When_BothImplementationsAgree covers Story
// 5.1.1's first acceptance criterion: for a fast-forward scenario, the harness reports
// TreeMatch and IndexStageMatch true (trivially, no conflict) and returns no diff.
func TestDifferentialMergeHarness_ReportsMatch_When_BothImplementationsAgree(t *testing.T) {
	t.Parallel()

	h := NewDifferentialMergeHarness(t)
	result := h.Run(DifferentialScenario{
		Base:   map[string]string{"a.txt": "line1\n"},
		Theirs: map[string]string{"main-fix.txt": "fix on main\n"},
	})

	assert.True(t, result.TreeMatch, "diff: %s", result.Diff)
	assert.True(t, result.IndexStageMatch, "diff: %s", result.Diff)
	assert.True(t, result.ConflictMarkerMatch)
	assert.Empty(t, result.Diff)
}

// TestDifferentialMergeHarness_DetectsInjectedMarkerMismatch covers Story 5.1.1's second
// acceptance criterion (the harness self-test): given a conflict scenario where the
// native side's rendered marker is corrupted to a 9-character marker instead of git's
// real 7-character "<<<<<<<"/">>>>>>>", the harness must report ConflictMarkerMatch:
// false with the exact byte diff.
//
// renderConflictHunk (native_merge_conflict.go) is a plain package-level function, not a
// func var — plan.md's Story 5.1.1 asked to check for exactly that seam before adding
// one, and this epic's scope is test-infrastructure only (no production-file changes).
// So instead of monkey-patching renderConflictHunk itself, this test calls the real,
// unmodified assembleConflictedFileContent/renderConflictHunk pipeline to get the
// correct marker bytes, then derives the "corrupted" side by string-replacing the
// 7-character markers with 9-character ones — reproducing exactly the byte difference a
// monkey-patched renderConflictHunk would have produced, without touching production
// code.
func TestDifferentialMergeHarness_DetectsInjectedMarkerMismatch(t *testing.T) {
	t.Parallel()

	hunk := MergeHunk{Kind: RegionConflict, Ours: []string{"ours line"}, Theirs: []string{"theirs line"}}
	correct, err := assembleConflictedFileContent([]MergeHunk{hunk}, "HEAD", "origin/main")
	require.NoError(t, err)
	require.Contains(t, correct, "<<<<<<< HEAD")
	require.Contains(t, correct, ">>>>>>> origin/main")

	corrupted := strings.NewReplacer(
		"<<<<<<< ", "<<<<<<<<< ",
		">>>>>>> ", ">>>>>>>>> ",
	).Replace(correct)
	require.NotEqual(t, correct, corrupted, "test setup: corruption must actually change the content")

	result := compareConflictMarkerBytes([]byte(correct), []byte(corrupted))

	assert.False(t, result.ConflictMarkerMatch)
	assert.Contains(t, result.Diff, correct)
	assert.Contains(t, result.Diff, corrupted)
}
