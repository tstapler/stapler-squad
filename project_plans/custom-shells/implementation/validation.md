# Custom Shells — Validation Plan

**Linked plan:** `project_plans/custom-shells/implementation/plan.md`  
**Linked requirements:** `project_plans/custom-shells/requirements.md`  
**Adversarial review verdict:** CONCERNS (two HIGH issues resolved by design choices below)

---

## Requirement-to-Test Traceability Matrix

| AC | Requirement | Test IDs |
|---|---|---|
| AC-1.1 | "New Shell" affordance visible | FE-1, E2E-1 |
| AC-1.2 | Dialog with command + working directory fields | FE-2, FE-3, E2E-1 |
| AC-1.3 | New tab appears with command label | FE-4, E2E-2 |
| AC-1.4 | Shell starts in specified working directory | INT-1, SEC-2 |
| AC-2.1 | Output streams into tab's terminal widget | INT-2, E2E-3 |
| AC-2.2 | Typing sends input to shell PTY | INT-3, SEC-1, E2E-3 |
| AC-2.3 | Scrollback history preserved while tab open | INT-4, E2E-4 |
| AC-3.1 | Running shell shows green indicator | FE-5, E2E-2 |
| AC-3.2 | Exited shell shows red indicator + exit code | FE-6, E2E-5 |
| AC-3.3 | Tab title shows command (truncated) | FE-7 |
| AC-4.1 | Stop button sends SIGTERM to process group | FE-8, INT-5, E2E-6 |
| AC-4.2 | Restart button relaunches same command/directory | FE-9, INT-6, E2E-7 |
| AC-4.3 | Close button removes tab and kills process | FE-10, INT-7, E2E-8 |
| AC-5.1 | Pausing session does not kill attached shells | INT-8 |
| AC-5.2 | Resuming session restores shell tabs with status | INT-9 |
| AC-5.3 | Output during pause visible after resume (best-effort) | INT-10 |
| AC-6.1 | No hard limit; soft cap ~10 | FE-11, INT-11 |
| AC-6.2 | Each shell has independent PTY, workdir, status | INT-1, SEC-1, RACE-1 |

**Coverage:** 18 / 18 ACs covered (100%)

---

## 1. Unit Tests — Go

### 1a. ent Schema (`session/ent/schema/shell_test.go`)

**ENT-1: Shell entity fields are persisted and retrieved correctly**
- Create a Shell entity via `ent.Shell.Create()` with all fields populated.
- Assert `ID`, `Name`, `Command`, `WorkingDir`, `TmuxSessionName` (per revised naming from adversarial review Challenge 2), `Status`, `ExitCode`, `OrderIndex`, `StartedAt` round-trip correctly.
- Assert `StoppedAt` is nil on creation; non-nil after `UpdateShellStatus`.

**ENT-2: Shell → Session foreign key edge is enforced**
- Attempt to create a Shell without a Session FK.
- Assert ent returns a constraint error.
- Create a valid Shell via the Session edge and assert `Shell.QuerySession()` returns the correct session.

**ENT-3: Session.QueryShells returns shells ordered by `order_index`**
- Create three shells with `order_index` values 2, 0, 1.
- Assert `session.QueryShells().Order(ent.Asc(shell.FieldOrderIndex)).All(ctx)` returns them in order 0, 1, 2.

**ENT-4: `tmux_session_name` field stores derived name, not raw UUID**
- Assert the field stores a value like `{parentPrefix}_shell_{uuid}` (the full sibling session name).
- Assert the value is not equal to the bare UUID.
- Rationale: adversarial review Challenge 2 — the field was `tmux_window_name` which was redundant/misnamed; it must store the full computed sibling-session name.

### 1b. ShellTmuxHandle — sibling session spawn/stop (`session/tmux/shell_handle_test.go`)

These tests use the existing `TmuxTestEnv` pattern (isolated `--socket-path` per test).

**TMUX-1: Spawn creates an independent sibling tmux session**
- Call `ShellTmuxHandle.Spawn(workDir, "bash")`.
- Assert `tmux list-sessions` shows a session named `{parentPrefix}_shell_{uuid}`.
- Assert the sibling session is independent — killing the parent session does not kill the sibling.
- Rationale: adversarial review Challenge 1 (HIGH) — shells must be sibling sessions, not windows.

