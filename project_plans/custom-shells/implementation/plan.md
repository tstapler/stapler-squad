# Custom Shells — Implementation Plan

## Overview

This plan delivers a tab-based multi-shell feature for Stapler Squad. Users can attach named tmux
windows (custom shells) to existing sessions, interact with them via the same xterm.js terminal
infrastructure, and persist them across session pause/resume cycles.

**Epics:** 4  
**Stories:** 11  
**Tasks:** 37

---

## Technology Choices with Rationale

| Decision | Choice | Rationale |
|---|---|---|
| Shell process host | tmux named windows (`new-window -n {shellUUID}`) | Survives server restart; aligns with existing PTY stack; name-based targeting avoids index instability (pitfalls.md §2) |
| PTY attach strategy | New `ShellTmuxHandle` struct that wraps `TmuxSession` fields but targets `{sessionName}:{shellUUID}` | Avoids polluting `TmuxSession` struct with `windowIndex`; reuses `buildTmuxCommand` and `ptyFactory.Start`; minimal diff |
| Shell identity | UUID stored as both ent PK and tmux window name | Single source of truth; tmux list-windows on restart uses name to re-associate |
| Streaming protocol | Extend `TerminalData`: `shell_id = 17` (top-level, outside oneof); `ShellStatusUpdate` inside oneof as field 18 | Field 17 is the first free slot after `resize_quiescence = 16`; top-level placement avoids oneof field collisions; backwards-compatible (zero default = main terminal) |
| Status push | `ShellStatusUpdate` in oneof of existing stream | Consistent with existing `TerminalError` / `ResizeQuiescence` push pattern; avoids polling |
| Storage | ent `Shell` entity with FK to `Session` + auto-migrate | Zero-migration overhead; WAL+MaxOpenConns=1 serializes writes |
| Scrollback key | `{sessionID}/{shellID}` scoped within existing scrollback infra | Isolates per shell; prevents verbose shells from crowding session scrollback |
| Frontend tab model | Static `SessionDetailTab` union unchanged; shell tabs rendered as `shellTabs: ShellTab[]` alongside | Avoids compile-time knowledge of runtime UUIDs; clean branching on `activeTab.startsWith("shell:")` |
| Terminal pool | Reuse existing 8-slot pool in `SessionDetailView`; shell tabs occupy pool slots | No new pool needed; shells and session tabs share the same LRU budget |
| Session creation registry | Not applicable | Shells are sub-resources of sessions, not creation modes; 7-touchpoint registry does not apply |

---

## Epic 1: Backend — ent Schema and Repository

**Goal:** Persist shell state in SQLite so shells survive server restarts.

### Story 1.1: Shell ent schema

**Tasks:**

1. **Create `session/ent/schema/shell.go`**
   - Fields: `id` (string PK, immutable), `name` (string), `command` (string, default `""`),
     `working_dir` (string, optional), `tmux_window_name` (string) — stores the UUID used as tmux
     window name, `status` (string, default `"running"`), `exit_code` (int, optional, nillable),
     `order_index` (int, default 0), `started_at` (time, default Now), `stopped_at` (time, optional,
     nillable).
   - Edge: `From("session", Session.Type).Ref("shells").Unique().Required()`.
   - Note: use `tmux_window_name` not `tmux_window_index` to store the durable tmux name
     (pitfalls.md §2: indices are not stable).

2. **Extend `session/ent/schema/session.go`**
   - Add `edge.To("shells", Shell.Type)` to Session's `Edges()`.

3. **Regenerate ent bindings**
   - Run the correct generate command (from `session/ent/generate.go`):
     ```
     go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema
     ```
   - Commit all generated files under `session/ent/` together.

4. **Add `Shell` in-memory type to `session/shell.go`** (new file)
   - `ShellStatus` string constants: `ShellStatusRunning`, `ShellStatusStopped`, `ShellStatusError`.
   - `Shell` struct: `ID`, `Name`, `Command`, `WorkingDir`, `TmuxWindowName`, `Status ShellStatus`,
     `ExitCode int`, `OrderIndex int`.
   - `ShellHandle` interface: `GetPTY() (*os.File, error)`, `Resize(cols, rows int) error`,
     `Close() error` — lets the streaming handler work with shells and main sessions uniformly.

### Story 1.2: Shell repository methods

**File:** `session/ent_repository.go`

5. **Add `CreateShell(ctx, sessionTitle string, shell ShellData) (*ent.Shell, error)`**
   - Looks up Session by title, creates Shell with FK.
   - `ShellData` struct (new, in `session/shell.go`): mirrors ent fields.

