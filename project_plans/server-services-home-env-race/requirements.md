# Requirements: server/services process-global env-var test race

Backlog item 33c80d25-ab60-4fbf-ba66-b9a1ae88f8a1.

## Problem
`HOME` and `STAPLER_SQUAD_TEST_DIR` are process-global. Tests in `server/services` mutate them with bare
`t.Setenv`, so a background goroutine from one test can resolve a path under another test's value and write
into a directory that test is removing (`TempDir RemoveAll cleanup: directory not empty`, or
`open .../config.json.N.tmp: no such file or directory`).

## Acceptance criteria
1. Every `t.Setenv("HOME", ...)` in `server/services` tests goes through `withFakeHome`/`withFakeHomeAsFile` or holds `homeEnvMu`; the one intentional exception (`session_service_search_index_test.go`) is commented.
2. Add `stateEnvMu` and `withIsolatedStateDir(t)` / `withIsolatedStateDirPath(t, dir)` to `home_env_test_helper_test.go`, mirroring `homeEnvMu`/`withFakeHome`.
3. Every `t.Setenv("STAPLER_SQUAD_TEST_DIR", ...)` goes through those helpers.
4. `envtest.NewIsolatedStateDir` doc comment points `server/services` callers at `withIsolatedStateDir`.
5. Add `TestWithIsolatedStateDir_ReleasesLockOnSuccessiveCalls`.
6. `go test ./server/services/ -race -count=5` shows no TempDir-cleanup / missing-dir failures.
7. `make lint` introduces no new violations.

## Constraints / risks
- Lock held until `t.Cleanup`: nested or subtest re-acquisition of the same mutex deadlocks; two mutexes need a fixed order (home, then state).
- `envtest.NewIsolatedStateDir` callers in this package bypass the lock too and must migrate for the lock to mean anything.
