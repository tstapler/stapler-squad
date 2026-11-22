package server

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"entgo.io/ent/dialect"

	"github.com/tstapler/stapler-squad/config"
	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
	warren "github.com/tstapler/stapler-squad/pkg/warren"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/reviewqueue"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/services/backlogflow"
	"github.com/tstapler/stapler-squad/server/services/backlogsvc"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/server/services/insights"
	"github.com/tstapler/stapler-squad/server/wakes"
	"github.com/tstapler/stapler-squad/server/workflows"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/backlog"
	"github.com/tstapler/stapler-squad/session/cdp"
	"github.com/tstapler/stapler-squad/session/contexthistory"
	"github.com/tstapler/stapler-squad/session/dbpool"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/driver"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/external"
	"github.com/tstapler/stapler-squad/session/headless"
	"github.com/tstapler/stapler-squad/session/importer"
	"github.com/tstapler/stapler-squad/session/instance"
	sessionjules "github.com/tstapler/stapler-squad/session/jules"
	"github.com/tstapler/stapler-squad/session/prpoller"
	queuepkg "github.com/tstapler/stapler-squad/session/queue"
	"github.com/tstapler/stapler-squad/session/scrollback"
	"github.com/tstapler/stapler-squad/session/stagecfg"
	"github.com/tstapler/stapler-squad/session/summaries"
	"github.com/tstapler/stapler-squad/session/tmux"
	"github.com/tstapler/stapler-squad/session/unfinished"
	"github.com/tstapler/stapler-squad/session/vnc"
	"github.com/tstapler/stapler-squad/session/workflowengine"
)

// Delays before the startup transcript backfill begins (live file changes are
// never delayed). Tokens go first because QuotaGate reads token usage at boot;
// a warm restart restores persisted checkpoints immediately regardless.
const (
	tokenBackfillDelay    = 15 * time.Second
	artifactBackfillDelay = 30 * time.Second
)

