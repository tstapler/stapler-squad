package services

import (
	"bufio"
	"claude-squad/config"
	sessionv1 "claude-squad/gen/proto/go/session/v1"
	"claude-squad/log"
	"claude-squad/server/adapters"
	"claude-squad/server/events"
	"claude-squad/session"
	"claude-squad/session/search"
	"connectrpc.com/connect"
	"context"
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/types/known/timestamppb"
	"io"
	"net"
	"os"
	"os/exec"
	"regexp"
	"runtime"
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
	storage           *session.Storage
	eventBus          *events.EventBus
	statusManager     *session.InstanceStatusManager
	reviewQueuePoller *session.ReviewQueuePoller

	// Extracted domain services.
	reviewQueueSvc *ReviewQueueService
	searchSvc      *SearchService
	githubSvc      *GitHubService
	workspaceSvc   *WorkspaceService

	// External session discovery (for mux-enabled sessions from external terminals)
	externalDiscovery *session.ExternalSessionDiscovery

	// Notification rate limiter (10 notifications/sec per session, burst of 20)
	notificationRateLimiter *NotificationRateLimiter

	// approvalStore holds pending Claude Code hook approval requests.
	approvalStore *ApprovalStore
}

// NewSessionService creates a new SessionService with the given storage and event bus.
// NOTE: Instances are NOT loaded here to prevent double-loading and initialization timing issues.
// Instances will be loaded in server.go after dependencies (statusManager, reviewQueue) are wired.
func NewSessionService(storage *session.Storage, eventBus *events.EventBus) *SessionService {
	reviewQueue := session.NewReviewQueue()

	// Initialize search engine with disk persistence for incremental index updates.
	var searchEngine *search.SearchEngine
	indexStore, err := search.NewIndexStore()
	if err != nil {
		log.WarningLog.Printf("Failed to create index store, using in-memory search: %v", err)
		searchEngine = search.NewSearchEngine()
	} else {
		searchEngine = search.NewSearchEngineWithPersistence(indexStore)
		if loadErr := searchEngine.LoadIndex(); loadErr != nil {
			log.WarningLog.Printf("Failed to load persisted search index: %v", loadErr)
		} else if searchEngine.GetSyncMetadata() != nil {
			meta := searchEngine.GetSyncMetadata()
			log.InfoLog.Printf("Loaded persisted search index: %d sessions, %d documents",
				meta.TotalSessions, meta.TotalDocuments)
		}
	}

	approvalStore := NewApprovalStore()
	reviewQueueSvc := NewReviewQueueService(reviewQueue, storage, eventBus)
	reviewQueueSvc.SetApprovalStore(approvalStore)

	return &SessionService{
		storage:                 storage,
		eventBus:                eventBus,
		reviewQueueSvc:          reviewQueueSvc,
		searchSvc:               NewSearchService(searchEngine, search.NewSnippetGenerator(), 5*time.Minute),
		githubSvc:               NewGitHubService(storage),
		workspaceSvc:            NewWorkspaceService(storage, eventBus),
		notificationRateLimiter: NewNotificationRateLimiter(10, 20),
		approvalStore:           approvalStore,
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
		inst.SetReviewQueue(s.reviewQueueSvc.GetQueue())
		if s.statusManager != nil {
			inst.SetStatusManager(s.statusManager)
		}
	}

	return instances, nil
}

// NewSessionServiceFromConfig creates a SessionService using EntRepository as storage backend.
// On first startup, if the legacy state.json exists and Ent DB is empty, sessions are
// auto-migrated from JSON to Ent.
func NewSessionServiceFromConfig() (*SessionService, error) {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to determine config directory: %w", err)
	}
	dbPath := configDir + "/sessions.db"

	repo, err := session.NewEntRepository(session.WithDatabasePath(dbPath))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize EntRepository: %w", err)
	}

	// Auto-migrate from state.json if Ent DB is empty and legacy data exists
	if migrateErr := maybeAutoMigrateToEnt(repo); migrateErr != nil {
		log.WarningLog.Printf("auto-migration to Ent skipped or failed: %v", migrateErr)
	}

	storage, err := session.NewStorageWithRepository(repo)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage with EntRepository: %w", err)
	}

	eventBus := events.NewEventBus(100)
	return NewSessionService(storage, eventBus), nil
}

