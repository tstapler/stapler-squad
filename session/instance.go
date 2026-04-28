package session

import (
	"bufio"
	"context"
	"fmt"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/scrollback"
	"github.com/tstapler/stapler-squad/session/tmux"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/atotto/clipboard"
	"github.com/google/uuid"
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
	// Creating is the status when the instance is being initialized.
	Creating
	// Stopped is a terminal state: the instance has been shut down and cannot transition further.
	Stopped
)

// String returns a human-readable name for the status.
func (s Status) String() string {
	switch s {
	case Running:
		return "Running"
	case Ready:
		return "Ready"
	case Loading:
		return "Loading"
	case Paused:
		return "Paused"
	case NeedsApproval:
		return "NeedsApproval"
	case Creating:
		return "Creating"
	case Stopped:
		return "Stopped"
	default:
		return fmt.Sprintf("Status(%d)", int(s))
	}
}

// LifecycleEvent is a notification type emitted by an Instance when key state
// transitions occur (e.g., the session starts, or the program exits unexpectedly).
type LifecycleEvent int

const (
	// EventStarted fires at the end of start() when the instance has successfully
	// transitioned to Running and the controller is up.
	EventStarted LifecycleEvent = iota
	// EventExited fires when the underlying program exits unexpectedly (not via an
	// operator-initiated Kill/Stop). Callers may use this to drive auto-restart logic.
	EventExited
)

// LifecycleListener is implemented by any component that wants to receive Instance
// lifecycle notifications. Implementations must be non-blocking; use a goroutine
// or channel if the handler needs to do significant work.
type LifecycleListener interface {
	OnLifecycleEvent(event LifecycleEvent, reason string)
}

// ==== Instance -- Core Fields and Construction ====

// Instance is a running instance of claude code.
type Instance struct {
	// ID is the stable, immutable identifier for this instance.
	// Set once at creation; never changes even if Title is renamed.
	// Falls back to Title when empty for backward compatibility.
	ID string
	// Title is the title of the instance.
	Title string
	// UUID is a stable unique identifier for this instance, generated at creation time.
	// Unlike Title, UUID does not change when the session is renamed.
	UUID string
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
	// AutonomousMode enables autonomous Earpiece mode (crew autonomy).
	// When true, the Fixer will inject correction prompts without user confirmation.
	// When false (default), the session runs in supervised mode.
	AutonomousMode bool `json:"autonomous_mode,omitempty"`

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
	// GitHubIsFork is true when the remote repo is a fork (PR lookup uses upstream)
	GitHubIsFork bool `json:"github_is_fork,omitempty"`

	// PR status fields — populated by PRStatusPoller; not set on session creation
	// GitHubPRState is the PR lifecycle state: "open", "closed", "merged"
	GitHubPRState string `json:"github_pr_state,omitempty"`
	// GitHubPRIsDraft is true when the PR is in draft mode
	GitHubPRIsDraft bool `json:"github_pr_is_draft,omitempty"`
	// GitHubPRPriority is the derived priority: blocking/ready/pending/draft/complete/no_pr
	GitHubPRPriority string `json:"github_pr_priority,omitempty"`
	// GitHubApprovedCount is the count of current non-dismissed APPROVED reviews
	GitHubApprovedCount int `json:"github_approved_count,omitempty"`
	// GitHubChangesReqCount is the count of current non-dismissed CHANGES_REQUESTED reviews
	GitHubChangesReqCount int `json:"github_changes_req_count,omitempty"`
	// GitHubCheckConclusion is the CI rollup: success/failure/pending/action_required/neutral/""
	GitHubCheckConclusion string `json:"github_check_conclusion,omitempty"`
	// GitHubPRStatusTerminal is true when the PR is merged/closed and polling should stop
	GitHubPRStatusTerminal bool `json:"github_pr_status_terminal,omitempty"`
	// LastPRStatusCheck is when the PR status was last successfully fetched
	LastPRStatusCheck time.Time `json:"last_pr_status_check,omitempty"`

	Checkpoints      CheckpointList
	ActiveCheckpoint string
	ForkedFromID     string

	// OneShot runs claude in -p mode; the session exits after the task completes.
	OneShot bool

	// ProjectID is the optional project this session belongs to.
	ProjectID string

	// HistoryFilePath is the path to the Claude conversation JSONL history file.
	// Set by HistoryLinker when it correlates this session to an open JSONL file.
	HistoryFilePath string

	// MCPServerURL is the URL of the stapler-squad HTTP MCP endpoint.
	// When set, passed as --mcp-config to claude on session start so no
	// settings-file injection is needed.
	MCPServerURL string `json:"mcp_server_url,omitempty"`

	// LaunchCommand is the full command passed to tmux on session start, including
	// any injected flags (--resume, --mcp-config, -y, initial prompt). Set once on
	// first start and updated on restart. Empty for external (mux-discovered) sessions.
	LaunchCommand string `json:"launch_command,omitempty"`

	// historyDetector is used by tryExtractConversationUUID. When nil the
	// production inspector is used. Set in tests to inject a fake home dir.
	historyDetector *HistoryFileDetector

	// Claude Code session information for persistence and re-attachment
	claudeSession *ClaudeSessionData

	// Review queue integration for tracking sessions needing attention
	reviewQueue *ReviewQueue

	// ReviewState holds all review queue and terminal activity timestamps.
	// Fields are embedded (promoted) so external code can still access inst.LastViewed etc.
	// Protected by stateMutex.
	ReviewState

	// controllerManager owns the ClaudeController and InstanceStatusManager references.
	controllerManager ControllerManager

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
	// tmuxManager owns the tmux session and preview-size tracking state.
	tmuxManager TmuxProcessManager
	// gitManager owns the git worktree and diff stats.
	gitManager GitWorktreeManager

	// tagManager provides CRUD operations for session tags.
	// Backed by a pointer to Instance.Tags for zero-sync compatibility with
	// callers that read inst.Tags directly.
	tagManager TagManager

	// Mutex to protect concurrent access to instance state
	stateMutex sync.RWMutex
	// startMu prevents concurrent calls to start() from racing during session setup.
	// Held for the full duration of start(); callers that lose the race return early.
	startMu sync.Mutex

	// lifecycleListeners receives EventStarted / EventExited notifications.
	lifecycleListeners   []LifecycleListener
	lifecycleListenersMu sync.Mutex
}

