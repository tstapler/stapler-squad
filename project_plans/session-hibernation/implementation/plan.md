# Session State Machine Redesign + Hibernation — Implementation Plan

## Overview

Four epics, ordered by dependency. Epic 1 must fully land before any other epic can
ship safely. Epics 2–3 can proceed in parallel once Epic 1 is merged. Epic 4 depends
on the `Active` and `Hibernated` lifecycle states from Epic 1.

| Epic | Title | Blocks |
|---|---|---|
| **Epic 1** | State Machine Redesign | Epics 2, 3, 4 |
| **Epic 2** | Async Session Creation | Independent after Epic 1 |
| **Epic 3** | Sub-Status Visibility | Independent after Epic 1 |
| **Epic 4** | Session Hibernation | Requires Epic 1 |

**Architectural decisions to resolve before coding:**

- **ADR-A**: The Go `Status` iota is being renumbered to a clean sequence
  (`Creating=0, Active=1, Paused=2, Stopped=3, Hibernated=4`). Task 1.1.4 provides an
  ent versioned migration that remaps existing DB integer values before any code change
  goes live. The proto enum integer wire values are independent of the Go iota: the proto
  keeps `SESSION_STATUS_ACTIVE = 1` (reusing the old `RUNNING = 1` slot), `PAUSED = 4`,
  `STOPPED = 7`, and adds `HIBERNATED = 8`. The Go↔proto adapter (Task 1.4.2) handles
  the translation between the two numbering schemes.

- **ADR-B**: Sub-status is never stored in the database. It is derived at response
  time from `GetEffectiveStatus()` and serialized into the `Session` proto only.

- **ADR-C**: The `HibernateSession` and `ResumeSession` RPCs are new top-level RPCs,
  not overloads of `UpdateSession`, to keep state transition semantics explicit.

---

## Epic 1: State Machine Redesign

**Goal:** Reduce lifecycle states from 7 to 5 (`Creating`, `Active`, `Paused`,
`Stopped`, `Hibernated`). Remove `Running`, `Ready`, `Loading`, `NeedsApproval`
as top-level lifecycle states.

**Prerequisite for:** All other epics.

---

### Story 1.1 — Rename Go Status Constants

**Goal:** Add `Active` and `Hibernated` constants; deprecate `Running`, `Ready`,
`Loading`, `NeedsApproval`.

#### Task 1.1.1 — Add new constants; mark old ones deprecated

**File:** `session/instance.go` (lines 22–58)

**What to change:**
- Replace the old iota with a clean new iota (Task 1.1.4 provides the DB migration that
  remaps existing integer rows, so tombstone slots are not needed):
  ```
  Creating      Status = iota  // 0
  Active                       // 1 — replaces Running/Ready
  Paused                       // 2
  Stopped                      // 3
  Hibernated                   // 4 — new
  ```
  Keep `Running`, `Ready`, `Loading`, `NeedsApproval` as deprecated aliases so existing
  call sites compile without change during the transition:
  ```go
  // Deprecated: use Active.
  Running = Active
  // Deprecated: use Active.
  Ready = Active
  // Deprecated: use Creating.
  Loading = Creating
  ```
  `NeedsApproval` is removed entirely — it becomes a sub-status, not a lifecycle
  constant. Search all call sites before removing.
- Add `Hibernated` to `String()` switch.
- Add `Hibernated` method (mirrors `Paused()`):
  ```go
  func (i *Instance) Hibernated() bool { return i.Status == Hibernated }
  ```

**Acceptance criteria:**
- `go build ./...` passes.
- `Active.String()` returns `"Active"`.
- `Hibernated.String()` returns `"Hibernated"`.
- `Creating == 0`, `Active == 1`, `Paused == 2`, `Stopped == 3`, `Hibernated == 4`.
- `Running == Active`, `Ready == Active`, `Loading == Creating` (alias equality in tests).
- Task 1.1.4 (DB migration) must land alongside or before this task is merged to main.

---

#### Task 1.1.2 — Update `state_machine.go` transition table

**File:** `session/state_machine.go`

**What to change:**
Replace `allowedTransitions map[Status][]Status` with a `TransitionDef`-based model:

1. **Define `TransitionDef` and a `transitionKey` index type:**
   ```go
   type transitionKey struct{ from, to Status }

   type TransitionDef struct {
       From  Status
       To    Status
       // Guard is called before the status is updated. Return non-nil to abort.
       // nil means unconditionally allowed.
       Guard func(ctx context.Context, i *Instance) error
       // After is called once the status has been updated (side-effects: process
       // management, worktree ops, scrollback restore, etc.).
       // nil means no post-transition side-effect.
       After func(ctx context.Context, i *Instance)
   }
   ```

2. **Declare the canonical transition slice:**
   ```go
   var transitionDefs = []TransitionDef{
       {From: Creating,   To: Active,     Guard: nil,                After: afterColdStart},
       {From: Creating,   To: Stopped,    Guard: nil,                After: nil},
       {From: Active,     To: Paused,     Guard: guardWorktreePresent, After: afterPause},
       {From: Active,     To: Stopped,    Guard: nil,                After: nil},
       {From: Active,     To: Hibernated, Guard: guardIsIdle,        After: afterHibernate},
       {From: Paused,     To: Active,     Guard: nil,                After: afterResume},
       {From: Paused,     To: Stopped,    Guard: nil,                After: nil},
       {From: Stopped,    To: Active,     Guard: guardWorktreePresent, After: afterColdStart},
       {From: Hibernated, To: Active,     Guard: nil,                After: afterWakeResume},
       {From: Hibernated, To: Stopped,    Guard: nil,                After: nil},
   }
   ```

3. **Build an init-time O(1) lookup index:**
   ```go
   var transitionIndex map[transitionKey]TransitionDef

   func init() {
       transitionIndex = make(map[transitionKey]TransitionDef, len(transitionDefs))
       for _, def := range transitionDefs {
           transitionIndex[transitionKey{def.From, def.To}] = def
       }
   }
   ```

4. **Implement `transitionTo` as the single choreography point:**
   ```go
   func (i *Instance) transitionTo(ctx context.Context, to Status) error {
       def, ok := transitionIndex[transitionKey{i.Status, to}]
       if !ok {
           return fmt.Errorf("invalid transition %s → %s", i.Status, to)
       }
       if def.Guard != nil {
           if err := def.Guard(ctx, i); err != nil {
               return fmt.Errorf("transition %s → %s blocked: %w", i.Status, to, err)
           }
       }
       i.Status = to
       if def.After != nil {
           def.After(ctx, i)
       }
       return nil
   }
   ```

5. **Guard functions:**
   - `guardIsIdle(ctx, i) error` — for `Active → Hibernated`: returns an error if the
     session's sub-status indicates the AI process is actively processing (i.e.,
     `GetEffectiveStatus()` is not `StatusIdle`). Allows hibernation to be blocked
     while a task is in flight.
   - `guardWorktreePresent(ctx, i) error` — for `Active → Paused` and `Stopped → Active`:
     returns an error if `i.gitManager.HasWorktree()` is false, preventing a pause/cold-start
     when the worktree has already been cleaned up.

