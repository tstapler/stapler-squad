package services

// Delegators for the terminal and event stream RPCs (server/services/sessionstream)
// and the review queue and log RPCs that sat beside them.

import (
	"context"

	"connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/federation"
	"github.com/tstapler/stapler-squad/server/services/sessionstream"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

// streamHandler is built per call.
func (s *SessionService) streamHandler() *sessionstream.Handler {
	return sessionstream.New(s.eventBus, s.streamPorts())
}

// streamPorts reads the service's fields when a port is called, so setters that
// run after construction are seen.
func (s *SessionService) streamPorts() sessionstream.Ports {
	return sessionstream.Ports{
		FindPolled: func(id string) *instance.Instance {
			if s.ReviewQueuePoller == nil {
				return nil
			}
			return s.ReviewQueuePoller.FindInstance(id)
		},
		Polled: func() ([]*instance.Instance, bool) {
			if s.ReviewQueuePoller == nil {
				return nil, false
			}
			return s.ReviewQueuePoller.GetInstances(), true
		},
		LoadWired:        s.loadInstancesWithWiring,
		FindInstance:     s.findInstance,
		ListInstanceData: func() ([]domain.InstanceData, error) { return s.storage.ListInstanceData() },
		StatusManager:    func() *instance.InstanceStatusManager { return s.statusManager },
		Federation:       func() *federation.FederatedSessions { return s.federation },
	}
}

// StreamTerminal provides bidirectional terminal I/O for native Connect/gRPC clients.
// Browser clients use the WebSocket handler instead.
func (s *SessionService) StreamTerminal(ctx context.Context, stream *connect.BidiStream[sessionv1.TerminalData, sessionv1.TerminalData]) error {
	return s.streamHandler().StreamTerminal(ctx, stream)
}

// WatchSessions streams real-time session events.
func (s *SessionService) WatchSessions(ctx context.Context, req *connect.Request[sessionv1.WatchSessionsRequest], stream *connect.ServerStream[sessionv1.SessionEvent]) error {
	return s.streamHandler().WatchSessions(ctx, req, stream)
}

// GetSessionDiff retrieves the current git diff for a session.
func (s *SessionService) GetSessionDiff(ctx context.Context, req *connect.Request[sessionv1.GetSessionDiffRequest]) (*connect.Response[sessionv1.GetSessionDiffResponse], error) {
	return s.streamHandler().GetSessionDiff(ctx, req)
}

// GetDetectionEvents returns a session's recent status-detection events.
func (s *SessionService) GetDetectionEvents(ctx context.Context, req *connect.Request[sessionv1.GetDetectionEventsRequest]) (*connect.Response[sessionv1.GetDetectionEventsResponse], error) {
	return s.streamHandler().GetDetectionEvents(ctx, req)
}

// GetReviewQueue returns sessions needing user attention with priority ordering.
func (s *SessionService) GetReviewQueue(
	ctx context.Context,
	req *connect.Request[sessionv1.GetReviewQueueRequest],
) (*connect.Response[sessionv1.GetReviewQueueResponse], error) {
	return s.reviewQueueSvc.GetReviewQueue(ctx, req)
}

// AcknowledgeSession marks a session as acknowledged in the review queue.
// The session won't reappear in the queue until it receives an update.
func (s *SessionService) AcknowledgeSession(
	ctx context.Context,
	req *connect.Request[sessionv1.AcknowledgeSessionRequest],
) (*connect.Response[sessionv1.AcknowledgeSessionResponse], error) {
	return s.reviewQueueSvc.AcknowledgeSession(ctx, req)
}

// GetLogs retrieves application logs with optional filtering and search.
func (s *SessionService) GetLogs(
	ctx context.Context,
	req *connect.Request[sessionv1.GetLogsRequest],
) (*connect.Response[sessionv1.GetLogsResponse], error) {
	return s.debugLogs.GetLogs(ctx, req)
}

// WatchReviewQueue streams real-time review queue events.
func (s *SessionService) WatchReviewQueue(
	ctx context.Context,
	req *connect.Request[sessionv1.WatchReviewQueueRequest],
	stream *connect.ServerStream[sessionv1.ReviewQueueEvent],
) error {
	return s.reviewQueueSvc.WatchReviewQueue(ctx, req, stream)
}
