// Package binaries provides per-binary BinaryDetector implementations.
package binaries

import (
	"regexp"
	"strings"

	"github.com/tstapler/stapler-squad/session/detection/dtypes"
)

// ClaudeDetector implements dtypes.BinaryDetector for the Claude Code CLI.
type ClaudeDetector struct{}

// NewClaudeDetector returns a new ClaudeDetector.
func NewClaudeDetector() *ClaudeDetector { return &ClaudeDetector{} }

// Name returns "claude".
func (d *ClaudeDetector) Name() string { return "claude" }

// FilterContent returns content unchanged (no binary-specific filtering for Claude).
func (d *ClaudeDetector) FilterContent(content string) string { return content }

// Patterns returns the Claude Code-specific status patterns. This is the
// single source of truth for Claude's default pattern set: session/detection's
// generic getDefaultPatterns() (used as the fallback for any program with no
// registered per-binary detector) delegates to this method rather than
// maintaining its own copy — see detector.go's getDefaultPatterns() doc
// comment. Historically this method held its own hand-copied, materially
// thinner subset that drifted out of sync with getDefaultPatterns() (missing
// WaitingForAgent entirely, a glyph-tolerant esc_to_interrupt variant, the
// ✦ spinner glyph, bracket_insert_mode, claude_accept_edits, and cross-tool
// Ready/Processing/NeedsApproval patterns) — that drift regressed any claude
// session resolved through the built-in registry entry onto a worse pattern
// set than sessions falling back to NewStatusDetector(). Keeping exactly one
// literal (here) makes that class of drift impossible.
func (d *ClaudeDetector) Patterns() dtypes.StatusPatterns {
	return dtypes.StatusPatterns{
		Ready: []dtypes.StatusPattern{
			{
				Name:        "claude_prompt",
				Pattern:     `.*`,
				Description: "Claude Code command prompt",
				Priority:    1,
			},
			// NOTE: agy (Antigravity CLI) — agy uses the same TUI codebase as Gemini CLI
			// (requirements confirmed: "same TUI codebase, rewritten core in Go"). The four
			// gemini_* patterns below (gemini_ready, gemini_working, gemini_permission,
			// gemini_allow_execution) cover agy sessions without additional patterns.
			//
			// If agy introduces divergent UI strings (e.g. rebranded permission dialog text)
			// in a future version, add agy_* pattern variants alongside the gemini_* entries
			// at the same priority levels.
			//
			// Gemini CLI status indicators
			{
				Name:        "gemini_ready",
				Pattern:     `(?:◇|✓).*(?:Ready|ready)`,
				Description: "Gemini CLI ready status (◇ Ready)",
				Priority:    5,
			},
		},
		Processing: []dtypes.StatusPattern{
			{
				Name: "thinking",
				// Match "Thinking/Processing/etc." as standalone action indicators.
				// Require the keyword at or near the start of a line to avoid matching
				// mid-sentence prose like "I was thinking about it".
				Pattern:     `(?im)^\s*\W{0,3}\s*(thinking|processing|analyzing|working)\b`,
				Description: "Claude is processing a command",
				Priority:    10,
			},
			{
				Name: "tool_use",
				// Require tool action keywords at the start of a line (or after
				// whitespace/indicator) followed by a path-like target, to avoid
				// matching prose like "currently running in".
				Pattern:     `(?im)^\s*(Reading|Writing|Editing|Executing|Running)\s+[./\w]`,
				Description: "Claude is using tools",
				Priority:    9,
			},
			{
				Name:        "opencode_arrow_action",
				Pattern:     `→\s*(Read|Write|Edit|Create|Delete)\b`,
				Description: "OpenCode tool action arrow (→ Read, → Write, etc.)",
				Priority:    10,
			},
			// Gemini CLI working status
			{
				Name:        "gemini_working",
				Pattern:     `(?:✦|⏲).*(?:Working|working)`,
				Description: "Gemini CLI working status (✦ Working)",
				Priority:    11,
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
			{
				Name:        "aider_permission",
				Pattern:     `\(Y\)es/\(N\)o/\(D\)on't ask again`,
				Description: "Aider permission prompt",
				Priority:    18,
			},
			{
				Name:        "gemini_permission",
				Pattern:     `(?i)Yes, allow once`,
				Description: "Gemini permission prompt",
				Priority:    17,
			},
			{
				Name:        "gemini_allow_execution",
				Pattern:     `(?i)Allow execution of:`,
				Description: "Gemini tool execution permission prompt",
				Priority:    19,
			},
			{
				Name:        "opencode_permission",
				Pattern:     `\[\s*Allow\s*\([aA]\)\s*\]`,
				Description: "OpenCode permission prompt with [ Allow (a) ] buttons",
				Priority:    18,
			},
		},
		Error: []dtypes.StatusPattern{
			{
				Name: "error_message",
				// (?im) enables case-insensitive multiline matching.
				// Anchors to start of line (^) OR after sentence-ending punctuation ([.!?]\s+)
				// so that "Error:" in the middle of a paragraph (e.g. last N bytes of output)
				// is still detected, while still avoiding false positives from indented
				// shell/YAML content like `echo "ERROR: ..."` where ERROR is not at a
				// line boundary or sentence boundary.
				// error[\s:] matches "Error:" (colon) and "Error " (space) to catch
				// variants like "Error occurred" and "Error while processing".
				Pattern:     `(?im)(^|[.!?]\s+)(error[\s:]|fatal error|exception:|traceback|panic:)`,
				Description: "Generic error indicators (not test failures)",
				Priority:    30,
			},
			{
				Name: "connection_error",
				// (?im) case-insensitive multiline; require these to appear on their own
				// line to avoid matching variable names or code paths that contain these words.
				Pattern:     `(?im)^.*(connection refused|network timeout|network error)`,
				Description: "Network and connection errors",
				Priority:    29,
			},
		},
		// TestsFailing: DISABLED - These patterns cause too many false positives.
		// Test output varies wildly across languages/frameworks, and matching "FAIL"
		// anywhere in output catches non-test-related content. Focus on Claude's
		// actual status indicators (active, idle, approval, error) instead.
		TestsFailing: []dtypes.StatusPattern{},
		Idle: []dtypes.StatusPattern{
			{
				Name:        "insert_mode",
				Pattern:     `—\s*INSERT\s*—`,
				Description: "Claude Code in INSERT mode, waiting for input",
				Priority:    15,
			},
			{
				Name: "claude_readline_prompt",
				// Matches the Claude Code readline prompt ">" optionally followed by
				// the terminal cursor block character ▌ (U+258C) which tmux capture-pane
				// includes when the cursor sits on the prompt line.
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
				Name:        "bracket_insert_mode",
				Pattern:     `\[INSERT\]`,
				Description: "Gemini/editor in INSERT mode (bracket format)",
				Priority:    15,
			},
			{
				Name:        "claude_shortcuts_prompt",
				Pattern:     `\?\s+for shortcuts`,
				Description: "Claude Code idle prompt showing ? for shortcuts",
				Priority:    15,
			},
			{
				Name:        "claude_accept_edits",
				Pattern:     `⏵⏵\s+accept edits on`,
				Description: "Claude Code 'accept edits' review mode — session completed turn, user reviews proposed changes",
				Priority:    15,
			},
		},
		Active: []dtypes.StatusPattern{
			{
				Name: "esc_to_interrupt",
				// Matches "esc to interrupt" and "esc to cancel". Also matches when the
				// terminal cursor sits at column 0 of the line, replacing the 'e' with a
				// half-block glyph (▊ U+258A, ▌ U+258C, etc.) — a tmux capture-pane
				// rendering artifact seen as "[▊]sc to interrupt" in real captures.
				Pattern:     `[e▊▌▍▋▎▏█]sc\s+(to\s+)?(interrupt|cancel)`,
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
				Name: "claude_thinking_verb",
				// Full spinner frame set: · ✢ ✳ ✶ ✻ ✽ (macOS bounce cycle), * (legacy),
				// ● (reduced-motion), ✦ (Claude Code primary spinner U+2726 BLACK FOUR POINTED STAR).
				// Direct UTF-8 embedding in [...] is valid RE2; \uXXXX escapes are NOT supported.
				// Verb char class extends \w with hyphens (Dilly-dallying), apostrophes (Beboppin'),
				// and Latin-1 accented chars (Flambéing, Sautéing) — Go RE2 \w = [0-9A-Za-z_] only.
				// [ \t]* allows leading whitespace so indented spinners (e.g. task manager sub-items)
				// are detected: "  ✽ Roosting… (9m 52s · ↓ 2.8k tokens)"
				//
				// Two tail alternatives after the verb:
				//   1. Ellipsis/dots immediately follow the verb (original shape), with anything
				//      allowed afterward — e.g. "Roosting… (9m 52s · ↓ 2.8k tokens)".
				//   2. A batched multi-tool-call summary clause trails the verb — e.g. "Searching
				//      for 12 patterns, reading 3 files, running 1 shell command…". Deliberately
				//      narrow, not "any content containing a digit": requires the literal "for
				//      <N>" immediately after the verb (every observed batched-summary line has
				//      this exact shape) and the terminator to be a real ellipsis or exactly three
				//      dots (never a single/double dot) with nothing after it but trailing
				//      whitespace ([ \t]*$). A looser version of this branch (verb + arbitrary
				//      digit-bearing content + 1-3 trailing dots, unanchored to "for") was caught in
				//      review false-matching ordinary single-sentence bullet completions that
				//      happen to contain a count, e.g. "Added 3 new tests to cover edge cases…" and
				//      "Fixed 3 bugs." — both are common Claude output shapes, not contrived; see
				//      TestBug_BatchedToolCallSummary_ProseNotFalsePositive for the locked-in
				//      negative cases. (The multi-sentence-prose guard below — mid-line period, more
				//      text follows — predates that review and is a separate, narrower concern; see
				//      claude_cost_summary.txt: "I've completed the implementation. Here's a
				//      summary...".)
				Pattern: `(?m)^[ \t]*[·✢✳✶✻✽●*✦][ \t]+[A-Z][a-zA-Z'\-éèêàâùûôîïëüöäÿæœ]*(?:…|\.{1,3}|\s+for\s+\d+[^\n.…]*(?:…|\.{3})[ \t]*$)`,
				Description: "Claude thinking state with random verb — any spinner frame + capitalized verb + ellipsis " +
					"(optionally followed by a batched tool-call summary clause: 'for N ...')",
				Priority: 26,
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
				Name: "verb_duration_completion",
				// ✻ (asterism U+273B), ◉ (fisheye U+25C9), and ✦ (black four pointed
				// star U+2726, Claude Code primary spinner) are used as the turn-completion
				// bullet. The verb is a random past-tense word that rotates each turn
				// (Baked, Cooked, Pondered, Synthesized, etc.).
				Pattern:     `[✻◉✦]\s+\w+\s+for\s+\d+[hms]`,
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
		Compacting: []dtypes.StatusPattern{
			{
				Name: "compacting_conversation",
				// INFERRED, not verified against a live Claude Code capture — see Story 1.1.1
				// in project_plans/context-compaction-detection/implementation/plan.md for
				// provenance and the required follow-up verification item. Requires
				// "Compacting" capitalized at/near line start (optionally after a spinner
				// glyph) so this does NOT match the unrelated "NN% until auto-compact"
				// approaching-threshold indicator (lowercase "compact", mid-line) — see
				// claude_active.txt:5, claude_thinking_verb.txt:5, claude_asterism_active.txt:5.
				Pattern:     `(?im)^[ \t]*[·✢✳✶✻✽●*✦⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏]?[ \t]*Compacting\b`,
				Description: "Claude is compacting conversation history to free up context space",
				Priority:    26,
			},
		},
		WaitingForAgent: []dtypes.StatusPattern{
			{
				Name: "waiting_for_background_agent",
				// Matches Claude Code's "✻ Waiting for N background agent(s) to finish" and
				// "✻ Waiting for N dynamic workflow(s) to finish" lines.
				// ✻ (U+273B), ◉ (U+25C9), and ✦ (U+2726, primary spinner) are all used
				// as the turn-marker bullet.
				Pattern:     `[✻◉✦]\s+Waiting for (\d+) (?:background agent|dynamic workflow)`,
				Description: "Claude is waiting for one or more background agents or dynamic workflows to finish",
				Priority:    27,
			},
			{
				Name: "shells_still_running",
				// Matches "✻ Churned for 52s · 1 shell still running" and similar lines that
				// appear when Claude finishes a turn but background shell processes are still
				// active. The "N shell(s) still running" suffix overrides the turn-completion
				// verb-duration marker — the session is not done yet.
				// Also matches bare "N shell(s) running" / "N shells still running" variants
				// found in the Claude Code bottom status bar.
				Pattern:     `(\d+)\s+shells?\s+(?:still\s+)?running`,
				Description: "Background shell processes still running — session not yet idle",
				Priority:    27,
			},
			{
				Name: "monitors_still_running",
				// Matches "✻ Cogitated for 18m 41s · 1 monitor still running" and similar
				// lines that appear when Claude finishes a turn but background monitors (e.g.
				// CI run watchers) are still active. Requires "still" to avoid false positives
				// on generic "N monitors running" output from display tools, Prometheus, etc.
				Pattern:     `(\d+)\s+monitors?\s+still\s+running`,
				Description: "Background monitors still running — session not yet idle",
				Priority:    27,
			},
		},
		InputRequired: []dtypes.StatusPattern{
			// Claude Code's AskUserQuestion prompts have a very specific format:
			// "Do you want to proceed?"
			// " ❯ 1. Yes"
			// "   2. Type here to tell Claude what to do differently"
			//
			// We detect this by looking for the numbered option selector pattern.
			// This is much more reliable than trying to match generic question text.
			{
				Name: "numbered_option_selector",
				// Matches Claude/Gemini numbered selection format with arrow/bullet selector.
				// Uses ❯ and ● (not >) to avoid false positives on markdown blockquotes
				// like "> 1. Clone the repository".
				Pattern:     `[❯●]\s*\d+\.\s+\w`,
				Description: "Selection prompt with numbered options",
				Priority:    16,
			},
			{
				Name: "opencode_bar_prefixed_options",
				// Matches OpenCode's ┃-prefixed numbered options in the footer area.
				// Example: "┃  4. Icons:" or "┃ 1. Option A"
				Pattern:     `┃\s*\d+\.\s+\w`,
				Description: "OpenCode bar-prefixed numbered option selector",
				Priority:    17,
			},
			{
				Name: "opencode_permission_buttons",
				// Matches OpenCode's permission dialog buttons.
				// Example: "Allow once   Allow always   Reject"
				Pattern:     `(?i)Allow\s+once.*Allow\s+always|Allow\s+always.*Allow\s+once`,
				Description: "OpenCode permission dialog buttons",
				Priority:    18,
			},
		},
	}
}

// oscBrailleSpinnerRegex matches any Braille Pattern character (U+2800-U+28FF),
// the full block deliberately broader than the hand-listed frame sets used for
// screen-text matching above — OSC titles are short, so the false-positive
// risk is low.
var oscBrailleSpinnerRegex = regexp.MustCompile(`[\x{2800}-\x{28FF}]`)

// oscIdleGlyph is the exact glyph (U+2733 EIGHT SPOKED ASTERISK) Claude Code's
// OSC window title uses to signal idle/done — NOT the visually similar ✻
// (U+273B) or ✽ (U+273D) used elsewhere in this file's screen-text patterns.
const oscIdleGlyph = '✳'

// ClassifyOSCTitle inspects a Claude Code OSC window-title payload (already
// extracted via pkg/ansi.ExtractLastOSC) and returns a definitive OSC-derived
// status, or ok=false for an unrecognized title (callers fall back to
// text-pattern detection). Spinner is checked before the idle glyph: treating
// an ambiguous title as "still executing" is the lower-cost mistake.
func ClassifyOSCTitle(title string) (dtypes.OSCStatus, bool) {
	if title == "" {
		return dtypes.OSCStatusNone, false
	}
	if oscBrailleSpinnerRegex.MatchString(title) {
		return dtypes.OSCStatusExecuting, true
	}
	if strings.ContainsRune(title, oscIdleGlyph) {
		return dtypes.OSCStatusIdle, true
	}
	return dtypes.OSCStatusNone, false
}