**TMUX-2: Attach connects to the sibling session's PTY**
- After `Spawn`, call `Attach()`.
- Assert `GetPTY()` returns a non-nil `*os.File`.
- Write `echo hello\n` via PTY; read response; assert it contains `hello`.
- Assert the PTY file descriptor is distinct from the parent session's PTY file descriptor (different `os.File` instances with different underlying fds).
- Rationale: proves PTY isolation (adversarial HIGH issue A).

**TMUX-3: Two concurrent Spawn calls serialize via `spawnMu`**
- Start two goroutines, each calling `Spawn` on the same `Instance` simultaneously.
- Assert both return without error and two sibling sessions exist with distinct names.
- Assert no tmux command targeting a non-existent window/session was issued.
- Rationale: spawn serialization mutex (plan Task 12, risk register).

**TMUX-4: Close reaps the attach process (zombie mitigation)**
- Call `Spawn` + `Attach`.
- Store the PID of the attach `exec.Cmd` process.
- Call `Close()`.
- After 500ms, assert the PID is no longer in the process table (no zombie).
- Rationale: plan Task 9 zombie mitigation.

**TMUX-5: ExitCode after normal shell exit**
- Spawn a shell with command `exit 42`.
- Wait for EOF on PTY.
- Call `ExitCode()`.
- Assert `(42, true)` is returned.

**TMUX-6: Close while Attach is in-progress does not panic or deadlock**
- Start a long-running shell (`sleep 60`).
- Call `Attach()`.
- Concurrently: call `Close()` from a separate goroutine.
- Assert `Close()` returns within 2 seconds.
- Assert no panic occurs (use `recover` in test goroutine).
- Assert PTY read loop in `Attach` goroutine terminates cleanly.
- Rationale: adversarial HIGH issue B (stop-while-streaming race).

### 1c. Exit channel drain-before-delete (`session/shell_test.go`)

**SHELL-1: Per-shell exit channel is closed (not sent) on exit; multiple subscribers drain cleanly**
- Create a `Shell` struct with its `exitCh chan struct{}`.
- Spin up 3 goroutines that block on `<-shell.exitCh`.
- Trigger `watchShellExit` exit path (simulate PTY EOF).
- Assert all 3 goroutines unblock within 500ms.
- Assert no `send on closed channel` panic.
- Rationale: adversarial Challenge 3 — channel must be closed, not written to, for fan-out safety.

**SHELL-2: DeleteShell drains active StreamTerminal handlers before map removal**
- Simulate an active `StreamTerminal` handler holding a reference to `shells[id]`.
- Call `DeleteShell` concurrently.
- Assert `DeleteShell` waits for the handler's `WaitGroup.Done()` before removing from map.
- Assert no nil-map panic in the handler after `DeleteShell` returns.
- Rationale: adversarial Challenge 3 — `sync.WaitGroup` per shell handle.

**SHELL-3: GetShellPTYReader on a shell being torn down returns an error, not a closed fd**
- Set `shell.Status = ShellStatusStopped` (simulating `StopShell` mid-execution).
- Call `GetShellPTYReader` on the same shell from a separate goroutine.
- Assert the call returns `(nil, ErrShellStopped)` or similar sentinel error rather than a closed file.
- Rationale: adversarial Challenge 3 TOCTOU window.

### 1d. Spawn serialization mutex (`session/shell_test.go`)

**SHELL-4: SpawnShell is safe under concurrent calls from 10 goroutines**
- Launch 10 goroutines, each calling `instance.SpawnShell(ctx, req)` simultaneously.
- Assert all 10 calls return without error.
- Assert `instance.ListShellsInMemory()` has exactly 10 entries, each with a unique ID.
- Assert `tmux list-sessions` confirms 10 distinct sibling sessions.

**SHELL-5: watchShellExit accepts and honors context cancellation**
- Spawn a long-running shell.
- Pass a context with a 100ms timeout to `watchShellExit`.
- Assert the goroutine exits after the context is cancelled (does not leak).
- Rationale: adversarial Challenge 6 (goroutine leak on shutdown).

---

## 2. Integration Tests

These run against a real tmux server (test socket) and an in-memory SQLite ent client.