// GetStorage returns the storage instance for direct access (e.g., WebSocket handlers).
func (s *SessionService) GetStorage() *session.Storage {
	return s.storage
}

// GetApprovalStore returns the approval store for wiring up the HTTP hook handler.
func (s *SessionService) GetApprovalStore() *ApprovalStore {
	return s.approvalStore
}

// maybeAutoMigrateToEnt checks whether state.json exists in the config directory and the
// Ent repository is empty. If both conditions hold, it migrates all sessions from state.json
// to Ent automatically. This is a one-shot migration: once data is in Ent the check is a no-op.
func maybeAutoMigrateToEnt(repo *session.EntRepository) error {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return fmt.Errorf("could not determine config dir: %w", err)
	}

	stateJSONPath := configDir + "/state.json"
	if _, statErr := os.Stat(stateJSONPath); os.IsNotExist(statErr) {
		return nil // nothing to migrate
	}

	// Check if Ent DB is already populated — skip migration if so
	ctx := context.Background()
	existing, listErr := repo.List(ctx)
	if listErr != nil {
		return fmt.Errorf("failed to list Ent sessions: %w", listErr)
	}
	if len(existing) > 0 {
		return nil // already has data, skip
	}

	// state.json stores instances inside a wrapper: {"instances": [...], ...}
	type stateFileFormat struct {
		Instances []session.InstanceData `json:"instances"`
	}
	rawData, readErr := os.ReadFile(stateJSONPath)
	if readErr != nil {
		return fmt.Errorf("failed to read state.json: %w", readErr)
	}

	var stateFile stateFileFormat
	if unmarshalErr := json.Unmarshal(rawData, &stateFile); unmarshalErr != nil {
		return fmt.Errorf("failed to parse state.json: %w", unmarshalErr)
	}

	if len(stateFile.Instances) == 0 {
		return nil // nothing to migrate
	}

	log.InfoLog.Printf("Auto-migrating %d sessions from state.json to Ent repository", len(stateFile.Instances))

	for _, inst := range stateFile.Instances {
		if createErr := repo.Create(ctx, inst); createErr != nil {
			log.WarningLog.Printf("auto-migrate: failed to create session '%s': %v", inst.Title, createErr)
		}
	}

	log.InfoLog.Printf("Auto-migration to Ent complete")
	return nil
}

// GetEventBus returns the event bus instance for wiring up reactive components.
func (s *SessionService) GetEventBus() *events.EventBus {
	return s.eventBus
}

// GetReviewQueueInstance returns the review queue instance for wiring up reactive components.
func (s *SessionService) GetReviewQueueInstance() *session.ReviewQueue {
	return s.reviewQueueSvc.GetQueue()
}

// SetReactiveQueueManager sets the ReactiveQueueManager (dependency injection).
// This must be called before WatchReviewQueue is used.
func (s *SessionService) SetReactiveQueueManager(mgr ReactiveQueueManager) {
	s.reviewQueueSvc.SetReactiveQueueManager(mgr)
}

// SetExternalDiscovery sets the external session discovery for accessing mux-enabled sessions.
func (s *SessionService) SetExternalDiscovery(discovery *session.ExternalSessionDiscovery) {
	s.externalDiscovery = discovery
}

