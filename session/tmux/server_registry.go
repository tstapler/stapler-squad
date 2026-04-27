package tmux

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// ErrRegistryUnavailable is returned when the registry is not healthy and cannot
// serve a request that requires the registry to be up.
var ErrRegistryUnavailable = errors.New("tmux registry unavailable")

// Compile-time interface checks.
var _ SessionExistenceChecker = (*TmuxServerRegistry)(nil)
var _ SessionLister = (*TmuxServerRegistry)(nil)
var _ PaneExitSubscriber = (*TmuxServerRegistry)(nil)
var _ TmuxStatePort = (*TmuxServerRegistry)(nil)

// paneExitSub is a single pane-exit subscriber. sync.Once ensures the channel
// is closed exactly once regardless of which code path (ctx cancel, firePaneExit,
// or Stop) reaches the close first.
type paneExitSub struct {
	ch   chan struct{}
	once sync.Once
}

func (s *paneExitSub) close() { s.once.Do(func() { close(s.ch) }) }

// TmuxServerRegistry maintains a single tmux control-mode connection to a tmux
// server and pushes session-lifecycle events into an in-memory map. Callers
// query the map directly instead of forking tmux subprocesses.
type TmuxServerRegistry struct {
	serverSocket string

	mu       sync.RWMutex
	sessions map[string]bool

	// subsMu guards subscribers. CRITICAL: never close(ch) while holding subsMu.
	// Copy subscribers out under the lock, release the lock, then close outside.
	subsMu      sync.Mutex
	subscribers map[string][]*paneExitSub

	healthMu sync.RWMutex
	healthy  bool

	ctx    context.Context
	cancel context.CancelFunc
}

// NewTmuxServerRegistry creates a new registry for the given server socket.
// Call Start(ctx) to begin listening for events.
func NewTmuxServerRegistry(serverSocket string) *TmuxServerRegistry {
	ctx, cancel := context.WithCancel(context.Background())
	return &TmuxServerRegistry{
		serverSocket: serverSocket,
		sessions:     make(map[string]bool),
		subscribers:  make(map[string][]*paneExitSub),
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Start launches the control-mode process and begins processing events.
// It bootstraps the session map from list-sessions before marking the registry
// healthy. The returned error is non-nil only when the initial setup fails in a
// way that makes a retry impossible.
func (r *TmuxServerRegistry) Start(ctx context.Context) error {
	// Derive a child context so Stop() can cancel without affecting the caller.
	childCtx, childCancel := context.WithCancel(ctx)
	r.healthMu.Lock()
	r.healthy = false
	r.healthMu.Unlock()

	// Replace internal ctx/cancel with the derived pair so Stop() works.
	r.cancel()
	r.ctx = childCtx
	r.cancel = childCancel

	// Bootstrap session map from list-sessions before connecting control mode.
	if err := r.syncSessions(); err != nil {
		log.WarningLog.Printf("[registry] initial list-sessions failed, continuing: %v", err)
	}

	// Launch the auto-reconnect loop; it starts the first control-mode process.
	go r.reconnectLoop()

	return nil
}

// Stop shuts down the registry and closes all pending subscriber channels.
func (r *TmuxServerRegistry) Stop() {
	r.cancel()

	// Copy all subscribers under the lock, then close outside.
	r.subsMu.Lock()
	var allSubs []*paneExitSub
	for _, subs := range r.subscribers {
		allSubs = append(allSubs, subs...)
	}
	r.subscribers = make(map[string][]*paneExitSub)
	r.subsMu.Unlock()

	for _, sub := range allSubs {
		sub.close()
	}

	r.healthMu.Lock()
	r.healthy = false
	r.healthMu.Unlock()
}

// SessionExists implements SessionExistenceChecker.
func (r *TmuxServerRegistry) SessionExists(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.sessions[name]
}

// ListSessions implements SessionLister. Returns a copy of the live sessions map.
func (r *TmuxServerRegistry) ListSessions() map[string]bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]bool, len(r.sessions))
	for k, v := range r.sessions {
		out[k] = v
	}
	return out
}

// IsHealthy implements SessionExistenceChecker and SessionLister.
func (r *TmuxServerRegistry) IsHealthy() bool {
	r.healthMu.RLock()
	defer r.healthMu.RUnlock()
	return r.healthy
}

// SubscribePaneExit implements PaneExitSubscriber. The returned channel is
// closed when the named session/pane exits or when ctx is cancelled.
func (r *TmuxServerRegistry) SubscribePaneExit(ctx context.Context, sessionName string) <-chan struct{} {
	sub := &paneExitSub{ch: make(chan struct{}, 1)}

	r.subsMu.Lock()
	r.subscribers[sessionName] = append(r.subscribers[sessionName], sub)
	r.subsMu.Unlock()

	go func() {
		select {
		case <-ctx.Done():
			// Remove our subscriber from the list; close via Once (safe if
			// firePaneExit already closed it concurrently).
			r.subsMu.Lock()
			existing := r.subscribers[sessionName]
			filtered := existing[:0]
			for _, s := range existing {
				if s != sub {
					filtered = append(filtered, s)
				}
			}
			if len(filtered) == 0 {
				delete(r.subscribers, sessionName)
			} else {
				r.subscribers[sessionName] = filtered
			}
			r.subsMu.Unlock()
			sub.close()
		case <-sub.ch:
			// Closed by firePaneExit or Stop; nothing to do.
		case <-r.ctx.Done():
			// Registry is stopping; channel will be closed by Stop().
		}
	}()

	return sub.ch
}

