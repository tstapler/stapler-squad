# Requirements: Wire Jest (web-app) into CI

## Source

Migrated from GitHub issue [TylerStaplerAtFanatics/stapler-squad#185](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/185),
backlog item `98f006f2-1608-4509-854d-d1e89e7f139a`.

## Complexity

1 (quick task — CI YAML plumbing, no new architecture, no user-facing surface).

## Problem

`.github/workflows/*.yml` contains zero `jest` / `npm test` / `pnpm test` invocations.
Verified 2026-08-07 via `grep -rl "jest\|npm test\|pnpm test" .github/workflows/` — no
matches in any of the 13 workflow files. `web-app/package.json` defines `"test": "jest"`,
and there are 259 `*.test.ts(x)` files under `web-app/src/` (project `web-app`) plus
smaller Jest projects for `eslint-plugin-analytics`, `dev-stack`, and `e2e-dev-mode`
(all declared in `web-app/jest.config.js`). None of these run automatically. A regression
in application code, or an accidental deletion of a test file, has no CI guardrail today —
only a developer's local `pnpm test` catches it, and only if they remember to run it.

This gap was surfaced while closing out issue #146 ("acceptance criteria disappear while
editing during triage"): the regression test written for that fix
(`BacklogItemDetail.regression.test.tsx`) passes locally but nothing in CI enforces it.

## Goal

Add a CI job that runs the full `jest` suite (all four `jest.config.js` projects) on every
PR and push to `main` that touches `web-app/**`, and fails the check (blocks merge) on any
test failure.

## Proposed Approach (from issue)

Add a Jest step to an existing workflow — `lint.yml` already sets up pnpm/Node for
`web-app` and is suggested as the natural home:

```yaml
- name: Jest
  working-directory: web-app
  run: npx jest
```

Scope/tuning (parallelism, coverage thresholds, splitting into its own workflow) is
explicitly left open by the issue for whoever implements it.

## In Scope

- A CI step/job that runs `pnpm test` (or `npx jest`) for `web-app` on pull requests and
  pushes to `main`.
- Trigger paths cover changes that could break frontend tests: `web-app/**` at minimum
  (mirroring the existing `paths:` filters in `lint.yml`/`build.yml`).
- A failing Jest test fails the check and (via branch protection, already governing the
  other required checks in this repo) blocks merge.
- Decide and document: reuse `lint.yml`'s existing job (adds ~1-2 min to an existing
  required check) vs. a new dedicated workflow (parallelizes, isolates failure signal,
  but duplicates the pnpm/Node setup boilerplate already in `lint.yml`).

## Out of Scope

- Coverage thresholds / `--coverage` enforcement (not requested; separate follow-up if
  desired).
- Splitting Jest into its own workflow file vs. reusing `lint.yml` — a real decision to
  make in planning, not pre-decided by the issue.
- Playwright/e2e CI (`e2e-video.yml`, `ux-analysis.yml` already cover this separately).
- Fixing any currently-failing/flaky Jest tests uncovered by wiring this up — if the full
  suite doesn't currently pass in a clean CI environment, that's a blocking discovery for
  implementation, not something to silently work around (e.g. don't add `--passWithNoTests`
  or exclude failing suites to force green).
- Registering this as a new "feature" in `docs/registry/` — this is CI plumbing, not a
  product feature or RPC/UI surface.

## Acceptance Criteria

1. A GitHub Actions workflow step runs the `web-app` Jest suite (`pnpm test` / `npx jest`,
   covering all projects in `web-app/jest.config.js`) on pull requests targeting `main`.
2. The same step (or an equivalent trigger) runs on pushes to `main`.
3. The step's trigger `paths:` filter covers `web-app/src/**`, `web-app/package.json`,
   `web-app/pnpm-lock.yaml`, `web-app/jest.config.js`, and the workflow file itself, so
   frontend-only PRs actually run it (mirrors existing filters in `lint.yml`).
4. A deliberately introduced failing test (or a deleted test file that regresses coverage
   of a previously-tested behavior) causes the CI check to fail — verified by evidence
   (a real CI run or `act`/local equivalent showing red), not by reading the YAML.
5. Node/pnpm setup for the Jest step reuses the same versions already pinned in
   `lint.yml` (`pnpm 10.27.0`, `node 22`) to avoid a second source of truth.
6. The existing full Jest suite passes green in the new CI job as of the PR that adds it
   (i.e. this change does not merge while shipping a known-red required check).
7. Whatever workflow file hosts the step is reflected in `README.md`'s CI section if one
   exists, and any repo docs listing "required checks" are updated — only if such docs
   currently exist and list the other workflows (verify before adding files speculatively).

## Notes / Constraints from Codebase

- `web-app/jest.config.js` defines 4 projects: `web-app` (259 test files), `eslint-plugin-analytics`, `dev-stack` (roots at `../scripts/dev-stack`), `e2e-dev-mode` (roots at `../tests/e2e`, restricted to `*.test.ts` to avoid picking up Playwright `*.spec.ts` files). A bare `npx jest` from `web-app/` runs all four projects, since `jest.config.js` lives there and `roots` for the other two projects point outside `web-app/` via relative paths — this must be verified to work from a fresh CI checkout (no local `node_modules` cache assumptions).
- `lint.yml` already performs: pnpm setup (10.27.0), Node setup (22), `pnpm install --frozen-lockfile` in `web-app`, proto generation, ent generation, and a web dist stub — a Jest step doesn't need proto/ent generation but does need `pnpm install`.
- This repo's CSS/lint/registry rules (`.claude/rules/*`) don't apply to this change — it's CI-only, no new components, RPCs, or session-creation touchpoints.