// ListSessions returns all sessions with optional filtering.
// This includes both managed sessions and external mux-enabled sessions.
func (s *SessionService) ListSessions(
	ctx context.Context,
	req *connect.Request[sessionv1.ListSessionsRequest],
) (*connect.Response[sessionv1.ListSessionsResponse], error) {
	// Use the poller's live in-memory instances to avoid the side effect of
	// LoadInstances() → FromInstanceData() → Start() which restarts every session.
	var instances []*session.Instance
	if s.reviewQueuePoller != nil {
		instances = s.reviewQueuePoller.GetInstances()
	} else {
		var err error
		instances, err = s.loadInstancesWithWiring()
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
		}
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

	// Include external sessions from mux discovery if available
	if s.externalDiscovery != nil {
		for _, extInst := range s.externalDiscovery.GetSessions() {
			// Apply optional status filter (external sessions are always "running")
			if req.Msg.Status != nil && *req.Msg.Status != sessionv1.SessionStatus_SESSION_STATUS_UNSPECIFIED {
				// External sessions are running
				if *req.Msg.Status != sessionv1.SessionStatus_SESSION_STATUS_RUNNING {
					continue
				}
			}

			// Apply optional category filter
			if req.Msg.Category != nil && *req.Msg.Category != "" && extInst.Category != *req.Msg.Category {
				continue
			}

			sessions = append(sessions, adapters.InstanceToProto(extInst))
		}
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

	// Use the poller's live in-memory instances to avoid the side effect of
	// LoadInstances() → FromInstanceData() → Start() which restarts every session.
	if s.reviewQueuePoller != nil {
		if inst := s.reviewQueuePoller.FindInstance(req.Msg.Id); inst != nil {
			return connect.NewResponse(&sessionv1.GetSessionResponse{
				Session: adapters.InstanceToProto(inst),
			}), nil
		}
		// Not in poller — also check external sessions
		if s.externalDiscovery != nil {
			if inst := s.externalDiscovery.GetSession(req.Msg.Id); inst != nil {
				return connect.NewResponse(&sessionv1.GetSessionResponse{
					Session: adapters.InstanceToProto(inst),
				}), nil
			}
		}
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.Id))
	}

	// Fallback: poller not available — load from storage (has Start() side effect)
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

	// Resolve GitHub URLs to local paths (GOPATH-style: ~/.claude-squad/repos/github.com/owner/repo)
	resolvedPath := req.Msg.Path
	branch := req.Msg.Branch
	var gitHubRef *session.GitHubRef
	var clonedRepoPath string

	if session.IsGitHubURL(req.Msg.Path) {
		log.InfoLog.Printf("[CreateSession] Detected GitHub URL: %s", req.Msg.Path)
		localPath, ref, err := session.ResolveGitHubInput(req.Msg.Path)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("failed to resolve GitHub URL: %w", err))
		}
		resolvedPath = localPath
		gitHubRef = ref
		clonedRepoPath = localPath

		// Use branch from GitHub URL if not explicitly provided
		if branch == "" && ref.Branch != "" {
			branch = ref.Branch
		}

		log.InfoLog.Printf("[CreateSession] Resolved to local path: %s (branch: %s)", resolvedPath, branch)
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
	} else if branch != "" {
		// If branch is specified, create a new worktree
		sessionType = session.SessionTypeNewWorktree
	}

	// Build instance options
	instanceOpts := session.InstanceOptions{
		Title:            req.Msg.Title,
		Path:             resolvedPath,
		WorkingDir:       req.Msg.WorkingDir,
		Branch:           branch,
		Program:          program,
		AutoYes:          req.Msg.AutoYes,
		Prompt:           req.Msg.Prompt,
		ExistingWorktree: req.Msg.ExistingWorktree,
		Category:         req.Msg.Category,
		SessionType:      sessionType,
		TmuxPrefix:       "", // Use default from config
		ResumeId:         req.Msg.ResumeId,
	}

	// Add GitHub metadata if this was a GitHub URL
	if gitHubRef != nil {
		instanceOpts.GitHubOwner = gitHubRef.Owner
		instanceOpts.GitHubRepo = gitHubRef.Repo
		instanceOpts.GitHubSourceRef = req.Msg.Path
		instanceOpts.ClonedRepoPath = clonedRepoPath
		if gitHubRef.PRNumber > 0 {
			instanceOpts.GitHubPRNumber = gitHubRef.PRNumber
		}
	}

	// Create instance using NewInstance constructor
	instance, err := session.NewInstance(instanceOpts)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("failed to create instance: %w", err))
	}

	// Start the session (initializes tmux + git worktree)
	// Use Start(true) to indicate this is a first-time setup
	if err := instance.Start(true); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to start session: %w", err))
	}

	// Inject Claude Code HTTP hook config for remote approval from the web UI.
	// Non-fatal: session is fully functional even without this config.
	if err := InjectHookConfig(instance.GetEffectiveRootDir(), instance.Title); err != nil {
		log.WarningLog.Printf("[CreateSession] Failed to inject hook config for session '%s': %v", instance.Title, err)
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

	// Handle program update
	if req.Msg.Program != nil && *req.Msg.Program != "" {
		instance.Program = *req.Msg.Program
		updatedFields = append(updatedFields, "program")
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

	// Get the session instance - CRITICAL: Use the poller's instance to ensure
	// timestamp updates are visible to the review queue. Loading fresh from storage
	// creates a separate object that the poller never sees.
	var instance *session.Instance
	if s.reviewQueuePoller != nil {
		instance = s.reviewQueuePoller.FindInstance(initialMsg.SessionId)
	}

	// Fallback to storage if poller doesn't have it (shouldn't happen normally)
	if instance == nil {
		log.WarningLog.Printf("[StreamTerminal] Instance '%s' not found in poller, loading from storage (timestamps may desync)", initialMsg.SessionId)
		instances, err := s.loadInstancesWithWiring()
		if err != nil {
			return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
		}
		for _, inst := range instances {
			if inst.Title == initialMsg.SessionId {
				instance = inst
				break
			}
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

				case *sessionv1.TerminalData_CurrentPaneRequest:
					// NOTE: This handler is currently unused - browser clients use the WebSocket handler
					// (connectrpc_websocket.go) which intercepts streaming calls before they reach here.
					// This handler exists to satisfy the protobuf interface contract and could be used
					// by non-browser gRPC clients in the future.
					//
					// If this handler becomes active, the CurrentPaneRequest resize logic is implemented
					// in connectrpc_websocket.go:524-550 and should be synchronized here.
					log.WarningLog.Printf("[StreamTerminal] CurrentPaneRequest received (unexpected - WebSocket handler should intercept this)")

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

	// Update diff stats to get fresh data (the cached version may be stale or nil)
	if err := instance.UpdateDiffStats(); err != nil {
		log.WarningLog.Printf("Failed to update diff stats for session %s: %v", req.Msg.Id, err)
		// Continue anyway - we'll return empty stats if unavailable
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

		// Parse timestamp - use ParseInLocation with Local timezone since logs are written in local time
		timestampStr := fmt.Sprintf("%s %s", dateStr, timeStr)
		timestamp, err := time.ParseInLocation("2006/01/02 15:04:05", timestampStr, time.Local)
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
	return s.reviewQueueSvc.WatchReviewQueue(ctx, req, stream)
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
func (s *SessionService) LogUserInteraction(
	ctx context.Context,
	req *connect.Request[sessionv1.LogUserInteractionRequest],
) (*connect.Response[sessionv1.LogUserInteractionResponse], error) {
	return s.reviewQueueSvc.LogUserInteraction(ctx, req)
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

// ListClaudeHistory returns Claude session history entries with optional filtering.
func (s *SessionService) ListClaudeHistory(
	ctx context.Context,
	req *connect.Request[sessionv1.ListClaudeHistoryRequest],
) (*connect.Response[sessionv1.ListClaudeHistoryResponse], error) {
	return s.searchSvc.ListClaudeHistory(ctx, req)
}

// GetClaudeHistoryDetail retrieves detailed information for a specific history entry.
func (s *SessionService) GetClaudeHistoryDetail(
	ctx context.Context,
	req *connect.Request[sessionv1.GetClaudeHistoryDetailRequest],
) (*connect.Response[sessionv1.GetClaudeHistoryDetailResponse], error) {
	return s.searchSvc.GetClaudeHistoryDetail(ctx, req)
}

// GetClaudeHistoryMessages retrieves messages from a specific conversation.
func (s *SessionService) GetClaudeHistoryMessages(
	ctx context.Context,
	req *connect.Request[sessionv1.GetClaudeHistoryMessagesRequest],
) (*connect.Response[sessionv1.GetClaudeHistoryMessagesResponse], error) {
	return s.searchSvc.GetClaudeHistoryMessages(ctx, req)
}

// SearchClaudeHistory performs full-text search across Claude conversation history.
func (s *SessionService) SearchClaudeHistory(
	ctx context.Context,
	req *connect.Request[sessionv1.SearchClaudeHistoryRequest],
) (*connect.Response[sessionv1.SearchClaudeHistoryResponse], error) {
	return s.searchSvc.SearchClaudeHistory(ctx, req)
}

// GetPRInfo retrieves the latest PR information for a session.
func (s *SessionService) GetPRInfo(
	ctx context.Context,
	req *connect.Request[sessionv1.GetPRInfoRequest],
) (*connect.Response[sessionv1.GetPRInfoResponse], error) {
	return s.githubSvc.GetPRInfo(ctx, req)
}

// GetPRComments retrieves all comments on the PR for a session.
func (s *SessionService) GetPRComments(
	ctx context.Context,
	req *connect.Request[sessionv1.GetPRCommentsRequest],
) (*connect.Response[sessionv1.GetPRCommentsResponse], error) {
	return s.githubSvc.GetPRComments(ctx, req)
}

// PostPRComment posts a new comment to the PR for a session.
func (s *SessionService) PostPRComment(
	ctx context.Context,
	req *connect.Request[sessionv1.PostPRCommentRequest],
) (*connect.Response[sessionv1.PostPRCommentResponse], error) {
	return s.githubSvc.PostPRComment(ctx, req)
}

// MergePR merges the PR for a session using the specified merge method.
func (s *SessionService) MergePR(
	ctx context.Context,
	req *connect.Request[sessionv1.MergePRRequest],
) (*connect.Response[sessionv1.MergePRResponse], error) {
	return s.githubSvc.MergePR(ctx, req)
}

// ClosePR closes the PR without merging for a session.
func (s *SessionService) ClosePR(
	ctx context.Context,
	req *connect.Request[sessionv1.ClosePRRequest],
) (*connect.Response[sessionv1.ClosePRResponse], error) {
	return s.githubSvc.ClosePR(ctx, req)
}

// SendNotification allows tmux sessions and external Claude processes to send notifications.
// Enforces localhost-only restriction and rate limiting. Accepts both managed sessions
// and external sessions (e.g., Claude running in IntelliJ, VS Code, or other terminals).
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

	// Use the session ID as the display name. LoadInstances() cannot be used here because
	// it calls FromInstanceData() which calls Start() on every non-paused session —
	// a catastrophic side-effect that restarts all sessions on each notification.
	// The poller holds live instances; if the session exists there, use its title.
	sessionName := req.Msg.SessionId // Default to session ID
	if s.reviewQueuePoller != nil {
		if inst := s.reviewQueuePoller.FindInstance(req.Msg.SessionId); inst != nil {
			sessionName = inst.Title
		}
	}

	// Apply rate limiting (applies to both managed and external sessions)
	if !s.notificationRateLimiter.Allow(req.Msg.SessionId) {
		return nil, connect.NewError(connect.CodeResourceExhausted, fmt.Errorf("rate limit exceeded for session: %s", req.Msg.SessionId))
	}

	// Generate notification ID
	notificationID := uuid.New().String()

	// Broadcast notification via event bus
	event := events.NewNotificationEvent(
		req.Msg.SessionId,
		sessionName,
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
		"session_name":      sessionName,
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

// FocusWindow activates a window for the specified application.
// Uses AppleScript on macOS to bring the application to front.
func (s *SessionService) FocusWindow(
	ctx context.Context,
	req *connect.Request[sessionv1.FocusWindowRequest],
) (*connect.Response[sessionv1.FocusWindowResponse], error) {
	// Validate localhost-only origin
	if err := s.validateLocalhostOriginForFocus(ctx, req); err != nil {
		return nil, err
	}

	platform := detectPlatform()

	// Need at least bundle_id or app_name
	bundleID := ""
	if req.Msg.BundleId != nil {
		bundleID = *req.Msg.BundleId
	}
	appName := ""
	if req.Msg.AppName != nil {
		appName = *req.Msg.AppName
	}

	if bundleID == "" && appName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("bundle_id or app_name is required"))
	}

	// Only macOS is supported currently
	if platform != "darwin" {
		return connect.NewResponse(&sessionv1.FocusWindowResponse{
			Success:  false,
			Message:  fmt.Sprintf("window activation not supported on platform: %s", platform),
			Platform: platform,
		}), nil
	}

	// Try to activate the window using AppleScript
	var script string
	if bundleID != "" {
		// Prefer bundle ID for more reliable activation
		script = fmt.Sprintf(`tell application id "%s" to activate`, bundleID)
	} else {
		// Fallback to app name
		script = fmt.Sprintf(`tell application "%s" to activate`, appName)
	}

	// Execute AppleScript
	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	if err != nil {
		log.WarningLog.Printf("Failed to activate window (bundle=%s, app=%s): %v, output: %s",
			bundleID, appName, err, outputStr)

		// Check for common permission-related errors
		message := fmt.Sprintf("failed to activate window: %v", err)
		if strings.Contains(outputStr, "not allowed") ||
			strings.Contains(outputStr, "permission") ||
			strings.Contains(outputStr, "accessibility") ||
			strings.Contains(outputStr, "System Events") {
			message = "Permission denied. Please grant Accessibility permissions: " +
				"System Preferences > Security & Privacy > Privacy > Accessibility. " +
				"Add Terminal (or your terminal app) to the list."
		} else if strings.Contains(outputStr, "Application isn't running") ||
			strings.Contains(outputStr, "Can't get application") {
			targetApp := bundleID
			if targetApp == "" {
				targetApp = appName
			}
			message = fmt.Sprintf("Application '%s' is not running", targetApp)
		}

		return connect.NewResponse(&sessionv1.FocusWindowResponse{
			Success:  false,
			Message:  message,
			Platform: platform,
		}), nil
	}

	log.InfoLog.Printf("Window activated successfully (bundle=%s, app=%s)", bundleID, appName)
	return connect.NewResponse(&sessionv1.FocusWindowResponse{
		Success:  true,
		Message:  "Window activated successfully",
		Platform: platform,
	}), nil
}

// validateLocalhostOriginForFocus ensures FocusWindow requests come from localhost.
func (s *SessionService) validateLocalhostOriginForFocus(ctx context.Context, req *connect.Request[sessionv1.FocusWindowRequest]) error {
	// Check X-Real-IP header first (if behind a proxy)
	realIP := req.Header().Get("X-Real-IP")
	if realIP != "" {
		if !isLocalhostIP(realIP) {
			return connect.NewError(connect.CodePermissionDenied, fmt.Errorf("FocusWindow can only be called from localhost"))
		}
		return nil
	}

	// Check X-Forwarded-For header
	forwardedFor := req.Header().Get("X-Forwarded-For")
	if forwardedFor != "" {
		ips := strings.Split(forwardedFor, ",")
		if len(ips) > 0 {
			clientIP := strings.TrimSpace(ips[0])
			if !isLocalhostIP(clientIP) {
				return connect.NewError(connect.CodePermissionDenied, fmt.Errorf("FocusWindow can only be called from localhost"))
			}
			return nil
		}
	}

	// Direct connection mode - server binds to localhost
	return nil
}

// detectPlatform returns the current operating system.
func detectPlatform() string {
	switch os := os.Getenv("GOOS"); os {
	case "":
		// GOOS not set, use runtime detection
		return runtime.GOOS
	default:
		return os
	}
}

// RenameSession changes the title of an existing session.
// Validates that the new title doesn't conflict with existing sessions.
func (s *SessionService) RenameSession(
	ctx context.Context,
	req *connect.Request[sessionv1.RenameSessionRequest],
) (*connect.Response[sessionv1.RenameSessionResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	if req.Msg.NewTitle == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("new title is required"))
	}

	instances, err := s.loadInstancesWithWiring()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
	}

	// Find the instance to rename
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

	// Check if new title already exists (if different from current)
	if req.Msg.NewTitle != instance.Title {
		for _, inst := range instances {
			if inst.Title == req.Msg.NewTitle {
				return nil, connect.NewError(connect.CodeAlreadyExists,
					fmt.Errorf("session with title '%s' already exists", req.Msg.NewTitle))
			}
		}
	}

	// Rename the instance
	oldTitle := instance.Title
	if err := instance.Rename(req.Msg.NewTitle); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to rename session: %w", err))
	}

	// Update the instance in the list and save
	instances[instanceIndex] = instance
	if err := s.storage.SaveInstances(instances); err != nil {
		// Try to rollback the rename
		instance.Title = oldTitle
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save renamed instance: %w", err))
	}

	// Update the ReviewQueuePoller's instance references after renaming
	if s.reviewQueuePoller != nil {
		s.reviewQueuePoller.SetInstances(instances)
		log.InfoLog.Printf("[ReviewQueue] Updated poller instance references after RenameSession from '%s' to '%s'",
			oldTitle, req.Msg.NewTitle)
	}

	// Publish SessionUpdated event
	s.eventBus.Publish(events.NewSessionUpdatedEvent(instance, []string{"title"}))

	log.InfoLog.Printf("Successfully renamed session from '%s' to '%s'", oldTitle, req.Msg.NewTitle)

	return connect.NewResponse(&sessionv1.RenameSessionResponse{
		Session: adapters.InstanceToProto(instance),
	}), nil
}

// RestartSession restarts a session by killing and recreating the tmux session.
// Optionally preserves terminal output for debugging purposes.
func (s *SessionService) RestartSession(
	ctx context.Context,
	req *connect.Request[sessionv1.RestartSessionRequest],
) (*connect.Response[sessionv1.RestartSessionResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	instances, err := s.loadInstancesWithWiring()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
	}

	// Find the instance to restart
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

	// Restart the instance
	if err := instance.Restart(req.Msg.PreserveOutput); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to restart session: %w", err))
	}

	// Update the instance in the list and save
	instances[instanceIndex] = instance
	if err := s.storage.SaveInstances(instances); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save restarted instance: %w", err))
	}

	// Update the ReviewQueuePoller's instance references after restart
	if s.reviewQueuePoller != nil {
		s.reviewQueuePoller.SetInstances(instances)
		log.InfoLog.Printf("[ReviewQueue] Updated poller instance references after RestartSession for '%s'", instance.Title)
	}

	// Publish SessionUpdated event
	s.eventBus.Publish(events.NewSessionUpdatedEvent(instance, []string{"status", "updated_at"}))

	message := fmt.Sprintf("Session '%s' restarted successfully", instance.Title)
	if req.Msg.PreserveOutput {
		message += " (terminal output preserved)"
	}

	log.InfoLog.Printf("%s", message)

	return connect.NewResponse(&sessionv1.RestartSessionResponse{
		Session: adapters.InstanceToProto(instance),
		Success: true,
		Message: message,
	}), nil
}

