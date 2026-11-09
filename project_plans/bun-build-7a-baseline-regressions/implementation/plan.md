# Implementation Plan: bun-build-7a-baseline-regressions

**Feature**: Fix four regressions blocking PR 7b's green Bun-only baseline after PRs 4/5a/6a merged
**Date**: 2026-10-22
**Status**: Ready for implementation
**ADRs**: ADR-001-not-found-unification.md

---

## Creative Pass — Rejected Approaches

Three approaches were evaluated before settling on the plan below:

| Approach | Strength | Weakness |
|---|---|---|
| A. Minimal guard injection only (skip every Next baseline block when isBunBuild()) | Zero risk of touching production code; purely additive | Does not fix R3 (Next now shows NotFound, fixture is wrong) or R4 (shared scroll regression pre-exists) |
| B. Full unification now: create not-found.tsx, fix scroll container, guard tests, update fixture | Lands everything in one PR; PR 7b can open immediately after | R3 requires owner confirmation that the new not-found behavior is intentional before the fixture is updated |
| **C. Chosen: Guard + confirm + fix (sequential gates)** | Guards (R1/R2) and the scroll id (R4) are mechanical and unambiguous; R3 decision gates the fixture update but not the others | Slightly more coordination surface than approach A |

Chosen: **Approach C**. R1 and R2 are unambiguously wrong (Bun running Next assertions); R4 is a pre-existing shared failure with a concrete fix. R3 requires explicit owner sign-off encoded as a decision gate in Phase 3 before changing the fixture.

---

## Domain Glossary

| Term | Definition | Notes |
|---|---|---|
| Next baseline block | A `test.describe` block in an e2e spec that asserts the recorded behavior of the Next.js build, not the Bun build | Must be skipped when running against a Bun build |
| Bun baseline block | A `test.describe` block in an e2e spec that asserts the decided Bun policy | Carries `test.skip(!isBunBuild())` |
| `isBunBuild()` | Helper (`tests/e2e/helpers/web-build.ts`) that returns true when the test run is targeting the Bun/react-router build | Used as the guard predicate |
| next-baseline.json | Fixture file (`tests/e2e/fixtures/next-baseline.json`) recording measured Next build behavior | Four keys: `redirects`, `skipLink`, `historyNavAriaCurrent`, `notFound`, `scrollPolicy` |
| bun-expected-diff.json | Fixture file (`tests/e2e/baseline/bun-expected-diff.json`) listing tests that fail identically on both builds | R4 is already listed here under issue #1136 |
| RouteAnnouncer | Custom component (`web-app/src/app/RouteAnnouncer.tsx`) that uses `role=status` | Next's own `__next-route-announcer__` uses `role=alert` |
| FOUC prevention | Inline boot script that writes the stored theme class to `<html>` before first paint | Located in Next layout; PR 6a may have affected inlining |
| SCROLLER | Inline JS expression in `AppShellPage.ts` that finds the element with the largest scrollable overflow range | Breaks when a tab switch changes which panel is the largest scroller |
| Up Next container | The outer `<div className={styles.container}>` in `UnfinishedTab.tsx` | Has `overflowY: "auto"` in its CSS; is the correct scroll container to fix for R4 |
| not-found.tsx | Next.js App Router's special filename for a 404 page | Does not currently exist; `NotFound.tsx` exists as the component but has no App Router entry point |
| test.fail() | Playwright API that marks a test as expected-to-fail | R5 is documented here; changing it is out of scope for this item |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|---|---|---|---|---|
| Test build guard | `test.skip(isBunBuild())` on the Next baseline `describe` block | Existing pattern in Bun baseline blocks | Per-test `test.skip` inside the describe | Describe-level guard skips all tests in the block with one line; matches the Bun blocks' own convention |
| Not-found App Router entry | `not-found.tsx` as a one-line re-export of `NotFound` | Next.js App Router convention | Duplicating the JSX | Zero logic duplication; NotFound.tsx already exists and is tested by Bun block |
| Scroll container identification | `data-testid="up-next-scroller"` attribute on the outer container `<div>` in UnfinishedTab.tsx | CLAUDE.md e2e-test-conventions (data-testid or ARIA roles for locators) | Adding a DOM `id` attribute | data-testid is the project convention; id would also work but is not preferred |
| SCROLLER algorithm update | Prefer the element with `data-testid="up-next-scroller"` when found, before falling back to the largest-overflow scan | AppShellPage.ts inline expression | Rewriting SCROLLER to accept a selector parameter | Adding a testid-preference short-circuit in the existing inline expression is the smallest safe change |