6. **Add `ListShells(ctx, sessionTitle string) ([]*ent.Shell, error)`**
   - Queries shells joined to session, ordered by `order_index`.

7. **Add `UpdateShellStatus(ctx, shellID, status string, exitCode *int) error`**
   - Patch `status` and optionally `exit_code` + `stopped_at`.

8. **Add `DeleteShell(ctx, shellID string) error`**

---

## Epic 2: Backend — Shell Lifecycle and tmux Integration

**Goal:** Spawn, stop, and restart shell processes in tmux windows.

### Story 2.1: ShellTmuxHandle

**File:** `session/tmux/shell_handle.go` (new)

9. **Create `ShellTmuxHandle` struct**
   - Fields: `sessionName string`, `windowName string`, `serverSocket string`, `ptmx *os.File`,
     `attachCmd *exec.Cmd`, `ptyFactory PtyFactory`, `cmdExec executor.Executor`,
     `spawnMu sync.Mutex`, `lastKnownCols/Rows atomic.Int32`.
   - `Spawn(workDir, command string) error`:
     - Acquires `spawnMu`.
     - Runs `tmux new-window -t {sessionName} -n {windowName} -c {workDir} -- /bin/sh -c {command}`.
     - Uses `buildTmuxCommand` (or a local equivalent) to handle `-L {serverSocket}` isolation.
     - Returns error if tmux exits non-zero.
   - `Attach() error`:
     - Runs `tmux attach-session -t {sessionName}:{windowName}` via `ptyFactory.Start`.
     - Stores `ptmx` and `attachCmd`. Must call `attachCmd.Wait()` on close (zombie mitigation).
   - `GetPTY() (*os.File, error)`: returns `ptmx`; errors if nil.
   - `Resize(cols, rows int) error`:
     - Runs `tmux resize-window -t {sessionName}:{windowName} -x {cols} -y {rows}`.
     - Updates `lastKnownCols/Rows`.
   - `Close() error`:
     - Closes `ptmx`.
     - Sends `tmux kill-window -t {sessionName}:{windowName}`.
     - Calls `attachCmd.Wait()` with a 5s timeout goroutine to reap the attach process (zombie mitigation; matches pattern in `TmuxSession.Close()`).
   - `ExitCode() (int, bool)`:
     - Runs `tmux display-message -t {sessionName}:{windowName} "#{pane_dead_status}"` after EOF.
     - Returns `(code, true)` or `(0, false)` if window gone.

10. **Tests for `ShellTmuxHandle`** in `session/tmux/shell_handle_test.go`
    - Use the existing `TmuxTestEnv` pattern (isolated server socket) already established in `session/tmux/` tests.
    - Cover: Spawn + Attach succeeds; Close reaps attachCmd; ExitCode after normal exit; Close while Attach in progress (stop-while-streaming race).

### Story 2.2: Shell registry on Instance

**File:** `session/instance.go`

11. **Add shell registry fields to `Instance`**
    - `shells map[string]*Shell` — keyed by shell ID.
    - `shellHandles map[string]*tmux.ShellTmuxHandle` — keyed by shell ID.
    - `spawnMu deadlock.Mutex` — serializes `SpawnShell` calls (spawn race mitigation; pitfalls.md §3).
    - Initialize maps in `newInstance()` or wherever Instance is constructed.

12. **Add `SpawnShell(ctx, req SpawnShellRequest) (*Shell, error)`**
    - `SpawnShellRequest`: `Name, Command, WorkingDir string`.
    - Acquires `spawnMu` before creating tmux window to prevent concurrent spawn race.
    - Generates UUID for shell ID.
    - Resolves `WorkingDir`: default to `inst.WorkingDir` if empty.
    - Resolves `Command`: default to `$SHELL` env or `/bin/sh` if empty.
    - Creates `ShellTmuxHandle{sessionName: inst.TmuxPrefix, windowName: shellID, ...}`.
    - Calls `handle.Spawn(workDir, command)` — returns error if tmux fails.
    - Creates in-memory `Shell` and stores in `shells` and `shellHandles`.
    - Persists via `repository.CreateShell(...)`.
    - Launches `watchShellExit(shellID, handle)` goroutine.
    - Returns `*Shell`.

