# Stack Research: Wire Jest into CI

All findings below are VERIFIED by running commands in `web-app/` at
`/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/stapler-squad-wire-jest-ci_18c82ff329765ac2`
on 2026-08-02, dependencies already `pnpm install`-ed.

## 1. GitHub Actions Summary tab (AC5): no new reporter dependency needed

`jest-junit` is **not** a devDependency today — confirmed via
`grep -n -i "jest-junit\|junit" web-app/package.json web-app/pnpm-lock.yaml` (zero matches).
The only test-related deps present are `jest@^30.2.0`, `jest-environment-jsdom@^30.2.0`,
`@types/jest@^30.0.0`, `ts-jest@^29.4.11` (`web-app/package.json:127,131,143,144,153`).

Jest's **default reporter already prints the Test Suites/Tests/Time block** to its own
output on both success and failure — no JSON/JUnit reporter required. Verified two ways:

- Passing run (`detector.test.ts`, 17 tests): ended with
  ```
  Test Suites: 1 passed, 1 total
  Tests:       17 passed, 17 total
  Snapshots:   0 total
  Time:        0.509 s, estimated 1 s
  ```
- Failing run (`SessionDetail.embedded.test.tsx`, the known-broken suite): exit code 1, and
  the block still printed:
  ```
  Test Suites: 1 failed, 1 total
  Tests:       7 failed, 1 passed, 8 total
  Snapshots:   0 total
  Time:        0.668 s, estimated 1 s
  ```

**Recommendation**: capture Jest's combined stdout/stderr to a log file
(`... 2>&1 | tee jest-output.log`), then unconditionally (`if: always()`) append it — or just
the tail — to `$GITHUB_STEP_SUMMARY` wrapped in a ```` ```text ```` fence, matching the
existing pattern already used in `.github/workflows/benchmark.yml:100-101`
(`echo "## ..." >> $GITHUB_STEP_SUMMARY` / `cat file.txt >> $GITHUB_STEP_SUMMARY`). This
satisfies AC5 ("visible without opening the raw log, on both green and red runs") with zero
new dependencies and no reporter-config surface to maintain.

**Alternatives considered and rejected for this complexity-1 task**:
- `dorny/test-reporter` / `mikepenz/action-junit-report` — both consume JUnit XML, which
  means adding `jest-junit` as a new devDependency plus a `reporters` config block in
  `jest.config.js` (touching the shared 4-project config for a CI-only concern). They also
  render results as a separate GitHub Check/PR annotation, not literally inside the Actions
  "Summary" tab content the AC describes — extra moving parts for a task explicitly scoped
  to not do "coverage thresholds, parallelism tuning, or splitting Jest into its own
  workflow file." The direct `$GITHUB_STEP_SUMMARY` append is simpler and already
  repo-idiomatic.

## 2. Zero-test-suites failure (AC2): default Jest behavior already fails correctly

Verified with a testPathPatterns that matches nothing:
```
npx jest --selectProjects web-app --testPathPatterns="zzz_nonexistent_zzz"
```
Output:
```
No tests found, exiting with code 1
Run with `--passWithNoTests` to exit with code 0
```
Confirmed actual process exit code is **1** (checked via `echo $?` directly after the
command, not through a pipe). So:

- Jest's default (`passWithNoTests` unset/false) already exits non-zero when zero suites
  match — **no extra flag is needed**, only the absence of `--passWithNoTests` anywhere in
  the CI invocation or `jest.config.js`. Grep confirms `passWithNoTests` does not currently
  appear in `web-app/jest.config.js` or `package.json` scripts.
- `--ci` is a separate, unrelated flag (only affects snapshot-write behavior — blocks new
  snapshots from being written under CI). GitHub Actions sets `CI=true` in the environment
  automatically, which Jest auto-detects to enable CI mode even without passing `--ci`
  explicitly, but passing it explicitly is harmless/idiomatic for clarity.
- Note: Jest 30 renamed the CLI flag `--testPathPattern` → `--testPathPatterns` (old name
  errors out with a migration notice) — relevant if anyone copies a stale Jest 29-era
  example.

## 3. Scoping to the `web-app` project only

`web-app/jest.config.js` defines 4 projects: `web-app` (259 test files),
`eslint-plugin-analytics`, `dev-stack`, `e2e-dev-mode`.

Confirmed flag: `npx jest --selectProjects web-app` runs only the named project(s), matched
against each project's `displayName`. Verified:
```
npx jest --selectProjects web-app --listTests | wc -l   # → 260 (259 files + header line)
```
This is the correct, minimal-surface way to scope CI to just the `web-app` project per the
requirements' non-goal ("dev-stack and e2e-dev-mode ... unless they're already green and
cheap to include (decide during research)") — left to the plan phase to decide based on
whether those two projects are currently green; this doc only confirms the mechanism.

## 4. Quarantine mechanism (AC4): CLI `--testPathIgnorePatterns`, not in-file skips

Confirmed both failing files and their exact paths:
- `web-app/src/components/sessions/__tests__/SessionDetail.embedded.test.tsx`
- `web-app/src/components/backlog/BacklogEmptyState.test.tsx`

**Recommendation**: pass `--testPathIgnorePatterns` as a CI-only CLI flag in the workflow
step, not `describe.skip`/`it.skip`/`test.todo` in the test files themselves. Verified this
works and is safe:
```
npx jest --selectProjects web-app \
  --testPathIgnorePatterns="SessionDetail\.embedded\.test|BacklogEmptyState\.test" \
  --listTests
