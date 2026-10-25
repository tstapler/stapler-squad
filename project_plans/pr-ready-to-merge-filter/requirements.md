# Requirements: PR Ready-to-Merge Filter

**Item ID:** e7952c01-3efa-446a-a6d7-b17bb1835ae7  
**Feature area:** Up Next page → GitHub PRs section

## Problem Statement

The Up Next page's PR section currently has filter chips for "CI failing", "Changes req", "Has session", "Draft", and "Needs attention", but no way to quickly find PRs that are ready to merge (all checks passing, no merge conflicts). Additionally, merge conflict status is not a first-class filter criterion, so users can't isolate PRs blocked on conflicts.

## Requested Changes

1. Add a "Ready to merge" filter chip to the GitHub PRs section that shows only PRs where:
   - All CI checks are passing (`checkConclusion === "success"`)
   - No merge conflicts (`hasMergeConflict === false`, not `undefined`/unknown)
   - Not a draft
   - No changes requested (`changesReqCount === 0`)

2. Ensure merge conflict status is considered: PRs with `hasMergeConflict === true` or `undefined` (unknown) must NOT appear in the "Ready to merge" filter.

## Scope

- `web-app/src/lib/hooks/usePRListFilters.ts` — add `"ready-to-merge"` to `FilterStatus` union, add case in `applyFilter`
- `web-app/src/components/unfinished/GitHubPRsSection.tsx` — add chip to `STATUS_FILTERS`
- Tests: `usePRListFilters` (or a sibling test file) for the new filter case
- The `StatsBar` may optionally show a "ready to merge" count; not required but worthwhile

## Out of Scope

- Backend changes (all required fields — `hasMergeConflict`, `checkConclusion`, `approvedCount`, `changesReqCount` — are already populated in `UserPR`)
- Changing the existing "needs-attention" or "ci-failing" filter semantics
- Approval count as a required criterion (not mentioned in the request; `approvedCount` would be a future enhancement)
