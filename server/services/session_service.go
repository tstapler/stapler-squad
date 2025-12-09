package services

import (
	"bufio"
	"claude-squad/config"
	sessionv1 "claude-squad/gen/proto/go/session/v1"
	"claude-squad/log"
	"claude-squad/server/adapters"
	"claude-squad/server/events"
	"claude-squad/session"
	"connectrpc.com/connect"
	"context"
	"fmt"
	"google.golang.org/protobuf/types/known/timestamppb"
	"io"
	"net"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ReactiveQueueManager is an interface to avoid circular dependencies.
// The actual implementation is in server/review_queue_manager.go
type ReactiveQueueManager interface {
	AddStreamClient(ctx context.Context, filters interface{}) (<-chan *sessionv1.ReviewQueueEvent, string)
	RemoveStreamClient(clientID string)
}

// SessionService implements the SessionServiceHandler interface for ConnectRPC.
type SessionService struct {
	storage            *session.Storage
	eventBus           *events.EventBus
	reviewQueue        *session.ReviewQueue
	statusManager      *session.InstanceStatusManager
	reviewQueuePoller  *session.ReviewQueuePoller
	reactiveQueueMgr   ReactiveQueueManager

	// History cache
	historyCache      *session.ClaudeSessionHistory
	historyCacheTime  time.Time
	historyCacheTTL   time.Duration

	// Notification rate limiter (10 notifications/sec per session, burst of 20)
	notificationRateLimiter *NotificationRateLimiter
}

// NewSessionService creates a new SessionService with the given storage and event bus.
// NOTE: Instances are NOT loaded here to prevent double-loading and initialization timing issues.
// Instances will be loaded in server.go after dependencies (statusManager, reviewQueue) are wired.
func NewSessionService(storage *session.Storage, eventBus *events.EventBus) *SessionService {
	reviewQueue := session.NewReviewQueue()

	return &SessionService{
		storage:                 storage,
		eventBus:                eventBus,
		reviewQueue:             reviewQueue,
		historyCacheTTL:         5 * time.Minute, // Cache history for 5 minutes
		notificationRateLimiter: NewNotificationRateLimiter(10, 20), // 10/sec, burst of 20
	}
}

// loadInstancesWithWiring loads instances from storage and wires up dependencies.
// This ensures instances have reviewQueue and statusManager set properly.
func (s *SessionService) loadInstancesWithWiring() ([]*session.Instance, error) {
	instances, err := s.storage.LoadInstances()
	if err != nil {
		return nil, err
	}

	// Wire up dependencies on loaded instances
	for _, inst := range instances {
		inst.SetReviewQueue(s.reviewQueue)
		if s.statusManager != nil {
			inst.SetStatusManager(s.statusManager)
		}
	}

	return instances, nil
}

// NewSessionServiceFromConfig creates a SessionService using the default config and SQLite state.
func NewSessionServiceFromConfig() (*SessionService, error) {
	// Use SQLite-backed state for better performance and reliability
	sqliteState, err := session.LoadSQLiteState()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize SQLite state: %w", err)
	}
	storage, err := session.NewStorage(sqliteState)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}
	eventBus := events.NewEventBus(100) // Buffer 100 events per subscriber
	return NewSessionService(storage, eventBus), nil
}

// GetStorage returns the storage instance for direct access (e.g., WebSocket handlers).
func (s *SessionService) GetStorage() *session.Storage {
	return s.storage
}

// GetEventBus returns the event bus instance for wiring up reactive components.
func (s *SessionService) GetEventBus() *events.EventBus {
	return s.eventBus
}

// GetReviewQueueInstance returns the review queue instance for wiring up reactive components.
func (s *SessionService) GetReviewQueueInstance() *session.ReviewQueue {
	return s.reviewQueue
}

// SetReactiveQueueManager sets the ReactiveQueueManager (dependency injection).
// This must be called before WatchReviewQueue is used.
func (s *SessionService) SetReactiveQueueManager(mgr ReactiveQueueManager) {
	s.reactiveQueueMgr = mgr
}

