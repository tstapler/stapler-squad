package domain

import (
	"encoding/json"
	"strings"
	"time"
)

// WorkspaceKey returns a canonical identity for the repo/workspace a session belongs to,
// used to group sibling worktrees/branches of the same repo as peers. Prefers the GitHub
// owner/repo (stable across worktree paths); falls back to MainRepoPath, then Path.
// Returns "" when none are set (e.g. a bare one-off session with no git remote).
func WorkspaceKey(githubOwner, githubRepo, mainRepoPath, path string) string {
	if githubOwner != "" && githubRepo != "" {
		return "gh:" + strings.ToLower(githubOwner) + "/" + strings.ToLower(githubRepo)
	}
	if mainRepoPath != "" {
		return "path:" + mainRepoPath
	}
	if path != "" {
		return "path:" + path
	}
	return ""
}

// ShellData carries the input fields for creating a new shell.
type ShellData struct {
	// ID is the UUID for the shell. If empty, the caller must populate it.
	ID string
	// Name is the user-visible label.
	Name string
	// Command is the command to run in the shell.
	Command string
	// WorkingDir is the starting directory for the shell process.
	WorkingDir string
	// TmuxSessionName is the full sibling tmux session name.
	TmuxSessionName string
	// OrderIndex is the display order for the tab.
	OrderIndex int

	// Status, ExitCode and StartedAt are read-side only: populated by
	// ShellRepository.ListShells, ignored by CreateShell.
	Status    string
	ExitCode  *int
	StartedAt time.Time
}

