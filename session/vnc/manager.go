package vnc

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/executor"
	"github.com/tstapler/stapler-squad/log"
)

// VNCProcessManager is the interface satisfied by both the real VNC manager
// (vncProcessManager) and the no-op manager (noopVNCManager). The session
// layer holds this interface so it can be swapped out on non-Linux hosts or
// when required binaries are absent.
type VNCProcessManager interface {
	// StartDisplay allocates an X11 display number and starts Xvfb.
	// Call this BEFORE the tmux session is created so DISPLAY can be injected
	// at new-session time via ExtraEnv. Returns 0 (no-op) on unsupported platforms.
	StartDisplay(ctx context.Context) error

	// StartServer starts x11vnc and the window tracker goroutine.
	// Call this AFTER the tmux session is live.
	StartServer(ctx context.Context) error

	// Start is a convenience wrapper that calls StartDisplay then StartServer.
	// Use only when the two-phase split is not needed (e.g. tests).
	Start(ctx context.Context) error

	// Stop tears down x11vnc and Xvfb and releases the display number.
	// Safe to call if Start was never called or already stopped.
	Stop()
	// State returns a snapshot of the current VNC state.
	State() VNCState
	// DisplayNumber returns the allocated X11 display number (e.g. 100 for :100).
	// Returns 0 if no display has been allocated.
	DisplayNumber() int
	// Port returns the localhost TCP port on which x11vnc is listening.
	// Returns 0 if x11vnc is not running.
	Port() int
	// SetStateChangeCallback registers a callback that is invoked (in a goroutine)
	// each time the VNC state changes. Replaces any previously registered callback.
	SetStateChangeCallback(func(VNCState))
	// ReconcileOrphans scans X11 lock files in the allocator's range and removes
	// stale locks left behind by crashed processes from a previous run.
	ReconcileOrphans()
}

// New returns a VNCProcessManager appropriate for the current host. If
// dependency checks fail (missing binaries or non-Linux platform), a
// noopVNCManager is returned that satisfies the interface with all no-ops.
// cfg.SessionID must be set before calling New.
func New(cfg VNCConfig) VNCProcessManager {
	deps := CheckDependencies()
	if !deps.Available {
		log.Info("vnc: dependencies not available, using noop manager",
			"reason", deps.Reason,
			"missing", deps.Missing,
		)
		return &noopVNCManager{}
	}

	alloc := NewDisplayAllocator(cfg.DisplayBase, cfg.DisplayRangeMax)

	return &vncProcessManager{
		cfg:   cfg,
		alloc: alloc,
		state: VNCState{Status: VNCStatusUnspecified},
	}
}

// ---- Real implementation -------------------------------------------------------

// vncProcessManager manages the full Xvfb + x11vnc lifecycle for one session.
type vncProcessManager struct {
	cfg   VNCConfig
	alloc *DisplayAllocator

	mu            sync.RWMutex
	state         VNCState
	stateChangeCb func(VNCState)

	xvfb      *executor.ManagedProcess
	x11vnc    *executor.ManagedProcess
	// managerCtx is the context shared by Xvfb, x11vnc, and all goroutines.
	// It is created in doStartDisplay and cancelled by Stop().
	managerCtx context.Context
	cancelCtx  context.CancelFunc

	// displayStartOnce guards StartDisplay so concurrent callers are safe.
	displayStartOnce sync.Once
	displayStartErr  error

	// serverStartOnce guards StartServer so concurrent callers are safe.
	serverStartOnce sync.Once
	serverStartErr  error
}

// SetStateChangeCallback registers a callback invoked on every VNC state change.
func (m *vncProcessManager) SetStateChangeCallback(cb func(VNCState)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stateChangeCb = cb
}

// setState updates m.state and invokes the callback in a goroutine (non-blocking).
// Must be called with m.mu held.
func (m *vncProcessManager) setState(s VNCState) {
	m.state = s
	if m.stateChangeCb != nil {
		cb := m.stateChangeCb
		go cb(s)
	}
}

// State returns a snapshot of the current VNC state.
func (m *vncProcessManager) State() VNCState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state
}

// DisplayNumber returns the allocated X11 display number, or 0 if none.
func (m *vncProcessManager) DisplayNumber() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state.DisplayNumber
}

// Port returns the localhost x11vnc TCP port, or 0 if x11vnc is not running.
func (m *vncProcessManager) Port() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state.Port
}

// ReconcileOrphans delegates to the DisplayAllocator to remove stale X11 lock files.
func (m *vncProcessManager) ReconcileOrphans() {
	m.alloc.CleanupStaleDisplays()
}

