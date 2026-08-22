package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/artifacts"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/sessiongoal"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/tokens"
)

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

	Program          string          `json:"program"`
	ExistingWorktree string          `json:"existing_worktree,omitempty"`
	Worktree         GitWorktreeData `json:"worktree"`
	DiffStats        DiffStatsData   `json:"diff_stats"`

	// New fields for session organization and grouping
	Category   string   `json:"category,omitempty"`
	Note       string   `json:"note,omitempty"`
	IsExpanded bool     `json:"is_expanded,omitempty"`
	Tags       []string `json:"tags,omitempty"` // Multi-valued tags for flexible organization

	// Session type determines the workflow (directory, new_worktree, existing_worktree)
	SessionType SessionType `json:"session_type,omitempty"`

	// GitHub integration fields for PR/URL-based session creation
	GitHubPRNumber  int    `json:"github_pr_number,omitempty"`
	GitHubPRURL     string `json:"github_pr_url,omitempty"`
	GitHubOwner     string `json:"github_owner,omitempty"`
	GitHubRepo      string `json:"github_repo,omitempty"`
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
	// Crew autonomy mode — when true, the Fixer injects correction prompts without user confirmation.
	AutonomousMode bool `json:"autonomous_mode,omitempty"`

	// Claude Code session persistence
	ClaudeSession ClaudeSessionData `json:"claude_session,omitempty"`
	// Tmux session prefix for isolation
	TmuxPrefix string `json:"tmux_prefix,omitempty"`
	// Tmux server socket name for isolation (used with tmux -L flag)
	TmuxServerSocket string `json:"tmux_server_socket,omitempty"`

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

	// Prompt detection and interaction tracking for smart review queue behavior
	LastPromptDetected   time.Time `json:"last_prompt_detected,omitempty"`
	LastPromptSignature  string    `json:"last_prompt_signature,omitempty"`
	LastUserResponse     time.Time `json:"last_user_response,omitempty"`
	ProcessingGraceUntil time.Time `json:"processing_grace_until,omitempty"`

	// Checkpoint metadata for session state bookmarking (session resumption)
	Checkpoints      CheckpointList `json:"checkpoints,omitempty"`
	ActiveCheckpoint string         `json:"active_checkpoint,omitempty"`
	ForkedFromID     string         `json:"forked_from_id,omitempty"`

	// History file linkage for cold restore
	HistoryFilePath string `json:"history_file_path,omitempty"`

	// OneShot runs claude in -p mode; session exits after task completes.
	OneShot bool `json:"one_shot,omitempty"`

	// Hidden excludes this session from the default session list and review queue.
	Hidden bool `json:"hidden,omitempty"`

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

// InstanceStore is the minimal interface the server layer needs for session persistence.
// Defining it here (alongside the concrete Storage) allows test fakes to be built without
// depending on the full Storage implementation.
type InstanceStore interface {
	LoadInstances() ([]*Instance, error)
	// ListInstanceData returns raw persisted InstanceData without constructing Instance
	// objects or spawning PTY processes. Use this for read-only existence/title checks
	// where calling LoadInstances() would create unnecessary side effects.
	ListInstanceData() ([]InstanceData, error)
	SaveInstances([]*Instance) error
	AddInstance(*Instance) error
	DeleteInstance(title string) error
	UpdateInstanceLastUserResponse(title string, t time.Time) error
	UpdateInstanceMetadata(currentTitle string, newTitle, category, note, workingDir *string) error
}

// Compile-time assertion: *Storage must satisfy InstanceStore.
var _ InstanceStore = (*Storage)(nil)

// Storage handles saving and loading instances via the repository backend.
type Storage struct {
	repo *EntRepository
}

// NewStorageWithRepository creates a Storage backed by an EntRepository.
func NewStorageWithRepository(repo *EntRepository) (*Storage, error) {
	return &Storage{repo: repo}, nil
}

// Close performs graceful shutdown of storage.
func (s *Storage) Close() error {
	return nil
}

// GetEntClient returns the *ent.Client from the underlying EntRepository.
func (s *Storage) GetEntClient() *ent.Client {
	return s.repo.GetEntClient()
}

// SetItemChangePublisher wires p into the underlying repository.
func (s *Storage) SetItemChangePublisher(p ItemChangePublisher) {
	s.repo.SetItemChangePublisher(p)
}

// SetCallbackDispatcher forwards to the underlying *EntRepository's SetCallbackDispatcher.
func (s *Storage) SetCallbackDispatcher(d CallbackDispatcher) {
	s.repo.SetCallbackDispatcher(d)
}

// WireChainFirer constructs a ChainFirer bound to the underlying
// *EntRepository (so its ListItemSessions/UpdateBacklogItem calls share the
// exact same callbackDispatcher/itemChangePublisher wiring as every other
// backlog mutation) and wires it as that repository's own chain-fire
// dispatcher (EntRepository.SetChainFirer — the happy-path caller from
// TransitionBacklogItemStatus, webhook-triggers Phase 6).
func (s *Storage) WireChainFirer(workflows WorkflowRepository, fireEvents TriggerFireEventRepository, firer TriggerFirer, cfg *config.Config) *ChainFirer {
	cf := NewChainFirer(s.repo, workflows, fireEvents, firer, cfg)
	s.repo.SetChainFirer(cf)
	return cf
}

// SaveInstances upserts each started instance into the repository.
func (s *Storage) SaveInstances(instances []*Instance) error {
	return s.saveInstancesToRepo(instances)
}

// saveInstancesToRepo upserts each started instance into the repository.
// The DB handles concurrent writers so no merging is required.
func (s *Storage) saveInstancesToRepo(instances []*Instance) error {
	ctx := context.Background()
	for _, inst := range instances {
		if !inst.Started() {
			continue
		}
		data := inst.ToInstanceData()
		log.Info("SaveInstances: converting instance",
			"session", data.Title, "is_worktree", data.IsWorktree, "main_repo_path", data.MainRepoPath,
			"github_owner", data.GitHubOwner, "github_repo", data.GitHubRepo)
		if err := s.repo.Update(ctx, data); err != nil {
			// Not found → create it
			if createErr := s.repo.Create(ctx, data); createErr != nil {
				log.Error("SaveInstances: failed to upsert instance",
					"session", data.Title, "update_err", err, "create_err", createErr)
			}
		}
	}
	return nil
}

// SaveInstancesSync saves instances synchronously (same as SaveInstances for the repo backend).
func (s *Storage) SaveInstancesSync(instances []*Instance) error {
	return s.saveInstancesToRepo(instances)
}

