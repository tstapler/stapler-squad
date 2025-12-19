package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"claude-squad/log"
	"claude-squad/session/git"
	"claude-squad/session/vcs"
)

// WorkspaceSwitchType defines the type of workspace switch operation
type WorkspaceSwitchType int

const (
	// SwitchTypeDirectory is a simple directory change (no VCS, no restart)
	SwitchTypeDirectory WorkspaceSwitchType = iota
	// SwitchTypeRevision switches to a different revision/branch
	SwitchTypeRevision
	// SwitchTypeWorktree switches to or creates a different worktree
	SwitchTypeWorktree
)

func (t WorkspaceSwitchType) String() string {
	switch t {
	case SwitchTypeDirectory:
		return "directory"
	case SwitchTypeRevision:
		return "revision"
	case SwitchTypeWorktree:
		return "worktree"
	default:
		return "unknown"
	}
}

// WorkspaceSwitchRequest represents a request to switch the workspace
type WorkspaceSwitchRequest struct {
	// Type is the type of switch operation
	Type WorkspaceSwitchType
	// Target is the destination (directory path, revision/branch, or worktree path)
	Target string
	// ChangeStrategy determines how to handle uncommitted changes
	ChangeStrategy vcs.ChangeStrategy
	// CreateIfMissing creates the bookmark/branch/worktree if it doesn't exist
	CreateIfMissing bool
	// BaseRevision is the base for new bookmark creation (empty = current)
	BaseRevision string
	// VCSPreference overrides the default VCS preference for this operation
	VCSPreference vcs.VCSPreference
}

// WorkspaceSwitchResult contains the result of a workspace switch operation
type WorkspaceSwitchResult struct {
	// Success indicates if the switch was successful
	Success bool
	// Error contains any error that occurred
	Error error
	// PreviousRevision is the revision before the switch
	PreviousRevision string
	// CurrentRevision is the revision after the switch
	CurrentRevision string
	// VCSType is the VCS that was used
	VCSType vcs.VCSType
	// ChangesHandled describes how uncommitted changes were handled
	ChangesHandled string
}

// SwitchWorkspace switches the session's workspace according to the request.
// For directory changes, this is a simple cd operation.
// For revision/worktree switches, this restarts Claude with --resume to preserve conversation.
func (i *Instance) SwitchWorkspace(req WorkspaceSwitchRequest) (*WorkspaceSwitchResult, error) {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()

	result := &WorkspaceSwitchResult{}

	// Validate session state
	if !i.started {
		return nil, fmt.Errorf("cannot switch workspace for session that has not been started")
	}
	if i.Status == Paused {
		return nil, fmt.Errorf("cannot switch workspace for paused session - resume first")
	}

	log.InfoLog.Printf("[Workspace] Starting %s switch for session '%s' to '%s'",
		req.Type, i.Title, req.Target)

	// Handle simple directory change separately (no VCS, no restart)
	if req.Type == SwitchTypeDirectory {
		if err := i.changeDirectory(req.Target); err != nil {
			result.Error = err
			return result, err
		}
		result.Success = true
		result.ChangesHandled = "none (directory change only)"
		return result, nil
	}

	// For revision/worktree switches, we need to restart Claude

	// 1. Determine repository path
	repoPath := i.getRepoPath()
	if repoPath == "" {
		return nil, fmt.Errorf("cannot determine repository path for session")
	}

	// 2. Get VCS client
	detectOpts := vcs.DefaultDetectOptions()
	if req.VCSPreference != "" {
		detectOpts.Preference = req.VCSPreference
	}

	vcsClient, err := vcs.DetectWithOptions(repoPath, detectOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to detect VCS: %w", err)
	}
	result.VCSType = vcsClient.Type()

	// 3. Get current revision for result tracking
	if currentRev, err := vcsClient.GetCurrentRevision(); err == nil {
		result.PreviousRevision = currentRev.ShortID
	}

	// 4. Claude session ID is preserved in i.claudeSession
	// ClaudeCommandBuilder will use it on restart

	// 5. Kill tmux session (but keep claudeSession data for resume)
	log.InfoLog.Printf("[Workspace] Stopping tmux session for workspace switch")
	if err := i.KillSession(); err != nil {
		return nil, fmt.Errorf("failed to stop session: %w", err)
	}
	i.started = false

	// 6. Perform VCS operation
	var switchErr error
	switch req.Type {
	case SwitchTypeRevision:
		switchErr = i.switchRevision(vcsClient, req, result)
	case SwitchTypeWorktree:
		switchErr = i.switchWorktree(vcsClient, req, result)
	}

	if switchErr != nil {
		// Try to recover by restarting at original location
		log.WarningLog.Printf("[Workspace] Switch failed, attempting recovery: %v", switchErr)
		if err := i.Start(false); err != nil {
			log.ErrorLog.Printf("[Workspace] Recovery failed: %v", err)
		}
		result.Error = switchErr
		return result, switchErr
	}

	// 7. Restart Claude (ClaudeCommandBuilder adds --resume automatically)
	log.InfoLog.Printf("[Workspace] Restarting session with Claude --resume")
	if err := i.Start(false); err != nil {
		result.Error = fmt.Errorf("failed to restart session: %w", err)
		return result, result.Error
	}

	// 8. Get new revision for result
	if newRev, err := vcsClient.GetCurrentRevision(); err == nil {
		result.CurrentRevision = newRev.ShortID
	}

	result.Success = true
	log.InfoLog.Printf("[Workspace] Successfully switched session '%s' to '%s'",
		i.Title, req.Target)

	return result, nil
}

