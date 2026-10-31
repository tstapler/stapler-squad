# ADR-036: Cut tmux subprocess spawns with a shared pane-activity snapshot, then move the snapshot onto control mode

Status: Proposed — 2026-10-30

## Context

The polling audit ([`docs/explanation/polling-audit.md`](../explanation/polling-audit.md), PR #1316) measured about 252 tmux spawns and 103 s of
subprocess wall time per minute on the live service, and attributed them to the review-queue poller and the session driver
by inference: the exec gate had no caller label. This ADR replaces that inference with a measurement and decides what to do.

### Measurement

Method: throwaway instance (`STAPLER_SQUAD_INSTANCE`, private socket `ssq-manual-9`, port 62881) with 20 sessions running a stand-in
`claude` that prints once every 30 s; 10-minute windows. Spawns by subcommand and caller come from the new counters
`tmux_exec_total{subcommand,caller}` and `tmux_exec_site_total{subcommand,site}` (`executor/safeexec/tmuxspawn.go`). CPU and wall
time come from a wrapper binary set as `TMUX_BIN` that runs the real tmux and logs `wait4` rusage (user+sys) and wall time per
process. The wrapper adds its own process, so wall times include roughly one extra exec; CPU is the tmux process's own.

Baseline, 20 sessions, per minute (two independent windows, 594.7 and 596.6 capture-pane/min):

| subcommand | spawns/min | wall s/min | CPU s/min |
|---|---|---|---|
| capture-pane | 596.6 | 26.2 | 5.43 |
| list-panes | 15.1 | 0.5 | 0.13 |
| list-sessions | 4.6 | 0.4 | 0.05 |
| display-message | 4.0 | 0.1 | 0.03 |
| total | 620.3 | 27.2 | 5.63 |

| caller (goroutine root) | spawns/min |
|---|---|
| `session.StartSessionDriver` (one per session, 2 s tick) | 546.9 |
| `session.(*ReviewQueuePoller).checkSessions` | 30.2 |
| `sync.(*WaitGroup).Go` (misc: exit-status, dead-pane, list-sessions checks) | 14.2 |
| `session.(*ReviewQueuePoller).pollLoop` | 7.2 |
| `server.(*ReactiveQueueManager).OnControllerStatusChange` | 4.4 |

Findings (all VERIFIED by the table above unless marked):

1. 88% of spawns are the session driver's per-tick `capture-pane`: every driver runs `Instance.PreviewContext` on every 2 s tick
   (`session/session_driver.go`, main loop), so spawns scale as 30 per session per minute. 20 sessions give about 600.
2. The review-queue poller is already batched: one `list-panes -a` per tick, not one call per session.
3. **`#{pane_last_activity}` is not a tmux 3.7b format variable.** `tmux list-panes -a -F '#{session_name} #{pane_last_activity}'`
   prints an empty second column (verified against tmux 3.7b). `batchPaneActivity` therefore always returns an empty map, so the
   poller's documented "skip capture-pane when the pane is unchanged" path never fires on this tmux; it falls back to its 30 s
   TTL. `#{window_activity}` is populated and changes only on output (verified by sampling).
4. The per-spawn CPU is 4–10 ms of tmux CPU, and the wall time is 12 ms p50 and 195 ms p95 for capture-pane under this host's
   load. The cost is the spawn rate, not any single call.

Control mode today, and what it can and cannot replace:

- A per-session `-C attach-session` client exists only while a terminal is being viewed (`session/tmux/control_mode.go`). It handles
  `%output`, `%begin/%end/%error`, `%exit`, `%session-closed` and `%session-changed`; it uses no `refresh-client -B` subscriptions.
  Background captures already go over it when one is attached (`cmEnabledForBackground`), so those cost no spawn.
- `TmuxServerRegistry` (`session/tmux/server_registry.go`) keeps one `-C attach-session -t <keepalive>` client per socket and
  maintains a session-existence map from `%sessions-changed` and `%pane-exited`. It carries no activity, title or dead-pane state.
- Subscriptions cannot cover other sessions. On tmux 3.7b a client attached to session A that runs
  `refresh-client -B 'wa:@*:#{window_activity}'` and `-B 'pd:%*:#{pane_dead}'` receives `%subscription-changed` only for A's own
  window and pane; `-B 'pd:%3:...'` and `-B 'wa:@3:...'` naming another session's pane and window produced no event in 5 s. VERIFIED
  (scripted control client against the throwaway socket). One subscribing client per session would be the viewing model's N
  long-lived processes plus a once-per-second format scan each in tmux's single-threaded server.
- A control client can issue server-wide commands: `list-panes -a -F ...` over the registry's connection answers for every session
  with no new process. VERIFIED (scripted control client).

## Decision

Two phases.

**Phase 1 (this ADR's PR): one shared snapshot, and skip provably-redundant captures.** Behind the `driver_pane_activity_gate`
feature flag, default on (kill switch: set it false and every tick captures as before).

1. `batchWindowActivity` runs `list-panes -a -F '#{session_name} #{window_active}#{pane_active} #{window_activity}'` once and a
   `paneActivityCache` shares the result across all drivers for up to 2 s (one driver tick), with concurrent callers coalescing under the cache
   lock. N sessions cost one spawn per window instead of N captures per tick.
2. Each driver holds a `paneActivityGate`. It reuses its last capture only when all of these hold:
   - the session is in the snapshot with the same `window_activity` as at the last capture;
   - that capture started in a later whole second than the activity (so any later output moves `window_activity` to a later
     second, and the unchanged value proves no output arrived since);
   - the capture is under 30 s old (covers tmux-side screen changes that do not move the activity, such as resize or clear).

   Anything else captures exactly as before: session absent from the snapshot (dead or remote), failed snapshot (cached as nil
   for the same window, so every caller takes its uncached path), flag off, capture error (never cached).
3. Counters and gauges: `tmux_exec_total`, `tmux_exec_site_total`, `tmux_exec_gate_in_flight`, and a Grafana dashboard
   (`ssq-tmux-spawns`). `tmux_exec_gate_wait_duration_ms` and `tmux_exec_gate_exec_duration_ms` already exist per pool.

Window activity covers every pane in the window, so it can over-report change but never miss it. It reads the session's active
window and active pane, the pane `capture-pane -t <session>` targets.

**Phase 2 (proposed, not built): put the snapshot on the registry's control connection.** Add a command channel to
`TmuxServerRegistry` (it already owns the `-C` client and parses `%`-lines) and run the batched `list-panes -a` through it, so the
snapshot costs no spawn. Dead-pane state (`#{pane_dead}`, `#{pane_dead_status}`) and `pane_title` join the same format string, which
lets `BatchPaneDeadStatus` and `ExitStatus` (about 8 spawns/min) read the snapshot too. This needs a per-command response
correlation and timeout in the registry reader that does not exist today, so it is a separate PR.

### Correctness

- **Liveness.** The registry's existence map (`%sessions-changed`, `%pane-exited`, `%session-closed`) stays the source for "does
  the session exist"; the gate never overrides it. A session absent from the snapshot falls through to `PreviewContext`, which
  keeps its own `DoesSessionExist` short-circuit. A session that died and was recreated under the same name gets a new pane and
  an activity at or after the recreate second, so a stale hit would need the old activity value to repeat; INFERRED, not tested against a real recreate.
  The 30 s bound recaptures in any case.
- **Exit status and dead panes.** Unchanged in Phase 1. In Phase 2 they come from `pane_dead`/`pane_dead_status` in the batched
  format; a missed update is bounded by the poll interval.
- **Control-mode disconnect.** Phase 1 does not depend on control mode. Phase 2 falls back to the Phase 1 spawned `list-panes -a`
  while the registry reports unhealthy, and runs one backstop spawn per 30 s while healthy to catch a silently stalled connection.
- **Detection latency.** A driver may act on a snapshot up to 2 s old plus its 2 s tick, so worst-case detection of a new dialog is
  about 4 s against 2 s today. This is the one semantic difference; the 30 s recapture bound limits any stale content to that.
- **Socket scoping.** `listAllPanes` resolves its socket through `tmux.ResolveSocket` and builds args with `Socket.Args`, the form
  `tools/lint/tmuxsocketscope` requires, so `STAPLER_SQUAD_TMUX_SOCKET` applies and a test binary never reaches the shared default
  socket. The counters read the already-built argv and never touch a socket.
- **noarchivedrevival.** The gate only chooses between a cached and a fresh capture; it starts, revives and retries nothing, and
  the driver's existing archival guards are untouched.
- **instance-lock-free-reads.** The gate reads only `inst.GetTmuxSessionName()` and `inst.IsRemote()`; it reads no mutable raw
  `Instance` field.
- **Remote sessions.** The snapshot is the local server's, so the driver passes nil for `IsRemote()` instances and they capture
  every tick as before.

## Alternatives rejected

| Alternative | Why not |
|---|---|
| `refresh-client -B` subscriptions from one monitor client | Verified above: reports only the attached session. |
| One subscribing control client per session | N long-lived processes and N once-per-second format scans in the single-threaded server; it moves the cost, not removes it. |
| Fix `batchPaneActivity` to use `window_activity` for the poller in this PR | Turns on a code path (`lastSeenPaneActivity` equality) that has never run on tmux 3.7b, and that path lacks the same-second guard. Needs its own flag and shadow comparison. Filed as a follow-up. |
| Lengthen the driver tick | Changes detection latency for every session to save spawns that the gate removes without it. |

## Consequences

Expected, from the throwaway instance (20 sessions, 10 minutes, same method as the baseline):

| | before | after |
|---|---|---|
| tmux spawns/min | 620.3 | 147.6 |
| tmux CPU s/min | 5.63 | 1.53 |
| tmux wall s/min | 27.2 | 6.4 |
| capture-pane/min | 596.6 | 96.6 |
| list-panes/min | 15.1 | 42.5 |

The remaining capture-pane are the driver's 30 s bound (38/min in the after window by `StartSessionDriver`) and the poller's captures (51/min; INFERRED to be its 30 s TTL path, the window baseline showed 28/min, so this figure is noisy). The live
service's session count is not known to this ADR, so its absolute saving will differ; the spawn count now scales with output
and with 2 per session per minute instead of 30. Not measured: the pollers' tick duration (no such metric exists) and the live
service (off limits).

Follow-ups: Phase 2 above; fix `batchPaneActivity`'s format variable and harden the poller's equality check; give the gate wait
histogram a `subcommand` label by threading the subcommand into `runGatedWith`.
