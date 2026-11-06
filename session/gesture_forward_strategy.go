package session

// pageUpBytes is the xterm PageUp escape sequence (ESC [ 5 ~). Story 1.2.1's
// live spike (session/testdata/scroll_forward_claude_before.txt,
// ..._after.txt) confirmed it scrolls Claude Code's fullscreen conversation
// view: sent against a session whose transcript overflowed the pane, it
// revealed earlier content and a "Jump to bottom" indicator, and repeated
// presses walked the view all the way back to the session's first turn.
var pageUpBytes = []byte{0x1b, 0x5b, 0x35, 0x7e}

// GestureForwardStrategy is a ScrollAdapter strategy that forwards the CLI's
// own scroll keybinding directly, then captures the pane once its redraw
// quiesces (RedrawQuiescenceCapture). Distinct from NativeDumpFallbackStrategy,
// which instead delegates capture to the terminal's own scrollback.
type GestureForwardStrategy struct {
	name       string
	capability ScrollForwardCapability
}

// NewClaudeGestureForwardStrategy constructs the GestureForwardStrategy
// confirmed for Claude Code by Story 1.2.1's spike.
func NewClaudeGestureForwardStrategy() *GestureForwardStrategy {
	return &GestureForwardStrategy{
		name: "claude-gesture-forward",
		capability: ScrollForwardCapability{
			KeySequencesUp:         [][]byte{pageUpBytes},
			VerifiedAgainstVersion: "2.1.270",
			KnownFailureModes: []string{
				"recovers the full session transcript back to the first turn " +
					"(spike measured 20 PageUp presses across a ~4-turn session " +
					"before hitting AtTop); depth beyond that is untested",
			},
		},
	}
}

func (s *GestureForwardStrategy) Name() string {
	return s.name
}

// CanHandle always returns false: GestureForwardStrategy is selected by its
// owning adapter (e.g. ClaudeScrollAdapter.CanHandle), never resolved
// directly via resolveScrollAdapter.
func (s *GestureForwardStrategy) CanHandle(program string) bool {
	return false
}

func (s *GestureForwardStrategy) Capability() ScrollForwardCapability {
	return s.capability
}

// KeySequences only implements ScrollUp -- Phase 1 is scroll-up-only (no
// caller passes ScrollDown; see session/instance_scroll_forward.go). Panics
// rather than silently returning the PageUp sequence for a direction it was
// never verified against, since ScrollForwardCapability has no
// KeySequencesDown field to return instead.
func (s *GestureForwardStrategy) KeySequences(dir ScrollDirection) [][]byte {
	if dir != ScrollUp {
		panic("GestureForwardStrategy.KeySequences: ScrollDown not implemented")
	}
	return s.capability.KeySequencesUp
}

func (s *GestureForwardStrategy) CaptureVia() ScrollCaptureMode {
	return RedrawQuiescenceCapture
}
