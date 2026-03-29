package server

import (
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/middleware"
	"github.com/tstapler/stapler-squad/server/notifications"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/web"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
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
	addr      string
	httpServer *http.Server
	mux        *http.ServeMux
	tlsConfig  *tls.Config          // non-nil when TLS is enabled
	authMiddleware func(http.Handler) http.Handler // nil when auth is disabled
	httpsURL   string               // set when remote access is enabled
	hostnames  []string             // detected LAN hostnames
	origins    []string             // allowed CORS origins
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
		serverCtx := context.Background()
		go deps.ReactiveQueueMgr.Start(serverCtx)
		log.InfoLog.Printf("ReactiveQueueManager started")

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

		// Start background expiration cleanup for pending approvals
		services.StartExpirationCleanup(context.Background(), deps.SessionService.GetApprovalStore())

		// Register Escape Code Analytics handler for debugging terminal rendering
		escapeCodeHandler := services.NewEscapeCodeHandler()
		escapeCodeHandler.RegisterRoutes(srv.mux)
		log.InfoLog.Printf("Registered Escape Code Analytics handlers at /api/debug/escape-codes/*")

		// Register Circuit Breaker debug handler for observability
		cbHandler := services.NewCircuitBreakerHandler()
		cbHandler.RegisterRoutes(srv.mux)
		log.InfoLog.Printf("Registered Circuit Breaker debug handler at /api/debug/circuit-breakers")
	}

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

	return srv
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
