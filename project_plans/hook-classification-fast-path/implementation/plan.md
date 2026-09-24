# Implementation Plan: hook-classification-fast-path

**Feature**: Replace per-tool database initialization with an instance-scoped resident classifier and batched write-behind analytics.
**Date**: 2026-09-23
**Status**: Ready for implementation
**ADRs**: [ADR-001](../decisions/ADR-001-instance-scoped-http-over-unix-socket.md), [ADR-002](../decisions/ADR-002-bounded-loss-analytics-actor.md), [ADR-003](../decisions/ADR-003-direct-replacement-and-rollback.md), [ADR-004](../decisions/ADR-004-research-gate-broader-sqlite-redesign.md)

---

## Domain Glossary

| Term | Definition | Notes |
|---|---|---|
| `HookEndpoint` | Proven local address of one Stapler Squad instance's classification server | Value object; socket path plus `InstanceFingerprint` |
| `InstanceFingerprint` | SHA-256-derived identity of the resolved config/state directory | Newtype, never a raw routing string |
| `ProtocolVersion` | Supported hook IPC wire-contract version | Integer newtype with explicit compatibility check |
| `HookRequestID` | Unique request identity derived from Claude `session_id` and `tool_use_id`, or random when unavailable | Newtype; exact dedupe only |
| `ToolUseID` | Claude-provided stable identity for one tool invocation | Added to `PermissionRequestPayload` |
| `ClassificationEnvelope` | Validated protocol request containing identity, payload, and selected invocation context | Parse-at-boundary DTO |
| `ClassificationReply` | Exact PreToolUse decision plus protocol, rule, and source metadata | Source is primary/cache/defer |
| `DecisionSource` | Sum type: `PrimaryDecision`, `CachedDecision`, `DeferredDecision` | Exhaustive handling |
| `RuleSnapshotVersion` | Monotonic/content-derived identity of one immutable composed ruleset | Included in replies/cache |
| `CachedRuleSnapshot` | Checksummed, instance-bound fallback representation of a last-known-good ruleset | Atomically published file |
| `InvocationContext` | Cwd, selected environment values, and repository context used for classification | Never contains all environment variables |
| `RepositoryContext` | Cached Git facts: root, is-repo, is-worktree, observed time | Value object keyed by canonical cwd |
| `AnalyticsEvent` | One classification audit record identified by `HookRequestID` | Existing analytics schema representation |
| `AnalyticsBatch` | Ordered group persisted in one transaction | Actor-owned value |
| `HookAnalyticsActor` | Single owner of analytics queue, batching, retry/drop policy, and shutdown | Service Layer/Active Object |
| `FallbackState` | Endpoint failure/circuit state deciding primary attempt versus cache/defer | Explicit state machine |
| `HookHealth` | Non-sensitive diagnostic view of endpoint, protocol, cache, actor, and fallback state | Used by doctor command |

---

## Creative Pass: Alternatives

1. **Resident HTTP/JSON over instance-scoped Unix socket** — strongest isolation/reuse with simple standard-library operations; requires lifecycle/version work.
2. **Lightweight direct SQLite open per hook** — smaller code change, but retains process/database coupling, contention, and duplicated rule loading.
3. **Dedicated always-on `ssq-hooks serve` daemon** — isolates hook work, but duplicates the main server's rules/storage lifecycle and complicates instance ownership.

The resident primary-instance approach is selected. The latter two remain explicit rejected alternatives in the decisions below.

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|---|---|---|---|---|
| `HookClassifier` | Service Layer | Fowler PoEAA | Invoke `ApprovalHandler` as Transaction Script | Classification must be reusable without manual-approval side effects |
| `HookEndpointResolver` | Strategy + value object | GoF/type-driven | Scan sockets and choose first | Routing policy varies by explicit/default/isolated namespace and must reject ambiguity |
| `HookIPCClient` | Adapter | GoF | Direct repository access | Isolates Claude CLI from server/storage implementation |
| `HookAnalyticsActor` | Active Object / Unit of Work per batch | GoF/PoEAA | Goroutine per write or one transaction per row | One SQLite writer, bounded queue, transactional batches |
| `AnalyticsSink` | Repository port | PoEAA/Hexagonal | Actor imports Ent builders directly | Enables tests and benchmark-gated dedicated store later |
| `CachedRuleSnapshot` | Immutable Snapshot | GoF/type-driven | Reopen DB when socket fails | Safe fallback without migrations or live DB coupling |
| `RepositoryContextCache` | Cache + Singleflight | GoF/x/sync | Two Git subprocesses per call | Coalesces equal misses while preserving parallel decisions |
| `DecisionSource` / `FallbackState` | Sum types | Type-driven | free-form strings/booleans | Prevents illegal source/fallback combinations |
| Protocol DTO parsing | Parse at boundary | Type-driven | pass raw maps through layers | Rejects incompatible identity/version before policy logic |
| Installer normalization | Transaction Script | Fowler PoEAA | append-if-absent marker check | One atomic settings transformation is simple but must converge duplicates |

---

## Tech Debt Disposition

