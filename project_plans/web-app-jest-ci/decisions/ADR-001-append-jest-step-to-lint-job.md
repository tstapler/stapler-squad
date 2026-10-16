# ADR-001: Append the web-app Jest step to `lint.yml`'s existing `golangci` job

## Status
Accepted

## Context

`.github/workflows/lint.yml` has zero `jest`/`npm test`/`pnpm test` invocations (verified via `grep -rl "jest\|npm test\|pnpm test" .github/workflows/` across all 13 workflow files — no matches). `web-app/jest.config.js` defines 4 Jest projects (`web-app`, `eslint-plugin-analytics`, `dev-stack`, `e2e-dev-mode`) covering 266 test suites / 3658 tests, none of which run in CI today. A regression (e.g. the `SessionDetail.embedded.test.tsx` DOM-assertion drift found during research, since fixed upstream in PR #354) has no CI guardrail — only a developer's local `pnpm test` catches it.

Three structural options exist for where the Jest invocation lives:
1. Append a step to the existing `golangci` job in `lint.yml`.
2. Add a new, parallel job (`jest:`) within `lint.yml`.
3. Add a new standalone `jest.yml` workflow file.

## Decision

Append a `Jest` step to the existing `golangci` job in `lint.yml`, placed immediately after "Install web-app dependencies" (before proto/ent codegen, so a test regression fails fast without waiting on unrelated Go codegen).

```yaml
- name: Jest
  working-directory: web-app
  run: npx jest --ci --maxWorkers=4
```

## Alternatives Considered

**New parallel job within `lint.yml`.** Would isolate Jest failures from Go-lint failures and run in true parallel wall-clock time. Rejected: duplicates the entire pnpm/Node/checkout setup block (`lint.yml` lines 37–64) a second time in the same file, and duplicates the path-filter list across two jobs — the exact drift risk this ADR is trying to avoid, just moved one level down instead of eliminated.

**New standalone `jest.yml` workflow.** Would give Jest its own badge, independent trigger tuning, and full failure-surface separation. Rejected per `research/build-vs-buy.md`'s verdict: `build.yml`'s `prepare` → matrix split exists to amortize codegen across a genuinely parallel cross-compile matrix — a need this task doesn't have. A new file means a *third* copy of the pnpm 10.27.0 / Node 22 pin to keep in sync with `lint.yml`'s (already the second, after `build.yml`'s own) — for a complexity-1 task per `requirements.md`, that's disproportionate maintenance surface for zero near-term benefit.

## Consequences

- **Positive**: zero new toolchain setup, zero new path-filter list to maintain in parallel, minimal diff (`lint.yml` only), matches the requirements doc's own proposed approach verbatim.
- **Negative**: the `lint` check name is now overloaded to cover both Go linting and web-app testing — a Jest hang blocks/gets-blocked-by unrelated Go-only steps in the same job, and the check name doesn't obviously communicate "and also runs the frontend test suite" to a PR author skimming check names.
- **Reversibility**: cheap to revisit. If Jest's wall-clock time grows enough to matter (see `research/pitfalls.md`'s note on cold `node_modules`/ts-jest cache — currently unaddressed, "watch real CI timing first"), splitting into a parallel job or a standalone workflow later is a mechanical extraction, not a redesign — the `npx jest --ci --maxWorkers=4` invocation itself doesn't change.

## Related

- `project_plans/web-app-jest-ci/research/build-vs-buy.md`
- `project_plans/web-app-jest-ci/research/stack.md`
- `project_plans/web-app-jest-ci/implementation/plan.md` (Creative Pass, Pattern Decisions)
