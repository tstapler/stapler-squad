# BUG-030: Item Reopened for Rework via `AutoReopenAfterFailedReview` Can Get Silently Stranded With No Work Session and No Visible Error [SEVERITY: High]

**Status**: ✅ FIXED (2026-07-22)
**Discovered**: 2026-07-22 — `backlog-feature-improvement` skill audit / `sdd:fix-bug` investigation of BUG-029, live incident on backlog item `54e5aa1f-bd59-4457-9d6a-aa74fe7cd126` ("The camera dialog freezes forever on picture capture", stelekit repo)
**Fixed**: 2026-07-22 — `session/domain/backlog.go`, `server/services/backlog_service_triage.go`, `server/services/backlog_service_stuck.go`, `proto/session/v1/backlog.proto`, `web-app/src/components/backlog-stuck/stuckReason.ts`, `web-app/src/components/backlog-stuck/stuckReason.css.ts`
**Impact**: A backlog item can transition `review → in_progress` (the correct effect of `AutoReopenAfterFailedReview` reacting to a FAIL/PARTIAL verdict) while never getting a new work session spawned. If `SpawnSessionFromItem` then failed and the follow-up rollback to `review` *also* failed (its scoped precondition no longer matching), the double failure was only ever `log.ErrorLog.Printf`'d — the item was left permanently stranded `in_progress` with zero live sessions and no signal visible anywhere: not a notification, not a durable stuck row, not caught by `ListStuckBacklogItems` (none of its detectors check "in_progress with no live/attached work session").

## Live Incident Reproduction (2026-07-22)

Item `54e5aa1f-bd59-4457-9d6a-aa74fe7cd126`'s review-role session (tmux `staplersquad_review_54e5aa1f`) submitted a **PARTIAL** verdict on 2026-07-20 and its pane died. The item's `review → in_progress` transition happened at `2026-07-22T02:01:47Z`. At the time of writing (~15h later): `search_sessions("camera")` and a `backlog:work`-tagged search of the stelekit repo both returned zero live sessions for this item; its only work-role `item_session` (`fd5740cf-5965-436d-b3e5-7cf92280b7ef`) had already ended (`endedAt: 2026-07-21T23:41:03Z`); no new work-role session existed for the `in_progress` stay. Invisible to `ListStuckBacklogItems`.

## Root Cause

`AutoReopenAfterFailedReview` (`server/services/backlog_service_triage.go:962`) transitions `review → in_progress`, then calls `SpawnSessionFromItem` with `Autonomous: true`; on failure it rolls the item back to `review` using a precondition scoped to the `in_progress` row it just wrote (`ExpectedStatus: in_progress`, `ExpectedUpdatedAt` from that write). **If the rollback's own `TransitionBacklogItemStatus` call also fails** (its precondition no longer matches — e.g. something else touched the item's `updated_at` in the interim), the only thing that happened was `log.ErrorLog.Printf(...)`. Nothing marked the item stuck, nothing notified an operator, and no existing `StuckReason` covers "in_progress with zero live sessions" — so a double failure here was, and would remain, permanently invisible.

**The exact trigger for this specific live incident was not conclusively pinned down.** Two candidate gates (the WIP-cap gate, and the `hasActiveWorkSession` duplicate-work-session guard) were checked against this item's actual data and ruled out — both correctly bypassed/cleared for a reopen with an already-ended prior work session. Manually re-invoking `SpawnSessionFromItem(itemId=54e5aa1f, autonomous=true)` against the live server succeeded immediately and produced a real work session (which then ran and shipped the item to `done`) — so the spawn path itself is not deterministically broken; whatever caused the original failure was transient (a race, a concurrent operation, or an environment hiccup during the original ~29h-earlier attempt) and could not be reproduced or found in logs (the relevant window had already rotated out of `~/.stapler-squad/logs/` by the time of investigation).

Given that, the fix targets the **provable, independently real defect** — a double failure here is silently unrecoverable and invisible — rather than a still-unconfirmed one-off trigger. This is the correct fix regardless of what caused this specific incident: without it, *any* future transient spawn+rollback double-failure (for any reason) strands an item invisibly forever.

## Fix Applied

