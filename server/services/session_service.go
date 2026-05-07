package services

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/classifier"
	"github.com/tstapler/stapler-squad/server/adapters"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/notifications"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/namegen"
	"github.com/tstapler/stapler-squad/session/prompts"
	"github.com/tstapler/stapler-squad/session/search"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Compile-time interface check: SessionService must implement the full ConnectRPC handler.
var _ sessionv1connect.SessionServiceHandler = (*SessionService)(nil)

// ReactiveQueueManager is an interface to avoid circular dependencies.
// The actual implementation is in server/review_queue_manager.go
type ReactiveQueueManager interface {
	AddStreamClient(ctx context.Context, filters interface{}) (<-chan *sessionv1.ReviewQueueEvent, string)
	RemoveStreamClient(clientID string)
}

// SessionService implements the SessionServiceHandler interface for ConnectRPC.
type SessionService struct {
	storage           session.InstanceStore
	eventBus          *events.EventBus
	statusManager     *session.InstanceStatusManager
	reviewQueuePoller *session.ReviewQueuePoller

	// Extracted domain services.
	reviewQueueSvc  *ReviewQueueService
	searchSvc       *SearchService
	githubSvc       *GitHubService
	workspaceSvc    *WorkspaceService
	configSvc       *ConfigService
	notificationSvc *NotificationService
	approvalSvc     *ApprovalService
	utilitySvc      *UtilityService
	rulesSvc        *RulesService

	// External session discovery (for mux-enabled sessions from external terminals)
	externalDiscovery *session.ExternalSessionDiscovery

	// historyLinker tracks JSONL conversation files per session.
	// Must be kept in sync with deletions so the shutdown hook does not
	// re-persist sessions the user has deleted.
	historyLinker *session.HistoryLinker

	// approvalStore holds pending Claude Code hook approval requests.
	approvalStore *ApprovalStore

	// databaseSvc handles workspace/database switcher RPCs.
	databaseSvc *DatabaseService

	// fileSvc handles file tree browsing RPCs (ListFiles, GetFileContent).
	fileSvc *FileService

	// pathCompletionSvc handles filesystem path completion RPCs.
	pathCompletionSvc *PathCompletionService

	// defaultsSvc handles session defaults configuration RPCs.
	defaultsSvc *DefaultsService

	// projectSvc handles Project CRUD RPCs.
	projectSvc *ProjectService

	// promptStore persists prompt history for the "initial prompt" dropdown.
	promptStore *prompts.PromptStore

	// scrollbackMgr provides access to per-session scrollback sequence numbers
	// for checkpoint creation. May be nil if not wired (seq defaults to 0).
	scrollbackMgr scrollbackSequencer

	// mcpServerURL is the URL of the stapler-squad HTTP MCP endpoint.
	// When non-empty, passed to new sessions via InstanceOptions.MCPServerURL.
	mcpServerURL string

	// branchCache caches git branch lists per repo path. ADR-002.
	branchCache sync.Map // map[string]branchCacheEntry

	// errorRegistry persists deduplicated RPC errors to SQLite.
	// May be nil when wired without an ent-backed storage (e.g. in tests).
	errorRegistry *ErrorRegistry
}

// scrollbackSequencer is the minimal interface SessionService needs from ScrollbackManager.
type scrollbackSequencer interface {
	CurrentSequence(sessionID string) uint64
}

// NewSessionService creates a new SessionService with the given storage and event bus.
// NOTE: Instances are NOT loaded here to prevent double-loading and initialization timing issues.
// Instances will be loaded in server.go after dependencies (statusManager, reviewQueue) are wired.
func NewSessionService(storage session.InstanceStore, eventBus *events.EventBus) *SessionService {
	reviewQueue := session.NewReviewQueue()

	// concStorage is the concrete backing store used by sub-services that haven't migrated to
	// InstanceStore yet (ReviewQueueService, GitHubService, WorkspaceService). In tests using a
	// fake InstanceStore, concStorage will be nil — those sub-services degrade gracefully to nil storage.
	var concStorage *session.Storage
	if cs, ok := storage.(*session.Storage); ok {
		concStorage = cs
	}

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

	// Build approval store with disk persistence path
	approvalFilePath := ""
	configDir, configErr := config.GetConfigDir()
	if configErr == nil {
		approvalFilePath = configDir + "/pending_approvals.json"
	} else {
		log.WarningLog.Printf("Failed to get config dir for approval persistence: %v", configErr)
	}
	approvalStore := NewApprovalStore(approvalFilePath)
	reviewQueueSvc := NewReviewQueueService(reviewQueue, concStorage, eventBus)
	reviewQueueSvc.SetApprovalStore(approvalStore)

	notificationSvc := NewNotificationService(NewNotificationRateLimiter(10, 20), eventBus)
	approvalSvc := NewApprovalService(approvalStore)
	utilitySvc := NewUtilityService(approvalStore)

	// Build rules store, analytics store, and classifier for approval rules service.
	rulesStore, rulesErr := NewRulesStore(concStorage)
	if rulesErr != nil {
		log.WarningLog.Printf("Failed to load rules store, using empty store: %v", rulesErr)
		rulesStore = &RulesStore{storage: concStorage}
	}
	analyticsStore := NewAnalyticsStore(concStorage)
	analyticsStore.Start(context.Background())
	classifierObj := classifier.NewRuleBasedClassifier()
	// Merge user rules into the classifier.
	if userRules := rulesStore.ToRules(); len(userRules) > 0 {
		classifierObj.AddRules(userRules)
	}
	rulesSvc := NewRulesService(rulesStore, analyticsStore, classifierObj)

	workspaceSvc := NewWorkspaceService(concStorage, eventBus)

	return &SessionService{
		storage:           storage,
		eventBus:          eventBus,
		reviewQueueSvc:    reviewQueueSvc,
		searchSvc:         NewSearchService(searchEngine, search.NewSnippetGenerator(), 5*time.Minute),
		githubSvc:         NewGitHubService(concStorage),
		workspaceSvc:      workspaceSvc,
		configSvc:         NewConfigService(),
		notificationSvc:   notificationSvc,
		approvalSvc:       approvalSvc,
		utilitySvc:        utilitySvc,
		rulesSvc:          rulesSvc,
		approvalStore:     approvalStore,
		databaseSvc:       NewDatabaseService(),
		fileSvc:           NewFileService(workspaceSvc),
		pathCompletionSvc: NewPathCompletionService(),
		defaultsSvc:       NewDefaultsService(),
		projectSvc:        NewProjectService(concStorage),
		promptStore:       newPromptStore(),
	}
}

// newPromptStore creates a PromptStore backed by ~/.stapler-squad/prompts.json.
func newPromptStore() *prompts.PromptStore {
	dir, err := config.GetConfigDir()
	if err != nil {
		log.WarningLog.Printf("[PromptStore] Failed to get config dir: %v", err)
		return prompts.NewPromptStore(os.TempDir() + "/stapler-squad-prompts.json")
	}
	return prompts.NewPromptStore(dir + "/prompts.json")
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
		s.wireRateLimitCallbacks(inst)
	}

	return instances, nil
}

// NewSessionServiceFromConfig creates a SessionService using EntRepository as storage backend.
// On first startup, if the legacy state.json exists and Ent DB is empty, sessions are
// auto-migrated from JSON to Ent.
func NewSessionServiceFromConfig() (*SessionService, error) {
	// Write workspace metadata on startup so the workspace switcher can discover this workspace.
	config.EnsureWorkspaceMeta()

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

// NewSessionServiceWithEntClient creates a SessionService from a pre-existing *ent.Client.
// Use this when the caller already opened a database (e.g. in tests or when sharing a
// connection) and wants to bypass the config-based path discovery in NewSessionServiceFromConfig.
func NewSessionServiceWithEntClient(entClient *ent.Client) (*SessionService, error) {
	config.EnsureWorkspaceMeta()
	repo := session.NewEntRepositoryFromClient(entClient)
	storage, err := session.NewStorageWithRepository(repo)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage with provided ent client: %w", err)
	}
	eventBus := events.NewEventBus(100)
	return NewSessionService(storage, eventBus), nil
}

// GetStorage returns the concrete *session.Storage for components that haven't migrated to InstanceStore yet.
// Returns nil when SessionService was constructed with a fake InstanceStore (e.g., in unit tests).
// Prefer using the session.InstanceStore interface via GetInstanceStore() for new code.
func (s *SessionService) GetStorage() *session.Storage {
	if cs, ok := s.storage.(*session.Storage); ok {
		return cs
	}
	return nil
}

// GetInstanceStore returns the InstanceStore interface, suitable for both production and test code.
func (s *SessionService) GetInstanceStore() session.InstanceStore {
	return s.storage
}

