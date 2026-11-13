package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"entgo.io/ent/dialect"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent"
	entsession "github.com/tstapler/stapler-squad/session/ent/session"
	"github.com/tstapler/stapler-squad/session/ent/sessiongoal"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/hostclaim"
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

// InstanceStore is the minimal interface the server layer needs for session persistence.
// Defining it here (alongside the concrete Storage) allows test fakes to be built without
// depending on the full Storage implementation.
type InstanceStore interface {
	LoadInstances() ([]*Instance, error)
	// ListInstanceData returns raw persisted InstanceData without constructing Instance
	// objects or spawning PTY processes. Use this for read-only existence/title checks
	// where calling LoadInstances() would create unnecessary side effects.
	ListInstanceData() ([]InstanceData, error)
	// FindInstanceDataByID finds the single InstanceData whose stable ID or title
	// matches id via an indexed WHERE clause, rather than ListInstanceData() plus a
	// linear scan. Returns ErrInstanceDataNotFound when no match exists.
	FindInstanceDataByID(id string) (*InstanceData, error)
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
	Repo *EntRepository

	// nudgeGate dedupes manual-nudge recording per session before any DB read.
	nudgeGate     *MarkerLimiter
	nudgeGateOnce sync.Once

	// saved lets saveInstancesToRepo skip instances unchanged since their last save.
	saved savedFingerprints

	// claimRecorder is set once at startup but read from every creation path
	// (including background sync loops), so it is atomic.
	claimRecorder atomic.Pointer[ClaimRecorder]
	// foreignClaims is the local-only claim reader SyncOne consults; nil means
	// no cross-host claim checking.
	foreignClaims atomic.Pointer[ForeignClaimLookup]
	// provenance builds the PR provenance comment; nil means no stamping.
	provenance atomic.Pointer[PRProvenanceSource]
	// gate is the engine and denial reporter GatedTransition decides with; nil
	// means unwired and every caller gets the legacy (Off) answer.
	gate atomic.Pointer[transitionGate]

	// prMaintenance is the lifecycle of RunPRStartupMaintenance (service bootstrap only).
	prMaintenance prMaintenanceState
}

// NewStorageWithRepository creates a Storage backed by an EntRepository.
func NewStorageWithRepository(repo *EntRepository) (*Storage, error) {
	return &Storage{Repo: repo}, nil
}

// Close performs graceful shutdown of storage.
func (s *Storage) Close() error {
	return nil
}

// GetEntClient returns the *ent.Client from the underlying EntRepository.
func (s *Storage) GetEntClient() *ent.Client {
	return s.Repo.GetEntClient()
}

// MaintenanceDriver returns the instrumented driver behind the sessions database
// for session/dbmaint, or nil when the repository has none.
func (s *Storage) MaintenanceDriver() dialect.ExecQuerier {
	return s.Repo.MaintenanceDriver()
}

// SetItemChangePublisher wires p into the underlying repository.
func (s *Storage) SetItemChangePublisher(p ItemChangePublisher) {
	s.Repo.SetItemChangePublisher(p)
}

// SetClaimRecorder wires r as the recorder CreateBacklogItem calls for every
// item created with a non-empty ExternalURL. Passing nil disables recording.
func (s *Storage) SetClaimRecorder(r ClaimRecorder) {
	if r == nil {
		s.claimRecorder.Store(nil)
		return
	}
	s.claimRecorder.Store(&r)
}

// SetForeignClaimLookup wires l as the local-only claim reader ForeignClaim
// uses. Passing nil disables cross-host claim checking.
func (s *Storage) SetForeignClaimLookup(l ForeignClaimLookup) {
	if l == nil {
		s.foreignClaims.Store(nil)
		return
	}
	s.foreignClaims.Store(&l)
}

// ForeignClaim reports a claim on externalURL held by another host, consulting
// only local state. It reports false when no lookup is wired or externalURL is empty.
func (s *Storage) ForeignClaim(externalURL string) (hostclaim.ClaimRecord, bool) {
	lookup := s.foreignClaims.Load()
	if lookup == nil || externalURL == "" {
		return hostclaim.ClaimRecord{}, false
	}
	return (*lookup).ForeignClaim(externalURL)
}

// SetCallbackDispatcher forwards to the underlying *EntRepository's SetCallbackDispatcher.
func (s *Storage) SetCallbackDispatcher(d CallbackDispatcher) {
	s.Repo.SetCallbackDispatcher(d)
}

// Repository returns the backing *EntRepository, for wiring code that sits above the session package
// (for example WireChainFirer).
func (s *Storage) Repository() *EntRepository { return s.Repo }

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
		s.saveInstanceToRepo(ctx, inst)
	}
	return nil
}

// saveInstanceToRepo upserts one started instance. It holds the instance's PR
// persist lock across snapshotting and writing so a concurrent AttachPR persist
// (which snapshots under the same lock) cannot land an older mirror after this save.
func (s *Storage) saveInstanceToRepo(ctx context.Context, inst *Instance) {
	inst.prPersistMu.Lock()
	defer inst.prPersistMu.Unlock()
	data := inst.ToInstanceData()
	log.Debug("SaveInstances: converting instance",
		"session", data.Title, "is_worktree", data.IsWorktree, "main_repo_path", data.MainRepoPath,
		"github_owner", data.GitHubOwner, "github_repo", data.GitHubRepo)
	s.saved.persist(data, func(skip updateSkip) bool {
		if err := s.Repo.updateSkipping(ctx, data, skip); err != nil {
			// Not found → create it
			if createErr := s.Repo.Create(ctx, data); createErr != nil {
				log.Error("SaveInstances: failed to upsert instance",
					"session", data.Title, "update_err", err, "create_err", createErr)
				return false
			}
		}
		inst.clearPRPersistPending()
		inst.terminalClearPending.Store(false)
		return true
	})
}

// SaveInstancesSync saves instances synchronously (same as SaveInstances for the repo backend).
func (s *Storage) SaveInstancesSync(instances []*Instance) error {
	return s.saveInstancesToRepo(instances)
}

// LoadInstances loads the list of instances from the repository.
func (s *Storage) LoadInstances() ([]*Instance, error) {
	ctx := context.Background()
	dataSlice, err := s.Repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list instances from repository: %w", err)
	}
	s.hydrateSessionPRs(ctx, dataSlice)
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
		inst.SetShellRepository(s.Repo)
		inst.SetInitialPromptRepository(s.Repo)
		inst.SetPRPersister(s)
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
				var blob domain.SessionArtifactsBlob
				if err := json.Unmarshal([]byte(raw), &blob); err == nil {
					inst.Artifacts = &blob
				}
			}
		}
	}

	return instances, nil
}

// hydrateSessionPRs attaches each session's PR links to its InstanceData before
// the Instance (and so its first snapshot) is built: one bulk read of links+states
// (and one of tombstones) instead of a query per session. A legacy PR number with
// neither a link nor a tombstone is marked LegacyPRUnrepresented so full saves
// leave the legacy columns alone. If the bulk read fails, every legacy number is
// treated as unrepresented (protected) rather than guessed at.
func (s *Storage) hydrateSessionPRs(ctx context.Context, rows []InstanceData) {
	uuids := make([]string, 0, len(rows))
	for i := range rows {
		if rows[i].UUID != "" {
			uuids = append(uuids, rows[i].UUID)
		}
	}
	var links map[string][]PRView
	var tombs map[string][]PRRef
	var loadErr error
	if len(uuids) > 0 {
		if links, loadErr = s.Repo.ListBySessionUUIDs(ctx, uuids); loadErr == nil {
			tombs, loadErr = s.Repo.ListSessionTombstonesByUUIDs(ctx, uuids)
		}
		if loadErr != nil {
			log.Warn("LoadInstances: failed to bulk-load session PR links; protecting legacy PR numbers", "err", loadErr)
		}
	}
	for i := range rows {
		d := &rows[i]
		if loadErr == nil {
			d.PRs = sortedByAttach(links[d.UUID])
			d.HasPRLinks = len(d.PRs) > 0
		}
		d.LegacyPRUnrepresented = legacyPRUnrepresented(*d, tombs[d.UUID], loadErr != nil)
	}
}

// legacyPRUnrepresented: the session's scalar number is neither a link nor tombstoned.
// An unbuildable ref (no owner/repo) can never become a link, so it stays protected.
func legacyPRUnrepresented(d InstanceData, tombstones []PRRef, unknown bool) bool {
	if d.GitHubPRNumber <= 0 {
		return false
	}
	if unknown {
		return true
	}
	ref, err := sessionLegacyRef(d.GitHubOwner, d.GitHubRepo, d.GitHubHost, d.Path, d.GitHubPRNumber)
	if err != nil {
		return true
	}
	if containsRef(tombstones, ref) {
		return false
	}
	for _, v := range d.PRs {
		if v.Ref == ref {
			return false
		}
	}
	return true
}

func sortedByAttach(prs []PRView) []PRView {
	out := slices.Clone(prs)
	slices.SortStableFunc(out, func(a, b PRView) int { return a.AttachedAt.Compare(b.AttachedAt) })
	return out
}

// ListInstanceData returns raw InstanceData from the repository without constructing
// Instance objects. This avoids the side effect of FromInstanceData() calling Start()
// (which spawns PTY processes). Use for read-only existence and title checks.
func (s *Storage) ListInstanceData() ([]InstanceData, error) {
	return s.Repo.ListWithOptions(context.Background(), LoadMinimal)
}

