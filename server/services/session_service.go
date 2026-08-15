package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/classifier"
	"github.com/tstapler/stapler-squad/server/adapters"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/notifications"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/headless"
	"github.com/tstapler/stapler-squad/session/namegen"
	"github.com/tstapler/stapler-squad/session/prompts"
	"github.com/tstapler/stapler-squad/session/search"
	"github.com/tstapler/stapler-squad/session/tmux"
	"github.com/tstapler/stapler-squad/session/tokens"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Compile-time interface check: SessionService must implement the full ConnectRPC handler.
var _ sessionv1connect.SessionServiceHandler = (*SessionService)(nil)

// resumeIDRe validates the client-supplied resume_id field: must be a standard UUID.
// createSessionTimeout bounds the synchronous portion of CreateSession (path
// resolution, GitHub URL clone). It must stay comfortably above the slowest
// known synchronous sub-operation — GitHub URL resolution can shell out to
// `git clone`, which research puts at up to ~120s for large repos — so a
// legitimate slow-but-successful create still completes. NOTE: this is
// decoupled from the tmux startup poll (~10s), which runs in a background
// goroutine *after* the RPC returns and is intentionally not bound by this
// deadline; if that internal bound is retuned, revisit this value too.
const createSessionTimeout = 150 * time.Second

var resumeIDRe = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// ReactiveQueueManager is an interface to avoid circular dependencies.
// The actual implementation is in server/review_queue_manager.go
type ReactiveQueueManager interface {
	AddStreamClient(ctx context.Context, filters interface{}) (<-chan *sessionv1.ReviewQueueEvent, string)
	RemoveStreamClient(clientID string)
	OnControllerStatusChange(inst *session.Instance, newStatus detection.DetectedStatus)
}

// FeatureController is implemented by components that can be enabled/disabled at runtime.
// Used by GetFeatureFlags/UpdateFeatureFlag to toggle named subsystems.
type FeatureController interface {
	Enable(ctx context.Context) error
	Disable() error
	IsEnabled() bool
}

// SessionService implements the SessionServiceHandler interface for ConnectRPC.
type SessionService struct {
	storage           session.InstanceStore
	eventBus          *events.EventBus
	statusManager     *session.InstanceStatusManager
	reviewQueuePoller *session.ReviewQueuePoller

	// concStorage is the concrete backing store, used for operations (like
	// ListWorkspacePeers) not part of the InstanceStore interface. nil when storage is a
	// fake InstanceStore (tests) — callers must nil-check.
	concStorage *session.Storage

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

	// tmuxStreamerManager caches ExternalTmuxStreamer instances per tmux session
	// name (main sessions and shell siblings alike). Wired in so StopShell can
	// evict a shell's streamer on close instead of letting a stale/degraded one
	// persist across shell restarts.
	tmuxStreamerManager *session.ExternalTmuxStreamerManager

	// userPRCache supplies enterprise hosts from dynamically-added GitHub
	// accounts (gh CLI import, device auth) for CreateSession's GitHub URL
	// detection — mirrors ListGitHubAccounts' host union in github_user_service.go.
	userPRCache *githubpkg.UserPRCache

	// fileSvc handles file tree browsing RPCs (ListFiles, GetFileContent).
	fileSvc *FileService

	// pathCompletionSvc handles filesystem path completion RPCs.
	pathCompletionSvc *PathCompletionService

	// slashCommandSvc resolves slash commands from disk and built-ins.
	slashCommandSvc *SlashCommandService

	// defaultsSvc handles session defaults configuration RPCs.
	defaultsSvc *DefaultsService

	// callbackConfigSvc handles GetCallbackConfig/UpdateCallbackConfig RPCs
	// (webhook-triggers Phase 5, FR7).
	callbackConfigSvc *CallbackConfigService

	// launcherPresetsSvc handles the GetLauncherPresets RPC.
	launcherPresetsSvc *LauncherPresetsService

	// projectSvc handles Project CRUD RPCs.
	projectSvc *ProjectService

	// checkpointSvc handles CreateCheckpoint/ListCheckpoints/ClearConversationState RPCs.
	checkpointSvc *CheckpointService

	// featureFlagSvc handles GetFeatureFlags/UpdateFeatureFlag RPCs.
	featureFlagSvc *FeatureFlagService

	// terminalSvc handles GetTerminalSnapshot/WriteToSession RPCs.
	terminalSvc *TerminalService

	// promptStore persists prompt history for the "initial prompt" dropdown.
	promptStore *prompts.PromptStore

	// scrollbackMgr provides access to per-session scrollback sequence numbers
	// for checkpoint creation. May be nil if not wired (seq defaults to 0).
	scrollbackMgr ScrollbackSequencer

	// mcpServerURLFn lazily resolves the URL of the stapler-squad HTTP MCP
	// endpoint. It is invoked (not read as a stored string) at the point of
	// use so it always reflects the server's real bound address, even when
	// the listener was constructed before Start() resolved an OS-assigned
	// port (PORT=0). When non-nil, its result is passed to new sessions via
	// InstanceOptions.MCPServerURL.
	mcpServerURLFn func() string

	// errorRegistry persists deduplicated RPC errors to SQLite.
	// May be nil when wired without an ent-backed storage (e.g. in tests).
	errorRegistry *ErrorRegistry

	// backlogLifecycleListener is wired to each newly created session so that
	// backlog item state transitions fire when the session exits.
	backlogLifecycleListener *session.BacklogLifecycleListener

	// sessionSummaryGenerator is wired to each newly created session (alongside
	// backlogLifecycleListener, at the same call sites) so that session-completion-
	// summary generation fires on exit/stop. Nil until SetSessionSummaryGenerator is
	// called (session/session_summary_service.go's ent-client/headless-pool wiring
	// happens after SessionService construction).
	sessionSummaryGenerator *session.SessionSummaryGenerator

	// capacityMonitor tracks rate limits and triggers transitions.
	capacityMonitor *CapacityMonitor

	// quotaGate feeds account-wide rate-limit detections into the quota
	// headroom gate that pauses/resumes backlog automation. Late-wired via
	// SetQuotaGate since QuotaGate needs backlogCtrl, which doesn't exist yet
	// inside NewSessionService.
	quotaGate *QuotaGate

	// analyticsClient is the ent client for the analytics database (escape events, etc.).
	// May be nil when escape analytics is disabled or in tests that don't need it.
	analyticsClient *ent.Client

	// memoryCacheReader provides per-session RSS and system memory percentage.
	// Wired to the HibernationSweeper after startup. May be nil (fields default to 0).
	memoryCacheReader session.MemoryCacheReader

	// headlessPool is the shared LLM pool for non-interactive AI calls (RunOneShot, etc.).
	// May be nil when the claude binary is not found at startup.
	headlessPool *headless.Pool

	// autonomousSvc manages the lifecycle of AutonomousDriver instances.
	autonomousSvc *AutonomousOrchestrationService

	// workflowSvc handles workflow CRUD and RunWorkflow RPC delegation.
	// Injected after construction via SetWorkflowService to avoid bootstrapping cycle.
	workflowSvc *WorkflowService

	// workflowRepo is used to populate the workflow meta cache.
	// Injected via SetWorkflowRepository to avoid bootstrapping cycle.
	workflowRepo session.WorkflowRepository

	// workflowMetaCache provides workflow name and retention settings keyed by workflow UUID.
	// Populated on startup and refreshed every minute. Protected by workflowMetaMu.
	workflowMetaCache map[string]workflowMeta
	workflowMetaMu    sync.RWMutex

	// registry is the live-handle map for all running sessions. Wired after construction
	// via SetRegistry so that NewSessionService callers don't need to supply it at build time.
	registry *session.Registry

	// deleteCleanupWG tracks DeleteSession's background tmux/worktree cleanup
	// goroutines so Shutdown can await them instead of letting them outlive the
	// process (or, in tests, outlive the test that spawned them).
	deleteCleanupWG sync.WaitGroup

	// deleteCleanupMu guards deleteCleanupClosed and serializes it against
	// deleteCleanupWG.Add via trackCleanup, so Add never races Shutdown's Wait —
	// see deleteCleanupClosed's doc comment.
	deleteCleanupMu sync.Mutex

	// deleteCleanupClosed is set once Shutdown begins draining deleteCleanupWG.
	// sync.WaitGroup requires that any Add with a positive delta happen before
	// the matching Wait call is invoked (or after a prior Wait returns) —
	// calling Add concurrently with Wait when the counter may be zero is a
	// documented misuse that can panic or let Wait return before the new work
	// finishes. trackCleanup checks this flag under deleteCleanupMu before
	// calling Add; Shutdown sets it under the same mutex before calling Wait.
	// That makes the two mutually exclusive: any Add() that wins the lock race
	// happens-before closed is set true (and thus happens-before Wait()), and
	// any Add() that loses sees closed=true and never calls Add at all — it
	// runs the cleanup untracked instead, which is fine because Shutdown no
	// longer needs to wait for it.
	deleteCleanupClosed bool
}

// trackCleanup runs fn in a goroutine tracked by deleteCleanupWG so Shutdown
// can await it, unless Shutdown has already begun draining the WaitGroup — see
// deleteCleanupClosed's doc comment for why Add can't be allowed to race Wait.
func (s *SessionService) trackCleanup(fn func()) {
	s.deleteCleanupMu.Lock()
	if s.deleteCleanupClosed {
		s.deleteCleanupMu.Unlock()
		go fn()
		return
	}
	s.deleteCleanupWG.Add(1)
	s.deleteCleanupMu.Unlock()
	go func() {
		defer s.deleteCleanupWG.Done()
		fn()
	}()
}

// deleteSessionCleanupTimeout bounds how long DeleteSession's background
// liveInst.Destroy() cleanup (tmux kill, git diff stats, and worktree
// filesystem cleanup) is awaited before Shutdown/deleteCleanupWG.Wait() gives
// up on it. Destroy() takes no context and cannot be forcibly cancelled, so a
// timed-out cleanup keeps running in its own goroutine (untracked by the
// WaitGroup) after this deadline — only the *wait* is bounded, not the
// underlying work. Set well above KillTmuxSessionByTitle's 5s cap (which
// bounds a single subprocess call) because this timeout spans that same kill
// plus git-diff and worktree cleanup on the same instance, which can
// legitimately take longer on large repos under normal (non-hung) load.
const deleteSessionCleanupTimeout = 30 * time.Second

// destroyWithTimeout runs inst.Destroy() and waits up to timeout for it to
// finish. If it doesn't finish in time, this returns a timeout error immediately
// while Destroy() continues running in the background goroutine it was started
// in — see deleteSessionCleanupTimeout's doc comment for why the bound is on
// the wait, not the work itself.
func destroyWithTimeout(inst *session.Instance, timeout time.Duration) error {
	done := make(chan error, 1)
	go func() {
		done <- inst.Destroy()
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("timed out after %s waiting for session cleanup to finish (still running in background)", timeout)
	}
}

// workflowMeta holds cached metadata about a workflow used at session-list time.
type workflowMeta struct {
	name              string
	archiveAfterHours int
}

// ScrollbackSequencer is the minimal interface SessionService needs from ScrollbackManager.
// Exported so server/dependencies.go can use warren.Set to validate this wiring at startup.
type ScrollbackSequencer interface {
	CurrentSequence(sessionID string) uint64
}

// Dependency-injection audit (ADR-001, Story 1.3):
//
// All Set* wiring methods on SessionService inject Phase 2 or Phase 3 dependencies —
// none can be moved to constructor injection. Rationale by group:
//
//   Phase 2 (from server/dependencies.go after NewSessionService returns):
//     SetErrorRegistry      — built from storage.GetEntClient() after storage is opened
//     SetAnalyticsClient    — same ent client, pre-existing connection path
//     SetNotificationStore  — built from config dir (lazy path resolution)
//     SetConfigService      — thin wrapper, wired for test-overridability
//     SetMCPServerURL       — env-var / config value, not available inside package
//     SetBacklogLifecycleListener — depends on headless.Pool built after this service
//     SetHistoryLinker      — depends on storage + ent client both resolved
//     SetHeadlessPool       — constructed by BuildDependencies, not NewSessionService
//
//   Phase 3 (via warren.Set in BuildDependencies, after all Phase 2 deps):
//     SetReviewQueuePoller   — circular: poller takes SessionService as param
//     SetStatusManager       — built after reviewQueuePoller is available
//     SetExternalDiscovery   — built after headless pool
//     SetScrollbackManager   — built after storage is opened + log dir resolved
//     SetMemoryCacheReader   — optional, provided by HibernationSweeper
//     SetReactiveQueueManager — built after reviewQueuePoller + statusManager
//     SetLifecycleContext    — application-level ctx not available at pkg init
//     SetFeatureController   — one call per named flag (backlog), after backlog ctrl built
//     SetWorkflowService     — WorkflowService depends on SessionService (circular)
//     SetWorkflowRepository  — same cycle; repo available after workflowSvc constructed
//     SetTokenStoreReader    — built after storage open, wired late by warren
//
// Conclusion: the two-param constructor (storage, eventBus) is the correct boundary.
// All other deps are external to this package or have construction-order constraints.

// NewSessionService creates a new SessionService with the given storage and event bus,
// using a disk-backed search engine (or an in-memory one under config.IsTestMode(), to
// keep the ~78 existing test call sites working without change — see
// NewSessionServiceWithSearchEngine's doc comment for why IsTestMode() is only the
// default, not the seam itself).
// NOTE: Instances are NOT loaded here to prevent double-loading and initialization timing issues.
// Instances will be loaded in server.go after dependencies (statusManager, reviewQueue) are wired.
func NewSessionService(storage session.InstanceStore, eventBus *events.EventBus) *SessionService {
	return NewSessionServiceWithSearchEngine(storage, eventBus, newDefaultSearchEngine())
}

// newDefaultSearchEngine builds the search engine NewSessionService uses when the caller
// doesn't inject one: disk-backed with incremental persistence in production, in-memory
// under config.IsTestMode(). Under go test, skipping disk avoids a shared per-process test
// directory that every NewSessionService call in a test binary would otherwise persist
// into — the index grows across the whole package run and gob-decoding it gets slow enough
// under -race to blow CI's timeout budget, independent of what each test is exercising.
func newDefaultSearchEngine() *search.SearchEngine {
	if config.IsTestMode() {
		return search.NewSearchEngine()
	}
	indexStore, err := search.NewIndexStore()
	if err != nil {
		log.Warn("failed to create index store, using in-memory search", "err", err)
		return search.NewSearchEngine()
	}
	searchEngine := search.NewSearchEngineWithPersistence(indexStore)
	if loadErr := searchEngine.LoadIndex(); loadErr != nil {
		log.Warn("failed to load persisted search index", "err", loadErr)
	} else if meta := searchEngine.GetSyncMetadata(); meta != nil {
		log.Info("loaded persisted search index", "sessions", meta.TotalSessions, "documents", meta.TotalDocuments)
	}
	return searchEngine
}