| Area | Existing Issue | Disposition | Justification |
|---|---|---|---|
| `session/ent_repository.go` | Rank-6 complexity×churn hotspot; constructor combines connection, schema, and data migrations | **Isolate via seam** | Fast path never opens the repository; `AnalyticsSink` and resident service prevent hook-specific flags from deepening constructor debt |
| `server/services/approval_handler.go` | Classification, security checks, analytics, queueing, and wire response are intertwined | **Refactor-first** | Extract classify-only policy service before adding local IPC so behavior is shared without synthetic HTTP calls |
| `server/services/analytics_store.go` | Existing async channel still writes one transaction per event and has incomplete queue observability | **Refactor-first** | Evolve the existing owner into a batch actor instead of creating a competing queue |
| Rule source composition | Prior research identifies lost-update risk between independent reload paths | **Isolate via seam** | Snapshot publisher consumes the existing composed classifier only; source composition remains owned/serialized by `RulesService` |

---

## Migration Plan

No destructive schema migration is required for the mandatory fast path. Existing analytics IDs provide idempotency; `ToolUseID` is carried in protocol/request identity without requiring a new column. Batch persistence uses the existing table in one transaction.

- **Reversibility**: new socket/cache files are additive and ignored by old binaries; installer retains a settings backup.
- **Zero-downtime strategy**: server begins listening and reports protocol health before installer atomically replaces recognized hook commands. Mixed versions fall back to cache/defer.
- **Rollback procedure**: restore prior binary and backed-up Claude settings; stop/remove the new socket listener and cache files. No DB down-migration is needed.
- **Research-gated migrations**: a dedicated analytics DB or shard format requires a separate approved ADR/migration plan after benchmarks; it is not silently included in this implementation.

## Observability Plan

- **Metrics**:
  - `ssq_hook_request_duration_ms{path,result,protocol,instance_class}` histogram.
  - `ssq_hook_classify_duration_ms{cache_state}` histogram.
  - `ssq_hook_requests_total{path,result,error_code}` counter.
  - `ssq_hook_fallback_total{reason,target}` counter.
  - `ssq_hook_identity_mismatch_total{instance_class}` correctness counter.
  - `ssq_hook_actor_queue_depth`, `ssq_hook_actor_oldest_event_ms` gauges.
  - `ssq_hook_actor_batch_size`, `ssq_hook_actor_flush_duration_ms` histograms.
  - `ssq_hook_actor_dropped_total{reason}`, `ssq_hook_sqlite_busy_total` counters.
- **Logs**: structured lifecycle/version/identity errors with shortened fingerprints; never log raw tool input, cwd, environment values, session/tool-use IDs, or socket paths.
- **Alerts**: any accepted identity mismatch or decision inconsistency; p99 >50 ms; fallback >1% over a meaningful rolling window; sustained queue saturation/drops.
- **Dashboard repair**: fix p95/max panels currently showing no data and retain the 24-hour baseline (4,419 calls, 38,661 ms average, ~40/min peak) for before/after comparison.

## Risk Control

- **Feature flag**: not gated; direct replacement requested.
- **Compatibility guard**: explicit protocol handshake; incompatible versions use cache/defer, never direct DB fallback.
- **Rollback procedure**: atomic settings backup/restore plus previous binary; no database restore.
- **Staged rollout**: full replacement only after manual-instance, mixed-version, load, crash, race, and rollback tests pass.
- **Semantic compatibility gate**: run a sanitized golden corpus of current standalone `ssq-hooks check` payloads through both the old policy path and `HookClassifier`; any allow/deny/defer, reason, rule-ID, or stdout-format mismatch blocks replacement unless explicitly documented as an intended correction.
- **Endpoint ownership gate**: after binding and before installer cutover, the server must self-probe the exact `HookEndpoint`, verify its `InstanceFingerprint` and `ProtocolVersion`, and fail without changing settings on any mismatch or ambiguous ownership.
- **Provisional performance gates**: sustain 100 requests/second for 60 seconds with 100 concurrent clients, warm p95 <20 ms and p99 <50 ms, zero cross-instance responses, and classification independent of blocked analytics sink. This is deliberately far above the observed ~40/min peak and may be adjusted only with recorded benchmark evidence.

## Unresolved Questions

- [ ] Choose final `HookAnalyticsActor` queue capacity, batch size, and flush deadline from benchmark matrix — blocks Story 4.2.2 — owner: implementation lead. Provisional: capacity 1,000, batch 128, delay 2 ms.
- [ ] Decide whether to adopt a dedicated analytics database after primary-vs-dedicated benchmark and operational-cost comparison — blocks optional Story 4.3.1 — owner: architecture reviewer.
- [ ] Decide whether non-analytics application writes benefit from a shared actor using measured `database/sql` wait/commit data — blocks optional Story 4.3.2 — owner: architecture reviewer.

## Dependency Visualization

```text
Installer duplicate repair ───────────────────────────────────────────────┐
Protocol types → endpoint identity → IPC client/server → server wiring    │
      │               │                    │                              │
      │               └→ manual-instance isolation tests                 │
      ├→ HookClassifier extraction → context cache → snapshot publisher  │
      │                                  │              │                 │
      └──────────────────────────────────┴──────────────┴→ CLI fallback  │
AnalyticsSink → HookAnalyticsActor → batch persistence → benchmark gate  │
      └───────────────────────────────────────────────────────┐           │
Metrics/doctor/dashboard ← all runtime components             │           │
Integration/load/rollback tests ←─────────────────────────────┴───────────┘
```

