# Session Status Unification — Requirements

## Project Overview

Unify the session status/detection propagation pipeline from PTY pattern matching through
to the Redux store. Currently four parallel tracks (lifecycle Status, DetectedStatus in-memory,
SubStatus on the wire, detectedStatusMap in Redux) diverge on every state transition, causing
stale "Thinking…" badges when sessions exit unexpectedly. The root fix addressed the missing
event publication; this project addresses the architectural cause: heterogeneous, string-typed
events writing to disjoint Redux state.

**Related prior work**: `detection-architecture-refactors` (internal Go SRP refactor of
StatusDetector — no proto/frontend scope). This project is orthogonal and can be implemented
independently, though both benefit from the same `TerminalDetector` interface.

---

## Problem Statement

### The Four-Track Split

| Track | Where | Updated by |
|---|---|---|
| `session.Status` (Go enum) | DB + in-memory | State machine transitions |
| `DetectedStatus` (Go enum) | In-memory only | PTY pattern matching |
| `SubStatus` (Proto enum) | Wire only | Derived at snapshot time by adapter |
| `detectedStatusMap` (Redux) | Frontend only | `StatusChangedEvent` field (string) |

No single event carries all four. `StatusChangedEvent` writes to `detectedStatusMap`.
`SessionUpdatedEvent` writes to the entity adapter (updates `session.subStatus`) but does
NOT clear `detectedStatusMap`. Result: any path that publishes only one event type leaves
one track stale.

### Secondary Problems

1. `DetectedStatus.StatusActive` (PTY pattern: running a tool) and `session.Active`
   (lifecycle: process is running) share the same English word, causing constant
   context-switching when reading adapter/event code.

2. `StatusReady` is the `.*` catch-all fallback pattern — it matches anything not caught
   by a more specific pattern. It renders "● Ready" despite potentially matching garbled
   crash output or mid-stream terminal noise. `StatusIdle` already represents "genuinely idle."

3. `StatusBadge` switches on `DetectedStatus.String()` raw strings. Renaming a Go enum
   constant silently breaks the badge with no compile or test failure.

4. `WorkingState` (ACTIVE/PROCESSING/IDLE/WAITING) is derived server-side in
   `MapDetectedStatusToWorkingState` and round-trips through proto solely for
   `ReviewQueuePanel`. It is a coarser view of `SubStatus` that the frontend can derive
   without server involvement.

---

## Requirements

### R1 — `DetectedStatus` becomes a proto enum

- R1.1: Add `DetectedStatus` enum to `proto/session/v1/types.proto` with values:
  `DETECTED_STATUS_UNSPECIFIED`, `DETECTED_STATUS_EXECUTING` (previously StatusActive),
  `DETECTED_STATUS_PROCESSING`, `DETECTED_STATUS_IDLE`, `DETECTED_STATUS_NEEDS_APPROVAL`,
  `DETECTED_STATUS_INPUT_REQUIRED`, `DETECTED_STATUS_RATE_LIMITED`,
  `DETECTED_STATUS_UNKNOWN` (the `.*` fallback, previously StatusReady)
- R1.2: Regenerate proto bindings (`make generate-proto`) to produce Go + TypeScript types
- R1.3: The Go `detection.DetectedStatus` enum is the internal type; a mapping function
  `detection.DetectedStatusToProto(s DetectedStatus) sessionv1.DetectedStatus` converts
  it to the wire type
- R1.4: Frontend code uses the generated `DetectedStatus` TypeScript enum exclusively —
  no raw string literals for detected status values anywhere in the frontend

### R2 — Rename `StatusActive` and fix the `StatusReady` catch-all

- R2.1: Rename `detection.StatusActive` → `detection.StatusExecuting` in all Go source files
- R2.2: Replace the `StatusReady` `.*` catch-all pattern with `StatusUnknown` (new constant)
  `StatusReady` now means: "readline-style prompt explicitly detected, session genuinely idle"
  (currently handled by `StatusIdle` — evaluate whether to merge or keep distinct)
- R2.3: Update `toProtoSubStatus` adapter mapping, all switch statements, and all tests
- R2.4: Exhaustive switch enforcement: add a `var _ = [...]struct{}{ … }` compile-time array
  or use the `exhaustive` linter configured in `.golangci.yml` to catch unhandled cases

### R3 — Single event path: SessionUpdatedEvent carries detection info

