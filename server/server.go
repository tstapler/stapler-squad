package server

import (
	"claude-squad/gen/proto/go/session/v1/sessionv1connect"
	"claude-squad/log"
	"claude-squad/server/middleware"
	"claude-squad/server/services"
	"claude-squad/server/web"
	"claude-squad/session"
	"claude-squad/session/scrollback"
	"context"
	"errors"
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
	addr       string
	httpServer *http.Server
	mux        *http.ServeMux
}

// NewServer creates a new HTTP server instance with SessionService registered.
func NewServer(addr string) *Server {
	mux := http.NewServeMux()

	srv := &Server{
		addr: addr,
		mux:  mux,
		httpServer: &http.Server{
			Addr:         addr,
			Handler:      nil, // Will be set in Start() with middleware
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 0, // No write timeout - we have long-lived streaming connections
			IdleTimeout:  60 * time.Second,
		},
	}

	// Initialize SessionService and register ConnectRPC handlers
	sessionService, err := services.NewSessionServiceFromConfig()
	if err != nil {
		log.ErrorLog.Printf("Failed to initialize SessionService: %v", err)
		// Continue without SessionService - will return errors on RPC calls
	} else {
		// Initialize ReactiveQueueManager components
		storage := sessionService.GetStorage()
		eventBus := sessionService.GetEventBus()
		reviewQueue := sessionService.GetReviewQueueInstance()

		// CRITICAL: Create and wire dependencies BEFORE starting storage
		// This ensures instances have statusManager when they start, allowing
		// controller initialization to succeed
		statusManager := session.NewInstanceStatusManager()
		reviewQueuePoller := session.NewReviewQueuePoller(reviewQueue, statusManager, storage)

		// CRITICAL: Start storage AFTER creating statusManager
		// This loads and starts instances with proper dependency wiring
		if err := storage.Start(); err != nil {
			log.ErrorLog.Printf("Failed to start storage: %v", err)
		} else {
			log.InfoLog.Printf("Storage started - instances loaded and ready")
		}

		// Load instances (now they're already started with controllers running)
		instances, err := storage.LoadInstances()
		if err != nil {
			log.ErrorLog.Printf("Failed to load instances for reactive queue: %v", err)
		} else {
			// Wire up review queue and status manager to ALL instances
			// This handles both newly loaded instances and ensures consistency
			for _, inst := range instances {
				inst.SetReviewQueue(reviewQueue)
				inst.SetStatusManager(statusManager)
			}
			reviewQueuePoller.SetInstances(instances)

			// CRITICAL: Start loaded instances now that dependencies are wired
			// LoadInstances() reads data from disk but doesn't start tmux sessions
			// We must explicitly start instances here for controller initialization to work
			for _, inst := range instances {
				if !inst.Started() {
					// Start with firstTimeSetup=false since this is a loaded session
					if err := inst.Start(false); err != nil {
						log.ErrorLog.Printf("Failed to start loaded instance '%s': %v", inst.Title, err)
					} else {
						log.InfoLog.Printf("Started loaded instance '%s'", inst.Title)
					}
				}
			}

			// MIGRATION: Save instances back to disk to persist any auto-detected worktree info
			// During LoadInstances(), FromInstanceData() calls DetectAndPopulateWorktreeInfo()
			// which may discover IsWorktree, MainRepoPath, GitHubOwner, GitHubRepo for existing sessions.
			// We need to save this migrated data back to state.json so it persists across restarts.
			// CRITICAL: This must happen AFTER instances are started because SaveInstances()
			// only saves instances where Started() == true.
			if len(instances) > 0 {
				if saveErr := storage.SaveInstances(instances); saveErr != nil {
					log.WarningLog.Printf("Failed to persist migrated instance data: %v", saveErr)
				} else {
					log.InfoLog.Printf("Persisted migrated instance data for %d instances", len(instances))
				}
			}

			// Start controllers for loaded instances that deferred startup
			// Now that statusManager is wired, controller initialization can succeed
			log.InfoLog.Printf("Attempting controller startup for %d loaded instances", len(instances))
			for _, inst := range instances {
				started := inst.Started()
				paused := inst.Paused()
				log.InfoLog.Printf("Instance '%s': Started()=%v, Paused()=%v", inst.Title, started, paused)
				if started && !paused {
					// Check if controller already exists (shouldn't happen but defensive)
					if inst.GetController() == nil {
						if err := inst.StartController(); err != nil {
							log.WarningLog.Printf("Failed to start controller for loaded instance '%s': %v", inst.Title, err)
						} else {
							log.InfoLog.Printf("Started controller for loaded instance '%s'", inst.Title)
						}
					} else {
						log.InfoLog.Printf("Instance '%s' already has active controller", inst.Title)
					}
				}
			}

			// Create and start ReactiveQueueManager
			reactiveQueueMgr := NewReactiveQueueManager(
				reviewQueue,
				reviewQueuePoller,
				eventBus,
				statusManager,
				storage,
			)

			// Wire ReactiveQueueManager back to SessionService
			sessionService.SetReactiveQueueManager(reactiveQueueMgr)

			// Start the reactive queue manager in the background
			// It will be stopped when the server shuts down
			go reactiveQueueMgr.Start(context.Background())

			log.InfoLog.Printf("ReactiveQueueManager initialized and started")
		}

		// Initialize ScrollbackManager
		homeDir, _ := os.UserHomeDir()
		scrollbackPath := filepath.Join(homeDir, ".claude-squad", "sessions")

		scrollbackConfig := scrollback.DefaultScrollbackConfig()
		scrollbackConfig.StoragePath = scrollbackPath
		scrollbackManager := scrollback.NewScrollbackManager(scrollbackConfig)

		log.InfoLog.Printf("Initialized ScrollbackManager: path=%s, compression=%s, maxLines=%d",
			scrollbackPath, scrollbackConfig.CompressionType, scrollbackConfig.MaxLines)

		// Register ConnectRPC WebSocket handler FIRST for streaming RPCs
		// This must come before the general ConnectRPC handler to avoid response writer wrapping
		// Initialize tmux streamer manager first - needed for ALL sessions (managed + external)
		// ALL sessions run in tmux and use capture-pane polling for terminal streaming
		tmuxStreamerManager := session.NewExternalTmuxStreamerManager()

		// Default to "raw-compressed" streaming mode (LZMA compression for reduced bandwidth)
		// Mount under /api/ prefix for cleaner URLs
		wsHandler := services.NewConnectRPCWebSocketHandler(sessionService, scrollbackManager, tmuxStreamerManager, "raw-compressed")
		wsPath := "/api" + sessionv1connect.SessionServiceStreamTerminalProcedure
		srv.mux.HandleFunc(wsPath, wsHandler.HandleWebSocket)
		log.InfoLog.Printf("Registered ConnectRPC WebSocket handler: %s", wsPath)

		// Register general ConnectRPC handler (for unary calls)
		// Mount under /api/ prefix for cleaner URLs and to prevent conflicts with static files
		// Use StripPrefix to remove /api before requests reach the Connect handler
		path, handler := sessionv1connect.NewSessionServiceHandler(sessionService, ConnectOptions()...)
		apiPath := "/api" + path
		wrappedHandler := http.StripPrefix("/api", handler)
		srv.RegisterConnectHandler(apiPath, wrappedHandler)

		// Initialize External Session Discovery and Streaming (tmux-based)
		externalDiscovery := session.NewExternalSessionDiscovery()
		externalApprovalMonitor := session.NewExternalApprovalMonitor()

		// Wire external discovery to session service for unified session listing
		sessionService.SetExternalDiscovery(externalDiscovery)

		// Wire external session persistence - save to storage when discovered, remove when gone
		externalDiscovery.OnSessionAdded(func(instance *session.Instance) {
			if err := storage.AddInstance(instance); err != nil {
				log.ErrorLog.Printf("Failed to persist external session '%s': %v", instance.Title, err)
			} else {
				log.InfoLog.Printf("Persisted external session '%s' to storage", instance.Title)
			}

			// CRITICAL: Wire external session dependencies for review queue integration
			// Without these, external sessions won't appear in the review queue
			instance.SetReviewQueue(reviewQueue)
			instance.SetStatusManager(statusManager)

			// Add to review queue poller for monitoring
			reviewQueuePoller.AddInstance(instance)
			log.InfoLog.Printf("Added external session '%s' to review queue poller", instance.Title)
		})
		externalDiscovery.OnSessionRemoved(func(instance *session.Instance) {
			// Remove from review queue poller first
			reviewQueuePoller.RemoveInstance(instance.Title)
			log.InfoLog.Printf("Removed external session '%s' from review queue poller", instance.Title)

			// Remove from review queue (in case it has pending items)
			reviewQueue.Remove(instance.Title)

			if err := storage.DeleteInstance(instance.Title); err != nil {
				log.WarningLog.Printf("Failed to remove external session '%s' from storage: %v", instance.Title, err)
			} else {
				log.InfoLog.Printf("Removed external session '%s' from storage", instance.Title)
			}
		})

		// Start external session discovery
		externalDiscovery.Start(5 * time.Second)
		externalApprovalMonitor.Start()

		// Auto-integrate approval monitor with discovery (using tmux session names)
		externalApprovalMonitor.IntegrateWithDiscoveryTmux(externalDiscovery, tmuxStreamerManager)

		// CRITICAL: Bridge approval monitor to review queue
		// When an approval is detected in an external session, immediately add to review queue
		// This ensures external sessions get the same review queue treatment as managed sessions
		externalApprovalMonitor.OnApproval(func(event *session.ExternalApprovalEvent) {
			if event == nil || event.Request == nil {
				return
			}

			// Find the instance for this session
			instance := externalDiscovery.GetSessionByTmux(event.SessionID)
			if instance == nil {
				// Try by socket path as fallback
				instance = externalDiscovery.GetSession(event.SessionID)
			}

			// Build context string from the approval request
			context := event.Request.DetectedText
			if context == "" {
				context = "Permission request detected"
			}

			// Create review item with high priority for approval requests
			item := &session.ReviewItem{
				SessionID:   event.SessionTitle,
				SessionName: event.SessionTitle,
				Reason:      session.ReasonApprovalPending,
				Priority:    session.PriorityHigh,
				DetectedAt:  event.Request.Timestamp,
				Context:     context,
			}

			// Populate additional fields if we have the instance
			if instance != nil {
				item.Program = instance.Program
				item.Branch = instance.Branch
				item.Path = instance.Path
				item.WorkingDir = instance.WorkingDir
				item.Status = instance.Status
				item.Tags = instance.Tags
				item.Category = instance.Category
				item.DiffStats = instance.GetDiffStats()
				item.LastActivity = instance.LastMeaningfulOutput
			}

			// Add to review queue
			reviewQueue.Add(item)
			log.InfoLog.Printf("Added external session approval '%s' to review queue (type: %s, confidence: %.2f)",
				event.SessionTitle, event.Request.Type, event.Request.Confidence)
		})

		// Wire external session support to unified ConnectRPC WebSocket handler
		// This enables the unified handler to stream both managed and external sessions
		// through the same /api/session.v1.SessionService/StreamTerminal endpoint
		wsHandler.SetExternalSessionSupport(externalDiscovery)
		log.InfoLog.Printf("Unified WebSocket handler configured for external session support")

		// Legacy external session endpoints removed - unified WebSocket streaming now handles both session types
		// The following endpoints were removed as part of the Unified WebSocket Streaming Architecture:
		// - /api/ws/external (replaced by /api/session.v1.SessionService/StreamTerminal)
		// - /api/external/sessions (sessions now included in unified session listing)
		// - /api/external/resize (resize handled via unified WebSocket protocol)
		//
		// Approval endpoints are still needed for external session approval monitoring
		externalWsHandler := services.NewExternalWebSocketHandler(
			externalDiscovery,
			tmuxStreamerManager,
			externalApprovalMonitor,
			eventBus,
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
	// Build middleware chain with OpenTelemetry HTTP instrumentation
	// Order: otelhttp -> logging -> CORS -> handler
	// otelhttp provides automatic span creation for all HTTP requests
	handler := otelhttp.NewHandler(
		middleware.Logging(middleware.CORS(s.mux)),
		"claude-squad-http",
		otelhttp.WithMessageEvents(otelhttp.ReadEvents, otelhttp.WriteEvents),
	)
	s.httpServer.Handler = handler

	// Register health check endpoint
	s.mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok","service":"claude-squad-web"}`))
	})

	log.InfoLog.Printf("Starting HTTP server on %s", s.addr)
	log.InfoLog.Printf("Web UI: http://%s", s.addr)
	log.InfoLog.Printf("Health check: http://%s/health", s.addr)

	// Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	// Wait for context cancellation or server error
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
// Traces are sent to the configured OTLP endpoint (e.g., Datadog Agent).
func ConnectOptions() []connect.HandlerOption {
	// Create otelconnect interceptor for automatic RPC tracing
	otelInterceptor, err := otelconnect.NewInterceptor(
		otelconnect.WithTrustRemote(), // Trust remote span context for distributed tracing
	)
	if err != nil {
		log.WarningLog.Printf("Failed to create otelconnect interceptor: %v", err)
		return []connect.HandlerOption{}
	}

	return []connect.HandlerOption{
		connect.WithInterceptors(otelInterceptor),
	}
}
