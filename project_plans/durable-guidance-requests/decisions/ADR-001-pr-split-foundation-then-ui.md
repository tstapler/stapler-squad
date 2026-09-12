# ADR-001: Split Into a Foundation PR and a UI-Integration PR

**Status**: Accepted (Auto Mode — no open questions)
**Date**: 2026-09-11

## Context

The requirement (`project_plans/durable-guidance-requests/requirements.md`, "Out of
Scope" section) explicitly asks Phase 3 to decide, with reasoning shown, whether this
ships as one PR or is split into a "foundation" PR followed by a "UI integration" PR.

Full scope: a new ent schema (`session/ent/schema/guidance_request.go`), a new proto
file and ConnectRPC service (`proto/session/v1/guidance.proto`, `GuidanceService`), a
new MCP tool file (`server/mcp/tools_guidance.go`), notification-pipeline wiring,
lifecycle reconciliation, abuse/dedup guarding, automated-triage integration
(`server/services/backlog_service_triage.go`), autonomous-driver integration
(`session/autonomous_driver.go`), a new Redux slice + streaming hook
(`web-app/src/lib/hooks/useWatchGuidanceRequests.ts`), a shared form component, and
wiring into three separate React view components
(`BacklogItemDetail.tsx`, `TriageReviewPanel.tsx`, `SessionDetailView.tsx`), plus e2e
coverage.

## Decision

Split into two PRs:

- **PR #1 — Foundation** (this plan's Phase 1): ent schema, `GuidanceService`
  proto+RPC, storage layer, MCP tools, EventBus + `NotificationHistoryStore` wiring,
  `StuckReasonAwaitingGuidance` + lifecycle reconciler, abuse/dedup guard, triage
  integration, autonomous-driver integration, backend tests, backend feature-registry
  entries. No React changes.
- **PR #2 — UI Integration** (this plan's Phase 2): frontend hook/Redux slice, the
  shared `GuidanceRequestForm` component, wiring into the three named view
  components, the `StuckReasonAwaitingGuidance` badge in board/detail views, e2e
  tests, frontend feature-registry entries.

## Reasoning

- **Independently testable and mergeable.** PR #1 is fully verifiable without any
  UI: `make ci` covers the ent schema, RPC handlers, MCP tools (which any Claude Code
  session — the actual "any subscribed LLM/session" caller named in the
  requirement — already exercises), and the triage/autonomous-driver control-flow
  changes. A human can answer a request through the RPC directly (e.g. an
  integration test invoking `AnswerGuidanceRequest`) even before a form renders
  anywhere, mirroring how this repo already ships backend-only RPCs ahead of their
  UI (the current worktree's own git status shows several backend-only
  `docs/registry/features/backend/*.json` entries for the in-flight tymux-rollout
  feature with no corresponding frontend entries yet).
- **Bounded review size.** Each PR touches one architectural layer. A single
  combined PR would span ent/proto/Go backend AND React/CSS/e2e in one diff —
  exactly the "too large for one PR" case the requirements doc anticipates and asks
  the plan to call out rather than force through.
- **De-risks the harder problem first.** The durability/notification-across-restart
  mechanism (the requirement's hardest, most novel constraint — see
  `research/architecture.md` §2 and §5) is entirely backend. Landing and testing it
  before spending effort on three UI surfaces means a design problem discovered in
  code review doesn't require re-touching already-built UI.
- **No regression risk from the split itself.** Nothing in PR #1 changes existing
  behavior for callers that don't use `GuidanceService` — it is purely additive
  (new proto file, new service registration behind a new `guidance_requests`
  feature flag, new MCP tools, new triage/driver branches gated on the new
  construct actually being present). PR #2 only adds rendering for state PR #1
  already writes.
- **Precedent in this repo.** `NotificationService`'s RPCs, `ApprovalRulesPanel`,
  and `GoalPanel` were each added as backend-then-UI-follow-up slices historically
  (see `git log` on those files) rather than single mega-PRs — this plan follows
  the same convention rather than inventing a new one.

## Consequences

**Positive**: reviewable diff sizes; PR #1 can merge and start giving automated
triage a real "ask instead of guess" path (backend-verifiable via MCP tools and
tests) before any frontend work lands; PR #2 is a pure-frontend diff a
frontend-focused reviewer can evaluate without re-litigating the storage/proto
design.

**Negative**: between the two PRs merging, a human cannot see or answer a
guidance request through the stapler-squad UI — only through direct RPC/MCP calls
(acceptable, since it's a short-lived intermediate state and the alternative,
one oversized PR, is explicitly disfavored by requirements.md). The two PRs must
land in order (PR #2 depends on PR #1's generated proto types and Redux-visible
data shape).

## Alternatives Considered

**One combined PR.** Rejected — exceeds this repo's own duplication/complexity
gate scope for a single diff (`make ready`'s `dupl`/`gocognit` gates are scoped
per-PR via `--new-from-rev=origin/main`; a mega-diff makes it far harder for a
human reviewer to distinguish "new duplication in my diff" from "pre-existing"),
and the requirements doc explicitly names this exact scope combination as the
one that should trigger a split.

**Three-way split (foundation / triage+driver integration / UI).** Considered,
since triage and autonomous-driver integration are themselves nontrivial control-flow
changes. Rejected: both are small, backend-only, and depend on nothing UI-side —
splitting them out buys no independent-testability benefit over keeping them in
PR #1, and a third PR adds coordination overhead (three sequential review cycles
instead of two) for no corresponding risk reduction.
