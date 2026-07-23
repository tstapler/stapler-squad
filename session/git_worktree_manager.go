package session

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/linkdata/deadlock"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/session/git"
)

// GitWorktreeManager owns the git worktree and diff-stats state that were
// previously bare fields on Instance.
//
// Instance keeps thin wrapper methods that delegate here. GitWorktreeManager
// itself has no knowledge of Instance lifecycle; it only manages the worktree
// and diff operations.
//
// worktree/diffStats are guarded by mu, not by Instance.stateMutex: setup
// (setupFirstTimeWorktree, called under Instance.startMu) and read-side
// callers (e.g. GetEffectiveRootDir) don't consistently hold stateMutex, so
// GitWorktreeManager protects its own fields directly.
type GitWorktreeManager struct {
	mu         deadlock.RWMutex
	worktree   *git.GitWorktree
	diffStats  *git.DiffStats
	dirBaseSHA string
}

// SetDirBaseSHA sets the base commit SHA for directory-mode diff computation.
func (gm *GitWorktreeManager) SetDirBaseSHA(sha string) { gm.dirBaseSHA = sha }

// GetDirBaseSHA returns the base commit SHA for directory-mode diff computation.
func (gm *GitWorktreeManager) GetDirBaseSHA() string { return gm.dirBaseSHA }

// HasWorktree reports whether a git worktree has been initialized.
func (gm *GitWorktreeManager) HasWorktree() bool {
	gm.mu.RLock()
	defer gm.mu.RUnlock()
	return gm.worktree != nil
}

// GetWorktree returns the underlying GitWorktree (may be nil before Setup).
func (gm *GitWorktreeManager) GetWorktree() *git.GitWorktree {
	gm.mu.RLock()
	defer gm.mu.RUnlock()
	return gm.worktree
}

// SetWorktree replaces the underlying GitWorktree. Used during session start
// and by tests.
func (gm *GitWorktreeManager) SetWorktree(wt *git.GitWorktree) {
	gm.mu.Lock()
	defer gm.mu.Unlock()
	gm.worktree = wt
}

// PrimeDirtyCacheJitter staggers the dirty-cache TTL by setting the cache
// timestamp to a random point in [now-IsDirtyCacheTTL, now). Call this when
// adding a session to the poller so sessions added in a burst don't all run
// git-status subprocesses simultaneously when their caches expire.
func (gm *GitWorktreeManager) PrimeDirtyCacheJitter() {
	wt := gm.GetWorktree()
	if wt == nil {
		return
	}
	jitter := time.Duration(rand.Int63n(int64(git.IsDirtyCacheTTL)))
	wt.PrimeDirtyCacheAt(time.Now().Add(-jitter))
}

// GetWorktreePath returns the worktree path or "" if no worktree.
func (gm *GitWorktreeManager) GetWorktreePath() string {
	wt := gm.GetWorktree()
	if wt == nil {
		return ""
	}
	return wt.GetWorktreePath()
}

// GetRepoPath returns the repo root path or "" if no worktree.
func (gm *GitWorktreeManager) GetRepoPath() string {
	wt := gm.GetWorktree()
	if wt == nil {
		return ""
	}
	return wt.GetRepoPath()
}

// GetRepoName returns the repository name or "" if no worktree.
func (gm *GitWorktreeManager) GetRepoName() string {
	wt := gm.GetWorktree()
	if wt == nil {
		return ""
	}
	return wt.GetRepoName()
}

// GetBranchName returns the branch name or "" if no worktree.
func (gm *GitWorktreeManager) GetBranchName() string {
	wt := gm.GetWorktree()
	if wt == nil {
		return ""
	}
	return wt.GetBranchName()
}

// GetBaseCommitSHA returns the base commit SHA or "" if no worktree.
func (gm *GitWorktreeManager) GetBaseCommitSHA() string {
	wt := gm.GetWorktree()
	if wt == nil {
		return ""
	}
	return wt.GetBaseCommitSHA()
}

// Setup prepares the worktree (creates directories, checks out branch, etc.).
func (gm *GitWorktreeManager) Setup() error {
	wt := gm.GetWorktree()
	if wt == nil {
		return fmt.Errorf("git worktree not initialized")
	}
	return wt.Setup()
}

// Cleanup removes the worktree from the filesystem and git metadata.
// Returns nil if no worktree is set.
func (gm *GitWorktreeManager) Cleanup() error {
	wt := gm.GetWorktree()
	if wt == nil {
		return nil
	}
	return wt.Cleanup()
}

// Remove removes the worktree from git without pruning.
func (gm *GitWorktreeManager) Remove() error {
	wt := gm.GetWorktree()
	if wt == nil {
		return fmt.Errorf("git worktree not initialized")
	}
	return wt.Remove()
}

// Prune cleans up stale worktree references.
func (gm *GitWorktreeManager) Prune() error {
	wt := gm.GetWorktree()
	if wt == nil {
		return fmt.Errorf("git worktree not initialized")
	}
	return wt.Prune()
}

// IsDirty reports whether the worktree has uncommitted changes.
func (gm *GitWorktreeManager) IsDirty() (bool, error) {
	wt := gm.GetWorktree()
	if wt == nil {
		return false, fmt.Errorf("git worktree not initialized")
	}
	return wt.IsDirty()
}

// InvalidateDirtyCache clears the IsDirty TTL cache so the next call re-runs git status.
// Call after transitions that may change worktree dirty state (Resume, Stop).
// No-op if no worktree is set.
func (gm *GitWorktreeManager) InvalidateDirtyCache() {
	wt := gm.GetWorktree()
	if wt == nil {
		return
	}
	wt.InvalidateDirtyCache()
}