// FindLiveInstance returns the live in-memory instance held by the ReviewQueuePoller,
// or nil if the poller is not wired or the session is not found. Use this instead of
// LoadInstances() for read-only and mutation operations that need the live instance
// (with its PTY handles and controller state).
func (s *SessionService) FindLiveInstance(id string) *session.Instance {
	if s.reviewQueuePoller == nil {
		return nil
	}
	return s.reviewQueuePoller.FindInstance(id)
}

// GetApprovalStore returns the approval store for wiring up the HTTP hook handler.
func (s *SessionService) GetApprovalStore() *ApprovalStore {
	return s.approvalStore
}

// GetClassifier returns the rule-based classifier for wiring up the ApprovalHandler.
func (s *SessionService) GetClassifier() *classifier.RuleBasedClassifier {
	if s.rulesSvc == nil {
		return nil
	}
	return s.rulesSvc.classifier
}

// GetAnalyticsStore returns the analytics store for wiring up the ApprovalHandler.
func (s *SessionService) GetAnalyticsStore() *AnalyticsStore {
	if s.rulesSvc == nil {
		return nil
	}
	return s.rulesSvc.analyticsStore
}

// SetErrorRegistry wires the ErrorRegistry so the service can expose ListErrors and
// AcknowledgeError RPCs.  Must be called before the first RPC request.
func (s *SessionService) SetErrorRegistry(r *ErrorRegistry) {
	s.errorRegistry = r
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

// SetMCPServerURL configures the HTTP MCP endpoint URL passed to new sessions.
// Call this during server startup after the listen address is known.
func (s *SessionService) SetMCPServerURL(url string) {
	s.mcpServerURL = url
}

// SetHistoryLinker wires the HistoryLinker so deleted sessions are also removed
// from it and cannot be re-persisted by the shutdown hook.
func (s *SessionService) SetHistoryLinker(hl *session.HistoryLinker) {
	s.historyLinker = hl
}

// SetReviewQueuePoller wires the ReviewQueuePoller so new/deleted sessions are
// added/removed from the poller and AcknowledgeSession updates poller references.
// Must be called during server startup before any session mutation RPCs are used.
func (s *SessionService) SetReviewQueuePoller(poller *session.ReviewQueuePoller) {
	s.reviewQueuePoller = poller
	s.reviewQueueSvc.SetReviewQueuePoller(poller)
	s.notificationSvc.SetReviewQueuePoller(poller)
	s.utilitySvc.SetReviewQueuePoller(poller)
}

// SetStatusManager wires the InstanceStatusManager so that instances loaded via
// loadInstancesWithWiring (e.g., fallback path in ListSessions) receive status tracking.
// Must be called during server startup.
func (s *SessionService) SetStatusManager(mgr *session.InstanceStatusManager) {
	s.statusManager = mgr
}

// SetExternalDiscovery sets the external session discovery for accessing mux-enabled sessions.
func (s *SessionService) SetExternalDiscovery(discovery *session.ExternalSessionDiscovery) {
	s.externalDiscovery = discovery
}

// SetNotificationStore sets the notification history store for the notification history RPCs
// and wires it into the approval service so resolved approvals are stamped with their decision.
func (s *SessionService) SetNotificationStore(store *notifications.NotificationHistoryStore) {
	s.notificationSvc.SetNotificationStore(store)
	s.approvalSvc.SetNotificationStore(store)
}

// GetNotificationStore returns the notification history store.
func (s *SessionService) GetNotificationStore() *notifications.NotificationHistoryStore {
	return s.notificationSvc.GetNotificationStore()
}

// SetConfigService wires the ConfigService for delegating config RPCs.
func (s *SessionService) SetConfigService(svc *ConfigService) {
	s.configSvc = svc
}

// ListSessions returns all sessions with optional filtering.
// This includes both managed sessions and external mux-enabled sessions.
// +api: session:list
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

	// Find instance by ID (UUID or legacy Title).
	for _, inst := range instances {
		if inst.MatchesID(req.Msg.Id) {
			return connect.NewResponse(&sessionv1.GetSessionResponse{
				Session: adapters.InstanceToProto(inst),
			}), nil
		}
	}

	return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.Id))
}

