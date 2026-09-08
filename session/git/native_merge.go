package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/utils/merkletrie"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/telemetry"
)

// Merge outcome values for the git_merge_outcome_total counter below (Task 4.4.2b),
// matching the four outcome branches nativeMergeMainIntoWorktree's call flow can produce.
// Lower-cased to match requirements.md's Observability Requirements / plan.md's
// git_merge_outcome_total{outcome="conflicted"} example literally.
const (
	mergeOutcomeUpToDate    = "uptodate"
	mergeOutcomeFastForward = "fastforward"
	mergeOutcomeCleanMerge  = "cleanmerge"
	mergeOutcomeConflicted  = "conflicted"
)

// mergeOutcomeTotal is the conflict-rate counter named in plan.md's Observability Plan
// (Task 4.4.2b) — proves requirements.md's Observability Requirements are measurable, not
// just assumed. telemetry.GetMeter() is safe to call before telemetry.Initialize.
var mergeOutcomeTotal = mustInt64CounterGit(telemetry.GetMeter(), "git_merge_outcome_total",
	metric.WithDescription("Count of native merge pipeline outcomes: uptodate, fastforward, cleanmerge, conflicted"))

func recordMergeOutcome(outcome string) {
	mergeOutcomeTotal.Add(context.Background(), 1, metric.WithAttributes(attribute.String("outcome", outcome)))
}

// Merge-state file names real git's own `git merge --abort`/`git status` look for to
// recognize a worktree mid-merge (Domain Glossary: writeMergeStateFiles/
// clearMergeStateFiles). MERGE_HEAD is the one git actually requires to be present;
// MERGE_MSG/MERGE_MODE are written alongside it for parity with what a real `git merge`
// leaves behind.
const (
	mergeHeadFile = "MERGE_HEAD"
	mergeMsgFile  = "MERGE_MSG"
	mergeModeFile = "MERGE_MODE"
)

// WorktreePath and BranchName exist to eliminate this file's internal same-typed-string
// parameter pairs (primitive-obsession-checklist skill) — worktreePath/mainBranch,
// worktreePath/theirsCommitSHA, and mainBranch/refPath pairs used to compile a silent
// argument swap with no error. Deliberately NOT applied to the file's two true external
// boundaries — nativeMergeMainIntoWorktreeLocked (called from ops.go) and
// nativeMergeMainIntoWorktree (called from native_merge_differential_test.go) — both stay
// plain strings so callers outside this file need no changes; each converts to these types
// internally on its very first line instead.
type (
	// WorktreePath is an absolute filesystem path to a worktree's working directory.
	WorktreePath string
	// BranchName is a git branch name (e.g. "main"), never a path or a commit SHA.
	BranchName string
)

// threeWayMergeContext bundles the repo-level context nativeThreeWayMerge threads through
// to materializeConflictOnAbort/commitThreeWayMerge — introduced to keep all three
// functions under this repo's 5-parameter guideline (syntax-rules-go's long-parameter-list
// check) without losing which fields are shared, always-together context (worktreePath,
// repo, refPath, mainBranch) versus which are call-specific (ours/theirs, resolved paths).
type threeWayMergeContext struct {
	worktreePath WorktreePath
	repo         *git.Repository
	mainBranch   BranchName
	// refPath is the on-disk file path of the branch ref being advanced — see
	// checkedOutBranchRefPath's doc comment.
	refPath string
}

// worktreeGitDir returns the .git admin directory that applies to worktreePath (a real
// directory for the main working copy, or a linked worktree's private admin dir) — the
// same directory resolveWorktreeIndexPath resolves the index file into, minus the
// "index" filename itself.
func worktreeGitDir(worktreePath WorktreePath) (string, error) {
	indexPath, err := resolveWorktreeIndexPath(string(worktreePath))
	if err != nil {
		return "", fmt.Errorf("worktreeGitDir: %w", err)
	}
	return filepath.Dir(indexPath), nil
}

