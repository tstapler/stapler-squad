# UX Design: `duplicate` Backlog Status

**Project**: duplicate-backlog-status
**Date**: 2026-07-29
**Status**: Draft — sanity-checked against `implementation/plan.md` Phase 5 (Epics 5.1–5.4)
**Inputs**: `requirements.md` (FR7, AC10–AC12), `research/ux.md`, `implementation/plan.md`
**Non-goal reminder**: there is no "mark as duplicate" UI. `duplicate_of_id` is set exclusively
by the `mark_duplicate` MCP tool (agent sessions only). Every surface below is **read-only**
with respect to the duplicate relationship — badges render it, one row links to it, nothing
sets it.

---

## 0. Surface inventory

Six user-facing surfaces touch `duplicate`. Three are independently-coded color badges (the
"badge, 4 render locations" framing in the task brief is closer to 3 true color badges + 1
non-color status-aware surface; see note at the end of §1 for the reconciliation), plus the
detail-panel link row and the filter chips:

| # | Surface | File(s) | Nature |
|---|---|---|---|
| S1 | Compact badge | `BacklogItemBadge.tsx` / `.css.ts` | color-coded chip |
| S2 | Detail panel header badge | `BacklogItemDetail.tsx` / `.css.ts` | color-coded chip |
| S3 | Backlog table row chip | `page.tsx` / `backlog.css.ts` | color-coded chip |
| S4 | Card action button | `BacklogItemCard.tsx` | text-only label, no color |
| S5 | Status filter chips | `page.tsx` (`StatusFilterChips`) | toggle button, no color, default-hidden |
| S6 | "Duplicate of: …" link row | `BacklogItemDetail.tsx` / `.css.ts` | 3-state async resolution |

No new screens, modals, or routes. No new user-triggerable actions.

---

## 1. Badge surfaces (S1–S3): compact chip, detail header chip, table row chip

All three follow the **identical** visual contract — this is deliberate: a `duplicate` item
must look the same wherever it appears, so a user scanning the table, opening the detail
panel, or seeing a card all get the same signal.

### 1.1 Wireframe — compact badge (S1, used in board/card-style contexts)

```
┌──────────────────────────────────────────────┐
│  Fix .zshrc sourcing bug in install-service   │
│  ┌───────────┐                                │
│  │ DUPLICATE │  AC: 2/2                       │
│  └───────────┘                                │
└──────────────────────────────────────────────┘
        ▲
        uppercase text label + distinct background/border color
        (never the same color as ARCHIVED)
```

### 1.2 Wireframe — detail panel header chip (S2)

```
┌─────────────────────────────────────────────────────────────┐
│  install-service.sh sources .zshrc unconditionally      [x] │
│  ┌───────────┐  ┌────┐   Created Jul 2, 2026                │
│  │ DUPLICATE │  │ P2 │                                      │
│  └───────────┘  └────┘                                      │
│  ⤷ Duplicate of: install-service.sh .zshrc bug (canonical)  │  ← S6, see §3
├───────────────────────────────────────────────────────────────
│  (rest of detail panel: description, AC list, sessions, …)   │
└─────────────────────────────────────────────────────────────┘
```

### 1.3 Wireframe — backlog table row chip (S3)

```
Title                                    Status        Priority   Updated
─────────────────────────────────────────────────────────────────────────
install-service.sh sources .zshrc…      [DUPLICATE]      P2      Jul 2
Add duplicate detection embeddings      [IDEA]           P3      Jul 20
Fix flaky e2e test in session-lifecycle [ARCHIVED]       P4      Jun 30
```
Note the visually distinct chip color between the `DUPLICATE` and `ARCHIVED` rows — this is
the crux of AC10 and the specific regression the fallback (`?? styles.statusArchived`)
otherwise causes silently.

### 1.4 Interaction flow (S1–S3)

There is no interaction beyond hover — these are read-only status indicators.

```
Item has status="duplicate"
        │
        ▼
STATUS_CLASS["duplicate"] found in this surface's own map?
        │
   ┌────┴────┐
  yes         no (map not updated for this surface)
   │           │
   ▼           ▼
render      SILENTLY falls back to styles.statusArchived
statusDuplicate   → visually indistinguishable from an archived item
(correct)         → THIS IS THE BUG CLASS AC10 EXISTS TO PREVENT
```

### 1.5 Edge cases