// CreateSession initializes a new AI agent session with tmux and git worktree.
// +api: session:create
func (s *SessionService) CreateSession(
	ctx context.Context,
	req *connect.Request[sessionv1.CreateSessionRequest],
) (*connect.Response[sessionv1.CreateSessionResponse], error) {
	// Validate required fields
	if req.Msg.Title == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("title is required"))
	}
	if !req.Msg.OneOff &&
		req.Msg.SessionType != sessionv1.SessionType_SESSION_TYPE_NEW_PROJECT &&
		req.Msg.Path == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("path is required"))
	}

	// Check if session with this title already exists.
	// Use ListInstanceData (raw DB rows) rather than LoadInstances to avoid
	// the side-effect of FromInstanceData calling Start() on every session.
	existing, err := s.storage.ListInstanceData()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list instances: %w", err))
	}
	for _, data := range existing {
		if data.Title == req.Msg.Title {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("session with title '%s' already exists", req.Msg.Title))
		}
	}

	// Resolve GitHub URLs to local paths (GOPATH-style: ~/.stapler-squad/repos/github.com/owner/repo)
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

	// One-off session: generate a fresh directory and override resolvedPath.
	if req.Msg.OneOff {
		cfg := config.LoadConfig()
		baseDir, err := cfg.OneOffBaseDirOrDefault()
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to resolve one_off_base_dir: %w", err))
		}
		generatedPath, err := namegen.GenerateAndCreate(baseDir, 10)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create one-off directory: %w", err))
		}
		resolvedPath = generatedPath
	}

	// Resolve session defaults (global → directory → profile), then apply explicit request fields on top.
	// skip_defaults bypasses this for scripted or explicit-empty sessions.
	program := req.Msg.Program
	autoYes := req.Msg.AutoYes
	if !req.Msg.SkipDefaults {
		cfg := config.LoadConfig()
		workingDir := req.Msg.WorkingDir
		if workingDir == "" {
			workingDir = resolvedPath
		}
		resolved := config.ResolveDefaults(cfg, workingDir, req.Msg.Profile)
		// Apply resolved defaults only for fields not explicitly set in the request.
		if program == "" {
			program = resolved.Program
		}
		if !autoYes && resolved.AutoYes {
			autoYes = true
		}
	}

	// Determine session type - use explicit session_type if provided, otherwise infer from fields
	sessionType := resolveSessionType(req.Msg, branch)

	// For Directory mode: if path does not exist and create_if_missing is not set, return
	// CodeNotFound so the frontend can show a confirmation dialog.
	if sessionType == session.SessionTypeDirectory {
		if _, err := os.Stat(resolvedPath); os.IsNotExist(err) {
			if !req.Msg.CreateIfMissing {
				return nil, connect.NewError(connect.CodeNotFound,
					fmt.Errorf("path does not exist: %s", resolvedPath))
			}
			// create_if_missing=true: fall through; setupFirstTimeWorktree handles creation
		}
	}

	// Build instance options
	instanceOpts := session.InstanceOptions{
		Title:            req.Msg.Title,
		Path:             resolvedPath,
		WorkingDir:       req.Msg.WorkingDir,
		Branch:           branch,
		Program:          program,
		AutoYes:          autoYes,
		Prompt:           req.Msg.Prompt,
		ExistingWorktree: req.Msg.ExistingWorktree,
		Category:         req.Msg.Category,
		SessionType:      sessionType,
		TmuxPrefix:       "", // Use default from config
		ResumeId:         req.Msg.ResumeId,
		OneShot:          req.Msg.OneShot,
		ProjectID:        req.Msg.ProjectId,
		MCPServerURL:     s.mcpServerURL,
		CreateIfMissing:  req.Msg.CreateIfMissing,
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
		log.ErrorLog.Printf("[CreateSession] failed to start session '%s': %v", instance.Title, err)
		log.ForSession(instance.Title).Error("[CreateSession] failed to start: %v", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to start session: %w", err))
	}

	// Wire rate limit event callbacks so detection/recovery fire server-level notifications.
	s.wireRateLimitCallbacks(instance)

	// Inject Claude Code HTTP hook config for remote approval from the web UI.
	// Non-fatal: session is fully functional even without this config.
	if err := InjectHookConfig(instance.GetEffectiveRootDir(), instance.Title); err != nil {
		log.WarningLog.Printf("[CreateSession] Failed to inject hook config for session '%s': %v", instance.Title, err)
	}

	// Save only the new instance to storage.
	// Using AddInstance avoids loading and re-writing all instances (which would call
	// FromInstanceData/Start on every session and replace live poller instances with
	// cold storage copies).
	if err := s.storage.AddInstance(instance); err != nil {
		// Cleanup on save failure
		if destroyErr := instance.Destroy(); destroyErr != nil {
			// Log cleanup error but return original save error
			log.ErrorLog.Printf("Failed to cleanup after save error: %v", destroyErr)
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save instance: %w", err))
	}

	// Add only the new session to the poller, preserving all existing live instances.
	// Using AddInstance (not SetInstances) avoids replacing live instances with cold copies.
	if s.reviewQueuePoller != nil {
		s.reviewQueuePoller.AddInstance(instance)
		log.InfoLog.Printf("[ReviewQueue] Added new session '%s' to poller", instance.Title)
	}

	// Record initial_prompt in prompt history so it appears in the recent-prompts dropdown.
	if req.Msg.InitialPrompt != "" {
		s.promptStore.RecordUsage(req.Msg.InitialPrompt)
	}

	// Publish SessionCreated event to all watchers
	s.eventBus.Publish(events.NewSessionCreatedEvent(instance))

	return connect.NewResponse(&sessionv1.CreateSessionResponse{
		Session: adapters.InstanceToProto(instance),
	}), nil
}

// resolveSessionType maps a CreateSessionRequest + resolved branch to a session.SessionType.
// Priority: one_off (always directory) > explicit session_type > inference from branch/existing_worktree.
func resolveSessionType(msg *sessionv1.CreateSessionRequest, branch string) session.SessionType {
	var st session.SessionType
	if msg.SessionType != sessionv1.SessionType_SESSION_TYPE_UNSPECIFIED {
		switch msg.SessionType {
		case sessionv1.SessionType_SESSION_TYPE_DIRECTORY:
			st = session.SessionTypeDirectory
		case sessionv1.SessionType_SESSION_TYPE_NEW_WORKTREE:
			st = session.SessionTypeNewWorktree
		case sessionv1.SessionType_SESSION_TYPE_EXISTING_WORKTREE:
			st = session.SessionTypeExistingWorktree
		case sessionv1.SessionType_SESSION_TYPE_NEW_PROJECT:
			st = session.SessionTypeNewProject
		default:
			st = session.SessionTypeDirectory
		}
	} else {
		st = session.SessionTypeDirectory
		if msg.ExistingWorktree != "" {
			st = session.SessionTypeExistingWorktree
		} else if branch != "" {
			st = session.SessionTypeNewWorktree
		}
	}
	if msg.OneOff {
		st = session.SessionTypeDirectory
	}
	return st
}

// UpdateSession modifies session properties (pause/resume, category, title).
// +api: session:update
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
		if inst.MatchesID(req.Msg.Id) {
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

	// Handle title update (before status change so rename is atomic with resume)
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

	// Handle category update
	if req.Msg.Category != nil {
		instance.Category = *req.Msg.Category
		updatedFields = append(updatedFields, "category")
	}

	// Handle tags update.
	// In proto3, an empty repeated field is indistinguishable from "not provided",
	// so clients send tags=[""] to clear all tags.
	if len(req.Msg.Tags) > 0 {
		tags := req.Msg.Tags
		if len(tags) == 1 && tags[0] == "" {
			tags = nil // Clear all tags
		}
		if err := instance.SetTags(tags); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update tags: %w", err))
		}
		updatedFields = append(updatedFields, "tags")
	}

	// Handle program update
	if req.Msg.Program != nil && *req.Msg.Program != "" && instance.Program != *req.Msg.Program {
		instance.Program = *req.Msg.Program
		updatedFields = append(updatedFields, "program")

		// If the session is running, restart it with the new program
		if instance.Status == session.Running {
			if err := instance.Restart(true); err != nil {
				log.ErrorLog.Printf("[UpdateSession] failed to restart session '%s' after program change: %v", instance.Title, err)
				log.ForSession(instance.Title).Error("[UpdateSession] failed to restart after program change: %v", err)
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to restart session after program change: %w", err))
			}
		}
	}

	// Handle working directory update
	if req.Msg.WorkingDir != nil {
		instance.WorkingDir = *req.Msg.WorkingDir
		updatedFields = append(updatedFields, "working_dir")
	}

	// Handle rate limit enabled toggle. SetRateLimitEnabled persists to the
	// struct field. Also apply to the live poller instance (which has a running
	// controller) so the change takes effect immediately without a restart.
	if req.Msg.RateLimitEnabled != nil {
		instance.SetRateLimitEnabled(*req.Msg.RateLimitEnabled)
		if s.reviewQueuePoller != nil {
			if liveInst := s.reviewQueuePoller.FindInstance(req.Msg.Id); liveInst != nil {
				liveInst.SetRateLimitEnabled(*req.Msg.RateLimitEnabled)
			}
		}
		updatedFields = append(updatedFields, "rate_limit_enabled")
	}

	// Handle status change (pause/resume) LAST - after all metadata updates.
	// This ensures that if Resume() fails, no partial metadata changes are persisted
	// (save only happens after all changes succeed).
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
			statusEvent := events.NewSessionStatusChangedEvent(instance, oldStatus, instance.Status)
			// Augment with terminal-detected status when a controller is active for this session
			if s.statusManager != nil {
				statusInfo := s.statusManager.GetStatus(instance)
				if statusInfo.IsControllerActive {
					statusEvent.DetectedStatus = statusInfo.ClaudeStatus.String()
					statusEvent.DetectedContext = statusInfo.StatusContext
				}
			}
			s.eventBus.Publish(statusEvent)
		}
		// Also publish general update event
		s.eventBus.Publish(events.NewSessionUpdatedEvent(instance, updatedFields))
	}

	return connect.NewResponse(&sessionv1.UpdateSessionResponse{
		Session: adapters.InstanceToProto(instance),
	}), nil
}

// DeleteSession stops and removes a session, cleaning up resources.
// +api: session:delete
func (s *SessionService) DeleteSession(
	ctx context.Context,
	req *connect.Request[sessionv1.DeleteSessionRequest],
) (*connect.Response[sessionv1.DeleteSessionResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	// Verify existence using raw data — no PTY side effects.
	// Match by Title OR UUID so that sessions created after UUID assignment are found correctly.
	dataSlice, err := s.storage.ListInstanceData()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list instances: %w", err))
	}
	sessionTitle := ""
	sessionUUID := req.Msg.Id // fallback: use the supplied ID if UUID not found
	for _, d := range dataSlice {
		if d.Title == req.Msg.Id || d.UUID == req.Msg.Id {
			sessionTitle = d.Title
			if d.UUID != "" {
				sessionUUID = d.UUID
			}
			break
		}
	}
	if sessionTitle == "" {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.Id))
	}

	// Remove from all pollers BEFORE deleting from storage. This is atomic from the
	// poller's perspective and closes the race window where external discovery could
	// re-add the session between storage deletion and the old LoadInstances() reload.
	// Use sessionTitle (not req.Msg.Id) — pollers index by title, and req.Msg.Id may be a UUID.
	s.removeFromAllPollers(sessionTitle)

	// Destroy tmux/git resources asynchronously so the RPC returns immediately
	// after storage deletion. Cleanup errors are non-fatal — they are logged and
	// do not affect the success response the caller receives.
	if inst := s.FindLiveInstance(sessionTitle); inst != nil {
		go func() {
			if err := inst.Destroy(); err != nil {
				log.WarningLog.Printf("Failed to cleanup session resources for '%s': %v", req.Msg.Id, err)
			}
		}()
	}

	// Delete from storage using Title (the storage key), not the client-supplied ID which may be a UUID.
	if err := s.storage.DeleteInstance(sessionTitle); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete instance from storage: %w", err))
	}

	// Publish SessionDeleted event to all watchers. Use UUID so the frontend
	// entity adapter (keyed by UUID) matches and tombstones the correct entry.
	s.eventBus.Publish(events.NewSessionDeletedEvent(sessionUUID))

	return connect.NewResponse(&sessionv1.DeleteSessionResponse{
		Success: true,
		Message: fmt.Sprintf("Session '%s' deleted successfully", req.Msg.Id),
	}), nil
}

// removeFromAllPollers removes the session from the review queue and all pollers.
// Call this before deleting from storage to close the race window where LoadInstances()
// could re-add the session via external discovery.
func (s *SessionService) removeFromAllPollers(id string) {
	if s.reviewQueueSvc != nil {
		s.reviewQueueSvc.GetQueue().Remove(id)
	}
	if s.reviewQueuePoller != nil {
		s.reviewQueuePoller.RemoveInstance(id)
	}
	// Remove from HistoryLinker so the shutdown hook cannot re-persist a
	// deleted session via historyLinker.Instances() → SaveInstances().
	if s.historyLinker != nil {
		s.historyLinker.RemoveInstance(id)
	}
}

// RemoveFromAllPollers is the exported version for use by MCP tools and other
// callers outside the services package that need to clean up after deletion.
func (s *SessionService) RemoveFromAllPollers(id string) {
	s.removeFromAllPollers(id)
}

