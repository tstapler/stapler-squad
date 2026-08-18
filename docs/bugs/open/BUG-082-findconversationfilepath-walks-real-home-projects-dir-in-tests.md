# BUG-082: `findConversationFilePath` walks the real `~/.claude/projects/` directory instead of an isolated fixture, making dependent tests pathologically slow on a heavily-used dev box [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-08-17, while chasing full-`session`-package verification for BUG-081 (unrelated GitHub rate-limiter test-isolation fix)

## Problem Description

`session/history.go:352`'s `findConversationFilePath(sessionID string)` resolves `filepath.Join(home, ".claude", "projects")` via `os.UserHomeDir()` and `filepath.Walk`s it directly — there is no seam to point it at a test fixture directory instead of the real, current user's `~/.claude/projects/`. On a dev box that has been used heavily for real Claude Code sessions (this one has, evidenced by an extensive real project/session history), that directory can contain a very large number of real `.jsonl` conversation files, making the walk (and the file scans inside it — see `conversationScanBufPool` and the per-file `bufio.Scanner` in the same function) slow, and slower still under host resource contention.

`TestSessionDriver_DialogGaveUp_FallsThroughToInactivityEscalation` (`session/session_driver_test.go:1187`) exercises this path (via `FindConversationFilePath`) and was observed stuck for over 1m40s (`goroutine 9305`, blocked in `bufio.(*Scanner).Scan` → `os.(*File).Read` → real disk I/O) during a `go test ./session -short -timeout=180s` run, on a host simultaneously under `load average` 70–90 on 24 cores. The goroutine dump showed this was the *only* stuck test — no other goroutine in the dump was blocked — so the test itself, not host load alone, is the proximate cause: it has no way to bound or isolate the directory it scans.

## Reproduction Steps

1. On a dev machine with a large real `~/.claude/projects/` tree: `go test ./session -short -timeout=60s -run TestSessionDriver_DialogGaveUp_FallsThroughToInactivityEscalation -v`.
2. Compare wall-clock time against a machine/CI runner with a small or empty `~/.claude/projects/` — the discrepancy should be large and roughly proportional to the number/size of real `.jsonl` files present.

## Suggested Fix Approaches

- Thread a configurable base directory (e.g. via a package-level var overridable in tests, mirroring `session/backlog_plugin_github.go`'s `githubAPIBaseURL` pattern already used for the GitHub base URL) through `findConversationFilePath` / `FindConversationFilePath`, and have `TestSessionDriver_DialogGaveUp_FallsThroughToInactivityEscalation` (and any other test that reaches this path) point it at a small `t.TempDir()` fixture instead of the real `$HOME`.
- Alternatively, have the test set `$HOME` to a temp dir for its duration (`t.Setenv("HOME", t.TempDir())`), which is lower-effort but changes real env state for the whole test process unless carefully scoped.

## Why Not Fixed Here

Discovered as a side effect of verifying BUG-081 (a `github` package rate-limiter test-isolation fix); `session/history.go` and `session/session_driver_test.go` are a different subsystem entirely (conversation-history lookup / session-driver escalation, not GitHub API calls), so fixing it here would expand that fix's blast radius into unrelated code. Filed per this repo's "fix flaky tests when found, don't just defer" convention (`.claude/rules/fix-flaky-tests-dont-defer.md`) so it doesn't get silently re-discovered and re-excused later.

## Related

Surfaced while verifying [[BUG-081]] (`docs/bugs/fixed/BUG-081-defaultratelimiter-global-poisoned-across-session-package-tests.md`).
