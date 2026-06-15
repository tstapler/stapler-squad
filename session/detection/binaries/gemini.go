package binaries

import "github.com/tstapler/stapler-squad/session/detection/dtypes"

// GeminiDetector implements dtypes.BinaryDetector for the Gemini CLI.
type GeminiDetector struct{}

// NewGeminiDetector returns a new GeminiDetector.
func NewGeminiDetector() *GeminiDetector { return &GeminiDetector{} }

// Name returns "gemini".
func (d *GeminiDetector) Name() string { return "gemini" }

// FilterContent returns content unchanged.
func (d *GeminiDetector) FilterContent(content string) string { return content }

// Patterns returns Gemini CLI-specific status patterns.
func (d *GeminiDetector) Patterns() dtypes.StatusPatterns {
	return dtypes.StatusPatterns{
		Ready: []dtypes.StatusPattern{
			{
				Name:        "gemini_ready",
				Pattern:     `(?:◇|✓).*(?:Ready|ready)`,
				Description: "Gemini CLI ready status (◇ Ready)",
				Priority:    5,
			},
		},
		Processing: []dtypes.StatusPattern{
			{
				Name:        "gemini_working",
				Pattern:     `(?:✦|⏲).*(?:Working|working)`,
				Description: "Gemini CLI working status (✦ Working)",
				Priority:    11,
			},
		},
		NeedsApproval: []dtypes.StatusPattern{
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
		},
		InputRequired: []dtypes.StatusPattern{},
		Error:         []dtypes.StatusPattern{},
		TestsFailing:  []dtypes.StatusPattern{},
		Idle:          []dtypes.StatusPattern{},
		Active:        []dtypes.StatusPattern{},
		Success:       []dtypes.StatusPattern{},
	}
}
