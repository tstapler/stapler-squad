# Pitfalls Research: Hook Classification Fast Path

## 1. Unix socket lifecycle and security

**Pitfall:** blindly removing the socket before bind can steal the pathname from a still-live listener. Probe/connect first, use an instance lock/owner metadata, and only remove when the owner is demonstrably stale. Clean shutdown closes the listener and removes its own path.

**Pitfall:** AF_UNIX paths are short (roughly 104–108 bytes on common systems). Follow `session/tymux/daemon_config.go`: hash the resolved config directory into a short `$TMPDIR` directory, create it 0700, verify owner/mode, and create the socket 0600.

**Pitfall:** a world-accessible local socket can expose sensitive tool inputs or permit forged decisions. Validate peer filesystem access, instance fingerprint, protocol, bounded request body, method/path, and never log raw payload/environment.

## 2. Cross-instance isolation

**Pitfall:** cwd-based discovery routes two manual instances for the same repo to the wrong primary. Inject an explicit endpoint into managed sessions and derive fallback only from `config.GetConfigDirForDir` precedence. Named/test instances must never try the shared endpoint.

**Pitfall:** preferred workspace switches change storage while old sockets remain. Bind endpoint identity to a config-directory fingerprint and boot ID; reject mismatches and coordinate old listener drain/new listener publication.

## 3. Protocol and direct replacement

**Pitfall:** new CLI and old server disagree on response shape during upgrade/rollback. Version every request; use additive JSON fields; define supported version range; return a machine-readable incompatible response; then cached/defer.

**Pitfall:** changing global settings non-atomically can leave no classifier or duplicates. Installer should parse, normalize all recognized Stapler entries to one canonical entry, preserve unrelated hooks, write temp+fsync+rename, and retain a rollback backup.

**Pitfall:** duplicate requests can produce conflicting decisions if rules change between copies. Retain Claude's stable `tool_use_id`, key a short response cache by session/tool-use/event, and make analytics ID deterministic. Never dedupe on command bytes alone; the remote relay already documents stale replay from byte matching.

## 4. Parallel classification

**Pitfall:** a global lock around classification or context construction creates head-of-line blocking. Rules should be immutable snapshots; only snapshot replacement is serialized. Cache Git context and singleflight only equal cwd misses.

**Pitfall:** `BuildContext` has two Git subprocesses with 5-second timeouts and uses server environment. Use shorter bounded lookup, stale cache, and request-carried referenced variables. A cold cache must not turn into the new tail-latency failure.

**Pitfall:** unbounded goroutines or bodies permit local resource exhaustion. Configure server timeouts, body caps, and request concurrency limits with fast defer fallback under overload.

## 5. Actor/write-behind semantics

**Pitfall:** “actor” becomes a global database God object. Keep a typed analytics actor and sink interface first. Broader write serialization is research-gated.

**Pitfall:** batching timer bugs leak timers, starve low traffic, or never drain on shutdown. Use one owned timer, size-or-time flush, explicit states, and deterministic fake-clock tests.

**Pitfall:** closing a producer channel while concurrent sends occur panics. Prefer context cancellation plus actor-owned intake state or a synchronized `StopAccepting`; test concurrent shutdown under `-race`.

**Pitfall:** retry loops make the queue unbounded or block decisions indirectly. Producers never wait for commit. Actor retries boundedly, reports queue age/drop count, and sheds according to the accepted bounded-loss policy.

**Pitfall:** one bad row aborts every batch forever. Validate before enqueue; on batch failure, distinguish transient database failure from deterministic row failure and bisect/quarantine only if evidence requires it.

**Pitfall:** `Stop` can hang waiting on a locked DB. Set a shutdown deadline; drain what fits, report remaining loss, and exit.

## 6. SQLite/WAL

**Pitfall:** WAL is mistaken for multi-writer support. SQLite still serializes writers. One writer actor reduces lock races; readers can remain concurrent.

**Pitfall:** connection-local pragmas are only applied to one pooled connection. Apply through DSN/connection initialization. Existing `internal/sqlitedsn` does this for WAL and timeout.

**Pitfall:** copying only the main DB loses WAL-resident commits or creates inconsistency. Use SQLite online backup or close/atomically rename immutable shards.

**Pitfall:** aggressive manual checkpoints create latency or wait behind readers. Keep default autocheckpoint until metrics prove a problem; close rows promptly. `synchronous=NORMAL` is only acceptable for a separate bounded-loss analytics store, not silently for core state.

## 7. Snapshot fallback

**Pitfall:** stale or cross-instance cache weakens policy. Include instance fingerprint, protocol/schema, rule version, timestamp, checksum, and atomic publication. Reject unknown rule fields fail-closed at classification level—but the final transport behavior remains defer per requirements.

**Pitfall:** cache reader recompiles malicious/invalid regex indefinitely. Bound file size/rule count/regex length and preserve last known good.

**Pitfall:** fallback behavior accidentally returns auto-allow when decoding fails. All error paths should produce no hook decision and tests must inspect exact stdout.

## 8. Observability hazards

Do not label metrics with cwd, command, session ID, tool-use ID, socket path, or instance names with unbounded cardinality. Use normalized instance class (shared/named/test), protocol version, path (primary/cache/defer), result, and coarse error code. Sample traces if needed and redact payloads. Correctness alerts must distinguish rejected cross-instance attempts from an actually accepted mismatch.