// LoadInstances loads the list of instances from the repository.
func (s *Storage) LoadInstances() ([]*Instance, error) {
	ctx := context.Background()
	dataSlice, err := s.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list instances from repository: %w", err)
	}
	instances := make([]*Instance, 0, len(dataSlice))
	for _, data := range dataSlice {
		// Defer Start() to the async Step 6 loop in BuildRuntimeDeps so a bulk
		// load (server startup) doesn't block on cold-restoring every dead
		// session before the HTTP server can bind.
		inst, err := fromInstanceData(data, true)
		if err != nil {
			log.Warn("skipping instance from repository", "session", data.Title, "err", err)
			continue
		}
		// Inject shell repository so shell operations can persist to the DB.
		inst.SetShellRepository(s.repo)
		instances = append(instances, inst)
	}

	// Bulk-load all session goals in one query (N+1 → 1) and apply to instances.
	if client := s.GetEntClient(); client != nil && len(instances) > 0 {
		uuids := make([]string, 0, len(instances))
		for _, inst := range instances {
			if inst.UUID != "" {
				uuids = append(uuids, inst.UUID)
			}
		}
		if len(uuids) > 0 {
			goals, goalErr := client.SessionGoal.Query().
				Where(sessiongoal.SessionUUIDIn(uuids...)).
				All(ctx)
			if goalErr != nil {
				log.Warn("LoadInstances: failed to bulk-load session goals", "err", goalErr)
			} else {
				goalMap := make(map[string]*SessionGoalData, len(goals))
				for _, g := range goals {
					tasks, decodeErr := DecodeTasks(g.Tasks)
					if decodeErr != nil {
						log.Warn("LoadInstances: failed to decode tasks JSON", "session_uuid", g.SessionUUID, "err", decodeErr)
						tasks = []TaskNode{}
					}
					goalMap[g.SessionUUID] = &SessionGoalData{
						UUID:        g.ID.String(),
						SessionUUID: g.SessionUUID,
						Goal:        g.Goal,
						Status:      g.Status,
						Tasks:       tasks,
						SetBy:       g.SetBy,
						UpdatedAt:   g.UpdatedAt,
					}
				}
				for _, inst := range instances {
					if goal, ok := goalMap[inst.UUID]; ok {
						inst.SetSessionGoalCached(goal)
					}
				}
			}
		}
	}

	// Bulk-load stored artifact blobs so the first render shows cached artifacts
	// without waiting for ArtifactExtractor's startup walk to complete.
	// Single bulk query replaces N per-session queries (M-4 fix).
	if s.GetEntClient() != nil && len(instances) > 0 {
		allArtifacts, artifErr := s.GetAllInstanceArtifacts()
		if artifErr != nil {
			log.Warn("LoadInstances: failed to bulk-load artifacts", "err", artifErr)
		} else {
			for _, inst := range instances {
				raw := allArtifacts[inst.Title]
				if raw == "" {
					continue
				}
				var blob artifacts.SessionArtifactsBlob
				if err := json.Unmarshal([]byte(raw), &blob); err == nil {
					inst.Artifacts = &blob
				}
			}
		}
	}

	return instances, nil
}

// ListInstanceData returns raw InstanceData from the repository without constructing
// Instance objects. This avoids the side effect of FromInstanceData() calling Start()
// (which spawns PTY processes). Use for read-only existence and title checks.
func (s *Storage) ListInstanceData() ([]InstanceData, error) {
	return s.repo.ListWithOptions(context.Background(), LoadMinimal)
}

// ListInstanceDataWithWorktree returns raw InstanceData with the Worktree edge eager-loaded.
// Use this instead of ListInstanceData whenever a read-only pass needs Worktree.WorktreePath/
// RepoPath/BranchName/BaseCommitSHA (e.g. to stat the worktree or check dirty status) — plain
// ListInstanceData uses LoadMinimal, which never populates Worktree, so any such field will
// silently read as its zero value under that call.
func (s *Storage) ListInstanceDataWithWorktree() ([]InstanceData, error) {
	return s.repo.ListWithOptions(context.Background(), LoadOptions{LoadWorktree: true})
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

// ErrInstanceDataNotFound is returned by FindInstanceDataByID when no match exists.
var ErrInstanceDataNotFound = errors.New("instance data not found")

// FindInstanceDataByID finds the first InstanceData whose stable ID or title matches id.
// Returns ErrInstanceDataNotFound when no match exists.
func (s *Storage) FindInstanceDataByID(id string) (*InstanceData, error) {
	all, err := s.ListInstanceData()
	if err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].MatchesID(id) {
			return &all[i], nil
		}
	}
	return nil, ErrInstanceDataNotFound
}

// ListInstanceIDs returns the stable ID (UUID if set, else Title) for every stored
// InstanceData. Used by Registry.AcquireAll to seed the initial live-handle set.
func (s *Storage) ListInstanceIDs() ([]string, error) {
	all, err := s.ListInstanceData()
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(all))
	for i, d := range all {
		ids[i] = d.GetStableID()
	}
	return ids, nil
}

// ListSessionRecords returns a snapshot of all sessions as SessionRecords,
// for use by the tokens.Associator to match JSONL files to stapler-squad sessions.
func (s *Storage) ListSessionRecords() []tokens.SessionRecord {
	data, err := s.ListInstanceData()
	if err != nil {
		return nil
	}
	records := make([]tokens.SessionRecord, 0, len(data))
	for _, d := range data {
		sessionID := d.UUID
		if sessionID == "" {
			sessionID = d.Title
		}
		records = append(records, tokens.SessionRecord{
			SessionID:      sessionID,
			ConversationID: d.ClaudeSession.ConversationUUID,
			Path:           d.Path,
			CreatedAt:      d.CreatedAt,
		})
	}
	return records
}

// DeleteInstance removes an instance from storage.
func (s *Storage) DeleteInstance(title string) error {
	return s.repo.Delete(context.Background(), title)
}

// AddInstance adds a new instance to storage.
// Unlike SaveInstances, this does not require instance.Started() to be true.
func (s *Storage) AddInstance(instance *Instance) error {
	data := instance.ToInstanceData()
	ctx := context.Background()
	if err := s.repo.Create(ctx, data); err != nil {
		if !ent.IsConstraintError(err) {
			return fmt.Errorf("failed to persist session %q: %w", data.Title, err)
		}
		// Unique constraint violation → session already exists, update instead.
		if updateErr := s.repo.Update(ctx, data); updateErr != nil {
			return updateErr
		}
	}
	// Inject shell repository so shell operations can persist to the DB.
	instance.SetShellRepository(s.repo)
	return nil
}

// UpdateInstance updates an existing instance in storage.
func (s *Storage) UpdateInstance(instance *Instance) error {
	return s.repo.Update(context.Background(), instance.ToInstanceData())
}

// UpdateInstanceMetadata persists a narrow set of session metadata fields (title rename,
// category, note, working dir) via a single UPDATE, avoiding the full-row rewrite (and
// worktree/diffstats/tags/claude_session churn) that SaveInstances performs for every
// started session. currentTitle must be the title from before any rename already applied
// in-memory by the caller — see EntRepository.UpdateSessionMetadata for why. A nil field
// pointer leaves that field untouched.
func (s *Storage) UpdateInstanceMetadata(currentTitle string, newTitle, category, note, workingDir *string) error {
	return s.repo.UpdateSessionMetadata(context.Background(), currentTitle, newTitle, category, note, workingDir)
}

// DeleteAllInstances removes all stored instances.
func (s *Storage) DeleteAllInstances() error {
	ctx := context.Background()
	dataSlice, err := s.repo.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list instances for deletion: %w", err)
	}
	for _, data := range dataSlice {
		if err := s.repo.Delete(ctx, data.Title); err != nil {
			log.Warn("failed to delete instance", "session", data.Title, "err", err)
		}
	}
	return nil
}