// firePaneExit closes all subscriber channels for sessionName. It copies the
// subscribers out under the lock and then closes them outside to prevent deadlock.
func (r *TmuxServerRegistry) firePaneExit(sessionName string) {
	r.subsMu.Lock()
	subs := r.subscribers[sessionName]
	delete(r.subscribers, sessionName)
	r.subsMu.Unlock()
	// Lock NOT held here — close outside the critical section.
	for _, sub := range subs {
		sub.close()
	}
}

// syncSessions runs list-sessions and replaces the in-memory map atomically.
func (r *TmuxServerRegistry) syncSessions() error {
	args := prependSocket(r.serverSocket, []string{"list-sessions", "-F", "#{session_name}"})
	cmd := exec.Command("tmux", args...)
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("list-sessions: %w", err)
	}

	sessions := make(map[string]bool)
	for _, name := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if name != "" {
			sessions[name] = true
		}
	}

	// Identify sessions that existed before but are now gone. We fire
	// pane-exit for them after releasing the lock so subscribers are
	// notified even when control-mode events were missed (e.g. in a
	// headless environment where the control-mode connection is short-lived).
	r.mu.Lock()
	var disappeared []string
	for name := range r.sessions {
		if !sessions[name] {
			disappeared = append(disappeared, name)
		}
	}
	r.sessions = sessions
	r.mu.Unlock()

	for _, name := range disappeared {
		r.firePaneExit(name)
	}

	return nil
}