6. **After-hooks:**
   - `afterHibernate(ctx, i)` — writes the checkpoint file and kills the tmux session
     (replaces the body of `Hibernate()` post-transition side-effects).
   - `afterWakeResume(ctx, i)` — re-launches the AI process via `i.Start(false)` and
     restores scrollback (replaces the body of `ResumeFromHibernation()` post-transition
     side-effects).
   - `afterPause(ctx, i)` — deletes the git worktree.
   - `afterResume(ctx, i)` — recreates the git worktree and re-launches the AI process.
   - `afterColdStart(ctx, i)` — performs the cold-restore start (used for
     `Creating → Active` and `Stopped → Active`).

7. **Retain `CanTransition(from, to Status) bool`** as a pure lookup against
   `transitionIndex` for callers that only need a reachability check.

Remove entries for `Ready`, `Running`, `NeedsApproval`, `Loading`.

Update the comment block at the top to reflect the new diagram from `requirements.md`.

**Acceptance criteria:**
- `CanTransition(Active, Hibernated)` returns `true`.
- `CanTransition(Hibernated, Active)` returns `true`.
- `CanTransition(Active, NeedsApproval)` returns `false` (removed).
- `CanTransition(Ready, Running)` returns `false` (removed).
- `transitionTo(ctx, Hibernated)` on an `Active` instance with an idle sub-status
  runs `afterHibernate` and sets `Status == Hibernated`.
- `transitionTo(ctx, Hibernated)` on an `Active` instance that is processing returns
  an error from `guardIsIdle`.
- All existing state machine unit tests pass or are updated.

---

#### Task 1.1.3 — Add `exhaustive` linter to CI lint step

**File:** `Makefile` (same `lint` target that runs `nilaway`)

