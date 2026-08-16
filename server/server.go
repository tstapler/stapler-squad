package server

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/log"
	pkganalytics "github.com/tstapler/stapler-squad/pkg/analytics"
	"github.com/tstapler/stapler-squad/server/analytics"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/handlers"
	"github.com/tstapler/stapler-squad/server/interceptors"
	servermcp "github.com/tstapler/stapler-squad/server/mcp"
	"github.com/tstapler/stapler-squad/server/middleware"
	"github.com/tstapler/stapler-squad/server/notifications"
	"github.com/tstapler/stapler-squad/server/push"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/web"
	"github.com/tstapler/stapler-squad/server/workflows"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/memory"
	"github.com/tstapler/stapler-squad/session/tmux"

	"github.com/google/uuid"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/otelconnect"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Server manages the HTTP server with ConnectRPC handlers.
type Server struct {
	// addr holds the listen address. It starts as the requested address (e.g.
	// "localhost:0") and is overwritten with the real OS-assigned address once
	// Start()'s listener goroutine binds — read via GetAddr() from other
	// goroutines (lazy hook/MCP URL closures, tests), so it must be atomic.
	addr              atomic.Pointer[string]
	httpServer        *http.Server
	mux               *http.ServeMux
	tlsConfig         *tls.Config                     // non-nil when TLS is enabled
	authMiddleware    func(http.Handler) http.Handler // nil when auth is disabled
	httpsURL          string                          // set when remote access is enabled
	hostnames         []string                        // detected LAN hostnames
	origins           []string                        // allowed CORS origins
	shutdownHooks     []func()                        // called before HTTP server stops
	connCtxCancel     context.CancelFunc              // cancels BaseContext → closes active streams on shutdown
	availablePrograms []string                        // cached once at startup; programs change only on system changes
	startedAt         time.Time                       // set once in newServerBase; used to gate orphan notification pruning until instance data has had time to load
}

// newServerBase creates the base Server struct and returns it alongside the
// connection context that drives active-stream cancellation on shutdown.
// Both NewServer and NewServerWithDeps call this before wiring dependencies.
func newServerBase(addr string) (*Server, context.Context) {
	mux := http.NewServeMux()
	connCtx, connCtxCancel := context.WithCancel(context.Background())
	srv := &Server{
		mux:           mux,
		connCtxCancel: connCtxCancel,
		httpServer: &http.Server{
			Addr:         addr,
			Handler:      nil, // Set in Start() after middleware chain is built
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 0, // No write timeout — streaming connections are long-lived
			IdleTimeout:  60 * time.Second,
			// BaseContext is cancelled during Shutdown() so active streaming
			// connections (ConnectRPC terminal streams, SSE) see a done context
			// and self-close instead of blocking the graceful shutdown timeout.
			BaseContext: func(_ net.Listener) context.Context { return connCtx },
		},
	}
	srv.addr.Store(&addr)
	srv.startedAt = time.Now()
	return srv, connCtx
}

// NewServer creates a new HTTP server instance with SessionService registered.
//
// Initialization Order (dependencies flow downward):
//
//  1. SessionService      — creates Storage (Ent-backed), EventBus, ReviewQueue
//  2. StatusManager       — depends on nothing; created before instances load
//  3. ReviewQueuePoller   — depends on ReviewQueue, StatusManager, Storage
//  4. Instance wiring     — LoadInstances, SetReviewQueue + SetStatusManager on each
//  5. Instance.Start()    — starts tmux sessions; requires wired dependencies
//  6. Controller startup  — requires started instances and StatusManager
//  7. ReactiveQueueMgr    — depends on ReviewQueue, Poller, EventBus, StatusManager, Storage
//  8. ScrollbackManager   — independent; depends only on filesystem paths
//  9. TmuxStreamerManager — independent
//
// 11. ExternalDiscovery   — depends on Storage, ReviewQueue, StatusManager, Poller (via callbacks)
// 12. ExternalApprovalMonitor — depends on ExternalDiscovery
//
// Violating this order causes nil pointer panics or silent failures.
// Dependency construction is encapsulated in BuildDependencies (server/dependencies.go).
// See docs/tasks/architecture-refactor.md for the ongoing simplification plan.
func NewServer(addr string) *Server {
	srv, connCtx := newServerBase(addr)

	log.Info("Building server dependencies...")
	startTime := time.Now()
	deps, err := BuildDependencies()
	if err != nil {
		log.Error("Failed to build server dependencies", "err", err)
		// Continue without services — all RPC calls will return errors
	} else {
		log.Info("Server dependencies built", "elapsed", time.Since(startTime))
		wireDepsIntoServer(srv, deps, connCtx)
	}
	registerStaticRoutes(srv)
	return srv
}

// NewServerWithDeps creates a Server using pre-built dependencies.
// Use this when deps are constructed externally (e.g. via Warren lifecycle phases)
// so the build phases can be observed and timed independently.
func NewServerWithDeps(addr string, deps *ServerDependencies) *Server {
	srv, connCtx := newServerBase(addr)
	wireDepsIntoServer(srv, deps, connCtx)
	registerStaticRoutes(srv)
	return srv
}

// sessionHealthCheckInterval is how often SessionHealthChecker polls for dead
// tmux panes and stale sessions. With failureThreshold (session/health.go) at 2
// consecutive misses, a dead pane surfaces within ~2x this interval.
const sessionHealthCheckInterval = 15 * time.Second

