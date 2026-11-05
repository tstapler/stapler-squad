package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
)

// Story 1.5.5d: the operator RPC writers decide through the transition gate.

func TestTransitionHandler_ResolvesCallerClassFromContextActor(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		ctx  context.Context
		want session.TransitionCaller
		cls  session.CallerClass
	}{
		{"absent actor is the operator RPC", context.Background(), session.CallerOperatorTransitionRPC, session.ClassOperator},
		{"user actor is the operator RPC", WithTransitionActor(context.Background(), session.TriggeredByUser), session.CallerOperatorTransitionRPC, session.ClassOperator},
		{"agent actor is the MCP delegate", WithTransitionActor(context.Background(), session.TriggeredByAgent), session.CallerMCPTransitionDelegate, session.ClassAgent},
		{"unknown actor fails closed to the delegate", WithTransitionActor(context.Background(), "robot"), session.CallerMCPTransitionDelegate, session.ClassAgent},
		{"notification action rides the user actor", WithTransitionCaller(context.Background(), session.CallerOperatorNotificationAction), session.CallerOperatorNotificationAction, session.ClassOperator},
		{"a caller hint never lifts an agent", WithTransitionCaller(WithTransitionActor(context.Background(), session.TriggeredByAgent), session.CallerOperatorNotificationAction), session.CallerMCPTransitionDelegate, session.ClassAgent},
		{"only the notification caller is honored", WithTransitionCaller(context.Background(), session.CallerOperatorArchiveRPC), session.CallerOperatorTransitionRPC, session.ClassOperator},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveTransitionCaller(tc.ctx)
			assert.Equal(t, tc.want, got)
			spec, ok := session.LookupTransitionCaller(got)
			require.True(t, ok)
			assert.Equal(t, tc.cls, spec.Class)
		})
	}
}