// NewSessionServiceWithSearchEngine is the dependency-injection seam for the search engine:
// pass an explicit *search.SearchEngine (e.g. search.NewSearchEngine() for in-memory)
// instead of relying on NewSessionService's config.IsTestMode() default. Full migration of
// the ~78 existing NewSessionService(storage, eventBus) call sites to explicit injection is
// a separate, larger mechanical refactor — out of scope here; this seam exists so new or
// updated tests can opt in without waiting on that migration.
func NewSessionServiceWithSearchEngine(storage session.InstanceStore, eventBus *events.EventBus, searchEngine *search.SearchEngine) *SessionService {
	reviewQueue := session.NewReviewQueue()

	// concStorage is the concrete backing store used by sub-services that haven't migrated to
	// InstanceStore yet (ReviewQueueService, GitHubService, WorkspaceService). In tests using a
	// fake InstanceStore, concStorage will be nil — those sub-services degrade gracefully to nil storage.
	var concStorage *session.Storage
	if cs, ok := storage.(*session.Storage); ok {
		concStorage = cs
	}

	// Build approval store with disk persistence path
	approvalFilePath := ""
	configDir, configErr := config.GetConfigDir()
	if configErr == nil {
		approvalFilePath = configDir + "/pending_approvals.json"
	} else {
		log.Warn("failed to get config dir for approval persistence", "err", configErr)
	}
	approvalStore := NewApprovalStore(approvalFilePath)
	reviewQueueSvc := NewReviewQueueService(reviewQueue, concStorage, eventBus)
	reviewQueueSvc.SetApprovalStore(approvalStore)

	notificationSvc := NewNotificationService(NewNotificationRateLimiter(10, 20), eventBus)
	approvalSvc := NewApprovalService(approvalStore)
	approvalSvc.SetEventBus(eventBus)
	utilitySvc := NewUtilityService(approvalStore)

	// Build rules store, analytics store, and classifier for approval rules service.
	rulesStore, rulesErr := NewRulesStore(concStorage)
	if rulesErr != nil {
		log.Warn("failed to load rules store, using empty store", "err", rulesErr)
		rulesStore = &RulesStore{storage: concStorage}
	}
	analyticsStore := NewAnalyticsStore(concStorage)
	analyticsStore.Start(context.Background())
	classifierObj := classifier.NewRuleBasedClassifier()
	// Merge user rules into the classifier.
	if userRules := rulesStore.ToRules(); len(userRules) > 0 {
		classifierObj.AddRules(userRules)
	}
	// Wire AI rule generation. NewBestAvailableAIClient selects the highest-priority
	// available backend: Anthropic HTTP API (if ANTHROPIC_API_KEY is set) → claude CLI
	// → gemini CLI → opencode CLI. Returns nil when no backend is available.
	var promptBuilder RulePromptBuilder
	var aiClientImpl AIClient
	{
		apiKey := os.Getenv("ANTHROPIC_API_KEY")
		if c, backend := NewBestAvailableAIClient(apiKey, knownCLIAgents); c != nil {
			promptBuilder = &DefaultRulePromptBuilder{}
			aiClientImpl = c
			log.Info("[SessionService] AI rule generation enabled", "backend", backend)
		} else {
			log.Info("[SessionService] AI rule generation unavailable: set ANTHROPIC_API_KEY or install claude/gemini/opencode CLI")
		}
	}
	rulesSvc := NewRulesService(rulesStore, nil, analyticsStore, classifierObj, promptBuilder, aiClientImpl)

	// Initialize capacity monitor.
	var capCfg config.CapacityConfig
	if dir, err := config.GetConfigDir(); err == nil {
		if c, err := config.LoadConfigFromPath(filepath.Join(dir, "config.json")); err == nil {
			capCfg = c.Capacity
		}
	}
	capCfg = capCfg.CapacityConfigOrDefault()

	directCfg := &config.Config{}
	if dir, err := config.GetConfigDir(); err == nil {
		if c, err := config.LoadConfigFromPath(filepath.Join(dir, "config.json")); err == nil {
			directCfg = c
		}
	}
	credChain := NewDefaultChain(directCfg)

	capacityMonitor := NewCapacityMonitor(capCfg, eventBus, nil, nil, nil)
	capacityMonitor.RegisterClient("anthropic", NewAnthropicLimitsClient(credChain, ""))
	capacityMonitor.RegisterClient("google", NewGeminiLimitsClient(credChain, ""))

	if anthropicClient, ok := aiClientImpl.(*AnthropicAIClient); ok {
		anthropicClient.OnResponseHeaders = func(h http.Header) {
			capacityMonitor.UpdateFromResponseHeaders("anthropic", h)
		}
	}

	workspaceSvc := NewWorkspaceService(concStorage, eventBus)

	svc := &SessionService{
		storage:            storage,
		concStorage:        concStorage,
		eventBus:           eventBus,
		reviewQueueSvc:     reviewQueueSvc,
		searchSvc:          NewSearchService(searchEngine, search.NewSnippetGenerator(), 5*time.Minute),
		githubSvc:          NewGitHubService(concStorage),
		workspaceSvc:       workspaceSvc,
		configSvc:          NewConfigService(),
		notificationSvc:    notificationSvc,
		approvalSvc:        approvalSvc,
		utilitySvc:         utilitySvc,
		rulesSvc:           rulesSvc,
		approvalStore:      approvalStore,
		databaseSvc:        NewDatabaseService(),
		fileSvc:            NewFileService(workspaceSvc),
		pathCompletionSvc:  NewPathCompletionService(),
		slashCommandSvc:    NewSlashCommandService(),
		defaultsSvc:        NewDefaultsService(),
		callbackConfigSvc:  NewCallbackConfigService(),
		launcherPresetsSvc: NewLauncherPresetsService(),
		projectSvc:         NewProjectService(concStorage),
		checkpointSvc:      NewCheckpointService(storage, eventBus),
		featureFlagSvc:     NewFeatureFlagService(),
		terminalSvc:        NewTerminalService(),
		promptStore:        newPromptStore(),
		capacityMonitor:    capacityMonitor,
	}
	capacityMonitor.sessionSwitcher = svc
	capacityMonitor.poller = svc

	// Wire the fast-path live-instance lookup so WorkspaceService read-only RPCs
	// (GetVCSStatus, GetWorkspaceInfo, ListWorkspaceTargets) bypass LoadInstances.
	workspaceSvc.SetLiveFinder(svc)
	// Wire the live-instance lookup into ApprovalService's block-on-red-CI guard (AC5).
	// GitHubCheckConclusion is not persisted (see plan.md's Implementation Deviations),
	// so this must be the live registry, not storage.
	approvalSvc.SetLiveInstanceFinder(svc)
	// Wire the live-instance provider so ListClaudeHistory can populate
	// session_status on history entries without a separate storage call.
	svc.searchSvc.SetInstanceProvider(svc.allInstances)
	// Wire CheckpointService's instance-load fallback to this service's own
	// loadInstancesWithWiring so ClearConversationState gets properly-wired instances.
	svc.checkpointSvc.SetLoadInstancesFn(svc.loadInstancesWithWiring)

	// Wire the autonomous orchestration service with a storage getter closure.
	autonomousSvc := NewAutonomousOrchestrationService(nil, eventBus)
	autonomousSvc.SetStorageGetter(func() *session.Storage {
		if cs, ok := storage.(*session.Storage); ok {
			return cs
		}
		return nil
	})
	svc.autonomousSvc = autonomousSvc

	return svc
}

// newPromptStore creates a PromptStore backed by ~/.stapler-squad/prompts.json.
func newPromptStore() *prompts.PromptStore {
	dir, err := config.GetConfigDir()
	if err != nil {
		log.Warn("[PromptStore] failed to get config dir", "err", err)
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
		s.wireCallbacks(inst)
		// Backfill MCP server URL for sessions created before MCP integration was
		// wired up. Without this, buildLaunchCommand omits --mcp-config entirely and
		// the Claude process restarts without a session UUID or MCP connection.
		// Only applied in-memory; the DB value is updated lazily via SaveInstances.
		if mcpURL := s.resolveMCPServerURL(); inst.MCPServerURL == "" && mcpURL != "" {
			inst.SetMCPServerURL(mcpURL)
		}
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
		log.Warn("auto-migration to Ent skipped or failed", "err", migrateErr)
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

// ArchiveSessionByUUID satisfies the BacklogService.SessionStopper interface and the
// session.SessionArchiver interface (implemented here so both BacklogService and
// session.BacklogLifecycleListener can soft-archive backlog work sessions without
// reinventing the ArchiveSession RPC's logic — see ArchiveSession above).
// No-op (not an error) if the session isn't tracked live or is already archived, so
// callers can invoke this unconditionally from a sweep without extra existence checks.
func (s *SessionService) ArchiveSessionByUUID(ctx context.Context, sessionUUID string) error {
	inst := s.FindLiveInstance(sessionUUID)
	if inst == nil {
		return nil // already gone / never tracked
	}
	// SetArchivedAtIfNilAndStop also transitions Status to Stopped (see ArchiveSession's
	// comment) — safe to call unconditionally: no-ops the transition if already Stopped.
	if !inst.SetArchivedAtIfNilAndStop(time.Now()) {
		return nil // already archived
	}
	if err := s.storage.SaveInstances([]*session.Instance{inst}); err != nil {
		return fmt.Errorf("failed to save archived session %s: %w", sessionUUID, err)
	}
	return nil
}

// StopSessionByUUID satisfies the BacklogService.SessionStopper interface.
// It kills the live tmux session identified by UUID (best-effort; errors are non-fatal).
func (s *SessionService) StopSessionByUUID(ctx context.Context, sessionUUID string) error {
	inst := s.FindLiveInstance(sessionUUID)
	if inst == nil {
		return nil // already gone
	}
	if err := inst.Kill(); err != nil {
		log.Warn("StopSessionByUUID: kill failed", "uuid", sessionUUID, "err", err)
		return err
	}
	return nil
}

// IsSessionLive satisfies the BacklogService.SessionStopper interface.
// It returns true if the session UUID is currently tracked in the live in-memory poller.
func (s *SessionService) IsSessionLive(sessionUUID string) bool {
	return s.FindLiveInstance(sessionUUID) != nil
}

// TimeSinceLastMeaningfulOutput satisfies the BacklogService.SessionStopper
// interface. It reports how long it has been since sessionUUID's live
// Instance last produced meaningful terminal output. ok is false when the
// session isn't currently tracked live (mirrors IsSessionLive's "not found"
// case) — callers must not use dur in that case.
func (s *SessionService) TimeSinceLastMeaningfulOutput(sessionUUID string) (time.Duration, bool) {
	inst := s.FindLiveInstance(sessionUUID)
	if inst == nil {
		return 0, false
	}
	return inst.GetTimeSinceLastMeaningfulOutput(), true
}

// KillTmuxPaneOnly satisfies the BacklogService.SessionStopper interface.
// It closes the tmux pane only (Instance.KillSession), leaving the worktree
// intact — unlike StopSessionByUUID (Instance.Kill/Destroy), which also runs
// CleanupWorktree and would delete a worktree still in use by the next rework
// round. Best-effort: errors are logged, not returned, since this runs as
// cleanup alongside a new spawn that should proceed regardless.
func (s *SessionService) KillTmuxPaneOnly(ctx context.Context, sessionUUID string) error {
	inst := s.FindLiveInstance(sessionUUID)
	if inst == nil {
		return nil // already gone
	}
	if err := inst.KillSession(); err != nil {
		log.Warn("KillTmuxPaneOnly: kill failed", "uuid", sessionUUID, "err", err)
		return err
	}
	return nil
}

// KillTmuxSessionByTitle satisfies the BacklogService.SessionStopper interface.
// It kills the tmux session whose name is derived from title using the same sanitization
// as initTmuxSession (whitespace stripped, "." and ":" replaced with "_", "staplersquad_"
// prefix). This handles the case where the Instance is no longer tracked in memory
// but the underlying tmux session is still alive.
func (s *SessionService) KillTmuxSessionByTitle(ctx context.Context, title string) error {
	name := stapleSquadTmuxName(title)
	killCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	args := tmux.ResolveSocket("").Args("kill-session", "-t", name)
	cmd := safeexec.CommandContext(killCtx, tmux.Binary(), args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		combined := strings.ToLower(string(out))
		if strings.Contains(combined, "can't find session") ||
			strings.Contains(combined, "no server running") ||
			strings.Contains(combined, "error connecting to") {
			return nil // session already gone — not an error
		}
		return fmt.Errorf("tmux kill-session %q: %w (output: %s)", name, err, out)
	}
	return nil
}

// stapleSquadTmuxName computes the sanitized tmux session name for a given title,
// matching the logic in session/tmux.toStaplerSquadTmuxNameWithPrefix.
func stapleSquadTmuxName(title string) string {
	var sb strings.Builder
	for _, r := range title {
		if r != ' ' && r != '\t' && r != '\n' && r != '\r' {
			sb.WriteRune(r)
		}
	}
	sanitized := sb.String()
	sanitized = strings.ReplaceAll(sanitized, ".", "_")
	sanitized = strings.ReplaceAll(sanitized, ":", "_")
	return "staplersquad_" + sanitized
}

// expandTildePath replaces a leading ~ with the user's home directory.
func expandTildePath(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		} else {
			log.Warn("expandTildePath: failed to resolve home directory", "err", err)
		}
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			expanded := filepath.Join(home, path[2:])
			// Guard against path traversal: reject any result that escapes the home directory.
			if !strings.HasPrefix(expanded, home+string(filepath.Separator)) && expanded != home {
				log.Warn("expandTildePath: path traversal rejected", "input", path)
				return path
			}
			return expanded
		} else {
			log.Warn("expandTildePath: failed to resolve home directory", "err", err)
		}
	}
	return path
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

// Shutdown stops background goroutines owned by SessionService (currently the
// AnalyticsStore flush loop started in NewSessionService). Idempotent — safe
// to call multiple times (AnalyticsStore.Stop is itself sync.Once-guarded).
func (s *SessionService) Shutdown() {
	if store := s.GetAnalyticsStore(); store != nil {
		store.Stop()
	}
	// Stop accepting new tracked cleanup work before draining what's already
	// tracked — see deleteCleanupClosed's doc comment for why this ordering
	// (under deleteCleanupMu, before Wait) is what makes Add/Wait race-free.
	s.deleteCleanupMu.Lock()
	s.deleteCleanupClosed = true
	s.deleteCleanupMu.Unlock()
	// Await DeleteSession's background cleanup goroutines so they don't
	// outlive this process (or, in tests, outlive the test that spawned them).
	s.deleteCleanupWG.Wait()
}

// SetErrorRegistry wires the ErrorRegistry so the service can expose ListErrors and
// AcknowledgeError RPCs.  Must be called before the first RPC request.
func (s *SessionService) SetErrorRegistry(r *ErrorRegistry) {
	s.errorRegistry = r
}