// ListSessions returns all sessions with optional filtering.
func (s *SessionService) ListSessions(
	ctx context.Context,
	req *connect.Request[sessionv1.ListSessionsRequest],
) (*connect.Response[sessionv1.ListSessionsResponse], error) {
	instances, err := s.loadInstancesWithWiring()
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

	instances, err := s.loadInstancesWithWiring()
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
	updatedInstances := append(instances, instance)
	if err := s.storage.SaveInstances(updatedInstances); err != nil {
		// Cleanup on save failure
		if destroyErr := instance.Destroy(); destroyErr != nil {
			// Log cleanup error but return original save error
			log.ErrorLog.Printf("Failed to cleanup after save error: %v", destroyErr)
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save instance: %w", err))
	}

	// CRITICAL: Update the ReviewQueuePoller's instance references after creating new session
	if s.reviewQueuePoller != nil {
		s.reviewQueuePoller.SetInstances(updatedInstances)
		log.InfoLog.Printf("[ReviewQueue] Updated poller instance references after CreateSession for '%s'", instance.Title)
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

	// CRITICAL: Update the ReviewQueuePoller's instance references after updating session
	if s.reviewQueuePoller != nil {
		s.reviewQueuePoller.SetInstances(instances)
		log.InfoLog.Printf("[ReviewQueue] Updated poller instance references after UpdateSession for '%s'", instance.Title)
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
		log.WarningLog.Printf("Failed to cleanup session resources: %v", err)
	}

	// Delete from storage
	if err := s.storage.DeleteInstance(req.Msg.Id); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete instance from storage: %w", err))
	}

	// CRITICAL: Update the ReviewQueuePoller's instance references after deletion
	// The poller still has references to the old instances list which includes the deleted session.
	// Reload the instances and update the poller to prevent stale references.
	if s.reviewQueuePoller != nil {
		updatedInstances, err := s.storage.LoadInstances()
		if err != nil {
			log.ErrorLog.Printf("[ReviewQueue] Failed to reload instances after DeleteSession: %v", err)
		} else {
			s.reviewQueuePoller.SetInstances(updatedInstances)
			log.InfoLog.Printf("[ReviewQueue] Updated poller instance references after DeleteSession for '%s'", req.Msg.Id)
		}
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
	instances, err := s.loadInstancesWithWiring()
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

// StreamTerminal provides bidirectional streaming for terminal I/O with delta compression.
// Implements bidirectional streaming where:
// - Client sends: terminal input and resize events
// - Server sends: terminal deltas (compressed output) or raw output (fallback)
func (s *SessionService) StreamTerminal(
	ctx context.Context,
	stream *connect.BidiStream[sessionv1.TerminalData, sessionv1.TerminalData],
) error {
	// Get the first message to determine which session to attach to
	initialMsg, err := stream.Receive()
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to receive initial message: %w", err))
	}

	if initialMsg == nil {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("no initial message received"))
	}

	if initialMsg.SessionId == "" {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required"))
	}

	// Load the session instance with dependencies wired up
	instances, err := s.loadInstancesWithWiring()
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
	}

	var instance *session.Instance
	for _, inst := range instances {
		if inst.Title == initialMsg.SessionId {
			instance = inst
			break
		}
	}

	if instance == nil {
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", initialMsg.SessionId))
	}

	// Verify session is started and not paused
	if !instance.Started() {
		return connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("session not started"))
	}

	if instance.Paused() {
		return connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("session is paused"))
	}

	// Get PTY for reading terminal output
	ptyFile, err := instance.GetPTYReader()
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get PTY reader: %w", err))
	}

	// Create context for managing goroutines
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Channel for errors from goroutines
	errCh := make(chan error, 2)

	// Initialize terminal state for MOSH-style state synchronization (default 80x25)
	// Will be resized when client sends first resize message
	terminalState := session.NewTerminalState(25, 80)

	// Flow control state for backpressure management
	// Reference: https://xtermjs.org/docs/guides/flowcontrol/
	pauseCh := make(chan bool, 1) // Buffered channel for pause/resume signals
	var ptyPaused bool             // Current PTY pause state

	// Goroutine 1: Read from PTY and send deltas to client (terminal output)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				errCh <- fmt.Errorf("panic in output goroutine: %v", r)
			}
		}()

		buf := make([]byte, 1024) // 1KB chunks as per task requirements
		for {
			select {
			case <-streamCtx.Done():
				return
			case paused := <-pauseCh:
				// Update pause state
				ptyPaused = paused
				if paused {
					log.InfoLog.Printf("[FlowControl] PTY reading PAUSED for session %s", initialMsg.SessionId)
				} else {
					log.InfoLog.Printf("[FlowControl] PTY reading RESUMED for session %s", initialMsg.SessionId)
				}
			default:
				// Skip PTY reading when paused (backpressure from client)
				if ptyPaused {
					continue
				}

				n, readErr := ptyFile.Read(buf)
				if n > 0 {
					// Update terminal activity timestamps with the output content
					// This ensures LastMeaningfulOutput reflects web UI viewing activity
					instance.UpdateTerminalTimestamps(string(buf[:n]), true)

					// Process PTY output through terminal state
					if processErr := terminalState.ProcessOutput(buf[:n]); processErr != nil {
						log.WarningLog.Printf("Failed to process terminal output: %v", processErr)
						// Fallback to raw output on parse errors
						outputMsg := &sessionv1.TerminalData{
							SessionId: initialMsg.SessionId,
							Data: &sessionv1.TerminalData_Output{
								Output: &sessionv1.TerminalOutput{
									Data: buf[:n],
								},
							},
						}
						if sendErr := stream.Send(outputMsg); sendErr != nil {
							errCh <- fmt.Errorf("failed to send output: %w", sendErr)
							return
						}
						continue
					}

					// Generate complete terminal state (MOSH-style)
					stateMsg := terminalState.GenerateState()
					stateMsg.SessionId = initialMsg.SessionId

					// Send state to client
					if sendErr := stream.Send(stateMsg); sendErr != nil {
						errCh <- fmt.Errorf("failed to send state: %w", sendErr)
						return
					}
				}

				if readErr != nil {
					// EOF or other read error
					if readErr.Error() != "EOF" {
						errCh <- fmt.Errorf("PTY read error: %w", readErr)
					}
					return
				}
			}
		}
	}()

	// Goroutine 2: Receive from client and forward to PTY (terminal input + resize)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				errCh <- fmt.Errorf("panic in input goroutine: %v", r)
			}
		}()

		for {
			select {
			case <-streamCtx.Done():
				return
			default:
				msg, receiveErr := stream.Receive()
				if receiveErr != nil {
					// Check if this is a normal EOF (client closed connection)
					// ConnectRPC returns io.EOF or various "stream ended" errors
					errStr := receiveErr.Error()
					if receiveErr == context.Canceled ||
						receiveErr == context.DeadlineExceeded ||
						errStr == "EOF" ||
						errStr == "stream ended" ||
						strings.Contains(errStr, "stream closed") ||
						strings.Contains(errStr, "connection closed") {
						// Client closed gracefully, exit without error
						return
					}
					// Other errors should be reported
					errCh <- fmt.Errorf("stream receive error: %w", receiveErr)
					return
				}

				if msg == nil {
					// Stream ended cleanly
					return
				}

				switch data := msg.Data.(type) {
				case *sessionv1.TerminalData_Input:
					// Update terminal activity timestamps with user input
					// This ensures LastMeaningfulOutput reflects user interaction via web UI
					instance.UpdateTerminalTimestamps(string(data.Input.Data), true)

					// Forward input to PTY
					if _, writeErr := instance.WriteToPTY(data.Input.Data); writeErr != nil {
						// Send error back to client
						errorMsg := &sessionv1.TerminalData{
							SessionId: msg.SessionId,
							Data: &sessionv1.TerminalData_Error{
								Error: &sessionv1.TerminalError{
									Message: fmt.Sprintf("Failed to write to PTY: %v", writeErr),
									Code:    "WRITE_ERROR",
								},
							},
						}
						_ = stream.Send(errorMsg) // Best effort
						errCh <- writeErr
						return
					}

					// Publish user interaction event for immediate review queue reactivity
					s.eventBus.Publish(events.NewUserInteractionEvent(
						msg.SessionId,
						"terminal_input",
						"", // No additional context needed
					))

				case *sessionv1.TerminalData_Resize:
					// Handle terminal resize - update both PTY and terminal state
					cols := int(data.Resize.Cols)
					rows := int(data.Resize.Rows)

					if resizeErr := instance.ResizePTY(cols, rows); resizeErr != nil {
						// Send error back to client
						errorMsg := &sessionv1.TerminalData{
							SessionId: msg.SessionId,
							Data: &sessionv1.TerminalData_Error{
								Error: &sessionv1.TerminalError{
									Message: fmt.Sprintf("Failed to resize terminal: %v", resizeErr),
									Code:    "RESIZE_ERROR",
								},
							},
						}
						_ = stream.Send(errorMsg) // Best effort
						// Don't return on resize errors, they're not fatal
					} else {
						// Also resize terminal state to match
						terminalState.Resize(rows, cols)
						log.InfoLog.Printf("Resized terminal state to %dx%d for session %s", cols, rows, msg.SessionId)
					}

				case *sessionv1.TerminalData_FlowControl:
					// Handle flow control signals from client
					// Reference: https://xtermjs.org/docs/guides/flowcontrol/
					if data.FlowControl.Paused {
						log.InfoLog.Printf("[FlowControl] Client requested PAUSE (watermark: %d bytes) for session %s",
							data.FlowControl.Watermark, msg.SessionId)
						// Signal PTY reading goroutine to pause
						select {
						case pauseCh <- true:
						default:
							// Channel already has pause signal, skip
						}
					} else {
						log.InfoLog.Printf("[FlowControl] Client requested RESUME (watermark: %d bytes) for session %s",
							data.FlowControl.Watermark, msg.SessionId)
						// Signal PTY reading goroutine to resume
						select {
						case pauseCh <- false:
						default:
							// Channel already has resume signal, skip
						}
					}

				case *sessionv1.TerminalData_Error:
					// Client sent an error, log it
					log.ErrorLog.Printf("Client error: %s (%s)", data.Error.Message, data.Error.Code)
				}
			}
		}
	}()

	// Wait for either context cancellation or error
	select {
	case <-streamCtx.Done():
		log.InfoLog.Printf("StreamTerminal: context done for session %s", initialMsg.SessionId)
		return nil // Clean shutdown
	case err := <-errCh:
		log.ErrorLog.Printf("StreamTerminal: error for session %s: %v", initialMsg.SessionId, err)
		return connect.NewError(connect.CodeInternal, err)
	}
}

