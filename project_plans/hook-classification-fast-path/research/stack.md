# Stack Research: Hook Classification Fast Path

## Existing stack to reuse

- **Go 1.26.6** (`go.mod:3`) provides `net`, `net/http`, `database/sql`, `context`, `sync/atomic`, and benchmark/race tooling. A local HTTP/1.1 protocol over `net.Listen("unix", path)` is sufficient; no new RPC runtime is required.
- **SQLite via `modernc.org/sqlite` v1.56.0** (`go.mod:67`) is already configured through `internal/sqlitedsn` with WAL, a 5-second busy timeout, foreign keys, and Ent time compatibility. WAL permits concurrent readers but SQLite still has one writer; batching reduces transaction/fsync overhead rather than creating parallel writers.
- **Ent** remains the primary repository API, but hook startup currently reaches `session.NewEntRepository`, which runs `client.Schema.Create` and all startup migrations (`session/ent_repository.go`). The fast path must bypass repository construction entirely by classifying in the resident server.
- **`golang.org/x/sync` v0.22.0** (`go.mod:222`) is already available. `singleflight.Group` is appropriate for coalescing identical cold context lookups (for example, Git metadata keyed by canonical cwd), but not distinct permission decisions.
- **OpenTelemetry 1.46 / otelhttp 0.70** (`go.mod:51-57`) is available for latency and queue metrics. Existing hookmetrics/VictoriaMetrics reporting can remain for external end-to-end measurements.
- Existing socket precedents should be reused: instance-derived short socket paths in `session/tymux/daemon_config.go`, request/response relay and deadlines in `session/sshremote/approval_relay.go`, and standard server lifecycle ownership in `server/server.go`.

## Transport recommendation

Use the standard library: HTTP/1.1 over a Unix domain socket with compact JSON.

Reasons:

1. HTTP gives bounded bodies, status codes, deadlines, version headers, and test support through `httptest`-style handlers.
2. JSON matches Claude hook input/output and avoids an encode/decode translation dependency.
3. A short-lived `ssq-hooks` process cannot reuse a connection across tool calls, so protocol complexity aimed at connection pooling has little value. Unix connect + one small request remains far below the 20 ms p95 target.
4. ConnectRPC v1.20 is present, but adding protobuf generation and Connect framing for one local request adds compatibility and code-generation surface without a measurable requirement.

Protocol v1 should include: protocol version, instance/config fingerprint, request ID, rule-cache version, raw Claude payload, and only environment values required for command expansion. Response includes the exact `PreToolUse` decision, server/rule version, and source (`primary` or fallback metadata). Cap bodies and set read/write deadlines.

## Concurrency and actor primitives

Use built-in Go concurrency:

- HTTP server handles independent requests concurrently.
- Classifier rules are immutable snapshots. Existing `RuleBasedClassifier.ReplaceRules` swaps a copied/sorted rule slice under a lock (`pkg/classifier/classifier.go:435-442`); research from `dynamic-rule-reload` documents source-composition race risks, so snapshot publication should occur behind one composition owner.
- A bounded channel feeds one analytics writer actor.
- The writer collects by `maxBatch` or `maxDelay`, then persists one transaction. Initial values should be benchmarked; 64–256 records and 1–5 ms are sensible experiments, not hardcoded conclusions.
- `singleflight` coalesces Git context cache misses by cwd. Stable `tool_use_id` deduplicates duplicate delivery; current public Claude docs show it as a top-level PreToolUse field, but `PermissionRequestPayload` does not retain it and must be extended.

## SQLite details

Keep WAL. Consider `synchronous=NORMAL` only for a dedicated bounded-loss analytics database; do not silently weaken durability of the primary session database. Keep default WAL checkpointing until metrics demonstrate tail spikes. If immutable shards or snapshots are justified, never copy `sessions.db` alone while WAL is live. `modernc.org/sqlite` exposes SQLite Online Backup through `sql.Conn.Raw`/`NewBackup`; the installed v1.56 includes page-count support. Closed immutable analytics shards can instead be atomically renamed and attached/imported.

The existing `AnalyticsStore` already has a non-blocking 1,000-entry channel and dropped counter (`server/services/analytics_store.go:124-191`), but `flush` writes each event separately every five seconds (`:579+`). Evolve this component into an explicit actor with batch transaction APIs rather than adding a parallel queue.

## Dependencies

**No new production dependency is recommended.** Reuse stdlib, x/sync, modernc SQLite, Ent, and OTel. New dependencies would only be justified by benchmark evidence showing standard HTTP/JSON cannot meet the SLO, which is unlikely at the observed volume.

## Testing stack

- Unit tests for endpoint resolution, protocol validation, actor state transitions, and installer normalization.
- Real Unix-socket integration tests with isolated `STAPLER_SQUAD_TEST_DIR` values.
- `go test -race` for request concurrency, snapshot replacement, and actor shutdown.
- Go benchmarks reporting percentile distributions for warm/cold request paths and batched writes.
- Fault-injection tests for stale sockets, server death, queue saturation, old/new protocol versions, DB lock, and abrupt shutdown.
