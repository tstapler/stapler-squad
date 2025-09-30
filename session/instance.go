package session

import (
	"claude-squad/log"
	"claude-squad/session/git"
	"claude-squad/session/tmux"
	"path/filepath"

	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/atotto/clipboard"
)

type Status int

const (
	// Running is the status when the instance is running and claude is working.
	Running Status = iota
	// Ready is if the claude instance is ready to be interacted with (waiting for user input).
	Ready
	// Loading is if the instance is loading (if we are starting it up or something).
	Loading
	// Paused is if the instance is paused (worktree removed but branch preserved).
	Paused
	// NeedsApproval is if the instance is waiting for user approval on a prompt.
	NeedsApproval
)

// Instance is a running instance of claude code.
type Instance struct {
	// Title is the title of the instance.
	Title string
	// Path is the path to the workspace repository root.
	Path string
	// WorkingDir is the directory within the repository to start in.
	WorkingDir string
	// Branch is the branch of the instance.
	Branch string
	// Status is the status of the instance.
	Status Status
	// Program is the program to run in the instance.
	Program string
	// Height is the height of the instance.
	Height int
	// Width is the width of the instance.
	Width int
	// CreatedAt is the time the instance was created.
	CreatedAt time.Time
	// UpdatedAt is the time the instance was last updated.
	UpdatedAt time.Time
	// AutoYes is true if the instance should automatically press enter when prompted.
	AutoYes bool
	// Prompt is the initial prompt to pass to the instance on startup
	Prompt string
	// ExistingWorktree is an optional path to an existing worktree to reuse
	ExistingWorktree string
	// Category is used for organizing sessions into groups
	Category string
	// IsExpanded indicates whether this session's category is expanded in the UI
	IsExpanded bool
	// SessionType determines the session workflow (directory, new_worktree, existing_worktree)
	SessionType SessionType
	// TmuxPrefix is the prefix to use for tmux session names
	TmuxPrefix string
	// TmuxServerSocket is the server socket name for tmux isolation (used with -L flag)
	// If empty, uses the default tmux server. For complete isolation (e.g., testing),
	// set to a unique value like "test" or "teatest_123" to create separate tmux servers.
	TmuxServerSocket string

	// DiffStats stores the current git diff statistics
	diffStats *git.DiffStats

	// Claude Code session information for persistence and re-attachment
	claudeSession *ClaudeSessionData

	// The below fields are initialized upon calling Start().

	started bool
	// tmuxSession is the tmux session for the instance.
	tmuxSession *tmux.TmuxSession
	// gitWorktree is the git worktree for the instance.
	gitWorktree *git.GitWorktree

	// Mutex to protect concurrent access to instance state
	stateMutex sync.RWMutex

	// Preview size tracking to avoid unnecessary resize operations
	lastPreviewWidth  int
	lastPreviewHeight int
	lastPTYWarningTime time.Time
}

// ToInstanceData converts an Instance to its serializable form
func (i *Instance) ToInstanceData() InstanceData {
	data := InstanceData{
		Title:       i.Title,
		Path:        i.Path,
		WorkingDir:  i.WorkingDir,
		Branch:      i.Branch,
		Status:      i.Status,
		Height:      i.Height,
		Width:       i.Width,
		CreatedAt:   i.CreatedAt,
		UpdatedAt:   time.Now(),
		Program:     i.Program,
		AutoYes:     i.AutoYes,
		Prompt:      i.Prompt,
		Category:    i.Category,
		IsExpanded:  i.IsExpanded,
		SessionType: i.SessionType,
		TmuxPrefix:  i.TmuxPrefix,
	}

	// Only include worktree data if gitWorktree is initialized
	if i.gitWorktree != nil {
		data.Worktree = GitWorktreeData{
			RepoPath:      i.gitWorktree.GetRepoPath(),
			WorktreePath:  i.gitWorktree.GetWorktreePath(),
			SessionName:   i.Title,
			BranchName:    i.gitWorktree.GetBranchName(),
			BaseCommitSHA: i.gitWorktree.GetBaseCommitSHA(),
		}
	}

	// Only include diff stats if they exist
	if i.diffStats != nil {
		data.DiffStats = DiffStatsData{
			Added:   i.diffStats.Added,
			Removed: i.diffStats.Removed,
			Content: i.diffStats.Content,
		}
	}

	// Include Claude session data if it exists
	if i.claudeSession != nil {
		data.ClaudeSession = *i.claudeSession
	}

	return data
}

