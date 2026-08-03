# Pitfalls Research — Wiring Jest into CI

Agent 4 (Pitfalls), SDD research phase for `jest-ci`. Repo state verified 2026-08-02 at
`web-app/` in this worktree. All line/file references are to the current worktree tree
(not yet pushed — no stable SHA to link).

## 1. Silent-pass footguns beyond "0 suites"

| Footgun | Mechanism | Guard |
|---|---|---|
| `--passWithNoTests` | Jest CLI flag that makes an empty run exit 0. Not currently passed anywhere in `web-app/package.json`'s `"test": "jest"` script, and must **not** be added to the new CI step — its presence is exactly the AC-3 "0 suites should fail" case this task is guarding against. |
| Path filter too narrow | If the new/broadened `paths:` filter omits a file type that actually changes test behavior (e.g. a `.tsx` component file with no matching test, or `jest.config.js` itself), the workflow simply never runs on that PR — GitHub reports no status for the check at all, which if the check is *not* marked "required" is silently indistinguishable from "passed." This is the concrete failure mode requirements.md AC-1 is trying to close by adding `package.json`/`jest.config.js`/`pnpm-lock.yaml` to the filter. |
| `\| tee` losing exit code | `npx jest \| tee jest.log` in a `run:` step drops Jest's non-zero exit under `bash`'s default pipe semantics — the step exits with `tee`'s status (0), not Jest's. GitHub Actions `run:` steps use `bash -e` but **not** `pipefail` by default (only `-e` and `-o` are on; `pipefail` is not implied by `shell: bash` — confirmed by GitHub's own docs: the default `bash` shell in Actions is invoked as `bash --noprofile --norc -eo pipefail {0}` since ~2022, so pipefail *is* actually set for `shell: bash` — but this is easy to lose by explicitly overriding `shell:` to `sh` or writing a multi-line script that reassigns `$?` after an intermediate command). If a JSON-summary or step-summary step in this task pipes Jest's stdout through another process (`jq`, a formatter) to build the Step Summary, insert `set -o pipefail` explicitly and/or check `${PIPESTATUS[0]}` rather than relying on shell defaults, since the whole point of AC-5 is to add exactly this kind of piping. |
| Stray `continue-on-error: true` | Trivial to leave on from a "let me see the full log first" debugging pass while wiring this up; must not ship on the Jest step. Doesn't currently appear anywhere in `.github/workflows/lint.yml`. |
| Scoping only some Jest "projects" | `web-app/jest.config.js` defines 4 projects: `web-app` (259 files — the one this task cares about), `eslint-plugin-analytics`, `dev-stack` (roots `../scripts/dev-stack`), `e2e-dev-mode` (roots `../tests/e2e`, restricted to `*.test.ts` only). Running bare `npx jest` from `web-app/` runs **all four** projects (Jest's multi-project config runs every project by default unless `--selectProjects` is passed). If the CI step is later narrowed to `--selectProjects web-app` for speed/isolation, the other three projects' tests silently stop running in CI with no failure signal — worth deciding explicitly whether CI runs all 4 or just `web-app`, and documenting the choice (this maps to AC-6, the "which projects run and why" doc). Recommend running all 4 (they're cheap relative to the 259-file `web-app` project) rather than narrowing, to avoid this exact regression. |
| `forceExit` / open handles masking a hang | Not currently set in `jest.config.js`. If a future contributor adds `--forceExit` to work around a hanging test, it can mask a real resource leak that would otherwise fail the job via timeout — not an immediate risk for this task, but worth naming so it isn't silently added later during "green-ification." |

## 2. Path-filter risk — extending `lint.yml` vs. a separate `jest.yml`

Current `lint.yml` `paths:` filter (`.github/workflows/lint.yml:6-16` and `:20-30`) is:
```
'**.go', 'go.mod', 'go.sum', 'Makefile', 'proto/**',
'web-app/**.css', 'web-app/**.ts', 'web-app/**.tsx',
'tests/e2e/**', '.github/workflows/lint.yml'
```
Broadening this filter to add `web-app/package.json`, `web-app/jest.config.js`,
`web-app/pnpm-lock.yaml` (per requirements) has a **shared-blast-radius** consequence:
every job in `lint.yml` — `golangci-lint`, import-cycle check, ESLint, CSS lint, gofmt
check, feature-catalog validation — is a single job (`golangci:` — note despite the
`jobs:` key name there's only one job, not one per lint type; every step above runs
sequentially inside it) gated by the *same* top-level `on.push.paths` /
`on.pull_request.paths`. Broadening the filter to catch `web-app/package.json` means:

- A PR that touches `web-app/package.json` for a reason unrelated to Jest (e.g. bumping
  a UI-only dependency, adding a Storybook addon) now also triggers the Go-side
  `golangci-lint`, `go build ./...`, and gofmt-check steps — extra ~1-2 min of CI time
  and a new failure surface (e.g. transient `buf`/ent-generate flakiness) on a PR that
  never touched Go code. This is noise, not correctness risk, since those steps should
  no-op cleanly on unrelated code, but it does widen what a web-app-only PR author has
  to wait on and potentially debug.
- The reverse is *not* a new risk: this task only adds paths, so no existing trigger
  path is removed — Go-only or docs-only PRs still correctly skip the whole workflow
  (paths filters are OR'd, not AND'd, per GitHub's `on.<push|pull_request>.paths`
  semantics).
- **Required-check-skipped problem**: if any job in `lint.yml` (or a would-be `jest`
  job added to it) is marked as a *required* status check in branch protection, and a
  PR's diff doesn't intersect the `paths:` filter at all (e.g. a docs-only PR), GitHub
  does not report any status for that check — Actions' built-in path filtering operates
  at the whole-workflow level, not per-job, so a skipped-due-to-path-filter run produces
  **no check run**, which a required-check rule can't distinguish from "still pending"
  and blocks merge until someone re-runs or force-pushes a no-op commit. This is a
  known, actively-discussed GitHub limitation ([community discussion #26857](https://github.com/orgs/community/discussions/26857)),
  with `dorny/paths-filter` cited as the standard workaround (move the `paths:` check
  *inside* the job via a preceding filter step, so the job always runs and reports a
  status, just conditionally skips its real steps). Verify whether any `lint.yml` job
  is currently a required check before deciding the filter-broadening approach; if so,
  prefer the `dorny/paths-filter`-inside-job pattern over relying on top-level
  `on.paths` for the new Jest step.

**Separate `jest.yml` vs. extending `lint.yml` — trade-off:**

| | Extend `lint.yml` | New `jest.yml` |
|---|---|---|
| Blast radius on path-filter broadening | Shared: broadening for Jest's sake (package.json/lockfile) also re-triggers unrelated Go/ESLint/CSS jobs | Isolated: `jest.yml`'s own filter only needs to cover what Jest actually cares about, no cross-contamination with Go tooling |
| Setup duplication | Reuses existing pnpm/Node/cache setup already in the job (steps at `.github/workflows/lint.yml:45-64`) — no new boilerplate | Duplicates the `pnpm/action-setup` + `actions/setup-node` + `pnpm install --frozen-lockfile` block (~20 lines) in a second file |
| Required-check granularity | Coarser — can't mark "Jest passed" as a separately required check without also requiring the whole lint job (including Go steps) to pass on every PR that touches web-app | Finer — `jest.yml`'s job can be independently required, so a web-app-only PR isn't blocked on Go lint status and vice versa |
| Runtime/parallelism | Sequential inside the single existing job — Jest add-on lengthens the critical path of an already multi-step job (protobuf gen, ent gen, golangci-lint, ESLint, CSS lint, gofmt, catalog validation) | Runs as a fully parallel job/workflow, doesn't lengthen the existing lint job's critical path |
| Existing convention | `lint.yml` already mixes Go and web-app tooling (ESLint/CSS-lint steps already live there) — extending is "more of the same" | New file, but the repo already has 12 other workflow files (`build.yml`, `ux-analysis.yml`, `e2e-video.yml`, etc.) each scoped to one concern — a dedicated `jest.yml` matches that existing pattern better than continuing to overload `lint.yml` |

**Recommendation for the planning phase to weigh**: a separate `jest.yml` is the safer
default — it avoids widening `lint.yml`'s existing blast radius, keeps the new check
independently required/re-runnable, and matches the repo's established one-workflow-
per-concern convention (`ux-analysis.yml` already exists as a precedent for a frontend-
only workflow with its own `paths:` scoped to `web-app/src/`). The only cost is ~20
lines of duplicated pnpm/Node setup boilerplate, which is minor and already duplicated
elsewhere in the repo's workflow files (not investigated exhaustively, but `lint.yml`'s
pnpm/Node block is a copy-pasteable unit, not a shared composite action).

## 3. Quarantine pitfalls

**No existing precedent in this repo**: `grep -rn '\.skip(' src --include='*.test.*'` and
`grep -rn -E 'xdescribe|xit\(' src --include='*.test.*'` both returned **zero matches** —
this repo has never quarantined a Jest test before. There's no existing convention to
follow, which cuts both ways: nothing to be consistent with, but also no established
guardrail (no `eslint-plugin-jest` `no-disabled-tests` rule installed — `grep -n
'eslint-plugin-jest' package.json` returns nothing) to catch a skip that overstays its
welcome.

**Risk of permanence**: a `.skip()`/`testPathIgnorePatterns` quarantine with no forcing
function to revisit it typically becomes permanent — this is a well-documented industry
anti-pattern (skipped tests silently rot, especially once the CI job goes green and
nobody has a reason to look at the skip list again). Concretely for this task:
`SessionDetail.embedded.test.tsx` is quarantining a **real accessibility regression**
(missing tablist/tabpanel roles, aria-hidden wiring) per requirements.md — if that skip
becomes permanent, the regression itself becomes permanent and invisible, which is worse
than the status quo (no CI at all, but at least a human running `npx jest` locally would
see it fail).

**Recommended pattern** (no repo precedent exists, so this is a fresh recommendation
rather than "follow existing convention"):
- Prefix every quarantine with a `// TODO(#<issue-number>): <one-line reason>` comment
  directly above the `describe.skip`/`it.skip` call, referencing the tracked follow-up
  GitHub issue required by requirements.md AC-4. `grep -rn 'TODO(#' src --include=
  '*.test.*'` currently returns zero hits, so this would also be a new convention, not
  an existing one — worth confirming the planning phase is fine introducing it here
  first rather than assuming it's already the house style.
- Prefer `testPathIgnorePatterns` (config-level, one place to look, shows up in the
  Jest summary as fewer suites collected rather than a suite of all-skipped tests) over
  scattering `.skip()` inside the two files, *if* the reviewing team wants a single
  visible list of what's quarantined. Trade-off: `testPathIgnorePatterns` makes the
  files invisible to `npx jest` entirely (not even collected/skipped), which is a
  stronger form of the AC-3 "0 suites" risk this task is separately trying to guard
  against — a `testPathIgnorePatterns` entry could accidentally match more files than
  intended (it's a regex against the full path) and silently exclude something else.
  `it.skip`/`describe.skip` at the file level keeps the suite "collected" (shows as
  skipped, not absent) in the Jest summary, which is more consistent with AC-5's intent
  of showing suite/test counts in the Step Summary — a quarantined suite that still
  shows up as "1 skipped" is more honest than one that vanishes from the count entirely.
- No lint rule currently enforces skip hygiene; installing `eslint-plugin-jest`'s
  `no-disabled-tests` rule (as a warning, not error, so it doesn't block CI on the two
  intentional quarantines) would give a visible nudge on every PR touching test files
  without blocking. This is a "nice to have," not a hard requirement — flagging for the
  planning phase to size in or explicitly defer.

## 4. `BacklogEmptyState.test.tsx` unhandled-rejection crash — confirmed empirically

Reproduced locally in this worktree:
```
$ npx jest --testPathPatterns="BacklogEmptyState" --no-coverage
...
/…/BacklogEmptyState.test.tsx:120
            .mockRejectedValue(new Error("Server error"));
                               ^
[Error: Server error]

Node.js v26.0.0
```
The process **exits entirely** (not a per-test failure report) at test #9 ("component
stays rendered when onCreateItem rejects" — `src/components/backlog/
BacklogEmptyState.test.tsx:154-170`), which uses
`jest.fn().mockRejectedValue(new Error("Server error"))`. This is the mechanism
requirements.md describes: one bad test brings down the whole worker rather than
failing just its own file.

**Is this a known Jest 30 / Node behavior change?** Partially confirmed via search:
Node.js has treated unhandled promise rejections as fatal-by-default (throw mode) since
Node 15 — this predates Jest 30 and is a Node-level default, not new to Jest 30
specifically ([nodejs/node#20392](https://github.com/nodejs/node/issues/20392) tracks
the original change). However, [jestjs/jest#15887](https://github.com/jestjs/jest/issues/15887)
is an **open, currently-active issue** (title: "Unhandled promise rejections leading to
unexpected jest output (with workaround)") specifically about Jest 30.x's handling of
this interacting with worker processes, and the maintainers' documented workaround is
passing `--unhandled-rejections=strict` at the Node level, plus the older
[jestjs/jest#7179](https://github.com/jestjs/jest/issues/7179) and
[jestjs/jest#10364](https://github.com/jestjs/jest/issues/10364) threads describe the
general "unhandled rejection crashes the worker instead of failing the test" behavior
across multiple Jest versions — this is a long-standing rough edge in how Jest's worker
processes interact with Node's rejection-handling default, exacerbated (not newly
introduced) in Jest 30's worker model. **Label: VERIFIED that Node's throw-on-unhandled-
rejection default predates Jest 30; INFERRED (from an open, unresolved GH issue, not a
changelog entry) that Jest 30 specifically made this worse** — could not find a Jest 30
changelog line confirming a deliberate behavior change; treat "Jest 30 made this worse"
as unconfirmed and "unhandled rejections are fatal by Node default, and Jest doesn't
suppress that inside its worker processes" as the safe, confirmed claim.

**Is `it.skip(...)` sufficient?** Yes for preventing the crash specifically — if the one
test containing `mockRejectedValue` without a synchronous `.catch()`/`await` path is
skipped, its body never executes, so the rejection is never created and there's nothing
to go unhandled. This is narrower and safer than a broader `--detectOpenHandles` or
`--maxWorkers` change, which would be treating a symptom (worker instability in general)
rather than the actual cause (this one test's promise isn't awaited/caught anywhere in
either the test or the component under test — confirmed via `grep -n 'onCreateItem\|
catch\|async\|await' src/components/backlog/BacklogEmptyState.tsx`, which shows the
current component only takes a synchronous `() => void` `onCreateItem` callback with no
promise handling at all, suggesting this test file may be exercising an outdated/mocked
async contract that doesn't match the current component signature — worth flagging to
whoever eventually un-quarantines it, though root-causing that mismatch is explicitly
out of scope for this task).

**CI-vs-local behavior risk**: GitHub Actions `ubuntu-latest` runners default to 4 vCPUs
(2 for some private-repo tiers) vs. local dev machine core counts, which changes Jest's
default `--maxWorkers` (defaults to `cpus - 1`). Fewer workers in CI means fewer
parallel worker processes, so **if only one worker crashes**, Jest can still schedule
the remaining test files onto surviving workers and report a coherent (if incomplete)
result — but if `--maxWorkers` resolves to 1 in a constrained CI container, a single
worker crash takes down the *entire* run with no other worker to pick up remaining
suites, which is a strictly worse failure mode than what was observed locally (where
more workers were available to absorb the crash). This makes CI **not necessarily
identical** to the local repro — worth explicitly setting `--maxWorkers=2` (or similar)
in the CI Jest invocation rather than leaving it to auto-detect, so behavior is
deterministic and documented rather than accidentally best-effort based on runner
sizing GitHub changes over time.

## 5. Frozen lockfile + `actions/setup-node` pnpm cache interaction

`lint.yml` already uses this combination (`.github/workflows/lint.yml:50-55,62-64`):
`pnpm/action-setup` → `actions/setup-node` with `cache: 'pnpm'` and
`cache-dependency-path: web-app/pnpm-lock.yaml` → `pnpm install --frozen-lockfile`. Since
the new Jest step reuses this same job/pattern (or a near-copy in a new `jest.yml`), the
known pitfalls are already partially mitigated by precedent, but worth naming for the
plan:

- **Cache-key correctness, not staleness masking**: `actions/setup-node`'s `cache: 'pnpm'`
  keys its cache on a hash of the lockfile content ([confirmed via search](https://qaskills.sh/blog/ci-cache-pnpm-store-github-actions)),
  so a lockfile drift (someone hand-edited `pnpm-lock.yaml` or it's out of sync with
  `package.json`) does **not** get masked by a stale cache — a changed lockfile hash
  produces a cache miss and a fresh store fetch, and `--frozen-lockfile` independently
  fails outright if the lockfile doesn't satisfy `package.json`'s ranges. The two
  mechanisms don't have a masking interaction with each other; the real risk is the
  cache only warms the **package store**, not `node_modules` — `actions/setup-node`
  caches `~/.local/share/pnpm/store` (or equivalent), and `pnpm install` still has to
  re-link `node_modules` from that store every run, so a cache hit speeds up download,
  not the full install — don't expect near-zero install time even on a full cache hit.
- **Partial cache reuse observed in the wild**: [pnpm/action-setup#153](https://github.com/pnpm/action-setup/issues/153)
  documents that `actions/setup-node`'s pnpm cache integration sometimes only reuses
  roughly half of previously-cached packages on a `--frozen-lockfile` install, meaning
  first-run (cold cache) and even some warm-cache CI runs may be noticeably slower than
  the ~14s local baseline quoted in requirements.md — that 14s number was measured with
  a fully pnpm-installed local `node_modules`/store already warm; the *first* CI run
  (cold cache, cold store) should be expected to take meaningfully longer (dependency
  install alone, separate from the ~14s Jest run itself), and this shouldn't be
  mistaken for a regression when reviewing the first PR that adds this workflow.
- **Order dependency**: `pnpm/action-setup` must run **before** `actions/setup-node` —
  confirmed as a documented gotcha ([search result](https://dev.to/jtorchia/pnpm-workspaces-the-ci-cache-that-survived-the-fix-and-cost-me-40-minutes-per-build-2807)):
  if reversed, `setup-node` can't detect the `pnpm` binary on `PATH` yet and the
  `cache: 'pnpm'` option silently no-ops (falls back to no caching, not an error) rather
  than failing loudly. `lint.yml`'s existing step order (`pnpm/action-setup` at line 46,
  `actions/setup-node` at line 51) already has this right — just don't reorder it when
  copying the pattern into a new workflow file.

## Sources

- [jestjs/jest#15887 — Unhandled promise rejections leading to unexpected jest output (with workaround)](https://github.com/jestjs/jest/issues/15887)
- [jestjs/jest#7179 — Unhandled promise rejections are deprecated](https://github.com/jestjs/jest/issues/7179)
- [jestjs/jest#10364 — Jest process unhandled rejected promise and fails the test](https://github.com/jestjs/jest/issues/10364)
- [nodejs/node#20392 — Terminate process on unhandled promise rejection](https://github.com/nodejs/node/issues/20392)
- [GitHub community discussion #26857 — Path filtering on required pull request checks](https://github.com/orgs/community/discussions/26857)
- [dorny/paths-filter](https://github.com/dorny/paths-filter)
- [pnpm/action-setup#153 — Partial cache re-use on pnpm install --frozen-lockfile](https://github.com/pnpm/action-setup/issues/153)
- [dev.to — pnpm workspaces: the CI cache that survived the fix](https://dev.to/jtorchia/pnpm-workspaces-the-ci-cache-that-survived-the-fix-and-cost-me-40-minutes-per-build-2807)
- [qaskills.sh — Cache the pnpm Store in GitHub Actions](https://qaskills.sh/blog/ci-cache-pnpm-store-github-actions)
- [eslint-plugin-jest — no-disabled-tests rule](https://github.com/jest-community/eslint-plugin-jest/blob/main/docs/rules/no-disabled-tests.md)

## Repo-specific evidence trail (commands run)

```
grep -rn '\.skip(' web-app/src --include='*.test.*'          # 0 matches — no existing quarantine precedent
grep -rn -E 'xdescribe|xit\(' web-app/src --include='*.test.*' # 0 matches
grep -n 'eslint-plugin-jest' web-app/package.json              # 0 matches — not installed
npx jest --testPathPatterns="BacklogEmptyState" --no-coverage  # process crash, confirmed above
node -p "require('./node_modules/jest/package.json').version"  # 30.2.0
node -p "process.version"                                       # v26.0.0 (local; CI pins Node 22 per lint.yml)
```
