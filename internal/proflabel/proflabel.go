// Package proflabel tags a subsystem's goroutines with a pprof label, so a CPU profile can be
// grouped by subsystem (`go tool pprof -tagfocus=subsystem=review_queue_poller`, or
// `-tags` to list them) instead of by whichever function happened to burn the cycles.
package proflabel

import (
	"context"
	"runtime/pprof"
)

// Do runs fn with the pprof label subsystem=name set on the calling goroutine. Goroutines fn
// starts inherit the label. Call it as the first thing a subsystem goroutine does.
func Do(name string, fn func()) {
	pprof.Do(context.Background(), pprof.Labels("subsystem", name), func(context.Context) { fn() })
}
