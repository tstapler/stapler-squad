package termlat

import (
	"sort"
)

// Resync reasons. Client reasons come from the browser (cm.term_resync); server reasons are
// counted where a CurrentPaneRequest arrives. "skipped_*" are resyncs the client chose not to send.
const (
	ResyncVisibility         = "visibility"          // client sent one on tab/window focus or visibility
	ResyncResize             = "resize"              // client sent one after a settled resize
	ResyncSkippedFresh       = "skipped_fresh"       // focus/visibility, but size unchanged and the last sync is recent
	ResyncSkippedUnavailable = "skipped_unavailable" // focus/visibility, but the terminal could not be measured
	ResyncStaleShown         = "stale_shown"         // a pane that was hidden at a focus/visibility event resynced when shown
	ResyncDeferredHidden     = "deferred_hidden"     // focus/visibility reached a hidden pane: marked stale, no resync
	ResyncSkippedOutput      = "skipped_output"      // focus/visibility, but output was flowing on a connected stream
	ResyncWatchdogRetry      = "watchdog_retry"      // a stalled resync was re-sent once instead of dropping the stream
	ResyncWatchdogDisconnect = "watchdog_disconnect" // a resync stalled twice and the stream was dropped and re-attached
	ResyncServerStaleDims    = "server_stale_dims"   // server received a request the client flagged as same-size
	ResyncServerRequest      = "server_request"      // server received any other CurrentPaneRequest
)

// NormalizeResyncReason returns r if it is a known reason, else "unknown".
func NormalizeResyncReason(r string) string {
	switch r {
	case ResyncVisibility, ResyncResize, ResyncStaleShown, ResyncDeferredHidden, ResyncSkippedOutput, ResyncWatchdogRetry, ResyncWatchdogDisconnect, ResyncSkippedFresh, ResyncSkippedUnavailable, ResyncServerStaleDims, ResyncServerRequest:
		return r
	}
	return unknown
}

// ResyncCount is one row of the actuator's resync section.
type ResyncCount struct {
	Reason  string `json:"reason"`
	Backend string `json:"backend"`
	Total   uint64 `json:"total"`
}

// CountResync records one resync event under a bounded (reason, backend) label pair and the
// OTel counter terminal_resync_total.
func (r *Recorder) CountResync(reason, backend string) {
	if r == nil {
		return
	}
	k := [2]string{NormalizeResyncReason(reason), NormalizeBackend(backend)}
	r.resync.inc("terminal_resync_total", "Terminal resync requests and client-side skips by reason and backend", "reason", "backend", k)
}

// ResyncCounts returns every (reason, backend) count since process start, sorted.
func (r *Recorder) ResyncCounts() []ResyncCount {
	r.resync.mu.Lock()
	out := make([]ResyncCount, 0, len(r.resync.counts))
	for k, n := range r.resync.counts {
		out = append(out, ResyncCount{Reason: k[0], Backend: k[1], Total: n})
	}
	r.resync.mu.Unlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Reason != out[j].Reason {
			return out[i].Reason < out[j].Reason
		}
		return out[i].Backend < out[j].Backend
	})
	return out
}
