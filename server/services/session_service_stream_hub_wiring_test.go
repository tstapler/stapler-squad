package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/queue"
)

// A nil lookup wired into the rollout service would silently drop a session's custom tmux prefix.
func TestSessionService_should_KeyStreamHubOverrideByLiveInstancePrefix_When_SessionHasCustomPrefix(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	bus := events.NewEventBus(16)
	t.Cleanup(bus.Close)
	svc := NewSessionService(svctest.CreateTestStorage(t), bus)
	t.Cleanup(func() { svc.Shutdown() })
	poller := session.NewReviewQueuePoller(queue.NewReviewQueue(), instance.NewInstanceStatusManager(), nil)
	svc.SetReviewQueuePoller(poller)
	poller.AddInstance(&instance.Instance{Title: "wired", TmuxPrefix: "custom_"})

	force := true
	resp, err := svc.SetStreamHubSessionOverride(context.Background(), connect.NewRequest(&sessionv1.SetStreamHubSessionOverrideRequest{
		SessionName: "wired",
		ForceHub:    &force,
	}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.SessionOverrides, 1)
	assert.Equal(t, "custom_wired", resp.Msg.SessionOverrides[0].SessionName)
}
