# ADR-037: Cut tmux subprocess spawns with a shared pane-activity snapshot, then move the snapshot onto control mode

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

Baseline, 20 sessions, per minute (this table is one 10-minute window; a second window measured 594.7 capture-pane/min against this one's 596.6):

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
| other (below 0.5/min each: registry, runActor, main) | 17.4 |

Findings (1, 2 and 4 are VERIFIED by the tables above; 3 by the tmux command shown):

1. 88% of spawns are the session driver's per-tick `capture-pane`: every driver runs `Instance.PreviewContext` on every 2 s tick
   (`session/session_driver.go`, main loop), so spawns scale with session count: 27.3 per session per minute measured (one capture per 2 s tick would be 30). capture-pane is 96% of all spawns; the driver's share is 88%.
2. The review-queue poller is already batched: one `list-panes -a` per tick, not one call per session.
3. **`#{pane_last_activity}` is not a tmux 3.7b format variable.** `tmux list-panes -a -F '#{session_name} #{pane_last_activity}'`
   prints an empty second column (verified against tmux 3.7b). `batchPaneActivity` therefore always returns an empty map, so the
   poller's documented "skip capture-pane when the pane is unchanged" path never fires on this tmux; it falls back to its 30 s
   TTL. `#{window_activity}` is populated and changes only on output (verified by sampling).
4. The per-spawn CPU is about 9 ms of tmux CPU (5.43 s / 596.6), and the wrapper log gives capture-pane wall time of 12 ms p50 and
   195 ms p95 (5,966 samples) under this host's load. The cost is the spawn rate, not any single call.

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

**Phase 1 (the PR carrying this ADR): one shared snapshot, and skip provably-redundant captures.** Behind the `driver_pane_activity_gate`
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

The capture reaches tmux through `TmuxProcessManager.CapturePaneContentContext`, which serves a cached result for up to
`capturePaneCacheTTL` (1 s), stamped when the capture finishes. A capture that began at T may therefore show the pane as of T
minus 1 s minus the capture latency (up to 3 s, the control-mode capture timeout). The gate requires the capture start minus 4 s
to fall in a later whole second than the activity. Two review rounds found counterexamples to earlier, narrower margins;
`TestPaneActivityGate_ParityUnderCaptureCacheStaleness` enumerates output offset, cache warm-up offset, tick phase and capture
latency. An empty capture (what `previewBlocked` returns for a paused or not-started instance) is never cached, and a blocked
instance bypasses the gate (`driverPreview`).

Window activity covers every pane in the window, so it over-reports output but does not move on a pane or window *selection*: after
`select-pane` the active pane changes while `window_activity` stays put (reported by review, tmux 3.7b), so a gate hit can show the
previous pane for up to the 30 s bound. Sessions here run one pane; the bound is accepted rather than fixed. It reads the session's active
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
- **Cache staleness and blocked previews.** Covered above: the gate allows for the 1 s capture cache and never caches an empty preview, so a
  resumed instance captures on its first tick.
- **Detection latency.** A driver may act on a snapshot up to 2 s old plus its 2 s tick, so worst-case detection of a new dialog is
  about 4 s against 2 s today. After output, the gate recaptures for about 4 more seconds before trusting a capture again.
  Pane or window selection (above) and a hung `list-panes` are the remaining differences: a hung one holds the cache lock for up to
  its 5 s timeout, which delays driver shutdown by that much because the snapshot call ignores the driver's ctx.
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

Measured on the throwaway instance with the final code (20 sessions, one 10-minute window, same method as the baseline). Spawn counts
are comparable across runs; CPU and wall time are not, because host load differed (capture-pane wall time was 44 ms/call in the baseline
window and 11.9 ms/call in this one), so the CPU row is shown per spawn.

| | before | after |
|---|---|---|
| tmux spawns/min | 620.3 | 198.9 |
| capture-pane/min | 596.6 | 147.5 |
| list-panes/min | 15.1 | 42.8 |
| tmux CPU per spawn | 9.1 ms (5.63 s/min over 620 spawns) | 5.2 ms (1.03 s/min over 199 spawns) |

Earlier builds of the gate with narrower staleness margins measured 147.6 and 146.8 spawns/min; the 4 s margin costs about 50 more
spawns/min because each output now triggers a few more recaptures (INFERRED from the delta; the stand-in prints every 30 s). The
remaining capture-pane are output-triggered recaptures, the 30 s bound, and the review-queue poller's own captures. The live
service's session count and output rate are not known to this ADR, so its absolute saving will differ. Not measured: the pollers'
tick duration (no such metric exists) and the live service (off limits).

Follow-ups: Phase 2 above; fix `batchPaneActivity`'s format variable and harden the poller's equality check; give the gate wait
histogram a `subcommand` label by threading the subcommand into `runGatedWith`.
