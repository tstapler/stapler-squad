# Research: Jest CLI flags, CI tuning, and version pinning for web-app-jest-ci

## 1. Recommended jest CLI flags for CI

Confirmed via `npx jest --help` against the pinned `jest@30.2.0` binary in `web-app/node_modules`:

- **`--ci`** — auto-enabled when GitHub Actions sets `CI=true` (jest detects popular CI envs automatically), but pass it explicitly for clarity/determinism. Effects: disables interactive snapshot-write prompts (fails instead of auto-writing new snapshots), tweaks some reporter output.
- **`--maxWorkers`** — worth setting explicitly rather than relying on jest's default (CPU count − 1). See §2.
- **`--reporters`** — not needed unless adding a GH-annotation reporter (see §4); default reporter output is sufficient for a pass/fail gate.
- No JUnit/GitHub Actions reporter package is currently installed (see §4) — don't reference `--reporters=jest-junit` etc. without adding the dependency first.
- **`--testPathPatterns`** (plural) is the current jest 30 flag name — the singular `--testPathPattern` was renamed. This repo's own `CLAUDE.md` already documents the plural form (`npx jest --testPathPatterns="dispatch.test"`), so the new CI step should match.
- `--silent` is NOT recommended — it suppresses `console.log`/warnings that are often the only clue when a CI-only failure differs from local.

Recommended CI invocation:
```yaml
- name: Jest
  working-directory: web-app
  run: npx jest --ci --maxWorkers=4
```

## 2. maxWorkers tuning for GitHub-hosted runners

`ubuntu-latest` standard hosted runners are 4 vCPU / 16 GB RAM. Jest's default `maxWorkers` (CPU count − 1, i.e. would auto-detect to 3 on a 4-vCPU runner) is usually fine, but explicit `--maxWorkers=4` (or `50%`) is safer and more reproducible than relying on runtime auto-detection, and avoids over-subscription if the runner's reported CPU count is virtualized higher than actual (a known jest-on-containers gotcha).

**Empirical finding on this dev machine (24 cores, Node v26.0.0 — NOT what CI uses):**
```
npx jest --ci --maxWorkers=4
Test Suites: 2 failed, 264 passed, 266 total
Tests:       7 failed, 3651 passed, 3658 total
Time:        19.65s (wall), ~54s CPU-seconds
```
266 suites / 3658 tests complete in under 20s wall-clock with 4 workers — no indication that 259+ files across 4 jest projects will thrash a 4-vCPU CI runner. No OOM signs; memory pressure is unlikely to be the bottleneck given the suite runs in-process per worker and jsdom is the heaviest environment used (only in the `web-app` project).

