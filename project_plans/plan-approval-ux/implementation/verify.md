# Verification Report — plan-approval-ux

**Date**: 2026-08-01

## Technology Surface

| Technology | Files | Review approach |
|---|---|---|
| Go | server/services/backlog_service_{lifecycle,triage}.go, session/backlog_lifecycle.go, session/ent/schema/backlog_item.go, session/{repository,ent_repository_backlog}.go, tools/scanner/backend/proto_scanner.go | go-development skill (via general-purpose agent) |
| TypeScript/React | PlanVerdictBox.{tsx,css.ts}, PlanArtifactsSection.tsx, BacklogItemDetail.tsx, ActionsSection.tsx, planReviewStatus.ts, useBacklogService.ts | ui-react-best-practices skill (via general-purpose agent) |
| Protobuf | proto/session/v1/backlog.proto | reviewed inline (additive fields/messages only) |
| Playwright e2e | tests/e2e/plan-review.spec.ts | reviewed inline against e2e-test-conventions.md |

## Layer 1 — Idioms

| Technology | Findings | MUST FIX | Action taken |
|---|---|---|---|
| Go | 1 SUGGEST (N+1 GetBacklogItem in selfHealStuck's per-row loop), 1 NITPICK (os.IsNotExist vs errors.Is, matches existing convention) | 0 | Noted as follow-up — bounded by count of open plan-not-approved stuck rows, acceptable at this scale |
| React/TS | 1 MUST FIX (fetchContent had no request-sequencing — stale response could overwrite fresh content), 2 SUGGEST, 1 NITPICK (hardcoded CSS literals matching existing GateVerdictBox pattern) | 1 | **Fixed**: added requestIdRef guard + regression test (commit 9fa17291a) |

## Layer 2 — Architecture

| Finding | Severity | Action |
|---|---|---|
| selfHealStuck's PlanNotApproved case does a per-row DB fetch other cases avoid | CONCERN | Noted as follow-up (same as Go idiom finding — independently confirmed, not blocking at this scale) |
| Two comment blocks in BacklogItemDetail.tsx separately explain the same "outside CollapsibleGroup" exception | NITPICK | Noted, not fixed (cosmetic) |
| checkPlanArtifactFreshness uses raw os.Stat instead of resolveAndValidatePath | NITPICK | Noted — harmless since "plan.md" is a hardcoded literal, not client input |
| derivePlanReviewStatus single-source-of-truth, checkPlanArtifactFreshness placement, PlanArtifactsSection moved outside CollapsibleGroup | Confirmed correct/justified | No action needed |
| Refactor scan (5 items: test helper extraction, allowlist docs, redundant traversal check comment, mtime BigInt de-dup, precedence comment) | SUGGEST | Applied precedence comment + allowlist doc comment inline (commit 9fa17291a); rest noted as follow-up |

No BLOCKERs found in either architecture or idiom review. Implementation confirmed to match plan.md's design intent (Approach C) with no unauthorized scope changes.

## Layer 3 — Correctness

All 8 acceptance criteria verified against plan.md and the backlog item's own AC list — see report_progress calls (criteria 0–7, all `pass`).

### Tests

- Go: `go test ./server/services/...` — **ok** (69.6s)
- Go: `go test ./session/...` — **ok** (all packages; 2 unrelated tmux-flake failures confirmed pre-existing via stash comparison, pass in isolation)
- TypeScript/Jest: `npx jest --no-coverage` — **3674 passed**, 2 pre-existing unrelated failures (SessionDetail.embedded.test.tsx, BacklogEmptyState.test.tsx — confirmed identical on main via git stash, nothing to do with this feature)
- Playwright e2e: `tests/e2e/plan-review.spec.ts` (2 new tests) + `tests/e2e/plan-gate.spec.ts` (unmodified regression) — **all pass**
- `make lint` — **0 issues**
- `make registry-generate` — no net increase in coverage-gaps for this feature

### Security

No auth/authorization surface touched. `GetPlanArtifactContent`'s filename allowlist + `resolveAndValidatedPath` traversal check tested explicitly (`TestGetPlanArtifactContent_TraversalAttempt_ReturnsInvalidArgument`). No secrets, no new external HTTP calls. ✅ No issues.

### Error handling

All external calls (ent storage, os.Stat/Open, ConnectRPC) wrapped with typed connect errors and contextual messages. `checkPlanArtifactFreshness` fails **closed** on stat errors (adversarial-review.md's own required remediation, tested).

### Observability

Plan.md's Observability Plan (§5) is minimal by design (no new logging beyond the existing WarningLog-on-stale-token convention) — implemented as specified (`[ApprovePlan]`/`[RejectPlan] stale content token` log lines).

## Layer 4 — UX & Behavioral

Manually verified in a real browser (separate `PORT=8999` instance, not the deployed service) against a real backend with a real seeded plan.md file:

| UX Criterion | Result | Evidence |
|---|---|---|
| Plan renders as formatted markdown above the action row | ✅ PASS | Screenshot: heading, prose, table all rendered |
| Reject with reason → Changes requested + reason + Regenerate button | ✅ PASS | Screenshot after clicking Request Changes/Submit |
| Approve → Plan approved, Spawn Session gate opens | ✅ PASS | Screenshot: green "PLAN APPROVED" card, Spawn Session enabled |
| No dead ends | ✅ PASS | Every state (pending/changes-requested/approved) has a reachable next action |
| Console errors during golden path | ✅ PASS | No errors observed |

`quality:does-it-work`-equivalent manual walkthrough: ✅ golden path ran without errors.

## Fix Loop Summary

| Layer | Iterations used | Items resolved | Items remaining |
|---|---|---|---|
| L1+L2 | 1/5 | 1 MUST FIX + 2 SUGGEST (documentation) | 0 blocking (2 SUGGEST/CONCERN noted as follow-up, non-blocking) |
| L3 | 0/5 | — | 0 |
| L4 | 0/5 | — | 0 |

## Verdict

✅ **PASS** — all layers clean after the Layer 1 fix — ready for `/backlog/review` → `/backlog/ship`.

### Follow-ups (non-blocking, noted for future work)

1. `selfHealStuck`'s `StuckReasonPlanNotApproved` case does a per-row DB fetch — consider projecting `PlanApproved`/`SkipPlanning` onto `FindOpenStuckStates`'s row if the number of open plan-not-approved stuck rows ever grows large.
2. `isAllowedPlanArtifactFilename` doesn't yet cover `decisions/ADR-*.md` or `pre-mortem.md` — extend when a UI change needs to render them.
3. Line-level/section-anchored plan feedback (requirements.md Success Criterion 5) — explicitly deferred to a follow-up project per plan.md's P6.
4. Multi-entry rejection history in the item's status/progress timeline (requirements.md Should-Have) — deferred per plan.md §7 Q1; current pass serves most-recent-reason visibility only.