// FromInstanceData creates a new Instance from serialized data
func FromInstanceData(data InstanceData) (*Instance, error) {
	instance := &Instance{
		Title:       data.Title,
		Path:        data.Path,
		WorkingDir:  data.WorkingDir,
		Branch:      data.Branch,
		Status:      data.Status,
		Height:      data.Height,
		Width:       data.Width,
		CreatedAt:   data.CreatedAt,
		UpdatedAt:   data.UpdatedAt,
		Program:     data.Program,
		Prompt:      data.Prompt,
		Category:    data.Category,
		IsExpanded:  data.IsExpanded,
		SessionType: data.SessionType,
		TmuxPrefix:  data.TmuxPrefix,
		gitWorktree: git.NewGitWorktreeFromStorage(
			data.Worktree.RepoPath,
			data.Worktree.WorktreePath,
			data.Worktree.SessionName,
			data.Worktree.BranchName,
			data.Worktree.BaseCommitSHA,
		),
		diffStats: &git.DiffStats{
			Added:   data.DiffStats.Added,
			Removed: data.DiffStats.Removed,
			Content: data.DiffStats.Content,
		},
	}

	// Restore Claude session data if it exists
	if data.ClaudeSession.SessionID != "" || data.ClaudeSession.ConversationID != "" {
		claudeSessionCopy := data.ClaudeSession
		instance.claudeSession = &claudeSessionCopy
	}

	// Initialize session-specific logging
	_ = log.GetSessionLoggers

	// Check if the worktree still exists on disk if the instance is not paused
	if !instance.Paused() && instance.gitWorktree != nil {
		worktreePath := instance.gitWorktree.GetWorktreePath()
		if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
			// Worktree has been deleted, mark instance as paused
			log.LogForSession(instance.Title, "warning", "Worktree directory doesn't exist at '%s', marking as paused", worktreePath)
			instance.Status = Paused
		}
	}

	if instance.Paused() {
		instance.started = true
		// Use configurable prefix or default
		tmuxPrefix := instance.TmuxPrefix
		if tmuxPrefix == "" {
			tmuxPrefix = "claudesquad_" // Default fallback
		}

		// Use server socket isolation if specified, otherwise use prefix-only isolation
		if instance.TmuxServerSocket != "" {
			instance.tmuxSession = tmux.NewTmuxSessionWithServerSocket(instance.Title, instance.Program, tmuxPrefix, instance.TmuxServerSocket)
		} else {
			instance.tmuxSession = tmux.NewTmuxSessionWithPrefix(instance.Title, instance.Program, tmuxPrefix)
		}
	} else {
		if err := instance.Start(false); err != nil {
			return nil, err
		}
	}

	return instance, nil
}

// SessionType indicates the type of session workflow to use
type SessionType string

const (
	// SessionTypeDirectory creates a simple directory session without git worktree
	SessionTypeDirectory SessionType = "directory"
	// SessionTypeNewWorktree creates a new git worktree for the session
	SessionTypeNewWorktree SessionType = "new_worktree"
	// SessionTypeExistingWorktree uses an existing git worktree
	SessionTypeExistingWorktree SessionType = "existing_worktree"
)

// Options for creating a new instance
type InstanceOptions struct {
	// Title is the title of the instance.
	Title string
	// Path is the path to the workspace repository root.
	Path string
	// WorkingDir is the directory within the repository to start in.
	// If empty, defaults to repository root.
	WorkingDir string
	// Program is the program to run in the instance (e.g. "claude", "aider --model ollama_chat/gemma3:1b")
	Program string
	// If AutoYes is true, automatically accept prompts
	AutoYes bool
	// Prompt is the initial prompt to pass to the instance on startup
	Prompt string
	// ExistingWorktree is an optional path to an existing worktree to reuse
	ExistingWorktree string
	// Category is used for organizing sessions into groups
	Category string
	// SessionType determines the session workflow (directory, new_worktree, existing_worktree)
	SessionType SessionType
	// TmuxPrefix is the prefix to use for tmux session names (e.g., "claudesquad_")
	TmuxPrefix string
	// TmuxServerSocket is the server socket name for tmux isolation (used with -L flag)
	// If empty, uses the default tmux server. For complete isolation (e.g., testing),
	// set to a unique value like "test" or "teatest_123" to create separate tmux servers.
	TmuxServerSocket string
}

