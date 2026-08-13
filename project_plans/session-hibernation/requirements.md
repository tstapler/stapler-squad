# Session State Machine Redesign + Hibernation Requirements

## Problem Statement

Three interlocked problems motivate this work:

1. **Async creation bug**: `Creating` status exists in the proto and frontend but is never assigned by the Go backend. Creation is synchronous — repos with long initialization (clone, npm install) silently fail because there is no state to wait in. Sessions should be created asynchronously with a visible `Creating` state.

2. **State model complexity without payoff**: `Running` and `Ready` are separate lifecycle states that differ only in Claude's internal activity level. This causes `Running || Ready` guards throughout the codebase and leaks a UI distinction that should live in the detection layer. Meanwhile, `NeedsApproval` is a top-level lifecycle state when it is really a sub-state of "session is active."

3. **Status detail invisible to users**: The sessions list shows coarse lifecycle status (`Running`, `Paused`) but not `DetectedStatus` (`Processing`, `Idle`, `NeedsApproval`, `Error`, `RateLimited`). Users cannot see whether Claude is actively thinking or just idle.

4. **Memory waste from long-idle sessions**: AI agent sessions left running for hours hold memory/CPU with no user value.

## Goals

1. Redesign the state machine to be minimal, correct, and non-overlapping
2. Surface fine-grained sub-status visibly to users in the session list
3. Make session creation async so long-running inits don't block/fail
4. Hibernate idle sessions to free memory, with seamless resume

## Non-Goals

- Full process checkpointing (CRIU) — save scrollback + kill process only
- Suspending tmux itself — tmux stays alive
- Automatic context summarization on resume (v2)
- Requiring a clean worktree before hibernation

---

## Epic 1: State Machine Redesign

### Proposed Lifecycle States (6 total, down from 7+)

```
Creating   ──────────────────────────────► Active ──► Stopped
   │                                         │
   └──────────────────────────────────────── │──► Paused ──► Active
                                             │           └──► Stopped
                                             └──► Hibernated ──► Active
                                                              └──► Stopped
```

| State | Meaning | Process alive? | Worktree? |
|---|---|---|---|
| `Creating` | Async init in progress (clone, npm install, first start) | No (not yet) | Maybe |
| `Active` | Session running, AI process alive | Yes | Yes |
| `Paused` | Worktree removed, branch preserved; process dead | No | No |
| `Stopped` | Explicitly stopped; cold-restore possible | No | Yes |
| `Hibernated` | Process killed to save memory; scrollback checkpointed | No | Yes (dirty OK) |

**Removed:** `Running`, `Ready` (merged into `Active`), `Loading` (merged into `Creating`), `NeedsApproval` (becomes a sub-status of `Active`).

**Transition table:**
```
Creating      → Active, Stopped
Active        → Paused, Stopped, Hibernated
Paused        → Active, Stopped
Stopped       → Active (cold restore)
Hibernated    → Active (resume), Stopped
```

### Sub-Status Model

Sub-status is only meaningful when lifecycle state is `Active`. It is derived from `DetectedStatus` and displayed alongside the lifecycle badge in the sessions list.

| Sub-status | Source | Display |
|---|---|---|
| `Processing` | `detection.StatusProcessing` | Spinner / "Thinking" |
| `Idle` | `detection.StatusReady` | Dim dot / "Idle" |
| `NeedsApproval` | `detection.StatusNeedsApproval` | Bell icon / "Needs Approval" |
| `Error` | `detection.StatusError` | Red icon / "Error" |
| `TestsFailing` | `detection.StatusTestsFailing` | Orange icon |
| `RateLimited` | rate limit manager | Clock icon |

Sub-status is NOT stored in the database — it is always derived at read time from the detection layer.

### SM-1: Remove `Running` and `Ready` lifecycle states
- Merge both into `Active`
- Replace all `Running || Ready` guards with `Active`
- `status_mapping.go`: map detection results that currently set `Running`/`Ready` to `Active`
- State machine `allowedTransitions`: remove `Running` and `Ready` entries, add `Active`

### SM-2: Remove `Loading` lifecycle state
- `Loading` is a sub-phase of `Creating` — both mean "not yet operational"
- Merge: instances start at `Creating`, transition to `Active` once startup completes
- Update `FromInstanceData()` to use `Creating` for instances that are mid-start on deserialization, `Active` for fully started ones

### SM-3: `NeedsApproval` → sub-status of `Active`
- Remove `NeedsApproval` from lifecycle state enum
- The session is still `Active` when waiting for approval — only the displayed sub-status changes
- `approval_automation.go` / detection layer already drives this; no instance status change needed
- Update all switches that case on `NeedsApproval` to read sub-status instead

### SM-4: Proto + adapter + frontend updates
- Update `proto/session/v1/types.proto`: rename/consolidate enum values to match new model
- Update `server/adapters/instance_adapter.go`: map new Go constants ↔ proto enum
- Update frontend: all status switch statements, color maps, badge labels, CSS data-status values
- Maintain backward compatibility: the proto enum integer values for `Paused`, `Stopped` must not change (avoid forced re-migrations)

---

## Epic 2: Async Session Creation

### CREATE-1: `Creating` as a real async state
- When `CreateSession` RPC is called, immediately create the session record with status `Creating` and return to the client
- Spawn a background goroutine to: initialize worktree/repo, run any setup commands, start the AI process
- Once setup succeeds → transition to `Active`; if setup fails → transition to `Stopped` with error message stored

