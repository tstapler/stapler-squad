# BUG-053: Graceful Shutdown Kills In-Flight Triage LLM Calls and the Recovery Path Treats the Kill as a Real Failure [SEVERITY: Medium]

**Status**: ✅ FIXED (2026-08-01)
**Discovered**: 2026-08-01, live — user reported that automated backlog triage "doesn't show up" and has to be re-triggered manually.
**Fixed**: 2026-08-01 — `session/ent/schema/item_session.go`, `session/storage_backlog.go`, `session/storage.go`, `session/ent_repository_backlog.go`, `session/repository.go`, `server/services/backlog_service_triage.go`, `session/backlog_lifecycle.go`.
**Impact**: Every routine service restart (`make install-service`, or any `systemctl --user restart stapler-squad`) that lands while a headless triage call is in flight discards that call and forces the affected item(s) through the same 30m/2h/8h/24h/72h exponential backoff schedule built for OOM-crash bursts — even though the "failure" was entirely self-inflicted by the restart and carries zero evidence the triage itself would have failed. The user experiences this as "triage silently doesn't run; I have to notice and re-trigger it myself."

## Problem Description

Confirmed live against the running service (`~/.stapler-squad/logs/staplersquad.log`, `ListStuckBacklogItems` RPC):

1. The service restarted at 11:27:18 PDT today (new PID, confirmed via `systemctl --user show stapler-squad -p ActiveEnterTimestamp`).
2. Three in-flight `TriggerTriage` headless calls were canceled mid-run:
   ```
   ERROR: [TriggerTriage] headless triage failed item=d442f133... elapsed=4m58s errType=shutdown: context canceled
   ERROR: [TriggerTriage] headless triage failed item=7271b842... elapsed=5m0s errType=shutdown: context canceled
   ERROR: [TriggerTriage] headless triage failed item=a8a2505e... elapsed=4m58s errType=shutdown: context canceled
   ```
3. On restart, the existing orphaned-triage detector (`reconcileOrphanedTriageItems`, `session/backlog_lifecycle.go`) correctly found 6 dangling triage sessions (3 from the mid-flight kill above, plus 3 more from an earlier round) and:
   - Marked each item `STUCK_REASON_ORPHANED_TRIAGE`
   - Sent a "Triage may be stuck" notification for each
   - Dispatched one immediate auto-respawn (attempt 1, which fires with no backoff by design)
4. `remediation_attempts` was now `1` for all 6 rows, with `next_remediation_at` ~30 minutes out (`remediationBackoffSchedule[0]`, `session/backlog_remediation.go`) — i.e. if *that* respawned attempt is *also* orphaned by a second restart before it finishes (headless triage calls routinely take 7–15 minutes), the item is locked out of automatic recovery for 30 minutes, then 2 hours, and so on, eventually parking after `MaxRemediationAttempts` and requiring a manual "Reset" click.

**Root cause**: `TriggerTriage`'s async goroutine (`server/services/backlog_service_triage.go`) derives its LLM-call context directly from `s.shutdownCtx` — the same context that is canceled the instant the process receives SIGTERM:

```go
triageCtx, cancel := context.WithTimeout(s.shutdownCtx, 30*time.Minute)
...
raw, _, callErr := s.headlessPool.CallBlocking(triageCtx, ...)
```

`classifyHeadlessCallError` *already* correctly buckets this exact case as `"shutdown"` (as opposed to `"timeout"`, `"process_error"`, `"claude_not_found"`, `"other"`) — visible in the very error message logged above — but that classification was discarded after being written to the log. `UpdateItemSessionEnded` persisted only a timestamp, nothing else. When the orphan-detection sweep runs after restart, it has no way to tell "this row died because our own process was restarting" apart from "this row died because the triage logic itself is broken" — both look identical: an `ItemSession` with `EndedAt` set and the item still in `idea`. So both get routed through the same `MarkStuck` → notify → `RemediationDue` exponential-backoff path that `remediationBackoffSchedule` (`session/backlog_remediation.go`) was deliberately sized for *crash bursts*, not single, predictable, self-inflicted deploy restarts.

