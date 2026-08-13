# Validation Plan: backlog-status-visibility

**Date**: 2026-08-01
**Status**: Adopt `project_plans/backlog-session-lifecycle-ux/implementation/validation.md` verbatim — full requirement→test mapping (Go table-driven unit + integration tests against a real test-scoped SQLite `ent.Client`, Jest/RTL for frontend, Playwright for the 11 UX acceptance criteria + 2 Axe-Core accessibility checks) and the migration reversibility test design already exist there. Not re-derived here.

## Happy Path Scenario (unchanged from adopted validation)

Given a backlog item auto-remediated 3 times (`BacklogStuckState.remediation_attempts = 3`, ≥1 `ItemSession` with a non-empty `end_reason`, ≥1 recorded `RespawnEvent`), when the user opens the board then the item detail, then they see — with zero clicks/navigation for the board-card badge, and ≤2 clicks for the full respawn timeline — the `×3` suffix, the session end-reason chip, and the expandable "Respawn History" section, without opening `/unfinished` or reading logs.

## Coverage Targets

Same as adopted plan: ≥80% line coverage (Go and TS), every public service method has happy + error-path tests, every UX acceptance criterion has a Playwright test.

## Gate before this item is considered "validated" for execution

Per `requirements.md`'s duplicate-work finding: before anyone spends implementation time against *this* item's artifacts, confirm with the operator whether `project_plans/backlog-session-lifecycle-ux` (uncommitted, further along, already through adversarial review) is the canonical delivery vehicle instead. Running both to completion independently would produce two PRs touching the same files (`server/services/backlog_service_triage.go`, `BlockerChip.tsx`, `SessionCard.tsx`, `SessionsSection.tsx`, new `session/ent/schema/respawn_event.go`) — a guaranteed merge conflict, not a parallelization opportunity.
