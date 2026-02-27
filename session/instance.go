package session

import (
	"claude-squad/log"
	"claude-squad/session/git"
	"claude-squad/session/tmux"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/atotto/clipboard"
	"github.com/spaolacci/murmur3"
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
	// Tags are multi-valued labels for flexible session organization
	// Sessions can have multiple tags and appear in multiple groups simultaneously
	// Examples: ["frontend", "urgent", "client-work"]
	Tags []string

	// GitHub integration fields for PR/URL-based session creation
	// GitHubPRNumber is the PR number if this session was created from a PR URL
	GitHubPRNumber int `json:"github_pr_number,omitempty"`
	// GitHubPRURL is the full URL to the PR on GitHub
	GitHubPRURL string `json:"github_pr_url,omitempty"`
	// GitHubOwner is the repository owner (user or organization)
	GitHubOwner string `json:"github_owner,omitempty"`
	// GitHubRepo is the repository name
	GitHubRepo string `json:"github_repo,omitempty"`
	// GitHubSourceRef is the original URL or reference used to create this session
	GitHubSourceRef string `json:"github_source_ref,omitempty"`
	// ClonedRepoPath is the path where we cloned the repo (if cloned)
	ClonedRepoPath string `json:"cloned_repo_path,omitempty"`
	// MainRepoPath is the path to the main repository when Path is a worktree
	// Detected automatically via `git rev-parse --git-common-dir`
	MainRepoPath string `json:"main_repo_path,omitempty"`
	// IsWorktree indicates whether Path is a git worktree (not the main repo)
	IsWorktree bool `json:"is_worktree,omitempty"`

	// DiffStats stores the current git diff statistics
	diffStats *git.DiffStats

	// Claude Code session information for persistence and re-attachment
	claudeSession *ClaudeSessionData

	// Review queue integration for tracking sessions needing attention
	reviewQueue *ReviewQueue

	// ReviewState holds all review queue and terminal activity timestamps.
	// Fields are embedded (promoted) so external code can still access inst.LastViewed etc.
	// Protected by stateMutex.
	ReviewState

	// Status manager for idle detection and queue management
	statusManager *InstanceStatusManager
	// Claude controller for automated interaction and status monitoring
	claudeController *ClaudeController

	// Instance type and management metadata
	// InstanceType indicates whether this is a squad-managed or external instance
	InstanceType InstanceType
	// IsManaged is true if this is a squad-managed session (backward compatible helper)
	IsManaged bool
	// ExternalMetadata contains additional information for externally discovered instances
	ExternalMetadata *ExternalInstanceMetadata
	// Permissions defines what operations are allowed on this instance
	Permissions InstancePermissions

	// The below fields are initialized upon calling Start().

	started bool
	// tmuxSession is the tmux session for the instance.
	tmuxSession *tmux.TmuxSession
	// gitWorktree is the git worktree for the instance.
	gitWorktree *git.GitWorktree

	// Mutex to protect concurrent access to instance state
	stateMutex sync.RWMutex

	// Preview size tracking to avoid unnecessary resize operations
	lastPreviewWidth   int
	lastPreviewHeight  int
	lastPTYWarningTime time.Time
}