// writeMergeStateFiles writes the minimum real-git merge-state files
// (MERGE_HEAD/MERGE_MSG/MERGE_MODE) so a worktree mid-native-merge is recognizable by,
// and abortable via, a real `git merge --abort` fallback (Story 3.3.3,
// architecture.md §3's abort-compatibility gap).
func writeMergeStateFiles(worktreePath WorktreePath, theirsCommitSHA string, mainBranch BranchName) error {
	dir, err := worktreeGitDir(worktreePath)
	if err != nil {
		return fmt.Errorf("writeMergeStateFiles: %w", err)
	}
	w := NewAdminFileWriter(dir)

	if err := w.WriteFile(mergeHeadFile, []byte(theirsCommitSHA+"\n")); err != nil {
		return fmt.Errorf("writeMergeStateFiles: write %s: %w", mergeHeadFile, err)
	}
	msg := fmt.Sprintf("Merge branch 'origin/%s'\n", mainBranch)
	if err := w.WriteFile(mergeMsgFile, []byte(msg)); err != nil {
		return fmt.Errorf("writeMergeStateFiles: write %s: %w", mergeMsgFile, err)
	}
	if err := w.WriteFile(mergeModeFile, []byte{}); err != nil {
		return fmt.Errorf("writeMergeStateFiles: write %s: %w", mergeModeFile, err)
	}
	return nil
}

// clearMergeStateFiles removes the three merge-state files written by
// writeMergeStateFiles, ignoring not-exist — this project's own abort path
// (abortNativeMerge) calls it last, after restoring the pre-merge index and working
// tree.
func clearMergeStateFiles(worktreePath WorktreePath) error {
	dir, err := worktreeGitDir(worktreePath)
	if err != nil {
		return fmt.Errorf("clearMergeStateFiles: %w", err)
	}
	for _, name := range []string{mergeHeadFile, mergeMsgFile, mergeModeFile} {
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("clearMergeStateFiles: remove %s: %w", name, err)
		}
	}
	return nil
}

// PreMergeIndexSnapshot captures a conflicted path's state immediately before a merge
// attempt touched it, so abortNativeMerge can restore both the index and the
// working-tree file content after materializeConflictOnAbort's transient conflict
// markers/index entries are no longer wanted (Tech Debt Disposition: "always
// materialize, then abort" — this is the "then abort" half).
type PreMergeIndexSnapshot struct {
	// Entries are the original stage-0 index entries for every path that became
	// conflicted, captured before the merge attempt touched them.
	Entries []*index.Entry
	// Content is each conflicted path's original working-tree file content, keyed by
	// path relative to the worktree root.
	Content map[string][]byte
}

// abortNativeMerge reverses materializeConflictOnAbort's transient write: restores the
// pre-merge index (via writeIndexEntries — writeConflictedIndex's sibling clean-write
// path), rewrites the conflicted paths' working-tree content back to their pre-merge
// bytes, then clears the merge-state files. Returns an error rather than resetting to a
// wrong/empty index when snapshot is nil (a defensive check — this is a programmer
// error, not a real inbound state).
func abortNativeMerge(worktreePath WorktreePath, snapshot *PreMergeIndexSnapshot) error {
	if snapshot == nil {
		return errors.New("abortNativeMerge: pre-merge index snapshot is required")
	}

	touched := make(map[string]bool, len(snapshot.Entries))
	for _, e := range snapshot.Entries {
		touched[e.Name] = true
	}
	if err := writeIndexEntries(string(worktreePath), touched, snapshot.Entries); err != nil {
		return fmt.Errorf("abortNativeMerge: restore index: %w", err)
	}

	for path, content := range snapshot.Content {
		if err := restoreWorkingTreeFile(worktreePath, path, content); err != nil {
			return fmt.Errorf("abortNativeMerge: restore working tree content for %q: %w", path, err)
		}
	}

	return clearMergeStateFiles(worktreePath)
}

