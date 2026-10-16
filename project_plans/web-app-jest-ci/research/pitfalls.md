# Pitfalls Research: Wiring the web-app Jest Suite into CI

Research for `sdd:2-research` on wiring the 266-file / 3658-test Jest suite (4
`jest.config.js` projects: `web-app`, `eslint-plugin-analytics`, `dev-stack`,
`e2e-dev-mode`) into GitHub Actions for the first time. `.github/workflows/*.yml`
currently has zero Jest invocations — `build.yml`'s `test` job runs `go test`
only.

## 1. Does the suite pass today in a clean environment? — NO, VERIFIED

This is the headline finding. The suite does **not** pass clean, independent of
node version, worker count, or test ordering.

**Evidence — full run, local machine (Node v26.0.0, 24 cores), `npx jest --ci --silent`:**

```
Test Suites: 2 failed, 264 passed, 266 total
Tests:       7 failed, 3651 passed, 3658 total
Time:        12.517 s
```

**Evidence — same command re-run under Docker `node:22-bookworm`** (matching the
Node 22 the proposed workflow would install via `actions/setup-node`), isolated
to the failing file:

```
$ docker run --rm -v <repo>:/repo -w /repo/web-app node:22-bookworm bash -c \
    "corepack enable && corepack prepare pnpm@10.27.0 --activate && \
     pnpm install --frozen-lockfile && \
     npx jest --ci --testPathPatterns='BacklogEmptyState'"
...
/repo/web-app/src/components/backlog/BacklogEmptyState.test.tsx:120
            .mockRejectedValue(new Error("Server error"));
                               ^
[Error: Server error]
Node.js v22.23.2
```

Identical crash under Node 22 — this is **not** a local-Node-version artifact.
It will reproduce deterministically in GitHub Actions.

### Failure 1: `BacklogEmptyState.test.tsx` crashes the whole Jest worker process

`src/components/backlog/BacklogEmptyState.test.tsx:154-170`, test "component
stays rendered when onCreateItem rejects", does `jest.fn().mockRejectedValue(new
Error("Server error"))` and clicks through to trigger it. The rejection escapes
as a genuinely unhandled promise rejection at the Node level (not caught by
Jest's per-test error boundary), and Node's default `--unhandled-rejections=throw`
behavior (default since Node 15) kills the whole worker **process**, not just the
test. Confirmed identical on Node 22.23.2 and Node 26.0.0.

When this file runs as part of the full 266-suite run (not in isolation), the
crash surfaces as:

```
FAIL src/components/backlog/BacklogEmptyState.test.tsx
  ● Test suite failed to run
    Jest worker encountered 4 child process exceptions, exceeding retry limit