// ToInstanceData converts an Instance to its serializable form
func (i *Instance) ToInstanceData() InstanceData {
	data := InstanceData{
		Title:                i.Title,
		Path:                 i.Path,
		WorkingDir:           i.WorkingDir,
		Branch:               i.Branch,
		Status:               i.Status,
		Height:               i.Height,
		Width:                i.Width,
		CreatedAt:            i.CreatedAt,
		UpdatedAt:            time.Now(),
		Program:              i.Program,
		AutoYes:              i.AutoYes,
		Prompt:               i.Prompt,
		Category:             i.Category,
		IsExpanded:           i.IsExpanded,
		Tags:                 i.Tags, // Include tags in serialization
		SessionType:          i.SessionType,
		TmuxPrefix:           i.TmuxPrefix,
		LastTerminalUpdate:   i.LastTerminalUpdate,
		LastMeaningfulOutput: i.LastMeaningfulOutput,
		LastOutputSignature:  i.LastOutputSignature,
		LastAddedToQueue:     i.LastAddedToQueue,
		LastViewed:           i.LastViewed,
		LastAcknowledged:     i.LastAcknowledged,
		// Prompt detection and interaction tracking
		LastPromptDetected:   i.LastPromptDetected,
		LastPromptSignature:  i.LastPromptSignature,
		LastUserResponse:     i.LastUserResponse,
		ProcessingGraceUntil: i.ProcessingGraceUntil,
		// GitHub integration fields
		GitHubPRNumber:  i.GitHubPRNumber,
		GitHubPRURL:     i.GitHubPRURL,
		GitHubOwner:     i.GitHubOwner,
		GitHubRepo:      i.GitHubRepo,
		GitHubSourceRef: i.GitHubSourceRef,
		ClonedRepoPath:  i.ClonedRepoPath,
		// Worktree detection fields
		MainRepoPath: i.MainRepoPath,
		IsWorktree:   i.IsWorktree,
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
	// MIGRATION: Fix corrupted paths from before defensive tilde expansion was added
	// Detect paths like "/absolute/path/~/other/path" and fix them
	migratedPath := data.Path
	if strings.Contains(data.Path, "/~/") {
		// Path contains unexpanded tilde - extract and expand it
		log.WarningLog.Printf("Migrating corrupted path for instance '%s': %s", data.Title, data.Path)

		// Find the index of "/~/"
		idx := strings.Index(data.Path, "/~/")
		if idx >= 0 {
			// Extract the tilde path (everything from "~/" onwards)
			tildePath := data.Path[idx+1:] // Skip the leading "/"

			// Expand the tilde path
			if strings.HasPrefix(tildePath, "~/") {
				usr, err := user.Current()
				if err != nil {
					log.ErrorLog.Printf("Failed to expand corrupted path for '%s': %v", data.Title, err)
					// Fall back to original path
				} else {
					migratedPath = filepath.Join(usr.HomeDir, tildePath[2:])
					log.InfoLog.Printf("Migrated path for instance '%s': %s -> %s", data.Title, data.Path, migratedPath)
				}
			}
		}
	}

	// MIGRATION: Convert legacy Category to Tags for backward compatibility
	// If Tags is empty but Category exists, migrate category to tags
	tags := data.Tags
	if len(tags) == 0 && data.Category != "" {
		// Migrate existing category to tag format
		// Support both simple ("Work") and nested ("Work/Frontend") categories
		tags = []string{data.Category}
		log.InfoLog.Printf("Migrating category '%s' to tags for instance '%s'", data.Category, data.Title)
	}

	instance := &Instance{
		Title:                data.Title,
		Path:                 migratedPath, // Use migrated path
		WorkingDir:           data.WorkingDir,
		Branch:               data.Branch,
		Status:               data.Status,
		Height:               data.Height,
		Width:                data.Width,
		CreatedAt:            data.CreatedAt,
		UpdatedAt:            data.UpdatedAt,
		Program:              data.Program,
		Prompt:               data.Prompt,
		Category:             data.Category,
		IsExpanded:           data.IsExpanded,
		Tags:                 tags, // Use migrated tags (includes category if needed)
		SessionType:          data.SessionType,
		TmuxPrefix: data.TmuxPrefix,
		ReviewState: ReviewState{
			LastTerminalUpdate:   data.LastTerminalUpdate,
			LastMeaningfulOutput: data.LastMeaningfulOutput,
			LastOutputSignature:  data.LastOutputSignature,
			LastAddedToQueue:     data.LastAddedToQueue,
			LastViewed:           data.LastViewed,
			LastAcknowledged:     data.LastAcknowledged,
			LastPromptDetected:   data.LastPromptDetected,
			LastPromptSignature:  data.LastPromptSignature,
			LastUserResponse:     data.LastUserResponse,
			ProcessingGraceUntil: data.ProcessingGraceUntil,
		},
		InstanceType: InstanceTypeManaged,     // Restored instances are always managed
		IsManaged:            true,
		ExternalMetadata:     nil,                    // External instances are not persisted
		Permissions:          GetManagedPermissions(), // Full permissions for managed instances
		// GitHub integration fields
		GitHubPRNumber:  data.GitHubPRNumber,
		GitHubPRURL:     data.GitHubPRURL,
		GitHubOwner:     data.GitHubOwner,
		GitHubRepo:      data.GitHubRepo,
		GitHubSourceRef: data.GitHubSourceRef,
		ClonedRepoPath:  data.ClonedRepoPath,
		// Worktree detection fields
		MainRepoPath: data.MainRepoPath,
		IsWorktree:   data.IsWorktree,
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

	// Auto-detect worktree info for migration (existing sessions without this info)
	// This populates IsWorktree, MainRepoPath, GitHubOwner, and GitHubRepo
	if instance.GitHubOwner == "" || instance.GitHubRepo == "" {
		if err := instance.DetectAndPopulateWorktreeInfo(); err != nil {
			log.WarningLog.Printf("Failed to detect worktree info for '%s': %v", instance.Title, err)
			// Non-fatal - session can still work without this info
		} else if instance.GitHubOwner != "" {
			log.InfoLog.Printf("Auto-detected GitHub info for '%s': %s/%s (worktree=%v)",
				instance.Title, instance.GitHubOwner, instance.GitHubRepo, instance.IsWorktree)
		}
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
	// Branch is the git branch name to use when creating a new worktree.
	// If empty and SessionType is SessionTypeNewWorktree, a branch name is derived from the title.
	Branch string
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
	// Tags are multi-valued labels for flexible organization
	Tags []string
	// SessionType determines the session workflow (directory, new_worktree, existing_worktree)
	SessionType SessionType
	// TmuxPrefix is the prefix to use for tmux session names (e.g., "claudesquad_")
	TmuxPrefix string
	// TmuxServerSocket is the server socket name for tmux isolation (used with -L flag)
	// If empty, uses the default tmux server. For complete isolation (e.g., testing),
	// set to a unique value like "test" or "teatest_123" to create separate tmux servers.
	TmuxServerSocket string
	// GitHub integration fields for PR/URL-based session creation
	GitHubPRNumber  int    // PR number if created from PR URL
	GitHubPRURL     string // Full URL to the PR
	GitHubOwner     string // Repository owner
	GitHubRepo      string // Repository name
	GitHubSourceRef string // Original URL/reference used to create session
	ClonedRepoPath  string // Path where repo was cloned (if cloned)
	// ResumeId is the Claude conversation ID to resume (from history browser).
	// When set, the session will start with --resume <id> flag.
	ResumeId string
}

func NewInstance(opts InstanceOptions) (*Instance, error) {
	t := time.Now()

	// DEFENSIVE: Expand tilde (~) in path before converting to absolute
	// This prevents bugs where unexpanded tildes get concatenated with current directory
	// Example: ~/foo becomes /current/dir/~/foo instead of /home/user/foo
	expandedPath := opts.Path
	if strings.HasPrefix(expandedPath, "~/") {
		usr, err := user.Current()
		if err != nil {
			return nil, fmt.Errorf("failed to expand home directory in path '%s': %w", opts.Path, err)
		}
		expandedPath = filepath.Join(usr.HomeDir, expandedPath[2:])
	} else if expandedPath == "~" {
		usr, err := user.Current()
		if err != nil {
			return nil, fmt.Errorf("failed to expand home directory in path '%s': %w", opts.Path, err)
		}
		expandedPath = usr.HomeDir
	}

	// Convert to absolute path (after tilde expansion)
	absPath, err := filepath.Abs(expandedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for '%s': %w", expandedPath, err)
	}

	// Default to directory session if not specified for backward compatibility
	sessionType := opts.SessionType
	if sessionType == "" {
		sessionType = SessionTypeDirectory
	}

	instance := &Instance{
		Title:                opts.Title,
		Status:               Ready,
		Path:                 absPath,
		Branch:               opts.Branch,
		Program:              opts.Program,
		Height:               0,
		Width:                0,
		CreatedAt:            t,
		UpdatedAt:            t,
		AutoYes:              opts.AutoYes,
		Prompt:               opts.Prompt,
		ExistingWorktree:     opts.ExistingWorktree,
		Category:             opts.Category,
		Tags:                 opts.Tags, // Set tags from options
		SessionType:          sessionType,
		TmuxPrefix:           opts.TmuxPrefix,
		TmuxServerSocket:     opts.TmuxServerSocket,
		IsExpanded:           true, // Default to expanded for newly created instances
		InstanceType:         InstanceTypeManaged,
		IsManaged:            true,
		ExternalMetadata:     nil,                    // Only set for external instances
		Permissions: GetManagedPermissions(), // Full permissions for managed instances
		ReviewState: ReviewState{
			LastTerminalUpdate:   t, // Initialize to creation time
			LastMeaningfulOutput: t, // Initialize to creation time
		},
		// GitHub integration fields
		GitHubPRNumber:  opts.GitHubPRNumber,
		GitHubPRURL:     opts.GitHubPRURL,
		GitHubOwner:     opts.GitHubOwner,
		GitHubRepo:      opts.GitHubRepo,
		GitHubSourceRef: opts.GitHubSourceRef,
		ClonedRepoPath:  opts.ClonedRepoPath,
	}

	// Auto-detect worktree info if GitHub owner/repo not explicitly set
	// This extracts repository information from the git remote URL
	if instance.GitHubOwner == "" || instance.GitHubRepo == "" {
		if err := instance.DetectAndPopulateWorktreeInfo(); err != nil {
			log.WarningLog.Printf("Failed to detect worktree info for new instance '%s': %v", opts.Title, err)
			// Non-fatal - instance can still be created without this info
		} else if instance.GitHubOwner != "" {
			log.InfoLog.Printf("Auto-detected GitHub info for new instance '%s': %s/%s (worktree=%v)",
				opts.Title, instance.GitHubOwner, instance.GitHubRepo, instance.IsWorktree)
		}
	}

	// Handle ResumeId - set up claudeSession so the --resume flag gets added on Start()
	if opts.ResumeId != "" {
		instance.claudeSession = &ClaudeSessionData{
			SessionID:    opts.ResumeId,
			LastAttached: t,
			Metadata: map[string]string{
				"resumed_from_history": "true",
			},
		}
		log.InfoLog.Printf("Instance '%s' configured to resume Claude conversation: %s", opts.Title, opts.ResumeId)
	}

	return instance, nil
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
		// Build the command with session resumption support if applicable
		// This enables conversation continuity when restarting Claude sessions
		commandBuilder := NewClaudeCommandBuilder(i.Program, i.claudeSession)
		enrichedProgram := commandBuilder.Build()

		// Create new tmux session with enriched command
		log.InfoLog.Printf("Creating new tmux session for instance '%s' with program '%s'", i.Title, enrichedProgram)
		// Use configurable prefix or default
		tmuxPrefix := i.TmuxPrefix
		if tmuxPrefix == "" {
			tmuxPrefix = "claudesquad_" // Default fallback
		}

		// Use server socket isolation if specified, otherwise use prefix-only isolation
		if i.TmuxServerSocket != "" {
			tmuxSession = tmux.NewTmuxSessionWithServerSocket(i.Title, enrichedProgram, tmuxPrefix, i.TmuxServerSocket)
		} else {
			tmuxSession = tmux.NewTmuxSessionWithPrefix(i.Title, enrichedProgram, tmuxPrefix)
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

	// Start ClaudeController for idle detection and automation
	// Only start controller for new sessions - loaded sessions get controllers started
	// by server.go after wiring dependencies (statusManager, reviewQueue)
	if firstTimeSetup {
		// This is non-critical - we log errors but don't fail the instance startup
		if err := i.StartController(); err != nil {
			log.WarningLog.Printf("Failed to start controller for instance '%s': %v", i.Title, err)
			// Continue - controller is optional functionality
		}
	} else {
		log.DebugLog.Printf("Skipping controller startup for loaded instance '%s' (will be started after wiring)", i.Title)
	}

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

	// Stop the controller first
	i.StopController()

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

// KillExternalSession terminates an external mux session by killing its tmux session.
// This only works for external sessions that were started via claude-mux with tmux integration.
// Returns an error if this is not an external instance or lacks tmux session name.
func (i *Instance) KillExternalSession() error {
	if i.InstanceType != InstanceTypeExternal {
		return fmt.Errorf("not an external instance")
	}
	if i.ExternalMetadata == nil || i.ExternalMetadata.TmuxSessionName == "" {
		return fmt.Errorf("no tmux session name available (session may not support destroy)")
	}

	// Stop the controller if running
	i.StopController()

	// Kill the tmux session
	cmd := exec.Command("tmux", "kill-session", "-t", i.ExternalMetadata.TmuxSessionName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to kill tmux session '%s': %w", i.ExternalMetadata.TmuxSessionName, err)
	}

	return nil
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

	content, err := i.tmuxSession.CapturePaneContent()
	if err != nil {
		return "", err
	}

	// REMOVED: i.UpdateTerminalTimestamps(content, false)
	// Timestamps are managed separately by WebSocket streaming and user interactions.
	// Preview() is now a true read-only operation that doesn't update timestamps,
	// preventing it from breaking acknowledgment snooze when the poller refreshes stale timestamps.
	// See session/review_queue_poller.go lines 383-408 for context.

	return content, nil
}

func (i *Instance) HasUpdated() (updated bool, hasPrompt bool) {
	if !i.started || i.Status == Paused {
		return false, false
	}

	// Check if the tmux session is still alive
	if !i.TmuxAlive() {
		return false, false
	}

	updated, hasPrompt = i.tmuxSession.HasUpdated()

	// Update timestamps when content has actually changed
	// This ensures LastMeaningfulOutput is updated even when no web UI client is connected,
	// preventing false "stale session" notifications in the review queue.
	if updated {
		// Capture content for timestamp update (forceUpdate=false to respect banner filtering)
		if content, err := i.tmuxSession.CapturePaneContent(); err == nil {
			i.UpdateTerminalTimestamps(content, false)
		}
	}

	return updated, hasPrompt
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

// HasGitWorktree returns true if the instance has a git worktree
func (i *Instance) HasGitWorktree() bool {
	return i.gitWorktree != nil
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

	// Stop the controller when pausing
	i.StopController()

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

	// Setup git worktree if this session has one
	var worktreePath string
	if i.gitWorktree != nil {
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

		worktreePath = i.gitWorktree.GetWorktreePath()
	} else {
		// No git worktree, use the original path
		worktreePath = i.Path
	}

	// Handle Claude Code session re-attachment if configured
	if err := i.handleClaudeSessionReattachment(); err != nil {
		log.WarningLog.Printf("Failed to re-attach to Claude Code session: %v", err)
		// Continue with resume - Claude session attachment is not critical for basic functionality
	}

	// Check if tmux session still exists from pause, otherwise create new one
	if i.tmuxSession.DoesSessionExist() {
		// Session exists, just restore PTY connection to it (retains stdout from before pause)
		if err := i.tmuxSession.RestoreWithWorkDir(worktreePath); err != nil {
			log.ErrorLog.Print(err)
			// If restore fails, fall back to creating new session
			if err := i.tmuxSession.Start(worktreePath); err != nil {
				log.ErrorLog.Print(err)
				// Cleanup git worktree if tmux session creation fails
				if i.gitWorktree != nil {
					if cleanupErr := i.gitWorktree.Cleanup(); cleanupErr != nil {
						err = fmt.Errorf("%v (cleanup error: %v)", err, cleanupErr)
						log.ErrorLog.Print(err)
					}
				}
				return fmt.Errorf("failed to start new session: %w", err)
			}
		}
	} else {
		// Create new tmux session
		if err := i.tmuxSession.Start(worktreePath); err != nil {
			log.ErrorLog.Print(err)
			// Cleanup git worktree if tmux session creation fails
			if i.gitWorktree != nil {
				if cleanupErr := i.gitWorktree.Cleanup(); cleanupErr != nil {
					err = fmt.Errorf("%v (cleanup error: %v)", err, cleanupErr)
					log.ErrorLog.Print(err)
				}
			}
			return fmt.Errorf("failed to start new session: %w", err)
		}
	}

	i.SetStatus(Running)

	// Start ClaudeController for idle detection and automation
	// This is non-critical - we log errors but don't fail the resume
	if err := i.StartController(); err != nil {
		log.WarningLog.Printf("Failed to start controller for instance '%s': %v", i.Title, err)
		// Continue - controller is optional functionality
	}

	return nil
}

// Rename changes the title of the instance.
// Per ADR-001, this only updates metadata - the tmux session name remains unchanged.
// Returns an error if the title is invalid (wrong length or contains invalid characters).
func (i *Instance) Rename(newTitle string) error {
	// Validate title length
	if len(newTitle) < MinTitleLength || len(newTitle) > MaxTitleLength {
		return ErrInvalidTitleLength
	}

	// Validate title characters
	if !isValidTitle(newTitle) {
		return ErrInvalidTitleChars
	}

	if newTitle == i.Title {
		// No change needed
		return nil
	}

	// Use mutex for thread safety
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()

	// Update the title
	oldTitle := i.Title
	i.Title = newTitle
	i.UpdatedAt = time.Now()

	log.InfoLog.Printf("Renamed session from '%s' to '%s'", oldTitle, newTitle)
	return nil
}

// Restart restarts the session by killing and recreating the tmux session.
// The git worktree is preserved during restart.
// If preserveOutput is true, captures terminal output before killing the session.
// For Claude sessions, uses --resume flag with the stored session ID.
func (i *Instance) Restart(preserveOutput bool) error {
	if !i.started {
		return ErrCannotRestart
	}

	if i.Status == Paused {
		return fmt.Errorf("%w: session is paused, resume it first", ErrCannotRestart)
	}

	// Capture terminal output if requested
	var savedOutput string
	if preserveOutput && i.tmuxSession != nil {
		output, err := i.tmuxSession.CapturePaneContentWithOptions("-", "-")
		if err != nil {
			log.WarningLog.Printf("Failed to capture terminal output before restart: %v", err)
		} else {
			savedOutput = output
		}
	}

	// Capture Claude session ID if available for resuming
	var claudeSessionID string
	if i.claudeSession != nil && i.claudeSession.SessionID != "" {
		claudeSessionID = i.claudeSession.SessionID
	}

	// Stop the controller
	i.StopController()

	// Kill the current tmux session
	if err := i.KillSession(); err != nil {
		return fmt.Errorf("failed to kill tmux session: %w", err)
	}

	// Determine the working directory
	var worktreePath string
	if i.gitWorktree != nil {
		worktreePath = i.gitWorktree.GetWorktreePath()
	} else if i.SessionType == SessionTypeExistingWorktree && i.ExistingWorktree != "" {
		worktreePath = i.ExistingWorktree
	} else {
		worktreePath = i.Path
	}

	// Build the program command with Claude resume flag if applicable
	program := i.Program
	if claudeSessionID != "" && strings.Contains(program, "claude") {
		// Add --resume flag for Claude sessions
		program = fmt.Sprintf("%s --resume %s", program, claudeSessionID)
	}

	// Add AutoYes flag if needed
	if i.AutoYes {
		program = program + " -y"
	}

	// Add initial prompt if provided and not already restarting with resume
	if i.Prompt != "" && claudeSessionID == "" {
		program = fmt.Sprintf("%s %q", program, i.Prompt)
	}

	// Create a new tmux session
	// Use configurable prefix or default
	tmuxPrefix := i.TmuxPrefix
	if tmuxPrefix == "" {
		tmuxPrefix = "claudesquad_" // Default fallback
	}

	// Use server socket isolation if specified, otherwise use prefix-only isolation
	if i.TmuxServerSocket != "" {
		i.tmuxSession = tmux.NewTmuxSessionWithServerSocket(i.Title, program, tmuxPrefix, i.TmuxServerSocket)
	} else {
		i.tmuxSession = tmux.NewTmuxSessionWithPrefix(i.Title, program, tmuxPrefix)
	}

	// Start the new session
	if err := i.tmuxSession.Start(worktreePath); err != nil {
		return fmt.Errorf("failed to start new tmux session: %w", err)
	}

	// If output was preserved and we have saved output, write it back
	if preserveOutput && savedOutput != "" {
		// Add a marker to indicate this is restored output
		marker := fmt.Sprintf("\n=== Session restarted at %s ===\n=== Previous output restored below ===\n\n",
			time.Now().Format(time.RFC3339))
		if _, err := i.tmuxSession.SendKeys(fmt.Sprintf("echo '%s'", marker)); err != nil {
			log.WarningLog.Printf("Failed to write restart marker: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
		if err := i.tmuxSession.TapEnter(); err != nil {
			log.WarningLog.Printf("Failed to send enter after marker: %v", err)
		}
	}

	// Restart the controller
	if err := i.StartController(); err != nil {
		log.WarningLog.Printf("Failed to restart controller for instance '%s': %v", i.Title, err)
		// Continue - controller is optional functionality
	}

	i.SetStatus(Running)
	i.UpdatedAt = time.Now()

	log.InfoLog.Printf("Successfully restarted session '%s'", i.Title)
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

	// Use the existing SetWindowSize method - now returns error
	if err := i.tmuxSession.SetWindowSize(cols, rows); err != nil {
		return fmt.Errorf("failed to resize terminal: %w", err)
	}
	return nil
}

// CapturePaneContent captures the current visible tmux pane content.
// This is a simple wrapper around TmuxSession.CapturePaneContent() for compatibility
// with the terminal WebSocket handlers.
func (i *Instance) CapturePaneContent() (string, error) {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()

	if !i.started || i.Status == Paused {
		return "", fmt.Errorf("session not started or paused")
	}

	if i.tmuxSession == nil {
		return "", fmt.Errorf("tmux session not initialized")
	}

	return i.tmuxSession.CapturePaneContent()
}

// CapturePaneContentRaw captures pane content with ANSI codes preserved (no line joining).
// Essential for hybrid streaming where cursor positioning codes must be preserved.
func (i *Instance) CapturePaneContentRaw() (string, error) {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()

	if !i.started || i.Status == Paused {
		return "", fmt.Errorf("session not started or paused")
	}

	if i.tmuxSession == nil {
		return "", fmt.Errorf("tmux session not initialized")
	}

	return i.tmuxSession.CapturePaneContentRaw()
}

// GetCurrentPaneContent captures the current visible tmux pane content.
// This is what the user would see if they attached to tmux directly.
// Unlike scrollback, this gives a clean snapshot of the current terminal state,
// which is ideal for applications that rewrite lines (progress bars, htop, etc.)
func (i *Instance) GetCurrentPaneContent(lines int) (string, error) {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()

	if i.tmuxSession == nil {
		return "", fmt.Errorf("tmux session not initialized")
	}

	// OPTIMIZED: Only capture visible viewport (not all scrollback)
	// This dramatically improves session switching performance by avoiding
	// transfer of potentially thousands of lines of scrollback history
	//
	// -p: print to stdout
	// -e: include escape sequences (colors, formatting)
	// -J: join wrapped lines
	// -S {lines}: start N lines from the bottom (viewport only)
	// -E -: end at current line (bottom of visible pane)

	// If lines is 0 or negative, capture current viewport height only
	if lines <= 0 {
		// Get actual pane dimensions to capture exactly what's visible
		_, height, err := i.tmuxSession.GetPaneDimensions()
		if err != nil {
			// Fallback to reasonable default if we can't get dimensions
			lines = 40
		} else {
			lines = height
		}
	}

	// Capture only the last N lines (viewport) instead of entire scrollback
	// Format: -S -{lines} means "start {lines} lines from the bottom"
	startLine := fmt.Sprintf("-%d", lines)
	content, err := i.tmuxSession.CapturePaneContentWithOptions(
		startLine,  // Start N lines from bottom (viewport only)
		"-",        // End at current line (bottom)
	)
	if err != nil {
		return "", fmt.Errorf("failed to capture current pane content: %w", err)
	}

	return content, nil
}

// GetPaneCursorPosition gets the current cursor position in the tmux pane.
// Returns cursor X (column) and Y (row) coordinates, both 0-based.
func (i *Instance) GetPaneCursorPosition() (x, y int, err error) {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()

	if i.tmuxSession == nil {
		return 0, 0, fmt.Errorf("tmux session not initialized")
	}

	return i.tmuxSession.GetCursorPosition()
}

// GetPaneDimensions gets the current dimensions of the tmux pane.
// Returns width (columns) and height (rows).
func (i *Instance) GetPaneDimensions() (width, height int, err error) {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()

	if i.tmuxSession == nil {
		return 0, 0, fmt.Errorf("tmux session not initialized")
	}

	return i.tmuxSession.GetPaneDimensions()
}

// GetScrollbackHistory captures scrollback history from tmux using line ranges.
// Uses tmux's native scrollback capabilities instead of stored sequences.
// startLine and endLine follow tmux conventions: negative numbers go back from current position,
// use "-" for the start/end of history.
func (i *Instance) GetScrollbackHistory(startLine, endLine string) (string, error) {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()

	if i.tmuxSession == nil {
		return "", fmt.Errorf("tmux session not initialized")
	}

	return i.tmuxSession.CapturePaneContentWithOptions(startLine, endLine)
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

	content, err := i.tmuxSession.CapturePaneContentWithOptions("-", "-")
	if err != nil {
		return "", err
	}

	// REMOVED: i.UpdateTerminalTimestamps(content, false)
	// Like Preview(), this is now a true read-only operation that doesn't update timestamps.
	// Timestamps are managed separately by WebSocket streaming and user interactions.
	// This prevents app startup from falsely updating all "Last Activity" timestamps.

	return content, nil
}

// GetTmuxSession returns the underlying tmux session for direct access.
// Returns nil if the session hasn't been started yet.
func (i *Instance) GetTmuxSession() *tmux.TmuxSession {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()
	return i.tmuxSession
}

// SetTmuxSession sets the tmux session for testing purposes
func (i *Instance) SetTmuxSession(session *tmux.TmuxSession) {
	i.tmuxSession = session
	i.started = session != nil
}

// SetWindowSize propagates window size changes to the tmux session
// This enables proper terminal resizing in environments like IntelliJ where SIGWINCH doesn't work
func (i *Instance) SetWindowSize(cols, rows int) error {
	if i.tmuxSession != nil {
		return i.tmuxSession.SetWindowSize(cols, rows)
	}
	return nil
}

// RefreshTmuxClient forces the tmux client to refresh, triggering a redraw
// of the process running inside. This is critical after resizing to ensure
// cursor positions and line wrapping are recalculated for the new dimensions.
func (i *Instance) RefreshTmuxClient() error {
	if i.tmuxSession != nil {
		return i.tmuxSession.RefreshClient()
	}
	return nil
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

// GetReviewQueue returns the review queue for this instance
func (i *Instance) GetReviewQueue() *ReviewQueue {
	return i.reviewQueue
}

// SetReviewQueue sets the review queue for this instance
func (i *Instance) SetReviewQueue(queue *ReviewQueue) {
	i.reviewQueue = queue
}

// NeedsReview returns true if this session is in the review queue
func (i *Instance) NeedsReview() bool {
	if i.reviewQueue == nil {
		return false
	}
	return i.reviewQueue.Has(i.Title)
}

// GetReviewItem returns the review item for this instance if it exists
func (i *Instance) GetReviewItem() (*ReviewItem, bool) {
	if i.reviewQueue == nil {
		return nil, false
	}
	return i.reviewQueue.Get(i.Title)
}

// SetStatusManager sets the status manager for idle detection
func (i *Instance) SetStatusManager(manager *InstanceStatusManager) {
	i.statusManager = manager
}

// GetStatusManager returns the status manager
func (i *Instance) GetStatusManager() *InstanceStatusManager {
	return i.statusManager
}

// StartController creates and starts a ClaudeController for this instance.
// The controller enables automated idle detection and queue management.
func (i *Instance) StartController() error {
	// Check preconditions under lock
	i.stateMutex.Lock()

	// Only start if we have a status manager
	if i.statusManager == nil {
		i.stateMutex.Unlock()
		log.DebugLog.Printf("No status manager set for instance %s, skipping controller", i.Title)
		return nil
	}

	// Don't create controller if instance isn't started
	if !i.started {
		i.stateMutex.Unlock()
		log.DebugLog.Printf("Instance %s not started yet, skipping controller", i.Title)
		return nil
	}

	// Don't recreate if already exists
	if i.claudeController != nil {
		i.stateMutex.Unlock()
		log.DebugLog.Printf("Controller already exists for instance %s", i.Title)
		return nil
	}

	// Release lock before creating/starting controller
	// This prevents deadlock when Start() calls GetPTYReader() which acquires read lock
	i.stateMutex.Unlock()

	// Create new controller (no lock needed - NewClaudeController doesn't access mutex-protected fields)
	controller, err := NewClaudeController(i)
	if err != nil {
		return fmt.Errorf("failed to create controller: %w", err)
	}

	// Start the controller - this initializes all components and begins background operations
	// Single call replaces the old Initialize() + Start() pattern
	if err := controller.Start(context.Background()); err != nil {
		return fmt.Errorf("failed to start controller: %w", err)
	}

	// Re-acquire lock to update instance state
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()

	// Double-check controller hasn't been set by another goroutine (defensive)
	if i.claudeController != nil {
		log.DebugLog.Printf("Controller already exists for instance %s (race detected)", i.Title)
		return nil
	}

	// Register with status manager and store controller
	i.statusManager.RegisterController(i.Title, controller)
	i.claudeController = controller

	log.InfoLog.Printf("Started ClaudeController for instance %s", i.Title)
	return nil
}

// StopController stops and cleans up the ClaudeController for this instance
func (i *Instance) StopController() {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()

	if i.claudeController == nil {
		return
	}

	// Unregister from status manager
	if i.statusManager != nil {
		i.statusManager.UnregisterController(i.Title)
	}

	// Stop the controller
	i.claudeController.Stop()
	i.claudeController = nil

	log.InfoLog.Printf("Stopped ClaudeController for instance %s", i.Title)
}

// GetController returns the ClaudeController if one exists
func (i *Instance) GetController() *ClaudeController {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()
	return i.claudeController
}

// GetPermissions returns the permissions for this instance based on its type
func (i *Instance) GetPermissions() InstancePermissions {
	if i.IsManaged {
		return GetManagedPermissions()
	}

	// External instance - permissions depend on discovery configuration
	// For now, we'll use a conservative default (view-only)
	// TODO: This should be configurable via PTYDiscoveryConfig
	return GetExternalPermissions(false)
}

// GetStatusIconForType returns the appropriate status icon based on instance type
func (i *Instance) GetStatusIconForType() string {
	if !i.IsManaged {
		return "👁" // Eye icon for external/view-only instances
	}

	// Managed instance - use standard status icons
	switch i.Status {
	case Running:
		return "●"
	case Ready:
		return "○"
	case Paused:
		return "⏸"
	case Loading:
		return "⏳"
	case NeedsApproval:
		return "❓"
	default:
		return "?"
	}
}

// UpdateTerminalTimestamps updates the terminal activity timestamps based on captured output.
// This method checks for meaningful content (excluding tmux banners) and updates timestamps accordingly.
// It should be called whenever terminal output is captured or processed.
// The forceUpdate parameter bypasses meaningful content checking for user-initiated interactions.
//
// Content signatures are used to prevent false "new activity" notifications on app restarts:
// - Computes SHA256 hash of terminal content
// - Compares with stored LastOutputSignature
// - Only updates LastMeaningfulOutput if content actually changed
func (i *Instance) UpdateTerminalTimestamps(content string, forceUpdate bool) {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()

	now := time.Now()

	// Always update LastTerminalUpdate for any output (even if just banners)
	if len(strings.TrimSpace(content)) > 0 {
		i.LastTerminalUpdate = now
	}

	// For user-initiated interactions (viewing/typing in web UI), always update LastMeaningfulOutput
	// This ensures "Last Activity" reflects actual user engagement, not just terminal output content
	if forceUpdate {
		// Filter out tmux status bar before computing signature to prevent false positives
		// from timestamp updates in the status line
		filteredContent := content
		if i.tmuxSession != nil {
			filteredContent, _ = i.tmuxSession.FilterBanners(content)
		}

		// Compute content signature to detect real changes vs restarts
		signature := computeContentSignature(filteredContent)

		// Only update timestamp if content has actually changed
		if signature != i.LastOutputSignature {
			i.LastMeaningfulOutput = now
			i.LastOutputSignature = signature
			log.LogForSession(i.Title, "debug", "Updated LastMeaningfulOutput timestamp (user interaction - content changed)")
		} else {
			log.LogForSession(i.Title, "debug", "Skipped LastMeaningfulOutput update (user interaction but content unchanged since last update)")
		}
		return
	}

	// Check if the output contains meaningful content (not just tmux banners)
	// This path is for automated checks where banner filtering makes sense
	if i.tmuxSession != nil {
		hasMeaningful := i.tmuxSession.HasMeaningfulContent(content)
		log.LogForSession(i.Title, "debug", "HasMeaningfulContent=%v for %d bytes: %q", hasMeaningful, len(content), content)
		if hasMeaningful {
			// Filter out tmux status bar before computing signature to prevent false positives
			// from timestamp updates in the status line
			filteredContent, _ := i.tmuxSession.FilterBanners(content)

			// Compute content signature to detect real changes vs restarts
			signature := computeContentSignature(filteredContent)

			// Only update timestamp if content has actually changed
			if signature != i.LastOutputSignature {
				i.LastMeaningfulOutput = now
				i.LastOutputSignature = signature
				log.LogForSession(i.Title, "debug", "Updated LastMeaningfulOutput timestamp (detected non-banner output, content changed)")
			} else {
				log.LogForSession(i.Title, "debug", "Skipped LastMeaningfulOutput update (non-banner output but content unchanged since last update)")
			}
		} else {
			log.LogForSession(i.Title, "debug", "NOT updating LastMeaningfulOutput - content classified as non-meaningful (banners only)")
		}
	}
}

// GetTimeSinceLastMeaningfulOutput returns the duration since the last meaningful terminal output.
// Falls back to time since creation if no meaningful output has been recorded.
func (i *Instance) GetTimeSinceLastMeaningfulOutput() time.Duration {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()
	return i.ReviewState.TimeSinceLastMeaningfulOutput(i.CreatedAt)
}

// GetTimeSinceLastTerminalUpdate returns the duration since any terminal activity.
// Falls back to time since creation if no terminal output has been recorded.
func (i *Instance) GetTimeSinceLastTerminalUpdate() time.Duration {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()
	return i.ReviewState.TimeSinceLastTerminalUpdate(i.CreatedAt)
}

// computeContentSignature computes a MurmurHash3 64-bit hash of terminal content.
// This signature is used to detect actual content changes vs app restarts with unchanged content.
// MurmurHash3 is significantly faster than SHA256 and perfect for non-cryptographic checksums.
// Returns a hex-encoded string representation of the hash (16 characters for 64-bit hash).
func computeContentSignature(content string) string {
	hash := murmur3.Sum64([]byte(content))
	return fmt.Sprintf("%016x", hash)
}

// Tag management methods

// AddTag adds a tag to the instance if it doesn't already exist
func (i *Instance) AddTag(tag string) {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	
	// Check if tag already exists
	for _, existingTag := range i.Tags {
		if existingTag == tag {
			return // Tag already exists
		}
	}
	
	i.Tags = append(i.Tags, tag)
}

// RemoveTag removes a tag from the instance
func (i *Instance) RemoveTag(tag string) {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	
	newTags := make([]string, 0, len(i.Tags))
	for _, existingTag := range i.Tags {
		if existingTag != tag {
			newTags = append(newTags, existingTag)
		}
	}
	
	i.Tags = newTags
}

// HasTag returns true if the instance has the specified tag
func (i *Instance) HasTag(tag string) bool {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()
	
	for _, existingTag := range i.Tags {
		if existingTag == tag {
			return true
		}
	}
	
	return false
}

// GetTags returns a copy of the instance's tags
func (i *Instance) GetTags() []string {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()
	
	tags := make([]string, len(i.Tags))
	copy(tags, i.Tags)
	return tags
}

// SetTags replaces all tags with a new set
func (i *Instance) SetTags(tags []string) {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()

	i.Tags = make([]string, len(tags))
	copy(i.Tags, tags)
}

// GitHub integration helper methods

// IsPRSession returns true if this session was created from a GitHub PR URL
func (i *Instance) IsPRSession() bool {
	return i.GitHubPRNumber > 0
}

// GetGitHubRepoFullName returns "owner/repo" format, or empty string if not a GitHub session
func (i *Instance) GetGitHubRepoFullName() string {
	if i.GitHubOwner == "" || i.GitHubRepo == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s", i.GitHubOwner, i.GitHubRepo)
}

// GetPRDisplayInfo returns a human-readable string describing the PR for display in UI
// Returns empty string if not a PR session
func (i *Instance) GetPRDisplayInfo() string {
	if !i.IsPRSession() {
		return ""
	}
	return fmt.Sprintf("PR #%d on %s", i.GitHubPRNumber, i.GetGitHubRepoFullName())
}

// IsGitHubSession returns true if this session has any GitHub metadata
func (i *Instance) IsGitHubSession() bool {
	return i.GitHubOwner != "" && i.GitHubRepo != ""
}

// DetectAndPopulateWorktreeInfo detects if the instance path is a worktree
// and populates the IsWorktree, MainRepoPath, GitHubOwner, and GitHubRepo fields.
// This is useful for sessions created from existing worktrees where we want to
// display the actual repository information in the UI.
//
// IMPORTANT: For sessions with git worktrees, we check BOTH paths:
// 1. The worktree path (gitWorktree.GetWorktreePath()) - to detect IsWorktree and MainRepoPath
// 2. The original path (i.Path) - as fallback for GitHub owner/repo if worktree detection fails
//
// This is necessary because:
// - i.Path is the main repository path (e.g., ~/Documents/personal-wiki)
// - gitWorktree.GetWorktreePath() is the actual worktree (e.g., ~/.claude-squad/worktrees/...)
// - The main repo has .git as a directory; the worktree has .git as a file pointing to the main repo
func (i *Instance) DetectAndPopulateWorktreeInfo() error {
	// Determine the path to use for detection
	// For worktree sessions, use the worktree path; otherwise use i.Path
	detectPath := i.Path
	if i.gitWorktree != nil {
		worktreePath := i.gitWorktree.GetWorktreePath()
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

// detectAndTrackPrompt detects if current state is a prompt and tracks it for re-add prevention.
// Returns true if this is a NEW prompt (different from the last detected prompt).
func (i *Instance) detectAndTrackPrompt(content string, statusInfo InstanceStatusInfo) bool {
	// Detect if current state is a prompt requiring user input
	isPromptState := statusInfo.ClaudeStatus == StatusNeedsApproval ||
		statusInfo.ClaudeStatus == StatusInputRequired

	if !isPromptState {
		return false
	}

	// Compute signature of prompt content (last 10 lines for identity)
	promptSignature := i.computePromptSignature(content)

	// Check if this is a NEW prompt (different signature OR first time)
	isNewPrompt := promptSignature != i.LastPromptSignature ||
		i.LastPromptSignature == ""

	if isNewPrompt {
		i.LastPromptDetected = time.Now()
		i.LastPromptSignature = promptSignature
		log.InfoLog.Printf("[Prompt] New prompt detected for '%s': signature=%s...",
			i.Title, truncateString(promptSignature, 8))
	}

	return isNewPrompt
}

// computePromptSignature computes a hash of the prompt content for identity comparison.
// Uses the last 10 lines to capture the prompt context without earlier output.
func (i *Instance) computePromptSignature(content string) string {
	if content == "" {
		return ""
	}

	// Use last 10 lines for identity (captures prompt without earlier output)
	lines := strings.Split(content, "\n")
	const contextLines = 10
	startIdx := len(lines) - contextLines
	if startIdx < 0 {
		startIdx = 0
	}

	promptContext := strings.Join(lines[startIdx:], "\n")

	// Compute hash using murmur3 (already imported, same as content signatures)
	hash := murmur3.Sum64([]byte(promptContext))
	return fmt.Sprintf("%016x", hash)
}

// truncateString truncates a string to maxLen characters.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