---

## Phase 1: Canonical hook and protocol foundation

### Epic 1.1: Normalize Claude hook configuration
**Goal**: Guarantee exactly one canonical Stapler classifier invocation while preserving unrelated hooks.

#### Story 1.1.1: Detect and normalize duplicate classifier hooks
**As a** Claude Code user, **I want** hook installation to converge duplicate Stapler entries, **so that** one tool use produces one classification.

**Acceptance Criteria**:
- All PreToolUse commands containing the Stapler rules marker are replaced by one canonical entry while unrelated commands and settings remain byte-semantically intact.
  - *Given* settings containing wrapped and direct `ssq-hooks check` plus a Bash reminder, *When* `InstallRules` runs, *Then* one canonical Stapler group and the reminder remain.
- Concurrent installers leave valid JSON and one canonical entry.
  - *Given* eight concurrent installers, *When* all complete, *Then* parsing succeeds and the canonical marker count is one.

**Files**: `internal/claudehooks/claudehooks.go`, `internal/claudehooks/claudehooks_test.go`, `cmd/ssq-hooks/main.go`

##### Task 1.1.1a: Add canonical-group normalization (~5 min)
- Replace marker-presence no-op with collect/remove/prepend-one transformation.
- Preserve unrelated groups and commands.
- Files: `internal/claudehooks/claudehooks.go`

##### Task 1.1.1b: Add duplicate/wrapper/concurrency tests (~5 min)
- Cover direct, metrics-wrapped, malformed, and unrelated hooks.
- Files: `internal/claudehooks/claudehooks_test.go`

##### Task 1.1.1c: Add settings backup/reporting (~5 min)
- Back up before direct-replacement mutation and report duplicates removed.
- Files: `cmd/ssq-hooks/main.go`, `internal/claudehooks/claudehooks.go`

### Epic 1.2: Define versioned instance-scoped protocol
**Goal**: Parse raw hook data into proven identities before classification.

#### Story 1.2.1: Add protocol value objects and DTOs
**As a** server, **I want** incompatible or cross-instance requests rejected at the boundary, **so that** no decision is issued by the wrong instance.

**Acceptance Criteria**:
- Protocol v1 accepts matching versions/fingerprints and rejects mismatches with typed errors.
  - *Given* expected fingerprint `a1b2c3d4` and protocol 1, *When* a request carries fingerprint `ffff0000`, *Then* classification is not invoked and `ErrInstanceMismatch` is returned.
- Claude `tool_use_id` survives JSON decoding.
  - *Given* `tool_use_id=toolu_123`, *When* payload is decoded, *Then* `ToolUseID("toolu_123")` contributes to a stable request ID.

**Files**: `pkg/classifier/classifier.go`, `internal/hookipc/protocol.go`, `internal/hookipc/protocol_test.go`

##### Task 1.2.1a: Extend payload and define newtypes (~4 min)
- Add `ToolUseID`; define protocol/fingerprint/request/source types.
- Files: `pkg/classifier/classifier.go`, `internal/hookipc/protocol.go`

##### Task 1.2.1b: Implement boundary validation (~5 min)
- Decode with size/version/identity checks and typed errors.
- Files: `internal/hookipc/protocol.go`, `internal/hookipc/protocol_test.go`

#### Story 1.2.2: Resolve short isolated socket endpoints
**As a** manual-test operator, **I want** each state namespace to resolve its own short socket, **so that** it cannot contact production accidentally.

**Acceptance Criteria**:
- Distinct config directories produce distinct socket paths under the Unix path limit.
  - *Given* two long test directories and instance names `manual-a`/`manual-b`, *When* endpoints resolve, *Then* paths differ and each is under platform length limits.
- Isolated endpoint failure never returns the shared endpoint.
  - *Given* `STAPLER_SQUAD_INSTANCE=manual-a` and no listener, *When* resolution occurs, *Then* only manual-a's endpoint is returned.

**Files**: `internal/hookipc/endpoint.go`, `internal/hookipc/endpoint_test.go`, `session/tymux/daemon_config.go`

##### Task 1.2.2a: Extract shared secure socket-path helper (~5 min)
- Reuse config-dir hashing and permission checks without coupling hook IPC to tymux.
- Files: `internal/unixsocket/path.go`, `internal/unixsocket/path_test.go`, `session/tymux/daemon_config.go`

##### Task 1.2.2b: Implement endpoint precedence (~5 min)
- Explicit env/CLI override, then `config.GetConfigDirForDir(payload.Cwd)`-derived endpoint; no scan/fallback.
- Files: `internal/hookipc/endpoint.go`, `internal/hookipc/endpoint_test.go`

---

## Phase 2: Resident concurrent classification

### Epic 2.1: Extract classify-only service
**Goal**: Reuse policy without approval-queue or persistence coupling.

#### Story 2.1.1: Introduce `HookClassifier`
**As a** hook transport, **I want** one classify-only application service, **so that** primary and fallback decisions preserve policy without manual-review side effects.

