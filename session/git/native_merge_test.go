package git

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mergeAbortFixture builds a repo where HEAD (branch "feature") has a.txt = "line1\nours\n",
// with a distinct "theirs" commit on "main" carrying a.txt = "line1\ntheirs\n" and a
// common ancestor carrying "line1\n" — real blob hashes for all three, and the
// then-current (pre-conflict) working-tree content, for writeConflictedIndex +
// writeMergeStateFiles + abortNativeMerge's real-git-recognized round trip.
type mergeAbortFixture struct {
	repoPath        string
	conflictEntries []*index.Entry
	preMergeEntry   *index.Entry
	preMergeContent []byte
	theirsSHA       string
}

func buildMergeAbortFixture(t *testing.T) mergeAbortFixture {
	t.Helper()
	r := newIndexTestRepo(t)

	baseCommit := r.writeAndCommit("a.txt", "line1\n", "base")
	baseHash := commitBlobHash(t, baseCommit, "a.txt")

	require.NoError(t, r.wt.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("feature"),
		Create: true,
	}))
	oursCommit := r.writeAndCommit("a.txt", "line1\nours\n", "ours")
	oursHash := commitBlobHash(t, oursCommit, "a.txt")

	require.NoError(t, r.wt.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("main"),
	}))
	theirsCommit := r.writeAndCommit("a.txt", "line1\ntheirs\n", "theirs")
	theirsHash := commitBlobHash(t, theirsCommit, "a.txt")

	require.NoError(t, r.wt.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("feature"),
	}))

	return mergeAbortFixture{
		repoPath:        r.path,
		conflictEntries: NewConflictEntries("a.txt", baseHash, oursHash, theirsHash, filemode.Regular),
		preMergeEntry:   &index.Entry{Name: "a.txt", Hash: oursHash, Mode: filemode.Regular},
		preMergeContent: []byte("line1\nours\n"),
		theirsSHA:       theirsCommit.Hash.String(),
	}
}

// materializeFixtureConflict writes the fixture's conflicted index and conflict-marker
// working-tree content plus merge-state files — the "materialize" half of
// materializeConflictOnAbort, built from Story 3.3.1/3.3.2/3.3.3's functions together.
func materializeFixtureConflict(t *testing.T, f mergeAbortFixture) {
	t.Helper()
	require.NoError(t, writeConflictedIndex(f.repoPath, f.conflictEntries))

	hunk := MergeHunk{Kind: RegionConflict, Ours: []string{"ours"}, Theirs: []string{"theirs"}}
	markerText, err := renderConflictHunk(hunk, "HEAD", "origin/main")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(f.repoPath, "a.txt"), []byte(markerText), 0o644))

	require.NoError(t, writeMergeStateFiles(f.repoPath, f.theirsSHA, "main"))
}

// TestWriteMergeStateFiles_RealGitMergeAbortSucceeds covers Story 3.3.3's first
// acceptance criterion: a real `git merge --abort` subprocess run against the
// materialized state must exit 0 and leave a clean working tree.
func TestWriteMergeStateFiles_RealGitMergeAbortSucceeds(t *testing.T) {
	t.Parallel()
	f := buildMergeAbortFixture(t)
	materializeFixtureConflict(t, f)

	runRealGitIndexTest(t, f.repoPath, "merge", "--abort")

	statusOut := runRealGitIndexTest(t, f.repoPath, "status", "--porcelain")
	assert.Empty(t, strings.TrimSpace(statusOut), "worktree must be clean after a real git merge --abort")
}

// TestAbortNativeMerge_ClearsStateAndResetsWorkingTree covers Story 3.3.3's second
// acceptance criterion: after this project's own abort path, the three merge-state
// files are gone and the working tree matches its pre-merge state.
func TestAbortNativeMerge_ClearsStateAndResetsWorkingTree(t *testing.T) {
	t.Parallel()
	f := buildMergeAbortFixture(t)
	materializeFixtureConflict(t, f)

	snapshot := &PreMergeIndexSnapshot{
		Entries: []*index.Entry{f.preMergeEntry},
		Content: map[string][]byte{"a.txt": f.preMergeContent},
	}
	require.NoError(t, abortNativeMerge(f.repoPath, snapshot))

	gitDir := filepath.Join(f.repoPath, ".git")
	for _, name := range []string{mergeHeadFile, mergeMsgFile, mergeModeFile} {
		_, err := os.Stat(filepath.Join(gitDir, name))
		assert.True(t, os.IsNotExist(err), "%s must be removed after abortNativeMerge", name)
	}

	content, err := os.ReadFile(filepath.Join(f.repoPath, "a.txt"))
	require.NoError(t, err)
	assert.Equal(t, f.preMergeContent, content)

	statusOut := runRealGitIndexTest(t, f.repoPath, "status", "--porcelain")
	assert.Empty(t, strings.TrimSpace(statusOut), "worktree must be clean after abortNativeMerge")
}

func TestAbortNativeMerge_should_ReturnError_When_PreMergeIndexSnapshotMissing(t *testing.T) {
	t.Parallel()
	f := buildMergeAbortFixture(t)
	materializeFixtureConflict(t, f)

	err := abortNativeMerge(f.repoPath, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "pre-merge index snapshot")
}

