package domain

import "time"

// RuleSpec is the JSON-serializable form of a Rule.
// CommandPattern and FilePattern are stored as strings (compiled on load).
type RuleSpec struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	ToolName       string    `json:"tool_name,omitempty"`
	ToolPattern    string    `json:"tool_pattern,omitempty"`
	ToolCategory   string    `json:"tool_category,omitempty"`
	CommandPattern string    `json:"command_pattern,omitempty"`
	FilePattern    string    `json:"file_pattern,omitempty"`
	Decision       string    `json:"decision"`   // "auto_allow" | "auto_deny" | "escalate"
	RiskLevel      string    `json:"risk_level"` // "low" | "medium" | "high" | "critical"
	Reason         string    `json:"reason,omitempty"`
	Alternative    string    `json:"alternative,omitempty"`
	Priority       int       `json:"priority"`
	Enabled        bool      `json:"enabled"`
	Source         string    `json:"source"` // "user" | "seed" | "claude-settings"
	CreatedAt      time.Time `json:"created_at"`

	// Structured CommandCriteria fields — mutually exclusive with commandPattern.
	Programs              []string `json:"programs,omitempty"`
	Subcommands           []string `json:"subcommands,omitempty"`
	BlockedSubcommands    []string `json:"blocked_subcommands,omitempty"`
	RequiredFlags         []string `json:"required_flags,omitempty"`
	ForbiddenFlags        []string `json:"forbidden_flags,omitempty"`
	RequiredFlagPrefixes  []string `json:"required_flag_prefixes,omitempty"`
	PythonModes           []string `json:"python_modes,omitempty"`
	SafePythonImportsOnly bool     `json:"safe_python_imports_only,omitempty"`
	RequireCIPassing      bool     `json:"require_ci_passing,omitempty"`
	MinSessionIdleMinutes int32    `json:"min_session_idle_minutes,omitempty"`
}
