package interceptors

import (
	"context"
	"errors"
	"strings"
	"sync"

	"connectrpc.com/connect"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/telemetry"
)

// RPCSaturation measures server-side RPC pressure: handlers executing right now
// (by kind) and unary calls whose client went away before the handler finished.
// Method labels are Connect procedure names, a bounded set.
type RPCSaturation struct {
	inflight metric.Int64UpDownCounter
	aborted  metric.Int64Counter
}

var (
	rpcSatOnce sync.Once
	rpcSat     *RPCSaturation
)

// NewRPCSaturationInterceptor returns the process-wide interceptor (instruments register once).
func NewRPCSaturationInterceptor() *RPCSaturation {
	rpcSatOnce.Do(func() {
		meter := telemetry.GetMeter()
		r := &RPCSaturation{}
		var err error
		if r.inflight, err = meter.Int64UpDownCounter("rpc.server.inflight",
			metric.WithDescription("RPC handlers executing right now by kind (unary|stream); sustained growth in unary = handlers queued behind a saturated resource")); err != nil {
			log.Warn("interceptors: rpc inflight metric", "error", err)
			r.inflight = nil
		}
		if r.aborted, err = meter.Int64Counter("rpc.server.client_aborted_total",
			metric.WithDescription("Unary calls whose context was canceled by the client before the handler returned, by method")); err != nil {
			log.Warn("interceptors: rpc aborted metric", "error", err)
			r.aborted = nil
		}
		rpcSat = r
	})
	return rpcSat
}

// methodName reduces "/pkg.Service/Method" to "Method".
func methodName(procedure string) string {
	if i := strings.LastIndex(procedure, "/"); i >= 0 {
		return procedure[i+1:]
	}
	return procedure
}

func (r *RPCSaturation) track(ctx context.Context, kind string) func() {
	if r.inflight == nil {
		return func() {}
	}
	attrs := metric.WithAttributes(attribute.String("kind", kind))
	r.inflight.Add(ctx, 1, attrs)
	return func() { r.inflight.Add(context.WithoutCancel(ctx), -1, attrs) }
}

// WrapUnary implements connect.Interceptor.
func (r *RPCSaturation) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		done := r.track(ctx, "unary")
		defer done()
		resp, err := next(ctx, req)
		if err != nil && r.aborted != nil && errors.Is(ctx.Err(), context.Canceled) {
			r.aborted.Add(context.WithoutCancel(ctx), 1,
				metric.WithAttributes(attribute.String("method", methodName(req.Spec().Procedure))))
		}
		return resp, err
	}
}

// WrapStreamingClient implements connect.Interceptor (no client-side behavior).
func (r *RPCSaturation) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

// WrapStreamingHandler implements connect.Interceptor.
func (r *RPCSaturation) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		done := r.track(ctx, "stream")
		defer done()
		return next(ctx, conn)
	}
}
