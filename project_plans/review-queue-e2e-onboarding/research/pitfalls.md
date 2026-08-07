# Pitfalls Research: review-queue-e2e-onboarding

Scope: known pitfalls specific to porting the onboarding-dismissal pattern from
`tests/e2e/escalation-reasoning.spec.ts` (origin/main) into
`tests/e2e/review-queue.spec.ts`. All line numbers below are from the current
repo state; local `main` is 26 commits behind origin/main and lacks
`escalation-reasoning.spec.ts`, so that file was read via `git show
origin/main:tests/e2e/escalation-reasoning.spec.ts`.

## 1. Does `.catch(() => {})` risk masking a future regression?

Yes, and it's a real (not theoretical) risk given how `OnboardingModal.tsx` is built.

The skip button's accessible name is a plain string literal, not derived from a
constant shared with the test:

```tsx
// web-app/src/components/onboarding/OnboardingModal.tsx:206-212
<button
  className={styles.skipButton}
  onClick={handleSkip}
  aria-label="Skip onboarding"
>
  Skip
</button>
```

Nothing ties this `aria-label` to the test's `getByRole('button', { name: 'Skip
onboarding' })` selector — a future rename (e.g. "Skip tutorial", "Dismiss",
i18n) compiles fine and breaks the test silently:

1. `.click({ timeout: 5000 }).catch(() => {})` times out after 5s, error discarded.
2. The modal is still open, physically covering the review-queue content.
3. The *next* line's assertion fails — but with a generic timeout/occlusion
   error on `review-item-*` or `review-queue-loaded`, not anything mentioning
   "Skip onboarding" or the aria-label. A future debugger re-lives this exact
   triage from scratch, because the actual point of failure (the swallowed
   click) left no trace.

**More robust alternative found in this repo already**: `tests/e2e/global-setup.ts`
rewrites Playwright `storageState` fixture files (`fixtures/*.json`, e.g.
`matrix-theme.json`) with `{ origins: [{ origin, localStorage: [{ name:
'stapler-theme', value: 'matrix' }] }] }` to pre-seed localStorage before any
test in that project runs. The exact same technique — seeding
`localStorage['stapler-squad:onboarded'] = 'true'` (the key is exported as
`ONBOARDED_KEY` from `web-app/src/components/onboarding/useOnboarding.ts:3`)
either via a `storageState` fixture or `page.addInitScript(() =>
localStorage.setItem('stapler-squad:onboarded', 'true'))` before `page.goto()`
— would make the modal never mount at all, since `useOnboarding`'s effect
checks `localStorage.getItem(ONBOARDED_KEY)` synchronously before arming the
800ms timer (see §2). This eliminates the race and the swallowed-error surface
entirely, at the cost of not exercising the modal's "already onboarded" code
path (which arguably nothing here needs to exercise).

This diverges from the "same pattern as escalation-reasoning.spec.ts" instruction in
requirements.md AC1, so it should be flagged to the planning phase as an option,
not adopted silently — but it's worth recording since it removes pitfall #1 by
construction rather than mitigating it.

## 2. Ordering pitfalls — delayed mount and race timing

Confirmed: the modal mount is delayed, not synchronous:

```ts
// web-app/src/components/onboarding/useOnboarding.ts:7-16
useEffect(() => {
  let timerId: ReturnType<typeof setTimeout>;
  try {
    if (!localStorage.getItem(ONBOARDED_KEY)) {
      timerId = setTimeout(() => setShow(true), 800);
    }
  } catch { /* ignore storage errors */ }
  return () => clearTimeout(timerId);
}, []);
```

The modal appears **~800ms after mount**, not on first paint. Implications:

- Playwright locator actions (`.click()`, `.isVisible()`, etc.) auto-poll for
  the full timeout given, so `.click({ timeout: 5000 })` placed immediately
  after `page.goto()` is safe — 5000ms comfortably covers the 800ms delay with
  margin, including CI slowness (the 800ms `setTimeout` is a macrotask and can
  itself be pushed later if the main thread is busy hydrating/rendering).
