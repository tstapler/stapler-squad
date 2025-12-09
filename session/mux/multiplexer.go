package mux

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// Multiplexer handles PTY multiplexing for external Claude sessions.
// It creates a PTY, runs a command on it, and allows multiple clients
// to connect via Unix domain socket for bidirectional terminal access.
type Multiplexer struct {
	// Configuration
	command    string
	args       []string
	socketPath string

	// PTY
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
}

// NewMultiplexer creates a new PTY multiplexer for the given command.
func NewMultiplexer(command string, args []string) *Multiplexer {
	return &Multiplexer{
		command: command,
		args:    args,
		clients: make(map[net.Conn]struct{}),
	}
}

// Start launches the command in a PTY and starts the socket server.
func (m *Multiplexer) Start() error {
	// Create context for coordination
	m.ctx, m.cancel = context.WithCancel(context.Background())

	// Create socket path
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

	// Create command
	m.cmd = exec.Command(m.command, m.args...)

	// Get current working directory for metadata
	cwd, _ := os.Getwd()

	// Start command in PTY
	ptmx, err := pty.Start(m.cmd)
	if err != nil {
		m.listener.Close()
		os.Remove(m.socketPath)
		return fmt.Errorf("failed to start PTY: %w", err)
	}
	m.ptmx = ptmx

	// Create metadata
	m.metadata = &SessionMetadata{
		Command:    m.command,
		Args:       m.args,
		PID:        m.cmd.Process.Pid,
		Cwd:        cwd,
		SocketPath: m.socketPath,
		StartTime:  time.Now().Unix(),
		Env:        getRelevantEnv(),
	}

	// Start goroutines
	m.wg.Add(3)
	go m.acceptClients()
	go m.forwardPTYOutput()
	go m.forwardStdinToPTY()

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
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue // Timeout, check context and retry
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
	m := NewMultiplexer(command, args)

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
