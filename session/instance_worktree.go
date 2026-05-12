package session

// instance_worktree.go contains git/worktree-related methods for Instance.
// Workspace switching is in instance_workspace.go (already extracted).
// This file covers git worktree lifecycle, diff stats, and path resolution.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/git"
)

// RepoName returns the name of the git repository.
// Returns an error if the instance has not been started or has no worktree.
func (i *Instance) RepoName() (string, error) {
	if !i.started {
		return "", fmt.Errorf("cannot get repo name for instance that has not been started")
	}
	if i.Status == Paused {
		return "", fmt.Errorf("cannot get repo name for paused instance")
	}
	if !i.gitManager.HasWorktree() {
		return "", fmt.Errorf("gitWorktree is nil")
	}
	return i.gitManager.GetRepoName(), nil
}

// setupFirstTimeWorktree creates or attaches to the git worktree based on session type.
func (i *Instance) setupFirstTimeWorktree() error {
	switch i.SessionType {
	case SessionTypeNewWorktree:
		log.Info("creating git worktree for instance", "session", i.Title, "path", i.Path)
		gitWorktree, branchName, err := git.NewGitWorktreeWithBranch(i.Path, i.Title, i.Branch)
		if err != nil {
			return fmt.Errorf("failed to create git worktree: %w", err)
		}
		i.gitManager.SetWorktree(gitWorktree)
		if i.Branch == "" {
			i.Branch = branchName
		}
		log.Info("git worktree created", "session", i.Title, "branch", i.Branch)
	case SessionTypeExistingWorktree:
		if i.ExistingWorktree == "" {
			return fmt.Errorf("existing worktree path required for SessionTypeExistingWorktree")
		}
		log.Info("connecting to existing worktree", "session", i.Title, "path", i.ExistingWorktree)
		gitWorktree, err := git.NewGitWorktreeFromExisting(i.ExistingWorktree, i.Title)
		if err != nil {
			return fmt.Errorf("failed to connect to existing worktree: %w", err)
		}
		i.gitManager.SetWorktree(gitWorktree)
		i.Branch = gitWorktree.GetBranchName()
		log.Info("connected to existing worktree", "session", i.Title, "branch", i.Branch)
	case SessionTypeNewProject:
		log.Info("new project session, initializing git repo", "session", i.Title, "path", i.Path)
		if err := git.InitializeProjectDirectory(i.Path); err != nil {
			return fmt.Errorf("new_project initialization failed: %w", err)
		}
		i.gitManager.SetWorktree(nil)
		i.Branch = ""
		log.Info("new project initialized", "path", i.Path)
	default: // SessionTypeDirectory and unknown types → no worktree
		log.Info("directory session, no git worktree", "session", i.Title, "path", i.Path)
		if i.CreateIfMissing {
			if _, err := os.Stat(i.Path); os.IsNotExist(err) {
				if err := git.InitializeProjectDirectory(i.Path); err != nil {
					return fmt.Errorf("failed to create directory for session: %w", err)
				}
			}
		}
		i.gitManager.SetWorktree(nil)
		i.Branch = ""
	}
	return nil
}

// resolveStartPath returns the effective start directory, applying WorkingDir on top of basePath.
// Falls back to basePath if the resolved directory does not exist.
// For worktree sessions, absolute WorkingDir paths outside the worktree are ignored to prevent
// stale CWD snapshots (from CaptureCurrentState) from overriding the worktree root.
func (i *Instance) resolveStartPath(basePath string) string {
	if i.WorkingDir == "" {
		return basePath
	}
	startPath := i.WorkingDir
	if !filepath.IsAbs(i.WorkingDir) {
		startPath = filepath.Join(basePath, i.WorkingDir)
	} else if i.gitManager.HasWorktree() {
		// For worktree sessions, an absolute WorkingDir must be within the worktree.
		// CaptureCurrentState() can persist the process CWD (e.g. the main repo path
		// when Claude cd's there), which would otherwise bypass worktree isolation.
		rel, err := filepath.Rel(basePath, startPath)
		if err != nil || strings.HasPrefix(rel, "..") {
			log.Warn("working dir is outside worktree, using worktree path", "path", startPath, "worktree", basePath)
			return basePath
		}
	}
	if _, err := os.Stat(startPath); os.IsNotExist(err) {
		log.Warn("working directory doesn't exist, using fallback", "path", startPath, "fallback", basePath)
		return basePath
	}
	return startPath
}

// GetEffectiveRootDir returns the root directory where this session operates.
// For worktree sessions, this is the worktree path. For directory sessions, this is Path.
// Used for injecting configuration files (e.g., .claude/settings.local.json).
func (i *Instance) GetEffectiveRootDir() string {
	if i.gitManager.HasWorktree() {
		if p := i.gitManager.GetWorktreePath(); p != "" {
			return p
		}
	}
	return i.Path
}

// Workspace returns where this session is operating.
// Use this as the single source of truth for path resolution instead of
// accessing inst.Path directly, which is wrong for worktree sessions.
func (i *Instance) Workspace() Workspace {
	return Workspace{
		EffectivePath: i.GetEffectiveRootDir(),
		RepoRoot:      i.Path,
	}
}

