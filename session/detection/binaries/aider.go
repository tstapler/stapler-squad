package binaries

import "github.com/tstapler/stapler-squad/session/detection/dtypes"

// AiderDetector implements dtypes.BinaryDetector for the Aider CLI.
type AiderDetector struct{}

// NewAiderDetector returns a new AiderDetector.
func NewAiderDetector() *AiderDetector { return &AiderDetector{} }

// Name returns "aider".
func (d *AiderDetector) Name() string { return "aider" }

// FilterContent returns content unchanged.
func (d *AiderDetector) FilterContent(content string) string { return content }

// Patterns returns Aider-specific status patterns.
func (d *AiderDetector) Patterns() dtypes.StatusPatterns {
	return dtypes.StatusPatterns{
		Ready:         []dtypes.StatusPattern{},
		Processing:    []dtypes.StatusPattern{},
		InputRequired: []dtypes.StatusPattern{},
		Error:         []dtypes.StatusPattern{},
		TestsFailing:  []dtypes.StatusPattern{},
		Idle:          []dtypes.StatusPattern{},
		Active:        []dtypes.StatusPattern{},
		Success:       []dtypes.StatusPattern{},
		NeedsApproval: []dtypes.StatusPattern{
			{
				Name:        "aider_permission",
				Pattern:     `\(Y\)es/\(N\)o/\(D\)on't ask again`,
				Description: "Aider permission prompt",
				Priority:    18,
			},
		},
	}
}
