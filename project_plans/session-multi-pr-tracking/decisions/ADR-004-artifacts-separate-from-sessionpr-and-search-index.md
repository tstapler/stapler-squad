# ADR-004: Artifacts stay separate from `SessionPR`; structured blob v2; in-memory search index

**Status**: Accepted (owner decision 2026-11-03) — **Project**: session-multi-pr-tracking

## Context
`session_artifacts` (JSON blob, `session/artifacts/`) records PR URLs scraped from transcripts, including PRs the session only mentioned. `OnScanComplete` seeds the legacy PR number from the first one (`server/dependencies.go:1874-1888`). The tab shows "Extraction pending" forever for empty scans (`scan.go:63-67`). Search over history uses an in-memory BM25 engine (`session/search/`; FTS5 rejected in `session-search-fts5` ADR-001).

## Decision
- `SessionPR` holds agent-claimed and API-created PRs only. Scanner `PRURLs` and the new PR records are display-only and never seed `SessionPR` or the legacy number, regardless of the flag: the `OnScanComplete` seed block (`server/dependencies.go:1874-1888`) is deleted (Phase 4 B2: the old seed ends in `SetGitHubPRNumber`, which the plan turns into an `AttachPR` wrapper, so a flag-conditional skip would let a flag-off session create a phantom, polled PR). Older sessions' history is therefore not recovered from the scanner.
- Blob v2 holds typed `ArtifactRecord`s modeled as a sealed-interface sum type (one variant per kind: pr, branch, commit, issue, plan_doc, ci_link, command; decoded at one parse point) with timestamps, authored-vs-mentioned, tool_use/tool_result pairing, host-aware PR patterns and redaction. v1 lists remain readable and `pr_urls` stays populated in proto. The v2 writer keeps writing the v1 lists so an older binary (revert-PR rollback) still reads the blob; the history rescan triggered by the scan-version bump is behind the live flag `artifact_scan_v2` (default off).
- No new ent tables for artifacts. An in-memory `ArtifactIndex` (BM25 documents in a dedicated second `Engine`/`DocumentStore` instance that shares the tokenizer and BM25 code with history search but not its store, so artifact hits never leak into `SearchClaudeHistory` and corpus statistics stay separate; plus exact maps from `PRRef` and SHA prefix to session uuids) is built from stored blobs at start and updated by `OnScanComplete`. Exact PR lookup unions `SessionPR` links (orphans filtered) and artifact PR records and labels each hit with its source.
- Browse across sessions uses a paginated `ListSessionArtifacts` RPC over the same in-memory data; page route `/artifacts` with a `route-meta.ts` row.

## Alternatives rejected
- Seeding `SessionPR` from the scanner: phantom PRs get polled and can wedge `pr_pending`.
- Ent tables per artifact type: a second persisted store to keep consistent with the blob for a few hundred sessions; revisit if the index no longer fits memory.
- SQLite FTS5 or `LIKE` over the blob: FTS5 already rejected; `LIKE` is unindexed.

## Consequences
Index is lost on restart and rebuilt (bulk blob load already happens in `LoadInstances`). Scan-version bump triggers a paced re-scan of transcripts. Redaction must also apply to the debug bundle export.
