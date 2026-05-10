# ADR-013: Separate analytics.db SQLite File

## Status
Accepted

## Context

The analytics system needs to persist `AnalyticsEvent` rows via the ent ORM, consistent with how `Session`, `ErrorEvent`, `ClassificationAnalytics`, and `Worktree` entities are stored. The question is whether to store analytics events in the existing `sessions.db` file or in a new dedicated `analytics.db` file.

The existing architecture (documented in `session/ent_repository.go`) uses a single SQLite file for all session-management entities. SQLite is configured in WAL mode (`_journal_mode=WAL`) with `db.SetMaxOpenConns(1)` — a single writer at a time, which is correct for SQLite's serialized write model.

The pitfalls research (section 2) quantifies the write volume analytics will introduce:

| Source | Estimated events/min |
|--------|---------------------|
| onClick handlers | 5–20 |
| Omnibar dispatch cases | 2–5 |
| Page view transitions | 1–3 |
| RPC calls (all categories) | 5–30 |
| Web Vitals (CWV) | 3 per page load |

**Realistic peak: 50–100 analytics events per active minute per browser tab.** With client-side batching (25 events per flush, 2-second interval), this translates to 2–5 HTTP requests/min, each triggering a batch SQLite insert of up to 25 rows.

The critical risk is **write burst contention**: if analytics writes share the single `*ent.Client` connection with session management operations, a burst of analytics inserts (e.g., page load firing Web Vitals + RPC latency events simultaneously) will hold the SQLite write lock and delay session list queries, session creation, and worktree operations that the user is actively waiting on.

The `ClassificationAnalytics` entity stored in `sessions.db` already writes analytics-style data, but it fires at most once per session and is not on a hot path. The new `AnalyticsEvent` entity is a fundamentally higher-frequency workload.

## Decision

Store analytics events in a dedicated `analytics.db` SQLite file, separate from `sessions.db`.

A second `EntRepository` (or a thin `AnalyticsRepository`) is instantiated at startup, pointing to `~/.stapler-squad/<instance>/analytics.db`. It has its own `*sql.DB` connection pool configured with `MaxOpenConns(1)` and WAL mode. The `SQLiteAnalyticsProvider` holds a reference to this repository exclusively.

The file path follows the existing state isolation pattern from `config/config.go`:

```go
analyticsDBPath := filepath.Join(cfg.StateDir(), "analytics.db")
analyticsClient, err := openEntClient(analyticsDBPath)
```

The `AnalyticsEvent` ent schema (`session/ent/schema/analytics_event.go`) is added to the existing ent schema directory, but the `schema.Create()` call for it is made against the analytics client, not the sessions client. This may require a second call to `schema.Create()` at startup or a split schema registration.

Retention enforcement (max 90 days / 100k events) runs as a background goroutine on a configurable ticker, deleting rows from `analytics.db` only — it never touches `sessions.db`.

## Alternatives Considered

**Same `sessions.db` file (shared ent client)**

Adding `AnalyticsEvent` to the existing `sessions.db` is the path of least resistance:
- No second `EntRepository` to initialize and close
- All entities share one `schema.Create()` call
- `ClassificationAnalytics` already lives there — `AnalyticsEvent` is the same category of data

Rejected because: the write contention risk is real and measurable. SQLite's serialized writer model means analytics burst writes (on page load: LCP + FID + CLS + 10 RPC latency events in <500ms) will queue behind the lock. Session management RPCs that the user is actively waiting on will stall. In a single-user local tool, a 50–200ms stall on `ListSessions` is perceptible. Separating the files eliminates the cross-blocking entirely at the cost of a second connection.

**PostgreSQL**

A relational database with true concurrent write support would eliminate the contention problem entirely. Rejected because:
- Violates the "self-contained, no external services" non-functional requirement
- Adds significant operational complexity (process management, pg_hba, port conflicts)
- The existing codebase has no PostgreSQL driver or connection management code — the migration cost is disproportionate to the benefit for a local analytics store

**JSONL flat file append log**

Append-only writes to a JSONL file (`analytics.jsonl`) would be extremely fast (file system append) and eliminate SQLite contention entirely. The summary endpoint would read and aggregate the file in-process.

Rejected because:
- Aggregation queries (p50/p95/p99 RPC latency, top events by count over a time window) are expensive to compute from a flat file as the row count grows toward 100k
- No natural retention enforcement (cannot delete individual rows from the middle of a JSONL file without rewriting it)
- The existing ent ORM pattern already handles schema evolution, indexing, and type-safe queries — reinventing that for a flat file adds maintenance burden
- File locking for concurrent appends (multiple browser tabs) is platform-specific and error-prone

**Single `analytics.db` with raw `database/sql` (no ent)**

Using raw SQL (`database/sql` + `modernc.org/sqlite`) for `analytics.db` would avoid pulling analytics schema changes into the ent code generation pipeline. Rejected because:
- Inconsistent with the rest of the codebase (all other persistent entities use ent)
- Loses ent's type-safe query builder for the summary endpoint aggregations
- Raw SQL requires manual migration tracking that the project does not currently have
- At the projected event volume (<2 events/second), ent's overhead is negligible

## Consequences

**Positive:**
- Session management operations (`ListSessions`, `CreateSession`, `GetSession`) are fully isolated from analytics write bursts — no cross-blocking on the SQLite write lock
- Each database file has its own `MaxOpenConns(1)` connection, WAL journal, and page cache — optimal configuration for each workload independently
- `analytics.db` can be deleted or reset independently of `sessions.db` without affecting session state — useful for development, testing, and "clear analytics data" user operations
- The retention goroutine runs `DELETE` on `analytics.db` without interfering with session management queries
- Backup and restore operations can target each file independently

**Negative / Trade-offs:**
- Two `*ent.Client` instances to initialize, health-check, and close at startup/shutdown — slightly more boilerplate in `server/dependencies.go`
- The `AnalyticsEvent` ent schema lives in `session/ent/schema/` (consistent with all other entities) but must be `schema.Create()`'d against a different client than the other entities — requires care in the startup sequence to avoid accidentally registering the analytics schema against `sessions.db`
- Two SQLite WAL files (`analytics.db-wal`, `analytics.db-shm`) appear in the state directory alongside the existing session WAL files — minor operational noise

**Pitfall mitigations:**
- The ent schema `AnalyticsEvent` uses `.Optional()` on all non-core fields and `field.JSON("labels", map[string]string{})` for extensible metadata — designed for maximum schema stability, since `Schema.Create()` auto-migration is additive-only and column renames/type changes require a full table rebuild (see pitfalls research, section 3)
- At startup, log which analytics DB path is in use and whether `schema.Create()` succeeded, so failures are immediately visible in `~/.stapler-squad/logs/stapler-squad.log`