// ListInstanceDataWithWorktree returns raw InstanceData with the Worktree edge eager-loaded.
// Use this instead of ListInstanceData whenever a read-only pass needs Worktree.WorktreePath/
// RepoPath/BranchName/BaseCommitSHA (e.g. to stat the worktree or check dirty status) — plain
// ListInstanceData uses LoadMinimal, which never populates Worktree, so any such field will
// silently read as its zero value under that call.
func (s *Storage) ListInstanceDataWithWorktree() ([]InstanceData, error) {
	return s.Repo.ListWithOptions(context.Background(), LoadOptions{LoadWorktree: true})
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

// ArchiveInstanceDataByID sets ArchivedAt and Status=Stopped directly in storage for a
// session that is not (or is no longer) resident in the live in-memory instance registry
// (ReviewQueuePoller.instances) — e.g. an old backlog work/review session left over after a
// server restart. This is the storage-only counterpart to Instance.SetArchivedAtIfNilAndStop,
// used as ArchiveSessionByUUID's fallback so terminal-transition and safety-net archival
// sweeps don't silently no-op just because a session isn't currently being polled. Read-
// modify-write on the full InstanceData row (not a partial struct), so EntRepository.Update's
// guarded-optional-field pattern can't zero out fields this call didn't intend to touch.
// Returns false (no error) if the session doesn't exist or is already archived, matching
// SetArchivedAtIfNilAndStop's CAS semantics.
//
// The read and the write are two separate, unguarded round-trips: if the session is resumed
// (added back to the live poller) between them, this can race a concurrent live-instance
// write and leave a stale snapshot in storage. Callers that can't rule this out should
// re-check FindLiveInstance after calling this and, if now live, re-save the live instance's
// current state to overwrite any stale write (see ArchiveSessionByUUID).
func (s *Storage) ArchiveInstanceDataByID(id string, at time.Time) (bool, error) {
	data, err := s.FindInstanceDataByID(id)
	if err != nil {
		if errors.Is(err, ErrInstanceDataNotFound) {
			return false, nil
		}
		return false, err
	}
	if data.ArchivedAt != nil {
		return false, nil
	}
	data.ArchivedAt = &at
	data.Status = Stopped
	// The row now differs from what the live instance last saved; the caller's
	// re-save must not be skipped as "unchanged".
	if err := s.writeRow(data.Title, func() error { return s.Repo.Update(context.Background(), *data) }); err != nil {
		return false, fmt.Errorf("update session %s: %w", id, err)
	}
	return true, nil
}

// ArchivedSessionUUIDs returns the subset of uuids whose session row is
// archived, in one query per 500 uuids. Unknown uuids are absent.
func (s *Storage) ArchivedSessionUUIDs(ctx context.Context, uuids []string) (map[string]bool, error) {
	return s.Repo.ArchivedSessionUUIDs(ctx, uuids)
}

// FindInstanceDataByID finds the InstanceData whose stable ID or title matches id, via
// an indexed WHERE clause (EntRepository.FindByIDWithOptions) rather than loading and
// linear-scanning every session row — see that method's doc comment for the CPU-profile
// finding that motivated this. Returns ErrInstanceDataNotFound when no match exists.
func (s *Storage) FindInstanceDataByID(id string) (*InstanceData, error) {
	return s.Repo.FindByIDWithOptions(context.Background(), id, LoadMinimal)
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
//
// Uses LoadMinimal.WithTags() rather than plain ListInstanceData() (LoadMinimal):
// Tags is an eager-loaded ent edge (session/ent/schema/session.go), not a plain
// column, so it comes back empty under LoadMinimal — see LoadOptions.LoadTags's
// doc comment and TestStorage_UpdateInstance's identical note.
func (s *Storage) ListSessionRecords() []tokens.SessionRecord {
	// LoadWorktree so ActiveDir() below can resolve to the worktree path —
	// Claude's JSONL ProjectPath is derived from the process cwd (the
	// worktree), so matching against the identity Path orphans every
	// worktree session's token records.
	data, err := s.Repo.ListWithOptions(context.Background(), LoadOptions{LoadTags: true, LoadWorktree: true})
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
			Path:           d.ActiveDir(),
			CreatedAt:      d.CreatedAt,
			Tags:           d.Tags,
		})
	}
	return records
}

// DeleteInstance removes an instance from storage.
func (s *Storage) DeleteInstance(title string) error {
	s.saved.forget(title)
	err := s.Repo.Delete(context.Background(), title)
	s.saved.forget(title) // an in-flight save may have re-recorded the title before the row went away
	clearLoggedMissingWorktree(title)
	return err
}

// ErrTitleConflict is returned by AddInstance when the title's unique
// constraint is violated by a genuinely different instance (see its doc
// comment) rather than an idempotent re-save of the same one.
var ErrTitleConflict = errors.New("session with this title already exists")

// AddInstance adds a new instance to storage.
// Unlike SaveInstances, this does not require instance.Started() to be true.
func (s *Storage) AddInstance(instance *Instance) error {
	data := instance.ToInstanceData()
	ctx := context.Background()
	s.saved.forget(data.Title)
	defer s.saved.forget(data.Title) // an in-flight save may re-record the old fingerprint before the row lands
	if err := s.Repo.Create(ctx, data); err != nil {
		if !ent.IsConstraintError(err) {
			return fmt.Errorf("failed to persist session %q: %w", data.Title, err)
		}
		// Unique constraint violation on title. This is a legitimate
		// idempotent re-save only when the existing row is the SAME session
		// (matched by UUID, following the same convention as legacy rows
		// persisted before the uuid field existed, which share the
		// zero-value ""): a caller re-adding an Instance object it already
		// owns. When the UUIDs are both non-empty and differ, a second,
		// distinct instance (always freshly assigned a UUID by NewInstance)
		// is racing to create a session under an already-taken title --
		// updating in that case would silently steal/overwrite the first
		// instance's persisted row with the second's data instead of
		// failing. See
		// TestCreateSession_should_RejectSecondDuplicate_When_TwoRapidCallsShareTitle
		// (server/services/session_service_test.go), the regression this
		// guards.
		existingData, getErr := s.Repo.Get(ctx, data.Title)
		if getErr != nil || existingData.UUID != data.UUID {
			return fmt.Errorf("%w: %q", ErrTitleConflict, data.Title)
		}
		if updateErr := s.Repo.Update(ctx, data); updateErr != nil {
			return updateErr
		}
	}
	// Inject shell repository so shell operations can persist to the DB.
	instance.SetShellRepository(s.Repo)
	instance.SetInitialPromptRepository(s.Repo)
	instance.SetPRPersister(s)
	return nil
}

// CreateInstanceData persists a plain InstanceData record directly, bypassing
// Instance/ToInstanceData's actor synchronization entirely.
//
// Do not call this for a session that has (or will have) a live *Instance/
// actor attached — it can race or diverge from that actor's own writes. Only
// for callers building a DB-only fixture row with no backing tmux process
// (e.g. e2e debug seed handlers). Use AddInstance for anything with a live
// Instance.
func (s *Storage) CreateInstanceData(ctx context.Context, data InstanceData) error {
	return s.writeRow(data.Title, func() error { return s.Repo.Create(ctx, data) })
}

// UpdateInstance updates an existing instance in storage.
func (s *Storage) UpdateInstance(instance *Instance) error {
	data := instance.ToInstanceData()
	return s.writeRow(data.Title, func() error { return s.Repo.Update(context.Background(), data) })
}

// UpdateInstanceMetadata persists a narrow set of session metadata fields (title rename,
// category, note, working dir) via a single UPDATE, avoiding the full-row rewrite (and
// worktree/diffstats/tags/claude_session churn) that SaveInstances performs for every
// started session. currentTitle must be the title from before any rename already applied
// in-memory by the caller — see EntRepository.UpdateSessionMetadata for why. A nil field
// pointer leaves that field untouched.
func (s *Storage) UpdateInstanceMetadata(currentTitle string, newTitle, category, note, workingDir *string) error {
	s.saved.forget(currentTitle)
	err := s.Repo.UpdateSessionMetadata(context.Background(), currentTitle, newTitle, category, note, workingDir)
	s.saved.forget(currentTitle) // an in-flight save may have re-recorded it before the rename landed
	return err
}

// DeleteAllInstances removes all stored instances.
func (s *Storage) DeleteAllInstances() error {
	ctx := context.Background()
	dataSlice, err := s.Repo.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list instances for deletion: %w", err)
	}
	for _, data := range dataSlice {
		s.saved.forget(data.Title)
		if err := s.Repo.Delete(ctx, data.Title); err != nil {
			log.Warn("failed to delete instance", "session", data.Title, "err", err)
		}
		clearLoggedMissingWorktree(data.Title)
	}
	return nil
}

// UpdateInstanceTimestampsOnly updates ONLY the timestamp fields in storage without
// creating Instance objects. This preserves in-memory state like controllers.
// This is critical for WebSocket terminal streaming which updates timestamps frequently.
func (s *Storage) UpdateInstanceTimestampsOnly(title string, lastTerminalUpdate, lastMeaningfulOutput time.Time, lastOutputSignature string, lastViewed time.Time) error {
	if err := s.Repo.UpdateTimestamps(context.Background(), title, lastTerminalUpdate, lastMeaningfulOutput, lastOutputSignature); err != nil {
		return err
	}
	if !lastViewed.IsZero() {
		return s.Repo.UpdateLastViewed(context.Background(), title, lastViewed)
	}
	return nil
}

// UpdateInstanceLastAddedToQueue updates ONLY the LastAddedToQueue field for a specific instance.
func (s *Storage) UpdateInstanceLastAddedToQueue(title string, lastAddedToQueue time.Time) error {
	return s.Repo.UpdateLastAddedToQueue(context.Background(), title, lastAddedToQueue)
}

// UpdateInstanceInitialPromptSentAt persists when InitialPrompt was actually typed
// into the terminal, so a service restart doesn't have to re-derive (and risk
// getting wrong) whether it was already sent -- see Instance.InitialPromptSentAt.
func (s *Storage) UpdateInstanceInitialPromptSentAt(title string, t time.Time) error {
	return s.Repo.UpdateInitialPromptSentAt(context.Background(), title, t)
}

// UpdateInstanceLastUserResponse persists the LastUserResponse timestamp for a session.
// Uses a direct UPDATE (no read round-trip) that touches only last_user_response;
// UpdateReviewQueueState would also clear processing_grace_until.
func (s *Storage) UpdateInstanceLastUserResponse(title string, lastUserResponse time.Time) error {
	return s.Repo.UpdateLastUserResponse(context.Background(), title, lastUserResponse)
}

// UpdateInstanceAcknowledged sets the LastAcknowledged timestamp to now for a specific instance.
// Used by AcknowledgeSession when the instance is not available in the live poller.
func (s *Storage) UpdateInstanceAcknowledged(title string) error {
	return s.writeRow(title, func() error {
		return s.Repo.UpdateLastAcknowledged(context.Background(), title, time.Now())
	})
}

// UpdateInstanceProcessingGrace persists the ProcessingGraceUntil timestamp.
// Uses a direct UPDATE (no read round-trip) via UpdateReviewQueueState.
func (s *Storage) UpdateInstanceProcessingGrace(title string, processingGraceUntil time.Time) error {
	return s.Repo.UpdateReviewQueueState(context.Background(), title, time.Time{}, processingGraceUntil, time.Time{}, "")
}

