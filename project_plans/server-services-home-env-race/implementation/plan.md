# Implementation Plan: server-services-home-env-race

**Feature**: Route every process-global `HOME` / `STAPLER_SQUAD_TEST_DIR` mutation in `server/services` tests through lock-guarded helpers (`withFakeHome`, `withIsolatedStateDir`), so a test cannot observe another test's env value while it runs.
**Date**: 2026-10-13
**Status**: Ready for implementation (work is already in the tree; this plan is review-and-verify, not a rewrite)
**Complexity**: 2 (field absent in requirements.md; treated as 2)
**ADRs**: None (no non-standard technology; the design limit is recorded under Unresolved Questions)

Evidence labels: **VERIFIED** = command run in this planning session, output seen. **INFERRED** = reasoned from code, not measured.

---

## Current state of the tree (VERIFIED, planning session)

| Fact | Command | Result |
|---|---|---|
| 60 modified files: 57 under `server/services/`, plus `envtest/envtest.go`, `go.sum`, `session/instance_serialization.go` | `git diff --name-only \| grep -c '^server/services/'` and `\| wc -l` | `57`, `60` |
| Only untracked path is the planning dir | `git status --short \| grep -v '^ M'` | `?? project_plans/server-services-home-env-race/` |
| Bare `t.Setenv("HOME"` left: 2 in the helper, 1 intentional exception | `grep -rn 't.Setenv("HOME"' server/services` | `home_env_test_helper_test.go:52`, `:73`; `session_service_search_index_test.go:42` |
| Bare `t.Setenv("STAPLER_SQUAD_TEST_DIR"` left: helper only | `grep -rn 't.Setenv("STAPLER_SQUAD_TEST_DIR"' server/services` | `home_env_test_helper_test.go:105` only |
| `envtest.NewIsolatedStateDir` callers in `server/services` | `grep -rn 'envtest.NewIsolatedStateDir' server/services` | 0 call sites (one doc-comment mention, `home_env_test_helper_test.go:83`) |
| Sites migrated by the diff | `git diff -U0 -- server/services \| grep '^-.*<pattern>' \| wc -l` | 115 `envtest.NewIsolatedStateDir`, 41 `t.Setenv("HOME"`, 25 `t.Setenv("STAPLER_SQUAD_TEST_DIR"` removed; 154 added lines mention `withIsolatedStateDir` |
| `go.sum` change is accidental | `go mod tidy -diff` | tidy would delete exactly the 6 added lines (go-runewidth, tablewriter, uniseg); `go mod why -m github.com/olekukonko/tablewriter` says "main module does not need" it; `go.mod` has no entry for any of the three |
| Tree builds and vets | `go build ./...`; `go vet ./server/services/ ./session/ ./envtest/` | both exit 0, no output |
| golangci-lint (same flags as `make lint`) is clean on touched packages | `CGO_ENABLED=0 golangci-lint run --enable=nilnil,staticcheck,ineffassign,govet ./server/services/ ./envtest/` and `./session/` | `0 issues.` both |
| Custom analyzers clean on touched packages | `tools/lint` `linter ./server/services/ ./session/ ./envtest/` | no output (no findings) |
| New helper tests pass under `-race -count=3` | `go test ./server/services/ -race -count=3 -run 'TestWithIsolatedStateDir_ReleasesLockOnSuccessiveCalls\|TestWithFakeHome_ReleasesLockOnSuccessiveCalls\|TestEnvLock_ReentrantWithinOneTestAndSubtests\|TestTestRemoteConnection_TwoServiceInstances'` | `ok ... 3.944s`, 3x PASS each (incl. the test that deadlocked in migration v1) |
| `make lint` covers this package | `Makefile:824` `lint` = `lint-custom` + `golangci-lint run ./...` (no path filter); `.golangci.yml` excludes `server/` from `gochecknoglobals` and `_test.go` from `dupl`/`gocyclo`/etc. | new package-level `stateEnvMu`, `homeHolders`, `stateHolders` do not trip lint (confirmed by the `0 issues.` run above) |

Not verified in this session (another process owns it): `go test ./server/services/ -race -count=5` (AC6).

---

## Domain Glossary
*(Names exactly as they appear in `server/services/home_env_test_helper_test.go` unless noted.)*