// ServerDependencies holds all wired service components for the HTTP server.
// Use BuildDependencies to construct and wire them in the correct order.
// See the initialization order comment on NewServer for dependency constraints.
type ServerDependencies struct {
	// WakeMetrics is shared by the wake service's expiry sweep and the dispatcher
	// so both feed one wake_trigger_total mirror; created on first use by sharedWakeMetrics.
	WakeMetrics       *wakes.Metrics
	wakeMetricsMu     sync.Mutex // guards lazy creation in sharedWakeMetrics
	SessionService    *services.SessionService
	Storage           *session.Storage
	Instances         []*instance.Instance
	EventBus          *events.EventBus
	StatusManager     *instance.InstanceStatusManager
	ReviewQueue       *queuepkg.ReviewQueue
	ReviewQueuePoller *session.ReviewQueuePoller
	PRStatusPoller    *prpoller.PRStatusPoller

	// StopPRMaintenance stops the PR startup-maintenance retry ticker (see
	// BuildOptions.RunPRMaintenance); nil when maintenance did not run.
	StopPRMaintenance func()
	// StopBootInit cancels the background boot-init goroutine and waits for it to exit.
	StopBootInit            func()
	ReactiveQueueMgr        *reviewqueue.ReactiveQueueManager
	ScrollbackManager       *scrollback.ScrollbackManager
	TmuxStreamerManager     *external.ExternalTmuxStreamerManager
	ExternalDiscovery       *external.ExternalSessionDiscovery
	ExternalApprovalMonitor *external.ExternalApprovalMonitor
	HistoryLinker           *importer.HistoryLinker
	ErrorRegistry           *services.ErrorRegistry

	// ClaudeSettingsWatcher watches ~/.claude/settings.json (and project-level
	// equivalents) for edits and hot-reloads their derived auto-approval rules. Started
	// from wireDepsIntoServer with the server's lifecycle context so it stops cleanly
	// on shutdown.
	ClaudeSettingsWatcher *services.ClaudeSettingsWatcher

	// SlackNotifier is the single shared Slack notifier instance, wired into
	// both ReactiveQueueMgr (review-queue items) and ApprovalHandler (pending
	// approvals) so GetDeliveryStatus reflects sends from both trigger points.
	SlackNotifier *services.SlackNotifier

	// Unfinished work scanning.
	UnfinishedScanner     *unfinished.Scanner
	UnfinishedStateStore  *unfinished.StateStore
	UnfinishedWorkService *services.UnfinishedWorkService
	WorktreePRPoller      *prpoller.WorktreePRPoller

	// UnfinishedWatchDirWatcher discovers repos under the user-configured
	// watch directories (Settings → Unfinished Work Sources) and registers
	// them with UnfinishedScanner. Nil under the same conditions
	// UnfinishedScanner is nil (see BuildRuntimeDeps' unfinished-work
	// wiring block) — its own field, not folded into UnfinishedScanner,
	// because Start needs to run once from wireDepsIntoServer alongside
	// (not replacing) UnfinishedScanner.Start.
	UnfinishedWatchDirWatcher *unfinished.WatchDirWatcher

	// JulesSessionPoller polls open Jules sessions (google-jules-integration
	// Epic 2.3/2.4). Nil unless config.Config.Jules.Enabled and the API key
	// resolves at startup — see BuildRuntimeDeps' jules wiring block.
	JulesSessionPoller *sessionjules.JulesSessionPoller

	// GitHub user PR cache and service. Nil when no GitHub token is available.
	UserPRCache       *githubpkg.UserPRCache
	GitHubUserService *services.GitHubUserService

	// Token usage analytics.
	InsightsService *insights.InsightsService

	BacklogService *backlogsvc.BacklogService
	// BacklogFlowService is the single flow-observability service; the RPC handler and
	// the HTTP MCP core both use it so they share one aggregator cache.
	BacklogFlowService *backlogflow.BacklogFlowService
	// QuotaGate owns the account-wide session-quota pause/resume decision for
	// backlog automation (see BacklogService/BacklogEnabledCheck above).
	QuotaGate *services.QuotaGate
	SyncLoop  *session.SyncLoop

	// WorkflowEngine is the selector/recorder stack behind BacklogService's
	// engine; the server starts its recorder and wires the soak actuator.
	WorkflowEngine *WorkflowEngineWiring

	// BacklogEnabledCheck reports the live runtime state of the "backlog" feature
	// flag. See RuntimeDeps.BacklogEnabledCheck.
	BacklogEnabledCheck func() bool

	// Analytics storage. Nil when the analytics DB failed to open (LogAnalyticsProvider
	// is used as a fallback in that case).
	AnalyticsEntClient *ent.Client
	// AnalyticsDriver is the instrumented driver behind AnalyticsEntClient, used
	// by background SQLite maintenance. Nil whenever AnalyticsEntClient is nil.
	AnalyticsDriver dialect.ExecQuerier

	// Escape-event storage: db/escape_events.db, its own SQLite file and single
	// writer so escape inserts and retention deletes never queue analytics_events.
	// All three are nil when the database failed to open (escape capture is then off).
	EscapeEventsClient *ent.Client
	EscapeEventsDriver dialect.ExecQuerier
	EscapeEventsDB     *dbpool.Database

	// VNCDeps holds the result of the startup VNC dependency check.
	// Available=false means the Browser tab will be hidden on all sessions.
	VNCDeps vnc.DepsResult

	// CDPDeps holds the result of the startup CDP (Chrome) dependency check.
	// Available=false means CDP browser streaming is unavailable on this host.
	CDPDeps cdp.DepsResult

	// HeadlessPool manages headless LLM calls. Nil when the claude binary is not found.
	HeadlessPool *headless.Pool

	// LLMSelector and LLMClient route headless calls through the live backend
	// selection (config llm_backends). LLMClient is an untyped nil when HeadlessPool is nil.
	LLMSelector *headless.Selector
	LLMClient   headless.PoolClient

	// GeminiCaller is a headless.PoolClient adapter shelling out to the `gemini`
	// CLI (backlog-stage-execution-costs Epic 3.1). Nil when the gemini binary is
	// not found at startup. Registered into BacklogService's headlessCallers
	// registry under "gemini" via SetGeminiCaller (Epic 2.3), so a PipelineMode
	// stage executor configured with program="gemini" dispatches here via
	// resolveHeadlessCaller. Exposed on this struct mainly for tests/observability;
	// production callers reach it through BacklogService, not this field directly.
	GeminiCaller *headless.GeminiCaller

	// WorkflowRepo persists workflow definitions.
	WorkflowRepo stagecfg.WorkflowRepository

	// WorkflowScheduler manages cron-based workflow execution.
	WorkflowScheduler *workflows.Scheduler

	// TriggerFireEventRepo persists the trigger-fire audit trail (webhook-triggers
	// Epic 1.2), shared by Scheduler, WorkflowService, and the inbound webhook
	// handlers (Epic 2.2/2.3). Nil when storage is not ent-backed.
	TriggerFireEventRepo session.TriggerFireEventRepository

	// Registry is the live-handle map for all running sessions.
	Registry *session.Registry

	// SessionSummaryGenerator drives async session-completion-summary generation.
	// Nil when storage is not ent-backed. Its NotificationDecisionLister/TokenStore
	// dependencies are wired later via SetNotificationLister/SetTokenStore (see
	// server.go's RunServer) — see the comment on SetNotificationLister for why.
	SessionSummaryGenerator *summaries.SessionSummaryGenerator

	// HandoffSummaryGenerator drives async restart-handoff-summary generation
	// (Story 2.2.1). Nil when storage is not ent-backed, mirroring
	// SessionSummaryGenerator.
	HandoffSummaryGenerator *summaries.HandoffSummaryGenerator

	// BacklogLifecycleListener drives backlog item state transitions and, via its
	// TriggerPRFixForEvent method, satisfies services.PRFixEventRouter for the
	// pr-event-webhooks feature (immediate PR-fix reconciliation on a GitHub
	// webhook delivery, instead of waiting for PRStatusPoller's next tick).
	BacklogLifecycleListener *backlog.BacklogLifecycleListener

	// SessionTagClassificationPoller drives the Phase 4 LLM fallback tag classification
	// (session-classifier-pipeline). Nil when HeadlessPool is nil (no claude binary found).
	SessionTagClassificationPoller *session.SessionTagClassificationPoller

	// CtxHistoryStore persists per-turn context-window occupancy samples. Nil
	// when the history directory cannot be resolved at startup (see the
	// identically-named field on RuntimeDeps for the full comment). Passed to
	// SessionBundleService so the context_history section is populated when available.
	CtxHistoryStore *contexthistory.Store
}