// CleanupWorktree removes the git worktree, keeping session intact.
func (i *Instance) CleanupWorktree() error {
	if i.gitManager.HasWorktree() {
		if err := i.gitManager.Cleanup(); err != nil {
			return fmt.Errorf("failed to cleanup git worktree: %w", err)
		}
	}
	return nil
}

// GetGitWorktree returns the git worktree for the instance.
func (i *Instance) GetGitWorktree() (*git.GitWorktree, error) {
	if !i.started {
		return nil, fmt.Errorf("cannot get git worktree for instance that has not been started")
	}
	return i.gitManager.GetWorktree(), nil
}

// HasGitWorktree returns true if the instance has a git worktree.
func (i *Instance) HasGitWorktree() bool {
	return i.gitManager.HasWorktree()
}

// SetGitWorktree sets the git worktree for testing purposes.
func (i *Instance) SetGitWorktree(worktree *git.GitWorktree) {
	i.gitManager.SetWorktree(worktree)
	i.started = worktree != nil
}

// UpdateDiffStats updates the git diff statistics for this instance.
// Performs I/O (git diff) outside the lock, then updates state under the write lock.
func (i *Instance) UpdateDiffStats() error {
	// Read lock for initial state checks
	i.stateMutex.RLock()
	if !i.started {
		i.gitManager.ClearDiffStats()
		i.stateMutex.RUnlock()
		return nil
	}
	if i.Status == Paused {
		i.stateMutex.RUnlock()
		return nil
	}
	if !i.gitManager.HasWorktree() {
		i.gitManager.ClearDiffStats()
		i.stateMutex.RUnlock()
		return nil
	}
	i.stateMutex.RUnlock()

	// I/O outside lock: check worktree existence and compute diff
	stats, needsPause := i.gitManager.ComputeDiffIfReady()

	// Write lock to update state — keep non-logging work only to minimise hold time.
	i.stateMutex.Lock()
	var transitionErr error
	var didTransitionToPaused bool
	if needsPause {
		if i.Status != Paused {
			didTransitionToPaused = true
			transitionErr = i.transitionTo(Paused)
		}
		i.gitManager.ClearDiffStats()
		i.stateMutex.Unlock()
		if didTransitionToPaused {
			log.Warn("worktree directory doesn't exist, marking as paused", "session", i.Title)
		}
		if transitionErr != nil {
			log.Warn("failed to transition to paused", "session", i.Title, "err", transitionErr)
		}
		return nil
	}
	if stats != nil && stats.Error != nil {
		if strings.Contains(stats.Error.Error(), "base commit SHA not set") {
			i.gitManager.ClearDiffStats()
			i.stateMutex.Unlock()
			return nil
		}
		i.stateMutex.Unlock()
		return fmt.Errorf("failed to get diff stats: %w", stats.Error)
	}
	i.gitManager.SetDiffStats(stats)
	i.stateMutex.Unlock()
	return nil
}

// GetDiffStats returns the current git diff statistics.
func (i *Instance) GetDiffStats() *git.DiffStats {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()
	return i.gitManager.GetDiffStats()
}

// GetWorkingDirectory returns the working directory for this instance.
func (i *Instance) GetWorkingDirectory() string {
	if i.gitManager.HasWorktree() {
		return i.gitManager.GetWorktreePath()
	}
	return i.Path
}

// DetectAndPopulateWorktreeInfo detects if the instance path is a worktree
// and populates the IsWorktree, MainRepoPath, GitHubOwner, and GitHubRepo fields.
// NOTE: This method writes to GitHub fields (i.GitHubOwner, i.GitHubRepo) directly.
// A future pass could route writes through a setter method for encapsulation.
// This is useful for sessions created from existing worktrees where we want to
// display the actual repository information in the UI.
//
// IMPORTANT: For sessions with git worktrees, we check BOTH paths:
// 1. The worktree path (gitWorktree.GetWorktreePath()) - to detect IsWorktree and MainRepoPath
// 2. The original path (i.Path) - as fallback for GitHub owner/repo if worktree detection fails
//
// This is necessary because:
// - i.Path is the main repository path (e.g., ~/Documents/personal-wiki)
// - gitWorktree.GetWorktreePath() is the actual worktree (e.g., ~/.stapler-squad/worktrees/...)
// - The main repo has .git as a directory; the worktree has .git as a file pointing to the main repo
func (i *Instance) DetectAndPopulateWorktreeInfo() error {
	// Determine the path to use for detection
	// For worktree sessions, use the worktree path; otherwise use i.Path
	detectPath := i.Path
	if i.gitManager.HasWorktree() {
		worktreePath := i.gitManager.GetWorktreePath()
		if worktreePath != "" {
			detectPath = worktreePath
		}
	}

	if detectPath == "" {
		return nil
	}

	info, err := DetectWorktree(detectPath)
	if err != nil {
		return err
	}

	i.IsWorktree = info.IsWorktree
	if info.IsWorktree && info.MainRepoRoot != "" {
		i.MainRepoPath = info.MainRepoRoot
	}

	// Only populate GitHub info if not already set
	if i.GitHubOwner == "" && info.GitHubOwner != "" {
		i.GitHubOwner = info.GitHubOwner
	}
	if i.GitHubRepo == "" && info.GitHubRepo != "" {
		i.GitHubRepo = info.GitHubRepo
	}

	return nil
}