// switchRevision handles switching to a different revision/branch
func (i *Instance) switchRevision(vcsClient vcs.VCS, req WorkspaceSwitchRequest, result *WorkspaceSwitchResult) error {
	opts := vcs.SwitchOptions{
		ChangeStrategy:  req.ChangeStrategy,
		CreateIfMissing: req.CreateIfMissing,
		BaseRevision:    req.BaseRevision,
	}

	// Record how changes will be handled
	hasChanges, _ := vcsClient.HasUncommittedChanges()
	if hasChanges {
		result.ChangesHandled = req.ChangeStrategy.String()
	} else {
		result.ChangesHandled = "no changes to handle"
	}

	// Perform the switch
	if err := vcsClient.SwitchTo(req.Target, opts); err != nil {
		return fmt.Errorf("failed to switch to %s: %w", req.Target, err)
	}

	// Update instance branch
	i.Branch = req.Target

	return nil
}

// switchWorktree handles switching to or creating a worktree
func (i *Instance) switchWorktree(vcsClient vcs.VCS, req WorkspaceSwitchRequest, result *WorkspaceSwitchResult) error {
	// Check if target worktree exists
	worktrees, err := vcsClient.ListWorktrees()
	if err != nil {
		log.WarningLog.Printf("[Workspace] Failed to list worktrees: %v", err)
	}

	var targetWorktree *vcs.Worktree
	for _, wt := range worktrees {
		if wt.Path == req.Target || wt.Name == req.Target {
			targetWorktree = &wt
			break
		}
	}

	if targetWorktree == nil && req.CreateIfMissing {
		// Create new worktree
		// Use target as both path and name
		name := filepath.Base(req.Target)
		if err := vcsClient.CreateWorktree(req.Target, name); err != nil {
			return fmt.Errorf("failed to create worktree: %w", err)
		}

		// Refresh worktree info
		worktrees, _ = vcsClient.ListWorktrees()
		for _, wt := range worktrees {
			if wt.Path == req.Target {
				targetWorktree = &wt
				break
			}
		}
	}

	if targetWorktree == nil {
		return fmt.Errorf("worktree not found: %s", req.Target)
	}

	// Update instance to point to new worktree
	i.Path = targetWorktree.Path
	if targetWorktree.Bookmark != "" {
		i.Branch = targetWorktree.Bookmark
	}

	// Try to create GitWorktree wrapper for compatibility
	if gitWt, err := git.NewGitWorktreeFromExisting(targetWorktree.Path, i.Title); err == nil {
		i.gitWorktree = gitWt
	}

	result.ChangesHandled = "worktree switch (changes remain in original worktree)"
	return nil
}

