package backend

import "context"

// CallInfo attributes the git processes one Backend call spawns: which operation was asked
// for and why it was served by the CLI. The Router puts it in the context it hands the CLI
// backend, so the spawn counter (git_backend_cli_spawn_total) needs no runtime.Caller.
type CallInfo struct {
	Op     OperationName
	Reason FallbackReason
}

type callInfoKey struct{}

// WithCallInfo returns ctx carrying info, replacing any CallInfo already there.
func WithCallInfo(ctx context.Context, info CallInfo) context.Context {
	return context.WithValue(ctx, callInfoKey{}, info)
}

// CallInfoFrom returns the CallInfo carried by ctx; ok is false when the call was not
// routed (a spawn then counts under SpawnReasonUnattributed).
func CallInfoFrom(ctx context.Context) (info CallInfo, ok bool) {
	info, ok = ctx.Value(callInfoKey{}).(CallInfo)
	return info, ok
}