// CommitChanges stages all changes and creates a commit.
func (gm *GitWorktreeManager) CommitChanges(commitMsg string) error {
	wt := gm.GetWorktree()
	if wt == nil {
		return fmt.Errorf("git worktree not initialized")
	}
	return wt.CommitChanges(commitMsg)
}

// PushChanges commits and pushes the worktree branch.
func (gm *GitWorktreeManager) PushChanges(commitMsg string, open bool) error {
	wt := gm.GetWorktree()
	if wt == nil {
		return fmt.Errorf("git worktree not initialized")
	}
	return wt.PushChanges(commitMsg, open)
}

// IsBranchCheckedOut reports whether the branch is currently checked out.
func (gm *GitWorktreeManager) IsBranchCheckedOut() (bool, error) {
	wt := gm.GetWorktree()
	if wt == nil {
		return false, fmt.Errorf("git worktree not initialized")
	}
	return wt.IsBranchCheckedOut()
}

// OpenBranchURL opens the branch URL in the browser.
func (gm *GitWorktreeManager) OpenBranchURL() error {
	wt := gm.GetWorktree()
	if wt == nil {
		return fmt.Errorf("git worktree not initialized")
	}
	return wt.OpenBranchURL()
}

// ComputeDiffIfReady checks if the worktree path exists and computes a new diff.
// Returns (stats, needsPause) where needsPause is true if the worktree directory is missing.
// This method performs I/O and should be called WITHOUT holding Instance.mu.
// Returns (nil, false) if no worktree is set.
func (gm *GitWorktreeManager) ComputeDiffIfReady() (stats *git.DiffStats, needsPause bool) {
	wt := gm.GetWorktree()
	if wt == nil {
		return nil, false
	}
	worktreePath := wt.GetWorktreePath()
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		return nil, true
	}
	result := wt.Diff()
	return result, false
}

// ComputeDiff runs git diff and returns the result without storing it.
// Returns nil if no worktree is set.
func (gm *GitWorktreeManager) ComputeDiff() *git.DiffStats {
	wt := gm.GetWorktree()
	if wt == nil {
		return nil
	}
	return wt.Diff()
}

// UpdateDiffStats computes a new diff and stores it.
// Returns nil and clears stats if worktree is not ready.
func (gm *GitWorktreeManager) UpdateDiffStats() {
	wt := gm.GetWorktree()
	if wt == nil {
		gm.mu.Lock()
		gm.diffStats = nil
		gm.mu.Unlock()
		return
	}
	stats := wt.Diff()
	gm.mu.Lock()
	gm.diffStats = stats
	gm.mu.Unlock()
}

// GetDiffStats returns the most recently computed diff stats (may be nil).
func (gm *GitWorktreeManager) GetDiffStats() *git.DiffStats {
	gm.mu.RLock()
	defer gm.mu.RUnlock()
	return gm.diffStats
}

// SetDiffStats directly replaces the diff stats (used during deserialization).
func (gm *GitWorktreeManager) SetDiffStats(stats *git.DiffStats) {
	gm.mu.Lock()
	defer gm.mu.Unlock()
	gm.diffStats = stats
}

// ClearDiffStats sets diffStats to nil.
func (gm *GitWorktreeManager) ClearDiffStats() {
	gm.mu.Lock()
	defer gm.mu.Unlock()
	gm.diffStats = nil
}

// GitManager is the interface satisfied by *GitWorktreeManager.
// It covers all git worktree operations used by Instance and can be implemented
// by test doubles to avoid requiring a real git repository.
type GitManager interface {
	HasWorktree() bool
	GetWorktree() *git.GitWorktree
	SetWorktree(*git.GitWorktree)
	GetWorktreePath() string
	GetRepoPath() string
	GetRepoName() string
	GetBranchName() string
	GetBaseCommitSHA() string
	Setup() error
	Cleanup() error
	Remove() error
	Prune() error
	IsDirty() (bool, error)
	InvalidateDirtyCache()
	CommitChanges(commitMsg string) error
	PushChanges(commitMsg string, open bool) error
	IsBranchCheckedOut() (bool, error)
	OpenBranchURL() error
	ComputeDiffIfReady() (stats *git.DiffStats, needsPause bool)
	ComputeDiff() *git.DiffStats
	UpdateDiffStats()
	GetDiffStats() *git.DiffStats
	SetDiffStats(*git.DiffStats)
	ClearDiffStats()
	GetCurrentCommitSHA() (string, error)
	PrimeDirtyCacheJitter()
}

// compile-time check that *GitWorktreeManager satisfies GitManager.
var _ GitManager = (*GitWorktreeManager)(nil)

// GetCurrentCommitSHA returns the current HEAD commit SHA for the worktree.
// Returns an empty string (not an error) if no worktree is set or the repo
// has no commits yet — this is safe to use in checkpoint creation.
func (gm *GitWorktreeManager) GetCurrentCommitSHA() (string, error) {
	dir := gm.GetWorktreePath()
	if dir == "" {
		dir = gm.GetRepoPath()
	}
	if dir == "" {
		return "", nil
	}

	revCtx, revCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer revCancel()
	cmd := safeexec.CommandContext(revCtx, "git", "-C", dir, "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		// Not a fatal error — repo may have no commits yet.
		return "", nil
	}
	return strings.TrimSpace(string(output)), nil
}
