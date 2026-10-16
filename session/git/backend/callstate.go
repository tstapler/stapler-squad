package backend

import (
	"context"
	"sync/atomic"
)

// CallState records, for one Backend call, whether any step already wrote repository state.
// It is the call-level "Wrote" flag of plan Story 1.1.3: the OR over every lock scope and
// worktree write the call performed, so the Router never replays a call that half-applied.
// Only the carrier exists today: no backend marks it yet (the gogit lock layer does, Story
// 2.3.1). It is monotonic: once marked it stays marked.
type CallState struct {
	wrote atomic.Bool
}

// MarkWrote records that the call changed a ref, the index, config or the worktree.
func (s *CallState) MarkWrote() { s.wrote.Store(true) }

// Wrote reports whether MarkWrote was called.
func (s *CallState) Wrote() bool { return s.wrote.Load() }

type callStateKey struct{}

// WithCallState returns a context carrying a CallState, and that state. If ctx already carries
// one it is reused, so every nested scope of one call accumulates into the same flag.
func WithCallState(ctx context.Context) (context.Context, *CallState) {
	if s := CallStateFrom(ctx); s != nil {
		return ctx, s
	}
	s := &CallState{}
	return context.WithValue(ctx, callStateKey{}, s), s
}

// CallStateFrom returns the CallState carried by ctx, or nil when there is none.
func CallStateFrom(ctx context.Context) *CallState {
	s, _ := ctx.Value(callStateKey{}).(*CallState)
	return s
}
