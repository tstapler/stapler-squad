package session

import (
	"fmt"
	"sync"
)

// hotRestoreLocks serializes HotRestoreIfLive per instance so concurrent RPCs
// and boot reconcile cannot both pass the recoverable check and double-attach.
var hotRestoreLocks sync.Map // *Instance -> *sync.Mutex

// HotRestoreOutcome is the per-session result of HotRestoreIfLive.
type HotRestoreOutcome string

const (
	HotRestoreCleared     HotRestoreOutcome = "cleared"
	HotRestoreNotLive     HotRestoreOutcome = "not_live"
	HotRestoreArchived    HotRestoreOutcome = "archived"
	HotRestoreNotTerminal HotRestoreOutcome = "not_terminal"
	HotRestoreError       HotRestoreOutcome = "error"
)

// HotRestoreIfLive clears a Stopped/PermanentlyFailed/Failed status on inst by
// hot-attaching to its still-alive backing process (tymux pane or tmux
// session), without restarting that process. It is the single implementation
// shared by boot reconcile (server/dependencies.go Step 6b) and the
// ClearTerminalStatusIfLive RPC. isLive is the liveness probe seam; it is only
// called once archival and status checks pass. A non-live or archived session
// is left untouched.
func HotRestoreIfLive(inst *Instance, isLive func() bool) (HotRestoreOutcome, error) {
	if inst == nil {
		return HotRestoreError, nil
	}
	// Archived sessions are deliberately retired: adopting one would flip it
	// back off its terminal status (ADR-001); checked under the lock below.
	mu, _ := hotRestoreLocks.LoadOrStore(inst, &sync.Mutex{})
	lock := mu.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()
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
	if err := inst.Start(false); err != nil {
		// Leave the prior terminal status so a failed attach stays retryable
		// instead of stranding the session in Creating for the stale sweeper.
		if inst.GetLifecycleStatus() == Creating {
			inst.ForceStatus(prev)
		}
		return HotRestoreError, fmt.Errorf("hot attach: %w", err)
	}
	return HotRestoreCleared, nil
}
