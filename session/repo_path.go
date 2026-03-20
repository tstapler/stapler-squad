package session

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tstapler/stapler-squad/log"
)

// RepoPathManager handles GOPATH-style repository path management.
// Repositories are stored in a consistent location based on their URL:
//   - ~/.stapler-squad/repos/github.com/owner/repo (main clone)
//   - Worktrees are created relative to the main repo as needed
type RepoPathManager struct {
	baseDir string // Base directory for repos (default: ~/.stapler-squad/repos)
}

// NewRepoPathManager creates a new RepoPathManager with the default base directory.
func NewRepoPathManager() *RepoPathManager {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = os.TempDir()
	}
	return &RepoPathManager{
		baseDir: filepath.Join(homeDir, ".stapler-squad", "repos"),
	}
}

// NewRepoPathManagerWithBase creates a RepoPathManager with a custom base directory.
func NewRepoPathManagerWithBase(baseDir string) *RepoPathManager {
	return &RepoPathManager{
		baseDir: baseDir,
	}
}

// GitHubRef represents a parsed GitHub reference.
type GitHubRef struct {
	Owner    string
	Repo     string
	Branch   string
	PRNumber int
	Type     GitHubRefType
}

// GitHubRefType indicates what kind of GitHub reference this is.
type GitHubRefType int

const (
	GitHubRefTypeRepo GitHubRefType = iota
	GitHubRefTypeBranch
	GitHubRefTypePR
)

// ParseGitHubURL parses a GitHub URL and returns the components.
// Supported formats:
//   - https://github.com/owner/repo
//   - https://github.com/owner/repo.git
//   - https://github.com/owner/repo/tree/branch
//   - https://github.com/owner/repo/pull/123
//   - owner/repo (shorthand)
//   - owner/repo:branch (shorthand with branch)
func ParseGitHubURL(input string) (*GitHubRef, error) {
	input = strings.TrimSpace(input)

	// GitHub PR URL pattern
	prPattern := regexp.MustCompile(`^https?://github\.com/([^/]+)/([^/]+)/pull/(\d+)`)
	if match := prPattern.FindStringSubmatch(input); match != nil {
		prNum := 0
		fmt.Sscanf(match[3], "%d", &prNum)
		return &GitHubRef{
			Owner:    match[1],
			Repo:     strings.TrimSuffix(match[2], ".git"),
			PRNumber: prNum,
			Type:     GitHubRefTypePR,
		}, nil
	}

	// GitHub branch URL pattern
	branchPattern := regexp.MustCompile(`^https?://github\.com/([^/]+)/([^/]+)/tree/(.+)$`)
	if match := branchPattern.FindStringSubmatch(input); match != nil {
		return &GitHubRef{
			Owner:  match[1],
			Repo:   strings.TrimSuffix(match[2], ".git"),
			Branch: match[3],
			Type:   GitHubRefTypeBranch,
		}, nil
	}

	// GitHub repo URL pattern (must come after branch pattern)
	repoPattern := regexp.MustCompile(`^https?://github\.com/([^/]+)/([^/]+?)(?:\.git)?/?$`)
	if match := repoPattern.FindStringSubmatch(input); match != nil {
		return &GitHubRef{
			Owner: match[1],
			Repo:  strings.TrimSuffix(match[2], ".git"),
			Type:  GitHubRefTypeRepo,
		}, nil
	}

	// Shorthand pattern: owner/repo or owner/repo:branch
	shorthandPattern := regexp.MustCompile(`^([a-zA-Z0-9_-]+)/([a-zA-Z0-9_.-]+)(?::([a-zA-Z0-9_/.-]+))?$`)
	if match := shorthandPattern.FindStringSubmatch(input); match != nil {
		// Make sure it doesn't look like a local path
		if !strings.HasPrefix(input, "/") && !strings.HasPrefix(input, "~") && !strings.HasPrefix(input, ".") {
			ref := &GitHubRef{
				Owner: match[1],
				Repo:  match[2],
				Type:  GitHubRefTypeRepo,
			}
			if match[3] != "" {
				ref.Branch = match[3]
				ref.Type = GitHubRefTypeBranch
			}
			return ref, nil
		}
	}

	return nil, fmt.Errorf("not a recognized GitHub URL format: %s", input)
}

