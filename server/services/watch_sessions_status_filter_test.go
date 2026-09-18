package services

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// TestWatchSessions_should_DeliverEvent_When_StatusFilterMatchesFailed is Epic
// 1.1 Story 1.1.3's validation test: a WatchSessions subscriber filtering on
// StatusFilter=[FAILED] must receive an event for an instance that transitions
// to session.Failed and is published via SessionUpdatedEvent — proving
// adapters.StatusToProto's new Failed arm (Task 1.1.3a) actually reaches the
// filter comparison WatchSessions makes on the live event path (Task 1.1.3b).
func TestWatchSessions_should_DeliverEvent_When_StatusFilterMatchesFailed(t *testing.T) {
	t.Parallel()
	svc, bus, srv := newNotificationTestServer(t)

	poller := session.NewReviewQueuePoller(session.NewReviewQueue(), session.NewInstanceStatusManager(), nil)
	svc.SetReviewQueuePoller(poller)

	inst := &session.Instance{
		Title:   "watch-failed-status-filter",
		UUID:    "eeeeeeee-0000-0000-0000-000000000001",
		Status:  session.Failed,
		Program: "claude",
		Path:    "/tmp/test",
	}
	poller.AddInstance(inst)

	client := newTestClient(srv)

	callCtx, callCancel := context.WithCancel(context.Background())
	defer callCancel()

	failedFilter := sessionv1.SessionStatus_SESSION_STATUS_FAILED
	streamErrCh := make(chan error, 1)
	recvCh := make(chan *sessionv1.SessionEvent, 8)
	streamDone := make(chan struct{})
	go func() {
		defer close(streamDone)
		stream, err := client.WatchSessions(callCtx, connect.NewRequest(&sessionv1.WatchSessionsRequest{
			StatusFilter: &failedFilter,
		}))
		streamErrCh <- err
		if err != nil {
			return
		}
		for stream.Receive() {
			recvCh <- stream.Msg()
		}
	}()
	t.Cleanup(func() {
		callCancel()
		select {
		case <-streamDone:
		case <-time.After(5 * time.Second):
			t.Error("WatchSessions goroutine did not exit after context cancellation")
		}
	})

	select {
	case err := <-streamErrCh:
		require.NoError(t, err, "expected WatchSessions to connect")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for WatchSessions to connect")
	}

	// Re-publish on a ticker until observed: EventBus.Publish is best-effort
	// fan-out to already-registered subscribers, and there's an inherent race
	// between WatchSessions subscribing server-side and this goroutine's first
	// publish (same pattern as the native-HTTP/2 WatchSessions test).
	publishCtx, stopPublishing := context.WithCancel(context.Background())
	defer stopPublishing()
	go func() {
		bus.Publish(events.NewSessionUpdatedEvent(inst, []string{"status"}))
		ticker := time.NewTicker(25 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-publishCtx.Done():
				return
			case <-ticker.C:
				bus.Publish(events.NewSessionUpdatedEvent(inst, []string{"status"}))
			}
		}
	}()

	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev := <-recvCh:
			updated := ev.GetSessionUpdated()
			if updated != nil && updated.GetSession().GetId() == inst.GetStableID() {
				assert.Equal(t, sessionv1.SessionStatus_SESSION_STATUS_FAILED, updated.GetSession().GetStatus())
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for a SessionUpdated event matching StatusFilter=FAILED for session %q", inst.GetStableID())
		}
	}
}
