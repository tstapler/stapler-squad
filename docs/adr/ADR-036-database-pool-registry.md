# ADR-036: Many small per-use-case SQLite databases behind a typed, WAL-enforcing registry

Status: Proposed — 2026-10-30 (revision 3, after owner refinement and review rounds 1-2). Numbered 036 because ADR-034 (#1309) and ADR-035 (#1314) are taken.

## Context

Owner decisions (verbatim): "we definitely need multiple db pools/clients for different databases." and "In the per database it's
much better to have smaller per-usecase databases which don't share data if they don't need joins, making sure we have indexes,
and that our databases use WAL by default."

So the target is many small databases split by use case wherever no query, foreign key or transaction crosses the boundary.
This ADR makes that correct and states what it does not buy. Evidence below is VERIFIED on `origin/main` f52e50b1c and
`.backup` copies of the live DBs unless marked INFERRED.

### Inventory of SQLite files and connections

Live workspace `~/.stapler-squad/workspaces/6eb0b580fa0331d5/`: `sessions.db` 454 MB, `analytics.db` 490 MB,
`stapler-squad.db` 0 bytes. `PRAGMA journal_mode` on a `.backup` copy of every non-empty `*.db` under `~/.stapler-squad`
(57 files, excluding manual-build and worktree dirs): **57 of 57 are `wal`** (37 with `auto_vacuum=0`, 20 with `2`=incremental).
The 0-byte `stapler-squad.db` has no header yet, so no mode; it is unused (INFERRED from the empty file; not grepped to
exhaustion). So WAL is already true in practice; what is missing is enforcement and a metric.

| File | Opened by | Pools (writer/readers) | Pragmas |
|---|---|---|---|
| `sessions.db` | `session.NewEntRepository` (`session/ent_repository.go:178`, `ent_client_instrumented.go:48`); `dbobs` names `ent`, `ent-read` (`dbsplit`) | 1 / 4 (`STAPLER_SQUAD_DB_READ_CONNS`) | WAL, busy_timeout 5 s, FK, auto_vacuum=incremental, ent time compat; `synchronous=NORMAL` only if #1289 merges |
| `analytics.db` | `analytics.OpenAnalyticsDBWithDriver` (`server/analytics/db.go:34`), `dbobs` name `analytics` | 1 / 0 | WAL, NORMAL, FK, busy_timeout 5 s, `wal_autocheckpoint=1000`, auto_vacuum=incremental; no ent time compat |
| `internal/storage/sqlite.go` `NewSQLiteStore` | no caller (grep finds none outside its file) | n/a | dead code |
| Foreign / ad hoc: `opencode_adapter.go`, `agy_adapter.go`, `capacity_monitor.go:770`, `database_service.go:236,251,362`, `workflow_engine_replay.go:125`, `scripts/soak` | `sql.Open("sqlite", ...)` via `internal/sqlitedsn` | short-lived | various; opencode/agy files belong to other tools and are opened read-only |

Everything else (workflow ledger, claims, audit, tokens, backlog, context history) is in `sessions.db` through one ent client
(`session/contexthistory/store.go` takes the state `*ent.Client`). Both ent-managed files run `Schema.Create` for the full
schema: `analytics.db` has 47 tables, 45 empty (only `escape_events` 864,642 rows and `analytics_events` 100,044 have rows);
`sessions.db` carries empty copies of `escape_events` and `analytics_events`.

A latent cross-file bug the split must make impossible: `ListAnalyticsBySessionID` (`session/ent_repository.go:2030`) queries
`AnalyticsEvent` through the **state** client `r.client`, but the live rows are in `analytics.db`; `sessions.db.analytics_events`
has 0 rows, so the session bundle's analytics section (`server/services/session_bundle_service.go:351`) is empty regardless of intent: the file mismatch is real even though `AnalyticsEvent` rows carry fewer fields than classification rows.

### Tables, sizes and write rates (`dbstat` on the copy; rate = rows / days between min and max timestamp)

| Table (file) | Rows | MB incl. indexes | Rate |
|---|---|---|---|
| `classification_analytics` (sessions) | 427,630 | ~283 | ~4,200/day |
| `backlog_status_events` (sessions) | 249,615 | ~74 | ~2,900/day |
| `context_samples` (sessions) | 182,281 | ~33 | ~2,200/day |
| `diff_stats` (sessions) | 214 | 26.6 | rewritten per session |
| `escape_events` (analytics) | 864,642 | ~420 | ~123,000/day, 7-day retention |
| `analytics_events` (analytics) | 100,044 | ~46 | ~20,000/day (INFERRED, 5 days of history) |
| all other tables | under 3,000 rows each | under 3 each | low |

Write volume is small (4,200 rows/day is one per 20 s). The earlier finding stands: multi-second `RecordAnalyticsBatch` stalls
with `pool_wait 0` are process-wide CPU starvation plus a large heavily-indexed table, not writer contention.

## What the split buys and does not buy

| Buys (mechanism) | Does NOT buy |
|---|---|
| Writer isolation: each use case has its own single writer, so a classification batch, a retention delete or a VACUUM never queues behind backlog or session writes | Relief from CPU starvation or Go scheduler stalls; every pool in the process still shares them |
| Separate WAL per file: one feature's checkpoint never blocks another's readers or writer | Fewer queries; the 906k-query read load was the sweeps fixed by #1288 |
| Blast radius: a corrupt or oversized file affects one feature; telemetry-class files can be deleted and recreated | Cross-DB atomicity or FK enforcement (see invariants below) |
| Cheap per-DB `.backup`, VACUUM, restore, delete; `sessions.db` shrinks by ~316 MB (classification + context history) to ~138 MB | A durability win from `synchronous=OFF`: in WAL, NORMAL already fsyncs only at checkpoint and OFF risks corruption on OS crash; OFF is reserved for a disposable `cache` class |
| Per-DB pool sizes, busy_timeout, autocheckpoint, retention, slow-query thresholds | Gains from bigger `cache_size`/`mmap`: #1289 measured none here; those stay at defaults until a PR 1 benchmark shows a gain |
| | Free lunch: N files cost N pools, N WAL/SHM pairs and fds; mitigated by lazy open and idle close |

## Join and atomicity analysis (boundary justification)

Method: ent edges from `session/ent/schema/*.go` (`edge.To/From`); every `client.Tx`/`BeginTx`/`WithTx` site and the entities used
inside it; `ATTACH`/`JOIN` grep over Go outside generated code; per-table reference fields (`*_id`, `*_uuid`, `*_slug`). Raw SQL
joins: none found outside generated ent code and PRAGMAs. Two tables stay together only if an FK edge, a shared transaction or a
join connects them.

**Clusters connected by FK edges or shared transactions (stay together):**

| Cluster | Tables | Evidence |
|---|---|---|
| Session graph | `sessions`, `worktrees`, `diff_stats`, `tags`+`session_tags`, `claude_sessions`, `claude_metadata`, `shells`, `projects` | edges Session->{Worktree,DiffStats,Tag,ClaudeSession,Shell}, Project->Session; one tx touches Session+ClaudeSession+ClaudeMetadata+DiffStats+Shell+Worktree (`ent_repository.go:863`), Project+Session (`:2211`), Session+Worktree (`worktree_consistency_sweep.go:652`) |
| Backlog | `backlog_items`, `item_sessions`, `review_verdicts`, `backlog_status_events`, `backlog_stuck_states`, `backlog_progress_notes`, `backlog_activity_notes`, `backlog_item_dependencies`, `guidance_requests`, `item_sources`, `source_sync_events`, `deleted_item_session_costs` | edges BacklogItem->each child; item delete tx touches BacklogItem+DeletedItemSessionCost+ItemSession+ReviewVerdict (`ent_repository_backlog.go:1668`); `storage_backlog.go:917` ItemSession+ReviewVerdict |
| Stage graph | `backlog_stages`, `stage_transitions`, `transition_gates`, `gate_satisfaction_records`, `stage_liveness_definitions` | edges among them; stage CRUD tx also writes `backlog_items` (`ent_stage_config_repository.go:547`); `StageConfigFingerprint` (`workflow_stage_seed.go:255-268`) reads stages, transitions, gates and liveness definitions in one function, so liveness stays with the stage graph in the Backlog cluster |

**One FK edge crosses clusters and is dead:** `BacklogItem.sessions` <-> `Session.backlog_items` (many-to-many table
`backlog_item_sessions`, 0 rows). `grep` for `AddSession*`, `QuerySessions`, `HasSessions`, `WithSessions` outside generated code
finds no use. The live link is `item_sessions.conversation_uuid`, a plain string. Splitting Backlog from the Session graph therefore
requires removing that edge from the ent schema (using the repo's required `ent generate --feature sql/upsert` command, generated
output not committed); the empty table is left in place.

**Tables with no edges and no cross-table transaction (free to move on their own):**

| Proposed DB | Tables | Single-table tx / refs |
|---|---|---|
| `classification` | `classification_analytics` | written only by `RecordAnalyticsBatch` (`ent_repository.go:1908`); refs `session_id` as a string |
| `contexthistory` | `context_samples`, `compaction_events` | only `contexthistory/store.go`; `session_uuid` string; rebuildable from transcripts |
| `escape` (file `analytics.db`) | `escape_events` | batch writer; 7-day retention |
| `analytics_events` | `analytics_events` | `analytics` provider |
| `workflow` | `workflows`, `trigger_fire_events`, `webhook_registrations`, `webhook_request_ledgers`, `pipeline_modes` | tx single-table (`ent_repository_webhook_registration.go:184`, `ent_repository_webhook_lifecycle.go:58`); `workflow_id` is a plain UUID, same DB so unchanged |
| `wake` | `wake_triggers` | all tx single-table (`ent_repository_wake*.go`); `owner_session_uuid` string |
| `credentials` | `integration_credentials` | secrets; own file for blast radius and 0600 perms |
| `errors` | `error_events` | none |
| stay in `core` until justified | `session_goals`, `session_summaries`, `handoff_summaries`, `dismissed_findings`, `tagging_rules`, `tagging_rule_fires`, `approval_rules` | keyed by `session_id`/`session_uuid`/`rule_id` strings, no FKs; tiny and low write, so moving buys little |

I did not enumerate every read path for the `core`-stay row; its "no FK, single-table tx" status is from the schema and tx grep only.

**Cross-DB invariants that FKs can no longer enforce, and the app-level handling:**

| Invariant | Handling |
|---|---|
| A moved row's `session_id`/`session_uuid` names an existing session | Never enforced today (no FK on these tables). Orphans are expected telemetry, bounded by retention; session delete publishes a best-effort `SessionDeleted` hook repositories may subscribe to. |
| `wake_triggers.owner_session_uuid` names a live session | Check at fire time (`reviveStoppedOwner` already reads the session); sweep orphans on the wake loop. |
| `sessions.workflow_id` (`session.go:125`, indexed) and `backlog_items.next_workflow_id` (`backlog_item.go:147`, plus the `chain_fired` scan at `:153`) name a `workflows` row | Plain UUIDs with no FK today; readers treat a dangling id as "workflow gone"; the `workflow` DB moves only in a later wave, with a test that these reads tolerate a missing row. |
| Backlog item delete cascades to children | Unchanged: stays in one DB with real FKs. |
| `item_sessions.conversation_uuid` -> `claude_sessions` | A string today; cross-DB lookups are two queries. |
| Retention vs counts on both sides of a copy | Handled in the migration protocol. |

## Decision

1. **Registry.** New leaf package `session/dbpool`. `Registry.Open(ctx, Spec)` returns a typed handle; **opening is lazy** (first
   use, `sync.Once` per DB, schema subset created then); a failed open is a counted no-op for telemetry-class DBs and an error for
   `core`/`backlog`. `Spec{Name, Path, Class, WriterConns=1, ReaderConns, Profile, SlowQuery, Tables}`.
2. **Location and naming.** `<config-dir>/db/<usecase>.db`. Two legacy exceptions keep their path to avoid renaming a live file:
   `core` = `sessions.db`, `escape` = `analytics.db`. A registry table maps logical name to path; docs call these out.
3. **Profiles.**

   | Class | synchronous | busy_timeout | wal_autocheckpoint | journal_size_limit | readers | Use |
   |---|---|---|---|---|---|---|
   | `state` | NORMAL | 5 s | 1000 | 64 MB | 4 (core) / 2 | core, backlog, workflow, wake, credentials |
   | `telemetry` | NORMAL | 5 s | 4000 | 64 MB | 2 | classification, contexthistory, escape, analytics_events, errors |
   | `cache` | OFF | 1 s | 4000 | 64 MB | 1 | none yet |

   All numbers are target values. PR 1 keeps today's values for the existing `analytics.db` (no reader pool, `wal_autocheckpoint` 1000) and for `sessions.db` (`synchronous` stays at the driver default until #1289 merges) until a benchmark justifies the change; the table above is where each class is headed, not what PR 1 ships. Every ent-backed profile
   includes `WithEntTimeCompat` so reads on any pool parse the `T`/offset and Go `time.String()` text forms already present.
4. **WAL enforced.** Registry paths must be plain file paths without a query string, because `sqlitedsn.WithWAL()` is a no-op on a path that already has one (`internal/sqlitedsn/sqlitedsn.go:42-48`); the read-back below fails the open rather than trusting the DSN. The registry sets `journal_mode=WAL` on open for every file it opens, reads it back and fails the open if it is
   not `wal` (except `:memory:`/URI test DBs). It exports `db_journal_mode{db}` (1 = WAL) and `db_synchronous{db}`, also shown in
   `/actuator/db`.
5. **Typed handles.** (`GetEntClient()` has 31 non-generated, non-test call sites returning the raw client, e.g. `server/dependencies.go:407,1441`; PR 1 keeps `Core.Ent()` behind it unchanged and `nowrongdbentity` turns on per entity only in the PR that moves that entity, so PR 1 is not a 34-site rewrite.) One handle type per DB (`dbpool.Core`, `dbpool.Classification`, ...). Small DBs expose a facade holding an
   unexported `*ent.Client` and only their entity clients; `Core` exposes `Ent()`. Repositories take the handle, not `*ent.Client`.
   This is a ratchet, not a proof: every handle wraps the same generated `*ent.Client`, so aliasing or passing a raw client defeats
   the types. The analyzer `tools/lint/nowrongdbentity` closes the gap by rejecting a selector for any entity outside the receiving
   DB's table set on a `*ent.Client` outside `dbpool` (both directions, including moved entities reached through `Core`); a unit test
   asserts the analyzer's table lists equal the registry's.
6. **No raw opens.** New analyzer `tools/lint/norawdb` (modelled on `norawexec`) fails any `sql.Open("sqlite", ...)` or ent/entsql
   open outside `session/dbpool`, `internal/sqlitedsn`, tests and `scripts/`. Foreign tools' files (opencode, agy, capacity monitor,
   `database_service` backup/merge) use `dbpool.OpenForeignReadOnly`, which never changes the file's journal mode and sets `query_only`.
