# Requirements: context-usage-history

Persist per-session context-window usage history, detect unchecked growth, and record /compact telemetry. Prerequisite plumbing for #244 and #248.

## Acceptance criteria
1. Per-turn (or throttled periodic) context-tokens-used samples (input+cache_read+cache_creation, model, max) are durably stored per session and survive restart.
2. A query API returns a session's context series and "time near ceiling" (fraction of turns >=75%/90% of max) so sessions can be ranked.
3. New finding (FindingContextGrowthWithoutCompaction) fires when context grew by a configured factor/absolute amount across the session with no compaction event; abstains for unpriced/short sessions.
4. Compaction events (session, timestamp, tokens before, tokens after, tokens freed, trigger manual/auto) are recorded and queryable per session and in aggregate.
5. Recording is idempotent (re-parse of the same JSONL yields no duplicates) and does not block the capacity monitor path.
6. Retention/size bounded; existing detectors and CapacityMonitor behaviour unchanged; unit tests for each.

## Out of scope
UI for #244/#248, compaction automation.
