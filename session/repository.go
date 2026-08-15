package session

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/session/ent"
)

// ErrPreconditionFailed is returned when an optimistic-locking precondition check fails.
var ErrPreconditionFailed = errors.New("precondition failed: concurrent modification detected")

// ErrNotFound is returned when a requested entity does not exist.
var ErrNotFound = errors.New("not found")

// ErrConflict is returned when an operation would violate a uniqueness constraint.
var ErrConflict = errors.New("conflict")

// ErrPRReassignmentNotAllowed is returned by SetBacklogItemPRAndTransition
// when a caller attempts to reassign an already-pr_pending item's tracked
// PR to a different PR number without a valid PRReassignmentGuard.
var ErrPRReassignmentNotAllowed = errors.New("PR reassignment not allowed")

// PRReassignmentGuard carries the caller-verified preconditions required
// before SetBacklogItemPRAndTransition (session/storage.go) will accept a
// reassignment — a call where the observed item is already pr_pending with
// a DIFFERENT PR number than the one being recorded now. This function
// itself never calls GitHub; a caller supplies this guard to attest it
// already did that verification. A caller with no way to produce a valid
// guard (e.g. the manual-override RPC in
// server/services/backlog_service_lifecycle.go, which by design never
// calls GitHub) passes nil and gets a clear rejection instead of silently
// reassigning an unverified PR.
type PRReassignmentGuard struct {
	// OverrideReason must be non-empty — the caller's own already-validated
	// reason for the reassignment.
	OverrideReason string
	// CurrentPRMerged must reflect the caller's verified state of the
	// CURRENTLY tracked PR. true hard-blocks the reassignment
	// unconditionally — a merged PR's association must never be silently
	// swapped, even with OverrideReason set.
	CurrentPRMerged bool
	// NewPRAuthorVerified must be true only when the caller has verified the
	// new PR's GitHub author matches the caller's own verified identity.
	NewPRAuthorVerified bool
}

// ErrDependencyCycle is returned by AddBacklogItemDependency when the new
// blocker->blocked edge would create a cycle in the dependency graph.
var ErrDependencyCycle = errors.New("backlog item dependency would create a cycle")

// BacklogItemDependencyEdge names a blocker/blocked pair explicitly so the
// two bare ID strings can't be silently swapped at a call site — see
// .claude/rules/primitive-obsession-checklist.md.
type BacklogItemDependencyEdge struct {
	// BlockerID is the item that must reach a resolved status (done or
	// archived) before BlockedID is eligible for dequeue/start.
	BlockerID string
	// BlockedID is the dependent item, gated until BlockerID resolves.
	BlockedID string
}

