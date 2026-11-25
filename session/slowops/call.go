package slowops

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/internal/termlat"
	"github.com/tstapler/stapler-squad/telemetry"
)

// A terminal operation that finishes slower than slowCallThreshold is counted per session and
// traced after the fact. The duration histogram only says how many were slow; this says which
// session and gives Tempo a span to open. Fast calls pay one comparison.

// SessionNamer is how a timed operation learns its session, only when it turns out slow.
type SessionNamer interface{ GetSessionIdentifier() string }

// CallReport describes one slow call.
type CallReport struct {
	Op        string
	Backend   string
	SessionID string
	Start     time.Time
	Duration  time.Duration
}

var (
	slowCallThreshold = termlat.SlowCallThreshold() //nolint:gochecknoglobals // read once at init

	// 20 busiest sessions per 10 minutes keep their label; at most 100 distinct labels per process.
	slowCallSessions = termlat.NewHotSessions(termlat.HotSessionsConfig{Max: 20, Lifetime: 100, Window: 10 * time.Minute}, nil) //nolint:gochecknoglobals // process-wide cardinality cap

	slowCallCounter = mustInt64CounterBackend(telemetry.GetMeter(), "terminal_slow_call_total", //nolint:gochecknoglobals // instrument
		metric.WithDescription("Terminal backend calls slower than STAPLER_SQUAD_SLOW_CALL_MS, by op, backend and session (busiest sessions only; the rest are 'other')"))
)

func mustInt64CounterBackend(meter metric.Meter, name string, opts ...metric.Int64CounterOption) metric.Int64Counter {
	c, err := meter.Int64Counter(name, opts...)
	if err != nil {
		panic(err)
	}
	return c
}

// CallThreshold is the duration at which a call counts as slow (STAPLER_SQUAD_SLOW_CALL_MS).
func CallThreshold() time.Duration { return slowCallThreshold }

// ReportIfSlow passes the call to report when d reaches threshold; both are parameters so tests
// need neither the process-global threshold nor a real sleep. who may be nil.
func ReportIfSlow(ctx context.Context, who SessionNamer, op, backend string, start time.Time, d, threshold time.Duration, report func(context.Context, CallReport)) {
	if d < threshold {
		return
	}
	id := ""
	if who != nil {
		id = who.GetSessionIdentifier()
	}
	report(ctx, CallReport{Op: op, Backend: backend, SessionID: id, Start: start, Duration: d})
}

func slowCallSessionLabel(sessionID string) string { return slowCallSessions.Label(sessionID) }

// ReportCall counts the call per session and emits its slow span; it is ReportIfSlow's production sink.
func ReportCall(ctx context.Context, r CallReport) {
	slowCallCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("op", r.Op),
		attribute.String("backend", r.Backend),
		attribute.String("session", slowCallSessionLabel(r.SessionID))))
	termlat.EmitSlowSpan(ctx, termlat.SlowCall{
		Name:     r.Op,
		Start:    r.Start,
		Duration: r.Duration,
		Attrs: []attribute.KeyValue{
			attribute.String("backend", r.Backend),
			attribute.String("session", r.SessionID),
			attribute.Float64("duration_ms", float64(r.Duration.Microseconds())/1000),
		},
	})
}