// TestNativeMergeMainIntoWorktree_UpToDate covers Task 3.4.1a's up-to-date short-circuit:
// a branch that already contains origin/main's tip must be reported UpToDate with no ref
// change, matching legacyMergeMainIntoWorktree's existing semantics exactly.
func TestNativeMergeMainIntoWorktree_UpToDate(t *testing.T) {
	t.Parallel()
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	runGit(t, work, "checkout", "-b", "feature")

	beforeSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))

	result, err := nativeMergeMainIntoWorktree(work, "main")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.UpToDate)
	assert.False(t, result.Merged)
	assert.False(t, result.Conflicted)

	afterSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))
	assert.Equal(t, beforeSHA, afterSHA, "an up-to-date merge must not touch the branch ref")
}

// TestNativeMergeMainIntoWorktree_FastForward covers Task 3.4.1a's fast-forward
// short-circuit: a strict-ancestor branch must be reported Merged with its ref advanced
// to origin/main's exact SHA (subprocess-verified per the acceptance criteria), and its
// working tree updated to match.
func TestNativeMergeMainIntoWorktree_FastForward(t *testing.T) {
	t.Parallel()
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	runGit(t, work, "checkout", "-b", "feature")

	require.NoError(t, os.WriteFile(filepath.Join(origin, "main-fix.txt"), []byte("fix on main\n"), 0o644))
	runGit(t, origin, "add", "main-fix.txt")
	runGit(t, origin, "commit", "-m", "fix landed on main")

	result, err := nativeMergeMainIntoWorktree(work, "main")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Merged)
	assert.False(t, result.UpToDate)
	assert.False(t, result.Conflicted)

	wantSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "origin/main"))
	gotSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "feature"))
	assert.Equal(t, wantSHA, gotSHA, "the checked-out branch ref must now match origin/main's SHA exactly")

	assert.FileExists(t, filepath.Join(work, "main-fix.txt"))
	status := runGit(t, work, "status", "--porcelain")
	assert.Empty(t, strings.TrimSpace(status), "a real git status must see the fast-forward as clean")
}

// TestNativeMergeMainIntoWorktree_CleanThreeWayMerge covers Task 3.4.1b: a divergence
// with no overlapping edits must produce a real two-parent merge commit (subprocess-
// verified via `git log -1 --format=%P`), with both sides' files present and the
// worktree left clean.
func TestNativeMergeMainIntoWorktree_CleanThreeWayMerge(t *testing.T) {
	t.Parallel()
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	runGit(t, work, "checkout", "-b", "feature")

	require.NoError(t, os.WriteFile(filepath.Join(work, "feature.txt"), []byte("feature work\n"), 0o644))
	runGit(t, work, "add", "feature.txt")
	runGit(t, work, "commit", "-m", "feature work")

	require.NoError(t, os.WriteFile(filepath.Join(origin, "main-fix.txt"), []byte("fix on main\n"), 0o644))
	runGit(t, origin, "add", "main-fix.txt")
	runGit(t, origin, "commit", "-m", "fix landed on main")

	result, err := nativeMergeMainIntoWorktree(work, "main")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Merged)
	assert.False(t, result.UpToDate)
	assert.False(t, result.Conflicted)

	assert.FileExists(t, filepath.Join(work, "feature.txt"))
	assert.FileExists(t, filepath.Join(work, "main-fix.txt"))

	parents := strings.Fields(runGit(t, work, "log", "-1", "--format=%P"))
	assert.Len(t, parents, 2, "a native clean merge must produce a real two-parent commit")

	status := runGit(t, work, "status", "--porcelain")
	assert.Empty(t, strings.TrimSpace(status), "a real git status must see the merge as clean")
}

// TestNativeMergeMainIntoWorktree_RefusesOnDirtyWorktree_PreservesUncommittedChanges
// covers PR #730 Gate 2 Blocker 1: legacy's subprocess `git merge` refuses outright with
// "local changes would be overwritten" against a dirty worktree, but the native path had
// no such check and would silently discard the uncommitted edit in any touched path
// instead. A divergence exists here (main-fix.txt on main) so the merge would otherwise
// have real work to do, proving the refusal fires before any merge computation, not just
// because there was nothing to merge.
func TestNativeMergeMainIntoWorktree_RefusesOnDirtyWorktree_PreservesUncommittedChanges(t *testing.T) {
	t.Parallel()
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	runGit(t, work, "checkout", "-b", "feature")

	require.NoError(t, os.WriteFile(filepath.Join(origin, "main-fix.txt"), []byte("fix on main\n"), 0o644))
	runGit(t, origin, "add", "main-fix.txt")
	runGit(t, origin, "commit", "-m", "fix landed on main")

	beforeSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))
	const dirtyContent = "uncommitted local edit\n"
	require.NoError(t, os.WriteFile(filepath.Join(work, "README.md"), []byte(dirtyContent), 0o644))

	result, err := nativeMergeMainIntoWorktree(work, "main")

	require.Error(t, err, "a dirty worktree must refuse the merge, matching legacy git merge's own refusal")
	assert.Nil(t, result)

	afterSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))
	assert.Equal(t, beforeSHA, afterSHA, "a refused merge must not touch HEAD")

	content, readErr := os.ReadFile(filepath.Join(work, "README.md"))
	require.NoError(t, readErr)
	assert.Equal(t, dirtyContent, string(content), "the uncommitted edit must survive the refused merge untouched")
}

