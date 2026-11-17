package domain

import "time"

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