// GetSessionDiff retrieves the current git diff for a session.
func (s *SessionService) GetSessionDiff(
	ctx context.Context,
	req *connect.Request[sessionv1.GetSessionDiffRequest],
) (*connect.Response[sessionv1.GetSessionDiffResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	instances, err := s.loadInstancesWithWiring()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
	}

	// Find instance by ID (using Title as ID)
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

	// Get diff stats from the instance
	diffStats := instance.GetDiffStats()
	if diffStats == nil {
		// Return empty diff stats if none available
		return connect.NewResponse(&sessionv1.GetSessionDiffResponse{
			DiffStats: &sessionv1.DiffStats{
				Added:   0,
				Removed: 0,
				Content: "",
			},
		}), nil
	}

	// Convert to proto message
	protoDiffStats := &sessionv1.DiffStats{
		Added:   int32(diffStats.Added),
		Removed: int32(diffStats.Removed),
		Content: diffStats.Content,
	}

	return connect.NewResponse(&sessionv1.GetSessionDiffResponse{
		DiffStats: protoDiffStats,
	}), nil
}

// GetReviewQueue returns sessions needing user attention with priority ordering.
// Uses the global stateful queue managed by ReviewQueuePoller, with optional filtering.
func (s *SessionService) GetReviewQueue(
	ctx context.Context,
	req *connect.Request[sessionv1.GetReviewQueueRequest],
) (*connect.Response[sessionv1.GetReviewQueueResponse], error) {
	// Use global stateful queue managed by ReviewQueuePoller
	// This ensures dismissals persist and DetectedAt timestamps are preserved
	allItems := s.reviewQueue.List()

	// Apply filters from request if specified
	filteredItems := make([]*session.ReviewItem, 0, len(allItems))
	for _, item := range allItems {
		// Apply priority filter if specified
		if req.Msg.PriorityFilter != nil {
			targetPriority := adapters.ProtoToPriority(*req.Msg.PriorityFilter)
			if item.Priority != targetPriority {
				continue
			}
		}

		// Apply reason filter if specified
		if req.Msg.ReasonFilter != nil {
			targetReason := adapters.ProtoToAttentionReason(*req.Msg.ReasonFilter)
			if item.Reason != targetReason {
				continue
			}
		}

		filteredItems = append(filteredItems, item)
	}

	// Create temporary queue for proto conversion
	queue := session.NewReviewQueue()
	for _, item := range filteredItems {
		queue.Add(item)
	}

	// Convert to proto using adapters
	protoQueue := adapters.ReviewQueueToProto(queue)

	return connect.NewResponse(&sessionv1.GetReviewQueueResponse{
		ReviewQueue: protoQueue,
	}), nil
}

