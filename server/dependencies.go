package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/tstapler/stapler-squad/config"
	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
	warren "github.com/tstapler/stapler-squad/pkg/warren"
	"github.com/tstapler/stapler-squad/server/analytics"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/workflows"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/artifacts"
	"github.com/tstapler/stapler-squad/session/cdp"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/headless"
	"github.com/tstapler/stapler-squad/session/scrollback"
	"github.com/tstapler/stapler-squad/session/tmux"
	"github.com/tstapler/stapler-squad/session/tokens"
	"github.com/tstapler/stapler-squad/session/unfinished"
	"github.com/tstapler/stapler-squad/session/vnc"
)

// ServerDependencies holds all wired service components for the HTTP server.
// Use BuildDependencies to construct and wire them in the correct order.
// See the initialization order comment on NewServer for dependency constraints.
type ServerDependencies struct {
	SessionService          *services.SessionService
	Storage                 *session.Storage
	Instances               []*session.Instance
	EventBus                *events.EventBus
	StatusManager           *session.InstanceStatusManager
	ReviewQueue             *session.ReviewQueue
	ReviewQueuePoller       *session.ReviewQueuePoller
	PRStatusPoller          *session.PRStatusPoller
	ReactiveQueueMgr        *ReactiveQueueManager
	ScrollbackManager       *scrollback.ScrollbackManager
	TmuxStreamerManager     *session.ExternalTmuxStreamerManager
	ExternalDiscovery       *session.ExternalSessionDiscovery
	ExternalApprovalMonitor *session.ExternalApprovalMonitor
	HistoryLinker           *session.HistoryLinker
	ErrorRegistry           *services.ErrorRegistry

	// Unfinished work scanning.
	UnfinishedScanner     *unfinished.Scanner
	UnfinishedStateStore  *unfinished.StateStore
	UnfinishedWorkService *services.UnfinishedWorkService
	WorktreePRPoller      *session.WorktreePRPoller

	// GitHub user PR cache and service. Nil when no GitHub token is available.
	UserPRCache       *githubpkg.UserPRCache
	GitHubUserService *services.GitHubUserService

	// Token usage analytics.
	InsightsService *services.InsightsService

	BacklogService *services.BacklogService
	// QuotaGate owns the account-wide session-quota pause/resume decision for
	// backlog automation (see BacklogService/BacklogEnabledCheck above).
	QuotaGate *services.QuotaGate
	SyncLoop  *session.SyncLoop

	// BacklogEnabledCheck reports the live runtime state of the "backlog" feature
	// flag. See RuntimeDeps.BacklogEnabledCheck.
	BacklogEnabledCheck func() bool

	// Analytics storage. Nil when the analytics DB failed to open (LogAnalyticsProvider
	// is used as a fallback in that case).
	AnalyticsEntClient *ent.Client

	// VNCDeps holds the result of the startup VNC dependency check.
	// Available=false means the Browser tab will be hidden on all sessions.
	VNCDeps vnc.DepsResult

	// CDPDeps holds the result of the startup CDP (Chrome) dependency check.
	// Available=false means CDP browser streaming is unavailable on this host.
	CDPDeps cdp.DepsResult

	// HeadlessPool manages headless LLM calls. Nil when the claude binary is not found.
	HeadlessPool *headless.Pool

	// WorkflowRepo persists workflow definitions.
	WorkflowRepo session.WorkflowRepository

	// WorkflowScheduler manages cron-based workflow execution.
	WorkflowScheduler *workflows.Scheduler

	// Registry is the live-handle map for all running sessions.
	Registry *session.Registry

	// SessionSummaryGenerator drives async session-completion-summary generation.
	// Nil when storage is not ent-backed. Its NotificationDecisionLister/TokenStore
	// dependencies are wired later via SetNotificationLister/SetTokenStore (see
	// server.go's RunServer) — see the comment on SetNotificationLister for why.
	SessionSummaryGenerator *session.SessionSummaryGenerator
}

// ToServerDeps converts RuntimeDeps to the flat ServerDependencies struct consumed
// by NewServerWithDeps. This mirrors the projection done inside BuildDependencies.
func (rt *RuntimeDeps) ToServerDeps() *ServerDependencies {
	return &ServerDependencies{
		SessionService:          rt.SessionService,
		Storage:                 rt.Storage,
		Instances:               rt.Instances,
		EventBus:                rt.EventBus,
		StatusManager:           rt.StatusManager,
		ReviewQueue:             rt.ReviewQueue,
		ReviewQueuePoller:       rt.ReviewQueuePoller,
		PRStatusPoller:          rt.PRStatusPoller,
		ReactiveQueueMgr:        rt.ReactiveQueueMgr,
		ScrollbackManager:       rt.ScrollbackManager,
		TmuxStreamerManager:     rt.TmuxStreamerManager,
		ExternalDiscovery:       rt.ExternalDiscovery,
		ExternalApprovalMonitor: rt.ExternalApprovalMonitor,
		HistoryLinker:           rt.HistoryLinker,
		ErrorRegistry:           rt.ErrorRegistry,
		UnfinishedScanner:       rt.UnfinishedScanner,
		UnfinishedStateStore:    rt.UnfinishedStateStore,
		UnfinishedWorkService:   rt.UnfinishedWorkService,
		WorktreePRPoller:        rt.WorktreePRPoller,
		UserPRCache:             rt.UserPRCache,
		GitHubUserService:       rt.GitHubUserService,
		InsightsService:         rt.InsightsService,
		BacklogService:          rt.BacklogService,
		QuotaGate:               rt.QuotaGate,
		SyncLoop:                rt.SyncLoop,
		BacklogEnabledCheck:     rt.BacklogEnabledCheck,
		AnalyticsEntClient:      rt.AnalyticsEntClient,
		VNCDeps:                 rt.VNCDeps,
		CDPDeps:                 rt.CDPDeps,
		HeadlessPool:            rt.HeadlessPool,
		WorkflowRepo:            rt.WorkflowRepo,
		WorkflowScheduler:       rt.WorkflowScheduler,
		Registry:                rt.Registry,
		SessionSummaryGenerator: rt.SessionSummaryGenerator,
	}
}

// BuildDependencies constructs and wires all server dependencies in the correct order.
// Returns an error only for unrecoverable failures (SessionService init, Storage start).
// Non-fatal failures (individual instance start) are logged and skipped.
//
// Delegates to the three-phase constructors: BuildCoreDeps -> BuildServiceDeps -> BuildRuntimeDeps.
func BuildDependencies() (*ServerDependencies, error) {
	// Load config early for encryption key support
	cfg := config.LoadConfig()

	// Phase 1 (core): SessionService, Storage, EventBus, ReviewQueue, ApprovalStore
	// was: step 1 - SessionService + getter calls
	core, err := BuildCoreDeps()
	if err != nil {
		return nil, fmt.Errorf("phase 1 (core): %w", err)
	}

	// Phase 2 (services): StatusManager, ReviewQueuePoller, wiring into SessionService
	// was: steps 2-3 - StatusManager, ReviewQueuePoller, SetApprovalProvider, SetStatusManager, SetReviewQueuePoller
	svc, err := BuildServiceDeps(core)
	if err != nil {
		return nil, fmt.Errorf("phase 2 (services): %w", err)
	}

	// Phase 3 (runtime): ensure tmux server running, then load instances.
	// EnsureServerRunning must precede BuildRuntimeDeps — the token enforces it.
	tmuxReady, err := tmux.EnsureServerRunning("")
	if err != nil {
		log.Warn("BuildDependencies: failed to ensure tmux server running", "err", err)
	}
	rt, err := BuildRuntimeDeps(tmuxReady, svc, cfg)
	if err != nil {
		return nil, fmt.Errorf("phase 3 (runtime): %w", err)
	}

	return rt.ToServerDeps(), nil
}

