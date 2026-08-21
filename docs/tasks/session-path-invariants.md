# Implementation Plan: Session Path Invariants and Info Tab UX

**Feature**: Fix WorkingDir invariant for worktree sessions and clean up the Info tab path display
**Date**: 2026-04-22
**Status**: Ready for implementation

---

## Epic Overview

### User Value

When a developer opens the Info tab on a worktree session, they see `WorkingDir` pointing at the
original repo root — not the worktree where the process actually runs. This creates confusion when
diagnosing cold-restore failures or debugging "why did it start in the wrong place?". Worse, if the
server restarts and uses the stale `WorkingDir` to resolve the start path, the session may launch in
the wrong directory.

This feature corrects the invariant at its source, sanitizes stored data for already-broken
sessions, and redesigns the Info tab path section so each field is unambiguous and non-redundant.

### Success Metrics

1. After creating a `New Worktree` session, `instance.WorkingDir` equals the worktree path (or is
   empty), never the repo root — verified by unit test
2. Cold-restoring a `New Worktree` session after server restart launches the process in the worktree
   directory — verified by existing cold-restore integration test (no change required once the
   invariant is fixed at source)
3. The Info tab shows exactly one "where the process runs" path, clearly labeled, for all session
   types — verified by Playwright smoke test
4. Loading a legacy session with a stale `WorkingDir` no longer surfaces the wrong path in the UI —
   migration fires on `FromInstanceData`

### Scope

**Included:**
- Fix `WorkingDir` population for `SessionTypeNewWorktree` and `SessionTypeExistingWorktree` at
  session creation (first-time setup path in `start()`)
- Add data migration in `FromInstanceData` to clear stale `WorkingDir` for worktree sessions whose
  stored value equals their repo root (not the worktree)
- Redesign the Info tab path section: group paths by purpose, remove redundant fields, add clear
  labels per session type
- Unit tests for the invariant and the migration

**Excluded:**
- Editable `WorkingDir` field in the UI (deferred — see Architecture Decision below)
- Path-completion UI (omni-bar feature tracked separately)
- `ExistingWorktree` migration that requires on-disk stat to detect (too risky; handle with a
  weaker migration that clears only provably wrong values)
- Any changes to `Path` (the workspace repo root field) or `GitWorktree.*` — those are correct

---

## Architecture Decisions

### AD-1: WorkingDir Semantics — "Where the Process Runs"

**Decision**: `WorkingDir` is redefined to mean "the directory in which the session process
starts". For worktree sessions, this is the worktree path. For directory sessions, this is the
user-specified path.

**Rationale**: `GetEffectiveRootDir()` and `resolveStartPath()` already implement this semantic at
runtime. The only problem is that `WorkingDir` is *not set* (or is set wrong) at creation time for
worktree sessions. Aligning the stored field with the runtime behavior eliminates the
`GetEffectiveRootDir()` override path in the display layer and makes the stored state
self-consistent.

**Consequences**:
- For `SessionTypeDirectory`: behavior unchanged. `WorkingDir` remains the user-supplied path (may
  be an absolute path or a relative path within `Path`).
- For `SessionTypeNewWorktree`: after `gitManager.Setup()` completes and the worktree path is
  known, `WorkingDir` is set to `gitManager.GetWorktreePath()`.
- For `SessionTypeExistingWorktree`: `WorkingDir` is set to `ExistingWorktree` at session
  creation (it is already known before `start()` is called).
- `resolveStartPath()` remains correct as-is. It already falls back to `basePath` when `WorkingDir`
  is empty or non-existent, so setting `WorkingDir` to the worktree path is consistent with its
  current logic.
- `GetEffectiveRootDir()` remains correct as-is and continues to be the canonical runtime
  authority. The goal is for `WorkingDir` to agree with `GetEffectiveRootDir()` after this fix.

**Rejected alternatives**:
- *Remove `WorkingDir` for worktree sessions and always derive it at runtime*: would require
  changing the proto, removing the stored field, and updating all callers. Over-engineered for the
  problem.
- *Keep `WorkingDir` as "the repo the user originally pointed at" and rename the field*: confusing
  because callers already use `WorkingDir` as the process start directory.

### AD-2: Migration Strategy — Clear-If-Wrong, Not Rewrite-Blind