// AcknowledgeSession marks a session as acknowledged in the review queue.
// The session won't reappear in the queue until it receives an update.
func (s *SessionService) AcknowledgeSession(
	ctx context.Context,
	req *connect.Request[sessionv1.AcknowledgeSessionRequest],
) (*connect.Response[sessionv1.AcknowledgeSessionResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	instances, err := s.storage.LoadInstances()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
	}

	// Find the instance to acknowledge
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

	// Set the acknowledgment timestamp to now
	instance.LastAcknowledged = time.Now()

	// Update the instance in the list and save
	instances[instanceIndex] = instance
	if err := s.storage.SaveInstances(instances); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save instance: %w", err))
	}

	// CRITICAL: Update the ReviewQueuePoller's instance references
	// When we LoadInstances() above, we create brand new instance objects.
	// The poller still has references to the OLD objects from initialization.
	// If we don't update the poller's references, it will continue checking
	// stale objects with outdated LastAddedToQueue timestamps, causing
	// notification spam even after the user acknowledges sessions.
	if s.reviewQueuePoller != nil {
		s.reviewQueuePoller.SetInstances(instances)
		log.InfoLog.Printf("[ReviewQueue] Updated poller instance references after AcknowledgeSession for '%s'", instance.Title)
	}

	// Publish event for immediate reactivity
	s.eventBus.Publish(events.NewSessionAcknowledgedEvent(
		instance.Title,
		"user_acknowledged",
	))

	return connect.NewResponse(&sessionv1.AcknowledgeSessionResponse{
		Success: true,
		Message: fmt.Sprintf("Session '%s' acknowledged and removed from review queue", req.Msg.Id),
	}), nil
}

// GetLogs retrieves application logs with optional filtering and search.
func (s *SessionService) GetLogs(
	ctx context.Context,
	req *connect.Request[sessionv1.GetLogsRequest],
) (*connect.Response[sessionv1.GetLogsResponse], error) {
	// Get log file path from config
	cfg := log.ConfigToLogConfig(config.LoadConfig())
	logFilePath, err := log.GetLogFilePath(cfg)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get log file path: %w", err))
	}

	// Read log file
	file, err := os.Open(logFilePath)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to open log file: %w", err))
	}
	defer file.Close()

	// Parse logs with filters
	result, err := parseLogs(file, req.Msg)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to parse logs: %w", err))
	}

	return connect.NewResponse(&sessionv1.GetLogsResponse{
		Entries:    result.Entries,
		TotalCount: int32(result.TotalCount),
		HasMore:    result.HasMore,
	}), nil
}

// parseLogsResult contains the result of parsing logs with pagination info
type parseLogsResult struct {
	Entries    []*sessionv1.LogEntry
	TotalCount int
	HasMore    bool
}

// parseLogs reads log file and applies filters to return matching entries
func parseLogs(reader io.Reader, req *sessionv1.GetLogsRequest) (*parseLogsResult, error) {
	// Log line format: [instance] LEVEL:date time file:line: message
	// Example: [pid-12345-timestamp] INFO:2025/10/17 14:23:45 app.go:123: Starting session
	logLineRegex := regexp.MustCompile(`^\[([^\]]+)\]\s+(\w+):(\d{4}/\d{2}/\d{2})\s+(\d{2}:\d{2}:\d{2})\s+([^:]+:\d+):\s+(.*)$`)

	var entries []*sessionv1.LogEntry
	scanner := bufio.NewScanner(reader)

	// Default limit if not specified
	limit := 100
	if req.Limit != nil && *req.Limit > 0 {
		limit = int(*req.Limit)
	}

	// Parse offset (default: 0)
	offset := 0
	if req.Offset != nil && *req.Offset > 0 {
		offset = int(*req.Offset)
	}

	// Parse filters
	var searchQuery string
	if req.SearchQuery != nil {
		searchQuery = strings.ToLower(*req.SearchQuery)
	}

	var levelFilter string
	if req.Level != nil {
		levelFilter = strings.ToUpper(*req.Level)
	}

	var startTime, endTime *time.Time
	if req.StartTime != nil {
		t := req.StartTime.AsTime()
		startTime = &t
	}
	if req.EndTime != nil {
		t := req.EndTime.AsTime()
		endTime = &t
	}

	for scanner.Scan() {
		line := scanner.Text()

		// Try to parse the log line
		matches := logLineRegex.FindStringSubmatch(line)
		if matches == nil || len(matches) < 7 {
			// Skip lines that don't match expected format
			continue
		}

		// Extract fields from regex match
		// matches[1] = instance (ignored for API)
		level := matches[2]
		dateStr := matches[3]
		timeStr := matches[4]
		source := matches[5]
		message := matches[6]

		// Parse timestamp
		timestampStr := fmt.Sprintf("%s %s", dateStr, timeStr)
		timestamp, err := time.Parse("2006/01/02 15:04:05", timestampStr)
		if err != nil {
			// Skip entries with invalid timestamps
			continue
		}

		// Apply level filter
		if levelFilter != "" && level != levelFilter {
			continue
		}

		// Apply time range filters
		if startTime != nil && timestamp.Before(*startTime) {
			continue
		}
		if endTime != nil && timestamp.After(*endTime) {
			continue
		}

		// Apply search query filter (case-insensitive, searches message and source)
		if searchQuery != "" {
			messageAndSource := strings.ToLower(message + " " + source)
			if !strings.Contains(messageAndSource, searchQuery) {
				continue
			}
		}

		// Create log entry
		entry := &sessionv1.LogEntry{
			Timestamp: timestamppb.New(timestamp),
			Level:     level,
			Message:   message,
			Source:    &source,
		}

		entries = append(entries, entry)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading log file: %w", err)
	}

	// Reverse entries to show most recent first
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}

	// Store total count before pagination
	totalCount := len(entries)

	// Apply offset
	if offset >= len(entries) {
		// Offset beyond available entries, return empty result
		return &parseLogsResult{
			Entries:    []*sessionv1.LogEntry{},
			TotalCount: totalCount,
			HasMore:    false,
		}, nil
	}

	// Apply offset and limit
	start := offset
	end := offset + limit
	if end > len(entries) {
		end = len(entries)
	}

	paginatedEntries := entries[start:end]
	hasMore := end < len(entries)

	return &parseLogsResult{
		Entries:    paginatedEntries,
		TotalCount: totalCount,
		HasMore:    hasMore,
	}, nil
}

