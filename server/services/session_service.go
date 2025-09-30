package services

import (
	"claude-squad/config"
	sessionv1 "claude-squad/gen/proto/go/session/v1"
	"claude-squad/server/adapters"
	"claude-squad/server/events"
	"claude-squad/session"
	"connectrpc.com/connect"
	"context"
	"fmt"
)

// SessionService implements the SessionServiceHandler interface for ConnectRPC.
type SessionService struct {
	storage  *session.Storage
	eventBus *events.EventBus
}

// NewSessionService creates a new SessionService with the given storage and event bus.
func NewSessionService(storage *session.Storage, eventBus *events.EventBus) *SessionService {
	return &SessionService{
		storage:  storage,
		eventBus: eventBus,
	}
}

// NewSessionServiceFromConfig creates a SessionService using the default config and state.
func NewSessionServiceFromConfig() (*SessionService, error) {
	state := config.LoadState()
	storage, err := session.NewStorage(state)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}
	eventBus := events.NewEventBus(100) // Buffer 100 events per subscriber
	return NewSessionService(storage, eventBus), nil
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
func (s *SessionService) CreateSession(
	ctx context.Context,
	req *connect.Request[sessionv1.CreateSessionRequest],
) (*connect.Response[sessionv1.CreateSessionResponse], error) {
	// Validate required fields
	if req.Msg.Title == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("title is required"))
	}
	if req.Msg.Path == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("path is required"))
	}

	// Check if session with this title already exists
	instances, err := s.storage.LoadInstances()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
	}
	for _, inst := range instances {
		if inst.Title == req.Msg.Title {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("session with title '%s' already exists", req.Msg.Title))
		}
	}

	// Set default program if not specified
	program := req.Msg.Program
	if program == "" {
		cfg := config.LoadConfig()
		program = cfg.DefaultProgram
	}

	// Determine session type based on ExistingWorktree field
	sessionType := session.SessionTypeDirectory
	if req.Msg.ExistingWorktree != "" {
		sessionType = session.SessionTypeExistingWorktree
	} else if req.Msg.Branch != "" {
		// If branch is specified, create a new worktree
		sessionType = session.SessionTypeNewWorktree
	}

	// Create instance using NewInstance constructor
	instance, err := session.NewInstance(session.InstanceOptions{
		Title:            req.Msg.Title,
		Path:             req.Msg.Path,
		WorkingDir:       req.Msg.WorkingDir,
		Program:          program,
		AutoYes:          req.Msg.AutoYes,
		Prompt:           req.Msg.Prompt,
		ExistingWorktree: req.Msg.ExistingWorktree,
		Category:         req.Msg.Category,
		SessionType:      sessionType,
		TmuxPrefix:       "", // Use default from config
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("failed to create instance: %w", err))
	}

	// Start the session (initializes tmux + git worktree)
	// Use Start(true) to indicate this is a first-time setup
	if err := instance.Start(true); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to start session: %w", err))
	}

	// Save instance to storage
	// Note: Storage uses SaveInstances (plural) which saves all instances
	// We need to load, append, and save all instances
	if err := s.storage.SaveInstances(append(instances, instance)); err != nil {
		// Cleanup on save failure
		if destroyErr := instance.Destroy(); destroyErr != nil {
			// Log cleanup error but return original save error
			fmt.Printf("Failed to cleanup after save error: %v\n", destroyErr)
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save instance: %w", err))
	}

	// Publish SessionCreated event to all watchers
	s.eventBus.Publish(events.NewSessionCreatedEvent(instance))

	return connect.NewResponse(&sessionv1.CreateSessionResponse{
		Session: adapters.InstanceToProto(instance),
	}), nil
}

// UpdateSession modifies session properties (pause/resume, category, title).
func (s *SessionService) UpdateSession(
	ctx context.Context,
	req *connect.Request[sessionv1.UpdateSessionRequest],
) (*connect.Response[sessionv1.UpdateSessionResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	instances, err := s.storage.LoadInstances()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
	}

	// Find the instance to update
	var instance *session.Instance
	var instanceIndex int
	for i, inst := range instances {
		if inst.Title == req.Msg.Id {
			instance = inst
			instanceIndex = i
			break
		}
	}

	if instance == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.Id))
	}

	// Track which fields are being updated for event publishing
	var updatedFields []string
	var oldStatus session.Status

	// Handle status change (pause/resume)
	if req.Msg.Status != nil && *req.Msg.Status != sessionv1.SessionStatus_SESSION_STATUS_UNSPECIFIED {
		targetStatus := adapters.ProtoToStatus(*req.Msg.Status)
		oldStatus = instance.Status

		if targetStatus == session.Paused && instance.Status != session.Paused {
			if err := instance.Pause(); err != nil {
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to pause session: %w", err))
			}
			updatedFields = append(updatedFields, "status")
		} else if targetStatus != session.Paused && instance.Status == session.Paused {
			// Resume from paused state
			if err := instance.Resume(); err != nil {
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to resume session: %w", err))
			}
			updatedFields = append(updatedFields, "status")
		}
	}

	// Handle category update
	if req.Msg.Category != nil {
		instance.Category = *req.Msg.Category
		updatedFields = append(updatedFields, "category")
	}

	// Handle title update
	if req.Msg.Title != nil && *req.Msg.Title != "" && *req.Msg.Title != instance.Title {
		// Check if new title already exists
		for _, inst := range instances {
			if inst.Title == *req.Msg.Title {
				return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("session with title '%s' already exists", *req.Msg.Title))
			}
		}
		instance.Title = *req.Msg.Title
		updatedFields = append(updatedFields, "title")
	}

	// Update the instance in the list and save
	instances[instanceIndex] = instance
	if err := s.storage.SaveInstances(instances); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save instance: %w", err))
	}

	// Publish events based on what was updated
	if len(updatedFields) > 0 {
		// Check if status changed specifically
		if oldStatus != instance.Status && oldStatus != 0 {
			s.eventBus.Publish(events.NewSessionStatusChangedEvent(instance, oldStatus, instance.Status))
		}
		// Also publish general update event
		s.eventBus.Publish(events.NewSessionUpdatedEvent(instance, updatedFields))
	}

	return connect.NewResponse(&sessionv1.UpdateSessionResponse{
		Session: adapters.InstanceToProto(instance),
	}), nil
}

