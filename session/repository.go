package session

import (
	"context"
	"github.com/tstapler/stapler-squad/session/domain"
	"time"
)

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
	MinSessionIdleMinutes int32
}

// TaggingRuleData is the domain model for a user-editable tagging rule.
// Sibling of ApprovalRuleData — same shape convention, same unique-rule_id +
// atomic-upsert concurrency guarantee.
type TaggingRuleData struct {
	RuleID         string
	Name           string
	NamePattern    string
	BranchPattern  string
	PathPattern    string
	ProgramPattern string
	RequiredTags   []string
	OutputTag      string
	Priority       int
	Enabled        bool
	Source         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// DismissedFindingData is the domain model for a dismissed WasteFinding
// record. FindingID is the stable content-addressed dismissal key (see
// tokens.ComputeFindingID); SessionID/ConversationID/FindingType are kept
// alongside it only for debuggability, not for lookups.
type DismissedFindingData struct {
	FindingID      string
	SessionID      string
	ConversationID string
	FindingType    int32
	DismissedAt    time.Time
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
	// Source identifies which agent's hook produced this request ("claude" or
	// "pi"). Defaulted to "claude" at the recording boundary when empty — see
	// pi-support Epic 4.3 / PermissionRequestPayload.Source.
	Source    string
	CreatedAt time.Time
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

// ShellRepository is the minimal persistence interface for per-session shell management.
// It is implemented by EntRepository; pass nil to disable persistence (e.g., tests).
type ShellRepository interface {
	// CreateShell persists a new shell record under the given session title.
	CreateShell(ctx context.Context, sessionTitle string, data domain.ShellData) error
	// ListShells returns all shell records for the given session title, ordered by order_index.
	ListShells(ctx context.Context, sessionTitle string) ([]domain.ShellData, error)
	// UpdateShellStatus sets the status (and optionally exit code) for the shell with the given ID.
	UpdateShellStatus(ctx context.Context, shellID, status string, exitCode *int) error
	// DeleteShell removes the shell record with the given ID.
	DeleteShell(ctx context.Context, shellID string) error
}

// InitialPromptRepository is the minimal persistence interface for recording when
// Instance.InitialPrompt was actually sent. It is implemented by EntRepository;
// pass nil to disable persistence (e.g., tests).
type InitialPromptRepository interface {
	// UpdateInitialPromptSentAt sets the initial_prompt_sent_at field for a session.
	UpdateInitialPromptSentAt(ctx context.Context, title string, t time.Time) error
}

// RepositoryOption is a function that configures a repository
type RepositoryOption func(interface{}) error

// WithDatabasePath sets the database file path for the repository
func WithDatabasePath(path string) RepositoryOption {
	return func(r interface{}) error {
		if entRepo, ok := r.(*EntRepository); ok {
			entRepo.DbPath = path
			return nil
		}
		return nil // No-op for unsupported repository types
	}
}