// changeDirectory handles simple directory navigation without VCS
func (i *Instance) changeDirectory(newDir string) error {
	// Resolve path
	var absPath string
	var err error

	if filepath.IsAbs(newDir) {
		absPath = newDir
	} else {
		// Relative to current working directory
		basePath := i.getRepoPath()
		if basePath == "" {
			basePath = i.Path
		}
		absPath, err = filepath.Abs(filepath.Join(basePath, newDir))
		if err != nil {
			return fmt.Errorf("invalid path: %w", err)
		}
	}

	// Verify path exists
	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("directory does not exist: %s", absPath)
		}
		return fmt.Errorf("cannot access directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", absPath)
	}

	// Security: validate path is within allowed boundaries
	if err := i.validatePathSecurity(absPath); err != nil {
		return err
	}

	// Send cd command to tmux
	cdCmd := fmt.Sprintf("cd %q\n", absPath)
	if _, err := i.tmuxSession.SendKeys(cdCmd); err != nil {
		return fmt.Errorf("failed to change directory: %w", err)
	}

	// Update instance state
	i.WorkingDir = absPath
	i.UpdatedAt = timeNow()

	log.InfoLog.Printf("[Workspace] Changed directory to %s", absPath)
	return nil
}

// validatePathSecurity ensures the target path is within allowed boundaries
func (i *Instance) validatePathSecurity(targetPath string) error {
	// Clean and resolve the path
	cleanPath := filepath.Clean(targetPath)
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	// Get the repository root for boundary checking
	repoRoot := i.getRepoPath()
	if repoRoot == "" {
		// For directory sessions without a repo, use the session path
		repoRoot = i.Path
	}

	absRepo, err := filepath.Abs(repoRoot)
	if err != nil {
		return fmt.Errorf("failed to resolve repo path: %w", err)
	}

	// Check if target is within the repository (or session directory)
	if !strings.HasPrefix(absPath, absRepo) {
		// Allow going to parent directories up to a reasonable limit
		// This is a soft check - users should be able to navigate freely
		// but we log a warning for paths outside the repo
		log.WarningLog.Printf("[Workspace] Path %s is outside repository root %s", absPath, absRepo)
	}

	// Hard block on obvious path traversal attempts
	if strings.Contains(targetPath, "..") && !filepath.IsAbs(targetPath) {
		// Verify the resolved path doesn't escape too far
		homeDir, _ := os.UserHomeDir()
		if homeDir != "" && !strings.HasPrefix(absPath, homeDir) {
			return fmt.Errorf("path traversal outside home directory not allowed: %s", targetPath)
		}
	}

	return nil
}

// getRepoPath returns the repository root path for the session
func (i *Instance) getRepoPath() string {
	if i.gitWorktree != nil {
		return i.gitWorktree.GetWorktreePath()
	}
	if i.MainRepoPath != "" {
		return i.MainRepoPath
	}
	return i.Path
}

// GetVCSInfo returns information about the VCS for this session
func (i *Instance) GetVCSInfo() (*VCSInfo, error) {
	repoPath := i.getRepoPath()
	if repoPath == "" {
		return nil, fmt.Errorf("no repository path available")
	}

	hasJJ, hasGit, isColocated := vcs.GetVCSInfo(repoPath)

	info := &VCSInfo{
		HasJJ:       hasJJ,
		HasGit:      hasGit,
		IsColocated: isColocated,
		RepoPath:    repoPath,
	}

	// Get current branch/bookmark
	if vcsClient, err := vcs.Detect(repoPath); err == nil {
		info.VCSType = string(vcsClient.Type())
		if bookmark, err := vcsClient.GetCurrentBookmark(); err == nil {
			info.CurrentBookmark = bookmark
		}
		if rev, err := vcsClient.GetCurrentRevision(); err == nil {
			info.CurrentRevision = rev.ShortID
		}
		if status, err := vcsClient.GetStatus(); err == nil {
			info.HasUncommittedChanges = status.HasChanges
			info.ModifiedFileCount = status.ModifiedFiles + status.AddedFiles + status.DeletedFiles
		}
	}

	return info, nil
}