// syncOrphanedApprovalsToQueue adds review queue items for orphaned (persisted) approvals.
// This ensures sessions with known pending approvals appear in the queue immediately on startup,
// even before the first poll cycle detects them via terminal content scanning.
func syncOrphanedApprovalsToQueue(
	store *services.ApprovalStore,
	instances []*session.Instance,
	queue session.ReviewQueueWriter,
) {
	if store == nil {
		return
	}

	orphaned := store.ListAll()
	if len(orphaned) == 0 {
		return
	}

	// Build a lookup map for instances by title
	instMap := make(map[string]*session.Instance, len(instances))
	for _, inst := range instances {
		instMap[inst.Title] = inst
	}

	added := 0
	for _, approval := range orphaned {
		if !approval.Orphaned {
			continue
		}

		// Build context from approval metadata
		context := fmt.Sprintf("Permission required: %s", approval.ToolName)
		if cmd, ok := approval.ToolInput["command"].(string); ok && cmd != "" {
			if len(cmd) > 120 {
				context = cmd[:120] + "..."
			} else {
				context = cmd
			}
		}

		item := &session.ReviewItem{
			SessionID:   approval.SessionID,
			SessionName: approval.SessionID,
			Reason:      session.ReasonApprovalPending,
			Priority:    session.PriorityHigh,
			DetectedAt:  approval.CreatedAt,
			Context:     context,
			Metadata: map[string]string{
				"pending_approval_id": approval.ID,
				"tool_name":           approval.ToolName,
				"orphaned":            "true",
			},
			LastActivity: approval.CreatedAt,
		}

		// Enrich with instance data if available
		if inst, ok := instMap[approval.SessionID]; ok {
			item.Program = inst.Program
			item.Branch = inst.Branch
			item.Path = inst.Path
			item.WorkingDir = inst.WorkingDir
			item.Status = inst.Status.String()
			item.Tags = inst.Tags
			item.Category = inst.Category
			item.DiffStats = inst.GetDiffStats()
			if !inst.LastMeaningfulOutput.IsZero() {
				item.LastActivity = inst.LastMeaningfulOutput
			}
		}

		queue.Add(item)
		added++
		log.Info("[ApprovalSync] added orphaned approval to review queue", "session", approval.SessionID, "tool", approval.ToolName, "approval_id", approval.ID)
	}

	if added > 0 {
		log.Info("[ApprovalSync] synced orphaned approvals", "count", added)
	}
}

// ---------------------------------------------------------------------------
// Phased dependency structs (Dependency Initialization Hardening)
//
// These types decompose BuildDependencies into three ordered phases:
//   Phase 1 (CoreDeps)    - foundational components with no external prerequisites
//   Phase 2 (ServiceDeps) - management components that depend on CoreDeps
//   Phase 3 (RuntimeDeps) - runtime components involving processes and I/O
//
// BuildDependencies delegates to BuildCoreDeps -> BuildServiceDeps -> BuildRuntimeDeps.
// ---------------------------------------------------------------------------

// CoreDeps holds the foundational dependencies created during Phase 1.
// These have no external prerequisites and form the base for all other components.
type CoreDeps struct {
	SessionService *services.SessionService
	Storage        *session.Storage
	EventBus       *events.EventBus
	ReviewQueue    *session.ReviewQueue
	ApprovalStore  *services.ApprovalStore
	ErrorRegistry  *services.ErrorRegistry
}

// BuildOptions carries optional overrides for BuildCoreDepsWithOptions.
// The zero value uses all defaults (equivalent to calling BuildCoreDeps).
type BuildOptions struct {
	// EntClient supplies a pre-opened *ent.Client, bypassing config-based DB path
	// discovery and schema migration. nil = open from config as usual.
	EntClient *ent.Client
}

// BuildCoreDepsWithOptions constructs Phase 1 dependencies with optional overrides.
// Use BuildOptions to inject a pre-built EntClient (for tests).
func BuildCoreDepsWithOptions(opts BuildOptions) (*CoreDeps, error) {
	var sessionService *services.SessionService
	var err error
	if opts.EntClient != nil {
		sessionService, err = services.NewSessionServiceWithEntClient(opts.EntClient)
	} else {
		sessionService, err = services.NewSessionServiceFromConfig()
	}
	if err != nil {
		return nil, fmt.Errorf("initialize SessionService: %w", err)
	}

	storage := sessionService.GetStorage()

	// Wire the ErrorRegistry using the existing ent client from Storage.
	// GetEntClient returns nil when storage is not ent-backed (e.g. in tests),
	// in which case ErrorRegistry gracefully disables itself.
	errorRegistry := services.NewErrorRegistry(storage.GetEntClient(), true)

	w := warren.NewWire("CoreDeps")
	warren.Set(w, "ErrorRegistry", sessionService.SetErrorRegistry, errorRegistry)
	if err := w.Validate(); err != nil {
		return nil, err
	}

	return &CoreDeps{
		SessionService: sessionService,
		Storage:        storage,
		EventBus:       sessionService.GetEventBus(),
		ReviewQueue:    sessionService.GetReviewQueueInstance(),
		ApprovalStore:  sessionService.GetApprovalStore(),
		ErrorRegistry:  errorRegistry,
	}, nil
}

// BuildCoreDeps constructs Phase 1 dependencies using config defaults.
// It is a thin wrapper around BuildCoreDepsWithOptions(BuildOptions{}).
func BuildCoreDeps() (*CoreDeps, error) {
	return BuildCoreDepsWithOptions(BuildOptions{})
}

// ServiceDeps holds Phase 2 dependencies: management components that depend on CoreDeps.
type ServiceDeps struct {
	*CoreDeps
	StatusManager     *session.InstanceStatusManager
	ReviewQueuePoller *session.ReviewQueuePoller
	PRStatusPoller    *session.PRStatusPoller
	Registry          *session.Registry
}

// BuildServiceDeps constructs Phase 2 dependencies using Phase 1 outputs.
// Compile-time guarantee: cannot be called without a *CoreDeps.
func BuildServiceDeps(core *CoreDeps) (*ServiceDeps, error) {
	if core == nil {
		return nil, fmt.Errorf("BuildServiceDeps: CoreDeps is nil (Phase 1 not completed)")
	}
	if core.Storage == nil || core.EventBus == nil || core.ReviewQueue == nil ||
		core.SessionService == nil || core.ApprovalStore == nil {
		return nil, fmt.Errorf("BuildServiceDeps: CoreDeps has nil fields")
	}

	statusManager := session.NewInstanceStatusManager()
	reviewQueuePoller := session.NewReviewQueuePoller(
		core.ReviewQueue, statusManager, core.Storage,
	)
	prStatusPoller := session.NewPRStatusPoller(core.Storage)

	registry := session.NewRegistry(core.Storage, core.SessionService.WireInstanceCallbacks)
	core.SessionService.SetRegistry(registry)

	w := warren.NewWire("ServiceDeps")
	warren.Set(w, "ApprovalProvider", reviewQueuePoller.SetApprovalProvider, session.ApprovalMetadataProvider(core.ApprovalStore))
	warren.Set(w, "StatusManager", core.SessionService.SetStatusManager, statusManager)
	warren.Set(w, "ReviewQueuePoller", core.SessionService.SetReviewQueuePoller, reviewQueuePoller)
	if err := w.Validate(); err != nil {
		return nil, err
	}

	return &ServiceDeps{
		CoreDeps:          core,
		StatusManager:     statusManager,
		ReviewQueuePoller: reviewQueuePoller,
		PRStatusPoller:    prStatusPoller,
		Registry:          registry,
	}, nil
}

// RuntimeDeps holds Phase 3 dependencies: runtime components that involve
// process creation, filesystem I/O, and callback wiring.
type RuntimeDeps struct {
	*ServiceDeps
	Instances               []*session.Instance
	ReactiveQueueMgr        *ReactiveQueueManager
	ScrollbackManager       *scrollback.ScrollbackManager
	TmuxStreamerManager     *session.ExternalTmuxStreamerManager
	ExternalDiscovery       *session.ExternalSessionDiscovery
	ExternalApprovalMonitor *session.ExternalApprovalMonitor
	PRStatusPoller          *session.PRStatusPoller
	HistoryLinker           *session.HistoryLinker
	ErrorRegistry           *services.ErrorRegistry

	// Unfinished work scanning.
	UnfinishedScanner     *unfinished.Scanner
	UnfinishedStateStore  *unfinished.StateStore
	UnfinishedWorkService *services.UnfinishedWorkService
	WorktreePRPoller      *session.WorktreePRPoller

	// GitHub user PR cache and service.
	UserPRCache       *githubpkg.UserPRCache
	GitHubUserService *services.GitHubUserService

	// Token usage analytics.
	InsightsService *services.InsightsService

	BacklogService *services.BacklogService
	// QuotaGate owns the account-wide session-quota pause/resume decision for
	// backlog automation (see BacklogService/BacklogEnabledCheck above).
	QuotaGate *services.QuotaGate
	SyncLoop  *session.SyncLoop
	Config    *config.Config // Used for encryption of sensitive data

	// BacklogEnabledCheck reports the live runtime state of the "backlog" feature
	// flag (backlogCtrl.IsEnabled). Threaded into the MCP server so backlog/goal
	// tool calls are gated by the same source of truth as the ConnectRPC interceptor.
	BacklogEnabledCheck func() bool

	// Analytics storage.
	AnalyticsEntClient *ent.Client

	// VNCDeps holds the result of the startup VNC dependency check.
	VNCDeps vnc.DepsResult

	// CDPDeps holds the result of the startup CDP (Chrome) dependency check.
	CDPDeps cdp.DepsResult

	// HeadlessPool manages headless LLM calling. Nil when claude binary is not found.
	HeadlessPool *headless.Pool

	// WorkflowRepo persists workflow definitions.
	WorkflowRepo session.WorkflowRepository

	// WorkflowScheduler manages cron-based workflow execution.
	WorkflowScheduler *workflows.Scheduler

	// Registry is the live-handle map for all running sessions.
	Registry *session.Registry

	// SessionSummaryGenerator drives async session-completion-summary generation.
	// Nil when storage is not ent-backed.
	SessionSummaryGenerator *session.SessionSummaryGenerator
}

