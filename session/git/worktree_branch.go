package git

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	//
	// Two defensive guards before the os.RemoveAll, since this now touches the filesystem
	// directly instead of going through go-git's ref-name validation:
	//  - g.branchName can contain caller-supplied path segments (e.g. "../../etc") that were
	//    never sanitized for this use — reject anything that would resolve outside
	//    .git/worktrees/ rather than let RemoveAll delete an arbitrary directory.
	//  - g.repoPath may itself be a linked worktree (its .git is a redirect *file*, not a
	//    directory) if a caller reached here without first resolving to the main repo path.
	//    Joining ".git"/"worktrees"/... onto a file produces a hard "not a directory" error
	//    from RemoveAll where the old ref-API call would have just not found anything to
	//    remove — skip the cleanup instead of failing Setup() over stale admin metadata that
	//    isn't even reachable from this repoPath.
	worktreesRoot := filepath.Join(g.repoPath, ".git", "worktrees")
	if fi, err := os.Stat(filepath.Join(g.repoPath, ".git")); err != nil || !fi.IsDir() {
		// .git missing or a redirect file (linked worktree) — nothing safely cleanable here.
	} else {
		worktreeAdminDir := filepath.Join(worktreesRoot, g.branchName)
		if rel, err := filepath.Rel(worktreesRoot, worktreeAdminDir); err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("refusing to remove worktree reference for %s: resolves outside %s", g.branchName, worktreesRoot)
		} else if err := os.RemoveAll(worktreeAdminDir); err != nil {
			return fmt.Errorf("failed to remove worktree reference for %s: %w", g.branchName, err)
		}
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
