# Stack Research: review-queue-e2e-onboarding

## Playwright version & config

- `tests/e2e/package.json` declares `"@playwright/test": "^1.40.0"`; the resolved
  version in `tests/e2e/package-lock.json` (`node_modules/@playwright/test`) is
  **1.61.1**. `.click({ timeout })` and `.catch()` chaining on a `Locator` action
  and `page.getByRole()` are stable APIs present since well before 1.40, so no
  version-gating concerns.
- Config lives at `tests/e2e/playwright.config.ts` (NOT repo root — there's also
  an unrelated `web-app/playwright.config.ts`, not used for this spec).
  - `testDir: './'`, `timeout: 30000` (per-test), `expect.timeout: 5000`.
  - `fullyParallel: false` (confirmed, line 23) and `workers: 1` (line 29) — this
    matches the requirements doc's claim. Despite `workers: 1`, Playwright still
    gives each `test()` a fresh browser context (default `use.storageState`
    unset ⇒ empty `localStorage`) unless a project sets `storageState`, which is
    why the onboarding modal reappears per-test.
  - Relevant projects (lines 105–130): `chromium` (SwiftShader software WebGL)
    and `chromium-dom` (WebGL disabled, forces xterm.js DOM renderer). AC #2
    requires both to pass — neither project sets a `storageState` fixture, so
    both are equally exposed to the fresh-context/onboarding-modal problem and
    both need the fix applied (it's the same spec file run twice, so one code
    change covers both).

## Onboarding-dismissal pattern — exact source

`git show origin/main:tests/e2e/escalation-reasoning.spec.ts`, lines 16–28
(local `main` is 26 commits behind `origin/main` per `git status --short --branch`
showing `behind 26`; this file does not exist on local `main` at all — always
diff against `origin/main` for this project):

```ts
      await page.goto(`${BASE_URL}/review-queue`, { waitUntil: 'domcontentloaded', timeout: 15000 });
      await page.waitForSelector('[data-testid="review-queue-loaded"]', { timeout: 10000, state: 'attached' });

      // A fresh browser context (no prior localStorage) shows the first-run
      // onboarding modal on top of the page — it can appear a moment after
      // navigation (not synchronously), so wait for it rather than a single
      // isVisible() check, and dismiss it so it doesn't intercept clicks on
      // the review-queue card/buttons below. Timeout is short: on a context
      // that has already seen onboarding, the modal never appears at all.
      await page
        .getByRole('button', { name: 'Skip onboarding' })
        .click({ timeout: 5000 })
        .catch(() => {});
```

Key detail: in this file the dismissal block comes **after** the initial
`waitForSelector('[data-testid="review-queue-loaded"]')`, not before — i.e. it
waits for the shell to attach first, then races the modal dismissal against
the rest of the assertions. For `review-queue.spec.ts`, the requirements doc
places dismissal "after `page.goto(...)` and before the first
assertion/`waitForSelector`" — apply it right after each `page.goto()`, before
any `waitForSelector`/`expect`, since `review-queue.spec.ts`'s own
`waitForSelector('[data-testid="review-queue"]')` calls are exactly the ones
timing out today (the modal, not the page shell, is what's not present yet
when those calls race it).

## `review-queue.spec.ts` structure (origin/main)

`git show origin/main:tests/e2e/review-queue.spec.ts` — 18 tests total across
2 `test.describe` blocks (`Review Queue Smoke Tests`, `Session Creation Flow
(UI Only)`, plus presumably more describes further in the file beyond the
first 80 lines read). Every test calls `page.goto(...)` directly (no shared
setup) and several assert without any onboarding dismissal:

```ts
test('review queue page loads successfully', async ({ page }) => {
  await page.goto(`${BASE_URL}/review-queue`);
  await page.waitForSelector('[data-testid="review-queue"]', { timeout: 5000 });
  ...
});
```

```ts
test('session creation wizard has all steps', async ({ page }) => {
  await page.goto(`${BASE_URL}/sessions/new`);
  await expect(page.locator('[data-testid="wizard-step-label"]', ...)).toBeVisible();
  ...
});
```

Confirms AC #5: the two `/sessions/new` tests (`session creation wizard has
all steps`, `session creation form has required test IDs`) also `goto()` +
assert directly with no dismissal step, same failure mode.

`BASE_URL` pattern is identical to `escalation-reasoning.spec.ts`:
`process.env.TEST_SERVER_URL || 'http://localhost:8544'`.

## `tests/e2e/pages/` helper convention

Inspected `SessionsPage.ts` and `BacklogPage.ts` (10 files total in the dir:
`BacklogItemDetailPage.ts`, `BacklogMutations.ts`, `BacklogPage.ts`,
`BacklogQueuePage.ts`, `BacklogSourcesSettingsPage.ts`,
`PipelineModesSettingsPage.ts`, `SessionDetailPage.ts`, `SessionsPage.ts`,
`SettingsPage.ts`, `ShellTabsPage.ts`, `StuckItemsPage.ts`, `VcsWidgetPage.ts`).

Convention (from `SessionsPage.ts`):
- One `export class FooPage` per file, named `<Domain>Page`.
- Constructor takes `(page: Page)` and stores `readonly page: Page` plus
  pre-built `Locator` fields.
- Action methods are `async` and operate via `this.page`.
- Imports: `import { Page, Locator } from '@playwright/test';`
- Consuming specs import the class and instantiate it: e.g.
  `import { SessionsPage } from './pages/SessionsPage';` then
  `const sessionsPage = new SessionsPage(page);`.

`tests/e2e/helpers/` (2 files: `session-client.ts`, `test-server.ts`) holds
plain exported functions/classes not tied to a specific page object (e.g.
`SessionClient` for hitting the ConnectRPC API directly) — used for backend
setup, not UI page interaction.

## Where a shared `dismissOnboarding(page)` helper belongs

Neither existing convention is a perfect fit for a single one-off action that
isn't page-object state:
- `tests/e2e/pages/` is for stateful Page Object classes with locators/fields,
  one class per domain page — a bare dismissal function doesn't need
  constructor state.
- `tests/e2e/helpers/` currently holds only non-Playwright-Page-centric
  helpers (RPC client, server bootstrap), but nothing prevents adding a
  Playwright-`Page`-centric helper function there — no rule mandates
  page-object-class-only in `pages/`.

Given AC #4's own phrasing ("If extracted to a shared helper, reuse across
review-queue.spec.ts and escalation-reasoning.spec.ts per tests/e2e/pages/
convention") and that this is a single plain async function (not a page
object with locators), the two reasonable options are:
1. A new small file `tests/e2e/helpers/onboarding.ts` exporting
   `export async function dismissOnboarding(page: Page): Promise<void>`,
   imported by both specs.
2. Inline the dismissal as a local helper function or duplicated 4-line block
   directly in `review-queue.spec.ts` (matching escalation-reasoning.spec.ts's
   inline block) if extraction is judged unnecessary for a 1-reasoning-effort
   fix — AC #4 makes extraction conditional ("if extracted"), not mandatory.

No existing `dismissOnboarding`/`Skip onboarding` helper currently exists
anywhere in `tests/e2e/pages/*.ts` or `tests/e2e/helpers/*.ts` (confirmed via
grep — zero matches outside `escalation-reasoning.spec.ts`).
