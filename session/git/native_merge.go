package git

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/index"
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
