# Adversarial Review — Custom Shells Implementation Plan

**Reviewer role:** Adversarial challenger. Every assumption is suspect. Every omission is a bug
waiting to happen.

**Verdict:** CONCERNS (not BLOCKED — no fatal flaws, but three issues require resolution before
implementation begins)

---

## Challenge 1: tmux Named-Window Attach Is Not How `attach-session` Works

**Claim in plan (ADR-1 + Task 9):** `tmux attach-session -t {sessionName}:{windowName}` targets
a specific named window.

**Challenge:** This is incorrect tmux behavior. `attach-session` attaches to the *session*, not to
a specific window. The `-t` target syntax `session:window` is valid only for commands that act on
windows/panes directly (`send-keys`, `kill-window`, `resize-window`, `select-window`). When used
with `attach-session`, the window specifier selects which window is initially *active* in the
attached session — but the attach PTY sees **all output from all windows** or only the current
window depending on the tmux client and session configuration.

The existing `buildAttachCommand()` at line 606 of `tmux.go` already demonstrates the correct
behavior: `attach-session -t {sanitizedName}` — no window suffix. There is only one PTY per
`attach-session` call; the PTY represents the session's currently-active window.

**Consequence:** If `attach-session -t {session}:{shellWindowName}` is used, it will either (a)
fail with "no such window", (b) attach to the session with that window selected, or (c) on some
tmux versions, accept the target but not isolate that window's I/O. In all cases, typing into
the shell PTY would affect whichever window is currently active in the session, which for window 0
(Claude's terminal) would be catastrophic: keystrokes from a shell tab would be injected into
Claude's PTY.

**Required resolution:** The PTY attach strategy must change. Two viable alternatives:

**Option A (Recommended): `new-session` per shell** — spawn each shell as an *independent* tmux
session (not a window in the parent session). Use `tmux new-session -d -s {shellUUID} -c {workDir}
{command}`. Attach via `attach-session -t {shellUUID}`. The shell runs in isolation; stopping the
parent session does not affect shell sessions. This is the cleanest approach and maps directly onto
the existing `TmuxSession.Start()` / `AttachToExisting()` pattern with zero changes to the attach
logic.

Trade-off: shells are no longer "in" the parent session's tmux session — they are sibling sessions.
`tmux list-sessions` will show them. Name them `{parentPrefix}_shell_{shellUUID}` to namespace them.

**Option B: `split-window`** — split the parent session's window 0 into panes, attach to
specific panes via `{session}:{window}.{pane}`. `attach-session` *does* support pane-level targeting
for split-pane layouts. But this changes the Claude terminal layout and is likely unwanted.

**Option A is strongly recommended.** The plan's "named windows" approach requires a different
attach mechanism than exists today. Using sibling sessions instead of windows eliminates the
attach complexity entirely and reuses the existing, battle-tested `TmuxSession` infrastructure
without modification.

**Severity: HIGH** — the stated approach (named windows + `attach-session -t session:window`) is
architecturally incorrect. The plan must be revised to use sibling sessions or a different
attach strategy before implementation.

---

## Challenge 2: ent Schema Field Type — `tmux_window_name` vs Actual PK

**Claim in plan (Task 1):** `id` is a string PK (UUID), `tmux_window_name` stores the UUID used
as the tmux window/session name.

**Challenge:** These are the same value. The shell UUID *is* the tmux window name (or session name
under Option A). Storing it twice (once as `id` and once as `tmux_window_name`) is redundant if
they are always equal. This is confusing at the implementation layer: which field does
`ReconcileShells` query by? Which field does `StopShell` pass to tmux?

More specifically: if the plan adopts Option A (sibling sessions), the tmux session name would be
`{parentPrefix}_shell_{shellUUID}`, which is *derived* from the UUID, not equal to it. In that
case, `tmux_window_name` becomes a misnomer (it is a session name) and the field would need to be
renamed to `tmux_session_name` and store the full computed name, not the bare UUID.

**Required resolution:** Clarify in the schema whether the tmux identifier is derived or stored,
and rename the field appropriately. Remove redundancy if `id` == the tmux identifier with a
deterministic prefix.

**Severity: MEDIUM** — schema confusion leads to reconciliation bugs on restart.

---

## Challenge 3: Stop-While-Streaming Race Condition Is Incompletely Specified

**Claim in plan (Task 13, Task 22):** "Stop-while-streaming race guard: treat read error as EOF
if `shells[shellID].Status` is already `ShellStatusStopped`." The stream handler "sends a clean
exit event rather than a `TerminalError`."

**Challenge:** The proposed guard has a TOCTOU window. The sequence is:

1. `StreamTerminal` handler enters PTY read loop.
2. `StopShell` is called concurrently.
3. `StopShell` sets `Status = ShellStatusStopped` under lock.
4. `StopShell` calls `handle.Close()` → closes `ptmx`.
5. Read loop in step 1 gets a read error from closed `ptmx`.
6. Handler reads `shells[shellID].Status` — sees `ShellStatusStopped`, emits `ShellStatusUpdate{STOPPED}`.

This seems fine. But the plan does not specify:
- **What mutex guards the `shells` map when the stream handler reads from it?** The handler runs
  in a goroutine separate from the service layer. If `StopShell` deletes the shell from the map
  (`DeleteShell` calls `StopShell` then removes from map), the handler in step 6 may panic with
  a nil map read.
- **What happens if `GetShellPTYReader` is called while `StopShell` is mid-execution?** A
  new `StreamTerminal` connection could call `GetShellPTYReader` on a shell being torn down,
  getting a closed file descriptor.
- **The per-shell exit channel** referenced in Task 13 ("notifies any active `StreamTerminal`
  subscribers via a per-shell exit channel") is not defined anywhere in the plan. How is this
  channel created, who owns it, who closes it, and how does the stream handler subscribe without
  a race on the channel itself?

**Required resolution:** The plan needs a concrete synchronization contract for the shell registry:
- `Instance.shellsMu` (separate from `spawnMu` and `stateMutex`) guards read/write to `shells`
  and `shellHandles` maps.
- The per-shell exit channel must be defined in the `Shell` struct, initialized in `SpawnShell`,
  and closed (not sent-to) by `watchShellExit` on exit — so any number of subscriber goroutines
  can range-select on it without coordination.
- `DeleteShell` must not remove from map until all active `StreamTerminal` handlers for that shell
  have exited. A `sync.WaitGroup` per shell handle (similar to `TmuxSession.wg`) is the standard
  pattern.

**Severity: HIGH** — without this the implementation will have data races that may not be caught
by the race detector in integration tests (they depend on timing).

---

## Challenge 4: `ReconcileShells` Timing — When Is It Called?

**Claim in plan (Task 18):** "`ReconcileShells` is called after Instance is loaded from ent on
startup (e.g., in the existing service bootstrap)."

**Challenge:** The plan says "e.g." — it does not specify which code path triggers reconciliation.
The existing bootstrap does not have a dedicated "post-load hook" for each Instance. The
`WorkspaceService` and `EntRepository.Load` do not call Instance methods post-load. If
`ReconcileShells` is never called, shell in-memory state is never rebuilt and all shells appear
as stopped after restart even if their tmux sessions are still running.

**Required resolution:** Identify the exact call site. The most natural place is in
`WorkspaceService.LoadInstances()` (or whichever code loads sessions on startup) — after each
`Instance` is populated from ent, call `instance.ReconcileShells(ctx)`. This call site must be
named explicitly in the plan (exact file and function).

**Severity: MEDIUM** — if unspecified, the implementation phase will skip it or add it in the
wrong place, resulting in shells being lost on restart.

---

## Challenge 5: Scrollback Infrastructure for Shells

**Claim in plan (Task 22 + overall):** Shell scrollback uses the existing infrastructure keyed by
`{sessionID}/{shellID}`.

**Challenge:** The plan contains zero tasks for actually implementing this. The existing scrollback
infrastructure keys data by session ID (or title). The `StreamTerminal` handler writes to this
store. Extending the key to `{sessionID}/{shellID}` requires changes to:
- The scrollback recorder: it must receive the shell ID alongside the session ID.
- The scrollback replayer: when a shell's `StreamTerminal` opens, it must replay from the
  shell-scoped key.
- The scrollback cap: the plan mentions a smaller per-shell cap (500 lines) but adds no task to
  implement it.

None of these appear as explicit tasks in Epics 1–4. The claim in Task 22 ("All downstream code
(flow control, scrollback, output loop) is reused unchanged") is incorrect — the scrollback key
resolution must change for shells.

**Required resolution:** Add explicit tasks under Epic 3 for:
- Extend scrollback recorder to accept an optional `shellID` parameter.
- Pass `shellID` from the `StreamTerminal` handler when present.
- Apply per-shell scrollback cap.

**Severity: MEDIUM** — without this, shells have no scrollback replay on reconnect, breaking
AC-5.3 (output visible after resume).

---

## Challenge 6: `watchShellExit` Goroutine Leak on Server Shutdown

**Claim in plan (Task 13):** `watchShellExit` goroutine "reads from `handle.GetPTY()` until
EOF/error."

**Challenge:** If the server shuts down while shells are running, these goroutines block on PTY
read indefinitely. The existing `TmuxSession` uses `ctx` and `cancel` to coordinate shutdown.
The plan does not mention passing a context to `watchShellExit` or how these goroutines are
joined during shutdown.

**Required resolution:** `watchShellExit` must accept a `ctx context.Context` parameter. The
Instance's lifecycle context (passed to `SpawnShell`) should propagate. On context cancellation,
the goroutine exits via `select { case <-ctx.Done(): return; default: ... }` or by closing the
PTY file, whichever is simpler. This is a minor addition but must be stated explicitly.

**Severity: LOW** — server shutdown is not a common path, but goroutine leaks compound across
restarts in process-supervisor scenarios.

---

## Confirmations (Plan Gets Right)

- Field numbering: `shell_id = 17` outside oneof, `shell_status_update = 18` inside oneof —
  verified against actual `events.proto` (oneof occupies fields 2–16). Correct.
- ent auto-migrate is safe: confirmed from stack.md and existing `client.Schema.Create` usage.
- `spawnMu` for serializing `SpawnShell` — correctly identified spawn race.
- Soft cap of 8 shells (pool size matching) — practical and aligned with existing pool limit.
- `SessionDetailTab` union stays static — correct architectural choice.
- Feature registry update required — correctly identified.
- Security: `exec.Command` (not shell interpolation) for `tmux new-window` — correctly specified.

---

## Summary Table

| Challenge | Severity | Verdict |
|---|---|---|
| Named-window attach is architecturally wrong | HIGH | Requires plan revision — adopt sibling sessions or alternative PTY strategy |
| `tmux_window_name` field redundancy/misnomer | MEDIUM | Clarify naming in schema |
| Stop-while-streaming race incompletely specified | HIGH | Add explicit mutex contract and per-shell exit channel definition |
| `ReconcileShells` call site unspecified | MEDIUM | Name exact file and function |
| Scrollback extension has no tasks | MEDIUM | Add 3 explicit tasks |
| `watchShellExit` goroutine leak on shutdown | LOW | Pass context; document join |

**Overall verdict: CONCERNS**

Two HIGH-severity issues (tmux attach architecture, race condition specification) must be
resolved in a plan revision before implementation begins. The HIGH issues do not make the feature
unimplementable, but they mean the current plan would produce broken shell PTY routing and
potential data races if followed literally.

Recommended path: revise plan to adopt sibling tmux sessions (Option A from Challenge 1), update
`ShellTmuxHandle` accordingly, add explicit mutex contract, and add scrollback extension tasks.
Then the plan reaches CLEAN status.
