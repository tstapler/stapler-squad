package git

import (
	"context"
	"fmt"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/log"
	"os/exec"
	"strings"
	"time"
)

// runGitCommand executes a git command and returns any error.
// Uses the executor for circuit breaker support when available.
func (g *GitWorktree) runGitCommand(path string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	baseArgs := []string{"-C", path}
	cmd := safeexec.CommandContext(ctx, "git", append(baseArgs, args...)...)

	var output []byte
	var err error
	if g.cmdExec != nil {
		output, err = g.cmdExec.CombinedOutput(cmd)
	} else {
		output, err = cmd.CombinedOutput()
	}
	if err != nil {
		return "", fmt.Errorf("git command failed: %s (%w)", output, err)
	}

	return string(output), nil
}

// PushChanges commits and pushes changes in the worktree to the remote branch
func (g *GitWorktree) PushChanges(commitMessage string, open bool) error {
	if err := checkGHCLI(); err != nil {
		return err
	}

	// Check if there are any changes to commit
	isDirty, err := g.IsDirty()
	if err != nil {
		return fmt.Errorf("failed to check for changes: %w", err)
	}

	if isDirty {
		// Stage all changes
		if _, err := g.runGitCommand(g.worktreePath, "add", "."); err != nil {
			log.Error("failed to stage changes", "err", err)
			return fmt.Errorf("failed to stage changes: %w", err)
		}

		// Create commit
		if _, err := g.runGitCommand(g.worktreePath, "commit", "-m", commitMessage, "--no-verify"); err != nil {
			log.Error("failed to commit changes", "err", err)
			return fmt.Errorf("failed to commit changes: %w", err)
		}
		g.InvalidateDirtyCache()
	}

	// First push the branch to remote to ensure it exists
	pushCtx, pushCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer pushCancel()
	pushCmd := safeexec.CommandContext(pushCtx, "gh", "repo", "sync", "--source", "-b", g.branchName)
	pushCmd.Dir = g.worktreePath
	if err := g.runExec(pushCmd); err != nil {
		// If sync fails, try creating the branch on remote first
		gitPushCtx, gitPushCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer gitPushCancel()
		gitPushCmd := safeexec.CommandContext(gitPushCtx, "git", "push", "-u", "origin", g.branchName)
		gitPushCmd.Dir = g.worktreePath
		if pushOutput, pushErr := g.runCombinedOutput(gitPushCmd); pushErr != nil {
			log.Error("failed to push branch", "err", pushErr)
			return fmt.Errorf("failed to push branch: %s (%w)", pushOutput, pushErr)
		}
	}

	// Now sync with remote
	syncCtx, syncCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer syncCancel()
	syncCmd := safeexec.CommandContext(syncCtx, "gh", "repo", "sync", "-b", g.branchName)
	syncCmd.Dir = g.worktreePath
	if output, err := g.runCombinedOutput(syncCmd); err != nil {
		log.Error("failed to sync changes", "err", err)
		return fmt.Errorf("failed to sync changes: %s (%w)", output, err)
	}

	// Open the branch in the browser
	if open {
		if err := g.OpenBranchURL(); err != nil {
			// Just log the error but don't fail the push operation
			log.Error("failed to open branch URL", "err", err)
		}
	}

	return nil
}

// CommitChanges commits changes locally without pushing to remote
func (g *GitWorktree) CommitChanges(commitMessage string) error {
	// Check if there are any changes to commit
	isDirty, err := g.IsDirty()
	if err != nil {
		return fmt.Errorf("failed to check for changes: %w", err)
	}

	if isDirty {
		// Stage all changes
		if _, err := g.runGitCommand(g.worktreePath, "add", "."); err != nil {
			log.Error("failed to stage changes", "err", err)
			return fmt.Errorf("failed to stage changes: %w", err)
		}

		// Create commit (local only)
		if _, err := g.runGitCommand(g.worktreePath, "commit", "-m", commitMessage, "--no-verify"); err != nil {
			log.Error("failed to commit changes", "err", err)
			return fmt.Errorf("failed to commit changes: %w", err)
		}
		g.InvalidateDirtyCache()
	}

	return nil
}