// WatchSessions streams real-time session events (created/updated/deleted).
// Sends initial snapshot of all sessions, then subscribes to real-time updates.
// +api: session:watch
func (s *SessionService) WatchSessions(
	ctx context.Context,
	req *connect.Request[sessionv1.WatchSessionsRequest],
	stream *connect.ServerStream[sessionv1.SessionEvent],
) error {
	// Send initial snapshot using in-memory poller cache — avoids a full SQLite
	// scan on every new WatchSessions connection (same approach as ListSessions).
	var instances []*session.Instance
	if s.reviewQueuePoller != nil {
		instances = s.reviewQueuePoller.GetInstances()
	} else {
		var err error
		instances, err = s.loadInstancesWithWiring()
		if err != nil {
			return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
		}
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
			if inst.MatchesID(initialMsg.SessionId) {
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
		log.ErrorLog.Printf("[StreamSession] failed to get PTY reader for session '%s': %v", instance.Title, err)
		log.ForSession(instance.Title).Error("[StreamSession] failed to get PTY reader: %v", err)
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
	var ptyPaused bool            // Current PTY pause state

	// Goroutine 1: Read from PTY and send deltas to client (terminal output)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				errCh <- fmt.Errorf("panic in output goroutine: %v", r)
			}
		}()

		buf := make([]byte, 32*1024)
		for {
			// Block until unpaused rather than spinning.
			if ptyPaused {
				select {
				case <-streamCtx.Done():
					return
				case ptyPaused = <-pauseCh:
					if !ptyPaused {
						log.InfoLog.Printf("[FlowControl] PTY reading RESUMED for session %s", initialMsg.SessionId)
					}
				}
				continue
			}

			select {
			case <-streamCtx.Done():
				return
			case paused := <-pauseCh:
				ptyPaused = paused
				if paused {
					log.InfoLog.Printf("[FlowControl] PTY reading PAUSED for session %s", initialMsg.SessionId)
				}
			default:

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
		log.ForSession(initialMsg.SessionId).Error("StreamTerminal: stream error: %v", err)
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

	instance := s.findInstance(req.Msg.Id)
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
	return s.utilitySvc.GetLogs(ctx, req)
}

// WatchReviewQueue streams real-time review queue events.
func (s *SessionService) WatchReviewQueue(
	ctx context.Context,
	req *connect.Request[sessionv1.WatchReviewQueueRequest],
	stream *connect.ServerStream[sessionv1.ReviewQueueEvent],
) error {
	return s.reviewQueueSvc.WatchReviewQueue(ctx, req, stream)
}

// LogUserInteraction logs a user interaction event for audit trail and analytics.
func (s *SessionService) LogUserInteraction(
	ctx context.Context,
	req *connect.Request[sessionv1.LogUserInteractionRequest],
) (*connect.Response[sessionv1.LogUserInteractionResponse], error) {
	return s.reviewQueueSvc.LogUserInteraction(ctx, req)
}

// GetClaudeConfig retrieves a Claude configuration file by name.
func (s *SessionService) GetClaudeConfig(
	ctx context.Context,
	req *connect.Request[sessionv1.GetClaudeConfigRequest],
) (*connect.Response[sessionv1.GetClaudeConfigResponse], error) {
	return s.configSvc.GetClaudeConfig(ctx, req)
}

// ListClaudeConfigs returns all configuration files in the ~/.claude directory.
func (s *SessionService) ListClaudeConfigs(
	ctx context.Context,
	req *connect.Request[sessionv1.ListClaudeConfigsRequest],
) (*connect.Response[sessionv1.ListClaudeConfigsResponse], error) {
	return s.configSvc.ListClaudeConfigs(ctx, req)
}

// UpdateClaudeConfig updates a Claude configuration file with atomic write and backup.
func (s *SessionService) UpdateClaudeConfig(
	ctx context.Context,
	req *connect.Request[sessionv1.UpdateClaudeConfigRequest],
) (*connect.Response[sessionv1.UpdateClaudeConfigResponse], error) {
	return s.configSvc.UpdateClaudeConfig(ctx, req)
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
// +api: history:search
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
func (s *SessionService) SendNotification(
	ctx context.Context,
	req *connect.Request[sessionv1.SendNotificationRequest],
) (*connect.Response[sessionv1.SendNotificationResponse], error) {
	return s.notificationSvc.SendNotification(ctx, req)
}

// FocusWindow activates a window for the specified application.
func (s *SessionService) FocusWindow(
	ctx context.Context,
	req *connect.Request[sessionv1.FocusWindowRequest],
) (*connect.Response[sessionv1.FocusWindowResponse], error) {
	return s.utilitySvc.FocusWindow(ctx, req)
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
		if inst.MatchesID(req.Msg.Id) {
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

	// Prefer the live in-memory instance so Restart() sees the real started/status
	// state and avoids the LoadInstances side-effect of hot-restoring every session.
	instance := s.FindLiveInstance(req.Msg.Id)
	if instance == nil {
		// Fallback: session not yet tracked by the poller (e.g. brand-new or test).
		instances, err := s.loadInstancesWithWiring()
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
		}
		for _, inst := range instances {
			if inst.MatchesID(req.Msg.Id) {
				instance = inst
				break
			}
		}
	}

	if instance == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.Id))
	}

	// Restart the instance
	if err := instance.Restart(req.Msg.PreserveOutput); err != nil {
		log.ErrorLog.Printf("[RestartSession] failed to restart session '%s': %v", instance.Title, err)
		log.ForSession(instance.Title).Error("[RestartSession] failed to restart: %v", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to restart session: %w", err))
	}

	// Persist the updated instance state.
	if err := s.storage.SaveInstances([]*session.Instance{instance}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save restarted instance: %w", err))
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
// +api: workspace:list-targets
func (s *SessionService) ListWorkspaceTargets(
	ctx context.Context,
	req *connect.Request[sessionv1.ListWorkspaceTargetsRequest],
) (*connect.Response[sessionv1.ListWorkspaceTargetsResponse], error) {
	return s.workspaceSvc.ListWorkspaceTargets(ctx, req)
}

// SwitchWorkspace switches a session's workspace to a different branch, revision, or worktree.
// +api: workspace:switch
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
	return s.utilitySvc.CreateDebugSnapshot(ctx, req)
}

// GetNotificationHistory returns persisted notification history with optional filtering.
func (s *SessionService) GetNotificationHistory(
	ctx context.Context,
	req *connect.Request[sessionv1.GetNotificationHistoryRequest],
) (*connect.Response[sessionv1.GetNotificationHistoryResponse], error) {
	return s.notificationSvc.GetNotificationHistory(ctx, req)
}

// MarkNotificationRead marks specific notifications as read.
func (s *SessionService) MarkNotificationRead(
	ctx context.Context,
	req *connect.Request[sessionv1.MarkNotificationReadRequest],
) (*connect.Response[sessionv1.MarkNotificationReadResponse], error) {
	return s.notificationSvc.MarkNotificationRead(ctx, req)
}

// ClearNotificationHistory removes notifications from the history.
func (s *SessionService) ClearNotificationHistory(
	ctx context.Context,
	req *connect.Request[sessionv1.ClearNotificationHistoryRequest],
) (*connect.Response[sessionv1.ClearNotificationHistoryResponse], error) {
	return s.notificationSvc.ClearNotificationHistory(ctx, req)
}

// ResolveApproval allows the web UI to approve or deny a pending Claude Code tool use request.
func (s *SessionService) ResolveApproval(
	ctx context.Context,
	req *connect.Request[sessionv1.ResolveApprovalRequest],
) (*connect.Response[sessionv1.ResolveApprovalResponse], error) {
	return s.approvalSvc.ResolveApproval(ctx, req)
}

// ListPendingApprovals returns all pending Claude Code tool approval requests.
func (s *SessionService) ListPendingApprovals(
	ctx context.Context,
	req *connect.Request[sessionv1.ListPendingApprovalsRequest],
) (*connect.Response[sessionv1.ListPendingApprovalsResponse], error) {
	return s.approvalSvc.ListPendingApprovals(ctx, req)
}

// ListApprovalRules returns all auto-approval rules (user, seed, and claude-settings).
func (s *SessionService) ListApprovalRules(
	ctx context.Context,
	req *connect.Request[sessionv1.ListApprovalRulesRequest],
) (*connect.Response[sessionv1.ListApprovalRulesResponse], error) {
	return s.rulesSvc.ListApprovalRules(ctx, req)
}

// UpsertApprovalRule creates or updates a user-defined auto-approval rule.
func (s *SessionService) UpsertApprovalRule(
	ctx context.Context,
	req *connect.Request[sessionv1.UpsertApprovalRuleRequest],
) (*connect.Response[sessionv1.UpsertApprovalRuleResponse], error) {
	return s.rulesSvc.UpsertApprovalRule(ctx, req)
}

// DeleteApprovalRule removes a user-defined auto-approval rule by ID.
func (s *SessionService) DeleteApprovalRule(
	ctx context.Context,
	req *connect.Request[sessionv1.DeleteApprovalRuleRequest],
) (*connect.Response[sessionv1.DeleteApprovalRuleResponse], error) {
	return s.rulesSvc.DeleteApprovalRule(ctx, req)
}

// GetApprovalAnalytics returns aggregated analytics for classification decisions.
func (s *SessionService) GetApprovalAnalytics(
	ctx context.Context,
	req *connect.Request[sessionv1.GetApprovalAnalyticsRequest],
) (*connect.Response[sessionv1.GetApprovalAnalyticsResponse], error) {
	return s.rulesSvc.GetApprovalAnalytics(ctx, req)
}

// ListDatabases returns all discovered workspace databases with metadata.
func (s *SessionService) ListDatabases(
	ctx context.Context,
	req *connect.Request[sessionv1.ListDatabasesRequest],
) (*connect.Response[sessionv1.ListDatabasesResponse], error) {
	return s.databaseSvc.ListDatabases(ctx, req)
}

// GetCurrentDatabase returns metadata for the currently active workspace database.
func (s *SessionService) GetCurrentDatabase(
	ctx context.Context,
	req *connect.Request[sessionv1.GetCurrentDatabaseRequest],
) (*connect.Response[sessionv1.GetCurrentDatabaseResponse], error) {
	return s.databaseSvc.GetCurrentDatabase(ctx, req)
}

// SwitchDatabase switches to a different workspace database and restarts the server.
func (s *SessionService) SwitchDatabase(
	ctx context.Context,
	req *connect.Request[sessionv1.SwitchDatabaseRequest],
) (*connect.Response[sessionv1.SwitchDatabaseResponse], error) {
	return s.databaseSvc.SwitchDatabase(ctx, req)
}

// MergeDatabase copies sessions from a source workspace into the current database.
func (s *SessionService) MergeDatabase(
	ctx context.Context,
	req *connect.Request[sessionv1.MergeDatabaseRequest],
) (*connect.Response[sessionv1.MergeDatabaseResponse], error) {
	return s.databaseSvc.MergeDatabase(ctx, req)
}

// SetScrollbackManager wires a scrollback sequence provider for checkpoint creation.
func (s *SessionService) SetScrollbackManager(mgr scrollbackSequencer) {
	s.scrollbackMgr = mgr
}

// CreateCheckpoint captures the current state of a session as a named bookmark.
func (s *SessionService) CreateCheckpoint(
	ctx context.Context,
	req *connect.Request[sessionv1.CreateCheckpointRequest],
) (*connect.Response[sessionv1.CreateCheckpointResponse], error) {
	if req.Msg.SessionId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required"))
	}
	if req.Msg.Label == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("label is required"))
	}

	inst := s.findInstance(req.Msg.SessionId)
	if inst == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.SessionId))
	}

	var scrollbackSeq uint64
	if s.scrollbackMgr != nil {
		scrollbackSeq = s.scrollbackMgr.CurrentSequence(inst.Title)
	}

	cp, err := inst.CreateCheckpoint(req.Msg.Label, scrollbackSeq)
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}

	if err := s.storage.SaveInstances(s.allInstances()); err != nil {
		log.WarningLog.Printf("CreateCheckpoint: failed to persist checkpoint for '%s': %v", inst.Title, err)
	}

	return connect.NewResponse(&sessionv1.CreateCheckpointResponse{
		Checkpoint: checkpointToProto(cp),
	}), nil
}

