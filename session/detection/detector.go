package detection

import (
	"fmt"
	"os"
	"regexp"
	"strings"

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
	compiled    *regexp.Regexp
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

// compiledProgramPatterns holds a StatusPatterns set with pre-compiled regexes
// for a single program context.
type compiledProgramPatterns struct {
	patterns             StatusPatterns
	readyRegexes         []*regexp.Regexp
	processingRegexes    []*regexp.Regexp
	needsApprovalRegexes []*regexp.Regexp
	inputRequiredRegexes []*regexp.Regexp
	errorRegexes         []*regexp.Regexp
	testsFailingRegexes  []*regexp.Regexp
	idleRegexes          []*regexp.Regexp
	activeRegexes        []*regexp.Regexp
	successRegexes       []*regexp.Regexp
}

// StatusDetector analyzes PTY output to determine the current status of an AI tool session.
// It holds separate compiled pattern sets per program (claude, gemini, aider, opencode)
// plus a "" fallback that combines all patterns for backwards-compatible callers.
type StatusDetector struct {
	programs map[string]*compiledProgramPatterns
}

// NewStatusDetector creates a new status detector with default per-program patterns.
func NewStatusDetector() *StatusDetector {
	return &StatusDetector{
		programs: buildDefaultProgramPatterns(),
	}
}

// NewStatusDetectorFromFile creates a status detector with patterns loaded from a YAML file.
// The loaded patterns replace the fallback ("") pattern set; per-program sets are not affected.
func NewStatusDetectorFromFile(path string) (*StatusDetector, error) {
	sd := &StatusDetector{
		programs: make(map[string]*compiledProgramPatterns),
	}
	if err := sd.LoadPatterns(path); err != nil {
		return nil, err
	}
	return sd, nil
}

// LoadPatterns loads patterns from a YAML file, replacing the fallback ("") pattern set.
func (sd *StatusDetector) LoadPatterns(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read status patterns file: %w", err)
	}

	var patterns StatusPatterns
	if err := yaml.Unmarshal(data, &patterns); err != nil {
		return fmt.Errorf("failed to parse status patterns YAML: %w", err)
	}

	cp, err := compileStatusPatterns(patterns)
	if err != nil {
		return err
	}
	sd.programs[""] = cp
	return nil
}

// ExportPatterns exports the fallback ("") pattern set to a YAML file.
func (sd *StatusDetector) ExportPatterns(path string) error {
	cp := sd.getCompiledPatterns("")
	data, err := yaml.Marshal(&cp.patterns)
	if err != nil {
		return fmt.Errorf("failed to marshal status patterns: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write status patterns file: %w", err)
	}
	return nil
}

// getCompiledPatterns returns the compiled pattern set for the given program,
// falling back to the "" set if the program is unrecognized.
func (sd *StatusDetector) getCompiledPatterns(program string) *compiledProgramPatterns {
	if cp, ok := sd.programs[program]; ok {
		return cp
	}
	return sd.programs[""]
}

