# Validation Plan: backlog-bounce-escalation

**Date**: 2026-08-11

## Happy Path Scenario
Given a backlog item in `in_progress`/`review` that already has durable per-reason
`backlog_stuck_states` rows but no aggregate escalation signal (the Baseline), when it
accumulates 2 simultaneous open non-escalation stuck reasons (e.g. `bouncing` +
`abandoned_review`), then `reconcileMultiReasonEscalation` marks a durable
`StuckReasonMultipleReasons` row within one reconcile tick and, once `multiReasonNotifyDwell`
has elapsed, fires a differentiated `ERROR`/`URGENT` notification and the item renders under
its own escalated group in the Stuck Items UI — without the user cross-referencing
`backlog_stuck_states` by hand. *(This is the primary motivating example — items `ccbfe7a6` /
`e271db3d` from requirements.md's Problem Statement — and anchors all test design below;
Signal 2 (capped-while-bouncing) and the flaky-test-review deferral are variations, not
equal-priority scenarios.)*

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| REQ-1/REQ-2 shared plumbing: `StuckReason` constants valid end-to-end (Story 1.1.1) | `session/domain/backlog_test.go` | `TestStuckReasonMultipleReasons_should_beValid_When_Checked` | Unit | Happy path |
| REQ-1/REQ-2 shared plumbing (Story 1.1.1) | `session/domain/backlog_test.go` | `TestStuckReasonBounceCapExhausted_should_beValid_When_Checked` | Unit | Happy path |
| REQ-1/REQ-2 shared plumbing — omission guard (Story 1.1.1) | `session/domain/backlog_test.go` | `TestAllStuckReasons_should_contain16Entries_When_Enumerated` (existing test, count bumped 14→16) | Unit | Error/regression path — catches a forgotten `AllStuckReasons` registration |
| REQ-1/REQ-2 shared plumbing — proto round-trip (Story 1.1.2) | `server/services/backlog_stuck_rpc_test.go` | `TestToProtoStuckReason_should_ReturnMultipleReasons_When_DomainStuckReasonMultipleReasons` | Unit | Happy path |
| REQ-1/REQ-2 shared plumbing (Story 1.1.2) | `server/services/backlog_stuck_rpc_test.go` | `TestFromProtoStuckReason_should_ReturnBounceCapExhausted_When_ProtoBounceCapExhausted` | Unit | Happy path |
| REQ-1/REQ-2 shared plumbing (Story 1.1.2) | `server/services/backlog_stuck_rpc_test.go` | `TestToProtoStuckReason_should_mapToUnspecified_When_UnknownString` (existing, reused unchanged) | Unit | Error path — unknown string still falls back safely |
| REQ-1/REQ-2 shared plumbing — remediation-action exhaustiveness guard | `server/services/backlog_stuck_rpc_test.go` | `TestRemediationActionByReason_should_beDecidedForEveryStuckReason_When_NewReasonIsAdded` (existing, reused unchanged) | Unit | Error path — forces an explicit (likely "no action") remediation-action decision for both new reasons instead of a silent default |
| REQ-1: multi-reason severity signal (Story 1.2.1) | `session/stuck_decisions_test.go` | `TestIsMultiReasonEscalated_should_returnTrue_When_CountAtOrAboveThreshold` | Unit | Happy path |
| REQ-1 (Story 1.2.1) | `session/stuck_decisions_test.go` | `TestIsMultiReasonEscalated_should_returnFalse_When_CountBelowThreshold` | Unit | Error/negative path |
| REQ-1 (Story 1.2.1) | `session/stuck_decisions_test.go` | `TestMultiReasonEscalationNotifyReady_should_returnTrue_When_DwellElapsed` | Unit | Happy path |
| REQ-1 (Story 1.2.1) | `session/stuck_decisions_test.go` | `TestMultiReasonEscalationNotifyReady_should_returnFalse_When_WithinDwell` | Unit | Error/negative path |
| REQ-1: multi-reason detector (Story 1.2.4a) | `session/backlog_lifecycle_stuck_test.go` | `TestReconcileMultiReasonEscalation_should_MarkStuckWithoutNotifying_When_ThresholdFirstCrossed` | Integration | Happy path — escalate without premature notify |
| REQ-1 (Story 1.2.4a) | `session/backlog_lifecycle_stuck_test.go` | `TestReconcileMultiReasonEscalation_should_Notify_When_DwellElapsedAndStillOpen` | Integration | Happy path — dwell-gated notify fires |
| REQ-1 (Story 1.2.4b) | `session/backlog_lifecycle_stuck_test.go` | `TestReconcileMultiReasonEscalation_should_ResolveStuck_When_CountDropsBelowThreshold` | Integration | Happy path — de-escalation |
| REQ-1 (Story 1.2.4c) | `session/backlog_lifecycle_stuck_test.go` | `TestReconcileMultiReasonEscalation_should_ExcludeEscalationReasonsFromCount_When_Counting` | Integration | Error/edge path — guards against self-reinforcing escalation count (ADR-001 consequence) |
| REQ-2: capped-while-bouncing marker (Story 1.3.3a) | `session/backlog_lifecycle_test.go` | `TestAutoReopenWithBackoffGate_should_MarkBounceCapExhausted_When_JustParked` | Integration | Happy path |
| REQ-2 (Story 1.3.1, AC negative case) | `session/backlog_lifecycle_test.go` | `TestAutoReopenWithBackoffGate_should_NotMarkBounceCapExhausted_When_NotYetParked` | Integration | Error/negative path — attempt count below cap must not mark the row |
| REQ-2 (Story 1.3.1, itemStatus-passthrough pitfall) | `session/backlog_lifecycle_test.go` | `TestAutoReopenWithBackoffGate_should_PassActualItemStatus_When_MarkingBounceCapExhausted` | Integration | Error/regression path — guards the plan's named pitfall (hardcoded `BacklogStatusInProgress` would make `MarkStuck`'s `expectedStatus` precondition silently fail for `review`-status items) |
| REQ-2 (Story 1.3.3b) | `session/backlog_lifecycle_stuck_test.go` | `TestReconcileBouncingItems_should_ResolveBounceCapExhausted_When_BouncingResolves` | Integration | Happy path — marker clears with `bouncing` |
| REQ-2 (Story 1.3.2b, self-heal backstop) | `session/backlog_lifecycle_stuck_test.go` | `TestSelfHealStuck_should_ResolveBounceCapExhausted_When_ItemStatusLeavesInProgressOrReview` | Integration | Error/edge path — backstop for any status transition that bypasses the explicit resolve call sites |
| REQ-1/REQ-2 frontend surfacing (Story 2.1.3a) | `web-app/src/components/backlog-stuck/stuckReason.test.ts` | `getStuckReasonLabel_should_returnDistinctLabel_When_MultipleReasons` | Unit | Happy path |
| REQ-1/REQ-2 frontend surfacing (Story 2.1.3a) | `web-app/src/components/backlog-stuck/stuckReason.test.ts` | `getStuckReasonLabel_should_returnDistinctLabel_When_BounceCapExhausted` | Unit | Happy path |
| REQ-1/REQ-2 frontend surfacing (Story 2.1.3b) | `web-app/src/components/backlog-stuck/StuckItemsSection.test.tsx` | `StuckItemsSection_should_renderMultipleReasonsGroup_When_ItemEscalated` | Integration (component render) | Happy path — `GROUP_ORDER` omission-class regression guard |
| REQ-1: `otherReasonsCount` self-exclusion (Story 2.1.3c, added during triad review — pre-mortem.md Failure #3) | `web-app/src/components/backlog-stuck/StuckItemsSection.test.tsx` | `StuckItemsSection_should_ExcludeEscalationReasonFromOtherReasonsCount_When_ItemHasMultipleReasonsRow` | Integration (component render) | Error/edge path — guards against the escalation row inflating its own item's "+N other reasons" badge |
| REQ-1: de-escalation confirmation banner (Story 2.1.4b, added during triad review round 2 — research/ux.md §4) | `web-app/src/components/backlog-stuck/StuckItemsSection.test.tsx` | `StuckItemsSection_should_ShowDeescalationBanner_When_MultipleReasonsRowResolvesButItemRemainsOpen` | Integration (component render) | Happy path — de-escalation reuses the existing `justResolved` ghost pattern instead of the card silently vanishing |
| REQ-3: flaky-test review differentiation (Scope item 3) | N/A — ADR-002 + follow-up backlog item (Story 3.2.1, Task 3.2.1a) | N/A | Process/documentation, no code test | Deferred per Fallback Increment: recommendation is captured in ADR-002 and a linked follow-up backlog item; no implementation ships in this project, so no unit/integration test applies |
| Success Metrics verification (Story 3.3.1) | N/A — live/scripted DB query, not an automated test | N/A | Manual/live verification | Post-deploy: re-run the investigation queries against the live `backlog_stuck_states` table to confirm items with 2+ open reasons have an open `multiple_reasons` row, and items at `remediation_attempts >= MaxRemediationAttempts` with `bouncing` open have an open `bounce_cap_exhausted` row |

## UX Acceptance Tests
N/A — no design/ux.md; frontend surfacing is minor (label/icon/chip additions) covered by Story 2.1.3 unit tests in plan.md.

## Migration Tests
N/A — no schema migration in this project (ADR-001 chose synthetic `domain.StuckReason` string
values reusing the existing `BacklogStuckState` table rather than a new column/migration).

## Test Stack
- **Unit**: Go stdlib testing (table-driven), matching `session/stuck_decisions_test.go`'s and
  `session/domain/backlog_test.go`'s existing style; Jest for `stuckReason.test.ts`.
- **Integration**: Go stdlib testing against ent test DB, matching
  `session/backlog_lifecycle_stuck_test.go`'s and `session/backlog_lifecycle_test.go`'s existing
  style; React Testing Library (via Jest) for `StuckItemsSection.test.tsx`'s render assertions.
- **E2E / UX**: N/A for this project.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line |
| TypeScript/Jest | `npx jest --coverage --coverageThreshold='{"global":{"lines":80}}'` | ≥80% line |

- All public service methods: happy path + error paths covered
- All external integrations: unit mocked + at least one integration test