// Repository defines the interface for session persistence operations.
// This abstraction allows multiple storage backends (SQLite, JSON, etc.)
// while maintaining a consistent API for session management.
type Repository interface {
	// Create inserts a new session into storage
	Create(ctx context.Context, data InstanceData) error

	// Update modifies an existing session in storage
	Update(ctx context.Context, data InstanceData) error

	// Delete removes a session from storage by title
	Delete(ctx context.Context, title string) error

	// Get retrieves a single session by title with full child data
	// For selective loading, use GetWithOptions instead
	Get(ctx context.Context, title string) (*InstanceData, error)

	// GetWithOptions retrieves a single session with selective child data loading
	// Use LoadOptions presets (LoadMinimal, LoadSummary, LoadFull) or custom options
	GetWithOptions(ctx context.Context, title string, options LoadOptions) (*InstanceData, error)

	// List retrieves all sessions with summary child data (no diff content)
	// For selective loading, use ListWithOptions instead
	List(ctx context.Context) ([]InstanceData, error)

	// ListWithOptions retrieves all sessions with selective child data loading
	// Use LoadOptions presets (LoadMinimal, LoadSummary, LoadFull) or custom options
	ListWithOptions(ctx context.Context, options LoadOptions) ([]InstanceData, error)

	// ListByStatus retrieves sessions filtered by status with summary child data
	// For selective loading, use ListByStatusWithOptions instead
	ListByStatus(ctx context.Context, status Status) ([]InstanceData, error)

	// ListByStatusWithOptions retrieves sessions filtered by status with selective loading
	ListByStatusWithOptions(ctx context.Context, status Status, options LoadOptions) ([]InstanceData, error)

	// ListByTag retrieves sessions with a specific tag with summary child data
	// For selective loading, use ListByTagWithOptions instead
	ListByTag(ctx context.Context, tag string) ([]InstanceData, error)

	// ListByTagWithOptions retrieves sessions with a specific tag with selective loading
	ListByTagWithOptions(ctx context.Context, tag string, options LoadOptions) ([]InstanceData, error)

	// UpdateTimestamps efficiently updates only timestamp fields for a session
	// This is optimized for frequent updates from WebSocket terminal streaming
	UpdateTimestamps(ctx context.Context, title string, lastTerminalUpdate, lastMeaningfulOutput time.Time, lastOutputSignature string) error

	// UpdateReviewQueueState efficiently updates the review-queue interaction fields
	// (LastUserResponse, ProcessingGraceUntil, LastPromptDetected, LastPromptSignature)
	// without the read-modify-write overhead of a full Get+Update cycle.
	UpdateReviewQueueState(ctx context.Context, title string, lastUserResponse, processingGraceUntil, lastPromptDetected time.Time, lastPromptSignature string) error

	// UpdateLastAddedToQueue sets only the last_added_to_queue field for a session.
	// Issues a single UPDATE WHERE title=? without a prior SELECT.
	UpdateLastAddedToQueue(ctx context.Context, title string, t time.Time) error

	// UpdateLastAcknowledged sets only the last_acknowledged field for a session.
	// Issues a single UPDATE WHERE title=? without a prior SELECT.
	UpdateLastAcknowledged(ctx context.Context, title string, t time.Time) error

	// UpdateLastViewed sets only the last_viewed field for a session.
	// Issues a single UPDATE WHERE title=? without a prior SELECT.
	UpdateLastViewed(ctx context.Context, title string, t time.Time) error

	// UpdateSessionMetadata efficiently updates only title/category/note/working_dir
	// fields for a session, issuing a single UPDATE WHERE title=? without a prior SELECT
	// and without touching worktree/diffstats/tags/claude_session rows (unlike Update).
	// currentTitle must be the row's title from before any rename applied in this same
	// call — see the EntRepository implementation for why. A nil field pointer leaves
	// that field untouched; Note is written whenever non-nil (including "") since an
	// empty note is a meaningful cleared state, not "unset".
	UpdateSessionMetadata(ctx context.Context, currentTitle string, newTitle, category, note, workingDir *string) error

	// Close performs cleanup and releases resources
	Close() error

	// --- New Session-based methods (Phase 2 of schema normalization) ---
	// These methods use the new domain-driven Session type with optional contexts.
	// They are preferred over InstanceData methods for new code.

	// GetSession retrieves a session using the new Session domain model.
	// Use ContextOptions to control which optional contexts are loaded.
	// Returns nil if session not found.
	GetSession(ctx context.Context, title string, opts ContextOptions) (*Session, error)

	// ListSessions retrieves all sessions using the new Session domain model.
	// Use ContextOptions to control which optional contexts are loaded.
	ListSessions(ctx context.Context, opts ContextOptions) ([]*Session, error)

	// CreateSession creates a new session from the Session domain model.
	CreateSession(ctx context.Context, session *Session) error

	// UpdateSession updates an existing session using the Session domain model.
	UpdateSession(ctx context.Context, session *Session) error

	// --- Permissions & Analytics ---

	// AllRules returns all auto-approval rules.
	AllRules(ctx context.Context) ([]ApprovalRuleData, error)
	// UpsertRule creates or updates an auto-approval rule.
	UpsertRule(ctx context.Context, rule ApprovalRuleData) error
	// DeleteRule removes an auto-approval rule by ID.
	DeleteRule(ctx context.Context, id string) error

	// RecordAnalytics logs a classification decision.
	RecordAnalytics(ctx context.Context, data AnalyticsData) error
	// ListAnalytics retrieves recent classification decisions.
	ListAnalytics(ctx context.Context, limit int) ([]AnalyticsData, error)

	// ListAnalyticsSince retrieves analytics entries with created_at >= since.
	// Replaces the in-Go date filter in LoadWindow. Implements AC-1.
	// Pass limit=0 for no limit.
	ListAnalyticsSince(ctx context.Context, since time.Time, limit int) ([]AnalyticsData, error)

	// ListAnalyticsByProgramSince retrieves entries for a specific program since a time.
	// Uses the compound index (command_program, created_at). Implements AC-3.
	// Pass limit=0 for no limit.
	ListAnalyticsByProgramSince(ctx context.Context, program string, since time.Time, limit int) ([]AnalyticsData, error)

	// GetSubcommandBreakdown returns per-(subcommand, decision) counts for a program
	// in the given time window. Uses SQL GROUP BY via ent Aggregate. Implements AC-4.
	GetSubcommandBreakdown(ctx context.Context, program string, since time.Time) ([]SubcommandDecisionCount, error)

	// ListRecentCommandsByProgram returns the most recent n command_preview strings
	// for (program, subcommand). Pass subcommand="" to match all subcommands.
	// Implements AC-5.
	ListRecentCommandsByProgram(ctx context.Context, program, subcommand string, since time.Time, n int) ([]string, error)

	// GetSubcommandTrend returns raw analytics rows for (program, subcommand) since
	// a given time. The caller buckets these using ComputeDailyBuckets. Implements AC-6.
	// Pass subcommand="" to match all subcommands for the program.
	GetSubcommandTrend(ctx context.Context, program, subcommand string, since time.Time) ([]AnalyticsData, error)

	// --- Projects ---

	// CreateProject inserts a new project.
	CreateProject(ctx context.Context, data ProjectData) (*ProjectData, error)
	// ListProjects returns all projects.
	ListProjects(ctx context.Context) ([]ProjectData, error)
	// UpdateProject modifies an existing project.
	UpdateProject(ctx context.Context, data ProjectData) (*ProjectData, error)
	// DeleteProject removes a project by name; sessions are unassigned.
	DeleteProject(ctx context.Context, name string) error
	// AssignSessionsToProject links sessions (by title) to a project (by name).
	AssignSessionsToProject(ctx context.Context, projectName string, sessionTitles []string) error

	// --- Backlog ---

	// CreateBacklogItem inserts a new backlog item.
	CreateBacklogItem(ctx context.Context, data BacklogItemData) (*BacklogItemData, error)
	// GetBacklogItem retrieves a backlog item by UUID string.
	GetBacklogItem(ctx context.Context, id string) (*BacklogItemData, error)
	// ListBacklogItems returns backlog items with optional filtering.
	ListBacklogItems(ctx context.Context, filter BacklogItemFilter) ([]BacklogItemData, error)
	// UpdateBacklogItem modifies an existing backlog item with optional precondition check.
	UpdateBacklogItem(ctx context.Context, id string, update BacklogItemUpdate, precondition *BacklogItemPrecondition) (*BacklogItemData, error)
	// ArchiveBacklogItem sets the archived_at timestamp on a backlog item.
	ArchiveBacklogItem(ctx context.Context, id string) (*BacklogItemData, error)
	// UnarchiveBacklogItem clears archived_at and restores the item to "idea".
	UnarchiveBacklogItem(ctx context.Context, id string) (*BacklogItemData, error)
	// DeleteBacklogItem permanently removes an item and all its child records.
	DeleteBacklogItem(ctx context.Context, id string) error
	// TransitionBacklogItemStatus changes the status of a backlog item with optional precondition.
	// triggeredBy records who/what caused the transition (TriggeredByUser or TriggeredBySystem)
	// in the resulting BacklogStatusEvent audit row.
	TransitionBacklogItemStatus(ctx context.Context, id string, toStatus BacklogStatus, precondition *BacklogItemPrecondition, triggeredBy string) (*BacklogItemData, error)
	// GetAllItemSessionsWithBacklogInfo returns all item sessions joined with their parent backlog item metadata.
	// Used by the Insights dashboard to annotate sessions with backlog context.
	GetAllItemSessionsWithBacklogInfo(ctx context.Context) ([]ItemSessionBacklogEntry, error)
	// ListBacklogItemSummaries returns lightweight summaries for the list view.
	// Unlike ListBacklogItems it omits Description/plan fields and eagerly loads
	// ItemSessions (with ReviewVerdict) without over-fetching status events.
	ListBacklogItemSummaries(ctx context.Context, filter BacklogItemFilter) ([]BacklogItemSummary, error)
	// AddBacklogItemDependency records that edge.BlockedID may not be
	// dequeued/started until edge.BlockerID reaches a resolved status
	// (done). Upserts against the unique (blocker_id, blocked_id) index —
	// adding an existing pair is a no-op. Returns an error if the new edge
	// would create a cycle.
	AddBacklogItemDependency(ctx context.Context, edge BacklogItemDependencyEdge) error
	// UnresolvedBlockerItemIDs returns the subset of itemIDs that have at
	// least one BacklogItemDependency whose blocker has not reached done.
	// Batched by blocked_id so callers (DequeueNextQueuedItems,
	// transitionWithGuard) avoid an N+1 per-candidate query.
	UnresolvedBlockerItemIDs(ctx context.Context, itemIDs []string) (map[string]bool, error)
	// UnresolvedBlockerIDs returns the specific blocker item IDs still
	// unresolved for a single blocked item, for stuck-reason messaging.
	UnresolvedBlockerIDs(ctx context.Context, itemID string) ([]string, error)

	// --- ItemSource ---

	// CreateItemSource registers a new external item source.
	CreateItemSource(ctx context.Context, data ItemSourceData) (*ItemSourceData, error)
	// ListItemSources returns all registered item sources.
	ListItemSources(ctx context.Context) ([]ItemSourceData, error)
	// UpdateItemSource modifies an existing item source.
	UpdateItemSource(ctx context.Context, id string, update ItemSourceUpdate) (*ItemSourceData, error)
	// DeleteItemSource removes an item source by UUID string.
	DeleteItemSource(ctx context.Context, id string) error
}