// DeleteSession stops and removes a session, cleaning up resources.
func (s *SessionService) DeleteSession(
	ctx context.Context,
	req *connect.Request[sessionv1.DeleteSessionRequest],
) (*connect.Response[sessionv1.DeleteSessionResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	instances, err := s.storage.LoadInstances()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
	}

	// Find the instance to delete
	var instance *session.Instance
	for _, inst := range instances {
		if inst.Title == req.Msg.Id {
			instance = inst
			break
		}
	}

	if instance == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.Id))
	}

	// Destroy the session (cleanup tmux + git worktree)
	if err := instance.Destroy(); err != nil {
		// Log error but continue with deletion from storage
		fmt.Printf("Warning: failed to cleanup session resources: %v\n", err)
	}

	// Delete from storage
	if err := s.storage.DeleteInstance(req.Msg.Id); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete instance from storage: %w", err))
	}

	// Publish SessionDeleted event to all watchers
	s.eventBus.Publish(events.NewSessionDeletedEvent(req.Msg.Id))

	return connect.NewResponse(&sessionv1.DeleteSessionResponse{
		Success: true,
		Message: fmt.Sprintf("Session '%s' deleted successfully", req.Msg.Id),
	}), nil
}

// WatchSessions streams real-time session events (created/updated/deleted).
// Sends initial snapshot of all sessions, then subscribes to real-time updates.
func (s *SessionService) WatchSessions(
	ctx context.Context,
	req *connect.Request[sessionv1.WatchSessionsRequest],
	stream *connect.ServerStream[sessionv1.SessionEvent],
) error {
	// Send initial snapshot of all sessions
	instances, err := s.storage.LoadInstances()
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
	}

	// Apply optional filters from request
	for _, inst := range instances {
		// Filter by category if specified
		if req.Msg.CategoryFilter != nil && *req.Msg.CategoryFilter != "" {
			if inst.Category != *req.Msg.CategoryFilter {
				continue
			}
		}

		// Filter by status if specified
		if req.Msg.StatusFilter != nil && *req.Msg.StatusFilter != sessionv1.SessionStatus_SESSION_STATUS_UNSPECIFIED {
			if adapters.StatusToProto(inst.Status) != *req.Msg.StatusFilter {
				continue
			}
		}

		// Send as SessionCreated event for initial snapshot
		event := createInitialSnapshotEvent(inst)
		if err := stream.Send(event); err != nil {
			return fmt.Errorf("failed to send initial snapshot: %w", err)
		}
	}

	// Subscribe to real-time events from event bus
	eventCh, subID := s.eventBus.Subscribe(ctx)
	defer s.eventBus.Unsubscribe(subID)

	// Stream events until client disconnects or context is canceled
	for {
		select {
		case <-ctx.Done():
			// Client disconnected or context canceled
			return nil
		case event, ok := <-eventCh:
			if !ok {
				// Event channel closed (should not happen with proper cleanup)
				return nil
			}

			// Apply filters to real-time events
			if req.Msg.CategoryFilter != nil && *req.Msg.CategoryFilter != "" {
				if event.Session != nil && event.Session.Category != *req.Msg.CategoryFilter {
					continue
				}
			}

			if req.Msg.StatusFilter != nil && *req.Msg.StatusFilter != sessionv1.SessionStatus_SESSION_STATUS_UNSPECIFIED {
				if event.Session != nil && adapters.StatusToProto(event.Session.Status) != *req.Msg.StatusFilter {
					continue
				}
			}

			// Convert internal event to protobuf and send
			protoEvent := convertEventToProto(event)
			if err := stream.Send(protoEvent); err != nil {
				return fmt.Errorf("failed to send event: %w", err)
			}
		}
	}
}

// StreamTerminal provides bidirectional streaming for terminal I/O.
// TODO: Implement in Task 2.3
func (s *SessionService) StreamTerminal(
	ctx context.Context,
	stream *connect.BidiStream[sessionv1.TerminalData, sessionv1.TerminalData],
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
