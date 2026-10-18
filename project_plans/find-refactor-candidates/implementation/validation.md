# Find Refactor Candidates — Validation (Adversarial Review)

## Review method

Pre-mortem and adversarial review of the research findings and plan. Each challenge states the concern, then the response.

---

## Challenge 1: "Lines × commits is a crude proxy — it rewards files that do two things, not files that are actually hard to change"

**Concern**: A file with 3,000 lines touched 100 times might be large because it encodes a large, stable, well-factored domain (e.g., a complete state machine). High churn might reflect feature additions (good) rather than bug fixes (bad). The score does not distinguish these cases.

**Response**: Valid. The composite score is a triage filter, not a verdict. For each Tier 1 candidate the plan includes a structural description of what distinct concerns are mixed in the file. RC-01 (`connectrpc_websocket.go`) mixes WebSocket framing, snapshot caching, hub registry, and memory pool management — four separable concerns confirmed by reading the file's type and function list, not just its line count. Where the structural case was not confirmed (e.g., `backlog_lifecycle_pr.go` churn is estimated), the plan notes the uncertainty explicitly. The scores gate which files to investigate; the structural descriptions gate which to act on.

## Challenge 2: "RC-03 (backlog_lifecycle split) overlaps with backlog-service-refactor — this creates two competing plans for the same code"

**Concern**: `backlog-service-refactor` is the most mature plan in the codebase. `session/backlog_lifecycle.go` may already be addressed in its implementation.

**Response**: The research shows `backlog-service-refactor` targets `server/services/backlog_service.go` and the ORM repository layer (`session/ent_repository_backlog.go`). **Correction (re-checked against source):** this response was wrong. `backlog-service-refactor` names `session/backlog_lifecycle.go` directly (`requirements.md:13`, `implementation/plan.md:324,452,666,708`) — extracting `spawnReviewGate`, removing ent references, and a complexity target. RC-03 is therefore flagged as overlapping and gated in plan.md §5; its scope is limited to the wiring and stuck-reconciler blocks the existing plan does not target.

However, the plan acknowledges this ambiguity explicitly (Task 11: verify existing plans are not stale). Before dispatching RC-03, the implementer should confirm there is no overlap with the in-flight plan. If overlap exists, RC-03 should be merged into `backlog-service-refactor` rather than stand alone.

## Challenge 3: "Frontend splits (RC-04, RC-05) introduce breaking changes to all consumer components"

**Concern**: Splitting `BacklogItemDetail.tsx` means every parent component that imports it will see new sub-component imports; splitting `useBacklogService.ts` means every hook consumer must migrate. The diff will touch dozens of files.

**Response**: This is accurate. The plan flags this as "Moderate" risk for both candidates. The mitigation is a two-phase approach: (1) extract sub-components/hooks but keep the original file as a re-export barrel until all consumers are migrated; (2) remove the barrel in a follow-on PR. This is the standard approach for large React component splits — it keeps individual PRs within a manageable diff size and avoids breaking changes across the codebase in a single commit. The task estimates (6h each) account for the consumer survey step.

## Challenge 4: "The inventory misses server/services/session_service.go (229 commits, most-churned file)"

**Concern**: `session_service.go` at 229 commits is the most frequently modified file in the codebase by a wide margin, yet it appears only in Tier 2. This seems like an oversight.

**Response**: `session_service.go` has ~1,268 lines — large but not as large as the Tier 1 candidates. Its high churn suggests it is a coordination hotspot, but potentially because it is the correct place for new features to land (a stable hub), not necessarily because it is structurally problematic. Splitting a coordination file that is already reasonably sized is lower-value than splitting files that are 3× larger.

However, if a structural review of `session_service.go` shows it is absorbing multiple distinct concerns (e.g., RPC handler routing, session mutation, notification plumbing) rather than just coordinating them, it should be elevated to Tier 1. The inventory flags it for that check — the 229-commit churn alone justifies one pass with `make lint-custom` and an eyeball review of its function list.

## Challenge 5: "RC-01 and RC-02 are same-package file splits — they might not improve testability at all"

**Concern**: Moving functions into new files within the same package does not enable isolated unit tests — callers still import the same package and get the same global state. The real testability improvement requires interface extraction, which is higher risk.

**Response**: Correct. Same-package file splits do not improve testability in isolation. Their primary benefit is **readability and review-surface reduction**: a reviewer touching the hub-registry feature no longer needs to load the full 3,857-line WebSocket handler into their working memory — they open `connectrpc_ws_hub.go` instead. For testability improvements, interface extraction is needed, and that work should follow the split as a separate PR (lower risk to do after the split because the file boundaries clarify which concerns actually need interfaces). The plan is honest about this: RC-01 and RC-02 are "same-package file split (low risk)" and the test gate is build + existing tests, not new coverage.

## Challenge 6: "The churn numbers are from a 6-month window — is that enough history?"

**Concern**: A file modified heavily 8 months ago and frozen since would score zero on a 6-month window, but its structural debt is unchanged.

**Response**: Six months was chosen as a balance between recency (files touched recently are the ones developers are actively working in) and sample size (long enough to smooth out single-sprint spikes). For this codebase the 6-month window captured 95–229 commits per double-hotspot file — large enough to be statistically stable. The structural size data (lines) is independent of the window and provides a floor: any file over 2,000 lines is a candidate regardless of churn, just lower priority.

---

## Validation verdict

The inventory is **ready for dispatch** with the following caveats:

1. RC-03 (backlog_lifecycle split) must be cross-checked against `backlog-service-refactor` before dispatching a separate plan — merge if overlap found.
2. `session_service.go` (229 commits) warrants a structural review pass before its Tier 2 classification is confirmed.
3. RC-04 and RC-05 frontend splits require a pre-dispatch consumer survey to scope the barrel/re-export migration correctly.
4. The `backlog-service-refactor` and `backend-di-refactor` plans should be checked for staleness (Task 11) — if they are complete and merged, their Tier 2 candidates should be removed from the gap table and the remaining gaps re-scored.
