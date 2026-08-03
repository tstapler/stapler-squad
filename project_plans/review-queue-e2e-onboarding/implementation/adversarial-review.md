# Adversarial Review: review-queue-e2e-onboarding
**Date**: 2026-08-03
**Verdict**: CLEAN (re-reviewed after plan edits; see Re-review below)

## Re-review (2026-08-03, second pass)

Plan was edited after the original BLOCKED verdict. Scope of this pass:
re-verify the 2 original blockers and 2 original concerns only, plus a quick
sanity skim of the edit's blast radius. Full original findings are preserved
below for record; each is now annotated RESOLVED with the evidence checked.

- [x] **Blocker 1 — RESOLVED.** Task 1.1.1a (`plan.md:121-149`) no longer uses
  `isVisible({timeout})`. It now implements the bare
  `page.getByRole('button', { name: 'Skip onboarding' }).click({ timeout: 5000 }).catch(() => {})`
  pattern verbatim (`plan.md:140-145`), matching `escalation-reasoning.spec.ts`'s
  proven snippet exactly. `.click({timeout})` genuinely polls (unlike
  `.isVisible({timeout})`), so the modal's ~800ms mount delay is no longer a
  race. The doc comment above the function (`plan.md:132-139`) correctly
  explains why the click-and-catch form was chosen over the isVisible guard,
  citing the same deprecated-no-op reasoning this review originally raised.
- [x] **Blocker 2 — RESOLVED.** New Task 1.1.1e (`plan.md:180-200`) migrates
  `escalation-reasoning.spec.ts`'s existing inline click-and-catch block to
  call `dismissOnboardingIfPresent(page)`, with the import added at the top of
  the file. Story 1.1.1's **Files** line (`plan.md:118-119`) now correctly
  lists `tests/e2e/escalation-reasoning.spec.ts` alongside the new helper file
  and `review-queue.spec.ts`. AC4 ("reused, not duplicated, by both spec
  files") is now satisfied literally — one implementation, two call sites via
  the same imported function. The Pattern Decisions table row for
  `escalation-reasoning.spec.ts` (`plan.md:29`) also correctly cites this as
  the Blocker-2 fix rather than repeating the old "out of scope" rationale.
- [x] **Concern 1 — RESOLVED.** Since Task 1.1.1a now implements the literal
  bare click+catch snippet (not a structurally different `isVisible` guard),
  AC1's "same pattern as `escalation-reasoning.spec.ts`" is true byte-for-byte,
  not just "in effect" — the judgment call this concern flagged no longer
  exists; there's nothing left to unilaterally interpret.
- [x] **Concern 2 — RESOLVED.** New Task 1.1.2c (`plan.md:240-250`) runs
  `npx playwright test escalation-reasoning.spec.ts --reporter=list` and
  confirms the pass count matches the pre-migration baseline. It's wired into
  the dependency diagram (`plan.md:49-69`) as the terminal step after
  1.1.2a → 1.1.2b, and Story 1.1.2's acceptance criteria (`plan.md:218-220`)
  and goal statement (`plan.md:202-208`) both explicitly call out verifying
  `escalation-reasoning.spec.ts` post-migration.

### Sanity skim of the edit's blast radius

- Wiring tasks 1.1.1b/c/d (`plan.md:151-178`) all correctly reference
  `await dismissOnboardingIfPresent(page);` and rely on the single import
  added in 1.1.1b (`import { dismissOnboardingIfPresent } from
  './pages/OnboardingPage';`, `plan.md:152`) — consistent naming throughout,
  no leftover references to the old class-based `OnboardingPage.dismissIfPresent()`
  shape.
- The helper's class → plain-function change (driven by the separate
  architecture review) is consistent throughout the plan: the Pattern
  Decisions table (`plan.md:26`), Task 1.1.1a's implementation, and the
  Domain Glossary entry (`plan.md:17`) all agree on `export async function
  dismissOnboardingIfPresent(page: Page): Promise<void>`, no class, no
  leftover `Locator`/constructor references. Nothing reintroduced from the
  original class-shaped version.
- Dependency diagram (`plan.md:49-69`) correctly shows 1.1.1b/c/d/e as four
  parallel branches off 1.1.1a, converging into 1.1.2a → 1.1.2b → 1.1.2c —
  matches the task list; no orphaned or missing nodes.