**INT-1: Full StartShell → StreamTerminal → StopShell flow (AC-1.4, AC-2.2, AC-6.2)**
- Create a session with working directory `/tmp/test-workspace-<uuid>`.
- Call `StartShell` RPC with `command="bash"` and `working_dir="/tmp/test-workspace-<uuid>/subdir"`.
- Call `StreamTerminal` with the returned `shell_id`.
- Send `pwd\n` via PTY input.
- Assert output stream contains `/tmp/test-workspace-<uuid>/subdir`.
- Call `StopShell` RPC.
- Assert stream closes with a `ShellStatusUpdate{status: STOPPED}` message (not a `TerminalError`).

**INT-2: Shell output streams in real time (AC-2.1)**
- Start a shell with `command="bash -c 'for i in $(seq 5); do echo $i; sleep 0.1; done'"`.
- Open `StreamTerminal`.
- Assert output messages arrive incrementally (not all at once) — collect at least 3 separate `TerminalData` messages before the shell exits.

**INT-3: Input typed in shell tab goes to shell PTY, not Claude PTY (AC-2.2, AC-6.2)**
- Start a session (Claude terminal) and spawn a shell (sibling session).
- Open two separate `StreamTerminal` connections: one with `shell_id=""` (Claude terminal), one with `shell_id=<shellId>`.
- Send keystrokes via the shell stream.
- Assert Claude terminal stream receives no additional bytes.
- Assert shell terminal stream echoes the input.
- Rationale: proves PTY isolation end-to-end (adversarial HIGH issue A).

**INT-4: Scrollback replay on shell reconnect (AC-2.3)**
- Spawn a shell, run `echo preserved_output`, close the `StreamTerminal` connection.
- Open a new `StreamTerminal` connection for the same `shell_id`.
- Assert the new stream replays `preserved_output` from the shell-scoped scrollback key (`{sessionID}/{shellID}`).
- Rationale: adversarial Challenge 5 — scrollback recorder must accept shell ID.

**INT-5: StopShell sends SIGTERM to process group (AC-4.1)**
- Spawn a shell with `bash -c 'trap "exit 99" TERM; sleep 60'` (traps SIGTERM, exits 99).
- Call `StopShell` RPC.
- Wait for shell exit.
- Assert exit code is 99 (confirms SIGTERM was delivered, not SIGKILL).

**INT-6: RestartShell relaunches same command in same directory (AC-4.2)**
- Spawn a shell with `command="echo hello" working_dir="/tmp/wdir1"`.
- Wait for it to exit.
- Call `RestartShell` RPC.
- Open `StreamTerminal` for the restarted shell.
- Assert output includes `hello`.
- Assert working directory of new shell is `/tmp/wdir1`.

**INT-7: DeleteShell removes tab and kills process (AC-4.3)**
- Spawn a shell with `sleep 60`.
- Note the sibling tmux session name.
- Call `DeleteShell` RPC.
- Assert `tmux list-sessions` no longer shows the sibling session.
- Assert `ListShells` RPC returns an empty list.
- Assert in-memory `shells` map has no entry for the deleted ID.

**INT-8: Pausing session does not kill attached shells (AC-5.1)**
- Spawn a shell (`sleep 60`) attached to a session.
- Pause the session via the session service.
- Assert the shell's sibling tmux session still exists (`tmux list-sessions`).
- Assert the shell's ent status is still `running`.

**INT-9: Resuming session restores shell tabs with current status (AC-5.2)**
- Spawn two shells: one running (`sleep 60`), one that exits after 200ms (`exit 0`).
- Pause and resume the session.
- Call `ListShells` RPC after resume.
- Assert two shell entries are returned.
- Assert the running shell has `status=RUNNING`.
- Assert the exited shell has `status=STOPPED` with non-zero exit code.

**INT-10: Output during pause visible after resume via scrollback (AC-5.3)**
- Spawn a shell that emits output every 100ms.
- Pause the session.
- Wait 500ms (shell emits ~5 lines while paused).
- Resume the session.
- Open `StreamTerminal` for the shell.
- Assert scrollback replay contains lines emitted during the pause (up to tmux scrollback limit).
- Note: test may be flaky if the tmux scrollback buffer fills; skip if buffer size < 500 lines.

**INT-11: Multiple shells per session are independent (AC-6.1, AC-6.2)**
- Spawn 3 shells on the same session with different commands and working directories.
- Assert each has a distinct sibling tmux session name.
- Assert each has a distinct PTY file descriptor.
- Send input to shell 2 only; assert shells 1 and 3 receive no input.
- Stop shell 1; assert shells 2 and 3 continue running.

