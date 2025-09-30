package git

import (
	"claude-squad/log"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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

// setupFromExistingBranch creates a worktree from an existing branch
func (g *GitWorktree) setupFromExistingBranch() error {
	// Directory already created in Setup(), skip duplicate creation

	// Clean up any existing worktree first
	_, _ = g.runGitCommand(g.repoPath, "worktree", "remove", "-f", g.worktreePath) // Ignore error if worktree doesn't exist

	// Create a new worktree from the existing branch
	if _, err := g.runGitCommand(g.repoPath, "worktree", "add", g.worktreePath, g.branchName); err != nil {
		// Check if the error is because the branch is already checked out elsewhere
		if strings.Contains(err.Error(), "already checked out") {
			// Try to find and connect to the existing worktree
			log.InfoLog.Printf("Branch '%s' is already checked out, attempting to locate existing worktree", g.branchName)

			// List all worktrees to find where this branch is checked out
			output, listErr := g.runGitCommand(g.repoPath, "worktree", "list", "--porcelain")
			if listErr != nil {
				return fmt.Errorf("failed to list worktrees while handling checkout conflict: %w", listErr)
			}

			// Parse worktree list to find the one with our branch
			existingPath, found := g.findWorktreeForBranch(output, g.branchName)
			if found {
				log.InfoLog.Printf("Found existing worktree for branch '%s' at '%s', using it instead", g.branchName, existingPath)
				g.worktreePath = existingPath
				return nil
			}

			// If we can't find the existing worktree, return the original error
			return fmt.Errorf("failed to create worktree from branch %s (branch already checked out elsewhere, but could not locate existing worktree): %w", g.branchName, err)
		}

		return fmt.Errorf("failed to create worktree from branch %s: %w", g.branchName, err)
	}

	return nil
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

	// Clean up any existing branch or reference
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

// Cleanup removes the worktree and associated branch
func (g *GitWorktree) Cleanup() error {
	var errs []error

	log.InfoLog.Printf("Starting cleanup for worktree: %s", g.worktreePath)

	// Step 1: Check if worktree directory exists
	worktreeExists := true
	if _, err := os.Stat(g.worktreePath); os.IsNotExist(err) {
		worktreeExists = false
		log.InfoLog.Printf("Worktree directory does not exist: %s", g.worktreePath)
	}

	// Step 2: First prune any stale worktree references (always safe to do)
	if _, err := g.runGitCommand(g.repoPath, "worktree", "prune"); err != nil {
		// Log the prune error but don't fail - continue with removal
		log.WarningLog.Printf("Warning: failed to prune worktrees during cleanup: %v", err)
	}

	// Step 3: Try to remove the worktree using git command if it exists
	if worktreeExists {
		if _, err := g.runGitCommand(g.repoPath, "worktree", "remove", "-f", g.worktreePath); err != nil {
			// Check if this is the common "not a working tree" error - treat it as expected
			errStr := err.Error()
			isCorruptedWorktree := strings.Contains(errStr, "is not a working tree") ||
								  strings.Contains(errStr, "not a git repository") ||
								  strings.Contains(errStr, "worktree not found")

			if isCorruptedWorktree {
				log.InfoLog.Printf("Worktree is corrupted/invalid, cleaning up manually: %s", g.worktreePath)
			} else {
				log.WarningLog.Printf("Git worktree remove failed for %s: %v", g.worktreePath, err)
			}

			// If git command fails, try manual directory removal
			if rmErr := os.RemoveAll(g.worktreePath); rmErr != nil {
				log.WarningLog.Printf("Manual directory removal failed for %s: %v", g.worktreePath, rmErr)
				// Only add to errors if both git and manual removal fail
				errs = append(errs, fmt.Errorf("failed to remove worktree directory %s: git remove failed (%v), manual remove failed (%v)",
					g.worktreePath, err, rmErr))
			} else {
				if isCorruptedWorktree {
					log.InfoLog.Printf("Successfully cleaned up corrupted worktree directory: %s", g.worktreePath)
				} else {
					log.InfoLog.Printf("Successfully removed worktree directory manually: %s", g.worktreePath)
				}
			}
		} else {
			log.InfoLog.Printf("Successfully removed worktree with git command: %s", g.worktreePath)
		}
	}

	// Step 4: Always attempt to clean up git administrative files (safe even if directory is gone)
	if err := g.forceCleanupWorktree(); err != nil {
		log.WarningLog.Printf("Failed to cleanup worktree admin files for %s: %v", g.worktreePath, err)
		// Don't add to errors - this is supplementary cleanup
	}

	// Open the repository for branch cleanup
	repo, err := git.PlainOpen(g.repoPath)
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to open repository for cleanup: %w", err))
		return g.combineErrors(errs)
	}

	branchRef := plumbing.NewBranchReferenceName(g.branchName)

	// Check if branch exists before attempting removal
	if _, err := repo.Reference(branchRef, false); err == nil {
		if err := repo.Storer.RemoveReference(branchRef); err != nil {
			errs = append(errs, fmt.Errorf("failed to remove branch %s: %w", g.branchName, err))
		}
	} else if err != plumbing.ErrReferenceNotFound {
		errs = append(errs, fmt.Errorf("error checking branch %s existence: %w", g.branchName, err))
	}

	// Prune the worktree to clean up any remaining references
	if err := g.Prune(); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return g.combineErrors(errs)
	}

	return nil
}

