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
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/handlers"
	"github.com/tstapler/stapler-squad/server/interceptors"
	servermcp "github.com/tstapler/stapler-squad/server/mcp"
	"github.com/tstapler/stapler-squad/server/middleware"
	"github.com/tstapler/stapler-squad/server/notifications"
	"github.com/tstapler/stapler-squad/server/push"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/web"
	"github.com/tstapler/stapler-squad/session/tmux"

	"github.com/google/uuid"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/otelconnect"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Server manages the HTTP server with ConnectRPC handlers.
type Server struct {
	addr           string
	httpServer     *http.Server
	mux            *http.ServeMux
	tlsConfig      *tls.Config                     // non-nil when TLS is enabled
	authMiddleware func(http.Handler) http.Handler // nil when auth is disabled
	httpsURL       string                          // set when remote access is enabled
	hostnames      []string                        // detected LAN hostnames
	origins        []string                        // allowed CORS origins
	shutdownHooks  []func()                        // called before HTTP server stops
	connCtxCancel  context.CancelFunc              // cancels BaseContext → closes active streams on shutdown
}

// newServerBase creates the base Server struct and returns it alongside the
// connection context that drives active-stream cancellation on shutdown.
// Both NewServer and NewServerWithDeps call this before wiring dependencies.
func newServerBase(addr string) (*Server, context.Context) {
	mux := http.NewServeMux()
	connCtx, connCtxCancel := context.WithCancel(context.Background())
	srv := &Server{
		addr:          addr,
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

	log.InfoLog.Printf("Building server dependencies...")
	startTime := time.Now()
	deps, err := BuildDependencies()
	if err != nil {
		log.ErrorLog.Printf("Failed to build server dependencies: %v", err)
		// Continue without services — all RPC calls will return errors
	} else {
		log.InfoLog.Printf("Server dependencies built in %v", time.Since(startTime))
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

// wireDepsIntoServer wires pre-built ServerDependencies into srv: starts background
// components, registers shutdown hooks, and mounts all ConnectRPC/HTTP handlers.
// serverCtx (== connCtx from newServerBase) is cancelled by Shutdown() to signal
// active streaming connections to close.
func wireDepsIntoServer(srv *Server, deps *ServerDependencies, serverCtx context.Context) {
	// Start background components
	go deps.ReactiveQueueMgr.Start(serverCtx)
	log.InfoLog.Printf("ReactiveQueueManager started")

	deps.PRStatusPoller.Start(serverCtx)
	log.InfoLog.Printf("PRStatusPoller started")

	// Start HistoryLinker: detects Claude JSONL files and links conversation
	// UUIDs to sessions so cold restore can use --resume on restart.
	go deps.HistoryLinker.Start(serverCtx)
	log.InfoLog.Printf("HistoryLinker started")

	// Start UnfinishedWork scanner.
	if deps.UnfinishedScanner != nil {
		deps.UnfinishedScanner.Start(serverCtx)
		log.InfoLog.Printf("UnfinishedWork scanner started")
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
				log.WarningLog.Printf("[shutdown] Capture deadline exceeded; skipped %d of %d instances",
					len(instances)-captured, len(instances))
				break
			}
			if err := inst.CaptureCurrentState(); err != nil {
				log.WarningLog.Printf("[shutdown] CaptureCurrentState '%s': %v", inst.Title, err)
			}
			captured++
		}
		if err := storage.SaveInstances(instances); err != nil {
			log.WarningLog.Printf("[shutdown] SaveInstances: %v", err)
		} else {
			log.InfoLog.Printf("[shutdown] Persisted working dirs for %d instances", captured)
		}
	})

	// Initialize notification history store and EventBus subscriber.
	// notifStore is declared here so it can be wired into the approval handler below.
	var notifStore *notifications.NotificationHistoryStore
	configDir, configErr := config.GetConfigDir()
	if configErr != nil {
		log.ErrorLog.Printf("Failed to get config dir for notification store: %v", configErr)
	} else {
		notifStorePath := filepath.Join(configDir, "notifications.json")
		var storeErr error
		notifStore, storeErr = notifications.NewNotificationHistoryStore(notifStorePath)
		if storeErr != nil {
			log.ErrorLog.Printf("Failed to create notification history store: %v", storeErr)
			notifStore = nil
		} else {
			notifications.StartSubscriber(serverCtx, deps.EventBus, notifStore)
			log.InfoLog.Printf("NotificationHistoryStore initialized at %s", notifStorePath)

			// Wire the notification store into the session service for RPC access
			deps.SessionService.SetNotificationStore(notifStore)
		}
	}

	// Initialize push notification service.
	if configErr == nil {
		pushService := services.NewPushService(configDir)
		pushHandler := services.NewPushHandler(pushService)
		pushHandler.RegisterRoutes(srv.mux)
		push.StartPushSubscriber(serverCtx, deps.EventBus, pushService)
		log.InfoLog.Printf("Push notification service initialized")
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
		log.WarningLog.Printf("[ForkPressure] alert dispatched: level=%s %s", level, body)

		// Immediately reconcile to mark dead sessions Stopped, cutting spawn rate.
		if deps.ReviewQueuePoller != nil {
			deps.ReviewQueuePoller.ForceReconcile()
		}
	})

	// Start fork pressure logger (logs stats every 30s when activity > 0).
	tmux.StartForkPressureLogger(serverCtx, 30*time.Second, func(format string, args ...any) {
		log.InfoLog.Printf(format, args...)
	})

	// Become the subreaper for our process tree so that tmux's zombie children
	// get reparented to us (not init) when tmux hasn't yet reaped them.
	// No-op on macOS and Windows; effective on Linux only.
	if err := tmux.SetSubreaper(); err != nil {
		log.WarningLog.Printf("[zombie] SetSubreaper failed (non-fatal): %v", err)
	} else {
		log.InfoLog.Printf("[zombie] subreaper enabled: tmux descendant zombies will be reparented here")
	}

	// Start zombie watcher (scans for zombie child processes every 30s).
	tmux.StartZombieWatcher(serverCtx, 30*time.Second, func(format string, args ...any) {
		log.WarningLog.Printf(format, args...)
	})

	// Start zombie reaper (calls waitpid(-1, WNOHANG) every 60s to reap any
	// zombie children left by cmd.Start() paths that skipped cmd.Wait()).
	tmux.StartZombieReaper(serverCtx, 60*time.Second, func(format string, args ...any) {
		log.InfoLog.Printf(format, args...)
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
		log.InfoLog.Printf("[tmux] recovery notification sent to connected clients")
	})

	// Note: SetExternalDiscovery is now called inside BuildRuntimeDeps.

	// Start external session infrastructure
	deps.ExternalDiscovery.Start(5 * time.Second)
	deps.ExternalApprovalMonitor.Start()
	deps.ExternalApprovalMonitor.IntegrateWithDiscoveryTmux(deps.ExternalDiscovery, deps.TmuxStreamerManager)

	// Register ConnectRPC WebSocket handler (must come before unary handler)
	wsHandler := services.NewConnectRPCWebSocketHandler(
		deps.SessionService, deps.ScrollbackManager, deps.TmuxStreamerManager, "raw-compressed",
	)
	wsPath := "/api" + sessionv1connect.SessionServiceStreamTerminalProcedure
	srv.mux.HandleFunc(wsPath, wsHandler.HandleWebSocket)
	log.InfoLog.Printf("Registered ConnectRPC WebSocket handler: %s", wsPath)

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
	log.InfoLog.Printf("Registered StreamingWSBridge for %s and %s", watchSessionsPath, watchReviewQueuePath)

	srv.RegisterConnectHandler(apiPath, http.StripPrefix("/api", handler))

	// Register UnfinishedWorkService handler.
	if deps.UnfinishedWorkService != nil {
		uwPath, uwHandler := sessionv1connect.NewUnfinishedWorkServiceHandler(deps.UnfinishedWorkService, ConnectOptions(deps.ErrorRegistry)...)
		uwAPIPath := "/api" + uwPath
		srv.RegisterConnectHandler(uwAPIPath, http.StripPrefix("/api", uwHandler))
		log.InfoLog.Printf("Registered UnfinishedWorkService handler at %s", uwAPIPath)
	}

	// Wire external session support into the unified WebSocket handler
	wsHandler.SetExternalSessionSupport(deps.ExternalDiscovery)
	log.InfoLog.Printf("Unified WebSocket handler configured for external session support")

	// Register external approval endpoints
	externalWsHandler := services.NewExternalWebSocketHandler(
		deps.ExternalDiscovery,
		deps.TmuxStreamerManager,
		deps.ExternalApprovalMonitor,
		deps.EventBus,
	)
	srv.mux.HandleFunc("/api/external/approvals", externalWsHandler.HandleApprovals)
	srv.mux.HandleFunc("/api/external/approvals/respond", externalWsHandler.HandleApprovalResponse)
	log.InfoLog.Printf("Registered External Session approval handlers at /api/external/approvals/*")

	// Register Claude Code HTTP hook approval endpoint
	approvalHandler := services.NewApprovalHandler(
		deps.SessionService.GetApprovalStore(),
		deps.Storage,
		deps.EventBus,
	)
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
	}
	srv.mux.HandleFunc("/api/hooks/permission-request", approvalHandler.HandlePermissionRequest)
	log.InfoLog.Printf("Registered Claude Code hook approval handler at /api/hooks/permission-request")

	// Register non-approval hook receivers (stop, pre/post-tool-use, prompt-submit)
	hookReceiver := services.NewHookReceiver()
	hookReceiver.RegisterRoutes(srv.mux)
	log.InfoLog.Printf("Registered Claude Code hook receivers at /api/hooks/{stop,pre-tool-use,post-tool-use,prompt-submit}")

	// Register session-aware image upload endpoint (multipart/form-data, saves to worktree).
	sessionUploadHandler := services.NewSessionImageUploadHandler(deps.Storage, deps.ReviewQueuePoller)
	srv.mux.HandleFunc("POST /api/v1/upload-image", sessionUploadHandler.HandleUpload)
	log.InfoLog.Printf("Registered session image upload handler at POST /api/v1/upload-image")

	// Register MCP HTTP transport at /mcp so Claude sessions can connect
	// without spawning a subprocess. The URL is passed via --mcp-server to
	// claude when creating new sessions (no settings-file injection needed).
	mcpHTTPHandler := servermcp.NewHTTPHandler(deps.Storage, deps.SessionService, deps.ScrollbackManager)
	srv.mux.Handle("/mcp", mcpHTTPHandler)
	srv.mux.Handle("/mcp/", mcpHTTPHandler)
	mcpURL := "http://" + srv.addr + "/mcp"
	deps.SessionService.SetMCPServerURL(mcpURL)
	log.InfoLog.Printf("Registered MCP HTTP handler at /mcp (URL: %s)", mcpURL)

	// Start background expiration cleanup for pending approvals
	services.StartExpirationCleanup(context.Background(), deps.SessionService.GetApprovalStore())

	// Register Escape Code Analytics handler for debugging terminal rendering
	escapeCodeHandler := services.NewEscapeCodeHandler()
	escapeCodeHandler.RegisterRoutes(srv.mux)
	log.InfoLog.Printf("Registered Escape Code Analytics handlers at /api/debug/escape-codes/*")

	// Register runtime log-level handler (used by the debug menu in the web UI)
	logLevelHandler := services.NewLogLevelHandler()
	logLevelHandler.RegisterRoutes(srv.mux)
	log.InfoLog.Printf("Registered log-level handler at /api/debug/log-level")

	// Register Circuit Breaker debug handler for observability
	cbHandler := services.NewCircuitBreakerHandler()
	cbHandler.RegisterRoutes(srv.mux)
	log.InfoLog.Printf("Registered Circuit Breaker debug handler at /api/debug/circuit-breakers")

	// Register telemetry handler for frontend performance events
	telemetryHandler := handlers.NewTelemetryHandler()
	srv.mux.HandleFunc("POST /api/telemetry", telemetryHandler.HandleTelemetry)
	log.InfoLog.Printf("Registered telemetry handler at POST /api/telemetry")

	// Register raw file download endpoint.
	// Uses the FileService inside SessionService to validate paths against
	// the session worktree root (path traversal prevention).
	fileSvc := deps.SessionService.GetFileService()
	srv.mux.HandleFunc("/api/files/raw", fileSvc.ServeFileRaw)
	log.InfoLog.Printf("Registered raw file download handler at /api/files/raw")
}