// startControlMode ensures the keepalive sentinel session exists, then launches
// "tmux [-L socket] -C attach-session -t keepalive" and returns the command
// along with a scanner for its stdout. It does NOT block.
//
// IMPORTANT: tmux control-mode exits with %exit when it reads EOF on stdin.
// We must create a stdin pipe and hold it open for the lifetime of the process.
// The returned io.WriteCloser is the stdin pipe; Close() it to signal shutdown.
func (r *TmuxServerRegistry) startControlMode() (*exec.Cmd, *bufio.Scanner, io.WriteCloser, error) {
	keepaliveName := TmuxPrefix + "keepalive"

	// Only create the keepalive sentinel on the default server (empty socket).
	// Isolated servers (e.g., test harnesses using -L <socket>) manage their own
	// session lifecycle and must not have a keepalive injected into them.
	if r.serverSocket == "" {
		// Ensure the sentinel session exists so attach-session doesn't exit immediately.
		// "new-session -d -s <name>" is idempotent: if the session already exists tmux
		// exits with a non-zero code which we intentionally ignore.
		createArgs := []string{"new-session", "-d", "-s", keepaliveName}
		_ = exec.Command("tmux", createArgs...).Run()
	}

	// No -r flag: read-only is irrelevant for event monitoring, and it caused
	// immediate %exit on some tmux versions.
	baseArgs := []string{"-C", "attach-session", "-t", keepaliveName}
	args := prependSocket(r.serverSocket, baseArgs)
	cmd := exec.Command("tmux", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("StdoutPipe: %w", err)
	}
	// Hold stdin open — tmux sends %exit and terminates when it reads EOF on stdin.
	stdin, err := cmd.StdinPipe()
	if err != nil {
		stdout.Close()
		return nil, nil, nil, fmt.Errorf("StdinPipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		stdout.Close()
		stdin.Close()
		return nil, nil, nil, fmt.Errorf("cmd.Start: %w", err)
	}
	return cmd, bufio.NewScanner(stdout), stdin, nil
}

// reconnectLoop starts the control-mode process and reconnects with exponential
// backoff whenever it exits. It exits when the registry context is cancelled.
func (r *TmuxServerRegistry) reconnectLoop() {
	const (
		backoffBase = 100 * time.Millisecond
		backoffCap  = 30 * time.Second
	)
	backoff := backoffBase

	for {
		select {
		case <-r.ctx.Done():
			return
		default:
		}

		cmd, scanner, stdin, err := r.startControlMode()
		if err != nil {
			log.WarningLog.Printf("[registry] control-mode start failed: %v; retrying in %v", err, backoff)
			select {
			case <-r.ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < backoffCap {
				backoff *= 2
				if backoff > backoffCap {
					backoff = backoffCap
				}
			}
			continue
		}

		// Resync the session map before marking healthy so that sessions
		// created while the control-mode connection was down are not missed.
		if err := r.syncSessions(); err != nil {
			log.WarningLog.Printf("[registry] syncSessions on reconnect failed: %v", err)
		}

		// Mark healthy now that the control-mode process is running.
		r.healthMu.Lock()
		r.healthy = true
		r.healthMu.Unlock()

		// Yield so that other goroutines can observe the healthy state before
		// readLines processes the first event (which may immediately clear it).
		runtime.Gosched()

		log.InfoLog.Printf("[registry] control-mode connected (socket=%q)", r.serverSocket)

		connectTime := time.Now()

		// readLines blocks until the process exits or the context is cancelled.
		r.readLines(scanner)

		// Only reset backoff if the connection was stable for a meaningful
		// duration. Resetting on a connection that dies immediately (e.g. tmux
		// server unhealthy, keepalive session missing) would prevent exponential
		// backoff from protecting against fork-rate explosion.
		const minStableConnection = 5 * time.Second
		if time.Since(connectTime) >= minStableConnection {
			backoff = backoffBase
		}

		// Closing stdin signals tmux to exit cleanly (it sends %exit on EOF).
		stdin.Close()
		// Clean up the process.
		_ = cmd.Process.Kill()
		_ = cmd.Wait()

		r.healthMu.Lock()
		r.healthy = false
		r.healthMu.Unlock()

		select {
		case <-r.ctx.Done():
			return
		default:
			log.InfoLog.Printf("[registry] control-mode exited; reconnecting in %v", backoff)
			select {
			case <-r.ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < backoffCap {
				backoff *= 2
				if backoff > backoffCap {
					backoff = backoffCap
				}
			}
		}
	}
}

// debounce state for %sessions-changed handling.
var (
	debounceTimer *time.Timer
	debounceMu    sync.Mutex
	debounceDelay = 50 * time.Millisecond
)

// readLines processes control-mode event lines from scanner until the scanner
// returns false (process exited) or the registry context is cancelled.
func (r *TmuxServerRegistry) readLines(scanner *bufio.Scanner) {
	for scanner.Scan() {
		select {
		case <-r.ctx.Done():
			return
		default:
		}
		line := scanner.Text()
		r.handleEvent(line)
	}
}

// handleEvent parses a single control-mode notification line and updates state.
func (r *TmuxServerRegistry) handleEvent(line string) {
	switch {
	case strings.HasPrefix(line, "%session-created "):
		// %session-created $ID <name>
		parts := strings.Fields(line)
		if len(parts) >= 3 {
			name := parts[2]
			r.mu.Lock()
			r.sessions[name] = true
			r.mu.Unlock()
			log.InfoLog.Printf("[registry] session created: %q", name)
		}

	case strings.HasPrefix(line, "%session-closed "):
		// %session-closed $ID <name>
		parts := strings.Fields(line)
		if len(parts) >= 3 {
			name := parts[2]
			r.mu.Lock()
			delete(r.sessions, name)
			r.mu.Unlock()
			log.InfoLog.Printf("[registry] session closed: %q", name)
			r.firePaneExit(name)
		}

	case strings.HasPrefix(line, "%sessions-changed"):
		// Debounce: wait 50ms then sync from list-sessions.
		debounceMu.Lock()
		if debounceTimer != nil {
			debounceTimer.Stop()
		}
		debounceTimer = time.AfterFunc(debounceDelay, func() {
			if err := r.syncSessions(); err != nil {
				log.WarningLog.Printf("[registry] sync after sessions-changed failed: %v", err)
			}
		})
		debounceMu.Unlock()

	case strings.HasPrefix(line, "%pane-exited "):
		// %pane-exited ... -t <session-name>
		// Parse the target from the line; look for the token after "-t".
		parts := strings.Fields(line)
		for i, part := range parts {
			if part == "-t" && i+1 < len(parts) {
				target := parts[i+1]
				// Target may be "session:window.pane" — extract session name.
				if idx := strings.Index(target, ":"); idx >= 0 {
					target = target[:idx]
				}
				r.firePaneExit(target)
				break
			}
		}

	case strings.HasPrefix(line, "%exit"):
		// Server is going away.
		r.healthMu.Lock()
		r.healthy = false
		r.healthMu.Unlock()

	default:
		// Unknown event — ignore, no panic.
	}
}

// GetServerRegistry returns the singleton TmuxServerRegistry for the given
// socket. Creates and starts the registry on first call for each socket.
// Never call from init().
func GetServerRegistry(socket string) *TmuxServerRegistry {
	globalRegistryMu.Lock()
	defer globalRegistryMu.Unlock()

	if r, ok := globalRegistries[socket]; ok {
		return r
	}

	r := NewTmuxServerRegistry(socket)
	_ = r.Start(context.Background())
	globalRegistries[socket] = r
	return r
}

// StopServerRegistry stops and removes the registry for the given socket.
// Safe to call even if no registry was ever created for the socket.
// After this call, GetServerRegistry(socket) will create a fresh registry.
// Intended for test cleanup to prevent reconnectLoop from restarting a
// tmux server after it has been killed.
func StopServerRegistry(socket string) {
	globalRegistryMu.Lock()
	r, ok := globalRegistries[socket]
	if ok {
		delete(globalRegistries, socket)
	}
	globalRegistryMu.Unlock()
	if r != nil {
		r.Stop()
	}
}

// globalRegistryMu guards globalRegistries.
var globalRegistryMu sync.Mutex

// globalRegistries holds one registry per socket string.
var globalRegistries = make(map[string]*TmuxServerRegistry)
