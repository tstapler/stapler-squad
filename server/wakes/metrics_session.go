package wakes

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/telemetry"
)

// HistogramFiresPerSession is the Epic 4 cost outcome: wake deliveries a
// session received, sampled once when the session is deleted or archived.
const HistogramFiresPerSession = "wake_fires_per_session"

// sessionFires tallies steer deliveries per owner session. In-process only: a
// restart or a dispatcher stop/start drops the tallies of live sessions.
type sessionFires struct {
	mu      sync.Mutex
	pending map[string]int
	samples []float64
	hist    metric.Float64Histogram
}

// WakeDelivered counts one delivered wake against the owner session.
func (m *Metrics) WakeDelivered(ownerSessionUUID string) {
	if m == nil || ownerSessionUUID == "" {
		return
	}
	m.fires.mu.Lock()
	defer m.fires.mu.Unlock()
	if m.fires.pending == nil {
		m.fires.pending = map[string]int{}
	}
	m.fires.pending[ownerSessionUUID]++
}

// SessionEnded records the session's delivery tally (zero when it received
// none) as one wake_fires_per_session sample and forgets the session.
func (m *Metrics) SessionEnded(sessionUUID string) {
	if m == nil || sessionUUID == "" {
		return
	}
	f := &m.fires
	f.mu.Lock()
	n := float64(f.pending[sessionUUID])
	delete(f.pending, sessionUUID)
	f.samples = append(f.samples, n)
	if f.hist == nil {
		if h, err := telemetry.GetMeter().Float64Histogram(HistogramFiresPerSession,
			metric.WithDescription("Wake deliveries a session received, sampled when it ends"),
			metric.WithExplicitBucketBoundaries(0, 1, 2, 3, 5, 8, 13, 21, 34, 55)); err == nil {
			f.hist = h
		}
	}
	h := f.hist
	f.mu.Unlock()
	if h != nil {
		h.Record(context.Background(), n)
	}
}

// FiresPerSession returns the wake_fires_per_session samples, oldest first.
func (m *Metrics) FiresPerSession() []float64 {
	if m == nil {
		return nil
	}
	m.fires.mu.Lock()
	defer m.fires.mu.Unlock()
	return append([]float64(nil), m.fires.samples...)
}

// SessionEndObserver is an EventMatcher that never matches: it only samples
// wake_fires_per_session when a session is deleted or archived. Register it
// with Dispatcher.RegisterMatcher.
func (m *Metrics) SessionEndObserver() EventMatcher {
	return func(ev *events.Event) []EventMatch {
		if ev != nil && (ev.Type == events.EventSessionDeleted || ev.Type == events.EventSessionArchived) {
			m.SessionEnded(ev.SessionID)
		}
		return nil
	}
}
