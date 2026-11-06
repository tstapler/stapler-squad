# Architecture Research: backlog-flow-observability

Paths are relative to the repo root. Live-data numbers come from the read-only workspace DB
`~/.stapler-squad/workspaces/6eb0b580fa0331d5/sessions.db`, queried 2026-11-05. Confidence: VERIFIED =
opened or ran it; INFERRED = reasoned from what was read.

## 0. Decisions at a glance

| Question | Decision |
|---|---|
| Integration-point disposition | **Isolate via seam.** Do not add to `BacklogService`. Add a new leaf package, a new proto service, and a new MCP file. |
| Compute-on-read or materialized rollups | **Compute-on-read**, plus one additive index and a short in-process TTL cache. No persisted rollup tables in this pass. |
| Thrash collapse | **Query-time**, in the aggregation layer. Never at write time. |
| One aggregation, two consumers | A single `flow.Aggregator` is called by the ConnectRPC handler and by the MCP handler. Neither does its own math. |
| Package boundary | Pure logic in `session/flow` (no ent import). SQL reader in `session` implements a narrow `flow.Reader` interface. |
| MCP data path | **In-process.** The MCP handler holds the `*flow.Aggregator`. `server/mcp/proxy.go` already forwards stdio to the running HTTP server, so the stdio case needs no code. |

## 1. Hotspot disposition for the integration points

Evidence (VERIFIED): `wc -l` and `git log --since='90 days ago' --oneline -- <file> | wc -l`, run 2026-11-05.

| File | Lines | Funcs | Commits, 90d |
|---|---|---|---|
| `server/services/backlog_service_triage.go` | 4018 | 77 | 29 |
| `server/mcp/tools_backlog.go` | 3255 | 53 | 18 |
| `server/services/backlog_service.go` | 1161 | n/a | 15 |
| `server/services/backlog_service_lifecycle.go` | 1270 | n/a | 9 |
| `proto/session/v1/backlog.proto` | 1806 | n/a | 18 |

- All five are large and high-churn. The `BacklogService` struct is a god object that holds storage, session
  creator/stopper/steerer, the autonomous starter, the Jules and one-shot runners, and two mutexes
  (`server/services/backlog_service.go:122-162`).
- The earlier extraction design (`docs/architecture-audit-session-backlog-extraction-design-2026-09-07.md`) proposes moving
  `session/backlog_lifecycle.go` into `session/backlog`. It is still unstarted: `session/backlog/` does not exist.
  `backlog-service-refactor/research/architecture.md:16-37` chose a file split inside `package services`, not sub-packages.
- Disposition: **Isolate via seam.** A refactor-first pass would delay the feature by an unrelated multi-PR project. Extending in place
  would add churn to the two worst files. A seam costs one interface and keeps the new code testable without SQLite.
  - `BacklogService` is not touched, except for one optional line of wiring in `server/dependencies.go`.
  - The MCP change is one new file, `server/mcp/tools_backlog_flow.go`, plus one `registerBacklogFlowTools(s, h)` call beside
    `registerBacklogTools` at `server/mcp/server.go:77`. Do not append to `tools_backlog.go`.
  - The proto change is a new file, `proto/session/v1/backlog_flow.proto`, with its own `BacklogFlowService`. This leaves
    `backlog.proto` (1806 lines) and the registered `BacklogService` untouched.
  - Capture-gap writers (section 7) do land in hot files. Keep each to a one-line call into a helper.
- No kibitzer or hotspot report exists in the repo (`grep` for `kibitzer`/`hotspot` artifacts found only skills). The churn counts
  above are the evidence. Run `code-hotspot-analysis` in the plan phase if a ranked report is wanted.

## 2. What the data model gives us

### 2.1 Sources of truth (answers Open Question 1)

