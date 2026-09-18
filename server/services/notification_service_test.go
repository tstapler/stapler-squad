package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// newNotificationTestServer creates an httptest server running a real SessionService
// so that SendNotification requests arrive with a valid localhost peer address
// (required by the validateLocalhostOrigin check).
func newNotificationTestServer(t *testing.T) (*SessionService, *events.EventBus, *httptest.Server) {
	t.Helper()
	storage := createTestStorage(t)
	bus := events.NewEventBus(32)
	t.Cleanup(bus.Close)
	svc := NewSessionService(storage, bus)
	t.Cleanup(func() { svc.Shutdown() })

	mux := http.NewServeMux()
	path, handler := sessionv1connect.NewSessionServiceHandler(svc)
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return svc, bus, srv
}

// newTestClient creates a ConnectRPC client pointed at the test server.
func newTestClient(srv *httptest.Server) sessionv1connect.SessionServiceClient {
	return sessionv1connect.NewSessionServiceClient(srv.Client(), srv.URL)
}

// TestSendNotification_ResolvesSessionTitleToStableID verifies that when a
// notification arrives with a session identifier that matches a known session's
// title (the value hooks send), the published event carries the session's UUID
// — not the raw title — so the web client can match it.
func TestSendNotification_ResolvesSessionTitleToStableID(t *testing.T) {
	t.Parallel()
	svc, bus, srv := newNotificationTestServer(t)

	// Wire a ReviewQueuePoller containing an instance whose title differs from its UUID.
	queue := session.NewReviewQueue()
	statusMgr := session.NewInstanceStatusManager()
	poller := session.NewReviewQueuePoller(queue, statusMgr, nil)
	svc.SetReviewQueuePoller(poller)

	const sessionTitle = "stelekit"
	const sessionUUID = "aaaabbbb-cccc-dddd-eeee-ffffffffffff"
	inst := &session.Instance{
		Title: sessionTitle,
		UUID:  sessionUUID,
	}
	poller.SetInstances([]*session.Instance{inst})

	// Subscribe to the event bus before sending.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	eventCh, _ := bus.Subscribe(ctx)

	client := newTestClient(srv)
	_, err := client.SendNotification(ctx, connect.NewRequest(&sessionv1.SendNotificationRequest{
		SessionId:        sessionTitle, // hook sends the title, not the UUID
		Title:            "Claude needs attention",
		NotificationType: sessionv1.NotificationType_NOTIFICATION_TYPE_INFO,
		Priority:         sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM,
	}))
	require.NoError(t, err)

	// Drain the event bus until we find the notification event.
	var gotEvent *events.Event
	timeout := time.After(2 * time.Second)
	for gotEvent == nil {
		select {
		case e := <-eventCh:
			if e.Type == events.EventNotification {
				gotEvent = e
			}
		case <-timeout:
			t.Fatal("timed out waiting for notification event on event bus")
		}
	}

	if gotEvent.SessionID != sessionUUID {
		t.Errorf("event.SessionID = %q, want UUID %q (session title was %q)",
			gotEvent.SessionID, sessionUUID, sessionTitle)
	}
}