// ApprovalRuleData is the domain model for an auto-approval rule.
type ApprovalRuleData struct {
	ID             string
	Name           string
	ToolName       string
	ToolPattern    string
	ToolCategory   string
	CommandPattern string
	FilePattern    string
	Decision       int
	RiskLevel      int
	Reason         string
	Alternative    string
	Priority       int
	Enabled        bool
	Source         string
	CreatedAt      time.Time
	UpdatedAt      time.Time

	// Structured CommandCriteria fields — correspond to classifier.CommandCriteria.
	Programs              []string
	Subcommands           []string
	BlockedSubcommands    []string
	RequiredFlags         []string
	ForbiddenFlags        []string
	RequiredFlagPrefixes  []string
	PythonModes           []string
	SafePythonImportsOnly bool
	RequireCIPassing      bool
}

// SubcommandDecisionCount holds a (subcommand, decision) aggregate count.
// Returned by GetSubcommandBreakdown.
type SubcommandDecisionCount struct {
	Subcommand string
	Decision   string
	Count      int
}

// AnalyticsData is the domain model for classification analytics.
type AnalyticsData struct {
	ID                 string
	SessionID          string
	ToolName           string
	CommandPreview     string
	Cwd                string
	Decision           string
	RiskLevel          string
	RuleID             string
	RuleName           string
	Reason             string
	Alternative        string
	DurationMs         int64
	ApprovalID         string
	CommandProgram     string
	CommandCategory    string
	CommandSubcategory string
	PythonImports      []string
	CreatedAt          time.Time
}

