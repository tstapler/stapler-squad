package session

// instance_controller.go contains ClaudeController lifecycle methods and
// rate limit delegation for Instance.

import (
	"context"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/analytics"
	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/detection/ratelimit"
)

// StartController creates and starts a ClaudeController for this instance.
// The controller enables automated idle detection and queue management.
func (i *Instance) StartController() error {
	// Check preconditions under lock
	i.mu.Lock()

	// Only start if we have a status manager
	if i.controllerManager.GetStatusManager() == nil {
		i.mu.Unlock()
		log.Debug("no status manager set for instance, skipping controller", "session", i.Title)
		return nil
	}

	// Don't create controller if instance isn't started
	if !i.started.Load() {
		i.mu.Unlock()
		log.Debug("instance not started yet, skipping controller", "session", i.Title)
		return nil
	}

	// Don't recreate if already exists
	if i.controllerManager.HasController() {
		i.mu.Unlock()
		log.Debug("controller already exists for instance", "session", i.Title)
		return nil
	}

	// Release lock before creating/starting controller
	// This prevents deadlock when Start() calls GetPTYReader() which acquires read lock
	i.mu.Unlock()

	// Bail out if the instance was destroyed before we got here. This closes the
	// same TOCTOU window driverMu/driverDestroyed closes for StartSessionDriver
	// (see the doc comment on driverMu in instance.go): CreateSession's async
	// init goroutine can call StartController well after the RPC returned, racing
	// a fast-following DeleteSession's Destroy(). Destroy() sets i.destroyed
	// before calling StopController, so if it's already set here Destroy() either
	// already ran StopController (finding nothing to stop) or is about to — either
	// way this instance is being torn down and must not register a new controller.
	if i.destroyed.Load() {
		log.Debug("instance destroyed, refusing to start controller", "session", i.Title)
		return nil
	}

	// Create new controller (no lock needed - NewClaudeController doesn't access mutex-protected fields)
	controller, err := NewClaudeController(i)
	if err != nil {
		return fmt.Errorf("failed to create controller: %w", err)
	}

	// Wire PTY-EOF callback: when the ResponseStream detects PTY exit without an
	// explicit Stop(), transition the instance to Stopped and notify listeners.
	// If the exit tail contains a stale --resume error, auto-recover by clearing
	// the UUID and restarting fresh (no --resume on the next attempt).
	// Route through the actor mailbox to prevent TOCTOU races on i.Status.
	controller.SetOnEOFCallback(func() {
		log.Info("pty eof received from response stream", "session", i.Title)
		exitContent := controller.GetExitContent()
		i.send(func(s *instanceState) {
			if s.inst.Status == Active {
				if err := transitionToLocked(s, context.Background(), Stopped); err != nil {
					log.Warn("exit callback transition failed", "session", i.Title, "err", err)
				}
			}
		})
		i.fireLifecycleEvent(EventExited, "pty-eof")

		if isStaleResumeExit(exitContent) {
			go i.recoverFromStaleResume()
		}
	})

	// Wire status-change listener before Start() so no events are lost.
	i.wireStatusChangeCallback(controller)

	// Start the controller - this initializes all components and begins background operations
	// Single call replaces the old Initialize() + Start() pattern
	if err := controller.Start(context.Background()); err != nil {
		return fmt.Errorf("failed to start controller: %w", err)
	}

	// Wire rate limit callbacks BEFORE re-acquiring mu.
	// controller.Start() can launch goroutines that hold ctrl.mu and call back into
	// Instance methods (which acquire stateMutex). wireRateLimitCallbacks acquires
	// ctrl.mu.RLock(). Calling it inside stateMutex would create a lock-order cycle.
	i.wireRateLimitCallbacks(controller)

	// Re-acquire lock to update instance state
	i.mu.Lock()

	// Double-check controller hasn't been set by another goroutine (defensive)
	if i.controllerManager.HasController() {
		i.mu.Unlock()
		log.Debug("controller already exists for instance (race detected)", "session", i.Title)
		return nil
	}

	// Destroy() may have run StopController while controller.Start() (above) was
	// still executing; since nothing was registered yet at that point, StopController
	// no-op'd and the controller we just started would otherwise leak forever with
	// no one left to stop it. Discard it instead of registering.
	if i.destroyed.Load() {
		i.mu.Unlock()
		log.Debug("instance destroyed while controller was starting; stopping newly started controller", "session", i.Title)
		if stopErr := controller.Stop(); stopErr != nil {
			log.Warn("failed to stop discarded controller after concurrent destroy", "session", i.Title, "err", stopErr)
		}
		return nil
	}

	// Register with status manager and store controller
	i.controllerManager.RegisterController(i.Title, controller)
	i.mu.Unlock()

	log.Info("started claudecontroller for instance", "session", i.Title)
	return nil
}

