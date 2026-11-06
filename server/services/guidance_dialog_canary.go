package services

import (
	"sync"
	"time"
)

// guidance_dialog_canary.go is the layout-drift canary (pre-mortem #5, Task 3.3h).
// Every answer-time DialogMatch outcome is recorded; when more than half of at
// least canaryMinAttempts attempts in the last hour mismatch, the canary trips:
// it raises one operator notification and the descriptors of hook-mirrored
// questions degrade to read_only until the share falls back to 50% or less.

const (
	canaryWindow      = time.Hour
	canaryMinAttempts = 5

	claudeVersionUnknown = "unknown"
)

// DialogMatchResult is the label of question_dialog_match_total{result}.
type DialogMatchResult string

// The two DialogMatch outcomes.
const (
	DialogMatched  DialogMatchResult = "match"
	DialogMismatch DialogMatchResult = "mismatch"
)

// DialogMatchResultOf maps a DialogMatch boolean onto its metric label.
func DialogMatchResultOf(matched bool) DialogMatchResult {
	if matched {
		return DialogMatched
	}
	return DialogMismatch
}

type canaryAttempt struct {
	at       time.Time
	mismatch bool
}

// DialogMatchCanary tracks the DialogMatch mismatch share. The zero value is not
// usable; use NewDialogMatchCanary.
type DialogMatchCanary struct {
	now func() time.Time
	// count mirrors question_dialog_match_total{result,claude_version}; may be nil.
	count func(result DialogMatchResult, claudeVersion string)
	// notify raises the operator notification; may be nil.
	notify func()
	// version names the Claude Code version for the metric label; may be nil.
	version func() string

	mu       sync.Mutex
	attempts []canaryAttempt
	tripped  bool
}

// NewDialogMatchCanary builds a canary; now may be nil.
func NewDialogMatchCanary(now func() time.Time, count func(result DialogMatchResult, claudeVersion string), notify func()) *DialogMatchCanary {
	if now == nil {
		now = time.Now
	}
	return &DialogMatchCanary{now: now, count: count, notify: notify}
}

// SetVersionSource names where the metric's claude_version label comes from.
func (c *DialogMatchCanary) SetVersionSource(fn func() string) { c.version = fn }

// Record counts one DialogMatch outcome and re-evaluates the share. A nil canary ignores it.
func (c *DialogMatchCanary) Record(result DialogMatchResult) {
	if c == nil {
		return
	}
	if c.count != nil {
		v := claudeVersionUnknown
		if c.version != nil {
			if got := c.version(); got != "" {
				v = got
			}
		}
		c.count(result, v)
	}
	if c.evaluate(result) && c.notify != nil {
		c.notify()
	}
}

// evaluate adds the attempt and reports whether this call tripped the canary.
func (c *DialogMatchCanary) evaluate(result DialogMatchResult) (justTripped bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	c.attempts = append(c.attempts, canaryAttempt{at: now, mismatch: result == DialogMismatch})
	cutoff := now.Add(-canaryWindow)
	keep := c.attempts[:0]
	mismatches := 0
	for _, a := range c.attempts {
		if a.at.After(cutoff) {
			keep = append(keep, a)
			if a.mismatch {
				mismatches++
			}
		}
	}
	c.attempts = keep
	over := len(keep) >= canaryMinAttempts && mismatches*2 > len(keep)
	switch {
	case over && !c.tripped:
		c.tripped = true
		return true
	case !over && c.tripped && mismatches*2 <= len(keep):
		c.tripped = false
	}
	return false
}

// Tripped reports whether hook-mirrored answers are degraded to read_only. A nil
// canary is never tripped.
func (c *DialogMatchCanary) Tripped() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tripped
}