// WatchReviewQueueFilters contains filters for review queue event streaming.
type WatchReviewQueueFilters struct {
	PriorityFilter    []session.Priority
	ReasonFilter      []session.AttentionReason
	SessionIDs        []string
	IncludeStatistics bool
	InitialSnapshot   bool
}

// Implement FilterProvider interface for type-safe conversion
func (f *WatchReviewQueueFilters) GetPriorityFilter() []session.Priority {
	return f.PriorityFilter
}

func (f *WatchReviewQueueFilters) GetReasonFilter() []session.AttentionReason {
	return f.ReasonFilter
}

func (f *WatchReviewQueueFilters) GetSessionIDs() []string {
	return f.SessionIDs
}

func (f *WatchReviewQueueFilters) GetIncludeStatistics() bool {
	return f.IncludeStatistics
}

func (f *WatchReviewQueueFilters) GetInitialSnapshot() bool {
	return f.InitialSnapshot
}

// WatchReviewQueue streams real-time review queue events.
func (s *SessionService) WatchReviewQueue(
	ctx context.Context,
	req *connect.Request[sessionv1.WatchReviewQueueRequest],
	stream *connect.ServerStream[sessionv1.ReviewQueueEvent],
) error {
	if s.reactiveQueueMgr == nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("reactive queue manager not initialized"))
	}

	// Convert proto filters to internal type
	filters := &WatchReviewQueueFilters{
		PriorityFilter:    convertProtoPriorities(req.Msg.PriorityFilter),
		ReasonFilter:      convertProtoReasons(req.Msg.ReasonFilter),
		SessionIDs:        req.Msg.SessionIds,
		IncludeStatistics: req.Msg.IncludeStatistics,
		InitialSnapshot:   req.Msg.InitialSnapshot,
	}

	// Subscribe to queue events
	eventCh, clientID := s.reactiveQueueMgr.AddStreamClient(ctx, filters)
	defer s.reactiveQueueMgr.RemoveStreamClient(clientID)

	// Stream events
	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-eventCh:
			if !ok {
				return nil // Channel closed
			}
			if err := stream.Send(event); err != nil {
				return err
			}
		}
	}
}

// convertProtoPriorities converts proto Priority values to internal session.Priority
func convertProtoPriorities(protoPriorities []sessionv1.Priority) []session.Priority {
	result := make([]session.Priority, 0, len(protoPriorities))
	for _, p := range protoPriorities {
		switch p {
		case sessionv1.Priority_PRIORITY_URGENT:
			result = append(result, session.PriorityUrgent)
		case sessionv1.Priority_PRIORITY_HIGH:
			result = append(result, session.PriorityHigh)
		case sessionv1.Priority_PRIORITY_MEDIUM:
			result = append(result, session.PriorityMedium)
		case sessionv1.Priority_PRIORITY_LOW:
			result = append(result, session.PriorityLow)
		}
	}
	return result
}

// convertProtoReasons converts proto AttentionReason values to internal session.AttentionReason
func convertProtoReasons(protoReasons []sessionv1.AttentionReason) []session.AttentionReason {
	result := make([]session.AttentionReason, 0, len(protoReasons))
	for _, r := range protoReasons {
		switch r {
		case sessionv1.AttentionReason_ATTENTION_REASON_APPROVAL_PENDING:
			result = append(result, session.ReasonApprovalPending)
		case sessionv1.AttentionReason_ATTENTION_REASON_INPUT_REQUIRED:
			result = append(result, session.ReasonInputRequired)
		case sessionv1.AttentionReason_ATTENTION_REASON_ERROR_STATE:
			result = append(result, session.ReasonErrorState)
		case sessionv1.AttentionReason_ATTENTION_REASON_IDLE_TIMEOUT:
			result = append(result, session.ReasonIdleTimeout)
		case sessionv1.AttentionReason_ATTENTION_REASON_TASK_COMPLETE:
			result = append(result, session.ReasonTaskComplete)
		}
	}
	return result
}

// formatDuration formats a time.Duration in a human-readable way.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		minutes := int(d.Minutes()) % 60
		if minutes == 0 {
			return fmt.Sprintf("%dh", hours)
		}
		return fmt.Sprintf("%dh%dm", hours, minutes)
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	if hours == 0 {
		return fmt.Sprintf("%dd", days)
	}
	return fmt.Sprintf("%dd%dh", days, hours)
}

// LogUserInteraction logs a user interaction event for audit trail and analytics.
// This method records user actions for compliance, debugging, and product insights.
func (s *SessionService) LogUserInteraction(
	ctx context.Context,
	req *connect.Request[sessionv1.LogUserInteractionRequest],
) (*connect.Response[sessionv1.LogUserInteractionResponse], error) {
	// Extract request data
	sessionID := ""
	if req.Msg.SessionId != nil {
		sessionID = *req.Msg.SessionId
	}
	interactionType := req.Msg.InteractionType
	context := ""
	if req.Msg.Context != nil {
		context = *req.Msg.Context
	}
	notificationID := ""
	if req.Msg.NotificationId != nil {
		notificationID = *req.Msg.NotificationId
	}

	// Build structured log entry
	fields := map[string]interface{}{
		"interaction_type": interactionType.String(),
		"timestamp":        time.Now().Format(time.RFC3339),
	}

	if sessionID != "" {
		fields["session_id"] = sessionID
	}
	if context != "" {
		fields["context"] = context
	}
	if notificationID != "" {
		fields["notification_id"] = notificationID
	}

	// Add metadata if provided
	if req.Msg.Metadata != nil && len(req.Msg.Metadata) > 0 {
		for key, value := range req.Msg.Metadata {
			fields["meta_"+key] = value
		}
	}

	// Log the interaction using structured logging
	log.InfoS("User Interaction", fields)

	// Optionally emit event to event bus for real-time processing
	if s.eventBus != nil {
		// Use internal event type for event bus
		event := events.NewUserInteractionEvent(sessionID, interactionType.String(), context)
		s.eventBus.Publish(event)
	}

	// Return success response
	return connect.NewResponse(&sessionv1.LogUserInteractionResponse{
		Success: true,
	}), nil
}

