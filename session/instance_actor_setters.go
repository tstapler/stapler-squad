package session

// instance_actor_setters.go — actor-routed field setter methods (IAC Epic 5).
//
// Every function here follows the xxxLocked / public-wrapper pattern from Epic 4:
//   - xxxLocked(s *instanceState, ...)  — runs inside an actor command; writes
//     fields directly and updates the snapshot for nil-actor compatibility.
//   - Public method on *Instance          — routes through sendSyncErr so all writes
//     are serialised by the actor goroutine and never race with buildSnapshot.
//
// Files in server/services/, session/pr_status_poller.go,
// session/review_queue_poller.go, and daemon/daemon.go must call ONLY these
// methods (never direct `inst.Field = value` assignments).  The CI guard
// target `make actor-field-guard` enforces this invariant until Epic 7 makes
// it a go-build failure via field unexport.

import (
	"context"
	"time"
)

// ---- MCPServerURL ----------------------------------------------------------------
//
// Every xxxLocked function below takes i.mu.Lock() around its field write(s)
// AND the buildSnapshot() call, in one critical section (Store happens after
// Unlock, since it targets a separate atomic.Pointer and doesn't need mu).
//
// buildSnapshot reads every mutable Instance field in one pass. These actor
// commands otherwise mutate fields relying purely on actor-goroutine
// confinement (no lock needed relative to OTHER actor commands, since the
// mailbox serializes them onto one goroutine) — but a handful of legacy
// setters (MarkViewed, MarkUserResponded, MarkAcknowledged,
// SetLastMeaningfulOutput, RecoverFromStopped) still mutate fields directly
// from arbitrary caller goroutines while holding i.mu.Lock(), bypassing the
// actor entirely. Without taking i.mu here too, both the field write AND the
// buildSnapshot read can race with one of those legacy writers even though
// neither side is wrong by its own local contract — caught by -race via a
// concurrent MarkViewed()/ForceStatus() call during CreateSession. See
// runActor's doc comment in actor.go for the matching read-side fix.

func setMCPServerURLLocked(s *instanceState, url string) {
	s.inst.mu.Lock()
	s.inst.MCPServerURL = url
	snap := buildSnapshot(s.inst)
	s.inst.mu.Unlock()
	s.inst.snapshot.Store(snap)
}

// SetMCPServerURL sets the MCP server URL on this instance.
func (i *Instance) SetMCPServerURL(url string) {
	_ = i.sendSyncErr(func(s *instanceState) error {
		setMCPServerURLLocked(s, url)
		return nil
	})
}

// ---- CreationProgress ------------------------------------------------------------

func setCreationProgressLocked(s *instanceState, msg string) {
	s.inst.CreationProgress = msg
	// CreationProgress is not included in InstanceSnapshot (it is published via
	// the event bus directly from the live Instance pointer), so we do not call
	// snapshot.Store here.  Serialisation through the actor is still required to
	// avoid a data race with concurrent buildSnapshot calls on other fields.
}

// SetCreationProgress sets the human-readable creation progress message.
func (i *Instance) SetCreationProgress(msg string) {
	_ = i.sendSyncErr(func(s *instanceState) error {
		setCreationProgressLocked(s, msg)
		return nil
	})
}

// ---- AutonomousTurn + AutonomousMaxTurns ----------------------------------------

func setAutonomousTurnLocked(s *instanceState, turn, maxTurns int32) {
	s.inst.mu.Lock()
	s.inst.AutonomousTurn = turn
	s.inst.AutonomousMaxTurns = maxTurns
	snap := buildSnapshot(s.inst)
	s.inst.mu.Unlock()
	s.inst.snapshot.Store(snap)
}

// SetAutonomousTurn atomically updates the current turn counter and max-turns
// cap during an active autonomous run.
func (i *Instance) SetAutonomousTurn(turn, maxTurns int32) {
	_ = i.sendSyncErr(func(s *instanceState) error {
		setAutonomousTurnLocked(s, turn, maxTurns)
		return nil
	})
}

// ---- AutonomousMode toggle (UpdateSession RPC) -----------------------------------

