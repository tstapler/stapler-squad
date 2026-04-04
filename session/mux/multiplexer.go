//go:build !windows

package mux

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// Multiplexer handles PTY multiplexing for external Claude sessions.
// It creates a tmux session with stapler-squad's naming convention, allowing
// stapler-squad to discover and control the session (including killing it).
// Multiple clients can connect via Unix domain socket for bidirectional terminal access.
type Multiplexer struct {
	// Configuration
	command    string
	args       []string
	socketPath string

	// tmux session (uses stapler-squad naming convention for adoption)
	tmuxSession string

	// attachOnly indicates we're attaching to an existing tmux session
	// instead of creating a new one
	attachOnly bool

	// PTY (wraps tmux attach)
	ptmx *os.File
	cmd  *exec.Cmd

	// Socket server
	listener net.Listener

	// Connected clients
	clients   map[net.Conn]struct{}
	clientsMu sync.RWMutex

	// Synchronization
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	exitCode int
	exitErr  error

	// Metadata
	metadata *SessionMetadata

	// Hooks configuration file path (for cleanup)
	hooksPath string
}

// NewMultiplexer creates a new PTY multiplexer for the given command.
func NewMultiplexer(command string, args []string) *Multiplexer {
	return &Multiplexer{
		command: command,
		args:    args,
		clients: make(map[net.Conn]struct{}),
	}
}

// NewMultiplexerWithName creates a new PTY multiplexer with a custom session name.
func NewMultiplexerWithName(command string, args []string, sessionName string) *Multiplexer {
	m := NewMultiplexer(command, args)
	m.tmuxSession = sessionName
	return m
}

// NewMultiplexerAttach creates a multiplexer that attaches to an existing tmux session
// instead of creating a new one. This is useful for reconnecting to orphaned sessions
// after a restart.
func NewMultiplexerAttach(tmuxSession string) *Multiplexer {
	return &Multiplexer{
		tmuxSession: tmuxSession,
		attachOnly:  true,
		clients:     make(map[net.Conn]struct{}),
	}
}

// GenerateSessionName creates a human-readable session name based on the current directory
// and command. The name follows stapler-squad's naming convention for external sessions.
// Returns a name like "staplersquad_ext_myproject_claude_1234" where 1234 is the PID.
// Uses PID instead of timestamp to guarantee uniqueness across concurrent sessions.
func GenerateSessionName(command string) string {
	// Get the current directory name as the project identifier
	cwd, err := os.Getwd()
	projectName := "session"
	if err == nil && cwd != "" {
		projectName = filepath.Base(cwd)
	}

	// Sanitize the project name for tmux (replace invalid characters)
	projectName = sanitizeForTmux(projectName)

	// Get the command base name
	cmdBase := filepath.Base(command)
	cmdBase = sanitizeForTmux(cmdBase)

	// Use PID for unique suffix - guaranteed unique among running processes
	// This prevents collisions when starting multiple sessions in the same directory
	pid := os.Getpid()

	return fmt.Sprintf("staplersquad_ext_%s_%s_%d", projectName, cmdBase, pid)
}

// sanitizeForTmux removes or replaces characters invalid for tmux session names.
// tmux session names cannot contain "." or ":" and should avoid special characters.
func sanitizeForTmux(name string) string {
	// Replace invalid characters with underscores
	result := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z':
			result = append(result, c)
		case c >= 'A' && c <= 'Z':
			result = append(result, c)
		case c >= '0' && c <= '9':
			result = append(result, c)
		case c == '-' || c == '_':
			result = append(result, c)
		default:
			// Replace with underscore, but don't add consecutive underscores
			if len(result) == 0 || result[len(result)-1] != '_' {
				result = append(result, '_')
			}
		}
	}
	// Trim trailing underscores
	for len(result) > 0 && result[len(result)-1] == '_' {
		result = result[:len(result)-1]
	}
	// Limit length
	if len(result) > 30 {
		result = result[:30]
	}
	if len(result) == 0 {
		return "session"
	}
	return string(result)
}