// GetVCSStatus retrieves the current version control status for a session.
func (s *SessionService) GetVCSStatus(
	ctx context.Context,
	req *connect.Request[sessionv1.GetVCSStatusRequest],
) (*connect.Response[sessionv1.GetVCSStatusResponse], error) {
	return s.workspaceSvc.GetVCSStatus(ctx, req)
}

// GetWorkspaceInfo retrieves VCS and workspace information for a session.
func (s *SessionService) GetWorkspaceInfo(
	ctx context.Context,
	req *connect.Request[sessionv1.GetWorkspaceInfoRequest],
) (*connect.Response[sessionv1.GetWorkspaceInfoResponse], error) {
	return s.workspaceSvc.GetWorkspaceInfo(ctx, req)
}

// ListWorkspaceTargets returns available switch targets for a session.
func (s *SessionService) ListWorkspaceTargets(
	ctx context.Context,
	req *connect.Request[sessionv1.ListWorkspaceTargetsRequest],
) (*connect.Response[sessionv1.ListWorkspaceTargetsResponse], error) {
	return s.workspaceSvc.ListWorkspaceTargets(ctx, req)
}

// SwitchWorkspace switches a session's workspace to a different branch, revision, or worktree.
func (s *SessionService) SwitchWorkspace(
	ctx context.Context,
	req *connect.Request[sessionv1.SwitchWorkspaceRequest],
) (*connect.Response[sessionv1.SwitchWorkspaceResponse], error) {
	return s.workspaceSvc.SwitchWorkspace(ctx, req)
}