---

## 3. Frontend Tests (Jest/RTL)

All tests in `web-app/src/components/sessions/ShellTab.test.tsx` and `web-app/src/lib/hooks/useShells.test.ts`.

**FE-1: Tab strip renders "+" button for new shell (AC-1.1)**
- Render `SessionDetailView` with `shells=[]`.
- Assert a button with accessible label "New Shell" or `aria-label="Add shell"` is present in the tab strip.

**FE-2: NewShellDialog renders command and working directory fields (AC-1.2)**
- Render `NewShellDialog`.
- Assert "Command" input exists with placeholder or default value (`$SHELL` or `bash`).
- Assert "Working Directory" input exists.
- Assert "Confirm" / "Start" button is present.

**FE-3: NewShellDialog uses session path as working directory default (AC-1.2)**
- Render `NewShellDialog` with `sessionPath="/workspace/myproject"`.
- Assert working directory input value is `/workspace/myproject`.

**FE-4: Confirmed dialog creates a new tab with command label (AC-1.3)**
- Mock `startShell` to return `{ id: "sh-1", name: "bash", command: "bash", status: "running" }`.
- Render `SessionDetailView`, open dialog, submit.
- Assert a tab with text containing `bash` appears in the tab strip.

**FE-5: Running shell tab shows green status dot (AC-3.1)**
- Render `ShellTab` with `status="running"`.
- Assert the status indicator element has class/style corresponding to `--success` token (green).
- Assert `aria-label` or `title` communicates "running".

**FE-6: Stopped shell tab shows red indicator and exit code (AC-3.2)**
- Render `ShellTab` with `status="stopped" exitCode={1}`.
- Assert the status indicator uses `--error` token (red).
- Assert exit code `1` is visible in the tab label or tooltip.

**FE-7: Tab title truncates long commands at 20 characters (AC-3.3)**
- Render `ShellTab` with `command="this-is-a-very-long-command-name"`.
- Assert the displayed text is `"this-is-a-very-long-"` or ends with `…`.

**FE-8: Stop button is present and calls stopShell for running shell (AC-4.1)**
- Render `ShellTab` with `status="running"` and `onStop` mock.
- Assert "Stop" button is visible (or appears on hover via forced-hover test).
- Click "Stop"; assert `onStop` was called with the shell ID.

**FE-9: Restart button is present and calls restartShell for stopped shell (AC-4.2)**
- Render `ShellTab` with `status="stopped"` and `onRestart` mock.
- Assert "Restart" button is visible.
- Click "Restart"; assert `onRestart` was called with the shell ID.

**FE-10: Close button calls deleteShell and removes tab (AC-4.3)**
- Render `SessionDetailView` with one shell tab and `deleteShell` mock.
- Click "Close" on the shell tab.
- Assert `deleteShell` was called with the shell ID.
- Assert the tab is no longer rendered.

**FE-11: Soft cap: "+" button is dimmed/disabled at 8 shells (AC-6.1)**
- Render `SessionDetailView` with `shells` array of 8 running shells.
- Assert the "New Shell" button is `disabled` or has `aria-disabled="true"`.
- Assert tooltip or label communicates the limit (e.g., "Maximum shells reached").

---

## 4. E2E Playwright Tests

File: `tests/e2e/custom-shells.spec.ts`  
Header: `// @feature shell:start, shell:stop, shell:list, shell:restart, shell:delete`

Test server: `http://localhost:8544`

**E2E-1: Spawn a shell via dialog (AC-1.1, AC-1.2)**
```
Given an open session
When the user clicks the "New Shell" button
Then a dialog appears with "Command" and "Working Directory" fields
When the user clicks "Start"
Then a new shell tab appears in the tab strip
```

**E2E-2: New shell tab shows command name and green status (AC-1.3, AC-3.1)**
```
Given a freshly spawned bash shell
Assert the tab label contains "bash"
Assert the status indicator color is green (computed style matches --success token)
```

**E2E-3: Type a command in the shell, see output (AC-2.1, AC-2.2)**
```
Given an active shell tab focused in the terminal
When the user types "echo e2e_marker" and presses Enter
Assert the terminal widget displays "e2e_marker" in its content
Assert the Claude terminal tab is NOT affected (switch to Claude tab, confirm no "e2e_marker" there)
```
Note: this test verifies PTY isolation end-to-end (adversarial HIGH issue A).

