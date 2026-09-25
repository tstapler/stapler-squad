# Architecture Research: Hook Classification Fast Path

## Disposition: isolate via seam

`session/ent_repository.go` is ranked #6 in `docs/reference/hotspot-ranking.md` (complexity×churn score about 23.2k). The current hook reaches that hotspot through `loadStorage` and pays schema plus startup-migration cost. **Isolate via seam**: the new hook path must not extend the repository constructor or add hook-specific flags to this hotspot. Put local transport, classification orchestration, snapshot cache, and analytics actor behind narrow packages/interfaces; the primary server remains the only owner of full repository initialization.

Prior `project_plans/dynamic-rule-reload/research/architecture.md` finds that rules from seed/user/Claude settings share one classifier and source-composition read-modify-write can race. The fast path should consume one published immutable classifier snapshot and centralize composition rather than creating another loader.

## Proposed bounded components

### `internal/hookipc`

Owns protocol DTOs, version negotiation, endpoint identity, socket path derivation, client/server framing, limits, and deadlines. It knows neither Ent nor approval queues. Use HTTP/JSON over Unix sockets. Derive a short socket directory by hashing the resolved config directory, following `session/tymux/daemon_config.go:105+`; explicit `SSQ_HOOK_SOCKET` wins. Parent 0700, socket 0600.

### `server/services/HookClassifier`

A classify-only application service accepting payload plus invocation context. It reads an immutable rule snapshot, gets cached repository context, classifies, returns a `PreToolUse` decision, and non-blockingly emits an analytics event. It must not call the manual approval creation path in `ApprovalHandler`.

Extract shared pure classification/result-formatting behavior from `ApprovalHandler` rather than invoking the HTTP handler synthetically. Keep secret/domain checks explicit: planning must decide whether they belong in both paths or only PermissionRequest, then pin equivalence in tests.

### `server/services/HookAnalyticsActor`

Evolve existing `AnalyticsStore`, not a second queue. One goroutine owns queue consumption and batched transactions. Producers are non-blocking. The actor exposes metrics and bounded shutdown. Storage gets a `RecordAnalyticsBatch(ctx, []AnalyticsData)` transaction method. A dedicated analytics DB/shards are a research-gated optimization behind the same sink interface.

### Fallback snapshot

Primary publishes an atomic, checksummed snapshot in its config namespace after successful rule composition. The CLI reads it only when the intended endpoint fails. Snapshot includes protocol/schema version, instance fingerprint, rule version, creation time, and compiled-rule source data (regex strings, not Go regexp internals). Invalid/mismatched snapshots are ignored; final behavior is defer.

## Request path

1. CLI decodes Claude input, retaining `tool_use_id`.
2. Resolve endpoint using explicit override then config namespace; isolated namespaces never fall through to shared.
3. Send bounded request with protocol version, instance fingerprint, request ID/tool-use ID, payload, and referenced environment values.
4. Server validates transport identity and version.
5. Concurrent handler checks a short idempotency cache keyed by stable tool-use ID.
6. Context cache returns warm Git context or singleflight-coalesces a miss.
7. Immutable classifier returns decision.
8. Server writes response before any SQLite wait and non-blockingly enqueues analytics.
9. Actor batches inserts. Queue overflow increments a drop metric.
10. On transport failure, CLI classifies from a verified snapshot; if impossible, emits no decision.

## Event–Command–Policy model

| Domain Event | Policy trigger | Command | Actor / System |
|---|---|---|---|
| Tool Use Requested | Whenever Claude requests a tool, route to its intended instance | Classify Tool Use | Hook CLI |
| Endpoint Resolved | Whenever an isolated namespace resolves, never try shared production | Send Classification Request | Hook IPC client |
| Request Validated | Whenever protocol and instance match, classify concurrently | Evaluate Rules | Primary classifier |
| Decision Produced | Whenever classification completes, reply without waiting for persistence | Return Hook Decision | IPC server |
| Decision Produced | Whenever a decision is returned, enqueue its audit event | Queue Analytics Event | Analytics producer |
| Batch Ready | Whenever size/deadline is reached, persist one transaction | Commit Analytics Batch | Analytics actor |
| Endpoint Unavailable | Whenever primary cannot answer in budget, consult bound snapshot | Classify From Cache | Hook CLI |
| Snapshot Unusable | Whenever no verified cached result can be made, make no policy decision | Defer Decision | Hook CLI |
| Rules Replaced | Whenever composed rules change successfully, publish a new bound snapshot | Publish Rule Snapshot | Rules owner |
| Duplicate Tool Use Received | Whenever a stable ID repeats inside TTL, reuse same response and analytics ID | Return Idempotent Decision | IPC server |

## Consistency model

- Classification is linearizable against the in-memory snapshot selected at request start; response carries its rule version.
- Rule replacement is atomic, and old in-flight requests may finish on the prior version.
- Analytics is eventually persisted with bounded loss. Decision delivery does not imply durable analytics.
- Stable event IDs plus a unique DB key prevent duplicate writes during retries.
- Context cache is TTL/stale-while-revalidate data and cannot override explicit payload context.
- No raw live SQLite copying. If later justified, use online backup or closed immutable shard handoff.

## Migration/direct-replacement failure modes

- **Mixed binary versions:** protocol negotiation must produce a fast incompatibility response that drives cached/defer fallback, not malformed allow/deny output.
- **Install interruption:** atomically normalize Claude settings only after the new binary and server protocol are available; retain backup for rollback.
- **Old server/new CLI:** cache/defer works; no direct DB initialization fallback.
- **New server/old duplicate hooks:** server idempotency by tool-use ID prevents duplicate analytics/side effects until installer normalization runs.
- **Rollback:** old CLI may not understand new cache files but can ignore them; schema migration for batching should be additive or absent.
- **Two instances:** socket/config fingerprints and filesystem permissions prevent accidental cross-routing; no scan-and-pick discovery.

## Research-gated database expansion

Benchmark first. Hook analytics batching is justified by current one-row flush behavior. A shared actor for all application writes is not automatically justified: it risks coupling unrelated transactions and creating a new global bottleneck. A dedicated analytics DB may reduce the 624 MB primary DB growth and migration cost, but requires query migration and lifecycle work. Gate either extension on measured DB wait/commit latency, WAL growth, and query impact, and record the choice in an ADR.