// Start launches the command in a tmux session and starts the socket server.
// The tmux session uses stapler-squad's naming convention (staplersquad_ext_<PID>)
// so stapler-squad can discover and control it.
func (m *Multiplexer) Start() error {
	// Create context for coordination
	m.ctx, m.cancel = context.WithCancel(context.Background())

	// Create socket path using OS-specific temp directory
	// On macOS this is /var/folders/.../T/, on Linux it's /tmp
	m.socketPath = filepath.Join(os.TempDir(), fmt.Sprintf("claude-mux-%d.sock", os.Getpid()))

	// Remove any stale socket
	os.Remove(m.socketPath)

	// Create Unix domain socket listener
	listener, err := net.Listen("unix", m.socketPath)
	if err != nil {
		return fmt.Errorf("failed to create socket listener: %w", err)
	}
	m.listener = listener

	// Make socket readable by owner only (security)
	if err := os.Chmod(m.socketPath, 0600); err != nil {
		m.listener.Close()
		os.Remove(m.socketPath)
		return fmt.Errorf("failed to set socket permissions: %w", err)
	}

	// Get current working directory for metadata
	cwd, _ := os.Getwd()

	// If attach-only mode, verify the tmux session exists
	if m.attachOnly {
		if m.tmuxSession == "" {
			m.listener.Close()
			os.Remove(m.socketPath)
			return fmt.Errorf("attach mode requires a tmux session name")
		}
		// Check if session exists
		checkCmd := exec.Command("tmux", "has-session", "-t", m.tmuxSession)
		if err := checkCmd.Run(); err != nil {
			m.listener.Close()
			os.Remove(m.socketPath)
			return fmt.Errorf("tmux session not found: %s", m.tmuxSession)
		}
	} else {
		// Create tmux session with stapler-squad naming convention
		// "ext_" distinguishes external sessions from squad-created ones
		// Use pre-set name if provided, otherwise generate a descriptive one
		if m.tmuxSession == "" {
			m.tmuxSession = GenerateSessionName(m.command)
		}

		// Generate hooks configuration file for Claude Code integration
		// This enables notifications to be sent to stapler-squad with proper session context
		hooksMeta := &HooksMetadata{
			SocketPath:  m.socketPath,
			TmuxSession: m.tmuxSession,
			PID:         os.Getpid(),
			Cwd:         cwd,
			Command:     m.command,
		}
		hooksPath, hooksErr := GenerateHooksFile(hooksMeta)
		if hooksErr != nil {
			// Log warning but continue - hooks are optional enhancement
			// Don't fail session creation just because hooks couldn't be generated
			fmt.Fprintf(os.Stderr, "Warning: Failed to generate hooks file: %v\n", hooksErr)
		} else {
			m.hooksPath = hooksPath
		}

		// Build the full command string for tmux
		// If hooks were generated, wrap the command to set CLAUDE_CODE_HOOKS_PATH
		fullCmd := m.command
		if len(m.args) > 0 {
			// Quote args that contain spaces
			for _, arg := range m.args {
				fullCmd += " " + arg
			}
		}

		// Inject hooks environment variable if hooks file was created
		if m.hooksPath != "" {
			// Wrap command with env to set CLAUDE_CODE_HOOKS_PATH
			// Also set CS_SESSION_ID to the tmux session name for proper correlation
			fullCmd = fmt.Sprintf("env CLAUDE_CODE_HOOKS_PATH=%q CS_SESSION_ID=%q CS_MUX_SOCKET_PATH=%q %s",
				m.hooksPath, m.tmuxSession, m.socketPath, fullCmd)
		}

		// Start tmux session with the command (detached)
		startCmd := exec.Command("tmux", "new-session", "-d", "-s", m.tmuxSession, "-c", cwd, fullCmd)
		if err := startCmd.Run(); err != nil {
			// Clean up hooks file on failure
			if m.hooksPath != "" {
				CleanupHooksFile(m.hooksPath)
			}
			m.listener.Close()
			os.Remove(m.socketPath)
			return fmt.Errorf("failed to start tmux session: %w", err)
		}
	}

	// Attach to the tmux session - this is what we wrap in PTY for the local terminal
	m.cmd = exec.Command("tmux", "attach-session", "-t", m.tmuxSession)

	// Start attach command in PTY
	ptmx, err := pty.Start(m.cmd)
	if err != nil {
		// Clean up tmux session if attach fails (but not in attach-only mode)
		if !m.attachOnly {
			exec.Command("tmux", "kill-session", "-t", m.tmuxSession).Run()
		}
		m.listener.Close()
		os.Remove(m.socketPath)
		return fmt.Errorf("failed to attach to tmux session: %w", err)
	}
	m.ptmx = ptmx

	// Create metadata
	command := m.command
	if m.attachOnly {
		command = "(reattached)"
	}
	startTime := time.Now().Unix()
	m.metadata = &SessionMetadata{
		Command:     command,
		Args:        m.args,
		PID:         m.cmd.Process.Pid,
		Cwd:         cwd,
		SocketPath:  m.socketPath,
		StartTime:   startTime,
		Env:         getRelevantEnv(),
		TmuxSession: m.tmuxSession, // Include tmux session name for stapler-squad adoption
	}

	// Write metadata to tmux user options so the server can discover this session
	// without probing the socket (single list-sessions call, survives restarts).
	// Non-fatal: socket-based discovery remains a fallback.
	if err := WriteSessionUserOptions(m.tmuxSession, m.socketPath, cwd, command, os.Getpid(), startTime); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to write tmux user options: %v\n", err)
	}

	// Start goroutines
	m.wg.Add(4)
	go m.acceptClients()
	go m.forwardPTYOutput()
	go m.forwardStdinToPTY()
	go m.monitorTmuxSession()

	return nil
}

