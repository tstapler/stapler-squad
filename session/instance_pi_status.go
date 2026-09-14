package session

// instance_pi_status.go wires Epic 5.2's PiStatusSource into Instance's
// existing StartController/StopController lifecycle (session/instance_controller.go),
// mirroring how a ClaudeController is registered/unregistered but via the
// piSources parallel map (session/instance_status.go), not the controllers map.

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/log"
)

// piExtension implements programExtension (instance_controller.go) so
// StartController/StopController can route to a program-specific extension.
// This enables custom program support for the pi coding assistant via
// API/UI/MCP registration.
type piExtension struct {
  // piSession holds pi-coding-agent session information for
  // resume-on-restart (see buildPiCommand). Populated only when
  // isPi(i.Program) and the pi-support feature flag is enabled -- see
  // the capture in Restart. Guarded by piSessionMu.
  piSession *PiSessionData
  // piSessionMu protects piSession. Separate from i.mu: SetPiSessionID is
  // called from PiStatusSource's reader goroutine
  // callback) and Restart touches piSession too, so a dedicated lock makes
  // both access points structurally safe without relying on the (fragile,
  // and broken by Bug 2) argument that stopController always finishes
  // joining the writer goroutine before Restart reads/writes piSession.
  piSessionMu sync.Mutex

  // piStatusSrc holds the status-only `pi --mode json` subprocess (Epic
  // 5.2) for this instance. Set by startController, cleared by
  // stopController. atomic.Pointer since Start
  // can race concurrent GetController-style reads.
  piStatusSrc atomic.Pointer[PiStatusSource]

  // piStatusStartMu serializes startController
  // (load piStatusSrc, and if nil construct+Start()+Store() a new
  // PiStatusSource). Without it, two concurrent
  // startController calls on the same pi-backed instance can both observe a nil piStatusSrc, both
  // spawn a subprocess+goroutine pair, and the loser's PiStatusSource is
  // silently overwritten by the winner's Store() with no Stop() ever
  // called on it -- a leaked subprocess and restarted status manager.
  piStatusStartMu sync.Mutex

  // piController holds the PiStatusSource controller for this instance.
  // This is the new controller approach implemented as part of the
  // programExtension interface changes, replacing the old piStatusSrc
  // approach for consistency with other program extensions.
  piController *PiStatusSource
}

type piExtension struct {
  // piSession holds pi-coding-agent session information for
  // resume-on-restart (see buildPiCommand). Populated only when
  // isPi(i.Program) and the pi-support feature flag is enabled -- see
  // the capture in Restart. Guarded by piSessionMu.
  piSession *PiSessionData
  // piSessionMu protects piSession. Separate from i.mu: SetPiSessionID is
  // called from PiStatusSource's reader goroutine
  // callback) and Restart touches piSession too, so a dedicated lock makes
  // both access points structurally safe without relying on the (fragile,
  // and broken by Bug 2) argument that stopController always finishes
  // joining the writer goroutine before Restart reads/writes piSession.
  piSessionMu sync.Mutex

  // piStatusSrc holds the status-only `pi --mode json` subprocess (Epic
  // 5.2) for this instance. Set by startController, cleared by
  // stopController. atomic.Pointer since Start
  // can race concurrent GetController-style reads.
  piStatusSrc atomic.Pointer[PiStatusSource]

  // piStatusStartMu serializes startController
  // (load piStatusSrc, and if nil construct+Start()+Store() a new
  // PiStatusSource). Without it, two concurrent
  // startController calls on the same pi-backed instance can both observe a nil piStatusSrc, both
  // spawn a subprocess+goroutine pair, and the loser's PiStatusSource is
  // silently overwritten by the winner's Store() with no Stop() ever
  // called on it -- a leaked subprocess and restarted status manager.
  piStatusStartMu sync.Mutex

  // piController holds the PiStatusSource controller for this instance.
  // This is the new controller approach implemented as part of the
  // programExtension interface changes, replacing the old piStatusSrc
  // approach for consistency with other program extensions.
  piController *PiStatusSource
}

var _ programExtension = (*piExtension)(nil)

func (e *piExtension) Supported(i *Instance) bool {
	return isPi(i.Program)
}

func (e *piExtension) Running() bool {
	return i != nil && i.piSession != nil
}

func (e *piExtension) StartController(i *Instance) error {
	if !e.Supported(i) {
		return fmt.Errorf("pi program not supported for instance %s", i.Title)
	}

	// Create and start the PiStatusSource controller
	controller := NewPiStatusSource(i)
	if err := controller.Start(context.Background()); err != nil {
		return fmt.Errorf("failed to start pi controller: %w", err)
	}

	i.piController = controller
	log.Info("started pi controller", "session", i.Title)
	return nil
}

func (e *piExtension) StopController(i *Instance) {
	if i.piController != nil {
		i.piController.Stop()
		i.piController = nil
		log.Info("stopped pi controller", "session", i.Title)
	}
}

func (e *piExtension) BuildCommand(i *Instance) string {
	if !e.Supported(i) {
		return ""
	}
	// Build the command for the pi program using the same logic as the launch builder
	var cmdBuilder strings.Builder
	cmdBuilder.WriteString(i.Program)
	if i.Prefix != "" {
		cmdBuilder.WriteString(" -p ")
		cmdBuilder.WriteString(i.Prefix)
	}
	cmdBuilder.WriteString(" start")

	if i.ProgramArgs != "" {
		cmdBuilder.WriteString(" ")
		cmdBuilder.WriteString(i.ProgramArgs)
	}

	return cmdBuilder.String()
}

// piStatusSupported is a thin Instance-level alias for the promoted
// piExtension.supported method, kept because this package's tests exercise
// the Bug 2 fix directly by this name.
func (i *Instance) piStatusSupported() bool {
	return i.supported(i)
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