- Stale minor fixed: the original Minors note below ("Task 1.1.1a actually
  specifies a multi-field class") no longer applies now that 1.1.1a is a
  plain function — removed from Minors, not carried forward as a new finding.

**No new blockers or concerns found.** Verdict updated BLOCKED → CLEAN.

---

## Original Review (2026-08-03, first pass) — preserved for record

### Blockers (both resolved above)

- [x] **Task 1.1.1a's `isVisible({ timeout: 5000 })` does not wait — it is a
  deprecated no-op option, so `dismissIfPresent()` will almost always return
  `false` before the modal exists and the bug will NOT be fixed.** Playwright's
  own installed type definitions (`web-app/node_modules/.pnpm/playwright-core@1.57.0/node_modules/playwright-core/types/types.d.ts:13931-13938`)
  state explicitly:
  ```ts
  isVisible(options?: {
    /**
     * @deprecated This option is ignored.
     * [locator.isVisible([options])] does not wait for
     * the element to become visible and returns immediately.
     */
    timeout?: number;
  }): Promise<boolean>;
  ```
  Unlike `.click()`/`.waitFor()`, `Locator.isVisible()` checks the current DOM
  **synchronously** and returns immediately — it never polls. The modal mounts
  ~800ms after page load (`web-app/src/components/onboarding/useOnboarding.ts:7-16`,
  confirmed in `research/pitfalls.md` §2). Calling `dismissIfPresent()` as the
  first statement after `page.goto()` (as every wiring task 1.1.1b–d
  specifies) means `isVisible()` will evaluate before the modal exists in ~all
  cases, return `false`, skip the click, and the modal will still mount and
  intercept the subsequent assertion exactly as it does today. TypeScript will
  not catch this — `timeout` is a legal (if deprecated) property on the type,
  so the code compiles clean and looks correct on read, but is behaviorally a
  no-op. Recommendation: replace with `this.skipButton.waitFor({ state:
  'visible', timeout: 5000 }).then(() => true).catch(() => false)`, which
  actually polls, before merging Task 1.1.1a.
  - This also undercuts the Pattern Decision table's own stated rationale
    ("narrows the swallow to 'button never appeared'... lets a genuine click
    failure surface"): since `isVisible()` doesn't wait or throw for "not yet
    present," the `.catch(() => false)` is largely dead code for the case it
    claims to guard against, and the actual defect is a missing poll, not an
    over-broad catch. The plan's own justification for deviating from AC1's
    literal pattern is built on a mistaken premise.
  - Traced to source: this exact snippet (`isVisible({ timeout: 5000
    }).catch(() => false)`) was proposed verbatim in `research/pitfalls.md`
    §5 ("Narrower alternative") as the safer replacement for the bare
    click+catch — the research phase did not verify it against Playwright's
    actual API semantics before recommending it, and the plan copied it
    without independently checking. Fix the research artifact's example too
    if it's referenced again elsewhere.
  - Secondary risk: Task 1.1.2b's troubleshooting note ("re-check the exact
    test body where the `dismissIfPresent()` call is missing or misplaced")
    misdirects debugging toward call-site placement when the real defect is
    inside `dismissIfPresent()` itself — an implementer hitting 10/18 still
    failing after Story 1.1.1 would likely waste a cycle checking placement
    before finding this.

- [x] **Plan violates AC4 by explicitly declining to wire the new helper into
  `escalation-reasoning.spec.ts`.** AC4 reads: "If the dismissal logic is
  extracted into a shared helper ..., it is reused (not duplicated) by both
  `review-queue.spec.ts` and `escalation-reasoning.spec.ts`." The plan chooses
  to extract (`OnboardingPage.ts`), which triggers that clause, but the
  Pattern Decisions table states: "`escalation-reasoning.spec.ts` | Not
  modified in this task | requirements.md non-goals | Migrating it to
  `OnboardingPage` now | Out of scope ... flagged as a natural follow-up, not
  part of this fix." The non-goal cited (`requirements.md`: "Not adding
  onboarding-dismissal to unrelated spec files beyond review-queue.spec.ts")
  is about not *adding* dismissal to files that don't have it yet —
  `escalation-reasoning.spec.ts` already has its own dismissal block, so
  migrating that existing call to the shared helper is a DRY-up of an
  already-in-scope file, not scope expansion, and AC4's literal "reused ...
  by both" is not optional once extraction is chosen. As written, the plan
  leaves two divergent dismissal implementations in the codebase (bare
  click+catch in `escalation-reasoning.spec.ts` vs. the new — currently
  broken, see Blocker 1 — `isVisible` guard in `OnboardingPage.ts`), which is
  the "duplicated" outcome AC4 exists to prevent. Recommendation: add a task
  to migrate `escalation-reasoning.spec.ts`'s existing dismissal block to
  `OnboardingPage.dismissIfPresent()`, or explicitly get user sign-off to
  relax AC4 before implementation starts.

### Concerns (both resolved above)

- [x] **AC1's "same pattern" wording is resolved unilaterally inside the plan
  rather than escalated.** requirements.md AC1 says every test dismisses the
  modal "using the same ... pattern" as `escalation-reasoning.spec.ts` (bare
  `.click({timeout}).catch(()=>{})`). The plan's Pattern Decisions table
  swaps in a structurally different `isVisible` guard and asserts this
  "satisfies AC1 in effect, not byte-for-byte" — a judgment call that changes
  what a complexity-1 "quick task" was scoped to do. Given Blocker 1, "same in
  effect" isn't even true right now. Recommendation: once the `isVisible` bug
  is fixed, get explicit confirmation that "in effect" satisfies AC1's intent,
  or default to the literal bare click+catch snippet the requirements name.
- [x] **No verification step for `escalation-reasoning.spec.ts` if AC4 is
  properly honored.** If Blocker 2 is fixed by migrating that file to the
  shared helper, Story 1.1.2's verification (Tasks 1.1.2a/b) only re-runs
  `review-queue.spec.ts` — there's no equivalent "confirm
  escalation-reasoning.spec.ts still passes" step for the file that would now
  also depend on the new helper.

### Minors

- No non-standard technology choices in this plan — nothing to flag under
  "Technology bets."
- Verified **not** a gap: `npx playwright test review-queue.spec.ts` already
  runs against both `chromium` and `chromium-dom` projects by default —
  neither carries a `testMatch` restriction (only the four `visual-*`
  projects do, scoped to `visual-regression.spec.ts`) — so Task 1.1.2a's
  plain command does not need an explicit `--project` flag or a second
  invocation to cover `chromium-dom` (`tests/e2e/playwright.config.ts:66-133`).
