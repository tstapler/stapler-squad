package session

import (
	"context"
	"path/filepath"

	appconfig "github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/internal/syncutil"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/hibernation"
)

// SetHibernateReason sets the reason string that will be recorded in the checkpoint.
// Must be called before Hibernate(). Values: "manual", "idle", "resource_pressure".
func (i *Instance) SetHibernateReason(reason string) {
	i.hibernateReason = reason
}

// Hibernate transitions an Active session to Hibernated.
// It transitions state and dispatches the heavy I/O to a goroutine.
func (i *Instance) Hibernate(ctx context.Context) error {
	return i.sendSyncErr(func(s *instanceState) error {
		return transitionToLocked(s, ctx, Hibernated)
	})
}

// hibernateProcessLocked is the actor-safe twin of hibernateProcess.
// Called from transitionToLocked when transitioning Active→Hibernated;
// dispatches the I/O work to a goroutine so the actor is not blocked.
func hibernateProcessLocked(s *instanceState, ctx context.Context) {
	s.inst.hibernateWG.Add(1)
	go func() {
		defer s.inst.hibernateWG.Done()
		s.inst.hibernateProcess(ctx)
	}()
}

// hibernateProcess performs the actual hibernation side-effects:
//  1. Copies scrollback to checkpoint dir
//  2. Kills the tmux session (SIGKILL via tmux kill-session)
//
// Called from the Active → Hibernated After hook in a goroutine.
// Must NOT hold stateMutex.
func (i *Instance) hibernateProcess(ctx context.Context) {
	cfg := appconfig.LoadConfig()

	checkpointDir, err := cfg.HibernationCheckpointDirOrDefault()
	if err != nil {
		log.Error("hibernation: failed to resolve checkpoint dir", "session", i.Title, "err", err)
		// Continue — we'll still kill the process even if checkpoint write fails
	}

	if checkpointDir != "" {
		scrollbackPath := i.scrollbackPath()
		reason := i.hibernateReason
		if reason == "" {
			reason = "manual"
		}
		c := hibernation.Checkpoint{
			SchemaVersion:    1,
			SessionID:        i.UUID,
			SessionTitle:     i.Title,
			WorkingDirectory: i.Path,
			Program:          i.Program,
			HibernateReason:  reason,
			ScrollbackFile:   "scrollback.txt",
		}
		// Use context.Background() since the caller's context may have been cancelled
		writer := hibernation.NewWriter(checkpointDir)
		if err := writer.Write(context.Background(), c, scrollbackPath); err != nil {
			log.Warn("hibernation: checkpoint write failed, hibernating anyway",
				"session", i.Title, "err", err)
		} else {
			log.Info("hibernation: checkpoint written", "session", i.Title,
				"dir", filepath.Join(checkpointDir, i.UUID))
		}
	}

	// Kill the tmux session
	if err := i.KillSession(); err != nil {
		log.Error("hibernation: failed to kill tmux session",
			"session", i.Title, "err", err)
	} else {
		log.Info("hibernation: tmux session killed", "session", i.Title)
	}
}

// scrollbackPath returns the path to the session's scrollback JSONL file.
// Mirrors the pattern used in instance_checkpoint.go.
func (i *Instance) scrollbackPath() string {
	configDir, err := appconfig.GetConfigDir()
	if err != nil {
		log.Warn("hibernation: cannot resolve config dir for scrollback path",
			"session", i.Title, "err", err)
		return ""
	}
	return filepath.Join(configDir, i.Title, "scrollback.jsonl")
}

// ResumeFromHibernation transitions a Hibernated session back to Active.
// The actual process re-launch happens asynchronously via resumeFromHibernationLocked.
func (i *Instance) ResumeFromHibernation(ctx context.Context) error {
	return i.sendSyncErr(func(s *instanceState) error {
		return transitionToLocked(s, ctx, Active)
	})
}

// resumeFromHibernationLocked is the actor-safe twin of resumeFromHibernation.
// Called from transitionToLocked when transitioning Hibernated→Active;
// dispatches the re-launch work to a goroutine so the actor is not blocked.
func resumeFromHibernationLocked(s *instanceState, _ context.Context) {
	i := s.inst
	i.started.Store(false)
	i.hibernateWG.Add(1)
	go func() {
		defer i.hibernateWG.Done()
		if err := i.Start(false); err != nil {
			log.Error("hibernation resume: failed to start session",
				"session", i.Title, "err", err.Error())
			i.send(rollbackFailedResume)
			return
		}
		if i.controllerManager.GetStatusManager() != nil {
			if err := i.StartController(); err != nil {
				log.Warn("hibernation resume: failed to start controller",
					"session", i.Title, "err", err)
			}
		}
		StartSessionDriver(i, i.GetEffectiveRootDir())
		cfg := appconfig.LoadConfig()
		checkpointDir, err := cfg.HibernationCheckpointDirOrDefault()
		if err == nil && checkpointDir != "" {
			writer := hibernation.NewWriter(checkpointDir)
			if err := writer.Delete(i.UUID); err != nil {
				log.Warn("hibernation resume: failed to delete checkpoint",
					"session", i.Title, "err", err)
			}
		}
	}()
}

// JoinHibernation waits for any in-flight hibernateProcessLocked or
// resumeFromHibernationLocked goroutine to exit, up to stopJoinTimeout. Tests
// should call this before relying on t.TempDir() cleanup, since a resume
// goroutine can otherwise outlive the temp dir it was launched against (see
// the "session working directory missing" hibernation-resume errors this
// guards against).
func JoinHibernation(i *Instance) {
	if !syncutil.WaitWithTimeout(&i.hibernateWG, stopJoinTimeout) {
		log.Warn("JoinHibernation: hibernate/resume goroutine did not exit within timeout; it may still be running",
			"session", i.Title, "timeout", stopJoinTimeout)
	}
}

// rollbackFailedResume reverts Status to Hibernated after a failed
// hibernation-resume Start(), but only if the instance is still Active --
// i.e. nothing else legitimately transitioned it away in the meantime (e.g.
// a concurrent StopByUser() call landing before this async rollback runs).
// Applying the rollback unconditionally would silently clobber that later,
// legitimate transition with a stale Hibernated status.
func rollbackFailedResume(s *instanceState) {
	i := s.inst
	i.mu.Lock()
	if i.Status != Active {
		i.mu.Unlock()
		return
	}
	i.loadStatus(Hibernated)
	snap := buildSnapshot(i)
	i.mu.Unlock()
	i.snapshot.Store(snap)
}