// IsGitHubURL returns true if the input looks like a GitHub URL or shorthand.
func IsGitHubURL(input string) bool {
	_, err := ParseGitHubURL(input)
	return err == nil
}

// GetRepoPath returns the local path where a GitHub repo should be stored.
// Format: ~/.stapler-squad/repos/github.com/owner/repo
func (m *RepoPathManager) GetRepoPath(ref *GitHubRef) string {
	return filepath.Join(m.baseDir, "github.com", ref.Owner, ref.Repo)
}

// GetCloneURL returns the git clone URL for a GitHub ref.
func (m *RepoPathManager) GetCloneURL(ref *GitHubRef) string {
	return fmt.Sprintf("https://github.com/%s/%s.git", ref.Owner, ref.Repo)
}

// EnsureRepoCloned ensures the repository is cloned to the local path.
// If already cloned, it fetches the latest changes.
// Returns the path to the cloned repository.
func (m *RepoPathManager) EnsureRepoCloned(ref *GitHubRef) (string, error) {
	repoPath := m.GetRepoPath(ref)
	cloneURL := m.GetCloneURL(ref)

	// Check if repo already exists
	if _, err := os.Stat(filepath.Join(repoPath, ".git")); err == nil {
		// Repo exists, fetch latest
		log.InfoLog.Printf("[RepoPath] Repository exists at %s, fetching latest...", repoPath)
		cmd := exec.Command("git", "-C", repoPath, "fetch", "--all", "--prune")
		if output, err := cmd.CombinedOutput(); err != nil {
			log.WarningLog.Printf("[RepoPath] Failed to fetch: %v\nOutput: %s", err, string(output))
			// Don't fail - the existing repo is still usable
		}
		return repoPath, nil
	}

	// Create parent directory
	parentDir := filepath.Dir(repoPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %w", parentDir, err)
	}

	// Clone the repository
	log.InfoLog.Printf("[RepoPath] Cloning %s to %s...", cloneURL, repoPath)
	cmd := exec.Command("git", "clone", cloneURL, repoPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to clone repository: %w\nOutput: %s", err, string(output))
	}

	log.InfoLog.Printf("[RepoPath] Successfully cloned %s/%s", ref.Owner, ref.Repo)
	return repoPath, nil
}

// ResolveGitHubInput takes a GitHub URL/shorthand and returns a resolved path.
// It clones the repo if necessary and returns the local path.
// Also returns the parsed GitHubRef for storing metadata.
func (m *RepoPathManager) ResolveGitHubInput(input string) (localPath string, ref *GitHubRef, err error) {
	ref, err = ParseGitHubURL(input)
	if err != nil {
		return "", nil, err
	}

	localPath, err = m.EnsureRepoCloned(ref)
	if err != nil {
		return "", nil, err
	}

	return localPath, ref, nil
}

// DefaultRepoPathManager is the default instance used for GitHub URL resolution.
var DefaultRepoPathManager = NewRepoPathManager()

// ResolveGitHubInput is a convenience function using the default manager.
func ResolveGitHubInput(input string) (localPath string, ref *GitHubRef, err error) {
	return DefaultRepoPathManager.ResolveGitHubInput(input)
}

// WorktreeInfo contains information about a git worktree
type WorktreeInfo struct {
	// IsWorktree is true if the path is a git worktree (not the main repo)
	IsWorktree bool
	// MainRepoPath is the path to the main repository's .git directory
	// For a worktree at ~/.stapler-squad/worktrees/foo, this might be /path/to/main/repo/.git
	MainRepoPath string
	// MainRepoRoot is the working directory root of the main repository
	MainRepoRoot string
	// RemoteURL is the git remote origin URL (e.g., https://github.com/owner/repo.git)
	RemoteURL string
	// GitHubOwner is the owner extracted from a GitHub remote URL
	GitHubOwner string
	// GitHubRepo is the repo name extracted from a GitHub remote URL
	GitHubRepo string
}

