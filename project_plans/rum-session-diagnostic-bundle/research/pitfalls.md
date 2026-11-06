# RUM Session Diagnostic Bundle — Known Pitfalls & Risks

Research date: 2026-10-16

---

## 1. PII / Secret Exposure in Scrollback Content

**Risk: HIGH**

`ScrollbackEntry.Data` is raw terminal bytes (`[]byte`) captured verbatim from PTY output. There is **no sanitization layer** anywhere in `session/scrollback/` — `AppendOutput` in `manager.go` calls `buffer.Append(data)` directly on the raw bytes.

A user who types an API key, password, or OAuth token at a shell prompt will have that string persisted in `~/.stapler-squad/sessions/<sessionID>/scrollback.jsonl.zst`. Packaging that file into a diagnostic bundle and offering it for download or upload to a backlog item creates a direct secret-exfiltration vector — the user may not realize the bundle contains their credentials.

**Mitigations to design:**
- Offer a "tail only" mode (last N lines) instead of full history.
- Run a best-effort secrets scan (entropy heuristic or known patterns like `AKIA…` AWS keys) and redact or warn.
- Present an explicit "This bundle may contain terminal output including typed credentials" consent prompt before download/save.

---

## 2. Bundle Size Explosion

**Risk: HIGH**

The scrollback subsystem has two hard limits (from `session/scrollback/manager.go` `DefaultScrollbackConfig()`):
- `MaxLines = 10,000` (in-memory circular buffer)
- `MaxSizeBytes = 10 MB` on-disk (zstd-compressed)

At worst-case 10k lines × ~200 bytes/line = ~2 MB raw. However:
1. The on-disk file is zstd-compressed JSONL; the **uncompressed JSON** is larger. Each entry overhead is ~50 bytes of JSON plus raw data. Actual uncompressed size for 10k lines of 200 bytes = roughly 2.5–3 MB for scrollback alone.
2. **Context history** stores per-turn token counts. A 90-day retention with a busy session could produce tens of thousands of rows, adding several MB.
3. **Artifacts** — design documents, plan files — are the biggest unknown.
4. Peak **in-memory allocation** during assembly is 2–3× the uncompressed size of all components simultaneously.

**Mitigation:** Implement per-component size budgets (e.g., scrollback ≤ 20 MB, context history ≤ 10 MB, artifacts ≤ 15 MB) and truncate/skip rather than error out. Report skipped components in `manifest.json`.

---

## 3. Race Conditions During Bundle Assembly

**Risk: MEDIUM**

The scrollback `CircularBuffer` uses a `sync.RWMutex` with `+checklocks:mutex`. `FileScrollbackStorage` uses per-session `sync.Mutex` file locks via `getFileLock`. **These two locks are independent** — no global lock covers both the in-memory buffer and the on-disk file simultaneously.

During bundle assembly:
- Reading from `storage.Read()` acquires the file lock.
- Reading from `buffer.GetAll()` acquires the buffer's RLock.
- Between these two operations, `flushLoop()` (running every 5s) may write new buffer entries to storage, causing a **TOCTOU gap**.

For a diagnostic bundle the data is "best effort" so minor gaps are acceptable, but the design must document this and not assert completeness.

**Mitigation:** Take a point-in-time snapshot by: (a) calling `FlushSession` first to drain the in-memory buffer to disk, then (b) reading only from storage. Wrap context-history reads in a SQLite `READ` transaction for snapshot isolation.

---

## 4. Disk Space for Saved Bundles

**Risk: MEDIUM**

The feature spec calls for saving bundles to `~/.stapler-squad/diagnostics/`. There is currently **no such directory or any cleanup/retention policy** in the codebase. Nothing in `server/` or `session/` prunes this directory.

A user who repeatedly triggers bundle saves will accumulate zips indefinitely. At 10–50 MB per bundle, 100 saves = 1–5 GB of unbounded disk usage.

**Mitigation:** Implement a retention policy (e.g., keep last N bundles or delete bundles older than 30 days) run at startup or on a periodic timer — analogous to the `Prune` method in `session/contexthistory/store.go` and the `EnforceRetention` pattern in `server/analytics/retention.go`.

---

## 5. Analytics Event Query Performance

**Risk: LOW–MEDIUM**

`session/ent/schema/analytics_event.go` defines an index on `session_id` alone:
```go
index.Fields("session_id"),
```
A `WHERE session_id = ?` query uses an index and is O(matching rows), not O(total rows). For a session with 1,000 analytics events, the query time is well under the 5s budget.

