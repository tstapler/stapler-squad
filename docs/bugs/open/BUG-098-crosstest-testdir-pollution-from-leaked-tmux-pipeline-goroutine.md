# BUG-098: Leaked SessionService pipeline goroutine can pollute a later test's `STAPLER_SQUAD_TEST_DIR` [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-09-03, while running `go test ./session/... ./server/...` repeatedly to verify an
unrelated fix (`session/streamhub` resize-nudge consolidation + `require.Eventually` timeout bumps in
`session_creation_pipeline_test.go`/`session_service_retry_test.go`)
**Impact**: Test-only. Observed once across several full-suite runs under heavy concurrent load (many
simultaneous `go build`/`go test` processes on the same machine); not reproduced in isolation.

## Problem Description

`TestUpdateSlackConfig_RejectsInvalidWebhookURLFormat_When_DoesNotMatchSlackHooksPrefix`
(`server/services/slack_config_service_test.go`) failed with:

```
testing.go:1464: TempDir RemoveAll cleanup: unlinkat
  .../TestUpdateSlackConfig_RejectsInvalidWebhookURLFormat_When_DoesNo.../001/tmux-exec-gate/test_server_services_26422_34:
  directory not empty
```

The Slack config test itself never touches tmux — `newIsolatedSlackConfigService` only calls
`NewSlackConfigService(NewSlackNotifier())`. The polluting directory's key
(`test_server_services_<pid>_<n>`) comes from `session_service.go`'s `testTmuxServerSocketCounter`,
used only when a **different** test constructs a `SessionService` that spawns a real tmux session.

Root cause hypothesis: `config.GetConfigDirForDir`'s Priority 1 (`STAPLER_SQUAD_TEST_DIR`) is a
process-global env var. `session_service.go:2632`'s own doc comment already documents this exact class
of bug and its fix: an async pipeline goroutine not joined via `trackCleanup` before its owning test
returns can survive into a later test, and when it next calls `GetConfigDir()`, it picks up whatever
`STAPLER_SQUAD_TEST_DIR` the *current* test has since set via `t.Setenv` — writing its tmux-exec-gate
lock file into that later test's now-active `t.TempDir()`. That comment names a previously-fixed
instance of this (`TestCreateSession_should_ComposeProfileCLIFlagsBeforePresetExtraArgs_When_BothPresent`).
This is either a **new, uncovered** leak site (some async work not routed through `trackCleanup`), or
the same covered pipeline occasionally still racing under enough load.

## Also observed (2026-09-04)

Same symptom, this time on the leaking test itself rather than a later victim:
`TestCreateSession_EmptyPath_Autonomous_PassesPathValidation` (`server/services/session_creation_pipeline_test.go`)
failed its own `t.TempDir()` cleanup —
`unlinkat .../tmux-exec-gate/test_server_services_<pid>_31: directory not empty` — immediately after a
logged `WARN JoinSessionDriver: driver goroutine did not exit within timeout; it may still be running
session=autonomous-session timeout=10s`. Seen during `make quick-check`'s `test-coverage` target while
verifying an unrelated `log`/`config` workspace-path fix, with another concurrent Claude Code session on
the same machine independently running its own full `go test -race` suite at the time — consistent with
this bug's existing "load-dependent" classification, not a new root cause. Not re-filed separately per
this repo's `fix-flaky-tests-dont-defer` skill's guidance to avoid duplicate bug reports for the same
underlying class.

## Reproduction Steps

Not reliably reproducible in isolation (confirmed: re-running the Slack test alone, and the full
`server/services` suite alone under normal load, both pass). Observed once in:

```
go test ./session/... ./server/... -timeout=20m
```

run back-to-back with several other heavy `go build`/`go test` invocations competing for CPU on the
same machine (2026-09-03).

## Fix Approach

1. Reproduce reliably under controlled artificial load (see BUG-095's approach — a background CPU-burn
   loop or `-count` repetition alongside concurrent builds) to get a consistent repro rate.
2. Once reproducible, identify exactly which test owns `testTmuxServerSocketCounter`'s value 34 in a
   failing run, and which of its async goroutines is not covered by `trackCleanup` (or an equivalent
   join-before-return guarantee).
3. Route that goroutine through the same `trackCleanup` pattern `session_service.go:2632`'s comment
   describes, or — if a package-wide audit finds this is one of several uncovered sites — consider
   whether `GetConfigDir()`'s test-mode resolution should snapshot `STAPLER_SQUAD_TEST_DIR` at goroutine
   spawn time (captured once, passed down) rather than re-reading the process-global env var on every
   call, which would close the whole class at once instead of one call site at a time.

## Related

- `.claude/skills/fix-flaky-tests-dont-defer/SKILL.md` — filed per this repo's standing rule rather than
  silently re-excused, since reliable reproduction (needed before a real fix) was out of scope for the
  session that surfaced it.
- `server/services/session_service.go:2628-2636` — the `trackCleanup` doc comment documenting the
  already-fixed instance of this exact class of bug.
- `server/services/session_service_fork_test.go`'s `setupForkTestFixture` doc comment — documents a
  related but distinct ordering fix (bus-close-after-Shutdown) for a different symptom of the same
  underlying "goroutine outlives its test" hazard.
