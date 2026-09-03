package binaries

import "github.com/tstapler/stapler-squad/session/detection/dtypes"

// PiDetector implements dtypes.BinaryDetector for the pi coding-agent CLI
// (@earendil-works/pi-coding-agent). Unlike claude/aider/gemini, pi's live
// status is tracked via a separate `pi --mode json` side-channel subprocess
// (session/instance_pi_status.go), not by scraping this detector's patterns
// against the interactive tmux pane -- so Patterns() is empty for now,
// mirroring AiderDetector's shape before its own approval pattern was added.
// Registering pi here still matters for FilterContent and any future
// pattern-based detection this pane's content ends up needing.
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
