# ADR-001: Agent/Memory Column Disclosure Uses the Existing Shared `Tooltip`, Not a New Component

**Status**: Accepted
**Date**: 2026-09-24

## Context

`session-list-density`'s requirements demote the `agent` and `memory` columns from
`SessionRow`'s default grid (`web-app/src/components/sessions/session-columns.ts`).
Today those columns render a bare `title` attribute for their tooltip
(`SessionRow.tsx:452-455`, `:502-508`). Two independent research passes
(`research/pitfalls.md` §3, `research/ux.md` §3) confirm `title`-only tooltips are
already broken for keyboard-only and touch users — not just "less visible" once
demoted, but genuinely unreachable, since `title` never fires without a mouse hover
and the `<span>` carrying it isn't in the tab order. Demoting the column without
fixing this compounds an existing accessibility gap.

`ux.md` recommends the WCAG-aligned fix: an `aria-describedby` + `role="tooltip"`
disclosure triggered by tap/focus, not just hover.

## Decision

Reuse the existing shared `Tooltip` component
(`web-app/src/components/ui/Tooltip.tsx`), which wraps Radix UI's
`@radix-ui/react-tooltip` primitives. Radix's `Tooltip.Trigger`/`Tooltip.Content`
already implement the `aria-describedby` + `role="tooltip"` pattern natively, and
already show on keyboard focus, not just pointer hover — we get the WCAG-recommended
behavior for free, provided the trigger element is actually focusable
(`tabIndex={0}`, since the current `<span>` isn't).

Additionally, fold the agent program and memory value into the row-level
concatenated `aria-label` (`SessionRow.tsx:306`), mirroring how status/program/path/
context are already combined there — this gives screen-reader users the data with
zero extra interaction, independent of whether they ever focus the individual span.

Do not build a new bespoke tooltip/disclosure component.

## Consequences

- Fixes keyboard-focus and screen-reader reachability for agent/memory data.
- Does **not** fully fix touch-only sighted users: Radix tooltips are
  hover/focus-triggered, not tap-triggered, on touch devices (a per-project constraint,
  not something this ADR solves). The escape hatch is unchanged — the Columns picker
  lets a user re-add `agent`/`memory` as full default-visible columns if this gap
  matters to them, which is proportionate for a complexity-2, single-user feature.
- No new dependency — `@radix-ui/react-tooltip` is already installed and used
  throughout `SessionRow.tsx`/`SessionCard.tsx`.

## Alternative Considered

A custom `aria-describedby` + `role="tooltip"` element built from scratch, per
`ux.md`'s literal recommendation. Rejected: it would duplicate what
`Tooltip.tsx` already provides via Radix, for no additional capability, and would
be a second tooltip implementation in the same component tree — a duplication
risk in a file already flagged for jscpd sensitivity (`research/pitfalls.md` §4).