**E2E-4: Scrollback preserved while tab open (AC-2.3)**
```
Given a shell that ran "echo scrollback_test"
When the user scrolls up in the terminal widget
Assert "scrollback_test" is visible
```

**E2E-5: Shell crash shows red indicator with exit code (AC-3.2)**
```
Given a shell spawned with command "bash -c 'exit 2'"
Wait for shell to exit
Assert the tab status indicator is red
Assert exit code 2 is displayed on the tab or in a tooltip
```

**E2E-6: Stop button stops a running shell (AC-4.1)**
```
Given a running "sleep 30" shell
When the user clicks "Stop" on the tab
Assert the tab status changes to red/stopped within 3 seconds
Assert the sibling tmux session no longer appears in tmux list-sessions output
```

**E2E-7: Restart after crash relaunches shell (AC-4.2)**
```
Given a stopped shell (exited with code 0)
When the user clicks "Restart"
Assert the tab status changes back to green/running within 3 seconds
Assert the terminal receives fresh output (new bash prompt visible)
```

**E2E-8: Close tab kills process and removes tab (AC-4.3)**
```
Given a running "sleep 60" shell
When the user clicks the "Close" (×) button on the tab
Assert the tab disappears from the tab strip
Assert ListShells RPC returns empty list for this session
```

---

## 5. Security Consideration Tests

**SEC-1: Shell input cannot cross PTY boundary to Claude session (PTY isolation)**
- Covers: adversarial HIGH issue A.
- Go integration test (`INT-3` above) + E2E (`E2E-3` above).
- Additional assertion: inspect file descriptors of the sibling tmux session's PTY vs. the parent session's PTY using `/proc/<pid>/fd` (Linux) or `lsof` (macOS). Assert they reference distinct device files.

**SEC-2: Working directory is validated to be within the session workspace**
- Test: call `StartShell` RPC with `working_dir` set to a path outside the session's workspace (e.g., `/etc`).
- Assert the handler returns `connect.CodeInvalidArgument` or coerces the path to the session workspace root.
- Test: call with `working_dir=""` (empty).
- Assert the working directory defaults to the session's `WorkingDir`.

**SEC-3: Shell command is not run as root**
- Spawn a shell with `command="id"`.
- Assert the output does not contain `uid=0(root)`.
- This is a smoke test; the implementation must rely on the host OS user context (not setuid).

**SEC-4: Shell command is executed via exec.Command, not shell interpolation**
- Code review test (static): search for `ShellTmuxHandle.Spawn` implementation.
- Assert the command is passed as a positional argument to `tmux new-session` (`-- /bin/sh -c <command>`), not interpolated into a shell string.
- Assert no `exec.Command("sh", "-c", fmt.Sprintf("... %s ...", command))` pattern exists.

---

## 6. Race Condition Tests

**RACE-1: Stop-while-streaming does not panic or deadlock (adversarial HIGH issue B)**
- This test is a targeted concurrency test. Run with `go test -race`.
- Procedure:
  1. Spawn a shell with `sleep 60`.
  2. Open `StreamTerminal` in goroutine A (blocked on PTY read).
  3. After 50ms, call `StopShell` from goroutine B.
  4. Assert goroutine A receives a `ShellStatusUpdate{STOPPED}` (not a `TerminalError` or panic).
  5. Assert goroutine A exits cleanly (no data race detected by `-race`).
  6. Assert `StopShell` returns within 2 seconds.
- This covers `Instance.shellsMu` correctness and the per-shell exit channel closed-channel pattern.

**RACE-2: Concurrent ListShells and DeleteShell do not race on the map**
- Goroutine A: calls `ListShellsInMemory()` in a tight loop for 500ms.
- Goroutine B: concurrently calls `DeleteShell` for all shells.
- Run with `go test -race`.
- Assert no data race is flagged.

**RACE-3: Multiple StreamTerminal reconnects to the same shell do not leak goroutines**
- Spawn a shell.
- Open and close `StreamTerminal` 10 times in sequence.
- After the last close, call `runtime.NumGoroutine()`.
- Assert goroutine count returned to baseline (±5 goroutines for background work).

---

## Implementation Readiness Gate

### Gate 1: Every requirement has at least one test case