**Acceptance Criteria**:
- Allow/deny/defer matches the existing classifier for representative Bash, Write, and AskUserQuestion payloads.
  - *Given* a seed rule allowing `git status`, *When* both old policy and `HookClassifier` classify it, *Then* decisions, rule IDs, reasons, and output JSON agree.
- Classifying an escalation does not create a pending approval.
  - *Given* an unmatched Bash command, *When* `HookClassifier` handles it, *Then* it returns defer and approval storage remains empty.
- The fast path preserves existing standalone `ssq-hooks check` semantics without inheriting PermissionRequest-only domain-age, secret-scan, live-session, or queue side effects.
  - *Given* a payload that PermissionRequest would enrich from live session state, *When* PreToolUse `HookClassifier` handles it, *Then* only the standalone rule classifier and AskUserQuestion defer policy run.

**Files**: `server/services/hook_classifier.go`, `server/services/hook_classifier_test.go`, `server/services/approval_handler.go`

##### Task 2.1.1a: Extract pure policy orchestration (~5 min)
- Separate classification/security checks from queue creation/wire response.
- Files: `server/services/hook_classifier.go`, `server/services/approval_handler.go`

##### Task 2.1.1b: Add equivalence and no-side-effect tests (~5 min)
- Table-test outputs and approval-store invariants.
- Files: `server/services/hook_classifier_test.go`, `server/services/approval_handler_test.go`

#### Story 2.1.2: Add immutable rule snapshot ownership
**As a** concurrent classifier, **I want** each request to read one immutable rules version, **so that** reloads cannot produce partial decisions.

**Acceptance Criteria**:
- Concurrent replacement and 1,000 classifications are race-free and each reply names one complete version.
  - *Given* rule versions 10 and 11, *When* replacement races 1,000 reads under `-race`, *Then* every result is wholly version 10 or 11.

**Files**: `pkg/classifier/classifier.go`, `server/services/rules_service.go`, `server/services/hook_classifier_test.go`

##### Task 2.1.2a: Add versioned snapshot API (~5 min)
- Publish copied rules and version atomically through one composition owner.
- Files: `pkg/classifier/classifier.go`, `server/services/rules_service.go`

##### Task 2.1.2b: Add concurrency tests (~5 min)
- Exercise reload/classify interleavings under race detector.
- Files: `server/services/hook_classifier_test.go`

### Epic 2.2: Bound and cache invocation context
**Goal**: Remove Git/environment tail latency while preserving equivalence.

#### Story 2.2.1: Carry only referenced environment values
**As a** security-conscious user, **I want** command expansion preserved without sending my full environment, **so that** server decisions match local invocation safely.

**Acceptance Criteria**:
- Only variables referenced by the command are transmitted and no values are logged.
  - *Given* command `$RUSTC --version`, environment containing `RUSTC=rustc` and `TOKEN=secret`, *When* context is built, *Then* request carries only `RUSTC` and classification sees `rustc --version`.

**Files**: `pkg/classifier/env_context.go`, `pkg/classifier/env_context_test.go`, `internal/hookipc/protocol.go`

##### Task 2.2.1a: Add referenced-variable extractor (~5 min)
- Support existing simple `$VAR`/`${VAR}` semantics with caps.
- Files: `pkg/classifier/env_context.go`, `pkg/classifier/env_context_test.go`

##### Task 2.2.1b: Thread selected environment through context (~4 min)
- Stop resident path from reading server-global environment for client commands.
- Files: `pkg/classifier/classifier.go`, `server/services/hook_classifier.go`, `internal/hookipc/protocol.go`

#### Story 2.2.2: Cache Git repository context
**As a** concurrent user, **I want** repeated cwd lookups coalesced, **so that** Git subprocesses do not dominate p99.

**Acceptance Criteria**:
- One hundred simultaneous cold requests for one cwd execute at most one lookup and warm calls execute none.
  - *Given* cwd `/repo` and empty cache, *When* 100 requests arrive, *Then* lookup spy count is one and all receive the same context.
- A slow lookup does not exceed the request budget when stale context exists.
  - *Given* a 3-second Git lookup and a 3-second-old cached context, *When* classification occurs, *Then* stale context is returned within 20 ms and refresh runs asynchronously.

**Files**: `server/services/hook_context_cache.go`, `server/services/hook_context_cache_test.go`, `pkg/classifier/classifier.go`

##### Task 2.2.2a: Isolate repository-context provider (~5 min)
- Extract Git calls behind interface with bounded context.
- Files: `pkg/classifier/repository_context.go`, `pkg/classifier/classifier.go`

##### Task 2.2.2b: Implement TTL/singleflight cache (~5 min)
- Add stale-while-revalidate and bounded cardinality.
- Files: `server/services/hook_context_cache.go`, `server/services/hook_context_cache_test.go`

### Epic 2.3: Serve the local protocol
**Goal**: Make the primary instance answer parallel requests without SQLite on the response path.

#### Story 2.3.1: Implement secure Unix-socket server
**As a** hook CLI, **I want** a bounded concurrent endpoint, **so that** decisions return in milliseconds.

**Acceptance Criteria**:
- Matching requests receive exact PreToolUse JSON and blocked analytics does not delay response.
  - *Given* an analytics sink blocked for 10 seconds, *When* an allow request is sent, *Then* response arrives under 50 ms.
