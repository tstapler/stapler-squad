# Validation Plan: review-gate-stale-session-rework

**Date**: 2026-07-24

## Happy Path Scenario

Given a `review`-status backlog item whose review verdict just FAILed and whose prior work session is still alive (per the task-protocol's "stay in this session" instruction), when that session produces no output for longer than 15 minutes (`maxReworkBlockStaleness`), then the item is durably marked `StuckReasonReworkBlockedStale`, appears in the stuck-items list with clear, distinct copy, and the user can click through to the existing "Reopen for Revision" action to resolve it — all without relying on having seen the original one-shot toast.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| A: rework-block gate uses its own threshold, not the 2-min Review Queue value | `server/services/backlog_service_triage_test.go` | `notifyIfActiveWorkSessionStale_should_notFire_When_IdleUnder15Min` | Unit | Happy path — 10 min idle, no notification/mark |
| A: rework-block gate uses its own threshold, not the 2-min Review Queue value | `server/services/backlog_service_triage_test.go` | `notifyIfActiveWorkSessionStale_should_fire_When_IdleOver15Min` | Unit | Error/trigger path — 20 min idle, notification + mark fire |
| A: general Review Queue badge threshold recalibrated 2min → 5min | `session/review_queue_determiner_test.go` | `determineAttentionReason_should_notFlagStale_When_TimeSinceOutputUnder5Min` | Unit | Happy path — 3 min since output, no stale flag |
| A: general Review Queue badge threshold recalibrated 2min → 5min | `session/review_queue_determiner_test.go` | `determineAttentionReason_should_flagStale_When_TimeSinceOutputOver5Min` | Unit | Trigger path — 6 min since output, stale flag set |
| A: new threshold constant documented and distinct from the other two | `session/review_queue_poller_test.go` | `DefaultReviewQueuePollerConfig_should_return5MinStalenessThreshold_When_Called` | Unit | Regression guard on the config default |
| B: `StuckReasonReworkBlockedStale` registered at domain level | `session/domain/backlog_test.go` | `StuckReasonReworkBlockedStale_should_beValid_When_Checked` | Unit | Happy path — enum validity |
| B: `StuckReasonReworkBlockedStale` registered at domain level | `session/domain/backlog_test.go` | `AllStuckReasons_should_contain12Entries_When_Enumerated` | Unit | Regression guard — count assertion catches a future accidental removal |
| B: proto enum round-trips correctly | `server/services/backlog_service_stuck_test.go` | `toProtoStuckReason_should_mapReworkBlockedStale_When_Called` | Unit | Happy path |
| B: proto enum round-trips correctly | `server/services/backlog_service_stuck_test.go` | `fromProtoStuckReason_should_mapReworkBlockedStale_When_Called` | Unit | Happy path (reverse direction) |
| B: `MarkStuck` fires on threshold breach | `server/services/backlog_service_triage_test.go` | `notifyIfActiveWorkSessionStale_should_callMarkStuck_When_ThresholdExceeded` | Unit | Happy path |
| B: `MarkStuck` fires on threshold breach | `server/services/backlog_service_triage_test.go` | `notifyIfActiveWorkSessionStale_should_notCallMarkStuck_When_ThresholdNotExceeded` | Unit | Error/negative path |
| B: `MarkStuck` handles status-precondition race gracefully | `server/services/backlog_service_triage_test.go` | `notifyIfActiveWorkSessionStale_should_skipGracefully_When_StatusPreconditionMismatched` | Unit | Error path — item moved off `review` between read and write |
| B: existing notification is never regressed by the new durable-mark logic | `server/services/backlog_service_triage_test.go` | `notifyIfActiveWorkSessionStale_should_stillPublishNotification_When_MarkStuckErrors` | Unit | Error path — storage failure must not suppress the pre-existing toast |
| B: coexists with other open stuck reasons on the same item | `server/services/backlog_service_triage_test.go` | `notifyIfActiveWorkSessionStale_should_addSecondOpenReason_When_ItemAlreadyHasReworkCapRowOpen` | Unit | Edge case |
| B: resolve pass clears the row when the session recovers | `server/services/backlog_service_triage_test.go` | `ResolveReworkBlockedStaleIfRecovered_should_resolveStuckRow_When_SessionRecovered` | Unit | Happy path |
| B: resolve pass leaves the row open when still stale | `server/services/backlog_service_triage_test.go` | `ResolveReworkBlockedStaleIfRecovered_should_leaveRowOpen_When_StillStale` | Unit | Negative path |
| B: resolve pass no-ops when the work session already ended | `server/services/backlog_service_triage_test.go` | `ResolveReworkBlockedStaleIfRecovered_should_beNoOp_When_NoActiveWorkSession` | Unit | Edge case (belt-and-suspenders) |
| B: resolve orchestration delegates correctly per open row | `session/backlog_lifecycle_test.go` | `reconcileReworkBlockedStaleResolution_should_delegateToResolver_When_OpenRowsExist` | Unit | Happy path |
| B: resolve orchestration is a no-op with nothing open | `session/backlog_lifecycle_test.go` | `reconcileReworkBlockedStaleResolution_should_beNoOp_When_NoOpenRows` | Unit | Negative path |
| B: `ReworkBlockStaleResolver` wired end-to-end from tick to storage | `server/services/backlog_service_triage_test.go` (integration-style, real `session.Storage` + sqlite test DB, matching this repo's existing integration-test convention for `MarkStuck`/`FindOpenStuckStates`) | `TestReworkBlockedStale_should_markAndLaterResolve_When_SessionStallsThenRecovers_Integration` | Integration | Full round-trip: mark → appears in `FindOpenStuckStates` → session recovers → resolved on next reconcile pass |
| B: UI renders the new reason distinctly | `web-app/src/components/backlog-stuck/stuckReason.test.ts` | `getStuckReasonLabel_should_returnDistinctLabel_When_ReworkBlockedStale` | Unit | Happy path |
| B: UI renders the new reason distinctly | `web-app/src/components/backlog-stuck/stuckReason.test.ts` | `getStuckReasonIcon_should_returnNonFallbackIcon_When_ReworkBlockedStale` | Unit | Regression guard — catches accidental fallthrough to `UNSPECIFIED` |
| B: click-through path to the existing "Reopen for Revision" action | `web-app/src/components/backlog-stuck/StuckItemsSection.test.tsx` (or `StuckItemDetail.test.tsx`) | `StuckItemsSection_should_linkToItemDetail_When_ReasonIsReworkBlockedStale` | Component | Happy path |
| D: task-protocol cadence text includes a concrete interval | `session/backlog_context_test.go` (only if an existing snapshot test needs updating — see plan.md Task 3.1.1b) | `BuildSessionInitialPrompt_should_includeConcretePollingInterval_When_TaskProtocolRendered` | Unit | Happy path (add only if a test already exists to update; do not add a new prose-snapshot test purely for this per plan.md's own guidance) |

## UX Acceptance Tests

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| User reaches the resolution action in ≤2 clicks from the stuck-items list | `web-app/src/components/backlog-stuck/StuckItemsSection.test.tsx` | `StuckItemsSection_should_linkToItemDetail_When_ReasonIsReworkBlockedStale` (same as above) | RTL (component) | Render list with a `REWORK_BLOCKED_STALE` item → click card → assert navigation target is the item detail route |
| Label distinguishes this state from `StuckReasonStaleWork` | `web-app/src/components/backlog-stuck/stuckReason.test.ts` | `getStuckReasonLabel_should_returnDistinctLabel_When_ReworkBlockedStale` (same as above) | Jest | Assert label string is neither equal to nor a substring of `STALE_WORK`'s label |
| No dead end — "Reopen for Revision" always reachable | Manual (or extend `StuckItemDetail.test.tsx` if a suitable render-through pattern exists) | `StuckItemDetail_should_showReopenAction_When_ReworkBlockedStale` | RTL / Manual | Render `StuckItemDetail` with a `REWORK_BLOCKED_STALE` item whose backing verdict is FAIL → assert `GateVerdictBox`'s "Reopen for Revision" button renders |
| Icon is never the sole signal (text label always present) | `web-app/src/components/backlog-stuck/stuckReason.test.ts` | `getStuckReasonIcon_should_returnNonFallbackIcon_When_ReworkBlockedStale` (same as above, extended assertion) | Jest | Assert both `getStuckReasonIcon` and `getStuckReasonLabel` return non-empty, non-fallback values together |
| Keyboard navigable | Manual | N/A — inherited from existing components | Manual | Tab to the new card/label in a live or Storybook-equivalent render; confirm no new focus trap or unreachable control was introduced (there should be none — no new interactive control added by this feature) |

## Test Stack

- **Unit (Go)**: standard library `testing` + this repo's existing table-driven conventions (confirmed via `session/ent_repository_backlog_stuck_test.go`, `server/services/backlog_service_triage_test.go`).
- **Integration (Go)**: sqlite-backed `EntRepository`/`session.Storage` test harness already used by `TestMarkStuck_*`/`TestFindOpenStuckStates_*` in `session/ent_repository_backlog_stuck_test.go` — reuse that harness rather than introducing a new one.
- **Unit/Component (TypeScript)**: Jest + React Testing Library, matching `stuckReason.test.ts`/`StuckItemsSection.test.tsx`'s existing conventions.
- **E2E / UX**: Playwright, only as a fallback if no suitable component-test pattern exists for the click-through assertion (see plan.md Task 2.2.2a) — per `.claude/rules/e2e-test-conventions.md` if used.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./server/services/... ./session/... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line on the changed files (`backlog_service_triage.go`, `backlog_lifecycle.go`, `backlog_service_stuck.go`, `domain/backlog.go`) |
| TypeScript/Jest | `cd web-app && npx jest --coverage --testPathPatterns="backlog-stuck"` | ≥80% line on `stuckReason.ts` and the touched components |

- All public/exported methods touched by this plan (`notifyIfActiveWorkSessionStale`, `ResolveReworkBlockedStaleIfRecovered`, `reconcileReworkBlockedStaleResolution`, `toProtoStuckReason`/`fromProtoStuckReason`) have happy-path + error-path coverage per the table above.
- The one external-dependency-shaped integration (durable storage round-trip via ent/sqlite) has the dedicated integration test listed above.
- Every UX acceptance criterion in `design/ux.md` has a corresponding test or explicit manual step in the table above.

## Live-Data Verification (required — see requirements.md Success Metrics)

This bug was originally discovered via live observation (37/41 items flagged stale), not a failing automated test, and requirements.md's Success Metrics explicitly require live re-verification, not just green unit tests. This step is a **hard requirement of Phase 6 (Verify) / pre-ship checklist**, not optional polish:

1. After deploying (`make install-service`, mindful of `.claude/rules/tmux-keep-server-on-restart.md`), observe the live Review Queue's "Stale" badge count over a normal working session and confirm it no longer approaches the 37/41 false-positive ratio from the original report.
2. If/when a real `AutoReopenAfterFailedReview`-blocked scenario occurs live, confirm: (a) the existing toast still fires, (b) the item appears in the stuck-items list with the new reason and correct "since" duration, (c) clicking through reaches "Reopen for Revision," and (d) if the session later recovers on its own, the stuck row resolves without manual intervention.
3. Record the outcome (pass/needs-more-tuning) against ADR-001's explicitly-flagged-as-best-effort threshold values — if either needs adjustment, update ADR-001's rationale table rather than silently changing the constant (per ADR-001's own Consequences section).