- `session/domain/backlog.go`: added `StuckReasonSpawnFailed` (`"spawn_failed"`) to the validated `StuckReason` enum.
- `server/services/backlog_service_triage.go`: added `notifySpawnAndRollbackFailed`, mirroring the existing `notifyReworkCapHit`/`notifyRepeatedFailure` pattern — writes a durable `BacklogStuckState` row (`StuckReasonSpawnFailed`) plus an operator-facing event-bus notification. Wired into `AutoReopenAfterFailedReview`'s rollback-failure branch (previously log-only).
- `proto/session/v1/backlog.proto` + regenerated bindings (`make proto-gen`): added `STUCK_REASON_SPAWN_FAILED = 9`.
- `server/services/backlog_service_stuck.go`: added the new value to `toProtoStuckReason`/`fromProtoStuckReason`.
- `web-app/src/components/backlog-stuck/stuckReason.ts` + `stuckReason.css.ts`: added the label/icon/class entries the `Record<StuckReason, T>` exhaustiveness pattern requires (this is itself a Level-1 enforcement mechanism — the file's own comment notes adding a new proto enum value is a compile error here until mapped, which caught the need for this update mechanically).

## Live Item Remediation

Manually re-invoked `SpawnSessionFromItem` (`autonomous: true`) against the live item as part of verification — it spawned a real work session, which ran to completion and shipped the item. `GetBacklogItem` now reports `status: "done"`.

## Files Affected

- `session/domain/backlog.go`
- `server/services/backlog_service_triage.go`
- `server/services/backlog_service_stuck.go`
- `proto/session/v1/backlog.proto` (+ generated `gen/proto/go/session/v1/*`, `web-app/src/gen/session/v1/*`)
- `web-app/src/components/backlog-stuck/stuckReason.ts`
- `web-app/src/components/backlog-stuck/stuckReason.css.ts`

## Verification

- `TestNotifySpawnAndRollbackFailed_should_markStuckAndNotify_When_Called` (`server/services/backlog_service_triage_test.go`) — calls the new helper directly (same test-boundary convention as the existing `notifyReworkCapHit`/`notifyRepeatedFailure` tests in this file) and asserts a durable `StuckReasonSpawnFailed` row is created with the failure context, and an operator notification publishes. **Scoping note**: this tests the fix's own contract directly rather than forcing the full spawn-fails-then-rollback-fails race end-to-end — doing the latter deterministically would require a test-only hook into `AutoReopenAfterFailedReview`'s internals that doesn't exist and isn't worth adding for this. The live-item remediation above is the end-to-end confirmation that the surrounding spawn path itself works correctly.
- `go test ./server/services/...` and `go test ./session/...` — full existing suites green, no regressions.
- `npx jest --no-coverage --testPathPatterns="stuckReason|backlog-stuck"` — all 75 tests across 5 suites green, including the exhaustive `Record<StuckReason, T>` compile-time check picking up the new value with no gaps.
- `go build ./...`, `make build`, `make proto-gen` — clean.
- `golangci-lint run ./session/... ./server/services/...` — 0 issues.
- Live: manually re-triggered the actual stuck item's spawn via the running server's RPC; confirmed a real session spawned and the item shipped to `done`.

## Reflection (Phase D — fix the class, not the instance)

**Classification**: Integration Gap — two individually-reasonable pieces of code (a scoped-precondition rollback added specifically to avoid clobbering concurrent legitimate state changes, per BUG-026, and a bare error-log call site) interact to produce a failure mode neither one alone would: the rollback's own safety mechanism (refusing to blindly overwrite) means it can legitimately decline to run, and when it does, nothing was in place to notice.

**Earliest achievable enforcement**: A unit test on the notify helper's contract is the practical achievable level here — the underlying trigger condition (a spawn failure racing a rollback-precondition mismatch) is not something a type system or lint rule can express or reach; it requires the actual concurrent-state interaction. Considered and rejected: an integration test forcing the full race would require adding a test-only injection seam into production code with no other use — not worth the complexity for a defensive/visibility fix. The regression test added is the correct level.

**Recurring shape**: Directly the "a spawn/transition call silently doesn't produce the state the pipeline expects, with no error surfaced" shape already named in `docs/tasks/backlog-feature-improvement.md`'s audit history and the `backlog-feature-improvement` skill's "Prefer systemic fixes over instance patches" section — and a sibling to BUG-026's and BUG-029's shape ("a safety mechanism added to prevent one bad outcome creates a narrow gap where a different bad outcome goes unnoticed"). All three fixes in this general area (`AutoReopenAfterFailedReview` and its downstream sweeps) have now independently needed their own visibility/correctness patch. Flagged, same as BUG-029, for a future `quality:architecture-review` pass scoped to this reconciliation/reopen pathway as a whole, rather than continuing to patch one gap at a time as each is discovered live.

## Related

- BUG-029 (`docs/bugs/fixed/BUG-029-unprocessed-review-verdict-sweep-picks-wrong-session.md`) — found in the same audit pass, same two live repro items, different code path and root cause.
- BUG-026 (`docs/bugs/fixed/BUG-026-backlog-transition-status-toctou-reopen.md`) — introduced the scoped-rollback-precondition pattern this bug's rollback branch uses; that fix correctly prevented one failure mode (clobbering concurrent legitimate state) but, as originally shipped, had no handling for the rollback itself declining to apply.
