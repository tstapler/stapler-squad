# Adversarial Review: mcp-program-param

**Date**: 2026-09-24
**Verdict**: CLEAN

## Blockers

None.

**Previously-BLOCKED item — rate-limiter collision — RESOLVED.** Verified against source:
- `server/mcp/rate_limiter.go:22,61` confirms `createSessionLimiter` is a package-level `var` (not `const`): `var createSessionLimiter = newTokenBucket(3.0/60.0, 3)`, and `func newTokenBucket(rate, capacity float64) *tokenBucket` is real and matches the plan's reassignment expression exactly — legally reassignable from any `package mcp` (white-box) `_test.go` file.
- Plan's fix (§"Rate-limiter isolation", `plan.md:247`) adds `resetCreateSessionLimiterForTest(t *testing.T)` in Task 1.4.1a (`plan.md:260`), which every subsequent rate-limited test calls as its first line: Task 1.4.1b's three tests (bash, custom program, explicit aider — `plan.md:264`) and Task 1.4.2a's two tests (custom program, explicit aider — `plan.md:276`, explicit "must call ... as their first line, and must not use `t.Parallel()`"). Task 1.4.1a's own schema-only test is correctly called out as not needing a reset since it never calls the handler (`plan.md:259`).
- Math checks out: each of the 5 new rate-limited tests resets the bucket to a fresh capacity-3 token bucket before drawing exactly 1 token, so no test can starve another regardless of execution order. The one pre-existing draw (`TestCreateSessionForPR_should_ReturnExistingSession_When_PRAlreadyHasOne`, confirmed still at `server/mcp/tools_github_test.go:227`, with `t.Parallel()` already avoided per its own comment at line 228) is unaffected either way since it draws only 1 token total.
- No `t.Parallel()` anywhere in the new tests, consistent with the existing `t.Setenv`-driven convention — no race on the reassignment.

## Concerns

None outstanding — all three previously-flagged concerns are adequately addressed.

- **AC3 (claude/aider unchanged) — RESOLVED.** Task 1.4.1b adds `TestCreateSession_should_AcceptAiderUnchanged_When_ProgramIsExplicitlyAider` and Task 1.4.2a adds `TestCreateSessionForPR_should_AcceptAiderUnchanged_When_ProgramIsExplicitlyAider` (`plan.md:255,264,272,276`), both asserting `program="aider"` explicitly reaches `CreateSessionRequest.Program == "aider"` and the call succeeds. The Verification Checklist's AC3 row (`plan.md:338`) now cites both test names directly instead of only implementation tasks. Concrete, not vague.
- **Phase 2 (`list_programs`) scope creep — adequately addressed, accepted rather than eliminated.** The plan did not split Phase 2 into a separate PR (the reviewer's original recommendation), but it is explicitly and structurally isolated: labeled "Not required by any acceptance criterion as written" (`plan.md:283`), kept under its own Phase 2 / Epic 2.1 heading distinct from Phase 1's core-fix epics, and ADR-001 (`decisions/ADR-001-...md:80-81`) states it "can be reviewed, deferred, or dropped independently" of Phase 1. Given this is a CONCERN rather than a BLOCKER, explicit self-labeling plus structural separation within the same plan is an adequate resolution — a reviewer or the author can trivially drop Phase 2 at review time without touching Phase 1's tasks. Not eliminated, but no longer a silent scope-creep risk.
- **Wrong file citation for `newWorktreeGuardHandlers` — RESOLVED.** `grep -n "testhelpers_test.go" plan.md` returns zero hits. The current citation (`plan.md:126`, `tools_lifecycle_worktree_guard_test.go:44-57`) matches the verified source exactly — the function spans lines 44–57 in `server/mcp/tools_lifecycle_worktree_guard_test.go`.

## Minors

- None newly identified in this scoped re-review (out of scope: the plan's other pre-existing minors from the prior architecture review were not re-checked here per the requested scope).
