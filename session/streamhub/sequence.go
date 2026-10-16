package streamhub

import "sync/atomic"

// HubSequenceNumber is a monotonic, per-hub ordinal stamped on every
// broadcast unit (a flushed BatchWindow frame or a Bypass control message)
// at the moment it is actually broadcast, not when it was first buffered —
// giving every Subscriber the same total order regardless of its own flush
// cadence (plan.md's Domain Glossary; Story 2.1.2's AC).
type HubSequenceNumber uint64

// sequenceCounter is the monotonic counter backing HubSequenceNumber. Its
// zero value has never issued a number; the first call to next returns 1,
// so a zero-value HubSequenceNumber is never mistaken for "the first
// broadcast unit."
type sequenceCounter struct {
	n atomic.Uint64
}

// next returns the next HubSequenceNumber. Safe for concurrent use — a
// Bypass call racing a BatchWindow flush on a different goroutine still
// each get a distinct, monotonically increasing number, and whichever call
// actually reaches next first (broadcast time) gets the lower number,
// regardless of which one was triggered "first" in wall-clock terms of
// being buffered.
func (c *sequenceCounter) next() HubSequenceNumber {
	return HubSequenceNumber(c.n.Add(1))
}
