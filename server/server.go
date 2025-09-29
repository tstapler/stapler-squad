package server

import (
	"claude-squad/log"
	"claude-squad/server/middleware"
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

// NewServer creates a new HTTP server instance.
// Handlers can be registered before calling Start().
func NewServer(addr string) *Server {
	mux := http.NewServeMux()

	return &Server{
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
