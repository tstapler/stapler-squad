package session

import (
	"fmt"

	"github.com/tstapler/stapler-squad/log"
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
	inst.RecoverFromStopped()
	// Archival can land between the check above and the status flip; never
	// revive a session archived meanwhile (ADR-001).
	if inst.IsArchived() {
		inst.ForceStatus(prev)
		return HotRestoreArchived, nil
	}
	// Mirror restartForRetry: drop a stale controller, start a fresh one after attach.
	inst.StopController()
	inst.hotAttaching.Store(true)
	err := inst.Start(false)
	inst.hotAttaching.Store(false)
	if err != nil {
		// Leave the prior terminal status so a failed attach stays retryable
		// instead of stranding the session in Creating for the stale sweeper.
		if inst.GetLifecycleStatus() == Creating {
			inst.ForceStatus(prev)
		}
		return HotRestoreError, fmt.Errorf("hot attach: %w", err)
	}
	if ctrlErr := inst.StartController(); ctrlErr != nil {
		log.Warn("HotRestoreIfLive: failed to start controller after attach", "session", inst.Title, "err", ctrlErr)
	}
	return HotRestoreCleared, nil
}