However, **the `AnalyticsEvent` table has no per-session row cap**. A pathological long-lived session could accumulate thousands of analytics rows; reading them all into the bundle adds latency and memory.

**Mitigation:** Add a `LIMIT` clause (e.g., most recent 2,000 events) to the per-session analytics query used by bundle assembly.

---

## 6. ConnectRPC Message Size Limits

**Risk: HIGH**

A search across `server/` for `WithReadMaxBytes` finds **zero occurrences** in production code. `ConnectOptions()` in `server/server.go` only adds OTel and error-recorder interceptors — **no message size limit**.

Connect-RPC's default read-message limit is unlimited (or bounded only by available memory). If the bundle-download RPC serialises the entire zip into a single response message (rather than streaming), a 50 MB bundle will allocate 50+ MB in a single gRPC/HTTP2 frame, risking OOM on the server and hitting any proxy's buffer limits.

**Mitigation:** Use server-streaming RPC (`stream.Send()` in chunks) for the bundle download response, or add `connect.WithReadMaxBytes(64 * 1024 * 1024)` to the handler options. Do not serialize the entire zip into a single unary response.

---

## 7. OTel `session.uuid` Span Attribute — Fan-Out and Quota Risk

**Risk: MEDIUM**

`telemetry/telemetry.go` configures the tracer provider with `sdktrace.TraceIDRatioBased(cfg.SampleRate)` where `cfg.SampleRate` defaults to **1.0** (sample all traces).

Adding `session.uuid` as an attribute to every span creates a **high-cardinality attribute**. Backends like Datadog charge per unique tag value combination; with thousands of sessions each generating many spans, this could produce unexpected cost spikes.

More critically, if `session.uuid` is added at the root span level via `otelconnect.NewInterceptor` (already in `ConnectOptions`), every ConnectRPC call — including high-frequency streaming calls like `StreamTerminal` — will carry this attribute.

**Mitigation:**
- Only attach `session.uuid` to spans that are semantically about a session (bundle creation, backlog attachment) rather than all spans.
- Consider a `ParentBased` sampler.
- Document that `OTEL_ENABLED=true` with default `SampleRate=1.0` is not safe for high-traffic use cases.

---

## 8. Bundle File Path Locality — Backlog Item Cross-Machine Mismatch

**Risk: HIGH (workflow correctness)**

The server-side save path `~/.stapler-squad/diagnostics/<bundle>.zip` is a local filesystem path on the machine running the stapler-squad server. If this path is stored as a `string` field on the backlog item, the path is **meaningless** on any other machine.

Concrete failure modes:
1. User A generates a bundle on laptop A and creates a backlog item with `bundle_path = "/home/userA/.stapler-squad/diagnostics/session-abc.zip"`.
2. The same backlog item is opened by a CI agent or a teammate on a different host. The path does not exist; any automated processing fails silently.
3. If the bundle is synced to a remote backlog service, the local path provides no value.

**Mitigation:**
- Store only a **content hash** (SHA-256 of the zip) and a **human-readable filename** on the backlog item, not the absolute path.
- For cross-machine sharing, require an explicit upload step rather than path-based sharing.
- If the path must be recorded, namespace it with a `machine_id` or hostname field.

---

## Summary

| # | Pitfall | Severity | Evidence |
|---|---------|----------|----------|
| 1 | Scrollback contains raw terminal bytes — no sanitization | HIGH | `session/scrollback/manager.go:AppendOutput`, `buffer.go:Append` |
| 2 | Bundle size exceeds 50 MB with long-lived sessions + context history | HIGH | `manager.go:DefaultScrollbackConfig` (10k lines/10MB), `recorder.go:DefaultRetention=90d` |
| 3 | TOCTOU race between in-memory buffer and on-disk file during assembly | MEDIUM | `manager.go:GetScrollback` — two separate lock domains |
| 4 | No cleanup policy for `~/.stapler-squad/diagnostics/` | MEDIUM | Directory not referenced in any current code |
| 5 | Analytics query performance acceptable (indexed), but no per-session row cap | LOW–MEDIUM | `schema/analytics_event.go` has `session_id` index |
| 6 | No ConnectRPC message size limit — repo-wide acknowledged gap | HIGH | `server.go:ConnectOptions`, no `WithReadMaxBytes` |
| 7 | `session.uuid` on all spans risks high-cardinality quota and export backpressure | MEDIUM | `telemetry.go:SampleRate=1.0`, no attribute-level sampling |
| 8 | Server-side bundle path is machine-local; useless in cross-machine backlog items | HIGH | No path portability in current backlog item schema |
