package tymux

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/telemetry"
)

// Instruments are built against telemetry.GetMeter() at package init, which
// is OTel's global delegating meter until telemetry.Initialize installs the
// real provider — the same idiom as session/lifecycle and
// session/backend_observability.go. Label keys are deliberately low
// cardinality (cause, kind, reason): never a session id or pane id.
var (
	reconnectCounter = mustInt64Counter("tymux_attach_stream_reconnects_total",
		metric.WithDescription("Standing Attach stream reconnects, by cause (error = transport drop, output_gap = in-stream resync)"))
	attachEventCounter = mustInt64Counter("tymux_attach_events_total",
		metric.WithDescription("Notable AttachEvent handling, by kind (output_gap, gap_exceeded, snapshot_redraw)"))
)

// Attach event kinds for tymux_attach_events_total{kind}.
const (
	attachEventOutputGap      = "output_gap"
	attachEventGapExceeded    = "gap_exceeded"
	attachEventSnapshotRedraw = "snapshot_redraw"
)

// liveStreams is the set of sessions with a running standing-stream reader
// goroutine, read by the heartbeat-age gauge callback. A set rather than a
// per-session label keeps the gauge's cardinality at one series.
var liveStreams sync.Map // *tymuxGRPCSession -> struct{}

func (s *tymuxGRPCSession) markStreamEvent() {
	s.lastEventUnixNano.Store(time.Now().UnixNano())
}

// maxHeartbeatAge is the age of the stalest live stream's last event
// (0 when no stream is live).
func maxHeartbeatAge() time.Duration {
	now := time.Now().UnixNano()
	var oldest int64
	liveStreams.Range(func(k, _ any) bool {
		if age := now - k.(*tymuxGRPCSession).lastEventUnixNano.Load(); age > oldest {
			oldest = age
		}
		return true
	})
	return time.Duration(oldest)
}

func init() {
	_, err := telemetry.GetMeter().Float64ObservableGauge("tymux_attach_heartbeat_age_seconds",
		metric.WithDescription("Age of the stalest live Attach stream's last event (heartbeat or otherwise), max across streams; 0 when none are live"),
		metric.WithUnit("s"),
		metric.WithFloat64Callback(func(_ context.Context, o metric.Float64Observer) error {
			o.Observe(maxHeartbeatAge().Seconds())
			return nil
		}))
	if err != nil {
		panic(err)
	}
}

func mustInt64Counter(name string, opts ...metric.Int64CounterOption) metric.Int64Counter {
	c, err := telemetry.GetMeter().Int64Counter(name, opts...)
	if err != nil {
		panic(err) // malformed build-time constant; fail loudly in tests
	}
	return c
}

// recordReconnect increments tymux_attach_stream_reconnects_total{cause}.
func recordReconnect(cause string) {
	reconnectCounter.Add(context.Background(), 1, metric.WithAttributes(attribute.String("cause", cause)))
}

// recordAttachEvent increments tymux_attach_events_total{kind}.
func recordAttachEvent(kind string) {
	attachEventCounter.Add(context.Background(), 1, metric.WithAttributes(attribute.String("kind", kind)))
}
