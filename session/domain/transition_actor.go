package domain

import "context"

type transitionActorKey struct{}

// WithTransitionActor marks ctx so TransitionBacklogItemStatus records the given
// audit actor (default user), e.g. the agent actor for MCP callers.
func WithTransitionActor(ctx context.Context, actor string) context.Context {
	return context.WithValue(ctx, transitionActorKey{}, actor)
}

// TransitionActorFrom returns the actor set by WithTransitionActor, or "" when none was set.
func TransitionActorFrom(ctx context.Context) string {
	a, _ := ctx.Value(transitionActorKey{}).(string)
	return a
}

type transitionCallerKey struct{}

// WithTransitionCaller names the registered operator caller of an RPC that
// another operator-facing component forwards to TransitionBacklogItemStatus.
// The resolver honors only CallerOperatorNotificationAction, for the user actor.
func WithTransitionCaller(ctx context.Context, caller TransitionCaller) context.Context {
	return context.WithValue(ctx, transitionCallerKey{}, caller)
}

// TransitionCallerFrom returns the caller set by WithTransitionCaller, or "" when none was set.
func TransitionCallerFrom(ctx context.Context) TransitionCaller {
	c, _ := ctx.Value(transitionCallerKey{}).(TransitionCaller)
	return c
}