// UpdateInstanceTimestampsOnly updates ONLY the timestamp fields in storage without
// creating Instance objects. This preserves in-memory state like controllers.
// This is critical for WebSocket terminal streaming which updates timestamps frequently.
func (s *Storage) UpdateInstanceTimestampsOnly(title string, lastTerminalUpdate, lastMeaningfulOutput time.Time, lastOutputSignature string, lastViewed time.Time) error {
	if err := s.repo.UpdateTimestamps(context.Background(), title, lastTerminalUpdate, lastMeaningfulOutput, lastOutputSignature); err != nil {
		return err
	}
	if !lastViewed.IsZero() {
		return s.repo.UpdateLastViewed(context.Background(), title, lastViewed)
	}
	return nil
}

// UpdateInstanceLastAddedToQueue updates ONLY the LastAddedToQueue field for a specific instance.
func (s *Storage) UpdateInstanceLastAddedToQueue(title string, lastAddedToQueue time.Time) error {
	return s.repo.UpdateLastAddedToQueue(context.Background(), title, lastAddedToQueue)
}

// UpdateInstanceLastUserResponse persists the LastUserResponse timestamp for a session.
// Uses a direct UPDATE (no read round-trip) via UpdateReviewQueueState.
func (s *Storage) UpdateInstanceLastUserResponse(title string, lastUserResponse time.Time) error {
	return s.repo.UpdateReviewQueueState(context.Background(), title, lastUserResponse, time.Time{}, time.Time{}, "")
}

// UpdateInstanceAcknowledged sets the LastAcknowledged timestamp to now for a specific instance.
// Used by AcknowledgeSession when the instance is not available in the live poller.
func (s *Storage) UpdateInstanceAcknowledged(title string) error {
	return s.repo.UpdateLastAcknowledged(context.Background(), title, time.Now())
}

// UpdateInstanceProcessingGrace persists the ProcessingGraceUntil timestamp.
// Uses a direct UPDATE (no read round-trip) via UpdateReviewQueueState.
func (s *Storage) UpdateInstanceProcessingGrace(title string, processingGraceUntil time.Time) error {
	return s.repo.UpdateReviewQueueState(context.Background(), title, time.Time{}, processingGraceUntil, time.Time{}, "")
}

// UpdateInstancePRStatus updates the PR status fields for a specific instance.
// PR fields are not stored in the ent schema — they live in memory and are re-populated by
// PRStatusPoller on each poll cycle. No DB write is needed.
func (s *Storage) UpdateInstancePRStatus(_, _, _, _ string, _, _ int, _, _ bool) error {
	return nil
}

// UpdateInstancePRNumber persists the discovered PR number for a session so it
// survives restarts and avoids repeated branch-name lookups in PRStatusPoller.
func (s *Storage) UpdateInstancePRNumber(title string, prNumber int) error {
	return s.repo.UpdateGitHubPRNumber(context.Background(), title, prNumber)
}

// UpdateInstanceForkFlag is intentionally a no-op: fork status is not persisted in
// the ent schema. Callers (e.g. PRStatusPoller) call this as a persistence hook,
// but no DB write occurs.
func (s *Storage) UpdateInstanceForkFlag(_ string, _ bool) error {
	return nil
}

// UpdateInstanceArtifacts persists the JSON-encoded artifact blob for a session.
// Only the session_artifacts column is touched; all other fields are unchanged.
func (s *Storage) UpdateInstanceArtifacts(title string, blob string) error {
	return s.repo.UpdateSessionArtifacts(context.Background(), title, blob)
}

// GetInstanceArtifacts loads the raw JSON-encoded artifact blob for a session.
// Returns ("", nil) if the session exists but has no artifacts yet.
func (s *Storage) GetInstanceArtifacts(title string) (string, error) {
	return s.repo.GetSessionArtifacts(context.Background(), title)
}

// GetAllInstanceArtifacts returns a map of title → raw artifacts JSON for all sessions
// that have stored artifacts. Single bulk query (M-4 fix).
func (s *Storage) GetAllInstanceArtifacts() (map[string]string, error) {
	return s.repo.GetAllSessionArtifacts(context.Background())
}

// --- Session-first convenience methods (Task 2.5) ---
// These delegate directly to the Repository's Session-based methods.
// Prefer these over the deprecated InstanceData-based methods for new code.

// GetSession retrieves a session by title using the Session domain model.
// Use ContextOptions presets (ContextMinimal, ContextUIView, etc.) to control what is loaded.
func (s *Storage) GetSession(ctx context.Context, title string, opts ContextOptions) (*Session, error) {
	return s.repo.GetSession(ctx, title, opts)
}

// ListSessions retrieves all sessions using the Session domain model.
// Use ContextOptions presets (ContextMinimal, ContextUIView, etc.) to control what is loaded.
func (s *Storage) ListSessions(ctx context.Context, opts ContextOptions) ([]*Session, error) {
	return s.repo.ListSessions(ctx, opts)
}

// SaveSession upserts a session using the Session domain model.
// If the session exists it is updated; otherwise it is created.
// Deprecated InstanceData-based methods (SaveInstances, LoadInstances) remain for backward compatibility.
func (s *Storage) SaveSession(ctx context.Context, session *Session) error {
	if err := s.repo.UpdateSession(ctx, session); err != nil {
		return s.repo.CreateSession(ctx, session)
	}
	return nil
}

// --- Permissions & Analytics --------------------------------------------------

// AllRules returns all auto-approval rules from the repository.
func (s *Storage) AllRules(ctx context.Context) ([]ApprovalRuleData, error) {
	return s.repo.AllRules(ctx)
}

// UpsertRule creates or updates an auto-approval rule in the repository.
func (s *Storage) UpsertRule(ctx context.Context, rule ApprovalRuleData) error {
	return s.repo.UpsertRule(ctx, rule)
}

// DeleteRule removes an auto-approval rule from the repository.
func (s *Storage) DeleteRule(ctx context.Context, id string) error {
	return s.repo.DeleteRule(ctx, id)
}

// RecordAnalytics logs a classification decision to the repository.
func (s *Storage) RecordAnalytics(ctx context.Context, data AnalyticsData) error {
	return s.repo.RecordAnalytics(ctx, data)
}

// ListAnalytics retrieves recent classification decisions from the repository.
func (s *Storage) ListAnalytics(ctx context.Context, limit int) ([]AnalyticsData, error) {
	return s.repo.ListAnalytics(ctx, limit)
}

// ListAnalyticsSince retrieves analytics entries with created_at >= since.
func (s *Storage) ListAnalyticsSince(ctx context.Context, since time.Time, limit int) ([]AnalyticsData, error) {
	return s.repo.ListAnalyticsSince(ctx, since, limit)
}

// ListAnalyticsByProgramSince retrieves entries for a specific program since a time.
func (s *Storage) ListAnalyticsByProgramSince(ctx context.Context, program string, since time.Time, limit int) ([]AnalyticsData, error) {
	return s.repo.ListAnalyticsByProgramSince(ctx, program, since, limit)
}