func NewInstance(opts InstanceOptions) (*Instance, error) {
	t := time.Now()

	// Convert path to absolute
	absPath, err := filepath.Abs(opts.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Default to directory session if not specified for backward compatibility
	sessionType := opts.SessionType
	if sessionType == "" {
		sessionType = SessionTypeDirectory
	}

	return &Instance{
		Title:            opts.Title,
		Status:           Ready,
		Path:             absPath,
		Program:          opts.Program,
		Height:           0,
		Width:            0,
		CreatedAt:        t,
		UpdatedAt:        t,
		AutoYes:          opts.AutoYes,
		Prompt:           opts.Prompt,
		ExistingWorktree: opts.ExistingWorktree,
		Category:         opts.Category,
		SessionType:      sessionType,
		TmuxPrefix:       opts.TmuxPrefix,
		TmuxServerSocket: opts.TmuxServerSocket,
		IsExpanded:       true, // Default to expanded for newly created instances
	}, nil
}

// NewInstanceWithCleanup creates a new Instance and returns it along with a cleanup function.
// Usage: instance, cleanup, err := NewInstanceWithCleanup(opts); if err == nil { defer cleanup() }
func NewInstanceWithCleanup(opts InstanceOptions) (*Instance, tmux.CleanupFunc, error) {
	instance, err := NewInstance(opts)
	if err != nil {
		return nil, nil, err
	}

	cleanup := tmux.CleanupFunc(func() error {
		if instance.started {
			return instance.Destroy()
		}
		return nil
	})

	return instance, cleanup, nil
}

func (i *Instance) RepoName() (string, error) {
	if !i.started {
		return "", fmt.Errorf("cannot get repo name for instance that has not been started")
	}
	if i.Status == Paused {
		return "", fmt.Errorf("cannot get repo name for paused instance")
	}
	if i.gitWorktree == nil {
		return "", fmt.Errorf("gitWorktree is nil")
	}
	return i.gitWorktree.GetRepoName(), nil
}

func (i *Instance) SetStatus(status Status) {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	i.Status = status
}

// GetCategoryPath returns the category path as a slice of strings for nested category support
// Supports "Work/Frontend" syntax by splitting on "/" delimiter
func (i *Instance) GetCategoryPath() []string {
	if i.Category == "" {
		return []string{"Uncategorized"}
	}
	// Split category by "/" for nested support (e.g., "Work/Frontend" -> ["Work", "Frontend"])
	// Limit to max 2 levels deep for simplicity
	parts := strings.Split(i.Category, "/")
	if len(parts) > 2 {
		// Truncate to first 2 levels if more than 2 levels are provided
		parts = parts[:2]
	}
	return parts
}

// firstTimeSetup is true if this is a new instance. Otherwise, it's one loaded from storage.
func (i *Instance) Start(firstTimeSetup bool) error {
	return i.start(firstTimeSetup, false, nil)
}

// StartWithCleanup starts the instance and returns a cleanup function.
// Usage: cleanup, err := instance.StartWithCleanup(firstTimeSetup); if err == nil { defer cleanup() }
func (i *Instance) StartWithCleanup(firstTimeSetup bool) (tmux.CleanupFunc, error) {
	cleanup := tmux.CleanupFunc(func() error {
		return i.Destroy()
	})
	err := i.start(firstTimeSetup, true, &cleanup)
	if err != nil {
		return nil, err
	}
	return cleanup, nil
}

// start is the internal implementation for Start and StartWithCleanup
func (i *Instance) start(firstTimeSetup bool, setupCleanup bool, cleanup *tmux.CleanupFunc) error {
	log.InfoLog.Printf("Starting instance '%s' (firstTimeSetup: %v)", i.Title, firstTimeSetup)

	if i.Title == "" {
		return fmt.Errorf("instance title cannot be empty")
	}

	log.InfoLog.Printf("Initializing tmux session for instance '%s'", i.Title)
	var tmuxSession *tmux.TmuxSession
	if i.tmuxSession != nil {
		// Use existing tmux session (useful for testing)
		log.InfoLog.Printf("Reusing existing tmux session for instance '%s'", i.Title)
		tmuxSession = i.tmuxSession
	} else {
		// Create new tmux session
		log.InfoLog.Printf("Creating new tmux session for instance '%s' with program '%s'", i.Title, i.Program)
		// Use configurable prefix or default
		tmuxPrefix := i.TmuxPrefix
		if tmuxPrefix == "" {
			tmuxPrefix = "claudesquad_" // Default fallback
		}

		// Use server socket isolation if specified, otherwise use prefix-only isolation
		if i.TmuxServerSocket != "" {
			tmuxSession = tmux.NewTmuxSessionWithServerSocket(i.Title, i.Program, tmuxPrefix, i.TmuxServerSocket)
		} else {
			tmuxSession = tmux.NewTmuxSessionWithPrefix(i.Title, i.Program, tmuxPrefix)
		}
	}
	i.tmuxSession = tmuxSession

	if firstTimeSetup {
		// Handle different session types
		switch i.SessionType {
		case SessionTypeNewWorktree:
			log.InfoLog.Printf("Performing first-time setup: creating git worktree for instance '%s' at path '%s'", i.Title, i.Path)

			// Use the existing Branch field if set for worktree creation
			gitWorktree, branchName, err := git.NewGitWorktreeWithBranch(i.Path, i.Title, i.Branch)
			if err != nil {
				log.ErrorLog.Printf("Failed to create git worktree for instance '%s': %v", i.Title, err)
				return fmt.Errorf("failed to create git worktree: %w", err)
			}
			log.InfoLog.Printf("Git worktree created successfully for instance '%s', branch: '%s'", i.Title, branchName)
			i.gitWorktree = gitWorktree

			// Only set the branch if it wasn't already set manually
			if i.Branch == "" {
				i.Branch = branchName
			}
		case SessionTypeExistingWorktree:
			if i.ExistingWorktree != "" {
				log.InfoLog.Printf("Using existing worktree for instance '%s' at path '%s'", i.Title, i.ExistingWorktree)

				// Create GitWorktree from existing worktree path
				gitWorktree, err := git.NewGitWorktreeFromExisting(i.ExistingWorktree, i.Title)
				if err != nil {
					log.ErrorLog.Printf("Failed to create GitWorktree from existing worktree for instance '%s': %v", i.Title, err)
					return fmt.Errorf("failed to connect to existing worktree: %w", err)
				}

				log.InfoLog.Printf("Successfully connected to existing worktree for instance '%s', branch: '%s'", i.Title, gitWorktree.GetBranchName())
				i.gitWorktree = gitWorktree
				i.Branch = gitWorktree.GetBranchName()
			} else {
				log.WarningLog.Printf("SessionTypeExistingWorktree specified but no ExistingWorktree path provided for instance '%s'", i.Title)
				return fmt.Errorf("existing worktree path required for SessionTypeExistingWorktree")
			}
		case SessionTypeDirectory:
			log.InfoLog.Printf("Creating directory session for instance '%s' at path '%s' (no git worktree)", i.Title, i.Path)
			// No git worktree creation - just a simple directory session
			i.gitWorktree = nil
			i.Branch = ""
		default:
			// Fallback to directory session for backward compatibility
			log.InfoLog.Printf("Unknown session type '%s' for instance '%s', defaulting to directory session", i.SessionType, i.Title)
			i.gitWorktree = nil
			i.Branch = ""
		}
	}

	// Setup error handler to cleanup resources on any error
	var setupErr error
	defer func() {
		if setupErr != nil {
			if cleanupErr := i.Kill(); cleanupErr != nil {
				setupErr = fmt.Errorf("%v (cleanup error: %v)", setupErr, cleanupErr)
			}
			// If we have a cleanup function pointer, set it to nil since startup failed
			if setupCleanup && cleanup != nil {
				*cleanup = func() error { return nil }
			}
		}
	}()

	if !firstTimeSetup {
		// Reuse existing session - use worktree path if available, otherwise use original path
		var workDir string
		if i.gitWorktree != nil {
			workDir = i.gitWorktree.GetWorktreePath()
		} else {
			workDir = i.Path // For directory sessions
		}
		log.InfoLog.Printf("Restoring existing tmux session for instance '%s' with workDir '%s'", i.Title, workDir)
		if err := tmuxSession.RestoreWithWorkDir(workDir); err != nil {
			log.ErrorLog.Printf("Failed to restore tmux session for instance '%s': %v", i.Title, err)
			setupErr = fmt.Errorf("failed to restore existing session: %w", err)
			return setupErr
		}
		log.InfoLog.Printf("Successfully restored tmux session for instance '%s'", i.Title)
	} else {
		var startPath string

		if i.gitWorktree != nil {
			// Setup git worktree first
			log.InfoLog.Printf("Setting up git worktree for instance '%s'", i.Title)
			if err := i.gitWorktree.Setup(); err != nil {
				log.ErrorLog.Printf("Failed to setup git worktree for instance '%s': %v", i.Title, err)
				setupErr = fmt.Errorf("failed to setup git worktree: %w", err)
				return setupErr
			}
			log.InfoLog.Printf("Git worktree setup completed for instance '%s'", i.Title)

			// Create new session with worktree path
			worktreePath := i.gitWorktree.GetWorktreePath()
			startPath = worktreePath

			// Use the working directory if specified
			if i.WorkingDir != "" {
				// Calculate the full path combining worktree path with working dir
				if !filepath.IsAbs(i.WorkingDir) {
					startPath = filepath.Join(worktreePath, i.WorkingDir)
				} else {
					startPath = i.WorkingDir
				}

				// Verify the path exists
				if _, err := os.Stat(startPath); os.IsNotExist(err) {
					log.WarningLog.Printf("Working directory '%s' doesn't exist, using worktree root '%s' instead", startPath, worktreePath)
					startPath = worktreePath
				}
			}
		} else {
			// Directory session - use the original path directly
			startPath = i.Path

			// Use the working directory if specified
			if i.WorkingDir != "" {
				// Calculate the full path combining base path with working dir
				if !filepath.IsAbs(i.WorkingDir) {
					startPath = filepath.Join(i.Path, i.WorkingDir)
				} else {
					startPath = i.WorkingDir
				}

				// Verify the path exists
				if _, err := os.Stat(startPath); os.IsNotExist(err) {
					log.WarningLog.Printf("Working directory '%s' doesn't exist, using base path '%s' instead", startPath, i.Path)
					startPath = i.Path
				}
			}
			log.InfoLog.Printf("Starting directory session for instance '%s' in path '%s'", i.Title, startPath)
		}

		// Start the session in the specified directory
		if err := i.tmuxSession.Start(startPath); err != nil {
			// Cleanup git worktree if tmux session creation fails (only if worktree exists)
			if i.gitWorktree != nil {
				if cleanupErr := i.gitWorktree.Cleanup(); cleanupErr != nil {
					err = fmt.Errorf("%v (cleanup error: %v)", err, cleanupErr)
				}
			}
			setupErr = fmt.Errorf("failed to start new session: %w", err)
			return setupErr
		}
	}

	i.SetStatus(Running)

	// Mark instance as successfully started before returning
	// This must happen before the function returns so that any finalizers
	// that check Started() will see the correct state
	i.started = true

	return nil
}

// Kill terminates the instance and cleans up all resources
// Kill destroys both tmux session and worktree (legacy method)
func (i *Instance) Kill() error {
	return i.Destroy()
}

// Destroy completely destroys the instance - both tmux session and worktree
func (i *Instance) Destroy() error {
	if !i.started {
		// If instance was never started, just return success
		return nil
	}

	var errs []error

	// Always try to cleanup both resources, even if one fails
	// Clean up tmux session first since it's using the git worktree
	if err := i.KillSession(); err != nil {
		errs = append(errs, err)
	}

	// Then clean up git worktree
	if err := i.CleanupWorktree(); err != nil {
		errs = append(errs, err)
	}

	return i.combineErrors(errs)
}

// KillSession terminates only the tmux session, keeping worktree intact
func (i *Instance) KillSession() error {
	if i.tmuxSession != nil {
		if err := i.tmuxSession.Close(); err != nil {
			return fmt.Errorf("failed to close tmux session: %w", err)
		}
	}
	return nil
}

// CleanupWorktree removes the git worktree, keeping session intact
func (i *Instance) CleanupWorktree() error {
	if i.gitWorktree != nil {
		if err := i.gitWorktree.Cleanup(); err != nil {
			return fmt.Errorf("failed to cleanup git worktree: %w", err)
		}
	}
	return nil
}

// KillSessionKeepWorktree terminates tmux session but preserves worktree for recovery scenarios
func (i *Instance) KillSessionKeepWorktree() error {
	return i.KillSession()
}

// combineErrors combines multiple errors into a single error
func (i *Instance) combineErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}

	errMsg := "multiple cleanup errors occurred:"
	for _, err := range errs {
		errMsg += "\n  - " + err.Error()
	}
	return fmt.Errorf("%s", errMsg)
}

