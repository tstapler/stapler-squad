package streamhub

import (
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
func waitForQuiescence(updates <-chan []byte, timeout, quietFor time.Duration, sessionName string) {
	deadline := time.After(timeout)
	quiet := time.NewTimer(quietFor)
	defer quiet.Stop()
	start := time.Now()
	for {
		select {
		case _, ok := <-updates:
			if !ok {
				return
			}
			if !quiet.Stop() {
				select {
				case <-quiet.C:
				default:
				}
			}
			quiet.Reset(quietFor)
		case <-quiet.C:
			return
		case <-deadline:
			if elapsed := time.Since(start); elapsed >= timeout-5*time.Millisecond {
				log.Warn("streamhub quiescence timed out; session may be stalled",
					"tmux_session", sessionName, "elapsed", elapsed.Round(time.Millisecond))
			}
			return
		}
	}
}
