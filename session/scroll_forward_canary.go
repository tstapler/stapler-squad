package session

import (
	"bytes"
	"regexp"
	"sync"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
)

// scrollForwardFalseRedrawNormalizeRegex strips known false-redraw
// signatures -- content that changes on every pane redraw regardless of
// whether a scroll actually happened -- confirmed by the Story 1.2.1 spike's
// own golden fixture (session/testdata/scroll_forward_claude_before.txt's
// footer, "Cogitated for 29s ... 11:49 AM"): an elapsed-time/status line, a
// clock timestamp, and common spinner glyphs.
var scrollForwardFalseRedrawNormalizeRegex = regexp.MustCompile(`Cogitated for \d+s|\d{1,2}:\d{2}\s*[AP]M|[✻✽✢⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏·]`)

// scrollForwardCanaryLogged guards scrollForwardKeybindingCanary so it fires
// at most once per session (keyed by Instance.Title), mirroring
// compactingCanaryLogged's once-per-detector guard
// (session/detection/detector.go:126-128) -- this is called on every
// DELIVERED ForwardScroll outcome, so an unguarded version would log
// continuously for as long as a session keeps hitting the same false-redraw
// signature.
var scrollForwardCanaryLogged sync.Map

// scrollForwardKeybindingCanary is a heuristic drift detector (Story 1.5.2),
// not a strict proof: it fires only for a DELIVERED outcome (never AT_TOP or
// BLOCKED -- those don't represent an ambiguous "redraw happened" case) whose
// before/after capture, once known false-redraw signatures are stripped,
// becomes byte-identical -- i.e. the only thing that changed was a spinner or
// elapsed-time indicator, not real transcript content. A genuine scroll's
// capture differs in ways this normalization never collapses (new/shifted
// transcript lines), so it does not fire for a real scroll -- see
// scroll_forward_canary_test.go's golden-fixture regression guard using the
// Story 1.2.1 spike's own before/after capture.
//
// Returns whether it fired, so callers (and tests) can assert on the outcome
// without re-parsing the log.
func scrollForwardKeybindingCanary(sessionKey string, outcome sessionv1.ScrollForwardOutcome, before, after []byte) bool {
	if outcome != sessionv1.ScrollForwardOutcome_DELIVERED {
		return false
	}
	normBefore := scrollForwardFalseRedrawNormalizeRegex.ReplaceAll(before, nil)
	normAfter := scrollForwardFalseRedrawNormalizeRegex.ReplaceAll(after, nil)
	if !bytes.Equal(normBefore, normAfter) {
		return false
	}

	if _, already := scrollForwardCanaryLogged.LoadOrStore(sessionKey, struct{}{}); already {
		return false
	}
	log.Warn("scroll_forward: forwarded keystroke's captured redraw changed but only in a known false-redraw signature (spinner/timestamp) -- possible keybinding drift",
		"session", sessionKey,
		"before_len", len(before),
		"after_len", len(after))
	return true
}