---

## Tech Debt Disposition

| Area | Existing Issue | Disposition | Justification |
|---|---|---|---|
| R4 (tab-switch scroll) | Pre-existing in bun-expected-diff.json as issue #1136 | Fix in this item (stable testid on container) | The fix is one line; the bun-expected-diff.json entry and SCROLLER comment can be cleaned up here since the root cause is resolved |
| R5 (Bun POP scroll restore) | `test.fail()` in scroll-policy.spec.ts | Leave as `test.fail()` — out of scope | Needs ScrollPolicy.tsx investigation, separate issue |
| Missing `not-found.tsx` | NotFound.tsx exists but has no Next App Router entry point | Create in this item | One-line re-export; required for Next 404 routing and for the not-found.spec.ts baseline update |

---

## Effort Estimate (price-weighted, ESTIMATION.md format)

All work below executes as agent tasks. No human-weeks.

| Phase | Stories | Agent waves | Dominant cost class | Wall-clock blockers |
|---|---|---|---|---|
| Phase 1 (R1/R2 guards) | 1.1.1, 1.1.2 | 1 wave (2 files, 1-line edits each) | Output: 2 small edits | None |
| Phase 2 (verify Next) | 2.1.1 | 1 wave (run command, read output) | Input: test output | None — but if tests fail, Phase 3 is blocked |
| Phase 3 (R3 not-found) | 3.1.1, 3.1.2 | 2 waves (file creation + test run) | Output: 2 new/modified files + JSON fixture | **Owner decision gate** — must confirm R3 is intentional before first wave starts. If owner does not confirm, Phase 3 is skipped entirely; Phases 1, 2, 4 land independently |
| Phase 4 (R4 scroll) | 4.1.1, 4.1.2 | 2 waves (source + test edits + verification) | Output: 2 file edits + test run | None |
| Phase 5 (CI gate) | 5.1 | 1 wave (run make ci) | Input: CI output | None |

**R3 decision gate — explicit fallback**: If the owner does not confirm within the PR review window (before the PR lands), Phase 3 is skipped and the fixture stays at the current (wrong) value. The item must be re-opened. Phase 3 CANNOT be implemented speculatively.

---

## Migration Plan

No schema or data migrations. All changes are in test files and web-app source.

---

## Observability Plan

- The only observable signals are the Playwright e2e test results.
- After each phase, run `cd tests/e2e && bun run test` (or `make test` for the Jest unit suite if touching web-app source).
- No new metrics, logs, or feature flags are required.

---

## Risk Control

