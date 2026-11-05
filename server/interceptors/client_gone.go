package interceptors

import (
	"context"
	"errors"
	"syscall"

	"connectrpc.com/connect"
)

// NewClientGoneInterceptor rewrites errors caused by the caller disconnecting (broken pipe,
// connection reset, canceled ctx) into connect.CodeCanceled. Place it inside otelconnect:
// otelconnect leaves spans of CodeCanceled errors unset but marks any other error as failed.
func NewClientGoneInterceptor() connect.Interceptor { return clientGoneInterceptor{} }

type clientGoneInterceptor struct{}

func isClientGone(err error) bool {
	if err == nil {
		return false
	}
	var ce *connect.Error
	if errors.As(err, &ce) {
		return ce.Code() == connect.CodeCanceled
	}
	return errors.Is(err, context.Canceled) ||
		errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET)
}

func asCanceled(err error) error {
	if err == nil || !isClientGone(err) {
		return err
	}
	var ce *connect.Error
	if errors.As(err, &ce) {
		return err
	}
	return connect.NewError(connect.CodeCanceled, err)
}

func (clientGoneInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		resp, err := next(ctx, req)
		return resp, asCanceled(err)
	}
}

func (clientGoneInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (clientGoneInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		return asCanceled(next(ctx, conn))
	}
}
