package git

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// cleanupExistingBranch performs a thorough cleanup of any existing branch or reference
func (g *GitWorktree) cleanupExistingBranch(repo *git.Repository) error {
	branchRef := plumbing.NewBranchReferenceName(g.branchName)

	// Try to remove the branch reference
	if err := repo.Storer.RemoveReference(branchRef); err != nil && err != plumbing.ErrReferenceNotFound {
		return fmt.Errorf("failed to remove branch reference %s: %w", g.branchName, err)
	}

	// Remove any stale linked-worktree admin metadata left under .git/worktrees/<branch>/
	// (e.g. a HEAD file from a worktree removal that didn't fully clean up). This is a
	// real filesystem path, not a git ref — go-git's Storer.RemoveReference used to accept
	// "worktrees/<branch>/HEAD" as a loose-file path and quietly write/remove it there, but
	// go-git v5.19+ validates ref names against refs/ and a fixed pseudo-ref allowlist and
	// now rejects it ("reference name escapes the reference storage"). Removing the
	// directory directly is both more explicit and no longer dependent on that undocumented
	// go-git behavior.
	worktreeAdminDir := filepath.Join(g.repoPath, ".git", "worktrees", g.branchName)
	if err := os.RemoveAll(worktreeAdminDir); err != nil {
		return fmt.Errorf("failed to remove worktree reference for %s: %w", g.branchName, err)
	}

	// Clean up configuration entries
	cfg, err := repo.Config()
	if err != nil {
		return fmt.Errorf("failed to get repository config: %w", err)
	}

	delete(cfg.Branches, g.branchName)
	worktreeSection := fmt.Sprintf("worktree.%s", g.branchName)
	cfg.Raw.RemoveSection(worktreeSection)

	if err := repo.Storer.SetConfig(cfg); err != nil {
		return fmt.Errorf("failed to update repository config after removing branch %s: %w", g.branchName, err)
	}

	return nil
}