// InstanceData represents the serializable data of an Instance
type InstanceData struct {
	Title         string    `json:"title"`
	UUID          string    `json:"uuid,omitempty"`
	Path          string    `json:"path"`
	WorkingDir    string    `json:"working_dir"`
	Branch        string    `json:"branch"`
	Status        Status    `json:"status"`
	Height        int       `json:"height"`
	Width         int       `json:"width"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	AutoYes       bool      `json:"auto_yes"`
	AutoApprove   bool      `json:"auto_approve"`
	Prompt        string    `json:"prompt"`
	InitialPrompt string    `json:"initial_prompt,omitempty"`
	// InitialPromptSentAt records when InitialPrompt was actually typed into the
	// terminal, persisted so a service restart's fresh driver goroutine doesn't
	// have to re-derive (and potentially get wrong) whether it was already sent.
	InitialPromptSentAt time.Time `json:"initial_prompt_sent_at,omitempty"`

	Program          string          `json:"program"`
	ExistingWorktree string          `json:"existing_worktree,omitempty"`
	Worktree         GitWorktreeData `json:"worktree"`
	DiffStats        DiffStatsData   `json:"diff_stats"`

	// New fields for session organization and grouping
	Category   string   `json:"category,omitempty"`
	Note       string   `json:"note,omitempty"`
	IsExpanded bool     `json:"is_expanded,omitempty"`
	Tags       []string `json:"tags,omitempty"` // Multi-valued tags for flexible organization

	// RuleTagProvenance/SuppressedRuleTags back Instance's ADR-002 tag-provenance
	// fields — see their doc comments on Instance for the full semantics.
	RuleTagProvenance  map[string]string `json:"rule_tag_provenance,omitempty"`
	SuppressedRuleTags map[string]bool   `json:"suppressed_rule_tags,omitempty"`

	// Session type determines the workflow (directory, new_worktree, existing_worktree)
	SessionType SessionType `json:"session_type,omitempty"`

	// GitHub integration fields for PR/URL-based session creation
	GitHubPRNumber  int    `json:"github_pr_number,omitempty"`
	GitHubPRURL     string `json:"github_pr_url,omitempty"`
	GitHubOwner     string `json:"github_owner,omitempty"`
	GitHubRepo      string `json:"github_repo,omitempty"`
	GitHubHost      string `json:"github_host,omitempty"`
	GitHubSourceRef string `json:"github_source_ref,omitempty"`
	ClonedRepoPath  string `json:"cloned_repo_path,omitempty"`
	// Worktree detection fields
	MainRepoPath string `json:"main_repo_path,omitempty"` // Path to main repo when this is a worktree
	IsWorktree   bool   `json:"is_worktree,omitempty"`    // True if path is a git worktree
	GitHubIsFork bool   `json:"github_is_fork,omitempty"` // True when remote repo is a fork
	// PR status fields — populated by PRStatusPoller
	GitHubPRState          string    `json:"github_pr_state,omitempty"`
	GitHubPRIsDraft        bool      `json:"github_pr_is_draft,omitempty"`
	GitHubPRPriority       string    `json:"github_pr_priority,omitempty"`
	GitHubApprovedCount    int       `json:"github_approved_count,omitempty"`
	GitHubChangesReqCount  int       `json:"github_changes_req_count,omitempty"`
	GitHubCheckConclusion  string    `json:"github_check_conclusion,omitempty"`
	GitHubPRStatusTerminal bool      `json:"github_pr_status_terminal,omitempty"`
	LastPRStatusCheck      time.Time `json:"last_pr_status_check,omitempty"`
	// Multi-PR link set carried through full saves (never JSON-serialized: the
	// links table is the store). See Instance.PRs.
	PRs                   []PRView `json:"-"`
	LegacyPRUnrepresented bool     `json:"-"`
	HasPRLinks            bool     `json:"-"`
	// TerminalClearPending asks the full save to write terminal=false even for a
	// session without links, whose normal save only ever sets the flag.
	TerminalClearPending bool `json:"-"`
	// Crew autonomy mode — when true, the Fixer injects correction prompts without user confirmation.
	AutonomousMode bool `json:"autonomous_mode,omitempty"`

	// Claude Code session persistence
	ClaudeSession ClaudeSessionData `json:"claude_session,omitempty"`
	// Tmux session prefix for isolation
	TmuxPrefix string `json:"tmux_prefix,omitempty"`
	// Tmux server socket name for isolation (used with tmux -L flag)
	TmuxServerSocket string `json:"tmux_server_socket,omitempty"`
	// TymuxSessionID is the tymuxd session id this Instance owns. Restore adopts
	// it first so a rename (title -> session label) cannot orphan the live session.
	// Empty on records saved before it existed.
	TymuxSessionID string `json:"tymux_session_id,omitempty"`
	// Backend is the per-session ProcessManager backend pin (e.g. BackendTymux),
	// persisted so a process restart doesn't silently drop it — see
	// Instance.Backend's doc comment for precedence. Empty means "no pin,
	// fall through to the process-wide default."
	Backend ProcessManagerBackend `json:"backend,omitempty"`

	// Terminal update timestamps for activity tracking
	LastTerminalUpdate   time.Time `json:"last_terminal_update,omitempty"`
	LastMeaningfulOutput time.Time `json:"last_meaningful_output,omitempty"`

	// Content signature for detecting actual terminal changes vs restarts
	// This is a SHA256 hash of the terminal content used to prevent false "new activity"
	// notifications when app restarts but terminal content hasn't changed
	LastOutputSignature string `json:"last_output_signature,omitempty"`

	// Review queue spam prevention
	LastAddedToQueue time.Time `json:"last_added_to_queue,omitempty"`

	// User interaction tracking
	// LastViewed tracks when the user last viewed this session (terminal, session details, etc.)
	// Used for smarter review queue notifications (don't notify if just viewed)
	LastViewed time.Time `json:"last_viewed,omitempty"`

	// Review queue snooze tracking
	// LastAcknowledged tracks when the user last dismissed this session from review queue
	// Sessions acknowledged after their last update won't appear in the queue until they update again
	LastAcknowledged time.Time `json:"last_acknowledged,omitempty"`

	// CreationProgressUpdatedAt records when Instance.CreationProgress was last set
	// (Instance.creationProgressUpdatedAt, Epic 1.1.4). Persisted so the
	// Stale-Creation Sweeper (Epic 4.1) can judge a restored Creating row's actual
	// last-progress time after a process restart, not just its Creating-onset time.
	CreationProgressUpdatedAt time.Time `json:"creation_progress_updated_at,omitempty"`

	// Prompt detection and interaction tracking for smart review queue behavior
	LastPromptDetected   time.Time `json:"last_prompt_detected,omitempty"`
	LastPromptSignature  string    `json:"last_prompt_signature,omitempty"`
	LastUserResponse     time.Time `json:"last_user_response,omitempty"`
	ProcessingGraceUntil time.Time `json:"processing_grace_until,omitempty"`

	// Checkpoint metadata for session state bookmarking (session resumption)
	Checkpoints      CheckpointList `json:"checkpoints,omitempty"`
	ActiveCheckpoint string         `json:"active_checkpoint,omitempty"`
	ForkedFromID     string         `json:"forked_from_id,omitempty"`
	// RestartedFromSessionID — see Instance.RestartedFromSessionID's doc comment.
	RestartedFromSessionID string `json:"restarted_from_session_id,omitempty"`

	// History file linkage for cold restore
	HistoryFilePath string `json:"history_file_path,omitempty"`

	// EverHadConversationHistory and LastReviveOutcome persist the
	// session-revive-uuid-loss AC3 signal across full process restarts, not
	// just tmux restarts — see Instance.EverHadConversationHistory.
	EverHadConversationHistory bool   `json:"ever_had_conversation_history,omitempty"`
	LastReviveOutcome          string `json:"last_revive_outcome,omitempty"`

	// OneShot runs claude in -p mode; session exits after task completes.
	OneShot bool `json:"one_shot,omitempty"`

	// Hidden excludes this session from the default session list and review queue.
	Hidden bool `json:"hidden,omitempty"`
	// Pinned mirrors Instance.Pinned for JSON snapshot / ent round-trip.
	Pinned bool `json:"pinned,omitempty"`

	// ProjectID is the optional project this session belongs to.
	ProjectID string `json:"project_id,omitempty"`

	// LaunchCommand is the full command passed to tmux on session start, including
	// any injected flags (--resume, --mcp-config, -y, initial prompt).
	LaunchCommand string `json:"launch_command,omitempty"`

	// MCPServerURL is the stapler-squad HTTP MCP endpoint passed to claude via
	// --mcp-config on session start. Persisted so restarts re-inject the flag.
	MCPServerURL string `json:"mcp_server_url,omitempty"`

	// PauseReason records why this session was paused.
	// Values: "manual", "auto:inactivity", "auto:session_limit", "auto:resource".
	// Empty when session has never been paused.
	PauseReason string `json:"pause_reason,omitempty"`

	// ExitReason records why this session's pane crashed (Status == Crashed).
	// Empty otherwise. Set by SessionHealthChecker (session/health.go).
	ExitReason string `json:"exit_reason,omitempty"`

	// WorkflowID is the UUID of the Workflow that spawned this session.
	// Empty for manually-created sessions.
	WorkflowID string `json:"workflow_id,omitempty"`

	// ArchivedAt is set when the session is archived. Nil means not archived.
	ArchivedAt *time.Time `json:"archived_at,omitempty"`
}

// GitWorktreeData represents the serializable data of a GitWorktree
type GitWorktreeData struct {
	RepoPath      string `json:"repo_path"`
	WorktreePath  string `json:"worktree_path"`
	SessionName   string `json:"session_name"`
	BranchName    string `json:"branch_name"`
	BaseCommitSHA string `json:"base_commit_sha"`
}

// DiffStatsData represents the serializable data of a DiffStats
// Note: Content is excluded from JSON serialization to reduce state file size.
// Diffs are generated on-demand via GetSessionDiff RPC when needed.
type DiffStatsData struct {
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
	Content string `json:"-"` // Excluded from serialization - generated on-demand
}

// ClaudeSessionData represents Claude Code session information
type ClaudeSessionData struct {
	ConversationUUID string            `json:"session_id,omitempty"`       // Claude Code conversation UUID (used for --resume)
	SquadSessionID   string            `json:"squad_session_id,omitempty"` // claude-squad's own session identifier (= Instance.UUID)
	ProjectName      string            `json:"project_name,omitempty"`     // Project name in Claude Code
	LastAttached     time.Time         `json:"last_attached,omitempty"`    // When this session was last used
	Settings         ClaudeSettings    `json:"settings,omitempty"`         // User preferences for Claude Code
	Metadata         map[string]string `json:"metadata,omitempty"`         // Additional session metadata
}

// PiSessionData represents pi-coding-agent session information needed to
// resume a prior conversation via `pi --session <id>` (see buildPiCommand).
// Unlike ClaudeSessionData's ConversationUUID, SessionID's format is not
// validated here — pi 0.84.4 reports it as a standard dashed UUID in the
// JSONL "session" header event's "id" field (see plan.md's Phase 1 spike
// RESULTS), but this struct only requires it be non-empty to be usable.
type PiSessionData struct {
	SessionID    string    `json:"session_id,omitempty"`
	LastAttached time.Time `json:"last_attached,omitempty"`
}

// UnmarshalJSON keeps backward compatibility with persisted state written
// before SquadSessionID was renamed from ConversationID. The legacy
// "conversation_id" key is read as a fallback when "squad_session_id" is
// absent, so existing JSON state files continue to hydrate the field on load.
func (c *ClaudeSessionData) UnmarshalJSON(data []byte) error {
	type alias ClaudeSessionData
	aux := struct {
		*alias
		LegacyConversationID string `json:"conversation_id,omitempty"`
	}{alias: (*alias)(c)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if c.SquadSessionID == "" && aux.LegacyConversationID != "" {
		c.SquadSessionID = aux.LegacyConversationID
	}
	return nil
}

// ClaudeSettings contains user preferences for Claude Code integration
type ClaudeSettings struct {
	AutoReattach          bool   `json:"auto_reattach"`           // Automatically reattach to last session on resume
	PreferredSessionName  string `json:"preferred_session_name"`  // Preferred session naming pattern
	CreateNewOnMissing    bool   `json:"create_new_on_missing"`   // Create new session if previous one is missing
	ShowSessionSelector   bool   `json:"show_session_selector"`   // Show session selection menu on resume
	SessionTimeoutMinutes int    `json:"session_timeout_minutes"` // Consider sessions stale after this time
}

// GetStableID mirrors Instance.GetStableID for InstanceData: returns UUID when set,
// Title otherwise. Used by Registry.AcquireAll and ListInstanceIDs to produce stable
// per-session keys without constructing live Instance objects.
func (d InstanceData) GetStableID() string {
	if d.UUID != "" {
		return d.UUID
	}
	return d.Title
}

// MatchesID reports whether id refers to this InstanceData. Unlike Instance.MatchesID,
// there is no tmux-name arm because InstanceData has no GetTmuxSessionName (that method
// requires the live processManager). For tmux-name matching, call Instance.MatchesID.
func (d InstanceData) MatchesID(id string) bool {
	return d.Title == id || d.GetStableID() == id
}

// WorkspaceKey returns this instance data's workspace identity. See the package-level
// WorkspaceKey function for the derivation rules.
func (d InstanceData) WorkspaceKey() string {
	return WorkspaceKey(d.GitHubOwner, d.GitHubRepo, d.MainRepoPath, d.Path)
}

// ActiveDir returns this instance data's resolved working directory: the git
// worktree path if one was recorded (requires LoadOptions.LoadWorktree), else
// Path. Mirrors Instance.Workspace().ActiveDir for callers that only have the
// serialized InstanceData (see .claude/rules/instance-lock-free-reads.md) —
// use this, not the raw identity Path, wherever a lookup needs to match the
// directory a worktree session actually runs in.
func (d InstanceData) ActiveDir() string {
	if d.Worktree.WorktreePath != "" {
		return d.Worktree.WorktreePath
	}
	return d.Path
}
