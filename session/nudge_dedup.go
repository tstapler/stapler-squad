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

// lastNudge records the most recent nudge successfully delivered via SendKeys
// (both the message and Enter writes succeeded). Bundled into one value
// (rather than two same-typed string/time.Time locals) so run() carries a
// single piece of state and isDuplicateNudge takes one comparison target
// instead of two easily-swappable primitive parameters.
type lastNudge struct {
	text string
	at   time.Time
}

// isDuplicateNudge reports whether candidate is an exact repeat (after
// normalization) of n's text, sent within nudgeCooldown of now. A zero-value
// n.at (no nudge sent yet) or an empty n.text always returns false.
func isDuplicateNudge(candidate string, n lastNudge, now time.Time) bool {
	if n.text == "" || n.at.IsZero() {
		return false
	}
	if now.Sub(n.at) > nudgeCooldown {
		return false
	}
	return normalizeNudgeText(candidate) == normalizeNudgeText(n.text)
}
