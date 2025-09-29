package server

import (
	"claude-squad/gen/proto/go/session/v1/sessionv1connect"
	"claude-squad/log"
	"claude-squad/server/middleware"
	"claude-squad/server/services"
	"context"
	"errors"
	"net/http"
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
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
	}

	// Initialize SessionService and register ConnectRPC handlers
	sessionService, err := services.NewSessionServiceFromConfig()
	if err != nil {
		log.ErrorLog.Printf("Failed to initialize SessionService: %v", err)
		// Continue without SessionService - will return errors on RPC calls
	} else {
		path, handler := sessionv1connect.NewSessionServiceHandler(sessionService, ConnectOptions()...)
		srv.RegisterConnectHandler(path, handler)
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