**Decision**: In `FromInstanceData`, detect and clear `WorkingDir` values that are provably wrong
for worktree sessions (i.e., `WorkingDir == Path` and `SessionType` is a worktree type and a
worktree path is stored). Do not attempt to set `WorkingDir` to the correct worktree path during
migration — that risks setting it to a path that no longer exists on disk.

**Rationale**: A cleared `WorkingDir` is safe: `resolveStartPath()` falls back to
`GetEffectiveRootDir()` (the worktree path) when `WorkingDir` is empty. A wrong `WorkingDir` that
resolves to a path that exists (e.g., the original repo root) will silently launch the process in
the wrong location. Clearing is the conservative-correct action.

**Migration trigger**: The condition `WorkingDir == Path && isWorktreeSession &&
gitWorktreeDataIsPopulated` is a reliable signal that the bug occurred, because a correctly
configured worktree session would have `WorkingDir == worktreePath`, not `WorkingDir == repoRoot`.

### AD-3: Editable WorkingDir — Deferred for Directory Sessions

**Decision**: Do not add an editable `WorkingDir` field to the Info tab at this time.

**Rationale**: The use cases are weak:

1. *"I want to change where a running session starts on the next restart"*: the session would need
   to be stopped and a new session created. Editing `WorkingDir` without restarting achieves
   nothing observable.
2. *"I accidentally typed the wrong directory when creating the session"*: already handled — the
   creation dialog validates that the path exists. If the worktree was deleted, the session goes
   Paused and the correct fix is to resume via workspace switcher, not to edit a stored path.
3. *"I want to see what directory is in use and adjust it"*: for worktree sessions, the process
   runs in the worktree; there is nothing to adjust. For directory sessions, the path is set at
   creation and the session is already running there.

The cost of an editable path field is non-trivial: it requires an `UpdateSession` RPC field, a
migration from in-memory state (the running session) to storage, and filesystem validation. The
value delivered is marginal. Re-evaluate when a concrete user report motivates it.

### AD-4: Info Tab Path Display — One Path Per Concern

**Decision**: Replace the current flat dump of all path-like fields with a structured display that
shows exactly what the user needs to understand where the session is operating.

**For directory sessions**: Show one "Working Directory" row with `WorkingDir` (the process start
path). Do not show `path` (workspace root) — it is the same value and adds nothing.

**For worktree sessions**: Show two rows:
- "Repo Root" — `session.path` (the main git checkout, context for the worktree)
- "Worktree" — `session.gitWorktree.worktreePath` (where the process runs)

Do not show `workingDir` for worktree sessions — it duplicates `worktreePath` after this fix and
adds noise.

**Redundancy removal**: `gitWorktree.repoPath` duplicates `session.path` for managed sessions.
Remove `Repo Path` row from the worktree sub-section; it is already covered by the top-level "Repo
Root" row.

**Retained technical fields** (in a collapsed "Advanced" section or moved lower): `launchCommand`,
`tmuxPrefix`, `historyFilePath`, `claudeSession.*`, `clonedRepoPath`. These are diagnostics, not
operational information.

---

## Dependency Visualization

```
Story 1: Fix Invariant at Creation              Story 2: Migration on Load
====================================            ==========================
[1.1] Set WorkingDir in firstTimeSetup          [2.1] Migration in FromInstanceData
      |                                                |
[1.2] Unit test for invariant                  [2.2] Unit test for migration
      |                                                |
      +--------- both independent ----------------+
                                                  |
                              Story 3: Info Tab UX (no backend dep)
                              ====================================
                              [3.1] Redesign path section
                              [3.2] Smoke test
```

Stories 1 and 2 are independent and can run in parallel.
Story 3 is purely frontend and has no dependency on Stories 1 or 2, but benefits from them being
complete so the displayed data is correct during manual testing.

---

## Story 1: Fix WorkingDir at Session Creation

**User value**: New worktree sessions store the correct process start directory, so cold restores
and the Info tab are both accurate from day one.

**Acceptance criteria**:
- `Instance.WorkingDir` for a `SessionTypeNewWorktree` session equals `gitManager.GetWorktreePath()`
  after the first-time setup path in `start()` completes
- `Instance.WorkingDir` for a `SessionTypeExistingWorktree` session equals `ExistingWorktree` at
  the point `start()` is called
- `Instance.WorkingDir` for a `SessionTypeDirectory` session is unchanged
- `go test -race ./session/... -run TestWorkingDirInvariant` passes

