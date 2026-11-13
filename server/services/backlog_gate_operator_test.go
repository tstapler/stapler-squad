package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// Story 1.5.5d: the operator RPC writers decide through the transition gate.

func TestTransitionHandler_ResolvesCallerClassFromContextActor(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		ctx  context.Context
		want domain.TransitionCaller
		cls  domain.CallerClass
	}{
		{"absent actor is the operator RPC", context.Background(), domain.CallerOperatorTransitionRPC, domain.ClassOperator},
		{"user actor is the operator RPC", WithTransitionActor(context.Background(), session.TriggeredByUser), domain.CallerOperatorTransitionRPC, domain.ClassOperator},
		{"agent actor is the MCP delegate", WithTransitionActor(context.Background(), session.TriggeredByAgent), domain.CallerMCPTransitionDelegate, domain.ClassAgent},
		{"unknown actor fails closed to the delegate", WithTransitionActor(context.Background(), "robot"), domain.CallerMCPTransitionDelegate, domain.ClassAgent},
		{"notification action rides the user actor", WithTransitionCaller(context.Background(), domain.CallerOperatorNotificationAction), domain.CallerOperatorNotificationAction, domain.ClassOperator},
		{"a caller hint never lifts an agent", WithTransitionCaller(WithTransitionActor(context.Background(), session.TriggeredByAgent), domain.CallerOperatorNotificationAction), domain.CallerMCPTransitionDelegate, domain.ClassAgent},
		{"only the notification caller is honored", WithTransitionCaller(context.Background(), domain.CallerOperatorArchiveRPC), domain.CallerOperatorTransitionRPC, domain.ClassOperator},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveTransitionCaller(tc.ctx)
			assert.Equal(t, tc.want, got)
			spec, ok := domain.LookupTransitionCaller(got)
			require.True(t, ok)
			assert.Equal(t, tc.cls, spec.Class)
		})
	}
}