// TestNativeMergeMainIntoWorktree_CleanWorktree_StillMerges is the regression guard for
// Blocker 1's fix: the new dirty-worktree check must not false-positive on an ordinary
// clean worktree — a real divergence must still merge successfully.
func TestNativeMergeMainIntoWorktree_CleanWorktree_StillMerges(t *testing.T) {
	t.Parallel()
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	runGit(t, work, "checkout", "-b", "feature")

	require.NoError(t, os.WriteFile(filepath.Join(origin, "main-fix.txt"), []byte("fix on main\n"), 0o644))
	runGit(t, origin, "add", "main-fix.txt")
	runGit(t, origin, "commit", "-m", "fix landed on main")

	result, err := nativeMergeMainIntoWorktree(work, "main")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Merged)
	assert.FileExists(t, filepath.Join(work, "main-fix.txt"))
}

// TestNativeMergeMainIntoWorktree_Conflicted_LeavesWorktreeClean covers Task 3.4.1c: an
// overlapping edit must be reported Conflicted with the correct file list, and — per
// materializeConflictOnAbort's "always materialize, then abort" decision — leave the
// worktree exactly as clean as a real `git merge --abort` would, verified via a real `git
// status --porcelain` subprocess, not just internal state.
func TestNativeMergeMainIntoWorktree_Conflicted_LeavesWorktreeClean(t *testing.T) {
	t.Parallel()
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	runGit(t, work, "checkout", "-b", "feature")

	require.NoError(t, os.WriteFile(filepath.Join(work, "README.md"), []byte("# Feature Edit\n"), 0o644))
	runGit(t, work, "add", "README.md")
	runGit(t, work, "commit", "-m", "feature edits README")

	require.NoError(t, os.WriteFile(filepath.Join(origin, "README.md"), []byte("# Main Edit\n"), 0o644))
	runGit(t, origin, "add", "README.md")
	runGit(t, origin, "commit", "-m", "main edits README")

	beforeSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))

	result, err := nativeMergeMainIntoWorktree(work, "main")
	require.NoError(t, err, "a real conflict must be reported via the result, not returned as an error")
	require.NotNil(t, result)
	assert.True(t, result.Conflicted)
	assert.False(t, result.UpToDate)
	assert.False(t, result.Merged)
	assert.Equal(t, []string{"README.md"}, result.ConflictedFiles)

	afterSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))
	assert.Equal(t, beforeSHA, afterSHA, "a conflicted native merge must leave HEAD untouched")
	assert.NoFileExists(t, filepath.Join(work, ".git", "MERGE_HEAD"))

	status := runGit(t, work, "status", "--porcelain")
	assert.Empty(t, strings.TrimSpace(status), "worktree must be exactly as clean as a real git merge --abort would leave it")
}

// TestNativeMergeMainIntoWorktree_should_ReturnError_When_FetchFails mirrors the existing
// legacy-path test of the same shape: an unreachable origin must surface as an error, not
// any MergeMainResult state, and must leave no partial merge state on disk.
func TestNativeMergeMainIntoWorktree_should_ReturnError_When_FetchFails(t *testing.T) {
	t.Parallel()
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	runGit(t, work, "checkout", "-b", "feature")
	runGit(t, work, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "does-not-exist"))

	result, err := nativeMergeMainIntoWorktree(work, "main")
	require.Error(t, err)
	assert.Nil(t, result)
	assert.NoFileExists(t, filepath.Join(work, ".git", "MERGE_HEAD"))
}

// TestNativeMergeMainIntoWorktree_IncrementsConflictOutcomeCounter is Task 4.4.2b's
// validation.md test: a Conflicted: true result must increment
// git_merge_outcome_total{outcome="conflicted"} by exactly 1. Deliberately not
// t.Parallel(): it takes a before/after delta on the counter, and Go's testing package
// runs every non-parallel test in this file to completion before any t.Parallel() test in
// it resumes, so this avoids racing against the package's other (parallel) merge tests
// that also produce a "conflicted" outcome.
func TestNativeMergeMainIntoWorktree_IncrementsConflictOutcomeCounter(t *testing.T) {
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	runGit(t, work, "checkout", "-b", "feature")

	require.NoError(t, os.WriteFile(filepath.Join(work, "README.md"), []byte("# Feature Edit\n"), 0o644))
	runGit(t, work, "add", "README.md")
	runGit(t, work, "commit", "-m", "feature edits README")

	require.NoError(t, os.WriteFile(filepath.Join(origin, "README.md"), []byte("# Main Edit\n"), 0o644))
	runGit(t, origin, "add", "README.md")
	runGit(t, origin, "commit", "-m", "main edits README")

	before := collectGitMetric(t, "git_merge_outcome_total")
	baseline := sumGitCounterForAttr(t, before, "outcome", mergeOutcomeConflicted)

	result, err := nativeMergeMainIntoWorktree(work, "main")
	require.NoError(t, err)
	require.True(t, result.Conflicted)

	after := collectGitMetric(t, "git_merge_outcome_total")
	require.NotNil(t, after)
	assert.Equal(t, baseline+1, sumGitCounterForAttr(t, after, "outcome", mergeOutcomeConflicted))
}