func setAutonomousModeLocked(s *instanceState, mode bool, outcome string) {
	s.inst.mu.Lock()
	s.inst.AutonomousMode = mode
	s.inst.AutonomousOutcome = outcome
	snap := buildSnapshot(s.inst)
	s.inst.mu.Unlock()
	s.inst.snapshot.Store(snap)
}

// SetAutonomousMode sets the autonomous mode flag and outcome string atomically.
// Pass outcome="" to clear it when enabling; the existing value is preserved
// unless explicitly overwritten by the caller.
func (i *Instance) SetAutonomousMode(mode bool, outcome string) {
	_ = i.sendSyncErr(func(s *instanceState) error {
		setAutonomousModeLocked(s, mode, outcome)
		return nil
	})
}

// ---- Autonomous driver completion ------------------------------------------------

func setAutonomousCompleteLocked(s *instanceState, done bool) {
	s.inst.mu.Lock()
	s.inst.AutonomousMode = false
	s.inst.AutonomousTurn = 0
	s.inst.AutonomousMaxTurns = 0
	if done {
		s.inst.AutonomousOutcome = "done"
	} else {
		s.inst.AutonomousOutcome = "stuck"
	}
	snap := buildSnapshot(s.inst)
	s.inst.mu.Unlock()
	s.inst.snapshot.Store(snap)
}

// SetAutonomousComplete clears the autonomous-mode flag and turn counters,
// and records the outcome ("done" or "stuck") atomically.
func (i *Instance) SetAutonomousComplete(done bool) {
	_ = i.sendSyncErr(func(s *instanceState) error {
		setAutonomousCompleteLocked(s, done)
		return nil
	})
}

// ---- GitHubPRURL + GitHubPRNumber -----------------------------------------------

func setGitHubPRURLLocked(s *instanceState, prURL string, prNumber int) {
	s.inst.mu.Lock()
	s.inst.GitHubPRURL = prURL
	s.inst.GitHubPRNumber = prNumber
	snap := buildSnapshot(s.inst)
	s.inst.mu.Unlock()
	s.inst.snapshot.Store(snap)
}

// SetGitHubPR atomically sets the GitHub PR URL and PR number discovered after a
// RunOneShot or PR-discovery poll.  Pass prNumber=0 if not yet known.
func (i *Instance) SetGitHubPR(prURL string, prNumber int) {
	_ = i.sendSyncErr(func(s *instanceState) error {
		setGitHubPRURLLocked(s, prURL, prNumber)
		return nil
	})
}

// ---- GitHubPRNumber only (PR-status-poller discovery) ---------------------------

func setGitHubPRNumberLocked(s *instanceState, n int) {
	s.inst.mu.Lock()
	s.inst.GitHubPRNumber = n
	snap := buildSnapshot(s.inst)
	s.inst.mu.Unlock()
	s.inst.snapshot.Store(snap)
}

// SetGitHubPRNumber atomically updates the in-memory GitHubPRNumber field.
// Replaces the stateMutex-based implementation; now actor-routed so it is
// serialised with buildSnapshot.
func (i *Instance) SetGitHubPRNumber(n int) {
	_ = i.sendSyncErr(func(s *instanceState) error {
		setGitHubPRNumberLocked(s, n)
		return nil
	})
}

// ---- LastPRStatusCheck ----------------------------------------------------------

func setLastPRStatusCheckLocked(s *instanceState, t time.Time) {
	s.inst.mu.Lock()
	s.inst.LastPRStatusCheck = t
	snap := buildSnapshot(s.inst)
	s.inst.mu.Unlock()
	s.inst.snapshot.Store(snap)
}

// SetLastPRStatusCheck records the time of the most recent PR-status fetch.
func (i *Instance) SetLastPRStatusCheck(t time.Time) {
	_ = i.sendSyncErr(func(s *instanceState) error {
		setLastPRStatusCheckLocked(s, t)
		return nil
	})
}

// ---- ArchivedAt -----------------------------------------------------------------

func setArchivedAtLocked(s *instanceState, t *time.Time) {
	s.inst.mu.Lock()
	s.inst.ArchivedAt = t
	snap := buildSnapshot(s.inst)
	s.inst.mu.Unlock()
	s.inst.snapshot.Store(snap)
}

