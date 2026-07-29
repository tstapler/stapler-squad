# UX Research: `duplicate` backlog status

## 1. `BacklogItemBadge` — existing pattern

`web-app/src/components/backlog/BacklogItemBadge.tsx` renders a status chip via a local
`STATUS_CLASS: Record<string, string>` map (status string → vanilla-extract class), falling
back to `styles.statusArchived` for unknown statuses:

```ts
const STATUS_CLASS: Record<string, string> = {
  idea: styles.statusIdea, refining: styles.statusRefining, ready: styles.statusReady,
  in_progress: styles.statusInProgress, review: styles.statusReview, done: styles.statusDone,
  archived: styles.statusArchived,
};
const getStatusClass = (s: string): string => STATUS_CLASS[s] ?? styles.statusArchived;
```

Label text comes from a **shared** helper, `getStatusLabel` in `web-app/src/lib/backlog/status.ts`
(`STATUS_LABELS` map, falls back to `s.replace(/_/g, " ")`). The file's own comment states the
design intent explicitly: *"CSS class mappings are intentionally kept per-component (vanilla-extract
generates scoped class names), but labels and the fallback formatter are shared."* This is a
deliberate, already-established convention, not an oversight.

Colors come from `web-app/src/styles/theme.css.ts`'s `vars.statusBadge.*` token group (bg/fg/border
triplets: `approval*`, `input*`, `complete*`, `uncommitted*`, `idle*`, `processing*`) plus a couple
of statuses (`idea`, `archived`, `refining`) that reuse generic `vars.color.surfaceMuted` /
`vars.color.textMuted` / `vars.color.warning*` tokens instead of a dedicated `statusBadge` slot.

**Exact shape needed to add `duplicate` cleanly** (per component touched):
- `theme.css.ts`: add a new dedicated token triplet, e.g. `vars.statusBadge.duplicateBg/Fg/Border`,
  defined once per each of the 6 themes (`lightTheme`, `darkTheme`, `matrixTheme`, `cyberpunk77Theme`,
  `wh40kTheme`, `cleanTheme`) — do **not** alias `statusBadge.idleBg/Fg/Border` or reuse `archived`'s
  tokens (`surfaceMuted`/`textDisabled`), since AC requires a genuinely distinct color, not a shared one.
- `status.ts`: add `duplicate: "Duplicate"` to `STATUS_LABELS` (shared, one place).
- In each `.css.ts` file that defines status chip styles, add `export const statusDuplicate = style({ background: vars.statusBadge.duplicateBg, color: vars.statusBadge.duplicateFg, border: \`1px solid ${vars.statusBadge.duplicateBorder}\` })`.
- In each component/file's local `STATUS_CLASS`/`STATUS_CSS` map, add `duplicate: styles.statusDuplicate`.

## 2. Other touchpoints — NOT one shared component, 4 separate places

Confirmed by direct inspection — status-badge rendering is **duplicated in (at least) 4 places**,
each with its own local status→class map that must be updated independently:

| File | What it is | Status map |
|---|---|---|
| `web-app/src/components/backlog/BacklogItemBadge.tsx` + `.css.ts` | Compact badge (used in board/card contexts) | own `STATUS_CLASS` map, own CSS classes |
| `web-app/src/components/backlog/BacklogItemDetail.tsx` + `.css.ts` | Detail panel status chip | **duplicated** own `STATUS_CLASS` map (lines 21-29) + own CSS classes (`statusIdea`…`statusArchived` in `BacklogItemDetail.css.ts`) — does not import `BacklogItemBadge` |
| `web-app/src/app/backlog/page.tsx` + `web-app/src/app/backlog/backlog.css.ts` | The backlog **table** (row status chip, `<table className={styles.table}>`) AND the **filter chips** (`StatusFilterChips` component, `ALL_STATUSES` array, `styles.filterChip`/`filterChipActive`) — both live in this one file/css pair | **duplicated** own `STATUS_CSS` map (lines 39-47) + own CSS classes in `backlog.css.ts` |
| `web-app/src/components/backlog/BacklogItemCard.tsx` | Card's `getActionSpec(item)` — a `switch (item.status)` that picks a button label/action, unrelated to color | No `case "duplicate"` today → falls to `default: return { label: item.status, action: item.status, isDone: true }`, i.e. it would render the literal lowercase string `"duplicate"` as the button label instead of "Duplicate" |

