package domain

import "errors"

// ErrCreationAwaitTimeout is returned by AwaitCreationTerminal when timeout
// elapses before the watched instance reaches a terminal status (Active or
// Failed). Distinct from ErrCreationVanished: the instance is still present
// and (from this reader's point of view) genuinely still resolving.
var ErrCreationAwaitTimeout = errors.New("timed out waiting for session creation to reach a terminal status")

// ErrCreationVanished is returned by AwaitCreationTerminal when the watched
// instance, previously observed to exist, disappears mid-wait — the
// signature of a concurrent CancelSessionCreation deleting the row rather
// than writing a terminal status. Distinct from ErrCreationAwaitTimeout: the
// instance is gone, not merely slow.
var ErrCreationVanished = errors.New("session creation was cancelled while waiting for it to complete")

// CreationOutcome is the atomic snapshot AwaitCreationTerminal returns on its
// terminal-status exit. Status and FailureReason are captured together in one
// actor round-trip (session.Instance.StatusAndFailureReason) so the pair can
// never straddle an intervening TryStartRetry/TryForceStatusIfEpoch write —
// see the Domain Glossary entry in project_plans/async-session-creation for
// why a second, separate re-read of Status()/FailureReason() would be unsafe.
type CreationOutcome struct {
	Status        Status
	FailureReason string
	InstanceID    string
}
