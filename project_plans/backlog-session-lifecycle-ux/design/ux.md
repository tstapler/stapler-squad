# UX Design: backlog-session-lifecycle-ux

**Date**: 2026-08-01
**Status**: Ready for implementation review
**Inputs**: `requirements.md`, `research/ux.md`, `implementation/plan.md`

This document specifies the concrete layout, interaction flow, and edge-case
handling for the 5 user-facing surfaces this project touches. It extends —
never replaces — the existing `stuckReason.ts` icon/color severity vocabulary
and the codebase's established accessibility triad (see `research/ux.md` §3).

---

## Surface inventory

| # | Surface | Component | New/Changed | Data source |
|---|---------|-----------|--------------|--------------|
| 1 | Board card | `BacklogItemCard.tsx` (via `BlockerChip.tsx`, `variant="compact"`) | Changed | `StuckBacklogItem.remediationAttempts`/`nextRemediationAt` |
| 2 | Session card | `SessionCard.tsx` | Changed | `Session.pauseReason` |
| 3 | Item detail — Sessions section | `SessionsSection.tsx` | Changed | `ItemSession.endReason` |
| 4 | Item detail — Lifecycle Summary | `LifecycleSummary.tsx` (via `BlockerChip.tsx`, `variant="full"`) | Changed | `StuckBacklogItem.remediationAttempts`/`nextRemediationAt` |
| 5 | Item detail — Respawn History | `RespawnHistorySection.tsx` (new) | New | `BacklogItem.respawnEvents` |

All five reuse existing primitives (`BlockerChip`, `CollapsibleSection`,
`useShowMore`, `Tooltip`) rather than inventing parallel components — per
`code-review`'s interface-pollution guard and `research/ux.md`'s explicit
"extend, don't replace" verdict.

---

## Surface 1: Board card — `×N` remediation-count suffix on `BlockerChip`

### Wireframe (compact variant, board card footer)

```
┌─────────────────────────────────────────────────────┐
│ Fix flaky retry logic                       P2   ⏳  │  ← title / priority / status label
├─────────────────────────────────────────────────────┤
│                                                       │
│  [ View Session ]   2/4 done   🟠 Stale work session ×3 │  ← action btn, AC summary, BlockerChip
└─────────────────────────────────────────────────────┘
                                        ▲
                                        └─ NEW: ×3 suffix span
```

Parked state (remediation exhausted) swaps color/label, still on the same chip:

```
  [ View Session ]   2/4 done   🔴 Stale work session ×5 (parked)
```

No chip at all when `stuckItem` is undefined (item not currently flagged
stuck) — unchanged from today; this is the majority case and must stay
silent (research/ux.md §4).

### Interaction flow

1. **At rest**: user scans the board. The chip's icon+color (already
   established `stuckReason.ts` vocabulary) signals severity at a glance;
   the `×N` suffix is a secondary, smaller-weight signal answering "how many
   times has the system already tried."
2. **Click/tap anywhere on the card** (not the chip specifically — the chip
   is not independently interactive in compact variant): opens the item
   detail panel, where Surface 4 (Lifecycle Summary, `variant="full"`) shows
   the same chip plus duration, and Surface 5 (Respawn History) shows the
   full per-attempt breakdown. The compact chip itself has no separate
   click target or expand-in-place behavior — this matches the CI-badge
   precedent (`research/ux.md` §1): the count answers "how many," full
   detail is a navigation away, not an inline expand on the board.
3. **System response to state change**: when `remediationAttempts` changes
   (a respawn just fired) while the board is open, the existing
   `justChanged` flash (`BacklogItemCard.tsx` `FLASH_DURATION_MS`) already
   fires on any live item update — no new flash logic needed for this
   surface; the `×N` value simply updates in place on next render.

### Error / edge cases

- **`remediationAttempts === 0`**: no suffix rendered (unchanged from
  today) — avoids noise for the ~majority of items that never need
  remediation.
