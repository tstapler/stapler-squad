package session

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
	StatusError
	StatusIdle   // Waiting for user input (INSERT mode, command prompt, etc.)
	StatusActive // Actively executing commands (shows "esc to interrupt")
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
	Ready          []StatusPattern `yaml:"ready"`
	Processing     []StatusPattern `yaml:"processing"`
	NeedsApproval  []StatusPattern `yaml:"needs_approval"`
	Error          []StatusPattern `yaml:"error"`
	Idle           []StatusPattern `yaml:"idle"`   // Waiting for user input
	Active         []StatusPattern `yaml:"active"` // Actively executing commands
}

// StatusDetector analyzes PTY output to determine the current status of a Claude instance.
type StatusDetector struct {
	patterns StatusPatterns
	// Cache compiled regexes for performance
	readyRegexes         []*regexp.Regexp
	processingRegexes    []*regexp.Regexp
	needsApprovalRegexes []*regexp.Regexp
	errorRegexes         []*regexp.Regexp
	idleRegexes          []*regexp.Regexp
	activeRegexes        []*regexp.Regexp
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

	// Compile error patterns
	sd.errorRegexes = make([]*regexp.Regexp, len(sd.patterns.Error))
	for i, pattern := range sd.patterns.Error {
		sd.errorRegexes[i], err = regexp.Compile(pattern.Pattern)
		if err != nil {
			return fmt.Errorf("failed to compile error pattern '%s': %w", pattern.Name, err)
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

	return nil
}

// Detect analyzes the provided PTY output and returns the detected status.
// Patterns are checked in priority order: Error > NeedsApproval > Active > Processing > Idle > Ready.
// Returns StatusUnknown if no patterns match.
func (sd *StatusDetector) Detect(output []byte) DetectedStatus {
	text := string(output)

	// Check error patterns first (highest priority)
	for _, regex := range sd.errorRegexes {
		if regex.MatchString(text) {
			return StatusError
		}
	}

	// Check needs approval patterns
	for _, regex := range sd.needsApprovalRegexes {
		if regex.MatchString(text) {
			return StatusNeedsApproval
		}
	}

	// Check active patterns (e.g., "esc to interrupt")
	for _, regex := range sd.activeRegexes {
		if regex.MatchString(text) {
			return StatusActive
		}
	}

	// Check processing patterns
	for _, regex := range sd.processingRegexes {
		if regex.MatchString(text) {
			return StatusProcessing
		}
	}

	// Check idle patterns (e.g., "— INSERT —")
	for _, regex := range sd.idleRegexes {
		if regex.MatchString(text) {
			return StatusIdle
		}
	}

	// Check ready patterns
	for _, regex := range sd.readyRegexes {
		if regex.MatchString(text) {
			return StatusReady
		}
	}

	return StatusUnknown
}

// DetectWithContext returns the detected status along with the matching pattern context.
// This is useful for debugging and understanding why a particular status was detected.
func (sd *StatusDetector) DetectWithContext(output []byte) (DetectedStatus, string) {
	text := string(output)

	// Check error patterns first (highest priority)
	for i, regex := range sd.errorRegexes {
		if match := regex.FindString(text); match != "" {
			return StatusError, fmt.Sprintf("Matched error pattern '%s': %s", sd.patterns.Error[i].Name, match)
		}
	}

	// Check needs approval patterns
	for i, regex := range sd.needsApprovalRegexes {
		if match := regex.FindString(text); match != "" {
			return StatusNeedsApproval, fmt.Sprintf("Matched approval pattern '%s': %s", sd.patterns.NeedsApproval[i].Name, match)
		}
	}

	// Check active patterns
	for i, regex := range sd.activeRegexes {
		if match := regex.FindString(text); match != "" {
			return StatusActive, fmt.Sprintf("Matched active pattern '%s': %s", sd.patterns.Active[i].Name, match)
		}
	}

	// Check processing patterns
	for i, regex := range sd.processingRegexes {
		if match := regex.FindString(text); match != "" {
			return StatusProcessing, fmt.Sprintf("Matched processing pattern '%s': %s", sd.patterns.Processing[i].Name, match)
		}
	}

	// Check idle patterns
	for i, regex := range sd.idleRegexes {
		if match := regex.FindString(text); match != "" {
			return StatusIdle, fmt.Sprintf("Matched idle pattern '%s': %s", sd.patterns.Idle[i].Name, match)
		}
	}

	// Check ready patterns
	for i, regex := range sd.readyRegexes {
		if match := regex.FindString(text); match != "" {
			return StatusReady, fmt.Sprintf("Matched ready pattern '%s': %s", sd.patterns.Ready[i].Name, match)
		}
	}

	return StatusUnknown, "No patterns matched"
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
		},
		Processing: []StatusPattern{
			{
				Name:        "thinking",
				Pattern:     `(?i)(thinking|processing|analyzing|working)`,
				Description: "Claude is processing a command",
				Priority:    10,
			},
			{
				Name:        "tool_use",
				Pattern:     `(?i)(reading|writing|editing|executing|running)`,
				Description: "Claude is using tools",
				Priority:    9,
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
		},
		Error: []StatusPattern{
			{
				Name:        "error_message",
				Pattern:     `(?i)(error|failed|exception|fatal)`,
				Description: "Generic error indicators",
				Priority:    30,
			},
			{
				Name:        "connection_error",
				Pattern:     `(?i)(connection refused|timeout|network error)`,
				Description: "Network and connection errors",
				Priority:    29,
			},
		},
		Idle: []StatusPattern{
			{
				Name:        "insert_mode",
				Pattern:     `—\s*INSERT\s*—`,
				Description: "Claude Code in INSERT mode, waiting for input",
				Priority:    15,
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
		},
		Active: []StatusPattern{
			{
				Name:        "esc_to_interrupt",
				Pattern:     `\(esc to interrupt\)`,
				Description: "Active operation that can be interrupted",
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
				Pattern:     `[✓✔⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏].*(?:ing|Processing|Working|Executing|Verifying|Testing|Building)`,
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
	case StatusError:
		return "Error"
	case StatusIdle:
		return "Idle"
	case StatusActive:
		return "Active"
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
	case StatusError:
		patterns = sd.patterns.Error
	case StatusIdle:
		patterns = sd.patterns.Idle
	case StatusActive:
		patterns = sd.patterns.Active
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

// DetectFromLines analyzes multiple lines of output and returns the most relevant status.
// This is useful for analyzing scrollback history where multiple status indicators may be present.
// The most recent (last) matching pattern takes precedence.
func (sd *StatusDetector) DetectFromLines(lines []string) DetectedStatus {
	// Process lines in reverse order (most recent first)
	for i := len(lines) - 1; i >= 0; i-- {
		status := sd.DetectFromString(lines[i])
		if status != StatusUnknown {
			return status
		}
	}
	return StatusUnknown
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
