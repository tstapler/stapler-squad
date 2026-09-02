// Package dtypes contains shared types for the detection package and its sub-packages.
// It is intentionally minimal to avoid import cycles: both session/detection and
// session/detection/binaries import from here; neither imports the other.
package dtypes

// StatusPattern represents a regex pattern for detecting a specific status.
type StatusPattern struct {
	Name        string `yaml:"name"`
	Pattern     string `yaml:"pattern"`
	Description string `yaml:"description"`
	Priority    int    `yaml:"priority"` // Higher priority patterns checked first
}

// StatusPatterns contains all patterns for status detection.
type StatusPatterns struct {
	Ready           []StatusPattern `yaml:"ready"`
	Processing      []StatusPattern `yaml:"processing"`
	NeedsApproval   []StatusPattern `yaml:"needs_approval"`
	InputRequired   []StatusPattern `yaml:"input_required"` // Explicit input prompts
	Error           []StatusPattern `yaml:"error"`
	TestsFailing    []StatusPattern `yaml:"tests_failing"`     // Tests are failing
	Idle            []StatusPattern `yaml:"idle"`              // Waiting for user input
	Active          []StatusPattern `yaml:"active"`            // Actively executing commands
	Success         []StatusPattern `yaml:"success"`           // Task completed successfully
	WaitingForAgent []StatusPattern `yaml:"waiting_for_agent"` // Waiting for background agent(s)
	Compacting      []StatusPattern `yaml:"compacting"`        // Actively compacting/summarizing conversation history
}

// BinaryDetector provides per-binary pattern sets and optional content filtering.
type BinaryDetector interface {
	Name() string
	Patterns() StatusPatterns
	FilterContent(content string) string
}

// OSCStatus represents the classification of a Claude Code OSC window-title
// payload (see session/detection/binaries.ClassifyOSCTitle), independent of
// and prior to mapping into a DetectedStatus/IdleState by the caller.
type OSCStatus int

const (
	OSCStatusNone      OSCStatus = iota // no recognizable spinner/idle marker
	OSCStatusExecuting                  // Braille spinner (U+2800-U+28FF) present
	OSCStatusIdle                       // ✳ (U+2733) present
)
