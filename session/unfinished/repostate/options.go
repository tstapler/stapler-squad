package repostate

import "time"

// Options injects the clock and pressure signal into repostate types, so TTL
// and pressure behaviour are deterministic in tests with no package-level seam.
// The zero value is usable: real clock, never under pressure, no graph bound.
type Options struct {
	// Clock returns the current time; nil means time.Now.
	Clock func() time.Time
	// PressureFunc reports severe memory pressure; while true, caches skip
	// storing new entries. nil means never.
	PressureFunc func() bool
	// MaxGraphCommits bounds the commit graph (consumed by the graph, Story 2.1.x).
	MaxGraphCommits int
	// ClaimWait bounds how long a loser of a row's fill claim waits for the
	// holder to publish before decoding the commit itself without publishing;
	// zero means defaultClaimWait.
	ClaimWait time.Duration
}

func (o Options) now() time.Time {
	if o.Clock == nil {
		return time.Now()
	}
	return o.Clock()
}

func (o Options) underPressure() bool {
	return o.PressureFunc != nil && o.PressureFunc()
}
