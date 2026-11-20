package server

import (
	"context"
	"time"

	githubpkg "github.com/tstapler/stapler-squad/github"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/services/backlogflow"
	"github.com/tstapler/stapler-squad/server/services/backlogsvc"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/backlog"
	"github.com/tstapler/stapler-squad/session/backlog/liveness"
	"github.com/tstapler/stapler-squad/session/backlog/pipeline"
	"github.com/tstapler/stapler-squad/session/flow"
	"github.com/tstapler/stapler-squad/session/scrollback"
	"github.com/tstapler/stapler-squad/session/stagecfg"
	"github.com/tstapler/stapler-squad/session/tokens"
	"github.com/tstapler/stapler-squad/session/unfinished"
	"github.com/tstapler/stapler-squad/session/workflowengine"
)

// backlogEngines are the ent-backed pipeline, liveness and stage-config collaborators;
// each is nil when storage is not ent-backed.
type backlogEngines struct {
	pipelineEngine       pipeline.PipelineEngine
	pipelineModeRepo     stagecfg.PipelineModeRepository
	livenessRepo         liveness.LivenessRepository
	livenessEngine       liveness.LivenessEngine
	stageCRUDRepo        stagecfg.StageCRUDRepository
	stageConfigEngine    *workflowengine.ConfiguredWorkflowEngine
	gateSatisfactionRepo stagecfg.GateSatisfactionRepository
}

// buildBacklogEngines constructs the engines and repositories BacklogService and the
// lifecycle listener share. Non-fatal throughout: degraded engines stay nil.
func buildBacklogEngines(storage *session.Storage) backlogEngines {
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
	var pipelineEngine pipeline.PipelineEngine
	var pipelineModeRepo stagecfg.PipelineModeRepository
	if entClient := storage.GetEntClient(); entClient != nil {
		// Seed the built-in 9-stage/edge workflow graph (backlog_stages +
		// stage_transitions rows) so ConfiguredWorkflowEngine (Epic 2.3) has
		// real rows to load from day one once it's wired in. Create-if-missing
		// only — never touches rows once any backlog_stages row exists — and
		// never aborts boot on failure, matching the "sdd" pipeline mode seed's
		// posture immediately below (project_plans/backlog-custom-workflow-stages/
		// implementation/plan.md Epic 2.2).
		if seedErr := workflowengine.EnsureBuiltInWorkflowStages(context.Background(), entClient); seedErr != nil {
			log.Warn("failed to seed built-in workflow stages, continuing without them", "err", seedErr)
		}
		pipelineModeRepo = stagecfg.NewEntPipelineModeRepository(entClient)
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
		if cachingPipelineEngine, err := pipeline.NewPipelineEngine(pipelineModeRepo); err != nil {
			log.Warn("pipelineEngine construction failed; continuing with the default pipeline for all backlog items", "err", err)
		} else {
			pipelineEngine = cachingPipelineEngine
		}
	} else {
		log.Warn("pipelineEngine unavailable: storage is not ent-backed; continuing with the default pipeline for all backlog items")
	}

	// LivenessEngine resolves a backlog stage's stuck-detection threshold
	// (Epic 1.3 of backlog-custom-workflow-stages). Same non-fatal-fallback
	// posture as pipelineEngine immediately above: construction never aborts
	// boot, and a nil livenessEngine/livenessRepo (non-ent-backed storage, or
	// construction failure) makes every consumer fall back to
	// DefaultLivenessEngine's hardcoded constants — behavior identical to
	// pre-Epic-1.3 today. livenessRepo is wired into BacklogService via a
	// post-construction setter (SetLivenessRepository/SetLivenessEngine
	// below, near backlogSvc's construction) rather than as a
	// NewBacklogService constructor parameter, to avoid touching every
	// existing NewBacklogService test call site for a dependency this epic
	// introduces.
	var livenessRepo liveness.LivenessRepository
	var livenessEngine liveness.LivenessEngine
	if entClient := storage.GetEntClient(); entClient != nil {
		livenessRepo = liveness.NewEntLivenessRepository(entClient)
		if cachingLivenessEngine, err := liveness.NewCachingLivenessEngine(livenessRepo); err != nil {
			log.Warn("livenessEngine construction failed; continuing with DefaultLivenessEngine's built-in thresholds", "err", err)
		} else {
			livenessEngine = cachingLivenessEngine
		}
	} else {
		log.Warn("livenessEngine unavailable: storage is not ent-backed; continuing with DefaultLivenessEngine's built-in thresholds")
	}

	// stageCRUDRepo/stageConfigEngine back the Stage/StageTransition/
	// TransitionGate CRUD RPCs (Epic 2.7 of backlog-custom-workflow-stages).
	// stageConfigEngine here is used only for its InvalidateCache method —
	// those write handlers' downstream cache-invalidation target — not as
	// the active session.WorkflowEngine (the workflowEngine selector below is);
	// switching runtime transition/gate evaluation over to
	// ConfiguredWorkflowEngine is a later epic's job. Same non-fatal-fallback
	// posture as pipelineEngine/livenessEngine above.
	var stageCRUDRepo stagecfg.StageCRUDRepository
	var stageConfigEngine *workflowengine.ConfiguredWorkflowEngine
	var gateSatisfactionRepo stagecfg.GateSatisfactionRepository
	if entClient := storage.GetEntClient(); entClient != nil {
		entStageRepo := stagecfg.NewEntStageConfigRepository(entClient)
		stageCRUDRepo = entStageRepo
		entGateSatisfactionRepo := stagecfg.NewEntGateSatisfactionRepository(entClient)
		gateSatisfactionRepo = entGateSatisfactionRepo
		if engine, err := workflowengine.NewConfiguredWorkflowEngine(entStageRepo, entGateSatisfactionRepo, pipelineModeRepo); err != nil {
			log.Warn("stageConfigEngine construction failed; stage/transition/gate CRUD writes will not invalidate a cache", "err", err)
		} else {
			stageConfigEngine = engine
		}
	} else {
		log.Warn("stageCRUDRepo unavailable: storage is not ent-backed; Stage/StageTransition/TransitionGate CRUD RPCs will return CodeUnavailable")
	}

	return backlogEngines{
		pipelineEngine:       pipelineEngine,
		pipelineModeRepo:     pipelineModeRepo,
		livenessRepo:         livenessRepo,
		livenessEngine:       livenessEngine,
		stageCRUDRepo:        stageCRUDRepo,
		stageConfigEngine:    stageConfigEngine,
		gateSatisfactionRepo: gateSatisfactionRepo,
	}
}

