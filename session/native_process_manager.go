//go:build !windows

package session

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/log"
)

// NativeProcessManager implements ProcessManager using a raw PTY and process supervision.
// It launches the configured program directly under a PTY master fd (via creack/pty)
// and restarts it with exponential backoff when it exits unexpectedly.
//
// Phase 2 implementation: Start(), Close(), IsAlive(), GetPTY(), GetPanePID(),
// GetSessionIdentifier(), SetWindowSize(), GetPaneDimensions(), SendKeys(),
// TapEnter(), SetOnExitCallback(), SubscribeToControlModeUpdates(), and
// GetCurrentWorkingDirectory() are fully functional.
// Content capture (CapturePaneContent variants) and precise CWD via lsof/proc
// are deferred to Phase 3.
type NativeProcessManager struct {
	opts ProcessManagerOptions

	mu       sync.Mutex
	ptm      *os.File      // PTY master fd; nil when not started
	cmd      *exec.Cmd     // supervised process; nil when not started
	stopCh   chan struct{} // closed by Close(); signals supervise() to stop
	startDir string        // directory passed to the most recent Start() call

	lastSize *pty.Winsize // tracks last window size for reapplication on restart

	onExitCallback func(string)

	// subscriber fan-out for streaming
	subsMu sync.Mutex
	subs   map[string]PTYSubscriber
}

// NewNativeProcessManager creates a NativeProcessManager with the given options.
// Call Start() to launch the process.
func NewNativeProcessManager(opts ProcessManagerOptions) *NativeProcessManager {
	return &NativeProcessManager{
		opts:   opts,
		stopCh: make(chan struct{}),
		subs:   make(map[string]PTYSubscriber),
	}
}

// compile-time check that *NativeProcessManager satisfies ProcessManager.
var _ ProcessManager = (*NativeProcessManager)(nil)

// --- Lifecycle ---

// Start launches the configured program under a PTY in the given directory.
// If the process is already running, Start is a no-op.
// Start resets the stop signal so it is safe to call after Close().
func (n *NativeProcessManager) Start(dir string) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Double-checked locking: skip if already alive.
	if n.cmd != nil && n.cmd.ProcessState == nil {
		return nil
	}

	// Reset stop signal if Close() was previously called, to allow restart.
	select {
	case <-n.stopCh:
		n.stopCh = make(chan struct{})
	default:
	}

	if err := n.launchPTY(dir); err != nil {
		return err
	}
	go n.supervise(dir)
	return nil
}

// launchPTY creates and starts the PTY process. Caller must hold n.mu.
func (n *NativeProcessManager) launchPTY(dir string) error {
	program := n.opts.Program
	if program == "" {
		program = "bash"
	}

	cmd := safeexec.CommandContext(context.Background(), program, n.opts.Args...)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	ptm, err := pty.Start(cmd)
	if err != nil {
		return fmt.Errorf("NativeProcessManager: pty.Start failed: %w", err)
	}

	// Reapply last known window size so restarted processes inherit the correct dimensions.
	if n.lastSize != nil {
		_ = pty.Setsize(ptm, n.lastSize)
	}

	n.cmd = cmd
	n.ptm = ptm
	n.startDir = dir

	go n.fanOut(ptm)

	return nil
}

// supervise waits for the process to exit and restarts it unless stopCh is closed.
// Implements NM-1 (cmd.Wait() is the sole owner), NM-5 (exits on stopCh close).
func (n *NativeProcessManager) supervise(dir string) {
	const initialBackoff = 500 * time.Millisecond
	const maxBackoff = 30 * time.Second
	backoff := initialBackoff

	for {
		// Snapshot cmd under lock to avoid a race with concurrent Start() calls.
		n.mu.Lock()
		cmd := n.cmd
		n.mu.Unlock()

		if cmd == nil {
			return
		}

		// Wait for the process to exit — NM-1: sole Wait() caller, prevents zombies.
		_ = cmd.Wait()

		// Check whether Close() has been called.
		select {
		case <-n.stopCh:
			return // intentional stop; do not restart
		default:
		}

		// Fire the exit callback so the instance layer can publish an EventExited.
		// Snapshot under lock to avoid a race with SetOnExitCallback.
		n.mu.Lock()
		cb := n.onExitCallback
		n.mu.Unlock()
		if cb != nil {
			cb("crash: process exited unexpectedly")
		}

		// Exponential backoff before relaunching.
		select {
		case <-n.stopCh:
			return
		case <-time.After(backoff):
		}
		if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}

		// Relaunch under lock.
		n.mu.Lock()
		// Verify stop hasn't been requested while we waited.
		select {
		case <-n.stopCh:
			n.mu.Unlock()
			return
		default:
		}
		if err := n.launchPTY(dir); err != nil {
			log.Error("NativeProcessManager: relaunch failed", "session", n.opts.SessionName, "err", err)
			// Clear stale cmd so the next loop iteration hits cmd==nil and exits cleanly
			// rather than calling Wait() on an already-exited process repeatedly.
			n.cmd = nil
		}
		n.mu.Unlock()
	}
}

