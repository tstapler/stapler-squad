package session

import (
	"fmt"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
)

// HotRestoreOutcome is the per-session result of HotRestoreIfLive.
type HotRestoreOutcome string

const (
	HotRestoreCleared     HotRestoreOutcome = "cleared"
	HotRestoreNotLive     HotRestoreOutcome = "not_live"
	HotRestoreArchived    HotRestoreOutcome = "archived"
	HotRestoreNotTerminal HotRestoreOutcome = "not_terminal"
	HotRestoreBusy        HotRestoreOutcome = "busy"
	HotRestoreError       HotRestoreOutcome = "error"
)

// HotRestoreIfLive clears a Stopped/PermanentlyFailed/Failed status on inst by
// hot-attaching to its still-alive backing process (tymux pane or tmux
// session), without restarting that process. It is the single implementation
// shared by boot reconcile (server/dependencies.go Step 6b) and the
// ClearTerminalStatusIfLive RPC. isLive is the liveness probe seam; it is only
// called once archival and status checks pass. A non-live, archived or busy
// (restart in flight) session is left untouched. A failed attach releases, and
// never kills, the pre-existing process (see cleanupAfterFailedStart).
func HotRestoreIfLive(inst *Instance, isLive func() bool) (HotRestoreOutcome, error) {
	if inst == nil {
		return HotRestoreError, nil
	}
	inst.hotRestoreMu.Lock()
	defer inst.hotRestoreMu.Unlock()
	// Shares the restart choke point's guard so a retry restart cannot interleave.
	if !inst.retryInFlight.CompareAndSwap(false, true) {
		return HotRestoreBusy, nil
	}
	defer inst.retryInFlight.Store(false)

	if inst.IsArchived() {
		return HotRestoreArchived, nil
	}
	if !inst.IsHotRestoreRecoverable() {
		return HotRestoreNotTerminal, nil
	}
	if !isLive() {
		return HotRestoreNotLive, nil
	}
	prev := inst.GetLifecycleStatus()
	wasStarted := inst.Started()
	inst.mu.RLock()
	progressAt := inst.creationProgressUpdatedAt
	inst.mu.RUnlock()
	rollback := func() {
		inst.mu.Lock()
		inst.creationProgressUpdatedAt = progressAt
		inst.mu.Unlock()
		inst.started.Store(wasStarted)
		inst.ForceStatus(prev) // rebuilds the snapshot after started/progress are restored
	}
	inst.RecoverFromStopped()
	// Archival can land between the check above and the status flip; never
	// revive a session archived meanwhile (ADR-001).
	if inst.IsArchived() {
		rollback()
		return HotRestoreArchived, nil
	}
	// Mirror restartForRetry: drop a stale controller, start a fresh one after attach.
	inst.StopController()
	if err := inst.startHotAttach(); err != nil {
		// Leave the prior terminal status so a failed attach stays retryable
		// instead of stranding the session in Creating for the stale sweeper.
		if inst.GetLifecycleStatus() == domain.Creating {
			rollback()
		}
		return HotRestoreError, fmt.Errorf("hot attach: %w", err)
	}
	// The session is healthy again: drop pending backoff state exactly as RetryNow
	// does, or the driver's retry tick would restart the live pane.
	inst.mu.Lock()
	inst.reset()
	inst.mu.Unlock()
	if ctrlErr := inst.StartController(); ctrlErr != nil {
		log.Warn("HotRestoreIfLive: failed to start controller after attach", "session", inst.Title, "err", ctrlErr)
	}
	return HotRestoreCleared, nil
}

// startHotAttach is Start(false) restricted to adopting the pre-existing live
// process. The flag is set inside the actor command (so no other Start can run
// in the window) and cleared by defer (so a panic cannot leave it stuck).
func (i *Instance) startHotAttach() error {
	return i.sendSyncErr(func(s *instanceState) error {
		i.hotAttaching.Store(true)
		defer i.hotAttaching.Store(false)
		return startLocked(s, false)
	})
}
