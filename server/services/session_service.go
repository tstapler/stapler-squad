package services

import (
	"claude-squad/config"
	sessionv1 "claude-squad/gen/proto/go/session/v1"
	"claude-squad/server/adapters"
	"claude-squad/session"
	"connectrpc.com/connect"
	"context"
	"fmt"
)

// SessionService implements the SessionServiceHandler interface for ConnectRPC.
type SessionService struct {
	storage *session.Storage
}

// NewSessionService creates a new SessionService with the given storage.
func NewSessionService(storage *session.Storage) *SessionService {
	return &SessionService{
		storage: storage,
	}
}

// NewSessionServiceFromConfig creates a SessionService using the default config and state.
func NewSessionServiceFromConfig() (*SessionService, error) {
	state := config.LoadState()
	storage, err := session.NewStorage(state)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}
	return NewSessionService(storage), nil
}

// ListSessions returns all sessions with optional filtering.
func (s *SessionService) ListSessions(
	ctx context.Context,
	req *connect.Request[sessionv1.ListSessionsRequest],
) (*connect.Response[sessionv1.ListSessionsResponse], error) {
	instances, err := s.storage.LoadInstances()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
	}

	// Convert instances to proto messages
	sessions := make([]*sessionv1.Session, 0, len(instances))
	for _, inst := range instances {
		// Apply optional status filter
		if req.Msg.Status != nil && *req.Msg.Status != sessionv1.SessionStatus_SESSION_STATUS_UNSPECIFIED {
			protoStatus := adapters.InstanceToProto(inst).Status
			if protoStatus != *req.Msg.Status {
				continue
			}
		}

		// Apply optional category filter
		if req.Msg.Category != nil && *req.Msg.Category != "" && inst.Category != *req.Msg.Category {
			continue
		}

		sessions = append(sessions, adapters.InstanceToProto(inst))
	}

	return connect.NewResponse(&sessionv1.ListSessionsResponse{
		Sessions: sessions,
	}), nil
}

// GetSession retrieves a specific session by ID (Title).
func (s *SessionService) GetSession(
	ctx context.Context,
	req *connect.Request[sessionv1.GetSessionRequest],
) (*connect.Response[sessionv1.GetSessionResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	instances, err := s.storage.LoadInstances()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
	}

	// Find instance by ID (using Title as ID)
	for _, inst := range instances {
		if inst.Title == req.Msg.Id {
			return connect.NewResponse(&sessionv1.GetSessionResponse{
				Session: adapters.InstanceToProto(inst),
			}), nil
		}
	}

	return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.Id))
}

// CreateSession initializes a new AI agent session with tmux and git worktree.
// TODO: Implement in next task (Task 1.6)
func (s *SessionService) CreateSession(
	ctx context.Context,
	req *connect.Request[sessionv1.CreateSessionRequest],
) (*connect.Response[sessionv1.CreateSessionResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("CreateSession not yet implemented"))
}

// UpdateSession modifies session properties (pause/resume, category, etc).
// TODO: Implement in next task (Task 1.7)
func (s *SessionService) UpdateSession(
	ctx context.Context,
	req *connect.Request[sessionv1.UpdateSessionRequest],
) (*connect.Response[sessionv1.UpdateSessionResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("UpdateSession not yet implemented"))
}

// DeleteSession stops and removes a session, cleaning up resources.
// TODO: Implement in next task (Task 1.7)
func (s *SessionService) DeleteSession(
	ctx context.Context,
	req *connect.Request[sessionv1.DeleteSessionRequest],
) (*connect.Response[sessionv1.DeleteSessionResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("DeleteSession not yet implemented"))
}

// WatchSessions streams real-time session events (created/updated/deleted).
// TODO: Implement in Task 2.2
func (s *SessionService) WatchSessions(
	ctx context.Context,
	req *connect.Request[sessionv1.WatchSessionsRequest],
	stream *connect.ServerStream[sessionv1.WatchSessionsResponse],
) error {
	return connect.NewError(connect.CodeUnimplemented, fmt.Errorf("WatchSessions not yet implemented"))
}

// StreamTerminal provides bidirectional streaming for terminal I/O.
// TODO: Implement in Task 2.3
func (s *SessionService) StreamTerminal(
	ctx context.Context,
	stream *connect.BidiStream[sessionv1.StreamTerminalRequest, sessionv1.StreamTerminalResponse],
) error {
	return connect.NewError(connect.CodeUnimplemented, fmt.Errorf("StreamTerminal not yet implemented"))
}

// GetSessionDiff retrieves the current git diff for a session.
// TODO: Implement in Task 4.3
func (s *SessionService) GetSessionDiff(
	ctx context.Context,
	req *connect.Request[sessionv1.GetSessionDiffRequest],
) (*connect.Response[sessionv1.GetSessionDiffResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("GetSessionDiff not yet implemented"))
}