| Store | Use for | Note |
|---|---|---|
| `backlog_status_events` (`session/ent/schema/backlog_status_event.go:19-36`) | Stage dwell, funnel, rework counts | Append-only. Index is only `(item_id, created_at)` (`:49`). |
| `backlog_stuck_states` (`session/ent/schema/backlog_stuck_state.go:33-69`) | Time-in-stuck by reason | Has `first_detected_at` / `resolved_at`, so it is already interval data. It is unique on `(item_id, reason)` (`:90`). Rows are reopened in place, so each (item, reason) keeps only its latest episode and earlier ones are lost. This is a limit to flag in the UI. |
| `review_verdicts` (`session/ent/schema/review_verdict.go:21-45`) | Verdict mix, override rate | Links to the item only through `item_sessions` (`edge.From("item_session")`, `:54`). |
| `item_sessions` (`session/ent/schema/item_session.go:21-84`) | Session roles, end reasons, cost | `end_reason` is NULL for 501 of 505 work sessions and for 603 of 606 review sessions (VERIFIED, sqlite). |
| `backlog_activity_notes` | Agent-written progress | 39 rows only. Not a reliable signal. |
| `stage_transitions` (ent) | **Not a source** | It is the workflow definition table, with `from_stage_id`, `to_stage_id`, `enabled` (`session/ent/schema/stage_transition.go:23-27`). It holds config, not events. |
| `analytics.db` (`server/analytics`) | **Not a source** | It stores `AnalyticsEvent` and `EscapeEvent` rows (`server/analytics/retention.go:24-45`). It has no backlog stage events. The requirements' "analytics.db stage_transitions" reference is a mislabel. |

Decision: `backlog_status_events` is the single source for stage dwell and funnel. `backlog_stuck_states` and
`review_verdicts` are joined for the stuck and review views. No parallel store.

### 2.2 Event writers (where the flips come from)

- There is one choke point: `recordStatusEvent` (`session/ent_repository_backlog.go:49-63`). It logs write failures and never
  blocks the transition. The `triggered_by` constants live at `session/backlog.go:117`.
- Callers that matter for flow:
  - `ReconcileStuckItems` writes `in_progress -> review` at `session/storage_backlog.go:892`.
  - `DequeueNextQueuedItems` (`server/services/backlog_service_triage.go:837`) claims `ready -> in_progress` with note
    "dequeued: WIP slot freed" (`:889`). On spawn failure it rolls back with "dequeue spawn failed" (`:922`).
  - `AutoReopenAfterFailedReview` (`:1722`) writes "auto-reopened after failed review verdict" (398 rows).
  - `AutoReopenForPRFix` (`:2108`, `:2117`) writes "auto-reopened for PR fix (CI/review)" (4437 rows).
- Open Question 3, thrash on current code (VERIFIED, sqlite). There are two families, and both are still live.

  | Family | Evidence |
  |---|---|
  | `ready<->in_progress` ("dequeue" notes) | The 87k-event episodes on the top two items ended 2026-09-25. 278 dequeue-note events still landed on 2026-11-05, from smaller loops. |
  | `pr_pending<->in_progress` ("auto-reopened for PR fix") | Item `a90b99cb` accounts for 4325 of the 4437 rows. Daily counts: 1252 on 11-02, 1101 on 11-03, 279 on 11-04, 4 on 11-05. This family is not in the requirements. |

  The guard in `server/services/backlog_service_triage.go:2145-2152` fixed the version where an active session was already running.
  The item `a90b99cb` loop still ran after it, so some other path still drives it. That is a defect to hand off as a separate backlog
  item, per the requirements' out-of-scope list.
- Cause attribution comes from `note`. The `note` strings are free-form but come from a small set of literals (above), so a prefix
  classifier is reliable for the existing writers. Keep it in one table in the aggregator and add a test that fails when an
  unclassified note is seen.

### 2.3 Performance facts (VERIFIED, sqlite CLI)

- `backlog_status_events` has 257,973 rows. 10,123 are in the last 30 days. 238,553 are `ready<->in_progress` flips.
- `EXPLAIN QUERY PLAN select ... where created_at>=...` shows `SCAN backlog_status_events`. The only index is `(item_id, created_at)`.
  A 30-day window query that filters on `created_at` alone is a full scan. Measured 3.0s cold for one window-function query, and
  0.84s for three simple count queries.
- The service holds SQLite at `SetMaxOpenConns(1)` (`session/ent_repository.go:164`). One long aggregation read blocks every writer,
  including the 60s reconciler. This rules out a design that loads whole tables, and it makes the index mandatory.

## 3. Compute-on-read vs materialized rollups

**Decision: compute-on-read.**

Reasoning:
1. After an added `created_at` index, a 30-day window touches ~10k event rows (VERIFIED count). That fits the 1s SLO with room
   to spare. The other tables are tiny: 526 verdicts, hundreds of sessions.
2. Rollups add a second writer, a backfill, and staleness rules. They also have to replicate the thrash collapse at write time,
   which is the wrong place (section 4).