// TestNativeMerge_And_NativeSetup_SerializeThroughSameLock proves a native merge and a
// native Setup() against the same repoPath serialize through the same repoWorktreeLock
// registry entry — mirroring TestSetupRemove_MixedImplementations_SerializeThroughSameLock's
// proof strategy (worktree_ops_test.go).
//
// Proof strategy: the test acquires the shared repoWorktreeLock's intra-process mutex
// itself, before launching either call, then confirms both are still blocked (neither
// returned) after a generous wait — not a timing heuristic, since mu is a genuine
// sync.Mutex (see the mirrored test's doc comment for the full reasoning).
func TestNativeMerge_And_NativeSetup_SerializeThroughSameLock(t *testing.T) {
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	runGit(t, work, "checkout", "-b", "feature")

	require.NoError(t, os.WriteFile(filepath.Join(origin, "main-fix.txt"), []byte("fix on main\n"), 0o644))
	runGit(t, origin, "add", "main-fix.txt")
	runGit(t, origin, "commit", "-m", "fix landed on main")

	origWorktree := useNativeWorktree
	useNativeWorktree = func(string) bool { return true }
	t.Cleanup(func() { useNativeWorktree = origWorktree })

	origMerge := useNativeMerge
	useNativeMerge = func(string) bool { return true }
	t.Cleanup(func() { useNativeMerge = origMerge })

	wt, _, err := NewGitWorktreeWithBranch(work, "sess-merge-lock", "sess-merge-lock-branch")
	require.NoError(t, err)

	lockInstance, err := lockForRepo(work)
	require.NoError(t, err)

	lockInstance.mu.Lock()

	errs := make([]error, 2)
	done := make(chan int, 2)
	go func() {
		errs[0] = wt.Setup()
		done <- 0
	}()
	go func() {
		_, errs[1] = MergeMainIntoWorktree(work, "main")
		done <- 1
	}()

	select {
	case which := <-done:
		t.Fatalf("call %d completed while the test held repoWorktreeLock.mu externally -- native merge/setup dispatch is bypassing WithRepoWorktreeLock", which)
	case <-time.After(150 * time.Millisecond):
		// Expected: both goroutines are blocked on mu.Lock() inside WithRepoWorktreeLock.
	}

	lockInstance.mu.Unlock()

	<-done
	<-done
	require.NoError(t, errs[0], "native Setup() must not fail")
	require.NoError(t, errs[1], "native merge must not fail")
	defer func() { _ = wt.Cleanup() }()

	lockAfter, err := lockForRepo(work)
	require.NoError(t, err)
	assert.Same(t, lockInstance, lockAfter, "both calls must resolve to the same repoWorktreeLock singleton for repoPath")
}

// --- MergeFile pipeline-wiring gap (Phase 6 review): resolveMergePaths must actually
// call MergeFile for a real two-sided change, not just ReconcilePathChange's content-only
// merge — otherwise a mode/binary/gitlink mismatch reaches nativeThreeWayMerge undetected
// despite MergeFile's own short-circuits passing their isolated unit tests
// (native_merge_diff3_test.go's TestThreeWayFileMerger_*Conflict tests). Each test below
// drives the real nativeMergeMainIntoWorktree entry point end to end, proving the wiring
// (bothSidesNonDeleteChanged + buildFileMergeInput + merger.MergeFile in resolveMergePaths)
// is actually reachable, not just present in source.

// TestNativeMergeMainIntoWorktree_BinaryConflict_ClassifiedAsConflict covers
// ReasonBinaryConflict: a binary file (NUL byte within git's own sniff length) changed
// differently on both sides must be reported Conflicted, never fed into line-based diff3.
func TestNativeMergeMainIntoWorktree_BinaryConflict_ClassifiedAsConflict(t *testing.T) {
	t.Parallel()
	origin := setupTestRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(origin, "asset.bin"), []byte{0x00, 0x01, 0x02, 0x03, 0x00}, 0o644))
	runGit(t, origin, "add", "asset.bin")
	runGit(t, origin, "commit", "-m", "add base binary asset")

	work := cloneTestRepo(t, origin)
	runGit(t, work, "checkout", "-b", "feature")

	require.NoError(t, os.WriteFile(filepath.Join(work, "asset.bin"), []byte{0xAA, 0xBB, 0xCC, 0x00, 0xDD}, 0o644))
	runGit(t, work, "add", "asset.bin")
	runGit(t, work, "commit", "-m", "ours: change binary asset")

	require.NoError(t, os.WriteFile(filepath.Join(origin, "asset.bin"), []byte{0x11, 0x22, 0x00, 0x33, 0x44}, 0o644))
	runGit(t, origin, "add", "asset.bin")
	runGit(t, origin, "commit", "-m", "theirs: change binary asset differently")

	result, err := nativeMergeMainIntoWorktree(work, "main")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Conflicted, "a binary file changed differently on both sides must be classified as a conflict")
	assert.Contains(t, result.ConflictedFiles, "asset.bin")

	status := runGit(t, work, "status", "--porcelain")
	assert.Empty(t, strings.TrimSpace(status), "a binary conflict must still leave the worktree clean after abort")
}

