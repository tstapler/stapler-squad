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

// Daemon supervision metrics. tymux_daemon_restarts_total{reason}:
// cold_start = a spawn because no healthy daemon answered; replace = an
// operator-requested ReplaceDaemon; detected_restart = a stream observed the
// daemon had restarted underneath it (counted once per affected stream).
const (
	daemonRestartColdStart = "cold_start"
	daemonRestartReplace   = "replace"
	daemonRestartDetected  = "detected_restart"
)

var (
	daemonRestartCounter = mustInt64Counter("tymux_daemon_restarts_total",
		metric.WithDescription("tymuxd spawns/restarts observed by this process, by reason (cold_start, replace, detected_restart)"))
	daemonStartDuration = mustFloat64Histogram("tymux_daemon_start_duration_ms",
		metric.WithDescription("Time from spawning tymuxd until it answers health checks (or the retry budget is exhausted), by outcome"),
		metric.WithUnit("ms"),
		metric.WithExplicitBucketBoundaries(50, 100, 250, 500, 1000, 2000, 4000, 8000, 16000))
	daemonVersionMismatch = mustInt64Gauge("tymux_daemon_version_mismatch",
		metric.WithDescription("1 when the running tymuxd version differs from the binary stapler-squad would spawn, 0 when they match"))
)

const (
	daemonStartOK     = "ok"
	daemonStartFailed = "failed"
)

func recordDaemonRestart(reason string) {
	daemonRestartCounter.Add(context.Background(), 1, metric.WithAttributes(attribute.String("reason", reason)))
}

func recordDaemonStartDuration(d time.Duration, outcome string) {
	daemonStartDuration.Record(context.Background(), float64(d.Microseconds())/1000,
		metric.WithAttributes(attribute.String("outcome", outcome)))
}

// recordVersionMismatch leaves the gauge untouched when the direction is
// unknown (a version could not be read): reporting 0 there would hide a
// mismatch behind a probe failure.
func recordVersionMismatch(d VersionDirection) {
	switch d {
	case VersionSame:
		daemonVersionMismatch.Record(context.Background(), 0)
	case VersionUpgrade, VersionDowngrade:
		daemonVersionMismatch.Record(context.Background(), 1)
	case VersionUnknown:
	}
}

func mustFloat64Histogram(name string, opts ...metric.Float64HistogramOption) metric.Float64Histogram {
	h, err := telemetry.GetMeter().Float64Histogram(name, opts...)
	if err != nil {
		panic(err)
	}
	return h
}

func mustInt64Gauge(name string, opts ...metric.Int64GaugeOption) metric.Int64Gauge {
	g, err := telemetry.GetMeter().Int64Gauge(name, opts...)
	if err != nil {
		panic(err)
	}
	return g
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