// SetArchivedAt sets or clears the ArchivedAt timestamp atomically.
// Pass nil to clear (unarchive).
func (i *Instance) SetArchivedAt(t *time.Time) {
	_ = i.sendSyncErr(func(s *instanceState) error {
		setArchivedAtLocked(s, t)
		return nil
	})
}

// SetArchivedAtIfNil sets ArchivedAt to t only if it is currently nil.
// Returns true if the value was set (CAS semantics).  Now actor-routed.
func (i *Instance) SetArchivedAtIfNil(t time.Time) bool {
	var set bool
	_ = i.sendSyncErr(func(s *instanceState) error {
		s.inst.mu.Lock()
		if s.inst.ArchivedAt != nil {
			s.inst.mu.Unlock()
			return nil
		}
		s.inst.ArchivedAt = &t
		snap := buildSnapshot(s.inst)
		s.inst.mu.Unlock()
		s.inst.snapshot.Store(snap)
		set = true
		return nil
	})
	return set
}

// stopIfNotStoppedLocked transitions the instance to Stopped from within an
// actor command, unless it is already Stopped (Stopped→Stopped is not a
// defined transition edge, so calling transitionToLocked unconditionally
// would return ErrInvalidTransition for an already-archived-and-stopped
// session on a repeat sweep).
func stopIfNotStoppedLocked(s *instanceState, ctx context.Context) error {
	if s.inst.Status == Stopped {
		return nil
	}
	return transitionToLocked(s, ctx, Stopped)
}

// ArchiveWithStop sets ArchivedAt and transitions the instance to Stopped, in a
// single actor command. ArchiveSession previously only set ArchivedAt, which let
// a session sit with ArchivedAt set but Status still Active/Paused/Hibernated —
// archiving is meant to mean "this session is done," so the two must move together.
func (i *Instance) ArchiveWithStop(t time.Time) error {
	return i.sendSyncErr(func(s *instanceState) error {
		setArchivedAtLocked(s, &t)
		return stopIfNotStoppedLocked(s, context.Background())
	})
}

// SetArchivedAtIfNilAndStop is the CAS counterpart of ArchiveWithStop, used by
// ArchiveSessionByUUID (which must be safe to call unconditionally from a sweep).
// Returns true if ArchivedAt was set by this call (i.e. it was previously nil).
// No-ops the status transition if already Stopped.
func (i *Instance) SetArchivedAtIfNilAndStop(t time.Time) bool {
	var set bool
	_ = i.sendSyncErr(func(s *instanceState) error {
		s.inst.mu.Lock()
		if s.inst.ArchivedAt != nil {
			s.inst.mu.Unlock()
			return nil
		}
		s.inst.ArchivedAt = &t
		snap := buildSnapshot(s.inst)
		s.inst.mu.Unlock()
		s.inst.snapshot.Store(snap)
		set = true
		return stopIfNotStoppedLocked(s, context.Background())
	})
	return set
}

// ---- Program --------------------------------------------------------------------

func setProgramLocked(s *instanceState, program string) {
	s.inst.mu.Lock()
	s.inst.Program = program
	snap := buildSnapshot(s.inst)
	s.inst.mu.Unlock()
	s.inst.snapshot.Store(snap)
}

// SetProgram atomically updates the Program field during program-switch.
func (i *Instance) SetProgram(program string) {
	_ = i.sendSyncErr(func(s *instanceState) error {
		setProgramLocked(s, program)
		return nil
	})
}

// ---- LastAddedToQueue -----------------------------------------------------------

func setLastAddedToQueueLocked(s *instanceState, t time.Time) {
	s.inst.LastAddedToQueue = t
	// LastAddedToQueue is not included in InstanceSnapshot; no snapshot.Store needed.
}

// SetLastAddedToQueue records when this session was last added to the review queue.
func (i *Instance) SetLastAddedToQueue(t time.Time) {
	_ = i.sendSyncErr(func(s *instanceState) error {
		setLastAddedToQueueLocked(s, t)
		return nil
	})
}

// ---- AutoYes --------------------------------------------------------------------

func setAutoYesLocked(s *instanceState, v bool) {
	s.inst.mu.Lock()
	s.inst.AutoYes = v
	snap := buildSnapshot(s.inst)
	s.inst.mu.Unlock()
	s.inst.snapshot.Store(snap)
}

