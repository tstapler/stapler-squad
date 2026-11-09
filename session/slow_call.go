package session

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

// sessionNamer is how a timed operation learns its session, only when it turns out slow.
type sessionNamer interface{ GetSessionIdentifier() string }

type slowCallReport struct {
	op        string
	backend   backendLabel
	sessionID string
	start     time.Time
	duration  time.Duration
}

var (
	slowCallThreshold = termlat.SlowCallThreshold() //nolint:gochecknoglobals // read per call; tests override
	slowCallReporter  = reportSlowCall              //nolint:gochecknoglobals // swapped in tests

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

func slowCallSessionLabel(sessionID string) string { return slowCallSessions.Label(sessionID) }

func reportSlowCall(ctx context.Context, r slowCallReport) {
	slowCallCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("op", r.op),
		attribute.String("backend", string(r.backend)),
		attribute.String("session", slowCallSessionLabel(r.sessionID))))
	termlat.EmitSlowSpan(ctx, termlat.SlowCall{
		Name:     r.op,
		Start:    r.start,
		Duration: r.duration,
		Attrs: []attribute.KeyValue{
			attribute.String("backend", string(r.backend)),
			attribute.String("session", r.sessionID),
			attribute.Float64("duration_ms", float64(r.duration.Microseconds())/1000),
		},
	})
}