// ToServerDeps converts RuntimeDeps to the flat ServerDependencies struct consumed
// by NewServerWithDeps. This mirrors the projection done inside BuildDependencies.
func (rt *RuntimeDeps) ToServerDeps() *ServerDependencies {
	return &ServerDependencies{
		SessionService:                 rt.SessionService,
		Storage:                        rt.Storage,
		Instances:                      rt.Instances,
		EventBus:                       rt.EventBus,
		StatusManager:                  rt.StatusManager,
		ReviewQueue:                    rt.ReviewQueue,
		ReviewQueuePoller:              rt.ReviewQueuePoller,
		PRStatusPoller:                 rt.PRStatusPoller,
		StopPRMaintenance:              rt.StopPRMaintenance,
		StopBootInit:                   rt.StopBootInit,
		ReactiveQueueMgr:               rt.ReactiveQueueMgr,
		ScrollbackManager:              rt.ScrollbackManager,
		TmuxStreamerManager:            rt.TmuxStreamerManager,
		ExternalDiscovery:              rt.ExternalDiscovery,
		ExternalApprovalMonitor:        rt.ExternalApprovalMonitor,
		HistoryLinker:                  rt.HistoryLinker,
		ErrorRegistry:                  rt.ErrorRegistry,
		ClaudeSettingsWatcher:          rt.ClaudeSettingsWatcher,
		SlackNotifier:                  rt.SlackNotifier,
		UnfinishedScanner:              rt.UnfinishedScanner,
		UnfinishedStateStore:           rt.UnfinishedStateStore,
		UnfinishedWorkService:          rt.UnfinishedWorkService,
		UnfinishedWatchDirWatcher:      rt.UnfinishedWatchDirWatcher,
		WorktreePRPoller:               rt.WorktreePRPoller,
		JulesSessionPoller:             rt.JulesSessionPoller,
		UserPRCache:                    rt.UserPRCache,
		GitHubUserService:              rt.GitHubUserService,
		InsightsService:                rt.InsightsService,
		BacklogService:                 rt.BacklogService,
		BacklogFlowService:             rt.BacklogFlowService,
		QuotaGate:                      rt.QuotaGate,
		SyncLoop:                       rt.SyncLoop,
		WorkflowEngine:                 rt.WorkflowEngine,
		BacklogEnabledCheck:            rt.BacklogEnabledCheck,
		AnalyticsEntClient:             rt.AnalyticsEntClient,
		AnalyticsDriver:                rt.AnalyticsDriver,
		EscapeEventsClient:             rt.EscapeEventsClient,
		EscapeEventsDriver:             rt.EscapeEventsDriver,
		EscapeEventsDB:                 rt.EscapeEventsDB,
		VNCDeps:                        rt.VNCDeps,
		CDPDeps:                        rt.CDPDeps,
		HeadlessPool:                   rt.HeadlessPool,
		LLMSelector:                    rt.LLMSelector,
		LLMClient:                      rt.LLMClient,
		GeminiCaller:                   rt.GeminiCaller,
		WorkflowRepo:                   rt.WorkflowRepo,
		WorkflowScheduler:              rt.WorkflowScheduler,
		TriggerFireEventRepo:           rt.TriggerFireEventRepo,
		Registry:                       rt.Registry,
		SessionSummaryGenerator:        rt.SessionSummaryGenerator,
		HandoffSummaryGenerator:        rt.HandoffSummaryGenerator,
		BacklogLifecycleListener:       rt.BacklogLifecycleListener,
		SessionTagClassificationPoller: rt.SessionTagClassificationPoller,
		CtxHistoryStore:                rt.CtxHistoryStore,
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
	core, err := BuildCoreDepsWithOptions(BuildOptions{RunPRMaintenance: true})
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
	instances []*instance.Instance,
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
	instMap := make(map[string]*instance.Instance, len(instances))
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

		item := &queuepkg.ReviewItem{
			SessionID:   approval.SessionID,
			SessionName: approval.SessionID,
			Reason:      queuepkg.ReasonApprovalPending,
			Priority:    queuepkg.PriorityHigh,
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
			snap := inst.Snapshot()
			item.Program = inst.Program
			item.Branch = inst.Branch
			item.Path = snap.Path
			item.WorkingDir = snap.WorkingDir
			item.Status = inst.GetLifecycleStatus().String()
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
	ReviewQueue    *queuepkg.ReviewQueue
	ApprovalStore  *services.ApprovalStore
	ErrorRegistry  *services.ErrorRegistry

	// FlagRegistry is the one registry shared by the flag service and every other flag reader.
	FlagRegistry *featureflags.Registry

	// StopPRMaintenance stops the PR startup-maintenance retry ticker and waits for
	// its goroutine; nil unless BuildOptions.RunPRMaintenance was set.
	StopPRMaintenance func()
}

// BuildOptions carries optional overrides for BuildCoreDepsWithOptions.
// The zero value uses all defaults (equivalent to calling BuildCoreDeps).
type BuildOptions struct {
	// EntClient supplies a pre-opened *ent.Client, bypassing config-based DB path
	// discovery and schema migration. nil = open from config as usual.
	EntClient *ent.Client

	// RunPRMaintenance runs session.Storage.RunPRStartupMaintenance right after
	// Storage exists and before any poller, sweeper or ticker is built. Only the
	// service bootstrap sets it: the stdio MCP fallback (RunStdioMCP) and hook/CLI
	// processes must not import links or reset verification counters.
	RunPRMaintenance bool

	// PRMaintenanceRunner replaces the real maintenance call in tests (a recording
	// fake); nil means storage.RunPRStartupMaintenance.
	PRMaintenanceRunner func(context.Context, *session.Storage) error
	// Context bounds PR startup maintenance (its boot retries and retry ticker), so
	// a SIGTERM during boot stops it. Nil means context.Background().
	Context context.Context
}

// BuildCoreDepsWithOptions constructs Phase 1 dependencies with optional overrides.
// Use BuildOptions to inject a pre-built EntClient (for tests).
func BuildCoreDepsWithOptions(opts BuildOptions) (*CoreDeps, error) {
	driver.Register() // the instance package reaches the session driver only through this hook

	// A flag owned by a leaf package adds its Spec slice as one more argument here.
	flagRegistry := services.FlagRegistry()

	var sessionService *services.SessionService
	var err error
	if opts.EntClient != nil {
		sessionService, err = services.NewSessionServiceWithEntClient(opts.EntClient, flagRegistry)
	} else {
		sessionService, err = services.NewSessionServiceFromConfig(flagRegistry)
	}
	if err != nil {
		return nil, fmt.Errorf("initialize SessionService: %w", err)
	}

	storage := sessionService.GetStorage()

	var stopPRMaintenance func()
	if opts.RunPRMaintenance {
		if stopPRMaintenance, err = runPRMaintenance(opts, storage); err != nil {
			return nil, err
		}
	}

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
		FlagRegistry:   flagRegistry,

		StopPRMaintenance: stopPRMaintenance,
	}, nil
}

// runPRMaintenance runs the PR startup maintenance for the service bootstrap.
// Maintenance never fails the boot, so an error here means only that a test
// runner returned one. The retry ticker, if any, is owned by storage and stopped
// through the returned function.
func runPRMaintenance(opts BuildOptions, storage *session.Storage) (func(), error) {
	run := opts.PRMaintenanceRunner
	if run == nil {
		run = func(ctx context.Context, st *session.Storage) error { return st.RunPRStartupMaintenance(ctx) }
	}
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}
	if err := run(ctx, storage); err != nil {
		return nil, fmt.Errorf("pr startup maintenance: %w", err)
	}
	return storage.StopPRMaintenance, nil
}

