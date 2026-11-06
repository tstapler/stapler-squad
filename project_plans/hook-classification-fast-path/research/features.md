# Feature Research: Hook Classification Fast Path

## Existing comparable capabilities

1. **Resident permission classifier.** `ApprovalHandler.HandlePermissionRequest` already classifies using the server's in-memory classifier and asynchronously records through `AnalyticsStore` (`server/services/approval_handler.go:458-470`). The new local classify endpoint should reuse classification policy but produce the exact Claude `PreToolUse` response and must not create manual-review queue records.
2. **Remote socket round trip.** `session/sshremote/approval_relay.go` already defines a JSON envelope, bearer verification, Unix-socket path conventions, read/dial deadlines, request/response on one connection, and warns against matching repeated requests by raw bytes because stale answers can be replayed (`pendingRedeliveryTTL`). These are strong precedents, while remote transport itself is out of scope.
3. **Instance isolation.** `config.GetConfigDirForDir` defines test-dir, named-instance, test-process, preferred-workspace, cwd-workspace, and shared-state precedence. `session/tymux/daemon_config.go` hashes config paths into short `$TMPDIR` socket paths and enforces 0700 parent permissions.
4. **Asynchronous analytics.** `server/services/analytics_store.go` already uses a bounded channel and drop counter, but performs one SQLite write per event rather than transactional batches.
5. **Rule reload.** `ClaudeSettingsWatcher` and `RuleBasedClassifier.ReplaceRules` support last-known-good reload and atomic rule-list replacement. The fast path must consume the same source of truth rather than independently loading YAML/DB rules.

## Required behavior and edge cases

### Routing and identity

- Explicit endpoint override wins; otherwise derive from the same resolved config namespace as storage.
- Managed sessions must receive explicit socket/config fingerprint values. Cwd alone is insufficient because two manual instances can manage the same repository.
- Named/test instances must never try the shared production endpoint.
- Preferred-workspace changes require a clear handoff: publish new endpoint metadata atomically and allow old in-flight requests to finish without accepting new mismatched requests.
- Socket handshake rejects instance fingerprint and protocol mismatch before classification.

### Duplicate and replay handling

Current settings contain one wrapped and one direct `ssq-hooks check`; installer detection only asks whether any marker exists and does not normalize duplicates. Installation should converge all recognized Stapler classifier entries to exactly one canonical command while preserving unrelated hooks.

Public Claude hook documentation includes top-level `tool_use_id`; add it to the payload model. Use `(session_id, tool_use_id, hook_event_name)` as an idempotency key for a short bounded response cache and analytics unique key. Do not deduplicate identical command text: repeated commands can be distinct intentional actions. If no stable ID exists, process independently.

### Fallback

- Primary path: instance-local socket and resident rules.
- Socket failure/timeout: verified cached rule snapshot and local context.
- Missing/invalid/stale cache: emit no decision and defer to Claude's active permission mode, per requirements.
- Cache publication must be atomic, checksummed/versioned, source-instance-bound, and written only after successful rule composition.
- Never fall back from an isolated instance to production.
- Use a short failure backoff/circuit state so a down socket does not impose a full timeout on every tool call.

### Classification equivalence

`BuildContext` reads the classifier process environment and runs up to two Git commands (`pkg/classifier/classifier.go:695+`). Moving classification into the server changes environment semantics. The hook should extract referenced variable names from Bash input and send only those values; do not send/log the full environment. Git context should be cached by canonical cwd with singleflight, bounded command deadlines, and stale-while-revalidate behavior.

### Actor/write-behind behavior

- Independent requests classify concurrently; analytics enqueue must never hold the response.
- One actor owns analytics batch ordering and write transactions.
- Flush on size or short timer; graceful shutdown drains within a bounded budget.
- Abrupt shutdown may lose the documented in-memory window (bounded loss accepted).
- Queue full uses a counted drop policy rather than blocking classification.
- Batch failure should retry only with bounded backoff and should not reorder a later successful batch ahead of a failed batch unless explicitly documented.
- Exact-ID uniqueness makes retries/import idempotent.

## Unstated user needs

- Clear degraded-mode diagnostics without noisy output on every successful call.
- A health/version command showing resolved instance, socket, protocol, rule version, queue state, and fallback status without exposing tool input.
- Installer output that reports duplicates removed and where the canonical hook points.
- Manual test commands that make isolation obvious and cannot mutate production settings/data accidentally.
- Dashboards separating production and named/test instance labels.
- Rollback that restores a working hook even if server and CLI versions temporarily differ.

## Failure modes to design explicitly

- Stale socket file after crash; race where a new process unlinks a live old process's socket.
- Socket path length/ownership errors.
- Server accepts request but dies before response; client must not replay an unsafe decision from a prior identical command.
- Rules change between cached and primary decisions.
- Git context call hangs or repository disappears.
- Analytics queue fills, database is locked, batch partially fails, or shutdown budget expires.
- Old hook talks to new server or vice versa during direct replacement/rollback.
- Claude omits `tool_use_id` or changes payload fields.
- AskUserQuestion and other non-permission tools continue to defer correctly.
