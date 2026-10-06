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
// For each record it re-checks whether the committed Instance still exists
// in storage:
//   - If the Instance still exists (i.e. the prior incarnation's commit
//     completed and nothing has deleted it since), the original process may
//     still be actively managed by that Instance's lifecycle -- resuming it
//     here would let two writers (the original process and the managed
//     Instance) touch the same on-disk session/tmux pane concurrently. The
//     record and the suspension are left in place for whatever path
//     legitimately owns that Instance (confirm-kill/cancel) to resolve.
//   - If the Instance is missing (deleted out-of-band, or the prior
//     incarnation crashed before CommitImportExternalSession finished), this
//     is an orphaned suspension with nothing left to manage it: resume the
//     original process (SIGCONT) so it isn't left frozen forever.
//
// storage may be nil (e.g. in tests exercising only the suspended-process
// side); a nil storage is treated the same as "Instance not found" for every
// record, matching this function's prior behavior before storage was added.
//
// A record is removed only after a successful resume, so it is not
// reconciled again on the next restart. A resume failure leaves the record
// in place for the next reconciliation pass to retry.
func ReconcileSuspendedProcesses(_ context.Context, suspended *SuspendedProcessStore, storage InstanceStore) error {
	if suspended == nil {
		return nil
	}

	records, err := suspended.List()
	if err != nil {
		return fmt.Errorf("reconcile suspended processes: failed to list records: %w", err)
	}

	managed := map[string]bool{}
	if storage != nil {
		existing, err := storage.ListInstanceData()
		if err != nil {
			return fmt.Errorf("reconcile suspended processes: failed to list instances: %w", err)
		}
		for _, inst := range existing {
			managed[inst.Title] = true
		}
	}

	var firstErr error
	for _, record := range records {
		if managed[record.InstanceID] {
			// Committed Instance is still present -- leave the original
			// process suspended; it is (or should be) that Instance's
			// concern to resume/kill it via the normal confirm/cancel flow.
			continue
		}
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