// GetSubcommandBreakdown returns per-(subcommand, decision) counts for a program.
func (s *Storage) GetSubcommandBreakdown(ctx context.Context, program string, since time.Time) ([]SubcommandDecisionCount, error) {
	return s.repo.GetSubcommandBreakdown(ctx, program, since)
}

// ListRecentCommandsByProgram returns the most recent n command_preview strings.
func (s *Storage) ListRecentCommandsByProgram(ctx context.Context, program, subcommand string, since time.Time, n int) ([]string, error) {
	return s.repo.ListRecentCommandsByProgram(ctx, program, subcommand, since, n)
}

// GetSubcommandTrend returns raw analytics rows for (program, subcommand) since a time.
func (s *Storage) GetSubcommandTrend(ctx context.Context, program, subcommand string, since time.Time) ([]AnalyticsData, error) {
	return s.repo.GetSubcommandTrend(ctx, program, subcommand, since)
}

// --- Projects ---

// CreateProject inserts a new project into storage.
func (s *Storage) CreateProject(ctx context.Context, data ProjectData) (*ProjectData, error) {
	return s.repo.CreateProject(ctx, data)
}

// ListProjects returns all projects from storage.
func (s *Storage) ListProjects(ctx context.Context) ([]ProjectData, error) {
	return s.repo.ListProjects(ctx)
}

// UpdateProject modifies an existing project in storage.
func (s *Storage) UpdateProject(ctx context.Context, data ProjectData) (*ProjectData, error) {
	return s.repo.UpdateProject(ctx, data)
}

// DeleteProject removes a project from storage (sessions are unassigned).
func (s *Storage) DeleteProject(ctx context.Context, name string) error {
	return s.repo.DeleteProject(ctx, name)
}

// AssignSessionsToProject links sessions to a project in storage.
func (s *Storage) AssignSessionsToProject(ctx context.Context, projectName string, sessionTitles []string) error {
	return s.repo.AssignSessionsToProject(ctx, projectName, sessionTitles)
}

// --- Backlog ---

// CreateBacklogItem inserts a new backlog item.
func (s *Storage) CreateBacklogItem(ctx context.Context, data BacklogItemData) (*BacklogItemData, error) {
	return s.repo.CreateBacklogItem(ctx, data)
}

// GetBacklogItem retrieves a backlog item by UUID string.
func (s *Storage) GetBacklogItem(ctx context.Context, id string) (*BacklogItemData, error) {
	return s.repo.GetBacklogItem(ctx, id)
}

// ListBacklogItems returns backlog items with optional filtering.
func (s *Storage) ListBacklogItems(ctx context.Context, filter BacklogItemFilter) ([]BacklogItemData, error) {
	return s.repo.ListBacklogItems(ctx, filter)
}

// ListBacklogItemSummaries returns lightweight summaries for list views.
func (s *Storage) ListBacklogItemSummaries(ctx context.Context, filter BacklogItemFilter) ([]BacklogItemSummary, error) {
	return s.repo.ListBacklogItemSummaries(ctx, filter)
}

// AddBacklogItemDependency records a blocker/blocked dependency edge.
func (s *Storage) AddBacklogItemDependency(ctx context.Context, edge BacklogItemDependencyEdge) error {
	return s.repo.AddBacklogItemDependency(ctx, edge)
}

// UnresolvedBlockerItemIDs returns the subset of itemIDs blocked by an unresolved dependency.
func (s *Storage) UnresolvedBlockerItemIDs(ctx context.Context, itemIDs []string) (map[string]bool, error) {
	return s.repo.UnresolvedBlockerItemIDs(ctx, itemIDs)
}

// UnresolvedBlockerIDs returns the specific blocker item IDs still unresolved for a single item.
func (s *Storage) UnresolvedBlockerIDs(ctx context.Context, itemID string) ([]string, error) {
	return s.repo.UnresolvedBlockerIDs(ctx, itemID)
}

// UpdateBacklogItem modifies an existing backlog item.
func (s *Storage) UpdateBacklogItem(ctx context.Context, id string, update BacklogItemUpdate, precondition *BacklogItemPrecondition) (*BacklogItemData, error) {
	return s.repo.UpdateBacklogItem(ctx, id, update, precondition)
}

// ArchiveBacklogItem sets the archived_at timestamp.
func (s *Storage) ArchiveBacklogItem(ctx context.Context, id string) (*BacklogItemData, error) {
	return s.repo.ArchiveBacklogItem(ctx, id)
}

// UnarchiveBacklogItem clears archived_at and restores the item to "idea".
func (s *Storage) UnarchiveBacklogItem(ctx context.Context, id string) (*BacklogItemData, error) {
	return s.repo.UnarchiveBacklogItem(ctx, id)
}

// DeleteBacklogItem permanently removes an item and all its child records.
func (s *Storage) DeleteBacklogItem(ctx context.Context, id string) error {
	return s.repo.DeleteBacklogItem(ctx, id)
}

// TransitionBacklogItemStatus changes the status of a backlog item.
func (s *Storage) TransitionBacklogItemStatus(ctx context.Context, id string, toStatus BacklogStatus, precondition *BacklogItemPrecondition, triggeredBy string) (*BacklogItemData, error) {
	return s.repo.TransitionBacklogItemStatus(ctx, id, toStatus, precondition, triggeredBy)
}