// SetAnalyticsClient wires the ent client used for escape analytics queries.
// Must be called before the first QueryEscapeAnalytics or GetEscapeAnalyticsSummary RPC.
func (s *SessionService) SetAnalyticsClient(c *ent.Client) {
	s.analyticsClient = c
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

	log.Info("auto-migrating sessions from state.json to Ent repository", "count", len(stateFile.Instances))

	for _, inst := range stateFile.Instances {
		if createErr := repo.Create(ctx, inst); createErr != nil {
			log.Warn("auto-migrate: failed to create session", "session", inst.Title, "err", createErr)
		}
	}

	log.Info("auto-migration to Ent complete")
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

// SetMCPServerURL configures a lazily-invoked provider for the HTTP MCP
// endpoint URL passed to new sessions. Unlike a stored string, fn is called
// fresh at each point of use, so it can be wired up during server
// construction (before the listener has bound a real address) and still
// always observe the real bound address once Start() has resolved it, even
// under PORT=0.
func (s *SessionService) SetMCPServerURL(fn func() string) {
	s.mcpServerURLFn = fn
}

// resolveMCPServerURL invokes the lazily-configured MCP URL provider, if any,
// returning "" if it has not yet been configured.
func (s *SessionService) resolveMCPServerURL() string {
	if s.mcpServerURLFn == nil {
		return ""
	}
	return s.mcpServerURLFn()
}

// SetRegistry wires the Registry into this service. Called during server startup after
// the Registry is constructed in BuildServiceDeps.
func (s *SessionService) SetRegistry(r *session.Registry) {
	s.registry = r
}

// WireInstanceCallbacks is the onConstruct hook for Registry.Acquire. It wires all
// per-session callbacks (review queue, status manager, rate limit, etc.) onto a freshly
// constructed LiveInstance. Called exactly once per genuine construction in Acquire —
// never on refcount++ hits, never on Register (CreateSession wires callbacks explicitly).
func (s *SessionService) WireInstanceCallbacks(inst *session.LiveInstance) {
	inst.SetReviewQueue(s.reviewQueueSvc.GetQueue())
	if s.statusManager != nil {
		inst.SetStatusManager(s.statusManager)
	}
	s.wireRateLimitCallbacks(inst.Instance)
	s.wireStatusChangeCallback(inst.Instance)
	s.wireClaudeSessionIDCallback(inst.Instance)
	s.wireAutoArchiveCallback(inst.Instance)
	s.wireSessionExitedPublisher(inst.Instance)
	if mcpURL := s.resolveMCPServerURL(); inst.MCPServerURL == "" && mcpURL != "" {
		inst.SetMCPServerURL(mcpURL)
	}
}

// SetBacklogLifecycleListener wires the listener to all sessions created via
// CreateDirectorySession so that backlog state transitions fire on session exit.
func (s *SessionService) SetBacklogLifecycleListener(l *session.BacklogLifecycleListener) {
	s.backlogLifecycleListener = l
}

// GetBacklogLifecycleListener returns the wired BacklogLifecycleListener (nil if
// SetBacklogLifecycleListener was never called). Exported for the pointer-equality
// integration test proving BacklogService and BacklogLifecycleListener share a single
// PipelineEngine instance (Story 1.5.1) — see server/dependencies_test.go.
func (s *SessionService) GetBacklogLifecycleListener() *session.BacklogLifecycleListener {
	return s.backlogLifecycleListener
}

// SetSessionSummaryGenerator wires the generator to all sessions created via
// CreateSession/CreateDirectorySession/CreateWorktreeSession after this call,
// mirroring SetBacklogLifecycleListener's wiring pattern (see the WireToInstance
// call sites alongside session.WireSessionSummaryListener below).
func (s *SessionService) SetSessionSummaryGenerator(g *session.SessionSummaryGenerator) {
	s.sessionSummaryGenerator = g
}

// SetReviewGateTrigger wires the review gate trigger into the autonomous orchestration
// service so that completed work sessions immediately kick off headless review.
func (s *SessionService) SetReviewGateTrigger(t ReviewGateTrigger) {
	s.autonomousSvc.SetReviewGateTrigger(t)
}

// SetAutonomousStuckRespawner wires the respawner into the autonomous orchestration
// service so a turn-cap-stopped work session gets a fresh turn budget instead of
// being forced into review.
func (s *SessionService) SetAutonomousStuckRespawner(r AutonomousStuckRespawner) {
	s.autonomousSvc.SetAutonomousStuckRespawner(r)
}

// TriggerReviewForSession is a public passthrough to the wired ReviewGateTrigger.
// Satisfies mcp.ReviewTrigger so request_review can spawn a review gate immediately
// instead of waiting for the next ReconcileStuck tick.
func (s *SessionService) TriggerReviewForSession(sessionUUID string) {
	s.autonomousSvc.TriggerReviewForSession(sessionUUID)
}

// SpawnReviewSession satisfies the session.ReviewGateSpawner interface so that
// BacklogLifecycleListener can spawn one-shot review sessions automatically when
// a work session exits. The session is tagged "backlog:review" and runs one-shot.
func (s *SessionService) SpawnReviewSession(ctx context.Context, item *session.BacklogItemData, itemSessionID string, prompt string) (*session.Instance, error) {
	inst, err := s.CreateDirectorySession(ctx, "review:"+item.ID[:8], item.RepoPath, prompt, []string{"backlog:review"}, true, true)
	if err != nil {
		return nil, err
	}
	inst.SetCategory(session.CategoryBacklog)
	return inst, nil
}

// CreateDirectorySession satisfies the services.SessionCreator interface so that
// BacklogService can spawn sessions without importing SessionService directly.
// It creates a directory-type session with the given title, path, initial prompt,
// tags, and oneShot flag, wires it into the live poller, and returns the Instance.
func (s *SessionService) CreateDirectorySession(ctx context.Context, title, path, prompt string, tags []string, oneShot bool, hidden bool) (*session.Instance, error) {
	cfg := config.LoadConfig()
	resolved := config.ResolveDefaults(cfg, path, "")
	opts := session.InstanceOptions{
		Title:           title,
		Path:            path,
		Program:         resolved.Program,
		PermissionMode:  session.PermissionModeAuto, // automated sessions auto-approve tool uses without bypass prompt
		SessionType:     session.SessionTypeDirectory,
		Prompt:          prompt,
		Tags:            tags,
		OneShot:         oneShot,
		Hidden:          hidden,
		MCPServerURL:    s.resolveMCPServerURL(),
		CreateIfMissing: true,
	}
	instance, err := session.NewInstance(opts)
	if err != nil {
		return nil, fmt.Errorf("CreateDirectorySession: %w", err)
	}
	if err := instance.Start(true); err != nil {
		return nil, fmt.Errorf("CreateDirectorySession start: %w", err)
	}
	if s.statusManager != nil {
		instance.SetStatusManager(s.statusManager)
		if ctrlErr := instance.StartController(); ctrlErr != nil {
			log.Warn("[CreateDirectorySession] failed to start controller after wiring", "session", title, "err", ctrlErr)
		}
	}
	session.StartSessionDriver(instance, path)
	s.wireCallbacks(instance)
	if err := s.storage.AddInstance(instance); err != nil {
		_ = instance.Destroy()
		return nil, fmt.Errorf("CreateDirectorySession save: %w", err)
	}
	if s.reviewQueuePoller != nil {
		s.reviewQueuePoller.AddInstance(instance)
	}
	s.eventBus.Publish(events.NewSessionCreatedEvent(instance))
	if s.backlogLifecycleListener != nil {
		s.backlogLifecycleListener.WireToInstance(instance)
	}
	if s.sessionSummaryGenerator != nil {
		session.WireSessionSummaryListener(s.sessionSummaryGenerator, instance)
	}
	return instance, nil
}

// CreateWorktreeSession satisfies the services.SessionCreator interface.
// It spawns a session that uses an already-created git worktree at worktreePath.
// repoPath is the parent repo (for program resolution). worktreePath must exist on disk.
func (s *SessionService) CreateWorktreeSession(ctx context.Context, title, repoPath, worktreePath, prompt string, tags []string, oneShot bool, hidden bool) (*session.Instance, error) {
	cfg := config.LoadConfig()
	resolved := config.ResolveDefaults(cfg, repoPath, "")
	opts := session.InstanceOptions{
		Title:            title,
		Path:             repoPath,
		Program:          resolved.Program,
		PermissionMode:   session.PermissionModeAuto,
		SessionType:      session.SessionTypeExistingWorktree,
		ExistingWorktree: worktreePath,
		Prompt:           prompt,
		Tags:             tags,
		OneShot:          oneShot,
		Hidden:           hidden,
		MCPServerURL:     s.resolveMCPServerURL(),
		CreateIfMissing:  false,
	}
	instance, err := session.NewInstance(opts)
	if err != nil {
		return nil, fmt.Errorf("CreateWorktreeSession: %w", err)
	}
	if err := instance.Start(true); err != nil {
		return nil, fmt.Errorf("CreateWorktreeSession start: %w", err)
	}
	if s.statusManager != nil {
		instance.SetStatusManager(s.statusManager)
		if ctrlErr := instance.StartController(); ctrlErr != nil {
			log.Warn("[CreateWorktreeSession] failed to start controller after wiring", "session", title, "err", ctrlErr)
		}
	}
	session.StartSessionDriver(instance, repoPath)
	s.wireCallbacks(instance)
	if err := s.storage.AddInstance(instance); err != nil {
		_ = instance.Destroy()
		return nil, fmt.Errorf("CreateWorktreeSession save: %w", err)
	}
	if s.reviewQueuePoller != nil {
		s.reviewQueuePoller.AddInstance(instance)
	}
	s.eventBus.Publish(events.NewSessionCreatedEvent(instance))
	if s.backlogLifecycleListener != nil {
		s.backlogLifecycleListener.WireToInstance(instance)
	}
	if s.sessionSummaryGenerator != nil {
		session.WireSessionSummaryListener(s.sessionSummaryGenerator, instance)
	}
	return instance, nil
}

// SetHistoryLinker wires the HistoryLinker so deleted sessions are also removed
// from it and cannot be re-persisted by the shutdown hook.
func (s *SessionService) SetHistoryLinker(hl *session.HistoryLinker) {
	s.historyLinker = hl
}

// SetHeadlessPool wires the headless LLM pool for use by RunOneShot and other AI features.
func (s *SessionService) SetHeadlessPool(pool *headless.Pool) {
	s.headlessPool = pool
	s.autonomousSvc.SetPool(pool)
}

// SetLifecycleContext binds the server's root context to the service.
// Must be called once during server startup, before any sessions are created.
func (s *SessionService) SetLifecycleContext(ctx context.Context) {
	if s.capacityMonitor != nil {
		go s.capacityMonitor.Start(ctx)
	}
	s.autonomousSvc.SetLifecycleContext(ctx)
}

// wireCallbacks wires all per-instance lifecycle callbacks on inst.
// Consolidates the five wire* helpers that are always called together.
func (s *SessionService) wireCallbacks(inst *session.Instance) {
	s.wireRateLimitCallbacks(inst)
	s.wireStatusChangeCallback(inst)
	s.wireClaudeSessionIDCallback(inst)
	s.wireAutoArchiveCallback(inst)
	s.wireSessionExitedPublisher(inst)
	// Register with the HistoryLinker so its poll/fsnotify correlation loop
	// detects this session's Claude JSONL file and persists claude_session_id.
	// Without this, only sessions loaded at server boot (server/dependencies.go)
	// were ever registered — every session created afterward (regular sessions
	// via CreateSession, and every backlog/autonomous session via
	// CreateWorktreeSession/CreateDirectorySession) never got a conversation
	// UUID captured, so HasClaudeSession() stayed false and a session whose
	// tmux pane died (restart, hibernation, crash) started a fresh Claude
	// conversation on recovery instead of resuming — confirmed live
	// 2026-08-02 on backlog work sessions failing to resume post-restart.
	if s.historyLinker != nil {
		s.historyLinker.AddInstance(inst)
	}
}

// StopDriverForSession stops the AutonomousDriver registered under sessionTitle.
// Used by MCP handlers as a belt-and-suspenders stop after task completion.
// Satisfies mcp.ReviewCompletionSignaler.
func (s *SessionService) StopDriverForSession(sessionTitle string) {
	s.autonomousSvc.StopDriverForSession(sessionTitle)
}

// StartAutonomousDriverForInstance satisfies the AutonomousDriverStarter interface.
// Delegates to the autonomous orchestration service.
func (s *SessionService) StartAutonomousDriverForInstance(inst *session.Instance) {
	s.autonomousSvc.StartAutonomousDriverForInstance(inst)
}

// StartAutonomousDriverWithTimeout is like StartAutonomousDriverForInstance but
// uses a configurable startup timeout. Delegates to the autonomous orchestration service.
func (s *SessionService) StartAutonomousDriverWithTimeout(inst *session.Instance, startupTimeout time.Duration) {
	s.autonomousSvc.StartAutonomousDriverWithTimeout(inst, startupTimeout)
}

// Compile-time assertion: SessionService must implement AutonomousDriverStarter.
var _ AutonomousDriverStarter = (*SessionService)(nil)

// SetReviewQueuePoller wires the ReviewQueuePoller so new/deleted sessions are
// added/removed from the poller and AcknowledgeSession updates poller references.
// Must be called during server startup before any session mutation RPCs are used.
func (s *SessionService) SetReviewQueuePoller(poller *session.ReviewQueuePoller) {
	s.reviewQueuePoller = poller
	s.autonomousSvc.SetInstanceFinder(s.FindLiveInstance)
	s.reviewQueueSvc.SetReviewQueuePoller(poller)
	s.notificationSvc.SetReviewQueuePoller(poller)
	s.utilitySvc.SetReviewQueuePoller(poller)
	s.checkpointSvc.SetPoller(poller)
	s.terminalSvc.SetPoller(poller)
	if s.workflowSvc != nil {
		s.workflowSvc.SetPoller(poller)
	}
}

// SetMemoryCacheReader wires the HibernationSweeper so that ListSessions can
// populate memory_rss_mb, estimated_savings_mb, and system_memory_pct fields.
func (s *SessionService) SetMemoryCacheReader(r session.MemoryCacheReader) {
	s.memoryCacheReader = r
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
	s.checkpointSvc.SetExternalDiscovery(discovery)
	s.terminalSvc.SetExternalDiscovery(discovery)
}

// SetTmuxStreamerManager wires the shared ExternalTmuxStreamerManager so StopShell
// can evict a shell's streamer when the shell closes. Must be called during server
// startup with the same instance passed to NewConnectRPCWebSocketHandler.
func (s *SessionService) SetTmuxStreamerManager(mgr *session.ExternalTmuxStreamerManager) {
	s.tmuxStreamerManager = mgr
}

// SetUserPRCache wires the shared UserPRCache so CreateSession's GitHub URL
// detection recognizes enterprise hosts from dynamically-added accounts, not
// just hosts with a statically configured OAuth App in config.json.
func (s *SessionService) SetUserPRCache(cache *githubpkg.UserPRCache) {
	s.userPRCache = cache
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

// SetFeatureController wires a runtime controller for the named feature flag.
// Delegates to FeatureFlagService which owns the controller registry.
func (s *SessionService) SetFeatureController(name string, c FeatureController) {
	s.featureFlagSvc.SetFeatureController(name, c)
}

// SetStatusDetailProvider wires an optional status-detail provider for the
// named feature flag. Delegates to FeatureFlagService.
func (s *SessionService) SetStatusDetailProvider(name string, fn func() string) {
	s.featureFlagSvc.SetStatusDetailProvider(name, fn)
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
		// Apply optional status filter. Read the status directly off the instance
		// instead of building the full proto just to inspect one field — building it
		// runs the full GetEffectiveStatus/GetStatusAndIdleInfo/DetectStateFromContent
		// chain, which is expensive enough that doing it twice per instance (once here,
		// once for the real output below) roughly doubles ListSessions' allocation cost
		// whenever a status filter is applied.
		if req.Msg.Status != nil && *req.Msg.Status != sessionv1.SessionStatus_SESSION_STATUS_UNSPECIFIED {
			if adapters.StatusToProto(inst.GetEffectiveStatus()) != *req.Msg.Status {
				continue
			}
		}

		// Apply optional category filter
		if req.Msg.Category != nil && *req.Msg.Category != "" && inst.Category != *req.Msg.Category {
			continue
		}

		// Exclude hidden (system/background) sessions unless explicitly requested
		if inst.Hidden && !req.Msg.IncludeHidden {
			continue
		}

		// Exclude archived sessions unless explicitly requested
		if inst.ArchivedAt != nil && !req.Msg.IncludeArchived {
			continue
		}

		// Filter by workflow_id when specified
		if req.Msg.WorkflowId != nil && *req.Msg.WorkflowId != "" && inst.WorkflowID != *req.Msg.WorkflowId {
			continue
		}

		protoSess := adapters.InstanceToProto(inst, s.workflowNames())
		if s.memoryCacheReader != nil && inst.IsActive() {
			rss := s.memoryCacheReader.GetCachedRSSMB(inst.UUID)
			protoSess.MemoryRssMb = rss
			protoSess.EstimatedSavingsMb = rss
		}
		sessions = append(sessions, protoSess)
	}

	// Include external sessions from mux discovery if available
	if s.externalDiscovery != nil {
		for _, extInst := range s.externalDiscovery.GetSessions() {
			// Apply optional status filter (external sessions are always "running")
			if req.Msg.Status != nil && *req.Msg.Status != sessionv1.SessionStatus_SESSION_STATUS_UNSPECIFIED {
				// External sessions are running
				if *req.Msg.Status != sessionv1.SessionStatus_SESSION_STATUS_ACTIVE {
					continue
				}
			}

			// Apply optional category filter
			if req.Msg.Category != nil && *req.Msg.Category != "" && extInst.Category != *req.Msg.Category {
				continue
			}

			// Exclude hidden external sessions unless requested
			if extInst.Hidden && !req.Msg.IncludeHidden {
				continue
			}

			sessions = append(sessions, adapters.InstanceToProto(extInst, nil))
		}
	}

	var sysPct float32
	if s.memoryCacheReader != nil {
		pct, _ := s.memoryCacheReader.SystemMemoryPct()
		sysPct = float32(pct)
	}

	return connect.NewResponse(&sessionv1.ListSessionsResponse{
		Sessions:        sessions,
		SystemMemoryPct: sysPct,
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
		wfNames := s.workflowNames()
		if inst := s.reviewQueuePoller.FindInstance(req.Msg.Id); inst != nil {
			return connect.NewResponse(&sessionv1.GetSessionResponse{
				Session: adapters.InstanceToProto(inst, wfNames),
			}), nil
		}
		// Not in poller — also check external sessions
		if s.externalDiscovery != nil {
			if inst := s.externalDiscovery.GetSession(req.Msg.Id); inst != nil {
				return connect.NewResponse(&sessionv1.GetSessionResponse{
					Session: adapters.InstanceToProto(inst, nil),
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
	wfNames := s.workflowNames()
	for _, inst := range instances {
		if inst.MatchesID(req.Msg.Id) {
			return connect.NewResponse(&sessionv1.GetSessionResponse{
				Session: adapters.InstanceToProto(inst, wfNames),
			}), nil
		}
	}

	return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.Id))
}

// workspacePeersBlockFor returns a one-time "other active sessions in this workspace"
// nudge for a new session being created at repoPath, or "" when the workspacePeersNudgeFlagName
// feature flag is off (default), on any detection/lookup failure, when there's no concrete
// storage backing this service, or when there are no peers. Best-effort: this is a
// convenience nudge, not required session context. Delegates to
// session.WorkspacePeersBlockForPath, shared with BacklogService's initialPromptFor so the
// two callers can't drift on how the nudge is built.
func (s *SessionService) workspacePeersBlockFor(ctx context.Context, repoPath string) string {
	return workspacePeersBlockFor(ctx, s.concStorage, repoPath)
}

// CreateSession initializes a new AI agent session with tmux and git worktree.
// +api: session:create
func (s *SessionService) CreateSession(
	ctx context.Context,
	req *connect.Request[sessionv1.CreateSessionRequest],
) (*connect.Response[sessionv1.CreateSessionResponse], error) {
	ctx, cancel := context.WithTimeout(ctx, createSessionTimeout)
	defer cancel()

	// Validate required fields
	if req.Msg.Title == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("title is required"))
	}
	if req.Msg.SessionType != sessionv1.SessionType_SESSION_TYPE_ONE_OFF &&
		// AutonomousMode: the omnibar always submits an empty path for autonomous
		// sessions; see the directory-generation block below.
		!req.Msg.AutonomousMode &&
		req.Msg.AliasName == "" &&
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

	// Validate client-supplied resume_id before the fork block can overwrite it.
	if req.Msg.ResumeId != "" && req.Msg.ForkSourceId == "" {
		if !resumeIDRe.MatchString(req.Msg.ResumeId) {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				errors.New("resume_id must be a valid UUID"))
		}
	}

	// Fork dispatch: when fork_source_id is set, copy the source conversation
	// file and set resume_id to the new UUID so the normal start path picks it
	// up with --resume.
	if req.Msg.ForkSourceId != "" {
		srcPath, findErr := session.FindConversationFilePath(req.Msg.ForkSourceId)
		if findErr != nil {
			return nil, connect.NewError(connect.CodeNotFound,
				fmt.Errorf("fork source conversation not found: %w", findErr))
		}
		lineCount := uint64(req.Msg.ForkAtMessage) //nolint:gosec // bounded by int32
		newUUID, forkErr := session.ForkClaudeConversation(srcPath, lineCount, filepath.Dir(srcPath))
		if forkErr != nil {
			return nil, connect.NewError(connect.CodeInternal,
				fmt.Errorf("fork conversation failed: %w", forkErr))
		}
		req.Msg.ResumeId = newUUID
		log.Info("[CreateSession] forked conversation",
			"source", req.Msg.ForkSourceId, "new_uuid", newUUID,
			"fork_at_message", req.Msg.ForkAtMessage)
	}

	// Load config once; used by the GitHub URL resolution below as well as the
	// one-off path and the defaults/alias path further down.
	cfg := config.LoadConfig()

	// Resolve GitHub URLs to local paths (GOPATH-style: ~/.stapler-squad/repos/<host>/owner/repo)
	resolvedPath := expandTildePath(req.Msg.Path)
	branch := req.Msg.Branch
	var gitHubRef *session.GitHubRef
	var clonedRepoPath string

	// Union statically-configured hosts with hosts from dynamically-added
	// accounts (gh CLI import, device auth) — mirrors ListGitHubAccounts'
	// host union in github_user_service.go so CreateSession recognizes the
	// same enterprise URLs the omnibar's detector does.
	enterpriseHosts := s.enterpriseHosts(cfg)

	if session.IsGitHubURLWithHosts(req.Msg.Path, enterpriseHosts) {
		log.Info("[CreateSession] detected GitHub URL", "path", req.Msg.Path)

		// ResolveGitHubInputCtxWithHosts threads ctx down to the underlying git
		// clone/fetch subprocess via safeexec.CommandContext, so the RPC's
		// timeout genuinely cancels the subprocess instead of abandoning it
		// to keep running in the background after the RPC returns. It also
		// recognizes URLs against any configured GitHub Enterprise hosts, not
		// just github.com.
		localPath, ref, err := session.ResolveGitHubInputCtxWithHosts(ctx, req.Msg.Path, enterpriseHosts)
		if err != nil {
			if ctx.Err() != nil {
				return nil, connect.NewError(connect.CodeDeadlineExceeded, fmt.Errorf("resolving GitHub URL timed out: %w", ctx.Err()))
			}
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("failed to resolve GitHub URL: %w", err))
		}
		resolvedPath = localPath
		gitHubRef = ref
		clonedRepoPath = localPath

		// Use branch from GitHub URL if not explicitly provided
		if branch == "" && gitHubRef.Branch != "" {
			branch = gitHubRef.Branch
		}

		log.Info("[CreateSession] resolved to local path", "path", resolvedPath, "branch", branch)
	}

	// One-off session: generate a fresh directory and override resolvedPath.
	// Autonomous sessions created without an explicit path (the omnibar's normal
	// flow) get the same treatment — the agent needs somewhere to run.
	if req.Msg.SessionType == sessionv1.SessionType_SESSION_TYPE_ONE_OFF ||
		(req.Msg.AutonomousMode && resolvedPath == "") {
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
	instanceEnvVars := make(map[string]string)
	instanceCLIFlags := ""
	aliasSessionType := config.SessionTypeDefault // session type from alias config (empty = no override)
	if !req.Msg.SkipDefaults {
		if req.Msg.AliasName != "" {
			resolved, err := config.ResolveAlias(cfg, req.Msg.AliasName, req.Msg.Branch, req.Msg.Title, "")
			if err != nil {
				if errors.Is(err, config.ErrAliasNotFound) {
					return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("alias %q not found: %w", req.Msg.AliasName, err))
				}
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to resolve alias %q: %w", req.Msg.AliasName, err))
			}
			if program == "" {
				program = resolved.Program
			}
			if !autoYes && resolved.AutoYes {
				autoYes = true
			}
			for k, v := range resolved.EnvVars {
				instanceEnvVars[k] = v
			}
			instanceCLIFlags = resolved.CLIFlags
			if resolvedPath == "" && resolved.Path != "" {
				resolvedPath = expandTildePath(resolved.Path)
			}
			// Read session type directly from the alias config — it is an alias-specific
			// property, not a cascading default, so it is not part of ResolvedDefaults.
			if alias := config.FindAlias(cfg, req.Msg.AliasName); alias != nil {
				aliasSessionType = alias.SessionType
			}
		} else {
			workingDir := req.Msg.WorkingDir
			if workingDir == "" {
				workingDir = resolvedPath
			}
			resolved := config.ResolveDefaults(cfg, workingDir, req.Msg.Profile)
			if program == "" {
				program = resolved.Program
			}
			if !autoYes && resolved.AutoYes {
				autoYes = true
			}
			for k, v := range resolved.EnvVars {
				instanceEnvVars[k] = v
			}
			instanceCLIFlags = resolved.CLIFlags
		}
	}

	// Merge explicit request env_vars on top of resolved defaults.
	for k, v := range req.Msg.EnvVars {
		instanceEnvVars[k] = v
	}
	// Append explicit request cli_flags on top of resolved defaults.
	if req.Msg.CliFlags != "" {
		if instanceCLIFlags != "" {
			instanceCLIFlags += " " + req.Msg.CliFlags
		} else {
			instanceCLIFlags = req.Msg.CliFlags
		}
	}

	// Determine session type - use explicit session_type if provided, otherwise infer from fields.
	// If the session was created via alias and the alias specifies a session type,
	// use it as the fallback when the request itself didn't set one.
	sessionType := resolveSessionType(req.Msg, branch)
	if aliasSessionType != config.SessionTypeDefault && req.Msg.SessionType == sessionv1.SessionType_SESSION_TYPE_UNSPECIFIED {
		sessionType = aliasSessionType
	}

	// One-off sessions run as directory sessions — the path was already generated above.
	if sessionType == session.SessionTypeOneOff {
		sessionType = session.SessionTypeDirectory
	}

	// For resume sessions, force DIRECTORY type — we must not create a new worktree
	// that would produce a different project path and break the --resume lookup.
	if req.Msg.ResumeId != "" && req.Msg.ForkSourceId == "" &&
		req.Msg.SessionType == sessionv1.SessionType_SESSION_TYPE_UNSPECIFIED {
		sessionType = session.SessionTypeDirectory
	}

	// For Directory mode: if path does not exist and create_if_missing is not set, return
	// CodeNotFound so the frontend can show a confirmation dialog.
	if sessionType == session.SessionTypeDirectory {
		if _, err := os.Stat(resolvedPath); os.IsNotExist(err) {
			if !req.Msg.CreateIfMissing {
				if req.Msg.ResumeId != "" {
					return nil, connect.NewError(connect.CodeNotFound,
						fmt.Errorf("cannot resume: project directory no longer exists: %s", resolvedPath))
				}
				return nil, connect.NewError(connect.CodeNotFound,
					fmt.Errorf("path does not exist: %s", resolvedPath))
			}
			// create_if_missing=true: fall through; setupFirstTimeWorktree handles creation
		}
	}

	// One-time workspace-peers nudge for genuinely new sessions (not resumes), when the
	// workspacePeersNudgeFlagName feature flag is enabled. Best-effort: any detection/lookup
	// failure just omits the nudge.
	initialPrompt := req.Msg.InitialPrompt
	if req.Msg.ResumeId == "" {
		initialPrompt += s.workspacePeersBlockFor(ctx, resolvedPath)
	}

	// auto_approve is representable-but-invalid for an agent yoloFlagFor can't inject a
	// bypass flag for -- the Omnibar UI disables the checkbox client-side, but that's not
	// a guarantee for other RPC callers (MCP tools, scripts, curl), so the invariant is
	// enforced here too rather than left purely client-enforced.
	if req.Msg.AutoApprove && !session.AutoApproveSupported(program) {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("auto_approve is not supported for program %q", program))
	}

	// Build instance options
	instanceOpts := session.InstanceOptions{
		Title:            req.Msg.Title,
		Path:             resolvedPath,
		WorkingDir:       req.Msg.WorkingDir,
		Branch:           branch,
		Program:          program,
		AutoYes:          autoYes,
		AutoApprove:      req.Msg.AutoApprove,
		Prompt:           req.Msg.Prompt,
		InitialPrompt:    initialPrompt,
		ExistingWorktree: req.Msg.ExistingWorktree,
		Category:         req.Msg.Category,
		SessionType:      sessionType,
		TmuxPrefix:       "", // Use default from config
		ResumeId:         req.Msg.ResumeId,
		OneShot:          req.Msg.OneShot,
		ProjectID:        req.Msg.ProjectId,
		MCPServerURL:     s.resolveMCPServerURL(),
		CreateIfMissing:  req.Msg.CreateIfMissing,
		AllowedTools:     req.Msg.AllowedTools,
		PermissionMode:   req.Msg.PermissionMode,
		AutonomousMode:   req.Msg.AutonomousMode,
		WorkflowID:       req.Msg.WorkflowId,
		EnvVars:          instanceEnvVars,
		CLIFlags:         instanceCLIFlags,
		// ExtraArgs is a direct passthrough of req.Msg.ExtraArgs — unlike CLIFlags, it has no
		// defaults-resolution concept to merge with. It composes with instanceCLIFlags at
		// launch time in buildLaunchCommand: CLIFlags-derived tokens first, ExtraArgs last —
		// an intentional, tested ordering (see TestCreateSession_should_ComposeProfileCLIFlagsBeforePresetExtraArgs_When_BothPresent).
		ExtraArgs: req.Msg.ExtraArgs,
	}

	// Add GitHub metadata if this was a GitHub URL
	if gitHubRef != nil {
		instanceOpts.GitHubOwner = gitHubRef.Owner
		instanceOpts.GitHubRepo = gitHubRef.Repo
		instanceOpts.GitHubSourceRef = req.Msg.Path
		instanceOpts.ClonedRepoPath = clonedRepoPath
		if gitHubRef.PRNumber > 0 {
			instanceOpts.GitHubPRNumber = gitHubRef.PRNumber
			instanceOpts.GitHubPRURL = gitHubRef.PRURL()
		}
	}

	// Construct and persist the instance (Creating status) via the shared
	// domain function -- see session/create_managed_instance.go (Story
	// 1.2.0a). This does NOT start tmux/the process; that happens in the
	// async goroutine below, exactly as before this extraction.
	instance, err := session.CreateManagedInstance(ctx, session.CreateManagedInstanceParams{
		Options:         instanceOpts,
		Storage:         s.storage,
		Registry:        s.registry,
		CreateIfMissing: req.Msg.CreateIfMissing,
		ResumeID:        req.Msg.ResumeId,
	})
	if err != nil {
		switch {
		case errors.Is(err, session.ErrPathNotExist), errors.Is(err, session.ErrResumePathNotExist):
			return nil, connect.NewError(connect.CodeNotFound, err)
		case errors.Is(err, session.ErrInstanceConstructionFailed):
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		default:
			// Covers ErrInstanceRegistrationFailed and ErrInstanceSaveFailed.
			return nil, connect.NewError(connect.CodeInternal, err)
		}
	}

	// Add the session to the poller so WatchSessions picks it up immediately.
	if s.reviewQueuePoller != nil {
		s.reviewQueuePoller.AddInstance(instance)
		log.Info("[ReviewQueue] added new session to poller", "session", instance.Title)
	}

	// Record initial_prompt (typed into the session terminal once the session reaches Ready state)
	// in prompt history so it appears in the recent-prompts dropdown.
	if req.Msg.InitialPrompt != "" {
		s.promptStore.RecordUsage(req.Msg.InitialPrompt)
	}

	// Publish SessionCreated event so watchers see the Creating-status session immediately.
	s.eventBus.Publish(events.NewSessionCreatedEvent(instance))

	// Capture refs needed inside the goroutine (avoid capturing req.Msg which may be GC'd).
	instanceTitle := instance.Title
	instanceRootDir := instance.GetEffectiveRootDir()

	// Pre-compute the Creating-state proto for the RPC response so the return
	// statement below does not race with the goroutine's SetCreationProgress calls.
	creatingProto := adapters.InstanceToProto(instance, s.workflowNames())

	// Perform the actual initialization asynchronously so the RPC returns within milliseconds.
	go func() {
		// Wire callbacks before starting so rate-limit and status-change events fire.
		s.wireCallbacks(instance)

		instance.SetCreationProgress("Starting session...")
		s.eventBus.Publish(events.NewSessionUpdatedEvent(instance, []string{"creation_progress"}))

		// Start the session (initializes tmux + git worktree).
		if startErr := instance.Start(true); startErr != nil {
			log.Error("[CreateSession] async start failed", "session", instanceTitle, "err", startErr)
			// Transition to Stopped on failure.
			instance.SetCreationProgress(fmt.Sprintf("Startup failed: %s", startErr.Error()))
			instance.ForceStatus(session.Stopped)
			_ = s.storage.SaveInstances([]*session.Instance{instance})
			s.eventBus.Publish(events.NewSessionUpdatedEvent(instance, []string{"status", "creation_progress"}))
			return
		}

		// Clear progress message now that we are Active.
		instance.SetCreationProgress("")

		// Inject Claude Code HTTP hook config for remote approval from the web UI.
		// Non-fatal: session is fully functional even without this config.
		if err := InjectHookConfig(instanceRootDir, instanceTitle); err != nil {
			log.Warn("[CreateSession] failed to inject hook config", "session", instanceTitle, "err", err)
		}

		if s.backlogLifecycleListener != nil {
			s.backlogLifecycleListener.WireToInstance(instance)
		}
		if s.sessionSummaryGenerator != nil {
			session.WireSessionSummaryListener(s.sessionSummaryGenerator, instance)
		}

		// Wire the status manager and start the controller AFTER Start() returns so the
		// tmux attach-session process has had time to fully initialize. Starting the
		// controller inside Start() caused immediate PTY EIO because tmux hadn't
		// stabilized yet. This mirrors the pattern used by loadInstancesWithWiring.
		if s.statusManager != nil {
			instance.SetStatusManager(s.statusManager)
			if ctrlErr := instance.StartController(); ctrlErr != nil {
				log.Warn("[CreateSession] failed to start controller after wiring", "session", instanceTitle, "err", ctrlErr)
			}
		}

		// Start the session driver goroutine so UI-created sessions receive their
		// initial prompt (typed into the session terminal once the session reaches Ready).
		// StartSessionDriver is idempotent (CAS guard) — safe to call even if a driver
		// was already started by another code path.
		session.StartSessionDriver(instance, instanceRootDir)

		if instance.AutonomousMode && s.headlessPool != nil {
			driver := session.NewAutonomousDriver(instance, s.headlessPool, instance.Prompt, 0)
			driver.RegisterCompletionCallback(s.autonomousSvc.onAutonomousDriverComplete)
			if driverErr := driver.Start(s.autonomousSvc.driverCtx()); driverErr != nil {
				log.Warn("[CreateSession] failed to start autonomous driver", "session", instanceTitle, "err", driverErr)
			} else {
				s.autonomousSvc.registerDriver(instanceTitle, driver)
			}
		} else if instance.AutonomousMode {
			log.Warn("[CreateSession] autonomous_mode requested but headlessPool is nil", "session", instanceTitle)
		}

		_ = s.storage.SaveInstances([]*session.Instance{instance})
		s.eventBus.Publish(events.NewSessionUpdatedEvent(instance, []string{"status", "creation_progress"}))
		log.Info("[CreateSession] async start complete", "session", instanceTitle)
	}()

	return connect.NewResponse(&sessionv1.CreateSessionResponse{
		Session: creatingProto,
	}), nil
}