- R3.1: `SessionUpdatedEvent` is extended to carry `DetectedStatus` (proto enum) and
  `DetectedContext` (string). These fields are populated by the publishing call sites.
- R3.2: All places that publish `SessionUpdatedEvent` with `[]string{"status"}` that also
  change the detected status must include the detection fields.
- R3.3: `StatusChangedEvent` is deprecated: its role is subsumed by `SessionUpdatedEvent`
  with detection fields. Existing `StatusChangedEvent` emission (in `UpdateSession` RPC and
  the new `wireSessionExitedPublisher`) is replaced with `SessionUpdatedEvent` carrying
  detection info.
- R3.4: If immediate deprecation is not practical in one PR, `StatusChangedEvent` handlers
  must also publish a `SessionUpdatedEvent` so the full state is always consistent regardless
  of which event fires first.

### R4 — Single Redux entry point

- R4.1: All session state changes in the frontend flow through `upsertSession`. The
  `updateSessionStatus` action is either removed or becomes a private implementation detail
  that delegates to `upsertSession`.
- R4.2: `upsertSession` clears `detectedStatusMap[session.id]` when the incoming session's
  `status !== SessionStatus.ACTIVE`. This atomically keeps the entity + the map in sync.
- R4.3: When `status === SessionStatus.ACTIVE` and the session carries a non-UNSPECIFIED
  `detectedStatus`, `upsertSession` updates `detectedStatusMap[session.id]` from the typed
  proto field — no string parsing.
- R4.4: The `WatchSessions` stream handler dispatches only `upsertSession` (or `removeSession`
  for deletes). The special `statusChanged` stream case dispatches `upsertSession` with a
  partial session proto that has at minimum `id`, `status`, `subStatus`, and `detectedStatus`.

### R5 — Move `WorkingState` derivation to frontend

- R5.1: Remove `MapDetectedStatusToWorkingState` from `server/adapters/instance_adapter.go`
- R5.2: Remove `working_state` from `Session` proto or mark deprecated
- R5.3: Add a `deriveWorkingState(session: Session): WorkingState` pure function in
  `web-app/src/lib/utils/` that derives `WorkingState` from `session.subStatus` or
  `session.detectedStatus`
- R5.4: `ReviewQueuePanel` and any other frontend consumers use `deriveWorkingState`

### R6 — Frontend type safety enforcement

- R6.1: No raw `DetectedStatus` string literals in frontend source (`"Processing"`, `"Active"`,
  etc.) — verified by ESLint rule or TypeScript exhaustive-switch pattern
- R6.2: `StatusBadge` switches on the typed `DetectedStatus` proto enum, not strings
- R6.3: Add a TypeScript compile-time exhaustiveness check (a `never`-typed default branch in
  every `switch` over `DetectedStatus`) so adding a new proto enum value forces a frontend update

---

## Implementation Order

Items have the following dependency graph:

```
R1 (proto enum)
  ↓
R2 (rename + catch-all fix)  R3 (SessionUpdatedEvent carries detection)
         ↓                              ↓
        R4 (single Redux entry point — needs typed proto enum + unified events)
         ↓
        R5 (WorkingState move — needs typed enum on frontend)
         ↓
        R6 (type safety enforcement — cap on the whole project)
```

R1 and R3 server-side changes can be done in parallel with R2.

---

## Acceptance Criteria

- `make build` passes
- `make test` passes
- `make lint` passes (including exhaustive switch lint)
- Frontend: `cd web-app && npx jest --no-coverage` passes
- No raw detected-status string literals in frontend (ESLint or grep check)
- `StatusBadge` switch uses typed `DetectedStatus` enum, not strings
- `upsertSession` clears `detectedStatusMap` when `status !== ACTIVE`
- `WorkingState` is no longer computed server-side
- `StatusActive` name no longer exists in Go source
- `StatusReady` is no longer the `.*` catch-all pattern
- Existing behavior: sessions detect the same statuses as before (no regression)
- A stopped session that was previously "Thinking…" shows no chip or badge after stopping

---

## Non-Goals

- Changing detection patterns (what regexes match PTY output)
- Adding new binary detectors (covered by `detection-architecture-refactors` plan)
- Internal SRP refactor of `StatusDetector` (covered by `detection-architecture-refactors` plan)
- Changing the WatchSessions streaming protocol beyond adding fields to `SessionUpdatedEvent`
