# Implementation Plan: Session Resumption Hardening

**Feature**: Close Phase 1 gaps + deliver Checkpoint Web UI
**Date**: 2026-04-15
**Status**: Ready for implementation
**Research**: `project_plans/session-resumption-hardening/research/synthesis.md`
**Binding ADRs**: `project_plans/session-resumption/decisions/ADR-001` (Session Identity), `ADR-002` (Two-Tier Resume), `ADR-003` (Checkpoint Storage)

---

## Epic Overview

### User Value

A developer running multiple Claude Code sessions restarts their machine. On next launch, every session resumes with the exact Claude conversation — no re-explaining context, no lost work. Separately, any running session can be bookmarked with one click, creating a named restore point visible in the UI.

### Success Metrics

1. After `tmux kill-server` + stapler-squad restart: sessions with a saved UUID start with `--resume <uuid>` — verified by integration test
2. Checkpoint creation succeeds with accurate `scrollbackSeq > 0` — verified by unit test
3. `http://localhost:8543` session cards show `ClaudeSession.SessionId` and `HistoryFilePath` — verified by manual smoke test
4. Checkpoint button visible on session cards; checkpoint list shows label + time + SHA pills — verified by Playwright smoke test

### Scope

**Included:**
- Cold restore branch in `Instance.start(false)` when tmux is dead and UUID is known
- `LatestSequence` on scrollback storage; callers updated to pass real sequence
- `HistoryFilePath` wired into proto adapter
- Checkpoint creation button + list web UI

**Excluded:**
- Fork from checkpoint web UI button (backend done; UI deferred)
- Phase 3: OSC 7, VT snapshots, read-only discovery
- Adopted bridge improvements

### Constraints

- Backward compatible: existing `sessions.json` loads without migration
- CI must stay green: no new unconditional test failures
- macOS-first: cold restore test skips gracefully if tmux not available (`testing.Short()` or `exec.LookPath("tmux")`)
- No new external services or dependencies

---

## Architecture Decisions

| ADR | Decision |
|---|---|
| `project_plans/session-resumption/decisions/ADR-002-two-tier-resume-strategy.md` | Hot restore (tmux alive) takes priority; cold restore (dead tmux + UUID) uses `--resume`; fresh start is last fallback |
| `project_plans/session-resumption/decisions/ADR-003-checkpoint-storage-model.md` | Checkpoints stored as `[]Checkpoint` on Instance, persisted in sessions.json with `omitempty` |

No new ADRs required — this iteration closes implementation gaps against existing binding decisions.

---

## Dependency Visualization

```
Story 1: Cold Restore Fix          Story 2: Data Surface             Story 3: Checkpoint UI
==========================         ======================             ======================
[1.1] cold restore branch          [2.1] LatestSequence method        [3.1] useSessionService hook
      |                                   |                                   |
[1.2] cold restore test            [2.2] caller updates               [3.2] CheckpointButton component
      |                                   |                                   |
      |                            [2.3] HistoryFilePath adapter       [3.3] CheckpointList component
      |                                   |
      +----- can run in parallel ---------+
                                          |
                               Story 2 completion unblocks
                               accurate fork operations (later)
```

Stories 1 and 2 are independent and can run in parallel.
Story 3 depends only on existing RPCs (already built — no Story 1/2 dependency).

---

## Story 1: Cold Restore Path

**User value**: Sessions with a saved conversation UUID are automatically resumed with `--resume` when tmux is dead after a reboot or kill.

**Acceptance criteria**:
- `Instance.start(false)` with `claudeSession.SessionID != ""` and dead tmux → new tmux session created with `--resume <uuid>` in the program command
- `Instance.start(false)` with no UUID and dead tmux → new tmux session created without `--resume` (existing behavior preserved)
- `Instance.start(false)` with alive tmux → existing session restored via `RestoreWithWorkDir` (hot restore path unchanged)
- `go test -race ./session/...` passes
- Integration test skips cleanly if tmux not installed

---

### Task 1.1: Cold Restore Branch in start() [Medium — 3h]

**Objective**: Add the dead-tmux detection and relaunch-with-resume branch to `Instance.start()`.

