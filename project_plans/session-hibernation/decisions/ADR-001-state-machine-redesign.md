# ADR-001: Session State Machine Redesign — 5-State Model

**Date:** 2026-05-18  
**Status:** Accepted  
**Deciders:** Tyler Stapler  

---

## Context

The current session lifecycle has 7+ states: `Loading`, `Running`, `Ready`, `NeedsApproval`, `Paused`, `Stopped`, and an unused `Creating`. Several problems have accumulated:

### Problem 1: Running and Ready are the same lifecycle state

`Running` and `Ready` differ only in Claude's internal activity level (actively generating tokens vs. idle and waiting). They have identical lifecycle properties — both have a live process, a live worktree, and are interactable. The distinction leaks into call sites that have nothing to do with activity level:

- `Running || Ready` guards appear throughout `session/`, `server/services/`, and frontend switch statements
- These guards have caused bugs when one branch of the OR is missed
- The activity-level distinction (is Claude thinking right now?) is already tracked by `DetectedStatus` via the detection layer — it does not need a second representation in the lifecycle enum

### Problem 2: NeedsApproval is a lifecycle state when it is a sub-state

`NeedsApproval` is currently a top-level lifecycle state. However, a session waiting for tool-use approval still has a live process, a live worktree, and can be interacted with — it is operationally identical to `Running`/`Ready`. Making it a lifecycle state forces every status switch to handle it explicitly and causes incorrect UI grouping (it appears alongside `Paused`/`Stopped` instead of within the "active" group).

### Problem 3: Loading and Creating are redundant

`Loading` was introduced as a transient state during session startup but was never fully wired: the Go backend never assigns `Creating`, and `Loading` represents the same concept — "init in progress." Having two names for one concept generates confusion in the adapter layer and makes async creation harder to reason about.

### Problem 4: No Hibernated state

The planned hibernation feature (Epic 4) requires a new `Hibernated` lifecycle state. Rather than bolting it onto the existing messy 7-state model, this is the right moment to rationalize the whole state machine.

---

## Decision

Adopt a **5-state lifecycle model** and move activity-level detail into a separately-tracked sub-status layer.

### New Lifecycle States

| State | Meaning | Process alive? | Worktree? |
|---|---|---|---|
| `Creating` | Async init in progress (clone, setup, first start) | No | Maybe |
| `Active` | Session running, AI process alive | Yes | Yes |
| `Paused` | Worktree removed, branch preserved; process dead | No | No |
| `Stopped` | Explicitly stopped; cold-restore possible | No | Yes |
| `Hibernated` | Process killed to save memory; scrollback checkpointed | No | Yes (dirty OK) |

**Removed states:**

| Old State | Disposition |
|---|---|
| `Running` | Merged into `Active` |
| `Ready` | Merged into `Active` |
| `Loading` | Merged into `Creating` |
| `NeedsApproval` | Demoted to sub-status of `Active` |

### Transition Table

```
Creating      → Active (init succeeded)
Creating      → Stopped (init failed; error stored on session)
Active        → Paused (explicit pause — removes worktree)
Active        → Stopped (explicit stop)
Active        → Hibernated (idle timeout, resource pressure, or manual)
Paused        → Active (resume — recreates worktree)
Paused        → Stopped (explicit delete without resume)
Stopped       → Active (cold restore)
Hibernated    → Active (resume — re-launches process)
Hibernated    → Stopped (explicit discard)
```

There are no transitions between `Paused` and `Hibernated` — they are distinct concepts (worktree removal vs. process kill) and should not be silently converted.

### Sub-Status Model

Sub-status is only meaningful when lifecycle state is `Active`. It is **derived at read time** from the detection layer and is **never persisted to the database**.

| Sub-status | Source | Display |
|---|---|---|
| `Processing` | `detection.StatusProcessing` | Spinner / "Thinking" |
| `Idle` | `detection.StatusReady` | Dim dot (default; may be suppressed) |
| `NeedsApproval` | `detection.StatusNeedsApproval` | Bell icon / "Needs Approval" |
| `Error` | `detection.StatusError` | Red icon / "Error" |
| `TestsFailing` | `detection.StatusTestsFailing` | Orange icon |
| `RateLimited` | rate-limit manager | Clock icon |