// DetectWorktree checks if the given path is a git worktree and extracts relevant info.
// Returns WorktreeInfo with IsWorktree=false if it's not a worktree or not a git repo.
func DetectWorktree(path string) (*WorktreeInfo, error) {
	info := &WorktreeInfo{}

	// Check if .git exists
	gitPath := filepath.Join(path, ".git")
	stat, err := os.Stat(gitPath)
	if err != nil {
		// Not a git repository
		return info, nil
	}

	// If .git is a file (not a directory), it's a worktree
	// The file contains: gitdir: /path/to/main/repo/.git/worktrees/name
	if !stat.IsDir() {
		info.IsWorktree = true

		// Read the .git file to get the gitdir path
		content, err := os.ReadFile(gitPath)
		if err != nil {
			return info, fmt.Errorf("failed to read .git file: %w", err)
		}

		// Parse: gitdir: /path/to/main/repo/.git/worktrees/name
		gitdirLine := strings.TrimSpace(string(content))
		if strings.HasPrefix(gitdirLine, "gitdir: ") {
			gitdir := strings.TrimPrefix(gitdirLine, "gitdir: ")
			// Extract the main repo .git path (remove /worktrees/name suffix)
			if idx := strings.Index(gitdir, "/.git/worktrees/"); idx != -1 {
				info.MainRepoPath = gitdir[:idx+5] // Include "/.git"
				info.MainRepoRoot = gitdir[:idx]
			}
		}
	}

	// Get the remote URL using git command (works for both worktrees and main repos)
	cmd := exec.Command("git", "-C", path, "config", "--get", "remote.origin.url")
	output, err := cmd.Output()
	if err == nil {
		info.RemoteURL = strings.TrimSpace(string(output))
		// Try to parse GitHub owner/repo from the URL
		info.GitHubOwner, info.GitHubRepo = parseGitHubRemoteURL(info.RemoteURL)
	}

	return info, nil
}

// parseGitHubRemoteURL extracts owner and repo from various GitHub URL formats.
// Supports:
//   - https://github.com/owner/repo.git
//   - https://github.com/owner/repo
//   - git@github.com:owner/repo.git
//   - git@github.com:owner/repo
func parseGitHubRemoteURL(url string) (owner, repo string) {
	url = strings.TrimSpace(url)

	// HTTPS format: https://github.com/owner/repo.git
	httpsPattern := regexp.MustCompile(`^https?://github\.com/([^/]+)/([^/]+?)(?:\.git)?$`)
	if match := httpsPattern.FindStringSubmatch(url); match != nil {
		return match[1], match[2]
	}

	// SSH format: git@github.com:owner/repo.git
	sshPattern := regexp.MustCompile(`^git@github\.com:([^/]+)/([^/]+?)(?:\.git)?$`)
	if match := sshPattern.FindStringSubmatch(url); match != nil {
		return match[1], match[2]
	}

	return "", ""
}

// GetMainRepoPath uses git rev-parse --git-common-dir to get the main repo path.
// This is more reliable than parsing the .git file.
func GetMainRepoPath(path string) (string, error) {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--git-common-dir")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get git common dir: %w", err)
	}

	gitCommonDir := strings.TrimSpace(string(output))

	// If it's an absolute path, use it directly
	if filepath.IsAbs(gitCommonDir) {
		// Remove the .git suffix to get the repo root
		if strings.HasSuffix(gitCommonDir, "/.git") || strings.HasSuffix(gitCommonDir, "\\.git") {
			return filepath.Dir(gitCommonDir), nil
		}
		return gitCommonDir, nil
	}

	// If relative, resolve it from the worktree path
	absPath := filepath.Join(path, gitCommonDir)
	absPath, err = filepath.Abs(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	// Remove the .git suffix
	if strings.HasSuffix(absPath, "/.git") || strings.HasSuffix(absPath, "\\.git") {
		return filepath.Dir(absPath), nil
	}
	return absPath, nil
}