**Context boundary**:
- Primary: `session/instance.go` (lines 762–879, ~120 lines of the `start()` method)
- Supporting: `session/instance.go` `HasClaudeSession()` (line 1918), `resolveStartPath()` (line 944), `initTmuxSession()` (line 1381–1439)
- Total context: ~250 lines

**Prerequisites**:
- Read `session/instance.go:808-818` (current `!firstTimeSetup` path)
- Understand `tmuxManager.DoesSessionExist()`, `tmuxManager.Start()`, `tmuxManager.RestoreWithWorkDir()`
- Understand that `initTmuxSession()` → `ClaudeCommandBuilder` already adds `--resume` when `claudeSession.SessionID != ""`

**Implementation approach**:

1. In `session/instance.go:start()`, before the `if !firstTimeSetup` block at line 808, no change needed.

2. Replace the `!firstTimeSetup` block (lines 808-818) with:

```go
if !firstTimeSetup {
    if !i.tmuxManager.DoesSessionExist() {
        if i.HasClaudeSession() {
            // Cold restore: tmux is dead but we have a conversation UUID.
            // resolveStartPath falls back gracefully if WorkingDir was not captured.
            startPath := i.resolveStartPath(i.Path)
            log.InfoLog.Printf("Cold restoring '%s' with --resume %s in %s",
                i.Title, i.claudeSession.SessionID, startPath)
            if err := i.tmuxManager.Start(startPath); err != nil {
                setupErr = fmt.Errorf("cold restore Start failed: %w", err)
                return setupErr
            }
            // Attach PTY (same pattern as firstTimeSetup path).
            _ = i.tmuxManager.RestoreWithWorkDir(startPath)
        } else {
            // Dead tmux, no UUID — start fresh session (no --resume).
            startPath := i.resolveStartPath(i.Path)
            log.WarningLog.Printf("Cold start '%s': tmux dead, no conversation UUID, starting fresh in %s",
                i.Title, startPath)
            if err := i.tmuxManager.Start(startPath); err != nil {
                setupErr = fmt.Errorf("cold start failed: %w", err)
                return setupErr
            }
            _ = i.tmuxManager.RestoreWithWorkDir(startPath)
        }
    } else {
        // Hot restore: tmux session is alive — attach to it.
        workDir := i.Path
        if i.gitManager.HasWorktree() {
            workDir = i.gitManager.GetWorktreePath()
        }
        log.InfoLog.Printf("Restoring existing tmux session for instance '%s' with workDir '%s'", i.Title, workDir)
        if err := i.tmuxManager.RestoreWithWorkDir(workDir); err != nil {
            setupErr = fmt.Errorf("failed to restore existing session: %w", err)
            return setupErr
        }
        log.InfoLog.Printf("Successfully restored tmux session for instance '%s'", i.Title)
    }
}
```

3. No changes needed to `initTmuxSession()` or `ClaudeCommandBuilder` — `--resume` is already added when `claudeSession.SessionID != ""`.

**Validation**:
- Unit: mock `tmuxManager.DoesSessionExist() = false`, `HasClaudeSession() = true` → verify `tmuxManager.Start()` called with expected startPath; verify program contains `--resume <uuid>`
- Integration: see Task 1.2
- Success criteria: `go test -race ./session/... -run TestStart` passes with no race

**INVEST check**: Independent (no external state changes) ✓ | Negotiable (exact log messages) ✓ | Valuable (unblocks zero-conversation-loss) ✓ | Estimable (3h) ✓ | Small (single method, ~30 LOC) ✓ | Testable (mock tmux) ✓

---

### Task 1.2: Cold Restore Integration Test [Medium — 3h]

**Objective**: Write `session/instance_cold_restore_test.go` with 3 test cases covering hot restore, cold restore with UUID, and cold restore without UUID.

**Context boundary**:
- Primary: `session/instance_cold_restore_test.go` (new file)
- Supporting: `session/instance.go` (Start, HasClaudeSession), `session/tmux/mock_manager.go` (if exists) or use real tmux with skip guard
- Total context: ~300 lines (new file + instance interface)

**Prerequisites**:
- Completion of Task 1.1
- Check whether a `MockTmuxManager` exists: `grep -r "MockTmux\|mock.*tmux" session/` — if yes, use it; if no, use `exec.LookPath("tmux")` guard + real tmux session

