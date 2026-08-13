# ADR-010: Session Artifacts Storage: JSON Blob vs. Separate Entity Table

**Date**: 2026-06-22
**Status**: Accepted
**Deciders**: Tyler Stapler

---

## Context

The `session-history-metadata` feature must persist structured artifacts (PR URLs, commit SHAs, external URLs, scan offset) extracted from Claude Code JSONL files. Two storage strategies were evaluated:

**Option A — JSON blob field** (`session_artifacts TEXT` on the Session ent entity): a single nullable TEXT column containing a JSON-encoded `SessionArtifactsBlob` struct.

**Option B — Separate `SessionArtifact` ent entity**: a new database table with one row per artifact, foreign-keyed to Session, with columns for `type`, `value`, `source_file`, `discovered_at`.

---

## Decision

**Option A: JSON blob field on Session** for V1.

---

## Rationale

### Why blob wins for V1

1. **Schema simplicity**: One `ALTER TABLE` (add nullable TEXT column) with zero foreign-key complexity. SQLite migration is a single line. Option B requires a new table, indexes, cascade deletes, and ent edge wiring — 3–4× more generated code.

2. **Access pattern**: Artifacts are always read alongside the session they belong to. The `Session` proto already includes all session fields in a single fetch; embedding artifacts as a sub-message (`SessionArtifacts`) keeps the read path as one query. Option B requires a JOIN or secondary query on every session load.

3. **Write pattern**: The extractor writes infrequently (on JSONL change events) and overwrites the entire blob. An upsert of one TEXT column is simpler and faster than an upsert-per-artifact row with dedup logic at the DB layer.

4. **Incremental scan state**: `ScanOffsetBytes` is per-file metadata, not per-artifact. Storing it in the blob co-locates scan state with artifact data — in a row-per-artifact table, the offset would need a separate `session_artifact_scan_state` table or a denormalized column on Session anyway.

5. **Feature scope**: The listed queries are "show all artifacts for a session" and "feed PR number to poller". Neither requires SQL-level filtering by artifact type. If cross-session artifact search were in scope, Option B would be the correct choice.

### When to migrate to Option B

Reconsider after V1 if any of these become true:
- Cross-session artifact search UI is added (e.g., "all PRs opened by all sessions this week")
- Artifact count per session routinely exceeds 500 entries (blob size concern)
- Per-artifact timestamps needed for timeline view at sub-session granularity

---

## Consequences

**Positive**:
- Minimal ent schema change (one field, no new edges)
- `UpdateInstanceArtifacts` is a single-column update — no JOIN, no cascade
- Blob size bounded by `maxExternalURLs = 50` cap + typical PR/commit counts (< 5 KB per session)
- Full blob written atomically — no partial-update inconsistency

**Negative / Accepted risks**:
- Not queryable at the SQL level by artifact type without JSON path operators (SQLite does support `json_extract`, but we avoid this complexity in V1)
- If the blob grows beyond ~10 KB (edge case: thousands of commit SHAs), SQLite TEXT storage remains fine but deserialization cost rises slightly
- Scan offset is per-file (in-memory `offsets` map in `ArtifactExtractor`); if a session links to multiple JSONL files (after `/clear`), each file gets its own offset but only the most recent blob is persisted. This is acceptable for V1 since artifacts accumulate monotonically.

---

## Alternatives Rejected

**Option B (separate entity table)**: Rejected for V1 due to disproportionate schema complexity for the required query patterns. Noted as the correct path if cross-session querying becomes a requirement.

**Option C (in-memory only, no persistence)**: Rejected because the requirements explicitly state "metadata persists across app restarts."