Practical consequence for the plan's task breakdown: this is **not** "add one case to a shared
badge component" — it is 3 independent CSS-class-map edits (`BacklogItemBadge`, `BacklogItemDetail`,
`page.tsx`/`backlog.css.ts`) plus 1 switch-case addition (`BacklogItemCard`), plus the 6-theme token
addition in `theme.css.ts`, plus one shared label entry in `status.ts`. Missing any one of the 3
CSS maps means `duplicate` silently falls back to `statusArchived` styling in that one surface only
(the existing `?? styles.statusArchived` fallback is exactly why AC calls out "not reusing
archived's" — that fallback is the trap).

Also note: `ALL_STATUSES` in `page.tsx` currently excludes `archived` from the filter-chip list via
`displayStatuses = ALL_STATUSES.filter((s) => s !== "archived")` — a product decision needed for
`duplicate` too (default-hidden like archived, or shown, since triage/cleanup work may want to
audit duplicates specifically). Flagging as an open decision for the plan phase, not deciding it here.

`BacklogItem.status` is typed as `KnownBacklogStatus | (string & {})` in
`web-app/src/lib/hooks/useBacklogService.ts` (line 20-23) — a string literal union with an escape
hatch, so adding `"duplicate"` to `KnownBacklogStatus` is additive and non-breaking.

## 3. Comparable patterns (brief)

Jira shows a "duplicates" link-type relationship in an "Issue Links" section with the target
ticket's key + summary as a clickable row. GitHub Issues marks an issue "duplicate of #N" via a
bot-style timeline comment plus a closed-with-reason badge, linking to the referenced issue. Linear
shows a compact "Duplicate of ENG-123" pill inline in the issue header linking straight to the
canonical issue. All three converge on the same minimal shape: **a status/badge indicator + one
inline link to the canonical item**, not a separate linking UI.

For stapler-squad, which already has a single-pane `BacklogItemDetail` (opened via `?item=<id>`
query param on `/backlog`, see `page.tsx` line 158 `searchParams.get("item")` and row-click handler
`handleRowClick`) rather than per-item routes, the minimal non-janky pattern is: render "Duplicate
of: <title>" as one line in the detail panel, where the title is a link that updates the same
`?item=` query param to the canonical item's id (identical mechanism to clicking a table row) —
no new route, no modal, no separate issue-linking UI.

## 4. Accessibility requirements

WCAG AA requires **4.5:1** contrast for normal (small) text and **3:1** for large text (≥18pt/24px,
or ≥14pt/18.66px bold) and for UI component boundaries (e.g. the chip's border against its
background) — per `WCAG_AA_NORMAL = 4.5` / `WCAG_AA_LARGE = 3.0` already encoded in
`web-app/scripts/check-theme-contrast.ts` line 81-82. The status chip text is 10px uppercase
(`fontSize: "10px"` in `BacklogItemBadge.css.ts` `statusChip`), which is well under the "large
text" threshold, so the **4.5:1 normal-text ratio applies**, not the relaxed 3:1 large-text one —
this matters because it's easy to mistakenly apply the looser threshold to a badge just because
it looks small/decorative.

**Gap found**: `check-theme-contrast.ts` currently only validates 4 generic text/background pairs
(`textPrimary`, `textSecondary`, `textMuted` vs `background`/`cardBackground`) for 4 of the 6 themes
(`matrix`, `cyberpunk77`, `wh40k`, `clean` — light/dark aren't in this script at all), and does
**not** check any `statusBadge.*` token pair. There is no existing automated gate for status-chip
contrast. The new `duplicate` token triplet's contrast will need manual verification (or, better,
extending this script to cover `statusBadge` pairs) across all 6 themes — this script is not a
safety net for this feature as-is.

**Color-blind safety**: the existing pattern already follows "color decorates, text discriminates" —
every status chip pairs its color with an always-visible uppercase text label (`{getStatusLabel(status)}`)
and an `aria-label={"Status: " + getStatusLabel(status)}`; color is never the only differentiator.
`duplicate` should follow the identical pattern: a new color token **and** the text label "DUPLICATE"
(via `statusDuplicate` class + `getStatusLabel`), so a colorblind user distinguishing `duplicate`
from `archived` reads different text, not just a subtly different gray/muted hue (which is the
actual risk today, since `archived` currently uses a fairly generic muted-gray token that a new
"duplicate" color must not visually collide with, especially in the muted-palette wh40k/clean themes).

## 5. Loading / resolved / missing states for "Duplicate of: <title>"

AC11 only specifies 2 of 3 states explicitly (success link, missing→plain text). The third
(loading) needs to be designed:

1. **Loading** (canonical item fetch in flight, via `getBacklogItem(id)` from
   `useBacklogService.ts` line 279/341, which is async and network-backed): show a low-emphasis
   placeholder, e.g. `Duplicate of: …` or a skeleton/muted "Loading…" span with
   `aria-live="polite"` — must not show a spinner-only state with no text (screen-reader users need
   the "Duplicate of:" label present immediately, only the title portion is pending). Keep it
   inline/compact — this is a one-line metadata row, not a blocking loader for the whole detail panel.
2. **Resolved (success)**: `getBacklogItem` returns a non-null item → render `Duplicate of: ` +
   an actual `<button>`/link styled as a link that on click updates the `?item=` query param to the
   canonical id (matching the existing row-click/`handleRowClick` navigation mechanism) — same-tab,
   in-app navigation, no external link semantics needed since this is 100% internal.
3. **Missing/failure** (`getBacklogItem` returns `null`, or throws — e.g. canonical item was deleted
   or the fetch errors): render plain, non-interactive text, e.g. `Duplicate of: (item not found)`
   — explicitly **not** a link, not a broken href, and the surrounding component must not throw/crash
   the detail panel render. This mirrors `getBacklogItem`'s existing contract: it already returns
   `Promise<BacklogItem | null>` and swallows/logs errors internally (`console.error` at line ~347),
   so callers can safely treat "null" as the one failure signal without needing a separate error
   boundary.

## 6. Job-to-be-done

- **Functional**: let someone triaging the backlog mark an item as a known duplicate and link it to
  its canonical counterpart, so nobody re-does the same investigation/implementation work twice.
- **Emotional**: gives the person doing cleanup confidence that closing out a duplicate isn't
  "losing" the earlier triage/discussion — it's still reachable one click away, not deleted.
- **Social/team**: turns "wait, didn't we already have a card for this?" tribal knowledge into a
  visible, navigable link on the item itself, so any teammate (or a future agent) can see the
  relationship without asking around.
