package domain

import (
	"context"
	"testing"
)

func TestTransitionActorFrom_should_ReturnTheActorSetOnTheContext(t *testing.T) {
	t.Parallel()
	if got := TransitionActorFrom(context.Background()); got != "" {
		t.Errorf("bare context: got %q, want empty", got)
	}
	ctx := WithTransitionActor(context.Background(), "agent")
	if got := TransitionActorFrom(ctx); got != "agent" {
		t.Errorf("got %q, want agent", got)
	}
	if got := TransitionActorFrom(WithTransitionActor(ctx, "user")); got != "user" {
		t.Errorf("inner value must win: got %q", got)
	}
}