// newBacklogLifecycleListener builds the always-created lifecycle listener with the
// pool and engines passed at construction, closing the late-SetHeadlessPool race.
func newBacklogLifecycleListener(svc *ServiceDeps, llm llmWiring, engines backlogEngines) *backlog.BacklogLifecycleListener {
	storage, eventBus, sessionService := svc.Storage, svc.EventBus, svc.SessionService
	headlessPool, llmClient := llm.pool, llm.client
	pipelineEngine, livenessEngine := engines.pipelineEngine, engines.livenessEngine
	// Backlog lifecycle listener — always created, enabled state set from config below.
	// The pool is passed at construction time to close the race window that existed when
	// SetHeadlessPool was called hundreds of lines after instance wiring.
	backlogLifecycleListener := backlog.NewBacklogLifecycleListenerWithPool(storage, headlessPool, pipelineEngine, livenessEngine)
	backlogLifecycleListener.SetNotifier(&services.EventBusNotifier{Bus: eventBus})
	if llmClient != nil {
		backlogLifecycleListener.SetHeadlessClient(llmClient)
	}
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

	return backlogLifecycleListener
}

// backlogControl is the backlog controller, the quota gate that can disable it, and the
// tracker plugin registry and key source the manual TriggerSync path shares.
type backlogControl struct {
	ctrl      *session.BacklogController
	quotaGate *services.QuotaGate
	registry  *session.PluginRegistry
	keyFunc   func() ([]byte, error)
}

// startBacklogControl builds the BacklogController, applies the boot-time enabled state,
// reconciles the quota gate once synchronously, and starts the 60s reconcile ticker.
func startBacklogControl(svc *ServiceDeps, cfg *config.Config, backlogLifecycleListener *backlog.BacklogLifecycleListener,
	workflowEngine *WorkflowEngineWiring, tokenStore *tokens.TokenStore) backlogControl {
	storage, eventBus, sessionService := svc.Storage, svc.EventBus, svc.SessionService
	// Build the BacklogController and initialize its enabled state from config.
	syncRegistry := NewItemSourceRegistry()
	var keyFunc func() ([]byte, error)
	if cfg != nil {
		keyFunc = cfg.GetOrCreateEncryptionKey
	}
	backlogCtrl := session.NewBacklogController(backlogLifecycleListener, storage, syncRegistry, keyFunc)
	backlogCtrl.SetWorkflowEngine(workflowEngine.Engine)
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
	services.LogQuotaEffectiveConfig("Using quota gate config", config.LoadConfig().Quota)
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
			recoverAndLog("wake reconcile ticker", func() { runWakeReconcile(ctx) })
		}
	}()

	return backlogControl{ctrl: backlogCtrl, quotaGate: quotaGate, registry: syncRegistry, keyFunc: keyFunc}
}

