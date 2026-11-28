// Package sessionports holds the session-control ports that the backlog service and
// the superseded-round sweeper consume; *services.SessionService implements them.
package sessionports

import (
	"context"
	"time"

	"github.com/tstapler/stapler-squad/server/services/access"
	"github.com/tstapler/stapler-squad/session/instance"
)

// AutonomousDriverStarter allows BacklogService to start an AutonomousDriver on an existing instance.
// Wired via SetAutonomousDriverStarter from server.go after both services are constructed.
type AutonomousDriverStarter interface {
	StartAutonomousDriverForInstance(inst *instance.Instance)
	StartAutonomousDriverWithTimeout(inst *instance.Instance, startupTimeout time.Duration)
}

// SessionStopper allows BacklogService to kill live sessions.
// It is nil-safe: BacklogService degrades gracefully when not wired.
type SessionStopper interface {
	StopSessionByUUID(ctx context.Context, sessionUUID string) error
	// KillTmuxSessionByTitle kills a tmux session by its title, regardless of
	// whether the Instance is still tracked in memory. Used to clear stale tmux
	// sessions before re-triggering so the fresh session gets its --append-system-prompt.
	// allowedOwnerUUIDs are the Instance UUID(s) the caller trusts to be
	// killed under this title -- see the implementation's doc comment
	// (ce71ad1a: a name match alone isn't enough to prove ownership).
	KillTmuxSessionByTitle(ctx context.Context, title string, allowedOwnerUUIDs ...string) error
	// IsSessionLive returns true if the session UUID is currently tracked in the
	// live in-memory poller. Used to distinguish genuinely-running sessions from
	// sessions that exited but whose DB records were not closed (e.g. after a
	// server restart that killed the underlying process).
	IsSessionLive(sessionUUID string) bool
	// KillTmuxPaneOnly closes the tmux pane for sessionUUID without touching its
	// worktree — unlike StopSessionByUUID/Instance.Kill, which also runs
	// CleanupWorktree. Rework rounds share one worktree/branch across their "-rN"
	// revisions (see buildRevisionTitle), so tearing down a finished round's
	// worktree would destroy the next round's checkout. No-op if the session
	// isn't tracked live (already gone).
	KillTmuxPaneOnly(ctx context.Context, sessionUUID string) error
	// ArchiveSessionByUUID soft-archives a session so it stops accumulating in
	// the default session list once its backlog item is done/superseded. No-op
	// (not an error) if the session isn't tracked live or is already archived.
	ArchiveSessionByUUID(ctx context.Context, sessionUUID string) error
	// TimeSinceLastMeaningfulOutput returns how long it has been since the live
	// Instance for sessionUUID last produced meaningful terminal output, backed
	// by the same Instance.GetTimeSinceLastMeaningfulOutput signal
	// review_queue_determiner.go's staleness detector uses — so "is this
	// session stale" has exactly one definition across the codebase instead of
	// each call site re-deriving its own. ok is false if the session isn't
	// currently tracked live (same "not live" cases as IsSessionLive); dur is
	// meaningless when ok is false.
	TimeSinceLastMeaningfulOutput(sessionUUID string) (dur time.Duration, ok bool)
	// IsRetryPending returns true if sessionUUID's live Instance currently has
	// an automated retry claimed or scheduled by the configurable retry policy
	// (session-retry-backoff). RemediateStaleWorkSession defers rather than
	// killing the pane and respawning when this is true, so the two
	// mechanisms never race the same session's process-level state (AC8).
	// Returns false if the session isn't tracked live.
	IsRetryPending(sessionUUID string) bool
}

// SessionSteerer allows BacklogService to inject a message into an already-
// active session (e.g. a PR-fix problem description) instead of skipping a
// respawn outright. It is nil-safe: BacklogService degrades gracefully when
// not wired, mirroring SessionStopper.
type SessionSteerer interface {
	SessionProgram(sessionUUID string) (program string, ok bool)
	// IsHiddenSession reports whether sessionUUID is a live hidden
	// (background) session. The automated steers refuse such a target: they
	// write to the item's work session, never to a hidden review, diagnose or
	// triage session (Story 5.1d, T-RO-39).
	IsHiddenSession(sessionUUID string) bool
	// IsReadyForSteer reports whether sessionUUID's pane is confirmed idle
	// and safe for an unattended PTY write. False — including when
	// readiness can't be determined — means the caller must not steer (see
	// *SessionService.IsReadyForSteer's doc comment for why "unknown" must
	// never default to true here).
	IsReadyForSteer(sessionUUID string) bool
	// SteerActiveSession writes message to the session unguarded (verdict steering).
	SteerActiveSession(ctx context.Context, sessionUUID, message string) error
	// SteerSessionGuarded delivers message under the per-session nudge guard
	// shared with the manual PR nudge: sig is the reason signature used for
	// the duplicate window, and a guard-caused outcome (SteerGuardBusy,
	// SteerDuplicate) is not a delivery failure.
	SteerSessionGuarded(ctx context.Context, sessionUUID, sig, message string) (access.SteerOutcome, error)
}
