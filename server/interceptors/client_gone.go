package interceptors

import (
	"context"
	"errors"
	"strings"
	"syscall"

	"connectrpc.com/connect"
)

// NewClientGoneInterceptor rewrites errors caused by the caller disconnecting (broken pipe,
// connection reset, canceled ctx) into connect.CodeCanceled. Place it inside otelconnect:
// otelconnect leaves spans of CodeCanceled errors unset but marks any other error as failed.
func NewClientGoneInterceptor() connect.Interceptor { return clientGoneInterceptor{} }

type clientGoneInterceptor struct{}

// connectWriteFailure is connect-go's own prefix for a failed write to the client; a backend
// socket error inside a handler never carries it.
const connectWriteFailure = "write envelope"

// isClientGone reports a caller disconnect: the request ctx is dead, or connect itself failed
// writing to the client with a broken pipe / reset. An EPIPE/ECONNRESET from a backend socket
// with a live caller is a real server error and is left alone.
func isClientGone(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}
	// connect wraps a failed client write as CodeUnknown, so look through that code to the cause;
	// any other deliberate code (Internal, Unavailable, ...) is a real server verdict.
	var ce *connect.Error
	if errors.As(err, &ce) {
		if ce.Code() == connect.CodeCanceled {
			return true
		}
		if ce.Code() != connect.CodeUnknown {
			return false
		}
	}
	if ctx.Err() != nil {
		return errors.Is(err, context.Canceled)
	}
	return (errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET)) &&
		strings.Contains(err.Error(), connectWriteFailure)
}

func asCanceled(ctx context.Context, err error) error {
	if !isClientGone(ctx, err) {
		return err
	}
	if connect.CodeOf(err) == connect.CodeCanceled {
		return err
	}
	return connect.NewError(connect.CodeCanceled, err)
}

func (clientGoneInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		resp, err := next(ctx, req)
		return resp, asCanceled(ctx, err)
	}
}

func (clientGoneInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (clientGoneInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		return asCanceled(ctx, next(ctx, conn))
	}
}