**Implementation approach**:

1. Add file header with build tag guard:
```go
//go:build !short
// Skip in -short mode (no tmux required).
```

2. `TestColdRestore_WithUUID`: Create instance with `claudeSession.SessionID = "test-uuid-1234"`. Do NOT create tmux session. Call `Start(false)`. Verify:
   - No error returned
   - `tmuxManager.DoesSessionExist()` returns true after Start
   - Session is in Running status

3. `TestColdRestore_WithoutUUID`: Instance with no claudeSession. Dead tmux. `Start(false)`. Verify no error, session running, no `--resume` in command.

4. `TestHotRestore_ExistingSession`: Instance with alive tmux session. `Start(false)`. Verify existing session reattached (not a new session created).

5. Add `TestMain` with `exec.LookPath("tmux")` check — skip all tests in this file if tmux not available (e.g., CI without tmux).

**Validation**:
- All 3 tests pass locally with `go test ./session/ -run TestColdRestore -v`
- Tests skip cleanly when run with `go test -short ./session/`
- `go test -race ./session/ -run TestColdRestore` passes (no races)
- Success criteria: test file compiles, all 3 cases exercise the new branch

**INVEST check**: Independent (tests Task 1.1 only) ✓ | Negotiable (test implementation details) ✓ | Valuable (only CI guard for cold restore) ✓ | Estimable (3h) ✓ | Small (single file) ✓ | Testable (self-contained) ✓

---

## Story 2: Data Surface

**User value**: Checkpoint creation records accurate scrollback sequence numbers (enabling correct fork truncation later). Session cards in the web UI display the linked JSONL file path.

**Acceptance criteria**:
- `CreateCheckpoint` RPC returns a checkpoint with `scrollback_seq > 0` for a session with active scrollback
- `Instance.HistoryFilePath` value appears in `ClaudeSession` or as a top-level proto field visible to web UI
- No existing tests broken

---

### Task 2.1: LatestSequence on Scrollback Storage [Small — 2h]

**Objective**: Add `LatestSequence(sessionID string) (uint64, error)` to `FileScrollbackStorage` and the `ScrollbackStorage` interface.

**Context boundary**:
- Primary: `session/scrollback/storage.go` (getFileLock at line 90, Write at 118, Read at 186)
- Supporting: `session/scrollback/storage.go` interface definition (check for `ScrollbackStorage` interface)
- Total context: ~200 lines

**Prerequisites**:
- Read `session/scrollback/storage.go:43-116` (fileLocks pattern, getFileLock)
- Read `session/scrollback/storage.go:186-240` (Read implementation — understand entry format)
- Understand `ScrollbackEntry.Sequence` field

**Implementation approach**:

1. Add method to `FileScrollbackStorage`:
```go
// LatestSequence returns the sequence number of the most recently written
// entry for the given session, or 0 if no entries exist or on error.
func (s *FileScrollbackStorage) LatestSequence(sessionID string) (uint64, error) {
    lock := s.getFileLock(sessionID)
    lock.Lock()
    defer lock.Unlock()

    entries, err := s.readUnlocked(sessionID, 0)
    if err != nil || len(entries) == 0 {
        return 0, err
    }
    return entries[len(entries)-1].Sequence, nil
}
```

Note: If `Read()` already acquires the lock, extract an internal `readUnlocked()` helper that both `Read()` and `LatestSequence()` call. Check the Read implementation first — if it acquires the lock internally, use a private unlocked variant.

2. Add `LatestSequence(sessionID string) (uint64, error)` to the `ScrollbackStorage` interface (if one exists — grep for `interface.*Scrollback`).

3. Add unit test in `session/scrollback/storage_test.go` (or create if missing):
- Write 3 entries with sequences 1, 2, 3 → `LatestSequence` returns 3
- Empty session → returns 0, nil
- Concurrent write + LatestSequence → no race (run with `-race`)

**Validation**:
- `go test -race ./session/scrollback/... -run TestLatestSequence` passes
- No deadlock with concurrent Write + LatestSequence calls
- Success criteria: method exists, returns last sequence, acquires correct lock

