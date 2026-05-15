package session

import (
	"context"
	"errors"
	"time"

	"github.com/tstapler/stapler-squad/session/ent"
)

// ErrPreconditionFailed is returned when an optimistic-locking precondition check fails.
var ErrPreconditionFailed = errors.New("precondition failed: concurrent modification detected")

// ErrNotFound is returned when a requested entity does not exist.
var ErrNotFound = errors.New("not found")

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
	// TransitionBacklogItemStatus changes the status of a backlog item with optional precondition.
	TransitionBacklogItemStatus(ctx context.Context, id string, toStatus BacklogStatus, precondition *BacklogItemPrecondition) (*BacklogItemData, error)

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

// BacklogItemData is the domain model for a backlog item.
type BacklogItemData struct {
	ID                 string
	Title              string
	Description        string
	AcceptanceCriteria string // raw JSON []AcCriterion
	Priority           int
	Status             string
	RepoPath           string
	SkipReviewGate     bool
	SkipPlanning       bool
	PlanApproved       bool
	PlanApprovedAt     *time.Time
	PlanArtifactsPath  string
	Notes              string
	ExternalID         string
	ArchivedAt         *time.Time
	SourceID           string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	// ItemSessions holds the eagerly-loaded item sessions for this backlog item.
	// Only populated when explicitly loaded by the caller (e.g. GetBacklogItem).
	ItemSessions []*ent.ItemSession
}

// BacklogItemFilter controls which items ListBacklogItems returns.
type BacklogItemFilter struct {
	// Statuses restricts results to these statuses. Empty means no restriction.
	Statuses []string
	// Priorities restricts results to these priority values. Empty means no restriction.
	Priorities []int
	// SortBy controls ordering ("priority", "updated_at"). Empty means default ordering.
	SortBy string
	// ExcludeTerminal, when true, excludes items with status "done" or "archived".
	ExcludeTerminal bool
	// Limit caps the number of results returned. 0 means use the default safety cap (1000).
	Limit int
	// Offset skips the first N results (for pagination). Only applied when Limit > 0.
	Offset int
}

// BacklogItemUpdate carries the mutable fields for UpdateBacklogItem.
type BacklogItemUpdate struct {
	Title              *string
	Description        *string
	AcceptanceCriteria *string // raw JSON
	Priority           *int
	RepoPath           *string
	SkipReviewGate     *bool
	SkipPlanning       *bool
	Notes              *string
	PlanApproved       *bool
	PlanApprovedAt     *time.Time
	PlanArtifactsPath  *string
}

// BacklogItemPrecondition is used for optimistic locking on update/transition.
type BacklogItemPrecondition struct {
	// ExpectedStatus, if non-empty, requires the item's current status to match.
	ExpectedStatus string
	// ExpectedUpdatedAt, if non-zero, requires the item's updated_at to match.
	ExpectedUpdatedAt *time.Time
}

// ItemSourceData is the domain model for an external item source.
type ItemSourceData struct {
	ID              string
	PluginID        string
	DisplayName     string
	Config          string // JSON, may contain encrypted token
	Enabled         bool
	TokenConfigured bool
	LastSyncedAt    *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// ItemSourceUpdate carries the mutable fields for UpdateItemSource.
type ItemSourceUpdate struct {
	DisplayName *string
	Enabled     *bool
	Config      *string
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
