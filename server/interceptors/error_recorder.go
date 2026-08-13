package interceptors

import (
	"context"
	"fmt"
	"runtime"

	"connectrpc.com/connect"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// ErrorRecorder is the minimal interface the interceptor needs to persist errors.
// Implemented by *services.ErrorRegistry; using an interface avoids a circular import.
type ErrorRecorder interface {
	Record(ctx context.Context, err error, procedure string)
}

// NewErrorRecorderInterceptor returns a ConnectRPC unary interceptor that records
// errors as OTel span events with the error message, first 5 stack frames, and RPC
// procedure name. It is safe to use when OTel is not configured (span.IsRecording()
// returns false and the span recording block is a no-op).
//
// When registry is non-nil, every error is also persisted to SQLite via
// registry.Record for the /ListErrors and /AcknowledgeError RPCs.
func NewErrorRecorderInterceptor(registry ErrorRecorder) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			resp, err := next(ctx, req)
			if err != nil {
				// OTel span recording (existing behaviour)
				span := trace.SpanFromContext(ctx)
				if span.IsRecording() {
					frames := captureStack(5)
					span.SetStatus(codes.Error, err.Error())
					span.AddEvent("rpc.error", trace.WithAttributes(
						attribute.String("error.message", err.Error()),
						attribute.String("error.stack", frames),
						attribute.String("rpc.procedure", req.Spec().Procedure),
					))
				}
				// SQLite error registry (new)
				if registry != nil {
					registry.Record(ctx, err, req.Spec().Procedure)
				}
			}
			return resp, err
		}
	}
}

func captureStack(maxFrames int) string {
	pcs := make([]uintptr, maxFrames)
	n := runtime.Callers(3, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	var out string
	for {
		f, more := frames.Next()
		out += fmt.Sprintf("%s:%d\n", f.Function, f.Line)
		if !more {
			break
		}
	}
	return out
}
