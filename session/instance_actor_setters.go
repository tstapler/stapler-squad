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
	// Bumped in the same actor command as the progress-text write (not a second
	// mailbox round-trip) so CreationProgressUpdatedAt always reflects the most
	// recent SetCreationProgress call — see its doc comment in instance.go.
	s.inst.creationProgressUpdatedAt = time.Now()
	// CreationProgress is not included in InstanceSnapshot (it is published via
	// the event bus directly from the live Instance pointer), so we do not call
	// snapshot.Store here.  Serialisation through the actor is still required to
	// avoid a data race with concurrent buildSnapshot calls on other fields.
}

// setFailureReasonLocked sets the human-readable reason the async creation
// pipeline failed. Callable exclusively from within TryForceStatusIfEpoch's
// own command closure (Epic 1.2, ADR-002) — there is no public setter;
// FailureReason is terminal-write metadata, not independently-settable
// progress text (contrast setCreationProgressLocked above).
func setFailureReasonLocked(s *instanceState, reason string) {
	s.inst.failureReason = reason
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

// ---- AutoApprove ------------------------------------------------------------------

func setAutoApproveLocked(s *instanceState, v bool) {
	s.inst.mu.Lock()
	s.inst.AutoApprove = v
	snap := buildSnapshot(s.inst)
	s.inst.mu.Unlock()
	s.inst.snapshot.Store(snap)
}

// SetAutoApprove sets the AutoApprove flag and, if the session is currently Active,
// restarts it so the flag takes effect immediately (the flag is baked into the launch
// command at spawn time, like Program -- not re-checked live, unlike AutonomousMode).
// persist is called before the restart so a crash between setting and restarting doesn't
// lose the change; the field mutation and persist survive even if the subsequent restart
// fails (matching SwitchProgram's documented ordering).
//
// The full set->persist->restart sequence runs under restartTriggerMu, the same lock
// SwitchProgram holds for its own restart sequence, so a program switch and an
// auto-approve toggle firing near-simultaneously on the same instance serialize instead
// of both observing Status == Active and double-restarting the tmux session.
func (i *Instance) SetAutoApprove(v bool, persist func() error) error {
	i.restartTriggerMu.Lock()
	defer i.restartTriggerMu.Unlock()

	if err := i.sendSyncErr(func(s *instanceState) error {
		setAutoApproveLocked(s, v)
		return nil
	}); err != nil {
		return err
	}
	if persist != nil {
		if err := persist(); err != nil {
			return err
		}
	}
	if i.Status == Active {
		return i.Restart(true)
	}
	return nil
}

// ---- CreationEpoch fencing (Epic 1.2, ADR-002) -----------------------------------
//
// creationEpoch is not part of InstanceSnapshot (like CreationProgress/failureReason
// above), so its Locked helpers need no i.mu.Lock()/buildSnapshot — actor-goroutine
// confinement alone serializes them against other actor commands.

// bumpCreationEpochLocked increments creationEpoch by one and returns the new value.
// Called only from cancel/retry code paths (Phase 3) and TryStartRetry below.
func bumpCreationEpochLocked(s *instanceState) uint64 {
	s.inst.creationEpoch++
	return s.inst.creationEpoch
}

// bumpCreationEpoch increments creationEpoch by one, actor-routed, and returns the
// new value. Exercised directly by tests; production callers are the cancel and
// retry code paths added in Phase 3.
func (i *Instance) bumpCreationEpoch() uint64 {
	var newEpoch uint64
	_ = i.sendSyncErr(func(s *instanceState) error {
		newEpoch = bumpCreationEpochLocked(s)
		return nil
	})
	return newEpoch
}

// BumpCreationEpoch is bumpCreationEpoch's exported form, callable from
// server/services' CancelSessionCreation RPC handler (Epic 3.2, Task
// 3.2.1b/c): fencing out a racing pipeline's terminal write is itself an
// actor-routed mailbox round-trip, so the caller must issue it as its own
// command (not inline with a status read) to get the ordering guarantee
// TryForceStatusIfEpoch relies on.
func (i *Instance) BumpCreationEpoch() uint64 {
	return i.bumpCreationEpoch()
}

// CreationEpoch returns the current fencing epoch for the async creation pipeline.
func (i *Instance) CreationEpoch() uint64 {
	var epoch uint64
	_ = i.sendSyncErr(func(s *instanceState) error {
		epoch = s.inst.creationEpoch
		return nil
	})
	return epoch
}

// TryForceStatusIfEpoch is the one atomic terminal-write primitive (Epic 1.2,
// ADR-002): it applies a forced status transition and failure-reason write in a
// single actor command, gated on the caller presenting the epoch it captured
// before starting background work. If creationEpoch has moved since (a cancel or
// retry raced ahead of this caller), the write is a no-op and this returns false —
// a stale background writer can never win a terminal status transition.
//
// setFailureReasonLocked has no other caller: this is the only place failureReason
// is ever written, guaranteeing it can only change in the same atomic step as
// Status. Mirrors ForceStatus's loadStatus/touchUpdatedAt/buildSnapshot shape
// (instance_state.go) rather than transitionTo, for the same reason ForceStatus
// bypasses transition validation: this is called from ad hoc background-pipeline/
// sweeper goroutines that must be able to force a terminal state unconditionally
// once they win the epoch check.
//
// The epoch match alone is not sufficient to guarantee exactly one winner: the
// live pipeline goroutine and the Stale-Creation Sweeper (Epic 4.1) can both
// legitimately hold the same still-current epoch (nothing bumps it between a
// pipeline's own success and the sweeper independently deciding the same
// instance is stale) and race to call this concurrently. The additional guard
// on Status == Creating breaks that tie: TryForceStatusIfEpoch only ever forces
// a Creating instance to a terminal status, so whichever caller's command runs
// first inside the actor mailbox wins and transitions away from Creating; the
// second caller — same epoch, but Status no longer Creating — observes that and
// no-ops, per ADR-002's "compound check-and-set" framing.
//
// The guard also accepts Status == status (the caller's own target) as a win,
// not only Status == Creating: Instance.Start()'s pre-existing, unrelated
// startLocked logic (session/instance.go) already calls i.transitionTo(ctx,
// Active) directly as part of ordinary (non-async-creation) startup, entirely
// out-of-band from this epoch mechanism -- by the time the Background
// Resolution Pipeline's (Epic 2.2) own success path reaches
// commitTerminalStatus(..., Active, ""), Status has therefore already left
// Creating. Without this widened check, the pipeline's terminal write for a
// *bare success* would always no-op (Status != Creating), even though nothing
// actually raced it -- caught by
// TestBackgroundResolutionPipeline_should_TransitionToActive_When_ResolutionSucceeds
// failing with "terminal write skipped" despite no concurrent writer. Confirming
// an already-current target status is a safe, idempotent win: a genuinely
// different racer (e.g. the Stale-Creation Sweeper's Failed, per this doc
// comment's tie-break example above) still only matches when Status equals
// *its own* target, so the exactly-one-winner property for two DIFFERENT
// target statuses is unaffected.
func (i *Instance) TryForceStatusIfEpoch(capturedEpoch uint64, status Status, failureReason string) bool {
	var applied bool
	_ = i.sendSyncErr(func(s *instanceState) error {
		if s.inst.creationEpoch != capturedEpoch {
			return nil
		}
		// Status is read and written under i.mu here (not just actor confinement,
		// unlike creationEpoch) because RecoverFromStopped/RetryNow mutate Status
		// directly under i.mu from outside the actor mailbox entirely -- confirmed
		// via go test -race: the old unguarded read below raced against that
		// write. i.mu is the only mechanism both call paths share.
		s.inst.mu.Lock()
		if s.inst.Status != Creating && s.inst.Status != status {
			s.inst.mu.Unlock()
			return nil
		}
		setFailureReasonLocked(s, failureReason)
		s.inst.loadStatus(status)
		s.inst.touchUpdatedAt()
		snap := buildSnapshot(s.inst)
		s.inst.mu.Unlock()
		s.inst.snapshot.Store(snap)
		applied = true
		return nil
	})
	return applied
}

// TryStartRetry is the one atomic primitive for starting a retry (Epic 1.2,
// ADR-002 addendum): it no-ops unless Status == Failed at the moment the command
// executes inside the actor mailbox; otherwise it bumps creationEpoch, resets
// Status to Creating with fresh creation_progress, and returns (newEpoch, true).
// Two concurrent retry clicks can never both spawn a live pipeline sharing the
// same epoch, since only the first to execute inside the mailbox observes
// Status == Failed.
//
// The creation_progress reset routes through setCreationProgressLocked (the same
// locked helper SetCreationProgress uses), not a direct field write, so
// CreationProgressUpdatedAt is bumped to "now" atomically in the same command as
// the status/progress reset. A direct write that skipped this helper would leave
// CreationProgressUpdatedAt at its stale pre-retry value, and the Stale-Creation
// Sweeper (Epic 4.1) would then immediately re-flip the retried session to
// Failed/Stale on its next tick, before the new pipeline has a chance to run.
func (i *Instance) TryStartRetry() (newEpoch uint64, started bool) {
	_ = i.sendSyncErr(func(s *instanceState) error {
		if s.inst.Status != Failed {
			return nil
		}
		newEpoch = bumpCreationEpochLocked(s)
		s.inst.mu.Lock()
		s.inst.loadStatus(Creating)
		s.inst.touchUpdatedAt()
		snap := buildSnapshot(s.inst)
		s.inst.mu.Unlock()
		s.inst.snapshot.Store(snap)
		setCreationProgressLocked(s, "")
		started = true
		return nil
	})
	if !started {
		return 0, false
	}
	return newEpoch, true
}

// ---- Background Resolution cancelFunc (Epic 2.2, Story 2.2.1) -------------------
//
// creationCancelFunc is not part of InstanceSnapshot and is process-local by
// nature (a context.CancelFunc cannot be persisted/reconstructed) -- like
// creationEpoch, actor-goroutine confinement alone serializes access to it, so
// no i.mu.Lock()/buildSnapshot is needed here.

// SetCreationCancelFunc stores cancel, the CancelFunc for this instance's
// Background Resolution Context, at pipeline-spawn time. Called once per
// pipeline invocation (including a retry, which spawns a fresh context) --
// reachable by the Cancel RPC (Epic 3.2) via CreationCancelFunc below.
func (i *Instance) SetCreationCancelFunc(cancel context.CancelFunc) {
	_ = i.sendSyncErr(func(s *instanceState) error {
		s.inst.creationCancelFunc = cancel
		return nil
	})
}

// CreationCancelFunc returns the stored context.CancelFunc for this
// instance's in-progress Background Resolution Context, or nil if none has
// been stored in this process -- e.g. the instance was loaded from storage
// without a live pipeline goroutine ever spawned here (Task 2.2.1a's
// documented nil case; Epic 3.2's Cancel RPC must guard against it).
func (i *Instance) CreationCancelFunc() context.CancelFunc {
	var cancel context.CancelFunc
	_ = i.sendSyncErr(func(s *instanceState) error {
		cancel = s.inst.creationCancelFunc
		return nil
	})
	return cancel
}

// ---- GitHubResolution -------------------------------------------------------------

// GitHubResolution carries the fields a deferred GitHub-URL clone resolves to
// (Epic 2.1, async-session-creation): SessionService.CreateSession publishes
// an instance immediately, before the clone runs, using the raw request
// path/branch as placeholders; once the background clone completes, it calls
// SetGitHubResolution with the real values. Grouped into one struct rather
// than a same-typed-string parameter pile (see
// .claude/rules/primitive-obsession-checklist.md).
type GitHubResolution struct {
	Path           string
	Branch         string
	Owner          string
	Repo           string
	SourceRef      string
	ClonedRepoPath string
	PRNumber       int
	PRURL          string
}

func setGitHubResolutionLocked(s *instanceState, r GitHubResolution) {
	s.inst.mu.Lock()
	s.inst.Path = r.Path
	if r.Branch != "" {
		s.inst.Branch = r.Branch
	}
	s.inst.GitHubOwner = r.Owner
	s.inst.GitHubRepo = r.Repo
	s.inst.GitHubSourceRef = r.SourceRef
	s.inst.ClonedRepoPath = r.ClonedRepoPath
	if r.PRNumber > 0 {
		s.inst.GitHubPRNumber = r.PRNumber
		s.inst.GitHubPRURL = r.PRURL
	}
	s.inst.touchUpdatedAt()
	snap := buildSnapshot(s.inst)
	s.inst.mu.Unlock()
	s.inst.snapshot.Store(snap)
}

// SetGitHubResolution patches Path/Branch/GitHub* metadata onto an instance
// once a deferred GitHub URL resolution (Epic 2.1) completes in the
// background, ahead of Start(). Actor-routed like every other field setter in
// this file — callers must not write these fields directly.
func (i *Instance) SetGitHubResolution(r GitHubResolution) {
	_ = i.sendSyncErr(func(s *instanceState) error {
		setGitHubResolutionLocked(s, r)
		return nil
	})
}

// applyWorktreeDetectionLocked writes DetectAndPopulateWorktreeInfo's detected
// IsWorktree/MainRepoPath/GitHubOwner/GitHubRepo fields under i.mu, mirroring
// setGitHubResolutionLocked's write discipline for the same GitHub fields --
// this is what closes the write/write race backlog item 10fc3913 flagged
// between the two (DetectAndPopulateWorktreeInfo's doc comment in
// instance_worktree.go covers why its i.Path *read* deliberately stays raw;
// this only fixes the writes).
func applyWorktreeDetectionLocked(s *instanceState, info *WorktreeInfo) {
	s.inst.mu.Lock()
	s.inst.IsWorktree = info.IsWorktree
	if info.IsWorktree && info.MainRepoRoot != "" {
		s.inst.MainRepoPath = info.MainRepoRoot
	}
	if s.inst.GitHubOwner == "" && info.GitHubOwner != "" {
		s.inst.GitHubOwner = info.GitHubOwner
	}
	if s.inst.GitHubRepo == "" && info.GitHubRepo != "" {
		s.inst.GitHubRepo = info.GitHubRepo
	}
	snap := buildSnapshot(s.inst)
	s.inst.mu.Unlock()
	s.inst.snapshot.Store(snap)
}