| Term | Definition | Notes |
|---|---|---|
| `homeEnvMu` | Package-level `sync.Mutex` serializing every test that mutates `HOME` | Pre-existing; lock order: acquired before `stateEnvMu` |
| `stateEnvMu` | Package-level `sync.Mutex` serializing every test that mutates `STAPLER_SQUAD_TEST_DIR` | New (AC2) |
| `withFakeHome(t)` | Locks `homeEnvMu`, points `HOME` at `t.TempDir()`, returns the path | Pre-existing; also `withFakeHomePath(t, dir)`, `withFakeHomeAsFile(t)` |
| `withIsolatedStateDir(t)` | Locks `stateEnvMu`, points `STAPLER_SQUAD_TEST_DIR` at `t.TempDir()`, returns the path | New (AC2) |
| `withIsolatedStateDirPath(t, dir)` | Same as above for a caller-supplied (pre-seeded) directory | New (AC2) |
| `acquireEnvLock(t, mu, holders)` | Re-entrant per test: takes `mu` once, releases via `t.Cleanup`; the holder test and its subtests pass through | New; added after migration v1 deadlocked on non-reentrant locks |
| `envLockHolders` / `homeHolders` / `stateHolders` | Records which `t.Name()` currently owns each mutex so `acquireEnvLock` can detect re-entry | New |
| `WithFakeHome` / `WithIsolatedStateDir` | Exported re-exports for the external `services_test` package (2 files declare `package services_test`) | Same binary, same mutexes |
| mutator | A test that writes a process-global env var | The only thing the mutexes serialize |
| leaked goroutine | A goroutine started by test A that outlives A and reads `HOME`/`STAPLER_SQUAD_TEST_DIR` live (`config/config.go:124,132`) | Not covered by either mutex; see Unresolved Questions |
| intentional exception | `TestNewSessionService_TestMode_NeverTouchesRealSearchIndex` in `session_service_search_index_test.go`, which sets raw `HOME` because it regression-tests the un-isolated fallback path | Commented at `:35-43` |
| `ToInstanceData` | `(*Instance).ToInstanceData()` in `session/instance_serialization.go`; serializes an instance for storage | Production race site, see Epic 2 |
| `GetDiffStats` / `SetDiffStats` | `GitWorktreeManager` accessor (takes `gm.mu.RLock`) / mutator (takes `gm.mu.Lock`) for `diffStats` | `session/git_worktree_manager.go:472`, `:489` |

---

## Creative pass: alternatives considered

| Approach | Strength | Weakness |
|---|---|---|
| A. Mutex-guarded test helpers, migrate every call site (what AC1-5 specify, and what is already in the tree) | Mechanical, reviewable, mirrors the existing `homeEnvMu` pattern, no production change | Serializes mutators only; does not stop a leaked goroutine reading env live (INFERRED, see risk R1) |
| B. Capture the resolved config dir at goroutine-spawn time in production code (or join goroutines before test cleanup) | Fixes the actual reader side; works for any env var | Touches production concurrency in many packages; unbounded blast radius; needs the leak located first |
| C. Set `HOME`/`STAPLER_SQUAD_TEST_DIR` once in `TestMain` and never mutate per test | Removes mutation entirely, no locks | ~100+ tests assert on per-test dirs; they would share state and become order-dependent |

**Chosen: A**, because the requirements fix the scope to it and it is already implemented. B is recorded as the follow-up that the goleak experiment (Task 1.6b) feeds; C rejected for shared-state coupling.

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|---|---|---|---|---|
| `withFakeHome` / `withIsolatedStateDir` | Test helper with `t.Cleanup` release (Resource Acquisition Is Initialization analogue) | Go testing idiom | Caller-managed `Lock()/defer Unlock()` | Cannot be forgotten; released on `t.Fatal` |
| `acquireEnvLock` | Re-entrant lock keyed by owner test name | Monitor pattern (GoF-adjacent) | Plain `sync.Mutex` (migration v1) | Deadlocked on `newTestRemoteService` called twice in one test |
| Two env mutexes | Fixed global lock order (home, then state) | Lock hierarchy | Single combined mutex | Tests that need only one var should not block on the other |
| Helper return type | `string` path | Existing `withFakeHome` signature | Newtype `StateDir` | Test-only helper; newtype would force edits at 150 call sites for no safety gain |
| `GetDiffStats()` in `ToInstanceData` | Use existing locked accessor | `.claude/rules/instance-lock-free-reads.md` spirit | Add `diffStats` to `InstanceSnapshot` | Larger change; the accessor already exists and is correct |

## Tech Debt Disposition

| Area | Existing Issue | Disposition | Justification |
|---|---|---|---|
| `session/instance_serialization.go:203` | Bare read of mutex-guarded `gitManager.diffStats` | Refactor-first (one-line fix, already in tree; separate commit) | AC6 cannot pass under `-race` while it exists (research/findings.md) |
| Leaked goroutine(s) reading env live | Reader-side race not covered by AC1-5 | Isolate via seam: out of scope here; Task 1.6b only locates it | Fix is production concurrency work (approach B); needs its own plan |
| `session_service_search_index_test.go` exception | Raw `HOME` + `os.Unsetenv` of `STAPLER_SQUAD_TEST_DIR` outside both locks | Extend as-is | AC1 explicitly permits one commented exception; test is serial (Go never runs serial top-level tests concurrently) |

## Migration Plan
Not applicable (no schema or data change).

## Observability Plan
- **Logs**: none new (test-only change plus one accessor swap).
- **Metrics**: none.
- **Alerts**: no new alerts required.

## Risk Control
- **Feature flag**: not gated.
- **Rollback procedure**: standard revert via PR close or revert commit. Commits are split so the production fix and the test migration revert independently.
- **Staged rollout**: full rollout on merge.

