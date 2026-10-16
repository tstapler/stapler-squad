# Research (code read, VERIFIED by reading)
- CapacityMonitor (server/services/capacity_monitor.go:239) computes ContextTokensUsed live from last turn; not persisted.
- session/tokens/parser.go builds ParseResult.TurnTimeline ([]TurnStats: Timestamp, Model, Input, Output, CacheCreation, CacheRead) from Claude JSONL; TokenStore (store.go:197) caches ParseResult in memory only, keyed by file path, invalidated by modtime. So history is *derivable* from transcripts but not stored/queryable across restarts or transcript deletion.
- Parser/jsonl_types have no handling of compaction (grep "compact" empty). Claude Code JSONL emits `system` entries with subtype `compact_boundary` (compactMetadata: trigger, preTokens) and `isCompactSummary` user messages — VERIFIED 2026-10-05 against a real transcript in ~/.claude/projects: `compactMetadata` has trigger, preTokens, postTokens, cumulativeDroppedTokens. Still need a sanitized fixture in-repo.
- findings.go: detectOversizedStartContext uses only TurnTimeline[0]; Finding types are proto enum aliases (finding_types.go) -> new type needs proto change + make proto-gen.
- /compact in slash_command_service.go is only a catalog entry; the command runs inside Claude Code, so server-side invocation logging is impossible — compaction must be detected from the transcript (or tmux output).
- ent schemas exist (analytics_event.go as precedent). Generated ent code is not committed; edit schema only.

## Options
A. Derive-only: extend parser with ContextSamples + CompactEvents, compute on demand. Cheap, no storage, but no history if transcript pruned and no cross-session aggregate without full reparse.
B. Persist in ent tables (ContextSample, CompactionEvent) written by TokenStore on parse, upsert keyed (session_uuid, turn_index/timestamp). Recommended: durable, queryable aggregates.
Recommend B built on A's parser extension. Tokens freed = preTokens minus first post-compact turn context.