---

### Task 1.1: Set WorkingDir After Worktree Setup [Small]

**Objective**: In the `firstTimeSetup` branch of `Instance.start()`, set `i.WorkingDir` to the
worktree path immediately after `gitManager.Setup()` succeeds.

**Context boundary**:
- Primary: `session/instance.go` lines 960–985 (the `firstTimeSetup` branch)
- Supporting: `session/instance.go` `resolveStartPath()` lines 1088–1103, `GetEffectiveRootDir()`
  lines 1105–1115
- Total context: ~80 lines

**Prerequisites**:
- Read `session/instance.go:961-985` (the `firstTimeSetup` block shown below for reference):
  ```go
  } else {
      basePath := i.Path
      if i.gitManager.HasWorktree() {
          if err := i.gitManager.Setup(); err != nil { ... }
          basePath = i.gitManager.GetWorktreePath()
      }
      startPath := i.resolveStartPath(basePath)
      ...
  }
  ```
- Understand that `basePath` is already correct after `Setup()` — we just need `WorkingDir` to
  agree with it for the worktree case

**Implementation approach**:

After `basePath = i.gitManager.GetWorktreePath()` and before `resolveStartPath`, add:
```go
// Invariant: WorkingDir must reflect where the process runs.
// For worktree sessions, this is the worktree path, not the original repo root.
if i.SessionType == SessionTypeNewWorktree || i.SessionType == SessionTypeExistingWorktree {
    i.WorkingDir = basePath
}
```

For `SessionTypeExistingWorktree` sessions that do not go through `gitManager.Setup()` (i.e., they
reuse an existing worktree without calling Setup), verify whether `HasWorktree()` is true at that
point. If not, set `WorkingDir = i.ExistingWorktree` at the top of the `firstTimeSetup` block
before the `hasWorktree` check.

To be safe, handle both cases explicitly:
```go
} else { // firstTimeSetup
    basePath := i.Path
    if i.gitManager.HasWorktree() {
        if err := i.gitManager.Setup(); err != nil { ... }
        basePath = i.gitManager.GetWorktreePath()
        // Fix invariant: store where the process will actually run
        i.WorkingDir = basePath
    } else if i.SessionType == SessionTypeExistingWorktree && i.ExistingWorktree != "" {
        // ExistingWorktree sessions whose worktree isn't tracked by gitManager yet
        i.WorkingDir = i.ExistingWorktree
    }
    startPath := i.resolveStartPath(basePath)
    ...
}
```

**Validation**:
- `go build ./...` succeeds
- `go test -race ./session/... -run TestWorkingDir` passes (see Task 1.2)
- Manual: create a `New Worktree` session, open Info tab, verify `Working Directory` shows the
  worktree path (not the repo root)

**INVEST check**: Independent ✓ | Negotiable (exact placement in start()) ✓ | Valuable (root cause
fix) ✓ | Small (< 10 LOC) ✓ | Testable (unit test) ✓

---

### Task 1.2: Unit Test for WorkingDir Invariant [Small]

**Objective**: Write a unit test that verifies `WorkingDir` is set to the worktree path (not the
repo root) for `New Worktree` and `Existing Worktree` session types after `start()`.

**Context boundary**:
- Primary: `session/instance_worktree_test.go` (new file or add to existing `instance_test.go`)
- Supporting: existing mock patterns for `gitManager`, `tmuxManager` in `session/` test files
- Total context: ~150 lines

**Prerequisites**:
- Completion of Task 1.1
- Locate existing test mocks: `grep -r "MockGitManager\|FakeGit\|mockGit" session/` to find the
  test doubles that already exist
- Understand the `GitWorktreeManager` interface methods used by `start()`

**Test cases**:

1. `TestWorkingDirInvariant_NewWorktree`: Create instance with `SessionType = SessionTypeNewWorktree`,
   `Path = "/repo"`, `WorkingDir = "/repo"` (simulating the stale state). After calling `start()`,
   assert `instance.WorkingDir == "/repo/worktrees/branch-name"` (the worktree path returned by
   the mock).

2. `TestWorkingDirInvariant_ExistingWorktree`: Create instance with
   `SessionType = SessionTypeExistingWorktree`, `ExistingWorktree = "/repo/worktrees/existing"`.
   After `start()`, assert `instance.WorkingDir == "/repo/worktrees/existing"`.

