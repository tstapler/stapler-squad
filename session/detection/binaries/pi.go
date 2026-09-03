package binaries

import "github.com/tstapler/stapler-squad/session/detection/dtypes"

// PiDetector implements dtypes.BinaryDetector for the pi coding-agent CLI.
// pi's live status is tracked via a separate `pi --mode json` side-channel
// subprocess (session/instance_pi_status.go), not by pane scraping, so
// Patterns() is empty -- mirrors AiderDetector's shape before its own
// approval pattern was added.
type PiDetector struct{}

// NewPiDetector returns a new PiDetector.
func NewPiDetector() *PiDetector { return &PiDetector{} }

// Name returns "pi".
func (d *PiDetector) Name() string { return "pi" }

// FilterContent returns content unchanged.
func (d *PiDetector) FilterContent(content string) string { return content }

// Patterns returns an empty pattern set -- see the type doc comment.
func (d *PiDetector) Patterns() dtypes.StatusPatterns {
	return dtypes.StatusPatterns{
		Ready:         []dtypes.StatusPattern{},
		Processing:    []dtypes.StatusPattern{},
		NeedsApproval: []dtypes.StatusPattern{},
		InputRequired: []dtypes.StatusPattern{},
		Error:         []dtypes.StatusPattern{},
		TestsFailing:  []dtypes.StatusPattern{},
		Idle:          []dtypes.StatusPattern{},
		Active:        []dtypes.StatusPattern{},
		Success:       []dtypes.StatusPattern{},
	}
}