13. **Add `watchShellExit(shellID string, handle *ShellTmuxHandle)`** (private goroutine)
    - Reads from `handle.GetPTY()` until EOF/error.
    - On EOF: calls `handle.ExitCode()`, updates `shells[shellID].Status` and `ExitCode`.
    - Calls `repository.UpdateShellStatus(shellID, status, &code)`.
    - Notifies any active `StreamTerminal` subscribers for this shell via a per-shell exit channel.
    - Stop-while-streaming race guard: treat read error as EOF if `shells[shellID].Status` is
      already `ShellStatusStopped` (set by `StopShell` before closing PTY).

14. **Add `StopShell(ctx, shellID string) error`**
    - Sets `shells[shellID].Status = ShellStatusStopped` under lock (stop race guard).
    - Calls `handle.Close()`.
    - Calls `repository.UpdateShellStatus(...)`.

15. **Add `RestartShell(ctx, shellID string) error`**
    - Calls `StopShell` if currently running.
    - Re-creates `ShellTmuxHandle` with same command/workDir.
    - Calls `handle.Spawn` + `handle.Attach`.
    - Updates status to `ShellStatusRunning`, clears `ExitCode`.
    - Launches new `watchShellExit` goroutine.

16. **Add `GetShellPTYReader(shellID string) (*os.File, error)`**
    - Calls `handle.Attach()` if not already attached (lazy attach for reconnect after pause/resume).
    - Returns `handle.GetPTY()`.

17. **Add `ListShellsInMemory() []*Shell`**
    - Returns shells sorted by `OrderIndex`.

18. **Server-restart reconciliation in `ReconcileShells(ctx context.Context)`** (new method on Instance)
    - Called after Instance is loaded from ent on startup (e.g., in the existing service bootstrap).
    - Queries ent `ListShells(title)` for shells with status `running`.
    - For each: runs `tmux list-windows -t {sessionName} -F "#{window_name}"` to check existence.
    - If window exists: rebuilds in-memory `Shell` + `ShellTmuxHandle` (but does NOT call `Attach` yet — lazy on first stream).
    - If window missing: calls `repository.UpdateShellStatus(id, "stopped", nil)`.

---

## Epic 3: Backend — ConnectRPC Service Layer

**Goal:** Expose shell lifecycle and terminal streaming RPCs.

### Story 3.1: Proto definitions

19. **Add shell types to `proto/session/v1/types.proto`**
    - `ShellStatus` enum (UNSPECIFIED=0, RUNNING=1, STOPPED=2, ERROR=3).
    - `Shell` message (id, session_id, name, command, working_dir, status, exit_code, started_at,
      order_index).

20. **Add shell RPCs and messages to `proto/session/v1/session.proto`**
    - `StartShell(StartShellRequest) returns (StartShellResponse)`
    - `StopShell(StopShellRequest) returns (StopShellResponse)`
    - `RestartShell(RestartShellRequest) returns (RestartShellResponse)`
    - `ListShells(ListShellsRequest) returns (ListShellsResponse)`
    - `DeleteShell(DeleteShellRequest) returns (DeleteShellResponse)`
    - All request/response messages with appropriate fields (see architecture.md §Required RPCs).

21. **Extend `proto/session/v1/events.proto`** — `TerminalData` message:
    - Add top-level field `string shell_id = 17;` (outside oneof, default empty = main terminal).
    - Add `ShellStatusUpdate shell_status_update = 18;` inside the `oneof data` block.
    - New `ShellStatusUpdate` message: `shell_id`, `ShellStatus status`, `int32 exit_code`.
    - Run `make generate-proto`.

### Story 3.2: Handler implementation

**File:** `server/services/session_service.go`

22. **Extend `StreamTerminal`** handler fork point:
    - After `session_id` validation and Instance lookup, read `req.ShellId`.
    - If `ShellId != ""`: call `instance.GetShellPTYReader(req.ShellId)` instead of
      `instance.GetPTYReader()`.
    - Subscribe to the per-shell exit channel; on exit, send `ShellStatusUpdate` into the stream
      before closing (stop-while-streaming guard — the handler sends a clean exit event rather than
      a `TerminalError`).
    - All downstream code (flow control, scrollback, output loop) is reused unchanged.

23. **Add `StartShell` handler**
    - Validates `session_id` non-empty; looks up Instance.
    - Calls `instance.SpawnShell(ctx, req)`.
    - Returns proto `Shell`.

24. **Add `StopShell` handler**
    - Calls `instance.StopShell(ctx, req.ShellId)`.

25. **Add `RestartShell` handler**
    - Calls `instance.RestartShell(ctx, req.ShellId)`.

