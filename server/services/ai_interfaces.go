package services

import "context"

// Redaction sentinel constants used when suppressing secrets before storage or
// prompt injection. Use these instead of inline string literals so grep-based
// audits can find every redaction site.
const (
	// redactedSecret is written to analytics storage when a secret is detected
	// in a command and must not be persisted.
	redactedSecret = "[REDACTED: secret detected]"
	// redactedPrompt is inserted into AI prompts in place of a command that
	// triggered the secret scanner (defense-in-depth, per FLAG-1).
	redactedPrompt = "[REDACTED]"
)

// RulePromptContext carries all domain data needed to build a suggestion prompt.
// Assembled by RulesService before passing to a RulePromptBuilder.
type RulePromptContext struct {
	ExistingRules  []RuleSpec     // user + seed + claude-settings rules
	AnalyticsGaps  []AnalyticsGap // unmatched commands grouped by tool/program
	CommandSample  string         // for COMMAND_SAMPLE source
	ToolNameFilter string         // optional single-tool scope
	ProgramFilter  string         // optional single-program scope
	WindowDays     int
}

// AnalyticsGap groups escalated, rule-less analytics entries by (ToolName, Program).
type AnalyticsGap struct {
	ToolName           string
	Program            string
	Count              int
	RepresentativeCmds []string // up to 5 truncated command previews
}

// RulePromptBuilder assembles system and user prompt strings from domain context.
// Implementations are pure functions — no I/O, no external calls. Fully testable.
type RulePromptBuilder interface {
	BuildSystemPrompt(ctx RulePromptContext) string
	BuildUserPrompt(ctx RulePromptContext) string
}

// AIClient sends assembled prompts to an AI backend and returns the raw response.
// ctx cancellation must abort the outbound request.
type AIClient interface {
	Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}
