package session

import "fmt"

// CancelPendingKillParams holds everything needed to abandon an in-progress
// import: delete the committed Instance, then resume the original process.
type CancelPendingKillParams struct {
	Storage   InstanceStore
	Suspended *SuspendedProcessStore

	InstanceID  string
	OriginalPID int32
}

// CancelPendingKill deletes instanceID (the Instance committed by
// CommitImportExternalSession) and, only if that delete succeeds, SIGCONTs
// the original process and removes its SuspendedProcessRecord. The ordering
// is deliberate and matches import.proto's doc comment on
// CancelPendingKillResponse.resumed: if the compensating delete fails, the
// original process is left SIGSTOP'd -- ResumeOriginalProcess is never
// called in that case, since resuming a process whose replacement Instance
// still exists would leave two writers on the same transcript.
func CancelPendingKill(params CancelPendingKillParams) (resumed bool, err error) {
	if params.Storage == nil {
		return false, fmt.Errorf("cancel pending kill: Storage is required")
	}

	if err := params.Storage.DeleteInstance(params.InstanceID); err != nil {
		return false, fmt.Errorf("cancel pending kill: failed to delete instance %q, original process left suspended: %w", params.InstanceID, err)
	}

	if params.OriginalPID > 0 {
		if err := ResumeOriginalProcess(params.OriginalPID); err != nil {
			return false, fmt.Errorf("cancel pending kill: instance deleted but failed to resume original process: %w", err)
		}
	}

	if params.Suspended != nil {
		_ = params.Suspended.Remove(params.InstanceID)
	}

	return true, nil
}