// BuildCoreDeps constructs Phase 1 dependencies using config defaults.
// It is a thin wrapper around BuildCoreDepsWithOptions(BuildOptions{}).
func BuildCoreDeps() (*CoreDeps, error) {
	return BuildCoreDepsWithOptions(BuildOptions{})
}

// ServiceDeps holds Phase 2 dependencies: management components that depend on CoreDeps.
type ServiceDeps struct {
	*CoreDeps
	StatusManager     *instance.InstanceStatusManager
	ReviewQueuePoller *session.ReviewQueuePoller
	PRStatusPoller    *prpoller.PRStatusPoller
	Registry          *session.Registry

	// AttentionOptions is the one cached attention-flag source shared by the review-queue poller and the startup scanner.
	AttentionOptions session.FlagOptionsSource
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

	statusManager := instance.NewInstanceStatusManager()
	reviewQueuePoller := session.NewReviewQueuePoller(
		core.ReviewQueue, statusManager, core.Storage,
	)
	attentionOptions := newAttentionOptionsSource(core.FlagRegistry)
	reviewQueuePoller.SetOptionsSource(attentionOptions)
	prStatusPoller := prpoller.NewPRStatusPoller(core.Storage)

	registry := session.NewRegistry(core.Storage, core.SessionService.WireInstanceCallbacks)
	core.SessionService.SetRegistry(registry)

	w := warren.NewWire("ServiceDeps")
	warren.Set(w, "ApprovalProvider", reviewQueuePoller.SetApprovalProvider, domain.ApprovalMetadataProvider(core.ApprovalStore))
	// Story 5.3.1: the same provider wired into StatusManager.GetStatus()'s
	// pi-fallback branch, so a pi session actually blocked on a pending
	// approval reports NeedsApproval instead of a stale Idle.
	warren.Set(w, "StatusManagerApprovalProvider", statusManager.SetApprovalProvider, domain.ApprovalMetadataProvider(core.ApprovalStore))
	warren.Set(w, "StatusManager", core.SessionService.SetStatusManager, statusManager)
	warren.Set(w, "ReviewQueuePoller", core.SessionService.SetReviewQueuePoller, reviewQueuePoller)
	// The session header's Refresh awaits one poll of a single PR row. There is no
	// user_refresh observation source yet, so it uses reconciler_refresh.
	// A successful fetch resolves an unverified-terminal row and resets
	// poll_fail_count; a counted failure leaves it unverified and increments it.
	core.SessionService.SetPRRefreshFunc(func(ctx context.Context, _ *instance.Instance, ref domain.PRRef) error {
		return prStatusPoller.Refresh(ctx, ref, domain.PRSourceReconcilerRefresh)
	})
	if err := w.Validate(); err != nil {
		return nil, err
	}

	return &ServiceDeps{
		CoreDeps:          core,
		StatusManager:     statusManager,
		ReviewQueuePoller: reviewQueuePoller,
		PRStatusPoller:    prStatusPoller,
		Registry:          registry,
		AttentionOptions:  attentionOptions,
	}, nil
}