// Remove removes the worktree but keeps the branch
func (g *GitWorktree) Remove() error {
	log.InfoLog.Printf("Starting worktree removal for: %s", g.worktreePath)

	// First, prune any stale worktree references
	if _, err := g.runGitCommand(g.repoPath, "worktree", "prune"); err != nil {
		// Log the prune error but don't fail - continue with removal
		log.WarningLog.Printf("Initial worktree prune failed (continuing with removal): %v", err)
	} else {
		log.InfoLog.Printf("Initial worktree prune completed successfully")
	}

	// Check if worktree directory exists before attempting git removal
	worktreeExists := true
	if _, err := os.Stat(g.worktreePath); os.IsNotExist(err) {
		worktreeExists = false
		log.InfoLog.Printf("Worktree directory does not exist: %s", g.worktreePath)
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
				log.InfoLog.Printf("Worktree is corrupted/invalid, cleaning up manually: %s", g.worktreePath)
			} else {
				log.WarningLog.Printf("Git worktree remove failed for %s: %v", g.worktreePath, err)
			}

			// Try manual directory removal as fallback
			if rmErr := os.RemoveAll(g.worktreePath); rmErr != nil {
				log.WarningLog.Printf("Manual directory removal also failed for %s: %v", g.worktreePath, rmErr)
				// Only return error if both git and manual removal fail for non-corrupted worktrees
				if !isCorruptedWorktree {
					return fmt.Errorf("failed to remove worktree: git remove failed (%v), manual remove failed (%v)", err, rmErr)
				} else {
					return fmt.Errorf("failed to remove corrupted worktree directory %s: %v", g.worktreePath, rmErr)
				}
			} else {
				if isCorruptedWorktree {
					log.InfoLog.Printf("Successfully cleaned up corrupted worktree directory: %s", g.worktreePath)
				} else {
					log.InfoLog.Printf("Successfully removed worktree directory manually: %s", g.worktreePath)
				}
			}
		} else {
			log.InfoLog.Printf("Successfully removed worktree with git command: %s", g.worktreePath)
		}
	}

	// Clean up any remaining administrative files
	if err := g.forceCleanupWorktree(); err != nil {
		log.WarningLog.Printf("Administrative cleanup had some issues (not critical): %v", err)
		// Don't fail the removal for admin cleanup issues
	}

	log.InfoLog.Printf("Worktree removal completed successfully: %s", g.worktreePath)
	return nil
}

