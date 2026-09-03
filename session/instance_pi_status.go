package session

// instance_pi_status.go wires Epic 5.2's PiStatusSource into Instance's
// existing StartController/StopController lifecycle (session/instance_controller.go),
// mirroring how a ClaudeController is registered/unregistered but via the
// piSources parallel map (session/instance_status.go), not the controllers map.

import (
	"os/exec"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
)

// piStatusSupported reports whether this instance should get a status-only
// pi subprocess: the program must resolve to pi and the pi-support feature
// flag must be enabled. Checked at both StartController and StopController
// so a mid-flight flag flip can't leave one side out of sync with the other.
func (i *Instance) piStatusSupported() bool {
	return isPi(i.Program) && config.LoadConfig().GetFeatureFlag(config.FeaturePiSupport)
}

// startPiStatusSource launches the status-only `pi --mode json` subprocess
// and registers it with the status manager, if one is set. A no-op if a
// source is already registered (mirrors StartController's
// "don't recreate if already exists" guard).
func (i *Instance) startPiStatusSource() error {
	if i.piStatusSrc.Load() != nil {
		log.Debug("pi status source already exists for instance", "session", i.Title)
		return nil
	}

	statusMgr := i.controllerManager.GetStatusManager()
	if statusMgr == nil {
		log.Debug("no status manager set for instance, skipping pi status source", "session", i.Title)
		return nil
	}

	src := NewPiStatusSource(i.Title, i.piStatusCommandFactory())
	if err := src.Start(); err != nil {
		return err
	}

	i.piStatusSrc.Store(src)
	statusMgr.RegisterPiStatusSource(i.Title, src)

	log.Info("started pi status source for instance", "session", i.Title)
	return nil
}

// stopPiStatusSource stops and unregisters the status-only pi subprocess,
// if one is running. Safe to call unconditionally (e.g. from a generic
// StopController path) even when no source was ever started.
func (i *Instance) stopPiStatusSource() {
	src := i.piStatusSrc.Swap(nil)
	if src == nil {
		return
	}

	if statusMgr := i.controllerManager.GetStatusManager(); statusMgr != nil {
		statusMgr.UnregisterPiStatusSource(i.Title)
	}
	src.Stop()

	log.Info("stopped pi status source for instance", "session", i.Title)
}

// piStatusCommandFactory builds the piCommandFactory used to (re)launch the
// status-only pi subprocess: `<program base> --mode json [--session <id>]`,
// run in the instance's working directory.
//
// This is a SEPARATE pi invocation from the tmux-launched interactive
// session (buildPiCommand in instance_tmux.go), not an attachment to it —
// pi's `--mode json` output mode was only verified (Phase 1 spike) as a
// fresh, non-interactive invocation; there is no confirmed way to attach a
// `--mode json` stream to an already-running interactive pi process. This
// means a pi session with pi-support enabled costs two live pi processes
// for the lifetime of the session (see plan.md Task 5.2.1c). If pi later
// adds a way to attach to an existing session's event stream, this factory
// is the place to switch to it.
func (i *Instance) piStatusCommandFactory() piCommandFactory {
	base := i.Program
	path := i.Path
	var piSessionID string
	if i.piSession != nil {
		piSessionID = i.piSession.SessionID
	}

	return func() *exec.Cmd {
		args := []string{"--mode", "json"}
		if piSessionID != "" {
			args = append(args, "--session", piSessionID)
		}
		cmd := exec.Command(base, args...) //nolint:gosec // base is the operator-configured program command, same trust boundary as the tmux-launched invocation (buildPiCommand)
		if path != "" {
			cmd.Dir = path
		}
		return cmd
	}
}