// ListCheckpoints returns all checkpoints for the specified session.
func (s *SessionService) ListCheckpoints(
	ctx context.Context,
	req *connect.Request[sessionv1.ListCheckpointsRequest],
) (*connect.Response[sessionv1.ListCheckpointsResponse], error) {
	if req.Msg.SessionId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required"))
	}

	inst := s.findInstance(req.Msg.SessionId)
	if inst == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.SessionId))
	}

	checkpoints := inst.GetCheckpoints()
	protos := make([]*sessionv1.CheckpointProto, 0, len(checkpoints))
	for i := range checkpoints {
		protos = append(protos, checkpointToProto(&checkpoints[i]))
	}

	return connect.NewResponse(&sessionv1.ListCheckpointsResponse{
		Checkpoints: protos,
	}), nil
}

// ForkSession creates a new independent session branched from a checkpoint on an existing session.
func (s *SessionService) ForkSession(
	ctx context.Context,
	req *connect.Request[sessionv1.ForkSessionRequest],
) (*connect.Response[sessionv1.ForkSessionResponse], error) {
	if req.Msg.SessionId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required"))
	}
	if req.Msg.CheckpointId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("checkpoint_id is required"))
	}
	if req.Msg.NewTitle == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("new_title is required"))
	}
	if strings.Contains(req.Msg.NewTitle, "..") || strings.ContainsRune(req.Msg.NewTitle, '/') || strings.ContainsRune(req.Msg.NewTitle, os.PathSeparator) {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("new_title must not contain path separators or '..'"))
	}

	src := s.findInstance(req.Msg.SessionId)
	if src == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.SessionId))
	}

	if s.findInstance(req.Msg.NewTitle) != nil {
		return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("session with title %q already exists", req.Msg.NewTitle))
	}

	configDir, err := config.GetConfigDir()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get config dir: %w", err))
	}

	newInst, err := src.ForkFromCheckpoint(req.Msg.CheckpointId, req.Msg.NewTitle, configDir)
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}

	if err := s.storage.AddInstance(newInst); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("persist forked session: %w", err))
	}

	if s.reviewQueuePoller != nil {
		updatedInstances := append(s.reviewQueuePoller.GetInstances(), newInst)
		s.reviewQueuePoller.SetInstances(updatedInstances)
		log.InfoLog.Printf("[ReviewQueue] Updated poller instance references after ForkSession for '%s'", newInst.Title)
	}

	respProto := adapters.InstanceToProto(newInst)

	go func() {
		if startErr := newInst.Start(true); startErr != nil {
			log.WarningLog.Printf("ForkSession: failed to start forked session '%s': %v", newInst.Title, startErr)
			newInst.Status = session.Stopped
			if saveErr := s.storage.SaveInstances(s.allInstances()); saveErr != nil {
				log.WarningLog.Printf("ForkSession: failed to persist Stopped status for '%s': %v", newInst.Title, saveErr)
			}
		}
	}()

	s.eventBus.Publish(events.NewSessionCreatedEvent(newInst))

	return connect.NewResponse(&sessionv1.ForkSessionResponse{
		Session: respProto,
	}), nil
}

// ClearConversationState removes the stored Claude conversation UUID from a session
// so that the next Resume starts a fresh conversation instead of attempting --resume
// with a stale or path-mismatched UUID.
func (s *SessionService) ClearConversationState(
	ctx context.Context,
	req *connect.Request[sessionv1.ClearConversationStateRequest],
) (*connect.Response[sessionv1.ClearConversationStateResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("id is required"))
	}

	instance := s.FindLiveInstance(req.Msg.Id)
	if instance == nil {
		instances, err := s.loadInstancesWithWiring()
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
		}
		for _, inst := range instances {
			if inst.MatchesID(req.Msg.Id) {
				instance = inst
				break
			}
		}
	}
	if instance == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.Id))
	}

	instance.ClearConversationState()

	if err := s.storage.SaveInstances([]*session.Instance{instance}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to persist cleared state: %w", err))
	}

	log.InfoLog.Printf("Cleared conversation state for session '%s'", instance.Title)
	s.eventBus.Publish(events.NewSessionUpdatedEvent(instance, []string{"claude_session"}))

	return connect.NewResponse(&sessionv1.ClearConversationStateResponse{
		Success: true,
		Message: fmt.Sprintf("Conversation state cleared for session '%s'", instance.Title),
	}), nil
}

// ListPathCompletions returns filesystem entries matching the given path prefix.
func (s *SessionService) ListPathCompletions(
	ctx context.Context,
	req *connect.Request[sessionv1.ListPathCompletionsRequest],
) (*connect.Response[sessionv1.ListPathCompletionsResponse], error) {
	return s.pathCompletionSvc.ListPathCompletions(ctx, req)
}

// ListWorktrees returns the git worktrees for a given repository path.
func (s *SessionService) ListWorktrees(
	ctx context.Context,
	req *connect.Request[sessionv1.ListWorktreesRequest],
) (*connect.Response[sessionv1.ListWorktreesResponse], error) {
	return s.pathCompletionSvc.ListWorktrees(ctx, req)
}

// branchCacheEntry holds a cached branch list for a repository path.
type branchCacheEntry struct {
	branches []string
	cachedAt time.Time
}

const branchCacheTTL = 5 * time.Minute