// GetClaudeConfig retrieves a Claude configuration file by name
func (s *SessionService) GetClaudeConfig(
	ctx context.Context,
	req *connect.Request[sessionv1.GetClaudeConfigRequest],
) (*connect.Response[sessionv1.GetClaudeConfigResponse], error) {
	mgr, err := config.NewClaudeConfigManager()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create config manager: %w", err))
	}

	configFile, err := mgr.GetConfig(req.Msg.Filename)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&sessionv1.GetClaudeConfigResponse{
		Config: &sessionv1.ClaudeConfigFile{
			Name:    configFile.Name,
			Path:    configFile.Path,
			Content: configFile.Content,
			ModTime: timestamppb.New(configFile.ModTime),
		},
	}), nil
}

// ListClaudeConfigs returns all configuration files in the ~/.claude directory
func (s *SessionService) ListClaudeConfigs(
	ctx context.Context,
	req *connect.Request[sessionv1.ListClaudeConfigsRequest],
) (*connect.Response[sessionv1.ListClaudeConfigsResponse], error) {
	mgr, err := config.NewClaudeConfigManager()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create config manager: %w", err))
	}

	configs, err := mgr.ListConfigs()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	protoConfigs := make([]*sessionv1.ClaudeConfigFile, 0, len(configs))
	for _, cfg := range configs {
		protoConfigs = append(protoConfigs, &sessionv1.ClaudeConfigFile{
			Name:    cfg.Name,
			Path:    cfg.Path,
			Content: cfg.Content,
			ModTime: timestamppb.New(cfg.ModTime),
		})
	}

	return connect.NewResponse(&sessionv1.ListClaudeConfigsResponse{
		Configs: protoConfigs,
	}), nil
}

// UpdateClaudeConfig updates a Claude configuration file with atomic write and backup
func (s *SessionService) UpdateClaudeConfig(
	ctx context.Context,
	req *connect.Request[sessionv1.UpdateClaudeConfigRequest],
) (*connect.Response[sessionv1.UpdateClaudeConfigResponse], error) {
	mgr, err := config.NewClaudeConfigManager()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create config manager: %w", err))
	}

	// Use validation if requested
	if req.Msg.Validate {
		err = mgr.UpdateConfigWithValidation(req.Msg.Filename, req.Msg.Content)
	} else {
		err = mgr.UpdateConfig(req.Msg.Filename, req.Msg.Content)
	}

	if err != nil {
		if strings.Contains(err.Error(), "validation failed") {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Read back the updated file
	configFile, err := mgr.GetConfig(req.Msg.Filename)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to read updated config: %w", err))
	}

	return connect.NewResponse(&sessionv1.UpdateClaudeConfigResponse{
		Config: &sessionv1.ClaudeConfigFile{
			Name:    configFile.Name,
			Path:    configFile.Path,
			Content: configFile.Content,
			ModTime: timestamppb.New(configFile.ModTime),
		},
	}), nil
}

// getOrRefreshHistoryCache returns the cached history or refreshes it if stale
func (s *SessionService) getOrRefreshHistoryCache() (*session.ClaudeSessionHistory, error) {
	now := time.Now()

	// Check if cache is valid
	if s.historyCache != nil && now.Sub(s.historyCacheTime) < s.historyCacheTTL {
		return s.historyCache, nil
	}

	// Cache is stale or doesn't exist - refresh it
	hist, err := session.NewClaudeSessionHistoryFromClaudeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to create history manager: %w", err)
	}

	// Update cache
	s.historyCache = hist
	s.historyCacheTime = now

	fmt.Printf("History cache refreshed: %d entries\n", hist.Count())
	return hist, nil
}

// ListClaudeHistory returns Claude session history entries with optional filtering
func (s *SessionService) ListClaudeHistory(
	ctx context.Context,
	req *connect.Request[sessionv1.ListClaudeHistoryRequest],
) (*connect.Response[sessionv1.ListClaudeHistoryResponse], error) {
	// Use cached history
	hist, err := s.getOrRefreshHistoryCache()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load history: %w", err))
	}

	var entries []session.ClaudeHistoryEntry

	// Apply filters
	if req.Msg.Project != nil && *req.Msg.Project != "" {
		entries = hist.GetByProject(*req.Msg.Project)
	} else if req.Msg.SearchQuery != nil && *req.Msg.SearchQuery != "" {
		entries = hist.Search(*req.Msg.SearchQuery)
	} else {
		entries = hist.GetAll()
	}

	// Apply limit
	totalCount := len(entries)
	if req.Msg.Limit > 0 && int(req.Msg.Limit) < len(entries) {
		entries = entries[:req.Msg.Limit]
	}

	// Convert to proto
	protoEntries := make([]*sessionv1.ClaudeHistoryEntry, 0, len(entries))
	for _, entry := range entries {
		protoEntries = append(protoEntries, &sessionv1.ClaudeHistoryEntry{
			Id:           entry.ID,
			Name:         entry.Name,
			Project:      entry.Project,
			CreatedAt:    timestamppb.New(entry.CreatedAt),
			UpdatedAt:    timestamppb.New(entry.UpdatedAt),
			Model:        entry.Model,
			MessageCount: int32(entry.MessageCount),
		})
	}

	return connect.NewResponse(&sessionv1.ListClaudeHistoryResponse{
		Entries:    protoEntries,
		TotalCount: int32(totalCount),
	}), nil
}

