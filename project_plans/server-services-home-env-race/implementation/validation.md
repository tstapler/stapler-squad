# Validation Plan: server-services-home-env-race

**Date**: 2026-10-13
**Complexity**: 2
**Inputs**: `plan.md` (including "Review amendments", which supersede earlier task text), `requirements.md`, `research/findings.md`, `adversarial-review.md`, `architecture-review.md`.

Evidence labels: **VERIFIED** = command run in this validation session, output seen. **RECORDED** = result taken from `plan.md`, not re-run here. **PROPOSED** = check does not exist in the tree yet.

All commands run from the worktree root. Full-package test runs are deliberately absent from the "run now" column; AC6 is a recorded result plus a rerun recipe.

## Happy Path Scenario

Given the migrated tree (57 `server/services` test files, `envtest/envtest.go`, one `session` accessor swap) with `go.sum` reverted, when a developer runs the `server/services` test binary under `-race -count=5`, then every test that sets `HOME` or `STAPLER_SQUAD_TEST_DIR` does so through `withFakeHome*` / `withIsolatedStateDir*`, no `TempDir RemoveAll cleanup: directory not empty` or `config.json.*.tmp: no such file or directory` failure appears, no `WARNING: DATA RACE` is reported, and `make lint` is clean.

Limit on what the scenario proves: baseline `main` passed 10/10 full runs and 45/45 targeted runs (research/findings.md), so a green run is necessary but not discriminating (adversarial-review concern 3; plan R1). Every AC6 verdict below carries that caveat.

## Requirement → Check Mapping (one row per acceptance criterion, minimum one concrete check each)