// InvalidateDirtyCache clears the IsDirty cache so the next call re-runs git status.
// Call this whenever worktree state changes outside of Claude's control (e.g. after a
// manual commit, after running git operations, or in tests after writing files directly).
func (g *GitWorktree) InvalidateDirtyCache() {
	g.isDirtyCacheMu.Lock()
	g.isDirtyCacheTime = time.Time{}
	g.isDirtyCacheMu.Unlock()
}

// IsDirty checks if the worktree has uncommitted changes.
// Results are cached for isDirtyCacheTTL (15 s) to avoid spawning a subprocess on every call.
func (g *GitWorktree) IsDirty() (bool, error) {
	return g.IsDirtyWithHint(false)
}

// IsDirtyWithHint checks if the worktree has uncommitted changes.
// When claudeActive is true the subprocess is skipped entirely and the cached value is returned
// (or false if no cached value is available yet), because Claude never modifies worktree state
// while it is actively generating output.
func (g *GitWorktree) IsDirtyWithHint(claudeActive bool) (bool, error) {
	// Fast path: hold read lock and check whether the cache is still fresh.
	g.isDirtyCacheMu.RLock()
	cacheValid := !g.isDirtyCacheTime.IsZero() && time.Since(g.isDirtyCacheTime) < isDirtyCacheTTL
	if cacheValid || claudeActive {
		cached := g.isDirtyCache
		g.isDirtyCacheMu.RUnlock()
		return cached, nil
	}
	g.isDirtyCacheMu.RUnlock()

	// Slow path: acquire write lock, double-check, then run subprocess.
	g.isDirtyCacheMu.Lock()
	defer g.isDirtyCacheMu.Unlock()

	// Re-check inside the write lock (another goroutine may have refreshed while we waited).
	if !g.isDirtyCacheTime.IsZero() && time.Since(g.isDirtyCacheTime) < isDirtyCacheTTL {
		return g.isDirtyCache, nil
	}

	output, err := g.runGitCommand(g.worktreePath, "status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("failed to check worktree status: %w", err)
	}

	g.isDirtyCache = len(output) > 0
	g.isDirtyCacheTime = time.Now()
	return g.isDirtyCache, nil
}

// IsBranchCheckedOut checks if the instance branch is currently checked out
func (g *GitWorktree) IsBranchCheckedOut() (bool, error) {
	output, err := g.runGitCommand(g.repoPath, "branch", "--show-current")
	if err != nil {
		return false, fmt.Errorf("failed to get current branch: %w", err)
	}
	return strings.TrimSpace(string(output)) == g.branchName, nil
}

// OpenBranchURL opens the branch URL in the default browser
func (g *GitWorktree) OpenBranchURL() error {
	// Check if GitHub CLI is available
	if err := checkGHCLI(); err != nil {
		return err
	}

	browseCtx, browseCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer browseCancel()
	cmd := safeexec.CommandContext(browseCtx, "gh", "browse", "--branch", g.branchName)
	cmd.Dir = g.worktreePath
	if err := g.runExec(cmd); err != nil {
		return fmt.Errorf("failed to open branch URL: %w", err)
	}
	return nil
}

// runExec runs a command through the executor (or directly if no executor is set).
func (g *GitWorktree) runExec(cmd *exec.Cmd) error {
	if g.cmdExec != nil {
		return g.cmdExec.Run(cmd)
	}
	return cmd.Run()
}

// runCombinedOutput runs a command through the executor and returns combined output.
func (g *GitWorktree) runCombinedOutput(cmd *exec.Cmd) ([]byte, error) {
	if g.cmdExec != nil {
		return g.cmdExec.CombinedOutput(cmd)
	}
	return cmd.CombinedOutput()
}