// buildBacklogService constructs BacklogService and the flow service and wires the
// service-side setters (engines, repos, runners, sync gates).
func buildBacklogService(svc *ServiceDeps, cfg *config.Config, backlogLifecycleListener *backlog.BacklogLifecycleListener,
	engines backlogEngines, workflowEngine *WorkflowEngineWiring, llm llmWiring, scrollbackManager *scrollback.ScrollbackManager,
	control backlogControl, unfinishedScanner *unfinished.Scanner) (*backlogsvc.BacklogService, *backlogflow.BacklogFlowService, error) {
	storage, eventBus, sessionService := svc.Storage, svc.EventBus, svc.SessionService
	headlessPool, llmSelector := llm.pool, llm.selector
	pipelineEngine, pipelineModeRepo, livenessRepo, livenessEngine := engines.pipelineEngine, engines.pipelineModeRepo, engines.livenessRepo, engines.livenessEngine
	stageCRUDRepo, stageConfigEngine, gateSatisfactionRepo := engines.stageCRUDRepo, engines.stageConfigEngine, engines.gateSatisfactionRepo
	backlogCtrl, quotaGate, syncRegistry, keyFunc := control.ctrl, control.quotaGate, control.registry, control.keyFunc
	backlogSvc := backlogsvc.NewBacklogService(storage, sessionService, cfg, workflowEngine.Engine, pipelineEngine, pipelineModeRepo)
	backlogSvc.SetFlagRegistry(sessionService.InjectedFlagRegistry())
	// One flow reader, aggregator and service for both the RPC handler and the HTTP MCP core.
	backlogFlowSvc := backlogflow.NewFlowServiceFromStorage(storage, flow.WithObserver(backlogflow.NewFlowObserver()))
	backlogFlowSvc.SetEnabledCheck(backlogCtrl.IsEnabled)
	if gateErr := workflowEngine.InjectTransitionGate(storage, backlogSvc.TransitionDeniedReporter()); gateErr != nil {
		return nil, nil, gateErr
	}
	backlogSvc.SetLivenessRepository(livenessRepo)
	backlogSvc.SetRepoSettingsLookup(githubpkg.RepoSettings)
	backlogSvc.SetRepoSettingsNonBlockingLookup(githubpkg.RepoSettingsNonBlocking)
	backlogSvc.SetWorkflowEngineStatusSource(workflowEngine.Status)
	backlogLifecycleListener.SetRepoSettingsSeam(backlog.RepoSettingsSeam{Cached: githubpkg.RepoSettingsCached, Fetch: githubpkg.RepoSettings})
	backlogSvc.SetLivenessEngine(livenessEngine)
	backlogSvc.SetStageCRUDRepository(stageCRUDRepo)
	backlogSvc.SetGateSatisfactionRepository(gateSatisfactionRepo)
	// Guarded rather than an unconditional Set: stageConfigEngine is a
	// *session.ConfiguredWorkflowEngine, and passing a nil one straight into
	// SetStageConfigEngine's interface parameter would box it as a
	// non-nil-interface-wrapping-nil-pointer — breaking every
	// s.stageConfigEngine == nil guard downstream (same typed-nil pitfall
	// pipelineEngine's doc comment above describes).
	if stageConfigEngine != nil {
		backlogSvc.SetStageConfigEngine(stageConfigEngine)
	}
	backlogSvc.SetEventBus(eventBus)
	backlogSvc.SetSessionStopper(sessionService)
	backlogSvc.SetSessionSteerer(sessionService)
	backlogSvc.StartVerdictSteering()
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
		backlogSvc.SetBackendSelector(llmSelector)
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

	return backlogSvc, backlogFlowSvc, nil
}