func (i *Instance) Preview() (string, error) {
	if !i.started || i.Status == Paused {
		return "", nil
	}

	// Check if the tmux session is still alive before trying to capture content
	if !i.TmuxAlive() {
		return "", nil
	}

	return i.tmuxSession.CapturePaneContent()
}

func (i *Instance) HasUpdated() (updated bool, hasPrompt bool) {
	if !i.started || i.Status == Paused {
		return false, false
	}

	// Check if the tmux session is still alive
	if !i.TmuxAlive() {
		return false, false
	}

	return i.tmuxSession.HasUpdated()
}

// TapEnter sends an enter key press to the tmux session if AutoYes is enabled.
func (i *Instance) TapEnter() {
	if !i.started || !i.AutoYes {
		return
	}
	if err := i.tmuxSession.TapEnter(); err != nil {
		log.ErrorLog.Printf("error tapping enter: %v", err)
	}
}

func (i *Instance) Attach() (chan struct{}, error) {
	if !i.started {
		return nil, fmt.Errorf("cannot attach instance that has not been started")
	}
	return i.tmuxSession.Attach()
}

func (i *Instance) SetPreviewSize(width, height int) error {
	if !i.started || i.Status == Paused {
		return fmt.Errorf("cannot set preview size for instance that has not been started or " +
			"is paused")
	}

	// Skip resize if dimensions haven't changed
	if width == i.lastPreviewWidth && height == i.lastPreviewHeight {
		return nil
	}

	// Defensive check: ensure tmux session is properly initialized before trying to resize PTY
	if i.tmuxSession == nil {
		return fmt.Errorf("tmux session not initialized")
	}

	// Attempt to set size, but handle PTY initialization errors gracefully
	if err := i.tmuxSession.SetDetachedSize(width, height); err != nil {
		// If it's a PTY initialization error, log it but don't propagate to avoid UI disruption
		if strings.Contains(err.Error(), "PTY is not initialized") {
			// Rate-limit warnings to avoid log spam (max once per 30 seconds per instance)
			now := time.Now()
			if now.Sub(i.lastPTYWarningTime) > 30*time.Second {
				log.WarningLog.Printf("PTY not ready for instance '%s', skipping resize: %v", i.Title, err)
				i.lastPTYWarningTime = now
			}
			return nil // Return nil to prevent UI disruption
		}
		// For other errors, propagate them
		return err
	}

	// Update tracked dimensions after successful resize
	i.lastPreviewWidth = width
	i.lastPreviewHeight = height
	return nil
}