- Live socket is never unlinked by a second server.
  - *Given* instance A listening, *When* instance B with same endpoint starts, *Then* B fails without interrupting A.

**Files**: `internal/hookipc/server.go`, `internal/hookipc/server_test.go`, `server/server.go`, `server/dependencies.go`

##### Task 2.3.1a: Add listener ownership and deadlines (~5 min)
- Probe stale sockets, bind, chmod, cap body, configure deadlines/shutdown.
- Files: `internal/hookipc/server.go`, `internal/hookipc/server_test.go`

##### Task 2.3.1b: Wire server lifecycle (~5 min)
- Construct with `HookClassifier`; start after dependencies and stop before DB close.
- Files: `server/server.go`, `server/dependencies.go`

#### Story 2.3.2: Add exact request idempotency
**As a** server during upgrade, **I want** duplicate delivery of one tool use to reuse one decision/event, **so that** old duplicate settings cannot double-write.

**Acceptance Criteria**:
- Same session/tool-use ID returns byte-equivalent decision and queues one event; identical command with another ID is processed independently.
  - *Given* `toolu_1` delivered twice and `toolu_2` with the same command, *When* all finish, *Then* three responses exist but two analytics IDs are queued.

**Files**: `server/services/hook_deduper.go`, `server/services/hook_deduper_test.go`, `internal/hookipc/server.go`

##### Task 2.3.2a: Implement TTL-bounded deduper (~5 min)
- Cache only stable IDs; cap entries and never raw-byte match.
- Files: `server/services/hook_deduper.go`, `server/services/hook_deduper_test.go`

##### Task 2.3.2b: Integrate response/event identity (~4 min)
- Apply deduper around service and deterministic analytics ID.
- Files: `internal/hookipc/server.go`, `server/services/hook_classifier.go`

---

## Phase 3: Client, cache, and direct replacement

### Epic 3.1: Primary-first hook client
**Goal**: Make `ssq-hooks check` a bounded IPC client rather than a repository bootstrapper.

#### Story 3.1.1: Send classification to intended primary
**As a** Claude user, **I want** each hook to contact the resident instance, **so that** tool use is not blocked by migrations.

**Acceptance Criteria**:
- Healthy primary path never calls `NewEntRepository` and returns its decision.
  - *Given* a listening manual instance and a repository-open panic spy, *When* `ssq-hooks check` runs, *Then* valid decision JSON is emitted and repository spy is untouched.
- AskUserQuestion remains empty-output defer.
  - *Given* `tool_name=AskUserQuestion`, *When* check runs, *Then* stdout is empty without dialing storage.

**Files**: `cmd/ssq-hooks/main.go`, `cmd/ssq-hooks/main_test.go`, `internal/hookipc/client.go`, `internal/hookipc/client_test.go`

##### Task 3.1.1a: Implement bounded client (~5 min)
- Dial intended socket, send envelope, validate reply, enforce total deadline.
- Files: `internal/hookipc/client.go`, `internal/hookipc/client_test.go`

##### Task 3.1.1b: Replace `handleCheck` storage path (~5 min)
- Preserve Gemini/Antigravity/OpenCode adapters while routing Claude through client/fallback.
- Files: `cmd/ssq-hooks/main.go`, `cmd/ssq-hooks/main_test.go`

#### Story 3.1.2: Add short failure suppression
**As an** automatic-mode user, **I want** a dead endpoint detected quickly, **so that** repeated hooks do not each wait for timeout.

**Acceptance Criteria**:
- After one timeout, requests inside the cooldown skip dialing and use fallback; successful probe closes circuit.
  - *Given* a 100 ms timeout and 1-second cooldown, *When* ten calls arrive during failure, *Then* one dial times out and nine immediately fall back.

**Files**: `internal/hookipc/circuit.go`, `internal/hookipc/circuit_test.go`, `internal/hookipc/client.go`

##### Task 3.1.2a: Implement explicit fallback state machine (~5 min)
- Closed/open/half-open transitions with injected clock.
- Files: `internal/hookipc/circuit.go`, `internal/hookipc/circuit_test.go`

### Epic 3.2: Verified cached rules and defer
**Goal**: Preserve useful automatic decisions without unsafe cross-instance fallback.

#### Story 3.2.1: Publish atomic last-known-good snapshots
**As a** primary instance, **I want** to publish bound rule snapshots, **so that** my hooks can classify during brief outages.

**Acceptance Criteria**:
- Snapshot is atomically replaced only after successful composition and checksum validation.
  - *Given* version 12 is valid and version 13 contains invalid regex, *When* publication is attempted, *Then* disk still contains valid version 12.

**Files**: `internal/hookcache/snapshot.go`, `internal/hookcache/snapshot_test.go`, `server/services/rules_service.go`

##### Task 3.2.1a: Define bounded snapshot schema (~5 min)
- Include schema/protocol, fingerprint, version, rules, checksum, timestamp and caps.
- Files: `internal/hookcache/snapshot.go`, `internal/hookcache/snapshot_test.go`

##### Task 3.2.1b: Wire atomic publication (~5 min)
- Temp write, sync, rename after composed rule replacement.
- Files: `server/services/rules_service.go`, `internal/hookcache/snapshot.go`