// SocketPath returns the path to the Unix domain socket.
func (m *Multiplexer) SocketPath() string {
	return m.socketPath
}

// Wait waits for the multiplexer to finish and returns the exit code.
func (m *Multiplexer) Wait() (int, error) {
	// Wait for command to exit
	err := m.cmd.Wait()

	// Record exit status
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			m.exitCode = exitErr.ExitCode()
		} else {
			m.exitErr = err
			m.exitCode = 1
		}
	}

	// Signal shutdown
	m.cancel()

	// Close PTY (will cause forwardPTYOutput to exit)
	if m.ptmx != nil {
		m.ptmx.Close()
	}

	// Close listener (will cause acceptClients to exit)
	if m.listener != nil {
		m.listener.Close()
	}

	// Close all client connections
	m.clientsMu.Lock()
	for conn := range m.clients {
		conn.Close()
	}
	m.clientsMu.Unlock()

	// Wait for goroutines
	m.wg.Wait()

	// Clean up socket
	os.Remove(m.socketPath)

	// Clean up hooks file
	if m.hooksPath != "" {
		CleanupHooksFile(m.hooksPath)
		m.hooksPath = ""
	}

	return m.exitCode, m.exitErr
}

// Shutdown gracefully shuts down the multiplexer.
func (m *Multiplexer) Shutdown() {
	// Cancel context
	if m.cancel != nil {
		m.cancel()
	}

	// Kill the process if it's still running
	if m.cmd != nil && m.cmd.Process != nil {
		m.cmd.Process.Signal(syscall.SIGTERM)
		// Give it a moment to exit gracefully
		time.Sleep(100 * time.Millisecond)
		m.cmd.Process.Kill()
	}

	// Kill tmux session on cleanup (user closed their terminal)
	// In attach-only mode, preserve the session so it can be reattached later
	if m.tmuxSession != "" && !m.attachOnly {
		exec.Command("tmux", "kill-session", "-t", m.tmuxSession).Run()
	}

	// Clean up hooks file
	if m.hooksPath != "" {
		CleanupHooksFile(m.hooksPath)
		m.hooksPath = ""
	}
}

// TmuxSessionName returns the tmux session name for this multiplexer.
// This allows stapler-squad to adopt and control the session.
func (m *Multiplexer) TmuxSessionName() string {
	return m.tmuxSession
}

// CapturePane captures clean terminal content from the tmux session.
// This provides a snapshot without ANSI escape sequences for reliable pattern matching.
func (m *Multiplexer) CapturePane() ([]byte, error) {
	if m.tmuxSession == "" {
		return nil, fmt.Errorf("tmux session not initialized")
	}
	output, err := exec.Command("tmux", "capture-pane", "-t", m.tmuxSession, "-p").Output()
	if err != nil {
		return nil, fmt.Errorf("failed to capture pane: %w", err)
	}
	return output, nil
}

// SetWindowSize sets the PTY window size.
func (m *Multiplexer) SetWindowSize(cols, rows uint16) error {
	if m.ptmx == nil {
		return fmt.Errorf("PTY not initialized")
	}
	return pty.Setsize(m.ptmx, &pty.Winsize{
		Rows: rows,
		Cols: cols,
	})
}

// acceptClients accepts new client connections on the Unix socket.
func (m *Multiplexer) acceptClients() {
	defer m.wg.Done()

	for {
		conn, err := m.listener.Accept()
		if err != nil {
			select {
			case <-m.ctx.Done():
				return
			default:
				// Log error but continue accepting
				continue
			}
		}

		// Register client
		m.clientsMu.Lock()
		m.clients[conn] = struct{}{}
		m.clientsMu.Unlock()

		// Handle client in separate goroutine
		m.wg.Add(1)
		go m.handleClient(conn)
	}
}