// reviewQueueLookupAdapter adapts session.Storage's ItemSession/ReviewVerdict
// queries to session.ReviewQueueLookup, which BuildDecisionsSnapshot
// (session/session_summary_snapshot.go) needs. A session's resolved/still-open
// review counts are derived from every ItemSession attached to the same backlog
// item as this session's own ItemSession (siblings included) — the backlog item,
// not just this one session, is the natural "review queue" scope, mirroring how
// review_queue_manager.go's itemSessionLookupTimeout bounds the identical
// ItemSession/ReviewVerdict storage lookup.
type reviewQueueLookupAdapter struct {
	storage *session.Storage
}

// ReviewQueueResolvedCount implements session.ReviewQueueLookup.
func (a *reviewQueueLookupAdapter) ReviewQueueResolvedCount(ctx context.Context, sessionID string) (resolved, stillOpen int, err error) {
	itemSession, err := a.storage.GetItemSessionBySessionUUID(ctx, sessionID)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return 0, 0, nil // no linked backlog item (FR-6's first-class empty case)
		}
		return 0, 0, err
	}
	sessions, err := a.storage.ListItemSessions(ctx, itemSession.BacklogItemID)
	if err != nil {
		return 0, 0, err
	}
	for _, is := range sessions {
		if is.OverallOutcome != "" {
			resolved++
		} else {
			stillOpen++
		}
	}
	return resolved, stillOpen, nil
}

// BuildRuntimeDeps constructs Phase 3 dependencies using Phase 2 outputs.
// This implements steps 5-12 from the original BuildDependencies:
//   - Step 5: LoadInstances + wire ReviewQueue/StatusManager on each instance
//   - Step 6: Start tmux sessions for loaded instances (non-fatal failures)
//   - Step 6.5: Persist auto-detected worktree info
//   - Step 7: Start controllers for running instances
//   - Step 7.5: Startup scan + orphaned approval sync
//   - Step 8: ReactiveQueueManager + wire into SessionService
//   - Step 9: ScrollbackManager (independent)
//   - Step 10: TmuxStreamerManager (independent)
//   - Step 11: ExternalDiscovery with session-added/removed callbacks
//   - Step 12: ExternalApprovalMonitor with approval-to-review-queue bridge
//   - SetExternalDiscovery on SessionService (moved from server.go)
//
// BuildRuntimeDeps requires a TmuxServerReady token to enforce that
// tmux.EnsureServerRunning was called before sessions are loaded. Without this
// ordering, DoesSessionExist() may trigger recoverFromServerFailure, which starts
// a fresh server that considers all sessions non-existent and cold-restores them.
// cfg may be nil; when non-nil, is used for token encryption in backlog sources.

// recoverAndLog runs fn with its own panic recovery, logging under label if it
// panics. Used by the 60s reconcile ticker so a panic in one tick's work
// (e.g. QuotaGate.Reconcile) never prevents a sibling call in the same tick,
// or any future tick, from running.
func recoverAndLog(label string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			log.Error(label+" recovered from panic", "recover", r)
		}
	}()
	fn()
}

