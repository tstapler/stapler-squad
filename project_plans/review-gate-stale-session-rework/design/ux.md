# UX Design: review-gate-stale-session-rework

**Date**: 2026-07-24

## Scope note

This feature adds no new screens, modals, or flows. It adds one new value to an existing, generically-rendered enum (`StuckReason`) that already flows through fully-built components. The design work here is limited to: (1) confirming the existing surfaces render the new value correctly, and (2) the one piece of new copy (label text) for that value. Per research/ux.md's own recommendation, this doc intentionally stays light rather than inventing new UI ceremony for a fix that shouldn't need any.

## Surface 1: Stuck Items list card (`StuckItemsSection.tsx` / `StuckItem.tsx`)

```
┌──────────────────────────────────────────────────────┐
│ 🟥  Fix login redirect bug                            │
│     Rework blocked — session stalled                  │
│     stuck 18m                                          │
└──────────────────────────────────────────────────────┘
```

- **Interaction flow**: Existing — the card is one of N cards in the existing `StuckItemsSection` list, sorted/filtered by the existing mechanism. No new interaction; clicking the card navigates to the item detail page (existing behavior, reused as-is).
- **No new error state**: if the reason can't be resolved to a known label (shouldn't happen — TypeScript's `Record<StuckReason, T>` makes a missing entry a compile error, not a runtime gap), the existing `UNSPECIFIED` fallback (`getStuckReasonLabel`'s `??` fallback) already handles it gracefully — no new fallback needed.

## Surface 2: Item Detail page (`StuckItemDetail.tsx` + existing `GateVerdictBox.tsx`)

```
┌──────────────────────────────────────────────────────┐
│ Fix login redirect bug                                │
│ 🟥 Rework blocked — session stalled     Since: 14:02  │
│                                                        │
│ [existing GateVerdictBox — verdict: FAIL]             │
│  "Missing test coverage for redirect edge case"       │
│  ┌──────────────────────────────────────────────┐    │
│  │ Reopen for Revision                            │    │
│  └──────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────┘
```

- **Interaction flow**: User lands here from Surface 1 (or directly via the board). Sees the stuck-reason banner (existing `StuckItemDetail` pattern — reused, only the label/icon differ from other reasons) above the existing `GateVerdictBox`, which already renders "Reopen for Revision" for this item's FAIL/PARTIAL/UNVERIFIABLE verdict. Clicking it opens the existing feedback-text confirmation flow (`handleReopenSubmit`) — no changes to that flow.
- **No dead end**: the "Reopen for Revision" exit path already exists and is unmodified by this feature — confirmed by build-vs-buy.md/architecture.md research, verified by Task 2.2.2a.
- **Snooze**: the existing `SnoozeStuckItem` mechanism (from `backlog-stuck-item-visibility`) applies generically to any `StuckReason` including this new one — no new snooze UI needed; confirm during implementation that nothing hardcodes the existing reason list in a way that would exclude the new value from snooze eligibility (a quick grep-and-check, not a task in its own right given the codebase's `Record<StuckReason, T>` pattern makes this unlikely).

## UX Acceptance Criteria

- User can identify a rework-blocked-by-stale-session item and reach its resolution action in ≤ 2 clicks from the stuck-items list (card → detail page, where the existing action is already visible without scrolling below the fold on a typical viewport — inherited from the existing `GateVerdictBox` placement, not new).
- The label text distinguishes this state from `StuckReasonStaleWork`'s "Stale work session" — never identical copy, never color-only differentiation (icon differs too, per Task 2.2.1a).
- No dead end: the detail page always offers "Reopen for Revision" for an item in this state (guaranteed by construction — this `StuckReason` is only ever set on items that already have a FAIL/PARTIAL/UNVERIFIABLE verdict on record, which is exactly `GateVerdictBox`'s existing render condition for that button).
- Accessibility: inherited from existing components (icon decorative + text label always present, per `stuckReason.ts`'s existing documented convention) — no new accessibility surface introduced, so no new audit needed beyond confirming the new map entries follow the same convention (covered by Task 2.2.1c's test).

## States not redesigned (explicitly reused, unmodified)

- **Empty state** (no open stuck items of any reason): unchanged — `StuckItemsSection`'s existing empty-state rendering already handles zero results generically; this feature adds a possible new *reason* for a card to appear, not a new empty-state condition.
- **Loading state**: unchanged — the existing fetch/loading skeleton for the stuck-items list and detail page applies identically regardless of which `StuckReason` values are present in the response.

## Surfaces NOT designed (explicitly out of scope)

- No new snooze UI, no new bulk-action UI, no new notification-center surface — all reuse existing, unmodified mechanisms.
