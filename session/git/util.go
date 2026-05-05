package git

import (
	"context"
	"fmt"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// sanitizeBranchName transforms an arbitrary string into a Git branch name friendly string.
// Note: Git branch names have several rules, so this function uses a simple approach
// by allowing only a safe subset of characters.
func sanitizeBranchName(s string) string {
	// Convert to lower-case
	s = strings.ToLower(s)

	// Replace spaces with a dash
	s = strings.ReplaceAll(s, " ", "-")

	// Remove any characters not allowed in our safe subset.
	// Here we allow: letters, digits, dash, underscore, slash, and dot.
	re := regexp.MustCompile(`[^a-z0-9\-_/.]+`)
	s = re.ReplaceAllString(s, "")

	// Replace multiple dashes with a single dash (optional cleanup)
	reDash := regexp.MustCompile(`-+`)
	s = reDash.ReplaceAllString(s, "-")

	// Trim leading and trailing dashes or slashes to avoid issues
	s = strings.Trim(s, "-/")

	return s
}

// checkGHCLI checks if GitHub CLI is installed and configured
func checkGHCLI() error {
	// Check if gh is installed
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("GitHub CLI (gh) is not installed. Please install it first")
	}

	// Check if gh is authenticated
	authCtx, authCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer authCancel()
	cmd := safeexec.CommandContext(authCtx, "gh", "auth", "status")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("GitHub CLI is not configured. Please run 'gh auth login' first")
	}

	return nil
}

// IsGitRepo checks if the given path is within a git repository
func IsGitRepo(path string) bool {
	for {
		_, err := git.PlainOpen(path)
		if err == nil {
			return true
		}

		parent := filepath.Dir(path)
		if parent == path {
			return false
		}
		path = parent
	}
}

func findGitRepoRoot(path string) (string, error) {
	// First check if the directory exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// Directory doesn't exist - create it and initialize git
		log.InfoLog.Printf("Directory '%s' doesn't exist, creating it and initializing git repository", path)

		if err := os.MkdirAll(path, 0755); err != nil {
			return "", fmt.Errorf("failed to create directory '%s': %w", path, err)
		}

		// Initialize git repository
		repo, err := git.PlainInit(path, false)
		if err != nil {
			return "", fmt.Errorf("failed to initialize git repository at '%s': %w", path, err)
		}

		// Create initial commit (required for worktrees)
		// Git worktrees require at least one commit to exist
		if err := createInitialCommit(repo, path); err != nil {
			return "", fmt.Errorf("failed to create initial commit at '%s': %w", path, err)
		}

		log.InfoLog.Printf("Successfully created and initialized git repository at '%s' with initial commit", path)
		return path, nil
	}

	// Directory exists - find the git repo root
	currentPath := path
	for {
		repo, err := git.PlainOpen(currentPath)
		if err == nil {
			// Found the repository root
			// Check if the repository has any commits (worktrees require at least one)
			_, err := repo.Head()
			if err != nil {
				// Repository has no commits - create initial commit
				log.InfoLog.Printf("Repository at '%s' has no commits, creating initial commit", currentPath)
				if err := createInitialCommit(repo, currentPath); err != nil {
					return "", fmt.Errorf("failed to create initial commit at '%s': %w", currentPath, err)
				}
				log.InfoLog.Printf("Successfully created initial commit at '%s'", currentPath)
			}
			return currentPath, nil
		}

		parent := filepath.Dir(currentPath)
		if parent == currentPath {
			// Reached the filesystem root without finding a repository
			return "", fmt.Errorf("failed to find Git repository root from path: %s", path)
		}
		currentPath = parent
	}
}

// getCurrentBranchName returns the current branch name for a git repository or worktree
func getCurrentBranchName(path string) (string, error) {
	branchCtx, branchCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer branchCancel()
	cmd := safeexec.CommandContext(branchCtx, "git", "-C", path, "branch", "--show-current")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current branch name: %w", err)
	}

	branchName := strings.TrimSpace(string(output))
	if branchName == "" {
		return "", fmt.Errorf("repository at '%s' is in detached HEAD state or has no branches", path)
	}

	return branchName, nil
}

// getHeadCommitSHA returns the SHA of the HEAD commit for a git repository or worktree
func getHeadCommitSHA(path string) (string, error) {
	shaCtx, shaCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shaCancel()
	cmd := safeexec.CommandContext(shaCtx, "git", "-C", path, "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD commit SHA: %w", err)
	}

	commitSHA := strings.TrimSpace(string(output))
	if commitSHA == "" {
		return "", fmt.Errorf("failed to get HEAD commit SHA: empty output")
	}

	return commitSHA, nil
}

// InitializeProjectDirectory creates a directory and initializes it as a git repository.
// Behavior by pre-existing state:
//   - Path does not exist: creates with os.MkdirAll(path, 0755), runs git init, commits.
//   - Path exists, no .git: runs git init in place, commits.
//   - Path exists, already a git repo: no-op, returns nil.
//   - Path exists but is a regular file: returns an error.
//
// On partial failure (dir created, git init failed): attempts os.RemoveAll to roll back
// the newly created directory. Logs a warning if rollback also fails.
func InitializeProjectDirectory(path string) error {
	// 1. Check if already a git repo (open succeeds) → no-op
	if _, err := git.PlainOpen(path); err == nil {
		return nil
	}

	// 2. Check for file collision
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		return fmt.Errorf("path exists and is not a directory: %s", path)
	}

	// 3. Track whether we created the directory so we can roll back on failure
	dirCreated := false
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
		dirCreated = true
	}

	// 4. git init
	repo, err := git.PlainInit(path, false)
	if err != nil {
		if dirCreated {
			if rmErr := os.RemoveAll(path); rmErr != nil {
				log.ErrorLog.Printf("InitializeProjectDirectory: rollback failed for %s: %v", path, rmErr)
			}
		}
		return fmt.Errorf("failed to init git repo: %w", err)
	}

	// 5. Initial commit (reuses the existing createInitialCommit helper)
	if err := createInitialCommit(repo, path); err != nil {
		if dirCreated {
			if rmErr := os.RemoveAll(path); rmErr != nil {
				log.ErrorLog.Printf("InitializeProjectDirectory: rollback failed for %s: %v", path, rmErr)
			}
		}
		return fmt.Errorf("failed to create initial commit: %w", err)
	}

	return nil
}

// createInitialCommit creates an initial commit in a new git repository
// This is required because git worktrees need at least one commit to exist
func createInitialCommit(repo *git.Repository, repoPath string) error {
	// Get the worktree
	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Create a .gitignore file as the initial commit content
	gitignorePath := filepath.Join(repoPath, ".gitignore")
	gitignoreContent := []byte("# Project gitignore\n")
	if err := os.WriteFile(gitignorePath, gitignoreContent, 0644); err != nil {
		return fmt.Errorf("failed to create .gitignore: %w", err)
	}

	// Add .gitignore to staging
	if _, err := worktree.Add(".gitignore"); err != nil {
		return fmt.Errorf("failed to add .gitignore: %w", err)
	}

	// Create the initial commit
	_, err = worktree.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Stapler Squad",
			Email: "stapler-squad@localhost",
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create initial commit: %w", err)
	}

	return nil
}
