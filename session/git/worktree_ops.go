package git

import (
	"context"
	"fmt"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// Setup creates a new worktree for the session
func (g *GitWorktree) Setup() error {
	// Ensure worktrees directory exists early (can be done in parallel with branch check)
	worktreesDir, err := getWorktreeDirectory()
	if err != nil {
		return fmt.Errorf("failed to get worktree directory: %w", err)
	}

	// Create directory and check branch existence in parallel
	errChan := make(chan error, 2)
	var branchExists bool

	// Goroutine for directory creation
	go func() {
		errChan <- os.MkdirAll(worktreesDir, 0755)
	}()

	// Goroutine for branch check
	go func() {
		repo, err := git.PlainOpen(g.repoPath)
		if err != nil {
			errChan <- fmt.Errorf("failed to open repository: %w", err)
			return
		}

		branchRef := plumbing.NewBranchReferenceName(g.branchName)
		if _, err := repo.Reference(branchRef, false); err == nil {
			branchExists = true
		}
		errChan <- nil
	}()

	// Wait for both operations
	for i := 0; i < 2; i++ {
		if err := <-errChan; err != nil {
			return err
		}
	}

	if branchExists {
		return g.setupFromExistingBranch()
	}
	return g.setupNewWorktree()
}

// setupFromExistingBranch creates a worktree from an existing branch, reusing one
// already checked out at g.worktreePath in place rather than tearing it down and
// recreating it. Backlog rework/reopen spawns intentionally reuse the same
// "backlog/<item>" branch and worktree path across every revision (see
// SpawnSessionFromItem's reopen comment) — force-removing and re-adding the worktree
// here on every single reopen discarded whatever uncommitted state the worktree held
// and needlessly recreated the directory, which is exactly the behavior that left a
// still in_progress/review item with a missing worktree once anything else (a
// concurrent cleanup call, or simply a slow-running review) touched it mid-recreation.
func (g *GitWorktree) setupFromExistingBranch() error {
	// Directory already created in Setup(), skip duplicate creation

	if g.worktreeAlreadyRegisteredForBranch() {
		log.Info("worktree already checked out for branch, reusing in place", "branch", g.branchName, "path", g.worktreePath)
		g.initBaseCommitSHA()
		return nil
	}

	// Clean up any existing worktree first. Unlock before removing: a worktree left
	// locked (initializing) by an interrupted `worktree add` — the exact state
	// worktreeAlreadyRegisteredForBranch just rejected above — otherwise refuses
	// `remove` regardless of -f, leaving the broken checkout stuck forever.
	_, _ = g.runGitCommand(g.repoPath, "worktree", "unlock", g.worktreePath)     // Ignore error if not locked
	_, _ = g.runGitCommand(g.repoPath, "worktree", "remove", "-f", g.worktreePath) // Ignore error if worktree doesn't exist

	// Create a new worktree from the existing branch
	if _, err := g.runGitCommand(g.repoPath, "worktree", "add", g.worktreePath, g.branchName); err != nil {
		// Check if the error is because the branch is already checked out elsewhere
		if strings.Contains(err.Error(), "already checked out") {
			// Try to find and connect to the existing worktree
			log.Info("branch is already checked out, attempting to locate existing worktree", "branch", g.branchName)

			// List all worktrees to find where this branch is checked out
			output, listErr := g.runGitCommand(g.repoPath, "worktree", "list", "--porcelain")
			if listErr != nil {
				return fmt.Errorf("failed to list worktrees while handling checkout conflict: %w", listErr)
			}

			// Parse worktree list to find the one with our branch
			existingPath, found := g.findWorktreeForBranch(output, g.branchName)
			if found {
				log.Info("found existing worktree for branch, using it instead", "branch", g.branchName, "path", existingPath)
				g.worktreePath = existingPath
				g.initBaseCommitSHA()
				return nil
			}

			// If we can't find the existing worktree, return the original error
			return fmt.Errorf("failed to create worktree from branch %s (branch already checked out elsewhere, but could not locate existing worktree): %w", g.branchName, err)
		}

		return fmt.Errorf("failed to create worktree from branch %s: %w", g.branchName, err)
	}

	// Worktree created successfully — record the base commit for diff tracking.
	g.initBaseCommitSHA()

	return nil
}

// initBaseCommitSHA finds the merge-base of HEAD with common default branches and
// stores it in g.baseCommitSHA. Non-fatal: if no default branch is found the field
// remains empty and Diff() will fall back to its own resolution.
func (g *GitWorktree) initBaseCommitSHA() {
	for _, branch := range []string{"main", "master", "develop", "trunk"} {
		output, err := g.runGitCommand(g.repoPath, "merge-base", "HEAD", branch)
		if err == nil {
			if sha := strings.TrimSpace(output); sha != "" {
				g.baseCommitSHA = sha
				log.Info("set base commit SHA for branch to merge-base", "branch", g.branchName, "with_branch", branch, "sha", sha[:min(8, len(sha))])
				return
			}
		}
	}
	log.Warn("could not find merge-base for branch with any default branch (main/master/develop/trunk)", "branch", g.branchName)
}

// worktreeAlreadyRegisteredForBranch reports whether g.worktreePath is already a live,
// fully-set-up git worktree checked out to g.branchName — i.e. reused in place rather
// than removed and recreated. Requires:
//   - git registration for this exact path+branch (via 'worktree list --porcelain'),
//   - the directory's actual presence on disk ('git worktree list' still reports
//     prunable entries for directories deleted out from under git, e.g. by an external
//     rm -rf, and reusing one of those would hand back a path that doesn't exist), and
//   - NOT locked with git's "initializing" marker. `worktree add` briefly holds this
//     lock while it populates the checkout and clears it on success; a worktree still
//     locked here means an earlier `worktree add` was interrupted mid-checkout (e.g.
//     killed by runGitCommand's 30s timeout under load) and left a half-populated
//     directory — reusing that in place would silently hand a broken checkout to the
//     new session instead of self-healing via a fresh remove+add, exactly the failure
//     this reuse-in-place logic exists to prevent for the *good* case.
func (g *GitWorktree) worktreeAlreadyRegisteredForBranch() bool {
	if _, statErr := os.Stat(g.worktreePath); statErr != nil {
		return false
	}
	output, err := g.runGitCommand(g.repoPath, "worktree", "list", "--porcelain")
	if err != nil {
		return false
	}
	path, found := g.findWorktreeForBranch(output, g.branchName)
	if !found || path != g.worktreePath {
		return false
	}
	return !isWorktreeLocked(output, g.worktreePath)
}

// isWorktreeLocked reports whether the worktree at targetPath is marked "locked" in
// 'git worktree list --porcelain' output (with or without a reason).
func isWorktreeLocked(porcelainOutput, targetPath string) bool {
	lines := strings.Split(strings.TrimSpace(porcelainOutput), "\n")
	var currentWorktreePath string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		switch {
		case line == "":
			currentWorktreePath = ""
		case strings.HasPrefix(line, "worktree "):
			currentWorktreePath = strings.TrimPrefix(line, "worktree ")
		case currentWorktreePath == targetPath && (line == "locked" || strings.HasPrefix(line, "locked ")):
			return true
		}
	}
	return false
}