// SetBacklogItemPRAndTransition is the shared primary-write path for
// recording a PR that genuinely exists on GitHub against a backlog item and
// moving it review -> pr_pending, or — when observed is already pr_pending —
// correcting an already-recorded PR to a different one (a reassignment, e.g.
// the tracked branch was polluted and the real PR was opened from a clean
// one instead). Used by both the agent-initiated report_pr_created MCP tool
// (server/mcp/tools_backlog.go, Epic 3.1), the reconciliation backstop
// detector (BacklogLifecycleListener.reconcileOrphanedAgentPRs, Epic 3.2),
// and the manual-override RPC (server/services/backlog_service_lifecycle.go)
// — see "PR Metadata Capture Fix", project_plans/backlog-agent-communication/implementation/plan.md.
//
// observed must be the caller's own, already-fetched snapshot of the item —
// this function never re-fetches it. That's load-bearing, not an
// optimization: the CAS precondition below pins to observed.Status and
// observed.UpdatedAt exactly as the caller read them. A prior version of
// this function did its own internal GetBacklogItem call and derived the
// precondition from THAT fresh read; under a real race, that internal read
// could land after a concurrent winner's write had already committed, so it
// would see the winner's (already valid) post-write state and re-derive a
// precondition that ALSO matched — letting a second, policy-unvalidated call
// silently succeed as an accidental "reassignment" the caller never actually
// decided to allow (its override_reason/merged-PR/author checks all ran
// against the caller's original, now-stale read). Pinning to the caller's
// own observed snapshot closes that: any state change since the caller's
// read — including a second call winning first — is guaranteed to fail this
// call's CAS, rather than being silently reinterpreted.
//
// Only observed.Status == "review" or "pr_pending" is accepted; anything
// else is rejected outright (ErrPreconditionFailed) rather than attempted
// against an arbitrary starting status.
//
// Unlike AppendProgressNote's best-effort discipline, a failure persisting
// the PR fields or performing the transition is returned to the caller, not
// merely logged — BUG-040's root cause #1 was exactly this class of silent
// failure (a write whose result was never checked against the invariant it
// protects), and this is the primitive that must not repeat it.
//
// Idempotent: if observed is already pr_pending with this exact prNumber,
// this is a no-op success — a retried report_pr_created call (network blip)
// or the reconciliation backstop re-scanning an item it already fixed on a
// prior tick must not error.
//
// guard is required whenever observed is already pr_pending (a
// reassignment): the caller must supply a PRReassignmentGuard attesting it
// already verified override_reason, the currently-tracked PR's merged
// state, and the new PR's author — nil (or a guard failing any of those
// checks) is rejected outright with ErrPRReassignmentNotAllowed. This
// function does not itself call GitHub — that verification stays in the
// caller (server/mcp/tools_backlog.go's reportPRCreated is the only caller
// today with that machinery) — but centralizing the *requirement* here
// means every caller of this shared primitive gets the same guarantee: a
// caller with no way to produce a valid guard (e.g. the manual-override RPC
// in server/services/backlog_service_lifecycle.go, which by design never
// calls GitHub — see its own doc comment) simply cannot reassign, rather
// than silently succeeding because the check only lived in one handler.
// guard is ignored (may be nil) when observed.Status is review — a
// first-time recording never needs one.
func (s *Storage) SetBacklogItemPRAndTransition(ctx context.Context, observed *BacklogItemData, prURL string, prNumber int, summary string, guard *PRReassignmentGuard) error {
	if observed.Status == string(BacklogStatusPRPending) && observed.PrNumber == prNumber && prNumber > 0 {
		return nil // already recorded — idempotent no-op
	}
	if observed.Status != string(BacklogStatusReview) && observed.Status != string(BacklogStatusPRPending) {
		return fmt.Errorf("%w: expected status %q or %q, got %q", ErrPreconditionFailed, BacklogStatusReview, BacklogStatusPRPending, observed.Status)
	}
	isReassignment := observed.Status == string(BacklogStatusPRPending)
	if isReassignment {
		switch {
		case guard == nil:
			return fmt.Errorf("%w: item already has PR #%d tracked (status pr_pending) — this caller did not supply a PRReassignmentGuard", ErrPRReassignmentNotAllowed, observed.PrNumber)
		case guard.OverrideReason == "":
			return fmt.Errorf("%w: override_reason is required to reassign PR #%d to a different PR", ErrPRReassignmentNotAllowed, observed.PrNumber)
		case guard.CurrentPRMerged:
			return fmt.Errorf("%w: currently tracked PR #%d is already merged — its association cannot be changed", ErrPRReassignmentNotAllowed, observed.PrNumber)
		case !guard.NewPRAuthorVerified:
			return fmt.Errorf("%w: the new PR's author must be verified to match the caller's identity before reassigning PR #%d", ErrPRReassignmentNotAllowed, observed.PrNumber)
		}
	}

	// The status transition (review -> pr_pending, or pr_pending ->
	// pr_pending on reassignment) and the PrURL/PrNumber field write must
	// land as a single atomic UPDATE, not two separate calls — even with the
	// transition ordered first (closing the original lost-update race: two
	// racing callers with different PR numbers both unconditionally passing
	// a field-write precondition before either transitioned), two separate
	// calls still leave a narrower gap between them where a concurrent
	// reader can observe status=pr_pending with PrNumber==0. That exact
	// shape is what the pr_pending_no_pr / BUG-040 stuck detector
	// (reconcilePRPendingWithoutPRItems, session/backlog_lifecycle.go)
	// exists to flag as a HIGH-priority, non-auto-recoverable alert — and
	// its resolution condition is anchored on the item leaving pr_pending
	// entirely, so a reconcile tick landing in that window could raise a
	// spurious alert that stays open for days. TransitionBacklogItemStatusWithPRFields
	// folds both writes into one UPDATE ... WHERE statement guarded by the
	// same CAS precondition, so they always commit together — no reader can
	// ever observe one without the other.
	// AC6: distinguish a first-time recording from a correction in the audit
	// trail — otherwise a pr_pending -> pr_pending event reads as a no-op in
	// BacklogStatusEvent history, and the reconciler/manual-override callers
	// share this same status-event write path.
	noteLabel := "PR recorded"
	progressNoteStatus := "pr_created"
	if isReassignment {
		noteLabel = "PR reassigned"
		progressNoteStatus = "pr_corrected"
	}
	expectedUpdatedAt := observed.UpdatedAt
	precondition := &BacklogItemPrecondition{
		ExpectedStatus:    observed.Status,
		ExpectedUpdatedAt: &expectedUpdatedAt,
		Note:              fmt.Sprintf("[%s] %s", noteLabel, summary),
	}
	if _, err := s.repo.TransitionBacklogItemStatusWithPRFields(ctx, observed.ID, BacklogStatusPRPending, prURL, prNumber, precondition, TriggeredBySystem); err != nil {
		return fmt.Errorf("transition to pr_pending with PR fields: %w", err)
	}

	// Best-effort from here: the primary contract (PR fields persisted, item
	// moved to pr_pending) already succeeded above. A failure enriching the
	// history or resolving a stale stuck row must not roll that back or be
	// reported as this call's own failure — mirrors report_progress's
	// primary-write/secondary-enrichment split (AppendProgressNote there).
	if appendErr := s.AppendProgressNote(ctx, observed.ID, -1, summary, progressNoteStatus); appendErr != nil {
		log.WarningLog().Printf("[Storage] SetBacklogItemPRAndTransition: failed to append summary note item=%s: %v", observed.ID, appendErr)
	}
	if _, resolveErr := s.ResolveStuck(ctx, observed.ID, domain.StuckReasonPushFailed); resolveErr != nil {
		log.WarningLog().Printf("[Storage] SetBacklogItemPRAndTransition: failed to resolve push_failed row item=%s: %v", observed.ID, resolveErr)
	}
	if _, resolveErr := s.ResolveStuck(ctx, observed.ID, domain.StuckReasonAbandonedReview); resolveErr != nil {
		log.WarningLog().Printf("[Storage] SetBacklogItemPRAndTransition: failed to resolve abandoned_review row item=%s: %v", observed.ID, resolveErr)
	}

	// AC7: reassigning to a new PR must not leave the old PR's
	// feedback-dedup watermark in place — a stale watermark could suppress a
	// genuinely new review comment on the new PR if a comment happened to
	// land after the old watermark's timestamp. Best-effort, same discipline
	// as the resolves above.
	if isReassignment {
		if _, clearErr := s.UpdateBacklogItem(ctx, observed.ID, BacklogItemUpdate{ClearPrFeedbackAddressedAt: true}, nil); clearErr != nil {
			log.WarningLog().Printf("[Storage] SetBacklogItemPRAndTransition: failed to clear pr_feedback_addressed_at item=%s: %v", observed.ID, clearErr)
		}
	}

	return nil
}

// FindDoneItemsOlderThan returns backlog items in "done" status whose most
// recent done-transition happened at/before cutoff. Thin passthrough to the
// ent-backed repository (same rationale as MarkStuck below) — returns
// nil, nil for backends that don't support it (e.g. an in-memory test
// double), never an error.
func (s *Storage) FindDoneItemsOlderThan(ctx context.Context, cutoff time.Time) ([]BacklogItemData, error) {
	return s.repo.FindDoneItemsOlderThan(ctx, cutoff)
}

