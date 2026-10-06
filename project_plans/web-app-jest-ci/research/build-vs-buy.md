# Build vs. Buy — Wire web-app Jest into CI

Research question: how should the existing Jest suite (259+ test files, 4
`jest.config.js` projects: `web-app`, `eslint-plugin-analytics`, `dev-stack`,
plus one more — see `web-app/jest.config.js`) get invoked in GitHub Actions,
given zero test invocations currently exist in `.github/workflows/*.yml`.

## 1. Reuse `lint.yml`'s job vs. new job vs. new workflow file

Surveyed all 13 files in `.github/workflows/`: `lint.yml`, `build.yml`,
`benchmark.yml`, `e2e-video.yml`, `ux-analysis.yml`, `mcp-integration.yml`,
`registry-validation.yml`, `backlog-scaffolding-guard.yml`,
`goreleaser-check.yml`, `release.yml`, `release-please.yml`,
`demo-publish.yml`, `deploy-pages.yml`.

Two relevant precedents in this repo:

- **`lint.yml`** (`golangci` job): one job, single checkout, sets up Go +
  pnpm 10.27.0 + Node 22 + buf, then runs a *sequence* of unrelated static
  checks back-to-back as steps in the same job — golangci-lint, import-cycle
  check, ESLint, CSS lint, gofmt check, feature-catalog validation. All are
  fast, non-matrixed, advisory-adjacent checks that share the same setup
  cost. This job already pays for pnpm/Node/web-app dependency install, so a
  Jest step would reuse warm caches for free.
- **`build.yml`** (`prepare` → `test`/`build`/`install-check` matrix): uses a
  dedicated `prepare` job that generates protos + builds the web UI once,
  uploads artifacts, and downstream jobs (`test`, `build` matrix,
  `install-check`) download them rather than re-running codegen. This
  pattern exists because those jobs are genuinely expensive/parallel
  (cross-compile matrix across 3 OS × 2 arch). The `test` job already runs
  Go tests (`go test ./server/... ./session/... ./config/...`) — it does
  **not** touch `web-app` at all today.

**Verdict: append a step to `lint.yml`'s existing `golangci` job**, matching
the issue's proposal. Reasoning:
- `lint.yml` already has the exact toolchain Jest needs (pnpm 10.27.0, Node
  22, `web-app` deps installed via `pnpm install --frozen-lockfile`) and
  already triggers on the right path filters (`web-app/**.ts`,
  `web-app/**.tsx`) — no new trigger/path-filter logic needed.
  `build.yml` triggers on `web-app/src/**`, `package.json`, `pnpm-lock.yaml`
  only (narrower — would miss test-only file changes outside `src/`, e.g.
  if fixtures live elsewhere).
- A new job in `lint.yml` would force a second checkout + second
  pnpm/Node/dependency-install cycle for a job that's otherwise a peer of
  ESLint/stylelint — same category of check ("does the web-app code pass
  static/test gates"), same cost profile (seconds-to-low-minutes, no
  matrix, no artifacts to share).
- A new standalone `jest.yml` is unjustified: the `build.yml` `prepare` →
  matrix split exists to amortize an expensive, shared codegen step across
  several *parallel, independently long-running* jobs (cross-compile
  matrix). Jest has no such shared-artifact need — it only needs
  `pnpm install`, which `lint.yml` already does inline. A standalone
  workflow file would duplicate the pnpm/Node setup block already in
  `lint.yml` for no isolation benefit, and would need its own path filters
  kept in sync with `lint.yml`'s (drift risk day one).
- Downside of appending to `lint.yml`: it lengthens an already-multi-step
  job and means a Jest failure blocks the same check as ESLint/gofmt
  (currently required-status-check surface, presumably). This is
  acceptable for a "just make it run" issue — if run time becomes a
  problem later, splitting into a second job *within* `lint.yml` (sharing
  checkout but running in parallel) is a low-cost follow-up, not a reason
  to over-engineer now.

## 2. Off-the-shelf action for PR annotations vs. raw `run: npx jest`

- `golangci-lint-action@v7` (used in `lint.yml`) gets PR inline annotations
  for free because `golangci-lint` itself emits GitHub Actions
  `::error file=...::` workflow commands / SARIF-style output that the
  action understands — it's a first-party integration, not a bolted-on
  reporter.
