package github

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
)

const (
	// DefaultCloneBase is the default base directory for cloned repositories
	DefaultCloneBase = "~/.stapler-squad/repos"
)

// CloneOptions specifies options for cloning or accessing a repository
type CloneOptions struct {
	Owner   string
	Repo    string
	Branch  string // Optional: specific branch to checkout after cloning
	Shallow bool   // Use shallow clone (--depth=1) for faster cloning
}

// CloneResult contains information about a cloned or existing repository
type CloneResult struct {
	Path      string // Full path to the repository
	WasCloned bool   // True if we just cloned it, false if it already existed
	Branch    string // Current branch name
}

// GetClonePath returns the path where a repository would be cloned
// Format: ~/.stapler-squad/repos/{owner}/{repo}
func GetClonePath(owner, repo string) string {
	base := expandPath(DefaultCloneBase)
	return filepath.Join(base, owner, repo)
}

// FindExistingClone checks if a repository is already cloned
// Returns the path and true if found, empty string and false otherwise
func FindExistingClone(owner, repo string) (string, bool) {
	clonePath := GetClonePath(owner, repo)

	// Check if .git directory exists (indicating a git repository)
	gitDir := filepath.Join(clonePath, ".git")
	if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
		return clonePath, true
	}

	return "", false
}

// GetOrCloneRepository ensures a repository is available locally
// It will clone it if not already present, or return the existing clone
func GetOrCloneRepository(opts CloneOptions) (*CloneResult, error) {
	// First check if already cloned
	if existingPath, found := FindExistingClone(opts.Owner, opts.Repo); found {
		result := &CloneResult{
			Path:      existingPath,
			WasCloned: false,
		}

		// If a specific branch is requested, fetch and checkout
		if opts.Branch != "" {
			if err := FetchBranch(existingPath, opts.Branch); err != nil {
				// Fetch might fail if branch doesn't exist remotely, try checkout anyway
				// as it might be a local branch
			}

			// Create worktree or checkout the branch
			if err := CheckoutBranch(existingPath, opts.Branch); err != nil {
				return nil, fmt.Errorf("failed to checkout branch '%s': %w", opts.Branch, err)
			}
			result.Branch = opts.Branch
		} else {
			// Get current branch name
			branch, err := getCurrentBranch(existingPath)
			if err != nil {
				result.Branch = "main" // Default assumption
			} else {
				result.Branch = branch
			}
		}

		return result, nil
	}

	// Need to clone the repository
	clonePath := GetClonePath(opts.Owner, opts.Repo)

	// Ensure parent directory exists
	parentDir := filepath.Dir(clonePath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory '%s': %w", parentDir, err)
	}

	// Clone the repository
	if err := CloneRepository(opts.Owner, opts.Repo, clonePath); err != nil {
		return nil, fmt.Errorf("failed to clone repository: %w", err)
	}

	result := &CloneResult{
		Path:      clonePath,
		WasCloned: true,
	}

	// If a specific branch is requested, checkout
	if opts.Branch != "" {
		if err := FetchBranch(clonePath, opts.Branch); err != nil {
			// Might be the default branch, continue
		}
		if err := CheckoutBranch(clonePath, opts.Branch); err != nil {
			return nil, fmt.Errorf("failed to checkout branch '%s': %w", opts.Branch, err)
		}
		result.Branch = opts.Branch
	} else {
		// Get current branch name (usually main or master after clone)
		branch, err := getCurrentBranch(clonePath)
		if err != nil {
			result.Branch = "main" // Default assumption
		} else {
			result.Branch = branch
		}
	}

	return result, nil
}

// EnsureCloneDirectory ensures the base clone directory exists
func EnsureCloneDirectory() error {
	base := expandPath(DefaultCloneBase)
	return os.MkdirAll(base, 0755)
}

// CleanupClone removes a cloned repository
// Use with caution - this deletes the entire repository directory
func CleanupClone(owner, repo string) error {
	clonePath := GetClonePath(owner, repo)

	// Verify it's in our clone directory before removing (safety check)
	base := expandPath(DefaultCloneBase)
	if !isSubPath(base, clonePath) {
		return fmt.Errorf("refusing to delete path outside clone directory: %s", clonePath)
	}

	return os.RemoveAll(clonePath)
}

// ListClonedRepos returns a list of all cloned repositories
func ListClonedRepos() ([]string, error) {
	base := expandPath(DefaultCloneBase)

	if _, err := os.Stat(base); os.IsNotExist(err) {
		return []string{}, nil
	}

	var repos []string

	// Walk through owner directories
	owners, err := os.ReadDir(base)
	if err != nil {
		return nil, err
	}

	for _, owner := range owners {
		if !owner.IsDir() {
			continue
		}

		ownerPath := filepath.Join(base, owner.Name())
		repoEntries, err := os.ReadDir(ownerPath)
		if err != nil {
			continue
		}

		for _, repo := range repoEntries {
			if !repo.IsDir() {
				continue
			}

			// Check if it's a valid git repository
			gitDir := filepath.Join(ownerPath, repo.Name(), ".git")
			if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
				repos = append(repos, fmt.Sprintf("%s/%s", owner.Name(), repo.Name()))
			}
		}
	}

	return repos, nil
}

// expandPath expands ~ to the user's home directory
func expandPath(path string) string {
	if len(path) == 0 || path[0] != '~' {
		return path
	}

	usr, err := user.Current()
	if err != nil {
		return path
	}

	if path == "~" {
		return usr.HomeDir
	}

	if len(path) > 1 && path[1] == '/' {
		return filepath.Join(usr.HomeDir, path[2:])
	}

	return path
}

// isSubPath checks if path is under basePath
func isSubPath(basePath, path string) bool {
	rel, err := filepath.Rel(basePath, path)
	if err != nil {
		return false
	}
	return !filepath.IsAbs(rel) && rel != ".." && !startsWith(rel, "..")
}

// startsWith checks if s starts with prefix
func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// getCurrentBranch returns the current branch name of a repository
func getCurrentBranch(repoPath string) (string, error) {
	// Read HEAD to determine current branch
	headPath := filepath.Join(repoPath, ".git", "HEAD")
	data, err := os.ReadFile(headPath)
	if err != nil {
		return "", err
	}

	content := string(data)
	// HEAD usually contains "ref: refs/heads/branchname\n"
	if len(content) > 16 && content[:16] == "ref: refs/heads/" {
		return content[16 : len(content)-1], nil // Remove "ref: refs/heads/" prefix and trailing newline
	}

	// Detached HEAD or other state
	return "", fmt.Errorf("could not determine current branch")
}