// StartDisplay allocates a display number and starts Xvfb. It is idempotent —
// concurrent callers all receive the same result via sync.Once. Call this BEFORE
// the tmux session is created so DISPLAY can be injected via ExtraEnv at
// new-session time.
func (m *vncProcessManager) StartDisplay(ctx context.Context) error {
	m.displayStartOnce.Do(func() {
		m.displayStartErr = m.doStartDisplay(ctx)
	})
	return m.displayStartErr
}

// doStartDisplay is the actual implementation of StartDisplay, called exactly once.
func (m *vncProcessManager) doStartDisplay(ctx context.Context) error {
	m.mu.Lock()
	m.setState(VNCState{Status: VNCStatusStarting})
	m.mu.Unlock()

	// Derive a cancellable context whose lifetime matches this manager's lifecycle.
	// Store both the context and the cancel func so doStartServer can use them.
	managerCtx, cancel := context.WithCancel(ctx)
	m.mu.Lock()
	m.managerCtx = managerCtx
	m.cancelCtx = cancel
	m.mu.Unlock()

	// Allocate a display number.
	displayN, err := m.alloc.Allocate(m.cfg.SessionID)
	if err != nil {
		cancel()
		m.mu.Lock()
		m.setState(VNCState{Status: VNCStatusUnavailable})
		m.mu.Unlock()
		return fmt.Errorf("vnc: display allocation failed: %w", err)
	}

	// Start Xvfb.
	if err := m.startXvfb(managerCtx, displayN); err != nil {
		cancel()
		m.alloc.Release(displayN)
		m.mu.Lock()
		m.setState(VNCState{Status: VNCStatusUnavailable})
		m.mu.Unlock()
		return fmt.Errorf("vnc: xvfb start failed: %w", err)
	}

	// Give Xvfb a moment to initialise the display socket before x11vnc tries to connect.
	time.Sleep(150 * time.Millisecond)

	// Find an OS-assigned port for x11vnc.
	port, err := pickFreePort()
	if err != nil {
		cancel()
		_ = m.stopXvfb()
		m.alloc.Release(displayN)
		m.mu.Lock()
		m.setState(VNCState{Status: VNCStatusUnavailable})
		m.mu.Unlock()
		return fmt.Errorf("vnc: port allocation failed: %w", err)
	}

	// Store display number and port in state so StartServer can read them.
	m.mu.Lock()
	m.setState(VNCState{
		Status:        VNCStatusStarting,
		DisplayNumber: displayN,
		Port:          port,
	})
	m.mu.Unlock()

	log.Info("vnc: Xvfb started and display allocated",
		"display", displayN,
		"port", port,
		"session", m.cfg.SessionID,
	)

	return nil
}

// StartServer starts x11vnc and the window tracker goroutine. It is idempotent —
// concurrent callers all receive the same result via sync.Once. Call this AFTER
// the tmux session is live so the agent inherits DISPLAY from the environment
// injected at new-session time.
func (m *vncProcessManager) StartServer(ctx context.Context) error {
	m.serverStartOnce.Do(func() {
		m.serverStartErr = m.doStartServer(ctx)
	})
	return m.serverStartErr
}

// doStartServer is the actual implementation of StartServer, called exactly once.
// It uses m.managerCtx (created in doStartDisplay) so that x11vnc and its goroutines
// are bound to the manager lifetime regardless of the ctx passed by the caller.
func (m *vncProcessManager) doStartServer(_ context.Context) error {
	m.mu.RLock()
	managerCtx := m.managerCtx
	displayN := m.state.DisplayNumber
	port := m.state.Port
	m.mu.RUnlock()

	if displayN <= 0 || managerCtx == nil {
		return fmt.Errorf("vnc: StartDisplay must be called successfully before StartServer")
	}

	// Start x11vnc in full-display mode (no -id yet; browser not yet detected).
	if err := m.startX11vnc(managerCtx, displayN, port, ""); err != nil {
		m.mu.Lock()
		m.setState(VNCState{Status: VNCStatusUnavailable})
		m.mu.Unlock()
		return fmt.Errorf("vnc: x11vnc start failed: %w", err)
	}

	m.mu.Lock()
	m.setState(VNCState{
		Status:        VNCStatusNoBrowser,
		DisplayNumber: displayN,
		Port:          port,
	})
	m.mu.Unlock()

	log.Info("vnc: x11vnc started",
		"display", displayN,
		"port", port,
		"session", m.cfg.SessionID,
	)

	// Launch crash-recovery goroutine for x11vnc.
	go m.x11vncCrashRecovery(managerCtx, displayN)

	// Launch window tracker.
	wt := NewWindowTracker(
		displayN,
		func(windowID string) {
			m.restartX11vncWithWindow(managerCtx, displayN, windowID)
		},
		func() {
			m.mu.Lock()
			s := m.state
			s.Status = VNCStatusNoBrowser
			s.BrowserWindowDetected = false
			m.setState(s)
			m.mu.Unlock()
			log.Info("vnc: browser window lost", "session", m.cfg.SessionID)
		},
	)
	wt.Start(managerCtx)

	return nil
}