## Unresolved Questions
- [ ] Does AC6 passing prove the race is fixed? Baseline `main` did not reproduce in 10 full runs, so a green `-race -count=5` is necessary, not sufficient (R1). Accept as-is per requirements; Task 1.6b gathers the evidence needed to decide on approach B. — blocks nothing in this plan — owner: Tyler
- [ ] Which goroutine leaks and reads env live? — answered by Task 1.6b; consumed by a future plan, not this one — owner: implementing agent
- [ ] Add a `-race` regression test for the `ToInstanceData` fix (Task 2.2b)? Not an acceptance criterion; recommended because the race was seen once and is otherwise unguarded. — blocks Story 2.2 only — owner: Tyler (default: yes)
- [ ] Should the intentional-exception test also hold `homeEnvMu`/`stateEnvMu` via `acquireEnvLock` while keeping raw `Setenv`? AC1 allows "holds `homeEnvMu`" as an alternative. Default: leave as-is (it is serial, and the AC blesses it). — blocks nothing — owner: Tyler

## Dependency Visualization

```
        [wait: in-flight `-race -count=5` run (other process) finishes]
                              |
                     2.1a revert go.sum
                              |
      +-----------+-----------+-----------+-----------+
      |           |           |           |           |
  1.1a AC1   1.2a AC2    1.3a AC3    1.4a AC4    1.5a AC5     2.2a review fix
  (greps)    (read)      (greps)     (read)      (run)             |
      |           |           |           |           |         2.2b regression test
      +-----------+-----------+-----------+-----------+-------------+
                              |
                    1.7a AC7 (make lint, full)      <- serialized: CPU contention
                              |                        causes load-sensitive timeouts
                    1.6a AC6 (-race -count=5)          (see commit 3c825969e)
                              |
                    1.6b goleak experiment (scratch, uncommitted)
                              |
                  3.1a commit C1 (session fix)
                  3.1b commit C2 (test migration + envtest doc)
                  3.1c commit C3 (planning artifacts)
                  3.1d push branch + open PR
```

---

## Phase 1: Verify and deliver existing work

### Epic 1.1: Prove each acceptance criterion against the tree

**Goal**: One story per acceptance criterion (1:1). Each says whether the criterion is already satisfied in the tree and runs the exact proof. No story rewrites the migration; a story only edits if its proof fails.

#### Story 1.1.1: AC1 - every `HOME` mutation goes through `withFakeHome*` or holds `homeEnvMu`
**As a** maintainer, **I want** no bare `t.Setenv("HOME", ...)` outside the helper, **so that** HOME writers are serialized.
**Status in tree**: SATISFIED (VERIFIED, table above).
**Acceptance Criteria**:
- `grep -rn 't.Setenv("HOME"' server/services` returns only `home_env_test_helper_test.go` and `session_service_search_index_test.go`; the latter carries an explanatory comment.
  - *Given* the migrated tree with `withFakeHome(t)` at 33 call sites, *When* `grep -rn 't.Setenv("HOME"' server/services` runs, *Then* it prints exactly 3 lines: `home_env_test_helper_test.go:52`, `home_env_test_helper_test.go:73`, `session_service_search_index_test.go:42`, and line 41 of the last file is the comment `// Intentional exception to the withFakeHome rule`.
**Files**: `server/services/home_env_test_helper_test.go`, `server/services/session_service_search_index_test.go`

##### Task 1.1.1a: Audit HOME call sites (XS)
- Run `grep -rnE 'Setenv\("HOME"|Setenv\(.?HOME|os\.Setenv\("HOME"' server/services`; expect the 3 lines above and no `os.Setenv("HOME"`.
- Run `grep -rn 'withFakeHome\(AsFile\|Path\)\?(' server/services | wc -l`; expect the count recorded in the audit note (33 sites at planning time).
- Spot-check 5 migrated files from `git diff` for sites that set HOME indirectly (e.g. `t.Setenv(homeVar, ...)`); report any.
- Files: read-only.

#### Story 1.1.2: AC2 - `stateEnvMu` + `withIsolatedStateDir` / `withIsolatedStateDirPath` exist and mirror `homeEnvMu`/`withFakeHome`
**Status in tree**: SATISFIED (VERIFIED by reading `home_env_test_helper_test.go:77-107`).
**Acceptance Criteria**:
- `stateEnvMu`, `withIsolatedStateDir(t)`, `withIsolatedStateDirPath(t, dir)` are defined in `home_env_test_helper_test.go`, acquire via `acquireEnvLock` before `t.Setenv`, and document the home-then-state lock order.
  - *Given* a test that calls `withFakeHome(t)` then `withIsolatedStateDir(t)`, *When* it runs while another test holds `stateEnvMu`, *Then* it blocks on `stateEnvMu` only after holding `homeEnvMu`, never the reverse, so two such tests cannot deadlock.