**INVEST check**: Independent ✓ | Negotiable (lock strategy) ✓ | Valuable (unblocks accurate fork) ✓ | Estimable (2h) ✓ | Small (1 method + test) ✓ | Testable ✓

---

### Task 2.2: Update Checkpoint Callers to Use Real Sequence [Small — 2h]

**Objective**: Update `CreateCheckpoint` RPC handler and workspace service to pass real `scrollbackSeq` from `ScrollbackManager`.

**Context boundary**:
- Primary: `server/services/session_service.go:1699-1732` (CreateCheckpoint handler)
- Supporting: `server/services/workspace_service.go:280-295`, `server/services/session_service.go` (ScrollbackManager access pattern)
- Total context: ~120 lines

**Prerequisites**:
- Completion of Task 2.1 (LatestSequence method exists)
- Understand how `SessionService` holds a reference to `ScrollbackManager` (grep for `scrollbackManager` field on service struct)
- Verify the session ID used as scrollback key matches the instance title

**Implementation approach**:

1. In `server/services/session_service.go`, in `CreateCheckpoint` handler before `inst.CreateCheckpoint(...)`:
```go
// Get current scrollback sequence for accurate checkpoint fork truncation.
var scrollbackSeq uint64
if s.scrollbackManager != nil {
    if seq, err := s.scrollbackManager.LatestSequence(req.Msg.SessionId); err == nil {
        scrollbackSeq = seq
    }
}
cp, err := inst.CreateCheckpoint(req.Msg.Label, scrollbackSeq)
```

2. Same pattern in `workspace_service.go:286` — obtain scrollbackSeq from its own scrollback manager reference (or pass 0 if not available, with a comment explaining why).

3. Verify `SessionService` has a `scrollbackManager` field (grep for it). If not, it needs to be injected at construction — check `NewSessionService()` signature.

**Validation**:
- `go test -race ./server/services/... -run TestCheckpoint` passes
- Creating a checkpoint on a session with scrollback returns `scrollback_seq > 0`
- Success criteria: no compilation errors, existing checkpoint tests still pass

**INVEST check**: Independent (from Task 2.1 only) ✓ | Negotiable (nil check detail) ✓ | Valuable (accurate fork data) ✓ | Estimable (2h) ✓ | Small (2 call sites) ✓ | Testable ✓

---

### Task 2.3: HistoryFilePath in Proto Adapter [Micro — 1h]

**Objective**: Wire `inst.HistoryFilePath` into the proto adapter so the web UI can display the linked JSONL file.

**Context boundary**:
- Primary: `server/adapters/instance_adapter.go:15-88` (InstanceToProto function, ~75 lines)
- Supporting: `proto/session/v1/types.proto:139` (confirm `history_file_path = 41` exists)
- Total context: ~80 lines

**Prerequisites**:
- Confirm `history_file_path` field exists in generated proto at `gen/proto/go/session/v1/types.pb.go`
- Confirm `inst.HistoryFilePath` is a `string` field on `*session.Instance`

**Implementation approach**:

1. In `server/adapters/instance_adapter.go`, inside `InstanceToProto`, add to the `protoSession` struct literal (near the ClaudeSession block, line ~55):

```go
// History file linkage
HistoryFilePath: inst.HistoryFilePath,
```

2. Verify the generated Go field name — run `grep -n "HistoryFilePath\|history_file_path" gen/proto/go/session/v1/types.pb.go` to confirm the exact Go field name.

3. No proto-gen needed — field already exists.

**Validation**:
- `go build ./...` succeeds
- `go test ./server/adapters/...` passes
- Success criteria: compilation succeeds, no existing adapter tests broken

**INVEST check**: Independent ✓ | Negligible risk ✓ | Valuable (web UI completeness) ✓ | Estimable (1h) ✓ | Micro (1 LOC) ✓ | Testable (build check) ✓

---

## Story 3: Checkpoint Web UI

**User value**: A developer can click a bookmark icon on any running session card, enter a label (or accept the default), and see a list of named checkpoints with timestamp, git SHA, and conversation UUID.

