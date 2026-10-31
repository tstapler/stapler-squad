# ADR-034: Serve VCS status from an event-fed materialized view, pushed over a Watch stream

Status: Proposed — 2026-10-30
Owner request: "Lets not poll for background panes, instead we should have an event driven system that the backend keeps a materialized/cached view up to date to serve."

## Context

The browser polls `GetVCSStatus` once per open session pane. Evidence (all VERIFIED by running the quoted command on 2026-10-30):

- HAR `stapler-squad-backend-har-10-30.har` (22 min, 468 entries; `python3 -I` script, path as argument): `GetVCSStatus` 42 calls over 12 sessions, p50 server wait 6.26 s, p95 17.42 s. The same capture holds `ListPendingApprovals` 88, `GetFeatureFlags` 45, `GetNotificationHistory` 44, `GetReviewQueue` 11 calls, all with p50 wait under 0.2 s (cheap per call, wasteful in count).
- The data barely changes. Of the 4 `ACTIVE` sessions on the live instance (`ListSessions` on `:8543`, 41 sessions listed), `.git/HEAD` and `.git/index` mtimes are 114, 1530, 1529 and 3325 minutes old. Across the 33 sessions whose directory still exists the median HEAD/index age is 1738 min and 1 of 33 changed in the last hour. Polling re-derives an unchanged answer; the working-tree dirty state (edits that never touch `.git`) is the only fast-moving part.
- Client entry points: `useVcsStatus` and `useSessionVcs` (web-app/src/lib/hooks), shared through `SessionVcsProvider` for `VcsPanel`, `FilesTab`, `DiffViewer`; plus `BacklogFileBrowserModal`, `BacklogItemDetail`, `useBacklogItemShipStatus`, `app/fanout/page.tsx`.
- Server: `WorkspaceService.GetVCSStatus` (server/services/workspace_service.go:186) serves from `vcsStatusCache` (15 s TTL, 60 s stale-while-revalidate) and starts a git computation on the request path on a miss or `wait_for_fresh`. `InvalidateVCSStatus` is called only by file writes and workspace switches (`file_write.go:301`, `workspace_service.go:643`), so an external `git commit` is noticed by TTL expiry only.

In flight, not duplicated here: #1283 (bounded shipped-commit walk, range cache, stage budgets, singleflight), #1299 (client supersede/dedupe/jitter), #1300 (more idempotent reads over the unary WebSocket). They make each call cheaper and rarer; they do not remove polling. This ADR builds on them.

Event infrastructure already in the repo, reused rather than reinvented:

- `internal/fsevents` (darwin FSEvents via purego) and `internal/treewatch` (Linux recursive inotify), with `Event.Flags.NeedsRescan()` and `SinceWhen` replay.
- `session/unfinished` `treeWatcher` + `scanner_gitwatch*.go` (worktree and gitdir events, debounced, with a stretched backstop tick) and `session/git_worktree_watcher*.go`.
- `Watch*` server streams carried over WebSocket by `web-app/src/lib/transport/watch-ws-transport.ts`, each with `seq`, `deploy_id`, heartbeat and snapshot-then-live phases (`WatchWorkflows`, proto/session/v1/session.proto:616; request message at :4332).
- `server/conditional` ETag plumbing, `vcsFlights` singleflight, `events.EventBus`.

## Measurements (Step 1)

| Quantity | Value | Command |
|---|---|---|
| Sessions listed / ACTIVE / HIBERNATED / PAUSED / FAILED / STOPPED | 41 / 4 / 26 / 1 / 9 / 1 | `curl -X POST .../ListSessions`, counted by `/tmp/ssq_sessions_stats.py` |
| Live (not stopped/hibernated) sessions with an existing dir | 14 | `/tmp/ssq_dircount.py` |
| Directories under those 14 worktrees (a hypothetical per-directory kqueue cost, not measured) (excluding `.git`, `node_modules`, `.next`) | 52,818 dirs, 308,528 files; one PAUSED session alone is 45,008 dirs | same |
| Live service open fds | 17,878 (14,927 REG, 2,799 DIR, 51 KQUEUE) | `lsof -p <pid>` |
| Largest groups of open REG fds, by parent directory | `~/.claude/projects/**/subagents`: 1,158; 450; 362; 344; 332 (consistent with a one-fd-per-file watch there; INFERRED, not traced to code) | `lsof -p <pid>` grouped by directory |
| FSEvents, 14 streams over those 14 roots, 20 s idle | fd delta 0, 85 ms CPU (includes setup), 6 events | throwaway `go test` in `internal/fsevents` (deleted after the run) |
| `git commit` to first FSEvents `.git` event, 50 ms latency setting | 49 ms (the commit itself took 2.25 s) | same |

