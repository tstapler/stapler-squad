# Build vs. Buy Research: Hook Classification Fast Path

## Recommendation

Build a thin project-specific orchestration layer using existing Go standard-library and repository dependencies. Reuse/adapt existing Stapler Squad socket, isolation, classifier, and analytics patterns. Do not buy a service and do not add an actor/RPC framework.

## 1. Existing OSS library or framework

### Go `net/http` over `net.UnixListener`

**Pros**
- Mature standard library, no dependency or license risk.
- Existing handlers and JSON DTOs can be tested directly.
- Supports deadlines, body limits, status codes, concurrency, and graceful shutdown.

**Cons**
- Must implement endpoint identity/version DTOs and safe socket lifecycle.
- HTTP adds small framing overhead.

**Verdict: Recommended.** Overhead is negligible relative to a 20 ms p95 target and short-lived process startup.

### ConnectRPC/gRPC

ConnectRPC already exists at v1.20.

**Pros:** generated contracts and interceptors; strong evolution model.

**Cons:** protobuf/codegen surface, transport adaptation for Unix sockets, and no need for streaming. The hook input/output is already JSON.

**Verdict: Viable but not recommended.** Use only if the protocol expands substantially or remote transport later demands a shared protobuf contract.

### Actor libraries (Proto.Actor, goakt, etc.)

**Pros:** supervision/mailbox abstractions.

**Cons:** new runtime semantics and dependency for a single-owner batching loop that is straightforward with a channel and goroutine. Harder shutdown and observability integration.

**Verdict: Not recommended.** Build the small typed actor with stdlib.

### Queue libraries / embedded brokers

**Pros:** durable retries and backpressure.

**Cons:** bounded loss is explicitly accepted; brokers add processes, storage, and operations far beyond current volume.

**Verdict: Not recommended.** A bounded channel is sufficient. Add a local spool only if later requirements change to durable-at-process-boundary.

### SQLite libraries

Keep `modernc.org/sqlite` v1.56 and Ent. The driver supports WAL and online backup. Switching to `mattn/go-sqlite3` introduces CGO/cross-platform build impact; a separate embedded analytics database engine is unjustified.

**Verdict: Recommended to reuse current driver.** Add a raw batch transaction method behind the repository/sink boundary if Ent cannot efficiently batch current rows.

## 2. SaaS / managed API

A hosted queue, policy engine, or analytics service would add network latency, outage coupling, cost, vendor lock-in, and transmit confidential local tool input. It violates local data-residency requirements and cannot provide offline/manual-instance behavior.

**Verdict: Not recommended.** Existing telemetry export can observe aggregated metrics, but it must not sit in the decision path.

## 3. Custom implementation versus battle-tested algorithms

Use battle-tested primitives for hard parts:

- stdlib HTTP/Unix sockets;
- `x/sync/singleflight` for equal-key context miss coalescing;
- SQLite unique constraints/upsert for idempotency;
- `database/sql` transaction semantics;
- cryptographic SHA-256 for endpoint fingerprints/checksums.

Custom code is justified for:

- Stapler-specific instance resolution and policy;
- protocol DTO mapping to Claude PreToolUse;
- rule snapshot serialization;
- analytics batch loop and bounded-loss policy;
- installer normalization.

Avoid bespoke lock-free queues, custom binary framing, homegrown hashing, copying live WAL files, and raw-command dedupe. Those are correctness-sensitive areas where standard primitives exist.

## 4. Fork or adapt

### Adapt `session/tymux/daemon_config.go`

Reuse its short hashed socket-directory and permission approach, ideally by extracting a shared internal helper rather than copying logic. Ensure helper semantics account for default/shared instances too.

**Verdict: Recommended.** Prevents repeating already-discovered SUN_LEN and ownership bugs.

### Adapt `session/sshremote/approval_relay.go`

Reuse lessons and potentially protocol test helpers: bounded request/response, identity token/fingerprint, stale-replay avoidance, and connection deadlines. Do not couple local fast path to SSH relay types.

**Verdict: Recommended via patterns/seams, not a direct fork.** Remote protocol remains future work.

### Evolve `server/services/analytics_store.go`

It already owns a bounded channel, drop counter, lifecycle, and conversion into `AnalyticsData`. Replace per-event flush with transaction batching and richer metrics rather than introducing another store.

**Verdict: Strongly recommended.** This is the natural write-behind seam.

### Reuse `ApprovalHandler`

Do not call its HTTP handler directly because it includes secret/domain checks and manual approval queue behavior. Extract/share a classify-only application service or pure policy pipeline.

**Verdict: Adapt shared logic, do not fork the whole handler.**

## Final decision

Build with existing dependencies and extracted shared helpers. Record an ADR choosing stdlib HTTP/JSON over Unix sockets and an ADR for bounded-loss analytics batching. Keep dedicated analytics DB/sharding as a benchmark-gated option, not a committed dependency.