// GetClaudeHistoryDetail retrieves detailed information for a specific history entry
func (s *SessionService) GetClaudeHistoryDetail(
	ctx context.Context,
	req *connect.Request[sessionv1.GetClaudeHistoryDetailRequest],
) (*connect.Response[sessionv1.GetClaudeHistoryDetailResponse], error) {
	// Use cached history
	hist, err := s.getOrRefreshHistoryCache()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load history: %w", err))
	}

	entry, err := hist.GetByID(req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	return connect.NewResponse(&sessionv1.GetClaudeHistoryDetailResponse{
		Entry: &sessionv1.ClaudeHistoryEntry{
			Id:           entry.ID,
			Name:         entry.Name,
			Project:      entry.Project,
			CreatedAt:    timestamppb.New(entry.CreatedAt),
			UpdatedAt:    timestamppb.New(entry.UpdatedAt),
			Model:        entry.Model,
			MessageCount: int32(entry.MessageCount),
		},
	}), nil
}

// GetClaudeHistoryMessages retrieves messages from a specific conversation
func (s *SessionService) GetClaudeHistoryMessages(
	ctx context.Context,
	req *connect.Request[sessionv1.GetClaudeHistoryMessagesRequest],
) (*connect.Response[sessionv1.GetClaudeHistoryMessagesResponse], error) {
	// Use cached history to validate session exists
	hist, err := s.getOrRefreshHistoryCache()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load history: %w", err))
	}

	// Validate session exists
	_, err = hist.GetByID(req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %w", err))
	}

	// Get messages from conversation file
	messages, err := hist.GetMessagesFromConversationFile(req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load messages: %w", err))
	}

	// Apply pagination
	totalCount := len(messages)
	offset := int(req.Msg.Offset)
	limit := int(req.Msg.Limit)

	if offset > 0 && offset < len(messages) {
		messages = messages[offset:]
	}
	if limit > 0 && limit < len(messages) {
		messages = messages[:limit]
	}

	// Convert to proto messages
	protoMessages := make([]*sessionv1.ClaudeMessage, 0, len(messages))
	for _, msg := range messages {
		protoMessages = append(protoMessages, &sessionv1.ClaudeMessage{
			Role:      msg.Role,
			Content:   msg.Content,
			Timestamp: timestamppb.New(msg.Timestamp),
			Model:     msg.Model,
		})
	}

	return connect.NewResponse(&sessionv1.GetClaudeHistoryMessagesResponse{
		Messages:   protoMessages,
		TotalCount: int32(totalCount),
	}), nil
}

// GetPRInfo retrieves the latest PR information for a session.
func (s *SessionService) GetPRInfo(
	ctx context.Context,
	req *connect.Request[sessionv1.GetPRInfoRequest],
) (*connect.Response[sessionv1.GetPRInfoResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	instances, err := s.loadInstancesWithWiring()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
	}

	// Find instance by ID (using Title as ID)
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

	// Check if this is a PR session
	if !instance.IsPRSession() {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("session '%s' is not a PR session", req.Msg.Id))
	}

	// Refresh PR info from GitHub
	prInfo, err := instance.RefreshPRInfo()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to refresh PR info: %w", err))
	}

	// Convert to proto message
	protoPRInfo := &sessionv1.PRInfo{
		Number:       int32(prInfo.Number),
		Title:        prInfo.Title,
		Body:         prInfo.Body,
		HeadRef:      prInfo.HeadRef,
		BaseRef:      prInfo.BaseRef,
		State:        prInfo.State,
		Author:       prInfo.Author,
		Labels:       prInfo.Labels,
		HtmlUrl:      prInfo.HTMLURL,
		CreatedAt:    timestamppb.New(prInfo.CreatedAt),
		UpdatedAt:    timestamppb.New(prInfo.UpdatedAt),
		IsDraft:      prInfo.IsDraft,
		Mergeable:    prInfo.Mergeable,
		Additions:    int32(prInfo.Additions),
		Deletions:    int32(prInfo.Deletions),
		ChangedFiles: int32(prInfo.ChangedFiles),
	}

	return connect.NewResponse(&sessionv1.GetPRInfoResponse{
		PrInfo: protoPRInfo,
	}), nil
}

// GetPRComments retrieves all comments on the PR for a session.
func (s *SessionService) GetPRComments(
	ctx context.Context,
	req *connect.Request[sessionv1.GetPRCommentsRequest],
) (*connect.Response[sessionv1.GetPRCommentsResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	instances, err := s.loadInstancesWithWiring()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
	}

	// Find instance by ID (using Title as ID)
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

	// Check if this is a PR session
	if !instance.IsPRSession() {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("session '%s' is not a PR session", req.Msg.Id))
	}

	// Get PR comments from GitHub
	comments, err := instance.GetPRComments()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get PR comments: %w", err))
	}

	// Convert to proto messages
	protoComments := make([]*sessionv1.PRComment, 0, len(comments))
	for _, comment := range comments {
		protoComment := &sessionv1.PRComment{
			Id:        int32(comment.ID),
			Author:    comment.Author,
			Body:      comment.Body,
			CreatedAt: timestamppb.New(comment.CreatedAt),
			IsReview:  comment.IsReview,
		}
		if comment.Path != "" {
			protoComment.Path = &comment.Path
		}
		if comment.Line != 0 {
			line := int32(comment.Line)
			protoComment.Line = &line
		}
		protoComments = append(protoComments, protoComment)
	}

	return connect.NewResponse(&sessionv1.GetPRCommentsResponse{
		Comments: protoComments,
	}), nil
}

