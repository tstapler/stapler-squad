package detection

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Status represents the current status of a Claude instance based on PTY output analysis.
// This extends the existing Status type in instance.go with additional detection capabilities.
type DetectedStatus int

const (
	StatusUnknown DetectedStatus = iota
	StatusReady
	StatusProcessing
	StatusNeedsApproval
	StatusInputRequired // Explicit user input prompts (questions, "enter X:", etc.)
	StatusError
	StatusTestsFailing // Tests are failing
	StatusIdle         // Waiting for user input (INSERT mode, command prompt, etc.)
	StatusActive       // Actively executing commands (shows "esc to interrupt")
	StatusSuccess      // Task completed successfully
)

// StatusPattern represents a regex pattern for detecting a specific status.
type StatusPattern struct {
	Name        string `yaml:"name"`
	Pattern     string `yaml:"pattern"`
	Description string `yaml:"description"`
	Priority    int    `yaml:"priority"` // Higher priority patterns checked first
}

// StatusPatterns contains all patterns for status detection.
type StatusPatterns struct {
	Ready         []StatusPattern `yaml:"ready"`
	Processing    []StatusPattern `yaml:"processing"`
	NeedsApproval []StatusPattern `yaml:"needs_approval"`
	InputRequired []StatusPattern `yaml:"input_required"` // Explicit input prompts
	Error         []StatusPattern `yaml:"error"`
	TestsFailing  []StatusPattern `yaml:"tests_failing"` // Tests are failing
	Idle          []StatusPattern `yaml:"idle"`          // Waiting for user input
	Active        []StatusPattern `yaml:"active"`        // Actively executing commands
	Success       []StatusPattern `yaml:"success"`       // Task completed successfully
}

// StatusDetector analyzes PTY output to determine the current status of a Claude instance.
type StatusDetector struct {
	patterns StatusPatterns
	// Cache compiled regexes for performance
	readyRegexes         []*regexp.Regexp
	processingRegexes    []*regexp.Regexp
	needsApprovalRegexes []*regexp.Regexp
	inputRequiredRegexes []*regexp.Regexp
	errorRegexes         []*regexp.Regexp
	testsFailingRegexes  []*regexp.Regexp
	idleRegexes          []*regexp.Regexp
	activeRegexes        []*regexp.Regexp
	successRegexes       []*regexp.Regexp
	// sessionID is set by SetSessionID() so detection events carry the owning session ID.
	sessionID string
	// ring is the per-detector in-memory detection event ring buffer.
	ring eventRing
}

// NewStatusDetector creates a new status detector with default patterns.
func NewStatusDetector() *StatusDetector {
	sd := &StatusDetector{
		patterns: getDefaultPatterns(),
	}
	sd.compilePatterns()
	return sd
}

// NewStatusDetectorFromFile creates a status detector with patterns loaded from a YAML file.
func NewStatusDetectorFromFile(path string) (*StatusDetector, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read status patterns file: %w", err)
	}

	var patterns StatusPatterns
	if err := yaml.Unmarshal(data, &patterns); err != nil {
		return nil, fmt.Errorf("failed to parse status patterns YAML: %w", err)
	}

	sd := &StatusDetector{
		patterns: patterns,
	}
	if err := sd.compilePatterns(); err != nil {
		return nil, err
	}

	return sd, nil
}

// LoadPatterns loads patterns from a YAML file.
func (sd *StatusDetector) LoadPatterns(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read status patterns file: %w", err)
	}

	var patterns StatusPatterns
	if err := yaml.Unmarshal(data, &patterns); err != nil {
		return fmt.Errorf("failed to parse status patterns YAML: %w", err)
	}

	sd.patterns = patterns
	return sd.compilePatterns()
}

