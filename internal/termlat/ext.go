package termlat

import (
	"context"
	"sort"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/telemetry"
)

// Frame drop causes for terminal_frame_drops_total.
const (
	DropHubQueueFull = "hub_queue_full"    // a hub subscriber's outbound queue was full; the frame was dropped
	DropFanoutFull   = "tymux_fanout_full" // a tymux ClientFanout subscriber queue was full
)

// NormalizeDropCause returns c if it is a known cause, else "unknown".
func NormalizeDropCause(c string) string {
	switch c {
	case DropHubQueueFull, DropFanoutFull:
		return c
	}
	return unknown
}

// DropCount is one row of the actuator's drop section.
type DropCount struct {
	Cause   string `json:"cause"`
	Backend string `json:"backend"`
	Total   uint64 `json:"total"`
}

type dropCounters struct {
	mu      sync.Mutex
	counts  map[[2]string]uint64
	once    sync.Once
	counter metric.Int64Counter
}

// CountDrop records one dropped output frame under a bounded (cause, backend) label pair and the
// OTel counter terminal_frame_drops_total.
func (r *Recorder) CountDrop(cause, backend string) {
	if r == nil {
		return
	}
	k := [2]string{NormalizeDropCause(cause), NormalizeBackend(backend)}
	r.drops.mu.Lock()
	if r.drops.counts == nil {
		r.drops.counts = map[[2]string]uint64{}
	}
	r.drops.counts[k]++
	r.drops.mu.Unlock()
	r.drops.once.Do(func() {
		c, err := telemetry.GetMeter().Int64Counter("terminal_frame_drops_total",
			metric.WithDescription("Output frames dropped on a full queue, by cause and backend"))
		if err != nil {
			log.Error("termlat: failed to register terminal_frame_drops_total", "error", err)
			return
		}
		r.drops.counter = c
	})
	if r.drops.counter != nil {
		r.drops.counter.Add(context.Background(), 1, metric.WithAttributes(
			attribute.String("cause", k[0]), attribute.String("backend", k[1])))
	}
}

// DropCounts returns every (cause, backend) count since process start, sorted.
func (r *Recorder) DropCounts() []DropCount {
	r.drops.mu.Lock()
	out := make([]DropCount, 0, len(r.drops.counts))
	for k, n := range r.drops.counts {
		out = append(out, DropCount{Cause: k[0], Backend: k[1], Total: n})
	}
	r.drops.mu.Unlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Cause != out[j].Cause {
			return out[i].Cause < out[j].Cause
		}
		return out[i].Backend < out[j].Backend
	})
	return out
}

const (
	otherSession       = "other"
	maxSessionLabelLen = 24
)

// HotSessions turns a session id into a metric label with bounded cardinality: the max most
// active sessions of the current window keep their (truncated) id, everything else is "other".
// A lifetime cap on distinct ids bounds the series ever created, since an OTel SDK keeps every
// attribute set it has seen until restart. Meant for rare events (slow operations), not hot paths.
type HotSessions struct {
	mu       sync.Mutex
	max      int
	lifetime int
	window   time.Duration
	now      func() time.Time
	winStart time.Time
	counts   map[string]int      // hits this window, members only
	members  map[string]struct{} // current window's labelled sessions
	minted   map[string]struct{} // every label ever handed out
}

// HotSessionsConfig sizes a HotSessions: Max labelled sessions per Window, at most Lifetime
// distinct labels ever minted.
type HotSessionsConfig struct {
	Max, Lifetime int
	Window        time.Duration
}

// NewHotSessions returns a labeler (time.Now when now is nil).
func NewHotSessions(cfg HotSessionsConfig, now func() time.Time) *HotSessions {
	if now == nil {
		now = time.Now
	}
	return &HotSessions{max: cfg.Max, lifetime: cfg.Lifetime, window: cfg.Window, now: now, winStart: now(),
		counts: map[string]int{}, members: map[string]struct{}{}, minted: map[string]struct{}{}}
}

// Label returns the metric label for id and counts the hit.
func (h *HotSessions) Label(id string) string {
	if len(id) > maxSessionLabelLen {
		id = id[:maxSessionLabelLen]
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if t := h.now(); t.Sub(h.winStart) >= h.window {
		h.winStart = t
		h.counts = map[string]int{}
		h.members = map[string]struct{}{}
	}
	h.counts[id]++
	if _, ok := h.members[id]; ok {
		return id
	}
	if len(h.members) < h.max {
		return h.admit(id)
	}
	weakest, weakestN := "", int(^uint(0)>>1)
	for m := range h.members {
		if n := h.counts[m]; n < weakestN {
			weakest, weakestN = m, n
		}
	}
	if _, minted := h.minted[id]; !minted && len(h.minted) >= h.lifetime {
		return otherSession // evicting a member for a label that cannot be minted would only lose one
	}
	if h.counts[id] > weakestN {
		delete(h.members, weakest)
		return h.admit(id)
	}
	return otherSession
}

// admit makes id a window member unless the lifetime cap forbids minting a new label.
func (h *HotSessions) admit(id string) string {
	if _, ok := h.minted[id]; !ok {
		if len(h.minted) >= h.lifetime {
			return otherSession
		}
		h.minted[id] = struct{}{}
	}
	h.members[id] = struct{}{}
	return id
}
