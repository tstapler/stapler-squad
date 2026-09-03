package services

import (
	"context"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

// terminalStatusStore is the narrow storage capability commitTerminalStatus
// needs. Defined here in the consumer package, scoped to the one method used,
// rather than added to the broad session.InstanceStore interface — extending
// that interface would require updating every existing InstanceStore test
// double in this package (fakeInstanceStore, fakePRInstanceStore, ...) for a
// capability none of them exercise. *session.Storage satisfies this
// structurally with no extra wiring.
type terminalStatusStore interface {
	UpdateInstanceIfEpoch(ctx context.Context, id string, capturedEpoch uint64, status session.Status, failureReason string) (bool, error)
}

// commitTerminalStatus is the one terminal-write entry point every background
// pipeline/sweeper call site in this project must use (never
// instance.TryForceStatusIfEpoch directly) — see ADR-002's addendum and
// pre-mortem.md failure #2 (P1).
//
// It persists durably first, and only syncs in-memory state if that succeeds:
// a process killed between the two steps can never leave a genuinely-successful,
// live worktree/tmux session behind a persisted row that still reads Creating,
// which the Stale-Creation Sweeper and Retry's cleanupPartialCreation would
// otherwise treat as orphaned and destroy. This reverses the naive
// "in-memory first, persist second" order: with that ordering, a crash after
// the in-memory win but before the persist leaves the on-disk row at Creating
// forever, indistinguishable from a still-running pipeline.
//
// If UpdateInstanceIfEpoch reports applied == false — no row matched, or the
// persisted creation_epoch had already moved past capturedEpoch — this returns
// false immediately without ever touching in-memory state: the database is the
// source of truth for a terminal write, and it already reflects a later
// writer's outcome, so there is nothing to reconcile.
func commitTerminalStatus(ctx context.Context, storage terminalStatusStore, instance *session.Instance, capturedEpoch uint64, status session.Status, failureReason string) bool {
	applied, err := storage.UpdateInstanceIfEpoch(ctx, instance.GetStableID(), capturedEpoch, status, failureReason)
	if err != nil {
		log.Error("commitTerminalStatus: durable persist failed", "session", instance.GetStableID(), "status", status, "err", err)
		return false
	}
	if !applied {
		return false
	}
	return instance.TryForceStatusIfEpoch(capturedEpoch, status, failureReason)
}