// Close terminates the supervised process and stops the restart loop.
// Implements NM-3 (SIGTERM before context cancel) and NM-5 (goroutines exit).
func (n *NativeProcessManager) Close() error {
	// Signal the restart loop to stop before killing the process so that
	// the supervise() goroutine sees the closed channel and does not restart.
	select {
	case <-n.stopCh:
		// Already closed; no-op.
	default:
		close(n.stopCh)
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	if n.cmd == nil || n.cmd.Process == nil {
		return nil
	}

	// NM-3: send SIGTERM to the process group before any other teardown.
	_ = syscall.Kill(-n.cmd.Process.Pid, syscall.SIGTERM)

	// Close the PTY master fd; this also breaks the fanOut goroutine's Read loop.
	if n.ptm != nil {
		_ = n.ptm.Close()
		n.ptm = nil
	}

	return nil
}

// IsAlive reports whether the supervised process is currently running.
func (n *NativeProcessManager) IsAlive() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.cmd != nil && n.cmd.Process != nil && n.cmd.ProcessState == nil
}

// HasSession reports whether a process has been started at least once.
// Alias for IsAlive() on the native backend.
func (n *NativeProcessManager) HasSession() bool {
	return n.IsAlive()
}

// RestoreWithWorkDir is a no-op for the native backend; the process is already
// running after Start() and does not need re-attachment.
func (n *NativeProcessManager) RestoreWithWorkDir(_ string) error {
	return nil
}

// --- Identification ---

// GetSessionIdentifier returns the stable session name set at construction.
func (n *NativeProcessManager) GetSessionIdentifier() string {
	return n.opts.SessionName
}

// --- PTY I/O ---

// GetPTY returns the PTY master file descriptor.
func (n *NativeProcessManager) GetPTY() (*os.File, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.ptm == nil {
		return nil, fmt.Errorf("NativeProcessManager: PTY not allocated (call Start first)")
	}
	return n.ptm, nil
}

// SendKeys writes the given string to the PTY master.
func (n *NativeProcessManager) SendKeys(keys string) (int, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.ptm == nil {
		return 0, fmt.Errorf("NativeProcessManager: PTY not initialized")
	}
	return n.ptm.Write([]byte(keys))
}

// TapEnter sends a carriage return + newline sequence to the PTY.
func (n *NativeProcessManager) TapEnter() error {
	_, err := n.SendKeys("\r\n")
	return err
}

// SendPromptWithEnter sends text followed by Enter.
func (n *NativeProcessManager) SendPromptWithEnter(prompt string) error {
	if _, err := n.SendKeys(prompt); err != nil {
		return fmt.Errorf("NativeProcessManager: SendKeys failed: %w", err)
	}
	time.Sleep(100 * time.Millisecond)
	return n.TapEnter()
}

// SendInputViaControlMode writes raw bytes directly to the PTY master.
// The native backend has no concept of tmux control mode; bytes are written directly.
func (n *NativeProcessManager) SendInputViaControlMode(_ context.Context, data []byte) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.ptm == nil {
		return fmt.Errorf("NativeProcessManager: PTY not initialized")
	}
	_, err := n.ptm.Write(data)
	return err
}

// --- Window size ---

// SetWindowSize resizes the PTY to the given columns and rows.
func (n *NativeProcessManager) SetWindowSize(cols, rows int) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	size := &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)}
	n.lastSize = size
	if n.ptm == nil {
		return nil // Store size for application on next Start().
	}
	return pty.Setsize(n.ptm, size)
}

// GetPaneDimensions returns the last window size set via SetWindowSize.
// Tracks the value in memory to avoid a TIOCGWINSZ syscall on the hot path
// (GetPaneDimensions is called 5× per resize event in connectrpc_websocket.go).
func (n *NativeProcessManager) GetPaneDimensions() (width, height int, err error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.lastSize == nil {
		return 0, 0, nil
	}
	return int(n.lastSize.Cols), int(n.lastSize.Rows), nil
}

// SetDetachedSize updates the stored window size without requiring an active PTY.
// The instanceTitle parameter is ignored; it exists only for interface compatibility.
func (n *NativeProcessManager) SetDetachedSize(width, height int, _ string) error {
	return n.SetWindowSize(width, height)
}

// RefreshClient is a no-op for the native backend (no tmux client to refresh).
func (n *NativeProcessManager) RefreshClient() error {
	return nil
}

// --- Process metadata ---

// GetPanePID returns the PID of the supervised process.
func (n *NativeProcessManager) GetPanePID() (int32, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.cmd == nil || n.cmd.Process == nil {
		return 0, fmt.Errorf("NativeProcessManager: process not started")
	}
	return int32(n.cmd.Process.Pid), nil //nolint:gosec // PID fits in int32 on all supported platforms
}