// wireDepsIntoServer wires pre-built ServerDependencies into srv: starts background
// components, registers shutdown hooks, and mounts all ConnectRPC/HTTP handlers.
// serverCtx (== connCtx from newServerBase) is cancelled by Shutdown() to signal
// active streaming connections to close.
func wireDepsIntoServer(srv *Server, deps *ServerDependencies, serverCtx context.Context) {
	// Start background components
	go deps.ReactiveQueueMgr.Start(serverCtx)
	log.Info("ReactiveQueueManager started")

	deps.PRStatusPoller.Start(serverCtx)
	log.Info("PRStatusPoller started")

	// Start SessionHealthChecker: polls for dead tmux panes (remain-on-exit
	// placeholders left after the wrapped program exits) and stale
	// started-but-tmux-missing instances, marking dead panes Crashed/Stopped so
	// the UI can surface a banner instead of raw pane text. Previously this
	// checker was constructed only in tests and never actually started in
	// production -- see session/health.go.
	if deps.Storage != nil {
		healthChecker := session.NewSessionHealthChecker(deps.Storage)
		healthCheckerStop := make(chan struct{})
		go func() {
			<-serverCtx.Done()
			close(healthCheckerStop)
		}()
		go healthChecker.ScheduledHealthCheck(sessionHealthCheckInterval, healthCheckerStop)
		log.Info("SessionHealthChecker started", "interval", sessionHealthCheckInterval)
	}

	// Start HistoryLinker: detects Claude JSONL files and links conversation
	// UUIDs to sessions so cold restore can use --resume on restart.
	// Known startup race: the initial ScanAll in Start fires before the
	// background goroutine below has re-populated live tmux sessions, so the
	// proc_pidinfo open-file path will miss; recoverFromStaleResume is the
	// safety net for sessions that slip through.
	go deps.HistoryLinker.Start(serverCtx)
	log.Info("HistoryLinker started")

	// Start UnfinishedWork scanner.
	if deps.UnfinishedScanner != nil {
		deps.UnfinishedScanner.Start(serverCtx)
		log.Info("UnfinishedWork scanner started")
	}

	// Start WorktreePRPoller: enriches worktrees-without-sessions with GitHub PR data.
	if deps.WorktreePRPoller != nil {
		deps.WorktreePRPoller.Start(serverCtx)
		log.Info("WorktreePRPoller started")
	}

	// Register shutdown hook: capture pane working dirs and persist instance
	// state so cold restore can find the right directory on next start.
	// Uses HistoryLinker.Instances() (not the startup snapshot) so externally
	// discovered sessions added after startup are also captured.
	historyLinker := deps.HistoryLinker
	storage := deps.Storage
	srv.shutdownHooks = append(srv.shutdownHooks, func() {
		instances := historyLinker.Instances()
		deadline := time.Now().Add(4 * time.Second) // leave headroom for HTTP graceful shutdown
		captured := 0
		for _, inst := range instances {
			if time.Now().After(deadline) {
				log.Warn("[shutdown] Capture deadline exceeded; skipped instances", "skipped", len(instances)-captured, "total", len(instances))
				break
			}
			if err := inst.CaptureCurrentState(); err != nil {
				log.Warn("[shutdown] CaptureCurrentState failed", "session", inst.Title, "err", err)
			}
			captured++
		}
		if err := storage.SaveInstances(instances); err != nil {
			log.Warn("[shutdown] SaveInstances failed", "err", err)
		} else {
			log.Info("[shutdown] Persisted working dirs for instances", "count", captured)
		}
	})

	// Initialize notification history store and EventBus subscriber.
	// notifStore is declared here so it can be wired into the approval handler below.
	var notifStore *notifications.NotificationHistoryStore
	configDir, configErr := config.GetConfigDir()
	if configErr != nil {
		log.Error("Failed to get config dir for notification store", "err", configErr)
	} else {
		notifStorePath := filepath.Join(configDir, "notifications.json")
		var storeErr error
		notifStore, storeErr = notifications.NewNotificationHistoryStore(notifStorePath)
		if storeErr != nil {
			log.Error("Failed to create notification history store", "err", storeErr)
			notifStore = nil
		} else {
			// Wire into SessionService BEFORE subscribing to EventBus so any
			// in-flight event handler that calls GetNotificationStore() sees a
			// non-nil value even if the subscriber goroutine races ahead.
			deps.SessionService.SetNotificationStore(notifStore)
			notifications.StartSubscriber(serverCtx, deps.EventBus, notifStore)
			log.Info("NotificationHistoryStore initialized", "path", notifStorePath)

			// Wire the batch-fetch session-existence lookup used by the store's
			// orphan-pruning sweep (enforceRetention → pruneOrphanedRecords).
			// See buildSessionExistenceLookup for the uptime-gate rationale.
			notifStore.SetSessionExistenceLookup(buildSessionExistenceLookup(storage, srv.startedAt))

			// Wire the notification decision lister into SessionSummaryGenerator now
			// that notifStore exists — session/session_summary_service.go's
			// SetNotificationLister doc comment explains why this is late-bound rather
			// than passed at construction (dependencies.go builds the generator before
			// notifStore exists).
			if deps.SessionSummaryGenerator != nil {
				deps.SessionSummaryGenerator.SetNotificationLister(&notificationDecisionListerAdapter{store: notifStore})
			}
		}
	}

	// Initialize push notification service.
	if configErr == nil {
		pushService := services.NewPushService(configDir)
		pushHandler := services.NewPushHandler(pushService)
		pushHandler.RegisterRoutes(srv.mux)
		push.StartPushSubscriber(serverCtx, deps.EventBus, pushService)
		log.Info("Push notification service initialized")
	}

	// Wire fork pressure monitor → push notification + emergency reconcile.
	// Fires when capture-pane subprocess failures or zombie counts exceed thresholds,
	// indicating that dead sessions are flooding the poller with fork() calls.
	tmux.RegisterForkPressureAlert(func(level tmux.ForkPressureLevel, stats tmux.ForkPressureStats) {
		body := fmt.Sprintf(
			"Subprocess failures: %d/%ds | Spawns: %d/%ds | Zombies: %d | Level: %s",
			stats.FailuresInWindow, int(stats.WindowDuration.Seconds()),
			stats.SpawnsInWindow, int(stats.WindowDuration.Seconds()),
			stats.ZombiesInWindow, level,
		)
		notifType := int32(sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING)
		if level == tmux.ForkPressureCritical {
			notifType = int32(sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR)
		}
		event := events.NewNotificationEvent(
			"fork-pressure",
			"System",
			uuid.New().String(),
			notifType,
			int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH),
			fmt.Sprintf("Fork Pressure: %s", level),
			body,
			nil,
		)
		deps.EventBus.Publish(event)
		log.Warn("[ForkPressure] alert dispatched", "level", level, "body", body)

		// Immediately reconcile to mark dead sessions Stopped, cutting spawn rate.
		if deps.ReviewQueuePoller != nil {
			deps.ReviewQueuePoller.ForceReconcile()
		}
	})

	// Start fork pressure logger (logs stats every 30s when activity > 0).
	tmux.StartForkPressureLogger(serverCtx, 30*time.Second, func(format string, args ...any) {
		log.Info(fmt.Sprintf(format, args...))
	})

	// Become the subreaper for our process tree so that tmux's zombie children
	// get reparented to us (not init) when tmux hasn't yet reaped them.
	// No-op on macOS and Windows; effective on Linux only.
	if err := tmux.SetSubreaper(); err != nil {
		log.Warn("[zombie] SetSubreaper failed (non-fatal)", "err", err)
	} else {
		log.Info("[zombie] subreaper enabled: tmux descendant zombies will be reparented here")
	}

	// Start zombie watcher (scans for zombie child processes every 30s).
	tmux.StartZombieWatcher(serverCtx, 30*time.Second, func(format string, args ...any) {
		log.Warn(fmt.Sprintf(format, args...))
	})

	// Start zombie reaper (calls waitpid(-1, WNOHANG) every 60s to reap any
	// zombie children left by cmd.Start() paths that skipped cmd.Wait()).
	tmux.StartZombieReaper(serverCtx, 60*time.Second, func(format string, args ...any) {
		log.Info(fmt.Sprintf(format, args...))
	})

	// Wire tmux server recovery → web UI toast notification.
	tmux.SetServerRecoveryCallback(func() {
		event := events.NewNotificationEvent(
			"tmux-server",
			"System",
			uuid.New().String(),
			int32(sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING),
			int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM),
			"Tmux Server Recovered",
			"Connection to the tmux server has been restored. Sessions will resume automatically.",
			nil,
		)
		deps.EventBus.Publish(event)
		log.Info("[tmux] recovery notification sent to connected clients")
	})

	// Note: SetExternalDiscovery is now called inside BuildRuntimeDeps.

	// Start external session infrastructure
	deps.ExternalDiscovery.Start(5 * time.Second)
	deps.ExternalApprovalMonitor.Start()
	deps.ExternalApprovalMonitor.IntegrateWithDiscoveryTmux(deps.ExternalDiscovery, deps.TmuxStreamerManager)

	// Wire the shared streamer manager into SessionService so StopShell can evict a
	// shell's streamer on close, before it's handed to the WebSocket handler below.
	deps.SessionService.SetTmuxStreamerManager(deps.TmuxStreamerManager)

	// Register ConnectRPC WebSocket handler (must come before unary handler)
	wsHandler := services.NewConnectRPCWebSocketHandler(
		deps.SessionService, deps.ScrollbackManager, deps.TmuxStreamerManager,
	)
	wsPath := "/api" + sessionv1connect.SessionServiceStreamTerminalProcedure
	srv.mux.HandleFunc(wsPath, wsHandler.HandleWebSocket)
	log.Info("Registered ConnectRPC WebSocket handler", "path", wsPath)

	// Register VNC browser-passthrough WebSocket proxy.
	// Use ReviewQueuePoller (in-memory) rather than Storage (SQLite) for session lookup.
	vncProxy := services.NewVNCProxyHandler(deps.ReviewQueuePoller)
	srv.mux.HandleFunc("/api/sessions/{id}/vnc", vncProxy.HandleWebSocket)
	log.Info("Registered VNC WebSocket proxy at /api/sessions/{id}/vnc")

	// Register CDP browser-streaming WebSocket handler.
	// Use ReviewQueuePoller (in-memory) rather than Storage (SQLite) for session lookup.
	cdpStream := services.NewCDPStreamHandler(deps.ReviewQueuePoller)
	srv.mux.HandleFunc("/api/sessions/{id}/cdp-stream", cdpStream.HandleWebSocket)
	log.Info("Registered CDP stream WebSocket handler at /api/sessions/{id}/cdp-stream")

	// Register general ConnectRPC handler (unary calls)
	path, handler := sessionv1connect.NewSessionServiceHandler(deps.SessionService, ConnectOptions(deps.ErrorRegistry)...)
	apiPath := "/api" + path

	// Register StreamingWSBridge for server-streaming Watch* RPCs so browsers use
	// WebSocket instead of HTTP long-polling, avoiding the 6-connection-per-origin limit.
	// Exact-path registration takes priority over the prefix-registered general handler.
	wsBridge := services.NewStreamingWSBridge(handler)
	watchSessionsPath := "/api" + sessionv1connect.SessionServiceWatchSessionsProcedure
	watchReviewQueuePath := "/api" + sessionv1connect.SessionServiceWatchReviewQueueProcedure
	srv.mux.Handle(watchSessionsPath, wsBridge.Handler("/api"))
	srv.mux.Handle(watchReviewQueuePath, wsBridge.Handler("/api"))
	log.Info("Registered StreamingWSBridge", "watchSessions", watchSessionsPath, "watchReviewQueue", watchReviewQueuePath)

	srv.RegisterConnectHandler(apiPath, http.StripPrefix("/api", handler))

	// Register UnfinishedWorkService handler.
	if deps.UnfinishedWorkService != nil {
		uwPath, uwHandler := sessionv1connect.NewUnfinishedWorkServiceHandler(deps.UnfinishedWorkService, ConnectOptions(deps.ErrorRegistry)...)
		uwAPIPath := "/api" + uwPath
		srv.RegisterConnectHandler(uwAPIPath, http.StripPrefix("/api", uwHandler))
		log.Info("Registered UnfinishedWorkService handler", "path", uwAPIPath)
	}

	// Register SessionSummaryService handler.
	if deps.SessionSummaryGenerator != nil {
		sessionSummaryService := services.NewSessionSummaryService(deps.SessionSummaryGenerator, deps.SessionService)
		ssPath, ssHandler := sessionv1connect.NewSessionSummaryServiceHandler(sessionSummaryService, ConnectOptions(deps.ErrorRegistry)...)
		ssAPIPath := "/api" + ssPath
		srv.RegisterConnectHandler(ssAPIPath, http.StripPrefix("/api", ssHandler))
		log.Info("Registered SessionSummaryService handler", "path", ssAPIPath)
	}

	// Register InsightsService handler for token usage analytics.
	if deps.InsightsService != nil {
		insightsPath, insightsHandler := sessionv1connect.NewInsightsServiceHandler(deps.InsightsService, ConnectOptions(deps.ErrorRegistry)...)
		insightsAPIPath := "/api" + insightsPath
		srv.RegisterConnectHandler(insightsAPIPath, http.StripPrefix("/api", insightsHandler))
		log.Info("Registered InsightsService handler", "path", insightsAPIPath)
	}

	// Register GitHubUserService handler (GitHub Work Continuity feature).
	if deps.GitHubUserService != nil {
		ghPath, ghHandler := sessionv1connect.NewGitHubUserServiceHandler(deps.GitHubUserService, ConnectOptions(deps.ErrorRegistry)...)
		ghAPIPath := "/api" + ghPath
		srv.RegisterConnectHandler(ghAPIPath, http.StripPrefix("/api", ghHandler))
		log.Info("Registered GitHubUserService handler", "path", ghAPIPath)
	}

	// Register BacklogService handler.
	// The feature-flag interceptor is added on top of the standard options so that
	// all BacklogService RPCs return CodeNotFound when the "backlog" flag is off.
	// isEnabled re-reads config on every request so flag changes take effect immediately.
	if deps.BacklogService != nil {
		blOpts := append(
			ConnectOptions(deps.ErrorRegistry),
			connect.WithInterceptors(interceptors.NewFeatureFlagInterceptor("backlog", func() bool {
				return config.LoadConfig().GetFeatureFlag("backlog")
			})),
		)
		blPath, blHandler := sessionv1connect.NewBacklogServiceHandler(deps.BacklogService, blOpts...)
		blAPIPath := "/api" + blPath
		srv.RegisterConnectHandler(blAPIPath, http.StripPrefix("/api", blHandler))
		log.InfoLog.Printf("Registered BacklogService handler at %s", blAPIPath)
	}

	// Start UserPRCache and register GitHubUserService handler.
	if deps.UserPRCache != nil {
		deps.UserPRCache.Start(serverCtx)
		srv.shutdownHooks = append(srv.shutdownHooks, deps.UserPRCache.Stop)
		log.Info("UserPRCache started")
	}
	// Start WorkflowScheduler (nil guard: disabled when workflow repo is unavailable).
	if deps.WorkflowScheduler != nil {
		deps.WorkflowScheduler.Start(serverCtx)
		srv.shutdownHooks = append(srv.shutdownHooks, deps.WorkflowScheduler.Stop)
		log.Info("WorkflowScheduler started")
	}

	// Register BacklogService shutdown so in-flight triage goroutines are signalled
	// to stop acquiring new semaphore slots and existing calls can complete cleanly.
	if deps.BacklogService != nil {
		srv.shutdownHooks = append(srv.shutdownHooks, deps.BacklogService.Shutdown)
	}

	// Start workflow session retention enforcer (hourly sweep).
	// Requires both the session ent client and a workflow repository.
	if deps.WorkflowRepo != nil && deps.Storage != nil {
		if entClient := deps.Storage.GetEntClient(); entClient != nil {
			workflows.StartRetentionEnforcer(serverCtx, entClient, deps.WorkflowRepo, time.Hour)
			log.Info("WorkflowRetentionEnforcer started")
		}
	}

	// Register HeadlessService handler (nil guard: pool may be absent if claude not found).
	if deps.HeadlessPool != nil {
		hlSvc := services.NewHeadlessService(deps.HeadlessPool)
		hlPath, hlHandler := sessionv1connect.NewHeadlessServiceHandler(hlSvc, ConnectOptions(deps.ErrorRegistry)...)
		hlAPIPath := "/api" + hlPath
		srv.RegisterConnectHandler(hlAPIPath, http.StripPrefix("/api", hlHandler))
		log.Info("Registered HeadlessService handler", "path", hlAPIPath)
	}

	// Register ImportService handler (import-external-session, Phase 1).
	// Gated behind STAPLER_SQUAD_ENABLE_SESSION_IMPORT: only the three
	// mutating RPCs (CommitImportExternalSession, ConfirmKillExternalSession,
	// CancelPendingKill) return CodeUnimplemented when the flag is off, per
	// Story 1.3.2 / plan.md Task 0. PreviewImportExternalSession is
	// read-only (no persistence, no signaling) and must always be reachable
	// regardless of flag state, so it is deliberately excluded from the
	// gated method list -- gating the whole handler (as
	// NewFeatureFlagInterceptor would) incorrectly blocks Preview too. Uses
	// CodeUnimplemented rather than the CodeNotFound used by the
	// config-persisted "backlog" flag, since this is an unshipped/opt-in
	// capability rather than a togglable UI feature.
	suspendedStore, err := session.NewSuspendedProcessStore()
	if err != nil {
		log.Error("failed to create suspended process store, import-external-session disabled", "error", err)
	} else if err := session.ReconcileSuspendedProcesses(serverCtx, suspendedStore, deps.Storage); err != nil {
		log.Error("failed to reconcile suspended processes from a prior server incarnation", "error", err)
	}
	importSvc := services.NewImportServiceWithRealInspector(deps.Storage, deps.Registry, deps.HistoryLinker, suspendedStore)
	importOpts := append(
		ConnectOptions(deps.ErrorRegistry),
		connect.WithInterceptors(interceptors.NewScopedFeatureFlagInterceptor(
			"session_import",
			func() bool { return config.ImportSessionEnabled() },
			"CommitImportExternalSession",
			"ConfirmKillExternalSession",
			"CancelPendingKill",
		)),
	)
	importPath, importHandler := sessionv1connect.NewImportServiceHandler(importSvc, importOpts...)
	importAPIPath := "/api" + importPath
	srv.RegisterConnectHandler(importAPIPath, http.StripPrefix("/api", importHandler))
	log.Info("Registered ImportService handler", "path", importAPIPath)

	// Wire external session support into the unified WebSocket handler
	wsHandler.SetExternalSessionSupport(deps.ExternalDiscovery)
	log.Info("Unified WebSocket handler configured for external session support")

	// Register external approval endpoints
	externalWsHandler := services.NewExternalWebSocketHandler(
		deps.ExternalDiscovery,
		deps.TmuxStreamerManager,
		deps.ExternalApprovalMonitor,
		deps.EventBus,
	)
	srv.mux.HandleFunc("/api/external/approvals", externalWsHandler.HandleApprovals)
	srv.mux.HandleFunc("/api/external/approvals/respond", externalWsHandler.HandleApprovalResponse)
	log.Info("Registered External Session approval handlers at /api/external/approvals/*")

	// Shared lazy base-URL resolver for Claude Code hook callbacks (Epic 1.3, Story 1.3.1).
	// Read at the moment a hook URL is actually needed (per-session, at hook-injection time),
	// never snapshotted before Start() binds the real listen address -- so hook URLs resolve
	// correctly even when PORT=0 assigns an OS-chosen port. Mirrors the mcpURL lazy-read
	// pattern below (srv.GetAddr(), Task 1.1.1c).
	hookBaseURLFn := func() string { return "http://" + srv.GetAddr() }

	// Register Claude Code HTTP hook approval endpoint
	approvalHandler := services.NewApprovalHandler(
		deps.SessionService.GetApprovalStore(),
		deps.Storage,
		deps.EventBus,
	)
	// Wire the lazy base-URL resolver into the hook injector (hook_injector.go); both
	// InjectHookConfig's PermissionRequest URL and InjectHooksConfig's stop/pre-tool-use/
	// post-tool-use/prompt-submit endpoints resolve through this single shared mechanism.
	services.SetHookBaseURLFn(hookBaseURLFn)
	// Wire the review queue poller for immediate queue checks on new approvals (Story 3, Task 3.1)
	approvalHandler.SetQueueChecker(deps.ReviewQueuePoller)
	// Wire the classifier and analytics store for auto-approve/deny before manual review
	approvalHandler.SetClassifier(deps.SessionService.GetClassifier())
	approvalHandler.SetAnalyticsStore(deps.SessionService.GetAnalyticsStore())
	// Wire the domain age checker (enabled by default) for newly-registered domain escalation
	approvalHandler.SetDomainChecker(services.NewDomainAgeChecker(true))
	// Wire the notification stamper so approval outcomes persist across page refreshes
	if notifStore != nil {
		approvalHandler.SetNotificationStamper(notifStore)
		approvalHandler.SetAutoApprovalLogger(notifStore)
	}
	// Wire LLM approval for autonomous sessions (E5)
	if deps.HeadlessPool != nil {
		approvalHandler.SetHeadlessPool(deps.HeadlessPool)
	}
	approvalHandler.SetAutonomousChecker(func(sessionID string) bool {
		inst := deps.SessionService.FindLiveInstance(sessionID)
		return inst != nil && inst.AutonomousMode
	})
	// Thread the poller's live-configured interval into the CI-status staleness guard
	// (Task 1.1.2b) so it can't silently desync from the real poll interval.
	if deps.PRStatusPoller != nil {
		approvalHandler.SetPollInterval(deps.PRStatusPoller.PollInterval())
	}
	// CI status (GitHubCheckConclusion/LastPRStatusCheck) is not persisted — it only
	// lives on the poller's in-memory Instance — so the classifier's ci_passing
	// condition (Task 1.1.2a) must read through the live registry, not deps.Storage.
	approvalHandler.SetLiveInstanceFinder(deps.SessionService)
	srv.mux.HandleFunc("/api/hooks/permission-request", approvalHandler.HandlePermissionRequest)
	log.Info("Registered Claude Code hook approval handler at /api/hooks/permission-request")

	// Register non-approval hook receivers (stop, pre/post-tool-use, prompt-submit,
	// post-tool-use-drift-check — the BUG-044 follow-up steering hook, wired only
	// into autonomous backlog work sessions by spawnSessionAfterGates)
	hookReceiver := services.NewHookReceiver()
	hookReceiver.RegisterRoutes(srv.mux)
	log.Info("Registered Claude Code hook receivers at /api/hooks/{stop,pre-tool-use,post-tool-use,prompt-submit,post-tool-use-drift-check}")

	// Register inbound webhook-trigger receivers (webhook-triggers Epic 2.2/2.3) — like
	// the hook receivers just above, these are external-POST, verify-signature-first,
	// trust-boundary-adjacent routes, registered near /api/hooks/permission-request per
	// Task 2.2.1e. Both handlers self-gate on the "webhook_triggers" feature flag as
	// their own first line (defense in depth); nil-guarded here too so route
	// registration itself is skipped entirely when there's no workflow repository to
	// back it, mirroring WorkflowScheduler's own nil guard above.
	if deps.WorkflowRepo != nil && deps.TriggerFireEventRepo != nil {
		// config.LoadConfig() (not a threaded deps.Config — ServerDependencies has no
		// such field) matches this function's own established pattern for feature-flag
		// reads elsewhere (see the "backlog" flag check above and cfg := config.LoadConfig()
		// further down in this function).
		webhookCfg := config.LoadConfig()
		// Route registration itself is flag-gated (not just each handler's internal
		// first-line check) so that when webhook_triggers is off, /webhooks/* falls
		// through to the same catch-all behavior as any other undefined path, rather
		// than deterministically 404ing from a registered-but-disabled handler — the
		// latter is a discoverable "feature exists but disabled" signal to an
		// unauthenticated prober (plan.md Risk Control). This does mean flipping the
		// flag off requires a restart to stop serving these routes, same limitation the
		// "backlog" flag already has for its own route-gated pieces.
		if webhookCfg.GetFeatureFlag("webhook_triggers") {
			githubWebhookHandler := services.NewGitHubWebhookHandler(deps.WorkflowRepo, deps.WorkflowScheduler, deps.TriggerFireEventRepo, webhookCfg)
			githubWebhookHandler.RegisterRoutes(srv.mux)
			genericWebhookHandler := services.NewGenericWebhookHandler(deps.WorkflowRepo, deps.WorkflowScheduler, deps.TriggerFireEventRepo, webhookCfg)
			genericWebhookHandler.RegisterRoutes(srv.mux)
			log.Info("Registered webhook-trigger receivers at POST /webhooks/{github,{slug}}")
		} else {
			// Pre-mortem P2 #4: route registration is boot-time only, so an operator who
			// flips this flag on expecting /webhooks/* to work immediately otherwise gets
			// silent 404s from the mux with zero signal anywhere in the app (no
			// TriggerFireEvent row is ever created for a request to an unregistered
			// route). This log line at least makes the boot-time-only nature of the gate
			// visible in the service log.
			log.Info("webhook-trigger routes NOT registered (webhook_triggers flag off) — /webhooks/* will 404 until the flag is enabled and the service restarts")
		}
	}

	// Register session-aware image upload endpoint (multipart/form-data, saves to worktree).
	sessionUploadHandler := services.NewSessionImageUploadHandler(deps.Storage, deps.ReviewQueuePoller)
	srv.mux.HandleFunc("POST /api/v1/upload-image", sessionUploadHandler.HandleUpload)
	log.Info("Registered session image upload handler at POST /api/v1/upload-image")

	// Register MCP HTTP transport at /mcp so Claude sessions can connect
	// without spawning a subprocess. The URL is passed via --mcp-server to
	// claude when creating new sessions (no settings-file injection needed).
	//
	// deps.BacklogService is nil-guarded here (mirroring the other three
	// nil-checks on this same field in this function) rather than passed
	// directly: boxing a nil *services.BacklogService straight into the
	// session.AutoReopenSpawner interface parameter would produce a non-nil
	// interface value around a nil pointer — submitReviewVerdict's own
	// `h.autoReopener != nil` guard would then read true, and calling
	// AutoReopenAfterFailedReview would panic on the nil receiver instead of
	// being skipped.
	var autoReopener session.AutoReopenSpawner
	if deps.BacklogService != nil {
		autoReopener = deps.BacklogService
	}
	mcpHTTPHandler := servermcp.NewHTTPHandler(deps.Storage, deps.SessionService, deps.ScrollbackManager, deps.Storage, deps.EventBus, deps.UserPRCache, deps.BacklogEnabledCheck, autoReopener, deps.BacklogService)
	// Wrap with middleware that injects session UUID from X-Stapler-Session-UUID header.
	mcpWithUUID := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if uuid := r.Header.Get("X-Stapler-Session-UUID"); uuid != "" {
			r = r.WithContext(servermcp.WithSessionUUID(r.Context(), uuid))
		}
		mcpHTTPHandler.ServeHTTP(w, r)
	})
	srv.mux.Handle("/mcp", mcpWithUUID)
	srv.mux.Handle("/mcp/", mcpWithUUID)
	deps.SessionService.SetMCPServerURL(func() string { return "http://" + srv.GetAddr() + "/mcp" })
	log.Info("Registered MCP HTTP handler at /mcp", "url", "http://"+srv.GetAddr()+"/mcp (resolved lazily at session-creation time)")

	// Bind server lifecycle context so autonomous driver goroutines exit on shutdown.
	deps.SessionService.SetLifecycleContext(serverCtx)

	// Register SessionService shutdown so the AnalyticsStore flush goroutine
	// exits cleanly instead of leaking for the life of the process.
	srv.shutdownHooks = append(srv.shutdownHooks, deps.SessionService.Shutdown)

	// Start background expiration cleanup for pending approvals
	services.StartExpirationCleanup(context.Background(), deps.SessionService.GetApprovalStore())

	// Register Escape Code Analytics handler for debugging terminal rendering
	escapeCodeHandler := services.NewEscapeCodeHandler()
	escapeCodeHandler.RegisterRoutes(srv.mux)
	log.Info("Registered Escape Code Analytics handlers at /api/debug/escape-codes/*")

	// Register runtime log-level handler (used by the debug menu in the web UI)
	logLevelHandler := services.NewLogLevelHandler()
	logLevelHandler.RegisterRoutes(srv.mux)
	log.Info("Registered log-level handler at /api/debug/log-level")

	// Register Circuit Breaker debug handler for observability
	cbHandler := services.NewCircuitBreakerHandler()
	cbHandler.RegisterRoutes(srv.mux)
	log.Info("Registered Circuit Breaker debug handler at /api/debug/circuit-breakers")

	// Register the backlog debug seed endpoints ONLY for the e2e test server
	// (STAPLER_SQUAD_INSTANCE=e2e-local) — lets the Playwright suite seed
	// BacklogStuckState rows and queued backlog items directly, bypassing the
	// reconciler's real thresholds and the real WIP-cap spawn flow. Never
	// registered outside that instance.
	if os.Getenv("STAPLER_SQUAD_INSTANCE") == "e2e-local" && deps.Storage != nil {
		backlogSeedHandler := services.NewBacklogDebugSeedHandler(deps.Storage)
		backlogSeedHandler.RegisterRoutes(srv.mux)
		log.Info("Registered backlog debug seed handlers at /api/debug/backlog/seed-stuck, /api/debug/backlog/seed-queued, and /api/debug/backlog/seed-headless-triage-session (e2e-local only)")

		// Registered for project_plans/backlog-event-driven-updates's Playwright
		// e2e layer — lets tests mutate a backlog item directly through the
		// storage layer (create/transition/update/archive/delete), simulating a
		// second actor (reconciler, another tab) without walking a real
		// TransitionBacklogItemStatus RPC through its engine/gate checks.
		backlogMutateHandler := services.NewBacklogDebugMutateHandler(deps.Storage)
		backlogMutateHandler.RegisterRoutes(srv.mux)
		log.Info("Registered backlog debug mutate handlers at /api/debug/backlog/mutate-* (e2e-local only)")
	}

	// Wire analytics provider: SQLite when DB client is available, log-only fallback otherwise.
	var analyticsProvider analytics.AnalyticsProvider
	if deps.AnalyticsEntClient != nil {
		analyticsProvider = analytics.NewSQLiteAnalyticsProvider(deps.AnalyticsEntClient)
		log.Info("Analytics: using SQLiteAnalyticsProvider")
	} else {
		analyticsProvider = analytics.NewLogAnalyticsProvider()
		log.Info("Analytics: using LogAnalyticsProvider (fallback)")
	}

	// Start analytics retention enforcer (hourly; exits when serverCtx is cancelled).
	cfg := config.LoadConfig()
	if deps.AnalyticsEntClient != nil {
		analytics.StartRetentionEnforcer(serverCtx, deps.AnalyticsEntClient,
			cfg.AnalyticsMaxRowsOrDefault(), cfg.AnalyticsMaxAgeDaysOrDefault(), cfg.EscapeAnalyticsRetentionDays)
		log.Info("Analytics retention enforcer started", "maxRows", cfg.AnalyticsMaxRowsOrDefault(), "maxAgeDays", cfg.AnalyticsMaxAgeDaysOrDefault())
	}

	// Start escape analytics batch writer and register it as the global writer.
	// New ResponseStream instances (created per session) will pick it up via GetGlobalEscapeWriter().
	if deps.AnalyticsEntClient != nil && cfg.EscapeAnalyticsCaptureLevel != "off" {
		escapeWriter := analytics.NewEscapeEventBatchWriter(deps.AnalyticsEntClient, cfg.EscapeAnalyticsMaxRowsPerSession)
		go escapeWriter.Start(serverCtx)
		pkganalytics.SetGlobalEscapeWriter(escapeWriter)
		log.Info("Escape analytics batch writer started",
			"captureLevel", cfg.EscapeAnalyticsCaptureLevel,
			"maxRowsPerSession", cfg.EscapeAnalyticsMaxRowsPerSession,
		)
	} else {
		log.Info("Escape analytics disabled (no DB client or captureLevel=off)")
	}

	// Wire analytics ent client into SessionService for escape analytics RPC handlers.
	if deps.AnalyticsEntClient != nil {
		deps.SessionService.SetAnalyticsClient(deps.AnalyticsEntClient)
		log.Info("Wired analytics ent client into SessionService for escape analytics RPCs")
	}

	// Start EventBus analytics subscriber (maps session lifecycle events to analytics records).
	analytics.StartAnalyticsSubscriber(serverCtx, deps.EventBus, analyticsProvider)
	log.Info("Analytics EventBus subscriber started")

	// Start EventBus GitHub forward-sync subscriber (closes the linked GitHub
	// issue when a backlog item transitions to done — AC3). Shares
	// BacklogService's plugin registry/key provider so behavior matches
	// TriggerSync; both are nil-safe when the backlog feature isn't wired.
	if deps.BacklogService != nil {
		services.StartBacklogGitHubForwardSyncSubscriber(serverCtx, deps.EventBus, deps.BacklogService.Registry(), deps.BacklogService.SyncLoopForForwardSync(), deps.Storage)
	}

	// Register analytics HTTP handler (POST /api/analytics, GET /api/analytics/summary).
	analyticsHandler := handlers.NewAnalyticsHandlerWithClient(analyticsProvider, deps.AnalyticsEntClient)
	analyticsHandler.RegisterRoutes(srv.mux)
	log.Info("Registered analytics handler at POST /api/analytics and GET /api/analytics/summary")

	// Register telemetry handler for frontend performance events
	telemetryHandler := handlers.NewTelemetryHandler(analyticsProvider)
	srv.mux.HandleFunc("POST /api/telemetry", telemetryHandler.HandleTelemetry)
	log.Info("Registered telemetry handler at POST /api/telemetry")

	// Register raw file download endpoint.
	// Uses the FileService inside SessionService to validate paths against
	// the session worktree root (path traversal prevention).
	fileSvc := deps.SessionService.GetFileService()
	srv.mux.HandleFunc("/api/files/raw", fileSvc.ServeFileRaw)
	log.Info("Registered raw file download handler at /api/files/raw")

	// Local file browser — serves arbitrary local filesystem paths.
	// Auth is provided by the existing middleware chain:
	// local HTTP = no auth; remote HTTPS = WebAuthn required.
	localFileSvc := services.NewLocalFileService()
	srv.mux.HandleFunc("/api/local/files/list", localFileSvc.ListLocalDirectory)
	srv.mux.Handle("/api/local/serve/", http.StripPrefix("/api/local/serve", http.HandlerFunc(localFileSvc.ServeLocalFile)))
	log.Info("Registered local file browser at /api/local/files/list and /api/local/serve/")

	// Register backlog attachment upload endpoint — durable image attachments
	// for backlog item descriptions, served back via /api/local/serve/.
	if backlogAttachmentDir, err := cfg.BacklogAttachmentDirOrDefault(); err != nil {
		log.Error("[Server] cannot resolve backlog attachment dir", "err", err)
	} else if backlogAttachmentHandler, err := services.NewBacklogAttachmentUploadHandler(backlogAttachmentDir); err != nil {
		log.Error("[Server] cannot create backlog attachment upload handler", "dir", backlogAttachmentDir, "err", err)
	} else {
		srv.mux.HandleFunc("POST /api/v1/upload-backlog-attachment", backlogAttachmentHandler.HandleUpload)
		log.Info("Registered backlog attachment upload handler at POST /api/v1/upload-backlog-attachment", "dir", backlogAttachmentDir)
	}

	// One-time best-effort sweep of the launch-prompt temp-file cache
	// (session/instance_tmux.go's promptArg). Instance.Destroy() and
	// promptFileCleanupDelay's timer both clean up individual files during
	// normal operation; this sweep catches orphans left behind by a crash or
	// an unclean shutdown.
	if promptCacheDir, err := cfg.PromptCacheDirOrDefault(); err != nil {
		log.Error("[Server] cannot resolve prompt cache dir", "err", err)
	} else if entries, err := os.ReadDir(promptCacheDir); err != nil {
		if !os.IsNotExist(err) {
			log.Warn("[Server] cannot read prompt cache dir for startup sweep", "dir", promptCacheDir, "err", err)
		}
	} else {
		removed := 0
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if err := os.Remove(filepath.Join(promptCacheDir, entry.Name())); err != nil {
				log.Warn("[Server] failed to remove orphaned prompt cache file", "path", entry.Name(), "err", err)
				continue
			}
			removed++
		}
		if removed > 0 {
			log.Info("Swept orphaned prompt cache files at startup", "dir", promptCacheDir, "count", removed)
		}
	}

	// Start hibernation sweeper (auto-hibernates idle sessions and prunes stale checkpoints).
	if cfg.Hibernation.Enabled {
		sweeper := session.NewHibernationSweeper(deps.Storage, cfg, memory.NewGopsutilReader())
		if deps.ReviewQueuePoller != nil {
			sweeper.SetLiveProvider(deps.ReviewQueuePoller)
		}
		deps.SessionService.SetMemoryCacheReader(sweeper)
		go sweeper.Start(serverCtx)
		log.Info("Hibernation sweeper started",
			"idle_timeout_minutes", cfg.Hibernation.IdleTimeoutMinutes)
	}

	// Start session retention sweeper (deletes archived sessions past the retention
	// window once they pass safety checks — see SessionRetentionSweeper doc comment).
	if cfg.SessionRetention.EnabledOrDefault() {
		retentionSweeper := services.NewSessionRetentionSweeper(deps.Storage, cfg, deps.SessionService)
		go retentionSweeper.Start(serverCtx)
		log.Info("Session retention sweeper started",
			"retention_days", cfg.SessionRetention.RetentionDaysOrDefault())
	}

	// Start stale session notifier (fires an operator-facing notification the first time an
	// ACTIVE session crosses the configured stale threshold — see StaleSessionNotifier doc
	// comment).
	if deps.ReviewQueuePoller != nil {
		staleNotifier := services.NewStaleSessionNotifier(deps.ReviewQueuePoller, deps.EventBus)
		go staleNotifier.Start(serverCtx)
		log.Info("Stale session notifier started",
			"threshold_minutes", cfg.StaleSession.ThresholdMinutesOrDefault(),
			"notify_enabled", cfg.StaleSession.NotifyEnabledOrDefault())
	}
}

