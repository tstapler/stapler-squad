package interceptors

import (
	"context"
	"strings"
	"sync"

	"connectrpc.com/connect"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/dbobs"
	"github.com/tstapler/stapler-squad/telemetry"
)

var (
	queriesPerRequestOnce sync.Once
	queriesPerRequest     metric.Int64Histogram
	inflightOnce          sync.Once
	inflightUnary         metric.Int64UpDownCounter
)

// trackUnaryInflight bumps rpc.server.inflight for the method until the
// returned func runs; a rising value with flat throughput is queueing.
func trackUnaryInflight(ctx context.Context, procedure string) func() {
	inflightOnce.Do(func() {
		c, err := telemetry.GetMeter().Int64UpDownCounter("rpc.server.inflight",
			metric.WithDescription("Unary RPC calls currently being handled, by rpc.method"))
		if err != nil {
			log.Error("interceptors: failed to register rpc.server.inflight", "error", err)
			return
		}
		inflightUnary = c
	})
	if inflightUnary == nil {
		return func() {}
	}
	opt := metric.WithAttributes(attribute.String("rpc.method", methodName(procedure)))
	inflightUnary.Add(ctx, 1, opt)
	return func() { inflightUnary.Add(context.WithoutCancel(ctx), -1, opt) }
}

func methodName(procedure string) string {
	return procedure[strings.LastIndex(procedure, "/")+1:] // bounded by the proto method set
}

// queriesPerRequestBuckets span a healthy request (a handful of statements) up
// to the N+1 pathologies seen in traces (hundreds).
var queriesPerRequestBuckets = []float64{0, 1, 2, 4, 8, 16, 32, 64, 128, 256, 512, 1024}

func recordQueriesPerRequest(ctx context.Context, procedure string, n int64) {
	queriesPerRequestOnce.Do(func() {
		h, err := telemetry.GetMeter().Int64Histogram("db.queries_per_request",
			metric.WithDescription("Database statements issued per RPC call, by rpc.method"),
			metric.WithExplicitBucketBoundaries(queriesPerRequestBuckets...))
		if err != nil {
			log.Error("interceptors: failed to register db.queries_per_request", "error", err)
			return
		}
		queriesPerRequest = h
	})
	method := methodName(procedure)
	if queriesPerRequest != nil {
		queriesPerRequest.Record(ctx, n, metric.WithAttributes(attribute.String("rpc.method", method)))
	}
	if span := trace.SpanFromContext(ctx); span.IsRecording() {
		span.SetAttributes(attribute.Int64("db.queries", n))
	}
}

// NewDBQueryCountInterceptor counts database statements per unary call or
// stream and reports them as the db.queries_per_request histogram plus a
// db.queries attribute on the RPC span, so N+1 handlers show up on a dashboard.
// Register it after the otel interceptor so the RPC span is in ctx.
func NewDBQueryCountInterceptor() connect.Interceptor { return dbQueryCountInterceptor{} }

type dbQueryCountInterceptor struct{}

func (dbQueryCountInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		ctx, counter := dbobs.WithQueryCounter(ctx)
		defer trackUnaryInflight(ctx, req.Spec().Procedure)()
		resp, err := next(ctx, req)
		recordQueriesPerRequest(ctx, req.Spec().Procedure, counter.Count())
		return resp, err
	}
}

func (dbQueryCountInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (dbQueryCountInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		ctx, counter := dbobs.WithQueryCounter(ctx)
		err := next(ctx, conn)
		recordQueriesPerRequest(ctx, conn.Spec().Procedure, counter.Count())
		return err
	}
}