// Start is a convenience wrapper that calls StartDisplay then StartServer.
// Use only when the two-phase split is not needed (e.g. unit tests or cases
// where DISPLAY injection into tmux new-session is not required).
func (m *vncProcessManager) Start(ctx context.Context) error {
	if err := m.StartDisplay(ctx); err != nil {
		return err
	}
	return m.StartServer(ctx)
}

// Stop tears down x11vnc and Xvfb and releases the display number.
func (m *vncProcessManager) Stop() {
	m.mu.Lock()
	cancel := m.cancelCtx
	displayN := m.state.DisplayNumber
	m.managerCtx = nil
	m.cancelCtx = nil
	m.mu.Unlock()

	// Cancel manager context — signals goroutines to stop.
	if cancel != nil {
		cancel()
	}

	// Stop x11vnc first, then Xvfb (order specified in plan).
	_ = m.stopX11vnc()
	_ = m.stopXvfb()

	if displayN > 0 {
		m.alloc.Release(displayN)
	}

	m.mu.Lock()
	m.setState(VNCState{Status: VNCStatusUnavailable})
	m.mu.Unlock()

	log.Info("vnc: processes stopped", "session", m.cfg.SessionID)
}

// startXvfb launches the Xvfb process on the given display number.
func (m *vncProcessManager) startXvfb(ctx context.Context, displayN int) error {
	resolution := m.cfg.Resolution
	if resolution == "" {
		resolution = "1280x800x24"
	}

	args := []string{
		fmt.Sprintf(":%d", displayN),
		"-screen", "0", resolution,
		"-nolisten", "tcp",
	}

	proc, err := executor.StartProcess(ctx, "Xvfb", args,
		executor.WithNoControllingTerminal(),
	)
	if err != nil {
		return fmt.Errorf("xvfb start: %w", err)
	}

	m.mu.Lock()
	m.xvfb = proc
	m.mu.Unlock()

	log.Info("vnc: Xvfb started", "display", displayN, "pid", proc.PID())
	return nil
}

// stopXvfb stops the Xvfb process if running.
func (m *vncProcessManager) stopXvfb() error {
	m.mu.Lock()
	proc := m.xvfb
	m.xvfb = nil
	m.mu.Unlock()

	if proc == nil {
		return nil
	}
	return proc.Stop()
}

// startX11vnc launches x11vnc on the given display and port.
// If windowID is non-empty, -id <windowID> is appended for focused mode.
func (m *vncProcessManager) startX11vnc(ctx context.Context, displayN, port int, windowID string) error {
	args := []string{
		"-display", fmt.Sprintf(":%d", displayN),
		"-rfbport", fmt.Sprintf("%d", port),
		"-localhost",
		"-nopw",
		"-noclipboard",
		"-shared",
		"-forever",
	}
	if windowID != "" {
		args = append(args, "-id", windowID)
	}

	proc, err := executor.StartProcess(ctx, "x11vnc", args,
		executor.WithNoControllingTerminal(),
	)
	if err != nil {
		return fmt.Errorf("x11vnc start: %w", err)
	}

	m.mu.Lock()
	m.x11vnc = proc
	m.mu.Unlock()

	log.Info("vnc: x11vnc started",
		"display", displayN,
		"port", port,
		"window_id", windowID,
		"pid", proc.PID(),
	)
	return nil
}

// stopX11vnc stops the x11vnc process if running.
func (m *vncProcessManager) stopX11vnc() error {
	m.mu.Lock()
	proc := m.x11vnc
	m.x11vnc = nil
	m.mu.Unlock()

	if proc == nil {
		return nil
	}
	return proc.Stop()
}

// restartX11vncWithWindow stops the current x11vnc and restarts it with -id <windowID>,
// then updates state to VNCStatusReady.
func (m *vncProcessManager) restartX11vncWithWindow(ctx context.Context, displayN int, windowID string) {
	log.Info("vnc: browser window detected, restarting x11vnc in -id mode",
		"window_id", windowID,
		"session", m.cfg.SessionID,
	)

	m.mu.Lock()
	port := m.state.Port
	m.mu.Unlock()

	if err := m.stopX11vnc(); err != nil {
		log.Warn("vnc: error stopping x11vnc before restart", "err", err)
	}

	if err := m.startX11vnc(ctx, displayN, port, windowID); err != nil {
		log.Error("vnc: failed to restart x11vnc with window ID",
			"window_id", windowID,
			"err", err,
		)
		m.mu.Lock()
		s := m.state
		s.Status = VNCStatusNoBrowser
		s.BrowserWindowDetected = false
		m.setState(s)
		m.mu.Unlock()
		return
	}

	m.mu.Lock()
	s := m.state
	s.Status = VNCStatusReady
	s.BrowserWindowDetected = true
	m.setState(s)
	m.mu.Unlock()

	log.Info("vnc: x11vnc restarted in window mode",
		"window_id", windowID,
		"session", m.cfg.SessionID,
	)
}