// GetCursorPosition returns (0, 0) for the native backend.
// There are zero callers in the server/ package that require real cursor position
// from the native backend (confirmed in plan.md).
func (n *NativeProcessManager) GetCursorPosition() (x, y int, err error) {
	return 0, 0, nil
}

// --- Exit notifications ---

// SetOnExitCallback registers a callback invoked when the supervised process exits
// unexpectedly (before the restart loop relaunches it).
func (n *NativeProcessManager) SetOnExitCallback(fn func(string)) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.onExitCallback = fn
}

// ResetExitOnce is a no-op for the native backend; the restart loop does not use
// a sync.Once guard.
func (n *NativeProcessManager) ResetExitOnce() {}

// --- Content helpers (stubs — Phase 2 follow-on) ---

// CapturePaneContent returns an empty string until scrollback capture is implemented.
func (n *NativeProcessManager) CapturePaneContent() (string, error) {
	return "", nil
}

// CapturePaneContentRaw returns an empty string until scrollback capture is implemented.
func (n *NativeProcessManager) CapturePaneContentRaw() (string, error) {
	return "", nil
}

// CapturePaneContentWithOptions returns an empty string until scrollback capture is implemented.
func (n *NativeProcessManager) CapturePaneContentWithOptions(_, _ string) (string, error) {
	return "", nil
}

// CaptureViewport returns an empty string until scrollback capture is implemented.
func (n *NativeProcessManager) CaptureViewport(_ int) (string, error) {
	return "", nil
}

// HasUpdated always returns (false, false, "") until content diffing is implemented.
func (n *NativeProcessManager) HasUpdated() (updated bool, hasPrompt bool, content string) {
	return false, false, ""
}

// FilterBanners returns content unchanged; banner detection is tmux-specific.
func (n *NativeProcessManager) FilterBanners(content string) (string, int) {
	return content, 0
}

// HasMeaningfulContent always returns false until content analysis is implemented.
func (n *NativeProcessManager) HasMeaningfulContent(_ string) bool {
	return false
}

// GetCurrentWorkingDirectory returns the directory passed to the most recent Start() call.
// Phase 3 follow-on: replace with /proc/<pid>/cwd on Linux or lsof on macOS for the
// true current working directory of the running process.
func (n *NativeProcessManager) GetCurrentWorkingDirectory() (string, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.startDir, nil
}

// --- Control mode (streaming) ---

// StartControlMode is a no-op for the native backend; raw PTY reads replace control mode.
func (n *NativeProcessManager) StartControlMode() error {
	return nil
}

// StopControlMode is a no-op for the native backend.
func (n *NativeProcessManager) StopControlMode() error {
	return nil
}

// SubscribeToControlModeUpdates adds a subscriber that receives raw PTY output bytes.
// Returns the subscription ID and a channel that receives byte slices.
func (n *NativeProcessManager) SubscribeToControlModeUpdates() (string, chan []byte) {
	n.subsMu.Lock()
	defer n.subsMu.Unlock()

	id := fmt.Sprintf("native-%d", time.Now().UnixNano())
	sub := newMemPTYSubscriber()
	n.subs[id] = sub
	return id, sub.ch
}

// UnsubscribeFromControlModeUpdates removes a subscriber by ID and closes its channel.
func (n *NativeProcessManager) UnsubscribeFromControlModeUpdates(id string) {
	n.subsMu.Lock()
	defer n.subsMu.Unlock()

	if sub, ok := n.subs[id]; ok {
		sub.Close()
		delete(n.subs, id)
	}
}

// fanOut reads raw bytes from ptm and broadcasts them to all active subscribers.
// One fanOut goroutine is started per launchPTY call. When the supervised process exits,
// the kernel closes the slave PTY fd and subsequent reads on the master return EIO,
// causing fanOut to return naturally — no goroutine leak across restarts.
// Subscribers that exceed their buffer capacity are closed and removed rather than
// having frames dropped — dropping any byte corrupts ANSI/cursor state for terminal consumers.
func (n *NativeProcessManager) fanOut(ptm *os.File) {
	buf := make([]byte, 4096)
	for {
		nr, err := ptm.Read(buf)
		if err != nil {
			return
		}
		data := buf[:nr]

		n.subsMu.Lock()
		for id, sub := range n.subs {
			if err := sub.Push(data); err != nil {
				sub.Close()
				delete(n.subs, id)
				log.Warn("NativeProcessManager: subscriber removed due to full buffer",
					"session", n.opts.SessionName, "subscriber", id)
			}
		}
		n.subsMu.Unlock()
	}
}

// --- Attach ---

// Attach is not supported for the native backend; returns an error.
// Interactive TUI attach requires a proper terminal multiplexer.
func (n *NativeProcessManager) Attach() (chan struct{}, error) {
	return nil, fmt.Errorf("NativeProcessManager: Attach() not supported; use terminal emulator directly")
}

// DetachSafely is a no-op for the native backend.
func (n *NativeProcessManager) DetachSafely() error {
	return nil
}