// registerStaticRoutes mounts routes that are always registered regardless of
// whether dependencies were successfully built (image upload, server-info, web UI).
func registerStaticRoutes(srv *Server) {
	// Register image upload endpoint — saves clipboard images to a temp directory
	// so the terminal process can reference them by path (e.g. for Claude Code image paste).
	pasteDir := filepath.Join(os.TempDir(), "stapler-paste")
	imageHandler := services.NewImageUploadHandler(pasteDir)
	srv.mux.HandleFunc("/api/upload/image", imageHandler.HandleUpload)
	log.InfoLog.Printf("Registered image upload handler at /api/upload/image (dir: %s)", pasteDir)

	// Register server-info endpoint for settings UI
	srv.registerServerInfoHandler()
	log.InfoLog.Printf("Registered server-info handler at /api/server-info")

	// Serve web UI static files
	distFS, err := web.GetDistFS()
	if err != nil {
		log.ErrorLog.Printf("Failed to load web UI filesystem: %v", err)
	} else {
		staticHandler := middleware.StaticFileServer(distFS, "index.html")
		srv.mux.Handle("/", staticHandler)
		log.InfoLog.Printf("Registered web UI static file server at /")
	}
}

// SetupTLS configures the server to use TLS with the provided tls.Config.
// Must be called before Start().
func (s *Server) SetupTLS(cfg *tls.Config) {
	s.tlsConfig = cfg
	s.httpServer.TLSConfig = cfg
	log.InfoLog.Printf("TLS enabled on %s", s.addr)
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
	log.InfoLog.Printf("Registered ConnectRPC handler: %s", path)
}