func BuildRuntimeDeps(_ tmux.TmuxServerReady, svc *ServiceDeps, cfg *config.Config) (*RuntimeDeps, error) {
	if svc == nil {
		return nil, fmt.Errorf("BuildRuntimeDeps: ServiceDeps is nil (Phase 2 not completed)")
	}

	// Alias embedded fields for readability (matches original BuildDependencies local vars).
	storage := svc.Storage
	reviewQueue := svc.ReviewQueue
	statusManager := svc.StatusManager
	reviewQueuePoller := svc.ReviewQueuePoller
	eventBus := svc.EventBus
	sessionService := svc.SessionService

	// Step 5: load instances from storage
	instances, err := storage.LoadInstances()
	if err != nil {
		return nil, fmt.Errorf("load instances: %w", err)
	}

	// WorkflowEngine governs backlog state transitions; constructed once and shared
	// by the service layer.
	workflowEngine := session.NewDefaultWorkflowEngine()

	// PipelineEngine resolves a backlog item's slash-command set / prompts (Epic 1.5).
	// Constructed here — before NewBacklogLifecycleListenerWithPool and
	// services.NewBacklogService below — so both share the exact same
	// *session.CachingPipelineEngine instance (Story 1.5.1's pointer-equality
	// requirement; see server/dependencies_test.go). storage.GetEntClient() is
	// already available this early (aliased above, and used successfully even
	// earlier by BuildCoreDepsWithOptions for NewErrorRegistry), so there is no
	// bootstrap-ordering obstacle to constructing it now rather than deferring via
	// a Set*-style setter. Degrades gracefully — never aborts boot — if the ent
	// client is unavailable (non-ent-backed storage, e.g. some test configurations)
	// or construction otherwise fails: pipelineEngine stays a true nil interface
	// (not a typed-nil *CachingPipelineEngine, which would break every
	// `s.pipelineEngine != nil` guard downstream) and every call site falls back to
	// the built-in default pipeline for all items.
	// pipelineModeRepo is lifted to this outer scope (rather than staying local to
	// the entClient-available branch below) because services.NewBacklogService
	// (Epic 2.2) needs the same repository instance to back its PipelineMode CRUD
	// RPCs, independent of whether pipelineEngine construction itself succeeded.
	var pipelineEngine session.PipelineEngine
	var pipelineModeRepo session.PipelineModeRepository
	if entClient := storage.GetEntClient(); entClient != nil {
		pipelineModeRepo = session.NewEntPipelineModeRepository(entClient)
		// Seed the "sdd" pipeline mode before the engine's first cache Load
		// below, so that Load already sees the seeded row in one pass rather
		// than needing a follow-up InvalidateCache. Create-if-missing only —
		// never overwrites an operator's later hand-edit — and never aborts
		// boot on failure, matching NewPipelineEngine's own non-fatal
		// posture immediately below (see
		// project_plans/backlog-sdd-default-pipeline/implementation/plan.md
		// Task 1.1.1c).
		if seedErr := session.EnsureDefaultSDDPipelineMode(context.Background(), pipelineModeRepo); seedErr != nil {
			log.Warn("failed to seed default sdd pipeline mode, continuing without it", "err", seedErr)
		}
		if cachingPipelineEngine, err := session.NewPipelineEngine(pipelineModeRepo); err != nil {
			log.Warn("pipelineEngine construction failed; continuing with the default pipeline for all backlog items", "err", err)
		} else {
			pipelineEngine = cachingPipelineEngine
		}
	} else {
		log.Warn("pipelineEngine unavailable: storage is not ent-backed; continuing with the default pipeline for all backlog items")
	}

	// Construct the headless LLM pool early so the lifecycle listener can receive it
	// via constructor (eliminating the post-construction wiring race). Non-fatal if
	// the claude binary is not found.
	var headlessPool *headless.Pool
	{
		p, poolErr := headless.NewPool(headless.PoolConfig{
			MaxCallsPerSession:    25,
			MaxConcurrentSessions: 5,
		})
		if poolErr != nil {
			log.Warn("headless pool disabled: claude binary not found", "err", poolErr)
		} else {
			headlessPool = p
			headless.SetDefaultPool(p)
			sessionService.SetHeadlessPool(p)
			log.Info("headless LLM pool initialized")
		}
	}

	// SessionSummaryGenerator — constructed here (not deferred to server.go) so it
	// can be wired to every instance in the loop just below, mirroring
	// backlogLifecycleListener's setup. Its NotificationDecisionLister/TokenStore
	// dependencies aren't ready yet at this point in startup (NotificationHistoryStore
	// is built later, in server.go's RunServer; the token store further below in this
	// function) — wired in later via SetNotificationLister/SetTokenStore, the same
	// "Set* called long after construction" ordering constraint SetHeadlessPool used
	// to have (see the comment above backlogLifecycleListener). Nil when storage isn't
	// ent-backed.
	var sessionSummaryGenerator *session.SessionSummaryGenerator
	if entClient := storage.GetEntClient(); entClient != nil {
		sessionSummaryGenerator = session.NewSessionSummaryGenerator(
			entClient,
			headlessPool,
			nil, // NotificationDecisionLister — wired later via SetNotificationLister
			nil, // TokenStoreReader — wired later via SetTokenStore
			&reviewQueueLookupAdapter{storage: storage},
		)
		sessionService.SetSessionSummaryGenerator(sessionSummaryGenerator)
	} else {
		log.Warn("session summary generation unavailable: storage is not ent-backed")
	}

	// Backlog lifecycle listener — always created, enabled state set from config below.
	// The pool is passed at construction time to close the race window that existed when
	// SetHeadlessPool was called hundreds of lines after instance wiring.
	backlogLifecycleListener := session.NewBacklogLifecycleListenerWithPool(storage, headlessPool, pipelineEngine)
	backlogLifecycleListener.SetNotifier(&services.EventBusNotifier{Bus: eventBus})
	// Wires the ItemChangePublisher adapter into the concrete *EntRepository
	// (via Storage's forwarding setter, session/storage.go) so its 9 hooked
	// backlog mutation methods (Phase 2) can publish BacklogItemChanged
	// events. This is a different struct than backlogLifecycleListener.SetNotifier
	// above — placed here for readability only, not because it mirrors that call.
	storage.SetItemChangePublisher(&services.BacklogItemEventPublisher{Bus: eventBus})
	// Review now always spawns a real, hidden session.Instance (via
	// SessionService.SpawnReviewSession) instead of an in-process headless LLM
	// call, so review-queue visibility (idle/error/approval detection) works the
	// same as for every other automated session. sessionService already
	// satisfies session.ReviewGateSpawner.
	backlogLifecycleListener.SetSessionCreator(sessionService)

	// Step 5 (continued): wire dependencies to each instance
	// inst.SetReviewQueue and inst.SetStatusManager are called per-instance in a loop;
	// Warren is designed for named scalar setters, not loop iterations. Left unwrapped.
	for _, inst := range instances {
		inst.SetReviewQueue(reviewQueue)
		inst.SetStatusManager(statusManager)
		backlogLifecycleListener.WireToInstance(inst)
		if sessionSummaryGenerator != nil {
			session.WireSessionSummaryListener(sessionSummaryGenerator, inst)
		}
	}

	// Restore dirBaseSHA for directory-mode backlog sessions that were persisted
	// before this process started. One batch query replaces N individual lookups.
	{
		var dirUUIDs []string
		dirInstMap := make(map[string]*session.Instance)
		for _, inst := range instances {
			if !inst.IsWorktree && inst.HasTag(session.TagBacklogWork) {
				dirUUIDs = append(dirUUIDs, inst.UUID)
				dirInstMap[inst.UUID] = inst
			}
		}
		if len(dirUUIDs) > 0 {
			baseSHAs, _ := storage.GetBaseCommitSHAsForSessions(context.Background(), dirUUIDs)
			for uuid, sha := range baseSHAs {
				if inst, ok := dirInstMap[uuid]; ok {
					inst.SetDirBaseSHA(sha)
				}
			}
		}
	}

	// Wire instances to pollers.
	// SetInstances accepts a slice (non-comparable) so use SetAlways (skips nil check).
	w2 := warren.NewWire("RuntimeDeps.Pollers")
	warren.SetAlways(w2, "ReviewQueuePoller.Instances", reviewQueuePoller.SetInstances, instances)
	warren.SetAlways(w2, "PRStatusPoller.Instances", svc.PRStatusPoller.SetInstances, instances)
	warren.SetAlways(w2, "PRStatusPoller.OnUpdated", svc.PRStatusPoller.SetOnUpdated, func(inst *session.Instance) {
		eventBus.Publish(events.NewSessionUpdatedEvent(inst, []string{"github_pr_priority", "github_pr_state", "github_check_conclusion"}))
	})
	if err := w2.Validate(); err != nil {
		return nil, err
	}

	// Perform heavy initialization (tmux starting, controllers, scanning) in the background
	// so the HTTP server can bind and start immediately.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.ErrorLog.Printf("[startup] panic in background init goroutine: %v", r)
			}
		}()
		// Step 6: start tmux sessions for loaded instances (non-fatal failures).
		// Stagger starts by 200ms each to avoid a fork burst that saturates the
		// cgroup pids.max limit when many sessions restore simultaneously.
		for i, inst := range instances {
			if !inst.Started() {
				if i > 0 {
					time.Sleep(200 * time.Millisecond)
				}
				if err := inst.Start(false); err != nil {
					log.Error("failed to start loaded instance", "session", inst.Title, "err", err)
				} else {
					log.Info("started loaded instance", "session", inst.Title)
				}
			}
		}

		// Step 6b: reconcile Stopped sessions that have a live tmux session.
		// This handles the case where the server crashed or restarted while a session
		// was running — the DB recorded Stopped but the tmux session survived.
		// RecoverFromStopped resets the status to Ready (bypassing the terminal-state
		// guard) so Start(false) can hot-attach to the existing tmux session.
		for _, inst := range instances {
			if inst.Status == session.Stopped && inst.TmuxSessionExists() {
				log.Info("Reconcile: session is Stopped in DB but tmux is alive — restoring", "session", inst.Title)
				inst.RecoverFromStopped()
				if err := inst.Start(false); err != nil {
					log.Warn("Reconcile: hot-restore failed", "session", inst.Title, "err", err)
				} else {
					log.Info("Reconcile: restored session (was Stopped, now Running)", "session", inst.Title)
				}
			}
		}

		// Step 6c: Reconcile custom shells — rebuild in-memory state from ent for
		// shells that were running when the server last shut down. Must run after
		// Step 6/6b so tmux sessions are attached before shell liveness probes fire.
		for _, inst := range instances {
			inst.ReconcileShells(context.Background())
		}

		// Step 6d: Kill orphaned tmux sessions — staplersquad_ sessions with no
		// matching DB record. These accumulate when DeleteSession removes the DB
		// row but the server restarted before the live in-memory instance was
		// available to call Destroy(). Must run after 6/6b so re-adopted sessions
		// are already registered and won't be mistaken for orphans.
		//
		// SKIP for any isolated-instance process: ReconcileOrphanedTmuxSessions calls plain
		// `tmux list-sessions` with no socket isolation -- it always targets the shared
		// default tmux socket, regardless of this process's own (isolated) DB/config
		// directory. This process's `instances` list only ever contains its own handful of
		// sessions, so every real session on the machine's shared tmux server -- including
		// production sessions from an entirely separate stapler-squad process -- looks like
		// an orphan and gets killed. This was the root cause of production sessions dying in
		// tight clusters whenever any integration test called BuildDependencies() on the same
		// machine, the E2E test harness's real-binary invocation (STAPLER_SQUAD_INSTANCE=
		// e2e-local, not caught by IsTestMode()), and the demo-server harness's --test-mode/
		// STAPLER_SQUAD_TEST_DIR invocation (also not caught by either IsTestMode() or
		// IsNamedInstance() alone). See config.IsIsolatedInstance's doc comment.
		if !config.IsIsolatedInstance() {
			session.ReconcileOrphanedTmuxSessions(instances)
		}

		// Step 6.5: Persist any auto-detected worktree info (must happen after Step 6)
		if len(instances) > 0 {
			if err := storage.SaveInstances(instances); err != nil {
				log.Warn("failed to persist migrated instance data", "err", err)
			} else {
				log.Info("persisted migrated instance data", "count", len(instances))
			}
		}

		// Step 6.6: Reconcile CDP orphan wrapper directories.
		// Collect active session IDs, then remove any cdp-bins subdirectory that
		// does not belong to a known session (left behind by crashed or deleted sessions).
		// Both the real manager and the noop manager implement ReconcileOrphans with
		// filesystem-only cleanup (no Chrome binary required).
		activeSessionIDs := make([]string, 0, len(instances))
		for _, inst := range instances {
			activeSessionIDs = append(activeSessionIDs, inst.GetStableID())
		}
		cdpCleanupMgr := cdp.New(cdp.CDPConfig{}) // noop when Chrome is absent; still cleans up dirs
		if err := cdpCleanupMgr.ReconcileOrphans(activeSessionIDs); err != nil {
			log.Warn("cdp: orphan cleanup failed (non-fatal)", "err", err)
		}

		// Step 7: start controllers (requires started instances + StatusManager)
		log.Info("attempting controller startup", "instances", len(instances))
		for _, inst := range instances {
			started := inst.Started()
			paused := inst.Paused()
			if started && !paused && inst.Status != session.Stopped {
				if inst.GetController() == nil {
					if err := inst.StartController(); err != nil {
						log.Warn("failed to start controller", "session", inst.Title, "err", err)
					} else {
						log.Info("started controller", "session", inst.Title)
					}
				}
			}
		}

		// Step 7.5: Resume session drivers for workflow sessions with an undelivered InitialPrompt.
		// After a service restart, drivers are not automatically restarted for loaded sessions.
		// Sessions created by the workflow scheduler that never had their prompt injected
		// (e.g., service restarted within 30 s of session creation) need the driver resumed.
		// The driver itself checks for an existing JSONL conversation file and skips the send
		// if the prompt was already delivered in a previous run.
		for _, inst := range instances {
			if inst.InitialPrompt == "" {
				continue
			}
			if inst.Status == session.Paused || inst.Status == session.Stopped || inst.Status == session.Hibernated {
				continue
			}
			session.StartSessionDriver(inst, inst.GetEffectiveRootDir())
		}

		// Step 7.6: Startup scan and orphaned approval sync
		// Brief settling delay to allow controllers to initialize their terminal readers.
		time.Sleep(500 * time.Millisecond)
		contentProvider := session.NewPollerContentProvider()
		scanner := session.NewStartupScanner(statusManager, contentProvider)
		scanner.Scan(instances, reviewQueue)
		syncOrphanedApprovalsToQueue(svc.ApprovalStore, instances, reviewQueue)
	}()

	// Step 8: ReactiveQueueManager
	reactiveQueueMgr := NewReactiveQueueManager(reviewQueue, reviewQueuePoller, eventBus, statusManager, storage)
	// Wires the opt-in AutoCreatePR policy — sessionService is available this early
	// (constructed in BuildCoreDepsWithOptions, aliased above), so no setter-injection
	// race window like SetHeadlessPool's had.
	reactiveQueueMgr.SetOneShotRunner(sessionService)
	log.Info("ReactiveQueueManager initialized")

	// Step 8.5: HistoryLinker — detects Claude JSONL files and links conversation
	// UUIDs to sessions so cold restore can use --resume on restart.
	historyLinker := session.NewHistoryLinkerFromRealInspector()
	log.Info("HistoryLinker initialized", "instances", len(instances))

	// Step 9: ScrollbackManager (independent of above)
	homeDir, _ := os.UserHomeDir()
	scrollbackPath := filepath.Join(homeDir, ".stapler-squad", "sessions")
	scrollbackConfig := scrollback.DefaultScrollbackConfig()
	scrollbackConfig.StoragePath = scrollbackPath
	scrollbackManager := scrollback.NewScrollbackManager(scrollbackConfig)
	log.Info("initialized ScrollbackManager", "path", scrollbackPath, "compression", scrollbackConfig.StoragePath, "maxLines", scrollbackConfig.MaxLines)

	// Step 10: TmuxStreamerManager (independent)
	tmuxStreamerManager := session.NewExternalTmuxStreamerManager()

	// Step 11: ExternalDiscovery with session-added/removed callbacks
	externalDiscovery := session.NewExternalSessionDiscovery()
	externalDiscovery.OnSessionAdded(func(instance *session.Instance) {
		if err := storage.AddInstance(instance); err != nil {
			log.Error("failed to persist external session", "session", instance.Title, "err", err)
		} else {
			log.Info("persisted external session to storage", "session", instance.Title)
		}
		// Wire dependencies so the external session appears in the review queue
		instance.SetReviewQueue(reviewQueue)
		instance.SetStatusManager(statusManager)
		reviewQueuePoller.AddInstance(instance)
		svc.PRStatusPoller.AddInstance(instance)
		historyLinker.AddInstance(instance)
		backlogLifecycleListener.WireToInstance(instance)
		if sessionSummaryGenerator != nil {
			session.WireSessionSummaryListener(sessionSummaryGenerator, instance)
		}
		log.Info("added external session to review queue poller, PR status poller, and history linker", "session", instance.Title)
	})
	externalDiscovery.OnSessionRemoved(func(instance *session.Instance) {
		reviewQueuePoller.RemoveInstance(instance.Title)
		svc.PRStatusPoller.RemoveInstance(instance.Title)
		historyLinker.RemoveInstance(instance.Title)
		log.Info("removed external session from review queue poller, PR status poller, and history linker", "session", instance.Title)
		reviewQueue.Remove(instance.Title)
		if err := storage.DeleteInstance(instance.Title); err != nil {
			log.Warn("failed to remove external session from storage", "session", instance.Title, "err", err)
		} else {
			log.Info("removed external session from storage", "session", instance.Title)
		}
	})

	// Step 12: ExternalApprovalMonitor — wire approval-to-review-queue bridge
	externalApprovalMonitor := session.NewExternalApprovalMonitor()
	externalApprovalMonitor.OnApproval(func(event *session.ExternalApprovalEvent) {
		if event == nil || event.Request == nil {
			return
		}
		// Resolve the instance (try tmux session name first, socket path as fallback)
		inst := externalDiscovery.GetSessionByTmux(event.SessionID)
		if inst == nil {
			inst = externalDiscovery.GetSession(event.SessionID)
		}

		context := event.Request.DetectedText
		if context == "" {
			context = "Permission request detected"
		}

		item := &session.ReviewItem{
			SessionID:   event.SessionTitle,
			SessionName: event.SessionTitle,
			Reason:      session.ReasonApprovalPending,
			Priority:    session.PriorityHigh,
			DetectedAt:  event.Request.Timestamp,
			Context:     context,
		}
		if inst != nil {
			item.Program = inst.Program
			item.Branch = inst.Branch
			item.Path = inst.Path
			item.WorkingDir = inst.WorkingDir
			item.Status = inst.Status.String()
			item.Tags = inst.Tags
			item.Category = inst.Category
			item.DiffStats = inst.GetDiffStats()
			item.LastActivity = inst.LastMeaningfulOutput
		}

		reviewQueue.Add(item)
		log.Info("added external session approval to review queue", "session", event.SessionTitle, "type", event.Request.Type, "confidence", event.Request.Confidence)
	})

	// Wire external discovery to SessionService for unified session listing
	// (moved from server.go to keep all dependency wiring in BuildRuntimeDeps)

	w3 := warren.NewWire("RuntimeDeps.SessionService")
	// ReactiveQueueManager is an exported interface; cast to infer correct type param.
	warren.Set(w3, "ReactiveQueueManager", sessionService.SetReactiveQueueManager, services.ReactiveQueueManager(reactiveQueueMgr))
	warren.Set(w3, "HistoryLinker", sessionService.SetHistoryLinker, historyLinker)
	// SetInstances accepts a slice (non-comparable) so use SetAlways.
	warren.SetAlways(w3, "HistoryLinker.Instances", historyLinker.SetInstances, instances)
	warren.Set(w3, "ScrollbackManager", sessionService.SetScrollbackManager, services.ScrollbackSequencer(scrollbackManager))
	warren.Set(w3, "ExternalDiscovery", sessionService.SetExternalDiscovery, externalDiscovery)
	// UnfinishedWorkService is optional — nil when config directory is unavailable.
	// Do not add to Warren Wire; nil is a valid production value documented on RuntimeDeps.
	if err := w3.Validate(); err != nil {
		return nil, err
	}

	// Initialize UnfinishedWork scanner and state store.
	var (
		unfinishedScanner    *unfinished.Scanner
		unfinishedStateStore *unfinished.StateStore
		unfinishedWorkSvc    *services.UnfinishedWorkService
		worktreePRPoller     *session.WorktreePRPoller
		userPRCache          *githubpkg.UserPRCache
	)
	if configDir, configErr := config.GetConfigDir(); configErr == nil {
		statePath := filepath.Join(configDir, "unfinished_state.json")
		unfinishedStateStore, _ = unfinished.NewStateStore(statePath)
		if unfinishedStateStore != nil {
			unfinishedScanner = unfinished.NewScanner(eventBus, unfinishedStateStore)
			unfinishedWorkSvc = services.NewUnfinishedWorkService(unfinishedScanner, unfinishedStateStore, eventBus, storage)
			log.Info("UnfinishedWorkService initialized", "state", statePath)
			if err := unfinished.RegisterMetrics(); err != nil {
				log.Warn("failed to register unfinished OTel metrics", "err", err)
			}

			// WorktreePRPoller enriches worktrees-without-sessions with GitHub PR data.
			// The scannerSource adapter bridges session/unfinished → session without a cycle.
			worktreePRPoller = session.NewWorktreePRPoller(
				githubpkg.NewETagCache(),
				svc.PRStatusPoller,
			)
			worktreePRPoller.SetSource(&scannerSource{s: unfinishedScanner})
			worktreePRPoller.SetOnUpdated(func(repoPath, branch string, info *githubpkg.PRInfo) {
				log.Info("worktree PR updated", "repo", repoPath, "branch", branch, "pr", info.Number)
			})
		}
	} else {
		log.Warn("could not initialize UnfinishedWork state store", "err", configErr)
	}

	// UserPRCache fetches all open PRs authored by the authenticated GitHub user.
	userPRCache = githubpkg.NewUserPRCache()
	userPRCache.SetOnUpdated(func(prs []githubpkg.UserPR) {
		annotateUserPRCache(userPRCache, svc.PRStatusPoller, unfinishedScanner)
	})
	githubUserSvc := services.NewGitHubUserService(userPRCache, cfg.GetGitHubEnterpriseHosts())
	sessionService.SetUserPRCache(userPRCache)

	// Open the dedicated analytics database (non-fatal: fall back gracefully on failure).
	var analyticsClient *ent.Client
	if configDir, configErr := config.GetConfigDir(); configErr == nil {
		ctx := context.Background()
		if ac, acErr := analytics.OpenAnalyticsDB(ctx, configDir); acErr != nil {
			log.Warn("could not open analytics DB (will use log-only fallback)", "err", acErr)
		} else {
			analyticsClient = ac
			log.Info("analytics DB opened", "path", configDir+"/analytics.db")
		}
	} else {
		log.Warn("could not determine config dir for analytics DB", "err", configErr)
	}

	// One-time startup backfill: seed durable BacklogStuckState rows (with
	// notified_at pre-set) for items that are already stuck, so the first
	// genuine reconcile tick does not re-notify for conditions already known
	// before this restart. Must run before the ticker starts (constructed
	// further below, after BacklogController/QuotaGate). Gated on the backlog
	// feature flag directly (backlogCtrl isn't constructed yet at this point)
	// — seeding rows a disabled ticker will never maintain would leave stale,
	// un-reconciled rows behind.
	if cfg.GetFeatureFlag("backlog") {
		backlogLifecycleListener.BackfillStuckStates(context.Background())
	}

	// Construct and start TokenStore early — before BacklogController/QuotaGate
	// below — so QuotaGate's very first Reconcile (synchronous, at boot) can
	// read real, restart-durable token-usage data instead of skipping the soft
	// signal entirely for up to 60s. Only tokenStore/historyDir/homeDir are
	// hoisted here; InsightsService construction, backlogSvc.SetTokenStore, and
	// the ArtifactExtractor wiring stay at their original later position
	// (below), now reading these already-constructed outer-scope variables.
	homeDir, homeDirErr := os.UserHomeDir()
	var tokenStore *tokens.TokenStore
	var historyDir string
	if homeDirErr == nil {
		historyDir = filepath.Join(homeDir, ".claude", "projects")
		tokenStore = tokens.NewTokenStore(historyDir)
		historyLinker.RegisterFileCallback(tokenStore.OnHistoryFileChanged)
		tokenStore.Start(context.Background())
	} else {
		log.Warn("could not determine home dir for InsightsService token store", "err", homeDirErr)
	}

	// Build the BacklogController and initialize its enabled state from config.
	syncRegistry := session.NewDefaultRegistry()
	var keyFunc func() ([]byte, error)
	if cfg != nil {
		keyFunc = cfg.GetOrCreateEncryptionKey
	}
	backlogCtrl := session.NewBacklogController(backlogLifecycleListener, storage, syncRegistry, keyFunc)
	if cfg.GetFeatureFlag("backlog") {
		if err := backlogCtrl.Enable(context.Background()); err != nil {
			log.Error("failed to enable backlog feature on startup — disk config says enabled but the runtime controller is not; TriggerSync will reject calls until this is retried", "err", err)
		} else {
			log.Info("backlog feature enabled")
		}
	} else {
		log.Info("backlog feature disabled (toggle via Settings → Features)")
	}

	// Construct the account-wide quota gate and reconcile it once, synchronously,
	// right after the boot-time Enable() decision above — so if quota state is
	// already bad, backlog is disabled again within the same boot sequence
	// rather than trusting the persisted flag for up to 60s until the first
	// ticker fire. Reads config.json fresh on every call (see cfgFn) so edits
	// take effect without a restart.
	var tokenStoreReader tokens.TokenStoreReader
	if tokenStore != nil {
		tokenStoreReader = tokenStore
	}
	quotaGate := services.NewQuotaGate(
		// config.LoadConfig() re-reads config.json from disk on every call
		// (same pattern as feature_flag_service.go's GetFeatureFlags) — cfg
		// here is the boot-time snapshot passed into BuildRuntimeDeps and is
		// never refreshed, so closing over cfg.Quota directly would freeze
		// every Quota field at boot, contradicting this closure's whole
		// purpose ("no restart needed" for config.json edits).
		func() config.QuotaConfig { return config.LoadConfig().Quota.QuotaConfigOrDefault() },
		tokenStoreReader,
		sessionService,
		backlogCtrl,
		eventBus,
	)
	sessionService.SetQuotaGate(quotaGate)
	quotaGate.Reconcile(context.Background())

	// 60 s reconcile ticker: safety net for abnormal exits where EventExited cannot fire.
	// This goroutine is the only fallback for review-gate respawn, stale-item detection,
	// and PR-pending polling (merge/CI/conflict) — a panic here must not kill it silently.
	// Also drives QuotaGate.Reconcile on the same cadence, independently
	// panic-recovered (via recoverAndLog) so a bug in one never blocks the
	// other from running this tick or any future tick.
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		ctx := context.Background()
		for range ticker.C {
			recoverAndLog("backlog reconcile ticker", func() { backlogLifecycleListener.ReconcileStuck(ctx) })
			recoverAndLog("quota gate reconcile ticker", func() { quotaGate.Reconcile(ctx) })
		}
	}()

	backlogSvc := services.NewBacklogService(storage, sessionService, cfg, workflowEngine, pipelineEngine, pipelineModeRepo)
	backlogSvc.SetEventBus(eventBus)
	backlogSvc.SetSessionStopper(sessionService)
	backlogSvc.SetAutonomousDriverStarter(sessionService)
	if unfinishedScanner != nil {
		backlogSvc.SetRepoWatchRemover(unfinishedScanner)
	}
	// Wires the self-service "Ship PR" action (TriggerShipPR) — sessionService
	// is available this early (constructed in BuildCoreDepsWithOptions, aliased
	// above), so no setter-injection race window, mirroring
	// reactiveQueueMgr.SetOneShotRunner(sessionService) below.
	backlogSvc.SetOneShotRunner(sessionService)
	if headlessPool != nil {
		backlogSvc.SetHeadlessPool(headlessPool)
	}
	backlogSvc.SetScrollbackManager(scrollbackManager)
	// Reuse the same registry/keyFunc backlogCtrl's periodic SyncLoop uses, so a
	// manual TriggerSync call decrypts tokens and dispatches to plugins identically.
	backlogSvc.SetPluginRegistry(syncRegistry)
	backlogSvc.SetSyncKeyFunc(keyFunc)
	// Refuse manual syncs while the backlog feature is toggled off, matching
	// the periodic SyncLoop's behavior.
	// Composed rather than a second gate: quota-critical (heavy — Disable(),
	// pausedByQuota) stays distinct from "a human is actively working" (light
	// — just skip this tick's dispatch), enforced through the one existing
	// consulted checker.
	backlogSvc.SetSyncFeatureEnabledCheck(func() bool {
		return backlogCtrl.IsEnabled() && !quotaGate.ShouldThrottleForeground()
	})
	backlogLifecycleListener.SetAutoReopener(backlogSvc)
	backlogLifecycleListener.SetPRFixSpawner(backlogSvc)
	backlogLifecycleListener.SetReviewRespawner(backlogSvc)
	// Wire the orphaned_triage respawner so an idea-status item whose triage
	// session orphaned (crashed, was killed, or a server restart happened
	// mid-triage) gets triage automatically re-triggered instead of sitting
	// stuck until a human notices the one-time notification (see
	// TriageRespawner's doc comment in session/backlog_lifecycle.go).
	backlogLifecycleListener.SetTriageRespawner(backlogSvc)
	backlogLifecycleListener.SetDequeuer(backlogSvc)
	// Share BacklogService's live *config.Config instance (and its guarding
	// mutex) with DefaultsService so a Settings update to the WIP cap / rework
	// cap takes effect on BacklogService's very next read instead of requiring
	// a process restart (PR #199 review F1) — see
	// DefaultsService.SetSharedBacklogConfig's doc comment.
	sessionService.SetSharedBacklogConfig(cfg, backlogSvc.ConfigMu())
	// Raising the concurrency limit via Settings should dequeue eligible items
	// immediately rather than waiting up to 60s for the next ReconcileStuck tick.
	sessionService.SetOnGlobalDefaultsUpdated(func() {
		if err := backlogSvc.DequeueNextQueuedItems(context.Background()); err != nil {
			log.Error("backlog dequeue after global defaults update failed", "err", err)
		}
	})
	// Wire the stale_work remediator so an in_progress item whose work session
	// has gone quiet (agent finished and is idle at an interactive prompt,
	// rather than crashed — TmuxAlive/PaneProcessDead both report healthy, so
	// the generic tmux health check never catches this) gets its stale session
	// closed out and a fresh one respawned instead of sitting stuck forever
	// (see StaleWorkRemediator's doc comment in session/backlog_lifecycle.go).
	backlogLifecycleListener.SetStaleWorkRemediator(backlogSvc)
	// Wire the rework_blocked_stale resolver so an open stuck row for a
	// review-status item's stale-but-alive blocking work session clears once
	// that session recovers, ends, or the item leaves review (see
	// ReworkBlockStaleResolver's doc comment in session/backlog_lifecycle.go).
	backlogLifecycleListener.SetReworkBlockStaleResolver(backlogSvc)
	// Wire the archive_terminal_sessions safety-net detector (ReconcileStuck) so it
	// can soft-archive AND kill the tmux pane of work/review sessions for items
	// already done/archived (session.IsTmuxBackedSessionRole decides which roles) —
	// reuses sessionService's ArchiveSessionByUUID/KillTmuxPaneOnly, the same methods
	// BacklogService's SessionStopper uses for the transition-hook/rework-respawn
	// archival paths.
	backlogLifecycleListener.SetSessionArchiver(sessionService)
	// Wire the agent-driven ship runner (shipViaAgentOrFallback,
	// session/backlog_lifecycle.go) so a PASS verdict whose work session has
	// already exited ships via a headless one-shot /backlog/ship run (CI
	// reaction, merge-conflict resolution) instead of going straight to the
	// mechanical pushAndCreatePR backstop. sessionService already satisfies
	// OneShotShipRunner via RunOneShotForSession — same method
	// services.PRRunner requires for TriggerShipPR's manual "Ship PR" button
	// just above.
	backlogLifecycleListener.SetOneShotShipRunner(sessionService)
	// Wire the autonomous-stuck respawner so a work session that hits its turn
	// cap without a DONE signal gets a fresh turn budget directly instead of
	// being forced into a doomed review cycle (see AutonomousStuckRespawner's
	// doc comment in autonomous_orchestration_service.go).
	sessionService.SetAutonomousStuckRespawner(backlogSvc)
	// Wire the zombie-session liveness checker (pre-mortem F3, Task 2.1.3d):
	// reuses the existing session.Registry + Instance.TmuxSessionExists rather
	// than inventing a new liveness mechanism. Acquire failure (session not
	// tracked live) is treated as "not alive" — the whole point of this check
	// is to catch sessions whose DB row looks active but whose process is gone.
	//
	// Prefer the already-live in-memory instance (tracked by ReviewQueuePoller,
	// which every backlog-spawned session — review-gate and work/rework alike —
	// registers with via CreateDirectorySession/CreateWorktreeSession) over
	// registry.WithInstance. Backlog-spawned sessions are never Registry.Register()'d
	// (only the main CreateSession RPC path does that), so registry.WithInstance's
	// Acquire falls through to its "construct fresh" branch on every call:
	// newLiveInstance -> FromInstanceData synchronously calls instance.Start(false)
	// as a side effect of merely constructing an Active-status Instance (see
	// fromInstanceData's Active branch), and WithInstance's deferred release() tears
	// the throwaway instance back down the moment this closure returns. Because this
	// checker runs on every 60s ReconcileStuck tick for every review-status item's
	// linked sessions, that reconstruct-Start-teardown cycle repeated forever,
	// spawning and killing a redundant tmux attach-session PTY client against the
	// SAME tmux pane the real, already-wired review session's own controller was
	// using — confirmed live 2026-07-20 (item 93565fa1) as the actual cause of a
	// review-gate session whose controller never made progress. TmuxSessionExists()
	// is a pure tmux-existence probe that works on the real, already-started
	// instance with no reconstruction needed, so checking it there avoids the churn
	// entirely. Fall back to the heavier Registry path only when no live instance is
	// tracked (e.g. immediately after a restart, before the poller reloads it).
	if svc.Registry != nil {
		backlogLifecycleListener.SetSessionLivenessChecker(
			newSessionLivenessChecker(sessionService.FindLiveInstance, svc.Registry))
	}
	sessionService.SetBacklogLifecycleListener(backlogLifecycleListener)
	sessionService.SetReviewGateTrigger(backlogLifecycleListener)
	// Wire the tmux-UUID → Claude-conversation-UUID resolver so GetClaudeHistoryMessages
	// can show history for backlog sessions that passed a tmux UUID as the session ID.
	sessionService.SetResolveConversationUUID(storage.GetClaudeConversationUUIDBySessionUUID)
	sessionService.SetFeatureController("backlog", backlogCtrl)
	sessionService.SetStatusDetailProvider("backlog", quotaGate.StatusDetail)

	// Check VNC dependencies once at startup so the server knows whether browser
	// passthrough is available on this host. Non-fatal: Missing deps log a warning.
	vncDeps := vnc.CheckDependencies()
	if !vncDeps.Available {
		log.Warn("VNC browser passthrough unavailable", "reason", vncDeps.Reason, "missing", vncDeps.Missing)
	} else {
		log.Info("VNC browser passthrough available")
	}

	// Check CDP (Chrome) dependencies once at startup. Non-fatal.
	cdpDeps := cdp.CheckDependencies()
	if !cdpDeps.Available {
		log.Warn("CDP browser streaming unavailable", "reason", cdpDeps.Reason)
	} else {
		log.Info("CDP browser streaming available", "chrome", cdpDeps.ChromePath)
	}

	// Initialize InsightsService for token usage analytics. tokenStore/historyDir
	// were already constructed and started earlier (see the QuotaGate wiring
	// above) so QuotaGate's boot-time Reconcile could read real data — this
	// block only builds the InsightsService/ArtifactExtractor machinery that
	// depends on them, plus backlogSvc/sessionSummaryGenerator, all of which
	// need backlogSvc (constructed after that early wiring).
	var insightsSvc *services.InsightsService
	if tokenStore != nil {
		pricing := tokens.DefaultPricingTable()
		if configDir, cfgErr := config.GetConfigDir(); cfgErr == nil {
			overridePath := filepath.Join(configDir, "pricing_overrides.json")
			if overrideTable, loadErr := tokens.LoadPricingOverride(overridePath); loadErr == nil {
				pricing = overrideTable
			} else if !os.IsNotExist(loadErr) {
				log.Warn("failed to load pricing override, using defaults", "path", overridePath, "err", loadErr)
			}
		} else {
			log.Warn("failed to resolve config dir, skipping pricing override, using defaults", "err", cfgErr)
		}
		if pricing.IsStale() {
			log.Warn("pricing table is stale (an entry's EffectiveDate is 30+ days old)", "loadedAt", pricing.LoadedAt)
		}
		associator := tokens.NewAssociator(storage)
		insightsSvc = services.NewInsightsService(tokenStore, pricing, associator)
		sessionService.SetTokenStoreReader(tokenStore)
		backlogSvc.SetTokenStore(tokenStore, pricing)
		if sessionSummaryGenerator != nil {
			sessionSummaryGenerator.SetTokenStore(tokenStore)
		}
		log.Info("InsightsService initialized", "historyDir", historyDir)

		// Wire ArtifactExtractor to extract PR links, commits, and URLs from JSONL history.
		// Uses historyLinker.Instances() for live instance snapshots so sessions created
		// after startup are included in lookupTitle and OnScanComplete.
		artifactExtractor := artifacts.NewArtifactExtractor(
			func(title, blob string) error {
				return storage.UpdateInstanceArtifacts(title, blob)
			},
			func(title string) (string, error) {
				return storage.GetInstanceArtifacts(title)
			},
			func(filePath string) (string, bool) {
				return session.FindInstanceByHistoryPath(historyLinker.Instances(), filePath)
			},
		)
		artifactExtractor.OnScanComplete = func(title string, blob *artifacts.SessionArtifactsBlob) {
			// Take a snapshot to avoid a data race with concurrent AddInstance calls (M-5 fix).
			snapshot := historyLinker.Instances()
			for _, inst := range snapshot {
				if inst.Title == title {
					inst.SetArtifacts(blob)
					eventBus.Publish(events.NewSessionUpdatedEvent(inst, []string{"artifacts"}))
					// Feed discovered PR URLs to the PR status poller if not already set.
					if !inst.HasGitHubPR() {
						for _, prURL := range blob.PRURLs {
							if ref, err := session.ParseGitHubURL(prURL); err == nil && ref.PRNumber > 0 {
								if err := storage.UpdateInstancePRNumber(inst.Title, ref.PRNumber); err != nil {
									log.Warn("ArtifactExtractor: failed to update PR number", "session", inst.Title, "err", err)
								} else {
									// Update in-memory state so HasGitHubPR() reflects the change (M-3 fix).
									inst.SetGitHubPRNumber(ref.PRNumber)
								}
								break
							}
						}
					}
					break
				}
			}
		}
		historyLinker.RegisterFileCallback(artifactExtractor.OnHistoryFileChanged)
		artifactExtractor.SeedOffsets(session.InstanceInfoSlice(historyLinker.Instances()))
		artifactExtractor.Start(context.Background(), historyDir)
		log.Info("ArtifactExtractor initialized", "historyDir", historyDir)
	}
	// else: home dir resolution already failed and was logged earlier, where
	// tokenStore/historyDir are constructed (see the QuotaGate wiring above).

	// Initialize WorkflowRepository using the ent client from storage.
	// Nil-safe: when the storage is not ent-backed (e.g. tests), WorkflowRepo is nil.
	var workflowRepo session.WorkflowRepository
	if entClient := storage.GetEntClient(); entClient != nil {
		workflowRepo = session.NewEntWorkflowRepository(entClient)
		log.Info("WorkflowRepository initialized")
	} else {
		log.Warn("WorkflowRepository unavailable: storage has no ent client")
	}

	// Initialize WorkflowScheduler and WorkflowService with deferred injection.
	// Order: SessionService → WorkflowScheduler → WorkflowService → SessionService.SetWorkflowService
	var workflowScheduler *workflows.Scheduler
	if workflowRepo != nil {
		workflowScheduler = workflows.NewScheduler(workflowRepo, sessionService, eventBus)
		workflowSvc := services.NewWorkflowService(workflowRepo, workflowScheduler, storage)
		sessionService.SetWorkflowService(workflowSvc)
		sessionService.SetWorkflowRepository(workflowRepo)
		log.Info("WorkflowService and WorkflowScheduler initialized")
	} else {
		log.Warn("WorkflowScheduler disabled: no workflow repository available")
	}

	// 30 min reaper: kill any tmux sessions still running for paused instances.
	// Safety net for sessions paused before the kill-on-pause change, or where the
	// initial kill attempt fell back to detach.
	go func() {
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			sessionService.ReapPausedTmuxSessions()
		}
	}()

	return &RuntimeDeps{
		HeadlessPool:            headlessPool,
		ServiceDeps:             svc,
		Instances:               instances,
		ReactiveQueueMgr:        reactiveQueueMgr,
		ScrollbackManager:       scrollbackManager,
		TmuxStreamerManager:     tmuxStreamerManager,
		ExternalDiscovery:       externalDiscovery,
		ExternalApprovalMonitor: externalApprovalMonitor,
		PRStatusPoller:          svc.PRStatusPoller,
		HistoryLinker:           historyLinker,
		ErrorRegistry:           svc.ErrorRegistry,
		UnfinishedScanner:       unfinishedScanner,
		UnfinishedStateStore:    unfinishedStateStore,
		UnfinishedWorkService:   unfinishedWorkSvc,
		WorktreePRPoller:        worktreePRPoller,
		UserPRCache:             userPRCache,
		GitHubUserService:       githubUserSvc,
		InsightsService:         insightsSvc,
		BacklogService:          backlogSvc,
		QuotaGate:               quotaGate,
		SyncLoop:                nil, // managed by BacklogController
		BacklogEnabledCheck:     backlogCtrl.IsEnabled,
		Config:                  cfg,
		AnalyticsEntClient:      analyticsClient,
		VNCDeps:                 vncDeps,
		CDPDeps:                 cdpDeps,
		WorkflowRepo:            workflowRepo,
		WorkflowScheduler:       workflowScheduler,
		Registry:                svc.Registry,
		SessionSummaryGenerator: sessionSummaryGenerator,
	}, nil
}