**Files**: `server/services/home_env_test_helper_test.go`

##### Task 1.1.2a: Audit helper semantics (XS)
- Read the helper file once; confirm in the audit note: (1) lock registered before `t.Setenv` so LIFO cleanup restores env, then unlocks (VERIFIED by reading `acquireEnvLock`: cleanup registered first, `t.Setenv` second); (2) `holders.heldBy` prefix match `name+"/"` cannot match an unrelated top-level test; (3) `-count=5` reruns reuse test names sequentially, so a stale holder name is cleared by the previous cleanup (`holders.set("")`).
- Grep for any test that takes `stateEnvMu` before `homeEnvMu`: `grep -rn 'withIsolatedStateDir' server/services | head` then, for each file that calls both `withFakeHome*` and `withIsolatedStateDir*` (list them with `grep -l` on each pattern and intersect), confirm the home call comes first.
- Files: read-only; edit only on a found defect.

#### Story 1.1.3: AC3 - every `STAPLER_SQUAD_TEST_DIR` mutation goes through the state helpers
**Status in tree**: SATISFIED for `t.Setenv` and `envtest.NewIsolatedStateDir` (VERIFIED); one raw `os.Unsetenv` remains in the intentional exception.
**Acceptance Criteria**:
- No `t.Setenv("STAPLER_SQUAD_TEST_DIR"` and no `envtest.NewIsolatedStateDir(` call outside `home_env_test_helper_test.go` in `server/services`.
  - *Given* 115 former `envtest.NewIsolatedStateDir(t)` sites, *When* `grep -rn 'envtest.NewIsolatedStateDir(' server/services` runs, *Then* it prints nothing (only a comment on `home_env_test_helper_test.go:83` mentions the name, without the `(`).
**Files**: `server/services/*_test.go` (read-only)

##### Task 1.1.3a: Audit state-dir call sites (XS)
- `grep -rn 'envtest.NewIsolatedStateDir(' server/services` -> expect empty.
- `grep -rn 'STAPLER_SQUAD_TEST_DIR' server/services` -> expect helper, the exception test (`:48-50`), `backlog_service_test.go` ambient-env comments, and nothing else mutating.
- Confirm `backlog_service_test.go:66` `envtest.ClearAmbientStaplerSquadStateEnv()` is process-level restore, not a per-test mutation (comment at `:52` says so) and needs no lock.
- Note in the audit: `create_pull_request_test.go:640` sets `XDG_CONFIG_HOME`; `config.GetConfigDirForDir` does not read it (VERIFIED: `grep -n XDG config/config.go` is empty), so it is out of scope.
- Files: read-only.

#### Story 1.1.4: AC4 - `envtest.NewIsolatedStateDir` doc points `server/services` callers at `withIsolatedStateDir`
**Status in tree**: SATISFIED (VERIFIED, `git diff envtest/envtest.go`: +4 lines).
**Acceptance Criteria**:
- The doc comment on `NewIsolatedStateDir` in `envtest/envtest.go` names `withIsolatedStateDir` and `stateEnvMu`.
  - *Given* a developer reading `go doc ./envtest NewIsolatedStateDir`, *When* the output is printed, *Then* it ends with "Callers in server/services must use that package's withIsolatedStateDir helper instead".
**Files**: `envtest/envtest.go`

##### Task 1.1.4a: Verify doc text (XS)
- `go doc ./envtest NewIsolatedStateDir | tail -6`; expect the paragraph above.
- Files: read-only.

#### Story 1.1.5: AC5 - `TestWithIsolatedStateDir_ReleasesLockOnSuccessiveCalls`
**Status in tree**: SATISFIED (VERIFIED: defined at `home_env_test_helper_test.go:140`; passes 3x under `-race`).
**Acceptance Criteria**:
- The test acquires and releases `stateEnvMu` twice in sequence (one subtest per call, because `t.Cleanup` only fires at subtest end) and fails if the two returned dirs are equal or the env var is not set.
  - *Given* two subtests `first` and `second`, *When* each calls `withIsolatedStateDir(t)` / `withIsolatedStateDirPath(t, t.TempDir())`, *Then* the second call does not block and `first != second`.
**Files**: `server/services/home_env_test_helper_test.go`

##### Task 1.1.5a: Run the test and prove it can fail (XS)
- Run: `go test ./server/services/ -race -count=3 -v -run 'TestWithIsolatedStateDir_ReleasesLockOnSuccessiveCalls'`; expect `--- PASS` x3 and `ok`.
- Mutation check (scratch only, then revert via `git diff` clean-up): in a scratch copy comment out the `t.Cleanup` unlock in `acquireEnvLock`; expect the test to hit `-timeout=30s` with a goroutine dump in `sync.Mutex.Lock`. Do this with `go test -overlay` using a scratch overlay JSON so the working tree is never edited.
- Files: scratch overlay under the scratchpad directory only.