// resolveSessionType maps a CreateSessionRequest + resolved branch to a session.SessionType.
// Priority: explicit session_type > inference from branch/existing_worktree.
// ONE_OFF is returned as SessionTypeOneOff; callers are responsible for converting it to
// SessionTypeDirectory after the one-off directory has been generated.
func resolveSessionType(msg *sessionv1.CreateSessionRequest, branch string) session.SessionType {
	if msg.SessionType != sessionv1.SessionType_SESSION_TYPE_UNSPECIFIED {
		switch msg.SessionType {
		case sessionv1.SessionType_SESSION_TYPE_DIRECTORY:
			return session.SessionTypeDirectory
		case sessionv1.SessionType_SESSION_TYPE_NEW_WORKTREE:
			return session.SessionTypeNewWorktree
		case sessionv1.SessionType_SESSION_TYPE_EXISTING_WORKTREE:
			return session.SessionTypeExistingWorktree
		case sessionv1.SessionType_SESSION_TYPE_NEW_PROJECT:
			return session.SessionTypeNewProject
		case sessionv1.SessionType_SESSION_TYPE_ONE_OFF:
			return session.SessionTypeOneOff
		default:
			return session.SessionTypeDirectory
		}
	}
	if msg.ExistingWorktree != "" {
		return session.SessionTypeExistingWorktree
	}
	if branch != "" {
		return session.SessionTypeNewWorktree
	}
	return session.SessionTypeDirectory
}

// enterpriseHosts unions statically-configured GitHub Enterprise hosts with hosts
// from dynamically-added accounts (gh CLI import, device auth) — mirrors
// ListGitHubAccounts' host union in github_user_service.go so every caller
// recognizes the same enterprise URLs the omnibar's detector does. CreateSession
// and PreviewDestinationPath both call this so the two never diverge.
func (s *SessionService) enterpriseHosts(cfg *config.Config) []string {
	configuredHosts := cfg.GetGitHubEnterpriseHosts()
	var cachedAccounts []githubpkg.CachedAccount
	if s.userPRCache != nil {
		cachedAccounts = s.userPRCache.GetCachedAccounts()
	}
	seenHosts := make(map[string]bool, len(configuredHosts)+len(cachedAccounts))
	hosts := make([]string, 0, len(configuredHosts)+len(cachedAccounts))
	addHost := func(host string) {
		host = githubpkg.NormalizeHost(host)
		if host == "" || githubpkg.IsGitHubCom(host) || seenHosts[host] {
			return
		}
		seenHosts[host] = true
		hosts = append(hosts, host)
	}
	for _, h := range configuredHosts {
		addHost(h.Host)
	}
	for _, a := range cachedAccounts {
		addHost(a.Host)
	}
	return hosts
}

// PreviewDestinationPath computes where a session's checkout/worktree would land
// without performing any git or filesystem mutation. Used by the Omnibar to show a
// live destination hint before the user submits session creation.
// +api: session:preview-destination-path
func (s *SessionService) PreviewDestinationPath(
	ctx context.Context,
	req *connect.Request[sessionv1.PreviewDestinationPathRequest],
) (*connect.Response[sessionv1.PreviewDestinationPathResponse], error) {
	cfg := config.LoadConfig()

	switch req.Msg.Mode {
	case "github_url":
		ref, err := session.ParseGitHubURLWithHosts(req.Msg.Input, s.enterpriseHosts(cfg))
		if err != nil {
			return connect.NewResponse(&sessionv1.PreviewDestinationPathResponse{
				UnresolvedReason: "not a recognized GitHub URL",
			}), nil
		}
		path := session.DefaultRepoPathManager.GetRepoPath(ref)
		return connect.NewResponse(&sessionv1.PreviewDestinationPathResponse{
			Path:    path,
			IsExact: true,
		}), nil

	case "new_worktree":
		if req.Msg.RepoPath == "" || req.Msg.SessionName == "" {
			return connect.NewResponse(&sessionv1.PreviewDestinationPathResponse{
				UnresolvedReason: "repo_path and session_name are required",
			}), nil
		}
		prefix, err := git.PreviewWorktreePath(req.Msg.RepoPath, req.Msg.SessionName)
		if err != nil {
			return connect.NewResponse(&sessionv1.PreviewDestinationPathResponse{
				UnresolvedReason: fmt.Sprintf("could not resolve repo path: %v", err),
			}), nil
		}
		return connect.NewResponse(&sessionv1.PreviewDestinationPathResponse{
			Path:    prefix,
			IsExact: false,
		}), nil

	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("unknown mode: %q", req.Msg.Mode))
	}
}