#### Story 3.2.2: Classify from cache then defer
**As an** auto-mode user, **I want** verified cached decisions during primary outage and native behavior otherwise, **so that** outages do not hard-deny work.

**Acceptance Criteria**:
- Matching valid cache not older than the configurable maximum age (default one hour) returns a decision; mismatched, corrupt, missing, or expired cache emits empty stdout.
  - *Given* manual-a cache bound to manual-b or created 61 minutes ago, *When* manual-a cannot dial, *Then* cache is rejected and output is empty.

**Files**: `cmd/ssq-hooks/fallback.go`, `cmd/ssq-hooks/fallback_test.go`, `internal/hookcache/snapshot.go`

##### Task 3.2.2a: Implement cache classifier (~5 min)
- Load bound snapshot, compile rules with caps, use invocation context.
- Files: `cmd/ssq-hooks/fallback.go`, `internal/hookcache/snapshot.go`

##### Task 3.2.2b: Pin exact defer/error output (~5 min)
- Table-test all protocol/cache/parse failures and agent modes.
- Files: `cmd/ssq-hooks/fallback_test.go`, `cmd/ssq-hooks/main_test.go`

### Epic 3.3: Manual-instance and managed-session propagation
**Goal**: Make endpoint selection unambiguous for every launched session.

#### Story 3.3.1: Inject endpoint identity into managed sessions
**As a** manual-instance operator, **I want** sessions to inherit their owning endpoint, **so that** same-cwd instances never collide.

**Acceptance Criteria**:
- Two servers managing the same cwd inject different endpoint fingerprints and each hook reaches its owner.
  - *Given* `manual-a` and `manual-b` both use `/repo`, *When* each launches Claude, *Then* probes report their respective fingerprints and state dirs.

**Files**: `session/instance.go`, `session/instance_tmux.go`, `server/services/hook_injector.go`, `server/services/hook_injector_test.go`

##### Task 3.3.1a: Add endpoint environment metadata (~5 min)
- Inject `SSQ_HOOK_SOCKET`, instance fingerprint, and protocol into the managed Claude process environment; plain-terminal hooks derive from `config.GetConfigDirForDir(payload.Cwd)` when no explicit metadata exists.
- Files: `session/instance.go`, `session/instance_tmux.go`, `server/services/hook_injector.go`

##### Task 3.3.1b: Add dual-instance integration test (~5 min)
- Start isolated listeners against same cwd and assert no production access.
- Files: `server/services/hook_injector_test.go`, `internal/hookipc/endpoint_test.go`

---

## Phase 4: Batched write-behind analytics

### Epic 4.1: Define analytics sink and batch unit of work
**Goal**: Persist existing events efficiently without tying response latency to SQLite.

#### Story 4.1.1: Add batch repository port
**As an** analytics actor, **I want** one batch transaction operation, **so that** N events do not incur N commits.

**Acceptance Criteria**:
- A 128-event batch commits atomically and duplicate IDs are idempotent.
  - *Given* 128 events including a repeated request ID, *When* batch persistence succeeds, *Then* one transaction commits 127 unique rows.
- Any transaction failure leaves no partial new rows.
  - *Given* a deterministic invalid row at position 64, *When* commit fails, *Then* none of the 128 rows appear.

**Files**: `server/services/analytics_store.go`, `session/ent_repository_analytics.go`, `session/ent_repository_analytics_test.go`

##### Task 4.1.1a: Define narrow `AnalyticsBatchSink` (~4 min)
- Define the consumer-owned narrow interface in `server/services`; satisfy it structurally from storage/repository adapters without widening the full `session.Repository` interface.
- Files: `server/services/analytics_store.go`, `session/ent_repository_analytics.go`

##### Task 4.1.1b: Implement transactional bulk insert (~5 min)
- Use existing schema/IDs, conflict-ignore exact duplicates, preserve atomicity.
- Files: `session/ent_repository_analytics.go`, `session/ent_repository_analytics_test.go`

### Epic 4.2: Evolve `AnalyticsStore` into actor
**Goal**: Batch with explicit bounded-loss and shutdown behavior.

#### Story 4.2.1: Implement size-or-deadline batching
**As a** primary instance, **I want** one actor to coalesce writes, **so that** classification remains parallel and SQLite writes are efficient.

**Acceptance Criteria**:
- Low traffic flushes by deadline and bursts flush by size in order.
  - *Given* batch size 128 and delay 2 ms, *When* one event arrives, *Then* it flushes within bounded delay; *When* 128 arrive, *Then* they flush without waiting for timer.
- Full queue drops and counts rather than blocking producer.
  - *Given* capacity 1,000 and blocked sink, *When* 1,001 events enqueue, *Then* enqueue remains non-blocking and drop count is one.

**Files**: `server/services/analytics_store.go`, `server/services/analytics_store_test.go`

##### Task 4.2.1a: Add actor batch loop with injectable clock (~5 min)
- Actor owns timer, queue drain, retry classification, metrics hooks.
- Files: `server/services/analytics_store.go`

##### Task 4.2.1b: Add timer/order/saturation tests (~5 min)
- Deterministic fake sink/clock and race tests.
- Files: `server/services/analytics_store_test.go`

