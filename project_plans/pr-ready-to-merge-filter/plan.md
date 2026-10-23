# Implementation Plan: PR Ready-to-Merge Filter

## Overview

Pure frontend change. Add a `"ready-to-merge"` chip to the PR filter bar on the Up Next page. A PR qualifies when: all checks passed, no merge conflicts (explicitly `hasMergeConflict === false`), not a draft, no changes requested.

## Touchpoints (5 files)

### 1. `web-app/src/lib/hooks/usePRListFilters.ts`

Add `"ready-to-merge"` to the `FilterStatus` union:
```ts
export type FilterStatus =
  | "all"
  | "ci-failing"
  | "changes-requested"
  | "with-session"
  | "draft"
  | "needs-attention"
  | "ready-to-merge";   // NEW
```

Add a case in `applyFilter`:
```ts
case "ready-to-merge":
  result = result.filter(
    (p) =>
      !p.isDraft &&
      p.checkConclusion === "success" &&
      p.hasMergeConflict === false &&   // explicitly false; undefined = unknown, excluded
      p.changesReqCount === 0
  );
  break;
```

### 2. `web-app/src/components/unfinished/GitHubPRsSection.tsx`

Add chip entry to `STATUS_FILTERS`:
```ts
const STATUS_FILTERS: { value: FilterStatus; label: string }[] = [
  { value: "all", label: "All" },
  { value: "ready-to-merge", label: "Ready to merge" },  // NEW — after All
  { value: "ci-failing", label: "CI failing" },
  { value: "changes-requested", label: "Changes req" },
  { value: "with-session", label: "Has session" },
  { value: "draft", label: "Draft" },
  { value: "needs-attention", label: "Needs attention" },
];
```

Optionally add a "ready to merge" count to `StatsBar`:
```ts
const readyToMerge = prs.filter(
  (p) => !p.isDraft && p.checkConclusion === "success" && p.hasMergeConflict === false && p.changesReqCount === 0
).length;
```

### 3. `web-app/src/lib/hooks/usePRListFilters.test.ts` (new file)

Test the new filter case with fixtures covering:
- `hasMergeConflict === false` + `checkConclusion === "success"` + not draft + 0 changesReq → included
- `hasMergeConflict === undefined` → excluded
- `hasMergeConflict === true` → excluded
- `checkConclusion === "failure"` → excluded
- `isDraft === true` → excluded
- `changesReqCount > 0` → excluded

### 4. `web-app/src/components/unfinished/GitHubPRsSection.test.tsx` (or `.test.ts`)

Verify the "Ready to merge" chip renders with `data-testid="filter-chip-ready-to-merge"` (matches the existing `filter-chip-${f.value}` pattern).

### 5. `web-app/src/components/unfinished/prs/prTestFixtures.ts`

Add a fixture PR with `hasMergeConflict: false`, `checkConclusion: "success"`, `isDraft: false`, `changesReqCount: 0` if none exists.

## Sequencing

1. `usePRListFilters.ts` — type + logic (compilable, all tests green after step 3)
2. `GitHubPRsSection.tsx` — chip + optional stats bar count
3. Test files
4. Verify `make quick-check` passes

## No backend changes needed.

## Adversarial Review Notes

- **hasMergeConflict=undefined treated as "no conflict"** → GUARDED. The filter explicitly requires `=== false`, not `!p.hasMergeConflict`, so undefined excludes the PR.
- **Filter chip count vs list count mismatch** → NOT a risk here: no count is shown on the chip itself (consistent with existing chips).
- **StatsBar count**: If we add a "ready to merge" count to the stats bar, use the same predicate as the filter, not a loose approximation. Extract a shared helper if both the filter and stats bar use it.
- **TypeScript exhaustiveness**: `FilterStatus` is a union used in a `switch` with no `default` in `applyFilter`. Adding a new value without a `case` compiles fine today (switch falls through to `return result`), but it would silently act as "all". The new `case` must be added together with the type change.
