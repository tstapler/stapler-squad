# UX Design: context-health-monitoring

Phase 3.5 design artifact. Builds on `research/ux.md` (Phase 2) and turns
`implementation/plan.md` Epic 3.1 (`ContextHealthBadge`) / Epic 3.2 (session-card
integration) into concrete wireframes, interaction flows, and testable UX acceptance
criteria. Does not contradict the plan's component contract (states, null-render rules,
tooltip/aria-label text, paused-state suffix) — it visualizes and verifies it.

---

## Surfaces identified

| # | Surface | In scope this iteration? |
|---|---|---|
| 1 | The badge itself (3 rendered states: suppressed/green, amber, red) | Yes — Epic 3.1 |
| 2 | The badge's tooltip (hover + the accessible-name contract) | Yes — Epic 3.1 |
| 3 | Composition with the existing badge row on `SessionCard` | Yes — Epic 3.2 |
| 4 | A session list/board-level summary (e.g. a health count in a column header, or a filter) | **No** — checked against `requirements.md` Users/Consumers and Scope; not listed. `requirements.md` "In Scope" names only "a green/amber/red health indicator surfaced on the session card, with a tooltip" — no board-level aggregate. `SessionRow.tsx` (list view) is a smaller-format sibling of `SessionCard.tsx` but is not named in the plan's file list; treated as a fast-follow, not this iteration's surface (see Gaps below). |

Four candidate surfaces were considered per Step 1; three are in scope. The board/list
aggregate is explicitly out of scope per requirements.md's Scope section, which lists only
the per-card badge + tooltip and the `ContextHealth` RPC field — no board-level rollup, no
new filter, no notification tray. This matches Phase 2 research §5's explicit scoping
("no modal/notification beyond the badge").

---

## Surface 1 + 2: The badge and its tooltip

### Wireframe — badge glyphs (per `getContextHealthInfo`, plan.md Task 3.1.1b)

```
GREEN / UNSPECIFIED / UNKNOWN-WIRE-VALUE          →  (nothing rendered — null)

AMBER   ┌─────────────────────┐
        │ ⚠  Context Degrading │   pill, amber bg/text, 999px radius
        └─────────────────────┘

RED     ┌──────────────────────────┐
        │ ✖  Context Needs Attention│   pill, red bg/text, 999px radius
        └──────────────────────────┘
```