```

— Jest retries the crashed worker 4 times, each retry hits the identical crash.
This is a real bug in either the test or the component under test (the
component's error-handling path doesn't await/catch the rejection the way the
test assumes), not test infra flakiness.

### Failure 2: `SessionDetail.embedded.test.tsx` — 7 assertion failures, reproduces in isolation

Confirmed via `npx jest --ci --testPathPatterns="SessionDetail.embedded"` alone
(not cross-project state leakage): 7 failed, 1 passed. Tests look for
`role="tablist"` / `role="tabpanel"` / `[aria-labelledby="tab-terminal"]` that no
longer exist in the rendered DOM.

Root cause (via `git log`): commit `ef83d1e9a` ("refactor(pane): remove
redundant tab strip from PaneHeader", 2026-07-06) removed `PaneHeader`'s tab
strip and touched this test file (13 lines), but didn't fully align the test's
DOM-structure assertions with the new markup — a genuine regression left
uncaught because nothing ran this suite in CI.

### What this means for the plan

Wiring this suite in as-is (even non-blocking) means the very first PR that adds
the workflow shows red, for reasons unrelated to that PR's own diff — the classic
first-time-CI-wiring trap of conflating "add the gate" with "fix what the gate
finds." The implementation plan should have an explicit, separate task to fix
(or skip-with-tracked-followup) these 2 files **before** the CI job is made a
required/blocking check, so the PR that introduces the gate is green on its own
merits.

### What's clean

- **Zero snapshot tests** (`grep -rl toMatchSnapshot src --include="*.test.ts*"`
  → 0 matches) — no snapshot-drift risk, `--ci`'s "fail instead of write" behavior
  for snapshots is moot here.
- **No filesystem/`~/.stapler-squad`/`systemctl`/tmux dependencies** — the
  `localhost:8543` hits found by grep (`useSessionService.test.ts`,
  `FileTree.test.tsx`) are mocked `getApiBaseUrl()` return values used only to
  assert on `fetch` call arguments; nothing makes a real network call or depends
  on the live systemd-managed instance.
- No timezone/locale-sensitive assertions found (`Intl.`, `toLocaleString`,
  `process.env.TZ` — 0 matches).

## 2. Known Jest-in-CI pitfalls

- **`--ci` flag**: `package.json`'s `"test": "jest"` script has no default
  `--ci`. GitHub Actions runners set `CI=true` automatically, which Jest
  auto-detects to disable watch mode, but explicitly passing `--ci` in the
  workflow step is still worth it for clarity and so the intent survives someone
  running the script differently later. Since there are 0 snapshot tests today,
  the "`--ci` fails on new snapshots instead of writing them" difference is not
  currently a live risk — but will become one the moment anyone adds a snapshot
  test, so it's worth calling out in the workflow step's `run:` comment.
- **Test isolation across the 4 projects**: verified the `SessionDetail.embedded`
  failures are not caused by cross-project or cross-file state leakage — same 7
  failures reproduce when that file is the *only* thing selected. No evidence of
  global-state bleed between projects in this pass, but note `jest.setup.js`
  (`setupFilesAfterEach`) is wired only into the `web-app` project, not the other
  3 — if a future test in `dev-stack` or `e2e-dev-mode` needs the same jsdom
  polyfills/mocks it won't get them silently.
- **Memory/CPU on `ubuntu-latest`**: local run used 1246% CPU across 24 cores
  (default `maxWorkers` ≈ core count). GitHub's standard `ubuntu-latest` runners
  have **4 cores / 16GB RAM** — over 5x fewer cores than the dev box this was
  timed on. The 12.5s local wall time is not a reliable estimate for CI; expect
  meaningfully longer wall time and higher risk of worker contention/OOM at
  default `maxWorkers`. Recommend capping workers explicitly (e.g.
  `--maxWorkers=2` or `--maxWorkers=50%`) rather than trusting the runner to pick
  a sane default, and watching first-run wall-clock time before assuming it's
  fine.
- **`ts-jest` transform cost with cold cache**: `tsconfig.json` has
  `"isolatedModules": true`, which keeps `ts-jest` in cheap per-file transpile
  mode (no whole-program type-check) rather than the expensive path. Locally,
  deleting `node_modules/.cache` before a run made no measurable difference
  (11.4s vs 12.5s) — but that test ran on the same 24-core/large-RAM box, so it
  doesn't prove cold-cache cost is negligible on a 4-core runner. Don't
  preemptively add a `node_modules/.cache/ts-jest` cache step; instead watch the
  first few real CI run times and add one only if `ts-jest` transform overhead
  actually shows up as the bottleneck.

## 3. `pnpm` caching coverage — store only, not `node_modules`

`actions/setup-node`'s `cache: 'pnpm'` (already used in `lint.yml`, keyed on
`web-app/pnpm-lock.yaml`) caches pnpm's **content-addressable store**
(`~/.local/share/pnpm/store`), not `node_modules` itself and not
`node_modules/.cache/ts-jest`. Every CI run still does a real
`pnpm install --frozen-lockfile` (fast — no re-download — but still does the
linking work) and produces a **fresh `node_modules` tree from scratch** each
run. Consequently `ts-jest`'s on-disk transform cache is cold on every single CI
run, unlike a warm local dev environment where it persists indefinitely between
test runs. Combined with `isolatedModules: true` (see above) this is probably
fine, but it's a structurally different cache-warmth situation than local
runs, worth naming explicitly rather than assuming "pnpm caching is already
set up" fully covers Jest's needs.

## 4. Path-filter / required-check "stuck pending" risk — latent, not active today

Checked whether this repo's existing `paths:` filter pattern (used today in
`lint.yml` and `build.yml` for both `push` and `pull_request`) already creates
the classic "required check with a path filter leaves doc-only PRs stuck
pending forever" GitHub Actions gotcha:

```
$ gh api repos/tstapler/stapler-squad/branches/main/protection
{"message":"Branch not protected", ...}  # 404