// findWorktreeForBranch parses the output of 'git worktree list --porcelain'
// and returns the path of the worktree that has the specified branch checked out
func (g *GitWorktree) findWorktreeForBranch(porcelainOutput, targetBranch string) (string, bool) {
	lines := strings.Split(strings.TrimSpace(porcelainOutput), "\n")
	var currentWorktreePath string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			// Empty line separates worktree entries
			currentWorktreePath = ""
			continue
		}

		if strings.HasPrefix(line, "worktree ") {
			// Extract worktree path
			currentWorktreePath = strings.TrimPrefix(line, "worktree ")
		} else if strings.HasPrefix(line, "branch ") && currentWorktreePath != "" {
			// Extract branch name and check if it matches
			branchName := strings.TrimPrefix(line, "branch refs/heads/")
			if branchName == targetBranch {
				return currentWorktreePath, true
			}
		}
	}

	return "", false
}

// setupNewWorktree creates a new worktree from HEAD
func (g *GitWorktree) setupNewWorktree() error {
	// Ensure worktrees directory exists
	worktreesDir := filepath.Join(g.repoPath, "worktrees")
	if err := os.MkdirAll(worktreesDir, 0755); err != nil {
		return fmt.Errorf("failed to create worktrees directory: %w", err)
	}

	// Clean up any existing worktree first
	_, _ = g.runGitCommand(g.repoPath, "worktree", "remove", "-f", g.worktreePath) // Ignore error if worktree doesn't exist

	// Open the repository
	repo, err := git.PlainOpen(g.repoPath)
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	// Check if the branch already exists - if so, use it instead of cleaning up
	branchRef := plumbing.NewBranchReferenceName(g.branchName)
	if _, err := repo.Reference(branchRef, false); err == nil {
		// Branch exists - use setupFromExistingBranch instead
		log.Info("branch already exists, using existing branch for worktree", "branch", g.branchName)
		return g.setupFromExistingBranch()
	}

	// Branch doesn't exist - clean up any orphaned references and create new branch
	if err := g.cleanupExistingBranch(repo); err != nil {
		return fmt.Errorf("failed to cleanup existing branch: %w", err)
	}

	output, err := g.runGitCommand(g.repoPath, "rev-parse", "HEAD")
	if err != nil {
		if strings.Contains(err.Error(), "fatal: ambiguous argument 'HEAD'") ||
			strings.Contains(err.Error(), "fatal: not a valid object name") ||
			strings.Contains(err.Error(), "fatal: HEAD: not a valid object name") {
			return fmt.Errorf("this appears to be a brand new repository: please create an initial commit before creating an instance")
		}
		return fmt.Errorf("failed to get HEAD commit hash: %w", err)
	}
	headCommit := strings.TrimSpace(string(output))
	g.baseCommitSHA = headCommit

	// Create a new worktree from the HEAD commit
	// Otherwise, we'll inherit uncommitted changes from the previous worktree.
	// This way, we can start the worktree with a clean slate.
	// TODO: we might want to give an option to use main/master instead of the current branch.
	if _, err := g.runGitCommand(g.repoPath, "worktree", "add", "-b", g.branchName, g.worktreePath, headCommit); err != nil {
		return fmt.Errorf("failed to create worktree from commit %s: %w", headCommit, err)
	}

	return nil
}

