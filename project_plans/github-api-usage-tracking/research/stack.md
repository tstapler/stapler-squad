# Stack Research: GitHub API Usage Tracking

## Summary

No new external dependency is needed for any part of this feature. Every
building block — persisted event storage, a config value read fresh per
call, a charting library, and a file-watch pattern for hot-reload — already
exists in this repo and has a canonical precedent to copy.

---

## 1. Persistence: reuse the `ent` + SQLite pattern from `analytics_store.go`

`go.mod` already pulls in:
- `entgo.io/ent v0.14.5` (direct)
- `github.com/mattn/go-sqlite3 v1.14.40` (direct, cgo-based SQLite driver)

There is no separate flat-file/JSON-only precedent for event history — the
repo's *existing* answer to "persisted event history with per-source
breakdown" is `server/services/analytics_store.go`'s `AnalyticsStore`, which:

- Is backed by an `ent.Client` over SQLite (`session/ent/schema/analytics_event.go`
  defines the `AnalyticsEvent` entity: `event_name`, `event_category`,
  `session_id`, `duration_ms`, `labels` (a `field.JSON(map[string]string{})`
  for open-ended attribution), `created_at`, plus compound indexes on
  `(event_name, created_at)`).
- Buffers writes asynchronously (bounded channel, `Start(ctx)`/`Stop()`,
  drops-with-WARN-log on overflow — see `AnalyticsStore.Record` at
  `server/services/analytics_store.go:175`) so the hot path (an HTTP
  round-trip or `gh` shell-out) never blocks on a DB write.
  This is the "negligible overhead" requirement essentially for free.
- Exposes windowed reads (`LoadWindow`, `LoadProgramWindow`,
  `GetSubcommandBreakdown`) that a UI panel queries — the same shape needed
  for "request volume over time" + "breakdown by source."
- Has a configurable retention cap already wired into `config/`:
  `Config.AnalyticsMaxRowsOrDefault()` and `Config.AnalyticsMaxAgeDaysOrDefault()`
  (`config/config.go:584`, `:641`) — the natural place to add a parallel
  `GitHubUsageMaxAgeDaysOrDefault()` or reuse the same knobs.