**Acceptance criteria**:
- Bookmark (🔖) button visible on session card action bar for Running sessions
- Clicking opens a popover with label input pre-filled as `"Checkpoint YYYY-MM-DD HH:MM"`
- Submitting calls `CreateCheckpoint` RPC and adds entry to the checkpoint list
- Checkpoint list shows: label, relative time ("3 minutes ago"), `git:abc1234` pill, `conv:def5678` pill
- Delete (✕) button per checkpoint calls `DeleteCheckpoint` RPC
- Empty state: "No checkpoints yet — click 🔖 to save your place"
- List capped at 10 most recent; "Show all (N)" expand link when > 10

---

### Task 3.1: Checkpoint Service Hooks [Small — 2h]

**Objective**: Add `createCheckpoint`, `listCheckpoints`, `deleteCheckpoint` methods to the existing session service hook.

**Context boundary**:
- Primary: `web-app/src/lib/hooks/useSessionService.ts` (find existing hook file)
- Supporting: `web-app/src/lib/client.ts` or equivalent ConnectRPC client setup
- Total context: ~150 lines

**Prerequisites**:
- Locate `useSessionService.ts` or equivalent (grep for `createSession\|useSession` in `web-app/src/lib/`)
- Understand the existing ConnectRPC client pattern used by other RPC calls
- Verify proto-generated TypeScript types include `CreateCheckpointRequest`, `CheckpointProto`

**Implementation approach**:

1. Add to the session service hook:
```typescript
createCheckpoint: async (sessionId: string, label: string): Promise<CheckpointProto> => {
  const response = await client.createCheckpoint({ sessionId, label });
  return response.checkpoint!;
},

listCheckpoints: async (sessionId: string): Promise<CheckpointProto[]> => {
  const response = await client.listCheckpoints({ sessionId });
  return response.checkpoints;
},

deleteCheckpoint: async (sessionId: string, checkpointId: string): Promise<void> => {
  await client.deleteCheckpoint({ sessionId, checkpointId });
},
```

2. Verify method names match the generated TypeScript client (check `gen/` directory for generated TS).

3. Add TypeScript types import if `CheckpointProto` not already imported.

**Validation**:
- `npm run build` in `web-app/` succeeds
- TypeScript type-check passes: `npx tsc --noEmit`
- Success criteria: three new methods on the hook object, compilation succeeds

**INVEST check**: Independent ✓ | Negotiable (error handling style) ✓ | Valuable (enables Task 3.2/3.3) ✓ | Estimable (2h) ✓ | Small (3 methods) ✓ | Testable (type-check) ✓

---

### Task 3.2: CheckpointButton Component [Medium — 3h]

**Objective**: Create a bookmark icon button that opens a popover with label input and calls `createCheckpoint`.

**Context boundary**:
- Primary: `web-app/src/components/sessions/CheckpointButton.tsx` (new file)
- Supporting: `web-app/src/components/sessions/SessionCard.tsx` (integration point), existing popover/tooltip component pattern
- Total context: ~200 lines

**Prerequisites**:
- Completion of Task 3.1
- Understand the session card action bar structure in `SessionCard.tsx`
- Check if a Popover/Tooltip component exists in the UI library (search `web-app/src/components/ui/`)
- Read `web-app/src/styles/theme.css.ts` (vanilla-extract tokens) — use `vars.xxx` not hardcoded hex

**Implementation approach**:

1. Create `web-app/src/components/sessions/CheckpointButton.tsx`:
```tsx
interface CheckpointButtonProps {
  sessionId: string;
  isRunning: boolean;
  onCheckpointCreated: (checkpoint: CheckpointProto) => void;
}
```

2. Internal state: `isOpen: boolean`, `label: string` (default: `"Checkpoint " + format(new Date(), "yyyy-MM-dd HH:mm")`)

3. UI: bookmark icon button (disabled when `!isRunning`). Popover contains:
   - `<input>` pre-filled with default label, `onChange` updates label
   - "Create" button → calls `createCheckpoint(sessionId, label)` → calls `onCheckpointCreated`
   - "Cancel" button closes popover

4. Create `CheckpointButton.css.ts` for vanilla-extract styles (per project CSS architecture: `docs/adr/009-vanilla-extract-type-safe-css.md`).

5. Add `<CheckpointButton>` to `SessionCard.tsx` action bar — only show for Running sessions.