// UpdateInstancePRStatus updates the PR status fields for a specific instance.
// Most PR fields are not stored in the ent schema — they live in memory and are
// re-populated by PRStatusPoller on each poll cycle, so no DB write is needed for
// them. terminal is the one exception: SessionRetentionSweeper.baseSafeToDelete
// reads it back from storage (not the live Instance) to decide whether a
// PR-linked archived session is safe to delete, so it must survive a restart or
// every such session is blocked from deletion forever (session-retention-cleanup).
func (s *Storage) UpdateInstancePRStatus(title, _, _, _ string, _, _ int, _, terminal bool) error {
	if !terminal {
		return nil
	}
	return s.writeRow(title, func() error {
		return s.Repo.UpdateGitHubPRStatusTerminal(context.Background(), title, terminal)
	})
}

// UpdateGitHubPRStatusTerminal writes the stored terminal flag of one session by
// title. Unlike UpdateInstancePRStatus it also writes false, which the terminal
// scan needs to clear a merged PR's flag that would otherwise ride onto a new PR.
func (s *Storage) UpdateGitHubPRStatusTerminal(title string, terminal bool) error {
	return s.writeRow(title, func() error {
		return s.Repo.UpdateGitHubPRStatusTerminal(context.Background(), title, terminal)
	})
}

// UpdateInstancePRNumber persists the discovered PR number for a session so it
// survives restarts and avoids repeated branch-name lookups in PRStatusPoller.
func (s *Storage) UpdateInstancePRNumber(title string, prNumber int) error {
	return s.writeRow(title, func() error {
		return s.Repo.UpdateGitHubPRNumber(context.Background(), title, prNumber)
	})
}

// AttachSessionPR writes one PR link, its state row (insert-if-absent) and the
// session's legacy mirror columns in a single transaction, keyed by session uuid.
// See EntRepository.AttachSessionPR for the error contract.
func (s *Storage) AttachSessionPR(ctx context.Context, sessionUUID string, ref PRRef, link PRLink, mirror PRMirror) (PRLinkResult, error) {
	res, err := s.Repo.AttachSessionPR(ctx, sessionUUID, ref, link, mirror)
	if err == nil {
		s.saved.reset() // the row changed behind the save fingerprints
	}
	return res, err
}

// PersistPRMirror writes (or clears) the legacy scalar PR columns of one session.
func (s *Storage) PersistPRMirror(sessionUUID, url string, number int, terminal bool) error {
	err := s.Repo.PersistPRMirror(context.Background(), sessionUUID, PRMirror{URL: url, Number: number, Terminal: terminal})
	if err == nil {
		s.saved.reset()
	}
	return err
}

// DetachSessionPR deletes the session's link to ref and writes its tombstone in
// one transaction (the user's "Not mine").
func (s *Storage) DetachSessionPR(ctx context.Context, sessionUUID string, ref PRRef) error {
	if sessionUUID == "" {
		return ErrNoSessionUUID
	}
	_, err := s.Repo.DeleteSessionPR(ctx, SessionPROwner(sessionUUID), ref)
	if err == nil {
		s.saved.reset()
	}
	return err
}

// AttachItemPR writes an item-direct PR link (agent-reported, Jules, manual). A
// tombstoned PR is not attached (PRLinkResult.Tombstoned).
func (s *Storage) AttachItemPR(ctx context.Context, itemID string, ref PRRef, link PRLink) (PRLinkResult, error) {
	if itemID == "" {
		return PRLinkResult{}, ErrInvalidPROwner
	}
	res, err := s.Repo.UpsertSessionPR(ctx, ItemPROwner(itemID), ref, link)
	if err == nil {
		s.saved.reset()
	}
	return res, err
}

// DetachItemPR deletes the item-direct link to ref and writes the item's
// tombstone in one transaction; it reports whether a link existed. Session links
// are untouched.
func (s *Storage) DetachItemPR(ctx context.Context, itemID string, ref PRRef) (bool, error) {
	if itemID == "" {
		return false, ErrInvalidPROwner
	}
	found, err := s.Repo.DeleteSessionPR(ctx, ItemPROwner(itemID), ref)
	if err == nil {
		s.saved.reset()
	}
	return found, err
}

// SetSessionPRPinned pins or unpins one of the session's PRs; pinning clears the others.
func (s *Storage) SetSessionPRPinned(ctx context.Context, sessionUUID string, ref PRRef, pinned bool) error {
	if sessionUUID == "" {
		return ErrNoSessionUUID
	}
	err := s.Repo.SetPinned(ctx, SessionPROwner(sessionUUID), ref, pinned)
	if err == nil {
		s.saved.reset()
	}
	return err
}

// ClearSessionPRTombstone removes the session's tombstone for ref, if any.
func (s *Storage) ClearSessionPRTombstone(ctx context.Context, sessionUUID string, ref PRRef) error {
	if sessionUUID == "" {
		return ErrNoSessionUUID
	}
	return s.Repo.ClearTombstone(ctx, SessionPROwner(sessionUUID), ref)
}

// ListPRMirrors returns the stored legacy PR columns by session uuid.
func (s *Storage) ListPRMirrors(ctx context.Context) (map[string]PRMirror, error) {
	return s.Repo.ListPRMirrors(ctx)
}

// UpdatePullRequestState applies one poll observation to a PR state row. Invariant I1:
// the PR status poller is its only production caller.
func (s *Storage) UpdatePullRequestState(ctx context.Context, ref PRRef, obs PRObservation) (PRObservationResult, error) {
	return s.Repo.UpdatePullRequestState(ctx, ref, obs)
}

// ListPollableStates is the PR poller's work list (see EntRepository.ListPollableStates).
func (s *Storage) ListPollableStates(ctx context.Context) ([]PRView, error) {
	return s.Repo.ListPollableStates(ctx)
}

// GetPullRequestState returns the state columns of one PR.
func (s *Storage) GetPullRequestState(ctx context.Context, ref PRRef) (PRView, error) {
	return s.Repo.GetPullRequestState(ctx, ref)
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
	return s.writeRow(title, func() error {
		return s.Repo.UpdateSessionArtifacts(context.Background(), title, blob)
	})
}

// GetInstanceArtifacts loads the raw JSON-encoded artifact blob for a session.
// Returns ("", nil) if the session exists but has no artifacts yet.
func (s *Storage) GetInstanceArtifacts(title string) (string, error) {
	return s.Repo.GetSessionArtifacts(context.Background(), title)
}

// UpdateInstanceIfEpoch performs a single ent bulk conditional UPDATE of status
// and failure_reason, gated on the persisted row's creation_epoch still matching
// capturedEpoch (Epic 1.2, ADR-002's "durable-first terminal write" addendum —
// see commitTerminalStatus in server/services, which calls this before touching
// any in-memory actor state). Returns applied = (affected rows == 1): false
// means either no row matched id, or the row's creation_epoch had already moved
// past capturedEpoch (a cancel or retry beat this caller to the database, at the
// database's own authoritative view — not just the in-process actor's).
//
// id is matched against both uuid and title, mirroring InstanceData.MatchesID's
// stable-ID-with-title-fallback convention used elsewhere in this file (e.g.
// FindInstanceDataByID).
func (s *Storage) UpdateInstanceIfEpoch(ctx context.Context, id string, capturedEpoch uint64, status Status, failureReason string) (bool, error) {
	client := s.GetEntClient()
	if client == nil {
		return false, fmt.Errorf("UpdateInstanceIfEpoch not supported by this backend")
	}
	titles := s.titlesMatchingID(ctx, client, id)
	defer s.forgetAll(titles)
	s.forgetAll(titles)
	n, err := client.Session.Update().
		Where(
			entsession.Or(entsession.UUID(id), entsession.Title(id)),
			entsession.CreationEpoch(capturedEpoch),
		).
		SetStatus(int(status)).
		SetFailureReason(failureReason).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to conditionally update instance %s: %w", id, err)
	}
	return n == 1, nil
}

// GetAllInstanceArtifacts returns a map of title → raw artifacts JSON for all sessions
// that have stored artifacts. Single bulk query (M-4 fix).
func (s *Storage) GetAllInstanceArtifacts() (map[string]string, error) {
	return s.Repo.GetAllSessionArtifacts(context.Background())
}

// GetSessionArtifactsByUUID loads and parses the artifact blob for the session
// identified by its stable UUID. Returns (nil, nil) when the session exists but
// has no stored artifacts.
func (s *Storage) GetSessionArtifactsByUUID(ctx context.Context, sessionUUID string) (*domain.SessionArtifactsBlob, error) {
	return s.Repo.GetSessionArtifactsByUUID(ctx, sessionUUID)
}

// --- Session-first convenience methods (Task 2.5) ---
// These delegate directly to the Repository's Session-based methods.
// Prefer these over the deprecated InstanceData-based methods for new code.

// GetSession retrieves a session by title using the Session domain model.
// Use ContextOptions presets (ContextMinimal, ContextUIView, etc.) to control what is loaded.
func (s *Storage) GetSession(ctx context.Context, title string, opts ContextOptions) (*Session, error) {
	return s.Repo.GetSession(ctx, title, opts)
}

// ListSessions retrieves all sessions using the Session domain model.
// Use ContextOptions presets (ContextMinimal, ContextUIView, etc.) to control what is loaded.
func (s *Storage) ListSessions(ctx context.Context, opts ContextOptions) ([]*Session, error) {
	return s.Repo.ListSessions(ctx, opts)
}

// SaveSession upserts a session using the Session domain model.
// If the session exists it is updated; otherwise it is created.
// Deprecated InstanceData-based methods (SaveInstances, LoadInstances) remain for backward compatibility.
func (s *Storage) SaveSession(ctx context.Context, session *Session) error {
	if err := s.Repo.UpdateSession(ctx, session); err != nil {
		return s.Repo.CreateSession(ctx, session)
	}
	return nil
}

// --- Permissions & Analytics --------------------------------------------------

// AllRules returns all auto-approval rules from the repository.
func (s *Storage) AllRules(ctx context.Context) ([]ApprovalRuleData, error) {
	return s.Repo.AllRules(ctx)
}

// UpsertRule creates or updates an auto-approval rule in the repository.
func (s *Storage) UpsertRule(ctx context.Context, rule ApprovalRuleData) error {
	return s.Repo.UpsertRule(ctx, rule)
}

// DeleteRule removes an auto-approval rule from the repository.
func (s *Storage) DeleteRule(ctx context.Context, id string) error {
	return s.Repo.DeleteRule(ctx, id)
}