// handleClient handles a single client connection.
func (m *Multiplexer) handleClient(conn net.Conn) {
	defer m.wg.Done()
	defer func() {
		m.clientsMu.Lock()
		delete(m.clients, conn)
		m.clientsMu.Unlock()
		conn.Close()
	}()

	// Send metadata to client
	metaMsg, err := NewMetadataMessage(m.metadata)
	if err == nil {
		WriteMessage(conn, metaMsg)
	}

	// Read messages from client
	for {
		select {
		case <-m.ctx.Done():
			return
		default:
		}

		// Set read deadline to allow checking context
		conn.SetReadDeadline(time.Now().Add(1 * time.Second))

		msg, err := DecodeMessage(conn)
		if err != nil {
			// Check for timeout errors - can come in multiple forms due to error wrapping:
			// 1. Direct net.Error with Timeout()
			// 2. errors.As for wrapped net.Error
			// 3. os.ErrDeadlineExceeded
			// 4. io.ErrUnexpectedEOF from partial reads during timeout
			// 5. Error message contains timeout indicators
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				continue // Timeout, check context and retry
			}
			if errors.Is(err, os.ErrDeadlineExceeded) {
				continue
			}
			if errors.Is(err, io.ErrUnexpectedEOF) {
				continue // Partial read during timeout - not a real disconnect
			}
			// Fallback: check error message for timeout indicators
			errStr := err.Error()
			if strings.Contains(errStr, "i/o timeout") || strings.Contains(errStr, "deadline exceeded") {
				continue
			}
			return // Connection closed or error
		}

		// Handle message
		switch msg.Type {
		case MessageTypeInput:
			// Forward input to PTY
			if m.ptmx != nil {
				m.ptmx.Write(msg.Data)
			}

		case MessageTypeResize:
			// Resize PTY
			if resize, err := ParseResizeMessage(msg); err == nil {
				m.SetWindowSize(resize.Cols, resize.Rows)
			}

		case MessageTypePing:
			// Respond with pong
			WriteMessage(conn, NewPongMessage())

		case MessageTypeSnapshot:
			// Client requests clean screen snapshot
			content, err := m.CapturePane()
			if err != nil {
				WriteMessage(conn, NewSnapshotReplyMessage(nil))
			} else {
				WriteMessage(conn, NewSnapshotReplyMessage(content))
			}

		case MessageTypeClose:
			// Client wants to disconnect
			return
		}
	}
}

// forwardPTYOutput reads from PTY and sends to stdout + all clients.
func (m *Multiplexer) forwardPTYOutput() {
	defer m.wg.Done()

	buf := make([]byte, 4096)
	for {
		select {
		case <-m.ctx.Done():
			return
		default:
		}

		n, err := m.ptmx.Read(buf)
		if err != nil {
			if err != io.EOF {
				// Log error if not expected EOF
			}
			return
		}

		if n > 0 {
			data := buf[:n]

			// Write to stdout (IntelliJ terminal)
			os.Stdout.Write(data)

			// Broadcast to all connected clients
			msg := NewOutputMessage(data)
			m.broadcastToClients(msg)
		}
	}
}

// forwardStdinToPTY reads from stdin and sends to PTY.
func (m *Multiplexer) forwardStdinToPTY() {
	defer m.wg.Done()

	buf := make([]byte, 1024)
	for {
		select {
		case <-m.ctx.Done():
			return
		default:
		}

		n, err := os.Stdin.Read(buf)
		if err != nil {
			if err != io.EOF {
				// Log error if not expected EOF
			}
			return
		}

		if n > 0 {
			m.ptmx.Write(buf[:n])
		}
	}
}

// monitorTmuxSession monitors the tmux session to detect when the subprocess exits.
// When the subprocess completes, it triggers a graceful shutdown of the multiplexer.
func (m *Multiplexer) monitorTmuxSession() {
	defer m.wg.Done()

	// Poll tmux session status every 500ms
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			// Check if the tmux session still exists
			checkCmd := exec.Command("tmux", "has-session", "-t", m.tmuxSession)
			if err := checkCmd.Run(); err != nil {
				// Session no longer exists - subprocess likely exited
				// Trigger shutdown of the multiplexer
				m.Shutdown()
				return
			}

			// Check if there are any running processes in the tmux session
			// This catches cases where the session exists but the command has exited
			listCmd := exec.Command("tmux", "list-panes", "-t", m.tmuxSession, "-F", "#{pane_dead}")
			output, err := listCmd.Output()
			if err == nil && len(output) > 0 {
				// Check if all panes are dead (1 means dead, 0 means alive)
				allDead := true
				for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
					if line == "0" {
						allDead = false
						break
					}
				}
				if allDead {
					// All panes are dead - subprocess has exited
					// Give it a moment for any final output to be captured
					time.Sleep(100 * time.Millisecond)

					// Trigger shutdown
					m.Shutdown()
					return
				}
			}
		}
	}
}

