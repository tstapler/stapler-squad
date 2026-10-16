package session

// ScrollDirection identifies which way a forwarded scroll gesture moves the
// underlying CLI's transcript.
type ScrollDirection int

const (
	ScrollUp ScrollDirection = iota
	ScrollDown
)

// ScrollCaptureMode tells Instance.ForwardScroll (Story 1.3.1) which capture
// path to run after sending a ScrollAdapter's KeySequences.
type ScrollCaptureMode int

const (
	// RedrawQuiescenceCapture waits for the pane's redraw to quiesce, then
	// captures the pane content directly.
	RedrawQuiescenceCapture ScrollCaptureMode = iota
	// NativeScrollbackDelegateCapture delegates to the terminal's own
	// scrollback buffer instead of capturing pane content.
	NativeScrollbackDelegateCapture
)

// ScrollAdapter maps a program to its scroll-forwarding capability: the key
// sequences that scroll its own transcript, and how to capture the result
// afterward.
//
// KeySequences returns an *ordered* slice -- one element for a single-write
// strategy, two for a two-write one -- so ForwardScroll always sends every
// element in order rather than assuming exactly one write. This shape
// replaced an earlier single-write ScrollGesture(dir) ([]byte, bool) during
// architecture review: that shape could not represent a two-write,
// different-capture-mechanism strategy, breaking Liskov substitutability
// between Epic 1.2's strategies.
type ScrollAdapter interface {
	Name() string
	CanHandle(program string) bool
	Capability() ScrollForwardCapability
	KeySequences(dir ScrollDirection) [][]byte
	CaptureVia() ScrollCaptureMode
}

// resolveScrollAdapter returns the ScrollAdapter that claims to handle
// program, or nil if none does. This is the single resolution point for
// scroll-forwarding adapter coverage, mirroring resolveHistoryAdapter
// (session/history_adapter.go) so adding or changing adapter coverage only
// needs one edit.
func resolveScrollAdapter(program string) ScrollAdapter {
	if claude := NewClaudeScrollAdapter(); claude.CanHandle(program) {
		return claude
	}
	return nil
}
