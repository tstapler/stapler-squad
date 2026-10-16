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