// ListBranches returns the git branches for a given repository path.
// Results are cached per repo path with a 5-minute TTL. ADR-002.
func (s *SessionService) ListBranches(
	ctx context.Context,
	req *connect.Request[sessionv1.ListBranchesRequest],
) (*connect.Response[sessionv1.ListBranchesResponse], error) {
	repoPath := req.Msg.GetRepoPath()
	if repoPath == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("repo_path is required"))
	}

	// Normalize and validate the path: must resolve within the user's home directory.
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid repo_path: %w", err))
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("cannot determine home directory: %w", err))
	}
	if !strings.HasPrefix(absPath, homeDir+string(filepath.Separator)) && absPath != homeDir {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("repo_path must be within the user home directory"))
	}
	if _, err := os.Stat(absPath); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("repo_path does not exist: %w", err))
	}

	maxResults := int(req.Msg.GetMaxResults())
	if maxResults <= 0 {
		maxResults = 200
	}
	filter := req.Msg.GetFilter()

	// Serve from cache if still fresh.
	if entry, ok := s.branchCache.Load(absPath); ok {
		cached := entry.(branchCacheEntry)
		if time.Since(cached.cachedAt) < branchCacheTTL {
			branches := filterBranches(cached.branches, filter, maxResults)
			return connect.NewResponse(&sessionv1.ListBranchesResponse{
				Branches:   branches,
				TotalCount: int32(len(branches)),
				Truncated:  false,
			}), nil
		}
	}

	// Run git for-each-ref with a 2-second timeout.
	cmdCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	refSpec := "refs/heads"
	if req.Msg.GetIncludeRemote() {
		refSpec = "refs/"
	}
	cmd := exec.CommandContext(cmdCtx, "git", "-C", absPath, "for-each-ref", refSpec, "--format=%(refname:short)")

	var out bytes.Buffer
	cmd.Stdout = &out

	start := time.Now()
	runErr := cmd.Run()
	latencyMs := time.Since(start).Milliseconds()
	log.InfoLog.Printf("[ListBranches] branch_list_latency_ms=%d repo=%s", latencyMs, absPath)

	truncated := false
	if runErr != nil {
		if cmdCtx.Err() == context.DeadlineExceeded {
			// Timeout: return whatever partial output was collected.
			truncated = true
		} else {
			// git failed (not a git repo, etc.): return empty list, not an error.
			log.InfoLog.Printf("[ListBranches] git for-each-ref failed for %s: %v", absPath, runErr)
			return connect.NewResponse(&sessionv1.ListBranchesResponse{
				Branches:   []string{},
				TotalCount: 0,
				Truncated:  false,
			}), nil
		}
	}

	var all []string
	scanner := bufio.NewScanner(&out)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			all = append(all, line)
		}
	}

	// Cache the full unfiltered list.
	s.branchCache.Store(absPath, branchCacheEntry{branches: all, cachedAt: time.Now()})

	branches := filterBranches(all, filter, maxResults)
	return connect.NewResponse(&sessionv1.ListBranchesResponse{
		Branches:   branches,
		TotalCount: int32(len(branches)),
		Truncated:  truncated,
	}), nil
}

// filterBranches applies a case-insensitive substring filter and caps results at maxResults.
func filterBranches(all []string, filter string, maxResults int) []string {
	lowerFilter := strings.ToLower(filter)
	var result []string
	for _, b := range all {
		if filter == "" || strings.Contains(strings.ToLower(b), lowerFilter) {
			result = append(result, b)
			if len(result) >= maxResults {
				break
			}
		}
	}
	return result
}

// findInstance finds an instance by title using the live in-memory poller.
func (s *SessionService) findInstance(id string) *session.Instance {
	if s.reviewQueuePoller != nil {
		if inst := s.reviewQueuePoller.FindInstance(id); inst != nil {
			return inst
		}
	}
	if s.externalDiscovery != nil {
		if inst := s.externalDiscovery.GetSession(id); inst != nil {
			return inst
		}
	}
	return nil
}

// allInstances returns all managed (poller-tracked) live instances.
// External discovery sessions are intentionally excluded: they are persisted via
// the OnSessionAdded/OnSessionRemoved callbacks in dependencies.go, and including
// them here would re-persist deleted external sessions during unrelated operations
// such as CreateCheckpoint or ForkSession.
func (s *SessionService) allInstances() []*session.Instance {
	if s.reviewQueuePoller != nil {
		return s.reviewQueuePoller.GetInstances()
	}
	return nil
}

// ListFiles returns the immediate children of a directory in a session's worktree.
func (s *SessionService) ListFiles(
	ctx context.Context,
	req *connect.Request[sessionv1.ListFilesRequest],
) (*connect.Response[sessionv1.ListFilesResponse], error) {
	return s.fileSvc.ListFiles(ctx, req)
}

// GetFileContent retrieves the text content of a file in a session's worktree.
func (s *SessionService) GetFileContent(
	ctx context.Context,
	req *connect.Request[sessionv1.GetFileContentRequest],
) (*connect.Response[sessionv1.GetFileContentResponse], error) {
	return s.fileSvc.GetFileContent(ctx, req)
}

// ─── Session Defaults delegates ──────────────────────────────────────────────

// GetSessionDefaults returns the full session defaults configuration.
func (s *SessionService) GetSessionDefaults(ctx context.Context, req *connect.Request[sessionv1.GetSessionDefaultsRequest]) (*connect.Response[sessionv1.GetSessionDefaultsResponse], error) {
	return s.defaultsSvc.GetSessionDefaults(ctx, req)
}

// ResolveDefaults merges all default layers for the given working directory and profile.
func (s *SessionService) ResolveDefaults(ctx context.Context, req *connect.Request[sessionv1.ResolveDefaultsRequest]) (*connect.Response[sessionv1.ResolveDefaultsResponse], error) {
	return s.defaultsSvc.ResolveDefaults(ctx, req)
}

// UpdateGlobalDefaults replaces the global default fields.
func (s *SessionService) UpdateGlobalDefaults(ctx context.Context, req *connect.Request[sessionv1.UpdateGlobalDefaultsRequest]) (*connect.Response[sessionv1.UpdateGlobalDefaultsResponse], error) {
	return s.defaultsSvc.UpdateGlobalDefaults(ctx, req)
}

// UpsertProfile creates or updates a named profile.
func (s *SessionService) UpsertProfile(ctx context.Context, req *connect.Request[sessionv1.UpsertProfileRequest]) (*connect.Response[sessionv1.UpsertProfileResponse], error) {
	return s.defaultsSvc.UpsertProfile(ctx, req)
}

// DeleteProfile removes a named profile by name.
func (s *SessionService) DeleteProfile(ctx context.Context, req *connect.Request[sessionv1.DeleteProfileRequest]) (*connect.Response[sessionv1.DeleteProfileResponse], error) {
	return s.defaultsSvc.DeleteProfile(ctx, req)
}

// UpsertDirectoryRule creates or updates a directory rule.
func (s *SessionService) UpsertDirectoryRule(ctx context.Context, req *connect.Request[sessionv1.UpsertDirectoryRuleRequest]) (*connect.Response[sessionv1.UpsertDirectoryRuleResponse], error) {
	return s.defaultsSvc.UpsertDirectoryRule(ctx, req)
}

// DeleteDirectoryRule removes a directory rule by path.
func (s *SessionService) DeleteDirectoryRule(ctx context.Context, req *connect.Request[sessionv1.DeleteDirectoryRuleRequest]) (*connect.Response[sessionv1.DeleteDirectoryRuleResponse], error) {
	return s.defaultsSvc.DeleteDirectoryRule(ctx, req)
}

// SearchFiles performs a recursive name-substring search in a session's worktree.
func (s *SessionService) SearchFiles(
	ctx context.Context,
	req *connect.Request[sessionv1.SearchFilesRequest],
) (*connect.Response[sessionv1.SearchFilesResponse], error) {
	return s.fileSvc.SearchFiles(ctx, req)
}

// GetFileService returns the underlying FileService so callers can register
// additional HTTP handlers (e.g. the raw file download endpoint).
func (s *SessionService) GetFileService() *FileService {
	return s.fileSvc
}

// ─── Prompt History ───────────────────────────────────────────────────────────

// +api: session:list-prompt-history
// ListPromptHistory returns saved prompt history entries.
func (s *SessionService) ListPromptHistory(
	ctx context.Context,
	req *connect.Request[sessionv1.ListPromptHistoryRequest],
) (*connect.Response[sessionv1.ListPromptHistoryResponse], error) {
	entries, err := s.promptStore.Load()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load prompt history: %w", err))
	}
	protos := make([]*sessionv1.PromptHistoryEntry, len(entries))
	for i, e := range entries {
		protos[i] = &sessionv1.PromptHistoryEntry{
			Id:        e.ID,
			Text:      e.Text,
			Label:     e.Label,
			UsedCount: int32(e.UsedCount),
			LastUsed:  timestamppb.New(e.LastUsed),
		}
	}
	return connect.NewResponse(&sessionv1.ListPromptHistoryResponse{Entries: protos}), nil
}

// +api: session:delete-prompt-history
// DeletePromptHistory removes a saved prompt from history.
func (s *SessionService) DeletePromptHistory(
	ctx context.Context,
	req *connect.Request[sessionv1.DeletePromptHistoryRequest],
) (*connect.Response[sessionv1.DeletePromptHistoryResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("id is required"))
	}
	if err := s.promptStore.Delete(req.Msg.Id); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete prompt history entry: %w", err))
	}
	return connect.NewResponse(&sessionv1.DeletePromptHistoryResponse{}), nil
}