26. **Add `ListShells` handler**
    - Calls `instance.ListShellsInMemory()` + merges with ent `ListShells` for stopped shells.
    - Returns sorted list.

27. **Add `DeleteShell` handler**
    - Calls `instance.StopShell` if running.
    - Calls `repository.DeleteShell(shellID)`.
    - Removes from in-memory maps.

28. **Register new handlers in `server/server.go`** — no new service registration needed, all RPCs
    are on the existing `SessionService`.

---

## Epic 4: Frontend — Shell Tabs and Terminal UI

**Goal:** Tab strip with dynamic shell tabs; reuse xterm.js terminal pool; status indicators.

### Story 4.1: Shell tab data and state

29. **Add `ShellTab` type and shell hooks in `web-app/src/lib/hooks/useShells.ts`** (new file)
    - `ShellTab`: `{ id: string; name: string; command: string; status: "running" | "stopped" | "error"; exitCode?: number; }`.
    - `useShells(sessionId)` hook:
      - Calls `ListShells` RPC on mount, returns `{ shells, startShell, stopShell, restartShell, deleteShell, isLoading }`.
      - Refetches on session change.

30. **Extend `web-app/src/lib/hooks/useSessionService.ts`**
    - Wire `startShell`, `stopShell`, `restartShell`, `listShells`, `deleteShell` RPC calls.
    - Thread `shellId` into `createSession`-equivalent streaming call (pass `shell_id` in initial
      `TerminalData` message from `useTerminalStream`).

31. **Extend `useTerminalStream` in `TerminalOutput.tsx`**
    - Add optional `shellId?: string` to hook params.
    - When `shellId` set: include `shell_id: shellId` in the first `TerminalData` message
      (the `CurrentPaneRequest` / handshake send).
    - Handle incoming `ShellStatusUpdate` message: call `onShellStatusChange(status, exitCode)` callback.

### Story 4.2: SessionDetailView tab strip extension

**File:** `web-app/src/components/sessions/SessionDetailView.tsx`

32. **Add `shellTabs` state alongside `tabs` array**
    - `const [shellTabs, setShellTabs] = useState<ShellTab[]>([])`.
    - Populated by `useShells(session.id)` hook.
    - Render in tab strip after the static tabs, with a "+" button at the end.

33. **Extend `handleTabChange` to accept `string` (not just `SessionDetailTab`)**
    - Branch: if `tabId.startsWith("shell:")` → activate shell tab; else existing switch.
    - `SessionDetailTab` union stays unchanged (no shell IDs in it).

34. **Extend terminal pool to cover shell tabs**
    - Pool slot key: `shellId` instead of `sessionId` for shell terminals.
    - On shell tab open: add `shellId` to `pooledSessionIds` (same pool, same 8-slot LRU).
    - On pool slot render (the `pooledSessionIds.map` block): if `poolId` is a shell UUID,
      render `<TerminalOutput shellId={poolId} sessionId={session.id} ... />` with `display:none`
      toggling as before.

### Story 4.3: Shell tab UI components

35. **Create `web-app/src/components/sessions/ShellTab.tsx`** (new file)
    - Tab label: command (truncated to 20 chars) with status dot.
    - Status dot: green for `running`, red for `error`/`stopped` (uses `--success`, `--error` CSS tokens).
    - Action buttons: Stop / Restart / Close (shown on hover or as context menu).
    - **Style file:** `ShellTab.css.ts` using vanilla-extract, tokens from `globals.css`.

36. **Create `web-app/src/components/sessions/NewShellDialog.tsx`** (new file)
    - Modal dialog: "Name" (optional), "Command" (default: `$SHELL`), "Working Directory"
      (default: session path, editable).
    - On confirm: calls `startShell(...)`.
    - Uses existing modal pattern (vanilla-extract styles, `createPortal` for overlay).

### Story 4.4: Feature registry and tests

37. **Update feature registry**
    - Add entries to `docs/registry/features/` for each new RPC:
      `shell:start`, `shell:stop`, `shell:restart`, `shell:list`, `shell:delete`.
    - Run `make registry-generate`.

38. **Unit tests — Go**
    - `session/tmux/shell_handle_test.go`: spawn/attach/close lifecycle; zombie reap; stop-while-streaming.
    - `session/shell_test.go`: SpawnShell serialization (concurrent calls); reconcileShells on restart.

39. **Unit tests — TypeScript (Jest)**
    - `useShells.test.ts`: listShells populates state; startShell optimistic add; status update.
    - `ShellTab.test.tsx`: renders status dot; stop/restart/close buttons call correct hook methods.