7. **Schema per DB.** `migrate.Create(ctx, s, tables, opts...)` (`session/ent/migrate/migrate.go:49`, present after `make ent-gen`)
   accepts a table subset; each DB creates only its own. A test asserts no table has an FK to a table in another DB. Moved tables
   stay in `CoreTables` for one release after the flag default flips, so a fresh install, an old binary or a flag-off start never hits
   "no such table"; a later release removes them.
8. **Pools and fds.** Writer fixed at 1; readers per profile; `SetMaxIdleConns` equals max conns; `SetConnMaxLifetime(1h)`.
   `telemetry`/`cache` DBs idle for 10 minutes (configurable) close their pools and reopen lazily; `state` DBs stay open. Per open DB
   the fd cost is about 2 per connection (db + wal) plus a shared shm map: nine DBs at 1+2 connections is on the order of 60 fds
   (INFERRED; PR 1 measures with `lsof`).
9. **Observability.** Existing pool names (`ent`, `ent-read`, `analytics`) and metrics stay; the registry adds `db.logical` and
   `db.class` attributes and per-DB slow-query thresholds. `/actuator/db` lists one row per DB (open/closed, file and WAL size, pool
   config, journal mode).
10. **Maintenance and retention.** (`classification_analytics` retention is 180 days, owner-decided; see decision 1.) The retention cutoff applies when the table's `retention_days` is non-zero (`classification_retention_days`, default 180, 0 disables); with 0 the copy has no time filter. Each DB registers with `session/dbmaint` (#1271). Retention is a table-keyed policy per DB:
    chunks of 2,000 walking `id` ascending until the first row newer than the cutoff, comparing `datetime(created_at)` (SQLite
    normalizes offsets) because stored `created_at` mixes `-07:00` and `+00:00` text (VERIFIED in the copy), so a plain text compare is
    wrong near the boundary. `classification_analytics` keeps 180 days and purges older rows; it is still config-gated through `classification_retention_days` (0 disables), implemented in the migration PR (PR 2), not PR 1.
