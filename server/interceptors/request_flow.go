package interceptors

import (
	"context"
	"errors"
	"sync"
	"time"

	"connectrpc.com/connect"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/telemetry"
)

type arrivalKey struct{}

// WithArrival stamps when the HTTP request first reached the process, so the
// request-flow interceptor can report how long it spent in the middleware chain
// before its handler started.
func WithArrival(ctx context.Context, t time.Time) context.Context {
	return context.WithValue(ctx, arrivalKey{}, t)
}

// preHandlerBuckets are in seconds: a healthy chain is sub-millisecond, so
// anything past 100ms is queueing behind something.
var preHandlerBuckets = []float64{0.0005, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 15}

type flowInstruments struct {
	preHandler metric.Float64Histogram
	aborted    metric.Int64Counter
}

var (
	flowOnce sync.Once
	flow     *flowInstruments
)

func flowMetrics() *flowInstruments {
	flowOnce.Do(func() {
		m := telemetry.GetMeter()
		pre, err := m.Float64Histogram("rpc.server.pre_handler_seconds",
			metric.WithUnit("s"),
			metric.WithDescription("Time from the HTTP request reaching the process to the RPC handler starting (middleware chain, guards, auth), by class unary|stream"),
			metric.WithExplicitBucketBoundaries(preHandlerBuckets...))
		if err != nil {
			log.Error("interceptors: failed to register rpc.server.pre_handler_seconds", "error", err)
			return
		}
		ab, err := m.Int64Counter("rpc.server.client_aborted_total",
			metric.WithDescription("RPC calls whose client went away (context canceled) before the handler finished, by rpc.method"))
		if err != nil {
			log.Error("interceptors: failed to register rpc.server.client_aborted_total", "error", err)
			return
		}
		flow = &flowInstruments{preHandler: pre, aborted: ab}
	})
	return flow
}

// NewRequestFlowInterceptor reports pre-handler time and client aborts.
func NewRequestFlowInterceptor() connect.Interceptor { return requestFlowInterceptor{} }

type requestFlowInterceptor struct{}

func observePreHandler(ctx context.Context, class string) {
	f := flowMetrics()
	if f == nil {
		return
	}
	if t, ok := ctx.Value(arrivalKey{}).(time.Time); ok {
		f.preHandler.Record(ctx, time.Since(t).Seconds(), metric.WithAttributes(attribute.String("class", class)))
	}
}

func observeAbort(ctx context.Context, procedure string, err error) {
	if err == nil || !errors.Is(ctx.Err(), context.Canceled) {
		return
	}
	if f := flowMetrics(); f != nil {
		f.aborted.Add(context.WithoutCancel(ctx), 1, metric.WithAttributes(attribute.String("rpc.method", methodName(procedure))))
	}
}

func (requestFlowInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		observePreHandler(ctx, "unary")
		resp, err := next(ctx, req)
		observeAbort(ctx, req.Spec().Procedure, err)
		return resp, err
	}
}

func (requestFlowInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (requestFlowInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		observePreHandler(ctx, "stream")
		return next(ctx, conn)
	}
}
