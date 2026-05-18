package session

// instance_cdp.go wires the CDP stream manager into Instance lifecycle.
// The CDPStreamManager is created in NewInstance() and started/stopped
// alongside the tmux session so each session owns a dedicated CDP port.
//
// Three-phase startup:
//  1. allocateCDPPort — allocates a free TCP port and writes Chrome wrapper
//     scripts. Called BEFORE the tmux session is created so CDP_PORT and the
//     updated PATH (containing the wrapper directory) can be injected via
//     ExtraEnv at new-session time. The agent process inherits CDP_PORT from
//     the start so Chrome launches with --remote-debugging-port set correctly.
//  2. startCDP — starts the polling/screencast goroutine. Called AFTER the
//     tmux session is live so Chrome has had a chance to start.
//  3. stopCDP — cancels goroutines and cleans up. Called in Destroy().

import (
	"context"
	"fmt"
	"os"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/cdp"
	"github.com/tstapler/stapler-squad/session/detection"
)

// CDPStreamManager is a local alias for the cdp package interface so that
// files within the session package can reference it without importing cdp directly.
type CDPStreamManager = cdp.CDPStreamManager

// initCDPManager creates the CDPStreamManager for this instance using the
// application config. Browser passthrough is enabled when either the feature
// flag or the explicit config field is set. If disabled or Chrome is not found,
// a no-op manager is returned.
func (i *Instance) initCDPManager(appCfg *config.Config) {
	cfg := &appCfg.BrowserPassthrough
	if !cfg.IsEnabled() && !appCfg.GetFeatureFlag("browser-passthrough") {
		i.cdpManager = cdp.New(cdp.CDPConfig{})
		return
	}
	deps := cdp.CheckDependencies()
	if !deps.Available {
		log.Info("cdp: Chrome not found, using noop manager",
			"reason", deps.Reason,
			"session", i.GetStableID(),
		)
		i.cdpManager = cdp.New(cdp.CDPConfig{})
		return
	}
	cdpCfg := cfg.CDP.CDPConfigOrDefault()
	i.cdpManager = cdp.New(cdp.CDPConfig{
		SessionID:           i.GetStableID(),
		ChromePath:          deps.ChromePath,
		ScreencastQuality:   cdpCfg.ScreencastQuality,
		ScreencastMaxWidth:  cdpCfg.ScreencastMaxWidth,
		ScreencastMaxHeight: cdpCfg.ScreencastMaxHeight,
		ScreencastMaxFPS:    cdpCfg.ScreencastMaxFPS,
	})
}

// CDPManager returns the CDPStreamManager for this instance.
// Always non-nil after NewInstance() — returns a no-op manager when Chrome
// is unavailable.
func (i *Instance) CDPManager() CDPStreamManager {
	return i.cdpManager
}

// CDPDisplayEnv returns the extra environment variable strings to inject into
// the tmux session for CDP:
//   - "CDP_PORT=<N>" — the allocated CDP debugging port
//   - "PATH=<wrapperDir>:<original PATH>" — prepends the wrapper script dir so
//     Chrome launcher scripts resolve to our wrappers, not the real binary
//
// Returns nil if CDP is unavailable or if Allocate has not been called yet.
func (i *Instance) CDPDisplayEnv() []string {
	if i.cdpManager == nil {
		return nil
	}
	port := i.cdpManager.Port()
	if port == 0 {
		return nil
	}
	wrapperDir := i.cdpManager.WrapperDir()
	if wrapperDir == "" {
		return nil
	}

	var envs []string
	envs = append(envs, fmt.Sprintf("CDP_PORT=%d", port))

	// Prepend wrapper dir to PATH so any Chrome launch command in the session
	// picks up our wrapper that injects --remote-debugging-port.
	originalPath := os.Getenv("PATH")
	if originalPath != "" {
		envs = append(envs, fmt.Sprintf("PATH=%s:%s", wrapperDir, originalPath))
	} else {
		envs = append(envs, fmt.Sprintf("PATH=%s", wrapperDir))
	}
	return envs
}

// allocateCDPPort allocates the CDP port and writes wrapper scripts.
// Called BEFORE tmuxManager.Start() so CDP_PORT can be injected into the
// new-session command via ExtraEnv. Non-fatal: logs a warning on failure.
func (i *Instance) allocateCDPPort() {
	if i.cdpManager == nil {
		return
	}

	// Register the state-change callback before any transitions can fire.
	i.cdpManager.SetStateChangeCallback(func(state cdp.CDPState) {
		i.onCDPStateChange()
	})

	if err := i.cdpManager.Allocate(); err != nil {
		log.Warn("failed to allocate CDP port (non-fatal)", "session", i.Title, "err", err)
	}
}

// startCDP starts the CDP polling/screencast goroutine.
// Called AFTER tmuxManager.Start() so the agent already has CDP_PORT in its
// environment from the ExtraEnv injection done in start(). Non-fatal.
func (i *Instance) startCDP(ctx context.Context) {
	if i.cdpManager == nil {
		return
	}
	if i.cdpManager.Port() == 0 {
		// Allocate was never called or failed; nothing to start.
		return
	}

	if err := i.cdpManager.Start(ctx); err != nil {
		log.Warn("failed to start CDP stream (non-fatal)", "session", i.Title, "err", err)
		return
	}

	log.Info("CDP stream started", "session", i.Title, "port", i.cdpManager.Port())
}

// stopCDP stops the CDP subsystem for this instance.
func (i *Instance) stopCDP() {
	if i.cdpManager == nil {
		return
	}
	i.cdpManager.Stop()
}

// onCDPStateChange is called by the cdpManager callback when CDP state changes.
// It re-uses the onStatusChange callback machinery to trigger a reactive queue
// refresh and a WatchSessions SessionUpdated event.
func (i *Instance) onCDPStateChange() {
	i.onStatusChangeMu.RLock()
	fn := i.onStatusChange
	i.onStatusChangeMu.RUnlock()
	if fn != nil {
		fn(detection.StatusIdle, "cdp_state_changed")
	}
}