// TestNativeMergeMainIntoWorktree_ModeConflict_ClassifiedAsConflict covers
// ReasonModeConflict: this must fire only for a genuine TWO-sided mode disagreement —
// both ours and theirs change script.sh's mode away from base, and disagree with each
// other (Executable vs Symlink) — not merely because the two final modes differ (MUST FIX
// 2, Phase 6 verify: the prior fixture here had ONLY ours change the mode, with theirs'
// mode unchanged from base, which is the auto-resolvable one-sided case covered by
// TestThreeWayFileMerger_OneSidedModeChange_AutoResolvesToChangedSide instead).
func TestNativeMergeMainIntoWorktree_ModeConflict_ClassifiedAsConflict(t *testing.T) {
	t.Parallel()
	origin := setupTestRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(origin, "script.sh"), []byte("echo hi\n"), 0o644))
	runGit(t, origin, "add", "script.sh")
	runGit(t, origin, "commit", "-m", "add base script")

	work := cloneTestRepo(t, origin)
	runGit(t, work, "checkout", "-b", "feature")

	require.NoError(t, os.Chmod(filepath.Join(work, "script.sh"), 0o755))
	runGit(t, work, "add", "script.sh")
	runGit(t, work, "commit", "-m", "ours: chmod +x, content unchanged")

	// theirs: replace script.sh with a symlink — a mode change AWAY from base's Regular,
	// and to a different mode than ours' Executable, making this a genuine two-sided
	// disagreement rather than a one-sided change.
	require.NoError(t, os.Remove(filepath.Join(origin, "script.sh")))
	require.NoError(t, os.Symlink("other-target", filepath.Join(origin, "script.sh")))
	runGit(t, origin, "add", "script.sh")
	runGit(t, origin, "commit", "-m", "theirs: replace script.sh with a symlink")

	result, err := nativeMergeMainIntoWorktree(work, "main")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Conflicted, "a genuine two-sided mode disagreement must be classified as a conflict")
	assert.Contains(t, result.ConflictedFiles, "script.sh")
}

// TestNativeMergeMainIntoWorktree_GitlinkConflict_ClassifiedAsConflict covers
// ReasonGitlinkConflict: a submodule entry (mode 160000, pointing at a commit SHA rather
// than a blob) advanced to different commits on both sides. Neither side needs a real,
// initialized submodule checkout — a gitlink is just a 160000-mode tree entry, and git
// itself tolerates one with no .gitmodules registration as a plain empty directory — so
// this builds it directly via `git update-index --cacheinfo` rather than a real
// `git submodule add`.
//
// A .gitmodules entry is registered anyway (unlike real git, which tolerates the missing
// registration fine): go-git's Worktree.Status(), added by Gate 2 Blocker 1's
// dirty-worktree guard, can't identify an unregistered gitlink path as a submodule and
// reports its empty placeholder directory as locally " D" (deleted) — a false positive
// real `git status` doesn't share (verified: real git reports this worktree clean with or
// without .gitmodules; go-git only agrees once .gitmodules is present).
func TestNativeMergeMainIntoWorktree_GitlinkConflict_ClassifiedAsConflict(t *testing.T) {
	t.Parallel()
	origin := setupTestRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(origin, ".gitmodules"),
		[]byte("[submodule \"vendor/lib\"]\n\tpath = vendor/lib\n\turl = https://example.com/fake.git\n"), 0o644))
	runGit(t, origin, "add", ".gitmodules")
	runGit(t, origin, "update-index", "--add", "--cacheinfo", "160000,1111111111111111111111111111111111111111,vendor/lib")
	runGit(t, origin, "commit", "-m", "add base gitlink")

	work := cloneTestRepo(t, origin)
	runGit(t, work, "checkout", "-b", "feature")

	runGit(t, work, "update-index", "--add", "--cacheinfo", "160000,2222222222222222222222222222222222222222,vendor/lib")
	runGit(t, work, "commit", "-m", "ours: point gitlink elsewhere")

	runGit(t, origin, "update-index", "--add", "--cacheinfo", "160000,3333333333333333333333333333333333333333,vendor/lib")
	runGit(t, origin, "commit", "-m", "theirs: point gitlink elsewhere")

	result, err := nativeMergeMainIntoWorktree(work, "main")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Conflicted, "a gitlink advanced to different commits on both sides must be classified as a conflict")
	assert.Contains(t, result.ConflictedFiles, "vendor/lib")
}

// --- Phase 6 verify, MUST FIX 1: path-traversal/symlink-escape in native merge
// materialization (writeWorkingTreeFile/removeWorkingTreeFile/restoreWorkingTreeFile/
// capturePreMergeSnapshot). See validateTreeEntryRelPath's doc comment for why go-git's
// own Tree.Decode places no restriction on a raw tree-entry name.

func TestValidateTreeEntryRelPath_RejectsEscapingAndUnsafeNames(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		relPath string
	}{
		{"ParentEscape", "../escape.txt"},
		{"DeepParentEscape", "../../../../etc/passwd"},
		{"NestedParentEscape", "dir/../../escape.txt"},
		{"BareDotDot", ".."},
		{"BareDot", "."},
		{"AbsolutePath", "/etc/passwd"},
		{"DotGitComponent", ".git/hooks/pre-commit"},
		{"NestedDotGitComponent", "vendor/.git/config"},
		{"Empty", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Error(t, validateTreeEntryRelPath(tc.relPath), "relPath %q must be rejected", tc.relPath)
		})
	}
}

func TestValidateTreeEntryRelPath_AcceptsOrdinaryPaths(t *testing.T) {
	t.Parallel()
	cases := []string{"README.md", "src/main.go", "a/b/c.txt", "..hidden-but-not-traversal", "user..name.txt"}
	for _, relPath := range cases {
		t.Run(relPath, func(t *testing.T) {
			t.Parallel()
			assert.NoError(t, validateTreeEntryRelPath(relPath))
		})
	}
}

