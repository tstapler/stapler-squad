package session

import (
	"context"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// ReconcileOrphanedTmuxSessions kills staplersquad_ tmux sessions that have no
// corresponding record in the current workspace DB.
//
// Orphans accumulate when DeleteSession removes the DB record but the server is
// restarted before (or while) the live in-memory instance is available — leaving
// the Claude process running inside a tmux pane with no owner. This sweep is called
// once during server startup, after all DB sessions have been loaded and re-adopted
// (steps 6/6b of BuildRuntimeDeps), so there is no risk of killing a session that
// is mid-adoption.
//
// Identification strategy (two-tier):
//  1. Tmux session has STAPLER_SESSION_UUID env var → compare against known UUIDs.
//  2. No env var (pre-UUID sessions) → compare the tmux session name against known
//     sanitized titles. If neither matches, the session is an orphan.
//
// The staplersquad_keepalive sentinel is always preserved — it keeps the tmux
// server alive between sessions and is never tracked in the DB.
func ReconcileOrphanedTmuxSessions(instances []*Instance) {
	// Resolve once, here -- not per-command below. This is what actually makes this
	// function test-safe: inside a `go test` binary it targets a per-process
	// isolated socket instead of the real shared default, so it can never again
	// enumerate-and-kill sessions belonging to some other, currently-running
	// stapler-squad process on the same machine. See tmux.ResolveSocket's doc
	// comment for the incident history this closes.
	socket := tmux.ResolveSocket("")
	socketArgs := func(args ...string) []string {
		if socket == "" {
			return args
		}
		return append([]string{"-L", socket}, args...)
	}

	knownUUIDs := make(map[string]struct{}, len(instances))
	knownTmuxNames := make(map[string]struct{}, len(instances))
	for _, inst := range instances {
		if inst.UUID != "" {
			knownUUIDs[inst.UUID] = struct{}{}
		}
		if name := inst.GetTmuxSessionName(); name != "" {
			knownTmuxNames[name] = struct{}{}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	out, err := safeexec.CommandContext(ctx, tmux.Binary(), socketArgs("list-sessions", "-F", "#{session_name}")...).Output()
	if err != nil {
		// tmux not running or no sessions — nothing to sweep
		return
	}

	keepalive := tmux.TmuxPrefix + "keepalive"

	killed := 0
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		sessionName := strings.TrimSpace(line)
		if sessionName == "" || !strings.HasPrefix(sessionName, tmux.TmuxPrefix) {
			continue
		}
		// Always preserve the keepalive sentinel session.
		if sessionName == keepalive {
			continue
		}
		// Session name matches a known DB session → re-adopted, leave alone.
		if _, known := knownTmuxNames[sessionName]; known {
			continue
		}

		// Try to match via STAPLER_SESSION_UUID from the tmux session environment.
		envOut, envErr := safeexec.CommandContext(ctx, tmux.Binary(), socketArgs("show-environment", "-t", sessionName, "STAPLER_SESSION_UUID")...).Output()
		if envErr == nil {
			uuid := strings.TrimPrefix(strings.TrimSpace(string(envOut)), "STAPLER_SESSION_UUID=")
			if _, known := knownUUIDs[uuid]; known {
				// UUID matches a DB session that happened to be renamed or sanitized
				// differently. Don't kill it.
				continue
			}
		}

		// No DB match by name or UUID — this is an orphan.
		log.Info("orphan sweep: killing stale tmux session with no DB record", "session", sessionName)
		killCtx, killCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if killErr := safeexec.CommandContext(killCtx, tmux.Binary(), socketArgs("kill-session", "-t", sessionName)...).Run(); killErr != nil {
			log.Warn("orphan sweep: failed to kill stale session", "session", sessionName, "err", killErr)
		} else {
			killed++
		}
		killCancel()
	}

	if killed > 0 {
		log.Info("orphan sweep: complete", "killed", killed)
	}
}