// registerStaticRoutes mounts routes that are always registered regardless of
// whether dependencies were successfully built (image upload, server-info, web UI).
func registerStaticRoutes(srv *Server) {
	// Register file upload endpoint — saves clipboard files to a temp directory
	// so the terminal process can reference them by path (e.g. for Claude Code image paste).
	pasteDir := filepath.Join(os.TempDir(), "stapler-paste")
	fileHandler := services.NewFileUploadHandler(pasteDir)
	srv.mux.HandleFunc("/api/upload/file", fileHandler.HandleUpload)
	log.Info("Registered file upload handler at /api/upload/file", "dir", pasteDir)

	// Detect available programs once at startup so /api/server-info never runs
	// shell subprocesses on a live request (each detection spawns 5 shells).
	srv.availablePrograms = config.GetAvailablePrograms()

	// Register server-info endpoint for settings UI
	srv.registerServerInfoHandler()
	log.Info("Registered server-info handler at /api/server-info")

	// Serve web UI static files
	distFS, err := web.GetDistFS()
	if err != nil {
		log.Error("Failed to load web UI filesystem", "err", err)
	} else {
		staticHandler := middleware.StaticFileServer(distFS, "index.html")
		srv.mux.Handle("/", staticHandler)
		log.Info("Registered web UI static file server at /")
	}
}