3. Before/after comparison takes two windows. Compute-on-read gives that with two calls to the same function.

Mechanics:
- Add one ent index `index.Fields("created_at")` on `BacklogStatusEvent`. It is additive. Verify with `EXPLAIN QUERY PLAN` on the
  live-size DB, and do not commit generated ent output (per CLAUDE.md).
- Read through short, bounded queries so the single connection is held briefly. Fetch only the columns needed, and `ORDER BY item_id, created_at`.
- Keep a TTL cache (about 30s) in `Aggregator`, keyed on (window, filters). Use `singleflight` so a refreshing UI cannot stack up queries.
  The cache is invalidated by `BacklogChange` events if the event bus is cheap to subscribe to. A fixed TTL is enough otherwise.
  Every response carries `as_of`, and a cache hit reports the compute time (requirements: data freshness).
- Debug-log the query duration and row count (requirements: observability).
- **Revisit trigger** for rollups: a 30-day summary over 500ms at 5x current event volume, measured by the debug log. Until then, none.
- Retention (Open Question 5): `session/` has no pruning for `backlog_status_events`. I only found a retention enforcer for
  `AnalyticsEvent`. That is INFERRED from a targeted read, not an exhaustive grep. If pruning is added later, it should
  replace collapsed thrash runs with a single summary row (`triggered_by="rollup"`), not delete them.

## 4. Thrash-episode collapsing

**Decision: collapse at query time. Do not dedupe at write time.**

- The event log is an immutable audit trail (`recordStatusEvent`, `session/ent_repository_backlog.go:43-48`). Write-time dedupe
  would drop information the second pass needs (when a loop began and which note drove it), and it would change pipeline behavior,
  which is out of scope. The fix for the loops belongs in `DequeueNextQueuedItems` and `AutoReopenForPRFix`.
- Query-time algorithm, in `session/flow` as a pure function `Collapse([]Event) []Span`:
  1. Walk each item's events in time order.
  2. A **flip pair** is an event whose `(from,to)` is in `{ready<->in_progress, pr_pending<->in_progress}` and whose note
     classifies as a system retry: the dequeue notes, or "auto-reopened for PR fix".
  3. A **thrash episode** is a maximal run of flip pairs with each gap under `G` (default 15 minutes, tunable) and at least `N`
     flips (default 6).
  4. Emit one `Span{Kind: Thrash, Count, FirstAt, LastAt, NetStatus, Cause}` per episode.
  5. Dwell is computed on the collapsed sequence. Time inside an episode goes to a `thrash` bucket, not to `ready` or `in_progress`.
     Otherwise stage dwell is meaningless (requirements, Rabbit Holes).
  6. Real transitions (`in_progress>review`, `review>in_progress`, `review>pr_pending`, `pr_pending>done`, `done>archived`)
     are never collapsed.
- Totals are reported as `raw_events`, `real_transitions`, and `thrash_episodes`, so the numbers reconcile with the 258k figure.
- A short loop that stays under `N` is not an episode and counts as normal flow. That matches the 278-events-today case of small
  loops, which should show up in the raw counts and in a per-item "transition rate" anomaly list, not as episodes.
- Cost: collapsing is O(rows) in Go on rows already bounded by the window. For an all-time window it is ~258k rows. That must
  stay in the same code path but is not the SLO path, so mark all-time responses `estimated: false, slow_path: true` rather than special-casing.

## 5. One aggregation service, two consumers

```
 web-app (Flow view) --ConnectRPC--> BacklogFlowService (server/services/backlog_flow_service.go, thin adapter)
                                                  |
 Claude (stdio) --proxy.go--> HTTP /mcp --> backlogFlowHandlers (server/mcp/tools_backlog_flow.go, thin adapter)
                                                  |
                                            flow.Aggregator  (session/flow, pure logic + TTL cache)
                                                  |  flow.Reader interface
                                            session.EntFlowReader (session/ent_repository_flow.go, SQL)
```

- Both adapters only translate: proto <-> `flow.Summary`, MCP args <-> `flow.Query`, and JSON out of the same structs.
  Windows, filters, thresholds, and cause classification live only in `flow`. A test asserts that the RPC and the MCP tool return
  identical JSON for the same query (requirement: no duplicated logic).