- **`remediationAttempts >= 5` (parked)**: chip visibly communicates
  "parked," reusing `StuckItem.tsx`'s exact established wording pattern
  ("remediation attempts exhausted") — not new copy. This is the signal
  that tells Tyler "system has given up automatically, this needs me,"
  which is the core job-to-be-done (`research/ux.md` §5).
  - **NEW in this project**: the compact-variant chip today has no
    click-through path from the board straight to the item's "Retry now"
    control (that control lives on `/unfinished`'s `StuckItem.tsx`, a
    different page). This is a known gap, out of scope per requirements.md
    ("Redesigning the board page layout beyond adding the new indicators"),
    but flagged here because a parked item that requires a manual retry
    is exactly the moment a user wants a shortcut — clicking the card still
    reaches the item detail panel, which is one hop closer than today
    (previously: no indication at all on the board that a retry was
    needed). Acceptable for v1; a direct "Retry now" board affordance is a
    reasonable follow-up item, not blocking this one.
- **`stuckItem` prop resolves after initial paint** (async poll): chip
  appears without a layout shift large enough to move the action button —
  verify in implementation that the chip wraps onto its own line on narrow
  cards rather than pushing the action button off-row (existing
  `styles.cardFooter` is presumably `flex-wrap`; confirm during Task 3.1.1b).

---

## Surface 2: Session card — always-visible `pause_reason` badge

### Wireframe (desktop, session card status row)

**Before (today — tooltip only, hover required):**
```
┌───────────────────────────────────────────────────┐
│ session-abc123   feature/foo   [Paused]ⓘ           │  ← hover "Paused" to see tooltip
└───────────────────────────────────────────────────┘
```

**After (always-visible text sibling):**
```
┌───────────────────────────────────────────────────┐
│ session-abc123   feature/foo   [Paused]  Paused    │
│                                           automatically —│
│                                           no recent activity │
└───────────────────────────────────────────────────┘
```

More realistically, inline on one row with wrapping at narrow widths:
```
Desktop (wide):
┌────────────────────────────────────────────────────────────┐
│ session-abc123  feature/foo  [Paused]  ⏸ Paused automatically — no recent activity │
└────────────────────────────────────────────────────────────┘

Mobile (narrow, wraps):
┌───────────────────────────┐
│ session-abc123             │
│ feature/foo                │
│ [Paused]                   │
│ ⏸ Paused automatically —   │
│   no recent activity       │
└───────────────────────────┘
```

### Interaction flow

1. **At rest, desktop**: badge text is visible without any interaction —
   satisfies the "no dead ends on touch" requirement directly, since the
   reason no longer depends on a `:hover` event that touch devices cannot
   fire.