3. `TestWorkingDirInvariant_Directory`: Create instance with `SessionType = SessionTypeDirectory`,
   `WorkingDir = "/some/dir"`. After `start()`, assert `WorkingDir` unchanged.

**Validation**:
- `go test -race ./session/... -run TestWorkingDirInvariant` passes all 3 cases
- Success criteria: tests compile, all 3 cases exercise the new branch

---

## Story 2: Migration for Existing Sessions

**User value**: Sessions created before this fix are automatically corrected on load — the wrong
`WorkingDir` value is cleared so cold restores use the correct worktree path.

**Acceptance criteria**:
- A session loaded via `FromInstanceData` with `SessionType = SessionTypeNewWorktree`,
  `WorkingDir == Path` (the stale value), and populated `Worktree.WorktreePath` has `WorkingDir`
  cleared to `""` after loading
- A session with a correct `WorkingDir` (already set to the worktree path) is not modified
- A `SessionTypeDirectory` session is never affected by the migration
- `go test -race ./session/... -run TestWorkingDirMigration` passes

---

### Task 2.1: Migration in FromInstanceData [Small]

**Objective**: Add a migration guard in `FromInstanceData` that clears `WorkingDir` when it can
be proven to be wrong for worktree sessions.

**Context boundary**:
- Primary: `session/instance.go` `FromInstanceData` lines 346–500 (specifically the migration
  block area around lines 347–383)
- Total context: ~60 lines in the function

**Prerequisites**:
- Read `session/instance.go:346-383` (existing migration block — corrupted path fix and category
  migration are already here; add below those)
- Understand the three-field pattern: `data.SessionType`, `data.WorkingDir`, `data.Path`,
  `data.Worktree.WorktreePath`

**Implementation approach**:

Add after the existing migrations, before constructing the `instance` struct literal:

```go
// MIGRATION: Fix stale WorkingDir for worktree sessions.
// Before this fix, worktree sessions stored WorkingDir = the repo root (data.Path)
// instead of the worktree path. This caused cold restores to start in the wrong directory.
// Condition: worktree session + WorkingDir equals the repo root + worktree path is stored.
// Action: clear WorkingDir so resolveStartPath() falls back to GetEffectiveRootDir() (the worktree).
migratedWorkingDir := data.WorkingDir
isWorktreeSession := data.SessionType == SessionTypeNewWorktree ||
    data.SessionType == SessionTypeExistingWorktree
if isWorktreeSession &&
    data.WorkingDir == data.Path &&
    data.Worktree.WorktreePath != "" &&
    data.WorkingDir != data.Worktree.WorktreePath {
    log.WarningLog.Printf(
        "Migrating stale WorkingDir for worktree session '%s': clearing '%s' (was repo root, not worktree)",
        data.Title, data.WorkingDir,
    )
    migratedWorkingDir = ""
}
```

Then use `migratedWorkingDir` in the `Instance` struct literal instead of `data.WorkingDir`.

**Edge cases to handle**:
- `data.Worktree.WorktreePath == ""`: no migration (worktree path not yet known; leave WorkingDir
  as-is and let `resolveStartPath` handle the fallback)
- `data.WorkingDir == data.Worktree.WorktreePath`: already correct; no migration needed
- `data.WorkingDir == ""`: already empty; no migration needed
- `data.SessionType == SessionTypeDirectory`: not a worktree session; skip entirely

**Validation**:
- `go test -race ./session/... -run TestWorkingDirMigration` passes (see Task 2.2)
- `go build ./...` succeeds
- Manual: load a hand-crafted `InstanceData` with the stale value; verify `WorkingDir` is cleared

---

### Task 2.2: Unit Test for Migration [Small]

**Objective**: Test `FromInstanceData` with stale `WorkingDir` values to verify migration fires or
does not fire correctly for each case.

**Context boundary**:
- Primary: `session/instance_test.go` or `session/migration_test.go`
- Total context: ~100 lines

**Test cases**:

1. `TestWorkingDirMigration_ClearsStaleValue`: `InstanceData` with `SessionType = New Worktree`,
   `Path = "/repo"`, `WorkingDir = "/repo"`, `Worktree.WorktreePath = "/repo/worktrees/foo"`.
   After `FromInstanceData`, assert `instance.WorkingDir == ""`.