**Validation**:
- Component renders without error for a Running session
- Component renders disabled for non-Running sessions
- Clicking "Create" calls `createCheckpoint` (mock the hook in test)
- `make restart-web` succeeds
- Success criteria: button visible on Running sessions, popover opens/closes, RPC called on submit

**INVEST check**: Independent (from 3.1 only) ✓ | Negotiable (popover vs modal) ✓ | Valuable (UX entry point) ✓ | Estimable (3h) ✓ | Small (1 component) ✓ | Testable (unit test + visual) ✓

---

### Task 3.3: CheckpointList Component [Medium — 3h]

**Objective**: Create a collapsible checkpoint list component that displays checkpoints with metadata pills and delete actions.

**Context boundary**:
- Primary: `web-app/src/components/sessions/CheckpointList.tsx` (new file)
- Supporting: `web-app/src/components/sessions/SessionCard.tsx` (integration), `CheckpointProto` type
- Total context: ~200 lines

**Prerequisites**:
- Completion of Tasks 3.1 and 3.2
- Understand the `CheckpointProto` fields: `id`, `label`, `timestamp`, `gitCommitSha`, `claudeConvUuid`, `scrollbackSeq`
- Check existing "pill" / "badge" components in `web-app/src/components/ui/`

**Implementation approach**:

1. Create `web-app/src/components/sessions/CheckpointList.tsx`:
```tsx
interface CheckpointListProps {
  sessionId: string;
  checkpoints: CheckpointProto[];
  onDelete: (checkpointId: string) => void;
}
```

2. Render list sorted by `timestamp` descending (most recent first).

3. Per entry:
   - `label` in bold
   - Relative time (`"3 minutes ago"` — use `date-fns` `formatDistanceToNow` which is likely already a dep)
   - `git:` + first 7 chars of `gitCommitSha` as small pill (omit if empty)
   - `conv:` + first 8 chars of `claudeConvUuid` as small pill (omit if empty)
   - ✕ delete button → `onDelete(checkpoint.id)`

4. Truncate at 10 entries; "Show all (N)" expand button when `checkpoints.length > 10`.

5. Empty state when `checkpoints.length === 0`: "No checkpoints yet — click 🔖 to save your place"

6. Collapse/expand toggle for the whole list (collapsed by default).

7. Colocate `CheckpointList.css.ts` with vanilla-extract styles.

8. Wire `CheckpointList` into `SessionCard.tsx`:
   - Load checkpoints via `listCheckpoints(session.id)` on mount and on checkpoint creation
   - Pass `onDelete` that calls `deleteCheckpoint` then refreshes the list

**Validation**:
- List renders 3 checkpoints correctly in tests
- "Show all" appears only when > 10 checkpoints
- Empty state renders when list is empty
- Delete calls `onDelete` prop
- `make restart-web` succeeds
- Success criteria: checkpoint list visible in session card, all metadata pills render

**INVEST check**: Independent (from 3.1/3.2 only) ✓ | Negotiable (collapse default state) ✓ | Valuable (completes checkpoint UX) ✓ | Estimable (3h) ✓ | Small (1 component + CSS) ✓ | Testable ✓

---

## Known Issues

### 🐛 Race Condition: Cold Restore During tmux Kill [SEVERITY: Low]
**Description**: In Task 1.1, `DoesSessionExist()` and `Start()` are not atomic. Between the check and the start, a different goroutine could create a session with the same name.
**Mitigation**: `Instance.start()` is serialized by the caller (not called concurrently for the same instance). The `stateMutex` is held for status transitions immediately after start. Low risk in practice.
**Files affected**: `session/instance.go`
**Prevention**: The existing single-instance-per-title constraint in storage prevents concurrent starts for the same instance.

### 🐛 LatestSequence Lock Contention [SEVERITY: Low]
**Description**: If `LatestSequence` acquires the file lock and the implementation reads all entries (O(N)), it could block concurrent `Write` calls for large scrollback files.
**Mitigation**: In Task 2.1, if read-all is too slow, fall back to seeking the end of the file and parsing only the last line. Start with read-all and optimize if benchmarks show regression.
**Files affected**: `session/scrollback/storage.go`
**Prevention**: Benchmark `BenchmarkLatestSequence` with 10k entries to verify < 5ms.