| AC | Requirement (requirements.md) | Check ID | Exact command | Expected output | In-tree status |
|---|---|---|---|---|---|
| AC1 | Every `t.Setenv("HOME", ...)` uses `withFakeHome*` or holds `homeEnvMu`; exception commented | V1.1 | `grep -rn 't.Setenv("HOME"' server/services \| grep -vE ':[0-9]+:\s*//'` | exactly 3 lines: `home_env_test_helper_test.go:52`, `home_env_test_helper_test.go:73`, `session_service_search_index_test.go:42`. (Raw grep without the comment filter prints 4: `home_env_test_helper_test.go:25` is a comment. `plan.md` "exactly 3" is only true with the filter.) | SATISFIED (VERIFIED) |
| AC1 | same | V1.2 | `grep -rnE 'os\.(Setenv\|Unsetenv)\("HOME"' server/services; echo rc=$?` | no match lines, `rc=1` | SATISFIED (VERIFIED) |
| AC1 | same | V1.3 | `grep -nE 'deliberately does NOT use withFakeHome\|Intentional exception' server/services/session_service_search_index_test.go` | `34:// This test deliberately does NOT use withFakeHome/homeEnvMu ...` and `41:	// Intentional exception to the withFakeHome rule: ...` | SATISFIED (VERIFIED) |
| AC1 | same | V1.4 | `grep -rn 'withFakeHome\(AsFile\|Path\)\?(' server/services \| grep -v 'func \|home_env' \| wc -l` | `60` (plan's "33" is stale; adversarial-review concern 4) | SATISFIED (VERIFIED) |
| AC2 | `stateEnvMu`, `withIsolatedStateDir`, `withIsolatedStateDirPath` mirror the HOME helpers | V2.1 | `grep -nE 'var stateEnvMu\|^func withIsolatedStateDir(Path)?\(' server/services/home_env_test_helper_test.go` | `87:var stateEnvMu sync.Mutex`, `94:func withIsolatedStateDir(t *testing.T) string`, `101:func withIsolatedStateDirPath(t *testing.T, dir string) string` | SATISFIED (VERIFIED) |
| AC2 | same | V2.2 | `go test ./server/services/ -race -count=1 -v -run 'TestEnvLock_ReentrantWithinOneTestAndSubtests\|TestTestRemoteConnection_TwoServiceInstances'` | `--- PASS` for both, `ok`. Covers the re-entrancy that migration v1 lacked (deadlocked at 20m timeout). | SATISFIED (VERIFIED, 2.3s) |
| AC2 | Constraint: fixed lock order home then state | V2.3 | `for f in $(comm -12 <(grep -l 'withFakeHome' server/services/*_test.go \| sort) <(grep -l 'withIsolatedStateDir' server/services/*_test.go \| sort)); do echo $f; grep -nE 'withFakeHome\|withIsolatedStateDir' $f \| head -4; done` | **Currently shows one inversion**: `session_service_create_settings_env_test.go:41` `withIsolatedStateDir(t)` then `:42` `withFakeHome(t)`. After amendment 3 (swap the two lines) that file must list `withFakeHome` first. Grep is per-file, not per-test, so also read the other 4 dual-use files' multi-test hits. | NOT SATISFIED (VERIFIED defect, plan R3 is wrong; amendment 3 not yet applied) |
| AC2 | same, enforced | V2.4 | PROPOSED: `TestAcquireEnvLock_should_FailFast_When_StateHeldBeforeHome` in `home_env_test_helper_test.go` | Needs a seam because `t.Fatalf` ends the test: run the inverted sequence in a child `go test -run` via `os/exec` of `os.Args[0]` with a marker env var and assert non-zero exit plus the "lock order" message. If a seam is declined, V2.3 stays the only gate and R3 stays open. | PROPOSED (amendment 3b) |
| AC3 | Every `t.Setenv("STAPLER_SQUAD_TEST_DIR", ...)` uses the state helpers | V3.1 | `grep -rn 't.Setenv("STAPLER_SQUAD_TEST_DIR"' server/services` | one line: `home_env_test_helper_test.go:105` | SATISFIED (VERIFIED) |
| AC3 | `envtest.NewIsolatedStateDir` callers migrated (requirements "Constraints") | V3.2 | `grep -rn 'envtest.NewIsolatedStateDir(' server/services; echo rc=$?` | no lines, `rc=1` | SATISFIED (VERIFIED) |
| AC3 | Other env vars steering `GetConfigDirForDir` are not mutated unguarded (amendment 6) | V3.3 | `grep -rnE 'Setenv\("STAPLER_SQUAD_(INSTANCE\|WORKSPACE_MODE\|TMUX_SOCKET)' server/services; echo rc=$?` | no lines, `rc=1` | SATISFIED (VERIFIED) |
| AC3 | Raw `os.Unsetenv`/`os.Setenv` of the state var outside the helpers | V3.4 | `grep -rnE 'os\.(Setenv\|Unsetenv)\("STAPLER_SQUAD_TEST_DIR"' server/services` | only `session_service_search_index_test.go:49-50` (inside the commented exception test). Literal AC3 wording covers `t.Setenv` only, so this is accepted; recorded so it is not a surprise. | SATISFIED with noted exception (VERIFIED) |
| AC4 | `NewIsolatedStateDir` doc points `server/services` callers at `withIsolatedStateDir` | V4.1 | `go doc ./envtest NewIsolatedStateDir \| tail -8` | ends with: "Callers in server/services must use that package's withIsolatedStateDir helper instead: it additionally serializes the process-global variable behind stateEnvMu, which this helper cannot do from outside the package." | SATISFIED (VERIFIED) |
| AC4 | doc edit is the only change to `envtest` | V4.2 | `git diff --stat -- envtest/envtest.go` | `1 file changed`, 4 insertions, 0 deletions (RECORDED: `+4`) | SATISFIED (RECORDED) |
| AC5 | `TestWithIsolatedStateDir_ReleasesLockOnSuccessiveCalls` exists and passes | V5.1 | `go test ./server/services/ -race -count=3 -v -run 'TestWithIsolatedStateDir_ReleasesLockOnSuccessiveCalls'` | three `--- PASS: TestWithIsolatedStateDir_ReleasesLockOnSuccessiveCalls` blocks (with `/first`, `/second`), then `ok` | SATISFIED (VERIFIED at `-count=1 -race`; `-count=3` RECORDED) |
| AC5 | Test can fail (not vacuous) | V5.2 | PROPOSED mutation check from plan Task 1.1.5a: scratch overlay JSON (scratchpad dir) that removes the `t.Cleanup(... mu.Unlock())` in `acquireEnvLock`, then `go test -overlay <overlay.json> ./server/services/ -run TestWithIsolatedStateDir_ReleasesLockOnSuccessiveCalls -timeout=30s` | `panic: test timed out after 30s` with a goroutine blocked in `sync.(*Mutex).Lock` from `acquireEnvLock`. Working tree is never edited. | PROPOSED (not run) |
| AC5 | Sibling tests for HOME and re-entrancy exist | V5.3 | `go test ./server/services/ -race -count=1 -v -run 'TestWithFakeHome_ReleasesLockOnSuccessiveCalls\|TestEnvLock_ReentrantWithinOneTestAndSubtests'` | both `--- PASS` | SATISFIED (VERIFIED) |
| AC5 | Gap from architecture review: subtest-acquires-first-then-parent | V5.4 | PROPOSED: add `t.Run("sub", ...)`-first case to `TestEnvLock_Reentrant...` (amendment 4), then rerun V5.3 | `--- PASS`, no timeout | PROPOSED |
| AC6 | `go test ./server/services/ -race -count=5` has no TempDir-cleanup / missing-dir failures | V6.1 | `go test ./server/services/ -race -count=5 -timeout=30m 2>&1 \| tee <scratchpad>/ac6.txt; grep -cE 'directory not empty\|DATA RACE\|race detected\|panic:\|^FAIL' <scratchpad>/ac6.txt` | last package line `ok  github.com/tstapler/stapler-squad/server/services <N>s`, grep count `0`. Run once, alone on an idle machine, after the final tree is fixed (lint and test runs contend for CPU; commit `3c825969e`). | SATISFIED per RECORDED run `ok ... 1099.157s` (plan amendment 1); not re-run here per instructions |
| AC6 | The run covered the final `session/instance_serialization.go` | V6.2 | `stat -c '%y' session/instance_serialization.go` compared with the run's start time | file mtime earlier than run start; otherwise rerun V6.1 (plan R6 / Task 1.1.6a) | UNVERIFIED (mtime comparison not done) |
| AC6 | The one race seen during AC6 is fixed at its source | V6.3 | `go test ./session/ -race -count=1 -v -run 'TestToInstanceData'` | all `--- PASS`, `ok` | SATISFIED (VERIFIED, existing `TestToInstanceData_*` only) |
| AC6 | No bare read of `gitManager.diffStats` remains outside the manager | V6.4 | `grep -rn '\.diffStats' session --include='*.go' \| grep -v _test.go \| grep -v git_worktree_manager.go; echo rc=$?` | no lines, `rc=1` | SATISFIED (VERIFIED) |
| AC6 | Regression test for the `ToInstanceData` race (amendment 7; recommended, not an AC) | V6.5 | PROPOSED: `TestToInstanceData_should_NotRace_When_SetDiffStatsRunsConcurrently` in `session/instance_serialization_test.go`; `go test ./session/ -race -count=1 -run TestToInstanceData_should_NotRace` | `ok`. Mutation proof: with the one-line fix reverted through `-overlay`, expect `WARNING: DATA RACE` and FAIL. No fixture exists yet (`grep -n 'NotRace\|DiffStats' session/instance_serialization_test.go` is empty). Must name the fixture (e.g. the constructor `TestToInstanceData_should_ReturnCurrentFields_When_ActorAlreadyStopped` uses at `:212`). Pointee check: `grep -rnE 'diffStats\.(Added\|Removed\|Content)\s*=' session server --include='*.go'` found no in-place mutation outside tests. | PROPOSED |
| AC6 | Verdict wording (adversarial concern 3) | V6.6 | PR checklist line, no command | reads "AC6: green, not falsifiable on baseline (baseline did not reproduce)"; must not claim "race fixed" | PROPOSED |
| AC7 | `make lint` introduces no new violations | V7.1 | `CGO_ENABLED=0 golangci-lint run --enable=nilnil,staticcheck,ineffassign,govet ./server/services/ ./envtest/ ./session/ 2>&1 \| tail -3` | `0 issues.` (the `unknown linters in //nolint` warning is pre-existing text in untouched files) | SATISFIED for touched packages (VERIFIED) |
| AC7 | Full gate as CI runs it | V7.2 | `make lint 2>&1 \| tail -20; echo rc=${PIPESTATUS[0]}` | `custom lint: ok`, golangci-lint `0 issues.`, `rc=0`. `bin/linter` does not exist yet in this worktree, so the first run builds it. | NOT RUN (plan lists it as unverified) |
| AC7 | PR-only new-code gate | V7.3 | `make ready-complexity-gate 2>&1 \| tail -20; echo rc=$?` | `rc=0` (needs `dupl` from `make install-tools`; say so if installed first) | NOT RUN |
| AC7 | Static check on migrated packages | V7.4 | `go vet ./server/services/ ./session/ ./envtest/; echo rc=$?` | no output, `rc=0` | SATISFIED (VERIFIED) |
| AC7 | `go.sum` carries no accidental churn that fails module checks | V7.5 | `git checkout -- go.sum; go mod tidy -diff >/dev/null 2>&1; echo rc=$?; go build ./... ; echo rc=$?` | `rc=0` then `rc=0`. Currently `go mod tidy -diff` returns `rc=1` (6 extra `go.sum` lines for go-runewidth, tablewriter, uniseg) and `git diff --stat -- go.sum` shows `6 insertions(+)`. | NOT SATISFIED until Task 2.1.1a reverts (VERIFIED defect) |

## Unit / Error / Integration Layer (Step 1 design, test-infrastructure variant)

The deliverable is test infrastructure, so "unit" means a test of the helper itself and "integration" means an existing production-package test that exercises the helper through real call paths. Names use the repo's existing style where the test exists; PROPOSED rows follow `methodName_should_Expected_When_Condition`.

| Requirement | Test File | Test Name | Type | Scenario | Status |
|---|---|---|---|---|---|
| AC2/AC5 | `server/services/home_env_test_helper_test.go` | `TestWithIsolatedStateDir_ReleasesLockOnSuccessiveCalls` | Unit | Happy: two sequential acquires, distinct dirs, env set | in tree, passes |
| AC1 | same | `TestWithFakeHome_ReleasesLockOnSuccessiveCalls` | Unit | Happy: HOME lock released between subtests | in tree, passes |
| AC2 | same | `TestEnvLock_ReentrantWithinOneTestAndSubtests` | Unit | Error path: would self-deadlock without re-entrancy | in tree, passes |
| AC2 | same | `TestEnvLock_should_NotDeadlock_When_SubtestAcquiresBeforeParent` | Unit | Error path: gap from architecture review | PROPOSED |
| AC2 | same | `TestAcquireEnvLock_should_FailFast_When_StateAcquiredBeforeHome` | Unit (child process) | Error path: lock-order inversion | PROPOSED |
| AC2/AC3 | `server/services/remote_service_test.go` | `TestTestRemoteConnection_TwoServiceInstances_DoNotShareAPoolEntry` | Integration | Real helper call twice in one test (the v1 deadlock) | in tree, passes |
| AC1/AC3 | `server/services/env_mutation_guard_test.go` (new) | `TestServicesTests_should_NotCallBareEnvMutation_When_ParsedWithGoParser` | Unit | Happy: `go/parser` over `*_test.go`, fails on bare `t.Setenv("HOME"\|"STAPLER_SQUAD_TEST_DIR")` or `envtest.NewIsolatedStateDir(` outside the helper file; allow-list `TestNewSessionService_TestMode_NeverTouchesRealSearchIndex` by name | PROPOSED (amendment 8, owner decision) |
| AC1/AC3 | same | `TestServicesTests_should_Report_When_FixtureHasBareSetenv` | Unit | Error path: guard fires on an in-memory source string | PROPOSED |
| AC6 | `session/instance_serialization_test.go` | `TestToInstanceData_should_NotRace_When_SetDiffStatsRunsConcurrently` | Integration (`-race`) | Concurrent `SetDiffStats` vs 1000 `ToInstanceData` calls; `Added` only ever 0 or 3; `sync.WaitGroup`, no sleeps | PROPOSED |

## UX Acceptance Tests

Not applicable: no user-facing surface (`design/ux.md` absent; test-only change plus one accessor swap).

## Migration Test

N/A. `plan.md` Migration Plan: "Not applicable (no schema or data change)". The `migration_should_be_reversible` test is not required. Delivery reversibility is checked separately by the per-commit loop in the next section.

## Delivery Checks (not ACs; from plan Story 3.1.1 and amendment 9)

| Check ID | Command | Expected |
|---|---|---|
| D1 | `git diff --stat origin/main..HEAD -- go.sum go.mod` | empty |
| D2 | In a scratch worktree, for each commit from `git rev-list --reverse origin/main..HEAD`: `go build ./... && go vet ./session/ ./server/services/` | no `FAIL <sha>` (build and vet only; say so, do not call it "green") |
| D3 | Per commit: C1 `go test ./session/ -race -count=1 -run TestToInstanceData`; C2a/b/c `go test ./server/services/ -race -count=1 -run 'WithIsolatedStateDir\|WithFakeHome\|EnvLock'`, plus the V7.1 lint line | `ok` each |
| D4 | `git diff --cached --name-only \| grep -vc -e '^server/services/.*_test\.go$' -e '^envtest/envtest.go$'` before committing C2 | `0` |

## Test Stack
- **Unit**: Go `testing` (`-race`), table-free helper tests, `go/parser` for the proposed guard.
- **Integration**: existing `server/services` tests that call the helpers (about 60 `withFakeHome*` and 139 `withIsolatedStateDir*` call sites, measured); `session` package `-race` tests.
- **Static**: `golangci-lint` with the `make lint` flag set, `tools/lint` custom analyzers via `make lint-custom`, `go vet`, `go mod tidy -diff`.
- **E2E / UX**: none.

## Coverage Targets and How to Measure

| Stack | Command | Target |
|---|---|---|
| Go (touched logic only: `ToInstanceData` accessor path) | `go test ./session/ -coverprofile=<scratchpad>/c.out -run TestToInstanceData && go tool cover -func=<scratchpad>/c.out \| grep -E 'ToInstanceData\|total'` | `ToInstanceData` reported, no regression; line coverage of the changed statement is the only meaningful target (package-wide 80% is unaffected by a 3-line change) |

- All 7 ACs have at least one concrete check (table above).
- Helper code (`acquireEnvLock`, 3 helper entry points): happy path and re-entrancy tests in tree; lock-order and subtest-first error paths PROPOSED.
- Drift guard: V1.1/V3.1/V3.2 are one-time greps unless the PROPOSED parser test lands; without it the next bare `t.Setenv` is undetected (architecture-review concern 6).

## Summary

| Metric | Count |
|---|---|
| Acceptance criteria | 7 |
| ACs with at least 1 concrete check | 7 of 7 |
| Check rows in the mapping table | 29 (V1.1-V1.4, V2.1-V2.4, V3.1-V3.4, V4.1-V4.2, V5.1-V5.4, V6.1-V6.6, V7.1-V7.5) |
| Satisfied, re-verified in this session | 17 |
| Satisfied by recorded result only (not re-run) | 2 (V4.2, V6.1) |
| Proposed, not in tree | 5 (V2.4, V5.2, V5.4, V6.5, V6.6) |
| Unverified | 1 (V6.2, mtime vs run start) |
| Failing in the current tree | 2 (V2.3 lock-order inversion, V7.5 go.sum churn) |
| Not run (full `make lint`, complexity gate) | 2 (V7.2, V7.3) |
| Unit test cases (helper): in tree / proposed | 3 / 4 |
| Integration test cases: in tree / proposed | 1 / 1 |
| UX acceptance tests | 0 (N/A) |
| Migration test | N/A |
