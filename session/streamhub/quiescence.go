package streamhub

import (
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

const (
	// defaultQuiescenceTimeout mirrors the 500ms deadline
	// server/services/connectrpc_websocket.go's waitForQuiescence call uses
	// for its initial post-connect resize wait (Task 1.3.2b/f).
	defaultQuiescenceTimeout = 500 * time.Millisecond

	// defaultQuiescenceQuietPeriod mirrors the same call site's 200ms quiet
	// window: no update for this long after a resize means the TUI's redraw
	// is done.
	defaultQuiescenceQuietPeriod = 200 * time.Millisecond
)

// waitForQuiescence blocks until no update arrives on updates for quietFor,
// or until timeout elapses first — whichever comes first. Adapted from
// server/services/connectrpc_websocket.go's function of the same name
// (Task 1.3.2b), generalized to a hub-owned, session-agnostic form: it takes
// the raw update channel directly (rather than a per-connection one) and
// logs its own hub-scoped QuiescenceTimedOut WARN (Task 1.3.2f) instead of
// leaving that to each caller.
func waitForQuiescence(clock Clock, updates <-chan []byte, timeout, quietFor time.Duration, sessionName string) {
	deadline := make(chan struct{})
	deadlineTimer := clock.AfterFunc(timeout, func() { close(deadline) })
	defer deadlineTimer.Stop()

	// mu orders re-arming against an already-fired quiet callback: a stale
	// callback sees a newer generation and stays silent, and arm drains any
	// signal it left behind, so the new timer's signal can never be dropped.
	var mu sync.Mutex
	quiet := make(chan struct{}, 1)
	gen := 0
	arm := func() Timer {
		mu.Lock()
		defer mu.Unlock()
		gen++
		g := gen
		select {
		case <-quiet:
		default:
		}
		return clock.AfterFunc(quietFor, func() {
			mu.Lock()
			defer mu.Unlock()
			if g == gen {
				quiet <- struct{}{}
			}
		})
	}
	quietTimer := arm()
	defer func() { quietTimer.Stop() }()
	start := clock.Now()
	for {
		select {
		case _, ok := <-updates:
			if !ok {
				return
			}
			quietTimer.Stop()
			quietTimer = arm()
		case <-quiet:
			return
		case <-deadline:
			if elapsed := clock.Now().Sub(start); elapsed >= timeout-5*time.Millisecond {
				log.Warn("streamhub quiescence timed out; session may be stalled",
					"tmux_session", sessionName, "elapsed", elapsed.Round(time.Millisecond))
			}
			return
		}
	}
}
