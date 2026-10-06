# BUG-084: `TestCommitImportExternalSession_PersistsAndLinksAndSuspends_When_StartAndSuspendSucceed` goleak flake under full `session` package run

## Summary

`go test -short -timeout=20m ./session/` (the exact invocation `make test` runs)
intermittently fails `TestCommitImportExternalSession_PersistsAndLinksAndSuspends_When_StartAndSuspendSucceed`
(`session/import_commit_test.go`) with a goroutine-leak failure. Running the
test in isolation — including repeated `-run`-scoped invocations — passes
reliably every time. Reproduced once out of 7 full-package attempts in this
session, so — like BUG-079/080/083 — it depends on cross-test
ordering/timing under full-suite load, not this test alone.

## Reproduction

```
go test -short -timeout=20m ./session/
# --- FAIL: TestCommitImportExternalSession_PersistsAndLinksAndSuspends_When_StartAndSuspendSucceed (0.00s)

go test -short -run TestCommitImportExternalSession_PersistsAndLinksAndSuspends_When_StartAndSuspendSucceed -v ./session/
# always passes (confirmed 2x)

go test -short -timeout=20m ./session/   # x6 more full-package retries
# always passes (0/6) -- consistent with a rare, timing-dependent flake, not a deterministic bug
```

## Leaked goroutines (from the one captured failure)

```
goroutine 2675 [chan receive (nil chan)]:
github.com/tstapler/stapler-squad/pkg/events.(*EventBus).Subscribe.func1()
	pkg/events/bus.go:61 +0x3e
created by github.com/tstapler/stapler-squad/pkg/events.(*EventBus).Subscribe in goroutine 2544
	pkg/events/bus.go:60 +0x111

goroutine 2871 [chan receive (nil chan)]: (same site)
	created by ... in goroutine 2525

goroutine 2745 [chan receive (nil chan)]: (same site)
	created by ... in goroutine 2526
```

All three are the generic `EventBus.Subscribe` forwarding goroutine
(`pkg/events/bus.go:60-61`), blocked receiving on a channel that's never
closed/written to again -- i.e. some *other* test in the package subscribed to
the shared `events.EventBus` and never called its unsubscribe/cleanup before
returning, so the goroutine is still alive (parked on a nil/abandoned channel)
when this test's own leak check runs. Not owned by
`TestCommitImportExternalSession_...` itself, which only exercises the
import→commit→suspend flow for one instance.

## Suspected root cause

Same shape as BUG-079/080/083: some earlier `session` package test subscribes
to `events.EventBus` (directly, or transitively via a `session.Instance`/
`ClaudeController`/notifier construction) and does not synchronously
unsubscribe or drain that goroutine in its own `t.Cleanup`/teardown before
control returns to the test runner. Under full-package scheduling pressure,
that goroutine is still alive when a later test's leak check
(`TestCommitImportExternalSession_...`'s own goleak assertion, or a shared
package-level check) fires.

## Fix Approach

Identify which `session` package test(s) call `events.NewEventBus(...)` /
`.Subscribe(...)` without a matching `Unsubscribe`/context-cancel in cleanup,
and audit for a missing synchronous drain — mirroring BUG-083's fix approach
for the analogous `server` package leak.

## Verification

After fix: `go test -short -timeout=20m ./session/` run repeatedly (~20x)
with zero `TestCommitImportExternalSession_..._StartAndSuspendSucceed` goleak
failures.

## Investigation note — 2026-08-28 (backlog item c0e88be9, flaky-tests-under-CI-load)

`TestCommitImportExternalSession_PersistsAndLinksAndSuspends_When_StartAndSuspendSucceed`
is one of the 4 tests originally named in backlog item c0e88be9. Confirmed **not
subsumed** by `a32a01d5d`/#548's `trackCleanup` fix: that fix joins `CreateSession`'s own
start goroutine in `server/services`, not the `pkg/events.(*EventBus).Subscribe`
forwarder goroutine leaked somewhere in the `session` package's own test suite — different
package, different goroutine, different owning code path.

1 full `TMUX_BIN=$(which tmux) go test -race -timeout=20m ./session` run today (171.4s,
no `-short`, no `-count`) did not reproduce this leak — consistent with, not proof against,
this bug's own documented ~1-in-7-full-runs rate (0/1 here is within that noise). Remains
open; still out of scope for c0e88be9 per this doc's own filed exception (needs a
package-wide `EventBus.Subscribe`-caller cleanup audit).

## Related

- `.claude/rules/fix-flaky-tests-dont-defer.md` — filed per this rule's
  exception clause (root-causing requires auditing EventBus subscriber
  cleanup across the whole `session` package test suite, well beyond the
  ssh-remote-workspaces change that surfaced it).
- BUG-079, BUG-080, BUG-083 — same full-suite-only goroutine-leak flake
  shape (cross-test teardown races), discovered in the same
  ssh-remote-workspaces session; BUG-083 is the closest analog (same
  `goleak`-style symptom, different package/goroutine site).