// SetAutoYes sets the AutoYes flag. Used by daemon.go to opt in automated
// sessions to non-interactive behaviour.
func (i *Instance) SetAutoYes(v bool) {
	_ = i.sendSyncErr(func(s *instanceState) error {
		setAutoYesLocked(s, v)
		return nil
	})
}

// ---- Title (direct/rollback path) -----------------------------------------------

func setTitleDirectLocked(s *instanceState, title string) {
	s.inst.mu.Lock()
	s.inst.Title = title
	snap := buildSnapshot(s.inst)
	s.inst.mu.Unlock()
	s.inst.snapshot.Store(snap)
}

// SetTitleDirect sets the Title field directly without tmux-session constraints.
// Use only from RPC handlers that have already validated uniqueness and title
// constraints (e.g. UpdateSession, RenameSession rollback).
func (i *Instance) SetTitleDirect(title string) {
	_ = i.sendSyncErr(func(s *instanceState) error {
		setTitleDirectLocked(s, title)
		return nil
	})
}

// ---- Category -------------------------------------------------------------------

func setCategoryLocked(s *instanceState, category string) {
	s.inst.mu.Lock()
	s.inst.Category = category
	snap := buildSnapshot(s.inst)
	s.inst.mu.Unlock()
	s.inst.snapshot.Store(snap)
}

// SetCategory sets the session category.
func (i *Instance) SetCategory(category string) {
	_ = i.sendSyncErr(func(s *instanceState) error {
		setCategoryLocked(s, category)
		return nil
	})
}

// ---- Note -----------------------------------------------------------------------

func setNoteLocked(s *instanceState, note string) {
	s.inst.mu.Lock()
	s.inst.Note = note
	// Bump UpdatedAt so the frontend's upsertSession no-op dedup (sessionsSlice.ts,
	// keyed on unchanged updatedAt) doesn't silently drop this update — without it,
	// a note saved on a session whose UpdatedAt hadn't otherwise moved (e.g. a
	// freshly created, still-idle session) would return success from the RPC but
	// never appear in the UI until some other field bumped UpdatedAt first.
	s.inst.UpdatedAt = time.Now()
	snap := buildSnapshot(s.inst)
	s.inst.mu.Unlock()
	s.inst.snapshot.Store(snap)
}

// SetNote sets the session's free-form markdown note.
func (i *Instance) SetNote(note string) {
	_ = i.sendSyncErr(func(s *instanceState) error {
		setNoteLocked(s, note)
		return nil
	})
}

// ---- WorkingDir -----------------------------------------------------------------

func setWorkingDirLocked(s *instanceState, dir string) {
	s.inst.mu.Lock()
	s.inst.WorkingDir = dir
	snap := buildSnapshot(s.inst)
	s.inst.mu.Unlock()
	s.inst.snapshot.Store(snap)
}

// SetWorkingDir sets the working directory for this session.
func (i *Instance) SetWorkingDir(dir string) {
	_ = i.sendSyncErr(func(s *instanceState) error {
		setWorkingDirLocked(s, dir)
		return nil
	})
}

// ---- PauseReason ----------------------------------------------------------------

func setPauseReasonLocked(s *instanceState, reason string) {
	s.inst.mu.Lock()
	s.inst.PauseReason = reason
	snap := buildSnapshot(s.inst)
	s.inst.mu.Unlock()
	s.inst.snapshot.Store(snap)
}

// SetPauseReason sets the reason this session was paused.
func (i *Instance) SetPauseReason(reason string) {
	_ = i.sendSyncErr(func(s *instanceState) error {
		setPauseReasonLocked(s, reason)
		return nil
	})
}

// ---- ExitReason -------------------------------------------------------------

func setExitReasonLocked(s *instanceState, reason string) {
	s.inst.mu.Lock()
	s.inst.ExitReason = reason
	snap := buildSnapshot(s.inst)
	s.inst.mu.Unlock()
	s.inst.snapshot.Store(snap)
}

// SetExitReason sets (or, passed "", clears) the reason this session's pane crashed.
func (i *Instance) SetExitReason(reason string) {
	_ = i.sendSyncErr(func(s *instanceState) error {
		setExitReasonLocked(s, reason)
		return nil
	})
}
