# Requirements: backlog-service-refactor

**Date**: 2026-07-09
**Type**: refactoring / architectural decomposition
**Complexity**: 3 — system design (multiple epics, cross-cutting change touching all backlog layers)

## Problem Statement

The backlog subsystem has six identified hotspots that compound each other's maintenance pain:

1. `server/services/backlog_service.go` (2570 lines, 48 revisions, cogn. 65) mixes query, lifecycle, and triage/review orchestration in one file — every backlog PR touches it
2. `server/mcp/tools_backlog.go::submitTriageResult` (cogn. 87, highest in scope) has inline AC merge logic with no isolated test coverage
3. `session/backlog_lifecycle.go::spawnReviewGate` (5 responsibilities, always co-changes with backlog_service at ratio 0.71) lacks a clean abstraction boundary
4. `BacklogItemData` (22 fields) and `ApprovalRuleData` (24 fields) are God structs — list queries over-hydrate, all callers pay the full cost regardless of need
5. `session/` package has 65+ importers pulling in infrastructure alongside pure domain types (no sub-package boundary)
6. `backlog_service.go ↔ ent_repository_backlog.go` coupling ratio 0.78 — schema changes always surface in the service layer, meaning the `Storage` interface abstraction is too thin

For developers: every backlog feature change drags an unrelated file; high cognitive complexity slows review; poor boundaries hide the true blast radius of changes.

## Baseline

- `backlog_service.go` line count: ~2570; cognitive complexity: 65; revision count in 500-commit window: 48
- `submitTriageResult` cognitive complexity: 87; AC merge logic is untested in isolation
- `spawnReviewGate` handles diff fetch, security check, headless call, verdict persist, auto-reopen — all in one function
- `BacklogItemData` has 22 fields returned for every caller including list views that need 5
- Any package needing `AcCriterion` or `BacklogStatus` must import `session`, pulling in tmux/worktree infra
- Every ent schema field addition requires a concurrent `backlog_service.go` change (ratio 0.78)

## Users / Consumers

- **Developers** working on the backlog feature (primary beneficiaries — faster PRs, lower review friction)
- **Go build system** — package boundaries affect compile times and test isolation
- **CI** — `make ci` must remain green throughout

## Success Metrics

- `backlog_service.go` splits into ≥3 files, each ≤900 lines
- Peak cognitive complexity per file in the backlog subsystem drops to <40 (from 87)
- `mergeAcCriteria` has ≥3 table-driven unit tests exercising edge cases (empty incoming, gap in indices, duplicate index)
- `spawnReviewGate` logic lives in its own file (`session/review_gate.go`) with a testable `ReviewGateRunner` type
- `BacklogItemSummary` struct exists for list-view queries; no list query returns full `BacklogItemData`
- `session/domain` sub-package exists with at least `AcCriterion`, `BacklogStatus`, `AcCriteriaJSON`, `ReviewOutcome` moved into it
- Packages that previously imported `session` only for domain types now import `session/domain`
- Storage interface adequately decouples: no ent-specific types visible in `backlog_service.go`
- All existing tests pass (`make ci` green) — zero behavior changes

## Appetite

**Medium (1–2 weeks)** — 6 items, some parallelizable. P1 (service split) is the largest and should sequence first because it unblocks accurate measurement of P4–P6. P2 (mergeAcCriteria extract) is independent and self-contained. P3 (ReviewGateRunner) follows P1.

## Constraints

- **Behavior-preserving only** — no feature changes, no API changes; this is pure structural refactoring
- `make ci` must stay green after each item lands (ship incrementally, not as one big bang)
- ent schema generation must use `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` (see `.claude/rules/ent-schema-generation.md`)
- No new external dependencies — use stdlib only

## Non-functional Requirements

- **Performance SLO**: Refactors must not regress any existing benchmark (run `make test-coverage`; no new allocations on hot paths)
- **Scalability**: Not applicable — structural change only
- **Security classification**: Internal
- **Data residency**: No special requirements

## Scope

### In Scope

1. **P1** — Split `server/services/backlog_service.go` into `backlog_query_service.go`, `backlog_lifecycle_service.go`, `backlog_triage_service.go`
2. **P2** — Extract `mergeAcCriteria(existing []AcCriterion, incoming []rawAC) ([]AcCriterion, error)` from `server/mcp/tools_backlog.go`; add table-driven tests
3. **P3** — Extract `ReviewGateRunner` struct into `session/review_gate.go`; make `spawnReviewGate` a thin coordinator
4. **P4** — Introduce `BacklogItemSummary` struct for list views; update Storage interface and list query callers
5. **P5** — Create `session/domain` sub-package; move pure domain types; update all importers
6. **P6** — Audit `backlog_service.go ↔ ent_repository_backlog.go` interface; ensure no ent types leak into service layer

### Out of Scope

- Feature changes to backlog behavior
- Proto/API contract changes
- Frontend changes (except where TypeScript bindings must update due to proto changes — but no proto changes are planned)
- Ent schema changes
- Adding new test infrastructure beyond table-driven unit tests for extracted functions
- Fixing unrelated test failures discovered during the refactor

## Rabbit Holes

- **P5 (session/domain) import cycle risk**: Moving types to a sub-package risks circular imports if `session` types reference each other. Must audit all type dependencies before moving. Start with truly leaf types only.
- **P4 list-view BacklogItemSummary**: If the ent ORM query layer doesn't support projection (selecting a subset of fields), returning a leaner struct may still hydrate the full row from DB. Check ent's Select() API.
- **P1 split ripple**: 52 exported functions in one file means many call sites across the codebase. A naive split will break the build temporarily — use a phased approach: move functions, add package-level re-exports briefly, then clean up callers incrementally.
- **P3 ReviewGateRunner**: `spawnReviewGate` uses several `session`-package private fields. Extracting into its own type may need those fields promoted to the interface or passed as constructor args — don't assume it's a simple move.

## Alternatives Considered

- **Do nothing**: Status quo is functional but the 0.78 coupling ratio and 87 peak cognitive complexity guarantee that the next backlog PR will be slow to review and risky to land
- **Rewrite**: Out of scope — no behavior changes, just structural reorganization
- **Micro-service extraction**: Overkill for a single-binary service; clean package boundaries inside the monolith achieve the same isolation benefit

## Feasibility Risks

- Import cycle between `session/domain` and `session` (P5) — mitigation: audit all cross-references before moving types
- ent's Select() support for partial hydration (P4) — mitigation: verify in research phase before designing BacklogItemSummary integration
- `backlog_service.go` split may temporarily break build if 52 exported functions move packages — mitigation: keep all in the same Go package (`package services`) across files; splitting files ≠ splitting packages

## Observability Requirements

Standard — no new metrics needed. Structural refactoring should not change observability behavior. Verify existing log lines survive the split.

## Risk Control

- Ship P2 first (smallest, fully isolated, adds test coverage) — confirms the CI pipeline is green before touching larger items
- Each P1–P6 item lands as a separate PR on its own branch
- No feature flag needed — all changes are structural, not behavioral
- Rollback: revert any individual PR; items are independent enough that reverting one does not require reverting others (except P5 which P6 may depend on)

## Open Questions

- Does ent's generated query builder support column-level projection for BacklogItemSummary (P4)? — research phase
- Are there any packages outside `server/` and `session/` that import `session` for domain types only? — research phase
- What is the exact list of ent-specific types currently visible in `backlog_service.go`? — research phase
