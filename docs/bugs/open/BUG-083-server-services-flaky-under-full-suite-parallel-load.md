# BUG-083: `server/services` Tests Flake Under `make test`'s Full-Suite Parallel Load [SEVERITY: Low]

**Status**: 🔓 Open
**Discovered**: 2026-08-20, while validating the `session`/`session/mux` `-p 1` scoping fix for the flaky-tmux-tests backlog item (see `docs/bugs/fixed/BUG-051-session-tmux-package-flaky-under-parallel-quick-check.md`).
**Impact**: `make test`'s second (unscoped) `go test` invocation intermittently fails on `server/services` tests under full-suite parallel load. Does not affect runtime correctness — only test-suite reliability.

## Problem Description

A full `make test` run (`/tmp/make-test-4.log`, 2026-08-20) failed `server/services` (263.513s) with two failures in the same run:

1. `TestWatchBacklogItems_should_deliverRaceWindowEventExactlyOnceAsSnapshot_When_PublishedBetweenSubscribeAndEventsSince` (`backlog_service_events_test.go:296`) — expected the race-window event delivered exactly twice (once via replay, once via live fan-out), got 3 deliveries (`seq:2` appears twice, then `seq:3`).
2. `TestListWorktrees_EmptyPath` (`workspace_misc_test.go:155`) — `deadline_exceeded: listing worktrees timed out`.

Both tests pass reliably in isolation:
```
go test -short ./server/services/... -run 'TestWatchBacklogItems_should_deliverRaceWindowEventExactlyOnceAsSnapshot_When_PublishedBetweenSubscribeAndEventsSince|TestListWorktrees_EmptyPath' -count=5 -v
--- PASS (x5 each), ok  	github.com/tstapler/stapler-squad/server/services	3.940s
```

This is the same failure shape as BUG-051 (fixed wall-clock budgets and event-ordering assumptions getting blown under `t.Parallel()` scheduler contention), but in a different package (`server/services`, not `session`/`session/mux`/`session/tmux`) not covered by that fix's `-p 1` scoping.

Confirmed unrelated to the diff in flight when this was found (`session/session_driver_test.go` only, widening a test deadline — see BUG-051's 2026-08-20 recurrence entry).

## Suggested Investigation

- `TestListWorktrees_EmptyPath`'s timeout suggests the same fixed-wall-clock-budget-under-load pattern as BUG-051 — check for a `context.WithTimeout` in the worktree-listing path that's too tight for high parallel load.
- `TestWatchBacklogItems_...`'s extra duplicate delivery suggests a genuine race in the replay/live-fanout dedup logic (an event landing in both the snapshot replay and the live subscription window), not just a timing budget — this one may need a real synchronization fix rather than a timeout widen.
- Consider whether `server/services` needs the same `-p 1` scoping as `session`/`session/mux`/`session/tmux`, or whether these two specific tests need per-test fixes.

## Related

- Filed per `.claude/rules/fix-flaky-tests-dont-defer.md` — found during BUG-051 remediation validation but out of scope to fix in that change (different package, different root cause per-test, would expand that change's blast radius).
