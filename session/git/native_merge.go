package git

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

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