// SetupTLS configures the server to use TLS with the provided tls.Config.
// Must be called before Start().
func (s *Server) SetupTLS(cfg *tls.Config) {
	s.tlsConfig = cfg
	s.httpServer.TLSConfig = cfg
	log.Info("TLS enabled", "addr", s.GetAddr())
}

// SetupAuth installs authentication middleware.  Must be called before Start().
// authMiddleware is a function that wraps an http.Handler; pass nil to disable.
func (s *Server) SetupAuth(authMiddleware func(http.Handler) http.Handler) {
	s.authMiddleware = authMiddleware
}

// RegisterConnectHandler registers a ConnectRPC service handler.
// This should be called before Start().
func (s *Server) RegisterConnectHandler(path string, handler http.Handler) {
	s.mux.Handle(path, handler)
	log.Info("Registered ConnectRPC handler", "path", path)
}

// RegisterHTTPHandler registers a standard HTTP handler.
// Useful for health checks, static files, etc.
func (s *Server) RegisterHTTPHandler(pattern string, handler http.Handler) {
	s.mux.Handle(pattern, handler)
	log.Info("Registered HTTP handler", "pattern", pattern)
}

// listenLoopbackAware binds addr for the HTTP server. When addr's host is
// "localhost", it binds both loopback address families (127.0.0.1 and ::1)
// explicitly instead of relying on net.Listen("tcp", "localhost:port"), which
// only ever binds whichever single address the resolver returns first. On
// dual-stack machines (macOS resolves "localhost" to both 127.0.0.1 and ::1)
// that gap means a browser WebSocket upgrade that resolves to the address we
// didn't bind gets ECONNREFUSED, even though plain HTTP requests succeed by
// falling back across addresses. Any other host is bound as before.
func listenLoopbackAware(addr string) (net.Listener, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || host != "localhost" {
		return net.Listen("tcp", addr)
	}

	ln4, err := net.Listen("tcp4", "127.0.0.1:"+port)
	if err != nil {
		return nil, err
	}
	ln6, err := net.Listen("tcp6", "[::1]:"+port)
	if err != nil {
		log.Warn("IPv6 loopback listener unavailable, continuing on IPv4 only", "error", err)
		return ln4, nil
	}
	return &dualStackListener{ln4: ln4, ln6: ln6, accepted: make(chan acceptResult)}, nil
}

