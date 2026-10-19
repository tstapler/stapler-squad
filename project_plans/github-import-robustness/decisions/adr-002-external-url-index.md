# ADR-002: Index `external_url` in ent Schema for `ListGitHubIssues` Dedup Lookup

## Status
Proposed

## Context
`ListGitHubIssues` will cross-reference returned issues against local backlog items using `external_url` (e.g., `https://github.com/owner/repo/issues/42`). Currently `GetBacklogItemsByExternalIDs` uses `(source_id, external_id)` — both indexed. A new `GetBacklogItemsByExternalURLs` method using `external_url` would do a full table scan without an index.

## Decision
Add an ent index on `external_url` in `session/ent/schema/backlogitem.go`. This is a single-column index; the field is already `Optional` and stored for all items with an external link.

## Consequences
- One additional ent schema migration (auto-generated via `make ent-gen`).
- `external_url` lookups become O(log N) instead of O(N).
- The field already exists; this is index-only, not a schema data change.
- Alternative (use `(source_id, external_id)` directly): requires knowing the source_id, which is unavailable for manually-imported items. Rejected.