40. **E2e test: `tests/e2e/custom-shells.spec.ts`**
    - `// @feature shell:start, shell:stop, shell:list`
    - Covers: open dialog → spawn shell → see tab → type command → see output → stop → red indicator.

---

## Dependency Order

```
Epic 1 (Schema + Repository)
  └─> Epic 2 (Instance Shell Registry + tmux Handle)
        └─> Epic 3 (Proto + RPC Handlers)
              └─> Epic 4 (Frontend)

Within Epic 1:
  Task 1 (shell.go schema) → Task 2 (session.go edge) → Task 3 (generate ent) → Task 4 (in-memory types)

Within Epic 2:
  Task 4 (Shell type) → Task 9 (ShellTmuxHandle) → Tasks 11-18 (Instance methods)

Within Epic 3:
  Tasks 19-21 (proto) → make generate-proto → Tasks 22-28 (handlers)

Within Epic 4:
  Tasks 29-31 (hooks) → Tasks 32-34 (SessionDetailView) → Tasks 35-36 (components) → Task 37 (registry+tests)
```

---

## ADR-Worthy Decisions

### ADR-1: Use tmux named windows (not indices) as shell identity anchor

**Decision:** Store the shell UUID as the tmux window name (`-n {shellUUID}`). All tmux commands
target `{sessionName}:{shellUUID}`. The ent `Shell` entity stores `tmux_window_name` (the UUID),
not an integer index.

**Rationale:** tmux window indices are reassigned when windows are deleted and recreated. A server
restart or any intermediate `kill-window` can shift indices. Name-based targeting is stable across
these operations. The UUID is already guaranteed to be safe as a tmux window name (no colons,
dots, or special characters).

**Consequences:** `tmux list-windows` must be used on restart to reconcile name→existence;
integer index is never stored or relied upon.

### ADR-2: `shell_id` as top-level field on `TerminalData` (not inside oneof)

**Decision:** Add `string shell_id = 17` as a top-level message field on `TerminalData`, outside
the `oneof data` block. `ShellStatusUpdate` goes inside `oneof data` as field 18.

**Rationale:** `shell_id` is a routing discriminator, not a data payload. Placing it inside `oneof`
would require it to be mutually exclusive with actual data messages. As a top-level field it can
be set alongside any `oneof` case (e.g., `shell_id = "abc" + output = ...`). This matches how
`session_id = 1` works on the same message.

**Consequences:** Existing clients that send `TerminalData` without `shell_id` default to `""`,
which routes to the main terminal. Proto3 unknown-field forwarding means old servers ignore `shell_id`
from new clients.

### ADR-3: Lazy PTY attach for shell terminals

**Decision:** `GetShellPTYReader` calls `handle.Attach()` if not already attached. Shells are
spawned in tmux windows before any streaming client connects; the PTY attach is deferred until
the first `StreamTerminal` call for that shell.

**Rationale:** On server restart, shells are reconciled from ent without immediately attaching PTYs
(which would open file descriptors and goroutines for every shell). Only shells that a client
actively views incur the cost of PTY attachment. This matches how the main session works
(`AttachToExisting` is called on stream open, not on session load).

**Consequences:** First stream connection to a shell after server restart has slightly higher
latency (one tmux attach command). Subsequent reconnects are immediate (handle reuse).

---

## Risk Register

| Risk | Severity | Mitigation in Plan |
|---|---|---|
| Zombie processes from shell attachCmd | Medium | `ShellTmuxHandle.Close()` calls `attachCmd.Wait()` with 5s timeout goroutine (Task 9) |
| Spawn race (two goroutines pick same window index) | Medium | `spawnMu` mutex on `Instance.SpawnShell` (Task 12) |
| Stop-while-streaming PTY read error misclassified as crash | High | `watchShellExit` checks `Status==Stopped` before emitting `ShellStatusError` (Task 13); stream handler sends `ShellStatusUpdate` on clean exit (Task 22) |
| tmux window index instability on restart | High | Name-based targeting (ADR-1); reconciliation runs `tmux list-windows` (Task 18) |
| ent migration drops data | Low | Auto-migrate only adds tables/columns; no destructive ops; SQLite WAL serializes writes |
| xterm.js pool eviction from too many shells | Low | Soft cap 8 shells per session (pool size); "+" button dims at limit (Task 32) |
| proto field number collision | Low | `shell_id = 17` outside oneof; `shell_status_update = 18` inside oneof; verified against existing oneof fields 2–16 |
