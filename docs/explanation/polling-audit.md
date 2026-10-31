# Polling audit: where stapler-squad still polls, and which sites to make event-driven next

Audit date: 2026-10-30 (origin/main `f52e50b1c`). This is the candidate list for the
event-driven materialized-view pattern ([ADR-034](../adr/ADR-034-materialized-vcs-status-view.md),
PR #1309; `internal/matview`, PR #1312). It ranks every polling site by measured volume and
cost and says what could replace each one. It does not repeat the pattern's design.

Confidence labels: **VERIFIED** = a command in [Method](#method-and-commands) produced the
number. **INFERRED** = read from code or derived; the gap is named.

## Summary

- The owner's HAR shows 468 requests in 22.2 minutes. 274 (58.5%) are unary polling RPCs, and
  382 (81.6%) once telemetry flushes (`/api/analytics`, `LogClientEvents`, `/api/client-logs`)
  are counted. The 468 denominator includes 51 stream requests (`StreamTerminal`, `WatchInsights`); excluding them from both sides gives 65.7% and 91.6%. VERIFIED
  (`har.py` below).
- One site dominates cost: `GetVCSStatus` averages 6.6 s per call server-side and consumed
  about 60 s of handler wall time per minute over the last hour. VERIFIED (PromQL below).
  ADR-034 already covers it; it is rank 1 here so the list is complete.
- Four sites are cheap to fix with an existing event source and no new infrastructure: the
  60 s `ReconcileStuck` sweep, the notification-history fallback poll, the feature-flag
  poll, and the tmux-version poll.
- Server-side CPU per ticker is **not measured** (no per-goroutine attribution was
  available without profiling the live service, which was off-limits). Server costs below use
  handler wall time, DB query counts, and subprocess counts instead.

## Ranked candidates (top 10)

"Volume" is one browser's polling in the HAR unless stated. "Server" columns come from
VictoriaMetrics over the last hour (the live service was restarted 7 minutes before
sampling; see [caveats](#caveats)).

| # | Site | Interval / trigger | Measured volume and cost | Class | Event source | Expected reduction | Risk |
|---|---|---|---|---|---|---|---|
| 1 | `GetVCSStatus` from `useSessionVcs` / `useVcsStatus` (`web-app/src/lib/hooks/useSessionVcs.ts:241`, `useVcsStatus.ts:174`) | 60 s active / 600 s unfocused pane; 15 s / 150 s in `useVcsStatus`; skips when `document.hidden` | HAR 42 calls (1.89/min, median gap 0.1 s, so bursts from several panes rather than the timer cadence); server 9.1/min, avg 6.6 s, 60.2 s handler wall per minute; p95 at the 10 s histogram cap | Pure-event | git fsnotify watcher (`session/git_worktree_watcher_tree.go`), ADR-034 view | 90%+ of calls become cache reads | Missed fsnotify event: keep a slow backstop reconcile |
| 2 | tmux subprocess traffic driven by the review-queue poller (`session/review_queue_poller.go` `pollLoop` (line 346), 2 s fast / 8 s slow) and per-session driver ticks (`session/session_driver.go:343`, 2 s) | Poll while any session is live | 252 tmux execs/min on the `default` exec-gate pool, 103 s of subprocess wall time per minute; control-mode `capture-pane` 19.9/min (attribution to the pollers is INFERRED: the exec-gate pool has no caller label, and the rank 3 sweep also reaches tmux via `findConfirmedLiveInstance`, `server/services/session_service_liveness.go:216-236`) | Needs-new-event-source | tmux control-mode `%output` / `pane_last_activity` notifications (control mode already exists) | Large but unquantified | High: status detection is the product's core signal; needs a shadow comparison first |
| 3 | `ReconcileStuck` 60 s sweep: `archive_terminal_sessions`, `cleanup_terminal_worktrees` (ticker at `server/dependencies.go:1502`; detectors documented at `session/backlog_lifecycle_archive.go:102,152`) | 60 s ticker, re-walks every done/archived backlog item and every session of each | Over a 7-minute window the `ent-read` pool ran 13,476 queries; `ArchiveInstanceDataByID` and `findConfirmedLiveInstance` lookups were 3,622 and 3,627 of them (about 54% together; a later re-read at 13 min uptime showed about 61%, roughly 1,400 reads/min), so on the order of 1,000+ reads/min (INFERRED to be sweep-driven from the call chains; not profiled) | Pure-event | Backlog status transition to done/archived already fires a hook (`TransitionBacklogItemStatus`); the sweep is its safety net | About 90%: sweep a dirty set, run the full walk every 15 min | Low: the hook already exists, the sweep stays as backstop |
| 4 | `GetNotificationHistory` fallback poll (`web-app/src/lib/hooks/useNotificationHistory.ts:140`) | 30 s, only when `streamLive` is false | HAR 44 calls, 73 KB/min, the largest bytes of any unary poll; server 5.96/min | Pure-event (stream exists) | Notification stream (`useStreamConnectionState`) | About 100% of polls once the stream reports connected; the fixed 30 s gaps (median 30.0 s) match the fallback timer, which only runs while `streamLive` is false (INFERRED; WebSocket watch frames are invisible in a HAR, so the stream state itself was not observed) | Low; find why `streamLive` stayed false first |
| 5 | `ListPendingApprovals` (`web-app/src/lib/contexts/ApprovalsContext.tsx:50`) | 15 s idle / 5 s while pending; paused when hidden | HAR 88 calls (3.97/min), the highest call count; server 20.75/min over 1 h, 8.2/min over 24 h; 0.78 ms avg, so cheap per call | Needs-new-event-source | `ApprovalStore` is in-process; emit a created/resolved event on the existing session watch stream | About 95% | Low; keep a 60 s backstop |
| 6 | `GetFeatureFlags` (`web-app/src/lib/features/flagStore.ts:41`) | 30 s while any subscriber exists; **no visibility pause** | HAR 45 calls (2.03/min); server 6.1/min, 29-41 ms avg; ETag path keeps the body to 113 B/min | Pure-event / cheap fix | Flag-version field on an existing Watch stream; minimum fix is pausing when hidden | 100% if pushed; about 50% from the hidden-tab pause alone | Low |
| 7 | `ListStuckBacklogItems` (`web-app/src/lib/hooks/useStuckBacklogItems.ts:130`) | 60 s | HAR 22 calls, 10.2 KB/min; server 2.57/min, **943 ms avg**, 2.4 s handler wall per minute | Pure-event | `BacklogStuckState` rows are written by `ReconcileStuck` (rank 3); publish on write via `WatchBacklogItems` | About 95% | Low |
| 8 | Unfinished-work scanner backstop (`session/unfinished/scanner.go:508`) and `periodicReWalk` (`session/unfinished/watcher.go:171`) | 60 s tick (2 min when the tree watcher is active) and 60 s watch-dir re-walk | 6.13 scan cycles/min at 26.0 s average per cycle (summed across parallel workers, so wall time is lower; CPU not measured) | Mostly event-driven already | Per-repo fsnotify tree watcher exists; new-repo discovery has none (needs a watch on the watch dir) | Further 50% by lengthening the backstop | Medium: scan-skip correctness is subtle (`scanner_eventdriven_test.go`) |
| 9 | GitHub PR pollers: `worktree_pr_poller` (`session/worktree_pr_poller.go:285`, 60 s), `pr_status_poller` (`session/pr_status_poller.go:262`, 60 s), `user_pr_cache` (`github/user_pr_cache.go:646`, 2 min), `server/prwatch/loop.go` (5 s default, timer-based) | Fixed ticks | 54 GitHub calls/min over 1 h, 80/min over 24 h; `worktree_pr_poller` is 67 of 80 per minute over 24 h; cache hit 28/min vs miss 26/min | Must-stay-poll, webhook-assisted | `/webhooks/github` endpoint and `server/conditional/` ETags exist | About 50-70% if webhooks are publicly reachable (see `docs/how-to/expose-github-webhook-endpoint.md`) | Medium: a dropped webhook needs the poll as backstop; rate limit has headroom (core remaining 4,528) |
| 10 | `GetTmuxVersionStatus` (`web-app/src/components/system/TmuxVersionMismatchBanner.tsx:57`) | 60 s; **no visibility pause** | HAR 22 calls (0.99/min); server 2.26/min, 9.5 ms avg | Pure-event / cheap fix | The answer changes only on tmux upgrade or server restart; fetch on mount and on stream reconnect | 100% | Low |

Next after the top 10 (same method, smaller cost): `DialogGoneSampler` 3 s DB read
(`server/services/guidance_dialog_gone_sampler.go:204`; 111 `ListAllPendingGuidanceRequests`
queries in 7 minutes at 55 ms each), `guidanceRowStore` 3 s / 15 s client poll
(`GetGuidanceRequest` is 4.51/min over 24 h server-side), `GetReviewQueue` fallback
(`useReviewQueue.ts:481`), `ListWorkflows` 90 s backstop (`useWorkflows.ts:146`), and the
`GetInsightsSummary` refetch on every `parse_complete` event (10 calls, 11.2 s average,
270 KB/min in the HAR; this is event-triggered, not a timer, but each event refetches the whole
summary instead of applying a delta).

## Client inventory

Every network-issuing timer or refetch in `web-app/src`. "Pauses hidden" is read from the
code. HAR volume is calls over 22.2 minutes.

| Site | Interval | Trigger | Pauses hidden | RPC | HAR calls |
|---|---|---|---|---|---|
| `lib/contexts/ApprovalsContext.tsx:50` | 5 s pending / 15 s idle | mount (RTK Query) | yes, plus refetch on return | ListPendingApprovals | 88 |
| `lib/features/flagStore.ts:41` | 30 s | first subscriber | no | GetFeatureFlags (`ifNoneMatch`) | 45 |
| `lib/hooks/useNotificationHistory.ts:140` | 30 s | `streamLive` false; also visibilitychange and `online` | yes | GetNotificationHistory | 44 |
| `lib/hooks/useSessionVcs.ts:241` | 60 s / 600 s | pane mount, session change | yes (`pollMode` off, `document.hidden`) | GetVCSStatus / GetSessionDiff | 42 (all callers) |
| `lib/hooks/useVcsStatus.ts:174` | 15 s / 150 s | detail view mount | yes | GetVCSStatus | in the 42 |
| `lib/hooks/useStuckBacklogItems.ts:130` | 60 s | provider mount | partial (one visibility reference) | ListStuckBacklogItems | 22 |
| `components/system/TmuxVersionMismatchBanner.tsx:57` | 60 s | mount | no | GetTmuxVersionStatus | 22 |
| `lib/hooks/useReviewQueue.ts:481` | 30 s fallback / 120 s reconcile when live | mount | no | GetReviewQueue | 11 |
| `lib/hooks/useWorkflows.ts:146` | 90 s | mount | yes | ListWorkflows | 0 (not in HAR window) |
| `lib/hooks/useWatchBacklogItems.ts:429` | 30 s fallback | stream down | no | ListBacklogItems | 0 |
| `lib/hooks/useBackgroundSessions.ts:84` | 30 s | `enabled` | no | ListSessions | 0 |
| `lib/hooks/useScheduledPrompts.ts:66` | 30 s | `active` | no | scheduled-prompts list | 0 |
| `lib/hooks/useSavedAnswers.ts:31` | 15 s | mount | no | ListSavedGuidanceAnswers | 0 |
| `lib/hooks/guidanceRowStore.ts:184` | 3 s / 15 s settled | row subscriber | no | GetGuidanceRequest, ListAllPendingGuidanceRequests | 0 |
| `lib/hooks/useBacklogCreationNotifier.ts:129` | 4 s | pending creation | no | GetBacklogItem | 0 |
| `components/sessions/DetectionEventsPanel.tsx:72` | 3 s | panel open | no | GetDetectionEvents | 0 |
| `components/backlog/SessionMonitor.tsx:101` | 5 s | monitor open | no | session lookup | 0 |
| `app/fanout/page.tsx:59` | 5 s | page open | no | fan-out status | 0 |
| `app/page.tsx:387` | `FALLBACK_SESSION_REFRESH_MS` | hidden-session fallback open | no | GetSession per id | 0 |
| `lib/hooks/useHandoffSummary.ts:98`, `useSessionSummary.ts:111` | 2 s, at most 10 null reads | summary pending | no | GetHandoffSummary / session summary | 0 |
| `app/settings/features/useGateStats.ts:43`, `GateKindOverrides.tsx` | 30 s | settings page | no | GetDeliveryGateStats | 0 |
| `lib/hooks/useBufferFreshness.ts:76` | 10 s / 100 s, off when hidden | pane mount | yes | local freshness check | 0 |
| `lib/telemetry/clientLogShipper.ts:133`, `logBuffer.ts:119` | 5 s flush | mount | no | LogClientEvents, `/api/client-logs` | 39 + 15 |
| `lib/analytics/HttpAnalyticsProvider.ts:89` | 20 s flush | event queued | no | `POST /api/analytics` | 54 |
| `lib/files/windowedFile.ts:123` | 1.5 s, at most 80 | file index build | n/a | file index status | 0 |

Keepalives (not data polls, leave): `unary-ws-transport.ts:595` ping 10 s,
`terminal-socket-pool.ts:283` ping 20 s, `useWatchStream.ts:214` watchdog 10 s.

Local-only timers (no network; leave): `SessionList.tsx:430` (60 s stale recompute),
`NotificationContext.tsx:372` (60 s sweep), `ApprovalCard.tsx:52`, `UnfinishedTab.tsx:77`,
`BacklogItemDetail.tsx:740`, `app/account/page.tsx:24`, `useNowTicker.ts:10`,
`ToastStack.tsx:215` (250 ms), `useStatusModel.ts:22` (250 ms), `TerminalOutputFilter.tsx:34`,
`XtermTerminal.tsx:1400` (50 ms sampler), `ScrollModeChip.tsx`, plus the test pages under
`app/test`, `app/debug`, `app/spike-lazydoc`.

## Server inventory

83 ticker sites in production Go (`grep` recipe below), grouped. "Work per tick" is read from
code; items marked (name) were classified from the file name and constant only.

| Site | Interval | Work per tick | Replacement event | Class |
|---|---|---|---|---|
| `server/dependencies.go:1502` ReconcileStuck + QuotaGate.Reconcile + wake reconcile | 60 s | Walks all terminal backlog items and sessions; PR-pending, review-gate, stuck detection | Backlog transitions, session archive events | Pure-event (rank 3) |
| `session/review_queue_poller.go:346` | 2 s fast / 8 s empty-queue slow; 30 s tmux reconcile | One batched pane-activity subprocess, then per-session evaluation | tmux control-mode notifications | Needs-new-event-source (rank 2) |
| `session/session_driver.go:343` | 2 s | Per-autonomous-session pane polling | Pane-activity events | Needs-new-event-source |
| `session/unfinished/scanner.go:508`, `watcher.go:171`, `scanner.go:360,983` | 60 s / 2 m backstop, 60 s re-walk, maintenance, 5 m repo prune | Repo scan with signature skip | Per-repo fsnotify (exists); new-repo watch | Mostly done (rank 8) |
| `session/history_linker.go:248` | 5 s | `scanAllSessions` correlates every monitored instance with Claude history | History-file fsnotify watcher (`HistoryFileWatcher` exists) | Pure-event |
| `session/mux/discovery.go:352` | 30 s backstop | Scan for external PTY sessions | Mux socket events | Cheap-enough |
| `session/health.go:636` | configured interval | `RecoverUnhealthySessions` over all instances, may `SaveInstances` | tmux session-closed notifications | Needs-new-event-source |
| `session/worktree_pr_poller.go:285`, `pr_status_poller.go:262`, `jules_session_poller.go:236`, `session_tag_poller.go:218` (2 m), `github/user_pr_cache.go:646` (2 m), `server/prwatch/loop.go` (5 s) | 60 s / 2 m / 5 s | GitHub REST/GraphQL, conditional where possible | `/webhooks/github` | Must-stay-poll, webhook-assisted (rank 9) |
| `server/services/guidance_dialog_gone_sampler.go:204` | 3 s | `ListAllPendingGuidanceRequests` DB read, 55 ms avg | In-process guidance row events | Pure-event |
| `server/services/guidance_ack_detector.go:155` | 3 s | per-row sample (name) | same | Pure-event |
| `server/services/config_file_rules_reload.go:76` | 3 s | stat `shared_rules.yaml` | fsnotify | Pure-event, trivial |
| `server/services/connectrpc_websocket.go:2015` | 1 s per terminal stream | `hub.SubscriberCount()` (O(1)) | Subscriber join/leave callback | Cheap-enough |
| `server/services/stale_session_notifier.go:71`, `memory_pressure_notifier.go:79`, `capacity_monitor.go:147`, `stale_creation_sweeper.go:63`, `superseded_session_sweeper.go:69` | 60 s each | Scan sessions or backlog items; `SupersededSessionSweeper` reads `ListBacklogItems` (6 queries, 2.5 s total in 7 min) | Backlog/session transitions | Pure-event for the sweepers, cheap for the rest |
| `server/server_leasewedge.go:13` | 10 s | Lease scan (name) | Lease events | Cheap-enough |
| `server/services/approval_handler.go:986` | 30 s | `CleanupExpired` in memory | Per-approval timer | Cheap-enough |
| `session/orphan_tmux_sweeper.go:75`, `tmux/leaked_control_mode_sweeper.go:33` | 5 m | Subprocess scans | none useful | Cheap-enough |
| `session/worktree_consistency_sweep.go:946` | 15 m (flag-gated, default off) | `git worktree list` reconcile | worktree watcher | Cheap-enough |
| `session/hibernation_sweeper.go:216`, `server/services/session_retention_sweeper.go:46` (1 h), `guidance_session_reaper.go:128` (6 h), `contexthistory/recorder.go:100` (6 h), `analytics/retention.go:31` (1 h), `workflows/retention.go:40`, `dependencies.go:1382` (24 h), `dependencies.go:1975` (30 m paused-tmux reaper) | minutes to hours | Retention deletes | none needed | Cheap-enough |
| `session/dbmaint/dbmaint.go:167` | 15 m | SQLite maintenance | none | Cheap-enough |
| `session/host_advertiser.go:76`, `claim_gossiper.go:107` (5 m), `main.go:1486,1730`, `sshremote/health_prober.go:300` | 5 m | Multi-host heartbeats | peer events | Cheap-enough |
| `server/notifications/subscriber.go:110,152`, `resolved_item_reaper.go:79`, `server/events` heartbeats (`pkg/events/heartbeat.go`, 15 s per Watch stream) | 15 s | Stream keepalives | n/a | Cheap-enough |
| `server/services/session_creation_await.go:87` (250 ms), `backlog_service_verdict_steer.go:148` (5 s), `server/wakes/revive.go` (1 s), `session/autonomous_driver.go` (150-500 ms settle polls) | short | Bounded waits | n/a | Cheap-enough (bounded) |
| `session/pty_discovery.go:459` (5 s), `external_tmux_streamer.go:426` (100 ms, control-mode-unavailable fallback only), `session/workspace/notify.go:51` (5 s, a no-op body), `daemon/daemon.go:107,210` (3 s / 5 s) | short | Discovery and fallbacks | n/a | Cheap-enough; `workspace/notify.go` ticker does nothing today |
| Telemetry: `server/analytics/escape_event_batch_writer.go:47` (500 ms), `profiling/profiling.go:201`, `log/service_log_rotation.go:34`, `server/auth/session.go:233`, `scrollback/manager.go:53`, `workflow_engine_shadow.go:622`, `gitwiring/soak.go:83`, `tmux/fork_metrics.go:197`, `server/server_gatestats.go:22` | 500 ms to 10 m | Flush and metrics | n/a | Cheap-enough |

Not ticker-based but polling by sleep or timer: `session/instance_scroll_forward.go:238`,
`session/pane_submit.go:248`, `session/tmux_process_manager.go:560`, `session/tmux/server_registry.go:633`,
`session/cdp/manager.go:476` (all bounded waits).

## Caveats

- **Short window.** The live service restarted 7 minutes before sampling (`ps` elapsed time
  `07:19`), so `/actuator/db` `top_statements` cover startup. The per-tick walk in rank 3 is read
  from `reconcileTerminalItemSessions`; its per-minute DB rate is not a steady-state figure.
- **Browser presence.** Rates fall sharply when no tab is open: `GetVCSStatus` is 9.1/min over
  1 h but 1.07/min over 24 h. Read the 1 h number as "with a browser attached".
- **Multiple clients.** Server-side rates exceed the HAR's single browser for `ListPendingApprovals` (20.75 vs 3.97/min), `GetFeatureFlags` (about 6 vs 2.03) and `GetTmuxVersionStatus` (about 2.3 vs 0.99), so other clients or tabs also poll.
- **Reduction estimates.** The "expected reduction" figures are INFERRED estimates, not measurements; hidden-tab time was not measured.
- **Re-run drift.** A reviewer re-running the queries 20 minutes later got values within about 15% except `GetFeatureFlags` latency (29 ms) and unfinished-scan cycles (7.5/min).
- **Histogram cap.** The `GetVCSStatus` p95 equals the top finite bucket (10,000 ms), so the
  true p95 is at least that.
- **No CPU attribution.** Per-ticker CPU was not measured.
- **Not read in depth.** Rows marked (name) above were classified from the constant name and
  file, not from reading the loop body.

## Method and commands

Run from the repo root. Replace the HAR path as needed; the scripts print aggregates only.

### Client volume (HAR)

```sh
python3 -I har.py /path/to/export.har
```

`har.py` groups entries by method and URL path and prints calls/min, bytes/min (uncompressed
`content.size`), average ms, and the median gap in seconds between consecutive calls (a median
gap near a constant, such as 30.0, identifies a fixed-interval poll):

```python
import json, sys, collections
from datetime import datetime
es = json.load(open(sys.argv[1]))['log']['entries']
ts = lambda e: datetime.fromisoformat(e['startedDateTime'].replace('Z', '+00:00')).timestamp()
span = (max(map(ts, es)) - min(map(ts, es))) / 60
agg = collections.defaultdict(lambda: [0, 0, 0.0, []])
for e in es:
    k = (e['request']['method'], e['request']['url'].split('?')[0].split('/', 3)[-1])
    r = e['response']; sz = max(r.get('content', {}).get('size', 0) or 0, 0)
    a = agg[k]; a[0] += 1; a[1] += sz; a[2] += e['time']; a[3].append(ts(e))
for (m, k), (c, b, t, tl) in sorted(agg.items(), key=lambda x: -x[1][0])[:40]:
    tl.sort(); g = sorted(tl[i+1]-tl[i] for i in range(len(tl)-1))
    print(f"{c/span:6.2f}/min {b/span:9.0f}B/min {t/c:7.1f}ms gap~{g[len(g)//2] if g else 0:6.1f}s {c:4d} {m} {k[:70]}")
```

### Server RPC volume and cost (VictoriaMetrics, `http://localhost:8428`)

```promql
# calls/min by RPC (use [1h] with a browser attached, [24h] for the average)
sum by (rpc_method)(rate(rpc_server_duration_milliseconds_count{rpc_method!~"Watch.*|StreamTerminal"}[1h]))*60
# handler wall ms per minute (cost); ignore Watch* here, those are stream lifetimes
sum by (rpc_method)(rate(rpc_server_duration_milliseconds_sum[1h]))*60
# average latency of a call
sum by (rpc_method)(rate(rpc_server_duration_milliseconds_sum[1h])) / sum by (rpc_method)(rate(rpc_server_duration_milliseconds_count[1h]))
# response bytes per minute
sum by (rpc_method)(rate(rpc_server_response_size_bytes_sum[1h]))*60
# client-side view (what the browsers sent)
sum by (method)(rate(client_rpc_duration_milliseconds_count[1h]))*60
# DB queries/min by pool
sum by (db_name)(rate(db_query_duration_milliseconds_count[1h]))*60
# tmux subprocesses
rate(tmux_exec_gate_acquired_total[1h])*60
rate(tmux_exec_gate_exec_duration_ms_milliseconds_sum[1h])*60
# GitHub calls by origin and call site
sum by (github_call_origin, github_call_site)(rate(github_calls_total[1h]))*60
# unfinished scanner
rate(unfinished_scan_cycles_total[30m])*60
rate(unfinished_repostate_scan_cycle_seconds_sum[30m]) / rate(unfinished_repostate_scan_cycle_seconds_count[30m])
```

DB callers (per-statement, since process start):

```sh
curl -s localhost:8543/actuator/db   # .pools[].top_statements[] has statement, caller, count, total_ms
```

The `caller` string is a call chain, so group by it to attribute queries to a ticker. A
`ps -o etime= -p $(lsof -nP -iTCP:8543 -sTCP:LISTEN -t)` tells you the window it covers.

### Finding polling sites

Go (production code only):

```sh
# tickers
grep -rnE "NewTicker\(|time\.Tick\(" --include='*.go' . | grep -vE "_test\.go|testutil|/gen/|tools/|example_"
# timers, sleeps and select loops
grep -rnE "time\.AfterFunc\(|<-time\.After\(|time\.Sleep\(" --include='*.go' server session github config daemon pkg | grep -v _test.go
# structural: the bare expression pattern 'time.NewTicker($D)' matches nothing in ast-grep's Go grammar,
# so match the statement forms
sg run -p '$V := time.NewTicker($D)' -l go server session github config
sg run -p '$V = time.NewTicker($D)' -l go server session github config
sg run -p 'time.AfterFunc($A, $B)' -l go server session github config
```

Web (run for both `ts` and `tsx`; exclude tests):

```sh
sg run -p 'setInterval($$$)' -l tsx web-app/src
sg run -p 'setInterval($$$)' -l ts web-app/src
grep -rnE "pollingInterval|refetchInterval|POLL_[A-Z_]+|_INTERVAL(_MS)?\b" --include='*.ts' --include='*.tsx' web-app/src
```

Counts at the audit commit: 83 production Go `NewTicker`/`time.Tick` lines, 42 production
`setInterval` lines in `web-app/src` (`grep -c`), 2 `pollingInterval`/`refetchInterval` lines.

## Proposed gate (not implemented)

Fail CI on a new polling site that has no stated reason, in the style of the existing
analyzers (`tools/lint/notimesleeptest`, `tools/lint/norawexec`, which share
`tools/lint/internal/nolintcomment`):

- **Convention:** a `// poll:justified <event source considered, or why none>` comment on the
  call's line or the line above, mirroring the `//nolint:<analyzer> <reason>` form. A bare
  directive with no reason is itself reported.
- **Go:** a `go/analysis` pass flagging `time.NewTicker`, `time.Tick`, and a `for { select { case
  <-time.After(...)` loop in non-test files that lacks the directive.
- **Web:** an ESLint `no-restricted-properties`-style rule (the repo already uses these for
  `pushState`) on `setInterval`, plus RTK Query `pollingInterval`.
- **Baseline:** 83 Go and 42 web sites exist today. Annotating all of them is the noise cost;
  the alternative is a checked-in allow-list keyed by file and count that may only shrink (a
  ratchet), which avoids touching 125 call sites. This PR does not implement either, because
  the owner asked for implementation only if the baseline is simple to annotate, and 125 sites
  across about 70 files is not.
