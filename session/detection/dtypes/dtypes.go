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

// BinaryDetector provides per-binary pattern sets and optional content filtering.
type BinaryDetector interface {
	Name() string
	Patterns() StatusPatterns
	FilterContent(content string) string
}