Sub-status is exposed on the `Session` proto as a non-persisted `string sub_status` field populated by the service layer on every `ListSessions` / `WatchSessions` response.

---

## Rationale

### Why merge Running and Ready?

The activity-level distinction is correctly tracked by `DetectedStatus` already. Duplicating it in the lifecycle enum couples the state machine to Claude's output-parsing internals and forces every status-aware code path to handle both. A single `Active` state with a derived sub-status achieves the same UI goal while halving the combinatorial surface.

### Why demote NeedsApproval to sub-status?

A session waiting for tool-use approval is operationally live: process running, worktree intact, commands can be issued. Treating it as a separate lifecycle state means it must appear in every lifecycle switch even though it has zero distinct lifecycle behavior. Moving it to sub-status means only the UI layer (status chip, grouping, filtering) needs to know about it.

### Why keep Creating separate from Active?

Async creation (Epic 2) requires a stable, user-visible state during long-running init. `Creating` is the right semantic — the session record exists but the session is not yet operational. Conflating it with `Active` would require callers to probe "is the process ready" rather than reading a clean state.

### Why not rename Stopped to something else?

`Stopped` already has the correct integer value (`6` in Go iota, `7` in proto) and is serialized in existing databases. Renaming it would be a pure churn change with migration cost and no semantic benefit.

---

## Consequences

### Migration path for serialized sessions

Existing persisted sessions use integer status values. The migration policy is:

- `Loading` (integer N) → rewrite to `Creating` on next read; no schema migration needed if the adapter performs the mapping
- `Running` (integer N) → rewrite to `Active`
- `Ready` (integer N) → rewrite to `Active`
- `NeedsApproval` (integer N) → rewrite to `Active` (sub-status is re-derived from detection layer)
- `Paused`, `Stopped` → no change; integer values must not shift

The Go `iota` for `SessionStatus` must be audited so the integer values for `Paused` and `Stopped` remain stable. Only add new values at the end (or fill gaps left by removed states with a `_deprecated` alias that the adapter ignores).

### Proto backward compatibility

The `SessionStatus` proto enum in `proto/session/v1/types.proto` must:

1. **Not renumber** existing values for `Paused` and `Stopped`
2. Add `SESSION_STATUS_ACTIVE = N` (using the slot previously occupied by `Running` or as a new value)
3. Add `SESSION_STATUS_CREATING = N` (using the slot previously occupied by `Loading`)
4. Add `SESSION_STATUS_HIBERNATED = N` (new value at the end)
5. Mark removed values (`RUNNING`, `READY`, `LOADING`, `NEEDS_APPROVAL`) as `reserved` with a comment — do not delete them, to prevent integer reuse

Old clients sending `SESSION_STATUS_RUNNING` will be treated as `SESSION_STATUS_ACTIVE` by the adapter.

### Code changes required

- `session/instance.go` — `SessionType` iota: remove `Running`, `Ready`, `Loading`, `NeedsApproval`; add `Active`, `Creating`, `Hibernated`
- `session/state_machine.go` — `allowedTransitions`: rewrite with new 5-state graph
- `session/status_mapping.go` — map detection results that currently set `Running`/`Ready` to `Active`
- `session/instance_serialization.go` — `FromInstanceData()`: map old integer values to new constants; add `Hibernated` to the auto-restart exclusion guard
- `server/adapters/instance_adapter.go` — map new Go constants ↔ proto enum
- `server/services/session_service.go` — all `case Running:`, `case Ready:`, `case NeedsApproval:` in switches
- `session/health.go` — add early bailout for `Hibernated`; remove `NeedsApproval` case from recovery path
- Frontend (`web-app/src/`) — status switch statements, color maps, badge labels, CSS `data-status` values, filter enums

### Risk: NeedsApproval detection gap

The NeedsApproval sub-status is derived from the detection layer at read time. If a session is `Active` but the detection goroutine has not yet fired, the sub-status will momentarily be `Idle`. This is acceptable — the detection layer updates frequently and the previous `NeedsApproval` lifecycle state had the same eventual-consistency property.