// acceptResult carries the outcome of one net.Listener.Accept call across a
// goroutine boundary.
type acceptResult struct {
	conn net.Conn
	err  error
}

// dualStackListener presents two underlying loopback listeners (IPv4 and
// IPv6) as a single net.Listener, so the http.Server can Serve() over both
// with one call.
type dualStackListener struct {
	ln4, ln6 net.Listener
	accepted chan acceptResult
	once     sync.Once
}

func (d *dualStackListener) relay(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		d.accepted <- acceptResult{conn: conn, err: err}
		if err != nil {
			return
		}
	}
}

func (d *dualStackListener) Accept() (net.Conn, error) {
	d.once.Do(func() {
		go d.relay(d.ln4)
		go d.relay(d.ln6)
	})
	res := <-d.accepted
	return res.conn, res.err
}

func (d *dualStackListener) Close() error {
	err4 := d.ln4.Close()
	err6 := d.ln6.Close()
	if err4 != nil {
		return err4
	}
	return err6
}

func (d *dualStackListener) Addr() net.Addr {
	return d.ln4.Addr()
}

// Start starts the HTTP server with middleware chain.
// This is a blocking call. Use Start() in a goroutine for concurrent operation.
func (s *Server) Start(ctx context.Context) error {
	// Register health check endpoint
	s.mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok","service":"stapler-squad-web"}`)) //nolint:errcheck
	})
	s.registerActuatorRoutes()

	// Build middleware chain:
	// otelhttp -> logging -> CORS -> gzip -> [auth] -> mux
	inner := http.Handler(s.mux)
	if s.authMiddleware != nil {
		inner = s.authMiddleware(inner)
	}
	handler := otelhttp.NewHandler(
		middleware.Logging(middleware.CORSWithOrigins(s.origins)(middleware.Compress(inner))),
		"stapler-squad-http",
		otelhttp.WithMessageEvents(otelhttp.ReadEvents, otelhttp.WriteEvents),
	)
	s.httpServer.Handler = handler

	scheme := "http"
	if s.tlsConfig != nil {
		scheme = "https"
	}

	errCh := make(chan error, 1)
	go func() {
		var err error
		if s.tlsConfig != nil {
			// TLS mode: cert/key are already in TLSConfig.Certificates; the
			// configured address is never rewritten, so it's safe to log now.
			log.Info("Starting server", "scheme", scheme, "addr", s.GetAddr())
			log.Info("Web UI", "url", scheme+"://"+s.GetAddr())
			log.Info("Health check", "url", scheme+"://"+s.GetAddr()+"/health")
			err = s.httpServer.ListenAndServeTLS("", "")
		} else {
			ln, lerr := listenLoopbackAware(s.GetAddr())
			if lerr != nil {
				errCh <- lerr
				return
			}
			resolvedAddr := ln.Addr().String()
			s.addr.Store(&resolvedAddr)
			// Log the real, OS-assigned address (never the pre-bind ":0"
			// request) now that the listener has actually bound.
			log.Info("Starting server", "scheme", scheme, "addr", resolvedAddr)
			log.Info("Web UI", "url", scheme+"://"+resolvedAddr)
			log.Info("Health check", "url", scheme+"://"+resolvedAddr+"/health")
			err = s.httpServer.Serve(ln)
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.Info("Shutting down HTTP server...")
		return s.Shutdown()
	case err := <-errCh:
		return err
	}
}