// TestSendNotification_PollerMissFallsBackToStorage verifies that when the
// in-memory ReviewQueuePoller hasn't (yet) been seeded with a session — e.g. shortly
// after a server restart, before the startup instance-load completes — SendNotification
// still resolves the hook-supplied title to the session's stable UUID via durable
// storage, instead of permanently recording the raw title. Reproduces the bug where
// "View Session" in the web UI wrongly concluded a genuinely live session was gone,
// because its notification's session_id never got past the title.
func TestSendNotification_PollerMissFallsBackToStorage(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	bus := events.NewEventBus(32)
	t.Cleanup(bus.Close)
	svc := NewSessionServiceWithSearchEngine(storage, bus, nil)
	t.Cleanup(func() { svc.Shutdown() })

	mux := http.NewServeMux()
	path, handler := sessionv1connect.NewSessionServiceHandler(svc)
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// Wire an empty poller (simulates the poller not having this session yet) — this
	// is also what wires storage into notificationSvc, via SetReviewQueuePoller.
	queue := session.NewReviewQueue()
	statusMgr := session.NewInstanceStatusManager()
	poller := session.NewReviewQueuePoller(queue, statusMgr, nil)
	svc.SetReviewQueuePoller(poller)

	const sessionTitle = "stapler-squad-perf"
	const sessionUUID = "14c67dc3-a950-4863-b01a-e4abe58045d1"
	require.NoError(t, storage.AddInstance(&session.Instance{
		Title:     sessionTitle,
		UUID:      sessionUUID,
		Path:      t.TempDir(),
		Status:    session.Paused,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	eventCh, _ := bus.Subscribe(ctx)

	client := newTestClient(srv)
	_, err := client.SendNotification(ctx, connect.NewRequest(&sessionv1.SendNotificationRequest{
		SessionId:        sessionTitle, // hook sends the title; poller doesn't have it yet
		Title:            "Claude is waiting for your input",
		NotificationType: sessionv1.NotificationType_NOTIFICATION_TYPE_APPROVAL_NEEDED,
		Priority:         sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM,
	}))
	require.NoError(t, err)

	var gotEvent *events.Event
	timeout := time.After(2 * time.Second)
	for gotEvent == nil {
		select {
		case e := <-eventCh:
			if e.Type == events.EventNotification {
				gotEvent = e
			}
		case <-timeout:
			t.Fatal("timed out waiting for notification event on event bus")
		}
	}

	if gotEvent.SessionID != sessionUUID {
		t.Errorf("event.SessionID = %q, want UUID %q (session title was %q, poller miss, storage fallback)",
			gotEvent.SessionID, sessionUUID, sessionTitle)
	}
}

// TestSendNotification_UnknownSessionUsesRawID verifies that when no session
// matches the incoming ID, the raw value is used as-is (graceful fallback).
func TestSendNotification_UnknownSessionUsesRawID(t *testing.T) {
	t.Parallel()
	svc, bus, srv := newNotificationTestServer(t)

	// No poller / no instances — nothing to resolve against.
	_ = svc

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	eventCh, _ := bus.Subscribe(ctx)

	client := newTestClient(srv)
	_, err := client.SendNotification(ctx, connect.NewRequest(&sessionv1.SendNotificationRequest{
		SessionId:        "unknown-session",
		Title:            "some notification",
		NotificationType: sessionv1.NotificationType_NOTIFICATION_TYPE_INFO,
		Priority:         sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_LOW,
	}))
	require.NoError(t, err)

	var gotEvent *events.Event
	timeout := time.After(2 * time.Second)
	for gotEvent == nil {
		select {
		case e := <-eventCh:
			if e.Type == events.EventNotification {
				gotEvent = e
			}
		case <-timeout:
			t.Fatal("timed out waiting for notification event")
		}
	}

	if gotEvent.SessionID != "unknown-session" {
		t.Errorf("event.SessionID = %q, want raw %q", gotEvent.SessionID, "unknown-session")
	}
}

func TestValidateLocalhostOrigin(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		peerAddr      string
		expectedError bool
	}{
		{
			name:          "Localhost IPv4",
			peerAddr:      "127.0.0.1:12345",
			expectedError: false,
		},
		{
			name:          "External IP",
			peerAddr:      "192.168.1.1:12345",
			expectedError: true,
		},
		{
			name:          "IPv6 Localhost",
			peerAddr:      "[::1]:12345",
			expectedError: false,
		},
		{
			name:          "Missing IP",
			peerAddr:      "",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateLocalhostAddr(tt.peerAddr)
			if (err != nil) != tt.expectedError {
				t.Errorf("validateLocalhostAddr(%q) error = %v, expectedError %v", tt.peerAddr, err, tt.expectedError)
			}
		})
	}
}