Reading: a per-directory kqueue/fsnotify watch of these worktrees would need roughly 52,800 fds, on a process that already holds 17.9k from the history watcher. FSEvents costs no fds and negligible idle CPU, so the view uses FSEvents streams (one per repo root, not per file or directory). The 49 ms figure is the first `.git` event, not the final ref update, so the sub-second target is confirmed in Step 3 end to end, not here. The git metadata ages above show `.git` is quiet; they say nothing about dirty working-tree churn, which only the tree stream (FSEvents) observes, and which Step 3 measures as events per minute per active worktree. The measurement scripts are throwaway under `/tmp`; the commands are quoted so they can be rerun.

## Options

| | A. Status quo + #1283/#1299/#1300 | B. Server-side adaptive polling | C. Event-fed materialized view, pushed over a Watch stream |
|---|---|---|---|
| Idle cost | one RPC per pane per poll; thundering herd on the 10-min backstop | server polls git per live worktree on a timer, whether or not anything changed | zero git work and zero RPCs when nothing changes |
| Latency to see a `git commit` | up to TTL (15 s) to 10 min | up to the adaptive interval | debounce (~300 ms) + one compute |
| Request path | may run git | may run git or serve stale | never runs git |
| New machinery | none | scheduler | change sources, view, publisher, stream, client store |
| Risk | already shipped | wasted CPU scales with live worktrees | missed events; mitigated below |

Decision: C, with B's timer retained only as a staggered per-repo backstop and A as the flag-off fallback path.

## Decision

### 1. Generic `matview` package, VCS first

`internal/matview` (new) holds the reusable parts; VCS supplies a compute function and change sources.

```go
type Versioned[V any] struct {
    Value    V
    Rev      uint64    // per-view, monotonic; bumps only when Value's content hash changes
    ETag     string    // content hash, same scheme as server/conditional
    ComputedAt time.Time
}
type View[K comparable, V any] interface {
    Get(ctx context.Context, k K, opt GetOpts) (Versioned[V], error) // GetOpts{WaitFresh bool, Deadline time.Time}
    Subscribe(ctx context.Context, keys []K) <-chan Update[K, V]      // initial snapshot, then conflated updates
    Invalidate(k K, why Reason)
}
type ChangeSource[K comparable] interface{ Subscribe(fn func(K, Reason)) (stop func()) }
```

`Get` never calls compute inline. A miss registers the key, enqueues it at top priority and waits (default deadline 10 s, below the 30 s compute bound) for that compute to complete; cold-miss latency is the queue wait plus one compute, the same work the old path did, and a warm hit is instant. Stale entries are served immediately while a recompute is queued, preserving stale-while-revalidate. The scheduler owns: per-key trailing debounce with a max-wait cap (default 300 ms quiet, forced at 2 s so continuous agent writes still publish), per-key coalescing (one pending compute per key), bounded concurrency (reuses the `vcsFlights` limit), and priority (a `Get` bumps its key to the front; subscribed-but-unread keys run after). Time comes from an injected `Clock`; change sources are an interface, so tests use a fake clock and a fake source with no sleeps.

### 2. Keys, lifecycle and bounds