2. **Hover, desktop only**: the existing `Tooltip` wrapper is *retained*
   (not removed) around the status chip — hovering still shows the
   tooltip, which is now redundant with the visible text but harmless
   (matches Story 2.1.1's explicit "retained... for desktop hover
   discoverability and screen-reader redundancy" requirement). No new
   interaction is added; this is pure regression-avoidance.
3. **Screen reader**: `aria-label="Session status: Paused — auto:inactivity"`
   (or equivalent full-sentence form) remains on the status chip container;
   the newly-visible text sibling is additionally read as its own text node
   — screen reader users get the information twice (chip `aria-label` +
   plain text), which is acceptable redundancy, not a violation (the triad
   requires text to accompany the icon/color; having it twice via two
   different elements is strictly better than the tooltip-only baseline).

### Error / edge cases

- **`session.pauseReason` is empty/falsy but `isPaused` is true**: no badge
  text renders (existing `isPaused && session.pauseReason` guard is
  unchanged) — a paused session with no reason string shows only the
  `[Paused]` status chip, same as today. This is not a new edge case
  introduced by this project.
- **Long pause-reason text on a narrow mobile card**: text must wrap, not
  truncate silently or overflow the card — verify `SessionCard.css.ts`'s
  pill/badge class wraps (`white-space: normal` or equivalent) rather than
  using `text-overflow: ellipsis`, since ellipsis would recreate the
  "information exists but isn't visible" problem this surface exists to
  fix. Task 2.1.1a should reuse an existing wrapping pill class, not
  introduce a truncating one.
- **`STOPPED` + `creationProgress` branch** (a different existing tooltip
  case, same code region, lines 501–514 of `SessionCard.tsx`): out of scope
  for this project (not `pause_reason`) — do not accidentally also promote
  this tooltip while touching the adjacent branch; requirements.md scopes
  this story to `pause_reason` only.

---

## Surface 3: Item detail — Sessions section `end_reason` chip

### Wireframe (per-session row in `SessionsSection.tsx`)

**Clean end (no badge) — the common case:**
```
┌──────────────────────────────────────────────────────┐
│ session-9f2a1c   work   feature/retry-fix   Jul 30    │
│ Pipeline: Standard TDD                                │
└──────────────────────────────────────────────────────┘
```

**Warning-severity end_reason (timeout / process_error):**
```
┌──────────────────────────────────────────────────────┐
│ session-9f2a1c   work   feature/retry-fix   Jul 30    │
│ Pipeline: Standard TDD                                │
│ ⚠️ Headless call failed (process error)                │
└──────────────────────────────────────────────────────┘
```

**Error-severity end_reason (claude_not_found / other):**
```
┌──────────────────────────────────────────────────────┐
│ session-9f2a1c   triage   Jul 30                      │
│ Pipeline: Standard TDD                                │
│ ⛔ Headless call failed — claude CLI not found          │
└──────────────────────────────────────────────────────┘
```

Placement: new chip line sits immediately below the existing
`pipelineGroup` row, above the (already-existing, conditionally-rendered)
`verdictDetail` block — i.e. appended to the per-row vertical stack
`SessionsSection.tsx` already builds (`sessionRowMain` → `pipelineGroup` →
**[NEW] end-reason chip** → `verdictDetail`).

### Interaction flow

1. **At rest**: user expands the "Sessions (N)" `CollapsibleSection` (already
   collapsible/expandable via existing chevron/click, unchanged) and scans
   rows. A clean-ended session shows no extra line — visual noise stays
   proportional to what actually needs attention.
2. **No new click target**: the chip is inert text (icon + label +
   `aria-label`), not a button — clicking it does nothing extra; the row's
   existing `<a>`/`<CollapsibleSection>` (for synthetic rows) click targets
   are unchanged. This matches k8s pod status reasons' "the reason string
   itself is the fact, not an interactive affordance" pattern
   (`research/ux.md` §1).
3. **`useShowMore` cap** (existing, `SHOW_MORE_CAP = 5`): unaffected —
   end_reason chips render only within the currently-visible session rows;
   "Show N more" still works exactly as today.

### Error / edge cases

- **`endReason === ""` or `undefined`**: `formatEndReason("")` returns
  `severity: "none"` → no chip renders. This is the *majority* case (most
  sessions end cleanly) and must stay silent — confirmed explicitly in
  requirements/ux research as "not an unknown-data case."
  - **Testable**: a session ended via normal `shutdown` shows no chip; a
    session that has not yet ended (still running, `endedAt` unset) also
    shows no chip (there is no end_reason yet, and the session's live
    status already communicates "running").
- **Unrecognized/future `end_reason` value** (schema drift — a new value
  added server-side before the frontend's `formatEndReason` switch is
  updated): the `default` branch returns `{ label: "", severity: "none" }`
  per Task 1.2.2a — i.e. an unrecognized reason silently shows no chip
  rather than crashing or showing a raw enum string. This is a deliberate
  fail-safe, but it does mean a genuinely-new failure mode could go
  invisible until the formatter is updated — acceptable given the low
  blast radius (single-user tool, formatter update is a 1-line change), but
  worth a code comment at the `default` branch pointing back here so a
  future implementer doesn't mistake it for "any string is handled."
- **A session row that never gets `endReason` populated because the backend
  conversion (`itemSessionToProto`) wasn't updated** (an implementation
  bug, not a UX case): would present identically to "clean end" — no chip.
  This is why Story 1.1.2's acceptance criteria pins a concrete
  `EndReason: "process_error"` example — the UX design cannot itself catch
  a silently-empty wire field; that is a backend test's job (already
  covered in the implementation plan).

---

## Surface 4: Item detail — Lifecycle Summary `×N` suffix (full variant)

### Wireframe (`LifecycleSummary.tsx`, header region of item detail)