11. **Tunables.** (Runtime resize and idle close need the raw `*sql.DB`s, which `dbsplit` and `dbobs` do not expose, so `dbpool` keeps them. Idle close drops the driver; `dbpool` re-registers the reopened one with `dbobs` and a per-DB `dbmaint` maintainer whose goroutine it starts and stops with the open/close lifecycle, so neither holds a stale driver. `journal_mode` and `synchronous` gauges read the writer connection only; reader connections are opened with the same DSN.) Defaults in profiles; override via `config.json` (`db.<name>.reader_conns`, `busy_timeout`, `retention_days`) and
    env `STAPLER_SQUAD_DB_<NAME>_READ_CONNS` (`STAPLER_SQUAD_DB_READ_CONNS` stays the `core` alias). Runtime:
    `POST /actuator/db/pool {name, reader_conns}` on the local listener (pattern: `/actuator/terminal-mux`) using
    `SetMaxOpenConns`/`SetMaxIdleConns` on the reader pool. The writer stays 1 because dedup/CAS code relies on it.
12. **Startup.** `core` opens first and is fatal on failure, running today's startup migrations unchanged. Every other DB opens
    lazily. Data migrations run in a background goroutine after the server is serving, throttled; nothing is added to the startup
    migration list that already runs a git subprocess per backlog item and took over 6 minutes on a loaded copy (INFERRED from the
    brief, not re-measured). Lazy open runs the small per-DB `Schema.Create` under `EntSchemaCreateMu`; subset schemas keep that hold
    short, and writes arriving before it finishes block on the `Once` rather than fail.
