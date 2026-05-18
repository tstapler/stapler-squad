package session

import (
	"context"
	"path/filepath"

	appconfig "github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/hibernation"
)

// SetHibernateReason sets the reason string that will be recorded in the checkpoint.
// Must be called before Hibernate(). Values: "manual", "idle", "resource_pressure".
func (i *Instance) SetHibernateReason(reason string) {
	i.hibernateReason = reason
}

// Hibernate transitions an Active session to Hibernated.
// It sets the reason, transitions state, and returns.
// The actual checkpoint write and process kill happen asynchronously via the
// Active → Hibernated After hook (see state_machine.go).
func (i *Instance) Hibernate(ctx context.Context) error {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	return i.transitionTo(ctx, Hibernated)
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
// The actual process re-launch happens asynchronously via the
// Hibernated → Active After hook (see state_machine.go).
func (i *Instance) ResumeFromHibernation(ctx context.Context) error {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	return i.transitionTo(ctx, Active)
}

// resumeFromHibernation re-launches the AI process and cleans up the checkpoint.
// Called from the Hibernated → Active After hook in a goroutine.
// Must NOT hold stateMutex.
func (i *Instance) resumeFromHibernation(ctx context.Context) {
	// Re-launch via the cold-restore path
	i.started = false
	if err := i.Start(false); err != nil {
		log.Error("hibernation resume: failed to start session",
			"session", i.Title, "err", err.Error())
		// Roll back to Hibernated on failure
		i.stateMutex.Lock()
		i.loadStatus(Hibernated)
		i.stateMutex.Unlock()
		return
	}

	// Clean up checkpoint files
	cfg := appconfig.LoadConfig()
	checkpointDir, err := cfg.HibernationCheckpointDirOrDefault()
	if err == nil && checkpointDir != "" {
		writer := hibernation.NewWriter(checkpointDir)
		if err := writer.Delete(i.UUID); err != nil {
			log.Warn("hibernation resume: failed to delete checkpoint",
				"session", i.Title, "err", err)
		}
	}
}