// x11vncCrashRecovery watches for x11vnc unexpected exits and restarts it
// (in full-display mode) with exponential backoff, up to cfg.MaxRestarts times.
// After exhausting retries it marks the VNC state as VNCStatusUnavailable.
func (m *vncProcessManager) x11vncCrashRecovery(ctx context.Context, displayN int) {
	maxRestarts := m.cfg.MaxRestarts
	if maxRestarts <= 0 {
		maxRestarts = 3
	}

	consecutiveFailures := 0
	backoff := 100 * time.Millisecond
	const maxBackoff = 30 * time.Second

	for {
		m.mu.RLock()
		proc := m.x11vnc
		m.mu.RUnlock()

		if proc == nil {
			// x11vnc was stopped intentionally (Stop() called).
			return
		}

		// Wait for x11vnc to exit.
		waitErr := proc.Wait()

		// Fix 1: Re-read m.x11vnc after Wait() returns. If Stop() cleared it while
		// Wait() was blocking, the exit was intentional — don't treat it as a crash.
		m.mu.RLock()
		current := m.x11vnc
		m.mu.RUnlock()

		// Check if the manager context has been cancelled — that means Stop() was called.
		select {
		case <-ctx.Done():
			return
		default:
		}

		if current == nil {
			// Stop() already cleaned up m.x11vnc; the exit was intentional.
			return
		}

		// If x11vnc exited and we're still running, it crashed.
		log.Warn("vnc: x11vnc exited unexpectedly",
			"err", waitErr,
			"attempt", consecutiveFailures+1,
			"max", maxRestarts,
			"session", m.cfg.SessionID,
		)

		consecutiveFailures++
		if consecutiveFailures > maxRestarts {
			log.Error("vnc: x11vnc crash limit reached, marking unavailable",
				"session", m.cfg.SessionID,
			)
			m.mu.Lock()
			m.setState(VNCState{
				Status:        VNCStatusUnavailable,
				DisplayNumber: displayN,
			})
			m.mu.Unlock()
			return
		}

		// Apply exponential backoff before restarting.
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}

		m.mu.RLock()
		port := m.state.Port
		m.mu.RUnlock()

		log.Info("vnc: restarting x11vnc after crash",
			"attempt", consecutiveFailures,
			"backoff", backoff,
			"session", m.cfg.SessionID,
		)

		if err := m.startX11vnc(ctx, displayN, port, ""); err != nil {
			log.Error("vnc: failed to restart x11vnc", "err", err)
			continue
		}

		// Reset failure count on successful restart.
		consecutiveFailures = 0
		backoff = 100 * time.Millisecond

		m.mu.Lock()
		s := m.state
		if s.Status == VNCStatusReady {
			// We lost the window during the crash; reset to NoBrowser.
			s.Status = VNCStatusNoBrowser
			s.BrowserWindowDetected = false
		}
		m.setState(s)
		m.mu.Unlock()
	}
}

// pickFreePort asks the OS for an available TCP port on localhost by binding
// to port 0, recording the assigned port, then immediately closing the listener.
// There is a narrow TOCTOU window, but this is acceptable on localhost per ADR-012.
func pickFreePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("pick free port: %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port, nil
}

// ---- No-op implementation ------------------------------------------------------

// noopVNCManager satisfies VNCProcessManager with all no-ops. Returned by New()
// on non-Linux hosts or when required binaries are absent.
type noopVNCManager struct{}

func (n *noopVNCManager) StartDisplay(_ context.Context) error  { return nil }
func (n *noopVNCManager) StartServer(_ context.Context) error   { return nil }
func (n *noopVNCManager) Start(_ context.Context) error         { return nil }
func (n *noopVNCManager) Stop()                                 {}
func (n *noopVNCManager) State() VNCState                       { return VNCState{Status: VNCStatusUnavailable} }
func (n *noopVNCManager) DisplayNumber() int                    { return 0 }
func (n *noopVNCManager) Port() int                             { return 0 }
func (n *noopVNCManager) SetStateChangeCallback(_ func(VNCState)) {}
func (n *noopVNCManager) ReconcileOrphans()                     {}
