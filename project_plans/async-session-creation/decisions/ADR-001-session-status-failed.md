# ADR-001: Add `SESSION_STATUS_FAILED` as a New, Non-Terminal Enum Value

**Status**: Accepted
**Date**: 2026-08-26

## Context

`CreateSession`'s async resolution/provisioning path can fail after the
`ManagedInstance` already exists (status `Creating`). Today's only two
precedents for surfacing an async failure both overload existing values:

- `session_service.go:2409` (async worktree/tmux startup failure) calls
  `instance.ForceStatus(session.Stopped)`.
- `session/health.go:62` (async crash detection) does the same.

`STOPPED`'s doc comment states it is a terminal state that "cannot
transition further." `CRASHED` (`proto/session/v1/types.proto:392`) is
semantically a *post-Running* failure ("tmux pane exited abnormally...
requires an explicit resume") — a session that got far enough to run and
then died, which carries salvageable output/worktree state. A creation
that never finished resolving has no such salvageable state and a
different valid recovery action (retry-creation, not resume).

Reusing either value would conflate two operationally distinct failure
modes and, for `STOPPED` specifically, contradict the "no further
transition" invariant retry needs to violate legally (`Failed → Creating`
must be a legal transition).

## Decision

Add `SESSION_STATUS_FAILED = 11` to `proto/session/v1/types.proto`'s
`SessionStatus` enum (a fresh wire value, no `allow_alias` needed since it
isn't aliasing an existing value). Document it explicitly as **non-terminal**
— the first status in this enum whose doc comment says so — since
`Failed → Creating` (retry) and `Failed → <removed>` (cancel-delete) are
both legal exits.

Extend `session/instance_state.go`'s `transitionIndex` with:
- `Creating → Failed` (background pipeline failure, stale-detector timeout)
- `Failed → Creating` (retry)

`session/instance_actor_setters.go` gains a `FailureReason` field with a
public read-only `FailureReason()` accessor (alongside the existing
`CreationProgress`) so a card can render a persistent, distinguishable
error message ("stale/orphaned" vs. "GitHub resolution error" vs. "startup
error") independent of whatever the last `creation_progress` string
happened to say. There is deliberately no public `SetFailureReason` —
per ADR-002, `FailureReason` is terminal-write metadata and is written only
from inside `TryForceStatusIfEpoch`'s own atomic command, via an unexported
`setFailureReasonLocked` helper, so it can never go stale relative to
`Status` the way an independently-callable setter would allow.

Both existing overload sites (`session_service.go:2409`,
`session/health.go:62`) are migrated to `ForceStatus(session.Failed)` (or
the new epoch-gated helper, see ADR-002) as part of this project — not left
in place — closing the exact gap `fix-flaky-tests-dont-defer.md`-style
technical debt would otherwise leave for the next person to rediscover.

## Consequences

- Every exhaustive switch over `Status`/`SessionStatus` must gain a
  `Failed`/`FAILED` case: `adapters.StatusToProto`, `adapters.InstanceToProto`,
  `WatchSessions`'s `StatusFilter` mapping, `SessionCard.tsx`'s
  `getStatusColor`/`getStatusText`. Missing one silently renders "Unknown"
  or mismatches every filter — covered by Phase 1 tasks with an explicit
  compile-time/lint check (Go: a default case that returns an error rather
  than silently falling through; TS: `exhaustive-switch`-shaped review).
- `STOPPED`'s doc comment ("terminal, cannot transition further") stays
  literally true — it never gains new outgoing transitions, so no
  cross-reference update is needed there.
- Wire compatibility: adding a new enum value is backward compatible;
  no proto migration needed.

## Alternatives Rejected

- **Reuse `CRASHED`**: rejected — conflates "agent process died after
  running" with "never got past resolution," which have different valid
  recovery actions (resume vs. retry) and different card affordances.
- **Reuse `STOPPED`**: rejected — its own doc comment states it's terminal;
  retry requires a legal exit transition, which would require rewriting
  that invariant for a value used by many other call sites, a much larger
  blast radius than adding one new value.