// AllTaggingRules returns all tagging rules from the repository.
func (s *Storage) AllTaggingRules(ctx context.Context) ([]TaggingRuleData, error) {
	return s.Repo.AllTaggingRules(ctx)
}

// UpsertTaggingRule creates or updates a tagging rule in the repository.
func (s *Storage) UpsertTaggingRule(ctx context.Context, rule TaggingRuleData) error {
	return s.Repo.UpsertTaggingRule(ctx, rule)
}

// DeleteTaggingRule removes a tagging rule from the repository.
func (s *Storage) DeleteTaggingRule(ctx context.Context, id string) error {
	return s.Repo.DeleteTaggingRule(ctx, id)
}

// RecordTaggingRuleFire records that a tagging rule matched at the given instant.
func (s *Storage) RecordTaggingRuleFire(ctx context.Context, ruleID string, firedAt time.Time) error {
	return s.Repo.RecordTaggingRuleFire(ctx, ruleID, firedAt)
}

// GetTaggingRuleFireCounts returns the number of recorded fires per rule ID since the
// given instant.
func (s *Storage) GetTaggingRuleFireCounts(ctx context.Context, since time.Time) (map[string]int, error) {
	return s.Repo.GetTaggingRuleFireCounts(ctx, since)
}

// DismissFinding persists a WasteFinding dismissal in the repository.
func (s *Storage) DismissFinding(ctx context.Context, data DismissedFindingData) error {
	return s.Repo.DismissFinding(ctx, data)
}

// ListDismissedFindingIDs returns the set of currently-dismissed finding_id values.
func (s *Storage) ListDismissedFindingIDs(ctx context.Context) (map[string]bool, error) {
	return s.Repo.ListDismissedFindingIDs(ctx)
}

// RecordAnalytics logs a classification decision to the repository.
func (s *Storage) RecordAnalytics(ctx context.Context, data AnalyticsData) error {
	return s.Repo.RecordAnalytics(ctx, data)
}

// RecordAnalyticsBatch writes one atomic, idempotent analytics batch without
// widening the broad Repository interface used by unrelated adapters.
func (s *Storage) RecordAnalyticsBatch(ctx context.Context, batch []AnalyticsData) error {
	return s.Repo.RecordAnalyticsBatch(ctx, batch)
}

// ListAnalytics retrieves recent classification decisions from the repository.
func (s *Storage) ListAnalytics(ctx context.Context, limit int) ([]AnalyticsData, error) {
	return s.Repo.ListAnalytics(ctx, limit)
}

// ListAnalyticsSince retrieves analytics entries with created_at >= since.
func (s *Storage) ListAnalyticsSince(ctx context.Context, since time.Time, limit int) ([]AnalyticsData, error) {
	return s.Repo.ListAnalyticsSince(ctx, since, limit)
}

// ListAnalyticsByProgramSince retrieves entries for a specific program since a time.
func (s *Storage) ListAnalyticsByProgramSince(ctx context.Context, program string, since time.Time, limit int) ([]AnalyticsData, error) {
	return s.Repo.ListAnalyticsByProgramSince(ctx, program, since, limit)
}

// GetSubcommandBreakdown returns per-(subcommand, decision) counts for a program.
func (s *Storage) GetSubcommandBreakdown(ctx context.Context, program string, since time.Time) ([]SubcommandDecisionCount, error) {
	return s.Repo.GetSubcommandBreakdown(ctx, program, since)
}

// ListRecentCommandsByProgram returns the most recent n command_preview strings.
func (s *Storage) ListRecentCommandsByProgram(ctx context.Context, program, subcommand string, since time.Time, n int) ([]string, error) {
	return s.Repo.ListRecentCommandsByProgram(ctx, program, subcommand, since, n)
}

// GetSubcommandTrend returns raw analytics rows for (program, subcommand) since a time.
func (s *Storage) GetSubcommandTrend(ctx context.Context, program, subcommand string, since time.Time) ([]AnalyticsData, error) {
	return s.Repo.GetSubcommandTrend(ctx, program, subcommand, since)
}

// --- Projects ---

// CreateProject inserts a new project into storage.
func (s *Storage) CreateProject(ctx context.Context, data ProjectData) (*ProjectData, error) {
	return s.Repo.CreateProject(ctx, data)
}

// ListProjects returns all projects from storage.
func (s *Storage) ListProjects(ctx context.Context) ([]ProjectData, error) {
	return s.Repo.ListProjects(ctx)
}

// UpdateProject modifies an existing project in storage.
func (s *Storage) UpdateProject(ctx context.Context, data ProjectData) (*ProjectData, error) {
	return s.Repo.UpdateProject(ctx, data)
}

// DeleteProject removes a project from storage (sessions are unassigned).
func (s *Storage) DeleteProject(ctx context.Context, name string) error {
	s.saved.reset() // unassigns an unknown set of sessions
	err := s.Repo.DeleteProject(ctx, name)
	s.saved.reset()
	return err
}

// AssignSessionsToProject links sessions to a project in storage.
func (s *Storage) AssignSessionsToProject(ctx context.Context, projectName string, sessionTitles []string) error {
	s.saved.reset() // ids may be titles or uuids; project ops are rare
	err := s.Repo.AssignSessionsToProject(ctx, projectName, sessionTitles)
	s.saved.reset()
	return err
}

// --- Backlog ---

// CreateBacklogItem inserts a new backlog item.
func (s *Storage) CreateBacklogItem(ctx context.Context, data backlogdto.BacklogItemData) (*backlogdto.BacklogItemData, error) {
	// Every creation path (create_backlog_item, import_github_issue, the web
	// UI's RPC handlers) funnels through here, so this is the one place that
	// guarantees every item's RepoPath is canonicalized regardless of which
	// caller filed it. Without it, two items that both target the same repo —
	// one filed with repo_path pointing at the main checkout, another filed by
	// an agent that passed its own in-progress worktree — end up with two
	// different RepoPath strings, fragmenting the web UI's "group by
	// repository" view into one bucket per worktree instead of one per repo.
	// Best-effort and non-fatal: falls through with RepoPath unchanged if
	// resolution fails (e.g. it isn't a git repo yet) or is empty (repo-less
	// item). Gated on filepath.IsAbs: ResolveMainRepoRoot's first step
	// (ResolveSessionPath) calls filepath.Abs, which would silently turn a
	// caller's mistaken relative/bare-slug RepoPath into a resolved-looking
	// absolute path — masking TriggerTriage's own "repo_path must be
	// absolute" validation instead of letting it reject the input as
	// intended (see TestTriggerTriage_should_RejectRelativeRepoPath_Before_CreatingAnyItemSession).
	if data.RepoPath != "" && filepath.IsAbs(data.RepoPath) {
		if resolved, err := ResolveMainRepoRoot(data.RepoPath); err == nil {
			data.RepoPath = resolved
		}
	}
	if data.ExternalURL != "" && data.ClaimingHostID == "" {
		if rec := s.claimRecorder.Load(); rec != nil {
			if p, ok := (*rec).(localHostIDProvider); ok {
				data.ClaimingHostID = p.LocalHostID()
			}
		}
	}
	created, err := s.Repo.CreateBacklogItem(ctx, data)
	if err != nil {
		return nil, err
	}
	s.recordClaim(ctx, created)
	return created, nil
}

// CreateBacklogItemForDebug is CreateBacklogItem for the operator-only debug
// seed and mutate endpoints, which may create an item at any status. It is
// counted as workflow_engine_bypass_total{caller=debug_create}; every other
// creator is held to idea (or no status) by the bypass inventory test
// (ADR-007 D1), which is why CreateBacklogItem itself rejects nothing.
func (s *Storage) CreateBacklogItemForDebug(ctx context.Context, data backlogdto.BacklogItemData) (*backlogdto.BacklogItemData, error) {
	created, err := s.CreateBacklogItem(ctx, data)
	if err == nil {
		RecordEngineBypass(CallerDebugCreate)
	}
	return created, err
}

// recordClaim is the single choke point (ADR-002 of
// project_plans/cross-host-claim-dedup/decisions/) that records a cross-host
// claim for every newly created item that has an ExternalURL. Best-effort: a
// failure is logged and never fails item creation.
func (s *Storage) recordClaim(ctx context.Context, item *backlogdto.BacklogItemData) {
	recorder := s.claimRecorder.Load()
	if recorder == nil || item == nil || item.ExternalURL == "" {
		return
	}
	err := (*recorder).RecordClaim(ctx, hostclaim.ClaimRecord{
		ExternalURL:  item.ExternalURL,
		ItemDeepLink: BacklogItemDeepLinkPath(item),
		ClaimedAt:    item.CreatedAt,
	})
	if err != nil {
		log.Warn("claim_index.record_failed", "external_url", item.ExternalURL, "err", err)
	}
}

// GetBacklogItem retrieves a backlog item by UUID string.
func (s *Storage) GetBacklogItem(ctx context.Context, id string) (*backlogdto.BacklogItemData, error) {
	return s.Repo.GetBacklogItem(ctx, id)
}

// GetBacklogItemByExternalURL retrieves a backlog item previously imported
// from externalURL (e.g. a GitHub issue URL), or ErrNotFound if none exists.
func (s *Storage) GetBacklogItemByExternalURL(ctx context.Context, externalURL string) (*backlogdto.BacklogItemData, error) {
	return s.Repo.GetBacklogItemByExternalURL(ctx, externalURL)
}

// ListBacklogItems returns backlog items with optional filtering.
func (s *Storage) ListBacklogItems(ctx context.Context, filter backlogdto.BacklogItemFilter) ([]backlogdto.BacklogItemData, error) {
	return s.Repo.ListBacklogItems(ctx, filter)
}

// ListBacklogItemSummaries returns lightweight summaries for list views.
func (s *Storage) ListBacklogItemSummaries(ctx context.Context, filter backlogdto.BacklogItemFilter) ([]backlogdto.BacklogItemSummary, error) {
	return s.Repo.ListBacklogItemSummaries(ctx, filter)
}

// AddBacklogItemDependency records a blocker/blocked dependency edge.
func (s *Storage) AddBacklogItemDependency(ctx context.Context, edge backlogdto.BacklogItemDependencyEdge) error {
	return s.Repo.AddBacklogItemDependency(ctx, edge)
}

// RemoveBacklogItemDependency deletes a blocker/blocked edge (idempotent).
func (s *Storage) RemoveBacklogItemDependency(ctx context.Context, edge backlogdto.BacklogItemDependencyEdge) error {
	return s.Repo.RemoveBacklogItemDependency(ctx, edge)
}