13. **Shutdown.** Stop producers, drain `AnalyticsStore` and the escape batch writer, stop migrators and retention, then per open DB in
    reverse open order: `PRAGMA optimize`, best-effort `wal_checkpoint(TRUNCATE)` bounded by the busy timeout, close readers, close
    writer. A checkpoint that cannot finish is logged; WAL replays on next open.

## Index audit

Method: `EXPLAIN QUERY PLAN` of hot ent queries on `/tmp/dbp_sessions.db`. Each use-case DB gets a `hotqueries_test.go` that lists its
registered hot queries and asserts the plan contains `SEARCH` and no `SCAN` on a fixture DB (an index-ordered `SCAN ... USING INDEX`
bounded by `LIMIT` is allowed and marked explicitly).

| Query | Plan on the live copy | Verdict |
|---|---|---|
| `ListAnalyticsSince` | SEARCH `classificationanalytics_created_at` | ok |
| `ByProgramSince`, `RecentCommandsByProgram`, `GetSubcommandTrend` | SEARCH `..._command_program_created_at` | ok |
| `GetSubcommandBreakdown` | SEARCH same index + TEMP B-TREE FOR GROUP BY | accepted: GROUP BY over an indexed range, insights page, not hot |
| `ListAnalytics(limit)` | SCAN USING INDEX `..._created_at` | ok: ordered walk bounded by LIMIT |
| `context_samples` by session / since | SEARCH both | ok |
| `backlog_status_events` by item | SEARCH `backlogstatusevent_item_id_created_at` | ok |
| `backlog_status_events` newest-N across all items | SCAN + TEMP B-TREE FOR ORDER BY (250k rows) | **gap**: no `created_at` index; audit callers in the Backlog wave, add an index only if a hot path uses it |
| `item_sessions` by backlog item | SCAN `item_sessions` (no index) | **gap**, fixed by #1284's FK index |
| `diff_stats` by session | SEARCH unique key | ok |
| `backlog_stuck_states` by item | SEARCH | ok |