- Key is `(RepoRoot, Workspace().ActiveDir)`, not `ExistingDir`: `ExistingDir` falls back to the repo root when a worktree is deleted (instance-lock-free-reads.md), which would flip a paused session's key and collide two paused sessions. `ActiveDir` is also what file writes invalidate by (`file_write.go:301`), fixing today's cache-key/invalidate-key mismatch. Sessions sharing an `ActiveDir` share one entry; the compute reads `ExistingDir` only to decide whether to run git. Session to key mapping is read through `Instance.Snapshot()` per `.claude/rules/instance-lock-free-reads.md`. Any code path that revives a session stays out of scope (`noarchivedrevival`): the view only reads.
- Entries are created lazily on first `Get` or `Subscribe`. No boot-time warm for the whole fleet. Entries with no subscriber and no read for 10 min are evicted; subscribed keys are pinned and the cap of 256 applies to unpinned entries only (a metric counts pinned overflow, and each entry's file lists keep the existing server-side caps). LRU eviction (counter `matview_evictions_total{reason}`).
- State mapping:

| Session state | View behaviour |
|---|---|
| Active, hidden, running | keyed, watched while subscribed or read |
| Paused (worktree deleted, branch kept) | entry holds last status marked `stale_reason=worktree_gone`; stream removed; one compute on resume |
| Hibernated, stopped, failed | served from the last entry if present, never watched; a cold miss (no entry, e.g. after restart) computes once through the scheduler at top priority, as today's code does; after that the entry is served without recompute until an explicit `wait_for_fresh` |
| Archived or deleted | entry dropped, stream refcount released, subscribers get a terminal `removed` update |

### 3. Event sources (reuse, fan-out, never one watcher per consumer)

1. Worktree tree events and gitdir events through one shared watcher with per-root refcounts. The scanner already runs a `treeWatcher` and gitdir watcher; Step 3 task 1 adds a fan-out seam so the view subscribes to the same events instead of opening a second stream per root. If the scanner's watch set turns out not to cover a view key, the view adds the root to the shared watcher (refcount), it does not create its own. Whether the scanner's set covers every live worktree is UNVERIFIED and is the first thing Step 3 checks.
2. Known gaps in the reused watchers (VERIFIED against the code): the scanner's gitdir watch is fsnotify (kqueue, one fd per directory, `scanner_gitwatch_tiera.go:36-90`), non-recursive, and `classifyGitEvent` documents that loose refs under `.git/refs` are not watched (`scanner_gitwatch_events.go:176-183`), so `git fetch` and branch creation are missed; the scanner debounces at a fixed 2 s (`:164-166`); the darwin tree stream starts at `SinceNow` and does not persist EventIDs (`treewatch_darwin.go:19-20,48`). Step 3 must therefore: add FSEvents streams on each repo's common dir and linked-worktree gitdirs (so gitdir coverage is fd-free, rather than relying on kqueue), give the view its own 300 ms debounce layer, and persist EventID plus volume UUID for resume replay. Gitdir coverage matters: a linked worktree's `HEAD`/`index` live in `<common>/.git/worktrees/<name>/`, outside the worktree root, and `refs/remotes`, `packed-refs` live in the common dir. The change source must watch the resolved gitdir and the common dir (one stream per repo, shared by all its worktrees) in addition to the worktree root. Relevant names: `HEAD`, `index`, `refs/**`, `packed-refs`, `MERGE_HEAD`, `REBASE_*`, `ORIG_HEAD`, `shallow`, `FETCH_HEAD`. `.git/objects/**` is ignored.
3. Rename safety: git writes `x.lock` then renames to `x`. Lock-file events are ignored; the rename produces an event on the final name, and the trailing debounce absorbs the create/rename/modify burst. A burst never produces a compute from a half-written state because the compute starts after the quiet period, and `git status` itself tolerates concurrent index writes.
4. App-initiated changes: existing `InvalidateVCSStatus` callers (file writes, `SwitchWorkspace`) become `Invalidate(key, ReasonAppWrite)`. These are immediate (no debounce) because the app knows the change just happened.
5. Session lifecycle from `events.EventBus` (created, updated, archived, paused, resumed, deleted): add or remove keys, trigger one compute on resume.
6. PR info from the existing PR poller/webhook results, as `Invalidate(key, ReasonPR)`. No new GitHub calls.
7. Linux uses `internal/treewatch` (inotify); where a watch cannot be added (limit hit) the key is flagged `degraded` and falls back to the backstop only, with a metric.

### 4. Correctness: missed events and staleness bound

Detection and repair, each with a metric:

| Failure | Detection | Repair |
|---|---|---|
| FSEvents drop/overflow (`KernelDropped`, `UserDropped`, `MustScanSubDirs`, `EventIdsWrapped`, `RootChanged`) | `Event.Flags.NeedsRescan()` (already surfaced by `internal/fsevents` and never swallowed) | immediate full recompute of every key on that stream |
| Server restart | process-local view; `deploy_id` changes | cold entries; clients resubscribe and receive fresh snapshots |
| Laptop sleep/resume | wall-clock jump larger than 2x the backstop interval, and FSEvents `SinceWhen` replay from the saved EventID where the volume UUID matches | mark all keys dirty, recompute with jittered stagger |
| Loss with no rescan flag (e.g. loose refs not watched today) | not detectable | per-repo staggered backstop (also the repair for the loose-refs gap until closed): interval 5 to 10 min, offset by hash(repo) plus per-cycle jitter, never a global cadence |
| Slow subscriber | bounded per-subscriber queue, conflated to latest-per-key | no gap in state, only skipped intermediate states |

Staleness bound (design target, verified in Step 3 with the before/after run): fs-visible change to published update is at most debounce (300 ms) + compute time, with a 2 s max-wait cap under continuous writes. Worst case with a lost event and no overflow flag is the backstop, at most 10 min plus jitter, equal to today's client backstop (today's server-side bound is the 15 s TTL plus 60 s stale-while-revalidate, which polling re-triggers; the view trades that for push plus the backstop). The serve-time staleness is exported as `vcs_view_staleness_seconds` and `status_as_of` stays on the wire.

### 5. Wire protocol (proto additive only)

New server-stream `WatchVcsStatus(WatchVcsStatusRequest{ repeated string session_ids; map<string,string> known_etags }) returns (stream VcsStatusEvent)`. Chosen over extending `WatchSessions`, which is global and would carry per-pane file lists to every client, and whose subscription lifecycle is unrelated. `VcsStatusEvent` carries `session_id`, `VCSStatus`, `rev`, `etag`, `not_modified`, `removed`, `snapshot_complete`, `heartbeat`, `deploy_id`, in the same shape as `WorkflowEvent`. It rides the existing watch WebSocket transport, so it does not consume an HTTP/1.1 connection slot (the failure mode documented in `watch_stream_metrics.go`).

