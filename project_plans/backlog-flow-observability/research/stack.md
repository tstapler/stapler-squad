# Stack research: backlog-flow-observability

Confidence labels: VERIFIED = code read or command run this session; INFERRED = reasoning.

## 1. Which store the live service reads (answers Open Q1)

- VERIFIED: backlog tables (`backlog_status_events`, `backlog_stuck_states`, `item_sessions`, `review_verdicts`) live in the **workspace DB** `~/.stapler-squad/workspaces/6eb0b580fa0331d5/sessions.db`, opened by ent in `session/ent_repository.go:150-175` (modernc `sql.Open("sqlite")`, WAL, busy_timeout 5s, **`SetMaxOpenConns(1)` at :164**).
- VERIFIED: `analytics.db` is a different file at `~/.stapler-squad/analytics.db`, opened in `server/dependencies.go:1167` via `analytics.OpenAnalyticsDB` (`server/analytics/db.go:25-68`). On disk it is 8192 bytes, `.tables` returns nothing, last modified Aug 11. It holds no stage data.
- VERIFIED: `stage_transitions` in the requirements doc is a misread. The ent `StageTransition` (`session/ent/schema/stage_transition.go:23-60`) is the pipeline-graph config (from_stage_id/to_stage_id/enabled/gates), not a dwell-time log. **Single source of truth for dwell = `backlog_status_events`** (plus `backlog_stuck_states` for stuck time and `item_sessions.started_at/ended_at/end_reason` for session time). Do not build on analytics.db.
- Implication: aggregate code must go through the same `*session.Storage`/ent client the BacklogService uses (`server/services/backlog_service_stuck.go:157-176` is the pattern: `s.storage.<Find...>(ctx)`, nil-storage guard, `connect.CodeInternal` wrap), so it automatically follows whichever workspace DB is live (Rabbit Hole 3). Never hardcode a hash path.

## 2. Schema and index facts (VERIFIED, schema files + `sqlite_master` on live DB)

| Table | Existing indexes | Gap for flow queries |
|---|---|---|
| `backlog_status_events` | `(item_id, created_at)` (`schema/backlog_status_event.go:47-51`) | none on `created_at` alone, `(to_status,created_at)`, or `note`. Window queries scan the table (plan: `SCAN backlog_status_events` + temp b-tree). |
| `backlog_stuck_states` | unique `(item_id, reason)` (`schema/backlog_stuck_state.go:85-91`) | one row per (item, reason), resolve-in-place upsert, so historical recurrence is not preserved; only `first_detected_at/resolved_at` of the latest cycle. "Historical" counts of stuck reasons are lower bounds. |
| `item_sessions` | `session_uuid`, `(created_at, backlog_item)`, `backlog_item` | OK for item to sessions join. |
| `review_verdicts` | unique edge `item_session` (one verdict per item_session) | join path: `review_verdicts -> item_sessions (FK) -> backlog_items`. Verdict has `created_at` and `override_*`. Index adequate at 526 rows. |

Measured on the live DB (258k rows): a 30-day `GROUP BY from_status,to_status` full scan takes 0.03s user / 0.19s wall via sqlite3 CLI. A raw scan would meet the <1s SLO today, but see the thrash caveat (still, collapse in SQL/Go, never page every raw row to the app).

**Timestamp trap (VERIFIED, `select substr(created_at,-6) ... group by`)**: `created_at` is stored as TEXT with mixed offsets: 247,333 rows `+00:00`, 10,654 rows `-07:00` (early rows). Lexicographic comparison (`created_at > datetime('now','-30 day')`, or `>= ?` with a Go time bound) is silently wrong across the two offset forms, and `datetime('now')` has no offset or fractional part. Use `julianday(created_at)` / `unixepoch(created_at)` (SQLite normalizes offsets) or, preferably, do window filtering in Go on parsed `time.Time` after an index-bounded fetch. The DSN uses `_texttotime`/`_time_format=sqlite` (`session/ent_repository.go:123-135`), so ent returns correct `time.Time`; raw SQL does not get that for free.

## 3. Compute strategy: Go-side over bounded ent queries, not SQL views (recommendation)

