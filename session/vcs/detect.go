package vcs

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/tstapler/stapler-squad/log"
)

// VCSPreference represents the user's preferred VCS
type VCSPreference string

const (
	// PreferenceAuto automatically detects VCS (JJ preferred if available)
	PreferenceAuto VCSPreference = "auto"
	// PreferenceJJ forces JJ usage
	PreferenceJJ VCSPreference = "jj"
	// PreferenceGit forces Git usage
	PreferenceGit VCSPreference = "git"
)

// DetectOptions configures VCS detection behavior
type DetectOptions struct {
	// Preference is the user's VCS preference
	Preference VCSPreference
}

// DefaultDetectOptions returns the default detection options
func DefaultDetectOptions() DetectOptions {
	return DetectOptions{
		Preference: PreferenceAuto,
	}
}

// Detect detects and returns the appropriate VCS client for the given repository path.
// It respects user preferences and falls back appropriately.
func Detect(repoPath string) (VCS, error) {
	return DetectWithOptions(repoPath, DefaultDetectOptions())
}

// DetectWithOptions detects VCS with custom options
func DetectWithOptions(repoPath string, opts DetectOptions) (VCS, error) {
	// Resolve to absolute path
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path: %w", err)
	}

	// Verify path exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("path does not exist: %s", absPath)
	}

	switch opts.Preference {
	case PreferenceJJ:
		if jjAvailable(absPath) {
			log.InfoLog.Printf("[VCS] Using JJ (user preference) for %s", absPath)
			return NewJJClient(absPath), nil
		}
		return nil, fmt.Errorf("JJ preferred but not available at %s", absPath)

	case PreferenceGit:
		if gitAvailable(absPath) {
			log.InfoLog.Printf("[VCS] Using Git (user preference) for %s", absPath)
			return NewGitClient(absPath), nil
		}
		return nil, fmt.Errorf("Git preferred but not available at %s", absPath)

	default: // PreferenceAuto
		// Auto-detect: prefer JJ if available (colocated repos work with both)
		if jjAvailable(absPath) {
			log.InfoLog.Printf("[VCS] Auto-detected JJ for %s", absPath)
			return NewJJClient(absPath), nil
		}
		if gitAvailable(absPath) {
			log.InfoLog.Printf("[VCS] Auto-detected Git for %s", absPath)
			return NewGitClient(absPath), nil
		}
		return nil, fmt.Errorf("no VCS detected at %s", absPath)
	}
}

// jjAvailable checks if JJ is available for the given path
func jjAvailable(repoPath string) bool {
	// Check if jj command exists in PATH
	if _, err := exec.LookPath("jj"); err != nil {
		return false
	}

	// Check if the repo is JJ-managed (has .jj directory)
	jjDir := filepath.Join(repoPath, ".jj")
	if _, err := os.Stat(jjDir); err == nil {
		return true
	}

	// Walk up to find .jj directory (might be in parent for worktrees)
	current := repoPath
	for {
		jjDir := filepath.Join(current, ".jj")
		if _, err := os.Stat(jjDir); err == nil {
			return true
		}

		parent := filepath.Dir(current)
		if parent == current {
			// Reached root
			break
		}
		current = parent
	}

	return false
}

// gitAvailable checks if Git is available for the given path
func gitAvailable(repoPath string) bool {
	// Check if git command exists in PATH
	if _, err := exec.LookPath("git"); err != nil {
		return false
	}

	// Check if the path is inside a git repository
	cmd := exec.Command("git", "-C", repoPath, "rev-parse", "--git-dir")
	if err := cmd.Run(); err == nil {
		return true
	}

	return false
}

// IsColocated checks if the repository is a JJ+Git colocated repository
func IsColocated(repoPath string) bool {
	jjDir := filepath.Join(repoPath, ".jj")
	gitDir := filepath.Join(repoPath, ".git")

	jjExists := false
	gitExists := false

	if _, err := os.Stat(jjDir); err == nil {
		jjExists = true
	}
	if _, err := os.Stat(gitDir); err == nil {
		gitExists = true
	}

	return jjExists && gitExists
}

// GetVCSInfo returns information about what VCS is available at the path
func GetVCSInfo(repoPath string) (hasJJ bool, hasGit bool, isColocated bool) {
	hasJJ = jjAvailable(repoPath)
	hasGit = gitAvailable(repoPath)
	isColocated = IsColocated(repoPath)
	return
}
