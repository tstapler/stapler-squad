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

## Recurrence — 2026-08-25 (PR #628, `backlog/stapler-squad-pr-event-webhooks`)

CI's `Test` job (`go test ./...` with coverage, GitHub Actions run [32887523759](https://github.com/tstapler/stapler-squad/actions/runs/32887523759/job/97933441915)) failed `server/services` (304.5s–304.7s across two consecutive attempts on the same commit) with a different set of tests each time — same non-deterministic-under-load shape as the original report, but no test names in common with it:

- `TestHandleCurrentPaneRequest_should_OnlyRouteFastLane_When_OnlyExecGateFastLaneFlagOn`
- `TestHandleCurrentPaneRequest_should_SkipResizeAndSigwinchLoop_When_StaleDimensionsTrueAndFlagOn`
- `TestDeepLinkResolver_should_LogResolvedOrFailedEvent_When_EveryOutcomeReasonOccurs`
- `TestDeleteSession_LiveInstance_LogsWarningOnSlowCleanupButStillWaits`

All four pass reliably in isolation:
```
go test ./server/services/... -run 'TestHandleCurrentPaneRequest_should_OnlyRouteFastLane_When_OnlyExecGateFastLaneFlagOn|TestDeleteSession_LiveInstance_LogsWarningOnSlowCleanupButStillWaits|TestDeepLinkResolver_should_LogResolvedOrFailedEvent_When_EveryOutcomeReasonOccurs|TestHandleCurrentPaneRequest_should_SkipResizeAndSigwinchLoop_When_StaleDimensionsTrueAndFlagOn' -v
--- PASS (all 4), ok  	github.com/tstapler/stapler-squad/server/services	0.528s
```

None of the four touch a file in PR #628's diff (`server/services/github_webhook_handler.go`, `github_webhook_pr_fix.go`, and their tests) — confirmed via `grep -rl` for each test name against `server/services/*.go`, all four live in unrelated files (`connectrpc_websocket_test.go`, `deep_link_resolver_test.go`, `session_service_test.go`). The two runs failing with entirely different test names in the same package, both passing in isolation, is consistent with this bug's existing "scheduler contention under full-suite parallel load" hypothesis rather than a new, distinct cause — widens the affected-test list rather than needing its own bug.

**Sharper root-cause candidate found for `TestHandleCurrentPaneRequest_should_OnlyRouteFastLane_When_OnlyExecGateFastLaneFlagOn`**: it (and its `setOnlyResyncFlag`/`setAllTerminalResyncFlags` sibling helpers, `connectrpc_websocket_test.go:2205-2230`) mutate feature flags via `config.LoadConfig().SetFeatureFlag(name, ...)` — a **process-wide singleton**, not a per-test fixture — and restore them via `t.Cleanup`, not synchronously. Any other test in the same package running under `t.Parallel()` that also reads/writes the same global config's feature flags (or a different flag, if `SetFeatureFlag`'s map isn't independently locked per key) can observe a flag value flip mid-assertion. This would explain both this bug's original "different tests fail each time" symptom (any parallel test touching the shared config singleton is a candidate, not just these two) and this occurrence's repeat of the *same* two tests across 3 consecutive job attempts (same package's scheduler tends to interleave the same subset of tests similarly run to run). Not fixed here — confirming this requires tracing every `t.Parallel()` test in `server/services` against `config.LoadConfig()` usage, which is its own investigation, out of scope for PR #628.

## Investigation note — 2026-08-28 (backlog item c0e88be9, flaky-tests-under-CI-load)

Confirmed **not subsumed** by `a32a01d5d`/#548's `trackCleanup` fix: that fix joins
`CreateSession`'s own fire-and-forget start goroutine at `Shutdown`/`DeleteSession` —
none of this bug's named tests (`TestListWorktrees_EmptyPath`,
`TestWatchBacklogItems_...`, the PR #628 recurrence's four) exercise that code path, and
the leading hypothesis here (a process-wide `config.LoadConfig()` feature-flag singleton
raced across `t.Parallel()` tests) is a different mechanism entirely.

3 consecutive full `TMUX_BIN=$(which tmux) go test -race -timeout=20m ./server/services/...`
runs today (233.959s, 228.817s, 217.899s) did not reproduce this bug — consistent with its
existing "intermittent, coverage/CI-load-dependent" characterization rather than evidence
it's fixed. Remains open; still out of scope for c0e88be9 (requires the
`config.LoadConfig()`-vs-`t.Parallel()` audit this doc's own "Sharper root-cause candidate"
section already flags as its own investigation).

## Related

- Filed per `.claude/rules/fix-flaky-tests-dont-defer.md` — found during BUG-051 remediation validation but out of scope to fix in that change (different package, different root cause per-test, would expand that change's blast radius).
