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

import "time"

// ---- MCPServerURL ----------------------------------------------------------------

func setMCPServerURLLocked(s *instanceState, url string) {
	s.inst.MCPServerURL = url
	s.inst.snapshot.Store(buildSnapshot(s.inst))
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
	s.inst.AutonomousTurn = turn
	s.inst.AutonomousMaxTurns = maxTurns
	s.inst.snapshot.Store(buildSnapshot(s.inst))
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
	s.inst.AutonomousMode = mode
	s.inst.AutonomousOutcome = outcome
	s.inst.snapshot.Store(buildSnapshot(s.inst))
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
	s.inst.AutonomousMode = false
	s.inst.AutonomousTurn = 0
	s.inst.AutonomousMaxTurns = 0
	if done {
		s.inst.AutonomousOutcome = "done"
	} else {
		s.inst.AutonomousOutcome = "stuck"
	}
	s.inst.snapshot.Store(buildSnapshot(s.inst))
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
	s.inst.GitHubPRURL = prURL
	s.inst.GitHubPRNumber = prNumber
	s.inst.snapshot.Store(buildSnapshot(s.inst))
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
	s.inst.GitHubPRNumber = n
	s.inst.snapshot.Store(buildSnapshot(s.inst))
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
	s.inst.LastPRStatusCheck = t
	s.inst.snapshot.Store(buildSnapshot(s.inst))
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
	s.inst.ArchivedAt = t
	s.inst.snapshot.Store(buildSnapshot(s.inst))
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
		if s.inst.ArchivedAt != nil {
			return nil
		}
		s.inst.ArchivedAt = &t
		s.inst.snapshot.Store(buildSnapshot(s.inst))
		set = true
		return nil
	})
	return set
}

// ---- Program --------------------------------------------------------------------

func setProgramLocked(s *instanceState, program string) {
	s.inst.Program = program
	s.inst.snapshot.Store(buildSnapshot(s.inst))
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
	s.inst.AutoYes = v
	s.inst.snapshot.Store(buildSnapshot(s.inst))
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
	s.inst.Title = title
	s.inst.snapshot.Store(buildSnapshot(s.inst))
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
	s.inst.Category = category
	s.inst.snapshot.Store(buildSnapshot(s.inst))
}

// SetCategory sets the session category.
func (i *Instance) SetCategory(category string) {
	_ = i.sendSyncErr(func(s *instanceState) error {
		setCategoryLocked(s, category)
		return nil
	})
}

// ---- WorkingDir -----------------------------------------------------------------

func setWorkingDirLocked(s *instanceState, dir string) {
	s.inst.WorkingDir = dir
	s.inst.snapshot.Store(buildSnapshot(s.inst))
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
	s.inst.PauseReason = reason
	s.inst.snapshot.Store(buildSnapshot(s.inst))
}

// SetPauseReason sets the reason this session was paused.
func (i *Instance) SetPauseReason(reason string) {
	_ = i.sendSyncErr(func(s *instanceState) error {
		setPauseReasonLocked(s, reason)
		return nil
	})
}