// RegisterHTTPHandler registers a standard HTTP handler.
// Useful for health checks, static files, etc.
func (s *Server) RegisterHTTPHandler(pattern string, handler http.Handler) {
	s.mux.Handle(pattern, handler)
	log.InfoLog.Printf("Registered HTTP handler: %s", pattern)
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
	log.InfoLog.Printf("Starting %s server on %s", scheme, s.addr)
	log.InfoLog.Printf("Web UI: %s://%s", scheme, s.addr)
	log.InfoLog.Printf("Health check: %s://%s/health", scheme, s.addr)

	errCh := make(chan error, 1)
	go func() {
		var err error
		if s.tlsConfig != nil {
			// TLS mode: cert/key are already in TLSConfig.Certificates
			err = s.httpServer.ListenAndServeTLS("", "")
		} else {
			err = s.httpServer.ListenAndServe()
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.InfoLog.Printf("Shutting down HTTP server...")
		return s.Shutdown()
	case err := <-errCh:
		return err
	}
}

// Shutdown gracefully shuts down the HTTP server.
func (s *Server) Shutdown() error {
	// Cancel the server's BaseContext first so active streaming connections
	// (ConnectRPC terminal streams) see a done context and close themselves,
	// preventing context deadline exceeded on the graceful shutdown below.
	if s.connCtxCancel != nil {
		s.connCtxCancel()
	}

	// Run registered hooks (e.g. capture pane paths, persist instance state) before
	// stopping the HTTP server so in-flight requests complete first.
	for _, hook := range s.shutdownHooks {
		hook()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := s.httpServer.Shutdown(ctx); err != nil {
		log.ErrorLog.Printf("HTTP server shutdown error: %v", err)
		return err
	}

	log.InfoLog.Printf("HTTP server stopped gracefully")
	return nil
}

// GetAddr returns the server address.
func (s *Server) GetAddr() string {
	return s.addr
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
			Programs:   config.GetAvailablePrograms(),
		}

		w.Header().Set("Content-Type", "application/json")
		if encErr := json.NewEncoder(w).Encode(info); encErr != nil {
			log.ErrorLog.Printf("server-info: encode error: %v", encErr)
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
	log.InfoLog.Printf("Remote HTTPS server listening on %s", remoteAddr)

	go func() {
		// Shutdown when the main context is cancelled.
		go func() {
			<-ctx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if shutdownErr := remoteSrv.Shutdown(shutdownCtx); shutdownErr != nil {
				log.ErrorLog.Printf("Remote HTTPS server shutdown error: %v", shutdownErr)
			} else {
				log.InfoLog.Printf("Remote HTTPS server stopped gracefully")
			}
		}()

		if serveErr := remoteSrv.ServeTLS(ln, "", ""); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			log.ErrorLog.Printf("Remote HTTPS server error: %v", serveErr)
		}
	}()

	return nil
}

// ConnectOptions returns standard ConnectRPC options with OpenTelemetry instrumentation
// and optional SQLite error recording.  Pass a non-nil registry to persist RPC errors.
func ConnectOptions(registry interceptors.ErrorRecorder) []connect.HandlerOption {
	otelInterceptor, err := otelconnect.NewInterceptor(
		otelconnect.WithTrustRemote(),
	)
	if err != nil {
		log.WarningLog.Printf("Failed to create otelconnect interceptor: %v", err)
		return []connect.HandlerOption{}
	}

	return []connect.HandlerOption{
		connect.WithInterceptors(
			interceptors.NewErrorRecorderInterceptor(registry),
			otelInterceptor,
		),
	}
}