// GetGitWorktree returns the git worktree for the instance
func (i *Instance) GetGitWorktree() (*git.GitWorktree, error) {
	if !i.started {
		return nil, fmt.Errorf("cannot get git worktree for instance that has not been started")
	}
	return i.gitWorktree, nil
}

func (i *Instance) Started() bool {
	return i.started
}

// SetTitle sets the title of the instance. Returns an error if the instance has started.
// We cant change the title once it's been used for a tmux session etc.
func (i *Instance) SetTitle(title string) error {
	if i.started {
		return fmt.Errorf("cannot change title of a started instance")
	}
	i.Title = title
	return nil
}

func (i *Instance) Paused() bool {
	return i.Status == Paused
}

// TmuxAlive returns true if the tmux session is alive. This is a sanity check before attaching.
func (i *Instance) TmuxAlive() bool {
	if i.Status == Paused || !i.started || i.tmuxSession == nil {
		return false
	}
	return i.tmuxSession.DoesSessionExist()
}

// Pause stops the tmux session and removes the worktree, preserving the branch
func (i *Instance) Pause() error {
	if !i.started {
		return fmt.Errorf("cannot pause instance that has not been started")
	}
	if i.Status == Paused {
		return fmt.Errorf("instance is already paused")
	}

	var errs []error

	// Check if there are any changes to commit
	if dirty, err := i.gitWorktree.IsDirty(); err != nil {
		errs = append(errs, fmt.Errorf("failed to check if worktree is dirty: %w", err))
		log.ErrorLog.Print(err)
	} else if dirty {
		// Commit changes locally (without pushing to GitHub)
		commitMsg := fmt.Sprintf("[claudesquad] update from '%s' on %s (paused)", i.Title, time.Now().Format(time.RFC822))
		if err := i.gitWorktree.CommitChanges(commitMsg); err != nil {
			errs = append(errs, fmt.Errorf("failed to commit changes: %w", err))
			log.ErrorLog.Print(err)
			// Return early if we can't commit changes to avoid corrupted state
			return i.combineErrors(errs)
		}
	}

	// Detach from tmux session instead of closing to preserve session output
	if err := i.tmuxSession.DetachSafely(); err != nil {
		errs = append(errs, fmt.Errorf("failed to detach tmux session: %w", err))
		log.ErrorLog.Print(err)
		// Continue with pause process even if detach fails
	}

	// Check if worktree exists before trying to remove it
	if _, err := os.Stat(i.gitWorktree.GetWorktreePath()); err == nil {
		// Remove worktree but keep branch
		if err := i.gitWorktree.Remove(); err != nil {
			errs = append(errs, fmt.Errorf("failed to remove git worktree: %w", err))
			log.ErrorLog.Print(err)
			return i.combineErrors(errs)
		}

		// Only prune if remove was successful
		if err := i.gitWorktree.Prune(); err != nil {
			errs = append(errs, fmt.Errorf("failed to prune git worktrees: %w", err))
			log.ErrorLog.Print(err)
			return i.combineErrors(errs)
		}
	}

	if err := i.combineErrors(errs); err != nil {
		log.ErrorLog.Print(err)
		return err
	}

	i.SetStatus(Paused)
	_ = clipboard.WriteAll(i.gitWorktree.GetBranchName())
	return nil
}