2. `TestWorkingDirMigration_DoesNotTouchCorrect`: Same setup but `WorkingDir =
   "/repo/worktrees/foo"`. After `FromInstanceData`, assert `WorkingDir` unchanged.

3. `TestWorkingDirMigration_SkipsDirectory`: `SessionType = Directory`, `WorkingDir = "/some/dir"`.
   Assert `WorkingDir` unchanged.

4. `TestWorkingDirMigration_NoWorktreeData`: Worktree session but `Worktree.WorktreePath = ""`.
   Assert `WorkingDir` unchanged (migration does not fire without worktree data).

**Validation**:
- All 4 cases pass
- `-race` clean
- Success criteria: migration test file compiles, all cases exercise distinct branches

---

## Story 3: Info Tab Path Section Redesign

**User value**: The Info tab shows exactly the paths a developer needs — no duplicates, no
confusing labels — regardless of session type. A worktree session shows "Repo Root" and "Worktree"
(two paths, clearly separated). A directory session shows one "Working Directory". Advanced
diagnostics (launch command, Claude session IDs, history file) move to the bottom.

**Acceptance criteria**:
- Directory session: only one path row visible in the "Location" group, labeled "Working Directory"
- Worktree session: two path rows visible — "Repo Root" and "Worktree", no `workingDir` row, no
  `gitWorktree.repoPath` row (it duplicates "Repo Root")
- The `launchCommand`, `tmuxPrefix`, `historyFilePath`, `claudeSession` fields are still visible
  but rendered below all operational fields
- `path` (workspace effective path) is not shown for worktree sessions when it equals `gitWorktree.worktreePath`
- Manual smoke: open Info tab on both session types, confirm no redundant path rows

---

### Task 3.1: Redesign Info Tab Path Section [Medium]

**Objective**: Replace the current flat field-by-field path rendering in `SessionDetail.tsx` with
a session-type-aware path display that eliminates redundancy and uses clear labels.

**Context boundary**:
- Primary: `web-app/src/components/sessions/SessionDetail.tsx` lines 363–627 (the Info tab
  content section)
- Supporting: `web-app/src/gen/session/v1/types_pb.ts` (field names and types)
- Total context: ~265 lines of the Info tab section

**Prerequisites**:
- Read `SessionDetail.tsx:363-627` in full (the info grid block)
- Understand the current field rendering order: identity → timestamps → location → organization →
  program → Claude session → git worktree → diff stats → GitHub → external metadata
- Confirm generated TypeScript field names: `session.path`, `session.workingDir`,
  `session.gitWorktree?.worktreePath`, `session.gitWorktree?.repoPath`, `session.sessionType`

**Implementation approach**:

Replace the current location block (the `session.path` and `session.workingDir` rows at lines
405–416) with a session-type-aware component or inline logic:

```tsx
{/* Location — session-type-aware */}
{session.sessionType === SessionType.DIRECTORY ? (
  // Directory sessions: one path, the working directory
  session.workingDir && (
    <div className={styles.infoItem}>
      <span className={styles.infoLabel}>Working Directory:</span>
      <span className={styles.infoValue} style={{ fontFamily: 'monospace', wordBreak: 'break-all' }}>
        {session.workingDir}
      </span>
    </div>
  )
) : (
  // Worktree sessions (new or existing): repo root + worktree
  <>
    {session.path && (
      <div className={styles.infoItem}>
        <span className={styles.infoLabel}>Repo Root:</span>
        <span className={styles.infoValue} style={{ fontFamily: 'monospace', wordBreak: 'break-all' }}>
          {session.path}
        </span>
      </div>
    )}
    {session.gitWorktree?.worktreePath && (
      <div className={styles.infoItem}>
        <span className={styles.infoLabel}>Worktree:</span>
        <span className={styles.infoValue} style={{ fontFamily: 'monospace', wordBreak: 'break-all' }}>
          {session.gitWorktree.worktreePath}
        </span>
      </div>
    )}
  </>
)}
```

In the `gitWorktree` block (lines 510–537), remove the `gitWorktree.repoPath` row — it now
duplicates "Repo Root" above. Keep `gitWorktree.branchName` and `gitWorktree.baseCommitSha`.