// --- BacklogStuckState (durable stuck-state read surface) ---

// FindOpenStuckStates returns every open (unresolved, un-snoozed)
// BacklogStuckState row, joined with rendering-relevant item fields. Returns
// an empty slice (no error) when the backend does not support stuck-state
// queries (e.g. an in-memory test double).
func (s *Storage) FindOpenStuckStates(ctx context.Context) ([]OpenStuckStateData, error) {
	return s.repo.FindOpenStuckStates(ctx)
}

// SnoozeStuckState sets snoozed_until on an open BacklogStuckState row for
// (itemID, reason). Returns false, nil when the backend does not support
// stuck-state writes or no matching open row exists — never an error for a
// missing row.
func (s *Storage) SnoozeStuckState(ctx context.Context, itemID string, reason domain.StuckReason, until time.Time) (bool, error) {
	return s.repo.SnoozeStuckState(ctx, itemID, reason, until)
}

// MarkStuck opens/refreshes/reopens a durable BacklogStuckState row for
// (itemID, reason). Thin passthrough so callers outside package session
// (e.g. server/services, which cannot reach the unexported repo field) can
// write stuck state. Returns false, nil when the backend does not support
// stuck-state writes — never an error for an unsupported backend.
func (s *Storage) MarkStuck(ctx context.Context, itemID string, reason domain.StuckReason, expectedStatus BacklogStatus, stuckContext string) (bool, error) {
	return s.repo.MarkStuck(ctx, itemID, reason, expectedStatus, stuckContext)
}

// ResolveStuck atomically, idempotently closes an open BacklogStuckState row
// for (itemID, reason). Thin passthrough, same rationale as MarkStuck above.
func (s *Storage) ResolveStuck(ctx context.Context, itemID string, reason domain.StuckReason) (bool, error) {
	return s.repo.ResolveStuck(ctx, itemID, reason)
}

// MarkStuckNotified sets notified_at=now on an open, not-yet-notified stuck
// row for (itemID, reason). Thin passthrough, same rationale as MarkStuck above.
func (s *Storage) MarkStuckNotified(ctx context.Context, itemID string, reason domain.StuckReason) (bool, error) {
	return s.repo.MarkStuckNotified(ctx, itemID, reason)
}

// --- ItemSource ---

// CreateItemSource registers a new external item source.
func (s *Storage) CreateItemSource(ctx context.Context, data ItemSourceData) (*ItemSourceData, error) {
	return s.repo.CreateItemSource(ctx, data)
}

// ListItemSources returns all registered item sources.
func (s *Storage) ListItemSources(ctx context.Context) ([]ItemSourceData, error) {
	return s.repo.ListItemSources(ctx)
}

// UpdateItemSource modifies an existing item source.
func (s *Storage) UpdateItemSource(ctx context.Context, id string, update ItemSourceUpdate) (*ItemSourceData, error) {
	return s.repo.UpdateItemSource(ctx, id, update)
}

// DeleteItemSource removes an item source by UUID string.
func (s *Storage) DeleteItemSource(ctx context.Context, id string) error {
	return s.repo.DeleteItemSource(ctx, id)
}

// GetItemSourceByID retrieves a single item source's domain data by UUID
// string. Used by the GitHub forward-sync EventBus subscriber (see
// server/services/backlog_github_forward_sync.go) to look up a backlog item's
// source (ForwardSyncEnabled, ForwardSyncCloseLabel, PluginID, Config) without
// needing an *EntRepository handle of its own.
func (s *Storage) GetItemSourceByID(ctx context.Context, id string) (*ItemSourceData, error) {
	src, err := s.repo.GetItemSourceByID(ctx, id)
	if err != nil {
		return nil, err
	}
	data := itemSourceToData(src)
	return &data, nil
}

// ListSourceSyncEvents returns sync history events for an item source, most
// recent first. Direct EntRepository delegation, like GetItemSession below.
func (s *Storage) ListSourceSyncEvents(ctx context.Context, sourceID string) ([]SourceSyncEventData, bool, error) {
	return s.repo.ListSourceSyncEvents(ctx, sourceID)
}

// CreateSourceSyncEvent records a sync run for an item source. Direct EntRepository
// delegation, like ListSourceSyncEvents above.
func (s *Storage) CreateSourceSyncEvent(ctx context.Context, sourceID, cursorAfter string, created, updated, skipped, errored int, errMsg string, startedAt, finishedAt time.Time) error {
	return s.repo.CreateSourceSyncEvent(ctx, sourceID, cursorAfter, created, updated, skipped, errored, errMsg, startedAt, finishedAt)
}

// RecordSourceSyncFailure records a forward-sync failure (e.g. CloseIssue
// erroring) as a queryable sync-history row. Direct EntRepository delegation,
// like CreateSourceSyncEvent above.
func (s *Storage) RecordSourceSyncFailure(ctx context.Context, sourceID, message string) error {
	return s.repo.RecordSourceSyncFailure(ctx, sourceID, message)
}

// --- ItemSession (direct EntRepository delegation) ---

// GetItemSession looks up an ItemSession by entity UUID (loads BacklogItem edge).
func (s *Storage) GetItemSession(ctx context.Context, id string) (ItemSessionSummary, error) {
	return s.repo.GetItemSession(ctx, id)
}

// GetBaseCommitSHAsForSessions returns a sessionUUID→base_commit_sha map for the given UUIDs.
func (s *Storage) GetBaseCommitSHAsForSessions(ctx context.Context, uuids []string) (map[string]string, error) {
	return s.repo.GetBaseCommitSHAsForSessions(ctx, uuids)
}

// GetItemSessionBySessionUUID looks up the ItemSession for a given session UUID (loads BacklogItem edge).
func (s *Storage) GetItemSessionBySessionUUID(ctx context.Context, sessionUUID string) (ItemSessionSummary, error) {
	return s.repo.GetItemSessionBySessionUUID(ctx, sessionUUID)
}

// GetWorktreeDataBySessionUUID returns the git worktree data for the Session with
// the given UUID. Returns empty GitWorktreeData for directory-mode sessions or if
// the session is not found.
func (s *Storage) GetWorktreeDataBySessionUUID(ctx context.Context, sessionUUID string) (GitWorktreeData, error) {
	return s.repo.GetWorktreeDataBySessionUUID(ctx, sessionUUID)
}

// UpdateItemSessionTriageResult stores the triage result JSON payload on an ItemSession.
func (s *Storage) UpdateItemSessionTriageResult(ctx context.Context, id string, triageResult string) error {
	return s.repo.UpdateItemSessionTriageResult(ctx, id, triageResult)
}

// UpdateItemSessionVerificationNotes stores verification evidence (commands run, manual
// checks performed) reported via request_review on an ItemSession.
func (s *Storage) UpdateItemSessionVerificationNotes(ctx context.Context, id string, verificationNotes string) error {
	return s.repo.UpdateItemSessionVerificationNotes(ctx, id, verificationNotes)
}