// broadcastToClients sends a message to all connected clients.
func (m *Multiplexer) broadcastToClients(msg *Message) {
	m.clientsMu.RLock()
	defer m.clientsMu.RUnlock()

	encoded, err := EncodeMessage(msg)
	if err != nil {
		return
	}

	for conn := range m.clients {
		// Non-blocking write with timeout
		conn.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))
		conn.Write(encoded)
	}
}

// getRelevantEnv returns selected environment variables for metadata.
func getRelevantEnv() map[string]string {
	relevantVars := []string{"SHELL", "TERM", "USER", "HOME", "LANG", "LC_ALL"}
	env := make(map[string]string)
	for _, key := range relevantVars {
		if val := os.Getenv(key); val != "" {
			env[key] = val
		}
	}
	return env
}

// Run is a convenience function that starts the multiplexer, handles signals, and waits.
func Run(command string, args []string) (int, error) {
	return RunWithName(command, args, "")
}

// RunWithName is like Run but allows specifying a custom session name.
// If name is empty, a descriptive name is generated based on the current directory.
func RunWithName(command string, args []string, sessionName string) (int, error) {
	var m *Multiplexer
	if sessionName != "" {
		// Ensure the name follows our naming convention
		if len(sessionName) < 15 || sessionName[:16] != "staplersquad_ext" {
			sessionName = "staplersquad_ext_" + sanitizeForTmux(sessionName)
		}
		m = NewMultiplexerWithName(command, args, sessionName)
	} else {
		m = NewMultiplexer(command, args)
	}

	if err := m.Start(); err != nil {
		return 1, err
	}

	// Handle terminal resize (SIGWINCH)
	sigwinch := make(chan os.Signal, 1)
	signal.Notify(sigwinch, syscall.SIGWINCH)

	// Handle termination signals
	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGINT, syscall.SIGTERM)

	// Set initial window size from terminal
	if size, err := pty.GetsizeFull(os.Stdin); err == nil {
		m.SetWindowSize(uint16(size.Cols), uint16(size.Rows))
	}

	// Start signal handlers
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			case <-sigwinch:
				if size, err := pty.GetsizeFull(os.Stdin); err == nil {
					m.SetWindowSize(uint16(size.Cols), uint16(size.Rows))
				}
			case <-sigterm:
				m.Shutdown()
				return
			}
		}
	}()

	exitCode, err := m.Wait()
	close(done)

	return exitCode, err
}

// RunAttach attaches to an existing tmux session and creates a streaming socket.
// This is useful for reconnecting to orphaned sessions after a restart.
// The session is NOT killed when detaching, allowing future reattachment.
func RunAttach(tmuxSession string) (int, error) {
	m := NewMultiplexerAttach(tmuxSession)

	if err := m.Start(); err != nil {
		return 1, err
	}

	// Handle terminal resize (SIGWINCH)
	sigwinch := make(chan os.Signal, 1)
	signal.Notify(sigwinch, syscall.SIGWINCH)

	// Handle termination signals
	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGINT, syscall.SIGTERM)

	// Set initial window size from terminal
	if size, err := pty.GetsizeFull(os.Stdin); err == nil {
		m.SetWindowSize(uint16(size.Cols), uint16(size.Rows))
	}

	// Start signal handlers
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			case <-sigwinch:
				if size, err := pty.GetsizeFull(os.Stdin); err == nil {
					m.SetWindowSize(uint16(size.Cols), uint16(size.Rows))
				}
			case <-sigterm:
				m.Shutdown()
				return
			}
		}
	}()

	exitCode, err := m.Wait()
	close(done)

	return exitCode, err
}

// ListStaplerSquadSessions returns a list of tmux sessions that match the stapler-squad naming convention.
// These are sessions that can be attached to using RunAttach.
func ListStaplerSquadSessions() ([]string, error) {
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
	output, err := cmd.Output()
	if err != nil {
		// tmux returns error if no sessions exist
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list tmux sessions: %w", err)
	}

	var sessions []string
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		// Include both new prefix (staplersquad_) and legacy prefix (claudesquad_) for migration compatibility
		if strings.HasPrefix(line, tmux.TmuxPrefix) || strings.HasPrefix(line, tmux.LegacyTmuxPrefix) {
			sessions = append(sessions, line)
		}
	}

	return sessions, nil
}
