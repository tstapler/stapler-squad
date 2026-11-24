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
