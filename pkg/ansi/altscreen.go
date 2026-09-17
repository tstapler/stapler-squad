package ansi

import "strings"

// decset1049Enter and decset1049Exit are the DEC private-mode sequences a
// terminal application sends to switch into and out of the alternate screen
// buffer (DECSET/DECRST 1049, used by full-screen TUIs like Claude Code, vim,
// less, etc. — see xterm's ctlseqs.txt).
const (
	decset1049Enter = "\x1b[?1049h"
	decset1049Exit  = "\x1b[?1049l"
)

// AltScreenTracker is a small stateful scanner that tracks whether a PTY
// output stream currently has its consumer in the alternate screen buffer.
// It mirrors this package's csi.go: a byte-string scanner, no regexp needed
// since the two markers are fixed literals rather than a variable-parameter
// CSI class.
//
// Not safe for concurrent use — callers must serialize Observe calls (one
// PTY output stream has one logical writer).
type AltScreenTracker struct {
	active bool
}

// Observe scans data for alt-screen enter/exit markers and updates the
// tracker's state accordingly. If data contains multiple markers, the last
// one observed wins (matching real terminal semantics: state after
// processing the whole chunk). Returns the resulting active state and
// whether this call changed it from what it was before.
func (t *AltScreenTracker) Observe(data string) (active, changed bool) {
	before := t.active

	enterIdx := strings.LastIndex(data, decset1049Enter)
	exitIdx := strings.LastIndex(data, decset1049Exit)
	switch {
	case enterIdx == -1 && exitIdx == -1:
		// No markers in this chunk; state persists unchanged.
	case enterIdx > exitIdx:
		t.active = true
	default:
		t.active = false
	}

	return t.active, t.active != before
}

// SetActive directly sets the tracker's state, bypassing byte-scanning --
// for bootstrapping from a source that already knows the current state
// authoritatively (e.g. a live tmux query), so a subsequent Observe call
// computes `changed` relative to the correct baseline instead of the
// tracker's zero-value default.
func (t *AltScreenTracker) SetActive(active bool) {
	t.active = active
}