// Cleanup removes the worktree. It deliberately does NOT delete the branch: branch deletion
// via go-git's RemoveReference is not a "safe if merged" check, it is unconditional, and a
// branch can hold commits that exist nowhere else (never pushed, never merged). Silently
// destroying those on session teardown was a live bug — see
// docs/tasks/backlog-feature-improvement.md ("stop_session silently deletes the git branch").
// A leftover local branch ref costs nothing; a lost commit is not recoverable through this
// code path. Equivalent to Remove() — kept as a separate method so callers don't need to know
// the two used to differ.
func (g *GitWorktree) Cleanup() error {
	log.Info("starting cleanup for worktree", "path", g.worktreePath)
	if err := g.Remove(); err != nil {
		return err
	}
	return g.Prune()
}

// Remove removes the worktree but keeps the branch
func (g *GitWorktree) Remove() error {
	log.Info("starting worktree removal", "path", g.worktreePath)

	// First, prune any stale worktree references
	if _, err := g.runGitCommand(g.repoPath, "worktree", "prune"); err != nil {
		// Log the prune error but don't fail - continue with removal
		log.Warn("initial worktree prune failed (continuing with removal)", "err", err)
	} else {
		log.Info("initial worktree prune completed successfully")
	}

	// Check if worktree directory exists before attempting git removal
	worktreeExists := true
	if _, err := os.Stat(g.worktreePath); os.IsNotExist(err) {
		worktreeExists = false
		log.Info("worktree directory does not exist", "path", g.worktreePath)
	}

	// Remove the worktree using git command if directory exists
	if worktreeExists {
		if _, err := g.runGitCommand(g.repoPath, "worktree", "remove", "-f", g.worktreePath); err != nil {
			// Check if this is the common "not a working tree" error - treat it as expected
			errStr := err.Error()
			isCorruptedWorktree := strings.Contains(errStr, "is not a working tree") ||
				strings.Contains(errStr, "not a git repository") ||
				strings.Contains(errStr, "worktree not found")

			if isCorruptedWorktree {
				log.Info("worktree is corrupted/invalid, cleaning up manually", "path", g.worktreePath)
			} else {
				log.Warn("git worktree remove failed", "path", g.worktreePath, "err", err)
			}

			// Try manual directory removal as fallback
			if rmErr := os.RemoveAll(g.worktreePath); rmErr != nil {
				log.Warn("manual directory removal also failed", "path", g.worktreePath, "err", rmErr)
				// Only return error if both git and manual removal fail for non-corrupted worktrees
				if !isCorruptedWorktree {
					return fmt.Errorf("failed to remove worktree: git remove failed (%v), manual remove failed (%v)", err, rmErr)
				} else {
					return fmt.Errorf("failed to remove corrupted worktree directory %s: %v", g.worktreePath, rmErr)
				}
			} else {
				if isCorruptedWorktree {
					log.Info("successfully cleaned up corrupted worktree directory", "path", g.worktreePath)
				} else {
					log.Info("successfully removed worktree directory manually", "path", g.worktreePath)
				}
			}
		} else {
			log.Info("successfully removed worktree with git command", "path", g.worktreePath)
		}
	}

	// Clean up any remaining administrative files
	if err := g.forceCleanupWorktree(); err != nil {
		log.Warn("administrative cleanup had some issues (not critical)", "err", err)
		// Don't fail the removal for admin cleanup issues
	}

	log.Info("worktree removal completed successfully", "path", g.worktreePath)
	return nil
}

