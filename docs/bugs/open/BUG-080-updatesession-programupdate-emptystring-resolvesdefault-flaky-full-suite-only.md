# BUG-080: `TestUpdateSession_ProgramUpdate_EmptyString_ResolvesDefault` fails under full `server/services` package run

## Summary

`go test ./server/services/... -race -count=1` fails
`TestUpdateSession_ProgramUpdate_EmptyString_ResolvesDefault`
(`server/services/session_service_program_test.go:184`) with:

```
Error:      Should NOT be empty, but was
Test:       TestUpdateSession_ProgramUpdate_EmptyString_ResolvesDefault
Messages:   test assumption: a default program must be configured
```

Running the test in isolation — `go test ./server/services/... -run
TestUpdateSession_ProgramUpdate_EmptyString_ResolvesDefault -race -count=1` —
passes reliably, with or without `-race`. Both `claude` and `aider` are on
`PATH` in this environment, so `config.DefaultConfig()`'s program-detection
should always populate `DefaultProgram` — the empty value only appears when
run alongside the rest of the package.

## Reproduction

```
go test ./server/services/... -race -count=1
# --- FAIL: TestUpdateSession_ProgramUpdate_EmptyString_ResolvesDefault
go test ./server/services/... -run TestUpdateSession_ProgramUpdate_EmptyString_ResolvesDefault -race -count=1
# --- PASS
```

Reproduces consistently (2/2 full-package runs during this investigation),
unlike some of this repo's other full-suite-only flakes — but only in
full-package mode, never isolated.

## Suspected root cause

The test reads `config.LoadConfig().DefaultProgram` after calling
`UpdateSession`, expecting `config.DefaultConfig()`'s PATH-based
program-detection to have populated it. `config.GetConfigDirForDir` (see
`config/config.go:119-133`) resolves the config directory from the
`STAPLER_SQUAD_TEST_DIR` environment variable when set, falling back to the
real `~/.stapler-squad` otherwise. Since `go test` runs top-level tests in a
package sequentially in one process (absent `t.Parallel()`), an earlier test
in `server/services` that sets/unsets `STAPLER_SQUAD_TEST_DIR` via
`os.Setenv`/`os.Unsetenv` instead of `t.Setenv` (which auto-restores at test
end) — or that writes a zero-value `config.Config` to a *shared* test
directory another test also reads from — would leave a stale or pointed-at-
the-wrong-directory config state for this test to read. Not confirmed by
tracing the exact offending test (the package has hundreds of tests reading/
writing config); this is an inference from the reproduction shape (isolated:
pass; full package: fail) plus `GetConfigDirForDir`'s env-var-based
resolution, not a captured root cause.

## Why not fixed here

Discovered while running this repo's standard verification commands
(`go test ./server/services/... -race -count=1`) for an unrelated change —
Story 6.1.1 of ssh-remote-workspaces (Settings → Remotes UI: `RemoteService`
proto/handler additions in `server/services/remote_service.go`,
`session/sshremote/keygen.go`, and the new frontend surface under
`web-app/src/app/settings/remotes/` and
`web-app/src/components/settings/`). None of those files touch
`session_service_program_test.go`, `config/config.go`, or any config-dir
isolation helper — confirmed via `git status`/`git diff` scope before filing.

Root-causing properly requires auditing every test in the (large)
`server/services` package for `STAPLER_SQUAD_TEST_DIR`
`os.Setenv`/`os.Unsetenv` usage instead of `t.Setenv`, and/or tests that call
`config.SaveConfig` against a directory another test also depends on — a
blast radius well beyond this backlog item's scope (a Settings UI feature)
to take on mid-implementation.

Per `.claude/rules/fix-flaky-tests-dont-defer.md`, filing rather than
silently re-excusing as "known pre-existing, unrelated" — this is the first
time it's been captured in writing rather than waved off in review.

## Recurrence: 2026-08-17

Reproduced 3/3 times while re-verifying `go test ./server/services/... -race
-count=1` for the same backlog item (ssh-remote-workspaces Phase 6 Epic 6.3,
Story 6.3.1 — feature registry + e2e tests: `docs/registry/features/`,
`tools/scanner/`, `tests/e2e/`, plus small fixes to
`web-app/src/lib/hooks/useRemotesService.ts`,
`web-app/src/lib/contexts/OmnibarContext.tsx`, and
`web-app/src/components/sessions/{SessionRow,SessionCard}.tsx` found while
writing that spec). None of this session's changed files touch
`session_service_program_test.go`, `config/config.go`, or any config-dir
isolation helper — confirmed via `git diff --stat` before appending this
note, same as the original report.

Same signature every time, exact match to the original report — only
`TestUpdateSession_ProgramUpdate_EmptyString_ResolvesDefault` failed, with
the identical `"Should NOT be empty, but was" / "test assumption: a default
program must be configured"` message, in all three consecutive full-package
runs. No other test failed in any of the three runs. This does NOT broaden
the bug's scope beyond what's already documented above — logging the
recurrence per this bug's own convention (see BUG-051's "Recurrence log"
section for the same pattern) rather than re-excusing it as "known,
unrelated" without writing it down.

```
go test ./server/services/... -race -count=1
# --- FAIL: TestUpdateSession_ProgramUpdate_EmptyString_ResolvesDefault (x3, identical)
```
