# Adversarial Review: bun-build-7a-baseline-regressions

**Reviewer**: Adversarial architecture review  
**Date**: 2026-10-22  
**Plan file**: `implementation/plan.md`  
**Prior review**: `implementation/architecture-review.md`  
**Verdict**: 1 BLOCKER, 2 CONCERNS, 2 MINORS  
**Re-review verdict (2026-10-22)**: Adversarial re-review: CLEAN — all blockers resolved

---

## Summary

R1, R2, and R4 are straightforward. The only deep hazard is in R3: the plan and the prior architecture review land on a solution (`"use client"; export { NotFound as default }`) that is internally contradictory — it satisfies the hooks requirement but silently breaks the title requirement. The `not found (Next baseline)` block runs on **both** builds without a guard. If the fixture's `title` field cannot be made to match what both builds return, the test fails on one of them after R3.

---

## BLOCKER 1 — R3: `"use client"` and `metadata` export are mutually exclusive; the plan's one-line re-export cannot produce the expected title on Next (Story 3.1.1, Story 3.1.2) — **RESOLVED**

**Re-review (2026-10-22)**: Plan now implements the two-file pattern exactly as required. Story 3.1.1 (lines 214–253 of plan.md) adds `"use client";` to `NotFound.tsx` (Client Component, owns hooks) and creates `not-found.tsx` as a Server Component wrapper that exports `metadata: { title: NOT_FOUND_META.title }` and re-exports `NotFound as default`. This correctly separates the hook-boundary from the metadata boundary. The summary table (plan.md lines 403–408) also reflects both files. Blocker is resolved.

### What the plan says

Story 3.1.1 proposes creating `not-found.tsx` as a one-line re-export:
```tsx
export { NotFound as default } from "./NotFound";
```
The prior architecture review correctly noted that `NotFound.tsx` calls `usePathname()` and `useRouter()`, so `"use client"` must be added. It proposed:
```tsx
"use client";
export { NotFound as default } from "./NotFound";
```

Story 3.1.2 then expects the measured Next build title to be `"Page not found - Stapler Squad"` (from `NOT_FOUND_META.title`).

### Why this is wrong

**Next.js App Router constraint (confirmed in codebase)**: A `"use client"` file cannot export `metadata`. Evidence:
- `web-app/src/app/settings/pipeline-modes/page.tsx` lines 16–20 say explicitly: "`export const metadata` only applies to server components — this page is a client component… so metadata is intentionally omitted here rather than silently ignored. The document `<title>` is set by the nearest server-rendered layout instead."
- Every `metadata` export in `src/app/` is in a Server Component (layout.tsx, fanout/layout.tsx, etc.); all `"use client"` page files omit metadata.

**Title path for each build**:
- **Bun build**: `AppLayout` in `web-app/src/lib/nav/entry.tsx` line 72 renders `<title>{spec.titleFor(pathname)}</title>` on every route change; `titleForPath('/nonexistent')` returns `NOT_FOUND_META.title` = `"Page not found - Stapler Squad"`. This is a React 19 client-side title hoist that the Next build does not use.
- **Next build without `metadata` export**: The root `layout.tsx` sets `metadata.title = "Stapler Squad Sessions"`. No component in the `not-found.tsx` render tree overrides this. `measureNotFound` calls `page.goto` (hard load) then `page.title()` — the HTML `<title>` element from the root layout is `"Stapler Squad Sessions"`.

**Test impact**: The `not found (Next baseline)` describe block in `not-found.spec.ts` line 17 has **no `isBunBuild()` guard** and runs on both builds. `expect(observed).toEqual(baseline.notFound)` compares the full `{ status, finalPath, title }` object. If the fixture's `title` field is set to `"Page not found - Stapler Squad"` (matching Bun), the Next build test fails. If set to `"Stapler Squad Sessions"` (matching Next-without-metadata), the Bun build test fails. There is no value that satisfies both with the plan's proposed `not-found.tsx` content.

### Required fix

The correct solution requires **two** file changes, not one:

1. Add `"use client";` as the first line of **`web-app/src/app/NotFound.tsx`** (the existing file). This is one line; nothing else about `NotFound.tsx` changes. The Bun build is unaffected — react-router renders everything client-side so the directive is a no-op there.

2. Create **`web-app/src/app/not-found.tsx`** as a Server Component that exports `metadata`:
```tsx
import type { Metadata } from "next";
import { NOT_FOUND_META } from "./route-meta";

export const metadata: Metadata = { title: NOT_FOUND_META.title };
export { NotFound as default } from "./NotFound";
```
With `"use client"` on `NotFound.tsx`, Next's bundler places it at the client boundary; the Server Component wrapper hosts the metadata and passes rendering to the client subtree.

**Story 3.1.1 impact**: Adds a task — add `"use client";` to `NotFound.tsx` — and replaces the one-line `not-found.tsx` snippet with the four-line Server Component version above.

**Verification gate**: `make build` alone will NOT catch this — a `"use client"; export...` re-export also compiles without error. Only running the `not found (Next baseline)` tests on the Next build AND on the Bun build (or reading `page.title()` in a debugger) will confirm the title converges.

---

## CONCERN 1 (carried from architecture review, classification upgrade) — Story 3.1.1 snippet is wrong as written