// PostPRComment posts a new comment to the PR for a session.
func (s *SessionService) PostPRComment(
	ctx context.Context,
	req *connect.Request[sessionv1.PostPRCommentRequest],
) (*connect.Response[sessionv1.PostPRCommentResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	if req.Msg.Body == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("comment body is required"))
	}

	instances, err := s.loadInstancesWithWiring()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
	}

	// Find instance by ID (using Title as ID)
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

	// Check if this is a PR session
	if !instance.IsPRSession() {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("session '%s' is not a PR session", req.Msg.Id))
	}

	// Post comment to GitHub
	if err := instance.PostComment(req.Msg.Body); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to post comment: %w", err))
	}

	return connect.NewResponse(&sessionv1.PostPRCommentResponse{
		Success: true,
		Message: fmt.Sprintf("Comment posted successfully to PR for session '%s'", req.Msg.Id),
	}), nil
}

// MergePR merges the PR for a session using the specified merge method.
func (s *SessionService) MergePR(
	ctx context.Context,
	req *connect.Request[sessionv1.MergePRRequest],
) (*connect.Response[sessionv1.MergePRResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	instances, err := s.loadInstancesWithWiring()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
	}

	// Find instance by ID (using Title as ID)
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

	// Check if this is a PR session
	if !instance.IsPRSession() {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("session '%s' is not a PR session", req.Msg.Id))
	}

	// Get merge method (default to "merge" if not specified)
	method := "merge"
	if req.Msg.Method != nil && *req.Msg.Method != "" {
		method = *req.Msg.Method
	}

	// Merge PR using GitHub
	if err := instance.MergePR(method); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to merge PR: %w", err))
	}

	return connect.NewResponse(&sessionv1.MergePRResponse{
		Success: true,
		Message: fmt.Sprintf("PR merged successfully for session '%s' using method '%s'", req.Msg.Id, method),
	}), nil
}

// ClosePR closes the PR without merging for a session.
func (s *SessionService) ClosePR(
	ctx context.Context,
	req *connect.Request[sessionv1.ClosePRRequest],
) (*connect.Response[sessionv1.ClosePRResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	instances, err := s.loadInstancesWithWiring()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
	}

	// Find instance by ID (using Title as ID)
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

	// Check if this is a PR session
	if !instance.IsPRSession() {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("session '%s' is not a PR session", req.Msg.Id))
	}

	// Close PR using GitHub
	if err := instance.ClosePR(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to close PR: %w", err))
	}

	return connect.NewResponse(&sessionv1.ClosePRResponse{
		Success: true,
		Message: fmt.Sprintf("PR closed successfully for session '%s'", req.Msg.Id),
	}), nil
}

// SendNotification allows tmux sessions to send notifications to connected clients.
// Enforces localhost-only restriction, session validation, and rate limiting.
func (s *SessionService) SendNotification(
	ctx context.Context,
	req *connect.Request[sessionv1.SendNotificationRequest],
) (*connect.Response[sessionv1.SendNotificationResponse], error) {
	// Validate localhost-only origin
	if err := s.validateLocalhostOrigin(ctx, req); err != nil {
		return nil, err
	}

	// Validate required fields
	if req.Msg.SessionId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required"))
	}
	if req.Msg.Title == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("title is required"))
	}

	// Validate session exists
	instances, err := s.loadInstancesWithWiring()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
	}

	var instance *session.Instance
	for _, inst := range instances {
		if inst.Title == req.Msg.SessionId {
			instance = inst
			break
		}
	}

	if instance == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.SessionId))
	}

	// Apply rate limiting
	if !s.notificationRateLimiter.Allow(req.Msg.SessionId) {
		return nil, connect.NewError(connect.CodeResourceExhausted, fmt.Errorf("rate limit exceeded for session: %s", req.Msg.SessionId))
	}

	// Generate notification ID
	notificationID := uuid.New().String()

	// Broadcast notification via event bus
	event := events.NewNotificationEvent(
		req.Msg.SessionId,
		instance.Title, // Session name
		notificationID,
		int32(req.Msg.NotificationType),
		int32(req.Msg.Priority),
		req.Msg.Title,
		req.Msg.Message,
		req.Msg.Metadata,
	)
	s.eventBus.Publish(event)

	log.InfoS("Notification sent", map[string]interface{}{
		"session_id":        req.Msg.SessionId,
		"notification_type": req.Msg.NotificationType.String(),
		"priority":          req.Msg.Priority.String(),
		"title":             req.Msg.Title,
		"notification_id":   notificationID,
	})

	return connect.NewResponse(&sessionv1.SendNotificationResponse{
		Success:        true,
		Message:        "Notification sent successfully",
		NotificationId: notificationID,
	}), nil
}

// validateLocalhostOrigin ensures the request comes from localhost.
// This is a security measure to prevent external actors from sending notifications.
func (s *SessionService) validateLocalhostOrigin(ctx context.Context, req *connect.Request[sessionv1.SendNotificationRequest]) error {
	// Get peer address from request headers or context
	// ConnectRPC provides X-Forwarded-For or we can check the connection directly

	// Check X-Real-IP header first (if behind a proxy)
	realIP := req.Header().Get("X-Real-IP")
	if realIP != "" {
		if !isLocalhostIP(realIP) {
			return connect.NewError(connect.CodePermissionDenied, fmt.Errorf("notifications can only be sent from localhost"))
		}
		return nil
	}

	// Check X-Forwarded-For header
	forwardedFor := req.Header().Get("X-Forwarded-For")
	if forwardedFor != "" {
		// Take the first IP in the chain (original client)
		ips := strings.Split(forwardedFor, ",")
		if len(ips) > 0 {
			clientIP := strings.TrimSpace(ips[0])
			if !isLocalhostIP(clientIP) {
				return connect.NewError(connect.CodePermissionDenied, fmt.Errorf("notifications can only be sent from localhost"))
			}
			return nil
		}
	}

	// If no proxy headers, we're in direct connection mode
	// The server already binds to localhost, so requests reaching here are local
	// This is a defense-in-depth check
	return nil
}

// isLocalhostIP checks if the given IP string represents localhost.
func isLocalhostIP(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	return parsed.IsLoopback()
}