// UpdateItemSessionStarted records the start time for an ItemSession.
func (s *Storage) UpdateItemSessionStarted(ctx context.Context, id string, startedAt time.Time) error {
	return s.repo.UpdateItemSessionStarted(ctx, id, startedAt)
}

// SetItemSessionBaseCommit records the pre-work base commit SHA on an ItemSession.
// See the EntRepository method for why this is separate from git activity.
func (s *Storage) SetItemSessionBaseCommit(ctx context.Context, id, sha string) error {
	return s.repo.SetItemSessionBaseCommit(ctx, id, sha)
}

// UpdateItemSessionGitActivity records the session's current tip commit and
// related fields on an ItemSession. For the spawn-time baseline, use
// SetItemSessionBaseCommit.
func (s *Storage) UpdateItemSessionGitActivity(ctx context.Context, id string, sha, msg string, commitAt time.Time, commitCount int) error {
	return s.repo.UpdateItemSessionGitActivity(ctx, id, sha, msg, commitAt, commitCount)
}

// UpdateItemSessionEnded records the end time for an ItemSession.
func (s *Storage) UpdateItemSessionEnded(ctx context.Context, id string, endedAt time.Time) error {
	return s.repo.UpdateItemSessionEnded(ctx, id, endedAt)
}

// UpdateItemSessionEndedWithReason records the end time for an ItemSession alongside
// classifyHeadlessCallError's bucket (or "" for a successful end).
func (s *Storage) UpdateItemSessionEndedWithReason(ctx context.Context, id string, endedAt time.Time, reason string) error {
	return s.repo.UpdateItemSessionEndedWithReason(ctx, id, endedAt, reason)
}

// UpdateItemSessionFailureCapture records the absolute path to a durable raw-output
// capture file for a headless triage/review call that errored or produced
// unparseable output. See EntRepository.UpdateItemSessionFailureCapture.
func (s *Storage) UpdateItemSessionFailureCapture(ctx context.Context, id string, path string) error {
	return s.repo.UpdateItemSessionFailureCapture(ctx, id, path)
}

// UpdateItemSessionCost adds usd to an ItemSession's estimated_cost_usd. See
// EntRepository.UpdateItemSessionCost.
func (s *Storage) UpdateItemSessionCost(ctx context.Context, id string, usd float64) error {
	return s.repo.UpdateItemSessionCost(ctx, id, usd)
}

// AddHeadlessCostBySessionUUID adds usd to the estimated_cost_usd of the ItemSession
// for sessionUUID, if any. See EntRepository.AddHeadlessCostBySessionUUID.
func (s *Storage) AddHeadlessCostBySessionUUID(ctx context.Context, sessionUUID string, usd float64) error {
	return s.repo.AddHeadlessCostBySessionUUID(ctx, sessionUUID, usd)
}

// GetItemSessionBySessionAndItem looks up an ItemSession by both sessionUUID and backlog item ID.
// Returns ErrNotFound if no matching record exists.
func (s *Storage) GetItemSessionBySessionAndItem(ctx context.Context, sessionUUID string, itemID string) (ItemSessionSummary, error) {
	return s.repo.GetItemSessionBySessionAndItem(ctx, sessionUUID, itemID)
}

// GetClaudeConversationUUIDBySessionUUID returns the Claude conversation UUID
// for the session whose title matches the given UUID. Returns "" when the session
// has no ClaudeSession, and ErrNotFound when no session matches.
func (s *Storage) GetClaudeConversationUUIDBySessionUUID(ctx context.Context, sessionUUID string) (string, error) {
	return s.repo.GetClaudeConversationUUIDBySessionUUID(ctx, sessionUUID)
}

// GetMostRecentReviewVerdictForItem returns the OverallOutcome of the most recent
// ReviewVerdict linked to any ItemSession for itemID. Returns "" when none exists.
func (s *Storage) GetMostRecentReviewVerdictForItem(ctx context.Context, itemID string) (ReviewOutcome, error) {
	return s.repo.GetMostRecentReviewVerdictForItem(ctx, itemID)
}

// GetRecentReviewVerdictSummaries returns up to limit ReviewVerdicts for itemID,
// most recent first. Returns nil (not an error) when the repo isn't ent-backed.
func (s *Storage) GetRecentReviewVerdictSummaries(ctx context.Context, itemID string, limit int) ([]ReviewVerdictSummary, error) {
	return s.repo.GetRecentReviewVerdictSummaries(ctx, itemID, limit)
}

// SaveReviewVerdict upserts a ReviewVerdict for a given ItemSession UUID.
func (s *Storage) SaveReviewVerdict(ctx context.Context, itemSessionID string, verdict ReviewVerdictData) error {
	return s.repo.SaveReviewVerdict(ctx, itemSessionID, verdict)
}

// ComputeCurrentDiffHash resolves itemID's most recent completed work
// session's base..head commit range (via
// GetRepoPathAndLatestCompletedWorkSessionCommits — two bounded, no-edge
// queries, not GetBacklogItem/ListItemSessions' unbounded eager-loaded
// fetch) and returns a content hash of that diff (git.DiffHashBetween), for
// stamping onto a review verdict's DiffHash at save time — see
// stuck_decisions.go's IsFlakyVerdictFlipFlop, which the hash feeds.
//
// Best-effort: any resolution failure (item/session lookup, missing SHAs, a
// git error) returns "" rather than propagating an error, matching this
// codebase's "best-effort, never blocks the write it's attached to"
// convention (see e.g. SaveReviewVerdict's publish-hook comments) — a
// missing DiffHash just means IsFlakyVerdictFlipFlop treats that verdict as
// unknown, never as a false match.
func (s *Storage) ComputeCurrentDiffHash(ctx context.Context, itemID string) string {
	repoPath, baseSHA, headSHA, err := s.repo.GetRepoPathAndLatestCompletedWorkSessionCommits(ctx, itemID)
	if err != nil || repoPath == "" || baseSHA == "" || headSHA == "" {
		return ""
	}
	hash, err := git.DiffHashBetween(repoPath, baseSHA, headSHA)
	if err != nil {
		log.WarningLog().Printf("[ComputeCurrentDiffHash] item=%s base=%s head=%s: %v", itemID, baseSHA, headSHA, err)
		return ""
	}
	return hash
}

// UpdateAcCriterionStatus updates a single acceptance criterion's status by index.
func (s *Storage) UpdateAcCriterionStatus(ctx context.Context, itemID string, criterionIndex int, status string, note string) error {
	return s.repo.UpdateAcCriterionStatus(ctx, itemID, criterionIndex, status, note)
}

// AppendProgressNote records a single report_progress call as an immutable history
// entry, in addition to the current-note-per-criterion updated by UpdateAcCriterionStatus.
func (s *Storage) AppendProgressNote(ctx context.Context, itemID string, criterionIndex int, note, status string) error {
	return s.repo.AppendProgressNote(ctx, itemID, criterionIndex, note, status)
}

// ListProgressNotesForItem returns the full append-only history of report_progress
// calls for a backlog item, ordered by created_at ascending.
func (s *Storage) ListProgressNotesForItem(ctx context.Context, itemID string) ([]ProgressNoteData, error) {
	return s.repo.ListProgressNotesForItem(ctx, itemID)
}

