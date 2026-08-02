# ADR-003: `TriggeredByAgent` Applies to Both `request_review` Source-Status Paths

## Status
Proposed

## Context

FR7 requires introducing a `TriggeredByAgent = "agent"` constant (alongside the existing
`TriggeredByUser`/`TriggeredBySystem`, `session/backlog.go:90-94`) and using it for "every
status transition this feature causes." `request_review`'s current code passes
`session.TriggeredBySystem` (`tools_backlog.go:415`) for both its `in_progress`-sourced path
(pre-existing) and, after FR1's generalization, its new `pr_pending`-sourced path.

The ambiguity: does "every transition this feature causes" mean only the brand-new
`report_duplicate` call gets `TriggeredByAgent`, leaving `request_review`'s existing
`TriggeredBySystem` untouched (minimal-diff reading)? Or does it mean every transition this
PR's diff touches — including the *existing* `request_review` code this PR is generalizing —
should also switch, since FR1 is itself part of this feature (broader reading)?

## Decision

**Both `request_review` source-status paths (`in_progress→review` and `pr_pending→review`)
switch to `TriggeredByAgent`, alongside the new `report_duplicate` call.**

Rationale: `request_review` is called exclusively by AI work sessions today (confirmed — no
human-facing UI path calls it; it's an MCP tool). Labeling its transitions `"system"` was
always a slight misnomer (it's not a reconciler-ticker-driven transition like
`ReconcileStuck`'s auto-`done` path genuinely is) — this PR is generalizing the exact call
site that produces this label, so fixing the label at the same time is in scope, not scope
creep. Leaving `request_review` at `"system"` while the new `report_duplicate` (functionally
identical in kind — an agent self-reporting completion of its assigned work) reads `"agent"`
would make two equivalent actions audit-inconsistent for no reason a future reader of
`BacklogStatusEvent` rows could infer without reading this ADR.

## Consequences

### Positive
- Consistent audit attribution: any transition originating from an AI work/review session's
  own tool call (not a background reconciler tick) now reads `"agent"` uniformly.
- Single code-site change (one line at the `TransitionBacklogItemStatus` call in
  `requestReview`), no branching needed by source status.

### Negative
- Any existing dashboard, query, or downstream consumer that filters `BacklogStatusEvent`
  rows on `TriggeredBy == "system"` to find `request_review`-originated transitions will stop
  matching them after this change. **Grep check before merging**: `grep -rn "TriggeredBySystem"
  web-app/src session server` to confirm no such consumer exists — the only known reader is
  `WorkflowHistorySection.tsx`, which renders `triggeredBy` as opaque text with no filtering
  logic (confirmed, ux.md §4), so this risk is believed low but not exhaustively ruled out by
  research alone.

### Neutral
- `ReconcileStuck`'s own ticker-driven transitions (e.g. the merged-PR auto-`done` path,
  `backlog_lifecycle.go:3921,4184`) are unaffected — those are genuinely system-triggered and
  keep `TriggeredBySystem`. Only the two call sites inside `requestReview`/`reportDuplicate`
  change.