// ansiStripRegex matches ANSI escape sequences for stripping.
var ansiStripRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\x1b\][^\x07]*\x07`)

// stripANSI removes ANSI escape codes from text for cleaner pattern matching.
func stripANSI(text string) string {
	return ansiStripRegex.ReplaceAllString(text, "")
}

// detect runs the compiled patterns against pre-stripped text in priority order.
// Order: Error > TestsFailing > Success > NeedsApproval > InputRequired > Active > Processing > Idle > Ready.
func (cp *compiledProgramPatterns) detect(text string) DetectedStatus {
	for _, re := range cp.errorRegexes {
		if re.MatchString(text) {
			return StatusError
		}
	}
	for _, re := range cp.testsFailingRegexes {
		if re.MatchString(text) {
			return StatusTestsFailing
		}
	}
	for _, re := range cp.successRegexes {
		if re.MatchString(text) {
			return StatusSuccess
		}
	}
	for _, re := range cp.needsApprovalRegexes {
		if re.MatchString(text) {
			return StatusNeedsApproval
		}
	}
	for _, re := range cp.inputRequiredRegexes {
		if re.MatchString(text) {
			return StatusInputRequired
		}
	}
	for _, re := range cp.activeRegexes {
		if re.MatchString(text) {
			return StatusActive
		}
	}
	for _, re := range cp.processingRegexes {
		if re.MatchString(text) {
			return StatusProcessing
		}
	}
	for _, re := range cp.idleRegexes {
		if re.MatchString(text) {
			return StatusIdle
		}
	}
	for _, re := range cp.readyRegexes {
		if re.MatchString(text) {
			return StatusReady
		}
	}
	return StatusUnknown
}

// detectWithContext runs patterns and returns the matching pattern's Description alongside the status.
func (cp *compiledProgramPatterns) detectWithContext(text string) (DetectedStatus, string) {
	for i, re := range cp.errorRegexes {
		if re.MatchString(text) {
			return StatusError, cp.patterns.Error[i].Description
		}
	}
	for i, re := range cp.testsFailingRegexes {
		if re.MatchString(text) {
			return StatusTestsFailing, cp.patterns.TestsFailing[i].Description
		}
	}
	for i, re := range cp.successRegexes {
		if re.MatchString(text) {
			return StatusSuccess, cp.patterns.Success[i].Description
		}
	}
	for i, re := range cp.needsApprovalRegexes {
		if re.MatchString(text) {
			return StatusNeedsApproval, cp.patterns.NeedsApproval[i].Description
		}
	}
	for i, re := range cp.inputRequiredRegexes {
		if re.MatchString(text) {
			return StatusInputRequired, cp.patterns.InputRequired[i].Description
		}
	}
	for i, re := range cp.activeRegexes {
		if re.MatchString(text) {
			return StatusActive, cp.patterns.Active[i].Description
		}
	}
	for i, re := range cp.processingRegexes {
		if re.MatchString(text) {
			return StatusProcessing, cp.patterns.Processing[i].Description
		}
	}
	for i, re := range cp.idleRegexes {
		if re.MatchString(text) {
			return StatusIdle, cp.patterns.Idle[i].Description
		}
	}
	for i, re := range cp.readyRegexes {
		if re.MatchString(text) {
			return StatusReady, cp.patterns.Ready[i].Description
		}
	}
	return StatusUnknown, ""
}

// Detect analyzes the provided PTY output using all patterns (backwards-compatible fallback).
// Use DetectForProgram when the program name is known for more precise matching.
func (sd *StatusDetector) Detect(output []byte) DetectedStatus {
	return sd.DetectForProgram(output, "")
}

// DetectWithContext returns the detected status plus the matching pattern's description.
// Use DetectWithContextForProgram when the program name is known.
func (sd *StatusDetector) DetectWithContext(output []byte) (DetectedStatus, string) {
	return sd.DetectWithContextForProgram(output, "")
}

// DetectForProgram runs only patterns relevant to the given program (e.g. "claude", "gemini").
// Falls back to the merged all-patterns set if the program is unrecognized.
func (sd *StatusDetector) DetectForProgram(output []byte, program string) DetectedStatus {
	text := stripANSI(string(output))
	return sd.getCompiledPatterns(program).detect(text)
}

// DetectWithContextForProgram is like DetectForProgram but also returns the matching pattern's description.
func (sd *StatusDetector) DetectWithContextForProgram(output []byte, program string) (DetectedStatus, string) {
	text := stripANSI(string(output))
	return sd.getCompiledPatterns(program).detectWithContext(text)
}

// DetectFromString is a convenience method that accepts a string instead of []byte.
func (sd *StatusDetector) DetectFromString(output string) DetectedStatus {
	return sd.Detect([]byte(output))
}

// DetectFromLines analyzes multiple lines of output and returns the most relevant status.
// Lines are processed most-recent-first; the first match wins.
func (sd *StatusDetector) DetectFromLines(lines []string) DetectedStatus {
	for i := len(lines) - 1; i >= 0; i-- {
		status := sd.DetectFromString(lines[i])
		if status != StatusUnknown {
			return status
		}
	}
	return StatusUnknown
}

// DetectRecent analyzes the most recent n bytes of output for status detection.
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

// GetPatternNames returns the names of all loaded patterns for the given status
// from the fallback ("") pattern set.
func (sd *StatusDetector) GetPatternNames(status DetectedStatus) []string {
	cp := sd.getCompiledPatterns("")
	var patterns []StatusPattern
	switch status {
	case StatusReady:
		patterns = cp.patterns.Ready
	case StatusProcessing:
		patterns = cp.patterns.Processing
	case StatusNeedsApproval:
		patterns = cp.patterns.NeedsApproval
	case StatusInputRequired:
		patterns = cp.patterns.InputRequired
	case StatusError:
		patterns = cp.patterns.Error
	case StatusTestsFailing:
		patterns = cp.patterns.TestsFailing
	case StatusIdle:
		patterns = cp.patterns.Idle
	case StatusActive:
		patterns = cp.patterns.Active
	case StatusSuccess:
		patterns = cp.patterns.Success
	default:
		return nil
	}
	names := make([]string, len(patterns))
	for i, p := range patterns {
		names[i] = p.Name
	}
	return names
}

// HasPattern checks if a specific pattern name exists for the given status in the fallback set.
func (sd *StatusDetector) HasPattern(status DetectedStatus, name string) bool {
	for _, p := range sd.GetPatternNames(status) {
		if strings.EqualFold(p, name) {
			return true
		}
	}
	return false
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

// ── Pattern compilation ───────────────────────────────────────────────────────

func compileRegexSlice(patterns []StatusPattern) ([]*regexp.Regexp, error) {
	regexes := make([]*regexp.Regexp, len(patterns))
	for i, p := range patterns {
		re, err := regexp.Compile(p.Pattern)
		if err != nil {
			return nil, fmt.Errorf("failed to compile pattern '%s': %w", p.Name, err)
		}
		regexes[i] = re
	}
	return regexes, nil
}

func compileStatusPatterns(patterns StatusPatterns) (*compiledProgramPatterns, error) {
	cp := &compiledProgramPatterns{patterns: patterns}
	var err error

	if cp.readyRegexes, err = compileRegexSlice(patterns.Ready); err != nil {
		return nil, err
	}
	if cp.processingRegexes, err = compileRegexSlice(patterns.Processing); err != nil {
		return nil, err
	}
	if cp.needsApprovalRegexes, err = compileRegexSlice(patterns.NeedsApproval); err != nil {
		return nil, err
	}
	if cp.inputRequiredRegexes, err = compileRegexSlice(patterns.InputRequired); err != nil {
		return nil, err
	}
	if cp.errorRegexes, err = compileRegexSlice(patterns.Error); err != nil {
		return nil, err
	}
	if cp.testsFailingRegexes, err = compileRegexSlice(patterns.TestsFailing); err != nil {
		return nil, err
	}
	if cp.idleRegexes, err = compileRegexSlice(patterns.Idle); err != nil {
		return nil, err
	}
	if cp.activeRegexes, err = compileRegexSlice(patterns.Active); err != nil {
		return nil, err
	}
	if cp.successRegexes, err = compileRegexSlice(patterns.Success); err != nil {
		return nil, err
	}
	return cp, nil
}

func mustCompileStatusPatterns(patterns StatusPatterns) *compiledProgramPatterns {
	cp, err := compileStatusPatterns(patterns)
	if err != nil {
		panic(fmt.Sprintf("failed to compile default patterns: %v", err))
	}
	return cp
}

func mergePatterns(a, b StatusPatterns) StatusPatterns {
	return StatusPatterns{
		Ready:         append(append([]StatusPattern{}, a.Ready...), b.Ready...),
		Processing:    append(append([]StatusPattern{}, a.Processing...), b.Processing...),
		NeedsApproval: append(append([]StatusPattern{}, a.NeedsApproval...), b.NeedsApproval...),
		InputRequired: append(append([]StatusPattern{}, a.InputRequired...), b.InputRequired...),
		Error:         append(append([]StatusPattern{}, a.Error...), b.Error...),
		TestsFailing:  append(append([]StatusPattern{}, a.TestsFailing...), b.TestsFailing...),
		Idle:          append(append([]StatusPattern{}, a.Idle...), b.Idle...),
		Active:        append(append([]StatusPattern{}, a.Active...), b.Active...),
		Success:       append(append([]StatusPattern{}, a.Success...), b.Success...),
	}
}

// buildDefaultProgramPatterns constructs the per-program compiled pattern map.
// Each known program gets its own specific patterns merged with the common set.
// The "" key holds all patterns merged together for backwards-compatible callers.
func buildDefaultProgramPatterns() map[string]*compiledProgramPatterns {
	common := commonPatterns()

	perProgram := map[string]StatusPatterns{
		"claude":   claudePatterns(),
		"gemini":   geminiPatterns(),
		"aider":    aiderPatterns(),
		"opencode": opencodePatterns(),
	}

	result := make(map[string]*compiledProgramPatterns, len(perProgram)+1)

	// Build per-program sets: program-specific first, then common as fallback.
	// Program-specific patterns are checked before common ones within each status category.
	var allSpecific StatusPatterns
	for prog, specific := range perProgram {
		result[prog] = mustCompileStatusPatterns(mergePatterns(specific, common))
		allSpecific = mergePatterns(allSpecific, specific)
	}

	// "" = all patterns merged — used by Detect() for backwards compatibility.
	result[""] = mustCompileStatusPatterns(mergePatterns(allSpecific, common))

	return result
}

// ── Per-program pattern definitions ──────────────────────────────────────────

// commonPatterns returns patterns that apply across all AI tools:
// generic error/success detection, vim/shell UI states.
func commonPatterns() StatusPatterns {
	return StatusPatterns{
		Ready:      []StatusPattern{},
		Processing: []StatusPattern{},
		NeedsApproval: []StatusPattern{
			{
				Name:        "proceed_prompt",
				Pattern:     `(?i)Do you want to proceed\?`,
				Description: "Generic proceed confirmation",
				Priority:    19,
			},
		},
		InputRequired: []StatusPattern{},
		Error: []StatusPattern{
			{
				Name: "error_message",
				// (?im) enables case-insensitive multiline matching.
				// Anchors to start of line (^) OR after sentence-ending punctuation ([.!?]\s+)
				// so mid-paragraph "Error:" is still detected while avoiding false positives
				// from indented shell/YAML content where ERROR appears mid-sentence.
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
		// TestsFailing: DISABLED - too many false positives across languages/frameworks.
		TestsFailing: []StatusPattern{},
		Idle: []StatusPattern{
			{
				Name:        "insert_mode",
				Pattern:     `—\s*INSERT\s*—`,
				Description: "Vim INSERT mode, waiting for input",
				Priority:    15,
			},
			{
				Name:        "vim_normal_mode",
				Pattern:     `—\s*NORMAL\s*—`,
				Description: "Vim NORMAL mode",
				Priority:    13,
			},
			{
				Name:        "command_prompt",
				Pattern:     `\$\s*$`,
				Description: "Shell command prompt at end of output",
				Priority:    14,
			},
		},
		Active: []StatusPattern{
			{
				Name:        "synthesizing",
				Pattern:     `(?i)Synthesizing\.{0,3}`,
				Description: "Tool is synthesizing a response",
				Priority:    25,
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

// claudePatterns returns patterns specific to Claude Code's terminal UI.
func claudePatterns() StatusPatterns {
	return StatusPatterns{
		Ready: []StatusPattern{
			{
				Name:        "claude_prompt",
				Pattern:     `.*`,
				Description: "Claude Code command prompt",
				Priority:    1,
			},
		},
		Processing: []StatusPattern{
			{
				Name: "claude_thinking",
				// Require the word at the START of a line so we don't match it
				// mid-sentence ("current working directory", "analyzing the code").
				// Claude shows processing state as "Thinking...", "Analyzing...", etc.
				// at the beginning of a status line.
				Pattern:     `(?im)^(thinking|processing|analyzing|working)\.{0,3}`,
				Description: "Claude is thinking or processing",
				Priority:    10,
			},
			{
				Name: "claude_tool_use",
				// Require the verb at the START of a line so we don't match it
				// mid-sentence in Claude's conversational responses.
				// Claude's tool-use output always starts with the verb: "Reading foo.py",
				// "Writing bar.go" — not buried in prose like "currently running in?"
				Pattern:     `(?im)^(Reading|Writing|Editing|Executing|Running)\s+\S`,
				Description: "Claude is using a tool (reading/writing/executing)",
				Priority:    9,
			},
		},
		NeedsApproval: []StatusPattern{
			{
				Name:        "claude_file_permission",
				Pattern:     `(?i)(Yes, allow reading|Yes, allow writing|Yes, allow once|No, and tell Claude)`,
				Description: "Claude Code file permission prompt",
				Priority:    20,
			},
		},
		InputRequired: []StatusPattern{
			{
				// Claude Code's AskUserQuestion prompts show:
				//   ❯ 1. Yes
				//   2. No, and tell Claude what to do differently
				//
				// We match ONLY ❯ (U+276F), NOT ASCII >.
				// The > character is ubiquitous in terminal output: shell prompts,
				// Gradle output ("> Run gradlew tasks"), markdown blockquotes, heredocs.
				// Using > caused false positives for any program that outputs numbered lists.
				Name:        "numbered_option_selector",
				Pattern:     `❯\s+\d+\.\s+\S`,
				Description: "Selection prompt with numbered options",
				Priority:    16,
			},
		},
		Idle: []StatusPattern{
			{
				Name: "claude_shortcuts_hint",
				// "? for shortcuts" appears on the last line of the Claude Code UI
				// when idle and waiting for user input. Unique to this state.
				Pattern:     `\?\s+for shortcuts`,
				Description: "Claude Code idle prompt",
				Priority:    15,
			},
		},
		Active: []StatusPattern{
			{
				Name:        "claude_esc_to_interrupt",
				Pattern:     `esc to interrupt`,
				Description: "Claude Code active operation (interruptible)",
				Priority:    25,
			},
		},
		Success: []StatusPattern{},
	}
}

// geminiPatterns returns patterns specific to the Gemini CLI's terminal UI.
func geminiPatterns() StatusPatterns {
	return StatusPatterns{
		NeedsApproval: []StatusPattern{
			{
				Name: "gemini_action_required",
				// Gemini shows "Action Required" as the header of its approval dialog.
				Pattern:     `Action Required`,
				Description: "Gemini action required prompt",
				Priority:    17,
			},
			{
				Name: "gemini_allow_execution",
				// Gemini shows "Allow execution of: '<command>'?" for shell approval.
				Pattern:     `Allow execution of:`,
				Description: "Gemini shell execution approval",
				Priority:    17,
			},
		},
		Active: []StatusPattern{
			{
				Name: "gemini_thinking",
				// Gemini shows "Thinking... (esc to cancel, Ns)" while processing.
				Pattern:     `Thinking\.\.\.\s+\(esc to cancel`,
				Description: "Gemini is thinking",
				Priority:    25,
			},
			{
				Name: "gemini_running_agent",
				// Gemini shows "= Running Agent... (ctrl+o to expand)" when executing a tool.
				Pattern:     `= Running Agent\.\.\.`,
				Description: "Gemini is running an agent",
				Priority:    24,
			},
		},
		Idle: []StatusPattern{
			{
				Name: "gemini_insert_mode",
				// Gemini's status bar shows "[INSERT]" (not "— INSERT —" like vim) when
				// idle and waiting for user input. Unlike the Active state where "Thinking..."
				// is also visible, idle shows [INSERT] with no processing indicator.
				// When Active, gemini_thinking (priority 25) fires before this (priority 15).
				Pattern:     `\[INSERT\]`,
				Description: "Gemini CLI idle in INSERT mode, waiting for input",
				Priority:    15,
			},
		},
	}
}

// aiderPatterns returns patterns specific to Aider's terminal UI.
func aiderPatterns() StatusPatterns {
	return StatusPatterns{
		NeedsApproval: []StatusPattern{
			{
				Name:        "aider_permission",
				Pattern:     `\(Y\)es/\(N\)o/\(D\)on't ask again`,
				Description: "Aider permission prompt",
				Priority:    18,
			},
		},
	}
}