// classifyPauseResumeErr maps a Pause()/Resume() error to the appropriate connect
// error code. Permission and state-machine rejections are the caller's fault
// (FailedPrecondition, not a 500); anything else is an unexpected operational
// failure (git/tmux errors) and stays CodeInternal.
func classifyPauseResumeErr(err error, opDesc string) *connect.Error {
	var transErr session.ErrInvalidTransition
	if errors.As(err, &transErr) ||
		errors.Is(err, session.ErrPauseNotPermitted) ||
		errors.Is(err, session.ErrResumeNotPermitted) {
		return connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("failed to %s session: %w", opDesc, err))
	}
	return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to %s session: %w", opDesc, err))
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

	// Use the live poller list to avoid LoadInstances side-effects (Start() on Active
	// sessions) that can silently drop sessions if tmux is unavailable, which would then
	// clobber the poller's complete list via SetInstances.
	var instances []*session.Instance
	if s.reviewQueuePoller != nil {
		instances = s.reviewQueuePoller.GetInstances()
	} else {
		var loadErr error
		instances, loadErr = s.loadInstancesWithWiring()
		if loadErr != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", loadErr))
		}
	}

	// Find the instance to update
	var instance *session.Instance
	for _, inst := range instances {
		if inst.MatchesID(req.Msg.Id) {
			instance = inst
			break
		}
	}

	if instance == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.Id))
	}

	// Captured before any rename below mutates instance.Title in-memory. A narrow
	// metadata update must key its WHERE clause off the pre-rename title, or it misses
	// the DB row entirely once instance.Title has already moved to the new value.
	currentTitle := instance.Title

	// Validate the note length before any field below mutates live in-memory state
	// (SetTitleDirect/SetCategory publish immediately via snapshot.Store, not staged
	// until SaveInstances) — otherwise a rejected request could still leave title/category
	// changes visible to concurrent readers.
	if req.Msg.Note != nil && len(*req.Msg.Note) > session.MaxNoteLength {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("note exceeds maximum length of %d bytes", session.MaxNoteLength))
	}

	// Track which fields are being updated for event publishing
	var updatedFields []string

	// Metadata fields (title/category/note/working_dir) persist via a single narrow
	// UPDATE (UpdateInstanceMetadata) instead of the full-row SaveInstances rewrite.
	// A non-nil pointer here means "this field was part of the request".
	var metaTitle, metaCategory, metaNote, metaWorkingDir *string

	// sideEffectChanged tracks fields (tags, status, rate_limit_enabled, autonomous_mode)
	// that still need the full-row SaveInstances write — e.g. tags requires managing the
	// tags M2M relation, which a narrow column UPDATE can't replicate.
	var sideEffectChanged bool

	// Handle title update (before status change so rename is atomic with resume)
	if req.Msg.Title != nil && *req.Msg.Title != "" && *req.Msg.Title != instance.Title {
		// Check if new title already exists
		for _, inst := range instances {
			if inst.Title == *req.Msg.Title {
				return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("session with title '%s' already exists", *req.Msg.Title))
			}
		}
		instance.SetTitleDirect(*req.Msg.Title)
		updatedFields = append(updatedFields, "title")
		metaTitle = req.Msg.Title
	}

	// Handle category update
	if req.Msg.Category != nil {
		instance.SetCategory(*req.Msg.Category)
		updatedFields = append(updatedFields, "category")
		metaCategory = req.Msg.Category
	}

	// Handle note update. Length already validated above.
	if req.Msg.Note != nil {
		instance.SetNote(*req.Msg.Note)
		updatedFields = append(updatedFields, "note")
		metaNote = req.Msg.Note
	}

	// Handle note update. Length already validated above.
	if req.Msg.Note != nil {
		instance.SetNote(*req.Msg.Note)
		updatedFields = append(updatedFields, "note")
	}

	// Handle note update. Length already validated above.
	if req.Msg.Note != nil {
		instance.SetNote(*req.Msg.Note)
		updatedFields = append(updatedFields, "note")
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
		sideEffectChanged = true
	}

	// Handle program update. Empty string means "System default" — resolve to the
	// configured default so the DB NotEmpty constraint is satisfied. Consolidated with
	// the capacity-monitor auto-fallback path (UpdateSessionProgram below) via
	// Instance.SwitchProgram so the two entry points can't drift or double-restart.
	if req.Msg.Program != nil {
		// Flush any pending title/category/note rename now, keyed on currentTitle,
		// before SwitchProgram's callback below can trigger its own SaveInstances
		// call. That call persists via instance.ToInstanceData(), whose Title is
		// already the in-memory-renamed value — looking the DB row up by that new
		// title (before the narrow rename below has run) misses the still-old-titled
		// row and duplicates it via saveInstancesToRepo's Create fallback, exactly
		// the orphaned/duplicate-row bug this file's UpdateSessionMetadata exists to
		// avoid. Flushing here first keeps every later persist call in this handler
		// looking up the same, already-correct row.
		if metaTitle != nil || metaCategory != nil || metaNote != nil {
			if err := s.storage.UpdateInstanceMetadata(currentTitle, metaTitle, metaCategory, metaNote, nil); err != nil {
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save instance: %w", err))
			}
			if metaTitle != nil {
				currentTitle = *metaTitle
			}
			metaTitle, metaCategory, metaNote = nil, nil, nil
		}
		changed, _, switchErr := instance.SwitchProgram(ctx, *req.Msg.Program, func() error {
			return s.storage.SaveInstances([]*session.Instance{instance})
		})
		if changed {
			updatedFields = append(updatedFields, "program")
		}
		if switchErr != nil {
			log.Error("[UpdateSession] failed to restart session after program change", "session", instance.Title, "err", switchErr)
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to restart session after program change: %w", switchErr))
		}
	}

	// Handle working directory update
	if req.Msg.WorkingDir != nil {
		instance.SetWorkingDir(*req.Msg.WorkingDir)
		updatedFields = append(updatedFields, "working_dir")
		metaWorkingDir = req.Msg.WorkingDir
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
		sideEffectChanged = true
	}

	// Handle autonomous mode toggle. Starting/stopping the AutonomousDriver is a
	// live side-effect; we only act when the value actually changes.
	if req.Msg.AutonomousMode != nil && *req.Msg.AutonomousMode != instance.AutonomousMode {
		if *req.Msg.AutonomousMode && s.headlessPool == nil {
			return nil, connect.NewError(connect.CodeFailedPrecondition,
				fmt.Errorf("autonomous mode requires a headless LLM to be configured"))
		}
		instance.SetAutonomousMode(*req.Msg.AutonomousMode, "")
		if instance.AutonomousMode {
			s.StartAutonomousDriverForInstance(instance)
		} else {
			s.autonomousSvc.stopAndDeregisterDriver(instance.Title)
		}
		updatedFields = append(updatedFields, "autonomous_mode")
		sideEffectChanged = true
	}

	// Handle auto-approve toggle. Restart-on-Active-change (serialized against a
	// concurrent program switch via restartTriggerMu) is handled inside SetAutoApprove.
	// Same server-side invariant as CreateSession: auto_approve=true is rejected for an
	// agent yoloFlagFor can't inject a bypass flag for, not just disabled client-side.
	if req.Msg.AutoApprove != nil && *req.Msg.AutoApprove != instance.AutoApprove {
		if *req.Msg.AutoApprove && !session.AutoApproveSupported(instance.Program) {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("auto_approve is not supported for program %q", instance.Program))
		}
		if err := instance.SetAutoApprove(*req.Msg.AutoApprove, func() error {
			return s.storage.SaveInstances([]*session.Instance{instance})
		}); err != nil {
			log.Error("[UpdateSession] failed to restart session after auto-approve change", "session", instance.Title, "err", err)
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to restart session after auto-approve change: %w", err))
		}
		updatedFields = append(updatedFields, "auto_approve")
	}

	// Handle steering: inject a message into an active session. Autonomous
	// sessions keep the existing ClaudeController command-queue path (ADR-001);
	// non-autonomous, Instance-backed sessions fall back to the same PTY send
	// primitive the MCP steer_session tool already uses (tools_terminal.go's
	// SendKeys fallback branch) so browser-originated steering reaches ordinary
	// backlog work/review sessions too, not just autonomous ones.
	if req.Msg.SteerMessage != nil && *req.Msg.SteerMessage != "" {
		if len(*req.Msg.SteerMessage) > session.MaxSteerMessageLength {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("steer_message exceeds maximum length of %d bytes", session.MaxSteerMessageLength))
		}
		if instance.AutonomousMode {
			// Unchanged: autonomous sessions keep the ClaudeController command-queue path.
			controller := instance.GetController()
			if controller != nil {
				if _, sendErr := controller.SendCommandImmediate(*req.Msg.SteerMessage + "\r"); sendErr != nil {
					log.Warn("[UpdateSession] failed to send steer_message", "session", instance.Title, "err", sendErr)
				} else {
					s.notifySteerSent(instance, *req.Msg.SteerMessage)
				}
			}
		} else {
			// New: non-autonomous, Instance-backed sessions get the same PTY send
			// primitive the MCP steer_session tool already falls back to. Unlike the
			// autonomous branch, a send failure IS returned to the caller so the UI
			// can surface it (research/ux.md's Gap 2 error-state table).
			//
			// SendKeys is bounded with a timeout, mirroring terminal_service.go's
			// WriteToSession — a browser click against a wedged/dead session must not
			// hang this RPC handler goroutine forever.
			text := session.BuildSubmittableInput(*req.Msg.SteerMessage, true)
			errCh := make(chan error, 1)
			go func() { errCh <- instance.SendKeys(text) }()

			timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			select {
			case err := <-errCh:
				if err != nil {
					return nil, connect.NewError(connect.CodeFailedPrecondition,
						fmt.Errorf("failed to steer session %q: %w", instance.Title, err))
				}
			case <-timeoutCtx.Done():
				return nil, connect.NewError(connect.CodeDeadlineExceeded,
					fmt.Errorf("timed out steering session %q", instance.Title))
			}
			s.notifySteerSent(instance, *req.Msg.SteerMessage)
		}
	}

	// Handle status change (pause/resume) LAST - after all metadata updates.
	// This ensures that if Resume() fails, no partial metadata changes are persisted
	// (save only happens after all changes succeed).
	if req.Msg.Status != nil && *req.Msg.Status != sessionv1.SessionStatus_SESSION_STATUS_UNSPECIFIED {
		targetStatus := adapters.ProtoToStatus(*req.Msg.Status)

		if targetStatus == session.Paused && instance.Status != session.Paused {
			if err := instance.Pause(); err != nil {
				return nil, classifyPauseResumeErr(err, "pause")
			}
			// Set pause reason after a successful transition — mirrors HibernateSession
			// pattern, and avoids stamping the reason on a request that got rejected
			// (permission denied, invalid transition).
			if req.Msg.PauseReason == nil || *req.Msg.PauseReason == "" {
				instance.SetPauseReason(session.PauseReasonManual)
			} else {
				instance.SetPauseReason(*req.Msg.PauseReason)
			}
			updatedFields = append(updatedFields, "status")
			sideEffectChanged = true
		} else if targetStatus != session.Paused && instance.Status == session.Paused {
			// Resume from paused state
			if err := instance.Resume(); err != nil {
				return nil, classifyPauseResumeErr(err, "resume")
			}
			// Clear pause reason only after a successful resume.
			instance.SetPauseReason("")
			updatedFields = append(updatedFields, "status")
			sideEffectChanged = true
		}
	}

	// Persist changes. The narrow metadata UPDATE runs first so a title rename lands
	// under currentTitle in the DB before any side-effecting SaveInstances call below
	// looks the row up by the already-in-memory-mutated new title — doing it in the
	// other order would miss the still-old-titled DB row and orphan it via
	// SaveInstances' Update-fails-so-Create fallback (see UpdateSessionMetadata).
	if metaTitle != nil || metaCategory != nil || metaNote != nil || metaWorkingDir != nil {
		if err := s.storage.UpdateInstanceMetadata(currentTitle, metaTitle, metaCategory, metaNote, metaWorkingDir); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save instance: %w", err))
		}
	}
	if sideEffectChanged {
		if err := s.storage.SaveInstances([]*session.Instance{instance}); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save instance: %w", err))
		}
	}

	// Publish events based on what was updated
	if len(updatedFields) > 0 {
		s.publishSessionUpdatedEvent(instance, updatedFields)
	}

	return connect.NewResponse(&sessionv1.UpdateSessionResponse{
		Session: adapters.InstanceToProto(instance, s.workflowNames()),
	}), nil
}

// notifySteerSent logs and publishes the "steering input sent" notification
// shared by both the autonomous and non-autonomous steer branches in
// UpdateSession.
func (s *SessionService) notifySteerSent(instance *session.Instance, steerMessage string) {
	log.Info("[UpdateSession] steering message sent", "session", instance.Title)
	s.eventBus.Publish(events.NewNotificationEvent(
		instance.UUID, instance.Title, fmt.Sprintf("steer-%s", instance.UUID),
		int32(10), // NotificationType_INFO
		int32(2),  // NotificationPriority_MEDIUM
		"Steering input sent",
		fmt.Sprintf("%s: %s", instance.Title, steerMessage),
		nil,
	))
}

// HibernateSession checkpoints the session state, kills the AI process, and
// transitions the session to Hibernated status.
// +api: session:hibernate
func (s *SessionService) HibernateSession(
	ctx context.Context,
	req *connect.Request[sessionv1.HibernateSessionRequest],
) (*connect.Response[sessionv1.HibernateSessionResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	instances, err := s.storage.LoadInstances()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
	}

	var instance *session.Instance
	for _, inst := range instances {
		if inst.MatchesID(req.Msg.Id) {
			instance = inst
			break
		}
	}
	if instance == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.Id))
	}

	// Stop any running autonomous driver so it does not continue injecting prompts
	// into a session whose process is about to be killed.
	s.autonomousSvc.stopAndDeregisterDriver(instance.Title)

	// Set reason before transitioning so the After hook can read it
	reason := req.Msg.Reason
	if reason == "" {
		reason = "manual"
	}
	instance.SetHibernateReason(reason)

	if err := instance.Hibernate(ctx); err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}

	// Remove from the live poller and review queue immediately so the hibernated
	// session does not linger with a stale queue entry until reconcileSessions fires.
	s.removeFromAllPollers(instance.Title)

	if err := s.storage.SaveInstances([]*session.Instance{instance}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save instance: %w", err))
	}

	s.eventBus.Publish(events.NewSessionUpdatedEvent(instance, []string{"status"}))

	return connect.NewResponse(&sessionv1.HibernateSessionResponse{
		Session: adapters.InstanceToProto(instance, s.workflowNames()),
	}), nil
}

// ResumeHibernatedSession re-launches the AI process for a Hibernated session,
// transitioning it back to Active status.
// +api: session:resume_hibernated
func (s *SessionService) ResumeHibernatedSession(
	ctx context.Context,
	req *connect.Request[sessionv1.ResumeHibernatedSessionRequest],
) (*connect.Response[sessionv1.ResumeHibernatedSessionResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	instances, err := s.storage.LoadInstances()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
	}

	var instance *session.Instance
	for _, inst := range instances {
		if inst.MatchesID(req.Msg.Id) {
			instance = inst
			break
		}
	}
	if instance == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.Id))
	}

	if err := instance.ResumeFromHibernation(ctx); err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}

	if err := s.storage.SaveInstances([]*session.Instance{instance}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save instance: %w", err))
	}

	s.eventBus.Publish(events.NewSessionUpdatedEvent(instance, []string{"status"}))

	return connect.NewResponse(&sessionv1.ResumeHibernatedSessionResponse{
		Session: adapters.InstanceToProto(instance, s.workflowNames()),
	}), nil
}

