package session

import (
	"context"

	"github.com/tstapler/stapler-squad/log"
)

// MarkCrashed kills the stale dead-pane tmux session and transitions the
// instance from Active to Crashed with exitReason recorded. Called by
// SessionHealthChecker when it detects an abnormally-exited dead pane
// (session/health.go) — remain-on-exit keeps the tmux session/pane around as
// a placeholder after the wrapped program exits, so TmuxAlive() alone never
// notices. Returns an error (and leaves the instance's Status/ExitReason
// untouched) if the instance is not Active when the actor executes this
// command. KillSession() itself still runs regardless of that check (see
// below) -- a truly-dead pane being killed is harmless even on the rare race
// where the status changed concurrently between health.go's detection and
// this call, and the in-actor guard below prevents that race from ever
// corrupting Status/ExitReason.
//
// KillSession() runs on the CALLER's goroutine, outside the actor mailbox --
// matching how the health checker called it directly before this status
// existed. KillSession() only touches i.pm() (no actor-owned state), so this
// is safe, and it keeps a slow/hung `tmux kill-session` from blocking every
// other operation on this instance's actor (writes, resizes, other RPCs) for
// its duration; only the transition + ExitReason write need the actor.
//
// Fires EventExited on success (mirroring ReviewQueuePoller.reconcileSessions'
// "reconcile-session-missing" fire) so already-wired listeners -- the
// sessionExitedPublisher that pushes the new status to WatchSessions clients,
// and BacklogLifecycleListener (session/backlog_lifecycle.go) that notifies on
// EventExited/EventStopped -- pick this up without new plumbing. This is how a
// backlog automation session that crashes surfaces the failure instead of
// stalling silently. sessionSummaryListener also fires on this (it excludes
// only the reconciler's spurious "reconcile-session-missing" reason, per
// ADR-002 "natural exit and explicit stop dispatch identically") -- a crash is
// a real, confirmed exit (not a spurious signal), so generating a summary for
// it is consistent with every other genuine exit reason, not a new cost path.
func (i *Instance) MarkCrashed(exitReason string) error {
	if err := i.KillSession(); err != nil {
		log.Warn("markCrashed: failed to kill stale dead-pane session", "session", i.Title, "err", err)
	}
	err := i.sendSyncErr(func(s *instanceState) error {
		if s.inst.Status != Active {
			return ErrInvalidTransition{From: s.inst.Status, To: Crashed}
		}
		s.inst.mu.Lock()
		s.inst.ExitReason = exitReason
		s.inst.mu.Unlock()
		return transitionToLocked(s, context.Background(), Crashed)
	})
	if err == nil {
		i.fireLifecycleEvent(EventExited, "dead-pane-crashed: "+exitReason)
	}
	return err
}

// MarkExitedNormally kills the stale dead-pane tmux session and transitions the
// instance from Active to Stopped. Called by SessionHealthChecker when it
// detects a dead pane whose wrapped program exited normally (code 0, no
// signal) -- not a crash, so it must not be marked Crashed (see MarkCrashed).
// KillSession() runs outside the actor -- see MarkCrashed's doc comment.
// Fires EventExited on success -- see MarkCrashed's doc comment.
func (i *Instance) MarkExitedNormally() error {
	if err := i.KillSession(); err != nil {
		log.Warn("markExitedNormally: failed to kill stale dead-pane session", "session", i.Title, "err", err)
	}
	err := i.sendSyncErr(func(s *instanceState) error {
		if s.inst.Status != Active {
			return ErrInvalidTransition{From: s.inst.Status, To: Stopped}
		}
		return transitionToLocked(s, context.Background(), Stopped)
	})
	if err == nil {
		i.fireLifecycleEvent(EventExited, "dead-pane-exited-normally")
	}
	return err
}

// ResumeFromCrash transitions a Crashed instance back to Active, relaunching
// the wrapped program. The tmux session was already killed by MarkCrashed, so
// Start(false) takes the cold-restore path and threads --resume automatically
// when a conversation UUID is known (see ClaudeCommandBuilder).
func (i *Instance) ResumeFromCrash(ctx context.Context) error {
	return i.sendSyncErr(func(s *instanceState) error {
		return transitionToLocked(s, ctx, Active)
	})
}

// resumeFromCrashLocked is the actor-safe twin of ResumeFromCrash. Called from
// transitionToLocked when transitioning Crashed→Active; dispatches the
// re-launch work to a goroutine so the actor is not blocked, mirroring
// resumeFromHibernationLocked.
func resumeFromCrashLocked(s *instanceState, _ context.Context) {
	i := s.inst
	i.started.Store(false)
	go func() {
		if err := i.Start(false); err != nil {
			log.Error("crash resume: failed to start session", "session", i.Title, "err", err.Error())
			i.send(func(s *instanceState) {
				// Hold i.mu across the write and buildSnapshot — see runActor's doc
				// comment in actor.go for why this must not be a bare unlocked write.
				s.inst.mu.Lock()
				s.inst.loadStatus(Crashed)
				snap := buildSnapshot(s.inst)
				s.inst.mu.Unlock()
				s.inst.snapshot.Store(snap)
			})
			return
		}
		if i.controllerManager.GetStatusManager() != nil {
			if err := i.StartController(); err != nil {
				log.Warn("crash resume: failed to start controller", "session", i.Title, "err", err)
			}
		}
		StartSessionDriver(i, i.GetEffectiveRootDir())
		i.SetExitReason("")
	}()
}