// opencodePatterns returns patterns specific to OpenCode's terminal UI.
func opencodePatterns() StatusPatterns {
	return StatusPatterns{
		Processing: []StatusPattern{
			{
				Name:        "opencode_thinking",
				Pattern:     `(?i)Thinking:`,
				Description: "OpenCode is thinking about a request",
				Priority:    11,
			},
			{
				Name:        "opencode_reading",
				Pattern:     `(?i)→ (Read|read)`,
				Description: "OpenCode is reading a file",
				Priority:    10,
			},
			{
				Name:        "opencode_writing",
				Pattern:     `(?i)→ (Write|write|Edit|edit)`,
				Description: "OpenCode is writing/editing files",
				Priority:    10,
			},
		},
		NeedsApproval: []StatusPattern{
			{
				Name: "opencode_permission_required",
				// OpenCode shows a bordered dialog with "Permission Required" as the title
				// and "Allow (a)" / "Allow for session (s)" / "Deny (d)" buttons.
				Pattern:     `Permission Required`,
				Description: "OpenCode permission required dialog",
				Priority:    17,
			},
			{
				Name: "opencode_allow_button",
				// The "Allow (a)" button text appears in opencode's permission dialog.
				Pattern:     `Allow \(a\)`,
				Description: "OpenCode allow button in permission dialog",
				Priority:    17,
			},
		},
		InputRequired: []StatusPattern{
			{
				// OpenCode uses ┃ prefixed numbered format in its prompt/footer area:
				// "┃  4. Icons:" or "┃  1. Option A"
				Name:        "opencode_numbered_options",
				Pattern:     `(?m)┃\s*\d+\.\s+\S`,
				Description: "OpenCode numbered selection options",
				Priority:    16,
			},
			{
				// OpenCode permission dialogs show inline button choices:
				// "┃  Allow once   Allow always   Reject"
				// "Reject   Allow always   Allow once"
				Name:        "opencode_permission",
				Pattern:     `(?i)┃?\s*allow\s+once\s+allow\s+always\s+reject|┃?\s*reject\s+allow\s+always\s+allow\s+once`,
				Description: "OpenCode permission button choices",
				Priority:    16,
			},
		},
		Active: []StatusPattern{
			{
				Name:        "opencode_esc_interrupt",
				Pattern:     `esc interrupt`,
				Description: "OpenCode active operation that can be interrupted",
				Priority:    24,
			},
		},
	}
}
