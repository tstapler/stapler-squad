package testutil

import (
	"fmt"
	"github.com/tstapler/stapler-squad/executor"
	"github.com/tstapler/stapler-squad/session/tmux"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
)

// TmuxWaiter provides utilities for waiting on tmux operations
type TmuxWaiter struct {
	session *tmux.TmuxSession
}

// NewTmuxWaiter creates a new tmux waiter
func NewTmuxWaiter(session *tmux.TmuxSession) *TmuxWaiter {
	return &TmuxWaiter{session: session}
}

// WaitForSessionExists waits for the tmux session to exist
func (w *TmuxWaiter) WaitForSessionExists() error {
	config := DefaultWaitConfig()
	config.Description = fmt.Sprintf("tmux session '%s' to exist", w.getSessionName())

	return WaitForCondition(func() bool {
		return w.session.DoesSessionExist()
	}, config)
}

// WaitForSessionExistsWithConfig waits for session with custom config
func (w *TmuxWaiter) WaitForSessionExistsWithConfig(config WaitConfig) error {
	if config.Description == "condition" {
		config.Description = fmt.Sprintf("tmux session '%s' to exist", w.getSessionName())
	}

	return WaitForCondition(func() bool {
		return w.session.DoesSessionExist()
	}, config)
}

// WaitForContent waits for tmux session to have specific content
func (w *TmuxWaiter) WaitForContent(validator ContentValidator) (string, error) {
	config := DefaultWaitConfig()
	config.Description = fmt.Sprintf("tmux session '%s' to have expected content", w.getSessionName())

	return WaitForContent(
		func() (string, error) {
			return w.session.CapturePaneContent()
		},
		validator,
		config,
	)
}

// WaitForContentWithConfig waits for content with custom config
func (w *TmuxWaiter) WaitForContentWithConfig(validator ContentValidator, config WaitConfig) (string, error) {
	if config.Description == "condition" {
		config.Description = fmt.Sprintf("tmux session '%s' to have expected content", w.getSessionName())
	}

	return WaitForContent(
		func() (string, error) {
			return w.session.CapturePaneContent()
		},
		validator,
		config,
	)
}

// WaitForNonEmptyContent waits for any non-empty content
func (w *TmuxWaiter) WaitForNonEmptyContent() (string, error) {
	return w.WaitForContent(NonEmptyContent)
}

// WaitForContentContaining waits for content containing specific text
func (w *TmuxWaiter) WaitForContentContaining(text string) (string, error) {
	return w.WaitForContent(ContainsText(text))
}

// WaitForSessionReady waits for session to exist and have content
func (w *TmuxWaiter) WaitForSessionReady() error {
	// First wait for session to exist
	if err := w.WaitForSessionExists(); err != nil {
		return fmt.Errorf("session never became available: %v", err)
	}

	// Then wait for it to have content (indicating it's running)
	_, err := w.WaitForNonEmptyContent()
	if err != nil {
		return fmt.Errorf("session never produced content: %v", err)
	}

	return nil
}

// WaitForSessionReadyWithTimeout waits for session with custom timeout
func (w *TmuxWaiter) WaitForSessionReadyWithTimeout(config WaitConfig) error {
	// First wait for session to exist
	existsConfig := config
	existsConfig.Description = fmt.Sprintf("tmux session '%s' to exist", w.getSessionName())
	if err := w.WaitForSessionExistsWithConfig(existsConfig); err != nil {
		return fmt.Errorf("session never became available: %v", err)
	}

	// Then wait for it to have content
	contentConfig := config
	contentConfig.Description = fmt.Sprintf("tmux session '%s' to have content", w.getSessionName())
	_, err := w.WaitForContentWithConfig(NonEmptyContent, contentConfig)
	if err != nil {
		return fmt.Errorf("session never produced content: %v", err)
	}

	return nil
}

// getSessionName safely gets the session name for error messages
func (w *TmuxWaiter) getSessionName() string {
	if w.session == nil {
		return "<nil>"
	}
	// We can't access the private sanitizedName field, so use a generic name
	return "session"
}

// Package-level convenience functions

// WaitForTmuxSession waits for a tmux session to be ready
func WaitForTmuxSession(session *tmux.TmuxSession) error {
	return NewTmuxWaiter(session).WaitForSessionReady()
}