// forceCleanupWorktree tries multiple strategies to clean up worktree admin files
func (g *GitWorktree) forceCleanupWorktree() error {
	var cleanupErrors []error

	// Strategy 1: Direct cleanup of git worktree admin files (most reliable)
	worktreesDir := filepath.Join(g.repoPath, ".git", "worktrees")

	// Ensure worktrees directory exists before attempting cleanup
	if _, err := os.Stat(worktreesDir); os.IsNotExist(err) {
		log.InfoLog.Printf("Git worktrees directory does not exist, no admin cleanup needed: %s", worktreesDir)
		return nil
	}

	worktreeName := filepath.Base(g.worktreePath)
	worktreeAdminDir := filepath.Join(worktreesDir, worktreeName)

	log.InfoLog.Printf("Attempting cleanup of worktree admin directory: %s", worktreeAdminDir)

	// Remove the exact match administrative directory
	if _, err := os.Stat(worktreeAdminDir); err == nil {
		if rmErr := os.RemoveAll(worktreeAdminDir); rmErr != nil {
			log.WarningLog.Printf("Failed to remove worktree admin dir %s: %v", worktreeAdminDir, rmErr)
			cleanupErrors = append(cleanupErrors, fmt.Errorf("failed to remove admin dir %s: %w", worktreeAdminDir, rmErr))
		} else {
			log.InfoLog.Printf("Successfully removed worktree admin directory: %s", worktreeAdminDir)
		}
	} else {
		log.InfoLog.Printf("Exact worktree admin directory does not exist: %s", worktreeAdminDir)
	}

	// Strategy 2: Try to find and remove any matching worktree admin directories
	// This handles cases where the directory name might be slightly different
	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		log.WarningLog.Printf("Could not read worktrees directory %s: %v", worktreesDir, err)
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

				log.InfoLog.Printf("Found matching worktree admin directory: %s", adminPath)
				if rmErr := os.RemoveAll(adminPath); rmErr != nil {
					log.WarningLog.Printf("Failed to remove matching admin dir %s: %v", adminPath, rmErr)
					cleanupErrors = append(cleanupErrors, fmt.Errorf("failed to remove matching admin dir %s: %w", adminPath, rmErr))
				} else {
					log.InfoLog.Printf("Successfully removed matching worktree admin directory: %s", adminPath)
				}
			}
		}
	}

	// Return combined errors, but don't treat cleanup failures as critical
	if len(cleanupErrors) > 0 {
		return fmt.Errorf("some cleanup operations failed: %v", cleanupErrors)
	}

	// Strategy 3: Try git commands only after manual cleanup
	log.InfoLog.Printf("Attempting git worktree prune after manual cleanup")
	if _, err := g.runGitCommand(g.repoPath, "worktree", "prune", "--verbose"); err != nil {
		// Prune failed, but we've done manual cleanup, so this is not critical
		log.WarningLog.Printf("Git worktree prune failed after manual cleanup (not critical): %v", err)
	} else {
		log.InfoLog.Printf("Git worktree prune completed successfully")
	}

	// Strategy 4: Try to list and remove specific worktrees that might still be registered
	if output, err := g.runGitCommand(g.repoPath, "worktree", "list", "--porcelain"); err == nil {
		// Parse the output to find any remaining references to our worktree
		lines := strings.Split(output, "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "worktree ") && strings.Contains(line, filepath.Base(g.worktreePath)) {
				// Found our worktree still listed, try to remove it by path
				worktreePath := strings.TrimPrefix(line, "worktree ")
				log.InfoLog.Printf("Found worktree still listed, attempting removal: %s", worktreePath)
				if _, err := g.runGitCommand(g.repoPath, "worktree", "remove", "--force", worktreePath); err != nil {
					log.WarningLog.Printf("Failed to remove listed worktree (not critical): %v", err)
				} else {
					log.InfoLog.Printf("Successfully removed worktree from git registry: %s", worktreePath)
				}
			}
		}
	} else {
		log.WarningLog.Printf("Failed to list worktrees for cleanup verification (not critical): %v", err)
	}

	// Final prune to clean up any remaining stale references
	log.InfoLog.Printf("Performing final git worktree prune")
	if _, err := g.runGitCommand(g.repoPath, "worktree", "prune"); err != nil {
		log.WarningLog.Printf("Final git worktree prune failed (not critical): %v", err)
	} else {
		log.InfoLog.Printf("Final worktree prune completed successfully")
	}

	// Always return success - we've done our best with comprehensive cleanup
	log.InfoLog.Printf("Worktree administrative cleanup completed successfully")
	return nil
}

// Prune removes all working tree administrative files and directories
func (g *GitWorktree) Prune() error {
	if _, err := g.runGitCommand(g.repoPath, "worktree", "prune"); err != nil {
		return fmt.Errorf("failed to prune worktrees: %w", err)
	}
	return nil
}

// CleanupWorktrees removes all worktrees and their associated branches
func CleanupWorktrees() error {
	worktreesDir, err := getWorktreeDirectory()
	if err != nil {
		return fmt.Errorf("failed to get worktree directory: %w", err)
	}

	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		return fmt.Errorf("failed to read worktree directory: %w", err)
	}

	// Get a list of all branches associated with worktrees
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to list worktrees: %w", err)
	}

	// Parse the output to extract branch names
	worktreeBranches := make(map[string]string)
	currentWorktree := ""
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "worktree ") {
			currentWorktree = strings.TrimPrefix(line, "worktree ")
		} else if strings.HasPrefix(line, "branch ") {
			branchPath := strings.TrimPrefix(line, "branch ")
			// Extract branch name from refs/heads/branch-name
			branchName := strings.TrimPrefix(branchPath, "refs/heads/")
			if currentWorktree != "" {
				worktreeBranches[currentWorktree] = branchName
			}
		}
	}

	for _, entry := range entries {
		if entry.IsDir() {
			worktreePath := filepath.Join(worktreesDir, entry.Name())

			// Delete the branch associated with this worktree if found
			for path, branch := range worktreeBranches {
				if strings.Contains(path, entry.Name()) {
					// Delete the branch
					deleteCmd := exec.Command("git", "branch", "-D", branch)
					if err := deleteCmd.Run(); err != nil {
						// Log the error but continue with other worktrees
						log.ErrorLog.Printf("failed to delete branch %s: %v", branch, err)
					}
					break
				}
			}

			// Remove the worktree directory
			os.RemoveAll(worktreePath)
		}
	}

	// You have to prune the cleaned up worktrees.
	cmd = exec.Command("git", "worktree", "prune")
	_, err = cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to prune worktrees: %w", err)
	}

	return nil
}