- **The real ordering hazard is where the dismissal sits relative to the
  *first* `waitForSelector`/assertion, not relative to `goto`.** In
  `escalation-reasoning.spec.ts` the first wait is deliberately non-blocking on
  the modal:

  ```ts
  await page.waitForSelector('[data-testid="review-queue-loaded"]', { timeout: 10000, state: 'attached' });
  // ... comment: modal "can appear a moment after navigation ... dismiss it
  // so it doesn't intercept clicks on the review-queue card/buttons below"
  await page.getByRole('button', { name: 'Skip onboarding' }).click({ timeout: 5000 }).catch(() => {});
  ```

  It uses `state: 'attached'` (DOM presence only, ignores stacking/overlay),
  so that first wait can never be blocked by the overlay — the dismissal
  click can safely come *after* it. Most `waitForSelector` calls in the
  current `review-queue.spec.ts` (e.g. line 25, 45, 111 `state` unspecified
  → defaults to `'visible'`) don't have this hedge. Per requirements.md's
  confirmed root cause, these are exactly the calls that hang/timeout with
  the modal open. **Conclusion for the fix: the dismissal step must be
  inserted directly after every `page.goto()` and strictly before *any*
  subsequent `waitForSelector`/`expect(...).toBeVisible()` call in that test**
  — not merely "before the first `review-queue-loaded` wait" as the
  requirements phrasing might suggest, since some tests (e.g. current line
  32-40 "review queue badge is visible") never call `waitForSelector` at all
  and go straight to `expect(badge).toBeVisible()`, which is exactly the kind
  of assertion the modal can block.
- **Delayed-mount race after an initial failed click attempt**: not a risk
  here because there is only one dismissal attempt per test (not a
  click-then-recheck loop), and the single `.click({timeout: 5000})` already
  polls internally across the whole 800ms mount window — there's no scenario
  in this pattern where an earlier click "misses" and a later one is needed.
  This would only become a hazard if someone shortened the timeout below
  ~800ms+CI-jitter margin.

## 3. `retries: 1` masking + smarter fix (shared hook vs. per-test)

- `retries: 1` + `workers: 1` means Playwright reports only the *final*
  outcome per test across up to 2 attempts. The bug as described is
  deterministic (fresh context every test, modal always renders on first
  run), which is why it survives retries and shows up as 10/18 failing even
  in CI — so retries are not currently masking *this* bug. But going forward,
  if the fix is applied inconsistently (one test's dismissal omitted, or its
  timeout shortened), that specific test could flip between pass/fail
  depending on scheduler/CI-load timing relative to the 800ms mount delay —
  and `retries: 1` would then hide that flakiness in normal CI runs (only
  surfacing under `--workers=1`, single project, no-retry triage, exactly
  the conditions used to originally diagnose this bug). Worth calling out in
  the plan: verify with `--retries=0` locally before trusting a green run.
- **A file-wide `test.beforeEach` cannot fully replace per-test insertion
  here**: `beforeEach` runs before any `page.goto()`, but this spec file
  navigates to two different URLs (`/review-queue` in most tests,
  `/sessions/new` in the two "Session Creation Flow" tests at lines 58-95).
  A single `beforeEach` can't own "goto + dismiss" for both URLs without
  hardcoding one URL or adding conditional logic, which reintroduces the
  duplication/fragility it's meant to remove.
- **The right level of de-duplication, matching AC4 and this repo's existing
  convention**: `tests/e2e/pages/` already holds ~11 Page Object classes
  (`SessionsPage.ts`, `BacklogPage.ts`, etc.) with a `goto()`-style method
  pattern (see `SessionsPage.ts:18-21`). The fix should add a small shared
  helper — e.g. `dismissOnboardingIfPresent(page)` exported from a new
  `tests/e2e/pages/OnboardingHelper.ts` (or added as a shared function/module,
  not necessarily a full class) — called explicitly as the first line after
  each test's own `page.goto()` in both `review-queue.spec.ts` and
  `escalation-reasoning.spec.ts`. This removes the literal
  copy-pasted 3-line block (single point of truth for the button name/timeout)
  while still respecting that each test controls its own navigation target —
  which a bare `beforeEach` cannot do here.

