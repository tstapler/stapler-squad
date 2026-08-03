---
globs:
  - "web-app/**/*.test.ts"
  - "web-app/**/*.test.tsx"
  - ".github/workflows/jest.yml"
---

# Jest CI Wiring

`.github/workflows/jest.yml` runs the `web-app` Jest project on every push to
`main` and every pull request touching `web-app/**` or the workflow file
itself. Before this workflow existed, no frontend test suite ran in CI at
all — a regression or an accidentally-deleted test file could land silently.

## Which Jest projects run in CI

`web-app/jest.config.js` defines 4 Jest **projects**. Only one runs in CI:

| Project | Runs in CI? | Why |
|---|---|---|
| `web-app` | Yes (`--selectProjects web-app`) | The 259-file frontend suite this workflow exists to guard. |
| `eslint-plugin-analytics` | No | A separate lint-rule test project (`eslint-plugin-analytics/`), not part of the frontend regression surface this gate targets. |
| `dev-stack` | No | Tests for `scripts/dev-stack/`, a local dev tooling script, not shipped product code. |
| `e2e-dev-mode` | No | A small set of plain-Jest unit tests for the Playwright dev-mode harness itself (`tests/e2e/`) — most of `tests/e2e/` is the Playwright suite, run separately, not under Jest. |

Including the other 3 projects was explicitly deferred (see
`project_plans/jest-ci/requirements.md`'s non-goals) rather than silently
decided — if `dev-stack` or `e2e-dev-mode` gain their own CI need, add them
as their own `--selectProjects` entries (or a separate step) rather than
folding them into this one.

## Quarantine mechanism: in-file `.skip()`, not a CI flag

Two suites were broken when this workflow was wired up. They're quarantined
in-file, not excluded via a CI-side flag.

**Wrong:**
```yaml
# .github/workflows/jest.yml
run: npx jest --selectProjects web-app --testPathIgnorePatterns='BacklogEmptyState|SessionDetail.embedded'
```

**Right:**
```typescript
// BacklogEmptyState.test.tsx
// TODO(#311): ... quarantined while wiring Jest into CI; fix tracked in the linked issue.
describe.skip("BacklogEmptyState — first-run state", () => {
```

### Why in-file skip, not a CLI flag

A `--testPathIgnorePatterns` flag excludes a whole *file* — it can't express
"skip 9 of this file's 12 tests but keep the other 3 running." `it.skip()` /
`describe.skip()` skip at exactly the right granularity, and skipped tests
still show up in Jest's `Tests: N skipped` count (visible in the CI Step
Summary — see below) instead of silently vanishing from the total, which is
more consistent with this workflow's own anti-silent-deletion goal (AC2).

### The 2 quarantined suites

Tracked in [tstapler/stapler-squad#311](https://github.com/tstapler/stapler-squad/issues/311):

1. **`web-app/src/components/backlog/BacklogEmptyState.test.tsx`** — the
   entire `describe.skip("BacklogEmptyState — first-run state", ...)` block
   (9 of the file's 12 tests). One test's `.mockRejectedValue()` is never
   caught by the component, which crashes the whole Jest worker process
   under Node's unhandled-rejection default (see
   [jestjs/jest#15887](https://github.com/jestjs/jest/issues/15887)) rather
   than just failing that test. Quarantining just that one test then
   revealed 6 more pre-existing failures in the same describe block: the
   actual `BacklogEmptyState` component has no inline-create-form UX at all
   (its `+ Create First Item` button calls `onCreateItem()` directly, no
   arguments) but the tests expect a click-to-reveal form with a title
   input, priority select, and submit/cancel flow. The whole block is
   quarantined pending a decision on which side (component or tests) is
   wrong. The file's other two describe blocks (`FilterZeroState`,
   `FooterNudge`, 3 tests) are healthy and still run.
2. **`web-app/src/components/sessions/__tests__/SessionDetail.embedded.test.tsx`**
   — both describe blocks (`SessionDetail — embedded mode (Bug 4)`,
   `SessionDetail — initialTab sync (Bug 3)`, 8 tests total). A real
   accessibility regression: the component is missing `tablist`/`tabpanel`
   ARIA roles and `aria-hidden` wiring on `[aria-labelledby="tab-terminal"]`.

## Step Summary: Jest's own output, no new dependency

The workflow's `Run Jest (web-app project)` step captures Jest's own
`Test Suites: / Tests: / Time:` block and appends it to
`$GITHUB_STEP_SUMMARY` (visible in the Actions "Summary" tab, on both green
and red runs) — no new reporter dependency (`jest-junit`) or marketplace
Action (`dorny/test-reporter` etc.) was added. Jest already prints exactly
the text the Summary tab needs; adding a JUnit-XML reporter and a
third-party Action would be new supply-chain surface for a requirement met
by "put text Jest already produces somewhere visible." The append mechanic
follows the existing precedent in `.github/workflows/benchmark.yml`; the
exit-code-preserving `tee`/`PIPESTATUS` pipe is new to this workflow, since
`benchmark.yml`'s step never needed to preserve its command's real exit code.

## `--maxWorkers=2` pinning

The CI invocation pins `--maxWorkers=2` explicitly rather than relying on
the runner's auto-detected core count, so a worker crash (like the
unhandled-rejection one above) behaves the same way in CI as it does
locally when reproducing with the same flag. `2` is a deliberately
conservative starting value, confirmed/retuned against the real
`ubuntu-latest` runner's vCPU count after this workflow's first live runs
(see `project_plans/jest-ci/implementation/plan.md`, Story 2.4) rather than
guessed once and left unchecked.
