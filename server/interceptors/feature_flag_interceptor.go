package interceptors

import (
	"context"
	"fmt"
	"strings"

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

// NewScopedFeatureFlagInterceptor returns a ConnectRPC unary interceptor that
// gates only the given RPC method names behind a runtime feature flag,
// letting every other method on the same service handler through
// unconditionally. Use this when a service mixes read-only RPCs (which
// should always be reachable) with mutating RPCs that are still gated behind
// a rollout flag -- registering NewFeatureFlagInterceptor on the whole
// handler would incorrectly gate the read-only methods too.
//
// gatedMethods holds bare RPC method names (e.g. "CommitImportExternalSession"),
// not full procedure paths -- this matches against the last path segment of
// connect.Spec().Procedure ("/pkg.Service/MethodName").
//
// Returns connect.CodeUnimplemented (rather than CodeNotFound, see
// NewFeatureFlagInterceptor) since a gated mutating RPC on an otherwise-live
// service reads more accurately as "not yet available" than "not found".
func NewScopedFeatureFlagInterceptor(flagName string, isEnabled func() bool, gatedMethods ...string) connect.UnaryInterceptorFunc {
	gated := make(map[string]bool, len(gatedMethods))
	for _, m := range gatedMethods {
		gated[m] = true
	}

	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			method := procedureMethodName(req.Spec().Procedure)
			if gated[method] && !isEnabled() {
				return nil, connect.NewError(
					connect.CodeUnimplemented,
					fmt.Errorf("feature %q is not enabled", flagName),
				)
			}
			return next(ctx, req)
		}
	}
}

// procedureMethodName extracts the bare method name from a fully-qualified
// connect procedure path such as "/session.v1.ImportService/CommitImportExternalSession".
func procedureMethodName(procedure string) string {
	if idx := strings.LastIndex(procedure, "/"); idx >= 0 {
		return procedure[idx+1:]
	}
	return procedure
}