// ─── Batch Session Creation ───────────────────────────────────────────────────

// +api: session:batch-create
// BatchCreateSessions creates multiple sessions with bounded concurrency (max 3) and
// per-repo serialization to prevent git worktree races.
func (s *SessionService) BatchCreateSessions(
	ctx context.Context,
	req *connect.Request[sessionv1.BatchCreateSessionsRequest],
) (*connect.Response[sessionv1.BatchCreateSessionsResponse], error) {
	if len(req.Msg.Sessions) == 0 {
		return connect.NewResponse(&sessionv1.BatchCreateSessionsResponse{}), nil
	}

	// Server-side cap: never more than 3 concurrent worktree creations.
	maxConc := int(req.Msg.MaxConcurrency)
	if maxConc <= 0 || maxConc > 3 {
		maxConc = 3
	}

	// Pre-check: load existing sessions to detect title conflicts before spawning goroutines.
	existing, err := s.storage.LoadInstances()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
	}
	existingTitles := make(map[string]struct{}, len(existing))
	for _, inst := range existing {
		existingTitles[inst.Title] = struct{}{}
	}

	results := make([]*sessionv1.BatchCreateResult, len(req.Msg.Sessions))
	seenTitles := make(map[string]struct{}, len(req.Msg.Sessions))
	var pendingIdx []int

	// Validate and dedup all requests upfront; fail-fast invalid ones without goroutines.
	for i, sess := range req.Msg.Sessions {
		var earlyErr string
		switch {
		case sess.Title == "":
			earlyErr = "title is required"
		case sess.Path == "":
			earlyErr = "path is required"
		default:
			if _, exists := existingTitles[sess.Title]; exists {
				earlyErr = fmt.Sprintf("session '%s' already exists", sess.Title)
			} else if _, dup := seenTitles[sess.Title]; dup {
				earlyErr = fmt.Sprintf("duplicate title '%s' in batch request", sess.Title)
			}
		}
		if earlyErr != "" {
			results[i] = &sessionv1.BatchCreateResult{Success: false, Title: sess.Title, Error: earlyErr}
			continue
		}
		seenTitles[sess.Title] = struct{}{}
		pendingIdx = append(pendingIdx, i)
	}

	// Semaphore bounds concurrent goroutines.
	sem := make(chan struct{}, maxConc)

	// Per-repo mutex serializes git worktree creation within the same repo directory.
	var repoMutexes sync.Map // map[string]*sync.Mutex

	var wg sync.WaitGroup
	for _, idx := range pendingIdx {
		sess := req.Msg.Sessions[idx]
		wg.Add(1)
		go func(i int, batchReq *sessionv1.BatchSessionRequest) {
			defer wg.Done()

			sem <- struct{}{} // acquire slot
			defer func() { <-sem }()

			// Serialize worktree creation per repo to avoid git conflicts.
			muIface, _ := repoMutexes.LoadOrStore(batchReq.Path, &sync.Mutex{})
			repoLock := muIface.(*sync.Mutex)
			repoLock.Lock()
			defer repoLock.Unlock()

			createReq := connect.NewRequest(&sessionv1.CreateSessionRequest{
				Title:       batchReq.Title,
				Path:        batchReq.Path,
				WorkingDir:  batchReq.WorkingDir,
				Branch:      batchReq.Branch,
				Program:     batchReq.Program,
				Category:    batchReq.Category,
				AutoYes:     batchReq.AutoYes,
				SessionType: batchReq.SessionType,
				ProjectId:   batchReq.ProjectId,
			})

			resp, createErr := s.CreateSession(ctx, createReq)
			// Each goroutine writes to a distinct index, so no mutex needed.
			if createErr != nil {
				results[i] = &sessionv1.BatchCreateResult{
					Success: false,
					Title:   batchReq.Title,
					Error:   createErr.Error(),
				}
			} else {
				results[i] = &sessionv1.BatchCreateResult{
					Success:   true,
					Title:     batchReq.Title,
					SessionId: resp.Msg.Session.Id,
				}
			}
		}(idx, sess)
	}
	wg.Wait()

	// Tally final counts from results.
	var succeeded, failed int32
	for _, r := range results {
		if r == nil {
			continue
		}
		if r.Success {
			succeeded++
		} else {
			failed++
		}
	}

	return connect.NewResponse(&sessionv1.BatchCreateSessionsResponse{
		Results:   results,
		Succeeded: succeeded,
		Failed:    failed,
	}), nil
}

// ─── One-Shot ─────────────────────────────────────────────────────────────────

// +api: session:run-one-shot
// RunOneShot executes `claude -p <prompt>` in the session's worktree and returns
// the combined output along with an extracted PR URL and branch divergence status.
func (s *SessionService) RunOneShot(
	ctx context.Context,
	req *connect.Request[sessionv1.RunOneShotRequest],
) (*connect.Response[sessionv1.RunOneShotResponse], error) {
	if req.Msg.SessionId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required"))
	}
	if req.Msg.Prompt == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("prompt is required"))
	}

	inst := s.findInstance(req.Msg.SessionId)
	if inst == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.SessionId))
	}

	workDir := inst.GetEffectiveRootDir()
	if workDir == "" {
		workDir = inst.Path
	}

	// Clamp timeout: default 120 s, max 300 s.
	timeoutSecs := int(req.Msg.TimeoutSeconds)
	if timeoutSecs <= 0 {
		timeoutSecs = 120
	}
	if timeoutSecs > 300 {
		timeoutSecs = 300
	}

	claudeBin, err := exec.LookPath("claude")
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("claude binary not found in PATH: %w", err))
	}

	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSecs)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(runCtx, claudeBin, "-p", req.Msg.Prompt)
	cmd.Dir = workDir

	output, runErr := cmd.CombinedOutput()
	exitCode := 0
	errMsg := ""
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			errMsg = runErr.Error()
		}
	}

	outputStr := string(output)
	prURL := extractPRURL(outputStr)
	branchDiverged := checkBranchDivergence(workDir)

	// Persist the PR URL back to the session record so the GitHub badge appears.
	if prURL != "" {
		inst.GitHubPRURL = prURL
		if err := s.storage.SaveInstances(s.allInstances()); err != nil {
			log.WarningLog.Printf("RunOneShot: failed to persist PR URL for session '%s': %v", inst.Title, err)
		} else {
			s.eventBus.Publish(events.NewSessionUpdatedEvent(inst, []string{"github_pr_url"}))
		}
	}

	return connect.NewResponse(&sessionv1.RunOneShotResponse{
		Output:                 outputStr,
		Error:                  errMsg,
		ExitCode:               int32(exitCode),
		PrUrl:                  prURL,
		BranchDivergedFromBase: branchDiverged,
	}), nil
}

// extractPRURL scans the last 10 non-empty lines of output for a GitHub PR URL
// of the form https://github.com/…/pull/NNN.
func extractPRURL(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	start := len(lines) - 10
	if start < 0 {
		start = 0
	}
	for i := len(lines) - 1; i >= start; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.Contains(line, "github.com/") && strings.Contains(line, "/pull/") {
			for _, word := range strings.Fields(line) {
				if strings.Contains(word, "github.com/") && strings.Contains(word, "/pull/") {
					return strings.Trim(word, ".,;:\"'()")
				}
			}
		}
	}
	return ""
}

// checkBranchDivergence returns true when the current branch has commits not
// present on origin/HEAD (i.e., the branch has diverged / is ahead).
func checkBranchDivergence(workDir string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "rev-list", "--count", "origin/HEAD..HEAD")
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	count := strings.TrimSpace(string(out))
	return count != "" && count != "0"
}

// ─── Projects ─────────────────────────────────────────────────────────────────

// +api: project:create
// CreateProject creates a new project for grouping sessions.
func (s *SessionService) CreateProject(
	ctx context.Context,
	req *connect.Request[sessionv1.CreateProjectRequest],
) (*connect.Response[sessionv1.CreateProjectResponse], error) {
	return s.projectSvc.CreateProject(ctx, req)
}

// +api: project:list
// ListProjects returns all projects.
func (s *SessionService) ListProjects(
	ctx context.Context,
	req *connect.Request[sessionv1.ListProjectsRequest],
) (*connect.Response[sessionv1.ListProjectsResponse], error) {
	return s.projectSvc.ListProjects(ctx, req)
}

// +api: project:update
// UpdateProject updates an existing project's metadata.
func (s *SessionService) UpdateProject(
	ctx context.Context,
	req *connect.Request[sessionv1.UpdateProjectRequest],
) (*connect.Response[sessionv1.UpdateProjectResponse], error) {
	return s.projectSvc.UpdateProject(ctx, req)
}