Unused indexes: #1284 drops four `classification_analytics` indexes (`session_id`, `decision`, `risk_level`, `rule_id`). The empty
`classification_analytics` already in `analytics.db` was created with all seven, so the lean set in the new DB **depends on #1284
landing first**; otherwise the new DB gets the full set. I audited only the queries above, not all 47 tables; the per-DB hot-query
tests make the rest checkable as each DB is carved out. #1272 was not examined.

## Data migration (restartable, online, never a long state hold)

The Router covers both directions of data movement: `core`->new DB for classification and context history, and `analytics.db` (`escape`)->new DB for `analytics_events`, whose writer is the `server/analytics` provider and whose readers include `server/handlers/analytics_handler.go:287`; moving it re-points those and the state-client reader `ListAnalyticsBySessionID`.

Applies per moved table: first `classification_analytics`, then `context_samples`+`compaction_events`, then `analytics_events` out
of `analytics.db`, then later waves (Backlog, Workflow, Wake, Credentials, Errors). Flag `db_split_<usecase>`, default off.

**Ledger** `dbpool_migrations(name, phase, cursor, source_rows, copied_rows, updated_at)` lives in **`core`**, which is always open
and survives deleting any telemetry file.

Copy first, then one atomic cutover, so there are **no union reads and no double counting**:

0. **Per-table spec.** Each moved table declares its natural key and cursor; the protocol is not generic. `classification_analytics`: `id INTEGER PRIMARY KEY AUTOINCREMENT` (cursor `id`, the rowid alias, stable across VACUUM), natural key `analytics_id` (unique; 427,630 distinct of 427,630), time column `created_at`, copy every column except `id`. `context_samples`/`compaction_events`: integer `id`, natural keys `(session_uuid, turn_index)` / `(session_uuid, ordinal)`, time columns `sampled_at` / `occurred_at` (neither has `created_at`), copied without `id`. `analytics_events`: `id TEXT PRIMARY KEY` (not a rowid alias, so `rowid` is not stable across VACUUM and is never a cursor); time column `created_at`, natural key `id`, copied as is. Its ids are random UUIDs, so a keyset cursor is **not monotonic**: rows inserted during the copy land anywhere in key order and some fall below the cursor. This table therefore gets a **full anti-join delta pass at cutover** (`NOT EXISTS` on `id`, cheap at ~100k rows) instead of a cursor-based delta; the cursor is used only to bound each bulk batch. A per-table test asserts the spec's columns exist and the key is unique.
1. `copy`. A pinned `sql.Conn` from the **destination's writer pool** (the pattern at `database_service.go:259-276`; ATTACH is per
   physical connection and `SetConnMaxLifetime` can recycle it) runs per batch of 2,000: `ATTACH 'file:<src>?mode=ro' AS src;
   INSERT OR IGNORE INTO dst(<every column except id>) SELECT <same> FROM src.t WHERE <cursor> > ? [AND datetime(<time column>) >= <cutoff> when retention is non-zero]
   ORDER BY <cursor> LIMIT 2000; DETACH`, then advance `cursor` in `core` to the maximum key **scanned** in the batch, not the maximum copied, so a batch whose rows are all filtered out still makes progress. Idempotency and verification key on the natural unique key
   (`analytics_id`), never `id`: `id` is `AUTOINCREMENT` in both files, so copying it collides with destination ids (review finding).
   `dbpool` keeps each writer's underlying `*sql.DB` and exposes it only to the migrator (the `dbobs`/`dbsplit` wrappers do not expose `Conn`); a batch holds the destination writer for its duration, harmless because the destination is not routed until step 2. The source read is a short WAL read transaction and holds no writer; the destination writer is the new, nearly idle DB. Throttle:
   sleep between batches; back off while `dbobs` shows `core` writer wait p95 above a threshold. The source `-shm` exists because the
   process has the source open.