- Surface (one call per success metric):

  | Capability | Success metric | Method |
  |---|---|---|
  | Funnel and per-stage dwell | (a) | `GetFlowSummary(window)` |
  | Rework cycles per merged item, first-pass yield | (b), (e) | `GetFlowSummary` |
  | Stuck reasons, current vs historical, with time-in-stuck | (c) | `GetFlowSummary` |
  | Deadlock candidates | (d) | `ListDeadlockCandidates(window)` |
  | Thrash episodes and per-item rate anomalies | (f) | `ListThrashEpisodes(window)` |
  | Per-item timeline with thrash collapsed | drill-down | `GetItemFlowTimeline(item_id)` |
  | Before/after comparison | tuning check | `CompareFlowWindows(a, b)` |

  MCP tool names are `get_flow_summary`, `list_flow_deadlock_candidates`, `list_flow_thrash_episodes`,
  `get_item_flow_timeline`, `compare_flow_windows`. All are read-only.
- Deadlock heuristic (d). Per item, over its verdict sequence ordered by `review_verdicts.created_at`: at least 2 consecutive
  FAIL or PARTIAL verdicts, with an `in_progress>review` event after each (the agent re-requesting review is the "I'm done" claim),
  or `rework_cap` / `bounce_cap_exhausted` stuck rows open. Flag UNVERIFIABLE streaks separately, since they point at the reviewer
  rather than the agent. All outputs are labeled as heuristic.
- Join risk (requirements Feasibility Risks): verdict to item goes `review_verdicts -> item_sessions -> backlog_item`. One SQL join
  in the reader, covered by a test with a verdict whose session has no `ended_at`.

## 6. How the MCP server reaches the data

- `backlogHandlers` already holds `storage *session.Storage` and `backlogSvc *services.BacklogService` (`server/mcp/tools_backlog.go:323`,
  `:336`). Existing precedent for reusing a service rather than reimplementing is documented at `:876-879`
  ("Pattern A — call the in-process RPC handler"). `NewCore` takes both at `server/mcp/server.go:43` and registers at `:77`.
- Recommendation: pass the `*flow.Aggregator` into `NewCore` as one more optional argument, the same way `backlogSvc` is passed.
  Nil means the tool returns `ErrUnavailable`, matching the `link_session_to_item` convention (`tools_backlog.go:336-340`).
- `proxy.go` (`server/mcp/proxy.go:1-17`) forwards every stdio tool call to the running server's `/mcp` endpoint, so subagent
  processes never open their own SQLite connection. New tools get this for free. The fallback to a local core (when no daemon
  runs) would open its own storage. Flow tools should work there too because the aggregator only needs `session.Storage`.
- The deferred tool list shows `get_backlog_stuck_report`, but no such tool exists in this worktree (`grep -rln stuck_report` over
  the repo returned nothing). It may exist only in the running binary. UNVERIFIED. Check before naming collisions in the plan phase.
- The live `stapler-squad` MCP endpoint was unreachable in the originating session, so tool design here is verified against code and
  `server/mcp/tools_backlog_test.go` conventions, not a live call.

## 7. Capture gaps (Open Question 2: is "agent believes done" captured?)

| Signal | Where it exists today | Sufficient? |
|---|---|---|
| Agent asked for review | `in_progress>review` status event, written by `request_review` (`server/mcp/tools_backlog.go:1223`) or by the reconciler when all work sessions end (`session/storage_backlog.go:892`, note "all work sessions ended") | Yes. The note separates agent-claimed from reconciler-inferred. 37 "all work sessions ended" rows vs 633 total `in_progress>review`. |
| Autonomous driver said DONE | `onAutonomousDriverComplete` (`server/services/autonomous_orchestration_service.go:245`), in-memory only | No. Not persisted. |
| Work session end reason | `item_sessions.end_reason`, NULL for 501 of 505 work sessions (VERIFIED) | No. |
| Rework cap hit | `notifyReworkCapHit` (`server/services/backlog_service_triage.go:169`), ephemeral notification | Partly. Surfaces as a `rework_cap` stuck row. |
| PR-fix nudge outcome | `steerActiveSessionForPRFix` (`server/services/backlog_service_pr_fix_steer.go:245`) | No event. |

Minimum new capture, only if the first read shows the aggregates are too coarse. Each is an additive write that goes through one helper:
1. Persist `end_reason` for work sessions on `onAutonomousDriverComplete` (`done` / `max_turns` / `error`). Closes the "agent believes done" gap.
2. A `backlog_flow_marker` row (or a typed `note` prefix on an existing event) for PR-fix steer outcomes: `steered`, `respawned`, `skipped_active`.
Defer both to a later task in the plan, behind the read-only layers.

