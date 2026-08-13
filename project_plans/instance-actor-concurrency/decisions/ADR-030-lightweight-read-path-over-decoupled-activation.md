# ADR-030: Convert Ephemeral Lookups to a Lightweight Read Path, Not Decoupled Actor Activation

**Status**: Accepted
**Date**: 2026-06-30
**Deciders**: Tyler Stapler
**Relates to**: `project_plans/instance-actor-concurrency/implementation/adversarial-review.md` §3 (new blocker)

---

## Context

The second adversarial review pass found that wiring actor-spawn into every `*Instance`
construction path (as Task 1.1c/3.1c originally specified) leaks a permanent, un-stoppable
goroutine from `FromInstanceData`'s ~20 throwaway call sites — `GitHubService.findInstance`
(every `GetPRInfo` RPC), several MCP tool handlers, `health.go`'s periodic ticker, etc. — none of
which register or tear down the `*Instance` they construct; today these are merely wasteful
(bounded, GC-reclaimed), but would become an unbounded per-call goroutine leak once actor-spawn is
unconditional.

Two remediation directions were identified:
1. Decouple actor-spawn from construction: `finishInstanceConstruction` always guarantees a valid
   snapshot, but a separate explicit `Activate()`/`Register()` step (called only by the one
   canonical path that registers an `Instance` with `ReviewQueuePoller`) spawns the actor.
2. Convert the ~20 throwaway call sites to a lighter-weight read path that never invokes
   `FromInstanceData`'s full construction (including its `Start()` side effect for active
   sessions, confirmed at `instance_serialization.go:348,375`).

## Decision

Convert the ~20 throwaway call sites (option 2), not decoupled activation (option 1).

Option 1 is smaller in diff size, but it leaves a standing hazard: every future call site added to
this codebase that calls `FromInstanceData`/`LoadInstances()` must remember not to treat the
result as "live" (i.e., never call a mutating method on it, since its actor was never activated —
sending a command to a `nil`/dead mailbox would hang or panic depending on implementation). That's
the same class of implicit discipline (“remember to do X correctly at every call site”) that
caused the original `stateMutex` false-confidence problem this whole migration exists to fix — see
`requirements.md`'s background. Option 1 would reintroduce a smaller instance of the exact failure
mode this migration is designed to eliminate.

Option 2 removes the ambiguity by construction: a throwaway lookup returns a read-only projection
type that has no mutating methods at all, so there is no way to accidentally call `Pause()`/
`Rename()`/etc. on a result that was never meant to be live. The existing inline comments already
scattered across this codebase (`session_service.go:881,980`, `review_queue_poller.go:876`,
`notification_service.go:85`, `storage.go:357`) already warn callers that
`LoadInstances()`→`FromInstanceData()`→`Start()` is expensive and several call sites already go
out of their way to prefer `ReviewQueuePoller.GetInstances()` instead — i.e., this codebase already
half-recognizes these two use cases (canonical live registry vs. one-off lookup) as distinct; this
decision makes that distinction an explicit type-level one instead of a scattered set of comments.

## Consequences

### Positive
- No implicit "don't mutate this one" discipline for future contributors to get wrong — a
  read-only projection type simply has no method that could send a command to a mailbox.
- Directly fixes the pre-existing performance complaint those inline comments already document
  (throwaway lookups no longer call `Start()` on every invocation either, since the lightweight
  path skips `FromInstanceData`'s side effects entirely, not just the actor-spawn).
- Consistent with this migration's core thesis (encapsulation over discipline) rather than an
  exception to it.

### Negative / Accepted tradeoffs
- Larger diff than option 1: ~20 call sites across `server/services/`, `server/mcp/`, `session/`,
  and two binaries (`daemon/daemon.go`, `main.go`) each need their throwaway `LoadInstances()`
  call replaced with the new lightweight read path, plus the read path itself needs designing
  (what does it return — raw `InstanceData`? a narrower read-only struct? — scoped per call site's
  actual field/method usage).
- Requires cataloging each of the ~20 call sites' actual usage (do they need `MatchesID`, a single
  field, a small set of fields?) before the lightweight type's shape can be finalized — this is
  itself new planning work, tracked as new tasks in `implementation/plan.md`.