// registryInstanceChecker is the narrow slice of *session.Registry that
// newSessionLivenessChecker's fallback path needs — defined here (the consuming
// package) rather than in session, per this repo's interface-pollution convention,
// so a fake with no real *session.Storage can stand in for tests.
type registryInstanceChecker interface {
	WithInstance(ctx context.Context, sessionID string, fn func(*session.LiveInstance) error) error
}

// newSessionLivenessChecker builds the func(sessionUUID string) bool wired onto
// BacklogLifecycleListener.SetSessionLivenessChecker (see the call site in
// BuildRuntimeDeps for the full history/rationale).
//
// findLive should be SessionService.FindLiveInstance: it returns the session's
// already-live, already-wired *session.Instance when one is tracked by
// ReviewQueuePoller — which every backlog-spawned session (review-gate, work,
// rework, triage) registers with at creation via CreateDirectorySession /
// CreateWorktreeSession. Checking TmuxSessionExists() directly on that instance
// answers the liveness question with zero side effects.
//
// registry is consulted only when findLive returns nil (no live instance is
// currently tracked — e.g. immediately after a server restart, before the
// startup reconciliation loop reloads it). That path goes through
// registry.WithInstance/Acquire, which — for backlog-spawned sessions, never
// Registry.Register()'d — reconstructs a fresh *session.Instance from storage on
// every call (session.FromInstanceData synchronously calls Instance.Start() for
// an Active-status session as a side effect of construction) and tears it back
// down the moment the callback returns. Left as the unconditional path (as it
// was before this fix), that reconstruct-Start-teardown cycle ran on every 60s
// ReconcileStuck tick for every review-status item's linked sessions, spawning
// and killing a redundant tmux attach-session PTY client against the same pane
// the real, already-wired session's own controller was using — confirmed live
// 2026-07-20 (backlog item 93565fa1) as the actual reason a review-gate
// session's controller never made progress, despite the session having been
// correctly started and wired (Start + SetStatusManager + StartController) at
// spawn time in SpawnReviewSession → CreateDirectorySession.
func newSessionLivenessChecker(findLive func(sessionUUID string) *session.Instance, registry registryInstanceChecker) func(sessionUUID string) bool {
	return func(sessionUUID string) bool {
		if live := findLive(sessionUUID); live != nil {
			return live.TmuxSessionExists()
		}
		alive := false
		_ = registry.WithInstance(context.Background(), sessionUUID, func(li *session.LiveInstance) error {
			alive = li.TmuxSessionExists()
			return nil
		})
		return alive
	}
}

