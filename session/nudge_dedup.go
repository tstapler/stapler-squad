package session

import (
	"strings"
	"time"
)

// nudgeCooldown bounds how long an exact-repeat nudge stays suppressed. It is a
// var (not const), mirroring idleSettleWindow, so tests can shrink it. Kept
// separate from idleSettleWindow: conflating idle-detection timing with
// content-repeat backoff would slow down every turn transition, not just the
// duplicate-nudge minority case.
var nudgeCooldown = 3 * time.Minute

// normalizeNudgeText lowercases and collapses whitespace so near-identical
// nudges (differing only in case or incidental whitespace) compare equal.
func normalizeNudgeText(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// normalizePaneSnapshot collapses whitespace in a pane tail so that
// incidental spacing/redraw differences don't look like new output. Case is
// preserved (unlike normalizeNudgeText): pane content isn't a human
// instruction being fuzzily matched, and a case-only change (e.g. a log
// level flipping to uppercase) can be exactly the "new meaningful output"
// that should re-arm the guard.
func normalizePaneSnapshot(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// lastNudge records the most recent nudge successfully delivered via SendKeys
// (both the message and Enter writes succeeded). Bundled into one value
// (rather than several same-typed string/time.Time locals) so run() carries a
// single piece of state and isDuplicateNudge takes one comparison target
// instead of easily-swappable primitive parameters.
type lastNudge struct {
	text string
	at   time.Time
	pane string // normalized pane tail at the moment this nudge was delivered
}

// isDuplicateNudge reports whether candidate is an exact repeat (after
// normalization) of n's text, sent within nudgeCooldown of now, AND the pane
// has produced no new meaningful output since that nudge was delivered. A
// zero-value n.at (no nudge sent yet) or an empty n.text always returns
// false. currentPane differing from the snapshot recorded at delivery time
// re-arms the guard immediately, bypassing the cooldown — new pane activity
// means the agent may legitimately need the same instruction repeated.
func isDuplicateNudge(candidate string, n lastNudge, now time.Time, currentPane string) bool {
	if n.text == "" || n.at.IsZero() {
		return false
	}
	if normalizePaneSnapshot(currentPane) != n.pane {
		return false
	}
	if now.Sub(n.at) > nudgeCooldown {
		return false
	}
	return normalizeNudgeText(candidate) == normalizeNudgeText(n.text)
}

// nextLastNudge computes what lastSentNudge should become after an attempted
// delivery of nextMsg. delivered must be true only when BOTH SendKeys writes
// (content, then the submit keystroke) succeeded — a partially- or
// un-delivered nudge must not be recorded, or a later isDuplicateNudge check
// could wrongly suppress a genuinely new message that never actually reached
// the session. pane is the pane tail captured at delivery time, used later to
// detect new output and re-arm the guard. Extracted as a pure function so
// this invariant is directly unit-testable without a tmux-backed Instance.
func nextLastNudge(prev lastNudge, nextMsg string, delivered bool, pane string) lastNudge {
	if !delivered {
		return prev
	}
	return lastNudge{text: nextMsg, at: time.Now(), pane: normalizePaneSnapshot(pane)}
}
