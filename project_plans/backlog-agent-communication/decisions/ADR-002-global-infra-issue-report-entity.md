# ADR-002: New Global `InfraIssueReport` Entity Instead of Reusing StuckReason

**Status**: Proposed
**Date**: 2026-07-23
**Context**: project_plans/backlog-agent-communication

## Context

Dimension 2 (infra/orchestrator-bug reporting) needs a place for an agent to
report that the *tooling itself* — not its assigned item — is broken (a stuck
reconciler, a confusing MCP tool error, a missing capability). ADR-001 reuses
`StuckReason` for two *other* new signals (help requests, verdict disputes)
because those are genuinely item-scoped. Infra reports are not.

`BacklogStuckState`'s schema requires `item_id` — it is structurally an
item-anchored table (indexes, `selfHealStuck`'s resolution logic, and the
`/unfinished` per-item card rendering all assume a owning `BacklogItem`). A
systemic report (e.g. "the tmux control-mode client leak is overloading the
server," the literal BUG-042 example from today) has no single correct owning
item — attaching it to whichever item happened to be running when the agent
noticed would misrepresent it as item-specific and risk it being triaged/resolved
alongside that one item's fate (e.g. auto-resolved if that item happens to reach
`done`, even though the systemic issue is unrelated and unresolved).

## Decision

Introduce a new, deliberately separate entity, `InfraIssueReport`
(`session/ent/schema/infraissuereport.go`), with `related_item_id` as an
**optional** field (not the primary key of "what this report is about") rather
than forcing it into `BacklogStuckState`'s item-required shape. Give it its own
minimal CRUD (`session/storage_infra.go`), its own MCP tool
(`report_infra_issue`), and its own (visually distinct, not merged) UI section on
`/unfinished`.

This is intentionally *not* a case of "reuse existing machinery" per
requirements.md's compose-not-duplicate constraint — the constraint is to avoid
duplicating machinery that actually fits; `BacklogStuckState` does not fit a
non-item-scoped signal, and forcing the fit would create the exact "misrepresents
scope" failure mode described above. This ADR documents that decision explicitly,
per requirements.md's "flag anywhere a proposed capability overlaps existing
machinery" success metric — the answer here is "does not overlap, by design."

## Consequences

- **Positive**: infra reports are never silently resolved as a side effect of an
  unrelated item's lifecycle; their resolution requires an explicit human
  Acknowledge/Resolve action (Story 4.1.2), matching how a genuinely systemic
  issue should be tracked.
- **Positive**: mirrors `BacklogProgressNote`'s already-proven append-only,
  simple-CRUD shape — low implementation risk, no novel patterns introduced.
- **Negative**: a second, small "durable signal + UI list" pattern now exists
  alongside `BacklogStuckState`'s — a future maintainer must understand both
  rather than one unified system. Mitigated by keeping `InfraIssueReport`
  deliberately minimal (4 fields + status, no backoff/remediation-attempt
  machinery at all) so the two systems don't need to be held in the same amount
  of mental detail.
- **Follow-up consideration (not in this plan's scope)**: if infra reports prove
  to recur around the same handful of root causes, a future iteration might link
  `InfraIssueReport` rows to a lightweight "known issue" grouping — explicitly
  deferred; the manual `docs/bugs/*.md` convention already used for BUG-040
  through BUG-045 remains the source of truth for actual bug write-ups.
  `report_infra_issue` is a *signal* that something needs attention and possibly a
  bug doc, not a replacement for the bug-doc process itself.