Move the following fields to the bottom of the info grid (after GitHub fields), under a visual
separator or implicit grouping by proximity:
- `launchCommand`
- `tmuxPrefix`
- `historyFilePath`
- `claudeSession.sessionId` and `claudeSession.projectName`

The current order already puts these near the bottom; verify they stay below the operational fields
after the restructuring. No explicit "Advanced" accordion is required for this iteration — just
ensure ordering places diagnostics last.

**SessionType.UNSPECIFIED handling**: treat as Directory (the default session type) to avoid a
blank path section.

**Validation**:
- `npm run build` in `web-app/` succeeds
- `npx tsc --noEmit` passes (TypeScript type-check)
- Manual: open Info tab on `Directory` session — one path row labeled "Working Directory"
- Manual: open Info tab on `New Worktree` session — two path rows "Repo Root" and "Worktree", no
  raw `workingDir` row, no duplicate `Repo Path` under the worktree section

---

### Task 3.2: Playwright Smoke Test for Info Tab Paths [Small]

**Objective**: Add a Playwright test that verifies the correct path rows appear for each session
type in the Info tab.

**Context boundary**:
- Primary: `.playwright-mcp/` or `web-app/e2e/` (check existing Playwright test location:
  `ls web-app/` and `.playwright-mcp/`)
- Supporting: existing session fixture setup in the Playwright test suite

**Prerequisites**:
- Completion of Task 3.1
- Locate existing Playwright tests: check `web-app/e2e/` and `.playwright-mcp/` directories
- Understand how sessions are seeded for Playwright tests (mock API or real backend)

**Test cases**:

1. `Directory session Info tab`: navigate to a `Directory` session's Info tab, assert:
   - Row with label "Working Directory" is visible
   - No row with label "Worktree" is visible
   - No row with label "Workspace Path" is visible (the old `path` label)

2. `Worktree session Info tab`: navigate to a `New Worktree` session's Info tab, assert:
   - Row with label "Repo Root" is visible
   - Row with label "Worktree" is visible
   - No row with label "Working Directory" is visible
   - No row with label "Repo Path" is visible (the removed duplicate)

**Validation**:
- Tests pass in headed mode locally
- Success criteria: both cases pass; existing Playwright tests remain green

---

## Known Issues

### Bug 1 — Incorrect resolveStartPath Call During Cold Restore (Pre-fix) [SEVERITY: High]

**Description**: In `start(false)` (cold restore path), line 917 calls
`i.resolveStartPath(i.GetEffectiveRootDir())`. This is correct at runtime, but because
`i.WorkingDir` contains the repo root (not the worktree path) before this fix, `resolveStartPath`
resolves to the repo root. The process starts in the wrong directory.

**Status**: Fixed as a side effect of Story 1 (Task 1.1) — once `WorkingDir` is set correctly at
creation, `resolveStartPath` returns the correct directory. No separate fix needed.

**Files affected**: `session/instance.go` (start function, cold restore branch)

---

### Bug 2 — Stale WorkingDir in Persisted Storage for Existing Sessions [SEVERITY: Medium]

**Description**: Sessions created before this fix are persisted with the wrong `WorkingDir`.
Re-loading them after this fix does not automatically correct the stored value because storage is
only written when `SaveInstances` is called (on status change or periodic save).

**Mitigation**: Story 2 (Task 2.1) adds an in-memory migration in `FromInstanceData`. The corrected
value takes effect immediately on load. The next `SaveInstances` call will persist the corrected
(empty) value. No explicit migration script is needed.

**Residual risk**: If the server crashes between load (migration fires) and save (corrected value
written), the session is loaded again with the stale value — but the migration fires again. The
window for data loss is zero; the migration is idempotent.

**Files affected**: `session/instance.go` (FromInstanceData)

---

### Bug 3 — gitWorktree.repoPath Duplicates session.path in the UI [SEVERITY: Low]

**Description**: The current Info tab shows both `session.path` (labeled "Workspace Path") and
`session.gitWorktree.repoPath` (labeled "Repo Path") for worktree sessions. They contain the same
value. This confuses developers trying to understand which path matters.

**Status**: Resolved in Story 3 (Task 3.1) — the `Repo Path` row from the worktree sub-section is
removed, and `session.path` is relabeled to "Repo Root" for worktree sessions.

**Files affected**: `web-app/src/components/sessions/SessionDetail.tsx`

---