// WaitForTmuxSessionWithTimeout waits for tmux session with custom timeout
func WaitForTmuxSessionWithTimeout(session *tmux.TmuxSession, config WaitConfig) error {
	return NewTmuxWaiter(session).WaitForSessionReadyWithTimeout(config)
}

// WaitForTmuxContent waits for tmux session to have specific content
func WaitForTmuxContent(session *tmux.TmuxSession, validator ContentValidator) (string, error) {
	return NewTmuxWaiter(session).WaitForContent(validator)
}

// WaitForTmuxContentContaining waits for tmux content containing text
func WaitForTmuxContentContaining(session *tmux.TmuxSession, text string) (string, error) {
	return NewTmuxWaiter(session).WaitForContentContaining(text)
}

// ============================================================================
// Isolated Tmux Server Helpers - For Real Tmux Integration Testing
// ============================================================================

// serverCounter provides unique IDs for multiple servers in the same test
var serverCounter uint64

// TmuxTestServer represents an isolated tmux server for testing.
// Each test gets its own tmux server socket to prevent interference between tests.
type TmuxTestServer struct {
	socketName string
	executor   executor.Executor
	t          *testing.T
}

// CreateIsolatedTmuxServer creates a new isolated tmux server for testing.
// The server uses a unique socket name based on the test name to prevent conflicts.
// Cleanup is automatically registered with t.Cleanup().
func CreateIsolatedTmuxServer(t *testing.T) *TmuxTestServer {
	t.Helper()

	// Check if tmux is available
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available, skipping test")
	}

	// Generate unique socket name from test name + atomic counter
	// This ensures multiple servers in the same test get unique names
	counter := atomic.AddUint64(&serverCounter, 1)
	socketName := fmt.Sprintf("test_%s_%d", sanitizeTestName(t.Name()), counter)

	server := &TmuxTestServer{
		socketName: socketName,
		executor:   executor.MakeExecutor(),
		t:          t,
	}

	// Register cleanup to kill all sessions and server on test completion
	t.Cleanup(func() {
		server.Cleanup()
	})

	return server
}

// GetSocketName returns the socket name for this isolated server
func (s *TmuxTestServer) GetSocketName() string {
	return s.socketName
}

// CreateSession creates and starts a new tmux session on this isolated server
func (s *TmuxTestServer) CreateSession(sessionName string, command string) (*tmux.TmuxSession, error) {
	s.t.Helper()

	// Use tmux dependency injection to create session on isolated server
	// Use a test-specific prefix to avoid conflicts with production sessions
	prefix := "test_"
	session := tmux.NewTmuxSessionWithServerSocket(sessionName, command, prefix, s.socketName)

	// Start the session with current directory
	workDir := "."
	if err := session.Start(workDir); err != nil {
		return nil, fmt.Errorf("failed to start tmux session: %w", err)
	}

	return session, nil
}

// CreateSessionWithoutStarting creates a tmux session object without starting it.
// This is useful for testing timeout and hang scenarios.
func (s *TmuxTestServer) CreateSessionWithoutStarting(sessionName string, command string, prefix string) *tmux.TmuxSession {
	s.t.Helper()
	return tmux.NewTmuxSessionWithServerSocket(sessionName, command, prefix, s.socketName)
}

// ListSessions returns all session names on this isolated server
func (s *TmuxTestServer) ListSessions() ([]string, error) {
	s.t.Helper()

	cmd := exec.Command("tmux", "-L", s.socketName, "list-sessions", "-F", "#{session_name}")
	output, err := s.executor.Output(cmd)
	if err != nil {
		// No sessions or no server running is not an error - return empty list
		// tmux returns exit status 1 when no sessions exist
		errStr := err.Error()
		if strings.Contains(errStr, "no server running") ||
			strings.Contains(errStr, "no sessions") ||
			(strings.Contains(errStr, "exit status 1") && len(output) == 0) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}

	// Parse session names (one per line)
	names := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(names) == 1 && names[0] == "" {
		return []string{}, nil
	}
	return names, nil
}

