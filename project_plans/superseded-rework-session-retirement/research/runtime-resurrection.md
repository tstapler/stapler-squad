# Runtime Resurrection — Empirical Verification (2026-09-14)

Read-only investigation against the live service (PID 2837765, `:8543`), its rotated
JSON logs, a scratchpad copy of `sessions.db`, and `ps`/`tmux ls`. Nothing was written,
killed, or restarted.

**Headline:** the diagnosis is **confirmed but must be re-centered**. The revival engine is
not startup deserialization — it is `SessionHealthChecker` (`session/health.go`), a 15 s
ticker that runs forever and **really creates a new tmux session and a new `claude`
process**. The startup sites are secondary. Wave 2's 60 s cycle is a *mutual fight*
between that health checker and a second 60 s sweep
(`reconcileTerminalItemSessions`) that re-kills the same panes on every tick.

---

## Task A — Timeline

| Fact | Value | Source |
|---|---|---|
| Last service restart | **2026-09-13 23:52:52 PDT** | `systemctl --user show stapler-squad` → `ExecMainStartTimestamp=Sun 2026-09-13 23:52:52 PDT`, `NRestarts=0`, `MainPID=2837765` |
| Process still up | `2837765 Sun Sep 13 23:52:51 2026 09:52:28 …/stapler-squad --remote-access --tmux-keep-server --profile --profile-port 6060` | `ps -eo pid,lstart,etime,cmd` |
| Every log record's PID tag | `[pid-2837765-1789368772]` throughout | `logs/staplersquad*.log` |
| Wave 2 (`gate-request-review-on-ac-completion-r5/r6/r7`) | continuously looping from **≤ 00:43:25** to **09:44:58** (log coverage limits, not loop limits) | log |
| Wave 1 (`add-durable-guidance-request-r3/r4/r6/r7/r8/r9`) | single revival burst **09:42:55.83 → 09:43:11.07** | log |
| Wave 3 — *not reported by the operator* (`fix-fork-pressure-flap-and-status-banner-r3…r7`) | identical revival burst **01:55:10.81 → 01:55:26.15** | log |

**VERIFIED: both waves (and a third) occurred after the 23:52:52 restart with no restart in
between.** `NRestarts=0` and the single unchanging PID in every log line make this airtight.

Log-coverage caveat: rotation is size-based (10 MB). The oldest retained record is
`2026-09-14T00:42:52`, so the 23:52–00:42 window (including the startup banner and any
earlier operator kill of r3/r4/r8) is gone. The process-start timestamp above is the
authoritative restart evidence, not the banner.

---

## Task B — Wave 1 mechanism

### The emitting path

```
{"time":"2026-09-14T09:42:55.830156003-07:00","level":"WARN",
 "msg":"health check found issues for session",
 "session":"stapler-squad-add-durable-guidance-request-r3",
 "issues":["Instance marked as started but tmux session doesn't exist"]}
```

- `"health check found issues for session"` → `session/health.go:153` (`checkInstances`).
- `"Instance marked as started but tmux session doesn't exist"` → `session/health.go:332`
  (`recoverMissingSession`). This string exists **nowhere else** in the repo.

15 s later, the debounce (`failureThreshold = 2`, `health.go:35`) is satisfied and
`recoverMissingSession` calls `instance.Start(false)` (`health.go:344`):

```
{"time":"2026-09-14T09:43:10.817769877-07:00","msg":"starting instance","session":"stapler-squad-add-durable-guidance-request-r3",…}
{"time":"2026-09-14T09:43:10.830336025-07:00","msg":"cold restoring with --resume","session":"…-r3","uuid":"cdfede64-ea15-40e4-92ee-4440c0bcc1ee","path":"…/worktrees/triage-1c08da73-…"}
{"time":"2026-09-14T09:43:10.851423740-07:00","msg":"tmux session created successfully","session":"staplersquad_stapler-squad-add-durable-guidance-request-r3","program":"env HISTFILE=… /home/tstapler/.local/bin/claude --resume 'cdfede64-…' --mcp-config …"}
```