## 8. Web view placement (Open Question 4)

- `web-app/src/app/backlog/` has `board/`, `layout.tsx`, and `page.tsx`. `web-app/src/app/analytics/` holds only `escape/`, so `/analytics`
  has no general slot to reuse.
- Recommendation: a sibling route, `web-app/src/app/backlog/flow/page.tsx`, sharing `backlog/layout.tsx`, with a Board/Flow switch
  in the layout. This avoids growing `BacklogBoard.tsx` and gives deep links per window.
- Needs a feature-registry entry with `tested:true` and `testIds` (CI gate in project memory), a `// +feature:` marker, and an e2e
  spec following `e2e-test-conventions`.

## 9. Overlap with earlier backlog plans

| Plan | Overlap | Action |
|---|---|---|
| `backlog-stuck-item-visibility/research/architecture.md` (section 3, section 7) | Chose to persist stuck state in `backlog_stuck_states` and derive the rest. Its ECP table (`:210-224`) is the baseline for the table below. | Reuse the table as the source for stuck-time. Do not rebuild detection. |
| `backlog-session-thrashing/research/architecture.md` | Covers `AutonomousDriver` turn loop and the orchestrator hallucinating DONE (`session/autonomous_driver.go`, `:192-276`). | Source for the "agent believes done" discussion. No new store. |
| `backlog-item-activity-log/research/architecture.md` | Pure-append notes pattern (section 3a). | Follow the same append pattern for any new capture row. |
| `backlog-service-refactor/research/architecture.md` (`:16-37`, `:187-215`) | File split inside `package services`, ent leakage into DTOs. | Consistent with the seam: no new ent types in DTOs (the `flow.Reader` returns plain structs). |
| `backlog-bounce-escalation`, `backlog-pr-mergeability-policy`, `backlog-pr-conflict-detection` | Define the stuck reasons and PR-fix paths that the Flow view reports on. | Read-only consumers. Not re-opened here. |

## 10. Event-Command-Policy table, factory flow

Covers the policy pieces the Flow layer observes. "Observable today" says whether a durable row exists for the Flow view to read.

