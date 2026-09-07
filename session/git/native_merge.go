package git

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/utils/merkletrie"
)

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

// worktreeGitDir returns the .git admin directory that applies to worktreePath (a real
// directory for the main working copy, or a linked worktree's private admin dir) — the
// same directory resolveWorktreeIndexPath resolves the index file into, minus the
// "index" filename itself.
func worktreeGitDir(worktreePath string) (string, error) {
	indexPath, err := resolveWorktreeIndexPath(worktreePath)
	if err != nil {
		return "", fmt.Errorf("worktreeGitDir: %w", err)
	}
	return filepath.Dir(indexPath), nil
}

// writeMergeStateFiles writes the minimum real-git merge-state files
// (MERGE_HEAD/MERGE_MSG/MERGE_MODE) so a worktree mid-native-merge is recognizable by,
// and abortable via, a real `git merge --abort` fallback (Story 3.3.3,
// architecture.md §3's abort-compatibility gap).
func writeMergeStateFiles(worktreePath, theirsCommitSHA, mainBranch string) error {
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
func clearMergeStateFiles(worktreePath string) error {
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
func abortNativeMerge(worktreePath string, snapshot *PreMergeIndexSnapshot) error {
	if snapshot == nil {
		return errors.New("abortNativeMerge: pre-merge index snapshot is required")
	}

	touched := make(map[string]bool, len(snapshot.Entries))
	for _, e := range snapshot.Entries {
		touched[e.Name] = true
	}
	if err := writeIndexEntries(worktreePath, touched, snapshot.Entries); err != nil {
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

// restoreWorkingTreeFile overwrites path (relative to worktreePath) with content,
// preserving the file's existing permission bits if it still exists (falling back to
// 0o644 otherwise — e.g. if materializeConflictOnAbort's marker write itself failed
// part-way through and the file is missing).
func restoreWorkingTreeFile(worktreePath, path string, content []byte) error {
	fullPath := filepath.Join(worktreePath, path)
	mode := os.FileMode(0o644)
	if info, err := os.Stat(fullPath); err == nil {
		mode = info.Mode()
	}
	return os.WriteFile(fullPath, content, mode)
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

	repo, err := openWorktreeRepo(worktreePath)
	if err != nil {
		return nil, fmt.Errorf("nativeMergeMainIntoWorktree: failed to open repo at %s: %w", worktreePath, err)
	}

	oursSHA, err := getHeadCommitSHA(worktreePath)
	if err != nil {
		return nil, fmt.Errorf("nativeMergeMainIntoWorktree: failed to resolve HEAD: %w", err)
	}
	ours, err := repo.CommitObject(plumbing.NewHash(oursSHA))
	if err != nil {
		return nil, fmt.Errorf("nativeMergeMainIntoWorktree: failed to resolve ours commit %s: %w", oursSHA, err)
	}

	theirsRef, err := repo.Reference(plumbing.NewRemoteReferenceName("origin", mainBranch), true)
	if err != nil {
		return nil, fmt.Errorf("nativeMergeMainIntoWorktree: failed to resolve origin/%s: %w", mainBranch, err)
	}
	theirs, err := repo.CommitObject(theirsRef.Hash())
	if err != nil {
		return nil, fmt.Errorf("nativeMergeMainIntoWorktree: failed to resolve theirs commit %s: %w", theirsRef.Hash(), err)
	}

	// Up to date: everything on mainBranch is already reachable from ours (including the
	// trivial ours == theirs case — IsAncestor's preorder walk visits its starting commit
	// first, per plumbing/object/merge_base.go).
	upToDate, err := theirs.IsAncestor(ours)
	if err != nil {
		return nil, fmt.Errorf("nativeMergeMainIntoWorktree: failed to check ancestry (up-to-date): %w", err)
	}
	if upToDate {
		return &MergeMainResult{UpToDate: true}, nil
	}

	refPath, err := checkedOutBranchRefPath(worktreePath, repo)
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
		if err := nativeFastForwardMerge(worktreePath, ours, theirs, refPath); err != nil {
			return nil, fmt.Errorf("nativeMergeMainIntoWorktree: fast-forward: %w", err)
		}
		return &MergeMainResult{Merged: true}, nil
	}

	return nativeThreeWayMerge(worktreePath, repo, ours, theirs, mainBranch, refPath)
}

// checkedOutBranchRefPath resolves the on-disk file path of the branch ref currently
// checked out at worktreePath — the refPath writeRefWithLockSentinel advances for both
// the fast-forward and clean-merge paths (Task 3.4.1b). Errors on a detached HEAD: every
// real call site (drift.go, backlog_service_triage.go, branchReconciler) always operates
// on a named branch a backlog work session or fix agent has checked out, never a detached
// commit.
func checkedOutBranchRefPath(worktreePath string, repo *git.Repository) (string, error) {
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
func worktreeCommonGitDir(worktreePath string) (string, error) {
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
func nativeFastForwardMerge(worktreePath string, ours, theirs *object.Commit, refPath string) error {
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
func materializeTreeChanges(worktreePath string, changes object.Changes) error {
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

	return writeIndexEntries(worktreePath, touched, newEntries)
}

// writeWorkingTreeFile writes content to relPath (relative to worktreePath) with the
// permission bits mode implies, creating any missing parent directories. A Symlink mode
// writes content as the link target rather than a regular file's bytes.
func writeWorkingTreeFile(worktreePath, relPath string, mode filemode.FileMode, content []byte) error {
	fullPath := filepath.Join(worktreePath, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o750); err != nil {
		return fmt.Errorf("writeWorkingTreeFile: failed to create parent directory for %q: %w", relPath, err)
	}

	if mode == filemode.Symlink {
		_ = os.Remove(fullPath) // os.Symlink refuses to overwrite an existing entry.
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
// clearMergeStateFiles uses.
func removeWorkingTreeFile(worktreePath, relPath string) error {
	if err := os.Remove(filepath.Join(worktreePath, relPath)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// resolvedPathMerge is one non-conflicted path's outcome from resolveMergePaths: either a
// deletion, or new content+mode to write and index.
type resolvedPathMerge struct {
	path    string
	deleted bool
	mode    filemode.FileMode
	content []byte
}

// conflictedPathMerge is one conflicted path's outcome from resolveMergePaths: the full
// hunk sequence (resolved and conflicting regions interleaved, per ThreeWayFileMerger),
// plus the three-stage blob hashes NewConflictEntries needs.
type conflictedPathMerge struct {
	path                           string
	mode                           filemode.FileMode
	baseHash, oursHash, theirsHash plumbing.Hash
	hunks                          []MergeHunk
}

// mergePathResolution is resolveMergePaths' result: every touched path classified as
// either cleanly resolved or conflicted.
type mergePathResolution struct {
	resolved  []resolvedPathMerge
	conflicts []conflictedPathMerge
}

// resolveMergePaths runs the diff3 pipeline (Epics 3.1-3.2) over every path either side
// touched relative to base, in deterministic (sorted) path order so ConflictedFiles and
// the resulting tree are reproducible rather than dependent on Go's randomized map
// iteration order. It also detects a plain rename (a Modify Change whose From.Name the
// resulting path map never independently addresses) and schedules the old path for
// removal — GroupChangesByPath keys a rename solely by its new name, so without this the
// old path's stale index/working-tree entry would survive the merge.
func resolveMergePaths(byPath map[string]*PathChange) (*mergePathResolution, error) {
	paths := make([]string, 0, len(byPath))
	for p := range byPath {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	merger := ThreeWayFileMerger{}
	res := &mergePathResolution{}
	renamedAway := map[string]bool{}
	for _, p := range paths {
		pc := byPath[p]
		for _, c := range [...]*object.Change{pc.Ours, pc.Theirs} {
			if c != nil && c.From.Name != "" && c.From.Name != p {
				renamedAway[c.From.Name] = true
			}
		}

		deleted, err := pathChangeIsDelete(pc)
		if err != nil {
			return nil, fmt.Errorf("resolveMergePaths: failed to classify %q: %w", p, err)
		}
		if deleted {
			res.resolved = append(res.resolved, resolvedPathMerge{path: p, deleted: true})
			continue
		}

		mode, err := pathChangeMode(pc)
		if err != nil {
			return nil, fmt.Errorf("resolveMergePaths: failed to determine mode for %q: %w", p, err)
		}

		result, err := merger.ReconcilePathChange(pc)
		if err != nil {
			return nil, fmt.Errorf("resolveMergePaths: failed to reconcile %q: %w", p, err)
		}

		if result.Conflicted {
			base, ours, theirs, hashErr := conflictHashes(pc)
			if hashErr != nil {
				return nil, fmt.Errorf("resolveMergePaths: failed to resolve conflict blob hashes for %q: %w", p, hashErr)
			}
			res.conflicts = append(res.conflicts, conflictedPathMerge{
				path: p, mode: mode,
				baseHash: base, oursHash: ours, theirsHash: theirs,
				hunks: result.Hunks,
			})
			continue
		}

		res.resolved = append(res.resolved, resolvedPathMerge{path: p, mode: mode, content: []byte(result.Content)})
	}

	oldPaths := make([]string, 0, len(renamedAway))
	for old := range renamedAway {
		if _, stillAddressed := byPath[old]; stillAddressed {
			continue // the old path is itself independently addressed elsewhere
		}
		oldPaths = append(oldPaths, old)
	}
	sort.Strings(oldPaths)
	for _, old := range oldPaths {
		res.resolved = append(res.resolved, resolvedPathMerge{path: old, deleted: true})
	}

	return res, nil
}

// pathChangeIsDelete reports whether pc's resolution is an outright deletion: the side(s)
// that touched the path all deleted it, rather than modifying it. Computed independently
// of ReconcilePathChange's own delete handling because ReconcilePathChange conflates "the
// only side that touched this path deleted it" with "that side left empty content" (its
// singleSideResult helper reads changeContents' empty toContent for a Delete the same way
// it would read a genuinely emptied file) — this project's assembly layer needs the two
// told apart so a deleted path is removed from disk/index, not overwritten with an empty
// file.
func pathChangeIsDelete(pc *PathChange) (bool, error) {
	oursDeleted, err := changeIsDelete(pc.Ours)
	if err != nil {
		return false, fmt.Errorf("pathChangeIsDelete: failed to classify ours: %w", err)
	}
	theirsDeleted, err := changeIsDelete(pc.Theirs)
	if err != nil {
		return false, fmt.Errorf("pathChangeIsDelete: failed to classify theirs: %w", err)
	}
	switch {
	case pc.Ours == nil:
		return theirsDeleted, nil
	case pc.Theirs == nil:
		return oursDeleted, nil
	default:
		return oursDeleted && theirsDeleted, nil
	}
}

// changeIsDelete reports whether c (either side of a PathChange, possibly nil) is a
// Delete action. A nil Change (that side made no change relative to base) is never a
// delete.
func changeIsDelete(c *object.Change) (bool, error) {
	if c == nil {
		return false, nil
	}
	action, err := c.Action()
	if err != nil {
		return false, fmt.Errorf("changeIsDelete: failed to determine action: %w", err)
	}
	return action == merkletrie.Delete, nil
}

// pathChangeMode returns the file mode a non-deleted PathChange's resolved path should
// carry: whichever side made a non-delete change, preferring ours when both did (matching
// ReconcilePathChange's own ours-preference for the rename+modify collision case).
func pathChangeMode(pc *PathChange) (filemode.FileMode, error) {
	for _, c := range [...]*object.Change{pc.Ours, pc.Theirs} {
		if c == nil {
			continue
		}
		action, err := c.Action()
		if err != nil {
			return 0, fmt.Errorf("pathChangeMode: failed to determine action: %w", err)
		}
		if action != merkletrie.Delete {
			return c.To.TreeEntry.Mode, nil
		}
	}
	return filemode.Regular, nil
}

// conflictHashes returns the base/ours/theirs blob hashes NewConflictEntries needs for a
// conflicted PathChange. Safe to assume both sides are non-nil, non-delete Changes here:
// ReconcilePathChange only ever reaches its content-merge branch (the sole source of a
// RegionConflict hunk) once its own nil- and delete-side short-circuits have already ruled
// out every other shape (see ReconcilePathChange's doc comment) — a conflict is only
// signaled by resolveMergePaths after result.Conflicted, which requires that branch to
// have run.
func conflictHashes(pc *PathChange) (base, ours, theirs plumbing.Hash, err error) {
	if pc.Ours == nil || pc.Theirs == nil {
		return plumbing.ZeroHash, plumbing.ZeroHash, plumbing.ZeroHash,
			errors.New("conflictHashes: a real conflict requires both sides to have changed the path")
	}
	return pc.Ours.From.TreeEntry.Hash, pc.Ours.To.TreeEntry.Hash, pc.Theirs.To.TreeEntry.Hash, nil
}

// nativeThreeWayMerge runs the diff3 pipeline over every path base/ours/theirs disagree
// on and either produces a real merge commit (no conflicts) or materializes conflict
// markers and aborts (Tasks 3.4.1b/3.4.1c).
func nativeThreeWayMerge(worktreePath string, repo *git.Repository, ours, theirs *object.Commit, mainBranch, refPath string) (*MergeMainResult, error) {
	base, err := MergeBaseResolver(ours, theirs)
	if err != nil {
		return nil, fmt.Errorf("nativeThreeWayMerge: failed to resolve merge base: %w", err)
	}

	baseTree, err := base.Tree()
	if err != nil {
		return nil, fmt.Errorf("nativeThreeWayMerge: failed to resolve base tree: %w", err)
	}
	oursTree, err := ours.Tree()
	if err != nil {
		return nil, fmt.Errorf("nativeThreeWayMerge: failed to resolve ours tree: %w", err)
	}
	theirsTree, err := theirs.Tree()
	if err != nil {
		return nil, fmt.Errorf("nativeThreeWayMerge: failed to resolve theirs tree: %w", err)
	}

	baseToOurs, baseToTheirs, err := TreeDiffPair(baseTree, oursTree, theirsTree)
	if err != nil {
		return nil, fmt.Errorf("nativeThreeWayMerge: failed to diff trees: %w", err)
	}
	byPath, err := GroupChangesByPath(baseToOurs, baseToTheirs)
	if err != nil {
		return nil, fmt.Errorf("nativeThreeWayMerge: failed to group changes: %w", err)
	}

	resolution, err := resolveMergePaths(byPath)
	if err != nil {
		return nil, fmt.Errorf("nativeThreeWayMerge: failed to resolve paths: %w", err)
	}

	if len(resolution.conflicts) > 0 {
		return materializeConflictAndAbort(worktreePath, resolution.conflicts, theirs.Hash, mainBranch)
	}
	return commitThreeWayMerge(worktreePath, repo, ours.Hash, theirs.Hash, resolution.resolved, refPath, mainBranch)
}

// materializeConflictAndAbort writes every conflicted path's stage-1/2/3 index entries
// and conflict-marker working-tree content plus the merge-state files (Task 3.4.1c, per
// Story 3.3.3's "always materialize, then abort" decision), then immediately calls
// abortNativeMerge to restore the pre-merge state — the worktree ends up exactly as clean
// as a real `git merge --abort` would leave it, verified in
// TestNativeMergeMainIntoWorktree_Conflicted_LeavesWorktreeClean via a real `git status
// --porcelain` subprocess.
func materializeConflictAndAbort(worktreePath string, conflicts []conflictedPathMerge, theirsHash plumbing.Hash, mainBranch string) (*MergeMainResult, error) {
	snapshot, err := capturePreMergeSnapshot(worktreePath, conflicts)
	if err != nil {
		return nil, fmt.Errorf("materializeConflictAndAbort: failed to capture pre-merge snapshot: %w", err)
	}

	theirsLabel := "origin/" + mainBranch
	conflictedFiles := make([]string, 0, len(conflicts))
	var conflictEntries []*index.Entry
	for _, c := range conflicts {
		conflictedFiles = append(conflictedFiles, c.path)
		conflictEntries = append(conflictEntries, NewConflictEntries(c.path, c.baseHash, c.oursHash, c.theirsHash, c.mode)...)
	}
	if err := writeConflictedIndex(worktreePath, conflictEntries); err != nil {
		return nil, fmt.Errorf("materializeConflictAndAbort: failed to write conflicted index: %w", err)
	}

	for _, c := range conflicts {
		content, renderErr := assembleConflictedFileContent(c.hunks, "HEAD", theirsLabel)
		if renderErr != nil {
			return nil, fmt.Errorf("materializeConflictAndAbort: failed to render markers for %q: %w", c.path, renderErr)
		}
		if writeErr := writeWorkingTreeFile(worktreePath, c.path, c.mode, []byte(content)); writeErr != nil {
			return nil, fmt.Errorf("materializeConflictAndAbort: failed to write markers for %q: %w", c.path, writeErr)
		}
	}

	if err := writeMergeStateFiles(worktreePath, theirsHash.String(), mainBranch); err != nil {
		return nil, fmt.Errorf("materializeConflictAndAbort: failed to write merge-state files: %w", err)
	}

	if err := abortNativeMerge(worktreePath, snapshot); err != nil {
		return nil, fmt.Errorf("materializeConflictAndAbort: failed to abort: %w", err)
	}

	return &MergeMainResult{Conflicted: true, ConflictedFiles: conflictedFiles}, nil
}

// capturePreMergeSnapshot reads each conflicted path's current (pre-merge) stage-0 index
// entry and working-tree content, for abortNativeMerge to restore after
// materializeConflictAndAbort's transient write. A path with no pre-merge index entry
// (an add/add conflict neither side's ancestor had) is a known gap: abortNativeMerge's
// own touched-path set is derived solely from snapshot.Entries (see its doc comment), so
// such a path's conflict-stage entries would not be cleared from the index by this call —
// out of scope here since no real call site's tested scenarios (modify/modify conflicts)
// exercise it, but worth naming rather than silently mishandling.
func capturePreMergeSnapshot(worktreePath string, conflicts []conflictedPathMerge) (*PreMergeIndexSnapshot, error) {
	repo, err := openWorktreeRepo(worktreePath)
	if err != nil {
		return nil, fmt.Errorf("capturePreMergeSnapshot: failed to open repo: %w", err)
	}
	idx, err := repo.Storer.Index()
	if err != nil {
		return nil, fmt.Errorf("capturePreMergeSnapshot: failed to load index: %w", err)
	}
	byName := make(map[string]*index.Entry, len(idx.Entries))
	for _, e := range idx.Entries {
		byName[e.Name] = e
	}

	snapshot := &PreMergeIndexSnapshot{Content: make(map[string][]byte, len(conflicts))}
	for _, c := range conflicts {
		if e, ok := byName[c.path]; ok {
			snapshot.Entries = append(snapshot.Entries, e)
		}
		content, readErr := os.ReadFile(filepath.Join(worktreePath, c.path))
		if readErr != nil {
			return nil, fmt.Errorf("capturePreMergeSnapshot: failed to read %q: %w", c.path, readErr)
		}
		snapshot.Content[c.path] = content
	}
	return snapshot, nil
}

// assembleConflictedFileContent renders hunks (a conflicted path's full, in-order hunk
// sequence — resolved and conflicting regions interleaved) into the exact working-tree
// text a real `git merge` would materialize before an abort: resolved hunks contribute
// their resolved lines verbatim, RegionConflict hunks are rendered via renderConflictHunk
// (Epic 3.3), consecutive resolved runs are joined by newlines exactly like
// assembleContent does for the fully-auto-resolved case.
func assembleConflictedFileContent(hunks []MergeHunk, oursLabel, theirsLabel string) (string, error) {
	var b strings.Builder
	var pending []string

	flush := func() {
		if len(pending) == 0 {
			return
		}
		b.WriteString(strings.Join(pending, "\n"))
		b.WriteString("\n")
		pending = nil
	}

	for _, h := range hunks {
		if h.Kind != RegionConflict {
			pending = append(pending, resolvedLines(h)...)
			continue
		}
		flush()
		marker, err := renderConflictHunk(h, oursLabel, theirsLabel)
		if err != nil {
			return "", fmt.Errorf("assembleConflictedFileContent: %w", err)
		}
		b.WriteString(marker)
	}
	flush()
	return b.String(), nil
}

// commitThreeWayMerge writes every cleanly-resolved path (Task 3.4.1b) to the working
// tree, index, and object store, builds the resulting tree object, creates a real
// two-parent merge commit, and advances the checked-out branch ref to it via
// writeRefWithLockSentinel — never go-git's bare SetReference (see that function's doc
// comment).
func commitThreeWayMerge(worktreePath string, repo *git.Repository, oursHash, theirsHash plumbing.Hash, resolved []resolvedPathMerge, refPath, mainBranch string) (*MergeMainResult, error) {
	touched := make(map[string]bool, len(resolved))
	var newEntries []*index.Entry
	for _, r := range resolved {
		touched[r.path] = true
		if r.deleted {
			if err := removeWorkingTreeFile(worktreePath, r.path); err != nil {
				return nil, fmt.Errorf("commitThreeWayMerge: failed to remove %q: %w", r.path, err)
			}
			continue
		}

		hash, err := storeBlobObject(repo, r.content)
		if err != nil {
			return nil, fmt.Errorf("commitThreeWayMerge: failed to store blob for %q: %w", r.path, err)
		}
		if err := writeWorkingTreeFile(worktreePath, r.path, r.mode, r.content); err != nil {
			return nil, fmt.Errorf("commitThreeWayMerge: failed to write %q: %w", r.path, err)
		}
		newEntries = append(newEntries, &index.Entry{Name: r.path, Hash: hash, Mode: r.mode})
	}

	if err := writeIndexEntries(worktreePath, touched, newEntries); err != nil {
		return nil, fmt.Errorf("commitThreeWayMerge: failed to update index: %w", err)
	}

	idx, err := repo.Storer.Index()
	if err != nil {
		return nil, fmt.Errorf("commitThreeWayMerge: failed to reload index: %w", err)
	}
	treeHash, err := buildTreeFromIndex(repo, idx)
	if err != nil {
		return nil, fmt.Errorf("commitThreeWayMerge: failed to build tree: %w", err)
	}

	commitHash, err := createMergeCommit(repo, treeHash, oursHash, theirsHash, mainBranch)
	if err != nil {
		return nil, fmt.Errorf("commitThreeWayMerge: failed to create merge commit: %w", err)
	}

	if err := writeRefWithLockSentinel(refPath, commitHash); err != nil {
		return nil, fmt.Errorf("commitThreeWayMerge: failed to advance branch ref: %w", err)
	}
	return &MergeMainResult{Merged: true}, nil
}

// storeBlobObject stores content as a new blob object in repo's object store, returning
// its hash — the same construction the package's own test helper (storeBlob in
// native_merge_index_test.go) uses, needed here for real (non-test) merged-file content.
func storeBlobObject(repo *git.Repository, content []byte) (plumbing.Hash, error) {
	obj := repo.Storer.NewEncodedObject()
	obj.SetType(plumbing.BlobObject)
	w, err := obj.Writer()
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("storeBlobObject: failed to open writer: %w", err)
	}
	if _, err := w.Write(content); err != nil {
		_ = w.Close()
		return plumbing.ZeroHash, fmt.Errorf("storeBlobObject: failed to write content: %w", err)
	}
	if err := w.Close(); err != nil {
		return plumbing.ZeroHash, fmt.Errorf("storeBlobObject: failed to close writer: %w", err)
	}
	return repo.Storer.SetEncodedObject(obj)
}

// buildTreeFromIndex constructs and stores the nested git tree objects for idx's flat
// stage-0 entries, mirroring go-git's own worktree_commit.go buildTreeHelper.
// Reimplemented here (rather than calling Worktree.Commit) because that helper is
// unexported, and Worktree.Commit's own ref update (updateHEAD) uses go-git's bare
// SetReference — exactly what Task 3.4.1b's merge ref-advance must avoid (see
// writeRefWithLockSentinel's doc comment) — so this project needs the tree-building half
// without the ref-writing half.
func buildTreeFromIndex(repo *git.Repository, idx *index.Index) (plumbing.Hash, error) {
	type dirNode struct {
		entries []object.TreeEntry
	}
	dirs := map[string]*dirNode{"": {}}
	dirOf := func(p string) *dirNode {
		d, ok := dirs[p]
		if !ok {
			d = &dirNode{}
			dirs[p] = d
		}
		return d
	}

	seen := map[string]bool{"": true}
	for _, e := range idx.Entries {
		parts := strings.Split(e.Name, "/")
		var full string
		for i, part := range parts {
			parent := full
			full = path.Join(full, part)
			if seen[full] {
				continue
			}
			seen[full] = true

			te := object.TreeEntry{Name: part}
			if i == len(parts)-1 {
				te.Mode = e.Mode
				te.Hash = e.Hash
			} else {
				te.Mode = filemode.Dir
				dirOf(full)
			}
			d := dirOf(parent)
			d.entries = append(d.entries, te)
		}
	}

	var store func(dirPath string) (plumbing.Hash, error)
	store = func(dirPath string) (plumbing.Hash, error) {
		entries := append([]object.TreeEntry(nil), dirs[dirPath].entries...)
		sort.Slice(entries, func(i, j int) bool {
			return treeEntrySortKey(entries[i]) < treeEntrySortKey(entries[j])
		})
		for i, te := range entries {
			if te.Mode != filemode.Dir {
				continue
			}
			hash, err := store(path.Join(dirPath, te.Name))
			if err != nil {
				return plumbing.ZeroHash, fmt.Errorf("buildTreeFromIndex: failed to build subtree %q: %w", te.Name, err)
			}
			te.Hash = hash
			entries[i] = te
		}

		tree := &object.Tree{Entries: entries}
		obj := repo.Storer.NewEncodedObject()
		if err := tree.Encode(obj); err != nil {
			return plumbing.ZeroHash, fmt.Errorf("buildTreeFromIndex: failed to encode tree %q: %w", dirPath, err)
		}
		return repo.Storer.SetEncodedObject(obj)
	}

	return store("")
}

// treeEntrySortKey mirrors go-git's own tree-entry sort convention (sortableEntries in
// worktree_commit.go): a directory sorts as if its name had a trailing "/", matching how
// git itself orders tree entries.
func treeEntrySortKey(te object.TreeEntry) string {
	if te.Mode == filemode.Dir {
		return te.Name + "/"
	}
	return te.Name
}

// createMergeCommit builds and stores a real two-parent merge commit object over
// treeHash, with commit identity resolved from the repo's git config exactly the way
// go-git's own CommitOptions.Validate does for a normal commit.
func createMergeCommit(repo *git.Repository, treeHash plumbing.Hash, oursHash, theirsHash plumbing.Hash, mainBranch string) (plumbing.Hash, error) {
	opts := &git.CommitOptions{Parents: []plumbing.Hash{oursHash, theirsHash}}
	if err := opts.Validate(repo); err != nil {
		return plumbing.ZeroHash, fmt.Errorf("createMergeCommit: failed to resolve commit identity: %w", err)
	}

	commit := &object.Commit{
		Author:       *opts.Author,
		Committer:    *opts.Committer,
		Message:      fmt.Sprintf("Merge branch 'origin/%s'\n", mainBranch),
		TreeHash:     treeHash,
		ParentHashes: opts.Parents,
	}
	obj := repo.Storer.NewEncodedObject()
	if err := commit.Encode(obj); err != nil {
		return plumbing.ZeroHash, fmt.Errorf("createMergeCommit: failed to encode commit: %w", err)
	}
	return repo.Storer.SetEncodedObject(obj)
}
