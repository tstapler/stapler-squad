# Pre-Mortem: bun-build-7a-baseline-regressions

**Date**: 2026-10-22  
**Plan file**: `implementation/plan.md`  
**Scenario**: This project has shipped and failed — either tests still fail after the fix, new regressions were introduced, or PR 7b could not open. What went wrong?

---

## Failure Mode 1 — SCROLLER inline JS expression has a logic error that breaks all scroll tests

**Severity**: P1 — likely AND catastrophic

**Failure**: The new SCROLLER IIFE in `AppShellPage.ts` has a subtle defect — either the `r > 0` guard causes fallback to the scan when the unfinished tab has no loaded content yet (empty state, or content not yet rendered), or the `return { el: pinned, range: r }` short-circuit silently returns a different element type than the original `{ el: best, range }` path expected by the three callers (`scrollRange`, `scrollTop`, `scrollTo`) — breaking every scroll-related e2e test across both builds, not just the R4 test.

**First symptom**: `scroll-policy.spec.ts` starts failing with confusing "scrollTop returned undefined" or "cannot read properties of null" errors on tests unrelated to the tab-switch scenario — specifically the tests that were green before R4's change.

**Prevention**: In plan.md Story 4.1.2 Task 1, add an explicit verification step: after replacing SCROLLER, run the full `scroll-policy.spec.ts` suite (not just the R4 test) on both builds before removing the `bun-expected-diff.json` entry. Add a comment above the SCROLLER constant in `AppShellPage.ts` documenting the expected return shape `{ el: Element | null, range: number }` so future readers can verify consistency. Cross-check the `r > 0` guard against the case where the testid element is in the DOM but has no scrollable content — the fallback should still return the testid element with `range: 0` rather than discarding it.

---

## Failure Mode 2 — Phase 2 verification is skipped, hiding a real Next routing regression under the isBunBuild() guard

**Severity**: P2 — likely but recoverable

**Failure**: An implementer adds the `isBunBuild()` guards to the R1 and R2 Next baseline `describe` blocks (Phase 1), assumes the guards fix the failures, and moves on to Phase 3 without running Phase 2 against the Next build. The underlying root cause — that PRs 4/5a/6a changed `RootShell.tsx`'s nav links so Next's client-side router no longer fires on nav-link click — is still present on the Next build. The guard prevents Bun from seeing the failure, but the Next build tests are now also broken (just silently, because Phase 2 was never run). PR 7b opens green on Bun and immediately fails CI on Next.

**First symptom**: The first CI run after PR 7b opens shows `route-focus-and-announce.spec.ts (Next baseline)` failing on the Next build with "found 0 elements matching filter" — the same original symptom, now on Next.

**Prevention**: In plan.md Epic 2.1, make Story 2.1.1 a hard gate rather than a "confirm" step: add an explicit line "If any test fails in this step, do NOT proceed to Phase 3 — the guard is masking a Next regression, not fixing it. Root-cause the Next build failure before proceeding." Separately, add a validation.md requirement that Phase 2 must produce a recorded test output (grep for "X passed") before Phase 3 work begins.

---

## Failure Mode 3 — "nav still works" test fails after R3 because NotFound renders without AppShell navigation

**Severity**: P2 — likely but recoverable

**Failure**: `not-found.tsx` renders `NotFound.tsx` directly as a default export. `NotFound.tsx` is a standalone component that renders its own "Page not found" heading and a "Go to Sessions" link, but does not include or depend on the `AppShell` wrapper that provides the drawer nav links. The `not found (Next baseline)` test `'the nav still works from the page an unknown path lands on'` clicks a nav link (e.g., History) from the not-found landing page and expects navigation to succeed. If that nav link is provided by AppShell and AppShell is not mounted when Next renders the 404 via `not-found.tsx`, the test finds no nav link and fails.

**First symptom**: `not-found.spec.ts` baseline test "the nav still works from the page an unknown path lands on" fails with "element not found" for the nav link locator immediately after Story 3.1.1 is applied.

