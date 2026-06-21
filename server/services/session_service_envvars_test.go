package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// TestCreateSession_ThreadsEnvVars_WhenSetInRequest verifies that env_vars in the
// CreateSessionRequest are propagated to the live instance's EnvVars field.
func TestCreateSession_ThreadsEnvVars_WhenSetInRequest(t *testing.T) {
	storage := createTestStorage(t)
	bus := events.NewEventBus(16)
	t.Cleanup(bus.Close)
	svc := NewSessionService(storage, bus)

	// Wire poller so FindLiveInstance works immediately after CreateSession.
	queue := session.NewReviewQueue()
	statusMgr := session.NewInstanceStatusManager()
	poller := session.NewReviewQueuePoller(queue, statusMgr, nil)
	svc.SetReviewQueuePoller(poller)

	resp, err := svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:   "env-test-session",
		Path:    t.TempDir(),
		Program: "claude",
		EnvVars: map[string]string{"FOO": "bar", "BAZ": "qux"},
	}))

	if err != nil {
		// A pre-goroutine validation error is unexpected here; the path exists and title is set.
		// If it fails it's CodeInternal (e.g. storage issue).
		t.Fatalf("unexpected CreateSession error: %v", err)
	}
	t.Cleanup(func() { destroyCreatedSession(t, svc, resp.Msg.Session.Id) })

	// CreateSession adds the instance to the live poller synchronously before returning.
	inst := svc.FindLiveInstance(resp.Msg.Session.Id)
	require.NotNil(t, inst, "instance should be in the live poller immediately after CreateSession")

	require.Equal(t, "bar", inst.EnvVars["FOO"], "EnvVars[FOO] should be 'bar'")
	require.Equal(t, "qux", inst.EnvVars["BAZ"], "EnvVars[BAZ] should be 'qux'")
}