// shutdownHooksTimeout bounds how long shutdown hooks (state persistence,
// pollers, etc.) may run before Shutdown proceeds to stop the HTTP server
// regardless. Prevents a stuck hook from causing a SIGKILL on restart/stop.
const shutdownHooksTimeout = 30 * time.Second

// Shutdown gracefully shuts down the HTTP server.
func (s *Server) Shutdown() error {
	// Cancel the server's BaseContext first so active streaming connections
	// (ConnectRPC terminal streams) see a done context and close themselves,
	// preventing context deadline exceeded on the graceful shutdown below.
	if s.connCtxCancel != nil {
		s.connCtxCancel()
	}

	// Run registered hooks (e.g. capture pane paths, persist instance state) before
	// stopping the HTTP server so in-flight requests complete first. Bounded by an
	// overall deadline: a single slow/stuck hook must not be able to block process
	// exit past systemd's stop timeout, which would trigger a SIGKILL that skips
	// this cleanup entirely instead of just skipping whatever the slow hook was
	// doing (systemd's default TimeoutStopSec is ~90s but this app has many
	// sessions to persist under load, so give hooks a bounded but generous window).
	hooksDone := make(chan struct{})
	go func() {
		defer close(hooksDone)
		for _, hook := range s.shutdownHooks {
			hook()
		}
	}()
	select {
	case <-hooksDone:
	case <-time.After(shutdownHooksTimeout):
		log.Warn("[shutdown] shutdown hooks did not complete within deadline, proceeding anyway", "timeout", shutdownHooksTimeout)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := s.httpServer.Shutdown(ctx); err != nil {
		log.Error("HTTP server shutdown error", "err", err)
		return err
	}

	log.Info("HTTP server stopped gracefully")
	return nil
}

// GetAddr returns the server address.
func (s *Server) GetAddr() string {
	if p := s.addr.Load(); p != nil {
		return *p
	}
	return ""
}

// Mux returns the HTTP request multiplexer so callers can register additional
// routes before calling Start().
func (s *Server) Mux() *http.ServeMux {
	return s.mux
}

// SetHTTPSURL records the public HTTPS URL for this server (used by /api/server-info).
// Call this after remote access is configured in main.go.
func (s *Server) SetHTTPSURL(url string) {
	s.httpsURL = url
}

// SetHostnames records the detected LAN hostnames for this server.
func (s *Server) SetHostnames(hostnames []string) {
	s.hostnames = hostnames
}

// GetHostnames returns the detected LAN hostnames.
func (s *Server) GetHostnames() []string {
	return s.hostnames
}

// SetOrigins records the allowed CORS origins.
func (s *Server) SetOrigins(origins []string) {
	s.origins = origins
}

// GetOrigins returns the allowed CORS origins.
func (s *Server) GetOrigins() []string {
	return s.origins
}

// registerServerInfoHandler registers the /api/server-info endpoint which exposes
// the CA PEM file path and HTTPS URL for display in the settings UI.
func (s *Server) registerServerInfoHandler() {
	s.mux.HandleFunc("/api/server-info", func(w http.ResponseWriter, r *http.Request) {
		type serverInfoResponse struct {
			CAPEMPath  string   `json:"ca_pem_path"`
			HTTPSURL   string   `json:"https_url"`
			TLSEnabled bool     `json:"tls_enabled"`
			Hostnames  []string `json:"hostnames"`
			Programs   []string `json:"programs"`
		}

		configDir, err := config.GetConfigDir()
		var caPath string
		tlsEnabled := false
		if err == nil {
			caPath = filepath.Join(configDir, "tls-ca.pem")
			if _, statErr := os.Stat(caPath); statErr == nil {
				tlsEnabled = true
			}
		}

		info := serverInfoResponse{
			CAPEMPath:  caPath,
			HTTPSURL:   s.httpsURL,
			TLSEnabled: tlsEnabled,
			Hostnames:  s.hostnames,
			Programs:   s.availablePrograms,
		}

		w.Header().Set("Content-Type", "application/json")
		if encErr := json.NewEncoder(w).Encode(info); encErr != nil {
			log.Error("server-info: encode error", "err", encErr)
		}
	})
}

// StartRemote starts a second HTTPS server on remoteAddr, sharing the same
// route mux as the local server but protected by TLS and auth middleware.
// It binds eagerly (returns a bind error immediately if the port is in use),
// then runs the server in a background goroutine until ctx is cancelled.
func (s *Server) StartRemote(ctx context.Context, remoteAddr string, tlsCfg *tls.Config, authMW func(http.Handler) http.Handler) error {
	inner := http.Handler(s.mux)
	if authMW != nil {
		inner = authMW(inner)
	}
	handler := otelhttp.NewHandler(
		middleware.Logging(middleware.CORSWithOrigins(s.origins)(middleware.Compress(inner))),
		"stapler-squad-remote",
		otelhttp.WithMessageEvents(otelhttp.ReadEvents, otelhttp.WriteEvents),
	)

	remoteSrv := &http.Server{
		Addr:         remoteAddr,
		Handler:      handler,
		TLSConfig:    tlsCfg,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  60 * time.Second,
	}

	// Bind eagerly so the caller gets a port-in-use error immediately.
	ln, err := net.Listen("tcp", remoteAddr)
	if err != nil {
		return fmt.Errorf("bind remote server on %s: %w", remoteAddr, err)
	}
	log.Info("Remote HTTPS server listening", "addr", remoteAddr)

	go func() {
		// Shutdown when the main context is cancelled.
		go func() {
			<-ctx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if shutdownErr := remoteSrv.Shutdown(shutdownCtx); shutdownErr != nil {
				log.Error("Remote HTTPS server shutdown error", "err", shutdownErr)
			} else {
				log.Info("Remote HTTPS server stopped gracefully")
			}
		}()

		if serveErr := remoteSrv.ServeTLS(ln, "", ""); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			log.Error("Remote HTTPS server error", "err", serveErr)
		}
	}()

	return nil
}