// SessionExists checks if a session exists on this server
func (s *TmuxTestServer) SessionExists(sessionName string) bool {
	s.t.Helper()

	cmd := exec.Command("tmux", "-L", s.socketName, "has-session", "-t", sessionName)
	err := s.executor.Run(cmd)
	return err == nil
}

// KillSession kills a specific session on this server
func (s *TmuxTestServer) KillSession(sessionName string) error {
	s.t.Helper()

	cmd := exec.Command("tmux", "-L", s.socketName, "kill-session", "-t", sessionName)
	// Use CombinedOutput to get both stdout and stderr
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Session already gone or server not running is not an error (idempotent)
		// Check both error message and combined output (stdout + stderr)
		errStr := err.Error()
		outputStr := string(output)
		combinedMsg := errStr + " " + outputStr
		if strings.Contains(combinedMsg, "session not found") ||
			strings.Contains(combinedMsg, "can't find session") ||
			strings.Contains(combinedMsg, "no server running") ||
			strings.Contains(combinedMsg, "no current client") ||
			strings.Contains(combinedMsg, "error connecting") {
			return nil
		}
		return fmt.Errorf("failed to kill session %s: %w", sessionName, err)
	}
	return nil
}

// KillAllSessions kills all sessions on this isolated server
func (s *TmuxTestServer) KillAllSessions() error {
	s.t.Helper()

	sessions, err := s.ListSessions()
	if err != nil {
		return fmt.Errorf("failed to list sessions for cleanup: %w", err)
	}

	for _, sessionName := range sessions {
		if err := s.KillSession(sessionName); err != nil {
			// Log but don't fail on individual session kill errors
			s.t.Logf("Warning: failed to kill session %s: %v", sessionName, err)
		}
	}

	return nil
}

// KillServer kills the entire tmux server (all sessions)
func (s *TmuxTestServer) KillServer() error {
	s.t.Helper()

	cmd := exec.Command("tmux", "-L", s.socketName, "kill-server")
	err := s.executor.Run(cmd)
	if err != nil {
		// Server already gone is not an error
		if strings.Contains(err.Error(), "no server running") {
			return nil
		}
		return fmt.Errorf("failed to kill server: %w", err)
	}
	return nil
}

// Cleanup performs cleanup of the isolated tmux server.
// This is automatically called via t.Cleanup() but can also be called manually.
func (s *TmuxTestServer) Cleanup() {
	s.t.Helper()

	// Try to kill all sessions first (cleaner)
	if err := s.KillAllSessions(); err != nil {
		s.t.Logf("Warning: error during session cleanup: %v", err)
	}

	// Then kill the server
	if err := s.KillServer(); err != nil {
		s.t.Logf("Warning: error during server cleanup: %v", err)
	}
}

// sanitizeTestName converts test name to filesystem-safe socket name
func sanitizeTestName(testName string) string {
	// Replace problematic characters with underscores
	replacer := strings.NewReplacer(
		"/", "_",
		" ", "_",
		".", "_",
		":", "_",
		"(", "_",
		")", "_",
	)
	return replacer.Replace(testName)
}

// TempSessionName generates a temporary session name with an ID
func TempSessionName(prefix string, id int) string {
	return fmt.Sprintf("%s_%d", prefix, id)
}

// ============================================================================
// Cleanup Helpers for Legacy Tests
// ============================================================================

// CleanupTmuxSessionsWithPrefix cleans up tmux sessions with a specific prefix.
// This is useful for cleaning up sessions created by legacy tests that don't use
// isolated servers.
func CleanupTmuxSessionsWithPrefix(t *testing.T, prefix string) {
	t.Helper()

	execImpl := executor.MakeExecutor()

	// List all sessions
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
	output, err := execImpl.Output(cmd)
	if err != nil {
		// No sessions running is fine
		if strings.Contains(err.Error(), "no server running") {
			return
		}
		t.Logf("Warning: failed to list sessions for cleanup: %v", err)
		return
	}

	// Find and kill sessions with matching prefix
	sessions := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, sessionName := range sessions {
		if strings.HasPrefix(sessionName, prefix) {
			killCmd := exec.Command("tmux", "kill-session", "-t", sessionName)
			if err := execImpl.Run(killCmd); err != nil {
				t.Logf("Warning: failed to kill session %s: %v", sessionName, err)
			}
		}
	}
}