- Thrash collapse (consecutive `ready<->in_progress` with notes "dequeued: WIP slot freed" / "dequeue spawn failed") is a run-length/gap-fold over each item's ordered events. Expressible as window functions (`LAG` + running-sum islands) in SQLite 3.25+, but it needs the `note` free text and episode semantics that will evolve; versions of SQL in a view are hard to unit-test and ent does not model views (need raw `sql.Selector`/`QueryContext`).
- Recommended: one Go package (e.g. `session/flow` or `server/services/backlog_flow_*.go`) that (a) loads events for the window in a single query ordered by `(item_id, created_at)` using the existing index, projecting only `item_id, from_status, to_status, created_at, note` (no wide columns), (b) folds per item in one pass, emitting episodes (thrash episode = count, first/last, duration), dwell per stage, rework cycles, (c) joins stuck rows and verdicts via two additional bounded queries and in-memory maps. Pure functions on `[]Event` make table-driven tests trivial (project rule: deterministic fast tests).
- Two items account for ~175k events, so the bulk of the volume is **thrash on 2 items**. Cheap, SQL-side pre-aggregation is worth it for that: exclude/aggregate noise rows server-side with a `WHERE NOT (note LIKE 'dequeued:%' OR note LIKE 'dequeue spawn failed%')` pass plus a separate `GROUP BY item_id, ... COUNT(*), MIN, MAX` for the thrash set. Then rows fetched to Go drop from 258k to roughly 19k real transitions. INFERRED: counts from requirements (238,552 flips of 257,973).
- Index recommendation (additive ent schema change via `--feature sql/upsert` flow; generated output not committed): add `index.Fields("created_at")` and optionally `index.Fields("to_status","created_at")` to `BacklogStatusEvent`. Verify with `EXPLAIN QUERY PLAN` on the live-size copy (requirement "index additions must be verified"). Because of the mixed-offset text, an index on raw `created_at` only helps if the bound comparison is lexicographically valid; first consider a one-time normalizing migration of the 10,654 `-07:00` rows, or accept the Go-side window filter.
- Materialized rollups: INFERRED not needed for SLO at 258k rows (scan is 0.03s CPU). Defer; if retention (Open Q5) later prunes raw events, add a small `backlog_flow_rollups` table at that time. No existing retention for status events was found (grep of retention files returned nothing; `server/analytics/retention.go` covers analytics.db only: UNVERIFIED beyond file existence).
- **Concurrency risk (VERIFIED)**: `db.SetMaxOpenConns(1)` (`session/ent_repository.go:164`) serializes ALL storage access. A multi-second aggregate holds the only connection and stalls every writer (liveness engine, status transitions). Keep each query short and bounded, no per-item N+1 loops, run the fold outside the DB call, and add a small TTL cache (e.g. 30-60s, keyed by window) for the RPC so UI refreshes and MCP polling cannot hammer the single connection. Log duration/rows at debug (requirement).

## 4. ConnectRPC / proto (VERIFIED)

- Service `BacklogService` at `proto/session/v1/backlog.proto:1244`. Precedent for read-only aggregate RPC: `ListStuckBacklogItems` (`backlog.proto:1473`; handler `server/services/backlog_service_stuck.go:157`). New RPCs follow: add `rpc GetBacklogFlowSummary`, `GetBacklogFlowTimeline`/`ListFlowItems` (drill-down), `CompareBacklogFlowWindows` in the same service; `make proto-gen` (CLAUDE.md "New API Endpoints"); handler needs `// +api: backlog:<name>` marker; run `make registry-generate` and set `tested:true` + `testIds` in `docs/registry/features/` (CI gate, memory note: registry tested:true gate).
- Response messages should carry `as_of` (Timestamp), `window` echo, `partial`/`estimated` flags per metric (requirement Observability), and reuse enums/status strings already in `backlog.proto`.
- Shared aggregate type: define the result once as proto messages and have the MCP tool marshal the same message with `protojson` so UI and MCP cannot diverge (INFERRED design; check `tools_backlog.go` for how other tools serialize, below).

## 5. MCP tools (VERIFIED)