- **Unknown/future status string** (not `duplicate`, not in the map): existing `?? fallback`
  behavior is intentionally preserved for genuinely unknown statuses — only `duplicate` must
  resolve to its own class, not the fallback.
- **Empty/missing status**: not a new case introduced by this feature; existing behavior
  (fallback to archived styling) is unchanged and out of scope.
- **Color-blind users**: color is never the sole signal. Every chip carries the visible
  uppercase text `DUPLICATE` and an `aria-label="Status: Duplicate"` — text discriminates,
  color decorates. This matches the existing pattern for all other statuses and must not be
  weakened for `duplicate`.

### 1.6 Non-color surface — S4 (`BacklogItemCard` action button)

```
┌───────────────────────────────────────────┐
│ Fix .zshrc sourcing bug          [ P2 ]    │
│                                             │
│               [ Duplicate ]  ← disabled-   │
│                                 style button│
└───────────────────────────────────────────┘
```
- Button renders `actionSpec.label` = `"Duplicate"` (capitalized word, not the raw lowercase
  status string `"duplicate"` that the current `default:` branch would otherwise emit).
- `isDone: true` ⇒ button is visually in its "settled" state and the click handler is a no-op
  (`if (!actionSpec.disabled && !actionSpec.isDone) onAction(...)` — `isDone` short-circuits
  it), matching how `done`/`archived` already behave. No new interaction is introduced.
- `aria-label={actionSpec.label}` ⇒ screen readers hear "Duplicate" — reads as an adjective
  describing the card's state, same grammatical pattern as "Archived", "Done ✓". Acceptable;
  no ambiguity with a verb-form action button (there's no "Mark Duplicate" button anywhere —
  correctly enforced, since that would violate the stated Non-Goal).

### 1.7 Reconciling "4 render locations" with the actual codebase

The task brief's framing ("badge, 4 independent render locations") is a slight simplification.
Direct inspection of the 4 files Epic 5.3 touches shows:
- **3** are true color-coded status badges with independent `STATUS_CLASS`/`STATUS_CSS` maps
  (S1, S2, S3) — each is a distinct regression surface for the `?? statusArchived` fallback
  trap.
- **1** (`BacklogItemCard`, S4) is status-aware but renders no color at all — it's a `switch`
  producing a text label for an action button. Grouping it under "badge" is imprecise, but
  functionally it's still a 4th place where a missing `case "duplicate":` silently produces
  wrong output (the raw string `"duplicate"` instead of `"Duplicate"`), which is the same class
  of bug (missing entry → silent wrong render) as the 3 color-badge surfaces. Plan Epic 5.3's
  own title, "The 4 Independent Status-Aware Surfaces," is the more accurate framing and is
  what this design document follows.

---

## 2. Filter chips (S5): default-hidden

### 2.1 Wireframe

```
Filter by status:
┌──────┐ ┌──────────┐ ┌───────┐ ┌─────────────┐ ┌────────┐ ┌──────┐
│ Idea │ │ Refining │ │ Ready │ │ In Progress │ │ Review │ │ Done │
└──────┘ └──────────┘ └───────┘ └─────────────┘ └────────┘ └──────┘

                                          (Archived and Duplicate NOT shown by default —
                                           consistent "too noisy" treatment)
```