// RuntimeDeps holds Phase 3 dependencies: runtime components that involve
// process creation, filesystem I/O, and callback wiring.
type RuntimeDeps struct {
	*ServiceDeps
	// StopBootInit cancels the background boot-init goroutine and waits for it to exit.
	StopBootInit            func()
	Instances               []*instance.Instance
	ReactiveQueueMgr        *reviewqueue.ReactiveQueueManager
	ScrollbackManager       *scrollback.ScrollbackManager
	TmuxStreamerManager     *external.ExternalTmuxStreamerManager
	ExternalDiscovery       *external.ExternalSessionDiscovery
	ExternalApprovalMonitor *external.ExternalApprovalMonitor
	PRStatusPoller          *prpoller.PRStatusPoller
	HistoryLinker           *importer.HistoryLinker
	ErrorRegistry           *services.ErrorRegistry

	// ClaudeSettingsWatcher (see the identically-named field on ServerDependencies for
	// its full doc comment).
	ClaudeSettingsWatcher *services.ClaudeSettingsWatcher

	// SlackNotifier is the single shared Slack notifier instance (see the
	// identically-named field on ServerDependencies for its full doc comment).
	SlackNotifier *services.SlackNotifier

	// Unfinished work scanning.
	UnfinishedScanner     *unfinished.Scanner
	UnfinishedStateStore  *unfinished.StateStore
	UnfinishedWorkService *services.UnfinishedWorkService
	WorktreePRPoller      *prpoller.WorktreePRPoller

	// UnfinishedWatchDirWatcher (see the identically-named field on
	// ServerDependencies for its full doc comment).
	UnfinishedWatchDirWatcher *unfinished.WatchDirWatcher

	// JulesSessionPoller (see the identically-named field on ServerDependencies
	// for its full doc comment).
	JulesSessionPoller *sessionjules.JulesSessionPoller

	// GitHub user PR cache and service.
	UserPRCache       *githubpkg.UserPRCache
	GitHubUserService *services.GitHubUserService

	// Token usage analytics.
	InsightsService *insights.InsightsService

	BacklogService *backlogsvc.BacklogService
	// BacklogFlowService is the single flow-observability service; the RPC handler and
	// the HTTP MCP core both use it so they share one aggregator cache.
	BacklogFlowService *backlogflow.BacklogFlowService
	// QuotaGate owns the account-wide session-quota pause/resume decision for
	// backlog automation (see BacklogService/BacklogEnabledCheck above).
	QuotaGate *services.QuotaGate
	SyncLoop  *session.SyncLoop

	// WorkflowEngine (see the identically-named field on ServerDependencies).
	WorkflowEngine *WorkflowEngineWiring

	// BacklogController starts and stops the periodic SyncLoop; exposed so the
	// production-wiring test can check which engine that loop holds.
	BacklogController *session.BacklogController

	Config *config.Config // Used for encryption of sensitive data

	// BacklogEnabledCheck reports the live runtime state of the "backlog" feature
	// flag (backlogCtrl.IsEnabled). Threaded into the MCP server so backlog/goal
	// tool calls are gated by the same source of truth as the ConnectRPC interceptor.
	BacklogEnabledCheck func() bool

	// Analytics storage.
	AnalyticsEntClient *ent.Client
	AnalyticsDriver    dialect.ExecQuerier

	// Escape-event storage (see ServerDependencies).
	EscapeEventsClient *ent.Client
	EscapeEventsDriver dialect.ExecQuerier
	EscapeEventsDB     *dbpool.Database

	// VNCDeps holds the result of the startup VNC dependency check.
	VNCDeps vnc.DepsResult

	// CDPDeps holds the result of the startup CDP (Chrome) dependency check.
	CDPDeps cdp.DepsResult

	// HeadlessPool manages headless LLM calling. Nil when claude binary is not found.
	HeadlessPool *headless.Pool

	// LLMSelector/LLMClient: see Deps.LLMSelector.
	LLMSelector *headless.Selector
	LLMClient   headless.PoolClient

	// GeminiCaller is a headless.PoolClient adapter shelling out to the `gemini`
	// CLI (backlog-stage-execution-costs Epic 3.1). Nil when the gemini binary is
	// not found at startup — see the identical field's doc comment on
	// ServerDependencies above.
	GeminiCaller *headless.GeminiCaller

	// WorkflowRepo persists workflow definitions.
	WorkflowRepo stagecfg.WorkflowRepository

	// WorkflowScheduler manages cron-based workflow execution.
	WorkflowScheduler *workflows.Scheduler

	// TriggerFireEventRepo persists the trigger-fire audit trail (webhook-triggers
	// Epic 1.2), shared by Scheduler, WorkflowService, and the inbound webhook
	// handlers (Epic 2.2/2.3). Nil when storage is not ent-backed.
	TriggerFireEventRepo session.TriggerFireEventRepository

	// Registry is the live-handle map for all running sessions.
	Registry *session.Registry

	// SessionSummaryGenerator drives async session-completion-summary generation.
	// Nil when storage is not ent-backed.
	SessionSummaryGenerator *summaries.SessionSummaryGenerator

	// HandoffSummaryGenerator drives async restart-handoff-summary generation
	// (Story 2.2.1). Nil when storage is not ent-backed.
	HandoffSummaryGenerator *summaries.HandoffSummaryGenerator

	// BacklogLifecycleListener satisfies services.PRFixEventRouter for the
	// pr-event-webhooks feature — see the identical field's doc comment on
	// ServerDependencies above.
	BacklogLifecycleListener *backlog.BacklogLifecycleListener

	// SessionTagClassificationPoller drives the Phase 4 LLM fallback tag classification
	// (session-classifier-pipeline). Nil when HeadlessPool is nil (no claude binary found).
	SessionTagClassificationPoller *session.SessionTagClassificationPoller

	// CtxHistoryStore persists per-turn context-window occupancy samples for use
	// in diagnostic bundles (RUM session bundle Epic 1.1). Nil when analytics is
	// disabled or when the ent client is unavailable.
	CtxHistoryStore *contexthistory.Store
}

