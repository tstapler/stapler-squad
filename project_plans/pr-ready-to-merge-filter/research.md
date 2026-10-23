# Research: PR Ready-to-Merge Filter

## Relevant Code Inventory

### Filter system (`usePRListFilters.ts`)
- `FilterStatus` union: `"all" | "ci-failing" | "changes-requested" | "with-session" | "draft" | "needs-attention"`
- `applyFilter(prs, filter, search)` — switch on `filter`, returns filtered `UserPR[]`
- `STATUS_FILTERS` in `GitHubPRsSection.tsx` maps `FilterStatus` values to labels shown as chips

### UserPR proto fields (all server-populated today)
| Field | Type | Notes |
|---|---|---|
| `check_conclusion` | string | "success"/"failure"/"pending"/"" |
| `has_merge_conflict` | optional bool | nil = GitHub mergeable UNKNOWN |
| `changes_req_count` | int32 | 0 = none |
| `is_draft` | bool | |
| `approved_count` | int32 | not required by the request |

### Merge conflict signal
`has_merge_conflict` maps GitHub's lazily-computed `MERGEABLE`/`CONFLICTING`/`UNKNOWN` enum to a tri-state:
- `true` → conflicting
- `false` → mergeable
- `nil/undefined` → unknown (GitHub hasn't computed it yet or returned UNKNOWN)

**Critical:** `hasMergeConflict === undefined` must NOT count as "no conflict" — including unknown PRs in the ready-to-merge filter would be a false positive. Only `hasMergeConflict === false` (explicit "mergeable" from GitHub) qualifies.

### Existing precedent for mergeability in the codebase
- `mergeability.ts` → `deriveMergeabilityState` for the VCS widget (backlog item view) checks both local conflict files AND `github.mergeable === "conflicting"` to yield `"conflicted"` or `"diverged"`
- `prAttention.ts` already reads `hasMergeConflict` to compute `mergeConflict` and `conflictUnknown` — the same logic applies for the new filter
- The Up Next PR section already shows a `MergeConflict` chip on PRCard via `prAttention`

### Stats bar
`StatsBar` currently counts `ciFailures` and `needsReview`. A "ready to merge" count would be a natural addition, but is optional.

## No Backend Changes Required
All fields needed for the filter are already populated in `UserPR` by `github/user_pr_cache.go`'s widened query path. The filter is purely frontend logic.

## Existing Tests
- `usePRListFilters.ts` has no dedicated test file found (filter logic is currently tested implicitly via component tests)
- A new `usePRListFilters.test.ts` should be added to test the new filter case directly
- `prAttention.test.ts` tests the mergeConflict field path — same input field

## Risks
1. **hasMergeConflict=undefined is common**: GitHub computes mergeability lazily. If `details_loaded` is false, `has_merge_conflict` will be undefined, so the filter may show 0 results until detail data loads. This is correct behavior but needs a UX note (count could be 0 until fully loaded).
2. **No approvals check**: The request doesn't mention required approvals. If the user expects approved PRs in "ready to merge", `approved_count >= 1` might be debated. Leaving it out aligns with the literal request.
