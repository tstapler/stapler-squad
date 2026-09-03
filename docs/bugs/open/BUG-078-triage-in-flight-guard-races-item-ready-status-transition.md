# BUG-078: `triageInFlight` clear races the item's `ready` status transition, spuriously rejecting an immediate re-triage [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-08-16 (Gate 4 remote CI on PR #513, webhook-triggers-followups)
**Impact**: Intermittent `already_exists: triage session already running for item <id>` on
`TestCreateBacklogItemFromChat_should_UseChatModeRetriagePrompt_When_RefiningExistingItem`
(`server/services/backlog_service_chat_test.go:137`), CI-only so far — 50/50 local runs passed.
Same class of flake `.claude/rules/fix-flaky-tests-dont-defer.md` was written to stop re-excusing.

## Problem Description

CI run (https://github.com/tstapler/stapler-squad/actions/runs/31929993607/job/95123241760) failed:

```
backlog_service_chat_test.go:137:
    Error: Received unexpected error:
           already_exists: triage session already running for item 2973e1e8-349f-4154-9861-b284a6e83347
```

The test waits for the item's status to reach `ready` via `require.Eventually` (backlog_service_chat_test.go:128-131)
before immediately calling `CreateBacklogItemFromChat` (line 133), which delegates to `TriggerTriage`. The
guard it trips is the single-flight check at `server/services/backlog_service_triage.go:2484`:

```go
if _, alreadyInFlight := s.triageInFlight.LoadOrStore(req.Msg.ItemId, struct{}{}); alreadyInFlight {
    return nil, connect.NewError(connect.CodeAlreadyExists, ...)
}
```

The prior triage goroutine's cleanup (`backlog_service_triage.go:2769-2795`) transitions the item to
`ready` at line 2772, then does an `UpdateItemSessionEnded` DB write (line 2794) and only clears
`triageInFlight` at line 2795 — after the status write is visible to a reader. This exact ordering was
narrowed once already (commit 950ab86df / PR #453, see the comment at line 2782-2789: "leaving
triageInFlight held through auto-spawn's own I/O ... stretched a window where a well-timed retry got a
spurious AlreadyExists — exactly what made TestTriggerTriage_RefineWithFeedback flaky in CI"), but the
window wasn't fully closed: `UpdateItemSessionEnded`'s DB write still sits between the status flip and the
`triageInFlight.Delete` call, and on a loaded CI runner that's apparently still enough to lose the race.

Confirmed unrelated to the webhook-triggers-followups diff: `backlog_service_triage.go` and
`backlog_service_chat_test.go` are not touched by any of PR #513's commits — filed per the blast-radius
exception in `.claude/rules/fix-flaky-tests-dont-defer.md` rather than fixed there, since closing this
race means touching core backlog-triage synchronization, a different subsystem than webhook triggers.

## Fix Approach

- Make the `ready` status transition (line 2772) and the `triageInFlight.Delete` (line 2795) atomic from
  an external reader's point of view — e.g. clear `triageInFlight` immediately after the status transition
  succeeds, before `UpdateItemSessionEnded`, rather than after it. `UpdateItemSessionEnded` only affects the
  orphan-liveness check's `ended_at` input; the doc comment at line 2789-2793 already flags this as
  deliberately coupled to `triageInFlight`'s clear, so re-ordering needs to re-verify `IsTriageLive`/
  `tombstoneOrphanTriageSessions` still can't see an inconsistent state in between.
- Alternative: have `require.Eventually` in the flaky test (and its sibling
  `TestCreateBacklogItemFromChat_should_DelegateToTriggerTriageWithFeedback_When_ExistingItemIdSet`, which
  has the identical wait-then-immediately-call-again shape) also poll on triage no longer being in-flight
  (`s.IsTriageLive` or equivalent), not just item status — a test-side fix, lower risk, doesn't touch
  production synchronization.
- Verify with `go test -race -run 'TestCreateBacklogItemFromChat_should_UseChatModeRetriagePrompt_When_RefiningExistingItem|TestCreateBacklogItemFromChat_should_DelegateToTriggerTriageWithFeedback_When_ExistingItemIdSet' ./server/services/... -count=200` under artificial CPU contention (e.g. `GOMAXPROCS=1` or a background `stress` load) to increase odds of reproducing locally before and after the fix — 50 local runs at normal load didn't reproduce it.