Only then does `reconcileSessions` react:

```
{"time":"2026-09-14T09:43:12.191087405-07:00","msg":"reconcileSessions: stopped session found alive, reviving to Active","session":"…-r3",…}   # review_queue_poller.go:548
```

### Was a real process spawned? Yes — not a DB flip

```
$ ps -eo pid,lstart,etime,args | grep cdfede64
2554441 Mon Sep 14 09:43:09 2026  08:50 /home/tstapler/.local/bin/claude --resume cdfede64-… X-Stapler-Session-UUID:9001a2e3-…
2554503 … 09:43:09 …  e2978dc7-…
2554569 … 09:43:09 …  faee4b8a-…
2554649 … 09:43:09 …  9bcb0183-…
2554767 … 09:43:10 …  7be2889c-…
2554889 … 09:43:10 …  11d36b9a-…

$ tmux ls
staplersquad_stapler-squad-add-durable-guidance-request-r3: 1 windows (created Mon Sep 14 09:43:10 2026)
…r4, r6, r7, r8, r9 — all created Mon Sep 14 09:43:10–11 2026
```

**Six live `claude` agents, six fresh tmux sessions, all resuming the same conversation
`cdfede64-…` in the same worktree, each with its own MCP subprocess fleet.** This is the
severity shape of the 2026-07-29 OOM, not a cosmetic UI row.

### The crux: how does a tmux-liveness-keyed path revive a killed session?

It is **not** keyed on tmux being alive. It is keyed on tmux being **missing**:

`health.go:296` — `if !instance.TmuxAlive() { h.recoverMissingSession(instance, result) }`.

The operator's kill is the *trigger*, not an obstacle. `reconcileSessions`
(`review_queue_poller.go:548`) is downstream cosmetics — it flips the live row to Active
*because* the health checker had already recreated the pane 2 s earlier.

### Why the "Stopped is terminal" guard does not fire

`healthCheckSkipReason` (`health.go:225-244`) skips only `status.IsSuspended()` —
Paused / Hibernated / Stopped / Crashed / PermanentlyFailed. It has **no `ArchivedAt`
check**. The health checker loads its instances from the DB
(`CheckAllSessions` → `h.storage.LoadInstances()`, `health.go:88`), and the persisted
status of every zombie is **Active**:

```sql
-- scratchpad copy of sessions.db
SELECT title, status, archived_at FROM sessions WHERE archived_at IS NOT NULL AND status=1;
stapler-squad-add-durable-guidance-request-r3          1  2026-09-12 18:18:11+00
…-r4 / -r6 / -r7 / -r8 / -r9                           1  (archived)
stapler-squad-fix-fork-pressure-flap-and-status-banner-r3…r7   1  (archived)
stapler-squad-gate-request-review-on-ac-completion-r5/r6/r7    1  (archived)
-- 14 rows
```
(`Active Status = 1`, `Stopped = 3`, `PermanentlyFailed = 7` — `session/instance.go:45,49,67`.)

This is a **clean natural experiment**: in the same `add-durable-guidance-request` family,
`r5`, `r10`, `r11` are persisted `status = 3` (Stopped) and were **not** revived;
`r3/r4/r6/r7/r8/r9` are persisted `status = 1` (Active) and **all six** were. The single
differentiator is the persisted status, and the missing guard is `ArchivedAt` —
which is set on all of them.

Two further enablers, both verified in source:

1. **`session/instance_serialization.go:509-510`** — for an archived session the
   deserializer takes the fast path and unconditionally does `instance.started.Store(true)`
   (a fork-pressure optimisation, on the stated assumption that "there is no scenario where
   an archived session's pane is secretly still alive"). That force-sets exactly the
   `instance.Started()` precondition `checkSingleSession` (`health.go:284`) needs before
   calling `checkTmuxHealth`. The archived fast path *feeds* the revival path.
2. **`reconcileSessions`' `Active→Stopped` transition is not persisted for these rows.**
   `fix-fork-pressure…-r3` was transitioned to Stopped at `09:27:23.374` yet its DB
   `updated_at` is `16:26:56 UTC` (= 09:26:56 PDT) with `status = 1`. So the DB says Active
   forever, and every 15 s health tick re-reads Active. *(UNVERIFIED: the exact reason the
   write does not land — the 1.4 GB DB copy was taken from a live writer and could be torn.
   The log-only argument above is independent of this and stands on its own: the health
   checker logged at `health.go:153`, which is unreachable for a suspended status, so the
   instance it loaded was non-suspended.)*

---

## Task C — Wave 2 mechanism (the "short-lived" cluster)

**It did not self-terminate. It is an unbounded 60-second thrash loop, running for at least
9 hours.** The operator observed one lap.

One full cycle for `…-r5` (`08:00`–`08:03`, identical every minute):

| Time | Event | Emitter |
|---|---|---|
| `08:00:10.818` | `health check found issues for session` (debounce 1/2) | `health.go:153` / `:332` |
| `08:00:25.815` | `starting instance` → `cold restoring with --resume` → `tmux new-session command succeeded` → `tmux session created successfully` | `health.go:344` `Start(false)` |
| `08:00:28.570` | `reconcileSessions: stopped session found alive, reviving to Active` | `review_queue_poller.go:548` |
| `08:00:56.588` | `successfully killed tmux session` (`staplersquad_…-r5`, then r6, r7 within 6 ms) | `session/tmux/tmux.go:1802` |
| `08:00:58.961` | `reconcileSessions: managed session not found in live sessions, transitioning to Stopped` | `review_queue_poller.go:505` |
| `08:01:10.815` | … cycle repeats | |

### What kills them — identified

```
[pid-2837765-1789368772] INFO:2026/09/14 07:34:56 backlog_lifecycle_archive.go:144:
  [BacklogLifecycle] reconcileTerminalItemSessions: processed 1093 work/review session(s) across 189 terminal item(s)
[pid-2837765-1789368772] INFO:2026/09/14 07:35:56 … (identical, every 60 s)
```

`reconcileTerminalItemSessions` (`session/backlog_lifecycle_archive.go:110-146`) fires at
`HH:MM:56` every minute — **exactly** the kill timestamps (`08:00:56.588`, `08:01:56.646`,
`08:02:56.557`). It is the `archive_terminal_sessions` detector inside
`BacklogLifecycleListener.ReconcileStuck`, driven by the **60 s ticker at
`server/dependencies.go:1281-1289`**. For every backlog item in `done`/`archived` it calls,
per session, `ArchiveSessionByUUID` (CAS no-op when already archived) and then
**`KillTmuxPaneOnly` unconditionally** (`backlog_lifecycle_archive.go:137`) — every tick,
1093 sessions, forever.

### Why wave 2 loops but wave 1 doesn't — the backlog item's status

```sql
SELECT s.title, i.session_role, b.title AS item, b.status
FROM sessions s JOIN item_sessions i ON i.session_uuid=s.uuid
JOIN backlog_items b ON b.id=i.backlog_item_item_sessions
WHERE s.archived_at IS NOT NULL AND s.status=1;
```

| Session cluster | Backlog item | Item status | Behaviour |
|---|---|---|---|
| `add-durable-guidance-request-r3/r4/r6/r7/r8/r9` | Durable structured Q&A / form mechanism… | **review** | Not swept by `reconcileTerminalItemSessions`. Revived once, by the health checker, when the operator killed the pane. Still running now. |
| `gate-request-review-on-ac-completion-r5/r6/r7` | Backlog-driven sessions: self-validate completion… | **done** | Killed every 60 s by `reconcileTerminalItemSessions`, resurrected every 60 s by the health checker. Infinite loop. |
| `fix-fork-pressure-flap-and-status-banner-r3…r7` | Fork Pressure notification flapping… | **done** | Same shape; revived once at `01:55:10`, killed at `09:27:05`. Currently quiescent. |

**The "less severe, self-terminating" reading is wrong.** Wave 2 is *more* severe: same
resurrection defect as wave 1, plus a second sweep that re-triggers it once a minute.

**Anomaly (UNVERIFIED):** the `fix-fork-pressure` cluster has been archived + persisted-Active
with no pane and no process since `09:27:05` and has *not* been revived in the 23 minutes to
the DB snapshot, despite ~92 health ticks. No `health check: tmux server is down, skipping
session check` records explain it. Five sessions remain primed in exactly the wave-1 state;
treat this as a latent third wave, not as evidence of a guard that works.

---

## Complete periodic-sweep inventory

Every `time.NewTicker` / `time.Tick` / `time.AfterFunc` loop in `session/` and `server/`
(non-test). Grouped by whether it can change session or backlog-item state.

### A. Sweeps that can create, kill, or transition a session — the risk surface

| # | Sweep | Wired at | Interval | Acts on | `ArchivedAt`? | Terminal-item? | Current-round? |
|---|---|---|---|---|---|---|---|
| 1 | `SessionHealthChecker.ScheduledHealthCheck` → `recoverMissingSession` → **`Start(false)`** | `server/server.go:193`; loop `session/health.go:508` | **15 s** | Every instance from `LoadInstances()`; **creates tmux + `claude` process** | **NO** (`healthCheckSkipReason:225-244` only `IsSuspended()`) | **NO** | **NO** |
| 2 | `SessionHealthChecker` → `handleDeadPane` / `respawnWithinGraceWindow` (`health.go:366`,`:431`) | same | 15 s | Kills + respawns pane, or marks Crashed/Stopped | **NO** | NO | NO |
| 3 | `ReviewQueuePoller.reconcileSessions` (`review_queue_poller.go:450`) | `ReconcileInterval` `:50` | **30 s** | Status flips only: Stopped→Active `:548`, Hibernated→Active `:575`, Crashed→Active `:598`, Active→Stopped `:505` | **NO** (the `ArchivedAt` check at `:771` is `shouldSkipSession`, which guards the *review-queue* path only, never this switch) | NO | NO |
| 4 | `ReviewQueuePoller` main loop → `checkSession` (`:776`) | `PollInterval` `:45` | 2 s fast / 8 s slow | Review-queue membership | **YES** (`shouldSkipSession:762-771`) | n/a | n/a |
| 5 | `ReconcileStuck` → **`archive_terminal_sessions`** → `reconcileTerminalItemSessions` (`backlog_lifecycle_archive.go:110`) | `server/dependencies.go:1281` | **60 s** | **`KillTmuxPaneOnly` on every work/review session of every done/archived item, unconditionally, every tick** (1093 sessions × 189 items observed) | archive is CAS-idempotent; **the kill is not** | scope *is* terminal items | **NO** |
| 6 | `ReconcileStuck` → review-gate respawn (`FindReviewItemsWithoutGate` → `spawnReviewGate`, `backlog_lifecycle.go:1178-1216`) | same ticker | 60 s | **Spawns a review session** for any `review` item lacking a gate | NO | n/a | NO |
| 7 | `ReconcileStuck` → `auto_archive_done` → `archiveStaleDoneItems` (`backlog_lifecycle_archive.go:75`) | same | 60 s | `done` → `archived` after `maxDoneAge` = 3 d | n/a | yes by construction | n/a |
| 8 | `ReconcileStuck` → 15 further detectors: `work_commit_refresh`, `stale_work`, `rework_blocked_stale`, `respawn_blocked_active`, `abandoned_review`, `orphaned_agent_pr`, `unprocessed_review_verdict`, `bouncing`, `push_failed`, `orphaned_triage`, `orphaned_triage_remediation`, `gate_timeout`, `plan_not_approved`, `self_heal`, `multi_reason_escalation` (`backlog_lifecycle.go:1237-1378`) | same | 60 s | Mark/resolve stuck rows; several retry spawns (`orphaned_triage_remediation`, `push_failed`) | NO | varies | NO |
| 9 | `HibernationSweeper.Start` (`hibernation_sweeper.go:216`, `sweepInterval:29`) | — | **5 min** | Hibernates idle / memory-pressure sessions (kills tmux) | **NO** | NO | NO |
| 10 | `OrphanedTmuxSweeper.Start` (`orphan_tmux_sweeper.go:75`) | — | **5 min** | Kills `staplersquad_*` tmux with no live-registry row (`minAge` 5 min) | n/a (keyed on registry membership) | NO | NO |
| 11 | `SessionRetentionSweeper.Start` (`server/services/session_retention_sweeper.go:46`) | — | **1 h** | Deletes archived sessions past retention | **YES** — `ArchivedAt` is its entire scope (`:99`) | PR-terminal check `:132` | sibling-worktree check `:190` |
| 12 | `StaleCreationSweeper.Start` (`server/services/stale_creation_sweeper.go:63`) | — | 60 s | `Creating` → `Failed/Stale` | NO | n/a | n/a |
| 13 | `SessionDriver` loop (`session/session_driver.go:309`, `driverPollInterval:44`) | per session | 2 s | Drives autonomous session input | n/a | n/a | n/a |
| 14 | `tmux` zombie detector / reaper (`session/tmux/zombie_detector.go:95`, `zombie_reaper.go:30`) | — | configurable | Reaps zombie tmux child processes | n/a | n/a | n/a |

### B. Periodic loops that observe or notify but never change session lifecycle

`StaleSessionNotifier` 60 s (`stale_session_notifier.go:71`, notification only — its own doc
comment says so); `CapacityMonitor` 60 s; `MemoryPressureNotifier` 60 s; `PRStatusPoller`
60 s; `WorktreePRPoller` 60 s; `JulesSessionPoller` 60 s; `HistoryLinker` 5 s;
`git_worktree_watcher` 15 s; `BacklogSync` 15 min; `HostAdvertiser` 5 min;
`workspace/notify` 5 s; `pty_discovery`; `external_tmux_streamer`; `mux/discovery`;
`unfinished/scanner` (maintenance + `pruneRepoInterval` 5 min); `detection/plugin_watcher`
60 s; `pi_extension_health` 2 min; `hook_receiver_drift` 3 min; `server/auth/session` 10 min;
`server/analytics/retention` 1 h; `server/workflows/retention`;
`server/notifications` (`orphanPruneInterval` 1 min, coalesce 500 ms);
`analytics_store` 5 s; `approval_handler` 30 s; `connectrpc_websocket` 1 s;
`cdp_stream_handler` 66 ms; `escape_event_batch_writer` 500 ms;
`scrollback/manager` flush; `sshremote/health_prober` 15 s; `tmux/fork_metrics`;
`dependencies.go:1696` 30 min.

**Answer to "does each check ArchivedAt / terminal / current-round":** of the 14 sweeps in
group A, exactly **two** have an `ArchivedAt` check (#4 review-queue membership, #11
retention), **none** checks "is this the current rework round", and only #5/#7/#11 look at
terminal state at all.

---

## Task D — Verdict on the four-guard plan

### Do the four guards cover both waves?

| Wave | Caught by | Verdict |
|---|---|---|
| Wave 1 (`add-durable-guidance-request`) | **Guard 4** — `session/health.go` `healthCheckSkipReason` → `checkTmuxHealth` → `Start(false)`. All six rows have `archived_at` set, so an `ArchivedAt` skip stops it. | **Covered** |
| Wave 2 (`gate-request-review-on-ac-completion`) | **Guard 4 again** — same path, same rows, same `archived_at`. Guard 3 (`reconcileSessions`) only stops the *status flip*, not the process spawn. | **Resurrection covered; the 60 s kill churn is not** |
| Wave 3 (`fix-fork-pressure…`, unreported) | Guard 4 | Covered |

So the four sites are **sufficient to stop every observed resurrection**, and guard 4 alone
does all the work for all three waves. Guards 1 and 2 (startup) were not exercised at all in
this 10-hour window.

### Does the root cause change?

**Confirmed, but the emphasis is wrong and must be corrected in the plan.**

- Not "startup only, plus two ticker paths". The **only** path that actually resurrected
  anything in 10 hours of uptime is `session/health.go`. It is the primary defect.
- `reconcileSessions` (`review_queue_poller.go:548`) is a **downstream symptom**, not a
  cause: in every observed instance it fires 2 s *after* `health.go` has already created the
  tmux session. The prior research's "1584 of 1641 revival records were three archived
  zombies" measured the echo, not the source. Fixing only `reconcileSessions` would hide the
  zombies from the status column while six `claude` processes kept running.
- Add one enabler the plan does not mention: **`session/instance_serialization.go:509-510`**
  force-sets `started = true` for every archived instance on every `LoadInstances()`. That is
  precisely the precondition `health.go:284` requires. The comment there asserts "there is no
  scenario where an archived session's pane is secretly still alive" — true, and that is
  exactly why the health checker then tries to *make* one. Guard 4 is mandatory *because of*
  this line; alternatively, do not set `started` for archived instances.

### Is there a fifth site?

**Yes — one, and it is not a revival guard.**

**Site 5: `session/backlog_lifecycle_archive.go:137`** (`reconcileTerminalItemSessions`,
driven by `server/dependencies.go:1281`, 60 s).
`ArchiveSessionByUUID` is CAS-idempotent; `KillTmuxPaneOnly` on the next line is **not**. It
is re-issued for all 1093 work/review sessions of all 189 terminal items on every tick,
forever — verified by the identical `processed 1093 … across 189 terminal item(s)` line
repeating at `HH:MM:56` for hours. That unconditional kill is the second half of wave 2's
loop. The guard belongs on the kill: skip when the session is already archived *and* its pane
is already gone (or when a prior tick already handled it), so the sweep converges instead of
oscillating.

### For wave 2 specifically — `ArchivedAt` guard or terminal-item check?

**Both, for two different defects — and they are not interchangeable.**

- **The resurrection** is an `ArchivedAt` problem, not a terminal-item problem. Evidence:
  wave 1's item is in **`review`**, not a terminal status, and it was resurrected by the
  identical code path. A terminal-item check in `health.go` would therefore miss wave 1
  entirely, while an `ArchivedAt` check catches all 14 zombie rows (every one has
  `archived_at` set). **`ArchivedAt` is the correct guard for `health.go`.**
- **The 60 s churn** is a terminal-item-sweep idempotence problem. Evidence: the sweep's
  scope is already terminal items only — it is the *unconditional re-kill* within that
  scope that is wrong, not the scope. An `ArchivedAt` check there would be a no-op, since
  every session it touches is archived by its own preceding line.

Fix `health.go` and the resurrection stops everywhere (wave 2 degrades to a harmless
kill-a-corpse-every-minute). Fix only `reconcileTerminalItemSessions` and wave 2 stops
looping but wave 1 is untouched and any future operator kill resurrects six agents again.

---

## Evidence index

| Claim | How to re-derive |
|---|---|
| Restart time, no restarts | `systemctl --user show stapler-squad --property=ExecMainStartTimestamp,MainPID,NRestarts` |
| Wave timings | `zcat ~/.stapler-squad/workspaces/d685c4b1a423cca3/logs/*.gz` + `jq 'select(.msg\|test("health check found issues\|starting instance\|killed tmux"))'` |
| Real processes | `ps -eo pid,lstart,etime,args \| grep cdfede64` |
| Real tmux sessions | `~/.cache/stapler-squad/tmux/linux_amd64/tmux ls \| grep add-durable-guidance` |
| 14 archived-but-Active rows | `sqlite3 <copy> "SELECT title,status,archived_at FROM sessions WHERE archived_at IS NOT NULL AND status=1"` |
| Killer cadence | `grep reconcileTerminalItemSessions <log>` — one line per minute at `HH:MM:56` |
| Status enum | `session/instance.go:45,49,67` |