$ gh api repos/TylerStaplerAtFanatics/stapler-squad/rulesets
[]
```

Neither repo currently has branch protection rules or rulesets configured on
`main` (as visible via the API from this session's auth). **This means the
stuck-pending failure mode is not currently active anywhere in this repo**,
including for the existing `lint.yml`/`build.yml` path filters — nothing is
"required," so a PR that never triggers those jobs simply merges without them,
no stuck state.

This is still worth designing for, since it will become live risk the moment
anyone flips on "require status checks to pass" and includes the new Jest job
in the required list:
- A PR touching only `docs/**` or `project_plans/**` would never trigger a
  path-filtered Jest job, and GitHub shows a required-but-never-run check as
  perpetually "Expected — Waiting for status to be reported," blocking merge.
- Mitigation options if/when branch protection is added: (a) don't mark the
  Jest job as a required check, (b) add a cheap always-run stub job (e.g. via
  `dorny/paths-filter` + a fallback "skip" job that reports success when paths
  don't match) so the check name always reports something, or (c) drop the path
  filter for this specific job and let Jest's own fast no-op-if-nothing-changed
  behavior absorb the cost.

## 5. Precedent patterns in this repo worth reusing

- **`build.yml`'s `test` job** (Go tests) is the closest structural precedent:
  runs in its own job `needs: prepare`, does **not** gate the build matrix —
  "binaries are published regardless of test failures so the build artifact is
  always available for manual verification" (comment at `build.yml:116-118`).
  Given the task frames the new Jest job as an actual CI gate (not advisory),
  model the job structure (separate job, clear naming, `needs:` chaining if it
  needs generated proto/ent code) on this rather than on the advisory pattern
  below — but note `build.yml`'s Go `test` job is explicitly *not* a merge
  gate today either; confirm with the user/plan whether the new Jest job is
  meant to block merge from day one or start advisory (see finding #1 — 2
  pre-existing failures argue for starting non-blocking or fixing them first).
- **`ux-analysis.yml`** demonstrates useful conventions for a frontend-focused
  workflow: `continue-on-error: true` for non-blocking sub-steps (Lighthouse),
  a blocking step for the parts that matter (Axe critical/serious violations),
  an `if: always()` cleanup/log-tailing step for debuggability, and a
  PR-comment step keyed on a stable HTML marker comment so re-runs update
  rather than duplicate the comment. The PR-comment pattern could be reused to
  post a one-line pass/fail summary with failing test names, if that'd help
  triage without requiring reviewers to open workflow logs.
- **`benchmark.yml`** shows the `continue-on-error: true  # Advisory only —
  never blocks merge` idiom with an explanatory inline comment — a good model
  if the decision is to start the Jest gate non-blocking during the
  stabilization period covering finding #1's 2 known failures.
- **No existing retry-on-flake or coverage-upload convention** exists anywhere
  in `.github/workflows/*.yml` (grep for `artifact`/`retry`/`continue-on-error`
  across all workflows turned up nothing coverage-shaped) — this would be a
  genuinely new category for the repo, not an extension of an established
  pattern. `e2e-video.yml`'s `matrix.shard` sharding is a scaling precedent
  worth remembering if the suite grows enough to need parallel job splitting,
  but at 12.5s / 266 suites today it's not warranted yet.

## Summary of concrete plan implications

1. **Add a task to fix (or explicitly skip + track) `BacklogEmptyState.test.tsx`
   and `SessionDetail.embedded.test.tsx` before/alongside adding the workflow**,
   so the PR that wires CI is green under its own diff. The `BacklogEmptyState`
   failure in particular is a process-crashing bug, not a simple assertion
   mismatch — worth root-causing the component's unhandled-rejection path, not
   just patching the test.
2. Pass `--ci --maxWorkers=50%` (or a fixed low number) explicitly in the new
   workflow step; don't rely on Jest's runner-detected default worker count,
   since the 12.5s/24-core local timing is not representative of `ubuntu-latest`'s
   4 cores.
3. Don't add a `node_modules/.cache/ts-jest` cache step preemptively —
   `isolatedModules: true` likely keeps cold-cache cost low; verify with real
   first-run CI timing before adding cache complexity.
4. If/when branch protection with required checks is turned on for this repo,
   revisit whether the new Jest job needs a path-filter-independent fallback so
   it doesn't leave non-`web-app` PRs stuck pending — not urgent today since no
   required checks exist yet on either remote.
5. Model the workflow job on `build.yml`'s separate `test` job pattern; decide
   explicitly (and state in the plan) whether it blocks merge from day one or
   starts `continue-on-error: true` advisory during stabilization, given finding
   #1.