// ToInstanceData converts an Instance to its serializable form
func (i *Instance) ToInstanceData() InstanceData {
	data := InstanceData{
		Title:                i.Title,
		UUID:                 i.UUID,
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
		// GitHub integration fields
		GitHubIsFork: i.GitHubIsFork,
		// PR status fields (populated by PRStatusPoller)
		GitHubPRState:          i.GitHubPRState,
		GitHubPRIsDraft:        i.GitHubPRIsDraft,
		GitHubPRPriority:       i.GitHubPRPriority,
		GitHubApprovedCount:    i.GitHubApprovedCount,
		GitHubChangesReqCount:  i.GitHubChangesReqCount,
		GitHubCheckConclusion:  i.GitHubCheckConclusion,
		GitHubPRStatusTerminal: i.GitHubPRStatusTerminal,
		LastPRStatusCheck:      i.LastPRStatusCheck,
		// Crew autonomy mode
		AutonomousMode: i.AutonomousMode,
		// Checkpoint metadata
		Checkpoints:      i.Checkpoints,
		ActiveCheckpoint: i.ActiveCheckpoint,
		ForkedFromID:     i.ForkedFromID,
		// History file linkage
		HistoryFilePath: i.HistoryFilePath,
		// One-shot mode
		OneShot: i.OneShot,
		// Project association
		ProjectID: i.ProjectID,
		// Full launch command for diagnostics
		LaunchCommand: i.LaunchCommand,
		// MCP server URL for re-injection on restart
		MCPServerURL: i.MCPServerURL,
	}

	// Only include worktree data if gitWorktree is initialized
	if i.gitManager.HasWorktree() {
		data.Worktree = GitWorktreeData{
			RepoPath:      i.gitManager.GetRepoPath(),
			WorktreePath:  i.gitManager.GetWorktreePath(),
			SessionName:   i.Title,
			BranchName:    i.gitManager.GetBranchName(),
			BaseCommitSHA: i.gitManager.GetBaseCommitSHA(),
		}
	}

	// Only include diff stats if they exist
	if i.gitManager.diffStats != nil {
		data.DiffStats = DiffStatsData{
			Added:   i.gitManager.diffStats.Added,
			Removed: i.gitManager.diffStats.Removed,
			Content: i.gitManager.diffStats.Content,
		}
	}

	// Include Claude session data if it exists
	if i.claudeSession != nil {
		data.ClaudeSession = *i.claudeSession
	}
	// Always wire the squad session ID from Instance.UUID so the API response
	// always carries both identifiers in the ClaudeSession sub-object.
	data.ClaudeSession.SquadSessionID = i.UUID

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
		Title:       data.Title,
		UUID:        data.UUID,
		Path:        migratedPath, // Use migrated path
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
		Tags:        tags, // Use migrated tags (includes category if needed)
		SessionType: data.SessionType,
		TmuxPrefix:  data.TmuxPrefix,
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
		InstanceType:     InstanceTypeManaged, // Restored instances are always managed
		IsManaged:        true,
		ExternalMetadata: nil,                     // External instances are not persisted
		Permissions:      GetManagedPermissions(), // Full permissions for managed instances
		// GitHub integration fields
		GitHubPRNumber:  data.GitHubPRNumber,
		GitHubPRURL:     data.GitHubPRURL,
		GitHubOwner:     data.GitHubOwner,
		GitHubRepo:      data.GitHubRepo,
		GitHubSourceRef: data.GitHubSourceRef,
		ClonedRepoPath:  data.ClonedRepoPath,
		GitHubIsFork:    data.GitHubIsFork,
		// PR status fields (populated by PRStatusPoller)
		GitHubPRState:          data.GitHubPRState,
		GitHubPRIsDraft:        data.GitHubPRIsDraft,
		GitHubPRPriority:       data.GitHubPRPriority,
		GitHubApprovedCount:    data.GitHubApprovedCount,
		GitHubChangesReqCount:  data.GitHubChangesReqCount,
		GitHubCheckConclusion:  data.GitHubCheckConclusion,
		GitHubPRStatusTerminal: data.GitHubPRStatusTerminal,
		LastPRStatusCheck:      data.LastPRStatusCheck,
		// Worktree detection fields
		MainRepoPath: data.MainRepoPath,
		IsWorktree:   data.IsWorktree,
		// Crew autonomy mode
		AutonomousMode: data.AutonomousMode,
		// Checkpoint metadata
		Checkpoints:      data.Checkpoints,
		ActiveCheckpoint: data.ActiveCheckpoint,
		ForkedFromID:     data.ForkedFromID,
		// History file linkage
		HistoryFilePath: data.HistoryFilePath,
		// One-shot mode
		OneShot: data.OneShot,
		// Project association
		ProjectID: data.ProjectID,
		// Launch command for diagnostics
		LaunchCommand: data.LaunchCommand,
		// MCP server URL for re-injection on restart
		MCPServerURL: data.MCPServerURL,
	}

	// MIGRATION: Assign UUID to existing sessions that pre-date UUID assignment
	if instance.UUID == "" {
		instance.UUID = uuid.New().String()
	}

	// Initialize TagManager backed by the Instance.Tags slice
	instance.tagManager = NewTagManager(&instance.Tags)

	// Restore git worktree and diff stats via manager (cannot use struct literal for sub-manager fields).
	instance.gitManager.SetWorktree(git.NewGitWorktreeFromStorage(
		data.Worktree.RepoPath,
		data.Worktree.WorktreePath,
		data.Worktree.SessionName,
		data.Worktree.BranchName,
		data.Worktree.BaseCommitSHA,
	))
	instance.gitManager.SetDiffStats(&git.DiffStats{
		Added:   data.DiffStats.Added,
		Removed: data.DiffStats.Removed,
		Content: data.DiffStats.Content,
	})

	// Restore Claude session data if it exists
	if data.ClaudeSession.ConversationUUID != "" {
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

	// Check if the worktree still exists on disk if the instance is not paused.
	// No mutex is needed here because the instance is not yet shared.
	if !instance.Paused() && instance.gitManager.HasWorktree() {
		worktreePath := instance.gitManager.GetWorktreePath()
		if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
			// Worktree has been deleted — use transitionTo so the state machine is respected.
			// Ready → Paused and Loading → Paused are explicitly allowed for this case.
			log.ForSession(instance.Title).Warning("Worktree directory doesn't exist at '%s', marking as paused", worktreePath)
			if err := instance.transitionTo(Paused); err != nil {
				// If the transition is somehow invalid (e.g. already Stopped), fall back to setStatus.
				log.ForSession(instance.Title).Warning("Could not transition to Paused via state machine (%v), using setStatus", err)
				instance.setStatus(Paused)
			}
		}
	}

	if instance.Paused() || instance.Status == Stopped {
		instance.started = true
		// Use configurable prefix or default
		tmuxPrefix := instance.TmuxPrefix
		if tmuxPrefix == "" {
			tmuxPrefix = "staplersquad_" // Default fallback
		}

		// Use server socket isolation if specified, otherwise use prefix-only isolation.
		// WithRegistry(nil) prevents a background reconnect loop on isolated sockets —
		// the loop tries attach-session on a keepalive that doesn't exist there, causing
		// intermittent exit status 1 from concurrent new-session calls.
		if instance.TmuxServerSocket != "" {
			instance.tmuxManager.SetSession(tmux.NewTmuxSessionWithServerSocket(instance.Title, instance.Program, tmuxPrefix, instance.TmuxServerSocket, tmux.WithRegistry(nil)))
		} else {
			instance.tmuxManager.SetSession(tmux.NewTmuxSessionWithPrefix(instance.Title, instance.Program, tmuxPrefix))
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

// IsValid reports whether st is a recognized session type.
func (st SessionType) IsValid() bool {
	switch st {
	case SessionTypeDirectory, SessionTypeNewWorktree, SessionTypeExistingWorktree:
		return true
	default:
		return false
	}
}

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
	// TmuxPrefix is the prefix to use for tmux session names (e.g., "staplersquad_")
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

	// OneShot runs claude in -p mode; the session exits after the task completes.
	OneShot bool

	// ProjectID associates the session with a project.
	ProjectID string

	// MCPServerURL, when non-empty and the program is claude, passes
	// --mcp-config '{"stapler-squad":{"type":"http","url":"<MCPServerURL>"}}' so the
	// session can call back into stapler-squad without any file injection.
	MCPServerURL string
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
	if !sessionType.IsValid() {
		return nil, fmt.Errorf("invalid session type %q: must be one of %q, %q, %q",
			sessionType, SessionTypeDirectory, SessionTypeNewWorktree, SessionTypeExistingWorktree)
	}

	instance := &Instance{
		Title:            opts.Title,
		UUID:             uuid.New().String(),
		Status:           Ready,
		Path:             absPath,
		Branch:           opts.Branch,
		Program:          opts.Program,
		Height:           0,
		Width:            0,
		CreatedAt:        t,
		UpdatedAt:        t,
		AutoYes:          opts.AutoYes,
		Prompt:           opts.Prompt,
		ExistingWorktree: opts.ExistingWorktree,
		Category:         opts.Category,
		Tags:             opts.Tags, // Set tags from options
		SessionType:      sessionType,
		TmuxPrefix:       opts.TmuxPrefix,
		TmuxServerSocket: opts.TmuxServerSocket,
		IsExpanded:       true, // Default to expanded for newly created instances
		InstanceType:     InstanceTypeManaged,
		IsManaged:        true,
		ExternalMetadata: nil,                     // Only set for external instances
		Permissions:      GetManagedPermissions(), // Full permissions for managed instances
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
		// One-shot mode and project
		OneShot:      opts.OneShot,
		ProjectID:    opts.ProjectID,
		MCPServerURL: opts.MCPServerURL,
	}

	// Initialize TagManager backed by the Instance.Tags slice
	instance.tagManager = NewTagManager(&instance.Tags)

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
			ConversationUUID: opts.ResumeId,
			LastAttached:     t,
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
	if !i.gitManager.HasWorktree() {
		return "", fmt.Errorf("gitWorktree is nil")
	}
	return i.gitManager.GetRepoName(), nil
}

// MarkViewed records that the user has viewed this session.
func (i *Instance) MarkViewed() {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	i.LastViewed = time.Now()
}

// MarkUserResponded records that the user has responded to this session.
// Returns the timestamp that was set so callers can persist it without a second lock acquisition.
func (i *Instance) MarkUserResponded() time.Time {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	i.LastUserResponse = time.Now()
	return i.LastUserResponse
}

// MarkAcknowledged records that the user has acknowledged (dismissed) this session from the review queue.
func (i *Instance) MarkAcknowledged() {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	i.LastAcknowledged = time.Now()
}

// MarkNeedsApproval transitions a Running instance to NeedsApproval.
// Called by the review queue poller when terminal output indicates the session is awaiting approval.
func (i *Instance) MarkNeedsApproval() error {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	return i.transitionTo(NeedsApproval)
}

// LastMeaningfulOutputTime returns the time of the last meaningful terminal output.
func (i *Instance) LastMeaningfulOutputTime() time.Time {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()
	return i.LastMeaningfulOutput
}

// SetLastMeaningfulOutput sets the time of the last meaningful terminal output.
func (i *Instance) SetLastMeaningfulOutput(t time.Time) {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	i.LastMeaningfulOutput = t
}

// GetCreatedAt returns the time this instance was created. The field is immutable after creation.
func (i *Instance) GetCreatedAt() time.Time {
	return i.CreatedAt
}

// GetTitle returns the session title/name.
func (i *Instance) GetTitle() string {
	return i.Title
}

// GetStableID returns a stable identifier for this instance.
// If UUID is set, returns it. Falls back to Title for backward compatibility
// with sessions that pre-date UUID assignment.
func (i *Instance) GetStableID() string {
	if i.UUID != "" {
		return i.UUID
	}
	return i.Title
}

// MatchesID reports whether id refers to this instance.
// Accepts both the stable UUID and the legacy Title identifier so that all
// service lookups work correctly regardless of which form the caller has.
func (i *Instance) MatchesID(id string) bool {
	return i.Title == id || i.GetStableID() == id
}

// GetTmuxSessionName returns the sanitized tmux session name for reconciliation.
// Returns empty string for external or uninitialized sessions.
func (i *Instance) GetTmuxSessionName() string {
	return i.tmuxManager.GetTmuxSessionName()
}

// setStatus sets the instance status without locking.
// Must be called with i.stateMutex held.
func (i *Instance) setStatus(status Status) {
	i.Status = status
}

// transitionTo validates and executes a state transition using the state machine.
// Must be called with i.stateMutex held.
func (i *Instance) transitionTo(s Status) error {
	if !CanTransition(i.Status, s) {
		return ErrInvalidTransition{From: i.Status, To: s}
	}
	i.setStatus(s)
	return nil
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

// ==== Lifecycle Methods ====
// Start, Pause, Resume, Kill, Destroy, Restart and their internal helpers.
// These coordinate across sub-managers (tmuxManager, gitManager, controllerManager).

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

// start is the internal implementation for Start and StartWithCleanup.
func (i *Instance) start(firstTimeSetup bool, setupCleanup bool, cleanup *tmux.CleanupFunc) error {
	// Serialize concurrent start() calls for the same instance. A concurrent call
	// (e.g. from onExit callback triggering a restart while another goroutine is
	// already in start()) will block here until the first call finishes.
	i.startMu.Lock()
	defer i.startMu.Unlock()

	log.InfoLog.Printf("Starting instance '%s' (firstTimeSetup: %v)", i.Title, firstTimeSetup)

	if i.Title == "" {
		return fmt.Errorf("instance title cannot be empty")
	}

	i.initTmuxSession()

	// Wire the exit callback so control-mode %exit / PTY EOF fires our handler.
	// ResetExitOnce is called first so repeated start() calls (restarts) allow
	// the callback to fire again after the sync.Once was exhausted in the prior run.
	i.tmuxManager.ResetExitOnce()
	i.tmuxManager.SetOnExitCallback(func(reason string) {
		log.InfoLog.Printf("Instance '%s': unexpected exit detected via control mode (%s)", i.Title, reason)
		log.ForSession(i.Title).Info("Session exited unexpectedly (reason: %s)", reason)
		i.stateMutex.Lock()
		if i.Status == Running || i.Status == Ready {
			if err := i.transitionTo(Stopped); err != nil {
				log.WarningLog.Printf("Instance '%s': exit callback transition failed: %v", i.Title, err)
			}
		}
		i.stateMutex.Unlock()
		i.fireLifecycleEvent(EventExited, reason)
	})

	if firstTimeSetup {
		if err := i.setupFirstTimeWorktree(); err != nil {
			return err
		}
	}

	// Cleanup on error: kill session and invalidate the caller's cleanup handle.
	var setupErr error
	defer func() {
		if setupErr != nil {
			if cleanupErr := i.Kill(); cleanupErr != nil {
				setupErr = fmt.Errorf("%v (cleanup error: %v)", setupErr, cleanupErr)
			}
			if setupCleanup && cleanup != nil {
				*cleanup = func() error { return nil }
			}
		}
	}()

	if !firstTimeSetup {
		if !i.tmuxManager.DoesSessionExist() {
			// tmux session is dead (machine reboot, tmux kill-server, etc.)
			startPath := i.resolveStartPath(i.GetEffectiveRootDir())
			if i.HasClaudeSession() {
				// Cold restore: we have a conversation UUID — relaunch with --resume.
				// initTmuxSession() (called above) already built the program command
				// with --resume via ClaudeCommandBuilder, so Start() uses it directly.
				log.InfoLog.Printf("Cold restoring '%s' with --resume %s in %s",
					i.Title, i.claudeSession.ConversationUUID, startPath)
			} else {
				// Dead tmux, no UUID — start a fresh session without --resume.
				log.WarningLog.Printf("Cold start '%s': tmux dead, no conversation UUID, starting fresh in %s",
					i.Title, startPath)
			}
			if err := i.tmuxManager.Start(startPath); err != nil {
				setupErr = fmt.Errorf("cold restore Start failed for '%s': %w", i.Title, err)
				return setupErr
			}
			// Attach PTY — same pattern as firstTimeSetup path (lines 867-870).
			_ = i.tmuxManager.RestoreWithWorkDir(startPath)
			if _, ptyErr := i.tmuxManager.GetPTY(); ptyErr != nil {
				log.ErrorLog.Printf("Cold-restored session '%s': PTY attach failed (%v) — controller and SendKeys will be unavailable", i.Title, ptyErr)
			}
			// Clear the stored session ID so HistoryLinker re-detects the actual
			// UUID from the running process's open files. The --resume flag was
			// already embedded in the program command by initTmuxSession() above;
			// Claude may resume the same session or create a new one if the old
			// UUID is no longer valid. Either way, proc inspection is the source
			// of truth.
			if i.claudeSession != nil {
				i.claudeSession.ConversationUUID = ""
				i.HistoryFilePath = ""
			}
		} else {
			// Hot restore: tmux session is alive — attach to it.
			workDir := i.Path
			if i.gitManager.HasWorktree() {
				workDir = i.gitManager.GetWorktreePath()
			}
			log.InfoLog.Printf("Restoring existing tmux session for instance '%s' with workDir '%s'", i.Title, workDir)
			if err := i.tmuxManager.RestoreWithWorkDir(workDir); err != nil {
				setupErr = fmt.Errorf("failed to restore existing session: %w", err)
				return setupErr
			}
			log.InfoLog.Printf("Successfully restored tmux session for instance '%s'", i.Title)
		}
	} else {
		basePath := i.Path
		if i.gitManager.HasWorktree() {
			log.InfoLog.Printf("Setting up git worktree for instance '%s'", i.Title)
			if err := i.gitManager.Setup(); err != nil {
				log.ForSession(i.Title).Error("Failed to setup git worktree: %v", err)
				setupErr = fmt.Errorf("failed to setup git worktree: %w", err)
				return setupErr
			}
			basePath = i.gitManager.GetWorktreePath()
		}
		startPath := i.resolveStartPath(basePath)
		if err := i.tmuxManager.Start(startPath); err != nil {
			if i.gitManager.HasWorktree() {
				if cleanupErr := i.gitManager.Cleanup(); cleanupErr != nil {
					err = fmt.Errorf("%v (cleanup error: %v)", err, cleanupErr)
				}
			}
			setupErr = fmt.Errorf("failed to start new session: %w", err)
			return setupErr
		}
		// Establish PTY connection after creating the tmux session.
		// tmuxManager.Start() creates the detached session but does not attach a PTY.
		// RestoreWithWorkDir finds the existing session and attaches via attach-session,
		// setting t.ptmx so StartController() can call GetPTYReader() successfully.
		// Note: RestoreWithWorkDir always returns nil even on PTY failure; check GetPTY() to confirm.
		_ = i.tmuxManager.RestoreWithWorkDir(startPath)
		if _, ptyErr := i.tmuxManager.GetPTY(); ptyErr != nil {
			log.ErrorLog.Printf("New session '%s': PTY attach failed after retries (%v) — controller and SendKeys will be unavailable", i.Title, ptyErr)
		}
	}

	i.stateMutex.Lock()
	// Only transition if not already Running (e.g., recovery/restart after KillSession
	// preserves the Running status).
	if i.Status != Running {
		if err := i.transitionTo(Running); err != nil {
			i.stateMutex.Unlock()
			setupErr = fmt.Errorf("failed to transition to Running: %w", err)
			return setupErr
		}
	}
	i.stateMutex.Unlock()
	i.started = true
	i.fireLifecycleEvent(EventStarted, "")
	log.ForSession(i.Title).Info("Session started (firstTimeSetup: %v)", firstTimeSetup)

	// Start controller for new sessions only; loaded sessions are wired later by server.go.
	if firstTimeSetup {
		if err := i.StartController(); err != nil {
			// One retry: brief delay gives the tmux session time to stabilise, then
			// re-attempt PTY attachment (RestoreWithWorkDir is idempotent — skips
			// recreation if the session exists, only re-attaches PTY if ptmx is nil).
			log.WarningLog.Printf("Controller start failed for '%s': %v — retrying after PTY re-attach", i.Title, err)
			time.Sleep(200 * time.Millisecond)
			// Session already exists; workDir only matters for the fallback recreation path.
			_ = i.tmuxManager.RestoreWithWorkDir("")
			if retryErr := i.StartController(); retryErr != nil {
				log.ErrorLog.Printf("Controller start failed for '%s' after retry: %v — marking degraded", i.Title, retryErr)
				i.fireLifecycleEvent(EventExited, "controller-start-failed")
			}
		}
	} else {
		log.DebugLog.Printf("Skipping controller startup for loaded instance '%s' (will be started after wiring)", i.Title)
	}

	return nil
}

// initTmuxSession creates (or reuses) the tmux.TmuxSession object without starting it.
func (i *Instance) initTmuxSession() {
	if i.tmuxManager.HasSession() {
		log.InfoLog.Printf("Reusing existing tmux session for instance '%s'", i.Title)
		return
	}
	commandBuilder := NewClaudeCommandBuilder(i.Program, i.claudeSession)
	enrichedProgram := commandBuilder.Build()
	i.LaunchCommand = enrichedProgram
	log.InfoLog.Printf("Creating tmux session for instance '%s' with program '%s'", i.Title, enrichedProgram)

	tmuxPrefix := i.TmuxPrefix
	if tmuxPrefix == "" {
		tmuxPrefix = "staplersquad_"
	}

	var session *tmux.TmuxSession
	if i.TmuxServerSocket != "" {
		session = tmux.NewTmuxSessionWithServerSocket(i.Title, enrichedProgram, tmuxPrefix, i.TmuxServerSocket, tmux.WithRegistry(nil))
	} else {
		session = tmux.NewTmuxSessionWithPrefix(i.Title, enrichedProgram, tmuxPrefix)
	}
	i.tmuxManager.SetSession(session)
}

// setupFirstTimeWorktree creates or attaches to the git worktree based on session type.
func (i *Instance) setupFirstTimeWorktree() error {
	switch i.SessionType {
	case SessionTypeNewWorktree:
		log.InfoLog.Printf("Creating git worktree for instance '%s' at '%s'", i.Title, i.Path)
		gitWorktree, branchName, err := git.NewGitWorktreeWithBranch(i.Path, i.Title, i.Branch)
		if err != nil {
			return fmt.Errorf("failed to create git worktree: %w", err)
		}
		i.gitManager.SetWorktree(gitWorktree)
		if i.Branch == "" {
			i.Branch = branchName
		}
		log.InfoLog.Printf("Git worktree created for instance '%s', branch: '%s'", i.Title, i.Branch)
	case SessionTypeExistingWorktree:
		if i.ExistingWorktree == "" {
			return fmt.Errorf("existing worktree path required for SessionTypeExistingWorktree")
		}
		log.InfoLog.Printf("Connecting to existing worktree for instance '%s' at '%s'", i.Title, i.ExistingWorktree)
		gitWorktree, err := git.NewGitWorktreeFromExisting(i.ExistingWorktree, i.Title)
		if err != nil {
			return fmt.Errorf("failed to connect to existing worktree: %w", err)
		}
		i.gitManager.SetWorktree(gitWorktree)
		i.Branch = gitWorktree.GetBranchName()
		log.InfoLog.Printf("Connected to existing worktree for instance '%s', branch: '%s'", i.Title, i.Branch)
	default: // SessionTypeDirectory and unknown types → no worktree
		log.InfoLog.Printf("Directory session for instance '%s' at '%s' (no git worktree)", i.Title, i.Path)
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
			log.WarningLog.Printf("WorkingDir '%s' is outside worktree '%s', using worktree path", startPath, basePath)
			return basePath
		}
	}
	if _, err := os.Stat(startPath); os.IsNotExist(err) {
		log.WarningLog.Printf("Working directory '%s' doesn't exist, using '%s' instead", startPath, basePath)
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
	if i.tmuxManager.HasSession() {
		if err := i.tmuxManager.Close(); err != nil {
			return fmt.Errorf("failed to close tmux session: %w", err)
		}
	}
	return nil
}

// CleanupWorktree removes the git worktree, keeping session intact
func (i *Instance) CleanupWorktree() error {
	if i.gitManager.HasWorktree() {
		if err := i.gitManager.Cleanup(); err != nil {
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

	content, err := i.tmuxManager.CapturePaneContent()
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

	var content string
	updated, hasPrompt, content = i.tmuxManager.HasUpdated()

	// Update timestamps when content has actually changed.
	// HasUpdated returns the already-captured content, so no second CapturePaneContent call needed.
	if updated && content != "" {
		i.UpdateTerminalTimestamps(content, false)
	}

	return updated, hasPrompt
}

// TapEnter sends an enter key press to the tmux session if AutoYes is enabled.
func (i *Instance) TapEnter() {
	if !i.started || !i.AutoYes {
		return
	}
	if err := i.tmuxManager.TapEnter(); err != nil {
		log.ErrorLog.Printf("error tapping enter: %v", err)
	}
}

func (i *Instance) Attach() (chan struct{}, error) {
	if !i.started {
		return nil, fmt.Errorf("cannot attach instance that has not been started")
	}
	return i.tmuxManager.Attach()
}

func (i *Instance) SetPreviewSize(width, height int) error {
	if !i.started || i.Status == Paused {
		return fmt.Errorf("cannot set preview size for instance that has not been started or " +
			"is paused")
	}
	return i.tmuxManager.SetDetachedSize(width, height, i.Title)
}

// GetGitWorktree returns the git worktree for the instance
func (i *Instance) GetGitWorktree() (*git.GitWorktree, error) {
	if !i.started {
		return nil, fmt.Errorf("cannot get git worktree for instance that has not been started")
	}
	return i.gitManager.GetWorktree(), nil
}

// HasGitWorktree returns true if the instance has a git worktree
func (i *Instance) HasGitWorktree() bool {
	return i.gitManager.HasWorktree()
}

func (i *Instance) Started() bool {
	return i.started
}

// TmuxSessionExists reports whether the underlying tmux session is currently alive.
// Used at startup to reconcile stale Stopped status against live tmux sessions.
func (i *Instance) TmuxSessionExists() bool {
	return i.tmuxManager.DoesSessionExist()
}

// RecoverFromStopped resets a stale Stopped status to Ready so the instance can be
// hot-restored via Start(false). Only call this during startup reconciliation when
// the tmux session is confirmed alive; it bypasses the state machine intentionally.
func (i *Instance) RecoverFromStopped() {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	if i.Status == Stopped {
		i.setStatus(Ready)
		i.started = false
	}
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
	if i.Status == Paused || !i.started || !i.tmuxManager.HasSession() {
		return false
	}
	return i.tmuxManager.IsAlive()
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
	if dirty, err := i.gitManager.IsDirty(); err != nil {
		errs = append(errs, fmt.Errorf("failed to check if worktree is dirty: %w", err))
		log.ErrorLog.Print(err)
	} else if dirty {
		// Commit changes locally (without pushing to GitHub)
		commitMsg := fmt.Sprintf("[claudesquad] update from '%s' on %s (paused)", i.Title, time.Now().Format(time.RFC822))
		if err := i.gitManager.CommitChanges(commitMsg); err != nil {
			errs = append(errs, fmt.Errorf("failed to commit changes: %w", err))
			log.ErrorLog.Print(err)
			// Return early if we can't commit changes to avoid corrupted state
			return i.combineErrors(errs)
		}
	}

	// Detach from tmux session instead of closing to preserve session output
	if err := i.tmuxManager.DetachSafely(); err != nil {
		errs = append(errs, fmt.Errorf("failed to detach tmux session: %w", err))
		log.ErrorLog.Print(err)
		// Continue with pause process even if detach fails
	}

	// Check if worktree exists before trying to remove it
	if _, err := os.Stat(i.gitManager.GetWorktreePath()); err == nil {
		// Remove worktree but keep branch
		if err := i.gitManager.Remove(); err != nil {
			errs = append(errs, fmt.Errorf("failed to remove git worktree: %w", err))
			log.ErrorLog.Print(err)
			return i.combineErrors(errs)
		}

		// Only prune if remove was successful
		if err := i.gitManager.Prune(); err != nil {
			errs = append(errs, fmt.Errorf("failed to prune git worktrees: %w", err))
			log.ErrorLog.Print(err)
			return i.combineErrors(errs)
		}
	}

	if err := i.combineErrors(errs); err != nil {
		log.ErrorLog.Print(err)
		return err
	}

	i.stateMutex.Lock()
	if err := i.transitionTo(Paused); err != nil {
		i.stateMutex.Unlock()
		return fmt.Errorf("failed to transition to Paused: %w", err)
	}
	i.stateMutex.Unlock()
	log.ForSession(i.Title).Info("Session paused")
	_ = clipboard.WriteAll(i.gitManager.GetBranchName())
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
	if i.gitManager.HasWorktree() {
		// Check if branch is checked out
		if checked, err := i.gitManager.IsBranchCheckedOut(); err != nil {
			log.ErrorLog.Print(err)
			return fmt.Errorf("failed to check if branch is checked out: %w", err)
		} else if checked {
			return fmt.Errorf("cannot resume: branch is checked out, please switch to a different branch")
		}

		// Setup git worktree
		if err := i.gitManager.Setup(); err != nil {
			log.ErrorLog.Print(err)
			log.ForSession(i.Title).Error("Failed to setup git worktree: %v", err)
			return fmt.Errorf("failed to setup git worktree: %w", err)
		}

		worktreePath = i.gitManager.GetWorktreePath()
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
	if i.tmuxManager.DoesSessionExist() {
		// Session exists, just restore PTY connection to it (retains stdout from before pause)
		if err := i.tmuxManager.RestoreWithWorkDir(worktreePath); err != nil {
			log.ErrorLog.Print(err)
			// If restore fails, fall back to creating new session
			if err := i.tmuxManager.Start(worktreePath); err != nil {
				log.ErrorLog.Print(err)
				// Cleanup git worktree if tmux session creation fails
				if i.gitManager.HasWorktree() {
					if cleanupErr := i.gitManager.Cleanup(); cleanupErr != nil {
						err = fmt.Errorf("%v (cleanup error: %v)", err, cleanupErr)
						log.ErrorLog.Print(err)
					}
				}
				return fmt.Errorf("failed to start new session: %w", err)
			}
		}
	} else {
		// Create new tmux session
		if err := i.tmuxManager.Start(worktreePath); err != nil {
			log.ErrorLog.Print(err)
			// Cleanup git worktree if tmux session creation fails
			if i.gitManager.HasWorktree() {
				if cleanupErr := i.gitManager.Cleanup(); cleanupErr != nil {
					err = fmt.Errorf("%v (cleanup error: %v)", err, cleanupErr)
					log.ErrorLog.Print(err)
				}
			}
			return fmt.Errorf("failed to start new session: %w", err)
		}
	}

	i.stateMutex.Lock()
	if err := i.transitionTo(Running); err != nil {
		i.stateMutex.Unlock()
		return fmt.Errorf("failed to transition to Running on resume: %w", err)
	}
	i.stateMutex.Unlock()
	log.ForSession(i.Title).Info("Session resumed")

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

	waspaused := i.Status == Paused

	// Capture terminal output if requested
	var savedOutput string
	if preserveOutput && i.tmuxManager.HasSession() {
		output, err := i.tmuxManager.CapturePaneContentWithOptions("-", "-")
		if err != nil {
			log.WarningLog.Printf("Failed to capture terminal output before restart: %v", err)
		} else {
			savedOutput = output
		}
	}

	// Capture Claude session ID if available for resuming
	var claudeSessionID string
	if i.claudeSession != nil && i.claudeSession.ConversationUUID != "" {
		claudeSessionID = i.claudeSession.ConversationUUID
	}

	// Stop the controller
	i.StopController()

	// Kill the current tmux session
	if err := i.KillSession(); err != nil {
		return fmt.Errorf("failed to kill tmux session: %w", err)
	}

	// Determine the working directory
	var worktreePath string
	if i.gitManager.HasWorktree() {
		// Paused sessions have their worktree directory removed by Pause().
		// Recreate it now so the new tmux session starts in the right place.
		if waspaused {
			if err := i.gitManager.Setup(); err != nil {
				return fmt.Errorf("failed to recreate worktree for paused session: %w", err)
			}
			// Claude stores conversation history keyed by the project directory path.
			// After worktree recreation the encoded path matches the worktree, not the
			// main repo, so --resume with a UUID that was captured in the main repo
			// (or a previous worktree incarnation) will fail with "no conversation found"
			// and cause Claude to exit immediately.  Clear the UUID so Claude starts
			// fresh instead.
			claudeSessionID = ""
			if i.claudeSession != nil {
				i.claudeSession.ConversationUUID = ""
				i.HistoryFilePath = ""
			}
		}
		worktreePath = i.gitManager.GetWorktreePath()
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

	// Inject MCP server URL via CLI flag so no settings-file write is needed.
	// Only for claude commands; other programs (aider, etc.) don't support this flag.
	if i.MCPServerURL != "" && strings.Contains(program, "claude") {
		mcpFlag := fmt.Sprintf(`--mcp-config '{"mcpServers":{"stapler-squad":{"type":"http","url":%q}}}'`, i.MCPServerURL)
		program = program + " " + mcpFlag
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
		tmuxPrefix = "staplersquad_" // Default fallback
	}

	// Record the full launch command for diagnostics (MCP injection verification, etc.)
	i.LaunchCommand = program

	// Use server socket isolation if specified, otherwise use prefix-only isolation
	if i.TmuxServerSocket != "" {
		i.tmuxManager.SetSession(tmux.NewTmuxSessionWithServerSocket(i.Title, program, tmuxPrefix, i.TmuxServerSocket, tmux.WithRegistry(nil)))
	} else {
		i.tmuxManager.SetSession(tmux.NewTmuxSessionWithPrefix(i.Title, program, tmuxPrefix))
	}

	// Start the new session
	if err := i.tmuxManager.Start(worktreePath); err != nil {
		return fmt.Errorf("failed to start new tmux session: %w", err)
	}

	// If output was preserved and we have saved output, write it back
	if preserveOutput && savedOutput != "" {
		// Add a marker to indicate this is restored output
		marker := fmt.Sprintf("\n=== Session restarted at %s ===\n=== Previous output restored below ===\n\n",
			time.Now().Format(time.RFC3339))
		if _, err := i.tmuxManager.SendKeys(fmt.Sprintf("echo '%s'", marker)); err != nil {
			log.WarningLog.Printf("Failed to write restart marker: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
		if err := i.tmuxManager.TapEnter(); err != nil {
			log.WarningLog.Printf("Failed to send enter after marker: %v", err)
		}
	}

	// Restart the controller
	if err := i.StartController(); err != nil {
		log.WarningLog.Printf("Failed to restart controller for instance '%s': %v", i.Title, err)
		// Continue - controller is optional functionality
	}

	// For paused sessions, transition to Running now that the new tmux session is live.
	// For already-running sessions, preserve the existing status.
	i.stateMutex.Lock()
	if waspaused {
		if err := i.transitionTo(Running); err != nil {
			log.WarningLog.Printf("Restart: failed to transition '%s' from Paused to Running: %v", i.Title, err)
			i.setStatus(Running)
		}
		i.started = true
	}
	i.UpdatedAt = time.Now()
	i.stateMutex.Unlock()

	log.InfoLog.Printf("Successfully restarted session '%s'", i.Title)
	return nil
}

// ---- Terminal I/O Delegation ------------------------------------------------
// The following methods delegate to TmuxProcessManager with started/paused
// guards and stateMutex protection. They preserve the public Instance API
// while keeping terminal logic in TmuxProcessManager.

// GetPTYReader returns an io.Reader for streaming terminal output.
// Delegates to tmuxManager.GetPTY.
func (i *Instance) GetPTYReader() (*os.File, error) {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()

	if !i.started {
		return nil, fmt.Errorf("session not started")
	}
	return i.tmuxManager.GetPTY()
}

// WriteToPTY writes data to the PTY, sending input to the terminal session.
// This is used for forwarding client input to the tmux session.
func (i *Instance) WriteToPTY(data []byte) (int, error) {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()

	if !i.started {
		return 0, fmt.Errorf("session not started")
	}
	return i.tmuxManager.SendKeys(string(data))
}

// ResizePTY resizes the terminal dimensions.
// This is used when clients resize their terminal windows.
func (i *Instance) ResizePTY(cols, rows int) error {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()

	if !i.started {
		return fmt.Errorf("session not started")
	}
	if err := i.tmuxManager.SetWindowSize(cols, rows); err != nil {
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
	return i.tmuxManager.CapturePaneContent()
}

// CapturePaneContentRaw captures pane content with ANSI codes preserved (no line joining).
// Essential for hybrid streaming where cursor positioning codes must be preserved.
func (i *Instance) CapturePaneContentRaw() (string, error) {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()

	if !i.started || i.Status == Paused {
		return "", fmt.Errorf("session not started or paused")
	}

	return i.tmuxManager.CapturePaneContentRaw()
}

// GetCurrentPaneContent captures the current visible tmux pane content.
// Delegates to tmuxManager.CaptureViewport.
func (i *Instance) GetCurrentPaneContent(lines int) (string, error) {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()
	content, err := i.tmuxManager.CaptureViewport(lines)
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
	return i.tmuxManager.GetCursorPosition()
}

// GetPaneDimensions gets the current dimensions of the tmux pane.
// Returns width (columns) and height (rows).
func (i *Instance) GetPaneDimensions() (width, height int, err error) {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()
	return i.tmuxManager.GetPaneDimensions()
}

// GetScrollbackHistory captures scrollback history from tmux using line ranges.
// Uses tmux's native scrollback capabilities instead of stored sequences.
// startLine and endLine follow tmux conventions: negative numbers go back from current position,
// use "-" for the start/end of history.
func (i *Instance) GetScrollbackHistory(startLine, endLine string) (string, error) {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()
	return i.tmuxManager.CapturePaneContentWithOptions(startLine, endLine)
}

// ---- VCS/Git Delegation ------------------------------------------------
// UpdateDiffStats delegates to gitManager.ComputeDiffIfReady for I/O, then
// updates state under stateMutex. Other VCS methods (RepoName, HasGitWorktree,
// GetGitWorktree, GetWorkingDirectory, GetEffectiveRootDir, GetDiffStats)
// are thin delegation wrappers elsewhere in this file.

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

	// Write lock to update state
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()

	if needsPause {
		if i.Status != Paused {
			log.WarningLog.Printf("Worktree directory for '%s' doesn't exist, marking as paused", i.Title)
			if err := i.transitionTo(Paused); err != nil {
				log.WarningLog.Printf("Failed to transition '%s' to Paused: %v", i.Title, err)
			}
		}
		i.gitManager.ClearDiffStats()
		return nil
	}
	if stats != nil && stats.Error != nil {
		if strings.Contains(stats.Error.Error(), "base commit SHA not set") {
			i.gitManager.ClearDiffStats()
			return nil
		}
		return fmt.Errorf("failed to get diff stats: %w", stats.Error)
	}
	i.gitManager.SetDiffStats(stats)
	return nil
}

// GetDiffStats returns the current git diff statistics
func (i *Instance) GetDiffStats() *git.DiffStats {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()
	return i.gitManager.GetDiffStats()
}

// SendPrompt sends a prompt to the tmux session. Delegates to tmuxManager.SendPromptWithEnter.
func (i *Instance) SendPrompt(prompt string) error {
	if !i.started {
		return fmt.Errorf("instance not started")
	}
	return i.tmuxManager.SendPromptWithEnter(prompt)
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

	content, err := i.tmuxManager.CapturePaneContentWithOptions("-", "-")
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
	return i.tmuxManager.Session()
}

// ---- SessionStreamer delegation methods ----
// These methods satisfy the services.SessionStreamer interface without exposing
// the concrete *tmux.TmuxSession type to the server layer.

// StartControlMode starts the control mode stream on the underlying tmux session.
func (i *Instance) StartControlMode() error {
	return i.tmuxManager.StartControlMode()
}

// StopControlMode stops the control mode stream.
func (i *Instance) StopControlMode() error {
	return i.tmuxManager.StopControlMode()
}

// SubscribeControlModeUpdates returns a subscriber ID and a read-only output channel.
// Returns a pre-closed channel if the tmux session is not available.
func (i *Instance) SubscribeControlModeUpdates() (string, <-chan []byte) {
	return i.tmuxManager.SubscribeToControlModeUpdates()
}

// UnsubscribeControlModeUpdates removes a subscriber by ID.
func (i *Instance) UnsubscribeControlModeUpdates(id string) {
	i.tmuxManager.UnsubscribeFromControlModeUpdates(id)
}

// SetTmuxSession sets the tmux session for testing purposes
func (i *Instance) SetTmuxSession(session *tmux.TmuxSession) {
	i.tmuxManager.SetSession(session)
	i.started = session != nil
}

// SetWindowSize propagates window size changes to the tmux session
// This enables proper terminal resizing in environments like IntelliJ where SIGWINCH doesn't work
func (i *Instance) SetWindowSize(cols, rows int) error {
	if i.tmuxManager.HasSession() {
		return i.tmuxManager.SetWindowSize(cols, rows)
	}
	return nil
}

// RefreshTmuxClient forces the tmux client to refresh, triggering a redraw
// of the process running inside. This is critical after resizing to ensure
// cursor positions and line wrapping are recalculated for the new dimensions.
func (i *Instance) RefreshTmuxClient() error {
	return i.tmuxManager.RefreshClient()
}

// SetGitWorktree sets the git worktree for testing purposes
func (i *Instance) SetGitWorktree(worktree *git.GitWorktree) {
	i.gitManager.SetWorktree(worktree)
	i.started = worktree != nil
}

// SendKeys sends keys to the tmux session
func (i *Instance) SendKeys(keys string) error {
	if !i.started || i.Status == Paused {
		return fmt.Errorf("cannot send keys to instance that has not been started or is paused")
	}
	_, err := i.tmuxManager.SendKeys(keys)
	return err
}

// ==== Claude Session Management ====
// handleClaudeSessionReattachment and related helpers for Claude Code session persistence.

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
	if i.claudeSession.ConversationUUID != "" {
		log.InfoLog.Printf("Attempting to re-attach to Claude Code session '%s' for instance '%s'",
			i.claudeSession.ConversationUUID, i.Title)

		// Verify the session still exists
		session, err := sessionManager.GetSessionByID(i.claudeSession.ConversationUUID)
		if err != nil {
			if i.claudeSession.Settings.CreateNewOnMissing {
				log.InfoLog.Printf("Stored Claude session not found, will create new session for '%s'", i.Title)
				return i.createNewClaudeSession()
			}
			return fmt.Errorf("stored Claude session '%s' not found: %w", i.claudeSession.ConversationUUID, err)
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
		if i.gitManager.HasWorktree() {
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
		ConversationUUID: newSession.ID,
		SquadSessionID:   newSession.ConversationID,
		ProjectName:      newSession.ProjectName,
		LastAttached:     time.Now(),
		Settings:         i.claudeSession.Settings, // Preserve existing settings
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
	i.claudeSession.ConversationUUID = selectedSession.ID
	i.claudeSession.SquadSessionID = selectedSession.ConversationID
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
	if i.gitManager.HasWorktree() {
		return i.gitManager.GetWorktreePath()
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
	return i.claudeSession != nil && i.claudeSession.ConversationUUID != ""
}

// tryExtractConversationUUID attempts to detect the Claude conversation UUID
// by inspecting the open files of the tmux pane process. This uses the
// HistoryFileDetector to find JSONL files in ~/.claude/projects/.
//
// IMPORTANT: This method assumes stateMutex is already held by the caller.
// It must NOT be called without the lock (e.g., from SwitchWorkspace which
// holds stateMutex). It sets claudeSession fields directly.
//
// The tmux session must be alive for this to work, because it inspects
// the foreground process's open file descriptors via proc_pidinfo.
func (i *Instance) tryExtractConversationUUID() {
	// Skip if we already have a conversation UUID.
	if i.claudeSession != nil && i.claudeSession.ConversationUUID != "" {
		return
	}

	detector := i.historyDetector
	if detector == nil {
		detector = NewHistoryFileDetectorWithRealInspector()
	}
	var info *HistoryFileInfo

	// Fast path: inspect open files of the live tmux pane process.
	if i.tmuxManager.DoesSessionExist() {
		pid, err := i.tmuxManager.GetPanePID()
		if err != nil {
			log.DebugLog.Printf("tryExtractConversationUUID: could not get pane PID for '%s': %v", i.Title, err)
		} else {
			info, err = detector.Detect(pid)
			if err != nil {
				log.WarningLog.Printf("tryExtractConversationUUID: detect error for '%s' (pid=%d): %v", i.Title, pid, err)
			}
		}
	}

	// Fallback: scan the project directory by path (works after reboot / tmux kill).
	if info == nil && i.Path != "" {
		var err error
		info, err = detector.DetectByPath(i.Path)
		if err != nil {
			log.WarningLog.Printf("tryExtractConversationUUID: path-based detect error for '%s': %v", i.Title, err)
		}
		if info != nil {
			log.InfoLog.Printf("tryExtractConversationUUID: found conversation via path fallback for '%s'", i.Title)
		}
	}

	if info == nil {
		log.DebugLog.Printf("tryExtractConversationUUID: no JSONL file found for '%s'", i.Title)
		return
	}

	// Set the fields directly (caller holds stateMutex).
	if i.claudeSession == nil {
		i.claudeSession = &ClaudeSessionData{}
	}
	i.claudeSession.ConversationUUID = info.ConversationUUID
	i.HistoryFilePath = info.HistoryFilePath
}

// GetConversationUUID returns the Claude conversation UUID, or "" if not linked.
func (i *Instance) GetConversationUUID() string {
	if i.claudeSession == nil {
		return ""
	}
	return i.claudeSession.ConversationUUID
}

// GetPanePID returns the PID of the foreground process in the tmux pane.
// Returns 0 and an error if the session is not alive or PID cannot be retrieved.
func (i *Instance) GetPanePID() (int32, error) {
	if !i.tmuxManager.DoesSessionExist() {
		return 0, fmt.Errorf("tmux session not alive for '%s'", i.Title)
	}
	return i.tmuxManager.GetPanePID()
}

// SetHistoryInfo updates the conversation UUID and history file path.
// Thread-safe: acquires stateMutex write lock.
// No-op if the UUID is already set to the same value.
func (i *Instance) SetHistoryInfo(conversationUUID, historyFilePath string) {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()

	currentUUID := ""
	if i.claudeSession != nil {
		currentUUID = i.claudeSession.ConversationUUID
	}
	if currentUUID == conversationUUID && i.HistoryFilePath == historyFilePath {
		return
	}

	if i.claudeSession == nil {
		i.claudeSession = &ClaudeSessionData{}
	}
	i.claudeSession.ConversationUUID = conversationUUID
	i.HistoryFilePath = historyFilePath
}

// CaptureCurrentState records the pane's current working directory into WorkingDir.
// Called during graceful shutdown so cold restore can restart in the right directory.
// No-op if the session is not started, paused, or the tmux session is dead.
func (i *Instance) CaptureCurrentState() error {
	if !i.started || i.Paused() {
		return nil
	}
	if !i.tmuxManager.DoesSessionExist() {
		return nil
	}
	tmuxSession := i.tmuxManager.Session()
	if tmuxSession == nil {
		return nil
	}
	path, err := tmuxSession.GetPaneCurrentPath()
	if err != nil {
		return fmt.Errorf("CaptureCurrentState '%s': %w", i.Title, err)
	}
	if path == "" {
		return nil
	}
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	i.WorkingDir = path
	return nil
}

// CreateCheckpoint captures a named state bookmark for this session.
// scrollbackSeq should be the current scrollback high-water mark (from ScrollbackManager);
// pass 0 if the caller does not have access to scrollback state.
// Thread-safe: acquires stateMutex write lock.
// Returns an error if the instance is not started.
func (i *Instance) CreateCheckpoint(label string, scrollbackSeq uint64) (*Checkpoint, error) {
	if !i.started {
		return nil, fmt.Errorf("cannot create checkpoint on unstarted instance '%s'", i.Title)
	}

	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()

	// Collect git SHA — gracefully empty if no worktree.
	gitSHA, _ := i.gitManager.GetCurrentCommitSHA()

	// Conversation UUID — empty if not yet linked.
	convUUID := ""
	if i.claudeSession != nil {
		convUUID = i.claudeSession.ConversationUUID
	}

	// Count lines in history file for accurate fork truncation later.
	var convLineCount uint64
	if i.HistoryFilePath != "" {
		if f, err := os.Open(i.HistoryFilePath); err == nil {
			defer f.Close()
			sc := bufio.NewScanner(f)
			for sc.Scan() {
				if len(sc.Bytes()) > 0 {
					convLineCount++
				}
			}
			if scanErr := sc.Err(); scanErr != nil {
				log.WarningLog.Printf("CreateCheckpoint: error scanning history file: %v", scanErr)
			}
		}
	}

	cp := Checkpoint{
		ID:             newCheckpointID(),
		SessionID:      i.Title,
		Label:          label,
		ScrollbackSeq:  scrollbackSeq,
		ClaudeConvUUID: convUUID,
		ConvLineCount:  convLineCount,
		GitCommitSHA:   gitSHA,
		Timestamp:      time.Now().UTC(),
	}

	i.Checkpoints = append(i.Checkpoints, cp)
	i.ActiveCheckpoint = cp.ID

	return &cp, nil
}

// ForkFromCheckpoint creates a new, unstarted Instance that is an independent branch of i,
// seeded from the state captured at the checkpoint identified by checkpointID.
func (i *Instance) ForkFromCheckpoint(checkpointID, newTitle string, configDir string) (*Instance, error) {
	cp := i.Checkpoints.FindByID(checkpointID)
	if cp == nil {
		return nil, fmt.Errorf("checkpoint %q not found on session %q", checkpointID, i.Title)
	}
	if newTitle == "" {
		return nil, fmt.Errorf("newTitle must not be empty")
	}

	// Fork Claude conversation if we have the data.
	newConvUUID := ""
	if cp.ConvLineCount > 0 && cp.ClaudeConvUUID != "" && i.HistoryFilePath != "" {
		historyDir := filepath.Dir(i.HistoryFilePath)
		uuid, err := ForkClaudeConversation(i.HistoryFilePath, cp.ConvLineCount, historyDir)
		if err != nil {
			log.WarningLog.Printf("ForkFromCheckpoint: skipping conversation fork: %v", err)
		} else {
			newConvUUID = uuid
		}
	}

	// Fork scrollback.
	srcScrollback := filepath.Join(configDir, i.Title, "scrollback.jsonl")
	dstScrollback := filepath.Join(configDir, newTitle, "scrollback.jsonl")
	if err := scrollback.ForkScrollback(srcScrollback, cp.ScrollbackSeq, dstScrollback); err != nil {
		log.WarningLog.Printf("ForkFromCheckpoint: skipping scrollback fork: %v", err)
	}

	// Build the new instance.
	opts := InstanceOptions{
		Title:      newTitle,
		Path:       i.Path,
		WorkingDir: i.WorkingDir,
		Program:    i.Program,
		AutoYes:    i.AutoYes,
		Category:   i.Category,
		Tags:       append([]string(nil), i.Tags...),
		ResumeId:   newConvUUID,
	}

	newInst, err := NewInstance(opts)
	if err != nil {
		return nil, fmt.Errorf("fork from checkpoint: create instance: %w", err)
	}

	// Attach a git worktree branched from the checkpoint SHA.
	if i.gitManager.HasWorktree() && cp.GitCommitSHA != "" {
		branchName := "fork/" + newTitle
		wt, _, err := git.NewGitWorktreeFromCommitSHA(i.Path, newTitle, branchName, cp.GitCommitSHA)
		if err != nil {
			log.WarningLog.Printf("ForkFromCheckpoint: skipping git worktree: %v", err)
		} else {
			newInst.gitManager.SetWorktree(wt)
		}
	}

	newInst.ForkedFromID = i.Title

	return newInst, nil
}

// GetCheckpoints returns a snapshot copy of the checkpoint list, safe for
// concurrent reads from outside the instance's lock domain.
func (i *Instance) GetCheckpoints() CheckpointList {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()
	cp := make(CheckpointList, len(i.Checkpoints))
	copy(cp, i.Checkpoints)
	return cp
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

// ---- Controller Delegation ------------------------------------------------
// Delegates to ControllerManager for ClaudeController lifecycle.

// SetStatusManager sets the status manager for idle detection.
func (i *Instance) SetStatusManager(manager *InstanceStatusManager) {
	i.controllerManager.SetStatusManager(manager)
}

// GetStatusManager returns the status manager
func (i *Instance) GetStatusManager() *InstanceStatusManager {
	return i.controllerManager.GetStatusManager()
}

// GetEffectiveStatus returns the most accurate status for this instance,
// combining the lifecycle status with real-time terminal detection when available.
// Unlike Status (which only reflects lifecycle transitions), this consults the
// ClaudeController's detected terminal state to surface NeedsApproval, Idle, etc.
func (i *Instance) GetEffectiveStatus() Status {
	mgr := i.GetStatusManager()
	if mgr == nil {
		return i.Status
	}
	statusInfo := mgr.GetStatus(i)
	if !statusInfo.IsControllerActive || statusInfo.ClaudeStatus == 0 { // 0 = StatusUnknown
		return i.Status
	}
	return StatusFromDetected(statusInfo.ClaudeStatus)
}

// GetStatus returns the current lifecycle status of this instance as an int.
// This is intentionally returns int to implement the SessionAccessor interface.
func (i *Instance) GetStatus() int {
	return int(i.Status)
}

// StartController creates and starts a ClaudeController for this instance.
// The controller enables automated idle detection and queue management.
func (i *Instance) StartController() error {
	// Check preconditions under lock
	i.stateMutex.Lock()

	// Only start if we have a status manager
	if i.controllerManager.statusManager == nil {
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
	if i.controllerManager.controller != nil {
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

	// Wire PTY-EOF callback: when the ResponseStream detects PTY exit without an
	// explicit Stop(), transition the instance to Stopped and notify listeners.
	controller.SetOnEOFCallback(func() {
		log.InfoLog.Printf("Instance '%s': PTY EOF received from ResponseStream", i.Title)
		i.stateMutex.Lock()
		if i.Status == Running || i.Status == Ready {
			if err := i.transitionTo(Stopped); err != nil {
				log.WarningLog.Printf("Instance '%s': exit callback transition failed: %v", i.Title, err)
			}
		}
		i.stateMutex.Unlock()
		i.fireLifecycleEvent(EventExited, "pty-eof")
	})

	// Start the controller - this initializes all components and begins background operations
	// Single call replaces the old Initialize() + Start() pattern
	if err := controller.Start(context.Background()); err != nil {
		return fmt.Errorf("failed to start controller: %w", err)
	}

	// Re-acquire lock to update instance state
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()

	// Double-check controller hasn't been set by another goroutine (defensive)
	if i.controllerManager.controller != nil {
		log.DebugLog.Printf("Controller already exists for instance %s (race detected)", i.Title)
		return nil
	}

	// Register with status manager and store controller
	i.controllerManager.RegisterController(i.Title, controller)

	log.InfoLog.Printf("Started ClaudeController for instance %s", i.Title)
	return nil
}

// RegisterLifecycleListener adds a listener that will receive EventStarted and
// EventExited notifications for this instance.
// The listener is called synchronously on the goroutine that fires the event;
// implementations must return quickly (no long blocking operations).
func (i *Instance) RegisterLifecycleListener(l LifecycleListener) {
	i.lifecycleListenersMu.Lock()
	defer i.lifecycleListenersMu.Unlock()
	i.lifecycleListeners = append(i.lifecycleListeners, l)
}

// fireLifecycleEvent notifies all registered listeners of a lifecycle event.
func (i *Instance) fireLifecycleEvent(event LifecycleEvent, reason string) {
	i.lifecycleListenersMu.Lock()
	listeners := make([]LifecycleListener, len(i.lifecycleListeners))
	copy(listeners, i.lifecycleListeners)
	i.lifecycleListenersMu.Unlock()

	for _, l := range listeners {
		l.OnLifecycleEvent(event, reason)
	}
}

// StopController stops and cleans up the ClaudeController for this instance
func (i *Instance) StopController() {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()

	if !i.controllerManager.HasController() {
		return
	}

	i.controllerManager.UnregisterController(i.Title)

	log.InfoLog.Printf("Stopped ClaudeController for instance %s", i.Title)
}

// GetController returns the ClaudeController if one exists
func (i *Instance) GetController() *ClaudeController {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()
	return i.controllerManager.GetController()
}

// GetExitContent returns the last terminal bytes captured before the PTY exited.
// Returns nil if the controller is not running or no exit content was recorded.
func (i *Instance) GetExitContent() []byte {
	ctrl := i.GetController()
	if ctrl == nil {
		return nil
	}
	return ctrl.GetExitContent()
}

// GetRateLimitState returns the current rate limit detection state.
func (i *Instance) GetRateLimitState() int {
	ctrl := i.GetController()
	if ctrl == nil {
		return 0
	}
	return int(ctrl.GetRateLimitState())
}

// SetRateLimitEnabled enables or disables rate limit detection.
func (i *Instance) SetRateLimitEnabled(enabled bool) {
	ctrl := i.GetController()
	if ctrl != nil {
		ctrl.SetRateLimitEnabled(enabled)
	}
}

// IsRateLimitEnabled returns whether rate limit detection is enabled.
func (i *Instance) IsRateLimitEnabled() bool {
	ctrl := i.GetController()
	if ctrl == nil {
		return true // Default to enabled
	}
	return ctrl.IsRateLimitEnabled()
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

// ---- Review State Delegation ------------------------------------------------
// Coordinator methods that bridge TmuxProcessManager with ReviewState,
// plus thin wrappers for ReviewState query methods.

// UpdateTerminalTimestamps is a coordinator method that bridges TmuxProcessManager (I/O)
// with ReviewState (timestamp recording). It:
//  1. Calls tmuxManager.FilterBanners/HasMeaningfulContent (no lock needed, read-only tmux ops)
//  2. Acquires stateMutex
//  3. Delegates to ReviewState.UpdateTimestamps
//
// This method intentionally stays on Instance because it coordinates two sub-managers.
// The forceUpdate parameter bypasses meaningful content checking for user-initiated interactions.
func (i *Instance) UpdateTerminalTimestamps(content string, forceUpdate bool) {
	filteredContent := content
	shouldUpdateMeaningful := false

	if i.tmuxManager.HasSession() {
		if forceUpdate {
			shouldUpdateMeaningful = true
			filteredContent, _ = i.tmuxManager.FilterBanners(content)
		} else {
			hasMeaningful := i.tmuxManager.HasMeaningfulContent(content)
			log.ForSession(i.Title).Debug("HasMeaningfulContent=%v for %d bytes: %q", hasMeaningful, len(content), content)
			if hasMeaningful {
				shouldUpdateMeaningful = true
				filteredContent, _ = i.tmuxManager.FilterBanners(content)
			}
		}
	}

	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	i.UpdateTimestamps(content, filteredContent, shouldUpdateMeaningful, i.Title)
}

// GetTimeSinceLastMeaningfulOutput delegates to ReviewState.TimeSinceLastMeaningfulOutput.
// Falls back to time since creation if no meaningful output has been recorded.
func (i *Instance) GetTimeSinceLastMeaningfulOutput() time.Duration {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()
	return i.TimeSinceLastMeaningfulOutput(i.CreatedAt)
}

// GetTimeSinceLastTerminalUpdate delegates to ReviewState.TimeSinceLastTerminalUpdate.
// Falls back to time since creation if no terminal output has been recorded.
func (i *Instance) GetTimeSinceLastTerminalUpdate() time.Duration {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()
	return i.TimeSinceLastTerminalUpdate(i.CreatedAt)
}

// Approve transitions the instance from NeedsApproval to Running.
// Returns an error if the current state does not allow this transition.
func (i *Instance) Approve() error {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	if err := i.transitionTo(Running); err != nil {
		return fmt.Errorf("approve: %w", err)
	}
	return nil
}

// Deny transitions the instance from NeedsApproval to Paused.
// Returns an error if the current state does not allow this transition.
func (i *Instance) Deny() error {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	if err := i.transitionTo(Paused); err != nil {
		return fmt.Errorf("deny: %w", err)
	}
	return nil
}

// ---- Tag Management Delegation ------------------------------------------------
// The following methods delegate to TagManager with stateMutex protection.
// They preserve the public Instance API while keeping tag logic in TagManager.

// ensureTagManager lazily initializes the tagManager if it was not set up
// (e.g., when Instance is created via struct literal in tests).
// Must be called with stateMutex held.
func (i *Instance) ensureTagManager() {
	if i.tagManager.tags == nil {
		i.tagManager = NewTagManager(&i.Tags)
	}
}

// AddTag adds a tag to the instance. Delegates to TagManager.Add.
// Returns ErrTagTooLong if the tag exceeds MaxTagLength, or ErrDuplicateTag if it already exists.
func (i *Instance) AddTag(tag string) error {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	i.ensureTagManager()
	return i.tagManager.Add(tag)
}

// RemoveTag removes a tag from the instance. Delegates to TagManager.Remove.
func (i *Instance) RemoveTag(tag string) {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	i.ensureTagManager()
	i.tagManager.Remove(tag)
}

// HasTag returns true if the instance has the specified tag. Delegates to TagManager.Has.
func (i *Instance) HasTag(tag string) bool {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()
	if i.tagManager.tags == nil {
		// Fallback for struct-literal created instances (read-only path, no init needed)
		for _, t := range i.Tags {
			if t == tag {
				return true
			}
		}
		return false
	}
	return i.tagManager.Has(tag)
}

// GetTags returns a copy of the instance's tags. Delegates to TagManager.All.
func (i *Instance) GetTags() []string {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()
	if i.tagManager.tags == nil {
		// Fallback for struct-literal created instances (read-only path)
		result := make([]string, len(i.Tags))
		copy(result, i.Tags)
		return result
	}
	return i.tagManager.All()
}

// SetTags replaces all tags with a new deduplicated set. Delegates to TagManager.Set.
// Returns ErrTagTooLong on the first tag that exceeds MaxTagLength.
func (i *Instance) SetTags(tags []string) error {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	i.ensureTagManager()
	return i.tagManager.Set(tags)
}

// ---- GitHub Metadata Delegation ------------------------------------------------
// The following methods delegate to GitHubMetadataView value object.
// The 6 GitHub fields remain on Instance for backward compatibility with
// instance_adapter.go and serialization (ToInstanceData/FromInstanceData).

// GitHub returns a read-only view of the GitHub metadata for this instance.
func (i *Instance) GitHub() GitHubMetadataView {
	return GitHubMetadataView{
		PRNumber:       i.GitHubPRNumber,
		PRURL:          i.GitHubPRURL,
		Owner:          i.GitHubOwner,
		Repo:           i.GitHubRepo,
		SourceRef:      i.GitHubSourceRef,
		ClonedRepoPath: i.ClonedRepoPath,
	}
}

// IsPRSession returns true if this session was created from a GitHub PR URL.
// Delegates to GitHubMetadataView.IsPRSession.
func (i *Instance) IsPRSession() bool { return i.GitHub().IsPRSession() }

// GetGitHubRepoFullName returns "owner/repo" format, or empty string.
// Delegates to GitHubMetadataView.RepoFullName.
func (i *Instance) GetGitHubRepoFullName() string { return i.GitHub().RepoFullName() }

// GetPRDisplayInfo returns a human-readable PR description for UI display.
// Delegates to GitHubMetadataView.PRDisplayInfo.
func (i *Instance) GetPRDisplayInfo() string { return i.GitHub().PRDisplayInfo() }

// IsGitHubSession returns true if this session has GitHub owner and repo set.
// Delegates to GitHubMetadataView.IsGitHubSession.
func (i *Instance) IsGitHubSession() bool { return i.GitHub().IsGitHubSession() }

// UpdatePRStatus atomically updates the PR status fields on this instance.
// Called by PRStatusPoller on each successful fetch.
func (i *Instance) UpdatePRStatus(state, priority, checkConclusion string, approvedCount, changesReqCount int, isDraft, terminal bool) {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	i.GitHubPRState = state
	i.GitHubPRPriority = priority
	i.GitHubPRIsDraft = isDraft
	i.GitHubApprovedCount = approvedCount
	i.GitHubChangesReqCount = changesReqCount
	i.GitHubCheckConclusion = checkConclusion
	i.GitHubPRStatusTerminal = terminal
	i.LastPRStatusCheck = time.Now()
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

// detectAndTrackPrompt detects if current state is a new prompt and tracks it.
// Delegates to ReviewState.DetectAndTrackPrompt — caller must hold stateMutex.
func (i *Instance) detectAndTrackPrompt(content string, statusInfo InstanceStatusInfo) bool {
	return i.DetectAndTrackPrompt(content, statusInfo, i.Title)
}
