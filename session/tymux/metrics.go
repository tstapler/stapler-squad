package tymux

import (
	"context"

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