- For Jest, the equivalent ecosystem options are third-party:
  - `dorny/test-reporter` — parses JUnit XML (or Jest JSON via
    `jest-junit`), posts a check run with per-test pass/fail annotations.
    Actively maintained (2020–present, MIT), widely used, but requires an
    extra dependency (`jest-junit` reporter) to produce JUnit XML output
    first, plus `permissions: checks: write` on the workflow.
  - `mikepenz/action-junit-report` — same JUnit-XML-in model, MIT,
    actively maintained, similar tradeoffs.
  - No maintained *first-party* Jest GitHub Action exists (Jest itself
    ships no official Action); every option in this space is
    community-maintained and requires the extra `jest-junit`
    reporter/config step to bridge Jest's native output to JUnit XML.
- **Verdict: raw `run: npx jest` only, no reporter action, for this issue.**
  Reasoning:
  - Adding a reporter action means adding a new dependency
    (`jest-junit`), new Jest config (`reporters: [...]`), new workflow
    permissions, and a new action to pin/trust — for a repo whose stated
    goal here is "wire the existing suite in at all" (currently *zero*
    invocations). That's scope creep relative to the issue as scoped.
  - The golangci-lint-action parity argument doesn't hold: that action's
    annotation support is a side effect of adopting golangci-lint itself
    (which the repo needed anyway for linting), not a deliberate
    "let's add annotations" decision. There's no equivalent "we need this
    tool anyway" forcing function for a JUnit reporter here.
  - Plain `npx jest` failures already surface in the Actions log with file
    + test name + diff, and a non-zero exit code fails the job/check —
    sufficient for "make it run and gate on it." Annotation-quality
    tooling is a reasonable fast-follow once the suite is actually gating
    CI and someone finds plain log output insufficient in practice, not a
    prerequisite.

## 3. Hosted/SaaS test-reporting (Codecov, Buildkite Test Analytics, etc.)

Grepped all `.github/workflows/*.yml` and `README.md` for
`codecov|junit|test-reporter|dorny|buildkite|coveralls` — **zero matches**.
The repo's only coverage-related CI step is `vladopajic/go-test-coverage@v2`
in `build.yml`'s `test` job, which reads a local `coverage.out` and enforces
a 60% global threshold entirely in-workflow (no SaaS upload, no external
service, no account/token dependency).

**Verdict: no hosted test-reporting service is in use anywhere in this
repo, and none should be added for this issue.** The Go coverage gate is a
self-contained precedent for "add gating without SaaS," worth following if
this issue is later extended to a coverage threshold for `web-app` — but
that's out of scope for "wire the suite in."

## 4. Custom logic needed to merge the 4 project reports?

No. `web-app/jest.config.js` uses Jest's native `projects` array (currently
`web-app`, `eslint-plugin-analytics`, `dev-stack`, and one more — see file
for full list). A single top-level `npx jest` invocation from `web-app/`
already runs all configured projects in one process and produces one
aggregate pass/fail exit code plus a combined summary
(`Test Suites: X passed, Y total` etc. per project, then a combined
totals line) — this is Jest's built-in multi-project runner behavior, not
something CI-side tooling needs to reimplement. No merge script, no
matrix-per-project, no custom aggregation is justified: `npx jest`'s exit
code (0 = all projects passed) is exactly what a CI step needs to gate on.

The only scenario that would justify custom logic is wanting
**per-project** pass/fail visibility as separate GitHub check runs (e.g. so
a `dev-stack` test failure doesn't read as a `web-app` test failure in the
UI) — that's a nice-to-have, achievable later via `--selectProjects` in a
matrix if ever needed, not a requirement for "make the suite run in CI."

## Summary

| Question | Verdict |
|---|---|
| Where | Append `- name: Jest` step to `lint.yml`'s existing `golangci` job (reuses pnpm/Node/deps already set up there) |
| How | Raw `run: npx jest` (matches issue proposal) from `web-app/`, no `--ci`-specific flags needed beyond defaults |
| Reporter action | None — `dorny/test-reporter`/`mikepenz/action-junit-report` are real, maintained, MIT-licensed options but add a `jest-junit` dependency + config + permissions for no requirement this issue has; the golangci-lint-action annotation precedent doesn't transfer (that tool's annotations are incidental to adopting golangci-lint, not a deliberate reporter decision) |
| SaaS/hosted reporting | None in use anywhere in the repo (confirmed via grep across all workflows + README); none recommended here — Go's local-only coverage gate (`vladopajic/go-test-coverage`) is the repo's existing self-contained precedent if a coverage gate is wanted later |
| Custom merge logic | Not needed — Jest's native `projects` runner already aggregates all 4 projects into one process/exit code from a single `npx jest` invocation |
