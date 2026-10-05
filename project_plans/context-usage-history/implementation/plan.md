# Plan
1. Parser: add CompactEvent + context sample derivation (ContextTokens per turn) to ParseResult; detect compact_boundary/isCompactSummary; fixtures.
2. ent schemas ContextSample, CompactionEvent (unique idx session_uuid+ts); make ent-gen.
3. Repository with idempotent upsert + bounded retention; wire into TokenStore.parseAndCache (async, off hot path).
4. Query RPCs (proto) : GetContextHistory, GetCompactionStats; handler in server/services; make proto-gen; registry-generate.
5. New finding FindingContextGrowthNoCompaction in findings.go + proto enum + config thresholds; DollarImpact=0 so it never adds to the summed waste score (no ADR governs this; decision recorded here).
6. Tests: parser, repo idempotency, detector table tests, service.
7. Docs in docs/reference.

## Adversarial review
- Risk: compact JSONL schema assumption -> gate step 1 on real fixture.
- Risk: sample volume (every turn) -> store only per-turn rows keyed idempotently, retention by age.
- Risk: double counting with SessionTokenCeiling finding -> keep DollarImpact zero (decision in step 5).
- Risk: reparse writes on every file change -> upsert only new turns past stored high-water mark.

## Migration Plan
- Schema change is purely additive: two new ent tables (ContextSample, CompactionEvent); no existing table altered. Auto-migrate creates them on startup, zero downtime.
- Reversible: dropping the two tables (or ignoring them) restores prior behavior; nothing reads them except the new RPCs/finding.
- Compaction parsing uses `compact_boundary` system entries: compactMetadata.trigger, preTokens, postTokens (VERIFIED against a real transcript, 2026-10-05). Tokens freed = preTokens - postTokens; fall back to first post-boundary turn when postTokens is absent.