### Bug 4 — WorkingDir Shown for Worktree Sessions Even After Fix [SEVERITY: Low — future]

**Description**: After Story 1 fixes `WorkingDir` to equal `worktreePath`, the old UI code would
show both "Working Directory: /worktree" and "Worktree: /worktree" — still redundant. Story 3
addresses this by suppressing the `workingDir` row for worktree sessions entirely.

**Status**: Resolved in Story 3 (Task 3.1).

**Files affected**: `web-app/src/components/sessions/SessionDetail.tsx`

---

### Bug 5 — ExistingWorktree SessionType Has Two Code Paths for WorkingDir [SEVERITY: Low]

**Description**: `SessionTypeExistingWorktree` sessions may or may not have `gitManager.HasWorktree()
== true` at the time `start()` is called (depending on whether `Setup()` was called previously).
Task 1.1 must handle both code paths — the `HasWorktree()` branch and the fallback using
`i.ExistingWorktree`.

**Mitigation**: Task 1.1 explicitly handles both paths. The fallback to `i.ExistingWorktree` covers
the case where `gitManager` was not yet populated.

**Files affected**: `session/instance.go` (firstTimeSetup block)

---

## Integration Checkpoints

**After Story 1 (Invariant Fix)**:
- `go test -race ./session/... -run TestWorkingDirInvariant` — all 3 cases pass
- `go build ./...` succeeds
- Manual: create a `New Worktree` session, verify `WorkingDir` in Info tab equals the worktree path

**After Story 2 (Migration)**:
- `go test -race ./session/... -run TestWorkingDirMigration` — all 4 cases pass
- Manual: hand-edit `sessions.json` to set a stale `working_dir`, restart server, open Info tab,
  verify the stale path is no longer shown

**After Story 3 (Info Tab UX)**:
- `npm run build` succeeds
- `npx tsc --noEmit` passes
- Manual: `Directory` session Info tab shows exactly one path row labeled "Working Directory"
- Manual: `New Worktree` session Info tab shows "Repo Root" and "Worktree" with no duplicates
- Playwright smoke test passes

**Final acceptance**:
- `go test -race ./...` passes
- `go build ./...` succeeds
- All manual smoke tests pass for both session types
- No new `TODO` or stub code committed

---

## Context Preparation Guide

### Tasks 1.1 and 1.2
- Load: `session/instance.go` lines 845–1003 (start() body, all branches)
- Load: `session/instance.go` lines 1088–1125 (resolveStartPath, GetEffectiveRootDir, Workspace)
- Load: `session/types.go` lines 326–337 (Workspace type)
- Concepts: firstTimeSetup vs cold restore vs hot restore branching; gitManager.Setup() side
  effects; how resolveStartPath uses WorkingDir

### Tasks 2.1 and 2.2
- Load: `session/instance.go` lines 346–500 (FromInstanceData, including existing migrations)
- Load: `session/storage.go` lines 1–115 (InstanceData struct)
- Concepts: the three existing migration patterns (path corruption, tilde expansion, category→tags);
  idempotency requirements

### Tasks 3.1 and 3.2
- Load: `web-app/src/components/sessions/SessionDetail.tsx` lines 363–627 (Info tab content)
- Load: `web-app/src/gen/session/v1/types_pb.ts` (Session type, SessionType enum, GitWorktree type)
- Concepts: `SessionType.DIRECTORY`, `SessionType.NEW_WORKTREE`, `SessionType.EXISTING_WORKTREE`
  enum values; vanilla-extract CSS colocated `.css.ts` pattern; the existing info grid structure

---

## Success Criteria

- [ ] `go test -race ./session/... -run TestWorkingDirInvariant` passes (3 cases)
- [ ] `go test -race ./session/... -run TestWorkingDirMigration` passes (4 cases)
- [ ] `go build ./...` succeeds
- [ ] `npm run build` in `web-app/` succeeds
- [ ] `npx tsc --noEmit` passes
- [ ] Manual: `New Worktree` session Info tab shows "Repo Root" + "Worktree", no "Working Directory"
- [ ] Manual: `Directory` session Info tab shows exactly one path row labeled "Working Directory"
- [ ] Manual: no duplicate "Repo Path" / "Repo Root" rows in worktree sessions
- [ ] No cold restore regression: existing cold restore integration tests remain passing
- [ ] No new `TODO` or stub code committed
