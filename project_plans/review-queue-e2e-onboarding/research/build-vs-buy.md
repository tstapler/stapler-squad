# Build vs. Buy — review-queue-e2e-onboarding

Scope note: this is not an OSS-vs-build decision. `@playwright/test` already provides
every primitive needed; the only question is which of a few small refactor shapes to use
for a ~5-line onboarding-dismissal snippet.

## 1. `addInitScript` localStorage seeding vs. click-and-catch race

`web-app/src/components/onboarding/useOnboarding.ts:3` defines the gate:
```ts
export const ONBOARDED_KEY = "stapler-squad:onboarded";
```
checked at `useOnboarding.ts:11` (`if (!localStorage.getItem(ONBOARDED_KEY))` → show modal) and
set at `OnboardingModal.tsx:152` (`localStorage.setItem(ONBOARDED_KEY, "true")`) on skip/complete.

Since the key is a plain, stable, documented string, `context.addInitScript(() =>
localStorage.setItem("stapler-squad:onboarded", "true"))` (or per-page `page.addInitScript`)
run before `page.goto()` prevents the modal from ever mounting — no modal render, no click,
no race, no `.catch(() => {})` swallow-and-hope. This is strictly more "bought": it's a
built-in Playwright API (`addInitScript`, stable since early versions), zero custom glue,
and removes an entire DOM interaction (and its ~5s timeout budget) from every test.

Trade-off: it diverges from the existing `escalation-reasoning.spec.ts` production pattern
(click-and-catch), so the two specs would exercise the onboarding gate two different ways.
It also doesn't verify the *dismissal UI* itself works — but `review-queue.spec.ts` isn't
testing onboarding, it's testing the review queue, so that's not a loss of coverage.

**Verdict:** `addInitScript` seeding is the better mechanism on pure engineering merits
(eliminates a UI race entirely rather than papering over it with a timeout+catch), but see
Q3 — the existing proven pattern is the pragmatic choice here given the task's stated
complexity (1) and its "reuse what escalation-reasoning.spec.ts already does" framing.

## 2. Where the dismissal logic lives: new page helper vs. inline duplication vs. `beforeEach`

**Option A — new `tests/e2e/pages/OnboardingPage.ts` helper**
- Pros: matches repo convention (`tests/e2e/pages/` already holds 11 page-object files,
  per `.claude/rules/e2e-test-conventions.md` §4 "New page helpers go in
  `tests/e2e/pages/`"); single source of truth reusable by *both*
  `review-queue.spec.ts` and `escalation-reasoning.spec.ts` (and any future spec) if the
  latter is ever updated to import it; easy to swap the underlying strategy (Q1) in one
  place later.
- Cons: adds a new file for one 5-line action; slight indirection for a trivial operation;
  `escalation-reasoning.spec.ts` isn't in scope for this fix so the cross-file reuse benefit
  is only realized if someone deliberately goes back and refactors it too (not required by
  this task).

**Option B — inline duplication in every test (or every `describe` block)**
- Pros: zero new abstraction, matches the literal shape of the existing
  `escalation-reasoning.spec.ts` snippet exactly, trivial to review line-by-line.
- Cons: `review-queue.spec.ts` has 8 `page.goto()` call sites (per `grep -c page.goto`) —
  duplicating the snippet 8× is real repetition for a single-file fix; any future change to
  the dismissal logic requires touching 8 spots.

**Option C — `test.beforeEach` in `review-queue.spec.ts`**
- Pros: zero duplication within the file (one hook covers all 8 sites); zero new files;
  smallest possible diff; still trivially readable — a reviewer sees the fix once at the
  top of the file instead of scattered 8×.
- Cons: does not help `escalation-reasoning.spec.ts` or any future spec — the pattern stays
  copy-pasted at the spec-file level rather than centralized. Also, some `describe` blocks
  in the file navigate to different routes (`/review-queue`, `/sessions/new`), so a single
  top-level `beforeEach` needs to run after each `page.goto()`, not before it — either move
  the dismissal call to run once per `describe` block, or place it as a helper function
  called right after each `goto()` (this nudges toward a small local function, functionally
  identical to Option A's helper but file-scoped).

**Verdict:** Option A (new `OnboardingPage.ts` helper) is the right call — it's the
convention this repo already established for exactly this kind of cross-cutting page
interaction (11 precedents in `tests/e2e/pages/`), costs one small file, and immediately
removes duplication across all 8 call sites in this spec while leaving the door open for
`escalation-reasoning.spec.ts` to adopt it later without a second migration. Option C is an
acceptable fallback only if the task's complexity budget can't absorb a new file, but given
`.claude/rules/e2e-test-conventions.md` explicitly directs new page helpers into
`tests/e2e/pages/`, Option A is the convention-compliant choice, not just the "nicer" one.

## 3. Fork the existing `escalation-reasoning.spec.ts` snippet verbatim vs. invent a new approach

The click-and-catch pattern is already proven in production (an existing, presumably
passing, spec on `origin/main`). Reusing it verbatim — rather than introducing the
`addInitScript` alternative from Q1 — minimizes risk and review surface for a
complexity-1 fix: no new mechanism to validate, consistent behavior across specs, and a
reviewer can visually diff against a known-working precedent.

**Verdict:** yes — fork the verbatim snippet into the new `OnboardingPage.ts` helper (Q2
Option A) rather than switching to `addInitScript` seeding. The `addInitScript` approach
(Q1) is worth a follow-up note/backlog item since it is objectively more robust (removes
the race and its timeout budget entirely), but swapping mechanisms is out of scope for a
fix whose entire job is "match the working pattern that already exists elsewhere in this
repo."