// writeRefWithLockSentinel advances an *existing* branch ref to newHash using real git's
// own lockfile-presence protocol (create "<refPath>.lock" exclusively, write, rename onto
// refPath, fsync the containing directory) instead of go-git's bare SetReference (Story
// 2.5.3, Task 2.5.3b).
//
// Why this exists, and why only here: reading the pinned v5.19.2 source directly
// (storage/filesystem/dotgit/dotgit_setref.go's setRefRwfs) shows go-git's SetReference
// opens refPath in place (O_TRUNC, truncating immediately at open) and writes the new
// content in a separate, later syscall — never renaming a fully-written temp file over
// it. Its only concurrency guard is an advisory flock a real `git` process never
// participates in at all (git's own ref-write protocol is presence-of-a-"<ref>.lock"-file,
// not flock; confirmed via strace of `git update-ref`). TestNativeRefWrite_
// UnprotectedRace_CanCorruptRef proves the resulting window is real: a concurrent reader
// can observe refPath truncated to empty mid-write. Real git's own lock+rename protocol
// never exposes that window — a reader always sees either the fully-old or fully-new
// value — so replicating that exact protocol (not just adding our own flock) is what
// closes the gap.
//
// Per Story 2.5.3's resolution of the architecture.md/pitfalls.md contradiction (see
// plan.md Epic 2.5 and ADR-001's Update), this is required only for the merge ref-advance
// (Task 3.4.1b) — an already-existing branch ref a fix-agent's own subprocess `git
// merge`/`git rebase`, or CheckoutBranch, can plausibly touch during the same session's
// lifetime. Task 2.1.2a's fresh branch-ref creation in Add is unaffected: the ref doesn't
// exist until this project's own code creates it, so deferring the lock-sentinel there
// stands.
//
// Fails (does not overwrite, does not retry) if "<refPath>.lock" already exists — matching
// real git's own collision behavior, which reports a similar "unable to create ... File
// exists" error rather than silently clobbering a concurrent writer's in-progress lock.
// Callers that need retry-on-collision compose it themselves, the same way real git's own
// callers do.
func writeRefWithLockSentinel(refPath string, newHash plumbing.Hash) (err error) {
	lockPath := refPath + ".lock"

	f, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
	if err != nil {
		return fmt.Errorf("writeRefWithLockSentinel: failed to acquire lock %q: %w", lockPath, err)
	}
	defer func() {
		if err != nil {
			_ = f.Close()
			_ = os.Remove(lockPath)
		}
	}()

	if _, err = f.WriteString(newHash.String() + "\n"); err != nil {
		return fmt.Errorf("writeRefWithLockSentinel: failed to write %q: %w", lockPath, err)
	}
	if err = f.Sync(); err != nil {
		return fmt.Errorf("writeRefWithLockSentinel: failed to fsync %q: %w", lockPath, err)
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("writeRefWithLockSentinel: failed to close %q: %w", lockPath, err)
	}

	if err = os.Rename(lockPath, refPath); err != nil {
		return fmt.Errorf("writeRefWithLockSentinel: failed to rename %q onto %q: %w", lockPath, refPath, err)
	}

	dir, err := os.Open(filepath.Dir(refPath))
	if err != nil {
		return fmt.Errorf("writeRefWithLockSentinel: failed to open %q for directory fsync: %w", filepath.Dir(refPath), err)
	}
	defer func() { _ = dir.Close() }()
	if err = dir.Sync(); err != nil {
		return fmt.Errorf("writeRefWithLockSentinel: failed to fsync directory %q after renaming %q: %w", filepath.Dir(refPath), refPath, err)
	}

	return nil
}

// validateTreeEntryRelPath rejects a git tree-entry name that a raw tree object is never
// required to satisfy at the object-format level — verified against the pinned
// go-git/go-git/v5@v5.19.2 source: Tree.Decode performs none of real git's own
// verify_path() checks (no "..", no absolute path, no ".git" component). go-git's own
// Worktree.Checkout gets this for free from its chrooted billy.Filesystem; this package's
// merge-materialization code writes through raw os.* calls instead (see
// writeWorkingTreeFile's doc comment) and so must enforce it itself. Mirrors verify_path's
// checks closely enough to close the exploit class: absolute paths, "." / ".." segments
// (via filepath.Clean), and any ".git" path component are rejected outright.
func validateTreeEntryRelPath(relPath string) error {
	if relPath == "" {
		return errors.New("validateTreeEntryRelPath: empty tree entry path")
	}
	if filepath.IsAbs(relPath) {
		return fmt.Errorf("validateTreeEntryRelPath: tree entry path %q is absolute", relPath)
	}

	cleaned := filepath.Clean(relPath)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return fmt.Errorf("validateTreeEntryRelPath: tree entry path %q escapes the worktree", relPath)
	}
	for _, seg := range strings.Split(filepath.ToSlash(cleaned), "/") {
		if seg == ".git" {
			return fmt.Errorf("validateTreeEntryRelPath: tree entry path %q targets a .git component", relPath)
		}
	}
	return nil
}

// secureWorktreeJoin validates relPath via validateTreeEntryRelPath and joins it onto
// worktreePath. Every working-tree read/write in this file that takes a path straight from
// a git tree entry (rather than an already-trusted, program-controlled constant) must go
// through this rather than a bare filepath.Join — the sole containment point for
// writeWorkingTreeFile/removeWorkingTreeFile/restoreWorkingTreeFile/capturePreMergeSnapshot.
func secureWorktreeJoin(worktreePath WorktreePath, relPath string) (string, error) {
	if err := validateTreeEntryRelPath(relPath); err != nil {
		return "", fmt.Errorf("secureWorktreeJoin: %w", err)
	}
	return filepath.Join(string(worktreePath), relPath), nil
}

