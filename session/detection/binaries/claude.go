// Package binaries provides per-binary BinaryDetector implementations.
package binaries

import "github.com/tstapler/stapler-squad/session/detection/dtypes"

// ClaudeDetector implements dtypes.BinaryDetector for the Claude Code CLI.
type ClaudeDetector struct{}

// NewClaudeDetector returns a new ClaudeDetector.
func NewClaudeDetector() *ClaudeDetector { return &ClaudeDetector{} }

// Name returns "claude".
func (d *ClaudeDetector) Name() string { return "claude" }

// FilterContent returns content unchanged (no binary-specific filtering for Claude).
func (d *ClaudeDetector) FilterContent(content string) string { return content }

// Patterns returns the Claude Code-specific status patterns.
func (d *ClaudeDetector) Patterns() dtypes.StatusPatterns {
	return dtypes.StatusPatterns{
		Ready: []dtypes.StatusPattern{
			{
				Name:        "claude_prompt",
				Pattern:     `.*`,
				Description: "Claude Code command prompt",
				Priority:    1,
			},
		},
		Processing: []dtypes.StatusPattern{
			{
				Name:        "thinking",
				Pattern:     `(?im)^\s*\W{0,3}\s*(thinking|processing|analyzing|working)\b`,
				Description: "Claude is processing a command",
				Priority:    10,
			},
			{
				Name:        "tool_use",
				Pattern:     `(?im)^\s*(Reading|Writing|Editing|Executing|Running)\s+[./\w]`,
				Description: "Claude is using tools",
				Priority:    9,
			},
		},
		NeedsApproval: []dtypes.StatusPattern{
			{
				Name:        "file_permission_claude",
				Pattern:     `(?i)(Yes, allow reading|Yes, allow writing|Yes, allow once|No, and tell Claude)`,
				Description: "Claude Code file permission prompt",
				Priority:    20,
			},
			{
				Name:        "proceed_prompt",
				Pattern:     `(?i)Do you want to proceed\?`,
				Description: "Generic proceed confirmation",
				Priority:    19,
			},
		},
		InputRequired: []dtypes.StatusPattern{
			{
				Name:        "numbered_option_selector",
				Pattern:     `[❯●]\s*\d+\.\s+\w`,
				Description: "Selection prompt with numbered options",
				Priority:    16,
			},
		},
		Error: []dtypes.StatusPattern{
			{
				Name:        "error_message",
				Pattern:     `(?im)(^|[.!?]\s+)(error[\s:]|fatal error|exception:|traceback|panic:)`,
				Description: "Generic error indicators (not test failures)",
				Priority:    30,
			},
			{
				Name:        "connection_error",
				Pattern:     `(?im)^.*(connection refused|network timeout|network error)`,
				Description: "Network and connection errors",
				Priority:    29,
			},
		},
		TestsFailing: []dtypes.StatusPattern{},
		Idle: []dtypes.StatusPattern{
			{
				Name:        "insert_mode",
				Pattern:     `—\s*INSERT\s*—`,
				Description: "Claude Code in INSERT mode, waiting for input",
				Priority:    15,
			},
			{
				Name:        "claude_readline_prompt",
				Pattern:     `(?m)^>\s*▌?\s*$`,
				Description: "Claude Code readline input prompt",
				Priority:    16,
			},
			{
				Name:        "command_prompt",
				Pattern:     `\$\s*$`,
				Description: "Shell command prompt at end of output",
				Priority:    14,
			},
			{
				Name:        "vim_normal_mode",
				Pattern:     `—\s*NORMAL\s*—`,
				Description: "Vim in NORMAL mode",
				Priority:    13,
			},
			{
				Name:        "claude_shortcuts_prompt",
				Pattern:     `\?\s+for shortcuts`,
				Description: "Claude Code idle prompt showing ? for shortcuts",
				Priority:    15,
			},
		},
		Active: []dtypes.StatusPattern{
			{
				Name:        "esc_to_interrupt",
				Pattern:     `esc\s+(to\s+)?(interrupt|cancel)`,
				Description: "Active operation that can be interrupted or cancelled",
				Priority:    25,
			},
			{
				Name:        "synthesizing",
				Pattern:     `(?i)Synthesizing\.{0,3}`,
				Description: "Claude is synthesizing a response",
				Priority:    25,
			},
			{
				Name:        "claude_thinking_verb",
				Pattern:     `(?m)^[ \t]*[·✢✳✶✻✽●*✦][ \t]+[A-Z][a-zA-Z'\-éèêàâùûôîïëüöäÿæœ]*(?:…|\.{1,3})`,
				Description: "Claude thinking state with random verb — any spinner frame + capitalized verb + ellipsis",
				Priority:    26,
			},
			{
				Name:        "running_status",
				Pattern:     `Running\.{3,}`,
				Description: "Command actively running",
				Priority:    24,
			},
			{
				Name:        "progress_indicators",
				Pattern:     `[✓✔⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏★].*(?:ing|Processing|Working|Executing|Verifying|Testing|Building|Synthesizing)`,
				Description: "Progress indicators with action verbs",
				Priority:    23,
			},
			{
				Name:        "tool_execution_active",
				Pattern:     `(?i)(Executing|Verifying|Testing|Building|Deploying).*\(esc`,
				Description: "Tool execution with interrupt option",
				Priority:    22,
			},
		},
		Success: []dtypes.StatusPattern{
			{
				Name:        "cost_summary_line",
				Pattern:     `\$\d+\.\d+\s+•`,
				Description: "Claude cost summary line — turn complete",
				Priority:    22,
			},
			{
				Name:        "verb_duration_completion",
				Pattern:     `[✻◉]\s+\w+\s+for\s+\d+[hms]`,
				Description: "Claude turn complete — '<PastTenseVerb> for <duration>' format",
				Priority:    21,
			},
			{
				Name:        "task_complete",
				Pattern:     `(?i)(✓ Successfully completed|Task (completed|finished)|I've completed|All done)`,
				Description: "Task completed successfully",
				Priority:    20,
			},
			{
				Name:        "success_checkmark",
				Pattern:     `(?i)✓.*(?:complete|done|success|finished)`,
				Description: "Success indicator with completion words",
				Priority:    19,
			},
			{
				Name:        "finished_successfully",
				Pattern:     `(?i)(Finished successfully|Completed successfully)`,
				Description: "Explicit success confirmation",
				Priority:    18,
			},
			{
				Name:        "tests_passed",
				Pattern:     `(?i)(All tests passed|Tests: .*passed)`,
				Description: "Test suite completed successfully",
				Priority:    17,
			},
			{
				Name:        "build_success",
				Pattern:     `(?i)(Build succeeded|Build: SUCCESS)`,
				Description: "Build completed successfully",
				Priority:    16,
			},
		},
	}
}