// Resume recreates the worktree and restarts the tmux session
func (i *Instance) Resume() error {
	if !i.started {
		return fmt.Errorf("cannot resume instance that has not been started")
	}
	if i.Status != Paused {
		return fmt.Errorf("can only resume paused instances")
	}

	// Check if branch is checked out
	if checked, err := i.gitWorktree.IsBranchCheckedOut(); err != nil {
		log.ErrorLog.Print(err)
		return fmt.Errorf("failed to check if branch is checked out: %w", err)
	} else if checked {
		return fmt.Errorf("cannot resume: branch is checked out, please switch to a different branch")
	}

	// Setup git worktree
	if err := i.gitWorktree.Setup(); err != nil {
		log.ErrorLog.Print(err)
		return fmt.Errorf("failed to setup git worktree: %w", err)
	}

	// Handle Claude Code session re-attachment if configured
	if err := i.handleClaudeSessionReattachment(); err != nil {
		log.WarningLog.Printf("Failed to re-attach to Claude Code session: %v", err)
		// Continue with resume - Claude session attachment is not critical for basic functionality
	}

	// Check if tmux session still exists from pause, otherwise create new one
	worktreePath := i.gitWorktree.GetWorktreePath()
	if i.tmuxSession.DoesSessionExist() {
		// Session exists, just restore PTY connection to it (retains stdout from before pause)
		if err := i.tmuxSession.RestoreWithWorkDir(worktreePath); err != nil {
			log.ErrorLog.Print(err)
			// If restore fails, fall back to creating new session
			if err := i.tmuxSession.Start(worktreePath); err != nil {
				log.ErrorLog.Print(err)
				// Cleanup git worktree if tmux session creation fails
				if cleanupErr := i.gitWorktree.Cleanup(); cleanupErr != nil {
					err = fmt.Errorf("%v (cleanup error: %v)", err, cleanupErr)
					log.ErrorLog.Print(err)
				}
				return fmt.Errorf("failed to start new session: %w", err)
			}
		}
	} else {
		// Create new tmux session
		if err := i.tmuxSession.Start(i.gitWorktree.GetWorktreePath()); err != nil {
			log.ErrorLog.Print(err)
			// Cleanup git worktree if tmux session creation fails
			if cleanupErr := i.gitWorktree.Cleanup(); cleanupErr != nil {
				err = fmt.Errorf("%v (cleanup error: %v)", err, cleanupErr)
				log.ErrorLog.Print(err)
			}
			return fmt.Errorf("failed to start new session: %w", err)
		}
	}

	i.SetStatus(Running)
	return nil
}

// GetPTYReader returns an io.Reader for streaming terminal output.
// This method provides access to the PTY output for terminal streaming implementations.
// Returns an error if the session is not started or the tmux session is not initialized.
func (i *Instance) GetPTYReader() (*os.File, error) {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()

	if !i.started {
		return nil, fmt.Errorf("session not started")
	}

	if i.tmuxSession == nil {
		return nil, fmt.Errorf("tmux session not initialized")
	}

	// The tmux session's ptmx field is the PTY master that we can read from
	// We need to expose this via a method on TmuxSession
	return i.tmuxSession.GetPTY()
}

// WriteToPTY writes data to the PTY, sending input to the terminal session.
// This is used for forwarding client input to the tmux session.
func (i *Instance) WriteToPTY(data []byte) (int, error) {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()

	if !i.started {
		return 0, fmt.Errorf("session not started")
	}

	if i.tmuxSession == nil {
		return 0, fmt.Errorf("tmux session not initialized")
	}

	// Forward the input to the tmux PTY
	return i.tmuxSession.SendKeys(string(data))
}

// ResizePTY resizes the terminal dimensions.
// This is used when clients resize their terminal windows.
func (i *Instance) ResizePTY(cols, rows int) error {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()

	if !i.started {
		return fmt.Errorf("session not started")
	}

	if i.tmuxSession == nil {
		return fmt.Errorf("tmux session not initialized")
	}

	// Use the existing SetWindowSize method
	i.tmuxSession.SetWindowSize(cols, rows)
	return nil
}

