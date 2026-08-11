# Requirements: Severity Levels (P0/P1/P2) on Review Queue Items

Source: backlog item `8bd3f70e-5fe2-49e9-8a39-1968d4842598`, migrated from
[TylerStaplerAtFanatics/stapler-squad#40](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/40).
0 acceptance criteria were provided on the backlog item — this document derives
them from the issue description.

## Problem

The review queue (pending tool-use approvals awaiting a human decision) treats
every item as equal priority. A P2 test-file edit sits at the same visual
priority as a P0 `rm -rf` or force-push. As session count grows this makes the
queue a FIFO list instead of a risk-ordered triage view.

## Existing groundwork (found during codebase orientation — see research.md for depth)

This is **not a greenfield feature**. The classifier (`pkg/classifier`) already
computes a 4-level `RiskLevel` (`RiskLow`/`RiskMedium`/`RiskHigh`/`RiskCritical`)
on every `ClassificationResult`, including for seed rules on dangerous patterns
(destructive `rm`, `git push --force`, etc.). `ApprovalRuleProto` already has a
`risk_level` field (proto `session.proto`/`types.proto`). What's missing is that
`escalation.RiskLevel` is computed in `server/services/approval_handler.go`
(`HandlePermissionRequest`) but **dropped** before being persisted onto
`PendingApproval` (`server/services/approval_store.go`) — only
`EscalationReason`/`EscalationCategory` (strings) survive. `PendingApprovalProto`
(`proto/session/v1/types.proto`) has no risk/severity field at all, so the
frontend (`ReviewQueuePanel.tsx`, `ApprovalAnalyticsPanel.tsx`) has nothing to
render. This materially narrows the implementation to "plumb an existing signal
through" rather than "invent a classification system," except for the
agent-reported source, which is genuinely new.

Research phase must confirm/correct this narrative before planning locks it in.

## Scope

### 1. Severity levels

Three levels, per the issue: `P0` (dangerous/irreversible), `P1` (impactful but
reversible), `P2` (low-risk). Map onto the classifier's existing 4-level
`RiskLevel` rather than inventing a parallel taxonomy:

- `RiskCritical` → `P0`
- `RiskHigh` → `P1`
- `RiskMedium`, `RiskLow` → `P2`

(Research phase to validate this mapping is defensible against the actual seed
rule risk assignments — a straight 4→3 collapse must not put something
genuinely P0-dangerous into P1 by rounding.)

### 2. Severity sources

1. **Rules-derived**: the matched (or unmatched, i.e. no-match escalation)
   classifier rule already determines a `RiskLevel` today. Persist it.
2. **Pattern-derived**: the classifier's built-in seed rules already encode
   heuristics like destructive `rm`, `git push --force` → high/critical risk.
   No new heuristics needed unless research finds real gaps (e.g. env var
   mutation isn't currently scored as risky).
3. **Agent-reported**: new. Needs a structured way for an agent (Claude Code
   session) to self-report severity on a request — e.g. via the permission
   hook payload or a session-side annotation — that the server can trust or
   fall back from. This is the one part of the feature with no existing
   scaffolding; plan phase must size it explicitly and it may be reduced to
   "accept an optional field, don't yet wire a producer" if a producer would
   require touching the Claude Code hook contract itself (out of this repo's
   control).

### 3. UI changes

- **Severity badge** on each review-queue item, colour-coded (red=P0,
  amber=P1, green=P2), in `ReviewQueuePanel.tsx` and wherever approvals render
  individually (approval dialog).
- **Sort queue by severity by default** — P0 first. Must compose with, not
  replace, the existing `AttentionReason`/`Priority` (urgency) ordering
  already in `review_queue_manager.go` — these are two different axes
  (urgency-to-look-at vs. risk-of-the-operation) and the plan must define how
  they combine, not silently overwrite one with the other.
- **Filter queue by severity level** — consistent with existing filter
  patterns in the review queue / approvals UI.
- **Approval analytics breakdown by severity** — extend
  `ApprovalAnalyticsPanel.tsx` / `AnalyticsSummaryProto` (`analytics_store.go`
  already breaks decisions down by `EscalationCategory`; add an equivalent
  breakdown keyed by severity).

## Out of scope

- Changing how the classifier computes `RiskLevel` for existing rules (no
  rule-authoring UX changes beyond exposing severity).
- Auto-escalation/auto-routing behavior changes (Gastown/Bernstein-style
  automatic escalation to another agent) — the issue's "Competitive Context"
  section is inspiration, not a requirement; the "Proposed Behaviour" section
  is the actual ask, and it stops at triage/visibility, not routing.
- Building a general-purpose agent-reported-severity producer inside Claude
  Code itself — this repo can only accept and surface such a signal, not
  compel Claude Code to emit one.

## Acceptance criteria (derived)

1. Every pending approval surfaced to the review queue carries a severity
   (`P0`/`P1`/`P2`), derived from the classifier's existing `RiskLevel` for
   rules-derived and pattern-derived cases, with a defined (even if `P2`)
   default for items that predate this field (e.g. loaded from disk after a
   restart).
2. `PendingApprovalProto` (and any other proto surface carrying review-queue
   items to the frontend) includes the severity field, generated via
   `make proto-gen`.
3. The review queue UI shows a colour-coded severity badge per item.
4. The review queue defaults to sorting P0 first, without breaking existing
   urgency-based ordering semantics (plan must state the precise composite
   sort rule).
5. The review queue can be filtered by severity level.
6. `ApprovalAnalyticsPanel` (or equivalent) shows an approval-decision
   breakdown by severity, backed by `AnalyticsSummaryProto`.
7. There is a defined, documented mechanism for an agent to self-report
   severity on a request (structured field/schema), even if no in-repo
   producer populates it yet, and rules/pattern-derived severity always wins
   over an agent-reported value for the same request (a compromised or
   confused agent must not be able to downgrade its own severity below what
   the classifier already computed).
8. New/changed backend logic has Go test coverage; new/changed frontend logic
   has Jest coverage; a new UI feature gets a Playwright e2e test per this
   repo's registry rules.
9. `docs/registry/features/` entries are added/updated per
   `.claude/rules/feature-registry.md` for any new RPC fields or UI surfaces.
10. `make quick-check` (build + test + lint) passes.

## Constraints from repo conventions

- Proto changes go through `make proto-gen`, not hand-edited generated files.
- `session/ent/schema` changes (if persistence needs a schema change) must use
  `--feature sql/upsert` (see `.claude/rules/ent-schema-generation.md`) — only
  relevant if severity needs to be queryable/persisted beyond the existing
  JSON-file `ApprovalStore`/`PendingApproval` persistence; research phase to
  confirm whether `ent` is even in the persistence path for approvals (current
  reading says no — `ApprovalStore` persists to a flat JSON file, not `ent`).
- New CSS goes in `.css.ts` (vanilla-extract) per
  `.claude/rules/css-architecture.md`; badge colors must use theme tokens, not
  hardcoded hex.
- `web-app/` package management is pnpm-only.
- Feature registry (`docs/registry/`) and, if a new detector/action is added
  to the omnibar, the omnibar registries — not expected to apply here since
  this is queue display/filter, not a session-creation or omnibar capability,
  but plan phase should double check against
  `.claude/rules/feature-testing-registry.md`'s decision tree.
