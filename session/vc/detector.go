package vc

import (
	"os"
	"os/exec"
	"path/filepath"
)

// DetectVCS detects the type of version control system in the given directory.
// It checks for Jujutsu first since jj repositories can coexist with .git directories.
// The detection walks up the directory tree to find the VCS root.
func DetectVCS(path string) VCSType {
	// Check for Jujutsu first (it can coexist with .git)
	if isJujutsuRepo(path) {
		return VCSJujutsu
	}

	// Check for Git
	if isGitRepo(path) {
		return VCSGit
	}

	return VCSUnknown
}

// NewProvider creates a new VCSProvider for the given directory.
// It auto-detects the VCS type and returns the appropriate provider.
func NewProvider(path string) (VCSProvider, error) {
	vcsType := DetectVCS(path)
	switch vcsType {
	case VCSGit:
		return NewGitProvider(path)
	case VCSJujutsu:
		return NewJujutsuProvider(path)
	default:
		return nil, ErrNoVCSFound
	}
}

// isJujutsuRepo checks if the path is within a Jujutsu repository
// by looking for a .jj directory
func isJujutsuRepo(path string) bool {
	currentPath := path
	for {
		jjDir := filepath.Join(currentPath, ".jj")
		if info, err := os.Stat(jjDir); err == nil && info.IsDir() {
			return true
		}

		parent := filepath.Dir(currentPath)
		if parent == currentPath {
			// Reached filesystem root
			return false
		}
		currentPath = parent
	}
}

// isGitRepo checks if the path is within a Git repository
// by looking for a .git directory or file (for worktrees)
func isGitRepo(path string) bool {
	currentPath := path
	for {
		gitPath := filepath.Join(currentPath, ".git")
		if info, err := os.Stat(gitPath); err == nil {
			// .git can be a directory (normal repo) or file (worktree)
			if info.IsDir() || info.Mode().IsRegular() {
				return true
			}
		}

		parent := filepath.Dir(currentPath)
		if parent == currentPath {
			// Reached filesystem root
			return false
		}
		currentPath = parent
	}
}

// FindVCSRoot finds the root directory of the VCS repository
func FindVCSRoot(path string, vcsType VCSType) (string, error) {
	currentPath := path
	for {
		var markerPath string
		switch vcsType {
		case VCSJujutsu:
			markerPath = filepath.Join(currentPath, ".jj")
		case VCSGit:
			markerPath = filepath.Join(currentPath, ".git")
		default:
			return "", ErrNoVCSFound
		}

		if _, err := os.Stat(markerPath); err == nil {
			return currentPath, nil
		}

		parent := filepath.Dir(currentPath)
		if parent == currentPath {
			return "", ErrNoVCSFound
		}
		currentPath = parent
	}
}

// IsToolAvailable checks if a command-line tool is available in PATH
func IsToolAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