// reviewQueueLookupAdapter adapts session.Storage's ItemSession/ReviewVerdict
// queries to summaries.ReviewQueueLookup, which BuildDecisionsSnapshot
// (session/summaries/session_summary_snapshot.go) needs. A session's resolved/still-open
// review counts are derived from every ItemSession attached to the same backlog
// item as this session's own ItemSession (siblings included) — the backlog item,
// not just this one session, is the natural "review queue" scope, mirroring how
// review_queue_manager.go's itemSessionLookupTimeout bounds the identical
// ItemSession/ReviewVerdict storage lookup.
type reviewQueueLookupAdapter struct {
	storage *session.Storage
}

// ReviewQueueResolvedCount implements summaries.ReviewQueueLookup.
func (a *reviewQueueLookupAdapter) ReviewQueueResolvedCount(ctx context.Context, sessionID string) (resolved, stillOpen int, err error) {
	itemSession, err := a.storage.GetItemSessionBySessionUUID(ctx, sessionID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
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
	eventBus := svc.EventBus
	sessionService := svc.SessionService

	wireTCCEnrichment(cfg, sessionService)

	// Step 5: load instances from storage
	instances, err := storage.LoadInstances()
	if err != nil {
		return nil, fmt.Errorf("load instances: %w", err)
	}

	// Seed the delivery gate's visibility index synchronously, before any
	// instance is wired or started, so restore-time producers (cold-restore,
	// rate-limit callbacks) resolve hidden sessions. Flags are read once here;
	// the server's ticker keeps them fresh afterwards.
	if gate := sessionService.DeliveryGate(); gate != nil {
		gate.SeedFromInstances(instances)
		gate.Flags().Reload()
	}

	// WorkflowEngine governs backlog state transitions; constructed once and shared
	// by the service layer.
	defaultWorkflowEngine := workflowengine.NewDefaultWorkflowEngine()

	engines := buildBacklogEngines(storage)

	// The selector must exist, with its shadow recorder, before NewBacklogService
	// and the BacklogController's SyncLoop take their engine.
	workflowEngine := newWorkflowEngineWiring(defaultWorkflowEngine, engines.stageConfigEngine, storage.GetEntClient(), config.LoadConfig)
	session.InitNudgeOutcomes()

	llm := buildHeadlessLLM(sessionService)

	sessionTagPoller := buildTagPoller(cfg, llm, sessionService)

	sessionSummaryGenerator, handoffSummaryGenerator := buildSummaryGenerators(svc, llm.client)

	backlogLifecycleListener := newBacklogLifecycleListener(svc, llm, engines)

	if err := wireLoadedInstances(svc, instances, backlogLifecycleListener, sessionSummaryGenerator, sessionTagPoller); err != nil {
		return nil, err
	}

	stopBootInit := startInstanceBootInit(svc, instances, nil)

	reactiveQueueMgr, slackNotifier := buildReactiveQueue(svc, cfg)

	sessionIO := buildSessionIO(len(instances))
	historyLinker, scrollbackManager, tmuxStreamerManager := sessionIO.historyLinker, sessionIO.scrollbackManager, sessionIO.tmuxStreamerManager

	externalDiscovery, externalApprovalMonitor := wireExternalSessions(svc, historyLinker, sessionTagPoller, backlogLifecycleListener, sessionSummaryGenerator)

	if err := wireSessionServiceRuntime(sessionService, reactiveQueueMgr, historyLinker, instances, scrollbackManager, externalDiscovery); err != nil {
		return nil, err
	}

	unfinishedStack := buildUnfinishedWork(svc, cfg)

	userPRCache, githubUserSvc := buildGitHubUserService(svc, cfg, unfinishedStack.scanner)

	analyticsStack := openAnalyticsStores(sessionService)

	startDiagnosticsRetention()

	backfillStuckStates(cfg.GetFeatureFlag("backlog"), backlogLifecycleListener.BackfillStuckStates)

	tokenStack := startTokenStores(svc, cfg, historyLinker)

	control := startBacklogControl(svc, cfg, backlogLifecycleListener, workflowEngine, tokenStack.store)

	startHousekeeping(storage, eventBus)

	backlogSvc, backlogFlowSvc, err := buildBacklogService(svc, cfg, backlogLifecycleListener, engines, workflowEngine, llm, scrollbackManager, control, unfinishedStack.scanner)
	if err != nil {
		return nil, err
	}

	if err := wireBacklogLifecycle(svc, cfg, backlogLifecycleListener, backlogSvc, engines, workflowEngine, control); err != nil {
		return nil, err
	}

	vncDeps, cdpDeps := checkBrowserDeps()

	insightsSvc, geminiCaller := buildInsights(svc, tokenStack, historyLinker, backlogSvc, llm.selector, sessionSummaryGenerator)

	workflowRepo, workflowScheduler, triggerFireEventRepo := buildWorkflows(svc, cfg, backlogSvc, backlogLifecycleListener)

	julesPoller := buildJules(svc, cfg, backlogSvc)

	startPausedTmuxReaper(pausedTmuxReapInterval, sessionService.ReapPausedTmuxSessions)

	// Unwired, GatedTransition answers in Off mode, so every transition would pass unchecked.
	if !storage.TransitionGateWired() {
		return nil, errors.New("workflow engine transition gate is not wired; refusing to start with unchecked transitions")
	}

	return &RuntimeDeps{
		HeadlessPool:                   llm.pool,
		LLMSelector:                    llm.selector,
		LLMClient:                      llm.client,
		GeminiCaller:                   geminiCaller,
		ServiceDeps:                    svc,
		StopBootInit:                   stopBootInit,
		Instances:                      instances,
		ReactiveQueueMgr:               reactiveQueueMgr,
		ScrollbackManager:              scrollbackManager,
		TmuxStreamerManager:            tmuxStreamerManager,
		ExternalDiscovery:              externalDiscovery,
		ExternalApprovalMonitor:        externalApprovalMonitor,
		PRStatusPoller:                 svc.PRStatusPoller,
		HistoryLinker:                  historyLinker,
		ErrorRegistry:                  svc.ErrorRegistry,
		ClaudeSettingsWatcher:          sessionService.GetClaudeSettingsWatcher(),
		SlackNotifier:                  slackNotifier,
		UnfinishedScanner:              unfinishedStack.scanner,
		UnfinishedStateStore:           unfinishedStack.stateStore,
		UnfinishedWorkService:          unfinishedStack.workSvc,
		UnfinishedWatchDirWatcher:      unfinishedStack.watchDirWatcher,
		WorktreePRPoller:               unfinishedStack.prPoller,
		JulesSessionPoller:             julesPoller,
		UserPRCache:                    userPRCache,
		GitHubUserService:              githubUserSvc,
		InsightsService:                insightsSvc,
		BacklogService:                 backlogSvc,
		BacklogFlowService:             backlogFlowSvc,
		QuotaGate:                      control.quotaGate,
		SyncLoop:                       nil, // managed by BacklogController
		WorkflowEngine:                 workflowEngine,
		BacklogController:              control.ctrl,
		BacklogEnabledCheck:            control.ctrl.IsEnabled,
		Config:                         cfg,
		AnalyticsEntClient:             analyticsStack.client,
		AnalyticsDriver:                analyticsStack.driver,
		EscapeEventsClient:             analyticsStack.escClient,
		EscapeEventsDriver:             analyticsStack.escDriver,
		EscapeEventsDB:                 analyticsStack.escDB,
		VNCDeps:                        vncDeps,
		CDPDeps:                        cdpDeps,
		WorkflowRepo:                   workflowRepo,
		WorkflowScheduler:              workflowScheduler,
		TriggerFireEventRepo:           triggerFireEventRepo,
		Registry:                       svc.Registry,
		SessionSummaryGenerator:        sessionSummaryGenerator,
		HandoffSummaryGenerator:        handoffSummaryGenerator,
		BacklogLifecycleListener:       backlogLifecycleListener,
		SessionTagClassificationPoller: sessionTagPoller,
		CtxHistoryStore:                tokenStack.ctxHistory,
	}, nil
}

// registryInstanceChecker is the narrow slice of *session.Registry that
// newSessionLivenessChecker's fallback path needs — defined here (the consuming
// package) rather than in session, per this repo's interface-pollution convention,
// so a fake with no real *session.Storage can stand in for tests.
type registryInstanceChecker interface {
	WithInstance(ctx context.Context, sessionID string, fn func(*instance.LiveInstance) error) error
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
func newSessionLivenessChecker(findLive func(sessionUUID string) *instance.Instance, registry registryInstanceChecker) func(sessionUUID string) bool {
	return func(sessionUUID string) bool {
		if live := findLive(sessionUUID); live != nil {
			return live.TmuxSessionExists()
		}
		alive := false
		_ = registry.WithInstance(context.Background(), sessionUUID, func(li *instance.LiveInstance) error {
			alive = li.TmuxSessionExists()
			return nil
		})
		return alive
	}
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

func (a *scannerSource) GetWorktrees() []prpoller.WorktreeScanItem {
	results := a.s.GetAllResults()
	items := make([]prpoller.WorktreeScanItem, 0, len(results))
	for _, r := range results {
		items = append(items, prpoller.WorktreeScanItem{
			RepoPath:     r.RepoPath,
			Branch:       r.Branch,
			WorktreePath: r.WorktreePath,
			Fingerprint:  worktreeFingerprint(r),
		})
	}
	return items
}

// installWorktreeEnricher starts and registers the process-wide worktree enricher
// that instance loading consults (stopped by session.StopWorktreeEnricher). Any
// previously registered enricher is stopped first.
func installWorktreeEnricher(ctx context.Context, cfg instance.WorktreeEnricherConfig) *instance.WorktreeEnricher {
	instance.StopWorktreeEnricher()
	e := instance.NewWorktreeEnricher(cfg)
	e.Start(ctx)
	instance.SetWorktreeEnricher(e)
	return e
}

// deferToReconcile reports whether Step 6 must leave inst for Step 6b.
// Start(false) cannot take PermanentlyFailed/Failed -> Active, and its failure
// cleanup kills the session's live tmux/tymux session before 6b can adopt it.
// shouldBootStart reports whether the boot loop should Start(false) inst. Archived sessions are
// deliberately retired and never auto-started (ADR-001, superseded-rework-session-retirement).
// Re-evaluated after the boot stagger sleep because the answer can change while it sleeps.
func shouldBootStart(inst *instance.Instance) bool {
	return inst != nil && !inst.IsArchived() && !inst.Started() && !deferToReconcile(inst)
}

func deferToReconcile(inst *instance.Instance) bool {
	switch inst.GetLifecycleStatus() {
	case domain.PermanentlyFailed, domain.Failed:
		return true
	default:
		return false
	}
}
