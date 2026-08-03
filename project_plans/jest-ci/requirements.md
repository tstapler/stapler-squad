# Requirements: Wire Jest (web-app) into CI

Source: backlog item `98f006f2-1608-4509-854d-d1e89e7f139a` (migrated from
[stapler-squad#185](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/185)).

## Complexity

Complexity 1 (quick task) — a CI workflow change plus quarantining 2 known-
broken test suites and a documentation entry. No new architecture, no
user-facing surface, no multi-actor business logic.

## Problem

`.github/workflows/*.yml` contains zero `npx jest` / `npm test` / `pnpm test`
invocations. All `web-app/src/**/*.test.tsx` suites only run when a developer
manually invokes Jest locally — no CI guardrail prevents a regression (or
accidental deletion of a test file) from landing silently.

## Acceptance Criteria (verbatim from backlog item)

1. A GitHub Actions workflow step invokes Jest for the web-app test suites on
   both push to main and pull_request, scoped to relevant path filters
   (broadened beyond `lint.yml`'s extension-narrowed list to catch
   `package.json`/`jest.config.js`/lockfile changes).
2. The workflow fails (non-zero/red check) when a Jest test fails, or when
   Jest reports zero test suites collected (guards against silent test-file
   deletion).
3. The step runs against a clean install (`pnpm install --frozen-lockfile`),
   not potentially-stale local `node_modules`.
4. The 2 currently-failing/flaky suites (`SessionDetail.embedded.test.tsx`,
   `BacklogEmptyState.test.tsx`) are fixed or explicitly quarantined with a
   tracked follow-up issue, so turning the gate on does not immediately block
   unrelated PRs.
5. The CI step's run time is visible (GitHub Actions Summary tab shows
   Jest's Test Suites/Tests/Time block) without needing to open the raw log,
   on both green and red runs.
6. Which Jest projects run in CI and why is documented (reference doc +
   CLAUDE.md index entry, or equivalent), including the quarantine/fix
   decisions.

## Non-goals

- Fixing the root-cause bugs behind the 2 failing suites (a real
  accessibility regression in `SessionDetail` — missing `tablist`/`tabpanel`
  roles and `aria-hidden` wiring — and an unhandled promise rejection in
  `BacklogEmptyState`'s error-path test). These require component-level
  investigation outside the scope of "wire CI"; AC4 explicitly permits
  quarantine + a tracked follow-up issue instead.
- Coverage thresholds, parallelism tuning, or splitting Jest into its own
  workflow file — explicitly left open by the source issue ("Scope/tuning
  left to whoever picks this up").
- Running the `dev-stack` or `e2e-dev-mode` Jest projects in this gate unless
  they're already green and cheap to include (decide during research).

## Constraints / context

- `web-app/jest.config.js` defines 4 Jest **projects**: `web-app` (259 test
  files, the one referenced by AC4), `eslint-plugin-analytics`, `dev-stack`,
  and `e2e-dev-mode`.
- Baseline measured 2026-08-02: `pnpm install --frozen-lockfile && npx jest`
  on the `web-app` project → 257/259 suites pass, 3594/3601 tests pass. The 2
  failing suites are exactly the ones AC4 names.
- `lint.yml` already sets up pnpm/Node for `web-app` in CI and is the
  natural home per the source issue, but its `paths:` filter is scoped to
  specific extensions (`.go`, `.ts`, `.tsx`, `.css`, proto, etc.) — AC1
  requires broadening this (or a separate workflow) to also catch
  `package.json`, `jest.config.js`, and lockfile changes, which the current
  filter misses.
- Repo convention: new CI-relevant conventions get a `.claude/rules/*.md`
  entry + a row in `CLAUDE.md`'s Reference Documents Index (see existing
  rows for `e2e-test-conventions.md`, `feature-registry.md`, etc.) — this is
  the natural target for AC6's documentation requirement.