#### Story 1.1.6: AC6 - `go test ./server/services/ -race -count=5` shows no TempDir-cleanup / missing-dir failures
**Status in tree**: UNPROVEN. A run is in progress in another process. One earlier `-race` run on the migrated tree found the `ToInstanceData` data race (research/findings.md), fixed by Story 2.2.1.
**Acceptance Criteria**:
- Exit code 0, and neither `TempDir RemoveAll cleanup: directory not empty` nor `config.json.*.tmp: no such file or directory` appears as a test failure. (The latter also appears as a non-fatal WARN 76x per run on baseline; only failures count.)
  - *Given* the migrated tree plus the `GetDiffStats()` fix, *When* `go test ./server/services/ -race -count=5 -timeout=30m` runs on an otherwise idle machine, *Then* it prints `ok  github.com/tstapler/stapler-squad/server/services <N>s` and `grep -cE 'directory not empty|WARNING: DATA RACE|race detected' out.txt` prints `0`.
**Files**: none edited.

##### Task 1.1.6a: Collect and judge the AC6 run (S)
- Wait for the other process's run to finish; read its result. If it was started before Story 2.1/2.2 edits, it still counts only if it ran against the final `session/instance_serialization.go` (check file mtime vs run start); otherwise rerun once, alone, on an idle machine.
- Command: `go test ./server/services/ -race -count=5 -timeout=30m 2>&1 | tee <scratchpad>/ac6.txt`; then `grep -cE 'directory not empty|DATA RACE|race detected|panic:|FAIL' <scratchpad>/ac6.txt` expecting `0`, and the final line `ok`.
- If a failure appears: apply `code-root-cause-analysis` (state hypothesis, confirm) before changing anything; do not retry-until-green.
- Files: none.