// ResumeCrashedSession re-launches the AI process for a Crashed session (dead
// tmux pane detected by SessionHealthChecker, session/health.go), transitioning
// it back to Active status. The tmux session was already killed when the
// instance was marked Crashed, so Start(false) takes the cold-restore path and
// threads --resume automatically when a conversation UUID is known.
// +api: session:resume_crashed
func (s *SessionService) ResumeCrashedSession(
	ctx context.Context,
	req *connect.Request[sessionv1.ResumeCrashedSessionRequest],
) (*connect.Response[sessionv1.ResumeCrashedSessionResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	instances, err := s.storage.LoadInstances()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
	}

	var instance *session.Instance
	for _, inst := range instances {
		if inst.MatchesID(req.Msg.Id) {
			instance = inst
			break
		}
	}
	if instance == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.Id))
	}

	if err := instance.ResumeFromCrash(ctx); err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}

	if err := s.storage.SaveInstances([]*session.Instance{instance}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save instance: %w", err))
	}

	s.eventBus.Publish(events.NewSessionUpdatedEvent(instance, []string{"status"}))

	return connect.NewResponse(&sessionv1.ResumeCrashedSessionResponse{
		Session: adapters.InstanceToProto(instance, s.workflowNames()),
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

	// Stop any running autonomous driver before destroying resources.
	// This prevents the driver goroutine from calling inst.Preview() or SendCommandImmediate
	// on a freed/cleaned-up instance after the session is gone (use-after-delete hazard).
	s.autonomousSvc.stopAndDeregisterDriver(sessionTitle)

	// Capture the live instance BEFORE removing from pollers. removeFromAllPollers
	// (below) evicts this session from the ReviewQueuePoller's instance list — the
	// exact list FindLiveInstance searches — so calling FindLiveInstance after
	// removeFromAllPollers always returned nil here, silently skipping Destroy()'s
	// git worktree cleanup for every delete (live or not) in favor of the
	// tmux-only KillTmuxSessionByTitle fallback. Capturing the pointer first fixes
	// that without reopening the race the ordering comment below is about (that
	// race is between removeFromAllPollers and storage.DeleteInstance, not this).
	liveInst := s.FindLiveInstance(sessionTitle)

	// Remove from all pollers BEFORE deleting from storage. This is atomic from the
	// poller's perspective and closes the race window where external discovery could
	// re-add the session between storage deletion and the old LoadInstances() reload.
	// Use sessionTitle (not req.Msg.Id) — pollers index by title, and req.Msg.Id may be a UUID.
	s.removeFromAllPollers(sessionTitle)

	// Destroy tmux/git resources asynchronously so the RPC returns immediately
	// after storage deletion. Cleanup errors are non-fatal — they are logged and
	// do not affect the success response the caller receives. Both goroutines are
	// tracked via deleteCleanupWG so Shutdown (and tests) can await them instead
	// of letting them outlive the process/test — see deleteCleanupWG's doc comment.
	if liveInst != nil {
		s.trackCleanup(func() {
			if err := destroyWithTimeout(liveInst, deleteSessionCleanupTimeout); err != nil {
				log.Warn("failed to cleanup session resources", "session", req.Msg.Id, "err", err)
			}
		})
	} else {
		// Instance is not in the live in-memory poller (e.g. the server restarted
		// since this session was created). Fall back to killing the tmux session by
		// its deterministic name so the Claude process inside it doesn't survive as
		// an orphan after the DB record is gone.
		s.trackCleanup(func() {
			if err := s.KillTmuxSessionByTitle(context.Background(), sessionTitle); err != nil {
				log.Warn("failed to kill tmux session for non-live instance", "session", req.Msg.Id, "err", err)
			}
		})
	}

	// Cancel any pending approvals BEFORE deleting from storage, so blocked
	// approval-hook goroutines can exit cleanly while the session still exists.
	// Non-fatal: log at warn and continue even if there are no pending approvals.
	if cancelled := s.approvalStore.CancelSession(sessionUUID); len(cancelled) > 0 {
		log.Warn("cancelled pending approvals for deleted session", "session", req.Msg.Id, "count", len(cancelled))
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
	// Subscribe before building the snapshot so no events are lost between the
	// two phases (snapshot races are resolved by client-side upsert semantics).
	eventCh, subID := s.eventBus.Subscribe(ctx)
	defer s.eventBus.Unsubscribe(subID)

	if req.Msg.AfterSeq > 0 {
		// Reconnecting client: replay events missed since last disconnect.
		// This covers the period between disconnect and the new subscription above.
		for _, event := range s.eventBus.EventsSince(req.Msg.AfterSeq) {
			if event.Session != nil && event.Session.Hidden {
				continue
			}
			if err := stream.Send(convertEventToProto(event)); err != nil {
				return fmt.Errorf("failed to send replayed event: %w", err)
			}
		}
	} else {
		// Fresh connection: send initial snapshot using in-memory poller cache —
		// avoids a full SQLite scan on every new WatchSessions connection.
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

		for _, inst := range instances {
			if req.Msg.CategoryFilter != nil && *req.Msg.CategoryFilter != "" {
				if inst.Category != *req.Msg.CategoryFilter {
					continue
				}
			}
			if req.Msg.StatusFilter != nil && *req.Msg.StatusFilter != sessionv1.SessionStatus_SESSION_STATUS_UNSPECIFIED {
				// inst.GetStatus() reads the lock-free published snapshot rather than
				// inst.Status directly -- see reconcileSessions' identical fix
				// (9fcded805) for why a raw field read here races with the actor's
				// transitionToLocked write under -race.
				if adapters.StatusToProto(session.Status(inst.GetStatus())) != *req.Msg.StatusFilter {
					continue
				}
			}
			if inst.Hidden {
				continue
			}
			if err := stream.Send(createInitialSnapshotEvent(inst)); err != nil {
				return fmt.Errorf("failed to send initial snapshot: %w", err)
			}
		}
	}

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
				if event.Session != nil && adapters.StatusToProto(session.Status(event.Session.GetStatus())) != *req.Msg.StatusFilter {
					continue
				}
			}

			if event.Session != nil && event.Session.Hidden {
				continue
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
// Implements bidirectional streaming where:
// - Client sends: terminal input and resize events
// - Server sends: raw terminal output
//
// NOTE: browser clients never reach this method directly — the WebSocket
// handler (connectrpc_websocket.go) intercepts StreamTerminal calls made
// over its custom websocket transport before they reach here. This handler
// exists to satisfy the ConnectRPC service interface and could be used by
// non-browser gRPC/Connect clients.
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
		log.Warn("[StreamTerminal] instance not found in poller, loading from storage (timestamps may desync)", "session", initialMsg.SessionId)
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
		log.Error("[StreamSession] failed to get PTY reader", "session", instance.Title, "err", err)
		return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get PTY reader: %w", err))
	}

	// Duplicate the PTY fd for this goroutine's exclusive use. ptyFile is
	// shared with the instance's own internal consumers (response stream,
	// command executor), so calling SetReadDeadline directly on it would
	// mutate poll.FD state those other readers depend on. A dup'd fd gets
	// its own independent *os.File/poll.FD — closing or setting a deadline
	// on readFile has no effect on ptyFile or its other readers, since the
	// underlying open file description is only released once every fd
	// referencing it is closed. dupPTYFile is platform-specific
	// (dup_fd_unix.go / dup_fd_windows.go) since syscall.Dup isn't available
	// on Windows.
	readFile, err := dupPTYFile(ptyFile)
	if err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}

	// Create context for managing goroutines
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Channel for errors from goroutines
	errCh := make(chan error, 2)

	// wg tracks both goroutines below so the handler never returns (letting
	// Connect close the underlying stream) while either might still be
	// calling stream.Send/stream.Receive — doing so races with Connect's own
	// end-of-stream write. See BUG-025 follow-up: caught by -race under a
	// real PTY-backed StreamTerminal test.
	var wg sync.WaitGroup

	// sendMu serializes every stream.Send() call across the two goroutines
	// below. connect-go's BidiStream.Send() is documented as unsafe for
	// concurrent use from multiple goroutines: goroutine 1 continuously sends
	// PTY output while goroutine 2 can, on error, send an error message back
	// to the client (WRITE_ERROR / RESIZE_ERROR) — those two goroutines are
	// otherwise independent (one pumps PTY->client, the other pumps
	// client->PTY), so without a shared lock a PTY-output Send() and an
	// input-goroutine error-reply Send() can execute at the same instant on
	// the same stream. Caught by -race under a real PTY-backed StreamTerminal
	// test. Single-writer-via-channel was considered but would require
	// funneling ALL sends (including the hot PTY-output path) through an
	// extra hop; a mutex is the minimal change here since sends are already
	// synchronous, best-effort calls with no ordering requirements beyond
	// mutual exclusion.
	var sendMu sync.Mutex
	sendLocked := func(msg *sessionv1.TerminalData) error {
		sendMu.Lock()
		defer sendMu.Unlock()
		return stream.Send(msg)
	}

	// Flow control state for backpressure management
	// Reference: https://xtermjs.org/docs/guides/flowcontrol/
	pauseCh := make(chan bool, 1) // Buffered channel for pause/resume signals
	var ptyPaused bool            // Current PTY pause state

	// Goroutine 1: Read from PTY and send deltas to client (terminal output)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer readFile.Close() // our own dup'd fd; does not affect ptyFile or its other readers
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
						log.Info("[FlowControl] PTY reading RESUMED", "session", initialMsg.SessionId)
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
					log.Info("[FlowControl] PTY reading PAUSED", "session", initialMsg.SessionId)
				}
			default:
				// A short deadline on our own dup'd fd (see readFile above)
				// bounds how long Read can block, so this goroutine notices
				// streamCtx cancellation promptly instead of potentially
				// blocking until the next real PTY output — which could
				// arrive well after the handler has returned and Connect has
				// closed the stream. Safe to set here because readFile is
				// exclusively ours; it does not touch ptyFile's poll.FD.
				_ = readFile.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
				n, readErr := readFile.Read(buf)
				if n > 0 {
					// Update terminal activity timestamps with the output content
					// This ensures LastMeaningfulOutput reflects web UI viewing activity
					instance.UpdateTerminalTimestamps(string(buf[:n]), true)

					select {
					case <-streamCtx.Done():
						return
					default:
					}

					outputMsg := &sessionv1.TerminalData{
						SessionId: initialMsg.SessionId,
						Data: &sessionv1.TerminalData_Output{
							Output: &sessionv1.TerminalOutput{
								Data: buf[:n],
							},
						},
					}
					if sendErr := sendLocked(outputMsg); sendErr != nil {
						errCh <- fmt.Errorf("failed to send output: %w", sendErr)
						return
					}
				}

				if readErr != nil {
					if netErr, ok := readErr.(interface{ Timeout() bool }); ok && netErr.Timeout() {
						// Expected: the deadline above elapsed with no data.
						// Loop back around to re-check streamCtx/pauseCh.
						continue
					}
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
	wg.Add(1)
	go func() {
		defer wg.Done()
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
						_ = sendLocked(errorMsg) // Best effort
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
					// Handle terminal resize
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
						_ = sendLocked(errorMsg) // Best effort
						// Don't return on resize errors, they're not fatal
					} else {
						log.Info("resized terminal", "cols", cols, "rows", rows, "session", msg.SessionId)
					}

				case *sessionv1.TerminalData_FlowControl:
					// Handle flow control signals from client
					// Reference: https://xtermjs.org/docs/guides/flowcontrol/
					if data.FlowControl.Paused {
						log.Info("[FlowControl] client requested PAUSE", "watermark_bytes", data.FlowControl.Watermark, "session", msg.SessionId)
						// Signal PTY reading goroutine to pause
						select {
						case pauseCh <- true:
						default:
							// Channel already has pause signal, skip
						}
					} else {
						log.Info("[FlowControl] client requested RESUME", "watermark_bytes", data.FlowControl.Watermark, "session", msg.SessionId)
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
					log.Warn("[StreamTerminal] CurrentPaneRequest received (unexpected - WebSocket handler should intercept this)")

				case *sessionv1.TerminalData_Error:
					// Client sent an error, log it
					log.Error("client error", "message", data.Error.Message, "code", data.Error.Code)
				}
			}
		}
	}()

	// Wait for either context cancellation or error, then wait for both
	// goroutines to actually stop before returning. Returning early lets
	// Connect close the underlying HTTP/2 stream (write trailers/end-stream);
	// if either goroutine is still mid-Send/Receive when that happens, the
	// concurrent writes to the same connection race — caught by -race even
	// with sendLocked in place, because sendLocked only serializes OUR two
	// goroutines against each other, not against Connect's own teardown write
	// once this function returns. Goroutine 1 is reliably bounded (its dup'd
	// fd's 250ms read deadline means it notices streamCtx.Done() promptly
	// regardless of PTY activity). Goroutine 2's stream.Receive() has no
	// equivalent deadline (connect-go's BidiStream doesn't expose one), so it
	// can genuinely still be blocked here — an earlier version of this code
	// gave up waiting after a short timeout and returned anyway, which is
	// exactly what let the race happen. logSlowShutdown never gives up: it
	// blocks until wg is actually done (so the race is structurally
	// impossible), merely logging if that's taking unusually long so a client
	// that never disconnects is still visible in logs rather than silently
	// leaking the goroutine.
	const shutdownWarnAfter = 2 * time.Second
	select {
	case <-streamCtx.Done():
		log.Info("StreamTerminal: context done", "session", initialMsg.SessionId)
		logSlowShutdown(&wg, shutdownWarnAfter, initialMsg.SessionId, "context done")
		return nil // Clean shutdown
	case err := <-errCh:
		log.Error("StreamTerminal error", "session", initialMsg.SessionId, "err", err)
		cancel() // streamCtx.Done() wasn't otherwise closed on this path; signal both goroutines to stop.
		logSlowShutdown(&wg, shutdownWarnAfter, initialMsg.SessionId, "error")
		return connect.NewError(connect.CodeInternal, err)
	}
}