// forceCleanupWorktree tries multiple strategies to clean up worktree admin files
func (g *GitWorktree) forceCleanupWorktree() error {
	var cleanupErrors []error

	// Strategy 1: Direct cleanup of git worktree admin files (most reliable)
	worktreesDir := filepath.Join(g.repoPath, ".git", "worktrees")

	// Ensure worktrees directory exists before attempting cleanup
	if _, err := os.Stat(worktreesDir); os.IsNotExist(err) {
		log.Info("git worktrees directory does not exist, no admin cleanup needed", "path", worktreesDir)
		return nil
	}

	worktreeName := filepath.Base(g.worktreePath)
	worktreeAdminDir := filepath.Join(worktreesDir, worktreeName)

	log.Info("attempting cleanup of worktree admin directory", "path", worktreeAdminDir)

	// Remove the exact match administrative directory
	if _, err := os.Stat(worktreeAdminDir); err == nil {
		if rmErr := os.RemoveAll(worktreeAdminDir); rmErr != nil {
			log.Warn("failed to remove worktree admin dir", "path", worktreeAdminDir, "err", rmErr)
			cleanupErrors = append(cleanupErrors, fmt.Errorf("failed to remove admin dir %s: %w", worktreeAdminDir, rmErr))
		} else {
			log.Info("successfully removed worktree admin directory", "path", worktreeAdminDir)
		}
	} else {
		log.Info("exact worktree admin directory does not exist", "path", worktreeAdminDir)
	}

	// Strategy 2: Try to find and remove any matching worktree admin directories
	// This handles cases where the directory name might be slightly different
	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		log.Warn("could not read worktrees directory", "path", worktreesDir, "err", err)
		cleanupErrors = append(cleanupErrors, fmt.Errorf("failed to read worktrees dir: %w", err))
	} else {
		baseWorktreeName := filepath.Base(g.worktreePath)
		for _, entry := range entries {
			if entry.IsDir() && strings.Contains(entry.Name(), baseWorktreeName) {
				adminPath := filepath.Join(worktreesDir, entry.Name())
				// Skip if this is the exact match we already processed
				if adminPath == worktreeAdminDir {
					continue
				}

				log.Info("found matching worktree admin directory", "path", adminPath)
				if rmErr := os.RemoveAll(adminPath); rmErr != nil {
					log.Warn("failed to remove matching admin dir", "path", adminPath, "err", rmErr)
					cleanupErrors = append(cleanupErrors, fmt.Errorf("failed to remove matching admin dir %s: %w", adminPath, rmErr))
				} else {
					log.Info("successfully removed matching worktree admin directory", "path", adminPath)
				}
			}
		}
	}

	// Return combined errors, but don't treat cleanup failures as critical
	if len(cleanupErrors) > 0 {
		return fmt.Errorf("some cleanup operations failed: %v", cleanupErrors)
	}

	// Strategy 3: Try git commands only after manual cleanup
	log.Info("attempting git worktree prune after manual cleanup")
	if _, err := g.runGitCommand(g.repoPath, "worktree", "prune", "--verbose"); err != nil {
		// Prune failed, but we've done manual cleanup, so this is not critical
		log.Warn("git worktree prune failed after manual cleanup (not critical)", "err", err)
	} else {
		log.Info("git worktree prune completed successfully")
	}

	// Strategy 4: Try to list and remove specific worktrees that might still be registered
	if output, err := g.runGitCommand(g.repoPath, "worktree", "list", "--porcelain"); err == nil {
		// Parse the output to find any remaining references to our worktree
		lines := strings.Split(output, "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "worktree ") && strings.Contains(line, filepath.Base(g.worktreePath)) {
				// Found our worktree still listed, try to remove it by path
				worktreePath := strings.TrimPrefix(line, "worktree ")
				log.Info("found worktree still listed, attempting removal", "path", worktreePath)
				if _, err := g.runGitCommand(g.repoPath, "worktree", "remove", "--force", worktreePath); err != nil {
					log.Warn("failed to remove listed worktree (not critical)", "err", err)
				} else {
					log.Info("successfully removed worktree from git registry", "path", worktreePath)
				}
			}
		}
	} else {
		log.Warn("failed to list worktrees for cleanup verification (not critical)", "err", err)
	}

	// Final prune to clean up any remaining stale references
	log.Info("performing final git worktree prune")
	if _, err := g.runGitCommand(g.repoPath, "worktree", "prune"); err != nil {
		log.Warn("final git worktree prune failed (not critical)", "err", err)
	} else {
		log.Info("final worktree prune completed successfully")
	}

	// Always return success - we've done our best with comprehensive cleanup
	log.Info("worktree administrative cleanup completed successfully")
	return nil
}

// Prune removes all working tree administrative files and directories
func (g *GitWorktree) Prune() error {
	if _, err := g.runGitCommand(g.repoPath, "worktree", "prune"); err != nil {
		return fmt.Errorf("failed to prune worktrees: %w", err)
	}
	return nil
}

// CleanupWorktrees removes all worktree directories under the configured worktrees dir.
// It deliberately does NOT delete the associated branches: a branch can hold commits that
// exist nowhere else (never pushed, never merged), and this function has no way to know
// whether that's true for any given one. See GitWorktree.Cleanup's doc comment — same fix,
// same root cause (docs/tasks/backlog-feature-improvement.md).
func CleanupWorktrees() error {
	worktreesDir, err := getWorktreeDirectory()
	if err != nil {
		return fmt.Errorf("failed to get worktree directory: %w", err)
	}

	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		return fmt.Errorf("failed to read worktree directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			os.RemoveAll(filepath.Join(worktreesDir, entry.Name()))
		}
	}

	pruneCtx, pruneCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer pruneCancel()
	if _, err := safeexec.CommandContext(pruneCtx, "git", "worktree", "prune").Output(); err != nil {
		return fmt.Errorf("failed to prune worktrees: %w", err)
	}

	return nil
}
