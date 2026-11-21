package domain

import "errors"

// ErrPreconditionFailed is returned when an optimistic-locking precondition check fails.
var ErrPreconditionFailed = errors.New("precondition failed: concurrent modification detected")

// ErrNotFound is returned when a requested entity does not exist.
var ErrNotFound = errors.New("not found")

// ErrLaunchNotWired is the ProcessManager contract for a restore that would have to create a
// session before WireLaunch ran. Creating one then would start the backend's default shell, not
// the agent, so RestoreWithWorkDir must return this instead; callers treat it as "not started yet".
var ErrLaunchNotWired = errors.New("launch command not wired")

// ErrInstanceDataNotFound is returned by FindInstanceDataByID when no match exists.
var ErrInstanceDataNotFound = errors.New("instance data not found")

// ErrCrossHostClaimDedupDisabled is returned by the backlog service's LookupCrossHostClaim while
// the cross_host_claim_dedup feature flag is off.
var ErrCrossHostClaimDedupDisabled = errors.New("cross_host_claim_dedup is disabled")

// ErrConflict is returned when an operation would violate a uniqueness constraint.
var ErrConflict = errors.New("conflict")