```
┌───────────────────────────────────────────────────────────────┐
│  ○──●──○──○──○   Idea → Ready → In Progress → Review → Done    │ ← StageTracker
│                                                                  │
│  🟠 Stale work session ×3  ⏱ stuck 6h        Category: infra    │ ← BlockerChip full + duration
│                                                                  │
│  ● Last checked 2m ago                                          │ ← LivenessLine
└───────────────────────────────────────────────────────────────┘
```

`variant="full"` already renders icon + label + duration
(`formatStuckDuration`); this project adds the `×N` suffix between label and
duration, in the same relative position as Surface 1's compact chip, so a
user who has already learned the compact-chip vocabulary on the board sees
the identical suffix meaning here — no second vocabulary to learn.

### Interaction flow

1. **At rest**: this is the *first thing* a user sees on opening an item's
   detail panel, directly answering the job-to-be-done ("is this
   self-healing or truly stuck") before they need to expand anything else.
2. **From here, user can drill in**: scrolling down to Surface 5 (Respawn
   History, collapsed by default) gives the full per-attempt timeline if
   the `×N` count alone isn't enough — this is the "click into the run" step
   from the GitHub Actions precedent.
3. **`aria-label` on hover/focus** (desktop) and **on tap** (mobile, via
   screen reader/VoiceOver navigating to the element — not a hover
   gesture): reads the full sentence, e.g.
   `aria-label="Stale work session — respawned 3 times"`. No separate
   tooltip needed here since the text label is already always-visible
   (unlike Surface 2's pre-project tooltip-only state) — this chip was
   never tooltip-gated to begin with.

### Error / edge cases

- Same "parked" (`>= 5`) and "0 attempts, no suffix" cases as Surface 1 —
  identical data source (`StuckBacklogItem`), identical `BlockerChip`
  component, so behavior is guaranteed consistent by construction (one
  component, two variants) rather than needing to be independently
  verified per surface.
- **`stuckItem` undefined** (item not currently stuck): `LifecycleSummary`
  already guards this (`{stuckItem && <BlockerChip .../>}`) — no chip at
  all, StageTracker/Category/LivenessLine render normally. No new edge case.

---

## Surface 5: Item detail — new `RespawnHistorySection` (collapsible timeline)

### Wireframe — collapsed (default state)

```
┌───────────────────────────────────────────────────────────────┐
│  ▸ Respawn History                                              │ ← collapsed, click/tap to expand
└───────────────────────────────────────────────────────────────┘
```

### Wireframe — expanded, with events

```
┌───────────────────────────────────────────────────────────────┐
│  ▾ Respawn History                                               │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ Stale work session remediated              Jul 28, 3:14pm│   │
│  │ triggered by session stale-ab → resulted in session fresh-cd│ │
│  ├─────────────────────────────────────────────────────────┤   │
│  │ Stale work session remediated              Jul 29, 9:02am│   │
│  │ triggered by session fresh-cd → resulted in session new1-ef│ │
│  ├─────────────────────────────────────────────────────────┤   │
│  │ Autonomous turn budget respawned            Jul 30, 1:47pm│   │
│  │ (no session — spawn was queued or failed)                 │   │
│  └─────────────────────────────────────────────────────────┘   │
│  [ Show 5 more ]                                                 │  ← useShowMore, cap=8
└───────────────────────────────────────────────────────────────┘
```

### Wireframe — expanded, empty state (never-remediated item)

```
┌───────────────────────────────────────────────────────────────┐
│  ▾ Respawn History                                               │
│  No automated respawns recorded for this item.                  │
└───────────────────────────────────────────────────────────────┘
```

Note: unlike `ProgressHistorySection`'s hide-when-empty pattern, this
section **always renders the header** (collapsed or expanded), even for
items with zero respawn events — mirroring `WorkflowHistorySection.tsx`'s
precedent exactly (per Task 4.5.2's acceptance criteria and features.md's
explicit recommendation cited in the plan). This is a deliberate deviation
from `SessionsSection.tsx`'s own "return null if empty" pattern — the two
existing precedents disagree with each other, and this plan picks the
`WorkflowHistorySection` precedent because "never respawned" is itself
useful negative information for a *stuck* item (if an item is stuck but has
zero respawn events, that itself tells the user the system never attempted
automated recovery — worth knowing), whereas `SessionsSection`'s hide-when-
empty is correct there because an item with zero sessions hasn't started
yet at all (a different, earlier lifecycle state where showing nothing is
correct).

### Interaction flow

1. **At rest**: section appears collapsed below "Progress History" in the
   item detail panel's existing vertical stack of `CollapsibleSection`s,
   consistent with every other section on this page. No auto-expand, even
   for parked/heavily-respawned items — matches this codebase's uniform
   collapsed-by-default convention (`backlog-item-detail-ux`), so a user
   who wants full detail always takes the same one-click action regardless
   of item state.
2. **Click/tap the header**: expands in place (existing `CollapsibleSection`
   keyboard + click + `aria-expanded` behavior, unchanged — this project
   adds no new interaction primitive, only a new instance of the existing
   one). Expand state persists across remounts via
   `useSectionExpandState(itemId, "respawn-history", false)`, same as every
   sibling section.
3. **`useShowMore` cap (8)**: if more than 8 events exist, a "Show N more"
   button appends the remainder in place — no pagination, no new page,
   identical UX to `WorkflowHistorySection`'s existing show-more button.
4. **Session references are inert text, never links**: `triggering session`
   and `resulting session` values render as plain short-ID text (first 8
   chars), never as clickable links to `/?session=...` — this is a
   deliberate choice, not an oversight: a respawn event may reference a
   session that has since been deleted (row 3 of the frontend's own
   in-plan reasoning notes this can't currently occur due to cascade-delete
   scoping, but the *rendering* must not assume that invariant holds
   forever — treating the reference as inert text is defensive and costs
   nothing).

### Error / edge cases

- **`resultingSessionUuid === ""`** (a queued-not-yet-spawned or
  failed-to-spawn attempt — e.g. hit the concurrency cap): row renders
  `"(no session — spawn was queued or failed)"` in place of a session
  reference — never an empty string, dash, or blank area. This is the
  literal edge case named in the requirements' "queued respawn with no
  resulting session" scenario.
- **`triggeringSessionUuid === ""`** (e.g. `AutoRespawnReview`/
  `AutoRespawnTriage`, which always pass `""` for triggering session since
  there was no active session to begin with — an abandoned-review or
  orphaned-triage re-trigger starts from "nothing," not from a prior
  session): the "triggered by session X" clause is omitted entirely from
  the row (not rendered as "triggered by session (unknown)"), per Task
  4.5.2b's explicit "omit the 'triggered by' clause entirely" instruction —
  omitting an inapplicable field reads cleaner than a placeholder for data
  that was never expected to exist here.
- **Item that was never remediated (zero respawn events)**: explicit empty
  state text, not an omitted section — see wireframe above. This is
  expected to be the *majority* case across all items, so the empty-state
  copy must read as neutral/informational, not alarming (confirmed
  wording: "No automated respawns recorded for this item." — declarative,
  not "Nothing to show" or similar dead-end phrasing).
- **A respawn event whose target session was later deleted** (theoretically
  possible if cascade-delete scoping ever changes, per ux.md §4's original
  framing): renders identically to any other event row — short session-ID
  text, no link, no error state, no "(session no longer exists)" special
  case needed *in practice* today because the ID alone (not a live lookup)
  is all the row ever displays; there is nothing to "fail" to resolve since
  the UI never attempts to dereference the ID into a live session object.
  This is a stronger guarantee than ux.md's original "(session no longer
  exists)" qualifier suggested was needed — worth confirming during
  implementation that no future change accidentally adds a live lookup
  here without also adding that qualifier back.
- **Timestamp missing/malformed** (`createdAt: ""`, a proto conversion
  edge case): `formatDate("")` — verify during implementation that this
  existing shared formatter degrades gracefully (e.g. to an empty string
  or "unknown date") rather than rendering `"Invalid Date"` — this is an
  existing formatter already used by `WorkflowHistorySection`/
  `ProgressHistorySection`, so if it already has this guard, no new work is
  needed here; if not, it's a shared fix benefiting all three call sites,
  not a `RespawnHistorySection`-specific patch.

---

## Cross-surface consistency notes

- **One severity vocabulary, five surfaces**: `stuckReason.ts`'s
  icon/color mapping is the single source of truth reused (directly, via
  `BlockerChip`) on Surfaces 1 and 4, and conceptually mirrored (same
  three-tier none/warning/error shape) by `formatEndReason` on Surface 3.
  `pause_reason` (Surface 2) is deliberately *not* forced into this same
  severity scale — a pause is not a failure (it can be entirely manual/
  intentional), so it keeps its own neutral, informational chip styling
  rather than borrowing warning/error color tokens. Do not recolor the
  pause badge to match stuck-reason red/orange during implementation; that
  would misrepresent "user paused this on purpose" as an error condition.
- **Respawn count (`×N`) always pairs with the same underlying number** a
  user could otherwise only get by counting rows in Surface 5's timeline —
  the count is a derived convenience, not a second source of truth; if
  they ever disagree in a bug report, `RespawnHistorySection`'s row count
  is authoritative (it's the actual event log) and `remediationAttempts`
  is the backend's own tracked counter, which is expected to match it 1:1
  for `stale_work_remediation`/`autonomous_turn_respawn` reasons but is a
  *separate* field (`BacklogStuckState.remediation_attempts`, not derived
  from `RespawnEvent` rows) — worth a one-line code comment near the `×N`
  render call flagging this is not literally `respawnEvents.length`, to
  prevent a future "just count the array" refactor that would silently
  change behavior for stuck states not backed by the audit trail.

---

## UX Acceptance Criteria

Each criterion is written to be verifiable by a human clicking through the
running app (`make install-service`'s manual-test-instance pattern, not
`make ci`), organized by surface.

### Task completion / click-count

1. From the board page, a user can determine whether a stuck item is
   "actively retrying" vs. "parked" in **0 clicks** (visible on the card
   itself via the `×N` suffix + chip color) — no navigation required.
2. From the board page, a user can reach the full respawn history (reason,
   timestamp, session references) for any item in **≤ 2 clicks**: (1) click
   the card to open item detail, (2) click "Respawn History" to expand it.
3. From the item detail panel, a user can determine why a specific session
   ended (clean/timeout/error) in **0 additional clicks** once the Sessions
   section is expanded (the section itself may already require 1 click if
   collapsed — but no further interaction is needed per-row).
4. On a touch device with no hover capability, a user can read a paused
   session's pause reason in **0 gestures beyond normal scrolling** — no
   tap-and-hold, no hover-simulation workaround required (this is the
   explicit fix for Surface 2's pre-project tooltip-only gap).

### Error states / no dead ends

5. A respawn event with `resultingSessionUuid === ""` shows the literal
   text "(no session — spawn was queued or failed)" — never a blank space,
   dash-only, or `undefined` string rendered to the DOM. **Exit path**: none
   needed — this is a terminal, informational row, not an error requiring
   recovery; the "exit" is simply reading the next row.
6. An unrecognized/future `end_reason` enum value renders no chip (fails
   safe to "no signal" rather than a broken chip or raw enum text) — the
   **exit path** is implicit: no bad state is ever shown to get stuck in.
7. Every new chip/badge introduced by this project (Surfaces 1, 2, 3, 4)
   is reachable and dismissable via the same interaction as its pre-
   existing sibling content on that card/row — i.e. none of these chips
   introduce a click-trap (e.g. a chip that swallows the card's own click
   event and prevents opening item detail). Verify specifically: clicking
   the `×N` suffix on the board card (Surface 1) still opens item detail
   (the suffix is not an independently-clickable element that
   `stopPropagation()`s).

### Accessibility

8. **Icon/text/aria-label triad**, verified per new surface:
   - Surface 1/4 `×N` suffix: visible `×3` text + `aria-label="Respawned 3
     times"` (or `"...— remediation attempts exhausted"` when parked) on
     the suffix `<span>`; the suffix has no `aria-hidden` icon of its own
     (it's a text-only sub-element of the parent chip, which already
     carries its own icon/label/aria-label triad unchanged).
   - Surface 2 pause badge: existing `aria-hidden` icon (if any) + newly-
     visible text sibling + retained `aria-label` on the status chip
     container — verify all three are present after the change, not just
     the two that existed before.
   - Surface 3 end_reason chip: `<span aria-hidden="true">{icon}</span>` +
     visible text label + `aria-label` on the containing `<span>` carrying
     the full sentence (e.g. `aria-label="Headless call failed (process
     error)"`) — matches `formatEndReason`'s returned `label` string
     verbatim, not a truncated or icon-only variant.
   - Surface 5 timeline rows: each row is a `role="listitem"` inside a
     `role="list"` container (matches `SessionsSection.tsx`'s existing
     `Linked sessions` list pattern) with plain readable text content — no
     icon-only cells requiring a separate `aria-label`.
9. **Keyboard navigation**: `RespawnHistorySection`'s header is reachable
   via Tab and toggles via Enter/Space, identical to every other
   `CollapsibleSection` on the page (inherited behavior, verify no
   regression) — the "Show N more" button (when present) is a standard
   `<button type="button">`, reachable and activatable via keyboard alone.
10. **Color contrast**: the `×N` suffix text and the new end_reason chip
    text both meet ≥ 4.5:1 contrast against their chip background,
    reusing existing `vars.color.warningText`/`errorText`/`successText`
    tokens (already-audited per the design system) rather than any new ad
    hoc color — no new contrast audit needed *if and only if* implementation
    strictly reuses these tokens as specified in the Pattern Decisions
    table; flag for a manual contrast check only if a new color is
    introduced.
11. **No color-only signal anywhere in this project's scope**: every
    chip/badge/suffix added or promoted by this project carries a text
    label a screen-reader user or colorblind user can read independent of
    color — verified per-surface above; this is the single hard rule
    `BlockerChip.tsx`'s own doc comment already states, extended to every
    new chip this project introduces.

### Mobile (touch, no hover) vs. desktop (hover) — explicit verification

| Surface | Desktop behavior | Mobile/touch behavior | Regression risk if unverified |
|---|---|---|---|
| 2 (pause badge) | Text visible always; hovering chip also shows redundant `Tooltip` | Text visible always; no hover event ever fires, no gesture needed | **High** — this is the literal case named in requirements.md ("hover tooltip is not discoverable on touch devices"); must be re-verified visually post-implementation, not just unit-tested |
| 1/4 (`×N` suffix) | Suffix visible always; `aria-label` also readable via hover-triggered browser tooltip (native `title`-less span has none unless one is added) | Suffix visible always; identical rendering, no touch-specific behavior needed since nothing here was ever hover-gated | Low — chip was already always-visible pre-project; this only adds more always-visible text |
| 3 (end_reason chip) | Visible always | Visible always | Low — same reasoning as above |
| 5 (Respawn History) | Click header to expand; "Show N more" is a click | Tap header to expand (verify tap target ≥ 44×44px per the `CollapsibleSection` header's existing touch-target sizing — inherited, not new); "Show N more" is a tap | Low — reuses `CollapsibleSection`/`useShowMore`, both already mobile-verified by `backlog-item-detail-ux` |

Explicit acceptance: **Surface 2 must be manually re-verified on an actual
touch viewport** (browser devtools mobile emulation is sufficient, per
`.claude/CLAUDE.md`'s manual-test-instance pattern) after implementation,
specifically confirming the pause-reason text renders without any tap,
hold, or hover simulation — this is the single highest-risk mobile
regression in this project since it's the one surface changing from a
hover-gated to an always-visible interaction model.

---

## Summary

- **5 surfaces designed**: board card (`BlockerChip` compact), session card
  (pause-reason badge), item detail Sessions section (end-reason chip),
  item detail Lifecycle Summary (`BlockerChip` full), and the new item
  detail Respawn History section.
- **11 UX acceptance criteria** written across four categories: task
  completion/click-count (4), error states/no dead ends (3), accessibility
  (4), plus a explicit mobile/desktop parity verification table covering
  all 5 surfaces.
