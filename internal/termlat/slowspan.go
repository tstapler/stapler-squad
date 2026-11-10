package termlat

import (
	"context"
	"os"
	"strconv"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/tstapler/stapler-squad/telemetry"
)

// A terminal call that finishes slower than SlowCallThreshold gets a span written after the
// fact, with its start time backdated. Fast calls never touch the tracer, so a keystroke or a
// capture costs nothing extra and Tempo holds only the slow ones.

// EnvSlowCallMs overrides the threshold in milliseconds.
const EnvSlowCallMs = "STAPLER_SQUAD_SLOW_CALL_MS"

const defaultSlowCall = 250 * time.Millisecond

// SlowCallThreshold is the duration above which a terminal call is traced.
func SlowCallThreshold() time.Duration {
	if v, err := strconv.Atoi(os.Getenv(EnvSlowCallMs)); err == nil && v > 0 {
		return time.Duration(v) * time.Millisecond
	}
	return defaultSlowCall
}

// StageSpan is one named stretch inside a slow call, emitted as a child span.
type StageSpan struct {
	Name       string
	Start, End time.Time
}

// SlowCall describes one finished slow call: a span named Name covering [Start, Start+Duration]
// with each stage as a child span.
type SlowCall struct {
	Name     string
	Start    time.Time
	Duration time.Duration
	Attrs    []attribute.KeyValue
	Stages   []StageSpan
}

// EmitSlowSpan writes the slow call as a span, parented to ctx's span when ctx has one.
func EmitSlowSpan(ctx context.Context, c SlowCall) { emitSlowSpan(ctx, telemetry.GetTracer(), c) }

func emitSlowSpan(ctx context.Context, tracer trace.Tracer, c SlowCall) {
	spanCtx, span := tracer.Start(ctx, c.Name, trace.WithTimestamp(c.Start), trace.WithAttributes(c.Attrs...))
	for _, st := range c.Stages {
		_, child := tracer.Start(spanCtx, st.Name, trace.WithTimestamp(st.Start))
		child.End(trace.WithTimestamp(st.End))
	}
	span.End(trace.WithTimestamp(c.Start.Add(c.Duration)))
}
