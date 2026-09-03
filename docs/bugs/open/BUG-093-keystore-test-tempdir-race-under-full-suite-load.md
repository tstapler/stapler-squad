# BUG-093: `TestSetIdentity_FailsLoud_When_NoKeyringBackendAvailable` flakes under full-suite load

**Status**: Open
**Found**: 2026-09-01, during `/sdd:6-verify` Layer 3 correctness gate for `async-session-creation`
**Package**: `session/sshremote` (`keystore_test.go`) — unrelated to `async-session-creation`'s own diff

## Symptom

```
=== RUN   TestSetIdentity_FailsLoud_When_NoKeyringBackendAvailable
    keystore_test.go:229: walking config dir /tmp/stapler-squad-test-<pid>: open /tmp/stapler-squad-test-<pid>/worktrees/test-new-worktree_<id>: no such file or directory
--- FAIL: TestSetIdentity_FailsLoud_When_NoKeyringBackendAvailable (0.48s)
```

Fails only when run as part of the full `go test ./...` suite; passes 3/3 in isolation
(`go test ./session/sshremote/... -run TestSetIdentity_FailsLoud_When_NoKeyringBackendAvailable -count=3`).

## Root-cause hypothesis (not yet confirmed)

`keystore_test.go:229` walks a config directory tree (`/tmp/stapler-squad-test-<pid>`)
that another concurrently-running test (or a different session on this same shared
machine, which runs many parallel `stapler-squad` worktree-based sessions) creates and
tears down a `worktrees/test-new-worktree_*` subdirectory under. The walk loses the race
against that subdirectory's removal. This machine runs dozens of concurrent Claude Code
sessions each executing their own test suites against worktrees of the same repo, which
plausibly explains why this only reproduces under full-suite/full-machine load.

## Evidence this is unrelated to `async-session-creation`

- `git log -- session/sshremote/keystore_test.go` shows it was last touched by
  `ff4830cbd` ("SSH remote workspace support"), not by `async-session-creation`'s
  implementation commit (`365699d5e`), which touches no files under `session/sshremote/`.
- Passes reliably in isolation.

## Suggested fix direction

Either scope the config-dir walk in `keystore_test.go` to a directory tree the test
creates and owns exclusively (not a shared `/tmp/stapler-squad-test-*` prefix that other
concurrent processes may also be writing into), or make the walk tolerant of a
disappearing subdirectory (treat `ENOENT` on a walked path as "not present," not a hard
error) if that's consistent with the code path under test.