// ListBacklogItemDependencies returns the blocked-by and blocking refs for an item.
func (s *Storage) ListBacklogItemDependencies(ctx context.Context, itemID string) (blockedBy, blocking []backlogdto.BacklogItemDependencyRef, err error) {
	return s.Repo.ListBacklogItemDependencies(ctx, itemID)
}

// UnresolvedBlockersForItems returns unresolved blocker refs per blocked item (batched).
func (s *Storage) UnresolvedBlockersForItems(ctx context.Context, itemIDs []string) (map[string][]backlogdto.BacklogItemDependencyRef, error) {
	return s.Repo.UnresolvedBlockersForItems(ctx, itemIDs)
}

// UnresolvedBlockerItemIDs returns the subset of itemIDs blocked by an unresolved dependency.
func (s *Storage) UnresolvedBlockerItemIDs(ctx context.Context, itemIDs []string) (map[string]bool, error) {
	return s.Repo.UnresolvedBlockerItemIDs(ctx, itemIDs)
}

// UnresolvedBlockerIDs returns the specific blocker item IDs still unresolved for a single item.
func (s *Storage) UnresolvedBlockerIDs(ctx context.Context, itemID string) ([]string, error) {
	return s.Repo.UnresolvedBlockerIDs(ctx, itemID)
}

// UpdateBacklogItem modifies an existing backlog item.
func (s *Storage) UpdateBacklogItem(ctx context.Context, id string, update backlogdto.BacklogItemUpdate, precondition *backlogdto.BacklogItemPrecondition) (*backlogdto.BacklogItemData, error) {
	return s.Repo.UpdateBacklogItem(ctx, id, update, precondition)
}

// RawArchiveBacklogItem sets the archived_at timestamp and status. See
// EntRepository.ArchiveBacklogItem's doc comment for precondition/triggeredBy/note.
func (s *Storage) RawArchiveBacklogItem(ctx context.Context, id string, precondition *backlogdto.BacklogItemPrecondition, triggeredBy, note string) (*backlogdto.BacklogItemData, error) {
	return s.Repo.rawArchiveBacklogItem(ctx, id, precondition, triggeredBy, note)
}

// RawUnarchiveBacklogItem clears archived_at and restores the item to "idea".
func (s *Storage) RawUnarchiveBacklogItem(ctx context.Context, id string) (*backlogdto.BacklogItemData, error) {
	return s.Repo.rawUnarchiveBacklogItem(ctx, id, "")
}

// DeleteBacklogItem permanently removes an item and all its child records.
func (s *Storage) DeleteBacklogItem(ctx context.Context, id string) error {
	return s.Repo.DeleteBacklogItem(ctx, id)
}

// RawTransitionBacklogItemStatus changes the status of a backlog item.
func (s *Storage) RawTransitionBacklogItemStatus(ctx context.Context, id string, toStatus BacklogStatus, precondition *backlogdto.BacklogItemPrecondition, triggeredBy string) (*backlogdto.BacklogItemData, error) {
	return s.Repo.RawTransitionBacklogItemStatus(ctx, id, toStatus, precondition, triggeredBy)
}

// RawSetBacklogItemPRAndTransition is the shared primary-write path for
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
func (s *Storage) RawSetBacklogItemPRAndTransition(ctx context.Context, observed *backlogdto.BacklogItemData, prURL string, prNumber int, summary string, guard *backlogdto.PRReassignmentGuard) error {
	return s.setPRAndTransition(ctx, observed, PRAssociation{URL: prURL, Number: prNumber, Summary: summary, Guard: guard})
}

