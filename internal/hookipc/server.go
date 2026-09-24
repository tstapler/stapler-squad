package hookipc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"syscall"
	"time"
)

const (
	classifyPath       = "/v1/classify"
	defaultMaxBodySize = 1 << 20
)

var ErrEndpointInUse = errors.New("hookipc: endpoint already in use")

type Handler func(context.Context, ClassificationEnvelope) (ClassificationReply, error)

type Server struct {
	endpoint  HookEndpoint
	handler   Handler
	server    *http.Server
	listener  net.Listener
	closeOnce sync.Once
}

func NewServer(endpoint HookEndpoint, handler Handler) (*Server, error) {
	if endpoint.SocketPath == "" || endpoint.InstanceFingerprint == "" || endpoint.ProtocolVersion == 0 {
		return nil, errors.New("hookipc: complete endpoint is required")
	}
	if handler == nil {
		return nil, errors.New("hookipc: handler is required")
	}
	s := &Server{endpoint: endpoint, handler: handler}
	mux := http.NewServeMux()
	mux.HandleFunc(classifyPath, s.handleClassify)
	s.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 100 * time.Millisecond,
		ReadTimeout:       500 * time.Millisecond,
		WriteTimeout:      500 * time.Millisecond,
		IdleTimeout:       30 * time.Second,
	}
	return s, nil
}

// Start binds synchronously, then serves requests concurrently in the
// background. A live socket is never unlinked; only a socket that fails a
// bounded connection probe is treated as stale.
func (s *Server) Start() error {
	if err := removeStaleSocket(s.endpoint.SocketPath); err != nil {
		return err
	}
	listener, err := net.Listen("unix", s.endpoint.SocketPath)
	if err != nil {
		return fmt.Errorf("hookipc: listen: %w", err)
	}
	if err := os.Chmod(s.endpoint.SocketPath, 0o600); err != nil {
		_ = listener.Close()
		_ = os.Remove(s.endpoint.SocketPath)
		return fmt.Errorf("hookipc: secure socket: %w", err)
	}
	s.listener = listener
	go func() {
		if err := s.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			// Start has already returned. The owning server observes endpoint health;
			// request failures safely fall back at the client boundary.
			return
		}
	}()
	return nil
}

func (s *Server) Close(ctx context.Context) error {
	var closeErr error
	s.closeOnce.Do(func() {
		if s.listener == nil {
			return
		}
		closeErr = s.server.Shutdown(ctx)
		if err := os.Remove(s.endpoint.SocketPath); err != nil && !os.IsNotExist(err) && closeErr == nil {
			closeErr = fmt.Errorf("hookipc: remove socket: %w", err)
		}
	})
	return closeErr
}

func (s *Server) handleClassify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, defaultMaxBodySize)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var envelope ClassificationEnvelope
	if err := decoder.Decode(&envelope); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if err := envelope.Validate(s.endpoint); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, ErrProtocolMismatch) {
			status = http.StatusUpgradeRequired
		} else if errors.Is(err, ErrInstanceMismatch) {
			status = http.StatusForbidden
		}
		http.Error(w, http.StatusText(status), status)
		return
	}
	reply, err := s.handler(r.Context(), envelope)
	if err != nil {
		http.Error(w, "classification unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := reply.Validate(s.endpoint, envelope.RequestID); err != nil {
		http.Error(w, "invalid classification reply", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(reply); err != nil {
		return
	}
}

func removeStaleSocket(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("hookipc: inspect socket: %w", err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("hookipc: remove non-socket endpoint: %w", err)
		}
		return nil
	}
	conn, err := net.DialTimeout("unix", path, 50*time.Millisecond)
	if err == nil {
		_ = conn.Close()
		return ErrEndpointInUse
	}
	// Only connection-refused proves that a socket inode has no listener. A
	// permission or policy error must never authorize unlinking another
	// process's live endpoint.
	if !errors.Is(err, syscall.ECONNREFUSED) && !errors.Is(err, syscall.ENOENT) {
		return fmt.Errorf("hookipc: probe existing socket: %w", err)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("hookipc: remove stale socket: %w", err)
	}
	return nil
}