### CREATE-2: Progress reporting during `Creating`
- The `WatchSessions` stream already delivers `SessionEvent`s; use it to push status updates while in `Creating` state
- Optionally: add a `creation_progress` string field to `Session` proto (visible in UI as e.g. "Cloning repository…")

### CREATE-3: Frontend `Creating` UX
- Sessions in `Creating` state show a progress indicator (spinner + progress text)
- Users cannot hibernate, pause, or interact with a `Creating` session
- Existing `SessionWizard` progress phases should map to real backend events

---

## Epic 3: Sub-Status Visibility in Sessions List

### VIS-1: Sub-status field in `Session` proto
- Add `string sub_status = N` (or an enum) to the `Session` message
- Backend populates it at read-time from `GetEffectiveStatus()` / detection layer
- Never persisted to the database — derived on every `ListSessions` / `WatchSessions` response

### VIS-2: Sessions list UI
- `SessionRow` and `SessionCard` both show sub-status alongside the lifecycle badge
- Design: lifecycle badge (e.g., "Active") + sub-status chip (e.g., "Thinking…" with spinner)
- Sub-status chip is only shown when `Active` and sub-status ≠ `Idle`
- `NeedsApproval` sub-status gets a distinct action-required style (orange or bell icon)

### VIS-3: Filter/group by sub-status
- The existing grouping strategies (Category, Tag, Status, etc.) should be able to group by sub-status
- "Needs Approval" group is useful for quickly finding sessions that need attention

---

## Epic 4: Session Hibernation

### FR-1: Idle Auto-Hibernate
- Detect idle sessions: no new terminal output AND no user interaction for configurable duration
- Default: 2 hours; configurable globally in `config.json` and per-session
- Background sweeper goroutine; follows `SessionHealthChecker` ticker pattern

### FR-2: Manual Hibernate
- Right-click context menu → "Hibernate" on any `Active` session
- Available on same menu as Pause (both should be present — they have different semantics)

### FR-3: Resource-Pressure Hibernate
- Monitor system memory via gopsutil (already a dependency)
- At 85% threshold: hibernate oldest/most-idle `Active` sessions until pressure drops
- Hysteresis: re-check only after dropping to 75% to prevent oscillation

### FR-4: Checkpoint on Hibernate
- Write scrollback to `<checkpoint_dir>/<session_id>/scrollback.txt` (reference/copy from `ScrollbackManager`)
- Write metadata to `<checkpoint_dir>/<session_id>/checkpoint.json`
- SIGTERM AI process → wait 10s → SIGKILL
- Transition to `Hibernated` lifecycle state

### FR-5: Storage Configuration
- Default: `~/.stapler-squad/checkpoints/`
- Config fields: `hibernation.checkpoint_dir`, `hibernation.idle_timeout_minutes` (120), `hibernation.resource_pressure_threshold_pct` (85), `hibernation.enabled` (true)

### FR-6: Resume
- User clicks "Resume" on a `Hibernated` session
- Load scrollback from checkpoint, re-launch AI process
- Transition back to `Active`

### FR-7: Hibernated Badge
- Distinct status badge in both row and card views (snowflake or "zzz" icon)
- "Resume" action in context menu; "Hibernate" action for `Active` sessions

### FR-8: Cleanup
- Delete checkpoint files when session is deleted
- Optional retention period pruning

---

## Technical Constraints

### Auto-Resume Exclusion (critical)
`Hibernated` must be explicitly excluded from auto-restart paths:
1. `session/instance_serialization.go:329` — add `Hibernated` alongside `Stopped` in the exclusion guard
2. `session/health.go:130` — add early bailout for `Hibernated` before the recovery path
3. `session/instance_workspace.go:148,197,206` — workspace resume paths must skip `Hibernated`

### State machine guards
All places that check `i.Status == Paused` must also handle `Hibernated` correctly (both have no live process; both allow worktree operations).

### Database migration
- Existing integer status values must not shift — only add new values
- `Stopped` is currently `6` in the Go iota and `7` in proto; `Hibernated` should be the next unused value

### ConnectRPC
- Add `HibernateSession` and `ResumeSession` RPCs (do not overload `UpdateSession`)
- These RPCs trigger async work; they return immediately after validating the request

---

## Acceptance Criteria

- [ ] State machine has exactly 5 lifecycle states: `Creating`, `Active`, `Paused`, `Stopped`, `Hibernated`
- [ ] `Running || Ready` guards are gone from the codebase
- [ ] `NeedsApproval` no longer appears as a lifecycle state; sub-status is shown in sessions list
- [ ] Creating a session with a long-running init returns immediately; status transitions to `Active` async
- [ ] Sessions list shows sub-status chip for `Active` sessions (Processing / Idle / NeedsApproval / Error)
- [ ] A session idle for 2h is automatically hibernated and its process is gone
- [ ] Checkpoint files exist at `~/.stapler-squad/checkpoints/<id>/` after hibernation
- [ ] Resuming a hibernated session re-launches the process and shows saved scrollback
- [ ] Hibernated sessions are NOT auto-resumed on server restart or by the health checker
- [ ] No regressions in existing session create/stop/delete/pause flows
