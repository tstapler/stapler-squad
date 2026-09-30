# Requirements: hook-classification-fast-path

**Date**: 2026-09-23
**Type**: feature addition / cross-cutting performance redesign
**Complexity**: 4 — high-stakes / cross-cutting

## Problem Statement

Stapler Squad's global Claude Code `PreToolUse` classifier delays every tool call because each short-lived `ssq-hooks check` process opens the workspace's full SQLite database and runs Ent schema reconciliation and startup data migrations before classifying. The measured 24-hour average was initially approximately 22 seconds per invocation and later rose to **38,661 ms across 4,419 measured invocations**; direct reproduction is approximately 7–8 seconds even without queueing. The latest dashboard shows bursts around **40 invocations/minute** (with a sampled `ssq-hooks-pretool` point of 26/minute). The global Claude configuration also currently invokes `ssq-hooks check` twice for each tool call, doubling work and analytics writes.

This makes permission classification disruptive for developers and unattended/automatic Claude Code sessions. The hook path needs to return decisions from the correct running Stapler Squad instance with low latency, preserve isolated/manual test-instance boundaries, and decouple classification latency from SQLite persistence.

## Baseline

Today, every global Claude Code tool call launches the classifier executable. It opens a workspace database that can be hundreds of megabytes, requests WAL mode, performs schema initialization, executes all startup migrations (including repeated Git subprocesses), loads rules, classifies, and synchronously writes analytics. The latest measured wrapper average is 38,661 ms over 4,419 invocations in 24 hours; a current local invocation takes approximately 7–8 seconds. Observed dashboard bursts reach roughly 40 invocations/minute. The same classifier is registered twice in the active Claude Code global configuration, while only one copy is measured.

Stapler Squad already supports default, named, workspace, test-directory, and automatically isolated test state. Manual instances use distinct `STAPLER_SQUAD_INSTANCE` values and must not read, write, or route through the deployed instance. Existing remote/SSH approval transport remains separate.

## Users / Consumers

- Developers using Claude Code with Stapler Squad's global hooks.
- Unattended or automatic Claude Code sessions where interactive permission prompts are uncommon or undesirable.
- Stapler Squad's deployed primary instance.
- Named manual-development instances and test harnesses using `STAPLER_SQUAD_INSTANCE` or `STAPLER_SQUAD_TEST_DIR`.
- The classification analytics and audit consumers that read persisted hook events.
- Future remote transports consuming a versioned classification protocol.

## Success Metrics

- Warm synchronous classification through the intended running instance achieves p95 below 20 ms and p99 below 50 ms, measured end-to-end across the real local transport.
- Hook response latency is independent of analytics database write latency, migration work, and batch flushes.
- Exactly one classifier request is issued per Claude Code tool use after installation/configuration repair.
- Production, named manual, workspace-isolated, and test-directory instances route only to their own endpoint and persistence namespace; cross-instance routing is zero-tolerance.
- When the intended instance is unavailable, the hook uses a verified cached rule snapshot when possible and otherwise defers to Claude Code's current permission behavior rather than silently allowing or hard-denying.
- The design sustains realistic bursts with parallel classification and no head-of-line blocking; Phase 2 research must establish a benchmark load target substantially above the observed roughly 4,800 measured calls/day.
- Write-behind analytics provide bounded-loss semantics: decision responses are not delayed for durable persistence, and loss is limited to the documented in-memory batch/window on abrupt process termination.

## Appetite

Large (3–6 weeks)

Scope must fit the appetite. Broader SQLite actor and sharded-store work is research-gated: the hook fast path is mandatory, while broader storage changes proceed only when evidence shows measurable benefit and acceptable migration risk.

## Constraints