**Caveat / separate finding (not a CI-tuning issue, a pre-existing test-suite issue):** one file (`src/components/backlog/BacklogEmptyState.test.tsx`) crashed its whole worker process ("Jest worker encountered 4 child process exceptions, exceeding retry limit") when run with `--maxWorkers=4`, and crashed outright as an unhandled promise rejection when isolated with `--testPathPatterns="BacklogEmptyState"`. This reproduced on Node v26.0.0 (this machine's default `node`), not Node 22 (what `.github/workflows/lint.yml` pins). **This is very likely a Node-version-sensitive flake/bug in the test itself (an unhandled rejection from a `mockRejectedValue` in a `useEffect`), not a CI-infrastructure problem** — but it could not be fully ruled out without testing against Node 22 directly, which this research pass did not have installed locally. Per the requirements doc's explicit scope boundary ("Fixing currently-failing/flaky tests... is a blocking discovery, not something to paper over"), this is flagged for the implementation/validation phase to re-run under Node 22 and decide whether it's a real currently-broken test (in which case wiring CI will legitimately go red on first merge) or a local-Node-version artifact.

There were also 2 unrelated pre-existing assertion failures in `SessionDetail.embedded.test.tsx` (`Bug 3` initialTab sync tests) — same caveat applies: confirm against Node 22 before treating as a CI-wiring blocker vs. a real currently-failing test to report separately.

## 3. Does a bare `npx jest` from `web-app/` pick up all 4 projects, including roots outside web-app/?

**Verified empirically — yes.** `web-app/jest.config.js` uses `projects: [...]` with each project's `roots` resolved relative to `<rootDir>`, and jest's `<rootDir>` token resolves to *the directory containing the config file being used* (here, `web-app/`), not the process's cwd — so `roots: ["<rootDir>/../scripts/dev-stack"]` and `roots: ["<rootDir>/../tests/e2e"]` correctly resolve to `scripts/dev-stack/` and `tests/e2e/` at the repo root, one level up from `web-app/`, regardless of invocation cwd.

```
$ cd web-app && npx jest --listTests | wc -l
266
```

Breakdown by directory:
| Project | Path | Files found |
|---|---|---|
| `web-app` | `web-app/src/**` | 259 |
| `eslint-plugin-analytics` | `web-app/eslint-plugin-analytics/**` | 4 |
| `dev-stack` | `scripts/dev-stack/**` (outside web-app/) | 1 |
| `e2e-dev-mode` | `tests/e2e/**` (outside web-app/, restricted to `*.test.ts` only) | 2 |

Total 266, matching the requirements doc's "259 test files under web-app/src/" for the `web-app` project specifically, plus the three smaller projects. No CI-specific quirk expected here — this was tested from a fresh invocation with no jest cache and reflects `<rootDir>`-relative resolution, which is a build-time property of the config file's own location, not of `node_modules` state, so a clean CI checkout behaves identically. One implication: **the trigger `paths:` filter must not be scoped only to `web-app/**`** if the intent is "run when any of the 4 projects' source changes" — `scripts/dev-stack/**` and `tests/e2e/**` live outside `web-app/`. Requirements doc says "Trigger paths cover web-app/** at minimum" — this is a known/accepted gap, not an oversight, but worth flagging for the plan phase to decide whether to widen the trigger paths to include `scripts/dev-stack/**` and `tests/e2e/**` (mirroring `lint.yml`'s own trigger, which already includes `tests/e2e/**` separately from `web-app/**`).

## 4. GitHub annotations for Jest — reporter package needed?

**Not currently installed.** Checked `web-app/package.json` devDependencies — no `jest-junit`, no `jest-github-actions-reporter`, no equivalent. Checked all `.github/workflows/*.yml` — no `dorny/test-reporter`, no junit XML consumption anywhere in the repo.

The Go side's `golangci-lint-action@v7` provides annotations natively (no separate reporter dependency — the action itself parses lint output and posts annotations), which is a different mechanism than what Jest needs (Jest failures would require either a JUnit XML reporter + `dorny/test-reporter`, or a purpose-built GH Actions reporter package, both net-new dependencies).

**Recommendation for this task's scope:** skip it. The requirements doc explicitly leaves "coverage thresholds" and broader tuning out of scope and frames this as CI plumbing ("no new architecture"). Jest's default CI output already prints full failure diffs and stack traces in the Actions log — sufficient for a merge-blocking gate. Adding `jest-junit` + `dorny/test-reporter` is a reasonable *follow-up* enhancement (better PR-level annotations matching the golangci-lint experience) but would add a new devDependency and a second workflow step, which is more than this "quick task" (complexity 1) calls for. If the team wants annotation parity with the Go lint job, split it into a separate backlog item rather than bundling into this wiring task.

## 5. Pinned versions already in use (avoid a second source of truth)

From `web-app/package.json`:
```
"jest": "^30.2.0"
"jest-environment-jsdom": "^30.2.0"
"@types/jest": "^30.0.0"
"ts-jest": "^29.4.11"
```

From `.github/workflows/lint.yml` (already the canonical pin for web-app CI jobs):
```yaml
- name: Set up pnpm
  uses: pnpm/action-setup@f40ffcd9367d9f12939873eb1018b921a783ffaa # v4
  with:
    version: 10.27.0

- name: Set up Node.js
  uses: actions/setup-node@v4
  with:
    node-version: '22'
    cache: 'pnpm'
    cache-dependency-path: web-app/pnpm-lock.yaml

- name: Install web-app dependencies
  working-directory: web-app
  run: pnpm install --frozen-lockfile
```

**No new pin should be introduced.** The new Jest step should reuse this exact `pnpm-action-setup` SHA-pinned version + Node 22 + `pnpm install --frozen-lockfile` sequence, either by adding a `Jest` step inside the existing `lint.yml` `golangci` job (reusing its already-installed `web-app` deps — cheapest, matches the issue's own proposed approach) or by extracting this setup into a shared composite action if a second dedicated workflow is preferred. `.github/actions/prepare/` exists as a precedent for a reusable composite action (used by `build.yml`) — worth checking during the plan phase whether it already covers pnpm/Node setup and could be reused here instead of duplicating lint.yml's inline steps.

**Local environment caveat:** this research was run against locally installed `node v26.0.0` (not what CI pins — CI uses Node 22). The one worker-crash and two assertion failures noted in §2 should be re-verified against Node 22 specifically before concluding whether they're real currently-failing tests (a legitimate "blocking discovery" per the requirements doc's out-of-scope note) or artifacts of the local Node version mismatch.
