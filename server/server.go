package server

import (
	"claude-squad/gen/proto/go/session/v1/sessionv1connect"
	"claude-squad/log"
	"claude-squad/server/middleware"
	"claude-squad/server/services"
	"claude-squad/server/web"
	"context"
	"errors"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/otelconnect"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Server manages the HTTP server with ConnectRPC handlers.
type Server struct {
	addr       string
	httpServer *http.Server
	mux        *http.ServeMux
}

// NewServer creates a new HTTP server instance with SessionService registered.
//
// Initialization Order (dependencies flow downward):
//
//  1. SessionService      — creates Storage, EventBus, ReviewQueue
//  2. StatusManager       — depends on nothing; created before storage starts
//  3. ReviewQueuePoller   — depends on ReviewQueue, StatusManager, Storage
//  4. Storage.Start()     — loads instances from disk; must happen after StatusManager is wired
//  5. Instance wiring     — SetReviewQueue + SetStatusManager on each loaded instance
//  6. Instance.Start()    — starts tmux sessions; requires wired dependencies
//  7. Controller startup  — requires started instances and StatusManager
//  8. ReactiveQueueMgr    — depends on ReviewQueue, Poller, EventBus, StatusManager, Storage
//  9. ScrollbackManager   — independent; depends only on filesystem paths
// 10. TmuxStreamerManager — independent
// 11. ExternalDiscovery   — depends on Storage, ReviewQueue, StatusManager, Poller (via callbacks)
// 12. ExternalApprovalMonitor — depends on ExternalDiscovery
//
// Violating this order causes nil pointer panics or silent failures.
// Dependency construction is encapsulated in BuildDependencies (server/dependencies.go).
// See docs/tasks/architecture-refactor.md for the ongoing simplification plan.
func NewServer(addr string) *Server {
	mux := http.NewServeMux()

	srv := &Server{
		addr: addr,
		mux:  mux,
		httpServer: &http.Server{
			Addr:         addr,
			Handler:      nil, // Set in Start() after middleware chain is built
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 0, // No write timeout — streaming connections are long-lived
			IdleTimeout:  60 * time.Second,
		},
	}

	deps, err := BuildDependencies()
	if err != nil {
		log.ErrorLog.Printf("Failed to build server dependencies: %v", err)
		// Continue without services — all RPC calls will return errors
	} else {
		// Start background components
		go deps.ReactiveQueueMgr.Start(context.Background())
		log.InfoLog.Printf("ReactiveQueueManager started")

		// Wire external discovery to SessionService for unified session listing
		deps.SessionService.SetExternalDiscovery(deps.ExternalDiscovery)

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
		path, handler := sessionv1connect.NewSessionServiceHandler(deps.SessionService, ConnectOptions()...)
		apiPath := "/api" + path
		srv.RegisterConnectHandler(apiPath, http.StripPrefix("/api", handler))

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

		// Register Escape Code Analytics handler for debugging terminal rendering
		escapeCodeHandler := services.NewEscapeCodeHandler()
		escapeCodeHandler.RegisterRoutes(srv.mux)
		log.InfoLog.Printf("Registered Escape Code Analytics handlers at /api/debug/escape-codes/*")
	}

	// Serve web UI static files
	distFS, err := web.GetDistFS()
	if err != nil {
		log.ErrorLog.Printf("Failed to load web UI filesystem: %v", err)
	} else {
		staticHandler := middleware.StaticFileServer(distFS, "index.html")
		srv.mux.Handle("/", staticHandler)
		log.InfoLog.Printf("Registered web UI static file server at /")
	}

	return srv
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
	handler := otelhttp.NewHandler(
		middleware.Logging(middleware.CORS(s.mux)),
		"claude-squad-http",
		otelhttp.WithMessageEvents(otelhttp.ReadEvents, otelhttp.WriteEvents),
	)
	s.httpServer.Handler = handler

	s.mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok","service":"claude-squad-web"}`))
	})

	log.InfoLog.Printf("Starting HTTP server on %s", s.addr)
	log.InfoLog.Printf("Web UI: http://%s", s.addr)
	log.InfoLog.Printf("Health check: http://%s/health", s.addr)

	errCh := make(chan error, 1)
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
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

// ConnectOptions returns standard ConnectRPC options with OpenTelemetry instrumentation.
func ConnectOptions() []connect.HandlerOption {
	otelInterceptor, err := otelconnect.NewInterceptor(
		otelconnect.WithTrustRemote(),
	)
	if err != nil {
		log.WarningLog.Printf("Failed to create otelconnect interceptor: %v", err)
		return []connect.HandlerOption{}
	}

	return []connect.HandlerOption{
		connect.WithInterceptors(otelInterceptor),
	}
}