## 4. `workers: 1` / `fullyParallel: false` + `retries: 1` interaction risk

- No cross-test localStorage leakage: per requirements.md, each `test()` gets
  a fresh browser context regardless of serial execution, so one test's
  dismissed-modal state doesn't carry into the next test even though they run
  sequentially in the same worker process.
- Retries also get a fresh context per attempt (Playwright's default retry
  behavior creates a new page/context), so a failed dismissal on attempt 1
  doesn't poison attempt 2.
- The one real shared-state hazard in this file is **unrelated to onboarding**
  but is adjacent: `tests/e2e/review-queue.spec.ts:142-178` ("acknowledge
  button removes item from DOM") mutates the shared test-server's review
  queue (removes an item), and other tests in the same file/run read queue
  contents (`review-queue-loaded` sentinel, item counts). Because
  `workers: 1`/`fullyParallel: false` runs tests in file order without
  isolation between them at the *server* level (only the browser context is
  isolated), test order matters for these assertions. This is pre-existing
  and orthogonal to the onboarding fix — flagging so it isn't
  misattributed to the onboarding-dismissal change if a downstream test's
  item-count assertion looks flaky after this fix lands.

## 5. Playwright footguns with the bare `.catch(() => {})` idiom, and safer alternatives

- `.catch(() => {})` on a Playwright locator action swallows **every** error
  from that action — not just "element never appeared" (the intended case),
  but also strict-mode violations (selector matches >1 element), unexpected
  navigations, and detached-element races. All of these look identical from
  the outside: silence. This is the general footgun behind pitfall #1.
- **Narrower alternative** — separate the "does it exist" check from the
  click so only the existence probe is allowed to fail silently:
  ```ts
  const skipButton = page.getByRole('button', { name: 'Skip onboarding' });
  if (await skipButton.isVisible({ timeout: 5000 }).catch(() => false)) {
    await skipButton.click();
  }
  ```
  A genuine click failure (e.g. the button *is* visible per the accessibility
  tree but not actionable because some other unrelated overlay covers it)
  then surfaces as a real, attributable test failure instead of being
  absorbed into the same swallow as "button doesn't exist."
- **`test.step()` wrapping** doesn't prevent the swallow but makes the empty
  catch's outcome visible in the HTML/trace report as a named step ("Dismiss
  onboarding modal if present") — cheap to add, meaningfully shortens a
  future re-triage of the exact failure mode in pitfall #1, and costs nothing
  in AC-compliance since it's still test-file-only and still "the same
  pattern" functionally.
- **Most robust option** (see §1): pre-seed `localStorage` via
  `addInitScript`/`storageState` before `goto()`, removing the click race
  (and its `.catch()`) entirely. Not what AC1 asks for verbatim, but worth
  surfacing to the planning phase as the option with the smallest long-term
  maintenance/regression-masking surface, using the repo's own
  `global-setup.ts` theme-fixture precedent as prior art.

## Summary of actionable takeaways for planning/implementation

1. Place the dismissal block immediately after **every** `page.goto()` in the
   file (all `/review-queue` tests and both `/sessions/new` tests per AC5),
   strictly before *any* `waitForSelector`/`expect(...).toBeVisible()` —
   including tests that skip `waitForSelector` and assert directly (line
   32-40 today).
2. Extract to `tests/e2e/pages/` (AC4) as a plain exported helper function
   (not necessarily a full Page Object class, given both call sites just need
   one call after their own `goto()`), reused by both `review-queue.spec.ts`
   and `escalation-reasoning.spec.ts`.
3. Consider narrowing the swallow to an `isVisible().catch(() => false)` guard
   + unguarded `click()`, and/or wrapping in `test.step()`, so a future
   aria-label drift fails loudly instead of reproducing this exact bug
   silently.
4. Verify locally with `--retries=0` (not just the default `retries: 1`) so a
   flaky partial fix doesn't get CI-masked the way the original bug nearly was.
5. No cross-test contamination risk from `workers: 1`/`retries: 1` specific to
   this fix; the pre-existing shared-queue-state coupling between
   "acknowledge" and other read tests is unrelated and out of scope.