// waitWithTimeout waits for wg to complete, returning true if it did so
// within timeout and false if the timeout elapsed first. On timeout, this
// bookkeeping goroutine itself is harmlessly leaked (it will eventually
// complete and close the now-unread done channel) — but the caller's own
// tracked goroutines may still be running and may still touch shared state
// (e.g. a stream) after this function returns false. Callers on the false
// path must treat that as a real, logged condition, not a no-op.
//
// StreamTerminal itself does NOT use this — see logSlowShutdown below for why
// "give up and return anyway" is unsafe there. Kept for its own direct test
// coverage (TestWaitWithTimeout) and as a building block other bounded-wait
// callers can use where returning on timeout doesn't race a shared resource.
func waitWithTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// logSlowShutdown blocks until wg completes — unconditionally, with no
// give-up. StreamTerminal's two goroutines both call stream.Send()/Receive();
// once this function's caller returns, Connect writes the stream's
// end-of-stream trailers on the same connection, which races with either
// goroutine if it's still mid-Send/Receive. The only way to make that
// structurally impossible is to never return while wg is incomplete — unlike
// waitWithTimeout, this cannot give up and let the caller proceed anyway.
//
// warnAfter only controls a one-time log line so a client that never
// disconnects (holding goroutine 2's stream.Receive() open indefinitely,
// since connect-go's BidiStream has no per-call read deadline to bound it)
// is visible in logs as a real leak, rather than either silently hanging
// forever unnoticed or racing the stream teardown.
func logSlowShutdown(wg *sync.WaitGroup, warnAfter time.Duration, sessionID, reason string) {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return
	case <-time.After(warnAfter):
		log.Warn("StreamTerminal: goroutines still running past warn threshold, waiting for them before returning (not giving up, to avoid racing Connect's stream teardown)",
			"session", sessionID, "reason", reason)
		<-done
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

	var diffStats *git.DiffStats

	instance := s.findInstance(req.Msg.Id)
	if instance != nil {
		// Live session: update and read cached diff.
		if err := instance.UpdateDiffStats(); err != nil {
			log.Warn("failed to update diff stats", "session", req.Msg.Id, "err", err)
		}
		diffStats = instance.GetDiffStats()
	} else {
		// Completed session: reconstruct worktree from DB and compute diff on-demand.
		allData, err := s.storage.ListInstanceData()
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list sessions: %w", err))
		}
		var found *session.InstanceData
		for i := range allData {
			if allData[i].MatchesID(req.Msg.Id) {
				found = &allData[i]
				break
			}
		}
		if found == nil {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.Id))
		}
		if found.Worktree.WorktreePath != "" {
			wt := git.NewGitWorktreeFromStorage(
				found.Worktree.RepoPath,
				found.Worktree.WorktreePath,
				found.Worktree.SessionName,
				found.Worktree.BranchName,
				found.Worktree.BaseCommitSHA,
			)
			diffStats = wt.Diff()
		} else if found.Path != "" {
			// ponytail: directory sessions have no worktree; use the session path directly.
			// resolveBaseCommitSHA() will find the merge-base with main/master as fallback.
			wt := git.NewGitWorktreeFromStorage(found.Path, found.Path, found.Title, "", "")
			if wt != nil {
				diffStats = wt.Diff()
			}
		}
	}

	if diffStats == nil {
		return connect.NewResponse(&sessionv1.GetSessionDiffResponse{
			DiffStats: &sessionv1.DiffStats{},
		}), nil
	}

	return connect.NewResponse(&sessionv1.GetSessionDiffResponse{
		DiffStats: &sessionv1.DiffStats{
			Added:   int32(diffStats.Added),
			Removed: int32(diffStats.Removed),
			Content: diffStats.Content,
		},
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

	// Use the live poller list for the same reason as UpdateSession: avoid LoadInstances
	// side-effects that can drop sessions and clobber the poller list via SetInstances.
	var instances []*session.Instance
	if s.reviewQueuePoller != nil {
		instances = s.reviewQueuePoller.GetInstances()
	} else {
		var loadErr error
		instances, loadErr = s.loadInstancesWithWiring()
		if loadErr != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", loadErr))
		}
	}

	// Find the instance to rename
	var instance *session.Instance
	for _, inst := range instances {
		if inst.MatchesID(req.Msg.Id) {
			instance = inst
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

	// Narrow single-row rename, keyed on the pre-mutation title. The generic
	// SaveInstances path looks the DB row up by the (already renamed) in-memory
	// title, misses the still-old-titled row, and falls into a Create fallback that
	// orphans it — using oldTitle as the WHERE key avoids that.
	if err := s.storage.UpdateInstanceMetadata(oldTitle, &req.Msg.NewTitle, nil, nil, nil); err != nil {
		// Try to rollback the rename
		instance.SetTitleDirect(oldTitle)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save renamed instance: %w", err))
	}

	// Publish SessionUpdated event
	s.eventBus.Publish(events.NewSessionUpdatedEvent(instance, []string{"title"}))

	log.Info("successfully renamed session", "from", oldTitle, "to", req.Msg.NewTitle)

	return connect.NewResponse(&sessionv1.RenameSessionResponse{
		Session: adapters.InstanceToProto(instance, s.workflowNames()),
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
		log.Error("[RestartSession] failed to restart session", "session", instance.Title, "err", err)
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

	log.Info(message)

	return connect.NewResponse(&sessionv1.RestartSessionResponse{
		Session: adapters.InstanceToProto(instance, s.workflowNames()),
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

// GetProgramAnalytics returns drill-down analytics for a single command program.
func (s *SessionService) GetProgramAnalytics(
	ctx context.Context,
	req *connect.Request[sessionv1.GetProgramAnalyticsRequest],
) (*connect.Response[sessionv1.GetProgramAnalyticsResponse], error) {
	return s.rulesSvc.GetProgramAnalytics(ctx, req)
}

// GenerateSuggestedRule asks an AI to propose new auto-approval rules.
func (s *SessionService) GenerateSuggestedRule(
	ctx context.Context,
	req *connect.Request[sessionv1.GenerateSuggestedRuleRequest],
) (*connect.Response[sessionv1.GenerateSuggestedRuleResponse], error) {
	return s.rulesSvc.GenerateSuggestedRule(ctx, req)
}

// ValidateRules parses and validates a YAML rules file without applying it.
func (s *SessionService) ValidateRules(
	ctx context.Context,
	req *connect.Request[sessionv1.ValidateRulesRequest],
) (*connect.Response[sessionv1.ValidateRulesResponse], error) {
	return s.rulesSvc.ValidateRules(ctx, req)
}

// ExportRules serializes user-authored rules to YAML format for download.
func (s *SessionService) ExportRules(
	ctx context.Context,
	req *connect.Request[sessionv1.ExportRulesRequest],
) (*connect.Response[sessionv1.ExportRulesResponse], error) {
	return s.rulesSvc.ExportRules(ctx, req)
}

// BulkUpsertRules creates or updates multiple user-defined rules in one call.
func (s *SessionService) BulkUpsertRules(
	ctx context.Context,
	req *connect.Request[sessionv1.BulkUpsertRulesRequest],
) (*connect.Response[sessionv1.BulkUpsertRulesResponse], error) {
	return s.rulesSvc.BulkUpsertRules(ctx, req)
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
func (s *SessionService) SetScrollbackManager(mgr ScrollbackSequencer) {
	s.scrollbackMgr = mgr
	s.checkpointSvc.SetScrollbackMgr(mgr)
}

// CreateCheckpoint captures the current state of a session as a named bookmark.
func (s *SessionService) CreateCheckpoint(
	ctx context.Context,
	req *connect.Request[sessionv1.CreateCheckpointRequest],
) (*connect.Response[sessionv1.CreateCheckpointResponse], error) {
	return s.checkpointSvc.CreateCheckpoint(ctx, req)
}

// ListCheckpoints returns all checkpoints for the specified session.
func (s *SessionService) ListCheckpoints(
	ctx context.Context,
	req *connect.Request[sessionv1.ListCheckpointsRequest],
) (*connect.Response[sessionv1.ListCheckpointsResponse], error) {
	return s.checkpointSvc.ListCheckpoints(ctx, req)
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
		log.Info("[ReviewQueue] updated poller instance references after ForkSession", "session", newInst.Title)
	}

	respProto := adapters.InstanceToProto(newInst, s.workflowNames())

	go func() {
		if startErr := newInst.Start(true); startErr != nil {
			log.Warn("ForkSession: failed to start forked session", "session", newInst.Title, "err", startErr)
			newInst.Status = session.Stopped
			if saveErr := s.storage.SaveInstances(s.allInstances()); saveErr != nil {
				log.Warn("ForkSession: failed to persist Stopped status", "session", newInst.Title, "err", saveErr)
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
	return s.checkpointSvc.ClearConversationState(ctx, req)
}

// ListPathCompletions returns filesystem entries matching the given path prefix.
func (s *SessionService) ListPathCompletions(
	ctx context.Context,
	req *connect.Request[sessionv1.ListPathCompletionsRequest],
) (*connect.Response[sessionv1.ListPathCompletionsResponse], error) {
	return s.pathCompletionSvc.ListPathCompletions(ctx, req)
}

func (s *SessionService) ListSlashCommands(
	ctx context.Context,
	req *connect.Request[sessionv1.ListSlashCommandsRequest],
) (*connect.Response[sessionv1.ListSlashCommandsResponse], error) {
	return s.slashCommandSvc.ListSlashCommands(ctx, req)
}

// ListWorktrees returns the git worktrees for a given repository path.
func (s *SessionService) ListWorktrees(
	ctx context.Context,
	req *connect.Request[sessionv1.ListWorktreesRequest],
) (*connect.Response[sessionv1.ListWorktreesResponse], error) {
	return s.pathCompletionSvc.ListWorktrees(ctx, req)
}

// ListBranches returns the git branches for a given repository path.
// Results are cached per repo path with a 5-minute TTL. ADR-002.
// Delegates to WorkspaceService (Story 1.4).
func (s *SessionService) ListBranches(
	ctx context.Context,
	req *connect.Request[sessionv1.ListBranchesRequest],
) (*connect.Response[sessionv1.ListBranchesResponse], error) {
	return s.workspaceSvc.ListBranches(ctx, req)
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

// GetLauncherPresets returns the hand-authored launcher presets, freshly read on every call.
func (s *SessionService) GetLauncherPresets(ctx context.Context, req *connect.Request[sessionv1.GetLauncherPresetsRequest]) (*connect.Response[sessionv1.GetLauncherPresetsResponse], error) {
	return s.launcherPresetsSvc.GetLauncherPresets(ctx, req)
}

// ResolveDefaults merges all default layers for the given working directory and profile.
func (s *SessionService) ResolveDefaults(ctx context.Context, req *connect.Request[sessionv1.ResolveDefaultsRequest]) (*connect.Response[sessionv1.ResolveDefaultsResponse], error) {
	return s.defaultsSvc.ResolveDefaults(ctx, req)
}

// UpdateGlobalDefaults replaces the global default fields.
func (s *SessionService) UpdateGlobalDefaults(ctx context.Context, req *connect.Request[sessionv1.UpdateGlobalDefaultsRequest]) (*connect.Response[sessionv1.UpdateGlobalDefaultsResponse], error) {
	return s.defaultsSvc.UpdateGlobalDefaults(ctx, req)
}

// SetOnGlobalDefaultsUpdated wires in the callback invoked after every
// successful UpdateGlobalDefaults save (server/dependencies.go uses this to
// trigger an immediate backlog-queue dequeue sweep when the concurrency limit
// is raised).
func (s *SessionService) SetOnGlobalDefaultsUpdated(fn func()) {
	s.defaultsSvc.SetOnGlobalDefaultsUpdated(fn)
}

// SetSharedBacklogConfig wires the *config.Config instance (and its guarding
// mutex) BacklogService reads its concurrency fields from into this
// SessionService's DefaultsService, so UpdateGlobalDefaults can propagate a
// Settings change into BacklogService's live view without a process restart
// (PR #199 review F1). See DefaultsService.SetSharedBacklogConfig.
func (s *SessionService) SetSharedBacklogConfig(cfg *config.Config, mu *sync.RWMutex) {
	s.defaultsSvc.SetSharedBacklogConfig(cfg, mu)
}

// SetSharedCallbackConfig wires the *config.Config instance (and its guarding
// mutex) CallbackDispatcher reads callback URLs from into this SessionService's
// CallbackConfigService, so UpdateCallbackConfig can propagate a saved URL into
// CallbackDispatcher's live view without a process restart. See
// CallbackConfigService.SetSharedCallbackConfig.
func (s *SessionService) SetSharedCallbackConfig(cfg *config.Config, mu *sync.RWMutex) {
	s.callbackConfigSvc.SetSharedCallbackConfig(cfg, mu)
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

// ─── Callback Config delegates (webhook-triggers Phase 5, FR7) ──────────────

// +api: callback-config:get
// GetCallbackConfig reports which outbound-callback URLs are configured.
func (s *SessionService) GetCallbackConfig(ctx context.Context, req *connect.Request[sessionv1.GetCallbackConfigRequest]) (*connect.Response[sessionv1.GetCallbackConfigResponse], error) {
	return s.callbackConfigSvc.GetCallbackConfig(ctx, req)
}

// +api: callback-config:update
// UpdateCallbackConfig sets one or more outbound-callback URLs.
func (s *SessionService) UpdateCallbackConfig(ctx context.Context, req *connect.Request[sessionv1.UpdateCallbackConfigRequest]) (*connect.Response[sessionv1.UpdateCallbackConfigResponse], error) {
	return s.callbackConfigSvc.UpdateCallbackConfig(ctx, req)
}

// ListAliases returns all configured alias presets.
func (s *SessionService) ListAliases(ctx context.Context, req *connect.Request[sessionv1.ListAliasesRequest]) (*connect.Response[sessionv1.ListAliasesResponse], error) {
	return s.defaultsSvc.ListAliases(ctx, req)
}

// UpsertAlias creates or updates a named alias preset.
func (s *SessionService) UpsertAlias(ctx context.Context, req *connect.Request[sessionv1.UpsertAliasRequest]) (*connect.Response[sessionv1.UpsertAliasResponse], error) {
	return s.defaultsSvc.UpsertAlias(ctx, req)
}

// DeleteAlias removes an alias preset by name.
func (s *SessionService) DeleteAlias(ctx context.Context, req *connect.Request[sessionv1.DeleteAliasRequest]) (*connect.Response[sessionv1.DeleteAliasResponse], error) {
	return s.defaultsSvc.DeleteAlias(ctx, req)
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

	// Clamp timeout: default 900 s (raised from 120 s for longer operations), max 1800 s.
	timeoutSecs := int(req.Msg.TimeoutSeconds)
	if timeoutSecs <= 0 {
		timeoutSecs = 900
	}
	if timeoutSecs > 1800 {
		timeoutSecs = 1800
	}

	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSecs)*time.Second)
	defer cancel()

	var outputStr string
	exitCode := 0
	errMsg := ""

	if s.headlessPool != nil {
		// Use headless pool for improved streaming and session reuse.
		var callErr error
		outputStr, _, callErr = s.headlessPool.CallBlocking(runCtx, headless.FeatureKeyCustom, "", req.Msg.Prompt, headless.CallOptions{WorkDir: workDir})
		if callErr != nil {
			errMsg = callErr.Error()
			exitCode = 1
		}
	} else {
		// Fallback: direct subprocess (requires claude in PATH).
		claudeBin, err := exec.LookPath("claude")
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("claude binary not found in PATH: %w", err))
		}

		cmd := safeexec.CommandContext(runCtx, claudeBin, "-p", req.Msg.Prompt)
		cmd.Dir = workDir

		output, runErr := cmd.CombinedOutput()
		if runErr != nil {
			if exitErr, ok := runErr.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				errMsg = runErr.Error()
			}
		}
		outputStr = string(output)
	}
	prURL := extractPRURL(outputStr)
	branchDiverged := checkBranchDivergence(workDir)

	// Persist the PR URL (and number) back to the session record so the GitHub badge appears
	// and the PRStatusPoller can use the direct-number path instead of branch-name discovery.
	if prURL != "" {
		prNumber := 0
		if ref, parseErr := session.ParseGitHubURL(prURL); parseErr == nil && ref.PRNumber > 0 {
			prNumber = ref.PRNumber
		}
		inst.SetGitHubPR(prURL, prNumber)
		if err := s.storage.SaveInstances(s.allInstances()); err != nil {
			log.Warn("RunOneShot: failed to persist PR URL", "session", inst.Title, "err", err)
		} else {
			s.eventBus.Publish(events.NewSessionUpdatedEvent(inst, []string{"github_pr_url", "github_pr_number"}))
		}

		// This RunOneShot call may have been the Review Queue's manual "Create
		// PR" button for a backlog-linked session — that flow creates the PR
		// entirely outside the automated pushAndCreatePR path, which is the
		// only other place that ever moves a backlog item to pr_pending. Without
		// this call the item is silently left in "review" forever, invisible to
		// ReconcilePRPending (see RecordPRCreatedOutOfBand's doc comment in
		// session/backlog_lifecycle.go for the full root-cause trace). No-op for
		// non-backlog sessions.
		if s.backlogLifecycleListener != nil {
			s.backlogLifecycleListener.RecordPRCreatedOutOfBand(ctx, inst.UUID, prURL, prNumber)
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

// RunOneShotForSession runs a one-shot prompt against a session's worktree without
// the ConnectRPC request/response wrapper, for automation callers. It reuses
// RunOneShot's exact logic (same PR-URL extraction, same PR persistence) so
// automated and manual PR creation share one code path — currently used by the
// opt-in AutoCreatePR review-queue policy (server.ReactiveQueueManager).
// Returns the extracted PR URL, or an error if the prompt failed.
func (s *SessionService) RunOneShotForSession(ctx context.Context, sessionID, prompt string, timeoutSeconds int32) (string, error) {
	resp, err := s.RunOneShot(ctx, connect.NewRequest(&sessionv1.RunOneShotRequest{
		SessionId:      sessionID,
		Prompt:         prompt,
		TimeoutSeconds: timeoutSeconds,
	}))
	if err != nil {
		return "", err
	}
	if resp.Msg.Error != "" {
		return "", fmt.Errorf("one-shot prompt failed: %s", resp.Msg.Error)
	}
	return resp.Msg.PrUrl, nil
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
	cmd := safeexec.CommandContext(ctx, "git", "rev-list", "--count", "origin/HEAD..HEAD")
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

// GetTerminalSnapshot returns the last N lines of terminal output for a session.
// Uses inst.Preview() for a read-only snapshot without requiring an active stream.
func (s *SessionService) GetTerminalSnapshot(
	ctx context.Context,
	req *connect.Request[sessionv1.GetTerminalSnapshotRequest],
) (*connect.Response[sessionv1.GetTerminalSnapshotResponse], error) {
	return s.terminalSvc.GetTerminalSnapshot(ctx, req)
}

// +api: session:log-client-events
// +api: session:write-to-session
// WriteToSession sends raw text input to a running session's PTY.
func (s *SessionService) WriteToSession(
	ctx context.Context,
	req *connect.Request[sessionv1.WriteToSessionRequest],
) (*connect.Response[sessionv1.WriteToSessionResponse], error) {
	return s.terminalSvc.WriteToSession(ctx, req)
}

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

// wireAutoArchiveCallback registers a lifecycle listener that auto-archives a
// workflow-spawned session when it exits.
func (s *SessionService) wireAutoArchiveCallback(inst *session.Instance) {
	if inst == nil || inst.WorkflowID == "" {
		return
	}
	inst.RegisterLifecycleListener(&autoArchiveListener{svc: s, inst: inst})
}

// autoArchiveListener implements session.LifecycleListener to archive workflow sessions on exit.
type autoArchiveListener struct {
	svc  *SessionService
	inst *session.Instance
}

func (l *autoArchiveListener) OnLifecycleEvent(event session.LifecycleEvent, _ string) {
	if event == session.EventExited {
		go l.svc.maybeAutoArchive(l.inst)
	}
}

// wireSessionExitedPublisher registers a lifecycle listener that publishes a
// SessionUpdatedEvent whenever a session exits unexpectedly (PTY EOF, process
// crash, reconcile). Without this, the frontend WatchSessions stream never
// learns the session stopped, leaving the "Thinking…" chip visible indefinitely.
func (s *SessionService) wireSessionExitedPublisher(inst *session.Instance) {
	if inst == nil {
		return
	}
	inst.RegisterLifecycleListener(&sessionExitedPublisher{svc: s, inst: inst})
}

// sessionExitedPublisher publishes a SessionUpdatedEvent and saves the instance
// when a session exits so the frontend receives the updated Stopped status.
type sessionExitedPublisher struct {
	svc  *SessionService
	inst *session.Instance
}

func (l *sessionExitedPublisher) OnLifecycleEvent(event session.LifecycleEvent, _ string) {
	if event != session.EventExited {
		return
	}
	go func() {
		_ = l.svc.storage.SaveInstances([]*session.Instance{l.inst})
		l.svc.eventBus.Publish(events.NewSessionUpdatedEvent(l.inst, []string{"status"}))
	}()
}

// wireStatusChangeCallback registers a ReactiveQueueManager callback on inst so that
// ClaudeController status transitions immediately trigger a CheckSession call, bypassing
// the poll cycle. Also publishes a session update event so WatchSessions clients receive
// the detection state change without waiting for the next poll cycle.
// Safe to call before or after the controller is started.
func (s *SessionService) wireStatusChangeCallback(inst *session.Instance) {
	if inst == nil || s.reviewQueueSvc == nil {
		return
	}
	mgr := s.reviewQueueSvc.GetReactiveQueueManager()
	if mgr == nil {
		return
	}
	inst.SetStatusChangeCallback(func(newStatus detection.DetectedStatus, context string) {
		mgr.OnControllerStatusChange(inst, newStatus)
		s.eventBus.Publish(events.NewSessionUpdatedEventWithDetection(
			inst, []string{"detected_status"},
			newStatus, context,
		))
	})
}

// rateLimitLookupTimeout bounds the ItemSession lookup performed by
// onRateLimitDetected/onRateLimitRecovery to resolve a Hidden, backlog-linked
// session's item_id for notification metadata. These callbacks run from
// goroutines in the ratelimit package, so an unbounded lookup could otherwise
// hang that goroutine indefinitely on a slow/stuck storage backend.
const rateLimitLookupTimeout = 2 * time.Second

// wireRateLimitCallbacks registers server-level callbacks on an Instance so that
// rate-limit detection and recovery events are published to the event bus and
// trigger desktop push notifications.
func (s *SessionService) wireRateLimitCallbacks(inst *session.Instance) {
	if inst == nil {
		return
	}
	inst.SetRateLimitCallbacks(
		func(sessionID string, resetTime time.Time) {
			s.onRateLimitDetected(inst, sessionID, resetTime)
		},
		func(sessionID string, success bool, errMsg string) {
			s.onRateLimitRecovery(inst, sessionID, success, errMsg)
		},
	)
}

// rateLimitLinkedItemID looks up the backlog item ID linked to inst's session
// (via concStorage), bounded by rateLimitLookupTimeout. Returns "" when the
// instance isn't backlog-linked, when concStorage is nil (fake InstanceStore
// backing, e.g. in some test setups), or when the lookup fails/times out.
func (s *SessionService) rateLimitLinkedItemID(inst *session.Instance) string {
	if s.concStorage == nil {
		return ""
	}
	lookupCtx, cancel := context.WithTimeout(context.Background(), rateLimitLookupTimeout)
	defer cancel()
	itemSession, err := s.concStorage.GetItemSessionBySessionUUID(lookupCtx, inst.UUID)
	if err != nil {
		if !errors.Is(err, session.ErrNotFound) {
			log.Warn("wireRateLimitCallbacks: ItemSession lookup failed", "session", inst.UUID, "err", err)
		}
		return ""
	}
	return itemSession.BacklogItemID
}

// onRateLimitDetected publishes the rate-limit-detected notification for inst.
// Hidden instances (e.g. headless review sessions) never surface a
// notification for this.
func (s *SessionService) onRateLimitDetected(inst *session.Instance, sessionID string, resetTime time.Time) {
	if !inst.Hidden {
		linkedItemID := s.rateLimitLinkedItemID(inst)

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
			events.SessionScopedMetadata(nil, linkedItemID),
		))
	}
	// Session state sync (rate_limit_state/rate_limit_reset_time) must fire
	// regardless of Hidden — only the Notifications-page entry above is gated.
	s.eventBus.Publish(events.NewSessionUpdatedEvent(inst, []string{"rate_limit_state", "rate_limit_reset_time"}))

	// Feed the account-wide quota gate's hard/reactive override signal. Not
	// gated on inst.Hidden (unlike the notification above) — a rate limit hit
	// by a hidden/headless session still consumes real account quota.
	if s.quotaGate != nil {
		s.quotaGate.recordRateLimitEvent(time.Now())
	}
}

// onRateLimitRecovery publishes the rate-limit-recovery notification for inst.
// Hidden instances (e.g. headless review sessions) never surface a
// notification for this.
func (s *SessionService) onRateLimitRecovery(inst *session.Instance, sessionID string, success bool, errMsg string) {
	if !inst.Hidden {
		linkedItemID := s.rateLimitLinkedItemID(inst)

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
			title, message,
			events.SessionScopedMetadata(nil, linkedItemID),
		))
	}
	// Session state sync (rate_limit_state) must fire regardless of Hidden —
	// only the Notifications-page entry above is gated.
	s.eventBus.Publish(events.NewSessionUpdatedEvent(inst, []string{"rate_limit_state"}))
}

// wireClaudeSessionIDCallback registers a callback on inst so that when the
// session driver captures a Claude session_id, the instance is persisted.
func (s *SessionService) wireClaudeSessionIDCallback(inst *session.Instance) {
	if inst == nil {
		return
	}
	inst.SetClaudeSessionIDSavedCallback(func() {
		_ = s.storage.SaveInstances([]*session.Instance{inst})
	})
}

// logClientEntry writes a single browser log entry to the server log.
func logClientEntry(e *sessionv1.ClientLogEntry) {
	msg := sanitizeClientLogField(e.GetMessage(), 200)
	ua := sanitizeClientLogField(e.GetUserAgent(), 80)
	sid := sanitizeClientLogField(e.GetSessionId(), 64)
	lvl := sanitizeClientLogField(e.GetLevel(), 16)
	url := sanitizeClientLogField(e.GetUrl(), 256)

	args := []any{"level", lvl, "session", sid, "url", url, "ua", ua}
	if lvl == "error" {
		log.Error("[client-log] "+msg, args...)
	} else {
		log.Info("[client-log] "+msg, args...)
	}
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

// +api: feature-flags:list
// GetFeatureFlags returns all known feature flags and their current state.
func (s *SessionService) GetFeatureFlags(
	ctx context.Context,
	req *connect.Request[sessionv1.GetFeatureFlagsRequest],
) (*connect.Response[sessionv1.GetFeatureFlagsResponse], error) {
	return s.featureFlagSvc.GetFeatureFlags(ctx, req)
}

// +api: feature-flags:update
// UpdateFeatureFlag enables or disables a named feature flag and persists the change.
func (s *SessionService) UpdateFeatureFlag(
	ctx context.Context,
	req *connect.Request[sessionv1.UpdateFeatureFlagRequest],
) (*connect.Response[sessionv1.UpdateFeatureFlagResponse], error) {
	return s.featureFlagSvc.UpdateFeatureFlag(ctx, req)
}

// SetWorkflowService injects the workflow sub-service using deferred setter injection.
// Must be called after both SessionService and WorkflowService are constructed.
func (s *SessionService) SetWorkflowService(svc *WorkflowService) {
	s.workflowSvc = svc
}

// SetWorkflowRepository injects the workflow repository used to populate the meta cache.
// Must be called after both SessionService and WorkflowRepository are constructed.
func (s *SessionService) SetWorkflowRepository(repo session.WorkflowRepository) {
	s.workflowRepo = repo
	s.refreshWorkflowMetaCache(context.Background())
}

// refreshWorkflowMetaCache reloads all workflow names and archiveAfterHours from the repo.
func (s *SessionService) refreshWorkflowMetaCache(ctx context.Context) {
	if s.workflowRepo == nil {
		return
	}
	wfs, err := s.workflowRepo.ListAll(ctx)
	if err != nil {
		log.Warn("[SessionService] failed to refresh workflow meta cache", "err", err)
		return
	}
	cache := make(map[string]workflowMeta, len(wfs))
	for _, wf := range wfs {
		cache[wf.ID.String()] = workflowMeta{
			name:              wf.Name,
			archiveAfterHours: wf.ArchiveAfterHours,
		}
	}
	s.workflowMetaMu.Lock()
	s.workflowMetaCache = cache
	s.workflowMetaMu.Unlock()
}

// workflowNames returns a snapshot of the workflow ID→name map for use in InstanceToProto.
func (s *SessionService) workflowNames() map[string]string {
	s.workflowMetaMu.RLock()
	defer s.workflowMetaMu.RUnlock()
	if len(s.workflowMetaCache) == 0 {
		return nil
	}
	m := make(map[string]string, len(s.workflowMetaCache))
	for id, meta := range s.workflowMetaCache {
		m[id] = meta.name
	}
	return m
}

// +api: workflow:create
// CreateWorkflow delegates to WorkflowService.
func (s *SessionService) CreateWorkflow(ctx context.Context, req *connect.Request[sessionv1.CreateWorkflowRequest]) (*connect.Response[sessionv1.CreateWorkflowResponse], error) {
	if s.workflowSvc == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("workflow service not available"))
	}
	return s.workflowSvc.CreateWorkflow(ctx, req)
}

// +api: workflow:update
// UpdateWorkflow delegates to WorkflowService.
func (s *SessionService) UpdateWorkflow(ctx context.Context, req *connect.Request[sessionv1.UpdateWorkflowRequest]) (*connect.Response[sessionv1.UpdateWorkflowResponse], error) {
	if s.workflowSvc == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("workflow service not available"))
	}
	return s.workflowSvc.UpdateWorkflow(ctx, req)
}

// +api: workflow:delete
// DeleteWorkflow delegates to WorkflowService.
func (s *SessionService) DeleteWorkflow(ctx context.Context, req *connect.Request[sessionv1.DeleteWorkflowRequest]) (*connect.Response[sessionv1.DeleteWorkflowResponse], error) {
	if s.workflowSvc == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("workflow service not available"))
	}
	return s.workflowSvc.DeleteWorkflow(ctx, req)
}

// +api: workflow:list
// ListWorkflows delegates to WorkflowService.
func (s *SessionService) ListWorkflows(ctx context.Context, req *connect.Request[sessionv1.ListWorkflowsRequest]) (*connect.Response[sessionv1.ListWorkflowsResponse], error) {
	if s.workflowSvc == nil {
		return connect.NewResponse(&sessionv1.ListWorkflowsResponse{
			Workflows: []*sessionv1.WorkflowProto{},
		}), nil
	}
	return s.workflowSvc.ListWorkflows(ctx, req)
}

// +api: workflow:run
// RunWorkflow delegates to WorkflowService.
func (s *SessionService) RunWorkflow(ctx context.Context, req *connect.Request[sessionv1.RunWorkflowRequest]) (*connect.Response[sessionv1.RunWorkflowResponse], error) {
	if s.workflowSvc == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("workflow service not available"))
	}
	return s.workflowSvc.RunWorkflow(ctx, req)
}

