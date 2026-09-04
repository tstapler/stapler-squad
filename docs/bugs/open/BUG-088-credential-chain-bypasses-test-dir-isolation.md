# BUG-088: `CredentialChain` bypasses `STAPLER_SQUAD_TEST_DIR` isolation, can make live API calls from tests [SEVERITY: Medium]

**Status**: 🐛 Open
**Discovered**: 2026-08-23
**Impact**: Any Go test that constructs a real `*services.SessionService` and calls `SetLifecycleContext` (which starts `CapacityMonitor`) can silently read real OAuth credentials from the developer's `~/.claude/.credentials.json` and make genuine outbound Anthropic/Gemini API calls — bypassing this repo's standard test-isolation mechanism. On a machine with a live Claude Code session, this can consume real API quota/rate limits and introduce nondeterministic test behavior (observed directly: `github API: primary rate limit exhausted (403)` and a Gemini `403 PERMISSION_DENIED` while debugging an unrelated flake — see `docs/bugs/open/BUG-087-...md`'s discovery context, though that specific 403 was PR/Slack pollers, not this credential path — this bug covers the credential-chain-specific instance found in the same investigation).

## Problem Description

`server/services/credentials.go`'s `CredentialChain` (specifically `ClaudeOAuthCredentialSource`/`AgyCredentialSource`) resolves credentials via `os.UserHomeDir()` directly, rather than respecting `STAPLER_SQUAD_TEST_DIR` (the env var this codebase otherwise uses consistently for test-mode config isolation — see `session_service.go:607`'s existing `config.IsTestMode()` guard as the established pattern). This means any test — not just deliberately-integration tests — that builds a real `SessionService` and calls `SetLifecycleContext` (which unconditionally starts `CapacityMonitor`'s polling loop) picks up the *developer's own real credentials* and begins making *real* outbound API calls to Anthropic/Gemini as a side effect of running `go test`.

## Reproduction Steps

1. On a machine with a real, live `~/.claude/.credentials.json` (i.e., any developer machine with Claude Code set up), write or run a test that constructs a real `*services.SessionService` (e.g. via `server.BuildDependencies()`/`NewServerWithDeps`, or any narrower construction that still calls `SetLifecycleContext`).
2. Run the test.
3. Expected: the test's dependencies are fully isolated from the developer's real environment — no real credentials read, no real network calls made.
4. Actual: `CapacityMonitor.Start` begins polling Anthropic/Gemini using the developer's real OAuth credentials, resolved via `os.UserHomeDir()` — confirmed directly while fixing PR #605's `watch_sessions_native_streaming_integration_test.go` (worked around locally there by redirecting the `HOME` env var for that one test's duration, mirroring this package's unexported `withFakeHome` helper).

## Root Cause

`server/services/credentials.go`'s credential-resolution sources call `os.UserHomeDir()` directly instead of going through this repo's test-mode config path (`config.IsTestMode()` / `STAPLER_SQUAD_TEST_DIR`), unlike most of the rest of the codebase's test-isolation surface (e.g. `session_service.go:607`'s existing guard). `SessionService.SetLifecycleContext` unconditionally starts `CapacityMonitor`, which unconditionally resolves credentials via this chain — there is no test-mode short-circuit before real network calls are attempted.

## Files Likely Affected

- `server/services/credentials.go` — `CredentialChain`, `ClaudeOAuthCredentialSource`, `AgyCredentialSource` — the `os.UserHomeDir()` call sites.
- `server/services/capacity_monitor.go` — `CapacityMonitor.Start` — the call site that should have a test-mode guard, mirroring `session_service.go:607`'s existing pattern.
- `server/services/session_service.go:607` — the existing `config.IsTestMode()` guard to use as the precedent/template for the fix.

## Fix Approach

Gate `CapacityMonitor.Start`'s first credentialed call (or the `CredentialChain`'s resolution itself) behind `config.IsTestMode()`, consistent with the existing guard pattern at `session_service.go:607` — so tests that build a real `SessionService` and call `SetLifecycleContext` never reach real credential resolution or real network calls unless a test explicitly opts in and provides its own fake credentials/test-mode override.

## Verification

After the fix: a test that builds a real `SessionService`, calls `SetLifecycleContext`, and runs under `STAPLER_SQUAD_TEST_DIR`/test mode should produce zero outbound network calls and zero reads of the real `~/.claude/.credentials.json` — verifiable by running such a test on a machine with real credentials present and confirming (via strace/network monitoring, or simply the absence of any 403/rate-limit/API-related log lines) that no real API call was attempted.

## Related Tasks

Discovered and worked around locally (not fixed at the root) while fixing a CRITICAL code-review finding on PR #605 (`stapler-squad-web-transport` branch, project `web-transport-architecture-review`) — the local workaround was redirecting `HOME` for the one affected test's duration; this bug tracks the underlying gap for a proper fix. See also `docs/bugs/open/BUG-087-captureLogs-global-slog-swap-races-under-t-parallel.md`, filed in the same investigation for an unrelated test-isolation issue found while running the same PR's test gate.