// CreateDebugSnapshot captures diagnostic information and writes a JSON file to the log directory.
func (s *SessionService) CreateDebugSnapshot(
	ctx context.Context,
	req *connect.Request[sessionv1.CreateDebugSnapshotRequest],
) (*connect.Response[sessionv1.CreateDebugSnapshotResponse], error) {
	snapCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Collect live instances
	var instances []*session.Instance
	if s.reviewQueuePoller != nil {
		instances = s.reviewQueuePoller.GetInstances()
	}

	// Determine log line count
	logLines := int32(200)
	if req.Msg.LogLines != nil && *req.Msg.LogLines > 0 {
		logLines = *req.Msg.LogLines
	}

	note := ""
	if req.Msg.Note != nil {
		note = *req.Msg.Note
	}

	// Collect snapshot
	snap := CollectSnapshot(snapCtx, note, instances, s.approvalStore, int(logLines))

	// Get log directory for output
	logDir, err := log.GetLogDir(log.ConfigToLogConfig(config.LoadConfig()))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get log directory: %w", err))
	}

	// Write snapshot to disk
	filePath, err := WriteSnapshot(snap, logDir)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to write snapshot: %w", err))
	}

	// Get file size
	var fileSizeBytes int64
	if info, err := os.Stat(filePath); err == nil {
		fileSizeBytes = info.Size()
	}

	// Build summary
	pendingApprovals := 0
	if s.approvalStore != nil {
		pendingApprovals = len(s.approvalStore.ListAll())
	}
	summary := fmt.Sprintf("Captured %d sessions, %d pending approvals, %d log lines",
		len(instances), pendingApprovals, snap.RecentLogs.LineCount)
	if len(snap.Errors) > 0 {
		summary += fmt.Sprintf(" (%d collection errors)", len(snap.Errors))
	}

	log.InfoLog.Printf("[DebugSnapshot] Written to %s (%d bytes)", filePath, fileSizeBytes)

	return connect.NewResponse(&sessionv1.CreateDebugSnapshotResponse{
		FilePath:      filePath,
		Summary:       summary,
		Timestamp:     snap.Timestamp.Format(time.RFC3339),
		FileSizeBytes: fileSizeBytes,
	}), nil
}
