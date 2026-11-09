package termlat

import (
	"context"
	"sort"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/telemetry"
)

// Resync reasons. Client reasons come from the browser (cm.term_resync); server reasons are
// counted where a CurrentPaneRequest arrives. "skipped_*" are resyncs the client chose not to send.
const (
	ResyncVisibility         = "visibility"          // client sent one on tab/window focus or visibility
	ResyncResize             = "resize"              // client sent one after a settled resize
	ResyncSkippedFresh       = "skipped_fresh"       // focus/visibility, but size unchanged and the last sync is recent
	ResyncSkippedUnavailable = "skipped_unavailable" // focus/visibility, but the terminal could not be measured
	ResyncServerStaleDims    = "server_stale_dims"   // server received a request the client flagged as same-size
	ResyncServerRequest      = "server_request"      // server received any other CurrentPaneRequest
)

// NormalizeResyncReason returns r if it is a known reason, else "unknown".
func NormalizeResyncReason(r string) string {
	switch r {
	case ResyncVisibility, ResyncResize, ResyncSkippedFresh, ResyncSkippedUnavailable, ResyncServerStaleDims, ResyncServerRequest:
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
	r.resync.mu.Lock()
	if r.resync.counts == nil {
		r.resync.counts = map[[2]string]uint64{}
	}
	r.resync.counts[k]++
	r.resync.mu.Unlock()
	r.resync.once.Do(func() {
		c, err := telemetry.GetMeter().Int64Counter("terminal_resync_total",
			metric.WithDescription("Terminal resync requests and client-side skips by reason and backend"))
		if err != nil {
			log.Error("termlat: failed to register terminal_resync_total", "error", err)
			return
		}
		r.resync.counter = c
	})
	if r.resync.counter != nil {
		r.resync.counter.Add(context.Background(), 1, metric.WithAttributes(
			attribute.String("reason", k[0]), attribute.String("backend", k[1])))
	}
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