// notificationDecisionListerAdapter adapts *notifications.NotificationHistoryStore.List
// to session.NotificationDecisionLister, which BuildDecisionsSnapshot
// (session/session_summary_snapshot.go) needs. session cannot import
// server/notifications directly (server/notifications -> server/events ->
// pkg/events -> session is a real import cycle — see session.DecisionRecord's doc
// comment), so this thin adapter lives here where both packages are already
// imported.
type notificationDecisionListerAdapter struct {
	store *notifications.NotificationHistoryStore
}

// ListDecisionRecords implements session.NotificationDecisionLister.
func (a *notificationDecisionListerAdapter) ListDecisionRecords(_ context.Context, sessionID string) ([]session.DecisionRecord, error) {
	records, _, err := a.store.List(notifications.ListOptions{
		SessionID: sessionID,
		Limit:     notifications.MaxNotifications, // unpaginated — need every record for this session
	})
	if err != nil {
		return nil, err
	}
	out := make([]session.DecisionRecord, len(records))
	for i, r := range records {
		out[i] = session.DecisionRecord{
			NotificationType: r.NotificationType,
			ApprovalDecision: r.Metadata["approval_decision"],
		}
	}
	return out, nil
}

// instanceDataLister is the narrow slice of *session.Storage that
// buildSessionExistenceLookup needs — defined here (the consuming package) rather
// than in session, per this repo's interface-pollution convention, so a fake with
// no real *session.Storage (and no ent-backed repository) can stand in for tests.
type instanceDataLister interface {
	ListInstanceData() ([]session.InstanceData, error)
}