// setPRAndTransition is the single-UPDATE write shared by
// RawSetBacklogItemPRAndTransition and GatedSetPRAndTransition; the contract
// is documented on the former.
func (s *Storage) setPRAndTransition(ctx context.Context, observed *backlogdto.BacklogItemData, pr PRAssociation) error {
	prURL, prNumber, summary, guard := pr.URL, pr.Number, pr.Summary, pr.Guard
	if observed.Status == string(BacklogStatusPRPending) && observed.PrNumber == prNumber && prNumber > 0 {
		return nil // already recorded — idempotent no-op
	}
	if pr.Additive && observed.Status == string(BacklogStatusPRPending) {
		return s.attachAdditivePR(ctx, observed, pr)
	}
	if observed.Status != string(BacklogStatusReview) && observed.Status != string(BacklogStatusPRPending) {
		return fmt.Errorf("%w: expected status %q or %q, got %q", domain.ErrPreconditionFailed, BacklogStatusReview, BacklogStatusPRPending, observed.Status)
	}
	isReassignment := observed.Status == string(BacklogStatusPRPending)
	if isReassignment {
		switch {
		case guard == nil:
			return fmt.Errorf("%w: item already has PR #%d tracked (status pr_pending) — this caller did not supply a PRReassignmentGuard", backlogdto.ErrPRReassignmentNotAllowed, observed.PrNumber)
		case guard.OverrideReason == "":
			return fmt.Errorf("%w: override_reason is required to reassign PR #%d to a different PR", backlogdto.ErrPRReassignmentNotAllowed, observed.PrNumber)
		case guard.CurrentPRMerged:
			return fmt.Errorf("%w: currently tracked PR #%d is already merged — its association cannot be changed", backlogdto.ErrPRReassignmentNotAllowed, observed.PrNumber)
		case !guard.NewPRAuthorVerified:
			return fmt.Errorf("%w: the new PR's author must be verified to match the caller's identity before reassigning PR #%d", backlogdto.ErrPRReassignmentNotAllowed, observed.PrNumber)
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
	precondition := &backlogdto.BacklogItemPrecondition{
		ExpectedStatus:    observed.Status,
		ExpectedUpdatedAt: &expectedUpdatedAt,
		Note:              fmt.Sprintf("[%s] %s", noteLabel, summary),
	}
	var replaced *PRRef
	if isReassignment {
		if old, perr := ParseItemPRRef(observed.PrURL, observed.PrNumber); perr == nil {
			replaced = &old
		}
	}
	// The item-direct link is written for every item PR writer, flag on or off, in the
	// same transaction as the pr_number write.
	if _, err := s.Repo.rawTransitionBacklogItemStatusWithPRFields(ctx, observed.ID, BacklogStatusPRPending, prURL, prNumber, precondition, TriggeredBySystem, newItemPRLinkWrite(pr, replaced)); err != nil {
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
		if _, clearErr := s.UpdateBacklogItem(ctx, observed.ID, backlogdto.BacklogItemUpdate{ClearPrFeedbackAddressedAt: true}, nil); clearErr != nil {
			log.WarningLog().Printf("[Storage] SetBacklogItemPRAndTransition: failed to clear pr_feedback_addressed_at item=%s: %v", observed.ID, clearErr)
		}
	}

	return nil
}

// SetItemPRFieldsWithLink writes pr_url/pr_number and the item-direct link atomically.
func (s *Storage) SetItemPRFieldsWithLink(ctx context.Context, itemID, prURL string, prNumber int, source AttachSource) error {
	return s.Repo.SetItemPRFieldsWithLink(ctx, itemID, prURL, prNumber, source)
}

// attachAdditivePR records pr as a follow-up PR of an item already in pr_pending: it
// adds the item-direct link (tombstone-checked), then lets the item's pr_number/pr_url
// mirror follow the derived active PR. The status never changes and no override_reason
// or reassignment guard applies. If the mirror recompute fails, the next
// ReconcilePRPending pass repairs it.
func (s *Storage) attachAdditivePR(ctx context.Context, observed *backlogdto.BacklogItemData, pr PRAssociation) error {
	ref, err := ParseItemPRRef(pr.URL, pr.Number)
	if err != nil {
		return fmt.Errorf("additive PR: %w", err)
	}
	src := pr.Source
	if src == "" {
		src = AttachSourceReportPRCreated
	}
	res, err := s.Repo.UpsertSessionPR(ctx, ItemPROwner(observed.ID), ref, PRLink{
		Role: PRRoleFollowup, Source: src, Initial: PRInitial{State: PRStateOpen, URL: pr.URL},
	})
	if err != nil {
		return fmt.Errorf("additive PR: record link: %w", err)
	}
	if res.Tombstoned {
		return fmt.Errorf("%w: %s was detached from this item earlier; re-attach it from the UI first", backlogdto.ErrPRDetached, ref)
	}
	if _, mirrorErr := s.Repo.RecomputeItemPRMirror(ctx, observed.ID); mirrorErr != nil {
		log.WarningLog().Printf("[Storage] additive PR: mirror recompute item=%s: %v", observed.ID, mirrorErr)
	}
	if noteErr := s.AppendProgressNote(ctx, observed.ID, -1, pr.Summary, "pr_added"); noteErr != nil {
		log.WarningLog().Printf("[Storage] additive PR: progress note item=%s: %v", observed.ID, noteErr)
	}
	return nil
}

// FindDoneItemsOlderThan returns backlog items in "done" status whose most
// recent done-transition happened at/before cutoff. Thin passthrough to the
// ent-backed repository (same rationale as MarkStuck below) — returns
// nil, nil for backends that don't support it (e.g. an in-memory test
// double), never an error.
func (s *Storage) FindDoneItemsOlderThan(ctx context.Context, cutoff time.Time) ([]backlogdto.BacklogItemData, error) {
	return s.Repo.FindDoneItemsOlderThan(ctx, cutoff)
}

// --- BacklogStuckState (durable stuck-state read surface) ---

// FindOpenStuckStates returns every open (unresolved, un-snoozed)
// BacklogStuckState row, joined with rendering-relevant item fields. Returns
// an empty slice (no error) when the backend does not support stuck-state
// queries (e.g. an in-memory test double).
func (s *Storage) FindOpenStuckStates(ctx context.Context) ([]backlogdto.OpenStuckStateData, error) {
	return s.Repo.FindOpenStuckStates(ctx)
}

// ListStuckStatesForItem returns every BacklogStuckState row for itemID,
// open or resolved, ordered oldest-first — the full-history read backing
// get_backlog_stuck_report. Thin passthrough, same rationale as
// FindOpenStuckStates above.
func (s *Storage) ListStuckStatesForItem(ctx context.Context, itemID string) ([]backlogdto.StuckStateHistoryData, error) {
	return s.Repo.ListStuckStatesForItem(ctx, itemID)
}

// HasOpenStuckReason reports whether itemID has an open (unresolved,
// un-snoozed) stuck row for reason.
func (s *Storage) HasOpenStuckReason(ctx context.Context, itemID string, reason domain.StuckReason) (bool, error) {
	rows, err := s.Repo.FindOpenStuckStates(ctx)
	if err != nil {
		return false, err
	}
	_, ok := FindOpenStuckStateFor(rows, itemID, reason)
	return ok, nil
}

// SnoozeStuckState sets snoozed_until on an open BacklogStuckState row for
// (itemID, reason). Returns false, nil when the backend does not support
// stuck-state writes or no matching open row exists — never an error for a
// missing row.
func (s *Storage) SnoozeStuckState(ctx context.Context, itemID string, reason domain.StuckReason, until time.Time) (bool, error) {
	return s.Repo.SnoozeStuckState(ctx, itemID, reason, until)
}

// MarkStuck opens/refreshes/reopens a durable BacklogStuckState row for
// (itemID, reason). Thin passthrough so callers outside package session
// (e.g. server/services, which cannot reach the unexported repo field) can
// write stuck state. Returns false, nil when the backend does not support
// stuck-state writes — never an error for an unsupported backend.
func (s *Storage) MarkStuck(ctx context.Context, itemID string, reason domain.StuckReason, expectedStatus BacklogStatus, stuckContext string) (bool, error) {
	return s.Repo.MarkStuck(ctx, itemID, reason, expectedStatus, stuckContext)
}

// ResolveStuck atomically, idempotently closes an open BacklogStuckState row
// for (itemID, reason). Thin passthrough, same rationale as MarkStuck above.
func (s *Storage) ResolveStuck(ctx context.Context, itemID string, reason domain.StuckReason) (bool, error) {
	return s.Repo.ResolveStuck(ctx, itemID, reason)
}

// MarkStuckNotified sets notified_at=now on an open, not-yet-notified stuck
// row for (itemID, reason). Thin passthrough, same rationale as MarkStuck above.
func (s *Storage) MarkStuckNotified(ctx context.Context, itemID string, reason domain.StuckReason) (bool, error) {
	return s.Repo.MarkStuckNotified(ctx, itemID, reason)
}

// --- ItemSource ---

// CreateItemSource registers a new external item source.
func (s *Storage) CreateItemSource(ctx context.Context, data backlogdto.ItemSourceData) (*backlogdto.ItemSourceData, error) {
	return s.Repo.CreateItemSource(ctx, data)
}

// ListItemSources returns all registered item sources.
func (s *Storage) ListItemSources(ctx context.Context) ([]backlogdto.ItemSourceData, error) {
	return s.Repo.ListItemSources(ctx)
}

// UpdateItemSource modifies an existing item source.
func (s *Storage) UpdateItemSource(ctx context.Context, id string, update backlogdto.ItemSourceUpdate) (*backlogdto.ItemSourceData, error) {
	return s.Repo.UpdateItemSource(ctx, id, update)
}

// DeleteItemSource removes an item source by UUID string.
func (s *Storage) DeleteItemSource(ctx context.Context, id string) error {
	return s.Repo.DeleteItemSource(ctx, id)
}

// GetItemSourceByID retrieves a single item source's domain data by UUID
// string. Used by the GitHub forward-sync EventBus subscriber (see
// server/services/backlog_github_forward_sync.go) to look up a backlog item's
// source (ForwardSyncEnabled, ForwardSyncCloseLabel, PluginID, Config) without
// needing an *EntRepository handle of its own.
func (s *Storage) GetItemSourceByID(ctx context.Context, id string) (*backlogdto.ItemSourceData, error) {
	src, err := s.Repo.GetItemSourceByID(ctx, id)
	if err != nil {
		return nil, err
	}
	data := itemSourceToData(src)
	return &data, nil
}

// ListSourceSyncEvents returns sync history events for an item source, most
// recent first. Direct EntRepository delegation, like GetItemSession below.
func (s *Storage) ListSourceSyncEvents(ctx context.Context, sourceID string) ([]backlogdto.SourceSyncEventData, bool, error) {
	return s.Repo.ListSourceSyncEvents(ctx, sourceID)
}

// CreateSourceSyncEvent records a sync run for an item source. Direct EntRepository
// delegation, like ListSourceSyncEvents above.
func (s *Storage) CreateSourceSyncEvent(ctx context.Context, sourceID, cursorAfter string, created, updated, skipped, errored int, errMsg string, startedAt, finishedAt time.Time) error {
	return s.Repo.CreateSourceSyncEvent(ctx, sourceID, cursorAfter, created, updated, skipped, errored, errMsg, startedAt, finishedAt)
}

// RecordSourceSyncFailure records a forward-sync failure (e.g. CloseIssue
// erroring) as a queryable sync-history row. Direct EntRepository delegation,
// like CreateSourceSyncEvent above.
func (s *Storage) RecordSourceSyncFailure(ctx context.Context, sourceID, message string) error {
	return s.Repo.RecordSourceSyncFailure(ctx, sourceID, message)
}

// --- ItemSession (direct EntRepository delegation) ---

// GetItemSession looks up an ItemSession by entity UUID (loads BacklogItem edge).
func (s *Storage) GetItemSession(ctx context.Context, id string) (backlogdto.ItemSessionSummary, error) {
	return s.Repo.GetItemSession(ctx, id)
}

// GetBaseCommitSHAsForSessions returns a sessionUUID→base_commit_sha map for the given UUIDs.
func (s *Storage) GetBaseCommitSHAsForSessions(ctx context.Context, uuids []string) (map[string]string, error) {
	return s.Repo.GetBaseCommitSHAsForSessions(ctx, uuids)
}

// GetItemSessionBySessionUUID looks up the ItemSession for a given session UUID (loads BacklogItem edge).
func (s *Storage) GetItemSessionBySessionUUID(ctx context.Context, sessionUUID string) (backlogdto.ItemSessionSummary, error) {
	return s.Repo.GetItemSessionBySessionUUID(ctx, sessionUUID)
}

// ListSessionPRViewsByUUIDs returns each session's PR links joined to their
// states as row data (no live Instance needed). Sessions without links are absent.
func (s *Storage) ListSessionPRViewsByUUIDs(ctx context.Context, uuids []string) (map[string][]PRView, error) {
	return s.Repo.ListBySessionUUIDs(ctx, uuids)
}

// SessionUUIDsByPR returns the sessions holding a link to ref (see EntRepository.SessionUUIDsByPR).
func (s *Storage) SessionUUIDsByPR(ctx context.Context, ref PRRef) ([]string, error) {
	return s.Repo.SessionUUIDsByPR(ctx, ref)
}

// AttachItemPRLink upserts an item-direct link (state row plus link) for prURL.
// It reports attached=false, with no error, when the item carries a tombstone for
// that PR, so a re-poll cannot undo a detach.
func (s *Storage) AttachItemPRLink(ctx context.Context, itemID, prURL string, source AttachSource) (bool, error) {
	n, err := ParsePRNumberFromURL(prURL)
	if err != nil {
		return false, err
	}
	ref, err := ParseItemPRRef(prURL, n)
	if err != nil {
		return false, err
	}
	existing, err := s.Repo.ListByItem(ctx, itemID)
	if err != nil {
		return false, fmt.Errorf("attach item pr link: list links: %w", err)
	}
	res, err := s.Repo.UpsertSessionPR(ctx, ItemPROwner(itemID), ref, PRLink{
		Role: AssignRole(existing, ""), Source: source,
		Initial: PRInitial{State: PRStateOpen, URL: strings.TrimSpace(prURL)},
	})
	if err != nil {
		return false, fmt.Errorf("attach item pr link %s: %w", ref, err)
	}
	return !res.Tombstoned, nil
}

// ItemPRs returns the item's derived PR set (see PRSetReader).
func (s *Storage) ItemPRs(ctx context.Context, itemID string) ([]PRView, error) {
	return s.Repo.ItemPRsByID(ctx, itemID)
}

// ListItemGuardViewsByItemIDs serves the retention sweeper and the cleanup gate
// from one data path; see ItemGuardView.
func (s *Storage) ListItemGuardViewsByItemIDs(ctx context.Context, itemIDs []string) (map[string]ItemGuardView, error) {
	return s.Repo.ListItemGuardViews(ctx, itemIDs)
}

// ListLinkOwnersByStateIDs returns the session uuids and item ids linking the states.
func (s *Storage) ListLinkOwnersByStateIDs(ctx context.Context, stateIDs []int) (sessionUUIDs, itemIDs []string, err error) {
	return s.Repo.ListLinkOwnersByStateIDs(ctx, stateIDs)
}

// ListItemIDsBySessionUUIDs returns every item id of each session uuid.
func (s *Storage) ListItemIDsBySessionUUIDs(ctx context.Context, uuids []string) (map[string][]string, error) {
	return s.Repo.ListItemIDsBySessionUUIDs(ctx, uuids)
}

// ListItemSessionsBySessionUUIDs batches GetItemSessionBySessionUUID: one query for all UUIDs.
func (s *Storage) ListItemSessionsBySessionUUIDs(ctx context.Context, uuids []string) (map[string]backlogdto.ItemSessionSummary, error) {
	return s.Repo.ListItemSessionsBySessionUUIDs(ctx, uuids)
}

// IsDiagnoseCaller reports whether sessionUUID is currently linked to a
// backlog item with SessionRoleDiagnose — i.e. whether it's a dispatched
// Diagnose & Nudge investigation session, as opposed to any other role. Used
// by server/mcp's denyIfDiagnoseCaller to gate the general-purpose
// terminal-control MCP tools away from that narrow role: --allowedTools
// provides no real technical enforcement on its own (see
// session/backlog_review.go's BuildReviewCallOptions doc comment), so this is
// the actual, server-side gate. Returns false (not diagnose) on any lookup
// error, including "no link at all" — the check only ever narrows what a
// positively-identified diagnose session may do, so an unidentifiable caller
// falls through to the pre-existing unrestricted behavior for every other role.
func (s *Storage) IsDiagnoseCaller(ctx context.Context, sessionUUID string) bool {
	row, err := s.Repo.GetItemSessionBySessionUUID(ctx, sessionUUID)
	if err != nil {
		return false
	}
	return row.Role == SessionRoleDiagnose
}

// ClaimDiagnoseNudgeAttempt atomically claims sessionUUID's one nudge-write
// attempt — see EntRepository.ClaimDiagnoseNudgeAttempt's doc comment.
func (s *Storage) ClaimDiagnoseNudgeAttempt(ctx context.Context, sessionUUID string) (claimed bool, err error) {
	return s.Repo.ClaimDiagnoseNudgeAttempt(ctx, sessionUUID)
}

// GetWorktreeDataBySessionUUIDs forwards to EntRepository.GetWorktreeDataBySessionUUIDs.
func (s *Storage) GetWorktreeDataBySessionUUIDs(ctx context.Context, sessionUUIDs []string) (map[string]GitWorktreeData, error) {
	return s.Repo.GetWorktreeDataBySessionUUIDs(ctx, sessionUUIDs)
}

// GetWorktreeDataBySessionUUID returns the git worktree data for the Session with
// the given UUID. Returns empty GitWorktreeData for directory-mode sessions or if
// the session is not found.
func (s *Storage) GetWorktreeDataBySessionUUID(ctx context.Context, sessionUUID string) (GitWorktreeData, error) {
	return s.Repo.GetWorktreeDataBySessionUUID(ctx, sessionUUID)
}

// UpdateItemSessionTriageResult stores the triage result JSON payload on an ItemSession.
func (s *Storage) UpdateItemSessionTriageResult(ctx context.Context, id string, triageResult string) error {
	return s.Repo.UpdateItemSessionTriageResult(ctx, id, triageResult)
}

// UpdateItemSessionDiagnoseResult stores a diagnose session's structured outcome.
func (s *Storage) UpdateItemSessionDiagnoseResult(ctx context.Context, id string, result backlogdto.DiagnoseResultData) error {
	return s.Repo.UpdateItemSessionDiagnoseResult(ctx, id, result)
}

// AppendWorkflowSuggestion records a diagnose session's operator recommendation.
func (s *Storage) AppendWorkflowSuggestion(ctx context.Context, itemID string, sug WorkflowSuggestionData) (string, error) {
	return s.Repo.AppendWorkflowSuggestion(ctx, itemID, sug)
}

// DismissWorkflowSuggestion marks a recommendation dismissed.
func (s *Storage) DismissWorkflowSuggestion(ctx context.Context, itemID, suggestionID string) error {
	return s.Repo.DismissWorkflowSuggestion(ctx, itemID, suggestionID)
}

// UpdateItemSessionVerificationNotes stores verification evidence (commands run, manual
// checks performed) reported via request_review on an ItemSession.
func (s *Storage) UpdateItemSessionVerificationNotes(ctx context.Context, id string, verificationNotes string) error {
	return s.Repo.UpdateItemSessionVerificationNotes(ctx, id, verificationNotes)
}

// UpdateItemSessionStarted records the start time for an ItemSession.
func (s *Storage) UpdateItemSessionStarted(ctx context.Context, id string, startedAt time.Time) error {
	return s.Repo.UpdateItemSessionStarted(ctx, id, startedAt)
}

// SetItemSessionBaseCommit records the pre-work base commit SHA on an ItemSession.
// See the EntRepository method for why this is separate from git activity.
func (s *Storage) SetItemSessionBaseCommit(ctx context.Context, id, sha string) error {
	return s.Repo.SetItemSessionBaseCommit(ctx, id, sha)
}

// UpdateItemSessionGitActivity records the session's current tip commit and
// related fields on an ItemSession. For the spawn-time baseline, use
// SetItemSessionBaseCommit.
func (s *Storage) UpdateItemSessionGitActivity(ctx context.Context, id string, sha, msg string, commitAt time.Time, commitCount int) error {
	return s.Repo.UpdateItemSessionGitActivity(ctx, id, sha, msg, commitAt, commitCount)
}

// ListOpenJulesItemSessions returns every not-yet-ended jules_work ItemSession
// across all backlog items. See EntRepository.ListOpenJulesItemSessions.
func (s *Storage) ListOpenJulesItemSessions(ctx context.Context) ([]backlogdto.ItemSessionBacklogEntry, error) {
	return s.Repo.ListOpenJulesItemSessions(ctx)
}

// CountJulesItemSessionsSince counts confirmed, billed jules_work ItemSessions
// created since since. See EntRepository.CountJulesItemSessionsSince.
func (s *Storage) CountJulesItemSessionsSince(ctx context.Context, since time.Time) (int, error) {
	return s.Repo.CountJulesItemSessionsSince(ctx, since)
}

// TouchItemSessionProgress updates only last_progress_at on an ItemSession. See
// EntRepository.TouchItemSessionProgress.
func (s *Storage) TouchItemSessionProgress(ctx context.Context, id string, at time.Time) error {
	return s.Repo.TouchItemSessionProgress(ctx, id, at)
}

// UpdateItemSessionEnded records the end time for an ItemSession.
func (s *Storage) UpdateItemSessionEnded(ctx context.Context, id string, endedAt time.Time) error {
	return s.Repo.UpdateItemSessionEnded(ctx, id, endedAt)
}

// UpdateItemSessionEndedWithReason records the end time for an ItemSession alongside
// classifyHeadlessCallError's bucket (or "" for a successful end).
func (s *Storage) UpdateItemSessionEndedWithReason(ctx context.Context, id string, endedAt time.Time, reason string) error {
	return s.Repo.UpdateItemSessionEndedWithReason(ctx, id, endedAt, reason)
}

// UpdateItemSessionConversationUUID records a headless call's Claude transcript UUID.
// See EntRepository.UpdateItemSessionConversationUUID.
func (s *Storage) UpdateItemSessionConversationUUID(ctx context.Context, id string, conversationUUID string) error {
	return s.Repo.UpdateItemSessionConversationUUID(ctx, id, conversationUUID)
}

// UpdateItemSessionEndedWithDetail is UpdateItemSessionEndedWithReason plus the
// truncated error text for end_reason "other". See EntRepository.UpdateItemSessionEndedWithDetail.
func (s *Storage) UpdateItemSessionEndedWithDetail(ctx context.Context, id string, endedAt time.Time, reason, detail string) error {
	return s.Repo.UpdateItemSessionEndedWithDetail(ctx, id, endedAt, reason, detail)
}

// UpdateItemSessionFailureCapture records the absolute path to a durable raw-output
// capture file for a headless triage/review call that errored or produced
// unparseable output. See EntRepository.UpdateItemSessionFailureCapture.
func (s *Storage) UpdateItemSessionFailureCapture(ctx context.Context, id string, path string) error {
	return s.Repo.UpdateItemSessionFailureCapture(ctx, id, path)
}

// UpdateItemSessionCost adds usd to an ItemSession's estimated_cost_usd and
// records whether that cost is trustworthy. See EntRepository.UpdateItemSessionCost.
func (s *Storage) UpdateItemSessionCost(ctx context.Context, id string, usd float64, priced bool) error {
	return s.Repo.UpdateItemSessionCost(ctx, id, usd, priced)
}

// AddHeadlessCostBySessionUUID adds usd to the estimated_cost_usd of the ItemSession
// for sessionUUID, if any. See EntRepository.AddHeadlessCostBySessionUUID.
func (s *Storage) AddHeadlessCostBySessionUUID(ctx context.Context, sessionUUID string, usd float64) error {
	return s.Repo.AddHeadlessCostBySessionUUID(ctx, sessionUUID, usd)
}

// GetItemSessionBySessionAndItem looks up an ItemSession by both sessionUUID and backlog item ID.
// Returns ErrNotFound if no matching record exists.
func (s *Storage) GetItemSessionBySessionAndItem(ctx context.Context, sessionUUID string, itemID string) (backlogdto.ItemSessionSummary, error) {
	return s.Repo.GetItemSessionBySessionAndItem(ctx, sessionUUID, itemID)
}

// GetClaudeConversationUUIDBySessionUUID returns the Claude conversation UUID
// for the session whose title matches the given UUID. Returns "" when the session
// has no ClaudeSession, and ErrNotFound when no session matches.
func (s *Storage) GetClaudeConversationUUIDBySessionUUID(ctx context.Context, sessionUUID string) (string, error) {
	return s.Repo.GetClaudeConversationUUIDBySessionUUID(ctx, sessionUUID)
}

// GetMostRecentReviewVerdictForItem returns the OverallOutcome of the most recent
// ReviewVerdict linked to any ItemSession for itemID. Returns "" when none exists.
func (s *Storage) GetMostRecentReviewVerdictForItem(ctx context.Context, itemID string) (ReviewOutcome, error) {
	return s.Repo.GetMostRecentReviewVerdictForItem(ctx, itemID)
}

// GetRecentReviewVerdictSummaries returns up to limit ReviewVerdicts for itemID,
// most recent first. Returns nil (not an error) when the repo isn't ent-backed.
func (s *Storage) GetRecentReviewVerdictSummaries(ctx context.Context, itemID string, limit int) ([]backlogdto.ReviewVerdictSummary, error) {
	return s.Repo.GetRecentReviewVerdictSummaries(ctx, itemID, limit)
}

// SaveReviewVerdict upserts a ReviewVerdict for a given ItemSession UUID.
func (s *Storage) SaveReviewVerdict(ctx context.Context, itemSessionID string, verdict backlogdto.ReviewVerdictData) error {
	return s.Repo.SaveReviewVerdict(ctx, itemSessionID, verdict)
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
	repoPath, baseSHA, headSHA := s.resolveReviewedCommits(ctx, itemID)
	if repoPath == "" || baseSHA == "" || headSHA == "" {
		return ""
	}
	hash, err := git.DiffHashBetween(repoPath, baseSHA, headSHA)
	if err != nil {
		log.WarningLog().Printf("[ComputeCurrentDiffHash] item=%s base=%s head=%s: %v", itemID, baseSHA, headSHA, err)
		return ""
	}
	return hash
}

// ComputeReviewedCommitSha returns the tip commit of itemID's most recent
// completed work session — the commit a review verdict saved now reviews.
// Best-effort: "" when there is no completed work session or no resolvable
// commit.
func (s *Storage) ComputeReviewedCommitSha(ctx context.Context, itemID string) string {
	_, _, headSHA := s.resolveReviewedCommits(ctx, itemID)
	return headSHA
}

// resolveReviewedCommits returns the repo path and base/head SHAs of itemID's
// latest completed work session. When last_commit_sha is empty (the session
// ended between reconcile ticks, so refreshWorkSessionGitActivity never ran)
// head is resolved live from the session's worktree/branch instead.
func (s *Storage) resolveReviewedCommits(ctx context.Context, itemID string) (repoPath, baseSHA, headSHA string) {
	repoPath, baseSHA, headSHA, workUUID, err := s.Repo.GetRepoPathAndLatestCompletedWorkSessionCommits(ctx, itemID)
	if err != nil {
		return "", "", ""
	}
	// Prefer the live tip: a stored last_commit_sha can predate commits made after the last reconcile tick.
	if workUUID != "" && repoPath != "" {
		if live := s.ResolveLatestWorkCommit(ctx, workUUID, repoPath); live != "" {
			headSHA = live
		}
	}
	return repoPath, baseSHA, headSHA
}

// ResolveLatestWorkCommit returns the current tip commit of a work session,
// read from its worktree HEAD or, when the worktree is gone, its branch tip
// in repoPath. Returns "" when neither can be resolved.
func (s *Storage) ResolveLatestWorkCommit(ctx context.Context, sessionUUID, repoPath string) string {
	wt, err := s.GetWorktreeDataBySessionUUID(ctx, sessionUUID)
	if err != nil {
		log.Warn("[Storage] ResolveLatestWorkCommit: no worktree data for session", "session", sessionUUID, "error", err)
		return ""
	}
	if wt.WorktreePath != "" {
		if info, statErr := os.Stat(wt.WorktreePath); statErr == nil && info.IsDir() {
			branch, branchErr := git.GetCurrentBranchName(wt.WorktreePath)
			if branchErr == nil && (wt.BranchName == "" || branch == wt.BranchName) {
				sha, headErr := git.GetHeadCommitSHA(wt.WorktreePath)
				if headErr == nil && sha != "" {
					return sha
				}
			} else if branchErr == nil {
				log.Warn("[Storage] ResolveLatestWorkCommit: worktree path now holds a different branch than the session's (path recycled?) — falling back to repo-wide branch lookup", "worktree_path", wt.WorktreePath, "branch", branch, "expected_branch", wt.BranchName)
			}
		}
	}
	if wt.BranchName == "" {
		return ""
	}
	sha, revErr := ResolveBranchTip(ctx, repoPath, wt.BranchName)
	if revErr != nil {
		log.Warn("[Storage] ResolveLatestWorkCommit: rev-parse failed", "branch", wt.BranchName, "repo_path", repoPath, "error", revErr)
		return ""
	}
	return sha
}

// UpdateAcCriterionStatus updates a single acceptance criterion's status by index.
func (s *Storage) UpdateAcCriterionStatus(ctx context.Context, itemID string, criterionIndex int, status string, note string) error {
	return s.Repo.UpdateAcCriterionStatus(ctx, itemID, criterionIndex, status, note)
}

// WaiveAcCriterion records an owner waiver (status waived + reason) on one criterion.
func (s *Storage) WaiveAcCriterion(ctx context.Context, itemID string, criterionIndex int, reason string) error {
	return s.Repo.WaiveAcCriterion(ctx, itemID, criterionIndex, reason)
}

// RecordAcAudit appends an AC audit row for an attempt refused before reaching the criterion.
func (s *Storage) RecordAcAudit(ctx context.Context, d AcAuditEventData) error {
	return s.Repo.RecordAcAudit(ctx, d)
}

// PruneAcAuditOlderThan deletes AC audit rows older than cutoff and returns the count removed.
func (s *Storage) PruneAcAuditOlderThan(ctx context.Context, cutoff time.Time) (int, error) {
	return s.Repo.PruneAcAuditOlderThan(ctx, cutoff)
}

// ListAcAudit returns an item's AC audit trail, oldest first.
func (s *Storage) ListAcAudit(ctx context.Context, itemID string) ([]AcAuditEventData, error) {
	return s.Repo.ListAcAudit(ctx, itemID)
}

// AppendProgressNote records a single report_progress call as an immutable history
// entry, in addition to the current-note-per-criterion updated by UpdateAcCriterionStatus.
func (s *Storage) AppendProgressNote(ctx context.Context, itemID string, criterionIndex int, note, status string) error {
	return s.Repo.AppendProgressNote(ctx, itemID, criterionIndex, note, status)
}

// ListProgressNotesForItem returns the full append-only history of report_progress
// calls for a backlog item, ordered by created_at ascending.
func (s *Storage) ListProgressNotesForItem(ctx context.Context, itemID string) ([]backlogdto.ProgressNoteData, error) {
	return s.Repo.ListProgressNotesForItem(ctx, itemID)
}

// AppendActivityNote records a single post_backlog_update call as an immutable,
// append-only history entry — the ungated sibling to AppendProgressNote (ADR-001).
func (s *Storage) AppendActivityNote(ctx context.Context, itemID, authorSessionUUID, authorSessionTitle, message string) error {
	return s.Repo.AppendActivityNote(ctx, itemID, authorSessionUUID, authorSessionTitle, message)
}

// RecordFlowMarker records a server-decided flow marker, best-effort (see
// EntRepository.RecordFlowMarker). A nil Storage or repo is a no-op.
func (s *Storage) RecordFlowMarker(ctx context.Context, itemID string, kind FlowMarkerKind, value string, prNumber int) {
	if s == nil || s.Repo == nil {
		return
	}
	s.Repo.RecordFlowMarker(ctx, itemID, kind, value, prNumber)
}

// RecordManualNudgeForSession records a user-originated intervention on the
// backlog item linked to sessionUUID. Non-backlog sessions and lookup errors are
// silent no-ops. Callers must be human-driven handlers, never agent-reachable
// ones. The name deliberately omits "FlowMarker" (see
// flow_marker_no_mcp_exposure_test.go): no MCP handler may call this.
func (s *Storage) RecordManualNudgeForSession(ctx context.Context, sessionUUID, value string) {
	if s == nil || s.Repo == nil || sessionUUID == "" {
		return
	}
	// The gate runs before the item lookup so a hot input path (WriteToSession)
	// pays one map read per 30m window, and non-backlog sessions are remembered too.
	s.nudgeGateOnce.Do(func() { s.nudgeGate = NewMarkerLimiter(nil) })
	if !s.nudgeGate.Allow(sessionUUID, value) {
		return
	}
	// A client disconnect must not drop the marker mid-lookup.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	is, err := s.Repo.GetItemSessionBySessionUUID(ctx, sessionUUID)
	if err != nil || is.BacklogItemID == "" {
		return
	}
	s.Repo.RecordFlowMarker(ctx, is.BacklogItemID, FlowMarkerManualNudge, value, 0)
}

// RecordManualNudgeForItem records a user-originated intervention on itemID.
func (s *Storage) RecordManualNudgeForItem(ctx context.Context, itemID, value string) {
	if s == nil || s.Repo == nil {
		return
	}
	s.Repo.RecordFlowMarker(ctx, itemID, FlowMarkerManualNudge, value, 0)
}

// RecordFlowMarkerForItemPR passes through to EntRepository.RecordFlowMarkerForItemPR
// (limiter first, then a pr_number-only read). A nil Storage or repo is a no-op.
func (s *Storage) RecordFlowMarkerForItemPR(ctx context.Context, itemID string, kind FlowMarkerKind, value string) {
	if s == nil || s.Repo == nil {
		return
	}
	s.Repo.RecordFlowMarkerForItemPR(ctx, itemID, kind, value)
}

// ListFlowMarkersForItem passes through to EntRepository.ListFlowMarkersForItem.
func (s *Storage) ListFlowMarkersForItem(ctx context.Context, itemID string, kind FlowMarkerKind, since, until time.Time) ([]FlowMarkerData, error) {
	return s.Repo.ListFlowMarkersForItem(ctx, itemID, kind, since, until)
}

// ListActivityNotesForItem returns the full append-only activity-note history for
// a backlog item, ordered by created_at ascending.
func (s *Storage) ListActivityNotesForItem(ctx context.Context, itemID string) ([]backlogdto.ActivityNoteData, error) {
	return s.Repo.ListActivityNotesForItem(ctx, itemID)
}

// CreateItemSession creates a new ItemSession linked to a BacklogItem.
func (s *Storage) CreateItemSession(ctx context.Context, data backlogdto.ItemSessionData) (backlogdto.ItemSessionSummary, error) {
	return s.Repo.CreateItemSession(ctx, data)
}

// CreateItemSessionWithVerdict atomically creates an ItemSession and its initial
// ReviewVerdict in a single transaction. Falls back gracefully if the backend is
// not ent-based.
func (s *Storage) CreateItemSessionWithVerdict(ctx context.Context, isData backlogdto.ItemSessionData, verdict backlogdto.ReviewVerdictData) (backlogdto.ItemSessionSummary, error) {
	return s.Repo.CreateItemSessionWithVerdict(ctx, isData, verdict)
}

// ListItemSessions returns all ItemSessions for a given BacklogItem UUID string.
func (s *Storage) ListItemSessions(ctx context.Context, itemID string) ([]backlogdto.ItemSessionSummary, error) {
	return s.Repo.ListItemSessions(ctx, itemID)
}

// ListItemSessionsForItems batches ListItemSessions across UUID-shaped item IDs.
func (s *Storage) ListItemSessionsForItems(ctx context.Context, itemIDs []string) (map[string][]backlogdto.ItemSessionSummary, error) {
	return s.Repo.ListItemSessionsForItems(ctx, itemIDs)
}

// UpdateItemSessionSessionUUID updates the session_uuid on an existing ItemSession record.
func (s *Storage) UpdateItemSessionSessionUUID(ctx context.Context, id string, sessionUUID string) error {
	return s.Repo.UpdateItemSessionSessionUUID(ctx, id, sessionUUID)
}

// GetAllItemSessionsWithBacklogInfo returns all item sessions joined with backlog item metadata.
// Delegates to EntRepository; returns an error for non-ent backends.
func (s *Storage) GetAllItemSessionsWithBacklogInfo(ctx context.Context) ([]backlogdto.ItemSessionBacklogEntry, error) {
	return s.Repo.GetAllItemSessionsWithBacklogInfo(ctx)
}

// GetDeletedItemSessionCostLedger returns the durable deleted-item cost
// ledger. Delegates to EntRepository; returns an error for non-ent backends.
func (s *Storage) GetDeletedItemSessionCostLedger(ctx context.Context) ([]backlogdto.DeletedItemSessionCostEntry, error) {
	return s.Repo.GetDeletedItemSessionCostLedger(ctx)
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
		return nil, domain.ErrNotFound
	}
	g, err := client.SessionGoal.Query().
		Where(sessiongoal.SessionUUID(sessionUUID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, domain.ErrNotFound
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
			return nil, domain.ErrNotFound
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