// RegisterLifecycleListener adds a listener that will receive EventStarted,
// EventExited, and EventStopped notifications for this instance.
// The listener is called synchronously on the goroutine that fires the event;
// implementations must return quickly (no long blocking operations).
func (i *Instance) RegisterLifecycleListener(l LifecycleListener) {
	i.lifecycleListenersMu.Lock()
	defer i.lifecycleListenersMu.Unlock()
	i.lifecycleListeners = append(i.lifecycleListeners, l)
}

// hasLifecycleListeners reports whether any listener is currently registered
// for this instance. Used to skip paying for expensive event-only work (e.g.
// UpdateDiffStats' subprocess git-diff call in instanceOnExitCallback/Destroy)
// when nothing is wired to consume it — e.g. deployments where
// SessionSummaryGenerator is nil and no listener was ever registered.
func (i *Instance) hasLifecycleListeners() bool {
	i.lifecycleListenersMu.Lock()
	defer i.lifecycleListenersMu.Unlock()
	return len(i.lifecycleListeners) > 0
}

// fireLifecycleEvent notifies all registered listeners of a lifecycle event.
func (i *Instance) fireLifecycleEvent(event LifecycleEvent, reason string) {
	i.lifecycleListenersMu.Lock()
	listeners := make([]LifecycleListener, len(i.lifecycleListeners))
	copy(listeners, i.lifecycleListeners)
	i.lifecycleListenersMu.Unlock()

	for _, l := range listeners {
		l.OnLifecycleEvent(event, reason)
	}
}

// FireLifecycleEventForTest is the exported version of fireLifecycleEvent, used
// exclusively in cross-package tests that need to simulate an unexpected exit.
func (i *Instance) FireLifecycleEventForTest(event LifecycleEvent, reason string) {
	i.fireLifecycleEvent(event, reason)
}

// SetControllerForTest wires c as this instance's controller, bypassing
// StartController's normal preconditions (status manager wiring, a live PTY
// subprocess). Exported exclusively for cross-package tests (e.g.
// server/services' steerInstance autonomous-branch error/timeout tests) that
// need a real, pipe-backed *ClaudeController (session.NewClaudeController +
// Start against an os.Pipe()-backed InstanceContext, mirroring
// TestClaudeController_Start_TagsEscapeAnalyticsWithStableID's approach)
// without spinning up a full session. Mirrors FireLifecycleEventForTest's
// "exported for test" pattern.
func (i *Instance) SetControllerForTest(c *ClaudeController) {
	i.controllerManager.SetController(c)
}

// StopController stops and cleans up the ClaudeController for this instance.
func (i *Instance) StopController() {
	i.mu.Lock()
	defer i.mu.Unlock()

	if !i.controllerManager.HasController() {
		return
	}

	i.controllerManager.UnregisterController(i.Title)

	log.Info("stopped claudecontroller for instance", "session", i.Title)
}

// stopControllerLocked stops the ClaudeController from within an actor command.
func stopControllerLocked(s *instanceState) {
	i := s.inst
	if !i.controllerManager.HasController() {
		return
	}
	i.controllerManager.UnregisterController(i.Title)
	log.Info("stopped claudecontroller for instance", "session", i.Title)
}

// GetController returns the ClaudeController if one exists.
func (i *Instance) GetController() *ClaudeController {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.controllerManager.GetController()
}

// GetExitContent returns the last terminal bytes captured before the PTY exited.
// Returns nil if the controller is not running or no exit content was recorded.
func (i *Instance) GetExitContent() []byte {
	ctrl := i.GetController()
	if ctrl == nil {
		return nil
	}
	return ctrl.GetExitContent()
}

// GetEscapeParser returns the escape code parser from the session's response stream.
// Returns nil if the controller is not running or has no response stream.
func (i *Instance) GetEscapeParser() *analytics.EscapeCodeParser {
	ctrl := i.GetController()
	if ctrl == nil {
		return nil
	}
	return ctrl.GetEscapeParser()
}

// GetTotalBytesWritten returns the monotonic PTY byte offset from the session's
// circular buffer. This is the same counter used by Stage 1 analytics so Stage
// 2 session_seq values remain stable across WebSocket reconnections.
// Returns 0 if no controller is active or the buffer is unavailable.
func (i *Instance) GetTotalBytesWritten() int64 {
	ctrl := i.GetController()
	if ctrl == nil {
		return 0
	}
	return ctrl.GetTotalBytesWritten()
}