// prNumFromTitle extracts a PR number from a session title following the
// "pr-<number>-..." naming convention (e.g. "pr-1255-actions-spring-boot").
var prNumFromTitle = regexp.MustCompile(`(?i)^pr-(\d+)-`)

// annotateUserPRCache populates session IDs and worktree paths on the cached
// UserPR list. Called in the UserPRCache onUpdated callback. Lives here (not
// in the github package) to avoid an import cycle: github → session → github.
func annotateUserPRCache(cache *githubpkg.UserPRCache, poller *session.PRStatusPoller, scanner *unfinished.Scanner) {
	var annSessions []githubpkg.PRAnnotationSession
	if poller != nil {
		for _, inst := range poller.GetInstances() {
			// Use Snapshot() — actor-based writes (SetGitHubPRNumber etc.) do not hold
			// mu, so direct field reads would race with concurrent poller updates.
			snap := inst.Snapshot()
			prNumber := snap.GitHub.GitHubPRNumber

			// Resolve a full RepoRef (owner + repo) via a 3-tier fallback for RepoRef,
			// plus a 4th title-regex path for PR number extraction:
			// 1. Direct from DB fields (new sessions written since schema migration).
			// 2. Parse from stored PR URL.
			// 3. Infer from git remote.
			// 4. PR number from session title (e.g. "pr-1255-...").
			var repoRef githubpkg.RepoRef
			if snap.GitHub.GitHubOwner != "" && snap.GitHub.GitHubRepo != "" {
				repoRef, _ = githubpkg.NewRepoRef(snap.GitHub.GitHubOwner, snap.GitHub.GitHubRepo)
			}
			if !repoRef.IsValid() && snap.GitHub.GitHubPRURL != "" {
				if parsed, err := session.ParseGitHubURL(snap.GitHub.GitHubPRURL); err == nil {
					repoRef, _ = githubpkg.NewRepoRef(parsed.Owner, parsed.Repo)
					if prNumber == 0 {
						prNumber = parsed.PRNumber
					}
				}
			}
			if !repoRef.IsValid() && snap.Path != "" {
				repoRef, _ = githubpkg.GetOwnerRepoFromRemote(snap.Path)
			}
			if !repoRef.IsValid() {
				continue
			}
			// Last resort: extract PR number from session title (e.g. "pr-1255-...").
			if prNumber == 0 {
				if m := prNumFromTitle.FindStringSubmatch(inst.Title); m != nil {
					prNumber, _ = strconv.Atoi(m[1])
				}
			}
			annSessions = append(annSessions, githubpkg.PRAnnotationSession{
				ID:       inst.Title,
				Branch:   snap.Branch,
				Repo:     repoRef,
				PRNumber: prNumber,
			})
		}
	}

	var annWorktrees []githubpkg.PRAnnotationWorktree
	if scanner != nil {
		for _, r := range scanner.GetAllResults() {
			repoRef, err := githubpkg.GetOwnerRepoFromRemote(r.RepoPath)
			if err != nil || !repoRef.IsValid() || r.Branch == "" {
				continue
			}
			annWorktrees = append(annWorktrees, githubpkg.PRAnnotationWorktree{
				Branch:       r.Branch,
				Repo:         repoRef,
				WorktreePath: r.WorktreePath,
			})
		}
	}

	cache.Annotate(annSessions, annWorktrees)
}

// scannerSource adapts *unfinished.Scanner to session.WorktreeSource, bridging
// the two packages without creating an import cycle.
// (session/unfinished → pkg/events → session would cycle; this adapter lives here.)
type scannerSource struct {
	s *unfinished.Scanner
}

func (a *scannerSource) ScanDone() <-chan time.Time {
	return a.s.ScanDone()
}

func (a *scannerSource) GetWorktrees() []session.WorktreeScanItem {
	results := a.s.GetAllResults()
	items := make([]session.WorktreeScanItem, 0, len(results))
	for _, r := range results {
		items = append(items, session.WorktreeScanItem{
			RepoPath:     r.RepoPath,
			Branch:       r.Branch,
			WorktreePath: r.WorktreePath,
		})
	}
	return items
}