// pruneOrphanedMinUptime gates buildSessionExistenceLookup's returned func so a
// fresh server (before instance data has had a chance to load) never mistakes
// "haven't loaded yet" for "no sessions exist" and wipes legitimate
// session-scoped notification records.
const pruneOrphanedMinUptime = 5 * time.Minute

// buildSessionExistenceLookup builds the batch-fetch session-existence lookup wired
// onto NotificationHistoryStore.SetSessionExistenceLookup, consulted by the store's
// orphan-pruning sweep (enforceRetention → pruneOrphanedRecords).
//
// Return value contract (safety-critical — do not change without updating callers):
//   - nil: skip this prune pass entirely (either the uptime gate hasn't elapsed yet,
//     or ListInstanceData errored). This is the fail-closed, safe default.
//   - non-nil, possibly empty, map: authoritative — "these are all the sessions that
//     exist right now." An empty-but-non-nil map means zero live sessions, and tells
//     the pruning sweep it's safe to prune every session-scoped record.
//
// Conflating "haven't checked" (nil) with "checked, found nothing" (empty map) would
// cause the pruning sweep to delete every session-scoped notification record on a
// fresh start or after a transient ListInstanceData error — hence the explicit nil
// returns below rather than returning an empty map in those cases.
func buildSessionExistenceLookup(storage instanceDataLister, startedAt time.Time) func() map[string]struct{} {
	return func() map[string]struct{} {
		if time.Since(startedAt) < pruneOrphanedMinUptime {
			return nil
		}
		all, err := storage.ListInstanceData()
		if err != nil {
			log.Warn("SetSessionExistenceLookup: ListInstanceData failed; skipping this prune pass", "err", err)
			return nil
		}
		ids := make(map[string]struct{}, len(all)*2)
		for i := range all {
			ids[all[i].GetStableID()] = struct{}{}
			if all[i].Title != "" {
				ids[all[i].Title] = struct{}{}
			}
		}
		return ids
	}
}

// ConnectOptions returns standard ConnectRPC options with OpenTelemetry instrumentation
// and optional SQLite error recording.  Pass a non-nil registry to persist RPC errors.
func ConnectOptions(registry interceptors.ErrorRecorder) []connect.HandlerOption {
	otelInterceptor, err := otelconnect.NewInterceptor(
		otelconnect.WithTrustRemote(),
	)
	if err != nil {
		log.Warn("Failed to create otelconnect interceptor", "err", err)
		return []connect.HandlerOption{}
	}

	return []connect.HandlerOption{
		connect.WithInterceptors(
			interceptors.NewErrorRecorderInterceptor(registry),
			otelInterceptor,
		),
	}
}