// AppendActivityNote records a single post_backlog_update call as an immutable,
// append-only history entry — the ungated sibling to AppendProgressNote (ADR-001).
func (s *Storage) AppendActivityNote(ctx context.Context, itemID, authorSessionUUID, authorSessionTitle, message string) error {
	return s.repo.AppendActivityNote(ctx, itemID, authorSessionUUID, authorSessionTitle, message)
}

// ListActivityNotesForItem returns the full append-only activity-note history for
// a backlog item, ordered by created_at ascending.
func (s *Storage) ListActivityNotesForItem(ctx context.Context, itemID string) ([]ActivityNoteData, error) {
	return s.repo.ListActivityNotesForItem(ctx, itemID)
}

// CreateItemSession creates a new ItemSession linked to a BacklogItem.
func (s *Storage) CreateItemSession(ctx context.Context, data ItemSessionData) (ItemSessionSummary, error) {
	return s.repo.CreateItemSession(ctx, data)
}

// CreateItemSessionWithVerdict atomically creates an ItemSession and its initial
// ReviewVerdict in a single transaction. Falls back gracefully if the backend is
// not ent-based.
func (s *Storage) CreateItemSessionWithVerdict(ctx context.Context, isData ItemSessionData, verdict ReviewVerdictData) (ItemSessionSummary, error) {
	return s.repo.CreateItemSessionWithVerdict(ctx, isData, verdict)
}

// ListItemSessions returns all ItemSessions for a given BacklogItem UUID string.
func (s *Storage) ListItemSessions(ctx context.Context, itemID string) ([]ItemSessionSummary, error) {
	return s.repo.ListItemSessions(ctx, itemID)
}

// UpdateItemSessionSessionUUID updates the session_uuid on an existing ItemSession record.
func (s *Storage) UpdateItemSessionSessionUUID(ctx context.Context, id string, sessionUUID string) error {
	return s.repo.UpdateItemSessionSessionUUID(ctx, id, sessionUUID)
}

// GetAllItemSessionsWithBacklogInfo returns all item sessions joined with backlog item metadata.
// Delegates to EntRepository; returns an error for non-ent backends.
func (s *Storage) GetAllItemSessionsWithBacklogInfo(ctx context.Context) ([]ItemSessionBacklogEntry, error) {
	return s.repo.GetAllItemSessionsWithBacklogInfo(ctx)
}

// --- Session Goal ---

// SetSessionGoal upserts the goal for a session (1:1 per session_uuid).
// If a goal already exists for the session, it is replaced.
// workspaceKey, when non-empty, is stamped in the same upsert as the goal write (rather
// than a separate follow-up UPDATE) so the two never diverge on a crash between writes.
func (s *Storage) SetSessionGoal(ctx context.Context, sessionUUID string, goal string, status string, tasks []TaskNode, setBy string, workspaceKey string) (*SessionGoalData, error) {
	client := s.GetEntClient()
	if client == nil {
		return nil, fmt.Errorf("goal storage not supported by this backend")
	}
	if err := validateTasks(tasks); err != nil {
		return nil, err
	}
	tasksJSON, err := EncodeTasks(tasks)
	if err != nil {
		return nil, err
	}
	create := client.SessionGoal.Create().
		SetSessionUUID(sessionUUID).
		SetGoal(goal).
		SetStatus(status).
		SetSetBy(setBy).
		SetTasks(tasksJSON).
		SetNillableWorkspaceKey(nonEmptyPtr(workspaceKey))
	err = create.
		OnConflictColumns("session_uuid").
		Update(func(u *ent.SessionGoalUpsert) {
			u.SetGoal(goal)
			u.SetStatus(status)
			u.SetSetBy(setBy)
			u.SetTasks(tasksJSON)
			u.SetUpdatedAt(time.Now())
			if workspaceKey != "" {
				u.SetWorkspaceKey(workspaceKey)
			}
		}).
		Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to upsert session goal: %w", err)
	}
	return s.GetSessionGoal(ctx, sessionUUID)
}

// nonEmptyPtr returns nil for "" and &s otherwise, for SetNillableX ent builder calls.
func nonEmptyPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// GetSessionGoal retrieves the goal for a session by session UUID.
// Returns ErrNotFound if no goal has been set for the session.
func (s *Storage) GetSessionGoal(ctx context.Context, sessionUUID string) (*SessionGoalData, error) {
	client := s.GetEntClient()
	if client == nil {
		return nil, ErrNotFound
	}
	g, err := client.SessionGoal.Query().
		Where(sessiongoal.SessionUUID(sessionUUID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get session goal: %w", err)
	}
	tasks, decodeErr := DecodeTasks(g.Tasks)
	if decodeErr != nil {
		log.Warn("GetSessionGoal: failed to decode tasks JSON", "session_uuid", sessionUUID, "err", decodeErr)
		tasks = []TaskNode{}
	}
	return &SessionGoalData{
		UUID:        g.ID.String(),
		SessionUUID: g.SessionUUID,
		Goal:        g.Goal,
		Status:      g.Status,
		Tasks:       tasks,
		SetBy:       g.SetBy,
		UpdatedAt:   g.UpdatedAt,
	}, nil
}

// UpdateSessionTaskStatus loads the goal for a session, finds the task by ID,
// updates its status, and saves the goal back.
// Returns ErrNotFound if no goal exists, or an error if task_id is not found in the tree.
// The read-modify-write is wrapped in a transaction to prevent concurrent update races.
func (s *Storage) UpdateSessionTaskStatus(ctx context.Context, sessionUUID string, taskID string, newStatus string) (*SessionGoalData, error) {
	client := s.GetEntClient()
	if client == nil {
		return nil, fmt.Errorf("goal storage not supported by this backend")
	}

	tx, err := client.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	g, err := tx.SessionGoal.Query().
		Where(sessiongoal.SessionUUID(sessionUUID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get session goal: %w", err)
	}

	existingTasks, decodeErr := DecodeTasks(g.Tasks)
	if decodeErr != nil {
		log.Warn("UpdateSessionTaskStatus: failed to decode tasks JSON", "session_uuid", sessionUUID, "err", decodeErr)
		existingTasks = []TaskNode{}
	}

	updated, found := findAndUpdateTask(existingTasks, taskID, newStatus)
	if !found {
		err = fmt.Errorf("task %q not found in goal tree", taskID)
		return nil, err
	}

	tasksJSON, encErr := EncodeTasks(updated)
	if encErr != nil {
		err = encErr
		return nil, err
	}

	if _, saveErr := tx.SessionGoal.UpdateOne(g).
		SetTasks(tasksJSON).
		SetUpdatedAt(time.Now()).
		Save(ctx); saveErr != nil {
		err = saveErr
		return nil, fmt.Errorf("failed to save updated tasks: %w", err)
	}

	if commitErr := tx.Commit(); commitErr != nil {
		err = commitErr
		return nil, fmt.Errorf("failed to commit task status update: %w", err)
	}

	return &SessionGoalData{
		UUID:        g.ID.String(),
		SessionUUID: g.SessionUUID,
		Goal:        g.Goal,
		Status:      g.Status,
		Tasks:       updated,
		SetBy:       g.SetBy,
		UpdatedAt:   time.Now(),
	}, nil
}