- **Feature flag**: None required. All changes are either test-only (R1/R2) or add a `data-testid` attribute (R4) that does not affect rendered output.
- **Rollback procedure**: Each change is a single-file edit or a one-line new file. `git revert` per commit is sufficient.
- **Staged rollout**: Not applicable. Changes land in one PR (7b's cleanup step).

---

## Unresolved Questions

- **R3 owner decision**: Is it intentional that an unknown Next path now shows a NotFound view instead of redirecting to `/`? This is the only question that gates work. If yes, create `not-found.tsx` and update `next-baseline.json`. If no, investigate what changed. The plan assumes "yes" based on the research (build-vs-buy recommends creating it), but Phase 3 is explicitly sequenced after the owner confirms.

---

## Dependency Visualization

```
Phase 1 (R1/R2 guards)  ──────────────────────────────────────────────────┐
                                                                            │
Phase 2 (confirm guards pass on Next; no code change expected)  ───────────┤
                                                                            │
Phase 3 (R3: owner confirms intent → not-found.tsx + fixture update)  ─────┤
                                                                            │
Phase 4 (R4: testid on Up Next container + SCROLLER update)  ──────────────┘
                                                                            │
                                                              PR 7b opens  ─┘

R5 (test.fail) stays in place; no dependency.
```

---

## Phase 1: Test Guard Fixes (R1 and R2)

### Epic 1.1: Add `isBunBuild()` Guards to Next Baseline Describe Blocks

**Goal**: Prevent the Next baseline blocks in `route-focus-and-announce.spec.ts` and `theme-fouc.spec.ts` from running against the Bun build, where Next-specific behavior (role=alert, next/font classes) does not exist.

**Reference pattern** (from the existing Bun-only block in the same file):
```ts
test.describe('route focus and announcements (Bun only)', () => {
  test.skip(!isBunBuild(), "Next's own announcer (role=alert) is asserted by the Next-baseline block above");
  ...
```

The mirror for the Next baseline block must call `test.skip(isBunBuild(), ...)`.

---

#### Story 1.1.1: Guard `route-focus-and-announce.spec.ts` Next baseline describe block

**Size**: XS

**Files**: `tests/e2e/route-focus-and-announce.spec.ts`

**Change**: Inside `test.describe('route focus and announcements (Next baseline)', ...)`, add as the first line:
```ts
test.skip(isBunBuild(), "Next's __next-route-announcer__ (role=alert) is not present in the Bun build; see the Bun-only block below");
```

**Acceptance Criteria**:

- Given: the e2e suite runs against a Bun build
- When: the `route focus and announcements (Next baseline)` describe block is reached
- Then: all three tests in it are skipped, and the Bun-only block runs normally

- Given: the e2e suite runs against the Next build
- When: the `route focus and announcements (Next baseline)` describe block is reached
- Then: all three tests run and the first test (`clicking the History nav link...`) finds exactly one live region with `role="alert"` containing the History page title

**Tasks**:

1. (XS) Add `test.skip(isBunBuild(), "...")` as first line of the `test.describe('route focus and announcements (Next baseline)', ...)` callback body in `tests/e2e/route-focus-and-announce.spec.ts` (line 19's callback, before `test.beforeEach`).

---

#### Story 1.1.2: Guard `theme-fouc.spec.ts` Next baseline describe block

**Size**: XS

**Files**: `tests/e2e/theme-fouc.spec.ts`

**Change**: Inside `test.describe('theme FOUC prevention (Next baseline)', ...)` (line 56), add as the first line of the callback:
```ts
test.skip(isBunBuild(), "next/font variable classes are not present in the Bun build; the Bun-only block below asserts --font-* custom properties instead");
```

**Acceptance Criteria**:

- Given: the e2e suite runs against a Bun build
- When: the `theme FOUC prevention (Next baseline)` describe block is reached
- Then: all four tests in it are skipped, and the Bun-only block (`theme FOUC prevention (Bun only, UX G10)`) runs normally

- Given: the e2e suite runs against the Next build
- When: the `font variable classes survive every theme` test runs
- Then: `html` carries the same CSS class count across all themes (set size = 1), and `shared.length >= 4` holds

**Tasks**:

1. (XS) Add `test.skip(isBunBuild(), "...")` as first line of the `test.describe('theme FOUC prevention (Next baseline)', ...)` callback body in `tests/e2e/theme-fouc.spec.ts` (line 56's callback, before the first `test`).

---

## Phase 2: Verify Guards Pass (No Code Change)

### Epic 2.1: Confirm Next Build Is Still Green After Guards

**Goal**: Ensure the guard addition in Phase 1 did not accidentally break the Next build behavior. This is a verification phase with no code changes.

**Size**: XS

#### Story 2.1.1: Run the affected spec files on the Next build

**Files**: None (verification only)

**Acceptance Criteria**:

- Given: the Next build is running at the test server port
- When: `bun run test -- route-focus-and-announce.spec.ts theme-fouc.spec.ts` runs
- Then: all non-skipped tests pass (the previously-failing assertions about role=alert and font classes now pass because they only run on Next)

**Tasks**:

1. (XS) Run against the Next build by setting `WEB_BUILD=next` (check `docs/how-to/build-web-app.md` for the correct flag name — the Makefile and `web-app/build/dev.ts` both reference it). If using `make install-service`, run `WEB_BUILD=next make build` first so the test server serves Next build artifacts. Then run `cd tests/e2e && bun run test -- route-focus-and-announce.spec.ts theme-fouc.spec.ts` and confirm green output. If any test fails, root-cause and fix before proceeding to Phase 3.

---

## Phase 3: Not-Found Unification (R3) — Owner Decision Gate

### Epic 3.1: Create `app/not-found.tsx` and Update Baseline Fixture

**Goal**: Add the Next App Router not-found entry point so an unknown path renders the `NotFound` component instead of falling through to `/`. Update `next-baseline.json` to reflect the new observed behavior.

**Decision gate**: This phase starts only after the owner confirms the new behavior is intentional. The confirmation can be given in a PR review comment or inline in the task description before implementation begins.

---

#### Story 3.1.1: Wire `NotFound` into Next App Router (two file changes)

**Size**: XS

**Files**:
- `web-app/src/app/NotFound.tsx` (modify existing — add `"use client"` directive)
- `web-app/src/app/not-found.tsx` (new file — Server Component wrapper)

**Change**:

`"use client"` and a `metadata` export are mutually exclusive in the same file in Next App Router. The solution is a two-layer pattern: `NotFound.tsx` is the Client Component (it owns the hooks), and `not-found.tsx` is a Server Component wrapper that owns `metadata` and re-exports `NotFound` as the default.

**File 1 — modify `web-app/src/app/NotFound.tsx`**: Add `"use client";` as the first line (currently missing; required because the component uses `usePathname` and `useRouter`):
```tsx
"use client";
// ... rest of file unchanged
```

**File 2 — create `web-app/src/app/not-found.tsx`**: Server Component with no `"use client"` directive. It exports `metadata` (the page title) and re-exports `NotFound` as the default. Next App Router allows Server Components to re-export Client Components as default exports, so the runtime renders the Server Component wrapper to pick up `metadata` and then hydrates `NotFound.tsx` as the Client Component:
```tsx
import type { Metadata } from "next";
import { NOT_FOUND_META } from "./route-meta";
export const metadata: Metadata = { title: NOT_FOUND_META.title };
export { NotFound as default } from "./NotFound";
```

**Acceptance Criteria**:

- Given: Next.js App Router is running
- When: a request arrives for `/nonexistent`
- Then: the `NotFound` component renders (heading "Page not found", path text, "Go to Sessions" link), the HTTP status is 200 (Next SPA behavior, no server-side 404), and the URL stays at `/nonexistent`

- Given: both files are in place
- When: `make build` runs
- Then: the build completes without errors (no TypeScript errors, no "cannot export metadata from a client component" build error)

**Tasks**:

0. (XS) Pre-check: verify `NOT_FOUND_META` is already exported from `web-app/src/app/route-meta.ts` (search for `NOT_FOUND_META`). If not present, add it. Also check whether `route-meta.test.ts` enforces a row for `not-found.tsx` — if the test only scans `page.tsx` files (per the CLAUDE.md nav-facade rule which states "route-meta.test.ts only checks page.tsx files, not not-found.tsx"), no additional row is needed. Document the result of this check as a comment in not-found.tsx: `// NOT_FOUND_META from route-meta.ts; route-meta.test.ts does not enforce rows for not-found.tsx`.
1. (XS) Add `"use client";` as the first line of `web-app/src/app/NotFound.tsx`.
2. (XS) Create `web-app/src/app/not-found.tsx` with the Server Component wrapper shown above (imports `Metadata` from `"next"`, imports `NOT_FOUND_META` from `"./route-meta"`, exports `metadata`, re-exports `NotFound as default` from `"./NotFound"`).
3. (XS) Run `make build` to confirm no compilation errors.

---

#### Story 3.1.2: Measure the new Next not-found behavior and update `next-baseline.json`

**Size**: S

**Files**: `tests/e2e/fixtures/next-baseline.json`

**Context**: The `not-found.spec.ts` test `'an unknown path answers 200 and ends on the recorded page'` compares `measureNotFound(page)` against `baseline.notFound`. After Story 3.1.1, `measureNotFound` will observe:
- `status: 200` (unchanged)
- `finalPath: "/nonexistent"` (stays at the original path, no redirect to `/`)
- `title: "Page not found - Stapler Squad"` (from `NOT_FOUND_META.title` in route-meta.ts)

**Acceptance Criteria**:

- Given: the Next build with `not-found.tsx` is running
- When: `measureNotFound(page)` is called for `/nonexistent`
- Then: the function returns `{ status: 200, finalPath: "/nonexistent", title: "Page not found - Stapler Squad" }`

- Given: `next-baseline.json`'s `notFound` key is updated to the measured values
- When: the `not found (Next baseline)` tests run on the Next build
- Then: both tests pass (`'an unknown path answers 200 and ends on the recorded page'` and `'the nav still works from the page an unknown path lands on'`)

**Tasks**:

1. (S) Run the `not-found.spec.ts` Next baseline tests against the updated build and capture the exact output of `measureNotFound`. Confirm the three fields listed above.
2. (XS) Update `tests/e2e/fixtures/next-baseline.json` — replace the `notFound` key:
   ```json
   "notFound": {
     "status": 200,
     "finalPath": "/nonexistent",
     "title": "Page not found - Stapler Squad"
   }
   ```
3. (XS) Update `BaselineProbes.ts`'s `measureNotFound`: shorten or remove the `waitForURL` timeout. After R3 the URL stays at `/nonexistent` (no redirect), so `page.waitForURL((url) => url.pathname !== missing, { timeout: 5000 })` always times out before the `.catch()` fires — adding ~5s latency per call and ~10s to every not-found.spec.ts run. Change the timeout from `5000` to `500`ms (or add a comment that the timeout is expected to elapse silently when `next-baseline.json` records `finalPath !== "/"`).
4. (XS) Run `cd tests/e2e && bun run test -- not-found.spec.ts` on the Next build and confirm both baseline tests pass.

---

## Phase 4: Scroll Container Stability (R4)

### Epic 4.1: Add Stable `data-testid` to Up Next Scroll Container

**Goal**: The `SCROLLER` expression in `AppShellPage.ts` picks the element with the largest scrollable overflow. When a tab switch in Up Next changes which panel is tallest, the measured container changes identity, and `scrollTop()` returns 0. Adding a stable `data-testid` to the outer `UnfinishedTab` container lets SCROLLER prefer it over the measurement scan, so `scrollTop()` stays consistent across tab switches.

**Note on R5**: The `test.fail()` for Bun POP scroll restore in `scroll-policy.spec.ts` is not touched here. That failure requires a ScrollPolicy.tsx fix and is tracked separately.

---

#### Story 4.1.1: Add `data-testid="up-next-scroller"` to `UnfinishedTab.tsx`

**Size**: XS

**Files**: `web-app/src/app/unfinished/UnfinishedTab.tsx`

**Change**: Add `data-testid="up-next-scroller"` to the outer container `<div>`:
```tsx
// Before:
<div className={styles.container}>
// After:
<div className={styles.container} data-testid="up-next-scroller">
```

**Acceptance Criteria**:

- Given: the /unfinished page renders
- When: `document.querySelector('[data-testid="up-next-scroller"]')` is called
- Then: the element is found, and it is the same element returned by `scrollTop()` before and after a tab switch

**Tasks**:

1. (XS) Add `data-testid="up-next-scroller"` to the outer `<div className={styles.container}>` in `web-app/src/app/unfinished/UnfinishedTab.tsx` (line 213, the `return` block's outermost div).

---

#### Story 4.1.2: Update `SCROLLER` in `AppShellPage.ts` to prefer the testid element

**Size**: S

**Files**: `tests/e2e/pages/AppShellPage.ts`

**Context**: The current `SCROLLER` expression (`lines 86-97`) scans all elements and picks the one with the largest `scrollHeight - clientHeight`. It needs to be updated to short-circuit when `[data-testid="up-next-scroller"]` is present and has a non-trivial scrollable range, rather than competing with tab panel content.

**Change**: Replace the `SCROLLER` constant with a version that checks for the testid element first:

```ts
private static readonly SCROLLER = `(() => {
  const pinned = document.querySelector('[data-testid="up-next-scroller"]');
  if (pinned) {
    const overflow = getComputedStyle(pinned).overflowY;
    const r = pinned.scrollHeight - pinned.clientHeight;
    if (overflow === 'auto' || overflow === 'scroll') return { el: pinned, range: r };
  }
  let best = null;
  let range = document.scrollingElement ? document.scrollingElement.scrollHeight - innerHeight : 0;
  for (const el of document.querySelectorAll('*')) {
    if (el.closest('[data-testid=tray-scroll],[data-testid=notification-tray]')) continue;
    const overflow = getComputedStyle(el).overflowY;
    if (overflow !== 'auto' && overflow !== 'scroll') continue;
    const r = el.scrollHeight - el.clientHeight;
    if (r > range) { range = r; best = el; }
  }
  return { el: best, range };
})()`;
```

**Acceptance Criteria**:

- Given: the /unfinished page has scrolled to `SCROLL_OFFSET` px on the `prs` tab
- When: `upNext.selectTab('queue')` is called (changing the visible panel)
- Then: `shell.scrollTop()` returns `SCROLL_OFFSET` (within the scroll tolerance), not 0

- Given: the /unfinished page is NOT loaded (e.g., /history is loaded)
- When: `shell.scrollTop()` is called
- Then: the SCROLLER falls back to the largest-overflow scan as before (testid element not found)

**Tasks**:

1. (S) Replace `SCROLLER` in `tests/e2e/pages/AppShellPage.ts` with the testid-preferred version shown above, **with one correction to the code snippet**: remove `&& r > 0` from the testid-preference guard, so the element is returned as soon as it is found with `overflow: auto/scroll`, regardless of its current scroll range:
   ```js
   if (overflow === 'auto' || overflow === 'scroll') return { el: pinned, range: r };
   ```
   When `[data-testid='up-next-scroller']` is found AND has `overflow: auto/scroll`, return it immediately — even if `range` is 0 (not yet scrollable). This preserves the identity contract that callers rely on: `scrollTop()` always refers to the same element when on `/unfinished`, regardless of load state. Add a comment above the constant documenting the expected return shape `{ el: Element | null, range: number }` so future readers can verify that both the testid-preference path and the fallback scan return the same shape.
2. (S) **P1 guard** — Run the **full** `scroll-policy.spec.ts` suite on the Next build (`cd tests/e2e && bun run test -- scroll-policy.spec.ts`) and confirm ALL tests that were green before pass. Do not only check the R4 test — a logic error in the SCROLLER IIFE would break `scrollTop`, `scrollRange`, and `scrollTo` callers across the entire file. Also run the full suite on the Bun build. Both must be green (or showing only the expected `test.fail()` for R5) before proceeding to Task 3.
3. (XS) Remove the `bun-expected-diff.json` entry for `scroll-policy.spec.ts / switching the Up Next tab leaves scroll and the history length unchanged` **only after** both Task 2 builds (Next and Bun) confirm the test passes. If the test still fails on Bun after the SCROLLER fix, keep the entry and open a separate issue for the Bun-side root cause.

---

## Final Verification

### Epic 5.1: Green CI Gate Before Opening PR 7b

**Size**: S

**Files**: None (verification)

**Tasks**:

1. (S) Run `make ci` (or `make quick-check` for a faster local pass) and confirm it exits 0.
2. (XS) Run the full e2e suite against the Next build: `cd tests/e2e && bun run test`. Confirm:
   - R1 and R2: Next baseline tests pass; Bun-only blocks are skipped on Next (and vice versa).
   - R3: Both `not found (Next baseline)` tests pass.
   - R4: `switching the Up Next tab leaves scroll and the history length unchanged` passes.
   - R5: `scroll policy (Bun only, UX G8)` still shows `test.fail()` (unchanged).
3. (XS) Confirm `bun-expected-diff.json` no longer lists the R4 test entry.

---

## Summary Table

| Regression | Root Cause | Fix | Files Changed | Size |
|---|---|---|---|---|
| R1 — route announcer | Next baseline block lacks isBunBuild() guard | Add `test.skip(isBunBuild(), ...)` | route-focus-and-announce.spec.ts | XS |
| R2 — theme FOUC fonts | Next baseline block lacks isBunBuild() guard | Add `test.skip(isBunBuild(), ...)` | theme-fouc.spec.ts | XS |
| R3 — not-found baseline | No `not-found.tsx` App Router entry; baseline fixture stale | Add `"use client"` to NotFound.tsx; create not-found.tsx Server Component wrapper + update fixture | app/NotFound.tsx (modify), app/not-found.tsx (new), next-baseline.json | S |
| R4 — tab-switch scroll | SCROLLER picks wrong container after tab switch | Add data-testid to container; update SCROLLER | UnfinishedTab.tsx, AppShellPage.ts | S |
| R5 — Bun POP restore | ScrollPolicy.tsx sibling-index path breaks after re-render | Out of scope — stays as test.fail() | none | — |