#### Story 4.2.2: Benchmark and select actor parameters
**As an** operator, **I want** parameters chosen from evidence, **so that** bounded loss and tail latency are explicit.

**Acceptance Criteria**:
- Benchmark matrix records queue capacities, batch sizes, delays, throughput, p99, allocations, and crash-loss bound; selected values meet SLO.
  - *Given* matrices `{64,128,256}` × `{0.5,2,5 ms}`, *When* benchmark runs against WAL SQLite, *Then* ADR-002 records the winning values and loss window.

**Files**: `server/services/analytics_store_benchmark_test.go`, `project_plans/hook-classification-fast-path/decisions/ADR-002-bounded-loss-analytics-actor.md`

##### Task 4.2.2a: Add real-WAL benchmark matrix (~5 min)
- Measure batch and producer latency, not just operations/sec.
- Files: `server/services/analytics_store_benchmark_test.go`

##### Task 4.2.2b: Record selected tuning (~3 min)
- Update constants/ADR from reproducible result.
- Files: `server/services/analytics_store.go`, `project_plans/hook-classification-fast-path/decisions/ADR-002-bounded-loss-analytics-actor.md`

#### Story 4.2.3: Bound graceful shutdown
**As an** operator, **I want** shutdown to drain without hanging, **so that** accepted bounded loss remains visible.

**Acceptance Criteria**:
- Shutdown drains within budget or reports remaining drops; concurrent producers never panic.
  - *Given* a locked sink and 500 queued events, *When* 1-second shutdown begins, *Then* process returns by deadline and reports unflushed count.

**Files**: `server/services/analytics_store.go`, `server/services/analytics_store_test.go`, `server/server.go`

##### Task 4.2.3a: Add stop-accepting/drain state machine (~5 min)
- Avoid send-on-closed-channel and enforce deadline.
- Files: `server/services/analytics_store.go`, `server/services/analytics_store_test.go`

### Epic 4.3: Execute broader storage research gate
**Goal**: Avoid speculative database migration while preserving evidence and a clean seam.

#### Story 4.3.1: Compare primary versus dedicated analytics DB
**As an** architect, **I want** measured tradeoffs, **so that** 624 MB primary growth is addressed only if worthwhile.

**Acceptance Criteria**:
- Report compares latency, WAL growth, query changes, backup/upgrade/rollback cost, and disk footprint; decision is recorded as proceed/defer.
  - *Given* representative 270k-row data and hook burst workload, *When* both layouts run, *Then* ADR-004 contains metrics and an explicit verdict.

**Files**: `server/services/analytics_storage_benchmark_test.go`, `project_plans/hook-classification-fast-path/decisions/ADR-004-research-gate-broader-sqlite-redesign.md`

##### Task 4.3.1a: Build representative storage benchmark (~5 min)
- Compare current and dedicated layouts without production migration.
- Files: `server/services/analytics_storage_benchmark_test.go`

##### Task 4.3.1b: Record gate verdict (~3 min)
- If proceed, open a separately planned migration; do not implement ad hoc.
- Files: `project_plans/hook-classification-fast-path/decisions/ADR-004-research-gate-broader-sqlite-redesign.md`

#### Story 4.3.2: Measure broader writer contention
**As an** architect, **I want** DB wait evidence, **so that** a global write actor is adopted only when it helps.

**Acceptance Criteria**:
- Representative non-analytics operations are measured with/without analytics batching; global actor remains out of scope unless thresholds are breached.
  - *Given* concurrent session/backlog and analytics writes, *When* benchmark runs, *Then* p99 and `database/sql` wait metrics support an explicit proceed/defer verdict.

**Files**: `session/write_contention_benchmark_test.go`, `project_plans/hook-classification-fast-path/decisions/ADR-004-research-gate-broader-sqlite-redesign.md`

##### Task 4.3.2a: Add contention benchmark and verdict (~5 min)
- No production global actor code unless a follow-up plan is approved.
- Files: `session/write_contention_benchmark_test.go`, `project_plans/hook-classification-fast-path/decisions/ADR-004-research-gate-broader-sqlite-redesign.md`

---

## Phase 5: Diagnostics, observability, and shipping proof

### Epic 5.1: Instrument the fast path
**Goal**: Detect latency, fallback, queue, and correctness regressions without leaking sensitive data.

#### Story 5.1.1: Add low-cardinality metrics and safe logs
**As an** operator, **I want** primary/cache/defer and actor health visible, **so that** direct replacement can be rolled back quickly.

**Acceptance Criteria**:
- Metrics expose specified dimensions without cwd/command/session/tool-use cardinality.
  - *Given* requests from production and manual instances, *When* metrics export, *Then* labels contain instance class and protocol but no raw identifiers.
- Any accepted identity mismatch triggers a correctness error metric/log.
  - *Given* server validation is fault-injected to accept a mismatch, *When* response is produced, *Then* correctness counter increments and alert condition is true.

**Files**: `instrumentation/hook_metrics.go`, `instrumentation/hook_metrics_test.go`, `internal/hookipc/server.go`, `server/services/analytics_store.go`

##### Task 5.1.1a: Define metric instruments and safe dimensions (~5 min)
- Add counters/histograms/gauges and redaction tests.
- Files: `instrumentation/hook_metrics.go`, `instrumentation/hook_metrics_test.go`

