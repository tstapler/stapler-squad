# BUG-075: `server/services` tests that reach real tmux use the shared/production socket, not an isolated one [SEVERITY: Low]

**Status**: ✅ Fixed (2026-08-15)
**Discovered**: 2026-08-15
**Fixed**: 2026-08-15 — `server/services/session_service.go`

## Problem Description

The `session` package's tests consistently isolate their tmux server via `TmuxServerSocket: "test_" + t.Name()` or a `getTestTmuxSocket(t)`/`coldRestoreSocket(t)` helper (see `session/integration_test.go`, `session/session_creation_test.go`, `session/session_restart_test.go`, `session/instance_cold_restore_test.go`), which is threaded through `Instance.TmuxServerSocket` → `tmux.NewTmuxSessionWithServerSocket(..., serverSocket, ...)` (`session/instance.go:1581-1582,1710-1711`).

`server/services/session_service.go`'s `SessionService.CreateSession` had no equivalent: `grep -n "TmuxServerSocket" server/services/session_service.go` returned nothing, and no test under `server/services/*_test.go` set `TmuxServerSocket` either. `CreateSession` builds its `session.Instance` internally from an HTTP-shaped `CreateSessionRequest` with no socket-override field, so any `server/services` test that reaches a real tmux session targeted the shared/production default tmux socket rather than an isolated one.

## Fix

Added an unexported `testTmuxServerSocket string` field on `SessionService`, auto-populated inside the existing `NewSessionServiceWithSearchEngine` constructor under `config.IsTestMode()` — mirroring the identical `newDefaultSearchEngine()`/`config.IsTestMode()` precedent already in the same file, which was justified for the same reason (avoiding a mechanical migration of the package's ~80+ existing `NewSessionService(storage, eventBus)` test call sites). The value is derived from `os.Getpid()` plus a package-level atomic counter (`testTmuxServerSocketCounter`), so every `SessionService` created within a test binary run gets its own isolated socket suffix with no cross-test contention.

The field is threaded into all three `session.InstanceOptions{...}` construction sites in the package (`CreateDirectorySession`, `CreateWorktreeSession`, and the `CreateSession` RPC handler) via `TmuxServerSocket: s.testTmuxServerSocket`. In production (`config.IsTestMode() == false`), the field stays empty and `TmuxServerSocket` falls through to its existing shared-default behavior — no behavior change outside test mode.

No test files needed to change: `go test ./server/services/... -timeout 20m` passes (1605 tests).

## Related

Found while verifying acceptance criterion 3 ("server/services test runs use an isolated tmux socket") for the backlog item covering PR #503's `DeleteSession` goroutine-leak/timeout fix. `docs/bugs/open/BUG-067-server-services-race-suite-exceeds-150s-ci-timeout.md` is the related suite-timeout bug this criterion was meant to help mitigate.
