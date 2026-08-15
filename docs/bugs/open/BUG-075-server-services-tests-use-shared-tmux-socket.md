# BUG-075: `server/services` tests that reach real tmux use the shared/production socket, not an isolated one [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-08-15

## Problem Description

The `session` package's tests consistently isolate their tmux server via `TmuxServerSocket: "test_" + t.Name()` or a `getTestTmuxSocket(t)`/`coldRestoreSocket(t)` helper (see `session/integration_test.go`, `session/session_creation_test.go`, `session/session_restart_test.go`, `session/instance_cold_restore_test.go`), which is threaded through `Instance.TmuxServerSocket` → `tmux.NewTmuxSessionWithServerSocket(..., serverSocket, ...)` (`session/instance.go:1581-1582,1710-1711`).

`server/services/session_service.go`'s `SessionService.CreateSession` has no equivalent: `grep -n "TmuxServerSocket" server/services/session_service.go` returns nothing, and no test under `server/services/*_test.go` sets `TmuxServerSocket` either. `CreateSession` builds its `session.Instance` internally from an HTTP-shaped `CreateSessionRequest` with no socket-override field, so any `server/services` test that reaches a real tmux session (e.g. `TestCreateSession_EmptyPath_OneOff_PassesPathValidation` in `session_service_create_test.go`, and similar real-tmux-touching tests in `cdp_stream_handler_test.go`, `notification_service_test.go`, `review_queue_service_test.go`, `session_crud_test.go`, `session_service_alias_test.go`, `session_service_envvars_test.go`, `session_service_extraargs_test.go`, `session_service_fork_test.go`, `session_service_program_test.go`) targets the shared/production default tmux socket rather than an isolated one.

In practice this hasn't caused observed failures — `go test ./server/services/... -timeout 20m` passes cleanly (394s, single run, no `-race`/`-p 1` amplification applied here) — but it's a latent source of cross-test/cross-process contention on shared CI runners, and is inconsistent with the isolation pattern already established in `session/`.

## Why Filed Instead Of Fixed Inline

Fixing this requires adding a test-only socket-override path to `SessionService.CreateSession` (which currently has no such parameter, by design, since it's populated from over-the-wire `CreateSessionRequest` fields) or a package-level `TestMain` that redirects all `server/services` test-created instances to an isolated default socket. Either approach touches the production `CreateSession` code path or a shared test-fixture layer used by ~10 test files, which is a meaningfully larger blast radius than the `DeleteSession` goroutine-leak/cleanup-timeout fix in PR #503. Filed standalone per `.claude/rules/fix-flaky-tests-dont-defer.md`'s exception for changes that would expand a current change's scope into shared test infrastructure.

## Fix Approach

Options to consider:
- Add an unexported test-only hook (e.g. a package-level var or a `SessionService` field settable only via a `NewSessionServiceForTest` constructor) that defaults new instances' `TmuxServerSocket` to `"test_" + t.Name()`-style isolation when set.
- Alternatively, have `server/services` tests construct instances directly via `session.NewInstance` (as `TestDeleteSession_PublishesDeletedEvent` already does via `storage.AddInstance`) instead of going through the full `CreateSession` HTTP path, for tests that don't need to exercise `CreateSession` request-validation logic itself.

## Related

Found while verifying acceptance criterion 3 ("server/services test runs use an isolated tmux socket") for the backlog item covering PR #503's `DeleteSession` goroutine-leak/timeout fix. `docs/bugs/open/BUG-067-server-services-race-suite-exceeds-150s-ci-timeout.md` is the related suite-timeout bug this criterion was meant to help mitigate.