##### Task 5.1.1b: Instrument client/server/actor (~5 min)
- Record at component boundaries without payload values.
- Files: `internal/hookipc/client.go`, `internal/hookipc/server.go`, `server/services/analytics_store.go`

#### Story 5.1.2: Repair and extend hook dashboard
**As an** operator, **I want** percentile and max panels plus fallback/queue views, **so that** before/after and rollback thresholds are visible.

**Acceptance Criteria**:
- Dashboard renders p95/p99/max for current metric shape and shows baseline annotation.
  - *Given* 4,419 baseline samples and new fast-path samples, *When* dashboard opens, *Then* percentile panels show data and distinguish old/new paths.

**Files**: `~/dotfiles/stapler-scripts/observability/grafana/dashboards/general/hookmetrics.json` (companion dotfiles change), `instrumentation/hook_metrics.go`

##### Task 5.1.2a: Fix percentile queries and add actor/fallback panels (~5 min)
- Validate queries and low-cardinality variables. Land the dashboard as a separately reviewed companion commit in the dotfiles repository because its source is not stored in this repository.
- Files: `~/dotfiles/stapler-scripts/observability/grafana/dashboards/general/hookmetrics.json`

### Epic 5.2: Add non-sensitive diagnostics
**Goal**: Make manual and degraded paths understandable without success noise.

#### Story 5.2.1: Add `ssq-hooks doctor`
**As a** developer, **I want** one diagnostic command, **so that** I can verify endpoint, protocol, cache, and actor health.

**Acceptance Criteria**:
- Doctor reports healthy/degraded/incompatible with remediation and no sensitive fields.
  - *Given* manual-a with protocol mismatch, *When* doctor runs, *Then* output names manual isolated class, shortened fingerprints, versions, fallback state, and upgrade action but no command/cwd/token.

**Files**: `cmd/ssq-hooks/main.go`, `cmd/ssq-hooks/doctor.go`, `cmd/ssq-hooks/doctor_test.go`, `internal/hookipc/health.go`

##### Task 5.2.1a: Add health endpoint and doctor formatting (~5 min)
- Text and JSON output; no color-only status.
- Files: `internal/hookipc/health.go`, `cmd/ssq-hooks/doctor.go`

##### Task 5.2.1b: Add redaction and degraded-state tests (~5 min)
- Cover missing socket/cache, protocol mismatch, manual isolation.
- Files: `cmd/ssq-hooks/doctor_test.go`

### Epic 5.3: Prove direct replacement and rollback
**Goal**: Meet latency/correctness SLO under realistic failures before shipping.

#### Story 5.3.1: Run concurrency and load suite
**As a** release owner, **I want** repeatable evidence, **so that** the 38.7-second baseline is replaced safely.

**Acceptance Criteria**:
- Warm path sustains provisional load target with p95 <20 ms, p99 <50 ms and zero incorrect decisions, measured both at the socket service boundary and end-to-end by spawning the real `ssq-hooks check` binary (including process startup and JSON I/O).
  - *Given* 100 concurrent clients at 100 req/s for 60 seconds plus a representative short-lived-process run, *When* load tests run, *Then* both boundary and end-to-end SLOs pass and a blocked analytics sink does not affect response percentiles.
- Manual instances remain isolated under load.
  - *Given* production/manual-a/manual-b listeners and same cwd, *When* mixed requests run, *Then* every response fingerprint matches its caller.

**Files**: `tests/integration/hook_fast_path_test.go`, `tests/integration/hook_fast_path_benchmark_test.go`, `scripts/test-hook-fast-path.sh`

##### Task 5.3.1a: Add end-to-end socket/load harness (~5 min)
- Record p50/p95/p99, decisions, fallback and queue behavior for in-process socket clients and actual CLI subprocesses.
- Files: `tests/integration/hook_fast_path_benchmark_test.go`, `scripts/test-hook-fast-path.sh`

##### Task 5.3.1b: Add failure/isolation matrix (~5 min)
- Crash, stale socket, mismatch, queue lock, cache corruption, mixed versions.
- Files: `tests/integration/hook_fast_path_test.go`

#### Story 5.3.2: Validate atomic upgrade and rollback
**As a** release owner, **I want** an exercised rollback, **so that** direct replacement does not strand Claude hooks.

**Acceptance Criteria**:
- Interrupted install leaves old or new valid config, never partial JSON; rollback restores prior command and decisions.
  - *Given* process termination at every settings-write boundary, *When* recovery/rollback runs, *Then* settings parse and exactly one working classifier remains.

**Files**: `internal/claudehooks/claudehooks_test.go`, `scripts/test-hook-upgrade-rollback.sh`, `docs/how-to/hook-fast-path.md`

##### Task 5.3.2a: Add install fault-injection and rollback test (~5 min)
- Exercise old/new CLI/server combinations and backup restore.
- Files: `internal/claudehooks/claudehooks_test.go`, `scripts/test-hook-upgrade-rollback.sh`

##### Task 5.3.2b: Document operations and manual smoke test (~5 min)
- Include unique manual instance startup, doctor, classification probe, teardown, rollback.
- Files: `docs/how-to/hook-fast-path.md`, `CLAUDE.md`