**Prevention**: In plan.md Story 3.1.1, add a pre-implementation check: read `web-app/src/app/NotFound.tsx` and confirm that it either (a) renders within the root `layout.tsx` shell (which provides nav through AppShell) or (b) includes its own nav mechanism. If `layout.tsx` wraps `{children}` with AppShell and Next's not-found.tsx participates in the layout hierarchy, the nav is available; document this chain explicitly. Add an acceptance criterion to Story 3.1.2: "The nav link locator used in the 'nav still works' test is visible in a Playwright snapshot of `/nonexistent` on the Next build."

---

## Failure Mode 4 — bun-expected-diff.json R4 entry removed before the fix is confirmed on both builds

**Severity**: P2 — likely but recoverable

**Failure**: Story 4.1.2 Task 3 removes the `bun-expected-diff.json` entry for the R4 test ("switching the Up Next tab leaves scroll and the history length unchanged"). The plan confirms this test on the Next build (Task 2), but `bun-expected-diff.json` exists specifically for tests that fail on the Bun build. The SCROLLER testid-preference fix resolves the failure on Next, but the Bun build's scroll container might still reset `scrollTop` to 0 after a tab switch for a different reason (Bun/react-router remounts the tab panel with a different key, resetting state the Next build doesn't reset). Removing the entry before confirming on Bun causes CI to fail on the Bun build after the entry is gone.

**First symptom**: The Bun build CI run after the entry removal shows `scroll-policy.spec.ts / switching the Up Next tab leaves scroll and the history length unchanged` failing — the same test that the entry was guarding.

**Prevention**: In plan.md Story 4.1.2 Task 3, change the removal to be conditional: "Remove the `bun-expected-diff.json` entry only after running `bun run test -- scroll-policy.spec.ts` against **both** the Next build (Task 2 above) **and** the Bun build. If the test still fails on Bun after the SCROLLER fix, the entry must stay and a separate issue opened for the Bun-side root cause."

---

## Failure Mode 5 — BaselineProbes.ts 5s timeout not shortened, causing not-found.spec.ts to become a CI time sink

**Severity**: P3 — unlikely but catastrophic

**Failure**: Story 3.1.2 Task 3 (shorten `measureNotFound`'s `waitForURL` timeout from 5000ms to 500ms) is treated as optional based on the adversarial review's "acceptable as follow-on" framing and is deferred indefinitely. After R3, `measureNotFound` is called twice per `not-found.spec.ts` run; each call always waits the full 5s timeout before the `.catch(() => undefined)` fires. The suite now takes ~10s extra per run. Over time, additional `not-found` probes are added (e.g., for Bun variants), compounding the slowdown. When CI is under load, `not-found.spec.ts`'s wall-clock time exceeds the per-spec timeout, and the test is spuriously marked as a timeout failure rather than a latency issue — making it appear flaky rather than slow.

**First symptom**: `not-found.spec.ts` starts intermittently timing out in CI (not in local runs where the test server is faster) with "Test timeout of Xms exceeded", while the actual assertion values are correct on re-run.

**Prevention**: In plan.md Story 3.1.2, promote Task 3 from an optional annotation step to a required task: "Shorten `waitForURL` timeout from 5000 to 500ms before landing. This is not a follow-on — every additional ms here multiplies by every `measureNotFound` call site, and the adversarial review already flagged it as a known latency spike. Fix it in the same commit as the fixture update." Add a comment above the call explaining that the predicate is intentionally expected to time out (the URL never changes after R3), so future readers don't "fix" it by raising the timeout.

---

## Summary

| # | Failure Mode | Severity | Top Prevention Change |
|---|---|---|---|
| 1 | SCROLLER logic error breaks all scroll tests | **P1** | Add full-suite scroll verification step; document return shape |
| 2 | Phase 2 skipped; isBunBuild() guard hides real Next regression | P2 | Make Phase 2 a hard gate with required recorded output |
| 3 | "nav still works" fails — NotFound renders without AppShell nav | P2 | Add pre-implementation check that layout.tsx wraps not-found route |
| 4 | bun-expected-diff.json entry removed before Bun build confirmed | P2 | Require Bun build confirmation before entry removal |
| 5 | 5s waitForURL timeout never shortened; test becomes spuriously flaky | P3 | Promote Story 3.1.2 Task 3 from optional to required |

**Top failure mode**: The SCROLLER IIFE replacement (R4) is a single complex inline JS expression whose defect would silently break every scroll-related e2e test across both builds, not just the one regression it targets — making it the highest-consequence change in the plan relative to its apparent size.