// +api: workflow:list-trigger-fire-events
// ListTriggerFireEvents delegates to WorkflowService.
func (s *SessionService) ListTriggerFireEvents(ctx context.Context, req *connect.Request[sessionv1.ListTriggerFireEventsRequest]) (*connect.Response[sessionv1.ListTriggerFireEventsResponse], error) {
	if s.workflowSvc == nil {
		return connect.NewResponse(&sessionv1.ListTriggerFireEventsResponse{
			Events: []*sessionv1.TriggerFireEventProto{},
		}), nil
	}
	return s.workflowSvc.ListTriggerFireEvents(ctx, req)
}

// GetDetectionEvents returns recent status-detection events for a session's Claude controller.
// Used by the debug panel (FR-8) — returns an empty list when the session has no active controller.
func (s *SessionService) GetDetectionEvents(ctx context.Context, req *connect.Request[sessionv1.GetDetectionEventsRequest]) (*connect.Response[sessionv1.GetDetectionEventsResponse], error) {
	inst := s.findInstance(req.Msg.SessionId)
	if inst == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session %q not found", req.Msg.SessionId))
	}

	limit := int(req.Msg.Limit)
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	if s.statusManager == nil {
		return connect.NewResponse(&sessionv1.GetDetectionEventsResponse{}), nil
	}

	controller, ok := s.statusManager.GetController(inst.Title)
	if !ok || controller == nil {
		return connect.NewResponse(&sessionv1.GetDetectionEventsResponse{}), nil
	}

	events := controller.GetStatusDetector().RecentEvents(limit)
	protoEvents := make([]*sessionv1.DetectionEventProto, 0, len(events))
	for _, e := range events {
		protoEvents = append(protoEvents, &sessionv1.DetectionEventProto{
			SessionId:       e.SessionID,
			Timestamp:       timestamppb.New(e.Timestamp),
			MatchedPattern:  e.MatchedPattern,
			MatchedCategory: e.MatchedCategory,
			ResultStatus:    int32(e.ResultStatus),
		})
	}
	return connect.NewResponse(&sessionv1.GetDetectionEventsResponse{Events: protoEvents}), nil
}

// +api: session:archive
// ArchiveSession soft-archives a session by setting archived_at.
// Archived sessions are excluded from the default ListSessions response.
func (s *SessionService) ArchiveSession(
	ctx context.Context,
	req *connect.Request[sessionv1.ArchiveSessionRequest],
) (*connect.Response[sessionv1.ArchiveSessionResponse], error) {
	if req.Msg.SessionId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required"))
	}
	inst := s.FindLiveInstance(req.Msg.SessionId)
	if inst == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.SessionId))
	}
	now := time.Now()
	// ArchiveWithStop also transitions Status to Stopped (best-effort — archiving
	// previously left ArchivedAt set while Status stayed Active/Paused/Hibernated,
	// which the retention sweep and other Stopped-gated logic depend on being in sync).
	if err := inst.ArchiveWithStop(now); err != nil {
		log.Warn("failed to transition archived session to Stopped", "session", req.Msg.SessionId, "err", err)
	}
	if err := s.storage.SaveInstances([]*session.Instance{inst}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save session: %w", err))
	}
	return connect.NewResponse(&sessionv1.ArchiveSessionResponse{}), nil
}

// +api: session:unarchive
// UnarchiveSession clears archived_at, restoring the session to the default list.
func (s *SessionService) UnarchiveSession(
	ctx context.Context,
	req *connect.Request[sessionv1.UnarchiveSessionRequest],
) (*connect.Response[sessionv1.UnarchiveSessionResponse], error) {
	if req.Msg.SessionId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required"))
	}
	inst := s.FindLiveInstance(req.Msg.SessionId)
	if inst == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.SessionId))
	}
	inst.SetArchivedAt(nil)
	if err := s.storage.SaveInstances([]*session.Instance{inst}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save session: %w", err))
	}
	return connect.NewResponse(&sessionv1.UnarchiveSessionResponse{}), nil
}

// +api: session:archive-workflow-sessions
// ArchiveWorkflowSessions delegates to WorkflowService.
func (s *SessionService) ArchiveWorkflowSessions(
	ctx context.Context,
	req *connect.Request[sessionv1.ArchiveWorkflowSessionsRequest],
) (*connect.Response[sessionv1.ArchiveWorkflowSessionsResponse], error) {
	if s.workflowSvc == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("workflow service not available"))
	}
	return s.workflowSvc.ArchiveWorkflowSessions(ctx, req)
}

// +api: session:delete-workflow-failed-sessions
// DeleteWorkflowFailedSessions delegates to WorkflowService.
func (s *SessionService) DeleteWorkflowFailedSessions(
	ctx context.Context,
	req *connect.Request[sessionv1.DeleteWorkflowFailedSessionsRequest],
) (*connect.Response[sessionv1.DeleteWorkflowFailedSessionsResponse], error) {
	if s.workflowSvc == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("workflow service not available"))
	}
	return s.workflowSvc.DeleteWorkflowFailedSessions(ctx, req)
}

// maybeAutoArchive archives a workflow session that has just stopped.
// Called in the status-update path whenever a session transitions to Stopped.
// Only archives sessions spawned by a workflow (WorkflowID != "").
// If the workflow has archive_after_hours > 0, the retention enforcer handles
// time-delayed archival, so we skip immediate archival here (ADR-4).
func (s *SessionService) maybeAutoArchive(inst *session.Instance) {
	if inst == nil || inst.WorkflowID == "" {
		return
	}
	// Check if this workflow uses delayed archival via the retention enforcer.
	s.workflowMetaMu.RLock()
	meta, ok := s.workflowMetaCache[inst.WorkflowID]
	s.workflowMetaMu.RUnlock()
	if ok && meta.archiveAfterHours > 0 {
		// Retention enforcer will archive this after the configured delay.
		return
	}
	now := time.Now()
	// CAS: set ArchivedAt only if still nil. Prevents double-archive from concurrent EventExited fires.
	if !inst.SetArchivedAtIfNil(now) {
		return
	}
	if err := s.storage.SaveInstances([]*session.Instance{inst}); err != nil {
		log.Warn("[SessionService] failed to auto-archive workflow session",
			"session", inst.Title, "workflow_id", inst.WorkflowID, "err", err)
	}
}

// ReapPausedTmuxSessions kills any tmux session that is still running for a paused
// Instance. This is a safety net for sessions paused before the kill-on-pause change,
// or for cases where the initial kill attempt fell back to detach.
func (s *SessionService) ReapPausedTmuxSessions() {
	if s.reviewQueuePoller == nil {
		return
	}
	instances := s.reviewQueuePoller.GetInstances()
	for _, inst := range instances {
		if !inst.IsPaused() {
			continue
		}
		if err := inst.KillSession(); err != nil {
			log.Warn("[TmuxReaper] failed to kill tmux for paused session",
				"session", inst.Title, "err", err)
		}
	}
}

// GetProviderLimits returns the rate limit and usage details for a session.
func (s *SessionService) GetProviderLimits(
	ctx context.Context,
	req *connect.Request[sessionv1.GetProviderLimitsRequest],
) (*connect.Response[sessionv1.GetProviderLimitsResponse], error) {
	if req.Msg.SessionId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required"))
	}

	inst := s.findInstance(req.Msg.SessionId)
	if inst == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.SessionId))
	}

	provider := "anthropic"
	program := strings.ToLower(inst.Program)
	if strings.Contains(program, "agy") || strings.Contains(program, "antigravity") || strings.Contains(program, "gemini") {
		provider = "google"
	} else if strings.Contains(program, "openai") || strings.Contains(program, "opencode") {
		provider = "openai"
	}

	var limits ProviderLimits
	var found bool
	if s.capacityMonitor != nil {
		limits, found = s.capacityMonitor.GetSessionLimits(inst.Title)
		if !found {
			s.capacityMonitor.mu.RLock()
			globalLimits := s.capacityMonitor.current[provider]
			client, ok := s.capacityMonitor.clients[provider]
			s.capacityMonitor.mu.RUnlock()

			limits = globalLimits
			limits.Provider = provider
			limits.Model = inst.Program
			if ok {
				limits.ContextTokensMax = client.ModelContextWindow(inst.Program)
			}
		}
	} else {
		limits = ProviderLimits{
			Provider:  provider,
			Model:     inst.Program,
			Available: true,
		}
	}

	protoLimits := &sessionv1.ProviderLimitsProto{
		Provider:            limits.Provider,
		Model:               limits.Model,
		RequestsLimit:       int32(limits.RequestsLimit),
		RequestsRemaining:   int32(limits.RequestsRemaining),
		TokensLimit:         int32(limits.TokensLimit),
		TokensRemaining:     int32(limits.TokensRemaining),
		ContextTokensUsed:   int32(limits.ContextTokensUsed),
		ContextTokensMax:    int32(limits.ContextTokensMax),
		SessionInputTokens:  int32(limits.SessionInputTokens),
		SessionOutputTokens: int32(limits.SessionOutputTokens),
		EstimatedCostUsd:    limits.EstimatedCostUSD,
		Available:           limits.Available,
		LastErrorCode:       limits.LastErrorCode,
	}

	if !limits.RequestsReset.IsZero() {
		protoLimits.RequestsReset = timestamppb.New(limits.RequestsReset)
	}
	if !limits.TokensReset.IsZero() {
		protoLimits.TokensReset = timestamppb.New(limits.TokensReset)
	}
	if !limits.FetchedAt.IsZero() {
		protoLimits.FetchedAt = timestamppb.New(limits.FetchedAt)
	}

	return connect.NewResponse(&sessionv1.GetProviderLimitsResponse{
		Limits: protoLimits,
	}), nil
}

// publishSessionUpdatedEvent publishes a SessionUpdated event for instance covering
// updatedFields, using the statusManager-aware NewSessionUpdatedEventWithDetection variant
// when a controller is actively running (so clients see live ClaudeStatus/StatusContext),
// and falling back to the plain NewSessionUpdatedEvent otherwise. Shared by UpdateSession's
// end-of-handler publish and UpdateSessionProgram (the capacity-monitor auto-fallback path)
// so the two program-switch entry points publish identically instead of drifting.
func (s *SessionService) publishSessionUpdatedEvent(instance *session.Instance, updatedFields []string) {
	if s.statusManager != nil {
		statusInfo := s.statusManager.GetStatus(instance)
		if statusInfo.IsControllerActive {
			s.eventBus.Publish(events.NewSessionUpdatedEventWithDetection(
				instance, updatedFields,
				statusInfo.ClaudeStatus, statusInfo.StatusContext,
			))
			return
		}
	}
	s.eventBus.Publish(events.NewSessionUpdatedEvent(instance, updatedFields))
}

// UpdateSessionProgram handles switching programs for a session, doing the history
// porting, DB save, and PTY restart. Shares its implementation with the UpdateSession RPC
// handler via Instance.SwitchProgram (see session/instance_program.go) and
// publishSessionUpdatedEvent above so the two program-switch entry points — this
// auto-fallback path and the manual RPC — can't drift.
func (s *SessionService) UpdateSessionProgram(ctx context.Context, sessionID string, newProgram string) error {
	inst := s.findInstance(sessionID)
	if inst == nil {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	changed, _, err := inst.SwitchProgram(ctx, newProgram, func() error {
		return s.storage.SaveInstances([]*session.Instance{inst})
	})
	if !changed {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to restart session: %w", err)
	}

	s.publishSessionUpdatedEvent(inst, []string{"program"})

	return nil
}

// SetResolveConversationUUID wires the tmux-UUID → Claude-UUID resolver into the search service.
func (s *SessionService) SetResolveConversationUUID(fn func(ctx context.Context, tmuxUUID string) (string, error)) {
	s.searchSvc.SetResolveConversationUUID(fn)
}

// SetTokenStoreReader wires the global parsed token store into the capacity monitor.
func (s *SessionService) SetTokenStoreReader(store tokens.TokenStoreReader) {
	if s.capacityMonitor != nil {
		s.capacityMonitor.tokenStore = store
	}
}

// SetQuotaGate wires the account-wide quota gate so onRateLimitDetected can
// feed it the hard/reactive override signal.
func (s *SessionService) SetQuotaGate(g *QuotaGate) {
	s.quotaGate = g
}

// GetInstances returns all managed (poller-tracked) live instances, satisfying InstancePoller.
func (s *SessionService) GetInstances() []*session.Instance {
	return s.allInstances()
}

// GetConfigFileRules delegates to RulesService.
func (s *SessionService) GetConfigFileRules(
	ctx context.Context,
	req *connect.Request[sessionv1.GetConfigFileRulesRequest],
) (*connect.Response[sessionv1.GetConfigFileRulesResponse], error) {
	return s.rulesSvc.GetConfigFileRules(ctx, req)
}

// SaveRulesToConfigFile delegates to RulesService.
func (s *SessionService) SaveRulesToConfigFile(
	ctx context.Context,
	req *connect.Request[sessionv1.SaveRulesToConfigFileRequest],
) (*connect.Response[sessionv1.SaveRulesToConfigFileResponse], error) {
	return s.rulesSvc.SaveRulesToConfigFile(ctx, req)
}