The architecture review classified "missing `use client`" as CONCERN because Task 2's `make build` would catch it. Given the analysis above, the correct `"use client"` placement is on `NotFound.tsx`, not on `not-found.tsx`. The architecture review's suggested fix (`"use client"; export { NotFound as default }`) silently keeps the title wrong even though it compiles. Reclassify as BLOCKER-level (merged into BLOCKER 1 above).

---

## CONCERN 2 (carried from architecture review) — `measureNotFound` 5-second latency spike on Next build after R3 (Story 3.1.2)

`BaselineProbes.ts` line 18:
```ts
await page.waitForURL((url) => url.pathname !== missing, { timeout: 5000 }).catch(() => undefined);
```
After R3 the Next build stays at `/nonexistent` (no redirect). The predicate never fires; the 5-second timeout always elapses. `measureNotFound` is called twice in the `not found (Next baseline)` block, adding ~10 seconds to every Next-build run of `not-found.spec.ts`. (The Bun build already has this latency; R3 brings Next into the same position.)

The plan's Story 3.1.2 does not include a task to remove or shorten this call. The architecture review's CONCERN 2 notes this can be a follow-on; that is acceptable. However, Story 3.1.2 Task 1 ("capture the exact output of `measureNotFound`") should note the 5-second wait as a known artifact so the implementer is not surprised by the slow test run.

**Impact**: Not a correctness problem; tests pass. CI run time for `not-found.spec.ts` increases by ~10 seconds. Acceptable as a follow-on, but should be tracked.

---

## MINOR 1 — `not-found.tsx` metadata title is not validated by the drift test

`route-meta.test.ts` line 19 scans only files named `page.tsx`. `not-found.tsx` is excluded from the title drift check. If `not-found.tsx`'s `metadata.title` later diverges from `NOT_FOUND_META.title` (e.g., someone edits the string directly), `route-meta.test.ts` will not catch it.

The architecture review correctly noted (NITPICK) that no `ROUTE_META` row is needed. This is correct. But no mechanism validates that `not-found.tsx`'s metadata and `NOT_FOUND_META.title` stay in sync.

**Low impact**: `NOT_FOUND_META.title` is already pinned by `route-meta.test.ts` line 125–127 (`'exports the not-found title as data'`). As long as `not-found.tsx` imports `NOT_FOUND_META` rather than inlining a string literal, the pin is effective.

The proposed `not-found.tsx` content above imports `NOT_FOUND_META`, so this minor resolves itself with the BLOCKER 1 fix. No additional action needed.

---

## MINOR 2 — Story 3.1.1 scope statement is wrong ("one new file")

The plan summary table says R3 changes "app/not-found.tsx (new)" only. After applying BLOCKER 1's fix, the file set also includes a one-line edit to `web-app/src/app/NotFound.tsx`. The task list in Story 3.1.1 needs a new task: "(XS) Add `"use client";` as the first line of `web-app/src/app/NotFound.tsx`."

**Low impact**: The scope change is one line in an existing file. The PR diff grows by one line. The summary table and task list need updating before implementation begins.

---

## Scope, Technology Bets, Architecture Risks (No New Issues)

- **R1/R2**: Guard injection at `test.describe` level before `test.beforeEach` exactly matches the existing Bun-only block convention. Pattern is well-established. No concerns.
- **R4**: SCROLLER replacement preserves all call sites (`scrollRange`, `scrollTop`, `scrollTo`). The testid-preference short-circuit fires only when the element exists AND `r > 0`; falls through to the scan otherwise. No concerns.
- **R5**: Explicitly out of scope; `test.fail()` left in place. Correct.
- **Technology bets**: None. All patterns (`isBunBuild()`, `data-testid`, `"use client"`, metadata export) are established in this codebase.
- **Scope drift**: None found. The plan is tightly bounded to the four regressions.

---

## Findings Table

| # | Classification | Story | Finding | Required Action |
|---|---|---|---|---|
| 1 | **BLOCKER** | 3.1.1, 3.1.2 | `"use client"` (for hooks) and `metadata` export (for title) are mutually exclusive in Next App Router. Plan's one-line re-export cannot produce `title: "Page not found - Stapler Squad"` on the Next build. `not found (Next baseline)` block runs on both builds; fixture title cannot satisfy both builds simultaneously without the metadata fix. | Add `"use client";` to `NotFound.tsx`; make `not-found.tsx` a Server Component with `metadata: { title: NOT_FOUND_META.title }` export |
| 2 | **CONCERN** | 3.1.2 | (From architecture review) `measureNotFound`'s 5s `waitForURL` always times out after R3 on both builds (~10s extra per `not-found.spec.ts` run) | Add follow-on task; annotate Story 3.1.2 Task 1 so the implementer expects the latency |
| 3 | MINOR | 3.1.1 | Story scope statement says "one new file"; correct fix also modifies `NotFound.tsx` | Add task to Story 3.1.1; update summary table |
| 4 | MINOR | 3.1.1 | `not-found.tsx` metadata is not validated by the drift test, but self-resolves if the file imports `NOT_FOUND_META` rather than inlining a string | Use `NOT_FOUND_META` import (confirmed by BLOCKER 1 fix) |
