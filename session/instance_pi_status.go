package session

// instance_pi_status.go wires Epic 5.2's PiStatusSource into Instance's
// existing StartController/StopController lifecycle (session/instance_controller.go),
// mirroring how a ClaudeController is registered/unregistered but via the
// piSources parallel map (session/instance_status.go), not the controllers map.

import (
	"context"
	"os/exec"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/log"
)

// piStatusSupported reports whether this instance should get a NEW
// status-only pi subprocess started: the program must resolve to pi and the
// pi-support feature flag must be enabled. Checked only at StartController
// (Bug 2 fix) -- StopController/stopControllerLocked instead route on
// i.piStatusSrc's live registration state, so a running PiStatusSource
// always gets stopped regardless of a mid-flight flag flip. Re-checking this
// flag at stop time was the bug: disabling pi-support while a pi session's
// PiStatusSource was still running left it un-Stop()-ed, leaking its
// subprocess/goroutines and racing Restart's unsynchronized i.piSession
// access (see SetPiSessionID's doc comment).
func (i *Instance) piStatusSupported() bool {
	return isPi(i.Program) && config.LoadConfig().GetFeatureFlag(config.FeaturePiSupport)
}

// startPiStatusSource launches the status-only `pi --mode json` subprocess
// and registers it with the status manager, if one is set. A no-op if a
// source is already registered (mirrors StartController's
// "don't recreate if already exists" guard).
//
// piStatusStartMu is held for the entire check-then-act sequence (load,
// then construct+Start()+Store()) -- see the field's doc comment on
// instance.go for the double-start race this closes.
func (i *Instance) startPiStatusSource() error {
	i.piStatusStartMu.Lock()
	defer i.piStatusStartMu.Unlock()

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
	// Wire the "session" header event's ID back to this Instance (Task
	// 2.2.1e) so a later Restart's buildLaunchCommand call can inject
	// --session <id> and actually resume. Must be set before Start() --
	// see SetOnSessionIDCallback's doc comment.
	src.SetOnSessionIDCallback(i.SetPiSessionID)
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
	i.piSessionMu.Lock()
	var piSessionID string
	if i.piSession != nil {
		piSessionID = i.piSession.SessionID
	}
	i.piSessionMu.Unlock()

	return func() *exec.Cmd {
		args := []string{"--mode", "json"}
		if piSessionID != "" {
			args = append(args, "--session", piSessionID)
		}
		// context.Background(): this subprocess outlives any single request
		// and is torn down by PiStatusSource.Stop() calling cmd.Process.Kill()
		// directly (not via context cancellation) -- same convention as
		// session/native_process_manager.go's managed subprocess.
		cmd := safeexec.CommandContext(context.Background(), base, args...) //nolint:gosec // base is the operator-configured program command, same trust boundary as the tmux-launched invocation (buildPiCommand)
		if path != "" {
			cmd.Dir = path
		}
		return cmd
	}
}

// SetPiSessionID records the real pi session UUID observed from the
// status-only pi subprocess's "session" header event (Task 2.2.1e), so a
// later Restart's buildLaunchCommand call injects --session <id> and the pi
// conversation actually resumes across a restart. Mirrors
// SetClaudeConversationUUID's shape (instance_claude.go), but guarded by the
// dedicated piSessionMu rather than i.mu -- mirroring claudeSessionMu's
// rationale for claudeSession.
//
// Concurrency note: this is set from PiStatusSource's reader goroutine via
// the onSessionID callback. Restart's own piSession suppression/restore
// block (session/instance.go) also takes piSessionMu around its reads/writes
// of i.piSession, so this is safe structurally now -- not by relying on the
// (fragile, and broken by Bug 2's flag-flip leak) argument that Stop()
// always finishes joining the reader goroutine before Restart runs.
func (i *Instance) SetPiSessionID(id string) {
	if id == "" {
		return
	}
	i.piSessionMu.Lock()
	defer i.piSessionMu.Unlock()
	if i.piSession != nil && i.piSession.SessionID == id {
		return
	}
	if i.piSession == nil {
		i.piSession = &PiSessionData{}
	}
	i.piSession.SessionID = id
	i.piSession.LastAttached = time.Now()
}
