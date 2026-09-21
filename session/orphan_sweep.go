package session

import (
	"context"
	"strconv"
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
// the Claude process running inside a tmux pane with no owner. This was originally
// called only once during server startup, after all DB sessions have been loaded
// and re-adopted (steps 6/6b of BuildRuntimeDeps), specifically because at that
// point in the sequence no new session creation can be racing it. OrphanedTmuxSweeper
// now also calls this periodically during live operation to catch orphans that
// accumulate between restarts (e.g. a crash mid-DeleteSession, or a KillTmuxPaneOnly
// call in archiveItemWorkSessions that silently failed) — see its doc comment for
// why minAge exists and is mandatory there.
//
// Identification strategy:
//  1. Tmux session has STAPLER_SESSION_UUID env var → compare against known UUIDs.
//  2. No env var (pre-UUID sessions, and all shell sibling sessions) → compare the
//     tmux session name against known instance tmux names and known shell tmux
//     names. If none of the above match, the session is an orphan.
//
// The staplersquad_keepalive sentinel is always preserved — it keeps the tmux
// server alive between sessions and is never tracked in the DB.
//
// minAge is a grace period: a candidate orphan whose tmux session is younger than
// minAge is never killed, regardless of whether it appears in instances. This
// closes an unavoidable race for any periodic (post-startup) caller — CreateSession
// calls instance.Start() (which creates the tmux session) before it registers the
// new instance with the live provider a periodic sweep reads from (AddInstance),
// so a sweep tick landing in that window would otherwise see a legitimate
// brand-new session as an orphan. Pass 0 only for the one-time startup call, where
// this race cannot occur (no new session creation is possible before BuildRuntimeDeps
// finishes). Any periodic caller MUST pass a nonzero minAge.
func ReconcileOrphanedTmuxSessions(instances []*Instance, minAge time.Duration) {
	// Resolve once, here -- not per-command below. This is what actually makes this
	// function test-safe: inside a `go test` binary it targets a per-process
	// isolated socket instead of the real shared default, so it can never again
	// enumerate-and-kill sessions belonging to some other, currently-running
	// stapler-squad process on the same machine. See tmux.ResolveSocket's doc
	// comment for the incident history this closes.
	socketArgs := tmux.ResolveSocket("").Args

	knownUUIDs := make(map[string]struct{}, len(instances))
	knownTmuxNames := make(map[string]struct{}, len(instances))
	for _, inst := range instances {
		if inst.UUID != "" {
			knownUUIDs[inst.UUID] = struct{}{}
		}
		if name := inst.GetTmuxSessionName(); name != "" {
			knownTmuxNames[name] = struct{}{}
		}
		// Shells are independent sibling tmux sessions (see instance_shells.go) with
		// no Instance-level identity of their own — without this, every shell is
		// unconditionally treated as an orphan and killed on the next sweep/restart.
		for _, shell := range inst.shells.List() {
			if shell.TmuxSessionName != "" {
				knownTmuxNames[shell.TmuxSessionName] = struct{}{}
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// #{session_created} (unix seconds) rides along in the same call so the age
	// check below needs no extra per-candidate subprocess round-trip.
	out, err := safeexec.CommandContext(ctx, tmux.Binary(), socketArgs("list-sessions", "-F", "#{session_name}\t#{session_created}")...).Output()
	if err != nil {
		// tmux not running or no sessions — nothing to sweep
		return
	}

	keepalive := tmux.TmuxPrefix + "keepalive"

	killed := 0
	skippedYoung := 0
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		sessionName, createdRaw, _ := strings.Cut(line, "\t")
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

		// Grace period: never treat a session younger than minAge as an orphan —
		// see the doc comment above for the registration-race this closes.
		if minAge > 0 {
			if createdUnix, parseErr := strconv.ParseInt(createdRaw, 10, 64); parseErr == nil {
				age := time.Since(time.Unix(createdUnix, 0))
				if age < minAge {
					log.Debug("orphan sweep: skipping recently created session, still within grace period",
						"session", sessionName, "age", age.Round(time.Second))
					skippedYoung++
					continue
				}
			}
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

	if killed > 0 || skippedYoung > 0 {
		log.Info("orphan sweep: complete", "killed", killed, "skipped_within_grace_period", skippedYoung)
	}
}