- Preserve the current classifier's allow, deny, and defer behavior and Claude Code response formats.
- Support the full existing state-isolation hierarchy implemented by `config.GetConfigDirForDir`, including manual instances and test harnesses.
- An isolated or named instance must never silently fall back to the deployed/default instance.
- Keep SQLite WAL mode for databases that continue to use SQLite.
- Do not copy a live SQLite main file independently of its WAL; any copy/aggregation design must use a SQLite-safe mechanism or closed immutable shards.
- The synchronous decision path must not run schema migrations or block on analytics persistence.
- Automatic-mode sessions must still receive cached decisions where available. If no valid cached decision can be produced, defer to Claude Code even when that may result in mode-specific behavior.
- Replace the old path directly after validation rather than using a staged feature-flag rollout. A concrete rollback procedure is still required.
- Protocol and storage changes must remain compatible with local macOS and Linux operation.

## Non-functional Requirements

- **Performance SLO**: warm end-to-end p95 < 20 ms and p99 < 50 ms; no synchronous analytics write on the response path.
- **Scalability**: parallel request handling with burst benchmarks; expected steady-state volume is currently low (roughly 4,800 measured calls/day), but the implementation must avoid serialization and head-of-line blocking under concurrent agent sessions.
- **Security classification**: internal/confidential local developer data; tool inputs and selected environment values may contain sensitive material and must not be logged or exposed cross-instance.
- **Data residency**: local machine only for socket traffic, caches, queues, and SQLite storage unless an existing explicitly configured remote relay is used.

## Scope

### In Scope

- Remove duplicate Stapler Squad classifier registrations from Claude Code configuration and make installation/detection prevent recurrence.
- Add an instance-scoped, low-latency local classification transport with explicit protocol versioning and correct `PreToolUse` response behavior.
- Resolve endpoints consistently across default, named manual, workspace-isolated, test-directory, and test-mode state.
- Ensure managed sessions receive an unambiguous endpoint identity; reject cross-instance or protocol-mismatched requests.
- Process independent classifications concurrently using immutable/in-memory rule state.
- Cache or coalesce repeated context discovery work without coalescing semantically distinct tool decisions.
- Add write-behind analytics using an actor/single-writer model with batching and bounded-loss semantics.
- Coalesce safe database operations and deduplicate the same tool-use event when a stable request/tool-use identifier exists.
- Research whether broader primary-database writes should share the actor and whether hook analytics merit a dedicated database or immutable shards aggregated by the primary.
- Provide cached-rule fallback followed by defer behavior when the intended instance is unavailable.
- Add latency, queue, fallback, protocol, routing, batch, loss/drop, and decision-consistency observability.
- Add concurrency, isolation, protocol, failure, benchmark, and manual-instance tests.
- Define a transport-neutral protocol boundary that can support remote/SSH transport later without implementing that transport now.
- Document direct-replacement deployment and rollback.

### Out of Scope

- Replacing or redesigning the existing SSH remote approval relay in this project.
- Changing the semantic policy of existing classification rules.
- Guaranteeing zero analytics loss on abrupt crashes; bounded loss is explicitly accepted.
- Treating identical commands as duplicates without a stable tool-use/request identifier.
- Routing isolated requests to whichever Stapler Squad process happens to be available.
- Replacing all SQLite-backed application storage unless Phase 2 evidence and benchmarks justify a specifically bounded extension.
- General distributed or multi-host database replication.

## Rabbit Holes

- Expanding the write actor from hook analytics to every application write could become a repository-wide transaction and API redesign.
- SQLite shard aggregation may add more lifecycle, schema-version, backup, checkpoint, and recovery complexity than the current event volume warrants.
- Environment-dependent command expansion can differ between the hook process and the primary process; transmitting too much environment data risks exposing secrets, while transmitting too little can alter decisions.
- Git context construction currently executes subprocesses and can violate the latency SLO on cache misses or pathological repositories.
- Claude Code hook payload/version differences may affect availability of a stable `tool_use_id` for deduplication.
- Direct replacement without a feature flag increases the importance of protocol compatibility, health checks, and a tested binary/config rollback.
- Unix socket path length, ownership, stale-socket recovery, and multiple simultaneous manual instances require careful lifecycle handling.
- Preferred-workspace and per-directory workspace resolution can be ambiguous if endpoint identity is inferred only from cwd.

## Alternatives Considered