// GetRateLimitState returns the current rate limit detection state.
func (i *Instance) GetRateLimitState() int {
	ctrl := i.GetController()
	if ctrl == nil {
		return 0
	}
	return int(ctrl.GetRateLimitState())
}

// GetRateLimitResetTime returns the time when the rate limit is expected to reset.
// Returns zero time if no controller is active or no reset time is known.
func (i *Instance) GetRateLimitResetTime() time.Time {
	ctrl := i.GetController()
	if ctrl == nil {
		return time.Time{}
	}
	return ctrl.GetRateLimitResetTime()
}

// SetRateLimitEnabled enables or disables rate limit auto-resume.
// The setting is persisted in RateLimitAutoResume so it survives restarts,
// and is applied immediately to the running controller if one exists.
func (i *Instance) SetRateLimitEnabled(enabled bool) {
	i.RateLimitAutoResume = &enabled
	ctrl := i.GetController()
	if ctrl != nil {
		ctrl.SetRateLimitEnabled(enabled)
	}
}

// SetStatusChangeCallback registers fn to be called on every terminal status change
// detected by the ClaudeController. Safe to call before or after the controller is
// started; the callback is wired at controller start time via wireStatusChangeCallback.
func (i *Instance) SetStatusChangeCallback(fn func(detection.DetectedStatus, string)) {
	i.onStatusChange.Write(func(f *func(detection.DetectedStatus, string)) {
		*f = fn
	})

	// If a controller is already running, wire immediately.
	i.wireStatusChangeCallback(i.GetController())
}

// wireStatusChangeCallback wires the instance-level status-change callback to the
// ClaudeController's fan-out listener set. Called both from SetStatusChangeCallback
// and from StartController before controller.Start().
func (i *Instance) wireStatusChangeCallback(ctrl *ClaudeController) {
	if ctrl == nil {
		return
	}
	var fn func(detection.DetectedStatus, string)
	i.onStatusChange.Read(func(f func(detection.DetectedStatus, string)) {
		fn = f
	})
	if fn == nil {
		return
	}
	ctrl.AddStatusChangeListener(fn)
}

// RegisterStatusChangeCallback appends fn to the controller's fan-out listener set.
// Unlike SetStatusChangeCallback, it does not replace existing listeners.
// Safe to call before or after the controller is started.
func (i *Instance) RegisterStatusChangeCallback(fn func(detection.DetectedStatus, string)) {
	ctrl := i.GetController()
	if ctrl != nil {
		ctrl.AddStatusChangeListener(fn)
	}
}

// SetRateLimitCallbacks registers server-layer callbacks for rate limit events.
// onDetected is called when a rate limit is detected; onRecovery is called when
// recovery completes. Both are invoked from goroutines in the ratelimit package.
// Safe to call before or after the controller is started; callbacks are wired at
// controller start time via wireRateLimitCallbacks.
func (i *Instance) SetRateLimitCallbacks(
	onDetected func(sessionID string, resetTime time.Time),
	onRecovery func(sessionID string, success bool, errMsg string),
) {
	i.rateLimitCallbacksMu.Lock()
	i.onRateLimitDetected = onDetected
	i.onRateLimitRecovery = onRecovery
	i.rateLimitCallbacksMu.Unlock()

	// If a controller is already running, wire immediately.
	i.wireRateLimitCallbacks(i.GetController())
}

// wireRateLimitCallbacks wires the instance-level callbacks to the rate limit manager
// inside the controller. Called both from SetRateLimitCallbacks and from controller startup.
func (i *Instance) wireRateLimitCallbacks(ctrl *ClaudeController) {
	if ctrl == nil {
		return
	}
	handler := ctrl.GetRateLimitHandler()
	if handler == nil {
		return
	}
	mgr := handler.GetManager()
	if mgr == nil {
		return
	}

	i.rateLimitCallbacksMu.Lock()
	onDetected := i.onRateLimitDetected
	onRecovery := i.onRateLimitRecovery
	i.rateLimitCallbacksMu.Unlock()

	sessionID := i.GetStableID()
	if onDetected != nil {
		mgr.SetDetectionCallback(func(det ratelimit.Detection) {
			onDetected(sessionID, det.ResetTime)
		})
	}
	if onRecovery != nil {
		mgr.SetRecoveryCallback(func(success bool, _ ratelimit.Detection) {
			var errMsg string
			if !success {
				errMsg = "recovery input failed"
			}
			onRecovery(sessionID, success, errMsg)
		})
	}
}

// IsRateLimitEnabled returns whether rate limit auto-resume is enabled.
// Returns the persisted RateLimitAutoResume field (default: true when nil).
func (i *Instance) IsRateLimitEnabled() bool {
	if i.RateLimitAutoResume == nil {
		return true
	}
	return *i.RateLimitAutoResume
}