2. `cutover`. The copier keeps tailing the source cursor until lag is under one batch. Then a `Router` flips: persist `phase=routed` in the ledger **first**, then swap an `atomic` target. At startup routing is derived from the ledger, never from memory, so a crash between the two steps is safe. Only the moved tables' repository methods (`ent_repository.go:1901-2140` for classification, about ten) read the router per call instead of the cached `r.client`; an in-flight batch finishes on the old target and is picked up by a final delta pass over source keys above the flip-time cursor (small, not a full anti-join), which is idempotent. The full anti-join is verification (step 3), not on the cutover path. Reads right after the flip can miss only rows in that delta; telemetry class tolerates that.
3. `verify`. Per source chunk, a `NOT EXISTS` anti-join on the natural key returns 0; when retention is non-zero, rows older than the cutoff are excluded from the copy and this check, and `purge` deletes them from the source without copying (counted as `expired`), so the source can still empty; counts compare on the filtered set. `PRAGMA
   quick_check` on the destination runs in the background and is exposed as a gauge (inline `integrity_check` on 490 MB is too slow).
4. `purge` (after a soak, default 7 days, only while the flag is on). **Copy-verified chunk delete:** for each chunk of 2,000 source
   cursor keys, confirm every `analytics_id` exists in the destination, then delete exactly those rows; a row never confirmed is never
   deleted. This replaces any "delete everything below `cursor`" step, which a flag flip or an old-binary run could turn into data
   loss (review finding: rows written to the source after a rollback sit above the cursor). Then `DROP TABLE` after an emptiness check;
   `dbmaint` returns pages (`sessions.db` is `auto_vacuum=2`, VERIFIED).
5. The migration re-runs on every start until the source table is absent or empty; `DROP TABLE` runs only once the table has left `CoreTables` (otherwise the next `Schema.Create` would recreate it), so during the one-release overlap the source stays present and empty (one `SELECT 1 LIMIT 1`); a ledger "complete"
   does not stop that. An old binary recreates the empty source table and may write rows there; the next upgrade copies them.

Rollback: flag off reverts routing; rows written to the destination after cutover exist only there, so `dbpool-migrate --reverse`
runs the same chunk loop the other way. After `purge`, rollback is the same reverse copy.

