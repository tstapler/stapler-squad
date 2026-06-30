# ADR-021: Enrichment at Service Layer, Not Scanner

**Status**: Accepted
**Date**: 2026-06-24

## Context

The github-work-continuity feature needs to surface GitHub PR status (review state, CI status, mergeable, URL) alongside the git worktree data already produced by `session/unfinished/scanner.go`.

Two insertion points exist for this enrichment:

1. **Inside the scanner** (`session/unfinished/scanner.go`): The scanner walks the filesystem and reads git refs. It could be extended to call the GitHub API for each worktree it discovers.

2. **At the service layer** (`UnfinishedWorkService.ListUnfinishedWork()`): The service already aggregates `ScanResult` objects with session state. It could join scanner output with the `PRStatusPoller`'s in-memory index after the scan completes.

The scanner is currently a pure-git, stateless reader with no network I/O, no auth dependency, and no external imports beyond the standard library and git tooling. It is fast enough to run on every UI refresh.

The service layer already has precedent for enrichment: it merges raw scan results with `Instance` session data before returning `UnfinishedWork` items to callers.

## Decision

`session/unfinished/scanner.go` remains pure-git with no GitHub imports. All GitHub enrichment happens in `UnfinishedWorkService.ListUnfinishedWork()` by joining `ScanResult` data with the `PRStatusPoller`'s in-memory index (`map["owner/repo/branch"]PRInfo`).

The join key is derived from the `ScanResult`'s repository remote URL (normalized to `owner/repo`) combined with the worktree's head branch name. If the poller index has an entry for that key, its `PRInfo` is attached to the returned `UnfinishedWork` item. If not, the item is returned without PR enrichment — the feature degrades gracefully.

## Consequences

### Positive
- The scanner retains its current performance characteristics: no network I/O, no auth dependency, safe to call on every UI tick.
- Adding GitHub auth failures or rate limits to the scanner would block the entire scan; keeping enrichment in the service layer means scan results are always available regardless of GitHub API health.
- The enrichment join follows an existing pattern (scanner results + session data) in the service layer; reviewers have immediate context for why the join lives there.
- Unit-testing the scanner remains straightforward — no mock HTTP clients or credential fixtures needed.

### Negative / Risks
- The service layer now has two async data sources (scanner output and poller index) with different staleness characteristics. Callers see a point-in-time snapshot of both; there is no transactional consistency guarantee.
- The join key derivation (remote URL → `owner/repo` normalization) must handle SSH remotes, HTTPS remotes, and remotes with `.git` suffixes; incorrect normalization silently produces no enrichment rather than an error.

### Mitigations
- Remote URL normalization is implemented once in a shared helper and covered by a table-driven unit test that exercises SSH, HTTPS, and `.git`-suffix variants.
- The staleness gap between scanner and poller is bounded by the poller's refresh interval (configurable, defaulting to 60 seconds), which is documented in the `PRStatusPoller` struct comment.