### 🐛 Checkpoint List Staleness [SEVERITY: Low]
**Description**: Task 3.3 loads checkpoints on mount. If another browser tab creates a checkpoint, the list doesn't update until the page is refreshed.
**Mitigation**: The existing `ListSessions` streaming subscription could include checkpoint count as a trigger. For MVP, accept the staleness — the user can refresh. Add a "Refresh" button or rely on the session list subscription as a follow-on.
**Files affected**: `web-app/src/components/sessions/CheckpointList.tsx`
**Prevention**: Not blocking for MVP; document as known limitation.

### 🐛 KI-003: PID Reuse in HistoryLinker [SEVERITY: High — existing, not introduced here]
**Description**: From `project_plans/session-resumption/implementation/plan.md` KI-003: `Detect(pid)` called without verifying `ProcessInspector.IsAlive(pid, createTime)`. Wrong conversation UUID could be linked.
**Mitigation**: Not addressed in this iteration — it is in the existing implementation. Note for follow-on work.
**Files affected**: `session/history_linker.go`

---

## Integration Checkpoints

**After Story 1 (Cold Restore)**:
- `go test -race ./session/... -run TestColdRestore` → all 3 cases pass or skip
- Manual: kill tmux server, restart stapler-squad, verify sessions with UUID start with `--resume` in tmux command (`tmux list-panes -a -F "#{pane_start_command}"`)

**After Story 2 (Data Surface)**:
- `go test -race ./session/scrollback/... -run TestLatestSequence` passes
- `go test -race ./server/services/... -run TestCheckpoint` passes
- `go build ./...` succeeds

**After Story 3 (Checkpoint UI)**:
- `make restart-web` succeeds
- Manual: create a session, click 🔖, create a checkpoint, verify it appears in the list with timestamp + git SHA pill
- `go test ./...` still green

**Final acceptance**:
- All 4 stories complete
- `make quick-check` passes
- Manual smoke: restart stapler-squad after killing tmux → sessions resume with correct conversation → create checkpoint → verify in UI

---

## Context Preparation Guide

### Task 1.1
- Load: `session/instance.go` lines 762-879 (start method), lines 940-970 (resolveStartPath), lines 1381-1440 (initTmuxSession)
- Concepts: tmux session lifecycle, cold vs hot restore, `--resume` flag injection via ClaudeCommandBuilder

### Task 1.2
- Load: `session/instance_cold_restore_test.go` (new), `session/instance.go` Start method interface
- Concepts: Go test skip patterns (`testing.Short()`, `exec.LookPath`), tmux session name uniqueness

### Task 2.1
- Load: `session/scrollback/storage.go` lines 40-250 (entire storage implementation)
- Concepts: per-session file mutex pattern, JSONL sequence number model

### Task 2.2
- Load: `server/services/session_service.go` lines 1698-1732, `server/services/workspace_service.go` lines 280-295
- Concepts: how SessionService holds ScrollbackManager reference, session ID as scrollback key

### Task 2.3
- Load: `server/adapters/instance_adapter.go` lines 1-88, `gen/proto/go/session/v1/types.pb.go` (search for `HistoryFilePath`)
- Concepts: proto field mapping, generated Go struct field names

### Tasks 3.1-3.3
- Load: `web-app/src/lib/hooks/useSessionService.ts`, `web-app/src/components/sessions/SessionCard.tsx`
- Load: `gen/` directory for TypeScript proto types, `web-app/src/styles/theme.css.ts` for tokens
- Concepts: vanilla-extract CSS (`.css.ts` colocated), ConnectRPC TypeScript client pattern, existing session card action bar structure

---

## Success Criteria

- [ ] All 5 atomic tasks completed and validated
- [ ] `go test -race ./...` passes (cold restore tests skip if tmux absent)
- [ ] `make quick-check` passes
- [ ] Manual smoke: kill tmux server → restart stapler-squad → sessions with UUID resume with `--resume`
- [ ] Manual smoke: create checkpoint → verify in session card list with metadata
- [ ] No new `TODO` or stub code committed
- [ ] `go build ./...` succeeds on Linux (CGo off) — procinfo stubs handle gracefully
