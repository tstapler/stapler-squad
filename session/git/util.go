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
	"github.com/go-git/go-git/v5/plumbing"
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
		log.Info("directory does not exist, creating and initializing git repository", "path", path)

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

		log.Info("successfully created and initialized git repository with initial commit", "path", path)
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
				log.Info("repository has no commits, creating initial commit", "path", currentPath)
				if err := createInitialCommit(repo, currentPath); err != nil {
					return "", fmt.Errorf("failed to create initial commit at '%s': %w", currentPath, err)
				}
				log.Info("successfully created initial commit", "path", currentPath)
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

// findMainRepoPathForWorktree returns the main repository root for an existing git
// worktree at worktreePath.
//
// This must NOT reuse findGitRepoRoot: that function's "no commits found → create an
// initial commit here" fallback exists for locating or bootstrapping a fresh repo root
// by walking up parent directories, and is actively harmful for a worktree — a
// worktree's `.git` is a redirect FILE ("gitdir: /path/to/main/.git/worktrees/<name>"),
// not a directory to walk up from. This was observed in production and reproduced in a
// unit test: git.PlainOpen on a worktree path can succeed while the subsequent
// repo.Head() call errors (go-git struggling to resolve HEAD for a git-CLI-created
// worktree), which findGitRepoRoot misreads as "no commits" and "fixes" by creating a
// brand-new, disconnected initial commit directly inside the worktree directory and
// returning the worktree's own path as if it were the repo root — corrupting the
// worktree's link to its real branch/history entirely.
//
// Resolves via `git rev-parse --git-common-dir`, which is the shared .git directory a
// worktree and its main repo both point to; the main repo root is its parent.
func findMainRepoPathForWorktree(worktreePath string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := safeexec.CommandContext(ctx, "git", "rev-parse", "--path-format=absolute", "--git-common-dir")
	cmd.Dir = worktreePath
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to resolve git-common-dir for worktree %s: %w", worktreePath, err)
	}
	commonDir := strings.TrimSpace(string(out))
	return filepath.Dir(commonDir), nil
}

// GetCurrentBranchName returns the current branch name for a git repository or worktree.
// Returns an error if the repo is in detached HEAD state.
func GetCurrentBranchName(path string) (string, error) {
	return getCurrentBranchName(path)
}

// getCurrentBranchName returns the current branch name for a git repository or worktree.
// Uses go-git to read HEAD directly (file read, no subprocess).
func getCurrentBranchName(path string) (string, error) {
	repo, err := git.PlainOpenWithOptions(path, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return "", fmt.Errorf("failed to open git repo at %s: %w", path, err)
	}
	// Don't resolve the symbolic ref — we want the branch name, not the commit hash.
	ref, err := repo.Reference(plumbing.HEAD, false)
	if err != nil {
		return "", fmt.Errorf("failed to read HEAD at %s: %w", path, err)
	}
	if ref.Type() != plumbing.SymbolicReference {
		return "", fmt.Errorf("repository at '%s' is in detached HEAD state", path)
	}
	// ref.Target() is e.g. "refs/heads/main"; Short() trims to "main".
	return ref.Target().Short(), nil
}

// GetHeadCommitSHA returns the SHA of the HEAD commit for a git repository or worktree.
func GetHeadCommitSHA(path string) (string, error) { return getHeadCommitSHA(path) }

// headSHARetryAttempts and headSHARetryDelay bound the go-git torn-read mitigation in
// getHeadCommitSHA — see its doc comment.
const (
	headSHARetryAttempts = 3
	headSHARetryDelay    = 20 * time.Millisecond
)

// getHeadCommitSHA returns the SHA of the HEAD commit for a git repository or worktree.
// Reads via go-git (no subprocess) in the common case for speed — this repo can have
// many concurrent sessions, and shelling out to git for every SHA lookup adds real
// process overhead at that scale.
//
// go-git was observed to unreliably resolve HEAD for a worktree created via the git
// CLI's `git worktree add`, in two distinct ways: (1) repo.Head() erroring outright
// immediately after worktree creation (reproduced deterministically in a unit test,
// single-threaded, no concurrency involved — a go-git worktree-gitdir-parsing gap, not
// a race), and (2) in production, repo.Head() returning a syntactically-valid SHA that
// did not correspond to any real object in the repository at all (failed git cat-file
// -t, absent from git rev-list --all, git reflog show --all, and git fsck
// --unreachable) — consistent with go-git's unlocked, direct ref-file read racing a
// concurrent git operation against the same parent repo, unprotected by git's own
// atomic-rename ref-update guarantees.
//
// So every go-git read here is verified against go-git's own object store
// (repo.CommitObject, still no subprocess) before being trusted, and a Head() error is
// treated the same as an invalid result; both retry briefly, and only after repeated
// failures do we fall back to the git CLI, which is immune to both failure modes.
func getHeadCommitSHA(path string) (string, error) {
	// A failed open (path doesn't exist / isn't a git repo at all) is not retryable —
	// only the subsequent Head() resolution and object-existence check are, since those
	// are what were observed to fail transiently for worktrees.
	if _, err := git.PlainOpenWithOptions(path, &git.PlainOpenOptions{DetectDotGit: true}); err != nil {
		return "", fmt.Errorf("failed to open git repo at %s: %w", path, err)
	}

	var lastErr error
	for attempt := 0; attempt < headSHARetryAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(headSHARetryDelay)
		}
		// Re-open fresh each attempt in case go-git's failure originates in gitdir/ref
		// parsing at open time, not just in the Repository object's later reads.
		repo, err := git.PlainOpenWithOptions(path, &git.PlainOpenOptions{DetectDotGit: true})
		if err != nil {
			lastErr = fmt.Errorf("failed to open git repo at %s: %w", path, err)
			continue
		}
		ref, err := repo.Head()
		if err != nil {
			lastErr = fmt.Errorf("failed to read HEAD at %s: %w", path, err)
			continue
		}
		hash := ref.Hash()
		if _, commitErr := repo.CommitObject(hash); commitErr == nil {
			return hash.String(), nil
		}
		lastErr = fmt.Errorf("go-git resolved HEAD to %s at %s, which is not a real commit object", hash, path)
	}
	log.Warn("getHeadCommitSHA: go-git repeatedly resolved HEAD to a nonexistent object, falling back to git CLI", "path", path, "err", lastErr)
	return getHeadCommitSHAViaCLI(path)
}

// getHeadCommitSHAViaCLI is the fallback path for getHeadCommitSHA when go-git
// repeatedly fails to resolve HEAD to a real object. The git CLI relies on atomic-rename
// ref updates, so it doesn't observe the torn-read race go-git is susceptible to.
func getHeadCommitSHAViaCLI(path string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := safeexec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = path
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to read HEAD at %s: %w", path, err)
	}
	return strings.TrimSpace(string(out)), nil
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
				log.Error("InitializeProjectDirectory: rollback failed", "path", path, "err", rmErr)
			}
		}
		return fmt.Errorf("failed to init git repo: %w", err)
	}

	// 5. Initial commit (reuses the existing createInitialCommit helper)
	if err := createInitialCommit(repo, path); err != nil {
		if dirCreated {
			if rmErr := os.RemoveAll(path); rmErr != nil {
				log.Error("InitializeProjectDirectory: rollback failed", "path", path, "err", rmErr)
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
