# BUG-051: `session/tmux` Package Tests Flake Under `make quick-check`'s Parallel Load [SEVERITY: Low]

**Status**: ✅ Fixed (main@dccee742a, 2026-08-04)
**Discovered**: 2026-07-27, while running `make quick-check` as the final AC-7 gate for `insights-cost-pricing-gaps` (backlog bcb20604). Not caused by that project's diff — `session/tmux` was never touched by it (confirmed via `git diff --stat main...HEAD -- session/tmux/`, zero output across the entire change).
**Impact**: `make quick-check` / `make test-coverage` intermittently fails on real tmux server lifecycle tests in `session/tmux`, blocking a clean "all green" run of the standard pre-push gate. Does not affect runtime correctness of the shipped app — only test-suite reliability under this specific `go test` invocation's parallelism/resource pressure.

## Problem Description

Ran `make quick-check` three times in a row against an otherwise-unrelated diff. Each run failed `test-coverage`, but with a **different** failing test each time, always inside `session/tmux`:

1. Run 1 & 2: `TestEnsureServerRunning_NoOp` (`session/tmux/tmux_test.go:273`) — `tmux start-server failed: exit status 1 (output: server exited unexpectedly)`.
2. Run 3: `TestKillOrphanedControlModeClients` (`session/tmux/kill_orphaned_control_mode_clients_test.go:87`) — expected 2 killed orphaned clients, got 0. The *same* run also re-failed `TestEnsureServerRunning_NoOp` with the identical error as runs 1/2.

Each failing test **passes reliably when run in isolation** (`go test ./session/tmux/ -run TestEnsureServerRunning_NoOp -v` → PASS). This points to real tmux-server contention/resource exhaustion when many tests in this package spin up, kill, and restart actual tmux servers concurrently under `go test`'s default parallelism — not a logic bug in the tests or the code under test.

This is a known-fragile area: `git log --oneline -- session/tmux/tmux_test.go` shows a prior dedicated fix (`e476853dc fix(tmux): recover EnsureServerRunning from a transient check-race`) for an adjacent race in the same file, suggesting this class of flake has recurred before and wasn't fully closed.

## Suggested Investigation (not done here — out of scope for the project that surfaced this)

- Check whether `session/tmux`'s tests can be forced to run serially (`t.Parallel()` audit, or `go test -p 1`) without an unacceptable CI time cost.
- Check whether tests that spin up a real tmux server should share a single test-scoped server instance instead of each starting/killing their own, if the current per-test lifecycle is what's contending for the same underlying `tmux` binary/socket.
- Consider whether this sandboxed/CI environment has a tmux-server resource limit (max sessions, socket handles) that's being hit under full parallel load specifically, separate from any local-dev-machine behavior.

## Related

- Not related to `insights-cost-pricing-gaps` (backlog bcb20604) or any of its pricing/insights changes — filed as a fast-follow per this repo's convention of documenting adjacent issues found during unrelated work rather than silently ignoring them (see `ADR-002`/`ADR-003` in `project_plans/insights-cost-pricing-gaps/decisions/` for the same pattern applied to two other findings from that project).

## Recurrence log

- 2026-08-05, while running `make ci`/`make test` as the AC-7 gate for `stapler-squad-fix-idle-reviewer-wedge`: `TestEnsureServerRunning_NoOp` failed identically (`tmux start-server failed: exit status 1 (output: server exited unexpectedly)`) under the full-suite parallel run, passed 5/5 in isolation (`go test ./session/tmux -run TestEnsureServerRunning_NoOp -count=5`), and — to positively rule out this session's own diff — reproduced identically after `git stash`-ing every change back to the unmodified base branch and re-running `make test`. Confirmed still open and still purely load-triggered, not tied to any particular diff, on `main` as of that point.

## Resolution

Fixed on `main` by `dccee742a` ("fix(tests): fix flaky/failing tests across executor, tmux, gogitstore, and git packages", 2026-08-04) — root cause was several `session/tmux` test files hardcoding the literal `"tmux"` binary in `exec.Command`/`safeexec.CommandContext` calls, resolving to whatever `tmux` is on `$PATH` instead of the test-pinned `TMUX_BIN` (`bin/tmux`) the package's own `Binary()` helper resolves to — causing protocol/version mismatches when both connect to the same socket, plus widened retry logic in `tmux.go` for transient contention under heavy system load. Verified after merging that commit into `stapler-squad-fix-idle-reviewer-wedge`: full `go test $(go list ./...)`, including `session/tmux`, passed clean with no `-run`/isolation workaround needed.
