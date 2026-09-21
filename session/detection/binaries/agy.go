package binaries

import "github.com/tstapler/stapler-squad/session/detection/dtypes"

// AgyDetector implements dtypes.BinaryDetector for the Agy (Antigravity) CLI.
// Agy uses the same TUI codebase as Gemini CLI, so the patterns are equivalent
// but use an "agy_" prefix to keep names unique within the registry.
type AgyDetector struct{}

// NewAgyDetector returns a new AgyDetector.
func NewAgyDetector() *AgyDetector { return &AgyDetector{} }

// Name returns "agy".
func (d *AgyDetector) Name() string { return "agy" }

// FilterContent returns content unchanged.
func (d *AgyDetector) FilterContent(content string) string { return content }

// Patterns returns Agy-specific status patterns.
// The pattern strings are identical to GeminiDetector because agy shares the same
// TUI codebase; only the Names differ (agy_ prefix) so they remain unique.
func (d *AgyDetector) Patterns() dtypes.StatusPatterns {
	return dtypes.StatusPatterns{
		Ready: []dtypes.StatusPattern{
			{
				Name:        "agy_ready",
				Pattern:     `(?:◇|✓).*(?:Ready|ready)`,
				Description: "Agy CLI ready status (◇ Ready)",
				Priority:    5,
			},
		},
		Processing: []dtypes.StatusPattern{
			{
				Name:        "agy_working",
				Pattern:     `(?:✦|⏲).*(?:Working|working)`,
				Description: "Agy CLI working status (✦ Working)",
				Priority:    11,
			},
			{
				Name:        "agy_tool_bullet",
				Pattern:     `(?m)^\s*•\s*(Bash|Read|ManageTask|Edit|Write|Grep|Glob|WebFetch|WebSearch|TodoWrite|Task|Shell|Run)\b`,
				Description: "Agy tool-call bullet (• Bash/Read/Edit/...) — agent is working",
				Priority:    10,
			},
			{
				Name:        "agy_thought",
				Pattern:     `(?i)Thought for \d+s`,
				Description: "Agy thought summary (Thought for Ns) — agent is working",
				Priority:    10,
			},
		},
		NeedsApproval: []dtypes.StatusPattern{
			{
				Name:        "agy_requesting_permission",
				Pattern:     `(?i)Requesting permission for:`,
				Description: "Agy requesting permission header",
				Priority:    18,
			},
			{
				Name:        "agy_run_this_command",
				Pattern:     `(?i)Run this command\?`,
				Description: "Agy command approval prompt",
				Priority:    18,
			},
			{
				Name:        "agy_run_command_option",
				Pattern:     `(?i)Yes,\s*run\s*command`,
				Description: "Agy run command selection option",
				Priority:    17,
			},
			{
				Name:        "agy_permission",
				Pattern:     `(?i)Yes, allow once`,
				Description: "Agy permission prompt",
				Priority:    17,
			},
			{
				Name:        "agy_allow_execution",
				Pattern:     `(?i)Allow execution of:`,
				Description: "Agy tool execution permission prompt",
				Priority:    19,
			},
			{
				Name:        "agy_needs_approval_for",
				Pattern:     `(?i)needs\s+approval\s+for`,
				Description: "Agy subagent needs approval indicator",
				Priority:    19,
			},
			{
				Name:        "agy_ctrl_k_approve",
				Pattern:     `(?i)ctrl\+k\s+approve`,
				Description: "Agy ctrl+k approve shortcut prompt",
				Priority:    18,
			},
			{
				Name:        "agy_agent_blocked",
				Pattern:     `(?i)Blocked\s*·`,
				Description: "Agy agent blocked status indicator",
				Priority:    18,
			},
			{
				Name:        "agy_accept_file_edit",
				Pattern:     `(?i)Accept this file edit\?`,
				Description: "Agy file-edit approval prompt (Accept this file edit? with Yes/No options)",
				Priority:    19,
			},
			{
				Name:        "agy_pending_edit",
				Pattern:     `(?i)^Pending edit`,
				Description: "Agy pending-edit diff header awaiting an accept/reject decision",
				Priority:    18,
			},
		},
		InputRequired: []dtypes.StatusPattern{
			{
				Name:        "agy_numbered_option",
				Pattern:     `(?m)^>\s*\d+\.\s+\w`,
				Description: "Agy numbered option selector (> 1. Yes, ...) — agent awaits a choice",
				Priority:    16,
			},
		},
		Error: []dtypes.StatusPattern{
			// TODO: Add error patterns once real agy terminal output is captured.
			// agy shares the Gemini jetski TUI — check what Gemini shows on API errors.
			// Capture via: tmux capture-pane -p on a running agy session hitting a rate limit.
		},
		TestsFailing: []dtypes.StatusPattern{},
		Idle: []dtypes.StatusPattern{
			{
				Name:        "agy_idle_readline",
				Pattern:     `> ▌`,
				Description: "Agy readline input cursor on empty line (shared with Gemini idle TUI)",
				Priority:    5,
			},
			{
				Name:        "agy_idle_insert",
				Pattern:     `\[INSERT\]`,
				Description: "Agy INSERT mode indicator in status bar (shared with Gemini idle TUI)",
				Priority:    6,
			},
		},
		Active: []dtypes.StatusPattern{
			{
				Name:        "agy_active_running",
				Pattern:     `= Running Agent\.\.\.`,
				Description: "Agy running agent indicator (shared with Gemini active TUI)",
				Priority:    11,
			},
			{
				Name:        "agy_active_thinking",
				Pattern:     `Thinking\.\.\. \(esc to cancel`,
				Description: "Agy thinking spinner with cancel hint (shared with Gemini active TUI)",
				Priority:    12,
			},
			{
				Name:        "agy_esc_to_cancel",
				Pattern:     `[e▊▌▍▋▎▏█]sc\s+(to\s+)?(interrupt|cancel)`,
				Description: "Agy active operation with esc to interrupt/cancel hint (e.g. dialog footer)",
				Priority:    25,
			},
		},
		Success: []dtypes.StatusPattern{
			// TODO: Add success/completion patterns once real agy terminal output is captured.
			// After a task completes, agy likely returns to Ready state (agy_ready covers this).
		},
	}
}