// UpdateDiffStats updates the git diff statistics for this instance
func (i *Instance) UpdateDiffStats() error {
	// Use read lock for initial state checks, then upgrade to write lock if needed
	i.stateMutex.RLock()
	if !i.started {
		i.diffStats = nil
		i.stateMutex.RUnlock()
		return nil
	}

	if i.Status == Paused {
		// Keep the previous diff stats if the instance is paused
		i.stateMutex.RUnlock()
		return nil
	}

	// Check if the worktree directory exists before attempting git operations
	if i.gitWorktree == nil {
		i.diffStats = nil
		i.stateMutex.RUnlock()
		return nil
	}

	worktreePath := i.gitWorktree.GetWorktreePath()
	i.stateMutex.RUnlock()

	// Check if worktree path exists (this is an I/O operation, do it outside the lock)
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		// Need write lock to modify state
		i.stateMutex.Lock()
		defer i.stateMutex.Unlock()

		// Double-check state hasn't changed while acquiring lock
		if i.Status != Paused {
			// The worktree directory doesn't exist, mark the instance as paused
			log.WarningLog.Printf("Worktree directory for '%s' doesn't exist at '%s', marking as paused", i.Title, worktreePath)
			i.Status = Paused
		}
		i.diffStats = nil
		return nil
	}

	// Perform git diff operation (I/O operation, outside lock)
	stats := i.gitWorktree.Diff()
	if stats.Error != nil {
		if strings.Contains(stats.Error.Error(), "base commit SHA not set") {
			// Worktree is not fully set up yet, not an error
			i.stateMutex.Lock()
			i.diffStats = nil
			i.stateMutex.Unlock()
			return nil
		}
		return fmt.Errorf("failed to get diff stats: %w", stats.Error)
	}

	// Update diff stats with write lock
	i.stateMutex.Lock()
	i.diffStats = stats
	i.stateMutex.Unlock()
	return nil
}

// GetDiffStats returns the current git diff statistics
func (i *Instance) GetDiffStats() *git.DiffStats {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()
	return i.diffStats
}

// SendPrompt sends a prompt to the tmux session
func (i *Instance) SendPrompt(prompt string) error {
	if !i.started {
		return fmt.Errorf("instance not started")
	}
	if i.tmuxSession == nil {
		return fmt.Errorf("tmux session not initialized")
	}
	if _, err := i.tmuxSession.SendKeys(prompt); err != nil {
		return fmt.Errorf("error sending keys to tmux session: %w", err)
	}

	// Brief pause to prevent carriage return from being interpreted as newline
	time.Sleep(100 * time.Millisecond)
	if err := i.tmuxSession.TapEnter(); err != nil {
		return fmt.Errorf("error tapping enter: %w", err)
	}

	return nil
}

// PreviewFullHistory captures the entire tmux pane output including full scrollback history
func (i *Instance) PreviewFullHistory() (string, error) {
	if !i.started || i.Status == Paused {
		return "", nil
	}

	// Check if the tmux session is still alive before trying to capture content
	if !i.TmuxAlive() {
		return "", nil
	}

	return i.tmuxSession.CapturePaneContentWithOptions("-", "-")
}

// SetTmuxSession sets the tmux session for testing purposes
func (i *Instance) SetTmuxSession(session *tmux.TmuxSession) {
	i.tmuxSession = session
	i.started = session != nil
}

// SetWindowSize propagates window size changes to the tmux session
// This enables proper terminal resizing in environments like IntelliJ where SIGWINCH doesn't work
func (i *Instance) SetWindowSize(cols, rows int) {
	if i.tmuxSession != nil {
		i.tmuxSession.SetWindowSize(cols, rows)
	}
}

// SetGitWorktree sets the git worktree for testing purposes
func (i *Instance) SetGitWorktree(worktree *git.GitWorktree) {
	i.gitWorktree = worktree
	i.started = worktree != nil
}

// SendKeys sends keys to the tmux session
func (i *Instance) SendKeys(keys string) error {
	if !i.started || i.Status == Paused {
		return fmt.Errorf("cannot send keys to instance that has not been started or is paused")
	}
	_, err := i.tmuxSession.SendKeys(keys)
	return err
}

// Claude Code session management methods

// handleClaudeSessionReattachment attempts to re-attach to stored Claude Code session
func (i *Instance) handleClaudeSessionReattachment() error {
	if i.claudeSession == nil {
		log.InfoLog.Printf("No Claude Code session data stored for instance '%s'", i.Title)
		return nil
	}

	// Check if auto-reattachment is enabled
	if !i.claudeSession.Settings.AutoReattach {
		log.InfoLog.Printf("Auto-reattachment disabled for instance '%s'", i.Title)
		return nil
	}

	// Check if session is too old (based on timeout settings)
	timeoutMinutes := i.claudeSession.Settings.SessionTimeoutMinutes
	if timeoutMinutes > 0 {
		timeout := time.Duration(timeoutMinutes) * time.Minute
		if time.Since(i.claudeSession.LastAttached) > timeout {
			log.InfoLog.Printf("Claude Code session for '%s' has timed out (%v ago), skipping re-attachment",
				i.Title, time.Since(i.claudeSession.LastAttached))
			return nil
		}
	}

	// Initialize Claude session manager
	sessionManager := NewClaudeSessionManager()

	// Try to find and attach to the stored session
	if i.claudeSession.SessionID != "" {
		log.InfoLog.Printf("Attempting to re-attach to Claude Code session '%s' for instance '%s'",
			i.claudeSession.SessionID, i.Title)

		// Verify the session still exists
		session, err := sessionManager.GetSessionByID(i.claudeSession.SessionID)
		if err != nil {
			if i.claudeSession.Settings.CreateNewOnMissing {
				log.InfoLog.Printf("Stored Claude session not found, will create new session for '%s'", i.Title)
				return i.createNewClaudeSession()
			}
			return fmt.Errorf("stored Claude session '%s' not found: %w", i.claudeSession.SessionID, err)
		}

		// Attempt to attach to the existing session
		if err := sessionManager.AttachToSession(session.ID); err != nil {
			return fmt.Errorf("failed to attach to Claude session '%s': %w", session.ID, err)
		}

		// Update last attached timestamp
		i.claudeSession.LastAttached = time.Now()
		log.InfoLog.Printf("Successfully re-attached to Claude Code session '%s'", session.ID)
	} else {
		// No specific session ID stored, try to find matching sessions by project
		if i.gitWorktree != nil {
			return i.findAndAttachToProjectSession(sessionManager)
		}
	}

	return nil
}