// ProjectData is the domain model for a project that groups sessions.
type ProjectData struct {
	// ID is the unique project name (used as string external identifier)
	ID          string
	Name        string
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ReviewVerdictSummary is a domain DTO for a review verdict embedded in ItemSessionSummary.
type ReviewVerdictSummary struct {
	ID             string
	OverallOutcome string
	PerCriterion   string // JSON []CriterionVerdict
	Summary        string
	DiffHash       string
	DiffTokenCount int
	DiffTruncated  bool
	OverrideBy     string
	OverrideReason string
	OverrideAt     *time.Time
	CreatedAt      time.Time
}

// ItemSessionSummary is the domain DTO replacing *ent.ItemSession in Storage returns.
// Note: item_sessions table has NO status, triage_result_summary, or overall_outcome columns.
//   - EndedAt == nil means the session is still running
//   - TriageResultSummary: parsed from the triage_result JSON column
//   - OverallOutcome: from the review_verdicts table (populated via ReviewVerdict edge)
//   - ReviewVerdict: eagerly loaded when the query uses WithReviewVerdict()
type ItemSessionSummary struct {
	ID                       string
	BacklogItemID            string
	SessionUUID              string
	Role                     string
	AcSnapshot               AcCriteriaJSON
	PipelineModeSnapshot     string
	PipelineModeSnapshotHash string
	// BaseCommitSha is the worktree's pre-work HEAD, captured once at spawn —
	// the base of the review gate's base..HEAD diff, and by construction always
	// already an ancestor of main. Never use it as evidence that this session's
	// work shipped; that is LastCommitSha's job. See the ItemSession ent
	// schema's field comments for the full BUG-047 rationale.
	BaseCommitSha string
	// LastCommitSha is the session's current tip commit, refreshed each
	// reconciliation tick while the session is active (see
	// BacklogLifecycleListener.refreshWorkSessionGitActivity).
	LastCommitSha         string
	LastCommitMessage     string
	CommitCountSinceSpawn int
	StartedAt             *time.Time
	EndedAt               *time.Time
	EndReason             string // set alongside EndedAt for a headless call; see ItemSession.end_reason schema comment
	FailureCapturePath    string // absolute path to a durable raw-output capture; see ItemSession.failure_capture_path schema comment
	LastCommitAt          *time.Time
	LastFileTouchAt       *time.Time
	LastProgressAt        *time.Time
	CreatedAt             time.Time
	EstimatedCostUsd      float64
	TriageResult          string // raw JSON stored in triage_result column
	TriageResultSummary   string // summary field parsed from TriageResult
	VerificationNotes     string // freeform verification evidence reported via request_review
	OverallOutcome        string // from linked review_verdict (empty if none)
	ReviewVerdict         *ReviewVerdictSummary
	// ClaimantHostID identifies the physical stapler-squad process/host that claimed or
	// attached this session. See ItemSession.claimant_host_id's schema comment for the
	// full disambiguation against STAPLER_SQUAD_INSTANCE and CloudContext.InstanceID.
	ClaimantHostID string
}

// BacklogStatusEventData is the domain DTO replacing *ent.BacklogStatusEvent in Storage returns.
type BacklogStatusEventData struct {
	ID          string
	FromStatus  string
	ToStatus    string
	TriggeredBy string
	Note        *string
	CreatedAt   time.Time
}

// ProgressNoteData is the domain DTO replacing *ent.BacklogProgressNote in Storage returns.
// Unlike the current-note-per-criterion stored on BacklogItem.AcceptanceCriteria, this
// represents a single append-only history entry from one report_progress call.
type ProgressNoteData struct {
	ID             string
	CriterionIndex int
	Note           string
	Status         string
	CreatedAt      time.Time
}

// SourceSyncEventData is the domain DTO replacing *ent.SourceSyncEvent in Storage returns.
type SourceSyncEventData struct {
	ID           string
	ItemsCreated int
	ItemsUpdated int
	ItemsSkipped int
	ItemsErrored int
	ErrorMessage string
	CursorAfter  string
	StartedAt    time.Time
	FinishedAt   *time.Time
}

// BacklogItemData is the domain model for a backlog item.
type BacklogItemData struct {
	ID                 string
	Title              string
	Description        string
	AcceptanceCriteria AcCriteriaJSON
	Priority           int
	Status             string
	RepoPath           string
	SkipReviewGate     bool
	SkipPlanning       bool
	AutoSpawnSession   bool
	// AutoCreatePR, when true, automatically runs the same one-shot PR-creation
	// prompt the Review Queue's manual "Create PR" button uses, once a work
	// session for this item reaches TASK_COMPLETE (see
	// server.ReactiveQueueManager.maybeAutoCreatePR). Off by default — a
	// deliberate opt-in, since it removes the human review-the-prompt
	// checkpoint before an LLM-authored PR is created.
	AutoCreatePR bool
	// ReworkCapOverride is a per-item override for the auto-rework cap
	// (config.Config.MaxAutoReworkIterationsOrDefault). Nil = use the global
	// default. 0 = unlimited retries for this item. >0 = this item's own cap,
	// replacing (not adding to) the global value. See effectiveReworkCap in
	// server/services/backlog_service_triage.go.
	ReworkCapOverride *int
	// PipelineMode is the slug of the PipelineMode this item uses to drive
	// triage/work/review content (see session/pipeline_engine.go). Empty
	// string (PipelineModeDefault) means the built-in, hardcoded pipeline.
	//
	// Scope note: this field is introduced in Epic 1.3 (backlog-configurable-
	// pipeline) solely so PipelineEngine's mode-resolution/fail-closed
	// behavior is exercisable against this struct per Story 1.3.3's own
	// acceptance criteria. It is NOT yet wired to ent/proto/the repository
	// persistence layer or any RPC handler — every BacklogItemData produced
	// by the current storage layer has PipelineMode == "" today. That full
	// wiring (ent schema field, proto optional field, repository Create/
	// Update mapping, RPC handler presence-gating) is Epic 1.4's scope.
	PipelineMode string
	// Category is a coarse classification (bugfix/feature/chore/refactor) used
	// purely as a frontend-defaulting hint at creation time — see
	// BacklogCategory / IsValidBacklogCategory. Empty string means
	// uncategorized (today's behavior for every existing item, preserved
	// exactly). The server only persists and validates this value; it never
	// resolves or applies the per-category automation-toggle defaults itself
	// (that happens client-side, once, in BacklogItemForm.tsx at category-
	// selection time).
	Category          string
	PlanApproved      bool
	PlanApprovedAt    *time.Time
	PlanArtifactsPath string
	// PlanRejectionReason is the free-text reason from the most recent
	// RejectPlan call. Cleared on ApprovePlan, on the next TriggerTriage
	// completion, and on backward transition to idea/refining. See
	// project_plans/plan-approval-ux/decisions/ADR-001.
	PlanRejectionReason string
	PlanRejectedAt      *time.Time
	// QueuedAt is set when a fresh spawn hit the concurrency cap and the item
	// was transitioned to "queued" instead of rejected. Nil unless Status ==
	// BacklogStatusQueued (or the item was previously queued). Drives FIFO
	// dequeue ordering.
	QueuedAt *time.Time
	// QueuedAutonomous preserves the Autonomous flag from the spawn request
	// that got queued, so dequeue replays it faithfully.
	QueuedAutonomous bool
	Notes            string
	ExternalID       string
	// ExternalURL is the browser-facing URL of the linked external item (e.g.
	// the GitHub issue's html_url). Empty when the item has no linked source.
	ExternalURL string
	// Labels holds the external source's label set (e.g. GitHub issue labels)
	// as of the most recent Fetch. Nil/empty for items with no linked source
	// or no labels.
	Labels     []string
	ArchivedAt *time.Time
	SourceID   string
	PrURL      string
	PrNumber   int
	// ShippedCheckConclusion holds the durable GitHub CI-conclusion snapshot
	// captured at ship time — genuine GitHub CI-conclusion values only, never
	// a capture-failure sentinel. See ShippedSnapshotCaptureFailed.
	ShippedCheckConclusion string
	// ShippedApprovedCount is the durable review-approval-count snapshot
	// captured at ship time.
	ShippedApprovedCount int
	// ShippedChangesReqCount is the durable "changes requested" review-count
	// snapshot captured at ship time.
	ShippedChangesReqCount int
	// ShippedSnapshotAt is the timestamp the durable ship snapshot was
	// captured at. Nil when no snapshot has ever been captured.
	ShippedSnapshotAt *time.Time
	// PrFeedbackAddressedAt is the comment-feedback dedup watermark: the
	// newest substantive PR review-feedback timestamp a fix session has
	// already been dispatched to address. Nil when no feedback-triggered fix
	// has ever been dispatched for this item's current PR.
	PrFeedbackAddressedAt *time.Time
	// GitHubSyncedIssueUpdatedAt is the loop-prevention watermark: the GitHub
	// issue updated_at value most recently synced from GitHub into this item.
	// Nil when the item has never been synced from GitHub.
	GitHubSyncedIssueUpdatedAt *time.Time
	// UserModifiedFields is the JSON-encoded set of field names (title,
	// description, priority) the user has directly edited via UpdateBacklogItem
	// — see ParseUserModifiedFields/MergeUserModifiedFields. Empty string means
	// no field is locally locked; backward sync (SyncOne) treats any field in
	// this set as local-wins and skips overwriting it from the remote source.
	UserModifiedFields string
	// ShippedFileStats holds the JSON-encoded []ShippedFileStat snapshot of
	// per-file diff stats captured at ship time.
	ShippedFileStats string
	// ShippedSnapshotCaptureFailed is true when CaptureShipSnapshot's GitHub
	// fetch or file-stats computation failed — distinct from
	// ShippedCheckConclusion, which holds only genuine CI-conclusion values.
	ShippedSnapshotCaptureFailed bool
	// NextWorkflowID is the pipeline-chaining target (webhook-triggers FR10/AC5):
	// the Workflow ChainFirer fires once this item reaches BacklogStatusDone. Nil
	// means no chain is configured.
	NextWorkflowID *uuid.UUID
	// ChainFired is true once the NextWorkflowID chain-fire has reached a
	// terminal outcome (fired, depth-capped, or expired) — never retried again
	// once true. See ChainFirer/TriggerChainReconciler.
	ChainFired bool
	// ChainedAt is set atomically with the terminal done transition (when
	// NextWorkflowID is already configured) — the eligibility timestamp
	// TriggerChainReconciler's maxChainWaitDuration ceiling measures age
	// against. Nil until the item has reached done with a chain configured.
	ChainedAt *time.Time
	// TriggeredByChainDepth is how many chain hops produced this item —
	// propagated session->session and hard-capped at maxChainDepth (Epic 6.3).
	TriggeredByChainDepth int
	CreatedAt             time.Time
	UpdatedAt             time.Time
	// ItemSessions holds the eagerly-loaded item sessions for this backlog item.
	// Only populated when explicitly loaded by the caller (e.g. GetBacklogItem).
	ItemSessions []ItemSessionSummary
	// StatusEvents holds the eagerly-loaded status transition history.
	// Only populated when explicitly loaded by the caller (e.g. GetBacklogItem).
	StatusEvents []BacklogStatusEventData
	// ProgressNotes holds the eagerly-loaded report_progress audit trail (the
	// implementer's decision history). Only populated when explicitly loaded by
	// the caller (e.g. GetBacklogItem) — see StatusEvents for the same pattern.
	ProgressNotes []ProgressNoteData
}

// BacklogItemSummary is a lightweight projection of BacklogItemData for list views.
// It omits large text fields (Description, plan artifacts) and status-event history,
// but eagerly includes ItemSessions (with ReviewVerdict) for cost/status display.
type BacklogItemSummary struct {
	ID                 string               `json:"id"`
	ExternalID         string               `json:"external_id"`
	ExternalURL        string               `json:"external_url"`
	Labels             []string             `json:"labels"`
	Title              string               `json:"title"`
	Status             BacklogStatus        `json:"status"`
	Priority           int                  `json:"priority"`
	RepoPath           string               `json:"repo_path"`
	AcceptanceCriteria AcCriteriaJSON       `json:"acceptance_criteria"`
	Notes              string               `json:"notes"`
	PrURL              string               `json:"pr_url"`
	PrNumber           int                  `json:"pr_number"`
	CreatedAt          time.Time            `json:"created_at"`
	UpdatedAt          time.Time            `json:"updated_at"`
	ArchivedAt         *time.Time           `json:"archived_at"`
	ItemSessions       []ItemSessionSummary `json:"-"`
}

// ItemSessionBacklogEntry is a lightweight join record linking a tmux session UUID
// to its parent backlog item's metadata. Returned by GetAllItemSessionsWithBacklogInfo.
type ItemSessionBacklogEntry struct {
	SessionUUID string
	SessionRole string
	ItemID      string
	ItemTitle   string
	ItemStatus  string
}

// BacklogItemFilter controls which items ListBacklogItems returns.
type BacklogItemFilter struct {
	// Statuses restricts results to these statuses. Empty means no restriction.
	Statuses []string
	// Priorities restricts results to these priority values. Empty means no restriction.
	Priorities []int
	// SortBy controls ordering ("priority", "updated_at"). Empty means default ordering.
	SortBy string
	// ExcludeDone, when true and Statuses is empty, excludes items with status
	// "done". Independent of ExcludeArchived — split into two flags (rather
	// than one combined "ExcludeTerminal") so a caller can show done items by
	// default while still hiding archived ones. Renamed from ExcludeTerminal,
	// which used to combine both; verified via grep that ListBacklogItems and
	// ListBacklogItemSummaries were the only two callers of the old field, so
	// the rename is safe (no silent behavior change for any other caller).
	ExcludeDone bool
	// ExcludeArchived, when true and Statuses is empty, excludes items with
	// status "archived". Independent of ExcludeDone — see its doc comment.
	ExcludeArchived bool
	// Limit caps the number of results returned. 0 means use the default safety cap (1000).
	Limit int
	// Offset skips the first N results (for pagination). Only applied when Limit > 0.
	Offset int
	// ChainFired, when non-nil, restricts results to items whose chain_fired
	// column equals *ChainFired. Added so TriggerChainReconciler.ReconcileChains
	// (session/chain_firer.go) can push its "unfired pending chain" filter into
	// SQL instead of scanning every "done" item up to the default 1000-row
	// safety cap and filtering in Go — past 1000 done items, a pending unfired
	// chain outside that window was silently never reconciled (sdd:6-verify
	// finding). Backed by index.Fields("status", "chain_fired")
	// (session/ent/schema/backlog_item.go).
	ChainFired *bool
	// NextWorkflowIDSet, when non-nil, restricts results to items where
	// next_workflow_id IS NOT NULL (true) or IS NULL (false). See ChainFired's
	// doc comment — the two are combined by ReconcileChains's query.
	NextWorkflowIDSet *bool
}

// BacklogItemUpdate carries the mutable fields for UpdateBacklogItem.
type BacklogItemUpdate struct {
	Title              *string
	Description        *string
	AcceptanceCriteria *AcCriteriaJSON
	Priority           *int
	RepoPath           *string
	SkipReviewGate     *bool
	SkipPlanning       *bool
	AutoSpawnSession   *bool
	AutoCreatePR       *bool
	// PipelineMode is a pointer for partial-update presence: nil means "leave
	// the item's stored pipeline_mode untouched", while a non-nil pointer
	// (including one pointing at "") explicitly sets/resets it. See
	// BacklogItemData.PipelineMode for the field's semantics.
	PipelineMode *string
	// Category is a pointer for partial-update presence: nil means "leave the
	// item's stored category untouched", while a non-nil pointer (including
	// one pointing at "") explicitly sets/clears it. See
	// BacklogItemData.Category for the field's semantics.
	Category *string
	Notes    *string
	// ExternalURL and Labels follow the same partial-update-presence
	// convention as the other pointer fields on this struct: nil means "leave
	// untouched", a non-nil pointer (including one pointing at "" / an empty
	// slice) explicitly sets it.
	ExternalURL       *string
	Labels            *[]string
	PlanApproved      *bool
	PlanApprovedAt    *time.Time
	PlanArtifactsPath *string
	// PlanRejectionReason and PlanRejectedAt follow the same partial-update-
	// presence convention: nil means "leave untouched", a non-nil pointer
	// explicitly sets it. Since a plain pointer can't distinguish "leave
	// untouched" from "clear it back to nil", use ClearPlanRejectedAt to
	// explicitly clear the timestamp back to nil (e.g. alongside resetting
	// PlanRejectionReason back to "" on approval/re-triage) — see
	// PrFeedbackAddressedAt/ClearPrFeedbackAddressedAt below for the same
	// pattern.
	PlanRejectionReason *string
	PlanRejectedAt      *time.Time
	ClearPlanRejectedAt bool
	// QueuedAt and QueuedAutonomous follow the same partial-update-presence
	// convention as PlanApprovedAt: nil means "leave untouched".
	QueuedAt         *time.Time
	QueuedAutonomous *bool
	PrURL            *string
	PrNumber         *int
	// ShippedCheckConclusion, ShippedApprovedCount, ShippedChangesReqCount,
	// ShippedSnapshotAt, ShippedFileStats, and ShippedSnapshotCaptureFailed
	// are pointers for partial-update presence, following the existing
	// convention: nil means "leave the item's stored value untouched", a
	// non-nil pointer explicitly sets it. See BacklogItemData's fields of
	// the same name for semantics.
	ShippedCheckConclusion       *string
	ShippedApprovedCount         *int
	ShippedChangesReqCount       *int
	ShippedSnapshotAt            *time.Time
	ShippedFileStats             *string
	ShippedSnapshotCaptureFailed *bool
	// PrFeedbackAddressedAt follows the same partial-update-presence
	// convention: nil means "leave untouched", a non-nil pointer sets the
	// comment-feedback dedup watermark. Since a plain pointer can't
	// distinguish "leave untouched" from "clear it back to nil", use
	// ClearPrFeedbackAddressedAt to explicitly clear it (e.g. when a PR
	// closes without merging and a fresh PR should start with a clean
	// watermark).
	PrFeedbackAddressedAt      *time.Time
	ClearPrFeedbackAddressedAt bool
	// GitHubSyncedIssueUpdatedAt follows the same partial-update-presence
	// convention as PrFeedbackAddressedAt: nil means "leave untouched", a
	// non-nil pointer sets the loop-prevention watermark. Use
	// ClearGitHubSyncedIssueUpdatedAt to explicitly clear it back to nil.
	GitHubSyncedIssueUpdatedAt      *time.Time
	ClearGitHubSyncedIssueUpdatedAt bool
	// ReworkCapOverride follows the same single-pointer presence convention as
	// the fields above: nil means "leave untouched". A non-nil pointer sets the
	// item's override (0 = unlimited, >0 = this item's own cap). There is
	// currently no way to explicitly clear an override back to "use the global
	// default" via this struct — a deliberate simplification; add a
	// ClearReworkCapOverride bool alongside this if that's needed later.
	ReworkCapOverride *int
	// UserModifiedFields follows the same partial-update-presence convention:
	// nil means "leave untouched", a non-nil pointer sets the stored
	// JSON-encoded set of user-modified field names (e.g. `["title"]`). Build
	// the value with MergeUserModifiedFields rather than hand-encoding JSON.
	UserModifiedFields *string
	// NextWorkflowID/ClearNextWorkflowID follow the same nillable-clear
	// convention as GitHubSyncedIssueUpdatedAt: nil+false means "leave
	// untouched", ClearNextWorkflowID=true explicitly clears the chain
	// configuration back to nil, otherwise a non-nil pointer sets it
	// (webhook-triggers FR10/AC5 — see BacklogItemData.NextWorkflowID).
	NextWorkflowID      *uuid.UUID
	ClearNextWorkflowID bool
	// ChainFired is a normal presence pointer (no clear semantics needed — it
	// only ever moves false->true, by ChainFirer/TriggerChainReconciler).
	ChainFired *bool
	// ChainedAt/ClearChainedAt follow the same nillable-clear convention as
	// NextWorkflowID above.
	ChainedAt      *time.Time
	ClearChainedAt bool
	// TriggeredByChainDepth is a normal presence pointer — non-nillable in the
	// schema (Default 0), so no clear semantics are needed.
	TriggeredByChainDepth *int
}

// BacklogItemPrecondition is used for optimistic locking on update/transition.
type BacklogItemPrecondition struct {
	// ExpectedStatus, if non-empty, requires the item's current status to match.
	ExpectedStatus string
	// ExpectedUpdatedAt, if non-zero, requires the item's updated_at to match.
	ExpectedUpdatedAt *time.Time
	// Note, if non-empty, is stored in the status event audit log alongside this
	// transition. Use it to record why the transition happened (e.g. "auto-reopened
	// after FAIL verdict").
	Note string
}

// ItemSourceData is the domain model for an external item source.
type ItemSourceData struct {
	ID                    string
	PluginID              string
	DisplayName           string
	Config                string // JSON, may contain encrypted token
	Enabled               bool
	ForwardSyncEnabled    bool
	BackwardSyncEnabled   bool
	ForwardSyncCloseLabel string
	TokenConfigured       bool
	LastSyncedAt          *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// ItemSourceUpdate carries the mutable fields for UpdateItemSource.
type ItemSourceUpdate struct {
	DisplayName           *string
	Enabled               *bool
	ForwardSyncEnabled    *bool
	BackwardSyncEnabled   *bool
	ForwardSyncCloseLabel *string
	Config                *string
}

// ShellRepository is the minimal persistence interface for per-session shell management.
// It is implemented by EntRepository; pass nil to disable persistence (e.g., tests).
type ShellRepository interface {
	// CreateShell persists a new shell record under the given session title.
	CreateShell(ctx context.Context, sessionTitle string, data ShellData) (*ent.Shell, error)
	// ListShells returns all shell records for the given session title, ordered by order_index.
	ListShells(ctx context.Context, sessionTitle string) ([]*ent.Shell, error)
	// UpdateShellStatus sets the status (and optionally exit code) for the shell with the given ID.
	UpdateShellStatus(ctx context.Context, shellID, status string, exitCode *int) error
	// DeleteShell removes the shell record with the given ID.
	DeleteShell(ctx context.Context, shellID string) error
}

// RepositoryOption is a function that configures a repository
type RepositoryOption func(interface{}) error

// WithDatabasePath sets the database file path for the repository
func WithDatabasePath(path string) RepositoryOption {
	return func(r interface{}) error {
		if entRepo, ok := r.(*EntRepository); ok {
			entRepo.dbPath = path
			return nil
		}
		return nil // No-op for unsupported repository types
	}
}
