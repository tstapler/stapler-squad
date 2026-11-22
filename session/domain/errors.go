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

// ErrAnswererForbidden is returned when the answerer is not the one a guidance request names;
// the request is left unchanged.
var ErrAnswererForbidden = errors.New("answerer is not permitted to answer this request")

// ErrNudgeSubmitUncertain wraps every submit error after a successful nudge claim: the text may
// have landed unsubmitted, so the slot stays consumed and callers must leave the pane alone.
var ErrNudgeSubmitUncertain = errors.New("nudge delivery uncertain")