- Registration: `registerBacklogTools(s *mcpserver.MCPServer, h *backlogHandlers)` at `server/mcp/tools_backlog.go:2870`; tools are added with `s.AddTool(mcpgo.NewTool("name", mcpgo.WithDescription(..), mcpgo.WithString/WithArray/WithNumber(..., mcpgo.Required())), h.handler)` (e.g. `get_backlog_item` at :2871-2880, `list_backlog_items` at :2882+, which also shows the "default limit 10 to protect LLM context" convention). Library: mark3labs `mcp-go` (`mcpgo`/`mcpserver`).
- `backlogHandlers` (:322-330) holds `storage *session.Storage` and `store session.InstanceStore`; a flow handler should use `h.storage` and call the same Go package as the RPC (single implementation, two transports).
- Tests alongside: `server/mcp/tools_backlog_test.go` (per requirements). Keep MCP responses compact (top-N with `limit`, per-item timelines collapse thrash, include `as_of`) since output goes into LLM context.
- Proposed tools: `get_backlog_flow_summary(window_days|from,to)`, `get_backlog_item_timeline(item_id)` (collapsed), `list_backlog_flow_anomalies` (thrash, deadlock candidates), `compare_backlog_flow_windows(a_from,a_to,b_from,b_to)`. Note an existing `get_backlog_stuck_report` tool is deferred in the tool list, but grep of `server/mcp/*.go` for that name found no match, so UNVERIFIED where it is defined; check before naming to avoid overlap.

## 6. Web UI (VERIFIED)

- Next.js 15.3.2 + React 19 (`web-app/package.json:101-102`), **recharts ^3.8.1 already a dependency** (`:109`), used in `web-app/src/app/insights/` (`DailySpendChart.tsx` uses `ResponsiveContainer, LineChart, Line, XAxis, YAxis, Tooltip, CartesianGrid`). No new chart lib needed. Funnel/stage-dwell fits `BarChart` (stacked horizontal); Sankey is also in recharts but is optional; keep to bars/lines.
- CSS: vanilla-extract (`@vanilla-extract/css`, `/recipes`, `/next-plugin`; `.css.ts` siblings per component, e.g. `DailySpendChart.css.ts`). `pnpm run lint` runs `lint:css` and `lint:css-vars`, so use theme vars, not raw colors. Package manager is pnpm (CLAUDE.md).
- Component conventions to copy from Insights: `// +feature: insights-dashboard` marker in first 10 lines, `"use client"`, props typed from `@/gen/session/v1/*_pb`, sibling `*.test.tsx` (jest) per chart, `InsightsDashboardSkeleton.tsx` for loading, `insightsFormatters.ts` for formatting.
- Routing (answers Open Q4): `web-app/src/app/analytics/` contains only `escape/` (no page, no slot owned). `/backlog` has `layout.tsx`, `page.tsx`, `board/` (`web-app/src/app/backlog/`). Existing system-level aggregate UI is `/insights` (token spend), distinct domain. Recommend `web-app/src/app/backlog/flow/page.tsx` as a sibling of `board/`, sharing `backlog/layout.tsx` (tab nav alongside Board); this satisfies "Flow view alongside Board". Check `backlog/layout.tsx` for how the Board tab is rendered (INFERRED that it hosts the tab strip; I did not open it).
- E2E: Playwright spec in `tests/e2e/` with `// @feature` header, `data-testid`/ARIA locators only, no `waitForTimeout`; page helper in `tests/e2e/pages/`.

## 7. Existing analytics code to reuse / avoid

- `server/analytics/` (db.go, provider.go, sqlite_provider.go, retention.go, subscriber.go, escape_event_batch_writer.go): event-subscriber pipeline writing analytics.db (escape events, error events, token usage). Not backlog-flow data; analytics.db is empty. Reuse the pattern only if a new capture event is needed, otherwise ignore.
- `InsightsService` (`server/server.go:485-487`, `sessionv1connect.NewInsightsServiceHandler`, registered under `/api`): the precedent for registering a new service handler; a new RPC on the existing `BacklogService` avoids touching server.go routing.

## 8. Answers / flags for the plan

1. Dwell source of truth: `backlog_status_events` (workspace DB) only; `analytics.db` empty and `StageTransition` is graph config.
2. Index adds: `created_at` (and optionally `(to_status, created_at)`) on `backlog_status_events`; consider normalizing mixed-offset timestamps. No new index needed on verdict/session joins at current size.
3. SQL view/CTE vs Go: Go fold over bounded ent query, with SQL-side noise exclusion/grouping to cut 258k rows to ~19k; TTL cache because of the single-connection pool.
4. Nothing in this pass establishes agent "believes done" capture (Open Q2); not investigated here (out of stack scope).
