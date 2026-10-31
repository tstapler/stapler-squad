package services

import (
	"context"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/telemetry"
)

// Terminal stream creation telemetry. Every label is a fixed enum: session IDs
// never become labels (they belong on spans and log lines).

// Stage names for terminal_stream.stage_duration. They partition creation as
// the browser experiences it, from the HTTP handler starting to the first
// paint being written.
const (
	stageUpgrade         = "upgrade"          // handler entry -> 101 written
	stageHandshake       = "handshake"        // 101 -> Connect headers/body read and acknowledged
	stageResolve         = "resolve"          // handshake -> session resolved, hibernate-resume done, routed to the hub path
	stageBackendReady    = "backend_ready"    // restore/start wait: backend process alive and instance started
	stageControlMode     = "control_mode"     // StartControlMode
	stageHubCreate       = "hub_create"       // HubRegistry get-or-create
	stageHubAttach       = "hub_attach"       // subscriber attached and resize requested
	stageInitialSnapshot = "initial_snapshot" // subscriber attached -> first paint written
	stageTotal           = "total"            // handler entry -> first paint written
)

// Close causes for terminal_stream.closes.
const (
	closeCauseClean        = "clean"
	closeCauseServerClosed = "server_closed" // hub or transport ended the stream; reported as success
	closeCauseError        = "error"
)

var (
	terminalCreationsInFlight atomic.Int64

	terminalStageHist     metric.Float64Histogram
	terminalClosesCounter metric.Int64Counter
)

func init() {
	if err := registerTerminalStreamMetrics(); err != nil {
		log.Error("services: failed to register terminal stream OTel metrics", "error", err)
	}
}

func registerTerminalStreamMetrics() error {
	meter := telemetry.GetMeter()
	var err error
	if terminalStageHist, err = meter.Float64Histogram("terminal_stream.stage_duration",
		metric.WithUnit("ms"),
		metric.WithDescription("StreamTerminal creation latency by stage: upgrade, handshake, hub_attach, initial_snapshot, total"),
		metric.WithExplicitBucketBoundaries(5, 10, 25, 50, 100, 250, 500, 1000, 2000, 4000, 8000, 16000)); err != nil {
		return err
	}
	if terminalClosesCounter, err = meter.Int64Counter("terminal_stream.closes",
		metric.WithDescription("StreamTerminal closes by cause: clean, server_closed, error")); err != nil {
		return err
	}
	inFlight, err := meter.Int64ObservableGauge("terminal_stream.creations_in_flight",
		metric.WithDescription("StreamTerminal connections between handler entry and first paint"))
	if err != nil {
		return err
	}
	_, err = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		o.ObserveInt64(inFlight, terminalCreationsInFlight.Load())
		return nil
	}, inFlight)
	return err
}

// terminalCreation times one StreamTerminal creation. Mark records a stage as the
// time since the previous mark; End finishes the creation exactly once.
type terminalCreation struct {
	start time.Time
	last  time.Time
	now   func() time.Time
	done  atomic.Bool
	span  trace.Span
	ctx   context.Context
}

func beginTerminalCreation() *terminalCreation {
	return beginTerminalCreationAt(time.Now)
}

func beginTerminalCreationAt(now func() time.Time) *terminalCreation {
	ctx, span := telemetry.StartSpan(context.Background(), "terminal_stream.create")
	t := now()
	terminalCreationsInFlight.Add(1)
	return &terminalCreation{start: t, last: t, now: now, span: span, ctx: ctx}
}

// Mark records stage as the elapsed time since the previous Mark (or the start).
func (c *terminalCreation) Mark(stage string) time.Duration {
	if c == nil {
		return 0
	}
	t := c.now()
	d := t.Sub(c.last)
	c.last = t
	recordTerminalStage(stage, d)
	c.span.AddEvent(stage, trace.WithAttributes(attribute.Int64("duration_ms", d.Milliseconds())))
	return d
}

// End records the total and closes the span; later calls are no-ops. ok=false
// marks an attempt that never produced a first paint.
func (c *terminalCreation) End(ok bool) {
	if c == nil || !c.done.CompareAndSwap(false, true) {
		return
	}
	terminalCreationsInFlight.Add(-1)
	if ok {
		recordTerminalStage(stageTotal, c.now().Sub(c.start))
	}
	c.span.SetAttributes(attribute.Bool("first_paint", ok))
	c.span.End()
}

func recordTerminalStage(stage string, d time.Duration) {
	if terminalStageHist != nil {
		terminalStageHist.Record(context.Background(), float64(d)/float64(time.Millisecond),
			metric.WithAttributes(attribute.String("stage", stage)))
	}
}

func recordTerminalClose(cause string) {
	if terminalClosesCounter != nil {
		terminalClosesCounter.Add(context.Background(), 1, metric.WithAttributes(attribute.String("cause", cause)))
	}
}