// createNewClaudeSession creates a new Claude Code session for this instance
func (i *Instance) createNewClaudeSession() error {
	log.InfoLog.Printf("Creating new Claude Code session for instance '%s'", i.Title)

	// TODO: Implement actual Claude Code session creation
	// This would typically involve:
	// 1. Launching Claude Code with the project directory
	// 2. Waiting for session initialization
	// 3. Capturing the new session ID

	// For now, create placeholder session data
	// sessionManager := NewClaudeSessionManager() // TODO: Use this when implementing actual Claude session creation

	// Generate a placeholder session ID (in practice, this would come from Claude Code)
	newSessionID := fmt.Sprintf("session_%s_%d", i.Title, time.Now().Unix())

	newSession := ClaudeSession{
		ID:             newSessionID,
		ConversationID: "",
		ProjectName:    i.Title,
		LastActive:     time.Now(),
		WorkingDir:     i.GetWorkingDirectory(),
		IsActive:       true,
	}

	// Update the instance's Claude session data
	i.claudeSession = &ClaudeSessionData{
		SessionID:      newSession.ID,
		ConversationID: newSession.ConversationID,
		ProjectName:    newSession.ProjectName,
		LastAttached:   time.Now(),
		Settings:       i.claudeSession.Settings, // Preserve existing settings
		Metadata: map[string]string{
			"working_dir": newSession.WorkingDir,
			"created_at":  time.Now().Format(time.RFC3339),
		},
	}

	log.InfoLog.Printf("Created new Claude Code session '%s' for instance '%s'",
		newSessionID, i.Title)

	return nil
}

// findAndAttachToProjectSession finds Claude sessions matching this instance's project
func (i *Instance) findAndAttachToProjectSession(sessionManager *ClaudeSessionManager) error {
	projectPath := i.GetWorkingDirectory()
	if projectPath == "" {
		return fmt.Errorf("no working directory available for project matching")
	}

	// Find sessions that match this project
	matchingSessions, err := sessionManager.FindSessionByProject(projectPath)
	if err != nil {
		return fmt.Errorf("failed to find matching Claude sessions: %w", err)
	}

	if len(matchingSessions) == 0 {
		if i.claudeSession.Settings.CreateNewOnMissing {
			log.InfoLog.Printf("No matching Claude sessions found for project '%s', creating new session", projectPath)
			return i.createNewClaudeSession()
		}
		return fmt.Errorf("no matching Claude sessions found for project '%s'", projectPath)
	}

	// Use the most recently active session
	var selectedSession ClaudeSession
	for _, session := range matchingSessions {
		if selectedSession.ID == "" || session.LastActive.After(selectedSession.LastActive) {
			selectedSession = session
		}
	}

	// Attach to the selected session
	if err := sessionManager.AttachToSession(selectedSession.ID); err != nil {
		return fmt.Errorf("failed to attach to Claude session '%s': %w", selectedSession.ID, err)
	}

	// Update the instance's Claude session data
	if i.claudeSession == nil {
		i.claudeSession = &ClaudeSessionData{}
	}
	i.claudeSession.SessionID = selectedSession.ID
	i.claudeSession.ConversationID = selectedSession.ConversationID
	i.claudeSession.ProjectName = selectedSession.ProjectName
	i.claudeSession.LastAttached = time.Now()
	if i.claudeSession.Metadata == nil {
		i.claudeSession.Metadata = make(map[string]string)
	}
	i.claudeSession.Metadata["working_dir"] = selectedSession.WorkingDir

	log.InfoLog.Printf("Successfully attached to Claude Code session '%s' for project '%s'",
		selectedSession.ID, projectPath)

	return nil
}

// GetWorkingDirectory returns the working directory for this instance
func (i *Instance) GetWorkingDirectory() string {
	if i.gitWorktree != nil {
		return i.gitWorktree.GetWorktreePath()
	}
	return i.Path
}

// GetClaudeSession returns the Claude session data for this instance
func (i *Instance) GetClaudeSession() *ClaudeSessionData {
	return i.claudeSession
}

// SetClaudeSession sets the Claude session data for this instance
func (i *Instance) SetClaudeSession(sessionData *ClaudeSessionData) {
	i.claudeSession = sessionData
}

// HasClaudeSession returns true if this instance has Claude session data
func (i *Instance) HasClaudeSession() bool {
	return i.claudeSession != nil && i.claudeSession.SessionID != ""
}
