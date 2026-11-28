package domain

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestTransitionHandler_ResolvesCallerClassFromContextActor(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		ctx  context.Context
		want TransitionCaller
		cls  CallerClass
	}{
		{"absent actor is the operator RPC", context.Background(), CallerOperatorTransitionRPC, ClassOperator},
		{"user actor is the operator RPC", WithTransitionActor(context.Background(), TriggeredByUser), CallerOperatorTransitionRPC, ClassOperator},
		{"agent actor is the MCP delegate", WithTransitionActor(context.Background(), TriggeredByAgent), CallerMCPTransitionDelegate, ClassAgent},
		{"unknown actor fails closed to the delegate", WithTransitionActor(context.Background(), "robot"), CallerMCPTransitionDelegate, ClassAgent},
		{"notification action rides the user actor", WithTransitionCaller(context.Background(), CallerOperatorNotificationAction), CallerOperatorNotificationAction, ClassOperator},
		{"a caller hint never lifts an agent", WithTransitionCaller(WithTransitionActor(context.Background(), TriggeredByAgent), CallerOperatorNotificationAction), CallerMCPTransitionDelegate, ClassAgent},
		{"only the notification caller is honored", WithTransitionCaller(context.Background(), CallerOperatorArchiveRPC), CallerOperatorTransitionRPC, ClassOperator},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveTransitionCaller(tc.ctx)
			assert.Equal(t, tc.want, got)
			spec, ok := LookupTransitionCaller(got)
			require.True(t, ok)
			assert.Equal(t, tc.cls, spec.Class)
		})
	}
}
