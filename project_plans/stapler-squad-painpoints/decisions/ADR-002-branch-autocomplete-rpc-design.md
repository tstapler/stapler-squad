# ADR-002: Branch Autocomplete RPC Design

**Status**: Accepted
**Date**: 2026-04-16
**Deciders**: Tyler Stapler

---

## Context

The `SessionWizard.tsx` already imports `useBranchSuggestions` and wires it into `AutocompleteInput`. However, the current `useBranchSuggestions.ts` hook fetches branch suggestions by calling `ListSessions` and extracting unique branch names from existing sessions — it does NOT read actual git refs from the repository. When a user creates a session for a repo that has no existing sessions, the dropdown shows hardcoded fallback values (`main`, `master`, `develop`, `feature/`, etc.) rather than the actual branches in that repo.

The `AutocompleteInput` component handles keyboard navigation (arrows, Enter, Escape, Tab) and async loading state. No new UI component is needed — only the data source changes.

Two RPC options were evaluated:

- **Option A (New `ListBranches` RPC)**: Add `ListBranches(repoPath, includeRemote)` as a standalone unary RPC to `SessionService`. Shell out to `git for-each-ref refs/heads --format='%(refname:short)'` with a 2-second timeout and an in-process 5-minute LRU cache.
- **Option B (Extend path-completion)**: Add a `branches` field to the existing path-completion endpoint response and return branch names alongside directory completions when the input looks like a repo path.

---

## Decision

**Implement Option A: new `ListBranches` unary RPC.**

Proto definition:

```protobuf
rpc ListBranches(ListBranchesRequest) returns (ListBranchesResponse) {}

message ListBranchesRequest {
  string repo_path    = 1;
  bool include_remote = 2;  // default false for session creation
  string filter       = 3;  // optional substring filter (case-insensitive)
  int32 max_results   = 4;  // 0 = no limit; default 200
}

message ListBranchesResponse {
  repeated string branches    = 1;
  int32           total_count = 2;
  bool            truncated   = 3;
}
```

Backend implementation: shell out to `git -C <repoPath> for-each-ref refs/heads --format='%(refname:short)'` inside a `context.WithTimeout(ctx, 2*time.Second)`. Filter in Go (not via shell grep — avoids injection risk). Cache results per `repo_path` key with a 5-minute TTL using a `sync.Map` of `{branches []string, cachedAt time.Time}` entries.

Frontend: replace the `ListSessions`-based logic in `useBranchSuggestions.ts` with a `ListBranches` RPC call, triggered when `repositoryPath` changes. Abort in-flight requests via `AbortController` when `repositoryPath` changes before the previous request resolves.

---

## Rationale

Clean separation: branch listing and path completion are different domain concepts. Conflating them (Option B) would make the path-completion RPC harder to reason about and test, and would require client-side parsing to separate branch results from path results.

The `git for-each-ref refs/heads` command (without `--contains HEAD`) runs in p90 ~75ms for 100 branches (verified). The 5-minute in-memory cache eliminates subprocess cost for rapid re-queries (e.g., as the user types a filter into the autocomplete input).

Using `refs/heads` only (not `refs/remotes`) is intentional for Phase 1: session creation creates a new worktree branch based on the selected branch, so remote-tracking refs without a local branch would require an additional `git checkout -b` step. `include_remote: true` is reserved for Phase 2 when that workflow is supported.

---

## Consequences

**Positive:**
- `useBranchSuggestions` hook shows actual repo branches, not session-derived fallbacks.
- `AutocompleteInput` requires no changes — the hook feeds it the same `suggestions: string[]` interface.
- The RPC is independently cacheable and testable.
- Adding remote branch support later requires only a proto field change (already reserved as field 2).

**Negative / Accepted costs:**
- New proto RPC to maintain.
- Shell exec requires `context.WithTimeout` and `repoPath` sanitization (must be absolute path within known workspace directories — see Known Issues in main plan).
- Partial results (`truncated: true`) are returned on timeout rather than an error, which means the user sees a subset of branches rather than an error message; this trade-off is intentional.

---

## Alternatives Not Chosen

**Option B (Extend path-completion)**: Rejected because it conflates two different domain operations, makes the response schema more complex, and would require client-side logic to distinguish branch results from directory results in the same dropdown.

**go-git library**: Evaluated and rejected for this feature. `go-git Repository.Branches()` runs 30–100ms vs. shell `git for-each-ref` at 5–50ms. The shell approach is faster, already handles submodule and worktree edge cases correctly, and avoids adding a new binary dependency.
