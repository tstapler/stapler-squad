package session

import (
	"context"
	"fmt"
)

// ReconcileSuspendedProcesses runs once at server startup and resolves every
// SuspendedProcessRecord left behind by a prior server incarnation that
// crashed or was killed between CommitImportExternalSession's suspend step
// and a subsequent ConfirmKillExternalSession/CancelPendingKill call.
//
// For each record it re-checks whether the committed Instance still exists:
//   - If the Instance still exists, the import was never resolved by the
//     user -- resume the original process (SIGCONT) so it isn't left frozen
//     forever, but leave the Instance in place so the user can still
//     confirm-kill or cancel via the UI once they notice.
//   - If the Instance is gone (e.g. deleted out-of-band), this is an orphan:
//     resume the original process the same way.
//
// In both cases the record is removed after a successful resume so it is
// not reconciled again on the next restart. A resume failure leaves the
// record in place for the next reconciliation pass to retry.
func ReconcileSuspendedProcesses(_ context.Context, suspended *SuspendedProcessStore) error {
	if suspended == nil {
		return nil
	}

	records, err := suspended.List()
	if err != nil {
		return fmt.Errorf("reconcile suspended processes: failed to list records: %w", err)
	}

	var firstErr error
	for _, record := range records {
		if err := ResumeOriginalProcess(record.PID); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("reconcile suspended processes: failed to resume pid %d (instance %q): %w", record.PID, record.InstanceID, err)
			}
			continue
		}
		if err := suspended.Remove(record.InstanceID); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("reconcile suspended processes: resumed pid %d but failed to remove its record: %w", record.PID, err)
			}
		}
	}

	return firstErr
}
