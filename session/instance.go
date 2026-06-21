package session

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/atotto/clipboard"
	"github.com/google/uuid"
	"github.com/linkdata/deadlock"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/tmux"
)

type Status int

const (
	// Creating is the status when the instance is being initialized.
	Creating Status = 0
	// Active is the status when the instance has a live AI process (running or ready).
	Active Status = 1
	// Paused is if the instance is paused (worktree removed but branch preserved).
	Paused Status = 2
	// Stopped is a terminal state: the instance has been shut down and cannot transition further.
	Stopped Status = 3
	// Hibernated is the status when the instance has been checkpointed and the tmux session killed.
	Hibernated Status = 4
	// Restoring is the transient startup state when a hibernated session is being restored.
	// Never persisted to the database — transitions to Active or Creating on completion.
	Restoring Status = 5

	// Deprecated: use Active.
	Running = Active
	// Deprecated: use Active.
	Ready = Active
	// Deprecated: use Creating.
	Loading = Creating
)

// String returns a human-readable name for the status.
func (s Status) String() string {
	switch s {
	case Creating:
		return "Creating"
	case Active:
		return "Active"
	case Paused:
		return "Paused"
	case Stopped:
		return "Stopped"
	case Hibernated:
		return "Hibernated"
	case Restoring:
		return "Restoring"
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
	// InitialPrompt is the prompt injected into the tmux pane once the session reaches Ready state.
	// Replaces the static driverInitialPrompt when non-empty.
	InitialPrompt string
	// ExistingWorktree is an optional path to an existing worktree to reuse
	ExistingWorktree string
	// Category is used for organizing sessions into groups
	Category string
	// IsExpanded indicates whether this session's category is expanded in the UI
	IsExpanded bool
	// SessionType determines the session workflow (directory, new_worktree, existing_worktree)
	SessionType SessionType
	// CreateIfMissing: when SessionTypeDirectory, create the directory and run git init
	// if the path does not exist. Set from the request's create_if_missing field.
	// Not persisted — only relevant during initial session start.
	CreateIfMissing bool `json:"-"`
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
	// AutonomousTurn is the current turn during an active autonomous run.
	AutonomousTurn int32 `json:"autonomous_turn,omitempty"`
	// AutonomousMaxTurns is the configured max turns for the current run.
	AutonomousMaxTurns int32 `json:"autonomous_max_turns,omitempty"`
	// AutonomousOutcome is the result of the last autonomous run: "", "done", or "stuck".
	AutonomousOutcome string `json:"autonomous_outcome,omitempty"`

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

	// Hidden excludes this session from the default session list and review queue.
	// Set true for system/background sessions (triage, validation) that should not
	// appear in the user-facing session viewer.
	Hidden bool

	// ProjectID is the optional project this session belongs to.
	ProjectID string

	// HistoryFilePath is the path to the Claude conversation JSONL history file.
	// Set by HistoryLinker when it correlates this session to an open JSONL file.
	HistoryFilePath string

	// MCPServerURL is the URL of the stapler-squad HTTP MCP endpoint.
	// When set, passed as --mcp-config to claude on session start so no
	// settings-file injection is needed.
	MCPServerURL string `json:"mcp_server_url,omitempty"`

	// AppendSystemPrompt, when non-empty and the program is claude, passes
	// --append-system-prompt to inject extra instructions into the system prompt
	// without modifying any file on disk. Survives context compaction.
	AppendSystemPrompt string `json:"append_system_prompt,omitempty"`

	// AllowedTools, when non-empty, passes --allowedTools to claude to pre-approve
	// specific tool calls without requiring interactive permission prompts.
	// Format: "Bash,Read,Edit" or "Bash(git commit *),Read".
	AllowedTools string `json:"allowed_tools,omitempty"`

	// PermissionMode, when non-empty, passes --permission-mode to claude.
	// Values: "default", "acceptEdits", "bypassPermissions", "auto".
	PermissionMode string `json:"permission_mode,omitempty"`

	// CreationProgress holds a human-readable progress message during Creating state.
	// Set by the async creation goroutine; cleared once the session becomes Active.
	// Not persisted to the database — only meaningful in-memory during startup.
	CreationProgress string `json:"-"`

	// LaunchCommand is the full command passed to tmux on session start, including
	// any injected flags (--resume, --mcp-config, -y, initial prompt). Set once on
	// first start and updated on restart. Empty for external (mux-discovered) sessions.
	LaunchCommand string `json:"launch_command,omitempty"`

	// RateLimitAutoResume controls whether the rate-limit manager will automatically
	// send recovery input when a rate limit expires. Persisted so the setting survives
	// server restarts. Defaults to true (enabled) when zero value.
	RateLimitAutoResume *bool `json:"rate_limit_auto_resume,omitempty"`

	// historyDetector is used by tryExtractConversationUUID. When nil the
	// production inspector is used. Set in tests to inject a fake home dir.
	historyDetector *HistoryFileDetector

	// shellRepo is the persistence backend for shell operations. Injected by Storage
	// after instance creation/loading; nil disables persistence (tests, external instances).
	shellRepo ShellRepository

	// shellRegistryEmbed holds in-memory shell state via a concurrent ShellRegistry.
	// Initialized by initShellRegistry(); shell operations go through instance_shells.go.
	shellRegistryEmbed

	// hibernateReason records why this session was hibernated.
	// Values: "manual", "idle", "resource_pressure". Read by hibernateProcess.
	hibernateReason string

	// PauseReason records why this session was paused. Use PauseReason* constants.
	// Empty when session has never been paused.
	PauseReason string `json:"pause_reason,omitempty"`

	// WorkflowID is the UUID of the Workflow that spawned this session.
	// Empty for manually-created sessions.
	WorkflowID string `json:"workflow_id,omitempty"`

	// EnvVars are session-level environment variables injected at tmux session creation.
	EnvVars map[string]string `json:"env_vars,omitempty"`
	// CLIFlags are additional CLI flags appended to the program launch command.
	CLIFlags string `json:"cli_flags,omitempty"`

	// ArchivedAt is set when the session is archived. Nil means not archived.
	ArchivedAt *time.Time `json:"archived_at,omitempty"`

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
	// processManager abstracts the terminal process lifecycle and I/O.
	// Initialized to a TmuxBackend by default; future backends implement the ProcessManager interface.
	pmMu           sync.Mutex
	processManager ProcessManager
	// gitManager owns the git worktree and diff stats.
	gitManager GitWorktreeManager
	// vncManager owns the Xvfb + x11vnc lifecycle for this session.
	// Always non-nil after NewInstance() — on unsupported platforms or missing deps,
	// a no-op manager is returned. VNCManager() returns it for external access.
	vncManager VNCProcessManager
	// cdpManager owns the Chrome DevTools Protocol screencast lifecycle for this
	// session. Always non-nil after NewInstance() — when Chrome is absent, a
	// no-op manager is returned. CDPManager() returns it for external access.
	cdpManager CDPStreamManager

	// tagManager provides CRUD operations for session tags.
	// Backed by a pointer to Instance.Tags for zero-sync compatibility with
	// callers that read inst.Tags directly.
	tagManager TagManager

	// Mutex to protect concurrent access to instance state
	stateMutex deadlock.RWMutex
	// startMu prevents concurrent calls to start() from racing during session setup.
	// Held for the full duration of start(); callers that lose the race return early.
	startMu deadlock.Mutex
	// driverRunning tracks whether a SessionDriver goroutine is active for this instance.
	// Guarded by CompareAndSwap — see StartSessionDriver.
	driverRunning atomic.Bool

	// sessionGoal is the cached goal state for this session.
	// Always use GetSessionGoal/SetSessionGoalCached accessors.
	sessionGoal Locked[*SessionGoalData]

	// restartCount and recentRestartTimes track rapid restarts for storm detection.
	restartCount       int64
	recentRestartTimes []time.Time
	restartMu          deadlock.Mutex

	// lifecycleListeners receives EventStarted / EventExited notifications.
	lifecycleListeners   []LifecycleListener
	lifecycleListenersMu deadlock.Mutex

	// rateLimitCallbacksMu protects the rate limit event callback fields below.
	rateLimitCallbacksMu deadlock.Mutex
	// onRateLimitDetected is called (in a goroutine) when rate limit is detected.
	// Wired by the server layer to publish events to the server event bus.
	onRateLimitDetected func(sessionID string, resetTime time.Time)
	// onRateLimitRecovery is called (in a goroutine) when recovery completes.
	// success=true means recovery input was sent; false means it failed.
	onRateLimitRecovery func(sessionID string, success bool, errMsg string)

	// onStatusChange is called when the ClaudeController detects a status transition.
	// Wired by the server layer to trigger reactive queue checks.
	onStatusChange Locked[func(detection.DetectedStatus, string)]

	// claudeSessionIDSavedCallback is called when SetClaudeConversationUUID stores a
	// newly discovered session_id. Used by the service layer to trigger a storage save.
	claudeSessionIDSavedCallback func()
}

// SessionType indicates the type of session workflow to use
// Pause reason constants. Use these instead of bare string literals.
const (
	PauseReasonManual         = "manual"
	PauseReasonAutoInactivity = "auto:inactivity"
	PauseReasonAutoLimit      = "auto:session_limit"
	PauseReasonAutoResource   = "auto:resource"
)

// SessionType is an alias for config.SessionType so callers can use either package.
type SessionType = config.SessionType

const (
	// SessionTypeDirectory creates a simple directory session without git worktree
	SessionTypeDirectory = config.SessionTypeDirectory
	// SessionTypeNewWorktree creates a new git worktree for the session
	SessionTypeNewWorktree = config.SessionTypeNewWorktree
	// SessionTypeExistingWorktree uses an existing git worktree
	SessionTypeExistingWorktree = config.SessionTypeExistingWorktree
	// SessionTypeNewProject creates a new directory, initializes a git repo with an
	// initial commit, and opens the session. The directory need not exist beforehand.
	SessionTypeNewProject = config.SessionTypeNewProject
	// SessionTypeOneOff generates a fresh temporary directory under one_off_base_dir.
	SessionTypeOneOff = config.SessionTypeOneOff
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
	// InitialPrompt, when non-empty, is typed into the tmux pane once the session reaches Ready state,
	// replacing the static "Please proceed..." fallback.
	InitialPrompt string
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

	// Hidden excludes the session from the default session list and review queue.
	Hidden bool

	// ProjectID associates the session with a project.
	ProjectID string

	// MCPServerURL, when non-empty and the program is claude, passes
	// --mcp-config '{"stapler-squad":{"type":"http","url":"<MCPServerURL>"}}' so the
	// session can call back into stapler-squad without any file injection.
	MCPServerURL string

	// AppendSystemPrompt, when non-empty and the program is claude, passes
	// --append-system-prompt so extra instructions are injected into the system
	// prompt without touching any file on disk.
	AppendSystemPrompt string

	// AllowedTools pre-approves specific Claude Code tool calls (--allowedTools).
	AllowedTools string
	// PermissionMode sets Claude Code's permission handling mode (--permission-mode).
	PermissionMode string

	// CreateIfMissing: when SessionTypeDirectory, create the directory and run git init
	// if the path does not exist. Only set when the user has confirmed the action.
	CreateIfMissing bool

	// AutonomousMode, when true, starts an AutonomousDriver after session creation
	// so the session runs to completion without manual steering.
	AutonomousMode bool

	// WorkflowID is the UUID of the Workflow that spawned this session.
	// Set by the scheduler; empty for manually-created sessions.
	WorkflowID string

	// EnvVars are session-level environment variables injected at tmux session creation time.
	EnvVars map[string]string
	// CLIFlags are additional CLI flags appended to the program launch command.
	CLIFlags string
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
		return nil, fmt.Errorf("invalid session type %q: must be one of %q, %q, %q, %q",
			sessionType, SessionTypeDirectory, SessionTypeNewWorktree, SessionTypeExistingWorktree, SessionTypeNewProject)
	}

	instance := &Instance{
		Title:            opts.Title,
		UUID:             uuid.New().String(),
		Status:           Creating,
		Path:             absPath,
		Branch:           opts.Branch,
		Program:          opts.Program,
		Height:           0,
		Width:            0,
		CreatedAt:        t,
		UpdatedAt:        t,
		AutoYes:          opts.AutoYes,
		Prompt:           opts.Prompt,
		InitialPrompt:    opts.InitialPrompt,
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
		// One-shot mode, hidden flag, project, and workflow linkage
		OneShot:            opts.OneShot,
		Hidden:             opts.Hidden,
		ProjectID:          opts.ProjectID,
		WorkflowID:         opts.WorkflowID,
		MCPServerURL:       opts.MCPServerURL,
		AppendSystemPrompt: opts.AppendSystemPrompt,
		AllowedTools:       opts.AllowedTools,
		PermissionMode:     opts.PermissionMode,
		AutonomousMode:     opts.AutonomousMode,
		// Directory creation on missing path (R2 confirmation flow)
		CreateIfMissing: opts.CreateIfMissing,
		EnvVars:         opts.EnvVars,
		CLIFlags:        opts.CLIFlags,
	}

	// Initialize TagManager backed by the Instance.Tags slice
	instance.tagManager = NewTagManager(&instance.Tags)

	// Sync atomic shadow fields from the just-set ReviewState time.Time values.
	instance.SyncAtomicTimestamps()

	// Initialize the process manager via the factory so selectedBackend is honored.
	// The session itself is wired later by initTmuxSession() at Start() time.
	instance.processManager = NewProcessManager(context.Background(), BackendTmux, ProcessManagerOptions{})

	// Initialize shell registry maps.
	instance.initShellRegistry()

	// Auto-detect worktree info if GitHub owner/repo not explicitly set
	// This extracts repository information from the git remote URL
	if instance.GitHubOwner == "" || instance.GitHubRepo == "" {
		if err := instance.DetectAndPopulateWorktreeInfo(); err != nil {
			log.Warn("failed to detect worktree info for new instance", "session", opts.Title, "err", err)
			// Non-fatal - instance can still be created without this info
		} else if instance.GitHubOwner != "" {
			log.Info("auto-detected github info for new instance", "session", opts.Title, "owner", instance.GitHubOwner, "repo", instance.GitHubRepo, "worktree", instance.IsWorktree)
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
		log.Info("instance configured to resume claude conversation", "session", opts.Title, "resume_id", opts.ResumeId)
	}

	// Initialize the VNC manager (noop when deps are absent or platform is not Linux).
	cfg := config.LoadConfig()
	instance.initVNCManager(cfg)
	// Initialize the CDP manager (noop when Chrome is absent on any platform).
	instance.initCDPManager(cfg)

	return instance, nil
}

// SetShellRepository injects the shell persistence backend. Called by Storage after
// loading or creating an instance. Pass nil to disable persistence (e.g., in tests).
func (i *Instance) SetShellRepository(repo ShellRepository) {
	i.shellRepo = repo
}

// GetSessionGoal returns a thread-safe shallow copy of the current SessionGoalData (nil if not set).
// A copy is returned so callers cannot mutate the shared struct.
func (i *Instance) GetSessionGoal() *SessionGoalData {
	var result *SessionGoalData
	i.sessionGoal.Read(func(g *SessionGoalData) {
		if g != nil {
			copy := *g // shallow copy — Tasks slice is immutable after set
			result = &copy
		}
	})
	return result
}

// SetSessionGoalCached atomically updates the in-memory sessionGoal cache.
func (i *Instance) SetSessionGoalCached(g *SessionGoalData) {
	i.sessionGoal.Write(func(sg **SessionGoalData) {
		*sg = g
	})
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

	log.Info("starting instance", "session", i.Title, "path", i.Path, "program", i.Program, "first_time_setup", firstTimeSetup)

	if !firstTimeSetup {
		i.trackRestartRate()
	}

	if i.Title == "" {
		return fmt.Errorf("instance title cannot be empty")
	}

	i.initTmuxSession()

	// Wire the exit callback so control-mode %exit / PTY EOF fires our handler.
	// ResetExitOnce is called first so repeated start() calls (restarts) allow
	// the callback to fire again after the sync.Once was exhausted in the prior run.
	i.pm().ResetExitOnce()
	i.pm().SetOnExitCallback(func(reason string) {
		log.Info("unexpected exit detected via control mode", "session", i.Title, "reason", reason)
		log.ForSession(i.Title).Info("session exited unexpectedly", "reason", reason)
		i.stateMutex.Lock()
		if i.Status == Active {
			if err := i.transitionTo(context.Background(), Stopped); err != nil {
				log.Warn("exit callback transition failed", "session", i.Title, "err", err)
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
		if !i.pm().IsAlive() {
			// tmux session is dead (machine reboot, tmux kill-server, etc.)
			startPath := i.resolveStartPath(i.GetEffectiveRootDir())
			if i.HasClaudeSession() {
				// Cold restore: we have a conversation UUID — relaunch with --resume.
				// initTmuxSession() (called above) already built the program command
				// with --resume via ClaudeCommandBuilder, so Start() uses it directly.
				log.Info("cold restoring with --resume", "session", i.Title, "uuid", i.claudeSession.ConversationUUID, "path", startPath)
			} else {
				// Dead tmux, no UUID — start a fresh session without --resume.
				log.Warn("cold start: tmux dead, no conversation UUID, starting fresh", "session", i.Title, "path", startPath)
			}
			// Phase 1: Allocate X display before creating the tmux session so DISPLAY
			// can be injected via ExtraEnv at new-session time.
			// context.Background() is safe here: the VNC manager creates its own internal
			// cancellable context; goroutines are stopped via vncManager.Stop() in Destroy().
			i.startVNCDisplay(context.Background())
			// Allocate CDP port before creating the tmux session so CDP_PORT and the
			// updated PATH (wrapper dir) can be injected via ExtraEnv at new-session time.
			i.allocateCDPPort()
			if displayEnv := i.VNCDisplayEnv(); displayEnv != "" {
				if tb, ok := i.processManager.(*TmuxBackend); ok {
					if sess := tb.TmuxManager().Session(); sess != nil {
						sess.ExtraEnv = append(sess.ExtraEnv, displayEnv)
					}
				}
			}
			if cdpEnvs := i.CDPDisplayEnv(); len(cdpEnvs) > 0 {
				if tb, ok := i.processManager.(*TmuxBackend); ok {
					if sess := tb.TmuxManager().Session(); sess != nil {
						sess.ExtraEnv = append(sess.ExtraEnv, cdpEnvs...)
					}
				}
			}
			if err := i.pm().Start(startPath); err != nil {
				setupErr = fmt.Errorf("cold restore Start failed for '%s': %w", i.Title, err)
				return setupErr
			}
			// Attach PTY — same pattern as firstTimeSetup path (lines 867-870).
			_ = i.pm().RestoreWithWorkDir(startPath)
			if _, ptyErr := i.pm().GetPTY(); ptyErr != nil {
				log.Error("cold-restored session: pty attach failed, controller and sendkeys unavailable", "session", i.Title, "err", ptyErr)
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
			// Phase 1 (display) runs here too so VNC is available for the browser tab.
			// DISPLAY injection via ExtraEnv is not possible for an already-running
			// session, but x11vnc still needs to start so the browser passthrough works.
			// context.Background() is safe: VNC goroutines are cancelled via vncManager.Stop().
			i.startVNCDisplay(context.Background())
			// Allocate CDP port for hot-restore too. ExtraEnv injection is not possible
			// for an already-running session, but we still allocate so the screencast
			// goroutine can connect to Chrome if it is already running.
			i.allocateCDPPort()
			workDir := i.Path
			if i.gitManager.HasWorktree() {
				workDir = i.gitManager.GetWorktreePath()
			}
			log.Info("restoring existing tmux session", "session", i.Title, "path", workDir)
			if err := i.pm().RestoreWithWorkDir(workDir); err != nil {
				setupErr = fmt.Errorf("failed to restore existing session: %w", err)
				return setupErr
			}
			log.Info("successfully restored tmux session", "session", i.Title)
		}
	} else {
		basePath := i.Path
		if i.gitManager.HasWorktree() {
			log.Info("setting up git worktree", "session", i.Title)
			if err := i.gitManager.Setup(); err != nil {
				log.ForSession(i.Title).Error("failed to setup git worktree", "err", err)
				setupErr = fmt.Errorf("failed to setup git worktree: %w", err)
				return setupErr
			}
			basePath = i.gitManager.GetWorktreePath()
		}
		startPath := i.resolveStartPath(basePath)
		// Phase 1: Allocate X display before creating the tmux session so DISPLAY
		// can be injected via ExtraEnv at new-session time. This ensures the agent
		// process inherits the correct DISPLAY from the start rather than relying on
		// a post-hoc `tmux setenv` call that would miss the already-running process.
		// context.Background() is safe: VNC goroutines are cancelled via vncManager.Stop().
		i.startVNCDisplay(context.Background())
		// Allocate CDP port before creating the tmux session so CDP_PORT and the
		// updated PATH (wrapper dir) can be injected via ExtraEnv at new-session time.
		i.allocateCDPPort()
		if displayEnv := i.VNCDisplayEnv(); displayEnv != "" {
			if tb, ok := i.processManager.(*TmuxBackend); ok {
				if sess := tb.TmuxManager().Session(); sess != nil {
					sess.ExtraEnv = append(sess.ExtraEnv, displayEnv)
				}
			}
		}
		if cdpEnvs := i.CDPDisplayEnv(); len(cdpEnvs) > 0 {
			if tb, ok := i.processManager.(*TmuxBackend); ok {
				if sess := tb.TmuxManager().Session(); sess != nil {
					sess.ExtraEnv = append(sess.ExtraEnv, cdpEnvs...)
				}
			}
		}
		if err := i.pm().Start(startPath); err != nil {
			if i.gitManager.HasWorktree() {
				if cleanupErr := i.gitManager.Cleanup(); cleanupErr != nil {
					err = fmt.Errorf("%v (cleanup error: %v)", err, cleanupErr)
				}
			}
			setupErr = fmt.Errorf("failed to start new session: %w", err)
			return setupErr
		}
		// Establish PTY connection after creating the tmux session.
		// processManager.Start() creates the detached session but does not attach a PTY.
		// RestoreWithWorkDir finds the existing session and attaches via attach-session,
		// setting t.ptmx so StartController() can call GetPTYReader() successfully.
		// Note: RestoreWithWorkDir always returns nil even on PTY failure; check GetPTY() to confirm.
		_ = i.pm().RestoreWithWorkDir(startPath)
		if _, ptyErr := i.pm().GetPTY(); ptyErr != nil {
			log.Error("new session: pty attach failed after retries, controller and sendkeys unavailable", "session", i.Title, "err", ptyErr)
		}
	}

	i.stateMutex.Lock()
	// Only transition if not already Active (e.g., recovery/restart after KillSession
	// preserves the Active status).
	if i.Status != Active {
		if err := i.transitionTo(context.Background(), Active); err != nil {
			i.stateMutex.Unlock()
			setupErr = fmt.Errorf("failed to transition to Active: %w", err)
			return setupErr
		}
	}
	i.stateMutex.Unlock()
	i.started = true
	i.fireLifecycleEvent(EventStarted, "")

	// Phase 2: Start x11vnc and window tracker now that the tmux session is live.
	// Run unconditionally (not gated on firstTimeSetup) so hot-restores also get VNC.
	// context.Background() is safe: VNC goroutines are cancelled via vncManager.Stop().
	i.startVNCServer(context.Background())
	// Phase 2b: Start CDP screencast goroutine now that the tmux session is live.
	// context.Background() is safe: CDP goroutines are cancelled via cdpManager.Stop().
	i.startCDP(context.Background())
	log.ForSession(i.Title).Info("session started", "first_time_setup", firstTimeSetup)

	// Controller startup is always deferred to the caller after wiring (SetStatusManager).
	// For new sessions (firstTimeSetup=true), the caller (session_service.go async goroutine)
	// calls SetStatusManager + StartController after Start() returns.
	// For loaded sessions (firstTimeSetup=false), loadInstancesWithWiring does the same.
	// Starting the controller inside Start() causes immediate PTY EIO because tmux
	// attach-session hasn't fully initialized by the time the response stream reads.
	log.Debug("skipping controller startup, will be started after wiring", "session", i.Title, "firstTimeSetup", firstTimeSetup)

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

	// Stop VNC before killing tmux (x11vnc must stop before Xvfb).
	i.stopVNC()
	// Stop CDP screencast goroutines and clean up wrapper scripts.
	i.stopCDP()

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

	// Git operations only apply to worktree sessions.
	if i.IsWorktree {
		if dirty, err := i.gitManager.IsDirty(); err != nil {
			errs = append(errs, fmt.Errorf("failed to check if worktree is dirty: %w", err))
			log.Error("failed to check if worktree is dirty", "session", i.Title, "err", err)
		} else if dirty {
			commitMsg := fmt.Sprintf("[claudesquad] update from '%s' on %s (paused)", i.Title, time.Now().Format(time.RFC822))
			if err := i.gitManager.CommitChanges(commitMsg); err != nil {
				errs = append(errs, fmt.Errorf("failed to commit changes: %w", err))
				log.Error("failed to commit changes on pause", "err", err)
				return i.combineErrors(errs)
			}
		}
	}

	// Kill the tmux session to free memory. The Claude session UUID is already
	// persisted by wireClaudeSessionIDSavedCallback before we reach this point.
	// Resume() handles the dead-tmux case by reinitializing with --resume <uuid>.
	if err := i.KillSession(); err != nil {
		log.Warn("pause: failed to kill tmux session, falling back to detach", "session", i.Title, "err", err)
		// Non-fatal: try a plain detach so the session is at least unreachable.
		if detachErr := i.pm().DetachSafely(); detachErr != nil {
			errs = append(errs, fmt.Errorf("failed to detach tmux session: %w", detachErr))
			log.Error("failed to detach tmux session", "err", detachErr)
		}
	}

	// Check if worktree exists before trying to remove it
	if i.IsWorktree {
		if _, err := os.Stat(i.gitManager.GetWorktreePath()); err == nil {
			// Remove worktree but keep branch
			if err := i.gitManager.Remove(); err != nil {
				errs = append(errs, fmt.Errorf("failed to remove git worktree: %w", err))
				log.Error("failed to remove git worktree", "err", err)
				return i.combineErrors(errs)
			}

			// Only prune if remove was successful
			if err := i.gitManager.Prune(); err != nil {
				errs = append(errs, fmt.Errorf("failed to prune git worktrees: %w", err))
				log.Error("failed to prune git worktrees", "err", err)
				return i.combineErrors(errs)
			}
		}
	}

	if err := i.combineErrors(errs); err != nil {
		log.Error("pause encountered errors", "err", err)
		return err
	}

	i.stateMutex.Lock()
	if err := i.transitionTo(context.Background(), Paused); err != nil {
		i.stateMutex.Unlock()
		return fmt.Errorf("failed to transition to Paused: %w", err)
	}
	i.stateMutex.Unlock()
	log.ForSession(i.Title).Info("session paused")
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
			log.Error("failed to check if branch is checked out", "session", i.Title, "err", err)
			return fmt.Errorf("failed to check if branch is checked out: %w", err)
		} else if checked {
			return fmt.Errorf("cannot resume: branch is checked out, please switch to a different branch")
		}

		// Setup git worktree
		if err := i.gitManager.Setup(); err != nil {
			log.Error("failed to setup git worktree on resume", "err", err)
			log.ForSession(i.Title).Error("failed to setup git worktree", "err", err)
			return fmt.Errorf("failed to setup git worktree: %w", err)
		}

		worktreePath = i.gitManager.GetWorktreePath()
	} else {
		// No git worktree, use the original path
		worktreePath = i.Path
	}

	// Handle Claude Code session re-attachment if configured
	if err := i.handleClaudeSessionReattachment(); err != nil {
		log.Warn("failed to re-attach to claude code session", "err", err)
		// Continue with resume - Claude session attachment is not critical for basic functionality
	}

	// Check if tmux session still exists from pause, otherwise create new one
	if i.pm().IsAlive() {
		// Session exists, just restore PTY connection to it (retains stdout from before pause)
		if err := i.pm().RestoreWithWorkDir(worktreePath); err != nil {
			log.Error("restore failed, falling back to new session", "err", err)
			// If restore fails, fall back to creating new session
			if err := i.pm().Start(worktreePath); err != nil {
				log.Error("failed to start new session after restore failure", "err", err)
				// Cleanup git worktree if tmux session creation fails
				if i.gitManager.HasWorktree() {
					if cleanupErr := i.gitManager.Cleanup(); cleanupErr != nil {
						err = fmt.Errorf("%v (cleanup error: %v)", err, cleanupErr)
						log.Error("cleanup also failed", "err", err)
					}
				}
				return fmt.Errorf("failed to start new session: %w", err)
			}
		}
	} else {
		// Tmux session is dead (killed on pause to free memory).
		// Rebuild the TmuxSession object with the current Claude UUID so the program
		// is launched with the correct --resume flag, then start a fresh tmux session.
		var claudeSessionID string
		if i.claudeSession != nil {
			claudeSessionID = i.claudeSession.ConversationUUID
		}
		program := i.buildLaunchCommand(claudeSessionID)
		i.LaunchCommand = program
		tmuxPrefix := i.TmuxPrefix
		if tmuxPrefix == "" {
			tmuxPrefix = "staplersquad_"
		}
		if tb, ok := i.processManager.(*TmuxBackend); ok {
			if i.TmuxServerSocket != "" {
				tb.TmuxManager().SetSession(tmux.NewTmuxSessionWithServerSocket(i.Title, program, tmuxPrefix, i.TmuxServerSocket, tmux.WithRegistry(nil)))
			} else {
				tb.TmuxManager().SetSession(tmux.NewTmuxSessionWithPrefix(i.Title, program, tmuxPrefix))
			}
			if i.UUID != "" {
				tb.TmuxManager().Session().SetExtraEnv([]string{"STAPLER_SESSION_UUID=" + i.UUID})
			}
			if claudeSessionID != "" {
				log.Info("resume: reinitializing tmux session with --resume", "session", i.Title, "uuid", claudeSessionID)
			}
		} else {
			log.Warn("resume: non-TmuxBackend process manager — --resume flag not injected", "session", i.Title)
		}
		if err := i.pm().Start(worktreePath); err != nil {
			log.Error("failed to start new tmux session on resume", "err", err)
			// Cleanup git worktree if tmux session creation fails
			if i.gitManager.HasWorktree() {
				if cleanupErr := i.gitManager.Cleanup(); cleanupErr != nil {
					err = fmt.Errorf("%v (cleanup error: %v)", err, cleanupErr)
					log.Error("cleanup also failed", "err", err)
				}
			}
			return fmt.Errorf("failed to start new session: %w", err)
		}
	}

	i.stateMutex.Lock()
	if err := i.transitionTo(context.Background(), Active); err != nil {
		i.stateMutex.Unlock()
		return fmt.Errorf("failed to transition to Active on resume: %w", err)
	}
	i.stateMutex.Unlock()
	log.ForSession(i.Title).Info("session resumed")

	// Start ClaudeController for idle detection and automation
	// This is non-critical - we log errors but don't fail the resume
	if err := i.StartController(); err != nil {
		log.Warn("failed to start controller on resume", "session", i.Title, "err", err)
		// Continue - controller is optional functionality
	}

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
	if preserveOutput && i.pm().HasSession() {
		output, err := i.pm().CapturePaneContentWithOptions("-", "-")
		if err != nil {
			log.Warn("failed to capture terminal output before restart", "err", err)
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

	if worktreePath == "" {
		return fmt.Errorf("cannot restart session '%s': no working directory configured", i.Title)
	}

	program := i.buildLaunchCommand(claudeSessionID)

	// Create a new tmux session
	// Use configurable prefix or default
	tmuxPrefix := i.TmuxPrefix
	if tmuxPrefix == "" {
		tmuxPrefix = "staplersquad_" // Default fallback
	}

	// Record the full launch command for diagnostics (MCP injection verification, etc.)
	i.LaunchCommand = program

	// Use server socket isolation if specified, otherwise use prefix-only isolation
	if tb, ok := i.processManager.(*TmuxBackend); ok {
		if i.TmuxServerSocket != "" {
			tb.TmuxManager().SetSession(tmux.NewTmuxSessionWithServerSocket(i.Title, program, tmuxPrefix, i.TmuxServerSocket, tmux.WithRegistry(nil)))
		} else {
			tb.TmuxManager().SetSession(tmux.NewTmuxSessionWithPrefix(i.Title, program, tmuxPrefix))
		}
	}

	// Start the new session
	if err := i.pm().Start(worktreePath); err != nil {
		return fmt.Errorf("failed to start new tmux session: %w", err)
	}

	// If output was preserved and we have saved output, write it back
	if preserveOutput && savedOutput != "" {
		// Add a marker to indicate this is restored output
		marker := fmt.Sprintf("\n=== Session restarted at %s ===\n=== Previous output restored below ===\n\n",
			time.Now().Format(time.RFC3339))
		if _, err := i.pm().SendKeys(fmt.Sprintf("echo '%s'", marker)); err != nil {
			log.Warn("failed to write restart marker", "err", err)
		}
		time.Sleep(100 * time.Millisecond)
		if err := i.pm().TapEnter(); err != nil {
			log.Warn("failed to send enter after marker", "err", err)
		}
	}

	// Restart the controller
	if err := i.StartController(); err != nil {
		log.Warn("failed to restart controller", "session", i.Title, "err", err)
		// Continue - controller is optional functionality
	}

	// For paused sessions, transition to Active now that the new tmux session is live.
	// For already-active sessions, preserve the existing status.
	i.stateMutex.Lock()
	if waspaused {
		if err := i.transitionTo(context.Background(), Active); err != nil {
			log.Warn("restart: failed to transition from paused to active", "session", i.Title, "err", err)
			i.setStatus(Active)
		}
		i.started = true
	}
	i.UpdatedAt = time.Now()
	i.stateMutex.Unlock()

	log.Info("successfully restarted session", "session", i.Title)
	return nil
}
