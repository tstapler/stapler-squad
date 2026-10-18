# Find Refactor Candidates — Requirements

## Problem Statement

The Stapler Squad codebase has grown organically and now contains several large, high-churn files that are difficult to review, test in isolation, and extend without regressions. The codebase has existing refactor plans for some hotspots (`backend-di-refactor`, `backlog-service-refactor`, `front-end-refactor`, `session-path-domain-refactor`, `detection-architecture-refactors`), but no single inventory exists that ranks all candidates by measured impact and identifies which are already planned versus unaddressed.

**This item delivers:** A prioritized, evidence-based inventory of refactor candidates ranked by complexity × churn (the two strongest predictors of review friction and regression risk), with each candidate described precisely enough to be dispatched as a backlog item without redoing the analysis.

## Scope

- **In scope**: Go backend (`session/`, `server/`, `server/services/`, `server/mcp/`), React frontend (`web-app/src/`), and the ORM layer (`session/ent_repository*.go`). New candidates not already covered by existing full-lifecycle plans.
- **Out of scope**: Candidates already implemented or having a complete plan + implementation doc (e.g., `backend-di-refactor` which has `implementation/plan.md` + `validation.md`). Infrastructure (Makefile, CI).

## Goals

### G1 — Inventory
Produce a ranked list of refactor candidates with evidence (file size, git churn, structural observations) and impact rationale. Each candidate states: what it is, why it matters, and what the refactor entails.

### G2 — Gap identification
Show which candidates are already covered by an existing plan and which are not, so dispatch effort lands only on the unaddressed ones.

### G3 — Dispatch-ready descriptions
For the top 3–5 unaddressed candidates, produce enough detail that someone can open a project plan and begin an SDD research phase without reading the full source first.

## Acceptance Criteria

1. The output lists all refactor candidates found with line count, commit churn, and structural classification.
2. Each candidate is classified as "new" (no existing plan) or "existing plan" (plan name cited).
3. The top 3 new candidates have a one-paragraph scope description sufficient for a follow-on SDD session to use as its `requirements.md` seed.
4. Candidates are ranked by a composite score derived from measured data (file size × git-churn frequency), not impression.
5. The plan distinguishes between same-package file-split refactors (low risk) and cross-package interface extractions (moderate risk) so the dispatcher can set appropriate scope expectations.
6. Implementation tasks are scoped so each fits within a single PR (≤ ~600 lines of diff).

## Non-Goals

- Implementing any refactor — this is analysis and dispatch preparation only.
- Ranking candidates that already have an active implementation plan (they are catalogued but not scored further).
- Addressing test quality, CI speed, or infra concerns — those belong in separate backlog categories.
