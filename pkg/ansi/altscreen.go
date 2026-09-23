package ansi

import "strings"

// decset1049Enter and decset1049Exit are the DEC private-mode sequences a
// terminal application sends to switch into and out of the alternate screen
// buffer (DECSET/DECRST 1049, used by full-screen TUIs like Claude Code, vim,
// less, etc. — see xterm's ctlseqs.txt).
const (
	decset1049Enter = "\x1b[?1049h"
	decset1049Exit  = "\x1b[?1049l"
	// markerLen is shared by both markers (8 bytes each) -- carryLen below
	// relies on that equality.
	markerLen = len(decset1049Enter)
	carryLen  = markerLen - 1
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
	// carry holds up to carryLen trailing bytes from the previous Observe
	// call, prepended to the next call's data before scanning -- a real PTY
	// read can split an 8-byte marker across two chunks, and carry is always
	// too short to itself contain a complete marker, so this can only add
	// detections the single-chunk scan would otherwise miss, never
	// double-count one already found.
	carry string
}

// Observe scans data for alt-screen enter/exit markers and updates the
// tracker's state accordingly. If data contains multiple markers, the last
// one observed wins (matching real terminal semantics: state after
// processing the whole chunk). Returns the resulting active state and
// whether this call changed it from what it was before.
func (t *AltScreenTracker) Observe(data string) (active, changed bool) {
	before := t.active

	combined := t.carry + data
	enterIdx := strings.LastIndex(combined, decset1049Enter)
	exitIdx := strings.LastIndex(combined, decset1049Exit)
	switch {
	case enterIdx == -1 && exitIdx == -1:
		// No markers in this chunk (or split across the carry boundary);
		// state persists unchanged.
	case enterIdx > exitIdx:
		t.active = true
	default:
		t.active = false
	}

	if len(combined) > carryLen {
		t.carry = combined[len(combined)-carryLen:]
	} else {
		t.carry = combined
	}

	return t.active, t.active != before
}

// SetActive directly sets the tracker's state, bypassing byte-scanning --
// for bootstrapping from a source that already knows the current state
// authoritatively (e.g. a live tmux query), so a subsequent Observe call
// computes `changed` relative to the correct baseline instead of the
// tracker's zero-value default. Also clears carry, so a partial marker
// buffered before this authoritative resync can't combine with the next
// chunk into a stale false match.
func (t *AltScreenTracker) SetActive(active bool) {
	t.active = active
	t.carry = ""
}
