# Plan
1. Parser: add CompactEvent + context sample derivation (ContextTokens per turn) to ParseResult; detect compact_boundary/isCompactSummary; fixtures.
2. ent schemas ContextSample, CompactionEvent (unique idx session_uuid+ts); make ent-gen.
3. Repository with idempotent upsert + bounded retention; wire into TokenStore.parseAndCache (async, off hot path).
4. Query RPCs (proto) : GetContextHistory, GetCompactionStats; handler in server/services; make proto-gen; registry-generate.
5. New finding FindingContextGrowthNoCompaction in findings.go + proto enum + config thresholds; wire into waste score only if ADR-002 allows.
6. Tests: parser, repo idempotency, detector table tests, service.
7. Docs in docs/reference.

## Adversarial review
- Risk: compact JSONL schema assumption -> gate step 1 on real fixture.
- Risk: sample volume (every turn) -> store only per-turn rows keyed idempotently, retention by age.
- Risk: double counting with SessionTokenCeiling finding -> keep dollar impact zero/non-summable per ADR-002.
- Risk: reparse writes on every file change -> upsert only new turns past stored high-water mark.
