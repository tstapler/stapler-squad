package backend

import (
	"context"
	"sync/atomic"
)

// CallState records, for one Backend call, whether any step already wrote repository state.
// It is the call-level "Wrote" flag of plan Story 1.1.3: the OR over every lock scope and
// worktree write the call performed, so the Router never replays a call that half-applied.
type CallState struct {
	wrote atomic.Bool
}

// MarkWrote records that the call changed a ref, the index, config or the worktree.
func (s *CallState) MarkWrote() { s.wrote.Store(true) }

// Wrote reports whether MarkWrote was called.
func (s *CallState) Wrote() bool { return s.wrote.Load() }

type callStateKey struct{}

// WithCallState returns a context carrying a fresh CallState, and that state.
func WithCallState(ctx context.Context) (context.Context, *CallState) {
	s := &CallState{}
	return context.WithValue(ctx, callStateKey{}, s), s
}

// CallStateFrom returns the CallState carried by ctx, or nil when there is none.
func CallStateFrom(ctx context.Context) *CallState {
	s, _ := ctx.Value(callStateKey{}).(*CallState)
	return s
}