| Domain event | Policy (trigger) | Command | Actor | Source (file:line) | Observable today |
|---|---|---|---|---|---|
| `ItemQueued` (ready/queued, WIP slot freed) | Dequeue sweep when free slots exist | `transitionWithGuard(ready -> in_progress)`, then `spawnSessionAfterGates` | `BacklogService.DequeueNextQueuedItems` | `server/services/backlog_service_triage.go:837`, `:889`, `:922` | Yes, `backlog_status_events` with note "dequeued: WIP slot freed" |
| `DequeueSpawnFailed` | Spawn error after claim | Roll back `in_progress -> ready`, note "dequeue spawn failed" | Same | `server/services/backlog_service_triage.go:922` | Yes, but it is the thrash driver when it repeats |
| `ItemStageEntered` (any status change) | Every transition | `recordStatusEvent` (append, never blocks) | Storage | `session/ent_repository_backlog.go:49-63` | Yes, index only on `(item_id, created_at)` |
| `WorkSessionsAllEnded` | 60s reconcile tick | `in_progress -> review`, note "all work sessions ended" | `ReconcileStuckItems` | `session/storage_backlog.go:851`, `:892` | Yes |
| `AgentRequestedReview` | Agent calls `request_review` | `in_progress -> review`, spawn review gate | MCP handler | `server/mcp/tools_backlog.go:1223` | Yes, as a status event |
| `ReviewVerdictRecorded` | Review session calls `submit_review_verdict` | Persist `review_verdicts`; PASS then push and PR, FAIL/PARTIAL then rework | Review gate | `session/backlog_lifecycle_review.go:49`, `:367` | Yes, `review_verdicts` (join via `item_sessions`) |
| `ReviewSessionExitedNoVerdict` | Review session ends without `submit_review_verdict` | Treat as failed review, notify | Lifecycle listener | `session/backlog_lifecycle_review.go:100-127` | Partial: only the notification |
| `VerdictOverridden` | Operator override | `OverrideVerdict` sets `override_by/at/reason` | Human via RPC | `server/services/backlog_service_lifecycle.go:1065`; schema `session/ent/schema/review_verdict.go` | Yes (5 rows) |
| `ReviewFailed` | FAIL/PARTIAL verdict | `AutoReopenAfterFailedReview`: `review -> in_progress`, respawn work session | `BacklogService` | `server/services/backlog_service_triage.go:1722` | Yes (398 rows, note "auto-reopened after failed review verdict") |
| `ReworkCapHit` (work sessions >= `MaxAutoReworkIterations`, default 20; per-item `rework_cap_override`) | Checked inside `AutoReopen*` | `notifyReworkCapHit`, mark `rework_cap` stuck | `BacklogService` | `server/services/backlog_service_triage.go:169`; `config/config.go:296-301`, `:1039-1051`; `session/ent/schema/backlog_item.go:130` | Partial: stuck row only. The cap count is not stored as an event. |
| `StatusBounceDetected` (>= 3 `in_progress->review` round trips in 24h, no PASS) | Reconcile tick | `MarkStuck(bouncing)`, escalate | `reconcileBouncingItems` | `session/backlog_lifecycle.go:1675`; `session/stuck_decisions.go:35`, `:40` | Yes, stuck row (71 all-time) |
| `WorkSessionStale` (no progress > 2h) | Reconcile tick | `MarkStuck(stale_work)` | `reconcileStaleWorkSessions` | `session/backlog_lifecycle_stale.go:64` | Yes (55) |
| `ReviewAbandoned` (verdict exists, nothing in flight for 15 min) | Reconcile tick | `MarkStuck(abandoned_review)` | `FindStuckReviewItems` | `session/storage_backlog.go:961`; `session/stuck_decisions.go:23-27` | Yes (65) |
| `PRPolledReadyUnmerged` (CI green, mergeable, unmerged, >= 30 min) | PR poller | `MarkStuck(pr_ready_unmerged)` + notify | `ReconcilePRPending` | `session/backlog_lifecycle_pr.go:1592`; `session/stuck_decisions.go:20` | Yes (50) |
| `PRNeedsFix` (CI failure, requested changes, or merge conflict) | PR poller | `AutoReopenForPRFix`: `pr_pending -> in_progress`, respawn | `BacklogService` | `server/services/backlog_service_triage.go:2108-2117` | Yes (4437 rows). Thrash source on `a90b99cb`. |
| `PRFixSessionStillActive` | `AutoReopenForPRFix` finds a live work session | Steer it, no status flip | `steerActiveSessionForPRFix` | `server/services/backlog_service_pr_fix_steer.go:245`; guard `server/services/backlog_service_triage.go:2145-2158` | **No.** Steer outcome has no durable row (capture gap 2). |
| `ThrashEpisodeDetected` | **New:** Flow query, no write | None. Surfaced in `ListThrashEpisodes` | `flow.Aggregator` | this plan, section 4 | New, derived |
| `DeadlockCandidateDetected` | **New:** Flow query, no write | None. Surfaced in `ListDeadlockCandidates` | `flow.Aggregator` | this plan, section 5 | New, derived |
| `PRMerged` | Poller sees merged PR | `pr_pending -> done` | `ReconcilePRPending` | `session/backlog_lifecycle_pr.go:1592` | Yes (106 rows) |
| `ItemArchived` | Lifecycle archive | `done -> archived` | Lifecycle | `session/backlog_lifecycle_archive.go` | Yes (156) |

Gaps worth a policy later (second pass, not this one): a thrash-rate guard in `DequeueNextQueuedItems` and `AutoReopenForPRFix`
(for example, back off after N consecutive rollbacks on one item), and a persisted rework-cap counter. Both change pipeline behavior,
so they are outside this feature.

## 11. Risks and notes

- **Single connection.** Any flow query blocks writers (section 2.3). Keep queries short and indexed. Do not hold the connection
  across the Go-side collapse; read rows first, then compute.
- **`backlog_stuck_states` is one row per (item, reason).** Historical stuck time for a reason that recurs is lost, so "time-in-stuck,
  historical" is a lower bound for repeated reasons. Mark it so in the response.
- **Note-prefix classification is brittle.** Mitigated by the single classification table and the unclassified-note test.
- **Multiple workspace DBs.** The aggregator reads through the live `session.Storage`, so it follows whichever DB the instance opened.
  No path is hard-coded.
- **No restart for testing.** Use the manual-instance recipe in `CLAUDE.md` (ports from the manual dev port block).