Interaction with open PRs: #1284 (index audit); #1289's `synchronous=NORMAL` becomes the `state` profile default; #1296's `db_caller`
label is reused; #1307 is the writer this split protects and is not modified; #1271's `dbmaint` is the maintenance engine.
`.claude/skills/stapler-squad-rules/SKILL.md:67-101` runs raw `FROM classification_analytics` against `sessions.db` and must be
updated in the same PR as the move, as must `database_service` backup/merge/inspect so they cover each moved DB.

## Alternatives

| Option | Verdict |
|---|---|
| 2-3 big DBs (`state`/`telemetry`/`cache`), revision 1 of this ADR | Superseded by the owner's refinement; keeps one noisy writer per class |
| One DB per table | Rejected: breaks the FK clusters and transactions above; tiny tables gain nothing |
| Separate ent clients over the same file | No WAL or writer isolation |
| Telemetry in Postgres/DuckDB | Out of scope; new runtime dependency |
| Dual-write window or union reads | Doubles source-writer cost; union counts double-count aggregates (`GroupBy`, `ent_repository.go:2077`); replaced by copy-then-atomic-cutover |
| Rename `sessions.db`/`analytics.db` into `db/` | Renames live WAL files for no benefit; legacy paths kept as registry entries |

## Risks

- Migration correctness on the owner's real DB is the main risk; mitigated by per-chunk verified deletes, natural-key idempotency, a
  mandatory soak, and running the protocol first on `/tmp` copies including an interrupted-and-resumed test.
- No cross-DB atomicity: a crash between two DBs' writes leaves them briefly inconsistent; accepted for the string-ref tables (none has
  an FK today).
- More files, pools and fds; bounded by lazy open and idle close, measured in PR 1.
- Removing the dead Session<->BacklogItem edge is an ent schema change; the Backlog wave is last and separately reviewed.
- Typed handles are a ratchet backed by a lint, not a proof.

## Open decisions for the owner (proposed defaults, pending owner confirmation)

1. `classification_analytics` retention: **decided by the owner: 180 days**; rows older than 180 days are purged. It stays config-gated (`classification_retention_days`, 0 disables). VERIFIED on a `.backup` copy of the live `sessions.db`: `SELECT min(created_at) FROM classification_analytics` is `2026-07-20 09:57:44-07:00` (about 103 days old on 2026-10-30), so a 180-day policy deletes nothing until 2027-01-16.
2. Purge soak: 7 days before the source copy is deleted.
3. Removing the dead `BacklogItem.sessions` edge (0 rows in `backlog_item_sessions`, no code uses it) is its own small ent-schema PR, ahead of the Backlog wave.
4. `sessions.db` and `analytics.db` stay as legacy-path exceptions in the `db/` naming scheme.

## Rollout

1. PR 1: `session/dbpool` registry (lazy open and idle close ship as a tested `Lazy` wrapper; `core` and `escape` stay eagerly opened in PR 1 and the first moved DB is the first lazy user; `/actuator/db` gains a `databases` array from a package-level `dbpool.Snapshot()`, the POST resize endpoint is PR 3; `norawdb` and `nowrongdbentity` are registered in `tools/lint/cmd/linter/main.go` and the `Makefile` `lint-custom` list, the latter needing generated ent types so it runs after `make ent-gen` like `make lint`), handles, profiles, WAL enforcement + `db_journal_mode`, `norawdb` lint, subset schemas, per-DB
   hot-query plan tests, lazy open/idle close, pragma and fd benchmarks. No table moves; the two existing files are re-registered.
2. PR 2: online migration + routing for `classification`, `contexthistory`, `analytics_events`; fixture, interrupted-resume and
   contention tests; before/after numbers on a `/tmp` copy.
3. PR 3: runtime pool actuator, `/actuator/db` per-DB rows, dashboard panels (additive, stable uids, Tempo links), docs, CLAUDE.md row.
4. Later waves: Backlog (after the edge removal), Workflow, Wake, Credentials, Errors, each with its own hot-query plan test.