**What to change:**
Install and wire up the [`exhaustive`](https://github.com/nishanths/exhaustive) linter,
which flags `switch` statements over a named type (here `Status`) that are missing one
or more cases:

1. Add to `make install-tools`:
   ```makefile
   go install github.com/nishanths/exhaustive/cmd/exhaustive@latest
   ```

2. Add to the `lint` target (alongside the existing `nilaway` invocation):
   ```makefile
   exhaustive ./session/... ./server/...
   ```
   Configure via `.exhaustive.toml` (or `-default-signifies-exhaustive=false`) so that
   a `default:` branch does **not** satisfy exhaustiveness — every `Status` case must be
   explicit or the build fails.

3. Fix any pre-existing `switch Status` statements that are currently non-exhaustive
   before landing this task.

**Acceptance criteria:**
- `make lint` fails if a new `Status` constant is added without updating all
  `switch Status` statements in `session/` and `server/`.
- `make ci` passes with no exhaustiveness violations.

---

#### Task 1.1.4 — Ent versioned migration: remap integer status values

**Files:**
- `session/ent/migrate/migrations/` (new versioned migration file)
- `session/ent/schema/session.go` (verify `enums` field uses new iota)

**Background:**
The current `Status` iota stored as an integer column is:
```
Running=0, Ready=1, Loading=2, Paused=3, NeedsApproval=4, Creating=5, Stopped=6
```
The new iota must be:
```
Creating=0, Active=1, Paused=2, Stopped=3, Hibernated=4
```
This is a **breaking integer remapping** — existing rows must be migrated.

**Old → New integer mapping:**

| Old value | Old name | New value | New name | Notes |
|---|---|---|---|---|
| 0 | Running | 1 | Active | Running becomes Active |
| 1 | Ready | 1 | Active | Ready becomes Active |
| 2 | Loading | 0 | Creating | Loading becomes Creating |
| 3 | Paused | 2 | Paused | Paused shifts from 3→2 |
| 4 | NeedsApproval | 1 | Active | NeedsApproval becomes Active |
| 5 | Creating | 0 | Creating | Creating shifts from 5→0 |
| 6 | Stopped | 3 | Stopped | Stopped shifts from 6→3 |

**What to change:**
Generate a new ent versioned migration (using the correct command from CLAUDE.md):
```bash
go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema
```
Then create the versioned migration SQL file. Because multiple old values map to the
same new value, the UPDATE must be done in a single pass using a CASE expression to
avoid overwriting already-remapped rows. Use a temporary sentinel offset (+100) to
avoid collision during the multi-step remap:

```sql
-- Step 1: shift all old values up by 100 as sentinels (avoids collision)
UPDATE sessions SET status = status + 100;

-- Step 2: remap sentinel values to new integers
UPDATE sessions SET status = CASE status
    WHEN 100 THEN 1   -- old Running(0)   → new Active(1)
    WHEN 101 THEN 1   -- old Ready(1)     → new Active(1)
    WHEN 102 THEN 0   -- old Loading(2)   → new Creating(0)
    WHEN 103 THEN 2   -- old Paused(3)    → new Paused(2)
    WHEN 104 THEN 1   -- old NeedsApproval(4) → new Active(1)
    WHEN 105 THEN 0   -- old Creating(5)  → new Creating(0)
    WHEN 106 THEN 3   -- old Stopped(6)   → new Stopped(3)
    ELSE status        -- future-proof: leave unknowns (should not exist)
END;
```

Register the migration file with the ent versioned migration atlas workflow
(same pattern as any existing migration in `session/ent/migrate/migrations/`).

**Acceptance criteria:**
- Running the migration against a database with all old status values produces rows
  with only the values `{0, 1, 2, 3}` (no 4–106 remaining).
- `go build ./...` passes after schema regeneration.
- The migration is idempotent when run twice (second run is a no-op).
- A test in `session/ent/migrate/` verifies the remapping on a SQLite in-memory DB.

---

### Story 1.2 — Guard Auto-Resume Paths Against Hibernated Status

**Goal:** Ensure `Hibernated` sessions are never auto-started on server restart or
by the health checker. These two fixes are **blocking** — they must land before
Epic 4 is merged.

#### Task 1.2.1 — Deserialization exclusion guard

**File:** `session/instance_serialization.go` (lines 304–335)

**What to change:**
In `FromInstanceData()`, the final `else` branch currently calls `instance.Start(false)`
for any status that is not `Paused` or `Stopped`. Add a `Hibernated` exclusion branch
between the `Stopped` block and the final `else`:

```go
} else if instance.Status == Stopped {
    // ... existing recovery logic unchanged ...
} else if instance.Status == Hibernated {
    // Wire tmux session object (for DoesSessionExist checks at resume time)
    // but do NOT call Start — hibernated sessions resume only on explicit request.
    tmuxPrefix := instance.TmuxPrefix
    if tmuxPrefix == "" {
        tmuxPrefix = "staplersquad_"
    }
    if instance.TmuxServerSocket != "" {
        instance.tmuxManager.SetSession(tmux.NewTmuxSessionWithServerSocket(
            instance.Title, instance.Program, tmuxPrefix,
            instance.TmuxServerSocket, tmux.WithRegistry(nil)))
    } else {
        instance.tmuxManager.SetSession(tmux.NewTmuxSessionWithPrefix(
            instance.Title, instance.Program, tmuxPrefix))
    }
    instance.started = true
} else {
    if err := instance.Start(false); err != nil {
        return nil, err
    }
}
```

Also update the worktree-missing guard at line ~274 to skip the transition-to-Paused
path for `Hibernated` sessions (worktree may be intentionally dirty):
```go
if !instance.Paused() && !instance.Hibernated() && instance.gitManager.HasWorktree() {
```

**Acceptance criteria:**
- A session serialized with `Status == Hibernated` does not call `Start()` on
  `FromInstanceData()`.
- `instance.started` is `true` after deserialization (so health checks see it as "started").
- Worktree-missing path does not transition a `Hibernated` session to `Paused`.

---

#### Task 1.2.2 — Health checker early bailout

**File:** `session/health.go` (lines 95–105, after the `Paused()` guard)

**What to change:**
Add a guard for `Hibernated` immediately after the existing `Paused()` guard:
```go
// Skip paused instances - they're expected to not have active tmux sessions
if instance.Paused() {
    result.Actions = append(result.Actions, "Skipped (session is paused)")
    return result
}

// Skip hibernated instances - they have no tmux by design
if instance.Hibernated() {
    result.Actions = append(result.Actions, "Skipped (session is hibernated)")
    return result
}
```

**Acceptance criteria:**
- Health check on a `Hibernated` instance returns `IsHealthy: true`,
  `RecoveryAttempted: false`, and `Actions` containing "Skipped (session is hibernated)".
- No `Start()` call is made for hibernated instances.

---

#### Task 1.2.3 — Stale-resume path guard

**File:** `session/instance_claude.go` (line ~85, `recoverFromStaleResume()` callback)

**What to change:**
The stale-resume auto-restart fires when a `--resume` UUID points to a non-existent
JSONL file. Add a guard so this recovery does not fire for `Hibernated` sessions:
```go
func (i *Instance) recoverFromStaleResume() {
    if i.Hibernated() {
        log.Info("skipping stale-resume recovery for hibernated session", "session", i.Title)
        return
    }
    // ... existing recovery logic ...
}
```

**Acceptance criteria:**
- A hibernated session with a stale conversation UUID does not auto-restart.

---

### Story 1.3 — Replace Running/Ready Guards in Go Code

**Goal:** Remove all `Running || Ready` (and `NeedsApproval`) guards from non-test code.

#### Task 1.3.1 — Audit and replace all guards

**Files to audit** (search for `Running`, `Ready`, `NeedsApproval` in `session/` and
`server/`):
- `session/status_mapping.go` — detection results that set `Running`/`Ready` → set `Active`
- `server/services/session_service.go` — any status comparison/switch in RPC handlers
- `server/adapters/instance_adapter.go` — Go ↔ proto status mapping
- `session/instance.go` — `Started()`, `IsActive()` helper methods
- Any other files found by `grep -rn "Running\|Ready\|NeedsApproval" session/ server/`

**For each file:**
- Replace `instance.Status == Running || instance.Status == Ready` with
  `instance.Status == Active` (or `instance.Active()` helper).
- Replace `case Running:` / `case Ready:` in switch statements with `case Active:`.
- In `status_mapping.go`, mapping detection output to `Running` or `Ready` should
  instead set `Active`; the sub-status is now the fine-grained signal.
- `NeedsApproval` guards that were checking lifecycle state should route to sub-status
  check instead (Epic 3 handles the display; Epic 1 just removes the lifecycle state).

**Acceptance criteria:**
- `grep -rn "== Running\|== Ready\|== NeedsApproval\|case Running\|case Ready\|case NeedsApproval" session/ server/` returns zero hits (excluding deprecated alias declarations and test fixtures).
- All existing tests pass.

---

#### Task 1.3.2 — Add `Active()` helper method

**File:** `session/instance.go`

**What to change:**
```go
// Active returns true when the session is running with a live AI process.
func (i *Instance) Active() bool { return i.Status == Active }
```
Mirror the existing `Paused()` and `Stopped()` pattern.

**Acceptance criteria:**
- `Active()` returns `true` only when `Status == Active`.
- Callers that previously used `Running() || Ready()` now use `Active()`.

---

### Story 1.4 — Proto and Adapter Updates

**Goal:** Update the proto enum and the Go↔proto adapter to match the new model.
Preserve existing integer wire values.

#### Task 1.4.1 — Update `types.proto` SessionStatus enum

**File:** `proto/session/v1/types.proto` (lines 168–184)

**What to change:**
```protobuf
enum SessionStatus {
  SESSION_STATUS_UNSPECIFIED    = 0;
  // Session is active with a live AI process (replaces RUNNING and READY).
  SESSION_STATUS_ACTIVE         = 1;   // was SESSION_STATUS_RUNNING; integer value preserved
  // Deprecated: use SESSION_STATUS_ACTIVE.
  SESSION_STATUS_RUNNING        = 1 [deprecated = true];
  // Deprecated: use SESSION_STATUS_ACTIVE.
  SESSION_STATUS_READY          = 2 [deprecated = true];
  // Deprecated: use SESSION_STATUS_CREATING.
  SESSION_STATUS_LOADING        = 3 [deprecated = true];
  SESSION_STATUS_PAUSED         = 4;
  // Deprecated: NeedsApproval is now a sub-status, not a lifecycle state.
  SESSION_STATUS_NEEDS_APPROVAL = 5 [deprecated = true];
  SESSION_STATUS_CREATING       = 6;
  SESSION_STATUS_STOPPED        = 7;
  SESSION_STATUS_HIBERNATED     = 8;   // new
}
```

Note: Proto3 allows multiple names with the same integer value via `allow_alias = true`.
Add `option allow_alias = true;` to the enum if needed for the deprecated aliases.

Run `make generate-proto` to regenerate Go and TypeScript bindings.

**Acceptance criteria:**
- `make generate-proto` succeeds.
- `SESSION_STATUS_ACTIVE` = 1, `SESSION_STATUS_HIBERNATED` = 8.
- TypeScript generated types include `SessionStatus.ACTIVE` and `SessionStatus.HIBERNATED`.

---

#### Task 1.4.2 — Update `instance_adapter.go`

**File:** `server/adapters/instance_adapter.go`

**What to change:**
In the Go-to-proto status mapping function, add/update:
```go
case session.Active:
    return sessionv1.SessionStatus_SESSION_STATUS_ACTIVE
case session.Paused:
    return sessionv1.SessionStatus_SESSION_STATUS_PAUSED
case session.Creating:
    return sessionv1.SessionStatus_SESSION_STATUS_CREATING
case session.Stopped:
    return sessionv1.SessionStatus_SESSION_STATUS_STOPPED
case session.Hibernated:
    return sessionv1.SessionStatus_SESSION_STATUS_HIBERNATED
```
Remove cases for `Running`, `Ready`, `Loading`, `NeedsApproval` as separate entries
(they resolve to `Active` or `Creating` via the alias).

In the proto-to-Go reverse mapping:
```go
case sessionv1.SessionStatus_SESSION_STATUS_ACTIVE,
    sessionv1.SessionStatus_SESSION_STATUS_RUNNING,
    sessionv1.SessionStatus_SESSION_STATUS_READY:
    return session.Active
case sessionv1.SessionStatus_SESSION_STATUS_LOADING:
    return session.Creating
case sessionv1.SessionStatus_SESSION_STATUS_NEEDS_APPROVAL:
    return session.Active   // sub-status, not lifecycle
case sessionv1.SessionStatus_SESSION_STATUS_HIBERNATED:
    return session.Hibernated
```

**Acceptance criteria:**
- All status round-trip tests pass.
- `session.Active` → proto → `session.Active` lossless.
- `session.Hibernated` → proto → `session.Hibernated` lossless.
- Old proto values (1, 2, 3, 5) received from a legacy client all resolve to valid
  Go statuses without panicking.

---

### Story 1.5 — Frontend Status Updates

**Goal:** Update all frontend status handling to use the new model.

#### Task 1.5.1 — Update `getStatusColor` and `getStatusText`

**File:** `web-app/src/components/sessions/SessionCard.tsx`

**What to change:**
In `getStatusColor()`, replace `SessionStatus.RUNNING` and `SessionStatus.READY` cases
with `SessionStatus.ACTIVE`. Add `SessionStatus.HIBERNATED` case:
```typescript
case SessionStatus.ACTIVE:
case SessionStatus.RUNNING:   // backward compat
case SessionStatus.READY:     // backward compat
    return styles.statusRunning;  // reuse existing; rename in CSS later if desired
case SessionStatus.HIBERNATED:
    return styles.statusHibernated;  // new CSS class
```
In `getStatusText()`:
```typescript
case SessionStatus.ACTIVE:
    return "Active";
case SessionStatus.HIBERNATED:
    return "Hibernated";
```

**Acceptance criteria:**
- `getStatusText(SessionStatus.ACTIVE)` returns `"Active"`.
- `getStatusText(SessionStatus.HIBERNATED)` returns `"Hibernated"`.
- `getStatusColor(SessionStatus.HIBERNATED)` returns `styles.statusHibernated`.

---

#### Task 1.5.2 — Add CSS for `statusHibernated`

**Files:**
- `web-app/src/components/sessions/SessionCard.css.ts`
- `web-app/src/components/sessions/SessionRow.css.ts`

**What to change:**
In `SessionCard.css.ts`, add:
```ts
export const statusHibernated = style({
    backgroundColor: vars.color.statusHibernated ?? '#b0c4de',  // light steel blue
    color: vars.color.textInverse,
});
```
In `SessionRow.css.ts`, add data-status attribute rule:
```ts
'[data-status="hibernated"]': {
    color: vars.color.statusHibernated ?? '#b0c4de',
},
```
Add `statusHibernated` token to `web-app/src/styles/theme.css.ts` if it does not exist.

**Acceptance criteria:**
- `make ci` passes (`lint:css` does not fail on undefined CSS variables).
- Hibernated sessions show a visually distinct badge (blue-gray) in both card and row views.

---

#### Task 1.5.3 — Update all frontend status switches

**Files to audit:**
- Any TypeScript file with a `switch (status)` or `status ===` comparing against
  `SessionStatus.RUNNING`, `SessionStatus.READY`, `SessionStatus.LOADING`,
  `SessionStatus.NEEDS_APPROVAL`.

Search: `grep -rn "SessionStatus\." web-app/src/`

For each hit:
- `RUNNING` / `READY` / `LOADING` references → add `ACTIVE` as primary case, keep
  old names as fallthrough for backward compat during transition.
- `NEEDS_APPROVAL` lifecycle checks → replace with sub-status check (Epic 3).
- Add `HIBERNATED` to any exhaustiveness switch.

**Acceptance criteria:**
- `npx tsc --noEmit` in `web-app/` passes with no new errors.
- No TypeScript exhaustiveness errors for unhandled `SessionStatus` variants.

---

### Story 1.6 — State Machine Migration Tests

#### Task 1.6.1 — Update existing state machine tests

**File:** `session/state_machine_test.go` (create if not exists)

**What to change:**
- Verify all valid transitions in `allowedTransitions` succeed.
- Verify removed transitions (`Running → Ready`, `Running → NeedsApproval`) fail.
- Verify new transitions (`Active → Hibernated`, `Hibernated → Active`) succeed.

#### Task 1.6.2 — Update deserialization tests

**File:** `session/instance_serialization_test.go` (or nearest test file)

**What to change:**
- Add test: `FromInstanceData` with `Status == Hibernated` does not call `Start()`.
- Verify `instance.started == true` and `instance.Hibernated() == true` after
  deserializing a hibernated session.

**Acceptance criteria:**
- `make test` passes for `session/` package.

---

## Epic 2: Async Session Creation

**Depends on:** Epic 1 (Active state must exist).

**Goal:** `CreateSession` RPC returns immediately with a `Creating` status session.
Setup (repo clone, npm install, first tmux start) runs in a background goroutine.

---

### Story 2.1 — Proto: `creation_progress` field

#### Task 2.1.1 — Add field to `Session` proto

**File:** `proto/session/v1/types.proto`

**What to change:**
Add to the `Session` message (next available field number after 50):
```protobuf
// Human-readable progress message during Creating state (empty otherwise).
string creation_progress = 51;
```

Run `make generate-proto`.

**Acceptance criteria:**
- `Session.creation_progress` is accessible in Go and TypeScript generated code.

---

### Story 2.2 — Backend: Async creation goroutine

#### Task 2.2.1 — Refactor `CreateSession` to return early

**File:** `server/services/session_service.go`

**What to change:**
1. Create the session record in ent with `Status = Creating` before any setup work.
2. Save the session to storage and broadcast a `SessionEvent` with the `Creating` session.
3. Return the `Creating` session proto in `CreateSessionResponse` immediately.
4. Spawn `go func() { ... }()` to perform: worktree init, repo clone, npm install,
   `instance.Start()`. Progress updates broadcast via `WatchSessions` stream as
   `UpdatedSession` events with updated `creation_progress` strings.
5. On success: `instance.transitionTo(Active)` → save → broadcast.
6. On failure: `instance.transitionTo(Stopped)` → store error in `creation_progress`
   (repurposed as final error message) → save → broadcast.

**Acceptance criteria:**
- `CreateSession` RPC returns within 100ms for any input.
- The returned session has `status = Creating`.
- Long-running setups (simulated with `time.Sleep`) do not block the RPC.
- Session transitions to `Active` (or `Stopped`) after async setup completes.
- `WatchSessions` stream receives progress events during `Creating`.

---

#### Task 2.2.2 — Guard mutations on `Creating` sessions

**File:** `server/services/session_service.go`

**What to change:**
In handlers for `HibernateSession`, `PauseSession`, `UpdateSession`, `DeleteSession`:
add a guard returning `connect.CodeFailedPrecondition` if `instance.Status == Creating`.

**Acceptance criteria:**
- Attempting to hibernate a `Creating` session returns a `FailedPrecondition` error.
- Attempting to delete a `Creating` session returns a `FailedPrecondition` error
  (or succeeds after cancelling the background goroutine — document the choice).

---

### Story 2.3 — Frontend: Creating state UX

#### Task 2.3.1 — Show progress indicator for `Creating` sessions

**File:** `web-app/src/components/sessions/SessionRow.tsx`,
`web-app/src/components/sessions/SessionCard.tsx`

**What to change:**
- Sessions with `status == SessionStatus.CREATING` show a spinner alongside the
  "Creating" status text.
- Display `session.creation_progress` as a subtitle/secondary label beneath the title
  when non-empty.
- Disable hibernate, pause, restart, and delete action menu items when `Creating`.

**Acceptance criteria:**
- `Creating` session rows show a spinner.
- `creation_progress` text is visible when present.
- Disabled actions do not appear (or appear grayed-out with tooltip) for `Creating` sessions.

---

## Epic 3: Sub-Status Visibility in Sessions List

**Depends on:** Epic 1 (Active lifecycle state; NeedsApproval removed from lifecycle).

**Goal:** Surface fine-grained `DetectedStatus` as a sub-status chip alongside the
lifecycle badge in both row and card views.

---

### Story 3.1 — Proto: sub_status field

#### Task 3.1.1 — Add `sub_status` to `Session` proto

**File:** `proto/session/v1/types.proto`

**What to change:**
Add a new enum and field:
```protobuf
// SubStatus provides fine-grained activity state for Active sessions.
// Derived at read time from the detection layer; never stored in the database.
enum SubStatus {
  SUB_STATUS_UNSPECIFIED = 0;
  SUB_STATUS_IDLE        = 1;
  SUB_STATUS_PROCESSING  = 2;
  SUB_STATUS_NEEDS_APPROVAL = 3;
  SUB_STATUS_ERROR       = 4;
  SUB_STATUS_TESTS_FAILING = 5;
  SUB_STATUS_RATE_LIMITED = 6;
}
```
Add to `Session` message:
```protobuf
// Fine-grained activity state for Active sessions. Derived; not persisted.
SubStatus sub_status = 52;
```

Run `make generate-proto`.

**Acceptance criteria:**
- `Session.sub_status` is accessible in Go and TypeScript.
- `SubStatus` enum is accessible in TypeScript as `SubStatus`.

---

### Story 3.2 — Backend: Populate sub_status at response time

#### Task 3.2.1 — Map `DetectedStatus` to `SubStatus` in adapter

**File:** `server/adapters/instance_adapter.go`

**What to change:**
Add a `toProtoSubStatus(instance *session.Instance) sessionv1.SubStatus` helper:
```go
func toProtoSubStatus(i *session.Instance) sessionv1.SubStatus {
    if i.Status != session.Active {
        return sessionv1.SubStatus_SUB_STATUS_UNSPECIFIED
    }
    switch i.GetEffectiveStatus() {  // returns detection.Status
    case detection.StatusProcessing:
        return sessionv1.SubStatus_SUB_STATUS_PROCESSING
    case detection.StatusNeedsApproval:
        return sessionv1.SubStatus_SUB_STATUS_NEEDS_APPROVAL
    case detection.StatusError:
        return sessionv1.SubStatus_SUB_STATUS_ERROR
    case detection.StatusTestsFailing:
        return sessionv1.SubStatus_SUB_STATUS_TESTS_FAILING
    default:
        return sessionv1.SubStatus_SUB_STATUS_IDLE
    }
}
```
Call this from the session-to-proto conversion and populate `session.SubStatus`.

Also handle `RateLimited` state: if `instance.RateLimitState == RateLimitStateWaiting`,
return `SUB_STATUS_RATE_LIMITED` (takes precedence over detection layer sub-status).

**Acceptance criteria:**
- A `Running` (now `Active`) session with `detection.StatusProcessing` serializes to
  `sub_status = SUB_STATUS_PROCESSING`.
- A `Paused` session always serializes to `sub_status = SUB_STATUS_UNSPECIFIED`.

---

### Story 3.3 — Frontend: Sub-status chip component

#### Task 3.3.1 — Create `SubStatusChip` component

**Files:**
- `web-app/src/components/sessions/SubStatusChip.tsx` (new)
- `web-app/src/components/sessions/SubStatusChip.css.ts` (new)

**What to change:**
Create a small chip component:
```tsx
interface SubStatusChipProps {
  subStatus: SubStatus;
}
export function SubStatusChip({ subStatus }: SubStatusChipProps) {
  if (subStatus === SubStatus.SUB_STATUS_UNSPECIFIED ||
      subStatus === SubStatus.SUB_STATUS_IDLE) return null;
  // render appropriate icon + label
}
```
Icons:
- `PROCESSING` → spinner or animated dot, label "Thinking…"
- `NEEDS_APPROVAL` → bell icon, label "Needs Approval", orange style
- `ERROR` → red X icon, label "Error"
- `TESTS_FAILING` → orange triangle, label "Tests Failing"
- `RATE_LIMITED` → clock icon, label "Rate Limited"

**Acceptance criteria:**
- `SubStatusChip` renders null for `UNSPECIFIED` and `IDLE`.
- `NEEDS_APPROVAL` chip renders with orange/warning styling.
- CSS uses vanilla-extract `.css.ts`, no hardcoded hex values.

---

#### Task 3.3.2 — Integrate `SubStatusChip` into `SessionRow` and `SessionCard`

**Files:**
- `web-app/src/components/sessions/SessionRow.tsx`
- `web-app/src/components/sessions/SessionCard.tsx`

**What to change:**
- Import and render `<SubStatusChip subStatus={session.subStatus} />` next to the
  status dot/badge.
- In `SessionRow`: place chip after the status dot in the status cell.
- In `SessionCard`: place chip below the lifecycle badge in the header.

**Acceptance criteria:**
- `Active` sessions with `subStatus = PROCESSING` show a spinner chip.
- `Active` sessions with `subStatus = IDLE` show no chip.
- Non-`Active` sessions (Paused, Stopped, Hibernated) show no chip.

---

### Story 3.4 — Grouping by Sub-Status

#### Task 3.4.1 — Add `SubStatus` grouping strategy

**File:** The session grouping strategy file (likely
`web-app/src/lib/grouping/` or colocated with session list).

**What to change:**
- Add a `"SubStatus"` grouping strategy that groups sessions by their `subStatus`
  field value (only for `Active` sessions; all others group under "Inactive").
- Add `"SubStatus"` to the grouping strategy selector dropdown.

**Acceptance criteria:**
- Selecting "Group by Sub-Status" creates groups: "Needs Approval", "Thinking",
  "Error", "Idle/Other".
- The "Needs Approval" group is sorted to the top.

---

## Epic 4: Session Hibernation

**Depends on:** Epic 1 (Active + Hibernated lifecycle states, auto-resume guards).

**Goal:** Manual hibernate, auto-idle hibernate, resource-pressure hibernate, and
seamless resume.

---

### Story 4.1 — Config: HibernationConfig

#### Task 4.1.1 — Add `HibernationConfig` struct and config fields

**File:** `config/config.go`

**What to change:**
Following the `OneOffBaseDirOrDefault()` pattern (lines 388–434):
```go
type HibernationConfig struct {
    Enabled                  bool   `json:"enabled"`
    IdleTimeoutMinutes       int    `json:"idle_timeout_minutes"`
    MemoryThresholdPct       int    `json:"memory_threshold_pct"`
    MemoryHysteresisPct      int    `json:"memory_hysteresis_pct"`
    CheckpointDir            string `json:"checkpoint_dir"`
    RetentionDays            int    `json:"retention_days"`
}

func (c *Config) HibernationCheckpointDirOrDefault() (string, error) {
    dir := c.Hibernation.CheckpointDir
    if dir == "" {
        dir = "~/.stapler-squad/checkpoints"
    }
    // tilde expansion (same pattern as OneOffBaseDirOrDefault)
    return expandTilde(dir)
}
```
Default values: `Enabled=true`, `IdleTimeoutMinutes=120`, `MemoryThresholdPct=85`,
`MemoryHysteresisPct=75`, `RetentionDays=30`.

Add `Hibernation HibernationConfig` field to the root `Config` struct.

**Acceptance criteria:**
- `config.json` with `"hibernation": {"idle_timeout_minutes": 60}` is loaded correctly.
- `HibernationCheckpointDirOrDefault()` expands `~` and returns an absolute path.
- Default values are applied when fields are absent from `config.json`.

---

### Story 4.2 — Proto and RPC: HibernateSession + ResumeSession

#### Task 4.2.1 — Add RPCs to `session.proto`

**File:** `proto/session/v1/session.proto`

**What to change:**
Add request/response messages and RPC methods:
```protobuf
message HibernateSessionRequest {
    string id = 1;
}
message HibernateSessionResponse {
    Session session = 1;
}
message ResumeHibernatedSessionRequest {
    string id = 1;
}
message ResumeHibernatedSessionResponse {
    Session session = 1;
}
```
Add to `SessionService`:
```protobuf
rpc HibernateSession(HibernateSessionRequest) returns (HibernateSessionResponse) {}
rpc ResumeHibernatedSession(ResumeHibernatedSessionRequest) returns (ResumeHibernatedSessionResponse) {}
```

Run `make generate-proto`.

**Acceptance criteria:**
- `make generate-proto` succeeds.
- `HibernateSession` and `ResumeHibernatedSession` appear in generated Go and TypeScript.

---

### Story 4.3 — Go Core: `Hibernate()` and `Resume()` on Instance

#### Task 4.3.1 — Create `session/hibernate.go`

**File:** `session/hibernate.go` (new file)

**What to change:**
Implement `Hibernate(ctx context.Context, checkpointDir string) error` on `*Instance`:

```go
func (i *Instance) Hibernate(ctx context.Context, checkpointDir string) error {
    // 1. Validate: must be Active
    if err := i.transitionTo(Hibernated); err != nil {
        return fmt.Errorf("hibernate: invalid state transition: %w", err)
    }
    // 2. Write checkpoint: copy scrollback reference + InstanceData subset
    if err := i.writeCheckpoint(checkpointDir); err != nil {
        // roll back state — do not kill process if checkpoint failed
        i.setStatus(Active)
        return fmt.Errorf("hibernate: checkpoint write failed: %w", err)
    }
    // 3. Kill tmux session (SIGTERM via tmux kill-session; wait 10s; SIGKILL fallback)
    if err := i.killWithGrace(ctx, 10*time.Second); err != nil {
        log.Warn("hibernate: kill failed, session may still be running", "session", i.Title, "err", err)
        // Continue — checkpoint is written, status is Hibernated, process may self-exit
    }
    return nil
}
```

Implement `killWithGrace(ctx, timeout)`:
- Call `i.tmuxManager.KillSession()` (sends SIGKILL via `tmux kill-session`).
- For a more graceful path first, use `tmux send-keys -t <name> '' ''` + wait; then
  kill. Document the decision: full graceful SIGTERM to the Claude process is not
  currently possible because tmux kill-session is the only termination path; this is
  acceptable for v1.

**Checkpoint format** (write via `writeCheckpoint(dir)`):
- `<checkpointDir>/<uuid>/checkpoint.json` — JSON subset of `InstanceData`: Title, UUID,
  Path, Branch, Status, Program, timestamps, `ClaudeConversationUUID`.
- `<checkpointDir>/<uuid>/scrollback_ref.txt` — absolute path to the session's existing
  scrollback file at `~/.stapler-squad/sessions/<uuid>/scrollback.json[.zstd]`.

**Acceptance criteria:**
- `Hibernate()` on an `Active` instance writes checkpoint files and sets
  `Status == Hibernated`.
- `Hibernate()` on a non-`Active` instance returns an error without writing files.
- After `Hibernate()`, `TmuxAlive()` returns `false`.
- Checkpoint directory is created if it does not exist.

---

#### Task 4.3.2 — Implement `ResumeFromHibernation()` on Instance

**File:** `session/hibernate.go`

**What to change:**
```go
func (i *Instance) ResumeFromHibernation(ctx context.Context, checkpointDir string) error {
    if err := i.transitionTo(Active); err != nil {
        return fmt.Errorf("resume: invalid state transition: %w", err)
    }
    // Re-launch AI process (same as Start(false) with --resume flag if UUID known)
    if err := i.Start(false); err != nil {
        i.setStatus(Hibernated)  // roll back
        return fmt.Errorf("resume: start failed: %w", err)
    }
    // Clean up checkpoint files
    checkpointPath := filepath.Join(checkpointDir, i.UUID)
    if err := os.RemoveAll(checkpointPath); err != nil {
        log.Warn("resume: failed to clean up checkpoint", "session", i.Title, "err", err)
        // Non-fatal: session is running, checkpoint files are stale but harmless
    }
    return nil
}
```

**Acceptance criteria:**
- `ResumeFromHibernation()` on a `Hibernated` instance calls `Start()` and transitions
  to `Active`.
- Checkpoint files are deleted after successful resume.
- `ResumeFromHibernation()` on a non-`Hibernated` instance returns an error.

---

### Story 4.4 — RPC Handlers: HibernateSession + ResumeHibernatedSession

#### Task 4.4.1 — Implement `HibernateSession` handler

**File:** `server/services/session_service.go`

**What to change:**
```go
func (s *SessionService) HibernateSession(
    ctx context.Context,
    req *connect.Request[sessionv1.HibernateSessionRequest],
) (*connect.Response[sessionv1.HibernateSessionResponse], error) {
    instance, err := s.storage.GetByID(req.Msg.Id)
    if err != nil { return nil, connect.NewError(connect.CodeNotFound, err) }

    checkpointDir, err := s.cfg.HibernationCheckpointDirOrDefault()
    if err != nil { return nil, connect.NewError(connect.CodeInternal, err) }

    if err := instance.Hibernate(ctx, checkpointDir); err != nil {
        return nil, connect.NewError(connect.CodeFailedPrecondition, err)
    }

    if err := s.storage.Update(instance); err != nil {
        return nil, connect.NewError(connect.CodeInternal, err)
    }
    s.broadcastSessionEvent(instance)

    return connect.NewResponse(&sessionv1.HibernateSessionResponse{
        Session: s.instanceToProto(instance),
    }), nil
}
```

Register the handler in `server/server.go`.

**Acceptance criteria:**
- `HibernateSession` RPC on an `Active` session returns the updated `Hibernated` session.
- `HibernateSession` on a `Paused` session returns `FailedPrecondition`.
- `WatchSessions` stream receives a `UpdatedSession` event with the `Hibernated` session.

---

#### Task 4.4.2 — Implement `ResumeHibernatedSession` handler

**File:** `server/services/session_service.go`

**What to change:**
Mirror the `HibernateSession` handler, calling `instance.ResumeFromHibernation()`.
Guard: only `Hibernated` sessions are resumable via this RPC; `Paused` sessions use
the existing `UpdateSession` resume path.

Register in `server/server.go`.

**Acceptance criteria:**
- `ResumeHibernatedSession` on a `Hibernated` session returns the updated `Active` session.
- `ResumeHibernatedSession` on a `Paused` session returns `FailedPrecondition`.
- `WatchSessions` stream receives the `Active` session event.

---

### Story 4.5 — Idle Auto-Hibernate Sweeper

#### Task 4.5.1 — Create `session/hibernation_sweeper.go`

**File:** `session/hibernation_sweeper.go` (new file)

**What to change:**
Following the `SessionHealthChecker` ticker pattern (`session/health.go`):
```go
type HibernationSweeper struct {
    storage       Storage
    cfg           *config.Config
    ticker        *time.Ticker
    stopCh        chan struct{}
}

func NewHibernationSweeper(storage Storage, cfg *config.Config) *HibernationSweeper

func (s *HibernationSweeper) Start(ctx context.Context)

func (s *HibernationSweeper) sweep(ctx context.Context) {
    instances := s.storage.ListAll()
    idleTimeout := time.Duration(s.cfg.Hibernation.IdleTimeoutMinutes) * time.Minute
    checkpointDir, _ := s.cfg.HibernationCheckpointDirOrDefault()

    for _, inst := range instances {
        if inst.Status != Active { continue }
        lastActivity := inst.ReviewState.LastMeaningfulOutput
        if time.Since(lastActivity) > idleTimeout {
            log.Info("auto-hibernating idle session", "session", inst.Title,
                "idle_duration", time.Since(lastActivity))
            if err := inst.Hibernate(ctx, checkpointDir); err != nil {
                log.Warn("auto-hibernate failed", "session", inst.Title, "err", err)
                continue
            }
            s.storage.Update(inst)
            s.broadcastSessionEvent(inst)
        }
    }
}
```

**Acceptance criteria:**
- Sweeper fires on the configured interval (default: every 5 minutes to check the
  2-hour threshold).
- `Active` sessions idle past `IdleTimeoutMinutes` are hibernated.
- Non-`Active` sessions are skipped.

---

#### Task 4.5.2 — Wire sweeper into server startup

**File:** `server/dependencies.go` (or `server/server.go`)

**What to change:**
After `SessionService` and `Storage` are created:
```go
if cfg.Hibernation.Enabled {
    sweeper := session.NewHibernationSweeper(storage, cfg)
    go sweeper.Start(serverCtx)
}
```

**Acceptance criteria:**
- Sweeper is started when `hibernation.enabled = true` in config.
- Sweeper is not started when `hibernation.enabled = false`.
- Server shuts down cleanly (sweeper respects context cancellation).

---

### Story 4.6 — Resource-Pressure Hibernate

#### Task 4.6.1 — Memory monitor and pressure-based hibernation

**File:** `session/hibernation_sweeper.go` (add to sweeper)

**What to change:**
Add `checkResourcePressure(ctx)` called within `sweep()`:
```go
func (s *HibernationSweeper) checkResourcePressure(ctx context.Context) {
    vmStat, err := mem.VirtualMemory()
    if err != nil {
        log.Warn("resource pressure check: failed to read memory", "err", err)
        return
    }
    threshold := float64(s.cfg.Hibernation.MemoryThresholdPct)
    if vmStat.UsedPercent < threshold { return }

    // Hibernate sessions oldest-idle-first until memory drops below hysteresis threshold
    activeSessions := s.activeSessionsSortedByIdleTime()
    for _, inst := range activeSessions {
        checkpointDir, _ := s.cfg.HibernationCheckpointDirOrDefault()
        if err := inst.Hibernate(ctx, checkpointDir); err != nil { continue }
        s.storage.Update(inst)
        s.broadcastSessionEvent(inst)
        // Re-check memory after each hibernation
        vmStat, _ = mem.VirtualMemory()
        if vmStat.UsedPercent < float64(s.cfg.Hibernation.MemoryHysteresisPct) { break }
    }
}
```

Sort order for `activeSessionsSortedByIdleTime()`: idle time descending (oldest idle
first); tie-break by session creation time (oldest first).

**Acceptance criteria:**
- When simulated memory usage > 85%, the sweeper hibernates the oldest-idle `Active`
  session.
- Hibernation stops once memory drops below 75% (hysteresis).
- `gopsutil` read failure is logged and skipped gracefully; no panic.

---

### Story 4.7 — Frontend: Hibernate/Resume UI

#### Task 4.7.1 — Add `onHibernate` callback to `SessionActionsOverflow`

**File:** `web-app/src/components/sessions/SessionActionsOverflow.tsx`

**What to change:**
- Add `onHibernate?: () => void` to `SessionActionsOverflowProps`.
- Add menu item (after the Pause item):
  ```tsx
  {isActive && onHibernate && (
    <button role="menuitem" onClick={() => { close(); onHibernate(); }}>
      Hibernate
    </button>
  )}
  ```
- Where `isActive` is `session.status === SessionStatus.ACTIVE` (or RUNNING for compat).
- No confirmation dialog needed (non-destructive; checkpoint saved before kill).

**Acceptance criteria:**
- "Hibernate" appears in the overflow menu for `Active` sessions when `onHibernate` is provided.
- "Hibernate" does not appear for `Paused`, `Stopped`, `Creating`, or `Hibernated` sessions.
- Menu closes and `onHibernate()` is called on click.

---

#### Task 4.7.2 — Add `onResumeFromHibernation` callback

**File:** `web-app/src/components/sessions/SessionActionsOverflow.tsx`

**What to change:**
- Add `onResumeFromHibernation?: () => void` to props.
- Show "Resume" for `Hibernated` sessions (separate from existing `onResume` for `Paused`):
  ```tsx
  {isHibernated && onResumeFromHibernation && (
    <button role="menuitem" onClick={() => { close(); onResumeFromHibernation(); }}>
      Resume
    </button>
  )}
  ```
  Where `isHibernated = session.status === SessionStatus.HIBERNATED`.

**Acceptance criteria:**
- "Resume" appears for `Hibernated` sessions.
- Clicking "Resume" calls `onResumeFromHibernation()`.
- The existing "Resume" for `Paused` sessions is unchanged.

---

#### Task 4.7.3 — Add `hibernateSession` and `resumeHibernatedSession` to `useSessionService`

**File:** `web-app/src/lib/hooks/useSessionService.ts`

**What to change:**
```typescript
const hibernateSession = useCallback(
  async (id: string): Promise<Session | null> => {
    try {
      const response = await client.hibernateSession({ id });
      if (response.session) dispatch(upsertSession(response.session));
      return response.session ?? null;
    } catch (err) {
      dispatch(setSessionError(err));
      return null;
    }
  },
  [client, dispatch]
);

const resumeHibernatedSession = useCallback(
  async (id: string): Promise<Session | null> => {
    try {
      const response = await client.resumeHibernatedSession({ id });
      if (response.session) dispatch(upsertSession(response.session));
      return response.session ?? null;
    } catch (err) {
      dispatch(setSessionError(err));
      return null;
    }
  },
  [client, dispatch]
);
```

Return both from the hook.

**Acceptance criteria:**
- `hibernateSession(id)` calls the `HibernateSession` RPC and updates Redux store.
- `resumeHibernatedSession(id)` calls the `ResumeHibernatedSession` RPC and updates
  Redux store.

---

#### Task 4.7.4 — Wire callbacks into `SessionList` (or wherever rows/cards are rendered)

**File:** The parent component that renders `SessionRow` / `SessionCard` (likely
`web-app/src/components/sessions/SessionList.tsx` or `Sessions.tsx`).

**What to change:**
Pass callbacks to each row/card:
```tsx
onHibernate={() => sessionService.hibernateSession(session.id)}
onResumeFromHibernation={() => sessionService.resumeHibernatedSession(session.id)}
```

**Acceptance criteria:**
- Right-clicking an `Active` session shows "Hibernate" in the context menu.
- Right-clicking a `Hibernated` session shows "Resume".
- Both callbacks update the session status in the UI without a page reload.

---

### Story 4.8 — Hibernated Badge

#### Task 4.8.1 — Distinct visual badge for Hibernated sessions

The CSS changes from Story 1.5.2 cover the status dot/badge color. This task ensures
an icon is shown.

**File:** `web-app/src/components/sessions/SessionRow.tsx`,
`web-app/src/components/sessions/SessionCard.tsx`

**What to change:**
- For `SessionStatus.HIBERNATED`, render a snowflake icon (❄ or SVG) next to the
  status label.
- Use a `data-status="hibernated"` attribute on the row's status element for CSS targeting.

**Acceptance criteria:**
- Hibernated sessions show a visually distinct icon (not just a color change).
- Icon is accessible (has `aria-label="Hibernated"`).

---

### Story 4.9 — Checkpoint Cleanup

#### Task 4.9.1 — Delete checkpoints when session is deleted

**File:** `server/services/session_service.go` (DeleteSession handler)

**What to change:**
After deleting the session record from storage, delete the checkpoint directory:
```go
checkpointDir, err := s.cfg.HibernationCheckpointDirOrDefault()
if err == nil {
    checkpointPath := filepath.Join(checkpointDir, session.UUID)
    if err := os.RemoveAll(checkpointPath); err != nil && !os.IsNotExist(err) {
        log.Warn("failed to clean up checkpoint on delete", "session", session.Title, "err", err)
    }
}
```

**Acceptance criteria:**
- Deleting a `Hibernated` session removes `~/.stapler-squad/checkpoints/<uuid>/`.
- Deleting a non-hibernated session does not error even if no checkpoint directory exists.

---

#### Task 4.9.2 — Retention period pruning

**File:** `session/hibernation_sweeper.go`

**What to change:**
Add `pruneStaleCheckpoints(checkpointDir string, retentionDays int)` called from
`sweep()`:
- Walk `checkpointDir/*/checkpoint.json`.
- Parse `checkpoint.json` timestamp.
- If older than `retentionDays` and no matching live session exists, delete the directory.

**Acceptance criteria:**
- Checkpoint directories older than `RetentionDays` are pruned.
- Directories for sessions that still exist in storage are never pruned.

---

### Story 4.10 — Hibernation Tests

#### Task 4.10.1 — Unit tests for `Hibernate()` and `ResumeFromHibernation()`

**File:** `session/hibernate_test.go` (new)

**Test cases:**
1. `TestHibernateActiveSession` — happy path: Active → Hibernated, checkpoint written,
   tmux killed.
2. `TestHibernateNonActiveSession` — error path: returns error for Paused/Stopped/Creating.
3. `TestResumeHibernatedSession` — happy path: Hibernated → Active, checkpoint removed.
4. `TestResumeNonHibernatedSession` — error path.
5. `TestHibernatedSessionNotAutoStartedOnDeserialize` — regression guard from pitfalls.md.
6. `TestHealthCheckerSkipsHibernatedSessions` — regression guard from pitfalls.md.

**Acceptance criteria:**
- `make test` for `session/` package passes.
- Each test case matches the template in `research/pitfalls.md` § 5b.

---

#### Task 4.10.2 — Integration test: hibernate → server restart → manual resume

**File:** `session/session_restart_test.go` (extend existing file)

**What to change:**
Add `TestHibernationLifecycle`:
1. Start session, verify `Active`.
2. Call `Hibernate()`, verify `Hibernated`, `TmuxAlive() == false`.
3. Serialize to `InstanceData`, deserialize via `FromInstanceData()`.
4. Verify still `Hibernated`, not auto-started.
5. Call `ResumeFromHibernation()`, verify `Active`, `TmuxAlive() == true`.

**Acceptance criteria:**
- Test passes with the pinned tmux: `make test-with-pinned-tmux`.

---

## Cross-Epic Acceptance Criteria Checklist

These are the final gates from `requirements.md`, mapped to epics:

| Criterion | Epic |
|---|---|
| State machine has exactly 5 lifecycle states | Epic 1 |
| `Running \|\| Ready` guards gone from codebase | Epic 1 |
| `NeedsApproval` not a lifecycle state; sub-status shown | Epics 1 + 3 |
| `CreateSession` returns immediately; async transition to `Active` | Epic 2 |
| Sessions list shows sub-status chip for `Active` sessions | Epic 3 |
| Session idle 2h is automatically hibernated | Epic 4 |
| Checkpoint files exist at `~/.stapler-squad/checkpoints/<id>/` | Epic 4 |
| Resuming re-launches process and shows saved scrollback | Epic 4 |
| Hibernated sessions are NOT auto-resumed on server restart | Epic 1 (Tasks 1.2.1–1.2.3) |
| No regressions in existing create/stop/delete/pause flows | All epics |
