package session

// instance_vnc.go wires the VNC process manager into Instance lifecycle.
// The VNCProcessManager is created in NewInstance() and started/stopped
// alongside the tmux session so each session owns a dedicated virtual display.
//
// Two-phase startup:
//   1. startVNCDisplay — allocates the X display and starts Xvfb. Called BEFORE
//      the tmux session is created so DISPLAY=:<N> can be injected via ExtraEnv
//      at new-session time. The agent process inherits the correct DISPLAY from
//      the start rather than relying on a post-hoc `tmux setenv` call.
//   2. startVNCServer — starts x11vnc and the window tracker. Called AFTER the
//      tmux session is live.

import (
	"context"
	"fmt"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/vnc"
)

// VNCProcessManager is a local alias for the vnc package interface so that
// files within the session package can reference it without importing vnc directly.
type VNCProcessManager = vnc.VNCProcessManager

// initVNCManager creates the VNCProcessManager for this instance using the application config.
// If VNC is explicitly disabled via config, or deps are unavailable, a no-op manager is returned.
func (i *Instance) initVNCManager(cfg *config.BrowserPassthroughConfig) {
	if !cfg.IsEnabled() {
		i.vncManager = vnc.New(vnc.VNCConfig{})
		return
	}
	vncCfg := vnc.DefaultVNCConfig()
	vncCfg.SessionID = i.GetStableID()
	if cfg != nil {
		if cfg.DisplayBase > 0 {
			vncCfg.DisplayBase = cfg.DisplayBase
		}
		if cfg.DisplayRangeMax > 0 {
			vncCfg.DisplayRangeMax = cfg.DisplayRangeMax
		}
		if cfg.Resolution != "" {
			vncCfg.Resolution = cfg.Resolution
		}
	}
	i.vncManager = vnc.New(vncCfg)
}

// VNCManager returns the VNCProcessManager for this instance.
// Always non-nil — returns a no-op manager on unsupported platforms.
func (i *Instance) VNCManager() VNCProcessManager {
	return i.vncManager
}

// VNCDisplayEnv returns the DISPLAY environment variable string for this
// session's virtual display, e.g. "DISPLAY=:101". Returns "" if VNC is
// unavailable or if StartDisplay has not yet been called successfully.
func (i *Instance) VNCDisplayEnv() string {
	if i.vncManager == nil {
		return ""
	}
	n := i.vncManager.DisplayNumber()
	if n <= 0 {
		return ""
	}
	return fmt.Sprintf("DISPLAY=:%d", n)
}

// startVNCDisplay allocates the X display and starts Xvfb.
// Called BEFORE tmuxManager.Start() so DISPLAY can be injected into the
// new-session command via ExtraEnv. Non-fatal: logs a warning on failure.
func (i *Instance) startVNCDisplay(ctx context.Context) {
	if i.vncManager == nil {
		return
	}

	// Register the state-change callback before any state transitions can fire.
	i.vncManager.SetStateChangeCallback(func(state vnc.VNCState) {
		i.onVNCStateChange()
	})

	if err := i.vncManager.StartDisplay(ctx); err != nil {
		log.Warn("failed to start VNC display (non-fatal)", "session", i.Title, "err", err)
	}
}

// startVNCServer starts x11vnc and the window tracker goroutine.
// Called AFTER tmuxManager.Start() so the agent already has DISPLAY in its
// environment from the new-session injection done in start(). Non-fatal.
func (i *Instance) startVNCServer(ctx context.Context) {
	if i.vncManager == nil {
		return
	}

	if err := i.vncManager.StartServer(ctx); err != nil {
		log.Warn("failed to start VNC server (non-fatal)", "session", i.Title, "err", err)
		return
	}

	displayN := i.vncManager.DisplayNumber()
	log.Info("VNC server started", "session", i.Title, "display", displayN)
}

// stopVNC stops the VNC subsystem for this instance.
// Must be called before KillSession() so x11vnc exits before Xvfb.
func (i *Instance) stopVNC() {
	if i.vncManager == nil {
		return
	}
	i.vncManager.Stop()
}

// onVNCStateChange is called by the vncManager callback when VNC state changes.
// It fires an onStatusChange-equivalent event so WatchSessions clients receive an update.
func (i *Instance) onVNCStateChange() {
	// Re-use the onStatusChange callback machinery to trigger a reactive queue refresh
	// and a WatchSessions SessionUpdated event. The reactive queue manager wires this
	// callback in BuildRuntimeDeps via inst.SetOnStatusChange().
	i.onStatusChangeMu.RLock()
	fn := i.onStatusChange
	i.onStatusChangeMu.RUnlock()
	if fn != nil {
		// Trigger with StatusIdle — we just want to signal that session state changed
		// so the reactive queue manager broadcasts a SessionUpdated event with the
		// full session state (including the updated vnc_state field).
		fn(detection.StatusIdle, "vnc_state_changed")
	}
}