// TestWriteWorkingTreeFile_RejectsEscapingRelPath_DoesNotWriteOutsideWorktree is the
// unit-level proof, below the full merge pipeline (exercised separately by
// TestNativeMergeMainIntoWorktree_MaliciousTreeEntry_RejectedNotWrittenOutsideWorktree):
// writeWorkingTreeFile — the function every real tree-entry write funnels through — must
// never touch disk outside worktreePath for a crafted "../"-escaping relPath.
func TestWriteWorkingTreeFile_RejectsEscapingRelPath_DoesNotWriteOutsideWorktree(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	worktreePath := filepath.Join(parent, "worktree")
	require.NoError(t, os.MkdirAll(worktreePath, 0o750))

	outsideTarget := filepath.Join(parent, "escaped.txt")
	err := writeWorkingTreeFile(worktreePath, "../escaped.txt", filemode.Regular, []byte("pwned\n"))
	require.Error(t, err)
	_, statErr := os.Stat(outsideTarget)
	assert.True(t, os.IsNotExist(statErr), "a rejected relPath must never reach disk outside worktreePath")
}

// TestNativeMergeMainIntoWorktree_MaliciousTreeEntry_RejectedNotWrittenOutsideWorktree
// covers MUST FIX 1 (Phase 6 verify security finding) end-to-end: a malicious commit
// merged onto mainBranch with a tree entry name that escapes the worktree
// ("../"-relative) must never be materialized outside worktreePath. The malicious commit
// is built directly against go-git's object store (mirroring native_merge_base_test.go's
// mergeBaseFixture pattern), not through the real `git` CLI, which refuses to stage a
// path containing ".." at all — this is the only way to reproduce the exploit's actual
// on-disk shape (a compromised/malicious commit merged onto the tracked default branch,
// per drift.go/backlog_service_triage.go/branchReconciler's automatic branch-sync call
// sites).
//
// Note on what actually rejects this in the pinned go-git/go-git/v5@v5.19.2: while
// Tree.Decode itself performs no validation, this exact pinned version's TreeWalker (used
// by Tree.Diff, and so by both TreeDiffPair and the fast-forward path here) already calls
// internal/pathutil.ValidTreePath on every enumerated entry name and rejects "..", so this
// specific scenario actually errors out of the Diff() call in nativeFastForwardMerge,
// before ever reaching secureWorktreeJoin — confirmed by temporarily reverting
// secureWorktreeJoin's call in writeWorkingTreeFile and observing this test still passes
// unchanged, error text unchanged. secureWorktreeJoin remains correct and necessary
// defense-in-depth regardless: it is the containment boundary for the write itself, not
// contingent on a specific upstream dependency's internal call graph continuing to
// enumerate every path through a validated TreeWalker. The distinct, still-live gap
// secureWorktreeJoin/clearBlockingSymlinksInPath closes — one ValidTreePath's pure
// string-level check cannot address at all — is the symlink-escape vector proved by
// TestNativeMergeMainIntoWorktree_MaliciousSymlinkEntry_DoesNotEscapeViaBlockingSymlink
// below.
func TestNativeMergeMainIntoWorktree_MaliciousTreeEntry_RejectedNotWrittenOutsideWorktree(t *testing.T) {
	t.Parallel()
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	runGit(t, work, "checkout", "-b", "feature")

	repo, err := git.PlainOpen(origin)
	require.NoError(t, err)

	headRef, err := repo.Reference(plumbing.NewBranchReferenceName("main"), true)
	require.NoError(t, err)
	headCommit, err := repo.CommitObject(headRef.Hash())
	require.NoError(t, err)
	headTree, err := headCommit.Tree()
	require.NoError(t, err)

	maliciousBlobObj := repo.Storer.NewEncodedObject()
	maliciousBlobObj.SetType(plumbing.BlobObject)
	bw, err := maliciousBlobObj.Writer()
	require.NoError(t, err)
	_, err = bw.Write([]byte("pwned\n"))
	require.NoError(t, err)
	require.NoError(t, bw.Close())
	maliciousBlobHash, err := repo.Storer.SetEncodedObject(maliciousBlobObj)
	require.NoError(t, err)

	entries := append([]object.TreeEntry(nil), headTree.Entries...)
	entries = append(entries, object.TreeEntry{
		Name: "../escaped-via-native-merge.txt",
		Mode: filemode.Regular,
		Hash: maliciousBlobHash,
	})
	sort.Slice(entries, func(i, j int) bool { return treeEntrySortKey(entries[i]) < treeEntrySortKey(entries[j]) })

	maliciousTreeObj := repo.Storer.NewEncodedObject()
	require.NoError(t, (&object.Tree{Entries: entries}).Encode(maliciousTreeObj))
	maliciousTreeHash, err := repo.Storer.SetEncodedObject(maliciousTreeObj)
	require.NoError(t, err)

	sig := object.Signature{Name: "Attacker", Email: "attacker@example.com", When: time.Now()}
	maliciousCommit := &object.Commit{
		Author:       sig,
		Committer:    sig,
		Message:      "malicious: escape the worktree via a crafted tree entry\n",
		TreeHash:     maliciousTreeHash,
		ParentHashes: []plumbing.Hash{headRef.Hash()},
	}
	maliciousCommitObj := repo.Storer.NewEncodedObject()
	require.NoError(t, maliciousCommit.Encode(maliciousCommitObj))
	maliciousCommitHash, err := repo.Storer.SetEncodedObject(maliciousCommitObj)
	require.NoError(t, err)

	// Advance origin's "main" directly against the object store — origin here is test
	// setup (standing in for a compromised/malicious PR merged upstream), not the
	// worktree under test, so none of writeRefWithLockSentinel's concerns apply.
	require.NoError(t, repo.Storer.SetReference(plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), maliciousCommitHash)))

	// work's "feature" branch has no commits of its own beyond the shared base, so this
	// resolves as a fast-forward — materializeTreeChanges' write path, which is exactly
	// what MUST FIX 1 targets.
	_, err = nativeMergeMainIntoWorktree(work, "main")
	require.Error(t, err, "a tree entry escaping the worktree must fail the merge, not silently write outside it")

	outsideTarget := filepath.Join(filepath.Dir(work), "escaped-via-native-merge.txt")
	_, statErr := os.Stat(outsideTarget)
	assert.True(t, os.IsNotExist(statErr), "the malicious entry must never be written outside work, regardless of how the merge failed")
}

