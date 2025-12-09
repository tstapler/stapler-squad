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
		// Default to "raw" streaming mode (simplest, most reliable)
		// Mount under /api/ prefix for cleaner URLs
		wsHandler := services.NewConnectRPCWebSocketHandler(sessionService, scrollbackManager, "raw")
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
	// Build middleware chain
	handler := middleware.Logging(middleware.CORS(s.mux))
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

// ConnectOptions returns standard ConnectRPC options.
// These can be customized per-handler if needed.
func ConnectOptions() []connect.HandlerOption {
	return []connect.HandlerOption{
		connect.WithInterceptors(), // Add interceptors here as needed
	}
}
