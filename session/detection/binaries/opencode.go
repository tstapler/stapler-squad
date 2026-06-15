package binaries

import "github.com/tstapler/stapler-squad/session/detection/dtypes"

// OpencodeDetector implements dtypes.BinaryDetector for the OpenCode CLI.
type OpencodeDetector struct{}

// NewOpencodeDetector returns a new OpencodeDetector.
func NewOpencodeDetector() *OpencodeDetector { return &OpencodeDetector{} }

// Name returns "opencode".
func (d *OpencodeDetector) Name() string { return "opencode" }

// FilterContent returns content unchanged.
func (d *OpencodeDetector) FilterContent(content string) string { return content }

// Patterns returns OpenCode-specific status patterns.
func (d *OpencodeDetector) Patterns() dtypes.StatusPatterns {
	return dtypes.StatusPatterns{
		Ready: []dtypes.StatusPattern{},
		Processing: []dtypes.StatusPattern{
			{
				Name:        "opencode_arrow_action",
				Pattern:     `→\s*(Read|Write|Edit|Create|Delete)\b`,
				Description: "OpenCode tool action arrow (→ Read, → Write, etc.)",
				Priority:    10,
			},
		},
		NeedsApproval: []dtypes.StatusPattern{
			{
				Name:        "opencode_permission",
				Pattern:     `\[\s*Allow\s*\([aA]\)\s*\]`,
				Description: "OpenCode permission prompt with [ Allow (a) ] buttons",
				Priority:    18,
			},
		},
		InputRequired: []dtypes.StatusPattern{
			{
				Name:        "opencode_bar_prefixed_options",
				Pattern:     `┃\s*\d+\.\s+\w`,
				Description: "OpenCode bar-prefixed numbered option selector",
				Priority:    17,
			},
			{
				Name:        "opencode_permission_buttons",
				Pattern:     `(?i)Allow\s+once.*Allow\s+always|Allow\s+always.*Allow\s+once`,
				Description: "OpenCode permission dialog buttons",
				Priority:    18,
			},
		},
		Error:        []dtypes.StatusPattern{},
		TestsFailing: []dtypes.StatusPattern{},
		Idle:         []dtypes.StatusPattern{},
		Active:       []dtypes.StatusPattern{},
		Success:      []dtypes.StatusPattern{},
	}
}