// wireBacklogLifecycle connects the lifecycle listener, BacklogService, SessionService
// and the PR poller to each other (auto-reopen, respawn, archival, live-work probe).
func wireBacklogLifecycle(svc *ServiceDeps, cfg *config.Config, backlogLifecycleListener *backlog.BacklogLifecycleListener,
	backlogSvc *backlogsvc.BacklogService, engines backlogEngines, workflowEngine *WorkflowEngineWiring, control backlogControl) error {
	storage, eventBus, sessionService := svc.Storage, svc.EventBus, svc.SessionService
	stageConfigEngine, gateSatisfactionRepo := engines.stageConfigEngine, engines.gateSatisfactionRepo
	backlogCtrl, quotaGate := control.ctrl, control.quotaGate
	backlogLifecycleListener.SetAutoReopener(backlogSvc)
	backlogLifecycleListener.SetPRFixSpawner(backlogSvc)
	backlogLifecycleListener.SetReviewRespawner(backlogSvc)
	backlogSvc.SetReviewSpawnGuard(backlogLifecycleListener.ReviewSpawnGuard())
	// Wires reconcileCustomGateChecks' scan for overdue custom-check
	// invocations (Epic 2.4, Task 2.4.4c) — same gateSatisfactionRepo instance
	// already wired into backlogSvc above, guarded nil-safe by both consumers.
	backlogLifecycleListener.SetGateSatisfactionRepository(gateSatisfactionRepo)
	// Wires the underlying ReviewGateRunner's own GateSatisfactionRepository
	// (Epic 2.4 follow-up) so a configured automated_review gate's terminal
	// verdict is actually recorded, not just this listener's separate
	// reconcile-sweep copy above.
	backlogLifecycleListener.SetReviewGateSatisfactionRepository(gateSatisfactionRepo)
	// Wires resolveReviewGateContext/resolveCustomCheckGateContext's
	// ConfiguredWorkflowEngine consultation (Epic 2.4 follow-up) — without
	// this, a custom transition's automated-review/custom-check gates can
	// never fire, degrading to the built-in `to == BacklogStatusReview`
	// literal only. Guarded the same way backlogSvc.SetStageConfigEngine is
	// above: stageConfigEngine is a concrete *session.ConfiguredWorkflowEngine,
	// and passing a nil one through SetWorkflowEngine's interface parameter
	// would box it as a non-nil-interface-wrapping-nil-pointer. Never pass the
	// workflowEngine.Engine selector here: the listener type-asserts the concrete
	// engine, and the selector would silently disable gate-context resolution.
	if stageConfigEngine != nil {
		backlogLifecycleListener.SetWorkflowEngine(stageConfigEngine)
	}
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
	// Wire the worktree cleaner so internal transition paths that drive an
	// item straight to done (bounce-to-done, PR-merge/superseded-by-main
	// detection) trigger the same synchronous git-worktree cleanup +
	// session archival the manual TransitionBacklogItemStatus RPC already
	// runs inline, instead of relying solely on the 60s
	// reconcileTerminalItemSessions safety-net sweep.
	backlogLifecycleListener.SetWorktreeCleaner(backlogSvc)
	// One live-work probe for the lifecycle listener (merge hold), BacklogService
	// (RULE L) and the cleanup gate; a missed site would silently revert to
	// kill-at-done, so the wiring is asserted and exported as a gauge.
	if probeErr := wireLiveWorkProbe(backlogLifecycleListener, backlogSvc, sessionService, storage); probeErr != nil {
		return probeErr
	}
	backlogLifecycleListener.SetPRRowRefresher(svc.PRStatusPoller)
	sessionService.SetItemCleaner(backlogSvc)
	// Prompt cleanup when a done/archived item's last PR goes terminal (bounded worker;
	// the 60s reconcileTerminalItemWorktrees sweep remains the backstop).
	svc.PRStatusPoller.SetPRTerminalListener(backlogLifecycleListener)
	svc.PRStatusPoller.SetOnPRUnknown(newPRUnknownNotifier(storage, &services.EventBusNotifier{Bus: eventBus}))
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
	backlogSvc.SetStaleWorkInspector(sessionService.StaleWorkInspector())
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
	workflowEngine.RegisterStatusDetail(sessionService)
	// Read-only visibility for the restart-with-handoff-summary feature (see
	// services.HandoffSummaryFeatureController's doc comment): lets the frontend
	// discover config.json's handoff_summary.enabled up front via GetFeatureFlags,
	// instead of only finding out disabled on the first TriggerHandoffSummary
	// call's Code.FailedPrecondition.
	sessionService.SetFeatureController("handoff-summary", services.HandoffSummaryFeatureController{})
	return nil
}
