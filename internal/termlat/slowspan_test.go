package termlat

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestEmitSlowSpan_should_BackdateTheParentAndNestTheStages(t *testing.T) {
	t.Parallel()
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	start := time.Unix(1_700_000_000, 0)

	emitSlowSpan(context.Background(), tp.Tracer("test"), SlowCall{
		Name: "terminal.input", Start: start, Duration: 600 * time.Millisecond,
		Attrs:  []attribute.KeyValue{attribute.String("backend", "tymux")},
		Stages: []StageSpan{{Name: "terminal.input.send", Start: start.Add(100 * time.Millisecond), End: start.Add(550 * time.Millisecond)}},
	})

	spans := exp.GetSpans()
	require.Len(t, spans, 2)
	byName := map[string]tracetest.SpanStub{}
	for _, s := range spans {
		byName[s.Name] = s
	}
	parent, child := byName["terminal.input"], byName["terminal.input.send"]
	assert.Equal(t, start, parent.StartTime)
	assert.Equal(t, start.Add(600*time.Millisecond), parent.EndTime)
	assert.Equal(t, parent.SpanContext.SpanID(), child.Parent.SpanID(), "stage must be a child of the slow call")
	assert.Equal(t, start.Add(100*time.Millisecond), child.StartTime)
}

func TestSlowCallThreshold_should_HonourTheEnvOverride(t *testing.T) {
	t.Setenv(EnvSlowCallMs, "40")
	assert.Equal(t, 40*time.Millisecond, SlowCallThreshold())
	t.Setenv(EnvSlowCallMs, "junk")
	assert.Equal(t, defaultSlowCall, SlowCallThreshold())
}
