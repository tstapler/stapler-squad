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
		},
		NeedsApproval: []dtypes.StatusPattern{
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
		},
		InputRequired: []dtypes.StatusPattern{},
		Error:         []dtypes.StatusPattern{},
		TestsFailing:  []dtypes.StatusPattern{},
		Idle:          []dtypes.StatusPattern{},
		Active:        []dtypes.StatusPattern{},
		Success:       []dtypes.StatusPattern{},
	}
}
