# ADR-001: Extend StuckReason/BacklogStuckState to Agent-Initiated Rows

**Status**: Proposed
**Date**: 2026-07-23
**Context**: project_plans/backlog-agent-communication

## Context

Every one of the 11 existing `StuckReason` values (`session/domain/backlog.go`) is
detected by a periodic reconciler sweep (`ReconcileStuck` and its constituent
`reconcile*` functions) — the pipeline agent itself never directly creates a
`BacklogStuckState` row. This project needs two genuinely new triggers that are
agent-initiated, not reconciler-detected: "ask for help" (Epic 5.1,
`StuckReasonHelpRequested`) and "dispute a verdict" (Epic 6.1,
`StuckReasonVerdictDisputed`).

Two options were considered:
1. **Reuse `StuckReason`/`BacklogStuckState`/`MarkStuck`**, adding two new enum
   values, called directly from an MCP tool handler instead of a reconciler tick.
2. **Build a new, parallel "agent signals" table and UI surface**, structurally
   separate from `StuckReason`.

## Decision

Reuse `StuckReason`/`BacklogStuckState`/`MarkStuck` (option 1). An agent-initiated
call to `MarkStuck` is not fundamentally different from a reconciler-initiated one
from the data model's perspective — both are "this item needs a durable,
human-visible flag that something needs attention, with a specific typed reason."
The only real difference is *what detects it* (a periodic function vs. a live MCP
tool call), not the shape of the resulting row, its notification path, its
`/unfinished` visibility, or its resolution lifecycle (`selfHealStuck`/manual
reset).

This is the first time `MarkStuck` is called synchronously from an MCP tool
handler rather than from `ReconcileStuck`'s sweep — worth naming explicitly as a
new call pattern, not a hidden side effect, so future maintainers don't assume
every `BacklogStuckState` row was reconciler-detected.

## Consequences

- **Positive**: no new UI surface, no new notification transport, no new backoff
  system — Epics 5 and 6 inherit `/unfinished`'s existing rendering, the existing
  `Notifier` transport, and (where applicable) `RemediationDue`'s battle-tested
  accounting, exactly matching requirements.md's compose-not-duplicate constraint.
- **Positive**: `StuckReasonHelpRequested` and `StuckReasonVerdictDisputed` rows
  get the same audit trail (`created_at`, resolution tracking) every other reason
  already has, for free.
- **Negative / must be handled explicitly**: `RemediationDue`'s backoff-and-cap
  semantics are designed for *automated retry actions* — they do not fit an
  agent-initiated one-shot signal that must wait for a *human* response, not an
  automated respawn. Both Epic 5.1 and Epic 6.1 must NOT route resolution through
  `RemediationDue`; they call `MarkStuck` directly and resolve via an explicit new
  human-driven RPC (`RespondToHelpRequest`, `AdjudicateDispute`) instead of
  `selfHealStuck`'s automatic status-change-triggered resolution. This is a
  deliberate deviation from every existing reason's resolution path and must be
  called out in code comments at both new call sites so a future reader doesn't
  assume uniform resolution semantics across all `StuckReason` values.
- **Negative**: duplicate-call guards (Story 5.1.1, Story 6.1.1's dispute cap) are
  now the *agent-initiated* discipline's equivalent of `RemediationDue`'s backoff —
  they must be implemented explicitly per new reason, since the shared gate does
  not apply here.
