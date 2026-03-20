package session

import (
	"fmt"
	"os"

	"github.com/tstapler/stapler-squad/session/git"
)

// GitWorktreeManager owns the git worktree and diff-stats state that were
// previously bare fields on Instance.
//
// Instance keeps thin wrapper methods that delegate here. GitWorktreeManager
// itself has no knowledge of Instance lifecycle; it only manages the worktree
// and diff operations.
type GitWorktreeManager struct {
	worktree  *git.GitWorktree
	diffStats *git.DiffStats
}

// HasWorktree reports whether a git worktree has been initialized.
func (gm *GitWorktreeManager) HasWorktree() bool {
	return gm.worktree != nil
}

// GetWorktree returns the underlying GitWorktree (may be nil before Setup).
func (gm *GitWorktreeManager) GetWorktree() *git.GitWorktree {
	return gm.worktree
}

// SetWorktree replaces the underlying GitWorktree. Used during session start
// and by tests.
func (gm *GitWorktreeManager) SetWorktree(wt *git.GitWorktree) {
	gm.worktree = wt
}

// GetWorktreePath returns the worktree path or "" if no worktree.
func (gm *GitWorktreeManager) GetWorktreePath() string {
	if gm.worktree == nil {
		return ""
	}
	return gm.worktree.GetWorktreePath()
}

// GetRepoPath returns the repo root path or "" if no worktree.
func (gm *GitWorktreeManager) GetRepoPath() string {
	if gm.worktree == nil {
		return ""
	}
	return gm.worktree.GetRepoPath()
}

// GetRepoName returns the repository name or "" if no worktree.
func (gm *GitWorktreeManager) GetRepoName() string {
	if gm.worktree == nil {
		return ""
	}
	return gm.worktree.GetRepoName()
}

// GetBranchName returns the branch name or "" if no worktree.
func (gm *GitWorktreeManager) GetBranchName() string {
	if gm.worktree == nil {
		return ""
	}
	return gm.worktree.GetBranchName()
}

// GetBaseCommitSHA returns the base commit SHA or "" if no worktree.
func (gm *GitWorktreeManager) GetBaseCommitSHA() string {
	if gm.worktree == nil {
		return ""
	}
	return gm.worktree.GetBaseCommitSHA()
}

// Setup prepares the worktree (creates directories, checks out branch, etc.).
func (gm *GitWorktreeManager) Setup() error {
	if gm.worktree == nil {
		return fmt.Errorf("git worktree not initialized")
	}
	return gm.worktree.Setup()
}

// Cleanup removes the worktree from the filesystem and git metadata.
// Returns nil if no worktree is set.
func (gm *GitWorktreeManager) Cleanup() error {
	if gm.worktree == nil {
		return nil
	}
	return gm.worktree.Cleanup()
}

// Remove removes the worktree from git without pruning.
func (gm *GitWorktreeManager) Remove() error {
	if gm.worktree == nil {
		return fmt.Errorf("git worktree not initialized")
	}
	return gm.worktree.Remove()
}

// Prune cleans up stale worktree references.
func (gm *GitWorktreeManager) Prune() error {
	if gm.worktree == nil {
		return fmt.Errorf("git worktree not initialized")
	}
	return gm.worktree.Prune()
}

// IsDirty reports whether the worktree has uncommitted changes.
func (gm *GitWorktreeManager) IsDirty() (bool, error) {
	if gm.worktree == nil {
		return false, fmt.Errorf("git worktree not initialized")
	}
	return gm.worktree.IsDirty()
}

// CommitChanges stages all changes and creates a commit.
func (gm *GitWorktreeManager) CommitChanges(commitMsg string) error {
	if gm.worktree == nil {
		return fmt.Errorf("git worktree not initialized")
	}
	return gm.worktree.CommitChanges(commitMsg)
}

// PushChanges commits and pushes the worktree branch.
func (gm *GitWorktreeManager) PushChanges(commitMsg string, open bool) error {
	if gm.worktree == nil {
		return fmt.Errorf("git worktree not initialized")
	}
	return gm.worktree.PushChanges(commitMsg, open)
}

// IsBranchCheckedOut reports whether the branch is currently checked out.
func (gm *GitWorktreeManager) IsBranchCheckedOut() (bool, error) {
	if gm.worktree == nil {
		return false, fmt.Errorf("git worktree not initialized")
	}
	return gm.worktree.IsBranchCheckedOut()
}

// OpenBranchURL opens the branch URL in the browser.
func (gm *GitWorktreeManager) OpenBranchURL() error {
	if gm.worktree == nil {
		return fmt.Errorf("git worktree not initialized")
	}
	return gm.worktree.OpenBranchURL()
}

// ComputeDiffIfReady checks if the worktree path exists and computes a new diff.
// Returns (stats, needsPause) where needsPause is true if the worktree directory is missing.
// This method performs I/O and should be called WITHOUT holding Instance.stateMutex.
// Returns (nil, false) if no worktree is set.
func (gm *GitWorktreeManager) ComputeDiffIfReady() (stats *git.DiffStats, needsPause bool) {
	if gm.worktree == nil {
		return nil, false
	}
	worktreePath := gm.worktree.GetWorktreePath()
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		return nil, true
	}
	result := gm.worktree.Diff()
	return result, false
}

// ComputeDiff runs git diff and returns the result without storing it.
// Returns nil if no worktree is set.
func (gm *GitWorktreeManager) ComputeDiff() *git.DiffStats {
	if gm.worktree == nil {
		return nil
	}
	return gm.worktree.Diff()
}

// UpdateDiffStats computes a new diff and stores it.
// Returns nil and clears stats if worktree is not ready.
func (gm *GitWorktreeManager) UpdateDiffStats() {
	if gm.worktree == nil {
		gm.diffStats = nil
		return
	}
	gm.diffStats = gm.worktree.Diff()
}

// GetDiffStats returns the most recently computed diff stats (may be nil).
func (gm *GitWorktreeManager) GetDiffStats() *git.DiffStats {
	return gm.diffStats
}

// SetDiffStats directly replaces the diff stats (used during deserialization).
func (gm *GitWorktreeManager) SetDiffStats(stats *git.DiffStats) {
	gm.diffStats = stats
}

// ClearDiffStats sets diffStats to nil.
func (gm *GitWorktreeManager) ClearDiffStats() {
	gm.diffStats = nil
}