If a caller explicitly filters `status=duplicate` via URL/query (or a future "show all
statuses" affordance), the chip still exists in `ALL_STATUSES` and can be toggled — it's
excluded only from the **default-displayed** chip set, not from the underlying status vocabulary.

### 2.2 Interaction flow

```
Page loads (no explicit status filter in URL)
        │
        ▼
displayStatuses = ALL_STATUSES - {archived, duplicate}
        │
        ▼
User sees 6 chips (idea…done); duplicate items are absent from the
default table view (server-side ListBacklogItems exclusion, FR6)
AND absent from the filter-chip row (frontend-only cosmetic exclusion, FR7)
        │
        ▼
User can still reach duplicate items by:
  (a) an explicit deep link with ?item=<duplicate-item-id> (opens detail directly,
      independent of the list/filter state — verified: BacklogItemDetail fetches
      by itemId prop, not from the filtered `items` array)
  (b) clicking through from another item's "Duplicate of:" link (S6) if that other
      item points at a duplicate item as its canonical target (uncommon but not
      prevented — see chain edge case in §3.4)
```

### 2.3 Edge case: is hiding `duplicate` by default the right call?

`research/ux.md` flagged this as an open product decision rather than deciding it. This
design confirms the plan's choice (exclude by default, same as `archived`) is the right
default for the **primary/triage view** — a person doing day-to-day backlog work does not
want dead ends cluttering the list. The mitigating factor that makes "no way to bulk-audit
duplicates from the UI" acceptable for v1: this feature's Non-Goal already states the UI does
not manage the duplicate relationship; auditing/bulk-review of duplicates is an agent/MCP
concern (`list_backlog_items` with `include_terminal` from an agent session), not a human-UI
gap that needs solving here. No blocking issue; flagged as accepted scope limitation, not a fix.

---

## 3. "Duplicate of: <title>" link row (S6)

This is the one surface with actual state and async behavior. It renders directly under the
detail panel's status badge (see wireframe in §1.2), only when
`item.status === "duplicate" && item.duplicateOfId` is truthy.

### 3.1 State diagram

```
                    item.status === "duplicate" && item.duplicateOfId set?
                                        │
                                   ┌────┴────┐
                                  no          yes
                                   │           │
                                   ▼           ▼
                            (row not      duplicateOfItem state = undefined
                             rendered           │
                             at all)            ▼
                                          call getBacklogItem(item.duplicateOfId)
                                                 │
                              ┌──────────────────┼──────────────────┐
                              ▼                  ▼                  ▼
                        LOADING            RESOLVED            MISSING
                    (fetch in flight)   (non-null item)    (null: deleted,
                                                             bad id, or the
                                                             fetch errored —
                                                             getBacklogItem's
                                                             contract folds
                                                             both into null)
```

### 3.2 Wireframe per state

```
LOADING:
  ┌──────────────────────────────────────┐
  │ Duplicate of: Loading…               │   (aria-live="polite" container;
  └──────────────────────────────────────┘    plain text, no spinner-only state)

RESOLVED:
  ┌──────────────────────────────────────────────────────────────┐
  │ Duplicate of: [install-service.sh sources .zshrc bug]        │  ← clickable,
  └──────────────────────────────────────────────────────────────┘    underline on
                                                                        hover/focus

MISSING:
  ┌──────────────────────────────────────┐
  │ Duplicate of: (item not found)       │   ← plain text, NOT a link,
  └──────────────────────────────────────┘     no href, no crash
```

### 3.3 Interaction flow — resolved state click

```
User clicks/keyboard-activates the "Duplicate of: <title>" button
        │
        ▼
onNavigateToItem(canonicalId) fires
        │
        ▼
Same mechanism as handleRowClick in page.tsx:
  URLSearchParams.set("item", canonicalId) → router.push(`/backlog?item=<id>`)
        │
        ▼
BacklogItemDetail unmounts/remounts (or re-fetches) for the new itemId
        │
        ▼
Canonical item's OWN detail panel opens — same tab, in-app, no new route,
no modal. 1 click, 0 page reloads.
        │
        ▼
If the canonical item is itself archived/done/duplicate (i.e. excluded from the
default LIST view), it still opens correctly: BacklogItemDetail fetches by
itemId prop directly via getBacklogItem(id), independent of the filtered
`items` array or the current status-filter chips. Verified against page.tsx:
detail pane's itemId comes from `searchParams.get("item")`, not from `items`.
```

### 3.4 Edge cases (explicit ask: canonical deleted/archived/missing, and chain drift)

| Scenario | Behavior | Why it's safe |
|---|---|---|
| **Canonical item deleted** | `getBacklogItem` returns `null` → MISSING state: "Duplicate of: (item not found)", plain text | Matches `getBacklogItem`'s documented contract (`Promise<BacklogItem \| null>`); no separate error boundary needed |
| **Canonical item archived** (exists, but `status: "archived"`) | `getBacklogItem` still returns the item (archived ≠ deleted) → RESOLVED state, clickable link to the archived item's own detail view | Archived items remain individually fetchable by id; only the *list* view excludes them by default, not single-item fetch. Clicking through correctly opens the archived item's detail panel (its own status badge will read ARCHIVED there — no special-casing needed in the link row itself) |
| **Canonical item also `duplicate`** (chain: A → B → C) | Per `ADR-002` the backend guard should prevent a `duplicate` item from being set as *another* item's `duplicate_of_id` target... but if data drifts (manual DB edit, bug, or a future guard regression), the UI does **one hop only**: A's row resolves to B and shows "Duplicate of: <B's title>", clickable. It does **not** attempt to recursively resolve to C. | No recursion exists in the UI code (`getBacklogItem` is called exactly once, keyed on the *currently open* item's own `duplicateOfId`) — there is no code path that could infinite-loop or stack-overflow even if the backend invariant is violated. If the user clicks through to B, B's own detail panel independently renders B's own "Duplicate of: C" row at that time. This is intentional and should be called out in code comments so a future maintainer doesn't "helpfully" add chain-walking logic that could infinite-loop on a data-drift cycle (A→B→A) |
| **Self-reference drift** (`duplicate_of_id === item.id`, should be blocked by `TransitionGuard`'s self-reference check, but a direct DB edit could still produce it) | Not explicitly handled by the plan's Task 5.3.2d as written. **Recommended defensive addition** (see §5.4 below): if `duplicateOfItem.id === item.id`, treat as MISSING (render "Duplicate of: (invalid self-reference)" plain text) rather than rendering a link that points at the very panel already open | Cheap one-line guard; prevents a confusing (though not crashing) self-referential link if backend invariants are ever bypassed by direct data manipulation |
| **`duplicateOfId` set but `status` is not `"duplicate"`** (e.g. item was reopened via `duplicate → idea` and `duplicate_of_id` wasn't cleared) | Row does not render at all — the guard condition is `item.status === "duplicate" && item.duplicateOfId`, so a stale `duplicateOfId` on a non-duplicate item is invisible | Correct: a reopened item is no longer "a duplicate," so it should not show a duplicate-of link regardless of what's left in the field |
| **Fetch throws (network error)** | `getBacklogItem` swallows/logs internally and resolves `null` per its existing contract | Same as "deleted" — folds into MISSING state, no separate error UI needed |

### 3.5 Accessibility for S6 specifically

- `aria-live="polite"` on the container means the **whole phrase** must be self-describing in
  every state (never just a diff/fragment) — all three states already satisfy this: "Duplicate
  of: Loading…", "Duplicate of: <title>", "Duplicate of: (item not found)" are each complete,
  independently meaningful sentences.
- The resolved-state element is a real `<button>` (not a `<div onClick>` or bare `<a href="#">`),
  keyboard-focusable and activatable with Enter/Space, with `data-testid="duplicate-of-link"`
  for test targeting.
- The missing/loading states render plain `<span>` text with no interactive semantics
  (no `role="button"`, no `tabIndex`) — a screen reader user correctly perceives these as
  non-actionable, and a sighted user gets no misleading hover/focus affordance on inert text.

---

## 4. Cross-cutting accessibility requirements (all surfaces)

1. **Contrast**: every `duplicate` color pair (`statusBadge.duplicateBg` / `duplicateFg`) meets
   **4.5:1** (normal-text threshold — the chip's 10px uppercase text does not qualify for the
   3:1 large-text exception) in all 6 themes: light, dark, matrix, cyberpunk77, wh40k, clean.
2. **Never color-only**: every badge surface pairs its color with the visible text label
   "DUPLICATE" (or "Duplicate" in the action button/link row) and an `aria-label`. No surface
   may rely on color alone to distinguish `duplicate` from `archived` or any other status.
3. **Distinct from `archived`, specifically**: because the existing fallback pattern in all
   3 color-badge maps is `?? styles.statusArchived`, the single most important automated check
   is not "does `duplicate` have a color" but "is `duplicate`'s color/class **provably not
   equal to** `archived`'s" in each of the 3 independent maps. This is the exact regression the
   plan's Task 5.4.1a/b/c tests are designed to catch — see §5 acceptance criteria.
4. **Keyboard navigation**: the only new interactive element introduced by this feature is the
   resolved-state button in S6. It must be reachable via standard Tab order (no `tabIndex`
   manipulation needed since it's a native `<button>`) and activatable via Enter/Space.
5. **Screen-reader labels**: `aria-label="Status: Duplicate"` on every badge; the link row needs
   no separate `aria-label` beyond its own visible text (see §3.5) since the button's accessible
   name is already its full text content, "Duplicate of: <title>".

---

## 5. UX acceptance criteria (human-testable)

Each is phrased so a human reviewer (not just a unit test) can walk through it manually.

1. **Badge distinctness (S1)**: Open a card view showing a `duplicate` item and an `archived`
   item side by side. **In ≤ 0 clicks** (passive observation), their status chips are visibly
   different colors and both display distinct uppercase text (`DUPLICATE` vs `ARCHIVED`).
2. **Badge distinctness (S2, detail panel)**: Open the detail panel for a `duplicate` item.
   **In 1 click** (from table row or card), the header shows a `DUPLICATE` chip whose color is
   not the same as what an `archived` item's header chip shows.
3. **Badge distinctness (S3, table)**: On the backlog table with no status filter applied
   (default view), if a `duplicate` item is visible (e.g. via a direct deep link that bypasses
   the default list exclusion, or with an explicit status filter applied), its row chip is
   visibly distinct from any `archived` row's chip. **0 additional clicks** beyond having the
   row on screen.
4. **Action button label (S4)**: Open a card for a `duplicate` item. Its action button reads
   "Duplicate" (capitalized word), never the literal string `"duplicate"`, and clicking it is a
   no-op (consistent with `Done ✓`/`Archived`'s settled-state behavior). **0 clicks required to
   observe** (label is visible on render); clicking it must not error or trigger any RPC.
5. **Filter chips hide `duplicate` by default**: Load `/backlog` with no query params. The
   status filter chip row shows 6 chips (Idea, Refining, Ready, In Progress, Review, Done) and
   does **not** show a "Duplicate" chip. **In 0 clicks**, a first-time visitor's filter row is
   not cluttered with a status most workflows don't need to filter on directly.
6. **Duplicate-link happy path**: Open the detail panel for a `duplicate` item whose canonical
   target exists. **In ≤ 2 clicks total** (1 to open the duplicate item, 1 to follow the
   "Duplicate of:" link), the user lands on the canonical item's own detail panel, confirmed by
   the panel's title matching the canonical item's title.
7. **Duplicate-link loading state**: On a slow/throttled connection, opening a `duplicate`
   item's detail panel shows "Duplicate of: Loading…" (not a blank row, not a spinner with no
   text) before the canonical title appears. No layout shift beyond the text itself updating.
8. **Duplicate-link missing state (canonical deleted)**: Open a `duplicate` item whose
   `duplicate_of_id` points at a deleted/nonexistent item. The row reads "Duplicate of: (item
   not found)" as plain non-interactive text — attempting to click/tab to it does nothing (no
   focus ring, no cursor pointer, no href). **Exit path**: the rest of the detail panel (title,
   description, AC list, close button) remains fully usable — this row's failure never blocks
   or crashes the surrounding panel.
9. **Duplicate-link to an archived canonical item**: Open a `duplicate` item whose canonical
   target is itself `archived`. **In 1 click** on the link, the archived canonical item's detail
   panel opens successfully (not a 404, not a blank panel) even though archived items are
   excluded from the default table/list view.
10. **No dead ends**: In every state of S6 (loading, resolved, missing) and in the filter-chip
    default view, there is always at least one way forward — close the panel (`[x]`, always
    present), navigate to another item, or clear filters. No state described in this document
    can strand a user on a broken or blank screen.
11. **Contrast, all 6 themes**: For each theme (light, dark, matrix, cyberpunk77, wh40k, clean),
    the `duplicate` badge's text-on-background contrast ratio is **≥ 4.5:1**, verified either by
    the extended `check-theme-contrast.ts` script or by manual computation recorded in the plan
    (Task 5.1.2a already records computed ratios ranging 5.92:1–14.23:1 across the 6 themes —
    all pass with margin).
12. **Keyboard-only pass**: Using Tab/Shift+Tab/Enter only (no mouse), a keyboard user can:
    reach a `duplicate` item's action button and confirm it's a no-op; open its detail panel;
    Tab to the "Duplicate of:" link (when resolved) and activate it with Enter to navigate to
    the canonical item. **0 keyboard traps** at any point.
13. **Screen-reader announcement sanity**: With a screen reader active, opening a `duplicate`
    item's detail panel and waiting for the canonical fetch to resolve results in an
    announcement of the *canonical item's title* (via the `aria-live="polite"` region), not
    silence and not a repeat of "Duplicate of:" with no discernible content change. (See §6 —
    this is the one criterion where the current plan's literal copy needs a small edit to pass
    reliably.)

---

## 6. Sanity check against `plan.md` Phase 5 — findings

### 6.1 RESOLVED (fixed in a prior repair pass): loading-state copy — "Duplicate of: …" vs. explicit "Loading…"

**Original concern**: this design doc's initial instinct — "does `aria-live="polite"` announcing
the text change from `…` to the title work?" — turned out to be fine **for the transition
itself** (the container's full text content changes from one complete phrase to another; live
regions announce the new complete text, not a diff). The actual problem was upstream of that:
the bare ellipsis character `…` on its own, as the *initial* loading copy, is not reliably
announced as "loading" by a screen reader. Default punctuation-verbosity settings in the major
screen readers (NVDA, JAWS, VoiceOver) commonly suppress standalone punctuation like an
ellipsis, so a user may hear only **"Duplicate of:"** with nothing after it — indistinguishable
from a broken or empty label, not a perceptible "something is loading" signal.

**Status**: fixed. `plan.md` Task 5.3.2d and Task 5.4.1e now already contain the corrected copy
(`Duplicate of: Loading…` instead of bare `Duplicate of: …`) and the corresponding test asserts
on the substring "Loading" rather than the literal `…` character — this was applied in a prior
repair pass on `plan.md`, not left outstanding. No further action needed here; this section is
retained as a record of the finding and its resolution, not as a pending should-fix item.

### 6.2 Confirmed correct: 3-state resolution model

The `undefined | null | BacklogItem` tri-state (Task 5.3.2d) correctly maps to the 3 UX states
this document independently derived (loading/resolved/missing) and correctly folds "deleted"
and "fetch error" into a single MISSING state, matching `getBacklogItem`'s actual contract
(`Promise<BacklogItem | null>`, errors swallowed internally). No change needed here.

### 6.3 Confirmed correct: navigation mechanism reuse

Task 5.3.2c/5.3.2d's choice to thread `onNavigateToItem` down to reuse the exact same
`?item=`-query-param mechanism as `handleRowClick` (rather than inventing a new route, modal,
or navigation function) is the right call — it's the same pattern Linear/GitHub/Jira converge
on (a single inline link, no separate issue-linking UI), and it was independently verified
against `page.tsx` that the detail panel's fetch-by-id is decoupled from the filtered list, so
navigating to an archived/duplicate/done canonical item via this link works without needing any
special-casing.

### 6.4 Minor gap (non-blocking): no defensive handling for self-reference/chain drift in UI code

Neither Task 5.3.2d's snippet nor any other Phase 5 task explicitly guards against
`duplicateOfItem.id === item.id` (self-reference via data drift) — see §3.4. This is not
required to satisfy any stated AC (the backend guard is the source of truth preventing this at
write time), and the current code would not crash if it happened — it would just render a
slightly confusing self-pointing link. Recommended as an optional defensive one-liner
(`if (duplicateOfItem && duplicateOfItem.id !== item.id) { ...render link... } else if
(duplicateOfItem) { ...render as MISSING-style plain text... }`) but not required for AC10/AC11
sign-off. No chain-walking logic should ever be added (see §3.4) — this is a "don't add
something," not a "go build something," recommendation.

### 6.5 Confirmed correct: filter-chip default-hide decision

Excluding `duplicate` from `displayStatuses` alongside `archived` (Task 5.3.3c) is the right
default per §2.3 above — no change recommended.

### 6.6 No findings on: theme tokens (Epic 5.1), label/type plumbing (Epic 5.2), badge CSS-map
additions (Epic 5.3.1/5.3.2/5.3.3 CSS + map portions), action-spec branch (Epic 5.3.4), or test
coverage shape (Epic 5.4) — all match this document's surface analysis with no gaps found.

---

## 7. Summary

- **6 surfaces** designed: compact badge, detail-panel header badge, table row chip, card action
  button, filter chips, duplicate-of link row.
- **13 UX acceptance criteria** written (§5), each phrased as a human-walkable scenario with an
  explicit click count and, where relevant, an explicit exit path.
- **1 should-fix flagged and since resolved**: loading-state copy now reads "Duplicate of:
  Loading…" instead of bare "Duplicate of: …" for reliable screen-reader announcement (§6.1) —
  applied to Task 5.3.2d/5.4.1e in a prior repair pass on `plan.md`, not outstanding.
- **1 optional defensive addition flagged** (not required for sign-off): guard against
  self-referential `duplicateOfId` data drift in the link-row render (§6.4).
- Everything else in Phase 5 (theme tokens, label/type plumbing, the 3 CSS-map additions, the
  action-spec branch, the filter-chip exclusion, and the test coverage shape) checks out against
  this design with no changes needed.