Shape note: `chip` (StatusBadge/SubStatusChip's shared base, `SubStatusChip.css.ts`) uses
`borderRadius: vars.radii.full` already — visually already a pill. Plan Task 3.1.1a asks
for `ContextHealthBadge`'s own pill to read as "deliberately more circular... so the shape
differs at a glance," which in practice means: keep the glyph+label pill shape family
consistent with the row (so it doesn't look like a foreign element), but rely on **fixed
position** (always last) and **distinct glyph pair** (⚠/✖, never reused verbatim from
`SubStatusChip`'s own ⚠/✖ — see Distinguishing amber-from-error below) as the primary
differentiators, per `research/ux.md` §2's "position alone signals this is a different
kind of thing."

### Distinguishing amber-from-error, red-from-error

`SubStatusChip.NEEDS_APPROVAL` already uses `⚠ Approve Tool Use` and
`SubStatusChip.ERROR` already uses `✖ Error`. This means `ContextHealthBadge`'s AMBER and
RED glyphs are **not novel to the badge row** — the row can already contain a `⚠` chip and
a `✖` chip today, before this feature ships. The differentiator a scanning user relies on
is therefore **position + label text**, not glyph novelty:

- Position: `ContextHealthBadge` is always the last badge (plan Task 3.2.1a) — a fixed
  scan-order slot none of the existing chips occupy.
- Label: "Context Degrading" / "Context Needs Attention" vs. "Approve Tool Use" / "Error"
  — no shared words, so even a user who only skims label text (not glyphs) disambiguates
  correctly.
- Color pairing is intentionally shared with `chipNeedsApproval`/`chipError` (same
  `warningBg`/`warningText`/`errorBg`/`errorText` tokens, per plan Task 3.1.1a) — this is a
  deliberate reuse of the existing semantic amber/red meaning ("something here needs a
  human"), not a bug. The distinguishing signal is position + label, exactly as
  `research/ux.md` §2 concludes.

### Interaction flow — hover (mouse)

```
User action                         System response
─────────────────────────────────   ──────────────────────────────────────────
1. Mouse enters the badge's         Radix Tooltip.Root starts its open-delay
   hit area                         timer (Tooltip.tsx: delayDuration={400}).
2. (nothing — passive wait)         After 400ms, Tooltip.Portal renders
                                    tooltipContent, positioned side="top",
                                    sideOffset 4px, with an arrow pointing
                                    at the badge.
3. Mouse leaves the badge           Tooltip closes immediately (Radix default
                                    close-on-pointer-leave).
```

The badge itself does nothing on click — it has no destructive or navigational action
(confirmed against plan.md: the component's only props are `health`, `reason`,
`isPaused`; no `onClick`). This matches `research/ux.md` §1's "never an interrupt"
recommendation and the requirements doc's explicit exclusion of the handoff/restart flow
from this badge.

### Interaction flow — keyboard

```
User action                         System response (as currently planned)
─────────────────────────────────   ──────────────────────────────────────────
1. User tabs through the card       Focus lands on the next *focusable*
                                    element — SEE GAP below: the badge's
                                    <span> has no tabIndex in plan Task
                                    3.1.1b, so focus SKIPS the badge
                                    entirely and its tooltip never opens
                                    via keyboard.
```

**Gap found — flagged per Step 5.** Radix `Tooltip.Trigger asChild` wraps whatever child
it's given; it does not itself add `tabIndex`. It attaches the open behavior to the
child's native `onFocus`/`onBlur` in addition to `onMouseEnter`/`onMouseLeave`, but a
plain `<span>` is not in the browser's default tab order, so it never receives an
`onFocus` event to trigger from. Plan Task 3.1.1b's exact JSX
(`<span role="status" aria-label={...} title={...} data-testid="...">`) has no
`tabIndex={0}`, so the tooltip is **mouse-only** as specced. This is not a regression the
plan introduces alone — `SessionCard.tsx:492-500` and `:506-514` show the *exact same*
gap already exists for the paused-reason and creation-progress tooltips (also
`<Tooltip><span role="img" ...></Tooltip>` with no `tabIndex`). Recommend adding
`tabIndex={0}` to the badge's `<span>` in Task 3.1.1b so `Tab` reaches it and focus opens
the tooltip (Radix's `onFocus` handler already covers the rest once the element is
tabbable) — a one-line change, no new task needed, but implementers should not assume the
existing sibling pattern is the accessible reference to copy.

### Tooltip content — accessible name contract (verbatim from plan.md Story 3.1.1)

```
AMBER, has reason, not paused:
  "Context health: Context Degrading — Repeated the same Bash call 3 times in a row"

RED, has reason, paused:
  "Context health: Context Needs Attention — Repeated the same Bash call 3 times in a row (paused)"
```

Both the visible tooltip text (`title`, and the Radix `Tooltip label` prop) and the
`aria-label` carry the **same full sentence** — there is no truncation point in the
plan's string template (`Context health: ${label}${reason ? \` — ${reason}\` : ""}${isPaused ? " (paused)" : ""}`)
and no fixed-width container is specced for the tooltip content (`tooltipContent` in
`Tooltip.css.ts` is not width-constrained per the files skimmed), so the "must never be
truncated/cut off" criterion is satisfiable as planned — flag only if a future CSS pass
adds `max-width` + `overflow: hidden` to `tooltipContent` without also raising
`white-space: normal` + no `text-overflow: ellipsis`.

### Error / edge-case handling (surface 1+2)

```
Case                          Rendered state                 Tooltip/aria-label
─────────────────────────    ──────────────────────────────  ──────────────────────────
No data yet (new session,    null — nothing rendered          n/a
health = UNSPECIFIED)
Confirmed healthy             null — nothing rendered          n/a
(health = GREEN)              (SAME visual as "no data" —
                               see Gap below)
Amber                          ⚠ Context Degrading pill        "Context health: Context
                                                                Degrading — <reason>"
Red                            ✖ Context Needs Attention pill  "Context health: Context
                                                                Needs Attention — <reason>"
Paused (any level)             frozen at last computed level;  "...— <reason> (paused)"
                                card also dims via existing
                                cardPaused styling
Underlying computation         null — nothing rendered          n/a
error / unrecognized wire
value (forward-compat)
```

**Gap found — GREEN and UNSPECIFIED are visually identical (both `null`).** Plan Task
3.1.1b and Story 3.1.1's acceptance criteria are explicit that both render `null` — this
is correct per `research/ux.md` §4's "don't show a false green" reasoning and is *not* a
defect, but it does mean a user cannot visually distinguish "3-turn-old session, no
verdict yet" from "20+ turns, confirmed healthy" — both look like the absence of a badge.
This is an intentional trust trade-off already made in Phase 2 research (§4: "default to
suppression unless research turns up a reason users need to see 'pending'") — recorded
here so it isn't rediscovered as a bug later, not as an open gap to fix.

---

## Surface 3: Composition with the existing session-card badge row

### Wireframe — badge row, left to right (per `research/ux.md` §2 enumeration + plan 3.2.1)

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│ SessionCard                                                                        │
│ ┌────────────────────────────────────────────────────────────────────────────┐   │
│ │ [ext] [PR#] [review-queue] [● Ready] [rate-limit] [StatusBadge] [SubStatus] │   │
│ │ [memory] [autonomous] [workflow] [pending-program-change] [⚠ Ctx Degrading]│   │
│ └────────────────────────────────────────────────────────────────────────────┘   │
│  ^ existing badges, unchanged order              new: ContextHealthBadge, always  │
│                                                   the LAST element (plan 3.2.1a)  │
└──────────────────────────────────────────────────────────────────────────────────┘
```

### Interaction flow — composition, no conflict

```
User action                          System response
────────────────────────────────    ───────────────────────────────────────────────
Session is in SubStatus.ERROR       Both `✖ Error` (SubStatusChip) and
AND ContextHealth.RED               `✖ Context Needs Attention` (ContextHealthBadge)
                                     render simultaneously, each with its own
                                     role="status" and independent accessible name
                                     (plan Story 3.2.1, 2nd AC). Neither suppresses
                                     the other — a transient tool error and a
                                     cumulative degradation trend are reported as
                                     two independent facts, matching research/ux.md
                                     §2's "never conflate a one-off error with
                                     sustained degradation."
Session is Paused, health = RED     Card gets `cardPaused` dimming (existing,
                                     unrelated to this feature) AND the health
                                     badge's accessible name gets the " (paused)"
                                     suffix (plan Story 3.2.1, 3rd AC) — so a user
                                     scanning a dimmed, paused card sees a red badge
                                     that reads "this was the state when it paused,"
                                     not "act now."
```

### Row layout at capacity (edge case not explicitly in plan — worth flagging for QA)

A card showing external-source + PR + review-queue + status + rate-limit + StatusBadge +
SubStatusChip + memory + autonomous + workflow + pending-program-change + health badge is
11 badges deep. `badges` (`SessionCard.css.ts`) is presumed flex-wrap (not verified in this
pass — flagged for implementer/QA to confirm during Task 3.2.1b's build/lint check) so the
health badge, being last in DOM order, could wrap to a second row on a narrow card. That's
an acceptable layout outcome (no clipping), but if `badges` uses `overflow: hidden` instead
of wrap, the last-position contract would silently clip the health badge off dense cards —
worth a manual check against a synthetic session with all badges populated during Task
3.2.1b, since the plan's automated tests assert DOM order, not visible-on-screen position.

---

## UX acceptance criteria (human-testable)

1. **At-a-glance distinction, no hover required.** Given a board of a dozen session
   cards where one has `ContextHealth.RED` and none of the others do, a user can name
   which card needs attention within 2 seconds of looking at the board, without hovering
   any card — verified by the badge's fixed last-position + distinct label text (not
   relying on color alone, satisfying WCAG SC 1.4.1).
2. **Hover reveals the specific reason in ≤1 second beyond Radix's own open delay.**
   Given a card with `ContextHealth.AMBER` and a populated `reason`, when a user hovers
   the badge and waits past the 400ms Radix `delayDuration`, the full reason text is
   visible within 1 additional second (no spinner, no async fetch — `reason` ships with
   the same `WatchSessions` push as `health`, per plan's Domain Glossary
   `publishContextHealth` entry) and requires no click.
3. **Keyboard users can reach the same information.** *(Currently unmet as specced —
   see Gap above.)* Once `tabIndex={0}` is added per the recommendation above: given a
   user tabs to the health badge, the tooltip opens on focus and the full accessible name
   is announced by a screen reader, matching the mouse-hover experience content-for-content.
4. **No dead ends.** The badge has no `onClick`, no destructive action, and no modal —
   confirmed against the plan's prop contract (`health`, `reason`, `isPaused` only).
   Tabbing past it (once focusable) or clicking elsewhere dismisses the tooltip with no
   side effect on session state.
5. **Tooltip text is never truncated.** Given the longest specced reason string in the
   plan's examples (`"Repeated the same Bash call 3 times in a row"`, 45 chars) plus the
   `"Context health: Context Needs Attention — "` prefix (43 chars) and optional `" (paused)"`
   suffix (9 chars) — total ~97 characters — the tooltip renders the complete string with
   no `text-overflow: ellipsis` and no fixed `max-width` that would wrap-clip it (see
   Tooltip content section above).
6. **Screen-reader label is present and correctly worded.** Given any of the three
   non-null states, `role="status"` is set and `aria-label` contains the literal
   substring "Context health" (never just "Warning"/"Error"), per plan Story 3.1.1's 4th
   AC — testable via `getByRole("status", { name: /Context health/ })` in the Jest suite
   already planned in Task 3.1.1c.
7. **Color contrast ≥ 4.5:1.** Verified against the actual light/dark theme hex values
   in `web-app/src/styles/theme.css.ts`:
   - Light theme: `warningText` `#92400e` on `warningBg` `#fef3c7` = **6.37:1** (pass).
   - Light theme: `errorText` `#991b1b` on `errorBg` `#fee2e2` = **6.8:1** (pass).
   - Dark theme: `warningText` `#fbbf24` on `warningBg` `#78350f` = **5.43:1** (pass).
   - Dark theme: `errorText` `#fca5a5` on `errorBg` `#7f1d1d` = **5.28:1** (pass).
   - Note: the `error` token itself (`#ef4444` light / used for the 1px border per
     `SubStatusChip.css.ts`'s `chipError`) is only ~3.08:1 against `errorBg` — acceptable
     because it is used for a **decorative border**, not body text (WCAG 1.4.11 UI-component
     contrast floor is 3:1, which this meets; WCAG 1.4.3's 4.5:1 text floor does not apply
     to borders). No text in the badge uses the bare `error`/`warning` (non-Text) tokens
     as foreground color, per plan Task 3.1.1a's spec (`warningText`/`errorText` for
     color, `warning`/`error` reserved for `chipNeedsApproval`-style borders if the
     implementer adds one — not required by the plan for this badge, which has no
     border in Task 3.1.1a's spec beyond the pill background).
8. **Paused sessions never read as an active emergency.** Given `isPaused={true}` and
   `health={RED}`, the accessible name and visible tooltip both end in the literal
   string `" (paused)"`, and the card's pre-existing `cardPaused` dimming is also active
   — a user should describe the state as "this needs attention when resumed," not
   "this needs attention right now," when asked to paraphrase what they see.

---

## Summary of gaps flagged for implementation (not blocking, but should be triaged)

1. **Keyboard accessibility gap**: `ContextHealthBadge`'s `<span>` (plan Task 3.1.1b) has
   no `tabIndex`, so the Radix tooltip cannot be opened via keyboard focus — the badge's
   reason text is mouse-only as currently specced. Same pre-existing gap found in
   `SessionCard.tsx:492-500` and `:506-514` (pause-reason/creation-progress tooltips).
   Fix: add `tabIndex={0}` to the badge's `<span>` in Task 3.1.1b.
2. **Row-wrap vs. clip at high badge count**: not verified whether `badges`
   (`SessionCard.css.ts`) wraps or clips at 11 badges; worth a manual screenshot check in
   Task 3.2.1b against a synthetic all-badges-populated session.
3. **GREEN/UNSPECIFIED are visually identical** (both suppressed) — intentional per Phase
   2 research, recorded here so it isn't rediscovered as a bug.