```
→ 258 files listed (down from 259+header), zero matches for either quarantined filename,
and — importantly — **zero `node_modules` files got pulled in** despite the CLI flag
*replacing* (not merging with) the config's default `testPathIgnorePatterns: ["/node_modules/"]`
array. This is safe here specifically because the `web-app` project already scopes
`roots: ["<rootDir>/src"]` (`web-app/jest.config.js:8`), so there's no `node_modules`
directory under the search root to accidentally re-include — confirmed empirically, not just
by config-reading.

**Why this beats in-file skips**: zero diff to the test files (git blame/history stays
intact for whoever picks up the follow-up issue), trivially revertible (delete the flag/two
patterns from the workflow file, no code change), and doesn't silently change local
developer runs — `npx jest` with no CLI args still runs both quarantined suites locally, so
a developer fixing them isn't tripped up by a config-level skip they'd have to remember to
undo in two places. `test.todo` was rejected because it would require gutting the existing
assertions, destroying the record of what's actually broken. `describe.skip`/`it.skip` was
rejected because it lives in the same file CI hides via the flag, i.e. two places to keep in
sync instead of one.

Follow-up issue tracking should be referenced via a comment in the workflow file next to the
flag (e.g. `# quarantined — see issue #<N>, fixes tracked separately, non-goal of this task`).

## 5. pnpm/Node setup pattern (for consistency)

`.github/workflows/lint.yml:45-64` already has the exact pattern to reuse:
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
`lint.yml` is the natural home per the requirements doc, but its `paths:` filter
(`.github/workflows/lint.yml:6-16,20-30`) is narrowed to `web-app/**.css|ts|tsx` — AC1
requires broadening this (or adding a separate/wider filter) to also catch
`web-app/package.json`, `web-app/pnpm-lock.yaml`, `web-app/jest.config.js`,
`web-app/jest.setup.js` changes, none of which are currently covered by the extension-only
glob.

## Summary of concrete CLI invocation for the plan phase

```bash
cd web-app
pnpm install --frozen-lockfile
npx jest --selectProjects web-app \
  --testPathIgnorePatterns="<quarantine-pattern-1>|<quarantine-pattern-2>" \
  --ci 2>&1 | tee jest-output.log
# then append jest-output.log (or its tail) to $GITHUB_STEP_SUMMARY, if: always()
```
No `--passWithNoTests` anywhere. No new devDependencies required.