// clearBlockingSymlinksInPath mirrors go-git's own Worktree.clearBlockingSymlinks
// (worktree.go in the pinned v5.19.2): a symlink planted in a leading path component of
// fullPath (e.g. "s" while writing "s/config", where "s" links outside worktreePath) would
// otherwise be transparently followed by the caller's subsequent os.MkdirAll/os.WriteFile/
// os.Symlink/os.ReadFile, defeating secureWorktreeJoin's path-string containment check even
// though the string itself is clean. Removing it is always correct: a symlink can never
// legitimately be an intermediate directory of a tracked path. Only walks between
// worktreePath and fullPath's parent, never above worktreePath — secureWorktreeJoin already
// guarantees fullPath is a lexical descendant of worktreePath before this is ever called.
//
// Every one of writeWorkingTreeFile/removeWorkingTreeFile/restoreWorkingTreeFile/
// capturePreMergeSnapshot calls this immediately after secureWorktreeJoin and before
// touching disk — capturePreMergeSnapshot's os.ReadFile is a read, not a write, but a
// blocking symlink there is just as dangerous: it would read (and then, via
// abortNativeMerge's restoreWorkingTreeFile, write straight back into the worktree)
// arbitrary content from outside worktreePath, an information-disclosure primitive on top
// of the write-side escape the other three functions close.
func clearBlockingSymlinksInPath(worktreePath WorktreePath, fullPath string) error {
	rel, err := filepath.Rel(string(worktreePath), filepath.Dir(fullPath))
	if err != nil {
		return fmt.Errorf("clearBlockingSymlinksInPath: %w", err)
	}
	if rel == "." {
		return nil
	}

	dir := string(worktreePath)
	for _, seg := range strings.Split(rel, string(filepath.Separator)) {
		dir = filepath.Join(dir, seg)
		info, err := os.Lstat(dir)
		if err != nil {
			if os.IsNotExist(err) {
				// A missing leading component is created as a real directory by the
				// caller's own os.MkdirAll — nothing to clear yet.
				return nil
			}
			return fmt.Errorf("clearBlockingSymlinksInPath: failed to stat %q: %w", dir, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if err := os.Remove(dir); err != nil {
				return fmt.Errorf("clearBlockingSymlinksInPath: failed to remove blocking symlink %q: %w", dir, err)
			}
			// Everything beneath the removed symlink went with it.
			return nil
		}
	}
	return nil
}

// restoreWorkingTreeFile overwrites path (relative to worktreePath) with content,
// preserving the file's existing permission bits if it still exists (falling back to
// 0o644 otherwise — e.g. if materializeConflictOnAbort's marker write itself failed
// part-way through and the file is missing).
func restoreWorkingTreeFile(worktreePath WorktreePath, path string, content []byte) error {
	fullPath, err := secureWorktreeJoin(worktreePath, path)
	if err != nil {
		return fmt.Errorf("restoreWorkingTreeFile: %w", err)
	}
	if err := clearBlockingSymlinksInPath(worktreePath, fullPath); err != nil {
		return fmt.Errorf("restoreWorkingTreeFile: %w", err)
	}
	mode := os.FileMode(0o644)
	if info, err := os.Stat(fullPath); err == nil {
		mode = info.Mode()
	}
	return os.WriteFile(fullPath, content, mode)
}

// nativeMergeMainIntoWorktreeLocked wraps nativeMergeMainIntoWorktree in
// WithRepoWorktreeLock, keyed on the same repo path Setup()/removeLocked()/pruneLocked()
// already serialize their own dispatch through (worktree_ops.go). Without this,
// nativeMergeMainIntoWorktree ran with zero synchronization against a concurrent
// nativeSetupNewWorktree/nativeRemoveWorktree/nativeWorktreePrune on the same repo, or a
// second concurrent merge — a real gap against this project's own ADR-001/plan.md claim
// that WithRepoWorktreeLock is the load-bearing safety net making a flag flip mid-burst
// safe.
//
// KNOWN GAP (PR #730 Gate 2 review): a conflicted merge re-opens the repo (openWorktreeRepo)
// several times across this call tree instead of threading one already-open
// *git.Repository through — real but a larger refactor, tracked as a follow-up.
func nativeMergeMainIntoWorktreeLocked(worktreePath, mainBranch string) (*MergeMainResult, error) {
	repoPath, err := repoPathForWorktree(WorktreePath(worktreePath))
	if err != nil {
		return nil, fmt.Errorf("nativeMergeMainIntoWorktreeLocked: %w", err)
	}

	var result *MergeMainResult
	lockErr := WithRepoWorktreeLock(repoPath, func() error {
		var mergeErr error
		result, mergeErr = nativeMergeMainIntoWorktree(worktreePath, mainBranch)
		return mergeErr
	})
	return result, lockErr
}

// repoPathForWorktree resolves the main repository's working-directory path from a
// worktree (or main-checkout) path — the same repoPath value GitWorktree carries
// internally and locks worktree lifecycle operations against — by taking the parent of
// worktreeCommonGitDir's shared `.git` admin directory. Needed because
// MergeMainIntoWorktree only ever receives worktreePath, never a separate repoPath.
func repoPathForWorktree(worktreePath WorktreePath) (string, error) {
	commonGitDir, err := worktreeCommonGitDir(worktreePath)
	if err != nil {
		return "", fmt.Errorf("repoPathForWorktree: %w", err)
	}
	return filepath.Dir(commonGitDir), nil
}

// nativeMergeMainIntoWorktree is MergeMainIntoWorktree's pure-Go, go-git-based
// implementation (Epic 3.4), dispatched to by the public MergeMainIntoWorktree when
// useNativeMerge(worktreePath) is true (ADR-002). It fetches mainBranch from origin (the
// one subprocess call this path still makes — FetchBranch, shared with the legacy path),
// then resolves the merge outcome purely via go-git object reads: up-to-date and
// fast-forward are ancestor checks (Task 3.4.1a), a real divergence runs the diff3
// pipeline assembled across Epics 3.1-3.3 and either produces a real two-parent merge
// commit (Task 3.4.1b) or materializes conflict markers/index and immediately aborts
// (Task 3.4.1c) — MergeMainResult's semantics match legacyMergeMainIntoWorktree's exactly
// so every real call site (drift.go, backlog_service_triage.go,
// session.backlog_lifecycle.go's branchReconciler) behaves identically either way.
func nativeMergeMainIntoWorktree(worktreePath, mainBranch string) (*MergeMainResult, error) {
	if err := FetchBranch(worktreePath, mainBranch); err != nil {
		return nil, fmt.Errorf("nativeMergeMainIntoWorktree: failed to fetch %s: %w", mainBranch, err)
	}

	repo, ours, theirs, err := resolveMergeEndpoints(WorktreePath(worktreePath), BranchName(mainBranch))
	if err != nil {
		return nil, fmt.Errorf("nativeMergeMainIntoWorktree: %w", err)
	}

	// Up to date: everything on mainBranch is already reachable from ours (including the
	// trivial ours == theirs case — IsAncestor's preorder walk visits its starting commit
	// first, per plumbing/object/merge_base.go). Checked before any dirty-worktree
	// validation — matching real git, an up-to-date merge touches nothing on disk and
	// succeeds regardless of how dirty the working tree is (verified empirically: `git
	// merge` on an already-current branch reports "Already up to date." even with
	// uncommitted tracked and untracked changes present).
	upToDate, err := theirs.IsAncestor(ours)
	if err != nil {
		return nil, fmt.Errorf("nativeMergeMainIntoWorktree: failed to check ancestry (up-to-date): %w", err)
	}
	if upToDate {
		recordMergeOutcome(mergeOutcomeUpToDate)
		return &MergeMainResult{UpToDate: true}, nil
	}

	// Refuse only if the merge would actually overwrite something — see
	// refuseIfMergeWouldOverwriteWorkingTree's doc comment for why this isn't a blanket
	// status.IsClean() check.
	if err := refuseIfMergeWouldOverwriteWorkingTree(repo, ours, theirs, WorktreePath(worktreePath), BranchName(mainBranch)); err != nil {
		return nil, fmt.Errorf("nativeMergeMainIntoWorktree: %w", err)
	}

	refPath, err := checkedOutBranchRefPath(WorktreePath(worktreePath), repo)
	if err != nil {
		return nil, fmt.Errorf("nativeMergeMainIntoWorktree: failed to resolve checked-out branch ref: %w", err)
	}

	// Fast-forward: ours has made no divergent history of its own — theirs' tree can be
	// materialized directly with no merge commit, matching real git's own default
	// fast-forward behavior for `git merge`.
	fastForward, err := ours.IsAncestor(theirs)
	if err != nil {
		return nil, fmt.Errorf("nativeMergeMainIntoWorktree: failed to check ancestry (fast-forward): %w", err)
	}
	if fastForward {
		if err := nativeFastForwardMerge(WorktreePath(worktreePath), ours, theirs, refPath); err != nil {
			return nil, fmt.Errorf("nativeMergeMainIntoWorktree: fast-forward: %w", err)
		}
		recordMergeOutcome(mergeOutcomeFastForward)
		return &MergeMainResult{Merged: true}, nil
	}

	mc := threeWayMergeContext{
		worktreePath: WorktreePath(worktreePath),
		repo:         repo,
		mainBranch:   BranchName(mainBranch),
		refPath:      refPath,
	}
	return nativeThreeWayMerge(mc, ours, theirs)
}

// resolveMergeEndpoints opens worktreePath's repo and resolves the two commits
// nativeMergeMainIntoWorktree compares: ours (the checked-out HEAD) and theirs (mainBranch's
// freshly-fetched origin tip). Split out of nativeMergeMainIntoWorktree purely to keep that
// function's own body under this repo's long-function guideline — no behavioral change.
func resolveMergeEndpoints(worktreePath WorktreePath, mainBranch BranchName) (repo *git.Repository, ours, theirs *object.Commit, err error) {
	repo, err = openWorktreeRepo(string(worktreePath))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to open repo at %s: %w", worktreePath, err)
	}

	oursSHA, err := getHeadCommitSHA(string(worktreePath))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to resolve HEAD: %w", err)
	}
	ours, err = repo.CommitObject(plumbing.NewHash(oursSHA))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to resolve ours commit %s: %w", oursSHA, err)
	}

	theirsRef, err := repo.Reference(plumbing.NewRemoteReferenceName("origin", string(mainBranch)), true)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to resolve origin/%s: %w", mainBranch, err)
	}
	theirs, err = repo.CommitObject(theirsRef.Hash())
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to resolve theirs commit %s: %w", theirsRef.Hash(), err)
	}
	return repo, ours, theirs, nil
}

