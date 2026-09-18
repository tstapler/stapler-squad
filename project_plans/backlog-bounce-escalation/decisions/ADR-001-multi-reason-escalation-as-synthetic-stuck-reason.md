# ADR-001: Represent Multi-Reason Escalation and Bounce-Cap-Exhaustion as Synthetic `StuckReason` Values

**Status**: Accepted
**Date**: 2026-08-11

## Context

`backlog-bounce-escalation` needs two new durable, queryable signals:
1. An item has 2+ simultaneously open stuck-state reasons ("multi-reason escalation").
2. An item hit `MaxRemediationAttempts` while its `bouncing` reason is still open
   ("capped while bouncing").

Both must be durable (survive restarts, queryable, not a one-time toast), consistent with
the existing `backlog_stuck_states` infrastructure per `research/architecture.md`.

## Options considered

1. **New `severity`/`escalated_at` column(s) on `BacklogStuckState`.** Requires an ent
   migration, new upsert logic distinct from `MarkStuck`, and a parallel query path for the
   UI/RPC layer to read it.
2. **Purely computed at read time, no persistence.** Fails the requirement's own "durable...
   not a one-time toast" success metric — a signal that only exists inside a request handler
   isn't queryable independent of a live poll, and can't survive a missed toast.
3. **New synthetic `domain.StuckReason` values** (`multiple_reasons`,
   `bounce_cap_exhausted`), `MarkStuck`/`ResolveStuck`'d against the item using the exact
   same repository methods, notify-once dedup, snooze, and reset machinery every other
   reason already uses.

## Decision

Option 3. Zero schema change (the `reason` column is a plain, unconstrained string —
confirmed via `session/ent/schema/backlog_stuck_state.go` and `domain.StuckReason.IsValid()`
being the only validation boundary). The two new conditions get full `ListStuckBacklogItems`,
`SnoozeStuckItem`, `ResetStuckRemediation`, and durable-marker behavior for free, and the
existing frontend (`otherReasonsCount` badge, `StuckItemsSection`'s grouping) surfaces them
with only label/icon/class additions — no new component family.

## Consequences

- No ent migration, no new RPC message shape, no new frontend component.
- The two new reasons must be added everywhere `domain.StuckReason`'s existing 14 values are
  enumerated: `AllStuckReasons`, `IsValid()`, `toProtoStuckReason`/`fromProtoStuckReason`, the
  proto `StuckReason` enum, and the frontend's `Record<StuckReason, T>` maps (which are
  compile-checked exhaustive) plus `GROUP_ORDER` (which is **not** compile-checked — see the
  doc-comment warning already in `StuckItemsSection.tsx` about exactly this omission class of
  bug from a prior incident).
- `selfHealStuck`'s per-reason switch (`session/backlog_lifecycle.go:1643`) needs an explicit
  decision per new reason: add a case, or rely on the reason's own detector/event-site to
  resolve it. See plan.md Story 1.2.3 and 1.3.2 for the two different answers this ADR's two
  signals need.
- **Known divergence from `research/ux.md` §2** (surfaced during triad review, round 3):
  `research/ux.md` recommended swapping in a distinct visual treatment on the *same* card as
  the underlying reason ("not a new component"), but this ADR's synthetic-`StuckReason`
  approach gives each escalation its own `BacklogStuckState` row, which — per Task 2.1.1b's
  `GROUP_ORDER` entry — renders as its own card under its own group heading, i.e. an escalated
  item shows up twice (once under its real reason, once under "Multiple reasons
  stuck"/"Bounce cap exhausted"). Accepted trade-off, not fixed: the alternative (badging the
  *existing* reason's card instead of creating a new row) would mean the escalation signal
  isn't independently `MarkStuck`/`ResolveStuck`/snooze/notify-tracked the way every other
  reason is — reintroducing exactly the parallel-tracking problem Option 3 was chosen to avoid
  (see Options Considered above). The duplicate card is judged the cheaper cost of the two.
