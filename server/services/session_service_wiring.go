package services

import (
	"context"
	"fmt"
	"time"

	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/classifier"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/notifications"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/headless"
	"github.com/tstapler/stapler-squad/session/sshremote"
)

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

// GetTaggingEngine returns the live TaggingEngine (the same instance taggingRulesSvc
// mutates on CRUD) for wiring up SessionTagClassificationPoller (session-classifier-pipeline
// Epic 4.4) — nil-safe like GetClassifier, though taggingEngine is currently always
// constructed alongside the SessionService, unlike rulesSvc which can be nil in some paths.
func (s *SessionService) GetTaggingEngine() *classifier.TaggingEngine {
	return s.taggingEngine
}

// GetAnalyticsStore returns the analytics store for wiring up the ApprovalHandler.
func (s *SessionService) GetAnalyticsStore() *AnalyticsStore {
	if s.rulesSvc == nil {
		return nil
	}
	return s.rulesSvc.analyticsStore
}

// GetClaudeSettingsWatcher returns the claude-settings file watcher for starting/stopping it
// with the server lifecycle (see wireDepsIntoServer).
func (s *SessionService) GetClaudeSettingsWatcher() *ClaudeSettingsWatcher {
	if s.rulesSvc == nil {
		return nil
	}
	return s.rulesSvc.claudeSettingsWatcher
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

// waitForPendingCleanup blocks until every DeleteSession call made so far has
// finished its background tmux/instance cleanup (see trackCleanup). Unlike
// Shutdown, it doesn't stop accepting new cleanup work, so it's safe to call
// mid-test. Tests that set HOME/config dirs to a t.TempDir() and then delete a
// session must call this before returning: t.Cleanup runs LIFO, so the
// TempDir's RemoveAll (registered after the service, inside the test body)
// would otherwise race DeleteSession's async Destroy() for files under HOME
// (e.g. the tmux exec-gate directory) that only Shutdown's Wait would
// otherwise catch, and Shutdown itself only runs after TempDir cleanup in
// that ordering.
func (s *SessionService) waitForPendingCleanup() {
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
	inst.SetNotifier(&EventBusNotifier{Bus: s.eventBus})
	if s.statusManager != nil {
		inst.SetStatusManager(s.statusManager)
	}
	s.wireRateLimitCallbacks(inst.Instance)
	s.wireStatusChangeCallback(inst.Instance)
	s.wireClaudeSessionIDCallback(inst.Instance)
	s.wireAutoArchiveCallback(inst.Instance)
	s.wireSessionExitedPublisher(inst.Instance)
	// A provider, not a one-shot backfill: buildClaudeCommand re-resolves this
	// on every claude launch (session/instance_tmux.go), so a relaunch
	// (workspace switch, crash/hibernate resume) always observes the current
	// server address instead of staying permanently stuck at whatever
	// MCPServerURL happened to be at construction time.
	inst.SetMCPServerURLProvider(s.resolveMCPServerURL)
}

// SetBacklogLifecycleListener wires the listener to all sessions created via
// CreateDirectorySession so that backlog state transitions fire on session exit.
func (s *SessionService) SetBacklogLifecycleListener(l *session.BacklogLifecycleListener) {
	s.backlogLifecycleListener = l
	s.prCreationSvc.SetBacklogLifecycleListener(l)
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

// SetHistoryLinker wires the HistoryLinker so deleted sessions are also removed
// from it and cannot be re-persisted by the shutdown hook.
func (s *SessionService) SetHistoryLinker(hl *session.HistoryLinker) {
	s.historyLinker = hl
}

// SetSessionTagPoller wires the SessionTagClassificationPoller so sessions created
// after server startup are added to it (previously only the boot-time instance list
// ever reached it via a one-shot SetInstances call — see server/dependencies.go).
// Must be called during server startup before any session-creation RPCs are used.
func (s *SessionService) SetSessionTagPoller(poller *session.SessionTagClassificationPoller) {
	s.sessionTagPoller = poller
}

// SetHeadlessPool wires the headless LLM pool for use by RunOneShot and other AI features.
func (s *SessionService) SetHeadlessPool(pool *headless.Pool) {
	s.headlessPool = pool
	s.autonomousSvc.SetPool(pool)
	s.prCreationSvc.SetHeadlessPool(pool)
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
	// Tagging engine + fire-count recorder (session-classifier-pipeline Epic 3.3/Task
	// 2.3.3d) — wired here, the single per-instance callback chokepoint every construction
	// path (fresh CreateSession, CreateDirectorySession/CreateWorktreeSession, and every
	// instance loaded at startup via loadInstancesWithWiring) already calls, so
	// reclassifyTagsLocked has a real engine/recorder on every session, not just some paths.
	inst.SetTaggingEngine(s.taggingEngine)
	// Pre-spawn worktree-collision guards: wired here, before every Start() call
	// this codebase makes, so startLocked's firstTimeSetup branch never spawns
	// into a directory another live session already owns, and two concurrent
	// CreateSession calls resolving the same worktree path can't both win the race.
	inst.SetWorktreeSpawnReservation(func(worktreePath string) (func(), error) {
		cleanTarget, err := canonicalizeAbsPath(worktreePath)
		if err != nil {
			// filepath.Abs only fails if os.Getwd() fails -- vanishingly rare, but
			// silently skipping here would leave the TOCTOU-closing reservation
			// unclaimed with no trace, so warn instead. Returns a no-op release
			// (not nil, nil) so success always carries a real, callable release value.
			log.Warn("SetWorktreeSpawnReservation: failed to resolve worktree path, spawn reservation skipped", "session", inst.Title, "path", worktreePath, "err", err)
			return func() {}, nil
		}
		if _, loaded := s.inFlightWorktreeSpawns.LoadOrStore(cleanTarget, inst.UUID); loaded {
			return nil, fmt.Errorf("%w: claimed by an in-flight spawn", session.ErrDirectoryCollision)
		}
		return func() { s.inFlightWorktreeSpawns.Delete(cleanTarget) }, nil
	})
	inst.SetPreSpawnCollisionGuard(func(worktreePath string) error {
		if s.reviewQueuePoller == nil {
			log.Warn("SetPreSpawnCollisionGuard: reviewQueuePoller is nil, allowing spawn without a collision check", "session", inst.Title)
			return nil
		}
		blockingUUID, blocked := s.OtherLiveSessionInsideWorktree(inst.UUID, worktreePath)
		if !blocked {
			return nil
		}
		return fmt.Errorf("%w: blocked by session %s", session.ErrDirectoryCollision, blockingUUID)
	})
	// Cross-session conversation-ownership guard (worktree-envvars-hijack Story 1.4.2):
	// prevents tryExtractConversationUUID's DetectByPath fallback from silently
	// attributing another live session's conversation UUID to this instance.
	inst.SetConversationOwnershipGuard(func(candidateUUID, path string) (string, bool) {
		return s.ConversationOwnedByOtherLiveSession(conversationOwnershipQuery{
			selfUUID:         inst.UUID,
			conversationUUID: candidateUUID,
			path:             path,
		})
	})
	if analyticsStore := s.GetAnalyticsStore(); analyticsStore != nil {
		// Guard against the typed-nil-interface gotcha: passing a nil *AnalyticsStore
		// straight into the session.TagFireRecorder interface parameter would make
		// inst.tagFireRecorder != nil true even though the underlying pointer is nil,
		// and RecordTaggingRuleFire is not nil-receiver-safe.
		inst.SetTagFireRecorder(analyticsStore)
	}
	s.wireRateLimitCallbacks(inst)
	s.wireStatusChangeCallback(inst)
	s.wireClaudeSessionIDCallback(inst)
	s.wireAutoArchiveCallback(inst)
	s.wireSessionExitedPublisher(inst)
	s.wireColdRestoreOutcomeListener(inst)
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
	// Wired here (not a one-shot MCPServerURL field) so every caller of this
	// chokepoint -- CreateSession, CreateDirectorySession, CreateWorktreeSession,
	// loadInstancesWithWiring -- re-resolves the MCP URL fresh on every claude
	// relaunch instead of staying stuck at whatever it was at construction time.
	inst.SetMCPServerURLProvider(s.resolveMCPServerURL)
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
	s.notificationSvc.SetStorage(s.storage)
	s.utilitySvc.SetReviewQueuePoller(poller)
	s.checkpointSvc.SetPoller(poller)
	s.terminalSvc.SetPoller(poller)
	if s.workflowSvc != nil {
		s.workflowSvc.SetPoller(poller)
	}
}

// SetRemoteDeps wires the SSH-identity and host-key-trust stores CreateSession's
// remote-target mode-specific block needs (ssh-remote-workspaces Phase 4, Epic
// 4.2). server.go passes the same *sshremote.KeyStore/*sshremote.KnownHostsStore
// instances used to construct RemoteService, so a remote trusted/given an
// identity via Settings' TOFU flow is immediately usable by CreateSession with
// no separate wiring step. Until called, CreateSession rejects any request
// naming a remote with CodeFailedPrecondition.
func (s *SessionService) SetRemoteDeps(keyStore *sshremote.KeyStore, knownHosts *sshremote.KnownHostsStore) {
	s.remoteKeyStore = keyStore
	s.remoteKnownHosts = knownHosts
}

// SetPermissionRequestHandler wires the handler every remote session's
// *sshremote.RemoteApprovalRelay drives its PermissionRequest hook payloads
// through (ssh-remote-workspaces Phase 5 correction). server.go passes the
// same *ApprovalHandler registered at the local `/api/hooks/permission-request`
// HTTP endpoint, mirroring SetRemoteDeps's pattern of injecting a
// server.go-constructed dependency rather than SessionService owning its
// own separate instance. Must be called during server startup before any
// remote session is created; setupRemoteApprovalHooks degrades to a
// non-fatal logged error (not a panic) if a remote session is created
// before this is wired.
func (s *SessionService) SetPermissionRequestHandler(handler sshremote.PermissionRequestHandler) {
	s.permissionRequestHandler = handler
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

// SetSlackNotifier rewires slackConfigSvc onto the given SlackNotifier
// instance — intended for server/dependencies.go to call with the SAME
// *services.SlackNotifier wired into ReactiveQueueManager/ApprovalHandler,
// so GetSlackConfig's last_delivery reflects real production sends from
// those trigger points, not only sends made via TestSlackWebhook. Optional:
// if never called, slackConfigSvc keeps the private SlackNotifier instance
// NewSessionService constructed for it.
func (s *SessionService) SetSlackNotifier(n *SlackNotifier) {
	s.slackConfigSvc = NewSlackConfigService(n)
}

// SlackNotifierForTest returns the SlackNotifier instance currently wired
// into slackConfigSvc. Exported only so cross-package wiring regression
// tests (server package) can assert pointer identity against the other
// consumers (ReactiveQueueManager, ApprovalHandler) without restructuring
// production code — not intended for any non-test caller.
func (s *SessionService) SlackNotifierForTest() *SlackNotifier {
	return s.slackConfigSvc.slackNotifier
}

// SetJulesConfigDependencies rewires julesConfigSvc onto the real Jules
// keychain/source-registry/poller dependencies once server/dependencies.go
// has constructed them (Task 2.4.4a). keys and sources are nil when Jules
// is disabled or its key is unresolvable at startup — julesConfigSvc stays
// nil-safe for both (UpdateJulesConfig's api_key path and TestJulesConnection
// then return CodeUnavailable rather than panicking). poller is nil unless
// the poller was actually started.
func (s *SessionService) SetJulesConfigDependencies(keys julesKeyManager, sources julesSourceResolver, poller julesAuthReconnectReporter) {
	s.julesConfigSvc = NewJulesConfigService(keys, sources, poller)
}

// SetJulesKeyringTokenSource rewires credChain's JulesCredentialSource onto
// the single process-wide *jules.KeyringTokenSource server/dependencies.go
// constructs (the same instance passed to SetJulesConfigDependencies and
// wired into the Jules client/poller), so credential-chain resolution never
// spins up a second, independent cache/circuit-breaker/singleflight-group
// over the same OS keychain entry. A no-op if credChain is nil (should not
// happen outside tests that bypass NewSessionServiceWithSearchEngine).
func (s *SessionService) SetJulesKeyringTokenSource(tokens julesKeyringTokenSource) {
	if s.credChain != nil {
		s.credChain.SetJulesTokenSource(tokens)
	}
}

// SetJulesUsageCounter wires the process-wide *JulesUsageCounter (Task
// 4.1.1a) onto julesConfigSvc so GetJulesConfig's response carries a live
// usage snapshot. Constructed unconditionally in server/dependencies.go
// (cheap, no I/O) regardless of whether Jules itself is enabled, so this
// should be called once at startup alongside SetJulesConfigDependencies.
func (s *SessionService) SetJulesUsageCounter(usage julesUsageSnapshotter) {
	s.julesConfigSvc.SetUsageCounter(usage)
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
