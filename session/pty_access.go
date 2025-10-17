package session

import (
	"fmt"
	"os"
	"sync"
)

// PTYAccess provides thread-safe access to a tmux session's PTY for reading and writing.
// It wraps the PTY file descriptor with synchronization primitives to enable concurrent
// access from multiple goroutines (e.g., command execution, response streaming, status monitoring).
type PTYAccess struct {
	mu          sync.RWMutex
	sessionName string
	pty         *os.File
	buffer      *CircularBuffer
	closed      bool
}

// NewPTYAccess creates a new PTYAccess wrapper for a PTY file descriptor.
// The buffer parameter specifies the circular buffer for storing PTY output history.
func NewPTYAccess(sessionName string, pty *os.File, buffer *CircularBuffer) *PTYAccess {
	return &PTYAccess{
		sessionName: sessionName,
		pty:         pty,
		buffer:      buffer,
		closed:      false,
	}
}

// Write writes data to the PTY in a thread-safe manner.
// Returns the number of bytes written and any error encountered.
func (p *PTYAccess) Write(data []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return 0, fmt.Errorf("PTY access for session '%s' is closed", p.sessionName)
	}

	if p.pty == nil {
		return 0, fmt.Errorf("PTY for session '%s' is not initialized", p.sessionName)
	}

	return p.pty.Write(data)
}

// Read reads data from the PTY in a thread-safe manner.
// This is a blocking call that will wait for data to be available.
// Returns the number of bytes read and any error encountered.
func (p *PTYAccess) Read(buf []byte) (int, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed {
		return 0, fmt.Errorf("PTY access for session '%s' is closed", p.sessionName)
	}

	if p.pty == nil {
		return 0, fmt.Errorf("PTY for session '%s' is not initialized", p.sessionName)
	}

	return p.pty.Read(buf)
}

// GetBuffer returns the most recent output from the circular buffer.
// This provides access to historical PTY output without blocking.
// Returns a copy of the buffer contents to prevent concurrent modification issues.
func (p *PTYAccess) GetBuffer() []byte {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.buffer == nil {
		return nil
	}

	return p.buffer.GetAll()
}

// GetRecentOutput returns the last n bytes from the circular buffer.
// This is useful for status detection and response streaming.
func (p *PTYAccess) GetRecentOutput(n int) []byte {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.buffer == nil {
		return nil
	}

	return p.buffer.GetRecent(n)
}

// UpdatePTY updates the underlying PTY file descriptor.
// This is used when the PTY needs to be refreshed (e.g., after detach/reattach).
func (p *PTYAccess) UpdatePTY(pty *os.File) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return fmt.Errorf("cannot update PTY for closed session '%s'", p.sessionName)
	}

	p.pty = pty
	return nil
}

// Close marks the PTY access as closed and prevents further operations.
// It does NOT close the underlying PTY file descriptor - that's handled by the tmux session.
func (p *PTYAccess) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}

	p.closed = true
	return nil
}

// IsClosed returns whether the PTY access has been closed.
func (p *PTYAccess) IsClosed() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.closed
}

// GetSessionName returns the name of the session this PTY access is for.
func (p *PTYAccess) GetSessionName() string {
	return p.sessionName
}