AC coverage: **18 / 18 (100%)** — see traceability matrix above.  
Verdict: PASS

### Gate 2: plan.md has concrete file-level tasks for every story

Review:
- Story 1.1 (schema): Tasks 1–3 name exact files (`session/ent/schema/shell.go`, `session.go`). PASS
- Story 1.2 (repository): Task 4–8 name `session/ent_repository.go`. PASS
- Story 2.1 (ShellTmuxHandle): Task 9 names `session/tmux/shell_handle.go`. PASS
- Story 2.2 (Instance registry): Tasks 11–18 name `session/instance.go`. PASS
- Story 3.1 (proto): Tasks 19–21 name proto files. PASS
- Story 3.2 (handlers): Tasks 22–28 name `server/services/session_service.go`. PASS
- Story 4.1 (hooks): Tasks 29–31 name `web-app/src/lib/hooks/useShells.ts`. PASS
- Story 4.2 (tab strip): Tasks 32–34 name `SessionDetailView.tsx`. PASS
- Story 4.3 (components): Tasks 35–36 name `ShellTab.tsx` and `NewShellDialog.tsx`. PASS
- Story 4.4 (registry + tests): Tasks 37–40 named. PASS

Concern: Three tasks added by adversarial Challenge 5 (scrollback recorder extension) are not in the plan as explicit numbered tasks. They appear only as inline commentary. These must be tracked as Task 41a–41c:
- 41a: Extend scrollback recorder to accept optional `shellID` parameter.
- 41b: Pass `shellID` from `StreamTerminal` handler when `shell_id` field is set.
- 41c: Apply per-shell scrollback cap (500 lines or configurable).

Without these, INT-4 and AC-5.3 cannot pass.  
Verdict: PASS WITH NOTE (tasks 41a–41c must be added to plan before implementation sprint)

### Gate 3: adversarial-review.md has no BLOCKED verdict

Overall verdict from adversarial review: **CONCERNS** (not BLOCKED).  
No individual challenge has a "BLOCKED" verdict; all have resolution paths.  
Verdict: PASS

### Gate 4: High-severity adversarial issues have corresponding tests in validation.md

| Adversarial Issue | Severity | Test Coverage |
|---|---|---|
| Challenge 1: PTY isolation (sibling sessions) | HIGH | TMUX-1, TMUX-2, INT-3, SEC-1, E2E-3 |
| Challenge 3: Stop-while-streaming race / mutex contract | HIGH | SHELL-1, SHELL-2, SHELL-3, RACE-1, RACE-2 |
| Challenge 2: tmux_session_name field naming | MEDIUM | ENT-4 |
| Challenge 4: ReconcileShells call site | MEDIUM | INT-9 (smoke-tests the behavior) |
| Challenge 5: Scrollback extension missing tasks | MEDIUM | INT-4, INT-10 |
| Challenge 6: watchShellExit goroutine leak | LOW | SHELL-5, RACE-3 |

Verdict: PASS — both HIGH issues have 4+ test cases each

---

## Overall Readiness Gate Verdict: PASS WITH NOTE

| Gate | Result |
|---|---|
| 1. Every AC has a test | PASS |
| 2. Concrete file-level tasks for every story | PASS WITH NOTE (add tasks 41a–41c for scrollback) |
| 3. No BLOCKED verdict in adversarial review | PASS |
| 4. HIGH adversarial issues have tests | PASS |

The feature is ready for implementation with one required action before the sprint begins: add the three scrollback extension tasks (41a–41c) to plan.md. These unblock INT-4 and AC-5.3 and are straightforward (no architectural change required).

---

## Test Suite Summary

| Category | Count |
|---|---|
| Unit — ent schema | 4 (ENT-1 through ENT-4) |
| Unit — ShellTmuxHandle (tmux) | 6 (TMUX-1 through TMUX-6) |
| Unit — exit channel / registry | 5 (SHELL-1 through SHELL-5) |
| Integration | 11 (INT-1 through INT-11) |
| Frontend (Jest/RTL) | 11 (FE-1 through FE-11) |
| E2E (Playwright) | 8 (E2E-1 through E2E-8) |
| Security | 4 (SEC-1 through SEC-4) |
| Race condition | 3 (RACE-1 through RACE-3) |
| **Total** | **52** |

Requirements coverage: 18 / 18 ACs (100%)