- Keep opening the primary database per hook but skip migrations. This reduces cost but retains per-process startup, database contention, duplicated classification state, and direct analytics writes.
- Continue direct SQLite writes with WAL and busy timeouts. WAL is already enabled and does not remove schema/migration or process-startup overhead.
- Write every hook event into per-process SQLite shards and aggregate copies. This may be useful at higher volume but is research-gated; direct copying of a live WAL database is unsafe.
- Use append-only event files as a fallback spool. This is simpler than SQLite shards but may be unnecessary under the accepted bounded-loss model unless queue overflow handling requires it.
- Send classifications to a fixed localhost HTTP port. This is easier to inspect but introduces port collisions and weaker instance isolation than a config-derived local endpoint.
- Always defer or deny when the primary is unavailable. The chosen behavior instead prefers a verified cached decision, then defers.

## Feasibility Risks

- The running primary currently exposes classification through permission-request behavior, but the global `PreToolUse` hook requires a distinct response schema and classify-only semantics.
- Current rule reload behavior may not provide an immutable snapshot suitable for lock-free concurrent classification and fallback cache publication.
- The primary server's process environment is not necessarily the invoking Claude process's environment, which can change command expansion and therefore classification results.
- Existing analytics tables are large; migration or extraction into a dedicated store may be expensive and risky.
- SQLite remains a single-writer database even in WAL mode. Batching can improve throughput, but long unrelated transactions can still block a shared writer actor.
- Direct replacement requires hook binary/config protocol compatibility across upgrade and rollback boundaries.
- The current source may not retain Claude's stable tool-use identifier, limiting exact deduplication until the payload model is extended and validated.

## Observability Requirements

- Histograms for end-to-end hook latency and server-side classify latency, including p50/p95/p99.
- Counters by path: primary socket, cached fallback, defer fallback, connection failure, timeout, protocol mismatch, instance mismatch, and classification error.
- Actor metrics: queue depth, enqueue latency, batch size, batch flush duration, database busy time, dropped events, and oldest queued-event age.
- Decision-consistency instrumentation comparing request identity, rule snapshot version, and final decision without logging sensitive tool input.
- Correctness alerts on any cross-instance routing attempt, accepted identity mismatch, incompatible protocol, or detected decision inconsistency.
- Performance alerts when p99 exceeds 50 ms or more than 1% of requests use cached/defer fallback over a meaningful rolling window.
- Dashboard visibility for manual/test instances without merging their state or metrics identity into production.

## Risk Control

Direct replacement after pre-ship validation; no staged feature flag. The ship plan must include:

- a compatibility/version handshake before accepting a socket response;
- automatic cached/defer fallback on endpoint failure;
- an atomic installer/config update that removes duplicate and obsolete hook entries;
- a tested rollback that restores the previous hook binary/config without requiring database restoration;
- pre-ship load, manual-instance isolation, stale-socket, crash, and old/new-version compatibility tests;
- immediate rollback on correctness alerts or sustained breach of the latency/fallback thresholds.

## Open Questions

- Should the local transport use a custom framed Unix-socket protocol, HTTP over Unix sockets, or another existing local RPC mechanism?
- Which existing server/classifier components can be reused without inheriting permission-queue side effects?
- What safe subset of the invoking process environment is required to preserve classification equivalence?
- What cache key, TTL, invalidation, and stale policy are required for Git/context and rule snapshots?
- Does Claude Code reliably provide a stable tool-use identifier across supported hook versions, allowing exact request coalescing and analytics deduplication?
- What bounded-loss window and maximum actor queue size best balance latency, memory, and analytics value? *(unresolved after Phase 2 research; use provisional defaults and benchmark gates in the plan)*
- Do benchmarks justify extending the actor to other SQLite writes or creating a dedicated analytics database/shard aggregator? *(unresolved after Phase 2 research; benchmark-gated)*
- If dedicated shards are justified, what schema-version and lifecycle protocol makes them immutable and safely attachable/importable?
- How should preferred-workspace switches update endpoint discovery without routing in-flight hooks to the wrong instance?
- What concrete burst throughput should become the acceptance threshold after measuring realistic multi-session concurrency? *(unresolved after Phase 2 research; current observed peak is ~40/minute, and the plan must choose a deliberately conservative higher provisional target)*
