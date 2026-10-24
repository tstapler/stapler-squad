# Validation: PR Ready-to-Merge Filter

## Acceptance Criteria Check

| # | Criterion | How to verify |
|---|---|---|
| 1 | "Ready to merge" chip appears in the filter bar | `data-testid="filter-chip-ready-to-merge"` exists in the rendered DOM |
| 2 | Clicking it shows only PRs with `checkConclusion === "success"` | Unit test in `usePRListFilters.test.ts` |
| 3 | PRs with `hasMergeConflict === true` are excluded | Unit test |
| 4 | PRs with `hasMergeConflict === undefined` (unknown) are excluded | Unit test — critical to prevent false positives |
| 5 | Draft PRs are excluded | Unit test |
| 6 | PRs with `changesReqCount > 0` are excluded | Unit test |
| 7 | "All" filter chip still shows everything | Existing tests pass |
| 8 | TypeScript compiles without errors | `make build` or `cd web-app && bunx tsc --noEmit` |
| 9 | Jest tests pass | `cd web-app && bunx jest --no-coverage --testPathPatterns="usePRListFilters"` |

## Pre-mortem / Failure Modes

1. **Filter chip appears but shows 0 PRs always** — most likely `hasMergeConflict` is undefined for all PRs (details not yet loaded). Log `pr.hasMergeConflict` for a few PRs to confirm. If so, the filter is correct; add a tooltip or note explaining it requires detail data to be loaded.

2. **Filter chip appears but includes conflicted PRs** — logic bug: `!p.hasMergeConflict` (falsy) instead of `p.hasMergeConflict === false` (strict). Tests cover this.

3. **Chip label doesn't match existing visual style** — compare against other chips; "Ready to merge" is slightly longer than "Needs attention", so verify it doesn't wrap or overflow.

4. **StatsBar count diverges from filtered list count** — if stats bar is updated, extract a shared `isReadyToMerge(pr: UserPR)` predicate used by both to guarantee consistency.

## Test Plan

```bash
cd web-app

# New unit tests
bunx jest --no-coverage --testPathPatterns="usePRListFilters"

# Existing filter/PR tests still pass
bunx jest --no-coverage --testPathPatterns="GitHubPRsSection|PRGroupedList|prAttention"

# Full suite (no-coverage for speed)
bunx jest --no-coverage
```

## Not Tested Here

- E2E test coverage is out of scope for this item (no e2e test for filter chips today; adding one would go in `tests/e2e/up-next-tabs.spec.ts` per `@feature unfinished-github-prs`)
- StatsBar ready-to-merge count is optional; if added, include a snapshot or render test
