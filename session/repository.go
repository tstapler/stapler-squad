package session

import (
	"time"
)

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