// refuseIfMergeWouldOverwriteWorkingTree matches real git merge's own refusal semantics
// (verified empirically against a real `git merge` subprocess) — refuses only if a path
// the ours..theirs diff touches is also dirty/untracked in the working tree, never on
// unrelated dirty state elsewhere in the worktree. A blanket status.IsClean() check here
// used to be a false-positive trap: every real session worktree carries incidental
// untracked files (.mcp.json, .claude/settings.local.json) that have nothing to do with
// the paths being merged. Split out of nativeMergeMainIntoWorktree purely to keep that
// function's own body under this repo's long-function guideline — no behavioral change.
func refuseIfMergeWouldOverwriteWorkingTree(repo *git.Repository, ours, theirs *object.Commit, worktreePath WorktreePath, mainBranch BranchName) error {
	oursTree, err := ours.Tree()
	if err != nil {
		return fmt.Errorf("failed to resolve ours tree: %w", err)
	}
	theirsTree, err := theirs.Tree()
	if err != nil {
		return fmt.Errorf("failed to resolve theirs tree: %w", err)
	}
	changes, err := oursTree.Diff(theirsTree)
	if err != nil {
		return fmt.Errorf("failed to diff ours..theirs: %w", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree at %s: %w", worktreePath, err)
	}
	status, err := wt.Status()
	if err != nil {
		return fmt.Errorf("failed to check worktree status at %s: %w", worktreePath, err)
	}
	if conflictPath, blocked := worktreeBlocksMerge(status, changes); blocked {
		return fmt.Errorf("%q at %s has local changes that would be overwritten; refusing to merge %s (matches legacy git merge's refusal)", conflictPath, worktreePath, mainBranch)
	}
	return nil
}

// worktreeBlocksMerge reports whether status has any dirty entry at a path changes
// touches — the exact condition real git's own merge refuses on ("local changes ... would
// be overwritten" for a tracked modification, "untracked working tree files would be
// overwritten" for an untracked one). changes is ours..theirs (an Insert/Delete/Modify at
// path p means the merge is about to write or remove p's content), which is a safe
// superset of what a three-way merge can touch too: if ours and theirs already agree at a
// path, nothing needs to move regardless of algorithm, so it's absent from changes and
// correctly never checked. Any status entry NOT on a touched path — the common case for a
// real session worktree's incidental untracked files (.mcp.json, .claude/settings.local.json)
// — is intentionally ignored, unlike a blanket status.IsClean() check.
func worktreeBlocksMerge(status git.Status, changes object.Changes) (path string, blocked bool) {
	touched := make(map[string]bool, len(changes)*2)
	for _, c := range changes {
		if c.From.Name != "" {
			touched[c.From.Name] = true
		}
		if c.To.Name != "" {
			touched[c.To.Name] = true
		}
	}

	for p := range touched {
		fileStatus, hasEntry := status[p]
		if !hasEntry {
			continue
		}
		if fileStatus.Staging != git.Unmodified || fileStatus.Worktree != git.Unmodified {
			return p, true
		}
	}
	return "", false
}

// checkedOutBranchRefPath resolves the on-disk file path of the branch ref currently
// checked out at worktreePath — the refPath writeRefWithLockSentinel advances for both
// the fast-forward and clean-merge paths (Task 3.4.1b). Errors on a detached HEAD: every
// real call site (drift.go, backlog_service_triage.go, branchReconciler) always operates
// on a named branch a backlog work session or fix agent has checked out, never a detached
// commit.
func checkedOutBranchRefPath(worktreePath WorktreePath, repo *git.Repository) (string, error) {
	head, err := repo.Reference(plumbing.HEAD, false)
	if err != nil {
		return "", fmt.Errorf("checkedOutBranchRefPath: failed to resolve HEAD: %w", err)
	}
	if head.Type() != plumbing.SymbolicReference {
		return "", fmt.Errorf("checkedOutBranchRefPath: HEAD at %s is detached, expected a branch checkout", worktreePath)
	}

	commonDir, err := worktreeCommonGitDir(worktreePath)
	if err != nil {
		return "", fmt.Errorf("checkedOutBranchRefPath: %w", err)
	}
	return filepath.Join(commonDir, filepath.FromSlash(head.Target().String())), nil
}

// worktreeCommonGitDir resolves the shared git directory that holds refs/heads for
// worktreePath: worktreeGitDir itself for the main working copy, or the target of its
// admin dir's "commondir" file (written by nativeSetupNewWorktree, always "../..") for a
// linked worktree — refs/heads is shared across every worktree of a repo, unlike a
// worktree's own private admin dir.
func worktreeCommonGitDir(worktreePath WorktreePath) (string, error) {
	gitDir, err := worktreeGitDir(worktreePath)
	if err != nil {
		return "", fmt.Errorf("worktreeCommonGitDir: %w", err)
	}

	content, err := os.ReadFile(filepath.Join(gitDir, "commondir"))
	if err != nil {
		if os.IsNotExist(err) {
			// Main working copy: gitDir already IS the common dir.
			return gitDir, nil
		}
		return "", fmt.Errorf("worktreeCommonGitDir: failed to read commondir file: %w", err)
	}
	return filepath.Clean(filepath.Join(gitDir, strings.TrimSpace(string(content)))), nil
}

// nativeFastForwardMerge materializes theirs' tree directly into worktreePath's working
// tree and index (Task 3.4.1a's fast-forward short-circuit), then advances the checked-out
// branch ref to theirs via writeRefWithLockSentinel (Task 3.4.1b) rather than any bare
// go-git SetReference — see that function's doc comment for why this ref specifically
// requires it.
func nativeFastForwardMerge(worktreePath WorktreePath, ours, theirs *object.Commit, refPath string) error {
	oursTree, err := ours.Tree()
	if err != nil {
		return fmt.Errorf("nativeFastForwardMerge: failed to resolve ours tree: %w", err)
	}
	theirsTree, err := theirs.Tree()
	if err != nil {
		return fmt.Errorf("nativeFastForwardMerge: failed to resolve theirs tree: %w", err)
	}

	changes, err := oursTree.Diff(theirsTree)
	if err != nil {
		return fmt.Errorf("nativeFastForwardMerge: failed to diff ours..theirs: %w", err)
	}
	if err := materializeTreeChanges(worktreePath, changes); err != nil {
		return fmt.Errorf("nativeFastForwardMerge: failed to materialize working tree: %w", err)
	}

	if err := writeRefWithLockSentinel(refPath, theirs.Hash); err != nil {
		return fmt.Errorf("nativeFastForwardMerge: failed to advance branch ref: %w", err)
	}
	return nil
}

// materializeTreeChanges applies changes (an ours→theirs tree diff) directly to
// worktreePath's working tree and index: every changed path is either removed (Delete) or
// overwritten with theirs' blob content and mode (Insert/Modify), then the index is
// updated in one writeIndexEntries call — the same primitive writeConflictedIndex uses,
// so every index mutation in this package funnels through one atomic writer (ADR-001). A
// Modify whose From/To names differ is a detected rename (Tree.Diff runs with
// DetectRenames on by default) — the old path is removed too, or fast-forwarding would
// leave a stale duplicate file behind.
func materializeTreeChanges(worktreePath WorktreePath, changes object.Changes) error {
	touched := make(map[string]bool, len(changes))
	var newEntries []*index.Entry

	for _, c := range changes {
		action, err := c.Action()
		if err != nil {
			return fmt.Errorf("materializeTreeChanges: failed to determine action: %w", err)
		}

		if action == merkletrie.Delete {
			touched[c.From.Name] = true
			if err := removeWorkingTreeFile(worktreePath, c.From.Name); err != nil {
				return fmt.Errorf("materializeTreeChanges: failed to remove %q: %w", c.From.Name, err)
			}
			continue
		}

		if c.From.Name != "" && c.From.Name != c.To.Name {
			touched[c.From.Name] = true
			if err := removeWorkingTreeFile(worktreePath, c.From.Name); err != nil {
				return fmt.Errorf("materializeTreeChanges: failed to remove renamed-away %q: %w", c.From.Name, err)
			}
		}

		entryPath := c.To.Name
		mode := c.To.TreeEntry.Mode
		touched[entryPath] = true
		newEntries = append(newEntries, &index.Entry{Name: entryPath, Hash: c.To.TreeEntry.Hash, Mode: mode})

		if !mode.IsFile() {
			// A gitlink (submodule): no blob content to write to the working tree, only
			// the index entry above records its commit-SHA pointer.
			continue
		}
		_, to, err := c.Files()
		if err != nil {
			return fmt.Errorf("materializeTreeChanges: failed to read blob for %q: %w", entryPath, err)
		}
		content, err := to.Contents()
		if err != nil {
			return fmt.Errorf("materializeTreeChanges: failed to read content for %q: %w", entryPath, err)
		}
		if err := writeWorkingTreeFile(worktreePath, entryPath, mode, []byte(content)); err != nil {
			return fmt.Errorf("materializeTreeChanges: failed to write %q: %w", entryPath, err)
		}
	}

	return writeIndexEntries(string(worktreePath), touched, newEntries)
}

// writeWorkingTreeFile writes content to relPath (relative to worktreePath) with the
// permission bits mode implies, creating any missing parent directories. A Symlink mode
// writes content as the link target rather than a regular file's bytes.
//
// relPath comes straight from a git tree-entry name (GroupChangesByPath's
// object.Change.To.Name/From.Name), which a malicious commit fully controls — go-git's
// Tree.Decode never validates it (see validateTreeEntryRelPath's doc comment). Unlike
// go-git's own Worktree.Checkout, this function writes through raw os.* calls with no
// chrooted filesystem underneath it, so secureWorktreeJoin/clearBlockingSymlinksInPath are
// this function's only containment.
func writeWorkingTreeFile(worktreePath WorktreePath, relPath string, mode filemode.FileMode, content []byte) error {
	fullPath, err := secureWorktreeJoin(worktreePath, relPath)
	if err != nil {
		return fmt.Errorf("writeWorkingTreeFile: %w", err)
	}
	if err := clearBlockingSymlinksInPath(worktreePath, fullPath); err != nil {
		return fmt.Errorf("writeWorkingTreeFile: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o750); err != nil {
		return fmt.Errorf("writeWorkingTreeFile: failed to create parent directory for %q: %w", relPath, err)
	}

	if mode == filemode.Symlink {
		// os.Symlink refuses to overwrite an existing entry, so clear it first — but only
		// swallow "doesn't exist"; any other Remove failure (e.g. permission denied) would
		// otherwise surface as a misleading "file exists" error from Symlink below instead
		// of its real cause.
		if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("writeWorkingTreeFile: failed to remove existing entry at %q before writing symlink: %w", relPath, err)
		}
		return os.Symlink(string(content), fullPath)
	}

	perm := os.FileMode(0o644)
	if mode == filemode.Executable {
		perm = 0o755
	}
	return os.WriteFile(fullPath, content, perm)
}

// removeWorkingTreeFile removes relPath (relative to worktreePath), treating an
// already-absent file as success — the same "ignore not-exist" convention
// clearMergeStateFiles uses. See writeWorkingTreeFile's doc comment for why relPath needs
// secureWorktreeJoin/clearBlockingSymlinksInPath before ever touching disk: a leading
// symlink planted by an earlier malicious entry in the same tree would otherwise let this
// delete a file outside worktreePath entirely.
func removeWorkingTreeFile(worktreePath WorktreePath, relPath string) error {
	fullPath, err := secureWorktreeJoin(worktreePath, relPath)
	if err != nil {
		return fmt.Errorf("removeWorkingTreeFile: %w", err)
	}
	if err := clearBlockingSymlinksInPath(worktreePath, fullPath); err != nil {
		return fmt.Errorf("removeWorkingTreeFile: %w", err)
	}
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