// VCSInfo contains version control information for a session
type VCSInfo struct {
	// VCSType is "jj" or "git"
	VCSType string
	// HasJJ indicates if JJ is available
	HasJJ bool
	// HasGit indicates if Git is available
	HasGit bool
	// IsColocated indicates if this is a JJ+Git colocated repo
	IsColocated bool
	// RepoPath is the repository root path
	RepoPath string
	// CurrentBookmark is the current branch/bookmark name
	CurrentBookmark string
	// CurrentRevision is the current revision (short ID)
	CurrentRevision string
	// HasUncommittedChanges indicates if there are uncommitted changes
	HasUncommittedChanges bool
	// ModifiedFileCount is the count of modified/added/deleted files
	ModifiedFileCount int
}

// ListAvailableTargets returns available switch targets (branches, bookmarks, worktrees)
func (i *Instance) ListAvailableTargets() (*AvailableTargets, error) {
	repoPath := i.getRepoPath()
	if repoPath == "" {
		return nil, fmt.Errorf("no repository path available")
	}

	vcsClient, err := vcs.Detect(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to detect VCS: %w", err)
	}

	targets := &AvailableTargets{
		VCSType: string(vcsClient.Type()),
	}

	// Get bookmarks/branches
	if bookmarks, err := vcsClient.ListBookmarks(); err == nil {
		for _, b := range bookmarks {
			if !b.IsRemote { // Skip remote branches
				targets.Bookmarks = append(targets.Bookmarks, BookmarkTarget{
					Name:       b.Name,
					RevisionID: b.RevisionID,
					IsRemote:   b.IsRemote,
				})
			}
		}
	}

	// Get recent revisions
	if revisions, err := vcsClient.ListRecentRevisions(10); err == nil {
		for _, r := range revisions {
			targets.RecentRevisions = append(targets.RecentRevisions, RevisionTarget{
				ID:          r.ID,
				ShortID:     r.ShortID,
				Description: r.Description,
				Author:      r.Author,
				Timestamp:   r.Timestamp,
				IsCurrent:   r.IsCurrent,
			})
		}
	}

	// Get worktrees
	if worktrees, err := vcsClient.ListWorktrees(); err == nil {
		for _, wt := range worktrees {
			targets.Worktrees = append(targets.Worktrees, WorktreeTarget{
				Name:       wt.Name,
				Path:       wt.Path,
				Bookmark:   wt.Bookmark,
				RevisionID: wt.RevisionID,
				IsCurrent:  wt.IsCurrent,
			})
		}
	}

	return targets, nil
}

// AvailableTargets contains the available workspace switch targets
type AvailableTargets struct {
	VCSType         string
	Bookmarks       []BookmarkTarget
	RecentRevisions []RevisionTarget
	Worktrees       []WorktreeTarget
}

// BookmarkTarget represents a bookmark/branch as a switch target
type BookmarkTarget struct {
	Name       string
	RevisionID string
	IsRemote   bool
}

// RevisionTarget represents a revision as a switch target
type RevisionTarget struct {
	ID          string
	ShortID     string
	Description string
	Author      string
	Timestamp   time.Time
	IsCurrent   bool
}

// WorktreeTarget represents a worktree as a switch target
type WorktreeTarget struct {
	Name       string
	Path       string
	Bookmark   string
	RevisionID string
	IsCurrent  bool
}

// Helper to get current time (can be mocked in tests)
var timeNow = func() time.Time {
	return time.Now()
}
