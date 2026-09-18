# BUG-077: tmuxreap PID-reuse false positive permanently skips a subset of dead sockets [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-08-16
**Impact**: A subset of genuinely-dead leaked tmux test sockets are never reaped by `ReapLeakedTestServers()`, causing `/tmp/tmux-<uid>/` to accumulate indefinitely. Does not cause test hangs (bounded by `reapOverallBudget`, see the fix in this same file for BUG referenced in backlog item `e8d180d4-c4ff-4856-bc43-768365584420`) — purely a cleanup-completeness bug.

## Problem Description

`testutil/tmuxreap.ReapLeakedTestServers()` decides whether a leaked test socket is safe to reap by extracting a PID embedded in the socket's filename (`extractTestSocketPID`) and checking whether that PID is still alive (`isProcessAlive`). If the PID is alive, the socket is assumed to belong to "another live test runner" and is skipped.

This is a false-positive risk: PIDs are recycled by the OS. A socket's embedded PID can belong to a test process that exited long ago, and by the time the reaper runs, the OS may have reassigned that same PID number to a completely unrelated, currently-running process (e.g., a different, newer tmux server for a different socket). `isProcessAlive` has no way to distinguish "the original owner is still running" from "some unrelated process now happens to have this PID" — it only checks liveness of the PID number, not process identity.

## Reproduction Steps

1. On a dev machine with a large accumulated backlog of leaked test tmux sockets in `/tmp/tmux-<uid>/`, run `ReapLeakedTestServers()` repeatedly (e.g. via a small `main.go` calling it directly, or via any package's `TestMain`).
2. Observe the socket count plateaus after a few passes instead of continuing to shrink toward zero.
3. Pick one remaining socket name (e.g. `test-isolated-11627`) and confirm it's genuinely dead: `tmux -L test-isolated-11627 list-sessions` → `"no server running on ..."`.
4. Extract its embedded PID (`11627` in this example) and check `ps -p 11627` — it may show a **live** process, but for a **different** socket name (e.g. `tmux -L test_server_services_9415_43 start-server ...`), proving the PID was reused by an unrelated process, not the original owner of `test-isolated-11627`.

## Root Cause

`extractTestSocketPID` + `isProcessAlive` (`testutil/tmuxreap/tmuxreap.go`) treat "a process is alive at PID N" as equivalent to "the process that originally created this socket is alive," which is false under PID reuse. There is no cross-check that the live process at that PID is actually a tmux server bound to *this specific* socket name.

## Files Likely Affected

- `testutil/tmuxreap/tmuxreap.go` — `ReapLeakedTestServers`, `isProcessAlive`, `extractTestSocketPID`

## Fix Approach

Before trusting `isProcessAlive(ownerPID)` as a reason to skip a socket, verify the live process at that PID is actually the tmux server for *this* socket name — e.g. inspect its command line (`ps -p <pid> -o command=` or `/proc/<pid>/cmdline` on Linux) for a `-L <name>` argument matching the socket's own name. If the command line doesn't reference this socket name, the PID was reused by an unrelated process and the socket is safe to reap regardless of that PID's liveness.

## Verification

After fix: repeated `ReapLeakedTestServers()` passes against a real accumulated backlog should continue shrinking the socket count to (near) zero instead of plateauing, with no regression in the "don't interfere with another live test runner" safety property (verify via a test that starts a real tmux server under a test-prefixed socket name and confirms `ReapLeakedTestServers()` does not kill it while it's genuinely still running).

## Related Tasks

- Discovered while root-causing the flaky `TempDir` cleanup race backlog item `e8d180d4-c4ff-4856-bc43-768365584420`. The primary fix for that item (bounded concurrency + time budget + best-effort socket-file removal in `ReapLeakedTestServers`) already ships in the same change and is unaffected by this bug — this is a separate, lower-severity cleanup-completeness gap filed per `.claude/rules/fix-flaky-tests-dont-defer.md` rather than silently left unaddressed.