Note this is **not** a bug in the shutdown/cancellation behavior itself — killing in-flight subprocess LLM calls on SIGTERM is the correct, intentional behavior for a bounded graceful-shutdown model (`pkg/warren`'s `App.Stop`/`ShutdownTimeout`); making the process wait minutes for an LLM call to finish before a routine deploy restart would be a much bigger, riskier change and was deliberately not pursued here. The bug is narrower: the system already *computes* the distinguishing signal (via `classifyHeadlessCallError`) at the exact moment it would be useful, then throws it away before the recovery path that actually needs it ever sees it.

## Fix Applied

1. Added `end_reason` (optional string, default `""`) to the `ItemSession` ent schema — set only alongside `ended_at`, holding `classifyHeadlessCallError`'s bucket name (or `""` for a normal/successful end). Regenerated via the required `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`.
2. Added `EntRepository.UpdateItemSessionEndedWithReason` / `Storage.UpdateItemSessionEndedWithReason` (the existing `UpdateItemSessionEnded` now delegates to it with `reason=""`, so all 16 other call sites are unaffected).
3. `TriggerTriage`'s failure branch (`server/services/backlog_service_triage.go`) now persists the already-computed `errType` via `UpdateItemSessionEndedWithReason` instead of discarding it.
4. `reconcileOrphanedTriageItems`'s shape-2 branch (already-ended session, item still `idea` — `session/backlog_lifecycle.go`) now checks `latestTriage.EndReason`. When it is `"shutdown"`, the item is respawned immediately via the existing `TriageRespawner.AutoRespawnTriage`, with **no** `MarkStuck` call, **no** "Triage may be stuck" notification, and **no** `RemediationDue` accounting — the self-inflicted orphan is treated as if it never happened. Any other end reason (`""`, `"timeout"`, `"process_error"`, `"claude_not_found"`, `"other"`) keeps the existing `MarkStuck` + notify + backoff-gated remediation behavior unchanged.

Per `.claude/rules/interface-pollution-checklist.md`: no new interface or wrapper type introduced — `UpdateItemSessionEndedWithReason` is a plain additional method next to the existing one (same pattern the file already uses, e.g. `UpdateItemSessionGitActivity`), and the `end_reason` field is a single primitive column, not a new abstraction layer.

**Scope note**: this fix targets the `orphaned_triage` recovery path only, since that's the one the user reported. The same shape — a graceful-shutdown-caused orphan being indistinguishable from a real failure — exists identically in the four sibling remediation paths (`AutoRespawnReview`/`markAbandonedReview`, `RemediateStaleWorkSession`, push-failed retry, PR-fix retry; see the "automated X retry has been retried N times" notification strings in `session/backlog_lifecycle.go`). None of those were touched here. See Reflection below.

## Regression Test

`session/backlog_lifecycle_stuck_test.go`:

- `TestReconcileOrphanedTriageItems_should_respawnImmediatelyWithNoPenalty_When_EndedByGracefulShutdown` — creates an item with a triage session ended via `UpdateItemSessionEndedWithReason(..., "shutdown")`, runs `reconcileOrphanedTriageItems`, and asserts: (a) `AutoRespawnTriage` is dispatched immediately, (b) no `BacklogStuckState` row is created (`FindOpenStuckStates` empty — no remediation-attempt penalty), (c) no notification fires.

**Verified to fail against pre-fix code**: this exact scenario (an `ItemSession` ended while the item is still `idea`) is the pre-existing shape-2 case; before this fix, every occurrence unconditionally fell through to `MarkStuck` + notify, so the assertions `assert.Empty(t, open, ...)` and `assert.Empty(t, notifier.titles(), ...)` would have failed against the prior code (which had no `EndReason` field to branch on at all — the check simply didn't exist).

## Verification

```
$ go build ./...
(clean)

$ gofmt -l session/backlog_lifecycle.go session/ent/schema/item_session.go session/ent_repository_backlog.go \
    session/storage.go session/storage_backlog.go session/repository.go server/services/backlog_service_triage.go
(clean — no output)

$ go test ./session/ -run 'TestReconcileOrphanedTriageItems' -v
=== RUN   TestReconcileOrphanedTriageItems_should_writeDurableRowNotifyOnce_When_TriageSessionStale
--- PASS
=== RUN   TestReconcileOrphanedTriageItems_should_tombstoneStaleSession_When_Detected
--- PASS
=== RUN   TestReconcileOrphanedTriageItems_should_notFlag_When_TriageSessionRecent
--- PASS
=== RUN   TestReconcileOrphanedTriageItems_should_flagHeadlessSession_After30Min
--- PASS
=== RUN   TestReconcileOrphanedTriageItems_should_flagImmediately_When_TriageSessionEndedWithoutTransition
--- PASS
=== RUN   TestReconcileOrphanedTriageItems_should_respawnImmediatelyWithNoPenalty_When_EndedByGracefulShutdown
--- PASS
=== RUN   TestReconcileOrphanedTriageItems_should_preferNewerOpenSession_When_OlderEndedSessionExists
--- PASS
=== RUN   TestReconcileOrphanedTriageItems_should_notFlag_When_NoTriageSessionEverRan
--- PASS
PASS
ok  	github.com/tstapler/stapler-squad/session	1.621s
```

Broader run to confirm no unrelated regressions:

```
$ go build ./... && go test ./session/... ./server/services/...
ok  	github.com/tstapler/stapler-squad/session	44.994s
ok  	github.com/tstapler/stapler-squad/session/git	3.494s
ok  	github.com/tstapler/stapler-squad/session/headless	0.800s
ok  	github.com/tstapler/stapler-squad/session/memory	0.005s
ok  	github.com/tstapler/stapler-squad/session/mux	4.367s
ok  	github.com/tstapler/stapler-squad/session/namegen	0.012s
ok  	github.com/tstapler/stapler-squad/session/prompts	0.610s
ok  	github.com/tstapler/stapler-squad/session/queue	0.007s
ok  	github.com/tstapler/stapler-squad/session/scrollback	0.028s
ok  	github.com/tstapler/stapler-squad/session/search	0.022s
ok  	github.com/tstapler/stapler-squad/session/tmux	44.787s
ok  	github.com/tstapler/stapler-squad/session/tokens	0.444s
ok  	github.com/tstapler/stapler-squad/session/unfinished	2.352s
ok  	github.com/tstapler/stapler-squad/session/unfinished/gogitstore	62.314s
ok  	github.com/tstapler/stapler-squad/session/vc	2.247s
ok  	github.com/tstapler/stapler-squad/session/vnc	0.006s
ok  	github.com/tstapler/stapler-squad/session/workspace	0.068s
ok  	github.com/tstapler/stapler-squad/server/services	71.178s
```

24 packages exercised, zero `FAIL` lines.

## Related

- `docs/tasks/backlog-feature-improvement.md`, 2026-07-27 update — the `reconcileOrphanedTriageRemediation`/`retryOrphanedTriageWithBackoffGate` machinery this fix modifies was itself added in response to items `4f03de7b`/`505fb733` sitting in `idea` for 2 days with only one notification ever sent.
- Same recurring shape as that update's own precedent: "an event (a session ending) is lost across a service restart with no catch-up path" — except here the event *was* captured (in a log line) and the gap was narrower: the catch-up path existed, but the specific signal it needed to make the right decision was computed and then thrown away rather than persisted.

## Reflection (Phase D — fix the class, not the instance)

**Classification**: Integration Gap. Two pieces of already-correct logic — `classifyHeadlessCallError`'s bucketing and `reconcileOrphanedTriageItems`'s shape-2 detection — never had a wire connecting them. Each was individually right; the seam between "a headless call ends" and "a later, different-process sweep decides how to react to that ending" dropped the one piece of information that would have let the second reason correctly about the first.

**Earliest achievable enforcement**: A unit test is the earliest practical level here. This isn't a type-system-expressible invariant (nothing prevents a future call site from persisting an end without a reason field via plain values), and no lint rule can detect "a computed classification was logged but not persisted" without deep, project-specific static analysis disproportionate to a single field. The regression test added directly exercises the exact condition (`EndReason == "shutdown"`) that must route to the no-penalty path, which is the correct ceiling for this kind of cross-process-restart behavioral test.

**Recurring shape confirmed**: This is another instance of `docs/tasks/backlog-feature-improvement.md`'s named recurring shape — **"an event is lost across a service restart with no catch-up path."** Unlike the prior instances (a status transition never applied, a notify-once flag marked but never resolved), here the "catch-up path" (the orphan-detection sweep) already existed and already ran — what was missing was the *cause* information the catch-up path needed to react correctly, not the catch-up path itself. Prior fixes for this shape closed the "does recovery happen at all" gap; this fix closes a narrower, second-order version: "does recovery happen *appropriately*, given why the thing it's recovering from actually happened."

**Gap intentionally left open**: the four sibling remediation paths (review, stale-work, push-failed, PR-fix retry — all sharing `RemediationDue`/`remediationBackoffSchedule`) have the identical latent gap: a graceful-shutdown-caused orphan in any of them is still indistinguishable from a real failure and will still consume a remediation attempt today. A future audit pass should not re-discover this as a new finding — it's the same shape, deliberately scoped out of this fix because the user's report was specifically about triage, and generalizing the `end_reason` signal to `ItemSession`-adjacent state used by the other four paths (review sessions, work sessions) is a larger, cross-cutting change better suited to its own pass once it's clear the pattern established here (persist the classification already being computed; branch on it in the orphan sweep; no new stuck-state row or backoff penalty for a self-inflicted cause) is the right shape to generalize.