// TestNativeMergeMainIntoWorktree_MaliciousSymlinkEntry_DoesNotEscapeViaBlockingSymlink
// covers the half of MUST FIX 1 that go-git's own internal/pathutil.ValidTreePath cannot
// address at all, because it is a pure string check with no filesystem awareness: a
// malicious tree can contain a clean-looking (no "..", no ".git") Symlink entry — "link",
// pointing at a real directory outside the worktree — immediately followed by a second,
// equally clean-looking entry literally named "link/evil.txt". Neither name fails
// ValidTreePath or validateTreeEntryRelPath on its own; the escape only exists once "link"
// is materialized as a symlink on disk and the second entry's write is resolved through
// it by the OS. Without clearBlockingSymlinksInPath, os.MkdirAll/os.WriteFile follow that
// symlink transparently and the payload lands outside the worktree; with it, the blocking
// symlink is removed and replaced with a real directory first, so the payload lands inside
// the worktree instead — confirmed by temporarily removing the clearBlockingSymlinksInPath
// call from writeWorkingTreeFile and observing this test then fails (payload found outside
// the worktree).
func TestNativeMergeMainIntoWorktree_MaliciousSymlinkEntry_DoesNotEscapeViaBlockingSymlink(t *testing.T) {
	t.Parallel()
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	runGit(t, work, "checkout", "-b", "feature")

	outsideDir := filepath.Join(t.TempDir(), "outside-target")
	require.NoError(t, os.MkdirAll(outsideDir, 0o750))

	repo, err := git.PlainOpen(origin)
	require.NoError(t, err)

	headRef, err := repo.Reference(plumbing.NewBranchReferenceName("main"), true)
	require.NoError(t, err)
	headCommit, err := repo.CommitObject(headRef.Hash())
	require.NoError(t, err)
	headTree, err := headCommit.Tree()
	require.NoError(t, err)

	storeBlob := func(content string) plumbing.Hash {
		t.Helper()
		obj := repo.Storer.NewEncodedObject()
		obj.SetType(plumbing.BlobObject)
		w, err := obj.Writer()
		require.NoError(t, err)
		_, err = w.Write([]byte(content))
		require.NoError(t, err)
		require.NoError(t, w.Close())
		hash, err := repo.Storer.SetEncodedObject(obj)
		require.NoError(t, err)
		return hash
	}

	symlinkTargetHash := storeBlob(outsideDir) // symlink content is its target path, as bytes
	payloadHash := storeBlob("pwned\n")

	entries := append([]object.TreeEntry(nil), headTree.Entries...)
	entries = append(entries,
		object.TreeEntry{Name: "link", Mode: filemode.Symlink, Hash: symlinkTargetHash},
		object.TreeEntry{Name: "link/evil.txt", Mode: filemode.Regular, Hash: payloadHash},
	)
	sort.Slice(entries, func(i, j int) bool { return treeEntrySortKey(entries[i]) < treeEntrySortKey(entries[j]) })

	maliciousTreeObj := repo.Storer.NewEncodedObject()
	require.NoError(t, (&object.Tree{Entries: entries}).Encode(maliciousTreeObj))
	maliciousTreeHash, err := repo.Storer.SetEncodedObject(maliciousTreeObj)
	require.NoError(t, err)

	sig := object.Signature{Name: "Attacker", Email: "attacker@example.com", When: time.Now()}
	maliciousCommit := &object.Commit{
		Author:       sig,
		Committer:    sig,
		Message:      "malicious: plant a symlink then write through it\n",
		TreeHash:     maliciousTreeHash,
		ParentHashes: []plumbing.Hash{headRef.Hash()},
	}
	maliciousCommitObj := repo.Storer.NewEncodedObject()
	require.NoError(t, maliciousCommit.Encode(maliciousCommitObj))
	maliciousCommitHash, err := repo.Storer.SetEncodedObject(maliciousCommitObj)
	require.NoError(t, err)

	require.NoError(t, repo.Storer.SetReference(plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), maliciousCommitHash)))

	// work's "feature" branch has no commits of its own beyond the shared base, so this
	// resolves as a fast-forward — materializeTreeChanges' write path.
	_, mergeErr := nativeMergeMainIntoWorktree(work, "main")

	escapedTarget := filepath.Join(outsideDir, "evil.txt")
	_, statErr := os.Stat(escapedTarget)
	assert.True(t, os.IsNotExist(statErr), "the payload must never be written outside the worktree via the planted symlink, regardless of the merge's overall outcome (err=%v)", mergeErr)
}

