package interceptors

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
)

// NewFeatureFlagInterceptor returns a ConnectRPC unary interceptor that returns
// connect.CodeNotFound when isEnabled reports false.  Use this to gate an entire
// service handler behind a runtime feature flag so that unauthenticated API calls
// are rejected even if the caller bypasses the frontend redirect.
//
// isEnabled is called on every request so flag changes are reflected immediately
// without a server restart.  The recommended implementation is:
//
//	func() bool { return config.LoadConfig().GetFeatureFlag("backlog") }
func NewFeatureFlagInterceptor(flagName string, isEnabled func() bool) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if !isEnabled() {
				return nil, connect.NewError(
					connect.CodeNotFound,
					fmt.Errorf("feature %q is not enabled", flagName),
				)
			}
			return next(ctx, req)
		}
	}
}