**Recommendation**: add a new ent schema (e.g. `GitHubAPIEvent` or extend
`AnalyticsEvent`'s `event_category` taxonomy with a `github_api` category)
rather than inventing a second storage mechanism. This directly resolves the
Rabbit Hole flagged in requirements.md ("Historical storage choice") — reuse,
don't parallel-build.

### ent migration workflow (per repo `CLAUDE.md` / `.claude/rules/ent-schema-generation.md`)

```bash
# edit/add schema file under session/ent/schema/
go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema
go build ./...
# commit all session/ent/ generated output together with the schema change
```

The `--feature sql/upsert` flag is mandatory — omitting it silently breaks
`Upsert*` methods (compiles, but the upsert methods don't exist/behave).
Confirmed via `session/ent/generate.go`'s `//go:generate` directive, which is
the authoritative source for this command.

---

## 2. Charting: `recharts` is already a `web-app` dependency

`web-app/package.json` already lists `"recharts": "^3.8.1"` as a runtime
dependency (compatible with the already-pinned `react@^19.0.0` /
`react-dom@^19.0.0`). It's already in active use for exactly this kind of
time-series-with-range panel:

- `web-app/src/app/insights/DailySpendChart.tsx` — `ResponsiveContainer` +
  `LineChart` + `Line`/`XAxis`/`YAxis`/`Tooltip`/`CartesianGrid`, driven by a
  typed `DailyTokenBucket[]` prop straight off a protobuf-generated type
  (`@/gen/session/v1/insights_pb`).
- `web-app/src/app/insights/ModelBreakdownChart.tsx`,
  `ModelOverTimeChart.tsx` — same library, breakdown-by-category framing
  (directly analogous to "breakdown by source" here).

**Note on the UI-panel convention named in requirements.md**
(`ApprovalAnalyticsPanel.tsx`, `web-app/src/components/sessions/`): that
specific panel does *not* itself import `recharts` — it renders its own
bars/tables via hooks (`useApprovalAnalytics`) and CSS
(`ApprovalAnalyticsPanel.css.ts`, per the repo's vanilla-extract convention —
see `.claude/rules/css-architecture.md`). Follow `ApprovalAnalyticsPanel.tsx`
for panel *structure* (hook-backed data fetch, `.css.ts` styling, filter/tab
layout) and the `insights/*Chart.tsx` files for the actual `recharts` time-series
+ range-selector implementation — the two conventions compose rather than
conflict.

No `@storybook`/`chart.js`/`d3` or other visualization library needs adding.

---

## 3. Config: no hot-reload watcher exists for `config.json` — but `fsnotify` is already a dependency and the poller pattern to adopt is nearby

Grepped `config/config.go`, `config/singleton.go`: there is **no** `Watch`/
file-watcher mechanism for `config.json` itself. `LoadConfig()`
(`config/config.go:782`) is a plain synchronous read-and-parse function —
`server/services/defaults_service.go` and others call it fresh per-request
(no process-lifetime cache), so *most* config values are effectively "hot"
already: edit `config.json`, next `LoadConfig()` call picks it up, no
restart needed.

The two poll intervals in scope are the exception, because they're captured
once into a `time.Duration` at construction and locked into a
`time.NewTicker` for the life of the process:

- `session/pr_status_poller.go:178` — `ticker := time.NewTicker(p.config.PollInterval)`
  inside `PRStatusPoller.Start(ctx)`, config value set once via
  `NewPRStatusPollerWithConfig`.
- `session/worktree_pr_poller.go` — same shape (`DefaultWorktreePRPollerConfig`,
  `PollInterval` field), same construction-time-only binding.

So "config-driven poll interval, no rebuild" is **true but not free** — it
needs either:
1. The poller loop periodically re-reads `config.LoadConfig()` (e.g. once
   per existing tick) and calls `ticker.Reset(newInterval)` when the value
   changed, or
2. An `fsnotify` watch on `config.json` that triggers a poller restart/reset.

`fsnotify v1.9.0` is already a **direct** `go.mod` dependency and is already
used for exactly this shape of problem elsewhere in the codebase — no new
dependency required either way. Canonical precedent to copy:
`session/detection/plugin_watcher.go`'s `PluginWatcher`:
- Watches the *directory*, not the individual file (editors' write-temp-then-
  rename save pattern silently breaks per-file watches after the first save
  — documented in that file's own comment).
- Debounces bursts (`pluginReloadDebounce = 200 * time.Millisecond`) since
  one logical save often fires multiple fsnotify events.
- Falls back to a periodic safety-net rescan (`pluginRescanInterval = 60 *
  time.Second`) for environments where fsnotify doesn't fire reliably
  (network mounts/containers) — `fsnotify` is treated as an optimization,
  not the sole reload path.

Given the existing `LoadConfig()`-is-already-called-repeatedly pattern
throughout the codebase, **option 1 (periodic re-check inside the poller's
own existing tick, no fsnotify) is simplest and consistent with how the rest
of `config/` already behaves** — it avoids adding a watcher goroutine per
poller for a value that's read at most once every 60s anyway. Recommend
Phase 3 planning default to option 1 and only reach for the
`PluginWatcher`-style fsnotify approach if instant-apply (sub-tick-interval)
responsiveness is an explicit requirement — it currently isn't per
requirements.md's Success Metrics.

This resolves the requirements.md Open Question ("Does `config/` already
support hot-reload...") definitively: **no**, not for values bound at
construction time like poll intervals, but the fix is cheap and needs no new
dependency.

---

## 4. Dependency summary

| Need | Existing dependency | New dependency required? |
|---|---|---|
| Time-series/event persistence | `entgo.io/ent` v0.14.5 + `github.com/mattn/go-sqlite3` v1.14.40 (both direct in `go.mod`) | No |
| ent codegen tooling | `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert` (per `session/ent/generate.go`) | No |
| Historical charts + time-range selector | `recharts` ^3.8.1 (`web-app/package.json`, React 19-compatible) | No |
| Config-driven poll interval, no rebuild | `config.LoadConfig()` (already re-read-on-call elsewhere) + optional `fsnotify` v1.9.0 (already direct dep, pattern in `session/detection/plugin_watcher.go`) | No |
| Async non-blocking write path | Buffered-channel pattern in `server/services/analytics_store.go` (`AnalyticsStore.Record`) | No |

No community-version-currency check was needed beyond confirming these are
already pinned, current-enough versions in an actively maintained `go.mod`/
`package.json` — this feature adds zero new packages to either manifest.