// compilePatterns compiles all regex patterns for efficient matching.
func (sd *StatusDetector) compilePatterns() error {
	var err error

	// Compile ready patterns
	sd.readyRegexes = make([]*regexp.Regexp, len(sd.patterns.Ready))
	for i, pattern := range sd.patterns.Ready {
		sd.readyRegexes[i], err = regexp.Compile(pattern.Pattern)
		if err != nil {
			return fmt.Errorf("failed to compile ready pattern '%s': %w", pattern.Name, err)
		}
	}

	// Compile processing patterns
	sd.processingRegexes = make([]*regexp.Regexp, len(sd.patterns.Processing))
	for i, pattern := range sd.patterns.Processing {
		sd.processingRegexes[i], err = regexp.Compile(pattern.Pattern)
		if err != nil {
			return fmt.Errorf("failed to compile processing pattern '%s': %w", pattern.Name, err)
		}
	}

	// Compile needs approval patterns
	sd.needsApprovalRegexes = make([]*regexp.Regexp, len(sd.patterns.NeedsApproval))
	for i, pattern := range sd.patterns.NeedsApproval {
		sd.needsApprovalRegexes[i], err = regexp.Compile(pattern.Pattern)
		if err != nil {
			return fmt.Errorf("failed to compile needs_approval pattern '%s': %w", pattern.Name, err)
		}
	}

	// Compile input required patterns
	sd.inputRequiredRegexes = make([]*regexp.Regexp, len(sd.patterns.InputRequired))
	for i, pattern := range sd.patterns.InputRequired {
		sd.inputRequiredRegexes[i], err = regexp.Compile(pattern.Pattern)
		if err != nil {
			return fmt.Errorf("failed to compile input_required pattern '%s': %w", pattern.Name, err)
		}
	}

	// Compile error patterns
	sd.errorRegexes = make([]*regexp.Regexp, len(sd.patterns.Error))
	for i, pattern := range sd.patterns.Error {
		sd.errorRegexes[i], err = regexp.Compile(pattern.Pattern)
		if err != nil {
			return fmt.Errorf("failed to compile error pattern '%s': %w", pattern.Name, err)
		}
	}

	// Compile tests failing patterns
	sd.testsFailingRegexes = make([]*regexp.Regexp, len(sd.patterns.TestsFailing))
	for i, pattern := range sd.patterns.TestsFailing {
		sd.testsFailingRegexes[i], err = regexp.Compile(pattern.Pattern)
		if err != nil {
			return fmt.Errorf("failed to compile tests_failing pattern '%s': %w", pattern.Name, err)
		}
	}

	// Compile idle patterns
	sd.idleRegexes = make([]*regexp.Regexp, len(sd.patterns.Idle))
	for i, pattern := range sd.patterns.Idle {
		sd.idleRegexes[i], err = regexp.Compile(pattern.Pattern)
		if err != nil {
			return fmt.Errorf("failed to compile idle pattern '%s': %w", pattern.Name, err)
		}
	}

	// Compile active patterns
	sd.activeRegexes = make([]*regexp.Regexp, len(sd.patterns.Active))
	for i, pattern := range sd.patterns.Active {
		sd.activeRegexes[i], err = regexp.Compile(pattern.Pattern)
		if err != nil {
			return fmt.Errorf("failed to compile active pattern '%s': %w", pattern.Name, err)
		}
	}

	// Compile success patterns
	sd.successRegexes = make([]*regexp.Regexp, len(sd.patterns.Success))
	for i, pattern := range sd.patterns.Success {
		sd.successRegexes[i], err = regexp.Compile(pattern.Pattern)
		if err != nil {
			return fmt.Errorf("failed to compile success pattern '%s': %w", pattern.Name, err)
		}
	}

	return nil
}

