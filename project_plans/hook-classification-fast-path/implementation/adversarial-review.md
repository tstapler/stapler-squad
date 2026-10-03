# Adversarial Review: hook-classification-fast-path

**Date**: 2026-09-23
**Verdict**: CONCERNS

## Blockers

None.

## Concerns

- [ ] Direct replacement can strand hooks if installation normalizes settings before a compatible listener or fallback snapshot exists — enforce ADR-003 ordering: verify compatible primary or valid bound cache first, otherwise leave the old complete configuration untouched and provide remediation.
- [ ] A one-hour cached-rule validity default is a policy/security tradeoff, not merely tuning — expose it in diagnostics/config, test expiry boundaries, and ensure expiry always produces exact defer output.
- [ ] A failed analytics batch containing one deterministic bad event can poison subsequent flushes — validate before enqueue and classify transient versus permanent errors; add a test that the actor does not retry forever or block later shutdown.
- [ ] The 100 req/s target is far above observed load but can still hide synchronized cold Git misses and subprocess startup — test warm, cold-distinct-cwd, same-cwd singleflight, and real-binary paths separately.
- [ ] The plan covers Claude global PreToolUse while Gemini/Antigravity/OpenCode modes still call `loadStorage` — explicitly document this boundary and ensure refactoring `handleCheck` does not regress those adapters; create follow-up scope if they exhibit the same latency.
- [ ] A companion dashboard change can be omitted at ship time — make its commit/link a readiness artifact or document why alerting is satisfied elsewhere.
- [ ] Exact idempotency depends on Claude's `tool_use_id`; mixed/older clients may omit it — tests must prove omitted IDs are processed independently and never deduplicated by payload bytes.

## Minors

- Verify macOS behavior for stale Unix socket probing and path limits, not only Linux.
- Ensure settings backups are mode 0600 and bounded/rotated.
- Report analytics drops at aggregate intervals rather than one warning per dropped event during saturation.
- Avoid high-cardinality metrics derived from named instance strings; use class plus controlled identifiers only where necessary.