##### Task 1.1.6b: goleak experiment to locate the leaking reader (S, diagnostic, not committed)
- Purpose: answer research/findings.md "Open" (which goroutine leaks). `go.uber.org/goleak v1.3.0` is already in `go.mod` and used in `analytics_store_test.go`.
- Create `server/services/zz_leak_probe_test.go` (scratch; sorts last by filename, so it runs after every other file's tests) containing one test that calls `goleak.Find()` (no ignore options, so every surviving goroutine is reported) and `t.Logf`s each leaked stack that contains `config.GetConfigDir` or `ConfigDirForDir` or `loadConfigWithDefaultFallback`.
- Run once, non-race, after Task 1.1.6a: `go test ./server/services/ -count=1 -run '.' -v 2>&1 | grep -A15 'leak'`. Record the top 3 leaking stacks and the test names that spawned them in a new section of `research/findings.md` (this is the only file edited).
- Delete the scratch file; confirm `git status --short | grep zz_leak` is empty.
- Files: `server/services/zz_leak_probe_test.go` (scratch, deleted), `project_plans/server-services-home-env-race/research/findings.md`.

#### Story 1.1.7: AC7 - `make lint` introduces no new violations
**Status in tree**: PARTIALLY PROVEN. Touched packages are clean under the same golangci-lint flags and the custom analyzers (VERIFIED, table above). Full `make lint` (which also does `proto-gen`, `ent-gen`, `server/web/dist`, `lint-shell`) has not been run.
**Acceptance Criteria**:
- `make lint` exits 0, and `make ready-complexity-gate` (the PR-only `new-from-rev=origin/main` gate from `lint.yml`) exits 0.
  - *Given* the final tree (go.sum reverted, regression test added), *When* `make lint` runs, *Then* the last line is `custom lint: ok` followed by golangci-lint `0 issues.` and exit code 0.
**Files**: none edited.

##### Task 1.1.7a: Run lint gates (S)
- `make lint 2>&1 | tail -20; echo rc=${PIPESTATUS[0]}` -> expect `rc=0`.
- `make ready-complexity-gate 2>&1 | tail -20` -> expect rc 0 (needs `dupl` from `make install-tools`; if missing, run `make install-tools` first and say so).
- Baseline comparison if lint fails: `git stash` is shared across worktrees, so do not use it; instead `git worktree add <scratch> origin/main` and run the same command there, then diff the output. A finding present on both is pre-existing, not "new".
- Run this after, not concurrently with, any test run (CPU contention causes load-sensitive timeouts; see commit `3c825969e`).
- Files: none.

---

## Phase 2: Out-of-AC changes in the tree

### Epic 2.1: Decide and isolate the two non-test changes

**Goal**: Each non-migration change is either removed or in its own commit with its own proof.

#### Story 2.1.1: go.sum decision
**As a** reviewer, **I want** `go.sum` to match `go.mod`, **so that** the PR carries no unrelated dependency churn.
**Decision**: Revert. VERIFIED: `go mod tidy -diff` removes exactly the 6 added lines; `go mod why -m` reports the module is not needed; none of the three modules is in `go.mod`. Origin of the change is UNVERIFIED (probable side effect of running a dev tool via `go run`/`go install` inside the module; not reproducible from here).
**Acceptance Criteria**:
- `git diff --stat -- go.sum go.mod` is empty and `go mod tidy -diff` prints nothing.
  - *Given* `go.sum` with 6 extra lines, *When* `git checkout -- go.sum` runs and then `go mod tidy -diff; echo rc=$?`, *Then* the output is `rc=0` with no diff, and `go build ./...` exits 0.
**Files**: `go.sum`

##### Task 2.1.1a: Revert go.sum and re-verify the build (XS)
- Only after the in-flight AC6 run has finished (it shares this working tree; extra `go.sum` lines are harmless to it, but do not modify the tree mid-run anyway).
- `git checkout -- go.sum`; `go mod tidy -diff; echo rc=$?`; `go build ./... ; echo rc=$?`.
- If `go build` now fails for a missing `go.sum` entry (it should not), stop and record which package pulls the module in, then reopen the decision.
- Files: `go.sum`.

#### Story 2.2.1: Production race fix in `Instance.ToInstanceData`
**As a** developer running `-race`, **I want** `ToInstanceData` to read `diffStats` under the manager lock, **so that** AC6 is reachable.
**Status in tree**: Fix present (`session/instance_serialization.go:203`, `git diff`: bare field read replaced by `ds := i.gitManager.GetDiffStats()`). Race evidence is one `-race` observation (research/findings.md); "pre-existing on `main`" is INFERRED (6 `-race` runs of baseline showed 0 races).
**Acceptance Criteria**:
- `ToInstanceData` copies `Added`, `Removed`, `Content` from the pointer returned by `GetDiffStats()` and no other bare read of `gitManager.diffStats` exists outside `git_worktree_manager.go`.
  - *Given* an `Instance` whose `gitManager.diffStats` is being replaced by `SetDiffStats(&git.DiffStats{Added: 3})` in one goroutine, *When* another goroutine calls `ToInstanceData()` 1000 times under `-race`, *Then* no `WARNING: DATA RACE` is reported and `data.DiffStats.Added` is only ever `0` or `3`.
**Files**: `session/instance_serialization.go`, `session/instance_serialization_test.go` (if Task 2.2.1b is accepted)

##### Task 2.2.1a: Review the fix (XS)
- `grep -rn '\.diffStats' session --include='*.go' | grep -v _test.go`; every hit must be inside `session/git_worktree_manager.go` under `gm.mu` (VERIFIED at planning time: lines 84-91, 236, 459-467, 475, 489-490; reads at 84/91 and 236 need a check that `gm.mu` is held by their callers).
- Confirm no import or behavior change: `go build ./session/ && go vet ./session/`.
- Files: read-only.

##### Task 2.2.1b: Add a `-race` regression test (S, recommended, not an AC)
- Add `TestToInstanceData_should_NotRace_When_SetDiffStatsRunsConcurrently` to `session/instance_serialization_test.go`, following the file's existing `TestToInstanceData_*` fixture style (see `:15`, `:212`), with no sleeps (use a `sync.WaitGroup` and a fixed iteration count) per the `deterministic-fast-tests` skill.
- Prove it detects the bug: `go test ./session -race -count=1 -run TestToInstanceData_should_NotRace` passes on the fixed tree; temporarily revert the one-line fix using `go test -overlay` (scratch overlay, not the working tree) and expect `WARNING: DATA RACE` and a failure.
- Files: `session/instance_serialization_test.go`.

---

## Phase 3: Commit and ship

### Epic 3.1: Split commits and open the PR

**Goal**: Reviewable, independently revertible commits; each green on its own.

#### Story 3.1.1: Commit split
**Commit plan** (stage by explicit path; never `git add -A`/`.`; this branch has only these changes, but the repo is shared):

| # | Message | Files | Why this order |
|---|---|---|---|
| C1 | `fix(session): read gitManager diffStats under its lock in ToInstanceData` | `session/instance_serialization.go` (+ `session/instance_serialization_test.go` if 2.2.1b accepted) | First, so every later commit passes `-race`; patch bump |
| C2 | `test(services): serialize HOME and STAPLER_SQUAD_TEST_DIR mutation behind withFakeHome/withIsolatedStateDir` | the 57 `server/services/*_test.go` files in `git diff --name-only` (including `home_env_test_helper_test.go`) + `envtest/envtest.go` | The migration; the `envtest` doc edit rides along because it only points at the new helper |
| C3 | `chore(sdd): planning artifacts for server-services-home-env-race` | `project_plans/server-services-home-env-race/` | Per `docs/how-to/commit-sdd-planning-artifacts.md` |
| - | (no commit) `go.sum` | reverted by Task 2.1.1a | Accidental; tidy removes it |

**Acceptance Criteria** (delivery, not one of the 7 ACs):
- `git log --oneline origin/main..HEAD` lists C1, C2, C3 in that order; `git diff --stat origin/main..HEAD -- go.sum go.mod` is empty; each commit builds.
  - *Given* the three commits, *When* `for c in $(git rev-list --reverse origin/main..HEAD); do git checkout -q $c && go build ./... && go vet ./session/ ./server/services/ || echo FAIL $c; done` runs from a scratch worktree, *Then* it prints no `FAIL`.

##### Task 3.1.1a: Create C1 (XS)
- `git add session/instance_serialization.go [session/instance_serialization_test.go]`; `git diff --cached --stat` must list only those files; commit with the message above plus the attribution trailer the session specifies.

##### Task 3.1.1b: Create C2 (XS)
- `git add envtest/envtest.go server/services/`; `git diff --cached --name-only | grep -vc -e '^server/services/.*_test\.go$' -e '^envtest/envtest.go$'` must print `0`; commit.

##### Task 3.1.1c: Create C3 (XS)
- `git add project_plans/server-services-home-env-race/`; commit. Includes `implementation/plan.md` and the `research/findings.md` updated by Task 1.1.6b.

##### Task 3.1.1d: Push and open the PR (XS)
- Push only `backlog/stapler-squad-fix-services-env-race-lint`. Open with `gh pr create` (ready for review, not draft: this repo overrides the global default). Title `fix: serialize HOME/STAPLER_SQUAD_TEST_DIR test mutation and lock ToInstanceData diffStats read` (a `fix:` prefix because C1 is a production fix). PR body: 3-4 sentences, AC checklist with the proof outputs from Epic 1.1, and an explicit caveat that baseline did not reproduce the failure (R1).
- Run `/sdd:6-verify` first if the owner's pipeline requires it; this plan does not replace it.

---

## Effort estimate

Bands and weights INFERRED (ESTIMATION.md); recalibrate after the first implementer run. Output share assumed 10% for these (mostly runs and reads), so 1 raw token is about 1.4 CU.

| Story | Tasks | Class | Raw tokens | Output share | CU | Wall-clock blocker |
|---|---|---|---|---|---|---|
| 1.1.1 AC1 | 1 | XS | 15k | 5% | 20k | - |
| 1.1.2 AC2 | 1 | XS | 15k | 5% | 20k | - |
| 1.1.3 AC3 | 1 | XS | 15k | 5% | 20k | - |
| 1.1.4 AC4 | 1 | XS | 8k | 5% | 10k | - |
| 1.1.5 AC5 | 1 | XS | 15k | 10% | 21k | - |
| 1.1.6 AC6 | 2 | S+S | 110k | 10% | 154k | in-flight run; `-race -count=5` wall-clock |
| 1.1.7 AC7 | 1 | S | 50k | 5% | 68k | `make lint` / `proto-gen` wall-clock |
| 2.1.1 go.sum | 1 | XS | 10k | 5% | 14k | - |
| 2.2.1 race fix | 2 | XS+S | 60k | 15% | 87k | - |
| 3.1.1 commits + PR | 4 | XS x4 | 50k | 10% | 70k | CI queue, human review |
| **Total** | **15** | | **348k** | | **~484k** | |

- Verification multiplier x1.5 (review of each story, one rerun of lint/test): **~730k CU**, Medium band (low end), informational only.
- Critical path: 5 agent waves. W0 wait for in-flight run; W1 `2.1.1a`; W2 parallel `1.1.1a-1.1.5a` + `2.2.1a/b`; W3 `1.1.7a` then `1.1.6a` (serialized on CPU); W4 `1.1.6b`; W5 commits + PR.
- Overrun checkpoint: pause and report if cumulative spend exceeds ~910k CU (730k + 25%) before Phase 3 starts.
- Wall-clock blockers: (1) in-flight `-race -count=5` run, owner: whoever started it; (2) full-package `-race` runs and `make lint`, bounded by the machine; (3) CI queue and PR review, owner: Tyler.

---

## Risks

| ID | Risk | Evidence | Mitigation |
|---|---|---|---|
| R1 | Mutexes may not address the actual failure. Go forbids `t.Setenv` in parallel tests and runs serial top-level tests one at a time, so two mutators are rarely concurrent in the first place; the cross-test writer is more likely a leaked goroutine reading env live, which no lock covers. A green AC6 is therefore weak causal evidence. INFERRED from `config/config.go:124-132` and research/findings.md "Design limit"; baseline never reproduced. | INFERRED | Task 1.1.6b locates the leak; PR body states the caveat; follow-up plan for approach B. |
| R2 | Helper header comment (`home_env_test_helper_test.go:10-26`) claims two *serial* tests can mutate HOME concurrently. That is not how Go schedules serial tests (INFERRED); the comment may mislead the next reader. | INFERRED | Flag in the audit note from 1.1.2a; reword only if the owner approves (it is a comment edit in an already-migrated file). |
| R3 | Lock order home-then-state is by convention only; nothing enforces it. A new test calling `withIsolatedStateDir` then `withFakeHome` could deadlock against another. | VERIFIED no current violation if Task 1.1.2a's grep is clean | Task 1.1.2a; the comment at `:85-86` documents the order. |
| R4 | `t.TempDir()` is created before the lock is acquired in `withFakeHome`, so its `RemoveAll` runs after unlock (LIFO). The lock does not cover directory removal. INFERRED. | INFERRED | Record only; fixing ordering has no evidence of benefit. |
| R5 | Full-package `-race` runs are load-sensitive (recent commit `3c825969e` fixed timeouts of that kind). A failure under contention may be unrelated. | VERIFIED by commit message | Run AC6 and AC7 serially on an idle machine; judge failures by name against baseline. |
| R6 | AC6 run in progress started on a tree that may predate later edits. | VERIFIED process exists per caller | Task 1.1.6a checks mtime vs run start. |

## Review amendments (adversarial + architecture reviews: both CONCERNS, 0 blockers)

Source: `adversarial-review.md`, `architecture-review.md` in this directory. These supersede the task text above where they conflict.

1. **AC6 result (resolved):** `go test ./server/services/ -race -count=5` on the migrated tree returned `ok ... 1099.157s`. Baseline `main` never reproduced the failure (10/10 green), so green is necessary, not causal proof.
2. **Framing:** the mutexes serialize operations that cannot overlap (serial tests are sequential; `t.Setenv` panics in parallel tests). Commit/PR type is `test:`/`refactor:`, not `fix:`; the PR body states baseline did not reproduce. Correct the overclaiming header comment in `home_env_test_helper_test.go:10-26`.
3. **Lock-order inversion exists (corrects R3):** `session_service_create_settings_env_test.go:41-42` calls `withIsolatedStateDir` before `withFakeHome`. Swap them; enforce home-then-state in `acquireEnvLock` with `t.Fatalf` instead of a grep audit.
4. **Helper cleanup:** fold `mu`+`holders` into one `envLock` type with `acquire(t)`; reconcile the "at most once per t" doc with re-entrancy; create `t.TempDir()` after taking the lock; add tests for subtest-first-then-parent.
5. **goleak probe (Task 1.1.6b):** run in `TestMain` after `m.Run()`, not as a serial test.
6. **Audit scope:** add `STAPLER_SQUAD_INSTANCE`; withFakeHome call sites measured at 60, not 33.
7. **Regression test 2.2.1b:** needs a named fixture and a check that nothing mutates `*DiffStats` in place.
8. **Guard (optional, owner decision):** parser-based test or `tools/lint` analyzer rejecting bare `t.Setenv("HOME"|"STAPLER_SQUAD_TEST_DIR")`.
9. **Commit split:** C2 becomes C2a helper+tests+envtest doc, C2b HOME migration, C2c state-dir migration; per-commit proof runs targeted `-race` tests plus lint, not only build/vet.

## Pre-mortem amendments (pre-mortem.md: 2 P1, 5 P2, 2 P3) — supersede earlier text

- **P1 #1 (reader side untouched):** before merge, file a follow-up backlog item for the leaked-goroutine reader side; PR body says "not shown fixed — baseline never reproduced"; Task 1.1.6b's leak detector registers after `t.Setenv` so it runs at cleanup.
- **P1 #2 (AC6 non-discriminating):** AC6 is recorded as a no-regression check pinned to a tree hash; rerun `-race -count=5` after the last helper edit (the 1099s `ok` predates amendments 3-4 and does not count for the final tree).
- **P2 #3:** goleak probe groups by `created by`, not by `GetConfigDir` stack.
- **P2 #4 (supersedes review amendment 3's `t.Fatalf`):** do not `t.Fatalf` on lock-order inversion; it fails `TestEnvLock_ReentrantWithinOneTestAndSubtests` and the unreachable-deadlock case. Use `t.Errorf` or document only; still swap `session_service_create_settings_env_test.go:41-42`.
- **P2 #5 (supersedes amendment 8):** the parser-based guard test (bare `t.Setenv`/`os.Setenv` of HOME or STAPLER_SQUAD_TEST_DIR, and `envtest.NewIsolatedStateDir(`) is mandatory in this PR.
- **P2 #6:** rebase onto `origin/main` first (14 commits behind, 14 test files differ incl. `TestMain` file); re-apply the migration mechanically if conflicts; re-run AC1/AC3 greps and the guard after rebase.
- **P2 #7:** ship the `ToInstanceData` fix as its own `fix(session):` PR merged first (squash merges make in-PR commit independence false).
- **P3 #8:** regression test at `-count=20` with overlay-revert check; grep for in-place `DiffStats` writers; note sibling bare `i.claudeSession` read at `instance_serialization.go:212-213`.
- **P3 #9:** `envLock` stores the holder `*testing.T`, not a name prefix; add subtest-first and cleanup tests.

## Items not verified in this session
- ~~AC6 result~~ — now run; see amendment 1.
- Full `make lint` and `make ready-complexity-gate` (only equivalent flags on 3 packages were run).
- Which goroutine leaks (Task 1.1.6b).
- Why `go.sum` changed.
- Whether any test in the 57 migrated files changed assertions rather than only setup lines (only counts of replaced lines were taken; a diff read of a sample is part of Task 1.1.1a/1.1.3a).
