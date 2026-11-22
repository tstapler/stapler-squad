package services

import (
	"context"
	"time"

	"github.com/tstapler/stapler-squad/session/domain"
)

// creationAwaitPollInterval is the polling interval AwaitCreationTerminal uses
// in production. Tests inject a much shorter interval via
// awaitCreationTerminal (the unexported, interval-parameterized
// implementation) so they stay fast and deterministic without waiting out a
// real 250ms cadence — see Story 2.3.3c.
const creationAwaitPollInterval = 250 * time.Millisecond

// isTerminalCreationStatus reports whether status is a terminal outcome of
// the Background Resolution Pipeline: the instance either made it to
// Active/Running, or the pipeline failed before it got there. Every other
// status (Creating, Paused, Stopped, Hibernated, Restoring, Crashed) is not a
// creation-pipeline terminal state as far as this waiter is concerned.
func isTerminalCreationStatus(status domain.Status) bool {
	return status == domain.Active || status == domain.Failed
}

// AwaitCreationTerminal blocks (bounded by timeout) until instanceID's
// session-creation pipeline reaches a terminal status, the instance vanishes
// mid-wait, timeout elapses, or the caller's own ctx ends first. It is a pure
// reader: it never calls TryForceStatusIfEpoch, TryStartRetry, or bumps
// creationEpoch, so it introduces no new writer into the fencing model
// ADR-002 already guarantees exactly one terminal writer for.
//
// Returns:
//   - (domain.CreationOutcome{Status, FailureReason, InstanceID}, nil) as soon as the
//     polled instance's status is terminal (Active/Running or Failed). Status
//     and FailureReason come from the same poll iteration's atomic read —
//     never re-read afterward.
//   - (domain.CreationOutcome{}, domain.ErrCreationAwaitTimeout) if timeout elapses first.
//   - (domain.CreationOutcome{}, domain.ErrCreationVanished) if the instance, previously
//     found, is no longer found by FindLiveInstance (a concurrent
//     CancelSessionCreation removed it).
//   - (domain.CreationOutcome{}, ctx.Err()) if the caller's own ctx is done first —
//     returned as-is, not wrapped in a sentinel.
func (s *SessionService) AwaitCreationTerminal(ctx context.Context, instanceID string, timeout time.Duration) (domain.CreationOutcome, error) {
	return s.awaitCreationTerminal(ctx, instanceID, timeout, creationAwaitPollInterval)
}

// awaitCreationTerminal is AwaitCreationTerminal's interval-parameterized
// implementation. Production code always goes through AwaitCreationTerminal,
// which fixes pollInterval at creationAwaitPollInterval; tests call this
// directly with a much shorter interval to observe transitions quickly
// without waiting out the real 250ms cadence.
func (s *SessionService) awaitCreationTerminal(
	ctx context.Context,
	instanceID string,
	timeout time.Duration,
	pollInterval time.Duration,
) (domain.CreationOutcome, error) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	// seenAlive tracks whether FindLiveInstance has ever found instanceID, so a
	// caller-supplied ID that never existed at all (a caller bug) times out
	// rather than being misreported as domain.ErrCreationVanished — that sentinel is
	// reserved for "existed, then was removed."
	var seenAlive bool

	checkOnce := func() (domain.CreationOutcome, error, bool) {
		inst := s.FindLiveInstance(instanceID)
		if inst == nil {
			if seenAlive {
				return domain.CreationOutcome{}, domain.ErrCreationVanished, true
			}
			return domain.CreationOutcome{}, nil, false
		}
		seenAlive = true
		status, failureReason := inst.StatusAndFailureReason()
		if !isTerminalCreationStatus(status) {
			return domain.CreationOutcome{}, nil, false
		}
		return domain.CreationOutcome{
			Status:        status,
			FailureReason: failureReason,
			InstanceID:    instanceID,
		}, nil, true
	}

	// Check immediately before entering the select loop so a caller with an
	// already-terminal instance (or an already-vanished one) doesn't pay for a
	// full pollInterval tick before finding out.
	if outcome, err, done := checkOnce(); done {
		return outcome, err
	}

	for {
		select {
		case <-ctx.Done():
			return domain.CreationOutcome{}, ctx.Err()
		case <-deadline.C:
			return domain.CreationOutcome{}, domain.ErrCreationAwaitTimeout
		case <-ticker.C:
			if outcome, err, done := checkOnce(); done {
				return outcome, err
			}
		}
	}
}