// ansiStripRegex matches ANSI escape sequences for stripping
var ansiStripRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\x1b\][^\x07]*\x07`)

// stripANSI removes ANSI escape codes from text for cleaner pattern matching
func stripANSI(text string) string {
	return ansiStripRegex.ReplaceAllString(text, "")
}

// cursorUpRegex matches ANSI cursor-up escape sequences (\x1b[A or \x1b[NA).
var cursorUpRegex = regexp.MustCompile(`\x1b\[\d*A`)

// hasScreenOverwrite reports whether raw PTY bytes contain evidence of an in-progress
// spinner: a bare carriage return (not part of \r\n, which is a Windows newline) or
// an ANSI cursor-up escape sequence. Must be called on the raw output before
// collapseCarriageReturns() discards this information.
func hasScreenOverwrite(raw []byte) bool {
	s := string(raw)
	for i := 0; i < len(s); i++ {
		if s[i] == '\r' {
			// \r\n is a Windows newline — not a screen overwrite
			if i+1 < len(s) && s[i+1] == '\n' {
				continue
			}
			return true
		}
	}
	return cursorUpRegex.Match(raw)
}

// collapseCarriageReturns collapses CR-overwritten segments within each line,
// keeping only the final write. "foo\rbar" → "bar"; "\r\n" (Windows newline)
// is treated as a newline boundary and preserved.
func collapseCarriageReturns(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		// A trailing \r on a line segment is from a \r\n Windows newline; preserve it
		// by re-appending it after collapsing inner CR overwrites.
		trailingCR := strings.HasSuffix(line, "\r")
		if trailingCR {
			line = line[:len(line)-1]
		}
		if strings.ContainsRune(line, '\r') {
			segments := strings.Split(line, "\r")
			line = segments[len(segments)-1]
		}
		if trailingCR {
			line = line + "\r"
		}
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}

// StatusDetectionTailBytes is the number of bytes scanned from the tail of terminal
// output when determining session status. Matches DefaultIdleDetectorConfig().BufferSize.
const StatusDetectionTailBytes = 4096

// detectFromText runs all pattern checks on pre-processed text and raw bytes.
// Returns the matched status, the matched pattern's Name (or "" for screen-overwrite,
// "<none>" for no match), and the context description string.
// This is the shared core called by both Detect() and DetectWithContext().
//
// rawPTY must be the original PTY bytes before collapseCarriageReturns is applied.
func (sd *StatusDetector) detectFromText(text string, rawPTY []byte) (DetectedStatus, string, string) {
	// Error patterns (highest priority)
	for i, regex := range sd.errorRegexes {
		if regex.MatchString(text) {
			return StatusError, sd.patterns.Error[i].Name, sd.patterns.Error[i].Description
		}
	}
	// Tests failing
	for i, regex := range sd.testsFailingRegexes {
		if regex.MatchString(text) {
			return StatusTestsFailing, sd.patterns.TestsFailing[i].Name, sd.patterns.TestsFailing[i].Description
		}
	}
	// Success / task completion
	for i, regex := range sd.successRegexes {
		if regex.MatchString(text) {
			return StatusSuccess, sd.patterns.Success[i].Name, sd.patterns.Success[i].Description
		}
	}
	// Needs approval
	for i, regex := range sd.needsApprovalRegexes {
		if regex.MatchString(text) {
			return StatusNeedsApproval, sd.patterns.NeedsApproval[i].Name, sd.patterns.NeedsApproval[i].Description
		}
	}
	// Input required (explicit prompts)
	for i, regex := range sd.inputRequiredRegexes {
		if regex.MatchString(text) {
			return StatusInputRequired, sd.patterns.InputRequired[i].Name, sd.patterns.InputRequired[i].Description
		}
	}
	// Active (e.g. "esc to interrupt", thinking verb)
	for i, regex := range sd.activeRegexes {
		if regex.MatchString(text) {
			return StatusActive, sd.patterns.Active[i].Name, sd.patterns.Active[i].Description
		}
	}
	// Processing
	for i, regex := range sd.processingRegexes {
		if regex.MatchString(text) {
			return StatusProcessing, sd.patterns.Processing[i].Name, sd.patterns.Processing[i].Description
		}
	}
	// Screen-overwrite fallback: bare \r or ANSI cursor-up signals a spinner animating
	// in-place. Placed AFTER Active/Processing (visible text beats heuristic) but BEFORE
	// Idle/Ready catch-alls so a spinning session isn't misclassified as waiting.
	if hasScreenOverwrite(rawPTY) {
		return StatusActive, "screen_overwrite", "Screen overwrite — spinner actively redrawing"
	}
	// Idle (e.g. "— INSERT —", readline prompt)
	for i, regex := range sd.idleRegexes {
		if regex.MatchString(text) {
			return StatusIdle, sd.patterns.Idle[i].Name, sd.patterns.Idle[i].Description
		}
	}
	// Ready (includes ".*" catch-all — must be last)
	for i, regex := range sd.readyRegexes {
		if regex.MatchString(text) {
			return StatusReady, sd.patterns.Ready[i].Name, sd.patterns.Ready[i].Description
		}
	}
	return StatusUnknown, "<none>", ""
}

// appendDetectionEvent records the outcome of a detection call to the ring buffer.
// It acquires sd.ring.mu so that the sessionID read and the ring insert are atomic
// with respect to SetSessionID, eliminating the data race between the two.
func (sd *StatusDetector) appendDetectionEvent(status DetectedStatus, patternName, cleanedText string) {
	snippet := cleanedText
	if len(snippet) > TailSnippetBytes {
		snippet = snippet[len(snippet)-TailSnippetBytes:]
	}
	sd.ring.mu.Lock()
	defer sd.ring.mu.Unlock()
	sd.ring.pushLocked(DetectionEvent{
		SessionID:       sd.sessionID,
		Timestamp:       time.Now(),
		MatchedPattern:  patternName,
		MatchedCategory: categoryName(status),
		ResultStatus:    status,
		TailSnippet:     snippet,
	})
}

// SetSessionID sets the session identifier embedded in all future DetectionEvents.
// Call this once after creating the detector, before any detections run.
func (sd *StatusDetector) SetSessionID(id string) {
	sd.ring.mu.Lock()
	sd.sessionID = id
	sd.ring.mu.Unlock()
}

// RecentEvents returns up to n most-recent DetectionEvents, newest-first.
func (sd *StatusDetector) RecentEvents(n int) []DetectionEvent {
	return sd.ring.recent(n)
}

// Detect analyzes the provided PTY output and returns the detected status.
// Patterns are checked in priority order: Error > TestsFailing > Success > NeedsApproval > InputRequired > Active > Processing > Idle > Ready.
// Returns StatusUnknown if no patterns match.
func (sd *StatusDetector) Detect(output []byte) DetectedStatus {
	text := stripANSI(collapseCarriageReturns(string(output)))
	status, patternName, _ := sd.detectFromText(text, output)
	sd.appendDetectionEvent(status, patternName, text)
	return status
}

// DetectWithContext returns the detected status along with a user-friendly context message.
// Uses the pattern's Description field for human-readable messages instead of raw matched text.
func (sd *StatusDetector) DetectWithContext(output []byte) (DetectedStatus, string) {
	text := stripANSI(collapseCarriageReturns(string(output)))
	status, patternName, context := sd.detectFromText(text, output)
	sd.appendDetectionEvent(status, patternName, text)
	return status, context
}

// getDefaultPatterns returns the default status detection patterns for Claude Code.
func getDefaultPatterns() StatusPatterns {
	return StatusPatterns{
		Ready: []StatusPattern{
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
		Processing: []StatusPattern{
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
		NeedsApproval: []StatusPattern{
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
		Error: []StatusPattern{
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
		TestsFailing: []StatusPattern{},
		Idle: []StatusPattern{
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
		},
		Active: []StatusPattern{
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
				Name: "claude_thinking_verb",
				// Full spinner frame set: · ✢ ✳ ✶ ✻ ✽ (macOS bounce cycle), * (legacy),
				// ● (reduced-motion), ✦ (Claude Code primary spinner U+2726 BLACK FOUR POINTED STAR).
				// Direct UTF-8 embedding in [...] is valid RE2; \uXXXX escapes are NOT supported.
				// Verb char class extends \w with hyphens (Dilly-dallying), apostrophes (Beboppin'),
				// and Latin-1 accented chars (Flambéing, Sautéing) — Go RE2 \w = [0-9A-Za-z_] only.
				// [ \t]* allows leading whitespace so indented spinners (e.g. task manager sub-items)
				// are detected: "  ✽ Roosting… (9m 52s · ↓ 2.8k tokens)"
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
		Success: []StatusPattern{
			{
				Name:        "cost_summary_line",
				Pattern:     `\$\d+\.\d+\s+•`,
				Description: "Claude cost summary line — turn complete",
				Priority:    22,
			},
			{
				Name: "verb_duration_completion",
				// ✻ (asterism U+273B) and ◉ (fisheye U+25C9) are both used as the
				// turn-completion bullet. The verb is a random past-tense word that
				// rotates each turn (Baked, Cooked, Pondered, Synthesized, etc.).
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
		InputRequired: []StatusPattern{
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

// StatusString converts DetectedStatus to a human-readable string.
func (s DetectedStatus) String() string {
	switch s {
	case StatusReady:
		return "Ready"
	case StatusProcessing:
		return "Processing"
	case StatusNeedsApproval:
		return "Needs Approval"
	case StatusInputRequired:
		return "Input Required"
	case StatusError:
		return "Error"
	case StatusTestsFailing:
		return "Tests Failing"
	case StatusIdle:
		return "Idle"
	case StatusActive:
		return "Active"
	case StatusSuccess:
		return "Success"
	default:
		return "Unknown"
	}
}

// ExportPatterns exports the current patterns to a YAML file.
func (sd *StatusDetector) ExportPatterns(path string) error {
	data, err := yaml.Marshal(&sd.patterns)
	if err != nil {
		return fmt.Errorf("failed to marshal status patterns: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write status patterns file: %w", err)
	}

	return nil
}

// GetPatternNames returns the names of all loaded patterns for a given status.
func (sd *StatusDetector) GetPatternNames(status DetectedStatus) []string {
	var patterns []StatusPattern
	switch status {
	case StatusReady:
		patterns = sd.patterns.Ready
	case StatusProcessing:
		patterns = sd.patterns.Processing
	case StatusNeedsApproval:
		patterns = sd.patterns.NeedsApproval
	case StatusInputRequired:
		patterns = sd.patterns.InputRequired
	case StatusError:
		patterns = sd.patterns.Error
	case StatusTestsFailing:
		patterns = sd.patterns.TestsFailing
	case StatusIdle:
		patterns = sd.patterns.Idle
	case StatusActive:
		patterns = sd.patterns.Active
	case StatusSuccess:
		patterns = sd.patterns.Success
	default:
		return nil
	}

	names := make([]string, len(patterns))
	for i, p := range patterns {
		names[i] = p.Name
	}
	return names
}

// DetectFromString is a convenience method that accepts a string instead of []byte.
func (sd *StatusDetector) DetectFromString(output string) DetectedStatus {
	return sd.Detect([]byte(output))
}

// DetectForProgram detects the status for a specific program name.
// Currently delegates to Detect; reserved for future per-program pattern sets.
func (sd *StatusDetector) DetectForProgram(output []byte, program string) DetectedStatus {
	return sd.Detect(output)
}

// DetectFromLines analyzes multiple lines of output and returns the most relevant status.
// Lines are processed in reverse order (most recent first) so the current terminal
// state takes precedence over stale scrollback content.
//
// Blank/whitespace-only lines are skipped. StatusReady results are noted but the scan
// continues looking for a more specific status — this prevents the `.*` Ready catch-all
// from stopping the scan on an unrelated line (e.g. "PR #66") before reaching a real
// status pattern on an earlier line. StatusReady is returned as a fallback if no more
// specific status is found.
func (sd *StatusDetector) DetectFromLines(lines []string) DetectedStatus {
	bestSoFar := StatusUnknown
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) == "" {
			continue
		}

		// If the line contains \r (carriage return), scan its CR-split segments in
		// reverse (last-written first, matching visual display order).  The normal
		// collapseCarriageReturns path inside DetectFromString only keeps the LAST
		// segment, which can hide an earlier active indicator when a TUI overlay
		// (e.g. Claude Code's task manager panel) overwrites "esc to interrupt"
		// with "↑/↓ to select" on the same terminal row.
		//
		// Reverse-segment scan: the last segment takes precedence (it is what the
		// terminal actually shows), but if it is only Ready/Unknown we fall back to
		// earlier segments.  This preserves the existing "✻ Baked for X\r? for
		// shortcuts" → StatusIdle behaviour while fixing the active-session case.
		if strings.ContainsRune(lines[i], '\r') {
			segs := strings.Split(lines[i], "\r")
			for j := len(segs) - 1; j >= 0; j-- {
				if strings.TrimSpace(segs[j]) == "" {
					continue
				}
				s := sd.DetectFromString(segs[j])
				if s == StatusUnknown {
					continue
				}
				if s == StatusReady {
					if bestSoFar == StatusUnknown {
						bestSoFar = StatusReady
					}
					continue
				}
				// The last segment is always authoritative.
				// Earlier segments: only promote high-urgency statuses (Active, NeedsApproval,
				// InputRequired, Error) — these represent session states that can be visually
				// hidden by a TUI overlay writing via \r but still indicate the session needs
				// attention.  Low-urgency statuses (Success, Processing, Idle) in earlier
				// segments were overwritten and should not override the visual display.
				if j == len(segs)-1 || s == StatusActive || s == StatusNeedsApproval || s == StatusInputRequired || s == StatusError {
					return s
				}
				// Low-urgency earlier segment: record as candidate but keep scanning.
				if bestSoFar == StatusUnknown {
					bestSoFar = s
				}
			}
			continue // all segments of this CR line handled above
		}

		status := sd.DetectFromString(lines[i])
		if status == StatusUnknown {
			continue
		}
		if status != StatusReady {
			return status // specific match wins immediately
		}
		if bestSoFar == StatusUnknown {
			bestSoFar = StatusReady // note we saw Ready but keep looking
		}
	}
	return bestSoFar
}

// DetectWithContextFromLines analyzes lines in reverse order (most recent first) and returns
// the detected status with context. This ensures current terminal state (e.g. "? for shortcuts"
// on the last line) takes precedence over stale scrollback content (e.g. an old "esc to interrupt"
// from a previous turn that is still within the scanned window).
//
// Blank/whitespace-only lines are skipped. StatusReady is treated as a low-confidence
// fallback — the scan continues past Ready results looking for a more specific status,
// preventing the `.*` catch-all from masking real patterns on earlier lines.
func (sd *StatusDetector) DetectWithContextFromLines(lines []string) (DetectedStatus, string) {
	bestStatus := StatusUnknown
	bestDesc := ""
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) == "" {
			continue
		}

		// Mirror DetectFromLines: scan CR-split segments in reverse so a TUI overlay
		// that uses \r to overwrite "esc to interrupt" doesn't hide the active state.
		if strings.ContainsRune(lines[i], '\r') {
			segs := strings.Split(lines[i], "\r")
			for j := len(segs) - 1; j >= 0; j-- {
				if strings.TrimSpace(segs[j]) == "" {
					continue
				}
				s, desc := sd.DetectWithContext([]byte(segs[j]))
				if s == StatusUnknown {
					continue
				}
				if s == StatusReady {
					if bestStatus == StatusUnknown {
						bestStatus = StatusReady
						bestDesc = desc
					}
					continue
				}
				// The last segment is always authoritative.
				// Earlier segments: only promote high-urgency statuses (Active, NeedsApproval,
				// InputRequired, Error) — these represent session states that can be visually
				// hidden by a TUI overlay writing via \r but still indicate the session needs
				// attention.  Low-urgency statuses (Success, Processing, Idle) in earlier
				// segments were overwritten and should not override the visual display.
				if j == len(segs)-1 || s == StatusActive || s == StatusNeedsApproval || s == StatusInputRequired || s == StatusError {
					return s, desc
				}
				// Low-urgency earlier segment: record as candidate but keep scanning.
				if bestStatus == StatusUnknown {
					bestStatus = s
					bestDesc = desc
				}
			}
			continue
		}

		status, desc := sd.DetectWithContext([]byte(lines[i]))
		if status == StatusUnknown {
			continue
		}
		if status != StatusReady {
			return status, desc // specific match wins immediately
		}
		if bestStatus == StatusUnknown {
			bestStatus = StatusReady
			bestDesc = desc
		}
	}
	return bestStatus, bestDesc
}

// DetectRecent analyzes the most recent n bytes of output for status detection.
// This is optimized for real-time status monitoring.
func (sd *StatusDetector) DetectRecent(output []byte, n int) DetectedStatus {
	if n <= 0 || len(output) == 0 {
		return StatusUnknown
	}

	startPos := len(output) - n
	if startPos < 0 {
		startPos = 0
	}

	return sd.Detect(output[startPos:])
}

// HasPattern checks if a specific pattern name exists for the given status.
func (sd *StatusDetector) HasPattern(status DetectedStatus, name string) bool {
	patterns := sd.GetPatternNames(status)
	for _, p := range patterns {
		if strings.EqualFold(p, name) {
			return true
		}
	}
	return false
}