A server-stream cannot change its subscription in place. The client reopens with the new `session_ids` after a short coalescing delay (250 ms); `known_etags` makes unchanged panes cost a `not_modified` marker, not a body. Panes open at once are batched into one stream per tab. Reopen order is subscribe-new-then-release-old, and a repo's watcher refcount has a 30 s release grace, so a resubscribe never tears down a stream and loses events in the gap.

`GetVCSStatus` is unchanged on the wire (additive: optional response `rev`, `etag`). With the flag on, it reads the view; `wait_for_fresh` awaits a compute that started after the request and completed (tracked by `ComputedAt`, not by `Rev`, because an unchanged result does not bump `Rev`), with a deadline, then returns the current entry with `status_as_of` if the deadline passes. `if_none_match`/`not_modified` follows the #1257 pattern.

### 6. Client

A module-level `vcsFeedStore` (`useSyncExternalStore`) owns the single stream, writes pushed entries into the existing `vcsCache`, and exposes `connected`. `useVcsStatus` and `useSessionVcs` skip all interval polling for every pane (focused, visible-unfocused, hidden) while the flag is on and the feed is connected and has delivered a snapshot for that session. A user-requested refetch (`refetch()`, post-write) still calls `GetVCSStatus` with `waitForFresh`, preserving #1299's focused-pane semantics. When the stream is disconnected, or reconnects and the `deploy_id` differs, the existing polling code runs unchanged plus one jittered reconcile. Flag off, or any stream error, equals today's behaviour.

### 7. Flag

`vcs_status_view`, default off, in the feature-flag registry (`feature_flag_service.go`), read server-side (gate view construction, `WatchVcsStatus`, and the `GetVCSStatus` path) and client-side (gate the feed store). Flag off keeps the old cache path byte-for-byte as the fallback. Default flips on after a soak per the flag system, in its own PR.

### 8. Observability

`vcs_view_entries`, `vcs_view_events_total{source,outcome=received|coalesced|dropped|ignored}`, `vcs_view_recompute_total{reason}`, `vcs_view_recompute_seconds`, `vcs_view_queue_depth`, `vcs_view_staleness_seconds` (histogram at serve time), `vcs_view_subscribers`, `vcs_view_reconcile_total{cause=backstop|overflow|resume|gap}`, `vcs_view_watch_overflow_total`, `vcs_view_degraded_keys`. Tempo spans `vcs.view.recompute` (attributes: reason, key hash, queue wait) and `vcs.view.publish`. A Grafana row on the existing VCS dashboard with additive panels and stable uids, linked to Tempo like the sibling dashboards.

## Follow-on candidates (not in scope here; estimates are INFERRED)

| Poll | HAR calls (22 min) | Adoption | Expected reduction |
|---|---|---|---|
| `ListPendingApprovals` | 88 | `View[struct{}, []Approval]` fed by the approval store's change events | about 90 to 95 percent; keeps a slow reconcile |
| `GetFeatureFlags` | 45 | `View` fed by flag writes | about 95 percent; flags change on operator action only |
| `GetNotificationHistory` | 44 | `View` fed by notification inserts | about 90 percent |
| `GetReviewQueue` | 11 | `WatchReviewQueue` already exists; the poll is redundant where the stream is healthy (confirm in code before claiming) | up to 100 percent where the stream works |

## Consequences

- Positive: zero idle git work; sub-second visibility of commits; no request-path git; fewer aborted requests; one abstraction reusable for the table above.
- Negative: more moving parts; correctness now depends on event delivery plus the backstop; the Watch stream adds a long-lived stream per tab (WebSocket, no HTTP/1.1 slot).
- Risks and mitigations: event coverage gaps (gitdir and common-dir watching, Step 3 task 1 verifies the scanner's set); fd and inotify exhaustion (FSEvents costs 0 fds measured; Linux caps watches and degrades with a metric); flag-off parity (old path untouched, tested).

## Verification plan (Step 3)

Deterministic unit tests with injected `Clock` and fake `ChangeSource` (debounce, max-wait, coalescing, priority, overflow rescan, resume, eviction, archived and paused mapping); `go test -race`; jest with fake timers for the feed store; one Playwright spec per `tests/e2e` conventions. Before/after on a throwaway instance (ports 62877+, unique `STAPLER_SQUAD_INSTANCE` and `STAPLER_SQUAD_TMUX_SOCKET`, binary under `~/.stapler-squad/manual-builds/manual-7/`): `GetVCSStatus` calls per minute and p95 with N panes open, and the time from `git commit` in a worktree to the pane update.