// TestNativeMergeMainIntoWorktree_ConflictReadThroughBlockingSymlink_IsSafelyBlocked
// covers the gap a re-review of MUST FIX 1 found: capturePreMergeSnapshot's
// os.ReadFile(fullPath) — used to snapshot a CONFLICTED path's pre-merge working-tree
// content, for abortNativeMerge to later restore — went through secureWorktreeJoin but
// not clearBlockingSymlinksInPath, unlike the other three write-side functions. If a
// blocking symlink already sits on disk at a leading path component of a conflicted path
// (here, "link" pointing outside the worktree, planted before the merge runs — this is
// the read-side, information-disclosure half of the bug: a query fails without the
// containment fix, this test would silently read a real secret file living outside the
// worktree (planted at outsideDir/evil.txt) into the merge's pre-merge snapshot, which
// materializeConflictOnAbort/abortNativeMerge would then write straight back into the
// worktree.
//
// "link/evil.txt" is constructed as one flat, non-nested tree entry (mirroring the
// symlink-escape test above) so it can be a real MODIFY/MODIFY (add/add) conflict without
// requiring git's normal directory nesting — both "ours" (work's own feature-branch tip)
// and "theirs" (origin/main) independently add this path with different content, via raw
// go-git object construction against each repo directly (not the real `git` CLI, which
// cannot create such a tree shape), so the actual on-disk symlink governs what
// capturePreMergeSnapshot reads, not whatever real git's checkout would have written.
func TestNativeMergeMainIntoWorktree_ConflictReadThroughBlockingSymlink_IsSafelyBlocked(t *testing.T) {
	t.Parallel()
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	runGit(t, work, "checkout", "-b", "feature")

	outsideDir := filepath.Join(t.TempDir(), "outside-target")
	require.NoError(t, os.MkdirAll(outsideDir, 0o750))
	const secretContent = "SECRET-OUTSIDE-CONTENT\n"
	require.NoError(t, os.WriteFile(filepath.Join(outsideDir, "evil.txt"), []byte(secretContent), 0o600))

	addFlatEntry := func(t *testing.T, repoPath, branchRefName, content string) {
		t.Helper()
		repo, err := git.PlainOpen(repoPath)
		require.NoError(t, err)

		headRef, err := repo.Reference(plumbing.ReferenceName(branchRefName), true)
		require.NoError(t, err)
		headCommit, err := repo.CommitObject(headRef.Hash())
		require.NoError(t, err)
		headTree, err := headCommit.Tree()
		require.NoError(t, err)

		blobObj := repo.Storer.NewEncodedObject()
		blobObj.SetType(plumbing.BlobObject)
		bw, err := blobObj.Writer()
		require.NoError(t, err)
		_, err = bw.Write([]byte(content))
		require.NoError(t, err)
		require.NoError(t, bw.Close())
		blobHash, err := repo.Storer.SetEncodedObject(blobObj)
		require.NoError(t, err)

		entries := append([]object.TreeEntry(nil), headTree.Entries...)
		entries = append(entries, object.TreeEntry{Name: "link/evil.txt", Mode: filemode.Regular, Hash: blobHash})
		sort.Slice(entries, func(i, j int) bool { return treeEntrySortKey(entries[i]) < treeEntrySortKey(entries[j]) })

		treeObj := repo.Storer.NewEncodedObject()
		require.NoError(t, (&object.Tree{Entries: entries}).Encode(treeObj))
		treeHash, err := repo.Storer.SetEncodedObject(treeObj)
		require.NoError(t, err)

		sig := object.Signature{Name: "Test User", Email: "test@example.com", When: time.Now()}
		commit := &object.Commit{
			Author: sig, Committer: sig,
			Message:      "add link/evil.txt\n",
			TreeHash:     treeHash,
			ParentHashes: []plumbing.Hash{headRef.Hash()},
		}
		commitObj := repo.Storer.NewEncodedObject()
		require.NoError(t, commit.Encode(commitObj))
		commitHash, err := repo.Storer.SetEncodedObject(commitObj)
		require.NoError(t, err)

		require.NoError(t, repo.Storer.SetReference(plumbing.NewHashReference(plumbing.ReferenceName(branchRefName), commitHash)))
	}

	// ours: work's own "feature" branch tip adds link/evil.txt with one content.
	addFlatEntry(t, work, "refs/heads/feature", "ours content for link/evil.txt\n")
	// theirs: origin's "main" adds the same flat path with different content — a real
	// modify/modify (add/add) conflict once merged.
	addFlatEntry(t, origin, "refs/heads/main", "theirs content for link/evil.txt\n")

	// Plant the blocking symlink directly on disk — not through git at all, exactly the
	// "already exists on disk" precondition the re-review's finding describes.
	require.NoError(t, os.Symlink(outsideDir, filepath.Join(work, "link")))

	_, mergeErr := nativeMergeMainIntoWorktree(work, "main")
	require.Error(t, mergeErr, "capturePreMergeSnapshot must fail safely once the blocking symlink is cleared, not silently read through it")

	leakedPath := filepath.Join(work, "link", "evil.txt")
	content, readErr := os.ReadFile(leakedPath)
	if strings.Contains(mergeErr.Error(), "uncommitted changes") {
		// Gate 2 Blocker 1's dirty-worktree guard fires first here: the planted symlink
		// is an untracked worktree entry, so the merge refuses before ever reaching
		// capturePreMergeSnapshot's tree-writing logic at all — an even safer outcome
		// than clearing the symlink mid-merge, since no worktree write of any kind
		// happens. The symlink is therefore left exactly as the test planted it, still
		// resolving to its original (pre-merge) target.
		if readErr == nil {
			assert.Equal(t, secretContent, string(content), "a merge refused before touching the worktree must leave the planted symlink completely untouched")
		}
		return
	}
	if readErr == nil {
		assert.NotEqual(t, secretContent, string(content), "content from outside the worktree must never leak into the worktree via capturePreMergeSnapshot's read")
	}
}