// +api: project:delete
// DeleteProject removes a project (sessions are unassigned, not deleted).
func (s *SessionService) DeleteProject(
	ctx context.Context,
	req *connect.Request[sessionv1.DeleteProjectRequest],
) (*connect.Response[sessionv1.DeleteProjectResponse], error) {
	return s.projectSvc.DeleteProject(ctx, req)
}

// +api: project:assign-sessions
// AssignSessionsToProject assigns one or more sessions to a project.
func (s *SessionService) AssignSessionsToProject(
	ctx context.Context,
	req *connect.Request[sessionv1.AssignSessionsToProjectRequest],
) (*connect.Response[sessionv1.AssignSessionsToProjectResponse], error) {
	return s.projectSvc.AssignSessionsToProject(ctx, req)
}

// checkpointToProto converts a session.Checkpoint to a proto CheckpointProto.
func checkpointToProto(cp *session.Checkpoint) *sessionv1.CheckpointProto {
	if cp == nil {
		return nil
	}
	return &sessionv1.CheckpointProto{
		Id:             cp.ID,
		SessionId:      cp.SessionID,
		ParentId:       cp.ParentID,
		Label:          cp.Label,
		ScrollbackSeq:  cp.ScrollbackSeq,
		ScrollbackPath: cp.ScrollbackPath,
		ClaudeConvUuid: cp.ClaudeConvUUID,
		GitCommitSha:   cp.GitCommitSHA,
		Timestamp:      timestamppb.New(cp.Timestamp),
	}
}

// GetTerminalSnapshot returns the last N lines of terminal output for a session.
// Uses inst.Preview() for a read-only snapshot without requiring an active stream.
func (s *SessionService) GetTerminalSnapshot(
	ctx context.Context,
	req *connect.Request[sessionv1.GetTerminalSnapshotRequest],
) (*connect.Response[sessionv1.GetTerminalSnapshotResponse], error) {
	if req.Msg.SessionId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required"))
	}

	// Find the live instance via the poller (avoids loadInstancesWithWiring side effects)
	var inst *session.Instance
	if s.reviewQueuePoller != nil {
		inst = s.reviewQueuePoller.FindInstance(req.Msg.SessionId)
	}
	if inst == nil && s.externalDiscovery != nil {
		inst = s.externalDiscovery.GetSession(req.Msg.SessionId)
	}
	if inst == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.SessionId))
	}

	content, err := inst.Preview()
	if err != nil {
		// Non-fatal: return empty snapshot rather than error
		log.WarningLog.Printf("[GetTerminalSnapshot] Preview failed for session %s: %v", req.Msg.SessionId, err)
		content = ""
	}

	// Trim to last N lines
	lastN := int(req.Msg.LastNLines)
	if lastN <= 0 {
		lastN = 20
	}
	lines := strings.Split(content, "\n")
	if len(lines) > lastN {
		lines = lines[len(lines)-lastN:]
	}
	content = strings.Join(lines, "\n")

	return connect.NewResponse(&sessionv1.GetTerminalSnapshotResponse{
		Content: content,
		IsEmpty: strings.TrimSpace(content) == "",
	}), nil
}

// +api: session:log-client-events
// LogClientEvents receives batched browser console log entries from the web UI.
// Used for remote debugging of mobile browser sessions where DevTools are unavailable.
// Never returns an error — malformed or oversized entries are silently discarded.
func (s *SessionService) LogClientEvents(
	_ context.Context,
	req *connect.Request[sessionv1.LogClientEventsRequest],
) (*connect.Response[sessionv1.LogClientEventsResponse], error) {
	for _, entry := range req.Msg.GetEntries() {
		logClientEntry(entry)
	}
	return connect.NewResponse(&sessionv1.LogClientEventsResponse{}), nil
}

// wireRateLimitCallbacks registers server-level callbacks on an Instance so that
// rate-limit detection and recovery events are published to the event bus and
// trigger desktop push notifications.
func (s *SessionService) wireRateLimitCallbacks(inst *session.Instance) {
	if inst == nil {
		return
	}
	inst.SetRateLimitCallbacks(
		// onDetected: called when rate limit is detected.
		func(sessionID string, resetTime time.Time) {
			var resetMsg string
			if !resetTime.IsZero() {
				resetMsg = fmt.Sprintf(" — resumes at %s", resetTime.Format("3:04 PM"))
			}
			title := fmt.Sprintf("Session \"%s\" rate limited%s", inst.Title, resetMsg)
			notifID := fmt.Sprintf("rl-detect-%s", sessionID)
			s.eventBus.Publish(events.NewNotificationEvent(
				sessionID, inst.Title, notifID,
				int32(8), // NotificationType_WARNING
				int32(3), // NotificationPriority_HIGH
				title,
				fmt.Sprintf("Session hit the usage limit%s.", resetMsg),
				nil,
			))
			s.eventBus.Publish(events.NewSessionUpdatedEvent(inst, []string{"rate_limit_state", "rate_limit_reset_time"}))
		},
		// onRecovery: called when recovery completes (success or failure).
		func(sessionID string, success bool, errMsg string) {
			var title, message string
			notifID := fmt.Sprintf("rl-recover-%s", sessionID)
			if success {
				title = fmt.Sprintf("Session \"%s\" resumed after rate limit", inst.Title)
				message = "Session auto-resumed after rate limit expiry."
			} else {
				title = fmt.Sprintf("Session \"%s\" failed to resume after rate limit", inst.Title)
				message = fmt.Sprintf("Auto-resume failed: %s", errMsg)
			}
			notifType := int32(10) // NotificationType_INFO
			if !success {
				notifType = int32(9) // NotificationType_FAILURE
			}
			s.eventBus.Publish(events.NewNotificationEvent(
				sessionID, inst.Title, notifID,
				notifType,
				int32(2), // NotificationPriority_MEDIUM
				title, message, nil,
			))
			s.eventBus.Publish(events.NewSessionUpdatedEvent(inst, []string{"rate_limit_state"}))
		},
	)
}


// logClientEntry writes a single browser log entry to the server log.
func logClientEntry(e *sessionv1.ClientLogEntry) {
	msg := sanitizeClientLogField(e.GetMessage(), 200)
	ua := sanitizeClientLogField(e.GetUserAgent(), 80)
	sid := sanitizeClientLogField(e.GetSessionId(), 64)
	lvl := sanitizeClientLogField(e.GetLevel(), 16)
	url := sanitizeClientLogField(e.GetUrl(), 256)

	logger := log.InfoLog
	if lvl == "error" {
		logger = log.ErrorLog
	}
	logger.Printf("[client-log] %s session=%s %s (url: %s ua: %s)", lvl, sid, msg, url, ua)
}

// sanitizeClientLogField strips control characters and truncates to maxLen runes.
func sanitizeClientLogField(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	runes := []rune(s)
	if len(runes) > maxLen {
		return string(runes[:maxLen]) + "…"
	}
	return s
}

// +api: errors:list
// ListErrors returns persisted RPC error events from SQLite, ordered by last_seen desc.
func (s *SessionService) ListErrors(
	ctx context.Context,
	req *connect.Request[sessionv1.ListErrorsRequest],
) (*connect.Response[sessionv1.ListErrorsResponse], error) {
	if s.errorRegistry == nil {
		return connect.NewResponse(&sessionv1.ListErrorsResponse{}), nil
	}
	events, err := s.errorRegistry.List(ctx, req.Msg.GetIncludeAcknowledged())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	records := make([]*sessionv1.ErrorEventRecord, 0, len(events))
	for _, e := range events {
		rec := &sessionv1.ErrorEventRecord{
			Fingerprint:     e.Fingerprint,
			ErrorType:       e.ErrorType,
			Message:         e.Message,
			StackTrace:      e.StackTrace,
			RpcProcedure:    e.RPCProcedure,
			OccurrenceCount: int32(e.OccurrenceCount),
			Acknowledged:    e.Acknowledged,
		}
		if !e.FirstSeen.IsZero() {
			rec.FirstSeen = timestamppb.New(e.FirstSeen)
		}
		if !e.LastSeen.IsZero() {
			rec.LastSeen = timestamppb.New(e.LastSeen)
		}
		records = append(records, rec)
	}
	return connect.NewResponse(&sessionv1.ListErrorsResponse{Errors: records}), nil
}

// +api: errors:acknowledge
// AcknowledgeError marks a persisted error event as acknowledged.
func (s *SessionService) AcknowledgeError(
	ctx context.Context,
	req *connect.Request[sessionv1.AcknowledgeErrorRequest],
) (*connect.Response[sessionv1.AcknowledgeErrorResponse], error) {
	if s.errorRegistry == nil {
		return connect.NewResponse(&sessionv1.AcknowledgeErrorResponse{}), nil
	}
	if err := s.errorRegistry.Acknowledge(ctx, req.Msg.GetFingerprint()); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&sessionv1.AcknowledgeErrorResponse{}), nil
}
