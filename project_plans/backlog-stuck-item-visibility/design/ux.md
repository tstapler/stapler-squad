# UX Design: Backlog Stuck-Item Visibility

Companion to `requirements.md`, `research/ux.md`, and `implementation/plan.md`. This
document is the UX design artifact for Phase 4 (frontend) — wireframes, interaction
flows, error/edge-case handling, and testable acceptance criteria — for the "Stuck
Backlog Items" section being added to the existing `/unfinished` page.

Conventions followed throughout (per `research/ux.md` and direct inspection of
`web-app/src/app/unfinished/UnfinishedTab.tsx`, `web-app/src/components/unfinished/*`,
`web-app/src/components/backlog/BacklogItemBadge.tsx`):
- Nav badge: hidden at zero, `aria-label` full phrase (`UnfinishedNavBadge.tsx`).
- Filter chips: `role="group"`, `aria-pressed` per chip (`UnfinishedTab.tsx` lines 132-143).
- Card: click-to-expand, `role="button"`, `aria-expanded`, Enter/Space toggle, Escape
  collapse (`UnfinishedItem.tsx` lines 31-42, 56-63).
- Status chips: color class + text label pair, never color-only
  (`BacklogItemBadge.tsx` `STATUS_CLASS` + `getStatusLabel`).
- Empty states: reassuring, filter-aware distinct copy (`UnfinishedTab.tsx` line 152).
- Degraded/unavailable states get their own explicit banner, not a silent empty list
  (`GitHubPRsSection.tsx` `DeviceAuthBanner`).

---

## Surface Inventory

| # | Surface | Type | New/Extends |
|---|---|---|---|
| 1 | `StuckNavBadge` | Persistent counter badge (nav) | New component |
| 2 | `StuckItemsSection` — populated | Grouped/filterable list | New component, mounted on `/unfinished` |
| 3 | `StuckItemsSection` — empty (zero stuck) | Empty state | New |
| 4 | `StuckItemsSection` — filtered-empty | Empty state (filter active) | New |
| 5 | `StuckItemsSection` — loading | Loading state | New |
| 6 | `StuckItemsSection` — fetch error / stale | Error/degraded state | New |
| 7 | `StuckItem` card (glance row) | List item | New component |
| 8 | `StuckItem` — `pr_status_unknown` variant | Degraded per-item state | New |
| 9 | `StuckItemDetail` (expand-on-click) | Detail view | New component |
| 10 | Snooze flow | Interaction / confirmation | New (Phase 5) |
| 11 | Live background update while viewing | Background state sync | Cross-cutting behavior |
| 12 | Item resolves while expanded | Edge-case transition | Cross-cutting behavior |

12 surfaces total (7 distinct visual states across 3 new components, 1 nav badge, plus
2 cross-cutting behaviors that touch all of them).

---

## Surface 1: `StuckNavBadge`

**Purpose**: answer "is anything stuck?" in under a second, zero navigation — same job
as `UnfinishedNavBadge`/`ReviewQueueNavBadge`.

```
┌─ Primary Nav ──────────────────────────────────────┐
│  Sessions   Unfinished (3)   Backlog   Settings    │
│                        ▲                            │
│                        └─ StuckNavBadge, count=3    │
│                           hidden entirely if 0       │
└──────────────────────────────────────────────────────┘
```

### Interaction flow
1. User glances at nav bar (no click required).
2. Badge renders next to (or inline within) the existing "Unfinished" nav label,
   mirroring where `UnfinishedNavBadge` already sits.
3. Click/tap on the nav item navigates to `/unfinished`, which auto-scrolls or is
   already positioned so the Stuck Backlog Items section is visible without further
   scrolling on arrival (it renders above the fold, directly under the filter chips —
   see Surface 2).

### Error/edge cases
- **RPC fails on badge's poll**: badge keeps showing its last-known count rather than
  disappearing (disappearing would read as "nothing stuck," which is false and
  dangerous per the trust-restoration job-to-be-done). No error chrome on the badge
  itself — the detail is available on the full page (Surface 6 carries the error
  affordance).
- **Count is 0**: badge does not render at all (matches `UnfinishedNavBadge` precedent) —
  this is a deliberate zero state, not an error.
- **First load, no prior count cached** (before the first successful fetch ever
  completes, so there is no last-known count to fall back on): the badge shows a
  **neutral loading affordance** — no number, rendered as a subtle pulse/skeleton dot
  (or simply nothing until the first response) — and specifically **never a "0" and
  never a confident empty state**, because a stale/premature "0" (or a blank that reads
  as "confirmed zero") would be misread as "nothing is stuck" before the check has
  actually run, recreating exactly the false-confidence problem this feature exists to
  fix. Once the first fetch resolves, it settles to either the real count (≥1) or the
  genuine hidden-at-zero state above.

---

## Surface 2: `StuckItemsSection` — populated

**Purpose**: grouped-by-reason, scannable list; the "triage, not investigation" mental
model from `research/ux.md` §2 — duration-since-stuck visible at a glance, detail behind
a click.

```
┌─ /unfinished ──────────────────────────────────────────────────────────┐
│  Unfinished Work                          Last scanned 4s ago  [Refresh]│
│  [All] [Uncommitted] [Ahead] [Behind]                                   │
│                                                                          │
│  ▸ GitHub PRs section (existing, unchanged)                             │
│                                                                          │
│ ┌─ Stuck Backlog Items ────────────────────────── 6 stuck ───────────┐ │
│ │  [All (6)] [PR ready (1)] [Rework cap (1)] [Abandoned (2)]         │ │
│ │  [Bouncing (1)] [Push failed (1)]        role="group"              │ │
│ │  aria-live="polite" region wraps the "6 stuck" count               │ │
│ │                                                                     │ │
│ │  ── PR ready to merge (1) ──────────────────────────────────────   │ │
│ │  ┌─────────────────────────────────────────────────────────────┐  │ │
│ │  │ 🟢 PR ready to merge   fix: benchmark job CI       stuck 3d  │  │ │
│ │  │ PR #148 · repo/stapler-squad                        [Snooze]│  │ │
│ │  └─────────────────────────────────────────────────────────────┘  │ │
│ │                                                                     │ │
│ │  ── Abandoned review (2) ────────────────────────────────────────  │ │
│ │  ┌─────────────────────────────────────────────────────────────┐  │ │
│ │  │ 🟡 Abandoned review   feat: omnibar workflow detect stuck 18m│  │ │
│ │  │ item 96cc9eaa                                        [Snooze]│  │ │
│ │  └─────────────────────────────────────────────────────────────┘  │ │
│ │  ┌─────────────────────────────────────────────────────────────┐  │ │
│ │  │ 🟡 Abandoned review   fix: worktree cleanup path   stuck 22m│  │ │
│ │  │ item 3fa910bc                                        [Snooze]│  │ │
│ │  └─────────────────────────────────────────────────────────────┘  │ │
│ │                                                                     │ │
│ │  ── Rework cap hit (1) ──────────────────────────────────────────  │ │
│ │  ┌─────────────────────────────────────────────────────────────┐  │ │
│ │  │ 🔴 Rework cap hit     fix: diff auto-repair loop  stuck 2h  │  │ │
│ │  │ item 96cc9eaa · 3/3 work sessions used              [Snooze]│  │ │
│ │  └─────────────────────────────────────────────────────────────┘  │ │
│ │                                                                     │ │
│ │  ── Bouncing (1) ─────────────────────────────────────────────────  │ │
│ │  ┌─────────────────────────────────────────────────────────────┐  │ │
│ │  │ 🔁 Not converging    feat: bench regression retry  stuck 4d │  │ │
│ │  │ item df0d5872 · 3 review cycles in 24h              [Snooze]│  │ │
│ │  └─────────────────────────────────────────────────────────────┘  │ │
│ │                                                                     │ │
│ │  ── Push/PR-create failed (1) ───────────────────────────────────  │ │
│ │  ┌─────────────────────────────────────────────────────────────┐  │ │
│ │  │ ⛔ Push/PR-create failed  fix: flaky e2e retry     stuck 1h │  │ │
│ │  │ item a1b2c3d4 · no PR created (push rejected)       [Snooze]│  │ │
│ │  └─────────────────────────────────────────────────────────────┘  │ │
│ └─────────────────────────────────────────────────────────────────────┘ │
│                                                                          │
│  ▸ Repo-grouped worktree cards (existing, unchanged)                    │
└──────────────────────────────────────────────────────────────────────────┘
```

Ordering: groups appear reason-class first (the actionable unit, per `research/ux.md`
§1). **Group order is by typical actionability, NOT severity** — `pr_ready_unmerged`
leads because it is the most immediately actionable (one known next step: merge it),
followed by the reasons that need a decision or investigation (`abandoned_review`,
`rework_cap`, `bouncing`, `push_failed`). This ordering is a fixed, deliberate
convenience heuristic and must **not** be read as a danger/severity ranking — a
`push_failed` item lower in the list is not "less serious" than a PR-ready one, it is
merely less mechanically obvious what to do next. (If a maintainer later prefers strict
alphabetical or a different fixed order, that is acceptable too — the one hard rule is
that group order is never presented or color-coded as a severity signal.) **Secondary
sort (within each group): stuck-longest-first** (per plan.md AC "sorted
stuck-longest-first") — the item that has been stuck the longest sits at the top of its
group, so the most-overdue item in each reason class is always the first one seen.
Section sits directly below the existing filter-chip row and
above `GitHubPRsSection`/repo groups — it's the first thing that answers "what needs
me" before the more exploratory worktree list.

**Multi-reason fan-out cross-reference (same item in 2+ groups):** because the backend
writes one row per `(item_id, reason)` (plan.md Observability Plan "Multi-reason
fanout"), a single item that matches several `StuckReason` conditions at once appears
as a separate card in each reason group. In the wireframe above, item `96cc9eaa`
surfaces **twice** — once under "Abandoned review" and once under "Rework cap hit". To
stop the user miscounting ("is that 6 items or 5?") or assuming that acting on one card
fully clears the item, every card whose `item_id` also appears in another currently-shown
group carries a small, low-effort shared indicator:

```
┌─────────────────────────────────────────────────────────────┐
│ 🟡 Abandoned review   feat: omnibar workflow detect stuck 18m│
│ item 96cc9eaa · also stuck for 1 other reason ⓘ    [Snooze] │  ← "+N other reasons" badge
└─────────────────────────────────────────────────────────────┘
```

The badge reads **"also stuck for N other reason(s)"** (N = count of *other* open groups
this same `item_id` appears in, within the current filtered view). It is informational
only (a title/tooltip may name the other reasons); it does not need to be a link. The
matching item title across cards, plus this badge, are enough for the user to recognize
the two cards refer to the same underlying item. Cross-referencing is computed
client-side from the already-fetched list (group by `item_id`), so it costs no extra
fetch. When a reason filter is active such that the item only appears once in view, the
badge is suppressed (nothing to cross-reference).

**Responsive layout of the filter-chip row (6 chips):** at narrow viewports the chip row
must **wrap onto multiple lines** (`display: flex; flexWrap: "wrap"; gap` — the exact
convention already used by `/unfinished`'s own filter row in `UnfinishedTab.css.ts` lines
19 & 88) — it must **never** squeeze the chips into an unreadable single line, clip them
with `overflow: hidden`, or push any chip off-screen. (Horizontal scroll of the chip row
is an acceptable alternative if the design team later prefers it, but wrap is the default
and matches the existing page.) Each chip keeps its full text+count label when wrapped;
chips are never truncated to just an icon. This is the same responsive behavior the
existing `[All][Uncommitted][Ahead][Behind]` row already exhibits, so the new row sits
consistently beside it on a phone.

```
Narrow viewport (phone) — filter chips wrap, never overflow:
┌─ Stuck Backlog Items ───── 6 stuck ──┐
│ [All (6)] [PR ready (1)]             │
│ [Rework cap (1)] [Abandoned (2)]     │   ← wraps to as many rows as needed
│ [Bouncing (1)] [Push failed (1)]    │
└──────────────────────────────────────┘
```

### Interaction flow
1. Page loads → `useStuckBacklogItems` hook fetches once, then polls (matching the
   existing 60s reconcile cadence as its baseline poll interval).
2. User optionally clicks a reason-filter chip (`aria-pressed`) to narrow to one
   class — same interaction as the existing `[All][Uncommitted][Ahead][Behind]` row.
3. User clicks a card → expands in place to `StuckItemDetail` (Surface 9). **Firm
   decision: multiple cards may be expanded simultaneously (non-exclusive accordion) —
   clicking a second card leaves the first expanded.** This is intentional, not left to
   the implementer: this section is a scanning/triage tool where comparing two expanded
   items side-by-side (e.g. two abandoned reviews, or a rework-cap item against a
   bouncing one) is a plausible, expected workflow, so opening one card must never
   auto-collapse another. Each card owns its own independent `aria-expanded` state; there
   is no single "currently open" index. (This is a deliberate divergence from any
   single-open behavior — do not re-introduce exclusivity to "match" another component.)
4. User clicks "Snooze" (hover-revealed, mirroring `UnfinishedItem`'s dismiss/snooze
   buttons) → Surface 10.

### Error/edge cases
- See Surfaces 3–6, 8, 11, 12 below — each is reached from this surface.

---

## Surface 3: `StuckItemsSection` — empty (zero stuck items)

```
┌─ Stuck Backlog Items ──────────────────────────────────────────────────┐
│                                                                          │
│                    ✓  Nothing stuck — all backlog items                 │
│                       are progressing.                                  │
│                                                                          │
└──────────────────────────────────────────────────────────────────────────┘
```

Copy: **"Nothing stuck — all backlog items are progressing."** (matches
`research/ux.md` §4's recommended reassuring phrasing, structurally identical to
`UnfinishedTab.tsx`'s "No unfinished work found. All repos are clean.").

### Interaction flow
- No action available; this is a terminal, confidence-building state. The section
  still renders (not hidden entirely) so the user gets positive confirmation the
  check ran, not just absence of a section.

### Error/edge cases
- N/A — this is itself the "good" edge case.

---

## Surface 4: `StuckItemsSection` — filtered-empty

```
┌─ Stuck Backlog Items ─────────────────────────────── 5 stuck ─────────┐
│  [All (5)] [PR ready (0)] [Rework cap (1)] [Abandoned (2)] [Bouncing]  │
│                    ▲ active filter, aria-pressed=true                 │
│                                                                          │
│              No stuck items match "PR ready to merge".                 │
│              [Clear filter]                                            │
└──────────────────────────────────────────────────────────────────────────┘
```

### Interaction flow
1. User has selected a reason-filter chip with a zero count for the current data.
2. Copy explicitly names the active filter (distinguishing this from Surface 3, per
   `research/ux.md` §4's "must distinguish empty-because-nothing-stuck from
   empty-because-filter-active").
3. "Clear filter" link/button resets to "All" — **this is the required exit path**
   for this state (no dead end).

### Error/edge cases
- If the count for a filter chip reaches 0 while that chip is selected (item resolved
  mid-view), transition to this state rather than silently reverting the filter
  selection out from under the user.

---

## Surface 5: `StuckItemsSection` — loading (initial fetch)

```
┌─ Stuck Backlog Items ───────────────────────────────────────────────────┐
│  ⟳ Checking for stuck items…                                            │
└────────────────────────────────────────────────────────────────────────┘
```

### Interaction flow
1. On first mount, before the first successful response, show a lightweight spinner
   + label (matches the existing `styles.spinner` + "Scanning…" pattern in
   `UnfinishedTab.tsx` lines 108-110) — not a full-page blocking loader, since this is
   one section among several already-rendered ones.
2. Resolves to Surface 2, 3, or 6 once the first fetch completes or fails.

### Error/edge cases
- If the initial fetch takes unusually long (no fixed timeout specified — the RPC
  itself should carry a client-side ConnectRPC timeout), fall through directly to
  Surface 6 (error) rather than spinning indefinitely — no dead ends.

---

## Surface 6: `StuckItemsSection` — fetch error / stale data banner

```
┌─ Stuck Backlog Items ─────────────────────────── ⚠ May be out of date ─┐
│  Couldn't refresh stuck items (last updated 6m ago).        [Retry]    │
│                                                                          │
│  [ ...last successfully fetched list still rendered below, if any... ] │
└──────────────────────────────────────────────────────────────────────────┘
```

### Interaction flow
1. A poll fails (network error, RPC error, server 5xx). The section does **not**
   blank out or silently keep polling with no indication — it shows a small banner
   above the (stale but still-shown) last-good list.
2. Banner states the fact plainly: "Couldn't refresh stuck items (last updated Nm
   ago)." and offers **"Retry"** — the required exit path.
3. Clicking Retry re-invokes the fetch; on success the banner clears and the poll
   interval resumes normally; on repeated failure the banner persists with an updated
   "last updated" time (never lies about freshness).
4. If this is the very first fetch (no prior successful data, so nothing stale to
   show — see Surface 5's fallthrough), the banner becomes the entire section body
   instead of sitting above a list:
   ```
   ┌─ Stuck Backlog Items ────────────────────────────────────────────────┐
   │  ⚠ Couldn't check for stuck items right now.               [Retry]  │
   └────────────────────────────────────────────────────────────────────────┘
   ```

### Error/edge cases
- This directly implements the requirement that a reconciler/RPC failure must never
  be silently read as "nothing is stuck" (research/ux.md §4, `DeviceAuthBanner`
  precedent). Distinct from Surface 8 (`pr_status_unknown`), which is a *per-item*
  degraded state when the list itself loaded fine but one item's GitHub check failed
  server-side.

---

## Surface 7: `StuckItem` card (glance row)

```
Desktop (hover-capable pointer):            Touch / no-hover pointer:
┌─────────────────────────────────────┐     ┌─────────────────────────────────────┐
│ 🟢 PR ready to merge  fix: … stuck 3d│     │ 🟢 PR ready to merge  fix: … stuck 3d│
│ PR #148 · repo/stapler-squad [Snooze▾]│    │ PR #148 · repo/stapler-squad    [ ⋮ ]│
└─────────────────────────────────────┘     └─────────────────────────────────────┘
  ▲ Snooze hover-revealed (secondary)          ▲ always-on kebab/overflow button,
    role="button" tabIndex=0 aria-expanded=false   ≥44×44px tap target, opens the same
    click/Enter/Space to expand                    Snooze picker (Surface 10)
```

**Touch/no-hover affordance (mandatory):** hover-reveal is a desktop *progressive
enhancement* only. On any device without a hover-capable pointer (detected via the
`(hover: none)` / `(pointer: coarse)` media query, matching how the rest of the app
gates hover-only chrome), the Snooze action is exposed by a **persistently-visible**
affordance — a kebab/overflow icon-button (`⋮`) or an always-shown small icon-button —
never gated behind hover (which is unreachable on touch). That affordance has a tap
target of **≥ 44×44px** (standard touch-target guidance) even if the icon glyph itself
is smaller. The card body remains the expand target; the always-on control is
positioned so it does not compete with the card's own tap-to-expand.

Chip legend (color + text always paired, per `STATUS_CLASS`/`getStatusLabel`
convention):

| Chip (icon is decorative, not sole signal) | Text label | Color class |
|---|---|---|
| 🟢 | "PR ready to merge" | `chipPrReady` (success-family color) |
| 🟡 | "Abandoned review" | `chipAbandonedReview` (warning-family) |
| 🔴 | "Rework cap hit" | `chipReworkCap` (danger-family) |
| 🟠 | "Stale work session" | `chipStaleWork` (warning-family) |
| 🔁 | "Not converging" | `chipBouncing` (danger-family) |
| ⛔ | "Push/PR-create failed" | `chipPushFailed` (danger/error-family — same failure-shaped styling as `chipReworkCap`) |
| ⚪ | "Couldn't check" | `chipUnknown` (neutral/muted — Surface 8) |

`STUCK_REASON_CLASS` + `getStuckReasonLabel` (per plan.md's Task 4.1.2a) are the code
artifacts backing this table — direct analog of `BacklogItemBadge.tsx`'s
`STATUS_CLASS`/`getStatusLabel`.

### Interaction flow
1. Glance-level facts shown without expanding: reason chip (icon + text), item
   title, duration-since-stuck ("stuck 3d"), and a one-line identity context (PR
   number + repo, or item ID + relevant count like "3/3 work sessions used" or "3
   review cycles in 24h" — this satisfies "work-session count vs cap" and
   "review-cycle count" directly at the glance level for the two reasons where that
   number *is* the reason, not just detail).
2. On a hover-capable pointer, hover reveals the "Snooze" action (mirrors
   `UnfinishedItem`'s hover-reveal dismiss/snooze buttons) — no hidden-until-hover
   *primary* information, only the secondary action. On touch / no-hover pointers
   (`hover: none` / `pointer: coarse`), the Snooze action is instead exposed by a
   persistently-visible kebab/overflow icon-button (≥ 44×44px tap target) so it is
   reachable without a hover event that can never fire on touch.
3. Click (or Enter/Space when focused) toggles `aria-expanded` and mounts
   `StuckItemDetail` beneath — verbatim copy of `UnfinishedItem.tsx`'s
   `handleKeyDown` (Enter/Space toggle, Escape collapses when expanded).

### Responsive layout of the card glance line (narrow viewports)

The glance line (reason chip · title · duration, plus the identity sub-line) must degrade
gracefully at phone widths using the **same convention the existing `UnfinishedItem` card
already uses** (`UnfinishedItem.css.ts`): the header is `display: flex` with
`flexWrap: "wrap"` and a `gap`, so the pieces **wrap onto a second line rather than
overflowing** the card. The reason chip and the duration ("stuck 3d") are the
fixed-width, always-visible anchors (`flexShrink: 0`, mirroring `UnfinishedItem`'s
`branch` element) so they are never the thing that gets clipped; the **item title is the
single flexible element that truncates** — `flexGrow: 1; overflow: hidden;
textOverflow: "ellipsis"; whiteSpace: "nowrap"` (exactly `UnfinishedItem.css.ts`'s `path`
style) with a `title=` tooltip carrying the full text. Net effect on a phone: the chip
and duration always stay legible; a long title ellipsizes; and if the row still cannot
fit, it wraps rather than horizontally scrolling the whole card or pushing the duration
off-screen.

```
Narrow viewport (phone) — glance line wraps, chip + duration stay put, title truncates:
┌─────────────────────────────────┐
│ 🟢 PR ready to merge    stuck 3d │   ← chip + duration = fixed anchors (flexShrink:0)
│ fix: benchmark job C…            │   ← title truncates with ellipsis + title= tooltip
│ PR #148 · repo/stapler-squad     │   ← identity sub-line wraps below if needed
└─────────────────────────────────┘
```

### Error/edge cases
- See Surface 8 for the degraded reason state.
- If `title` is very long, truncate with `title=` tooltip (matches
  `BacklogItemBadge.tsx`'s `truncate()` helper convention, and the `UnfinishedItem`
  `path`-style ellipsis above) — never let a long title break the card layout or push
  the chip/duration off-screen at any supported viewport width.

---

## Surface 8: `StuckItem` — `pr_status_unknown` variant

```
┌───────────────────────────────────────────────────────────────────────┐
│ ⚪ Couldn't check PR status   fix: benchmark job CI          stuck 3d │
│ PR #148 · last checked 47m ago (check failing)             [Snooze ▾]│
└───────────────────────────────────────────────────────────────────────┘

Expanded (StuckItemDetail, pr_status_unknown variant):
┌─ StuckItem card (expanded, pr_status_unknown) ────────────────────────┐
│ ⚪ Couldn't check PR status   fix: benchmark job CI          stuck 3d │
│ PR #148 · last checked 47m ago (check failing)             [Snooze ▾]│
├────────────────────────────────────────────────────────────────────────┤
│  Couldn't check this PR's status — no action available.               │
│  Last check: 47m ago (GitHub check failing)                           │
│  This will clear on its own once the next status check succeeds.      │
│  PR:         🔗 View PR #148 on GitHub                                 │
│                                                                          │
│  [Snooze 1 day ▾]                                                       │
└────────────────────────────────────────────────────────────────────────┘
```

The **"Couldn't check this PR's status — no action available."** line is required
literal copy on the `pr_status_unknown` detail — it makes the "no action available"
state an explicit on-screen UI string, not just narrative prose, matching the
concreteness of the `pr_ready_unmerged` detail's required "This PR is ready — merge it
on GitHub when you're ready." line (Surface 9).

### Interaction flow
1. Server-side, the reconciler's GitHub poll failed (rate limit / API error) for a
   `pr_pending` item mid-check. Per `implementation/plan.md`, this is a **derived
   UI-only state**, not a stored `StuckReason` — rendered client-side when
   `last_checked_at` is stale relative to the expected poll cadence (i.e., older than
   ~2x the reconcile interval) for an item whose reason implies GitHub-derived data.
2. The chip explicitly reads "Couldn't check PR status" (not "PR ready" and not a
   blank/missing chip) — this is the direct implementation of research/ux.md §4's
   requirement that a failed poll is never conflated with "healthy" or silently
   omitted.
3. Expanding shows the same detail view (Surface 9) with the last successfully known
   context plus a visible "last checked Nm ago" timestamp so the user can judge
   staleness themselves, and the literal **"Couldn't check this PR's status — no action
   available."** line so the absence of a next step is stated on-screen, not merely
   implied by the muted chip.

### Error/edge cases
- This state must never silently upgrade to "PR ready to merge" (🟢) — it only
  changes to 🟢 once a **fresh** successful poll confirms the healthy condition;
  showing stale-cached "ready" data would recreate exactly the trust problem this
  feature exists to fix (per requirements.md's "Emotional job").
- No user action resolves this directly (no manual re-check button — the RPC surface
  is out of scope for remediation per requirements.md's explicit non-goals); the exit
  path is simply that the next successful reconcile tick clears it. Snooze remains
  available since it's a visibility control, not remediation.

---

## Surface 9: `StuckItemDetail` (expand-on-click)

```
┌─ StuckItem card (expanded) ────────────────────────────────────────────┐
│ 🟢 PR ready to merge     fix: benchmark job CI               stuck 3d │
│ PR #148 · repo/stapler-squad                               [Snooze ▾]│
├─────────────────────────────────────────────────────────────────────────┤
│  Why:        PR #148 green & mergeable, unmerged for 3 days           │
│  This PR is ready — merge it on GitHub when you're ready.              │
│  Since:      2026-07-11 14:02 UTC  (first detected)                   │
│  Last check: 47s ago                                                   │
│  Repo auto-merge:  off  (allow_auto_merge: false)   ← read-only line   │
│  PR:         🔗 View PR #148 on GitHub                                 │
│                                                                          │
│  [Snooze 1 day ▾]                                                       │
└──────────────────────────────────────────────────────────────────────────┘
```

The **"This PR is ready — merge it on GitHub when you're ready."** line is required
copy on the `pr_ready_unmerged` detail: it makes the one available action explicit,
mirroring how the `pr_status_unknown` detail (Surface 8) explicitly states "no action
available." Without it, the auto-merge-disabled note + PR link only *imply* "go merge
this yourself." Note this is a factual restatement of the user's own next step, not a
one-click remediation control (merging still happens on GitHub, out of scope — see
requirements.md non-goals).

For `rework_cap` / `bouncing` reasons, the detail line set differs slightly to carry
the specific counters called out in the task brief:

```
┌─ StuckItem card (expanded, rework_cap) ────────────────────────────────┐
│ 🔴 Rework cap hit       fix: diff auto-repair loop            stuck 2h │
│ item 96cc9eaa                                                [Snooze ▾]│
├─────────────────────────────────────────────────────────────────────────┤
│  Why:        Auto-rework stopped after 3 failed review cycles         │
│              (cap = 3 work sessions)                                   │
│  Work sessions used:  3 / 3                                            │
│  Since:      2026-07-14 08:41 UTC  (first detected)                   │
│  Last check: 12s ago                                                   │
│                                                                          │
│  [Snooze 1 day ▾]                                                       │
└──────────────────────────────────────────────────────────────────────────┘
```

```
┌─ StuckItem card (expanded, bouncing) ──────────────────────────────────┐
│ 🔁 Not converging       feat: bench regression retry           stuck 4d │
│ item df0d5872                                                [Snooze ▾]│
├─────────────────────────────────────────────────────────────────────────┤
│  Why:        3 in_progress → review round trips in the last 24h,      │
│              no PASS verdict recorded                                 │
│  Review cycles (24h):  3                                                │
│  Since:      2026-07-10 09:15 UTC  (first detected)                   │
│  Last check: 30s ago                                                   │
│                                                                          │
│  [Snooze 1 day ▾]                                                       │
└──────────────────────────────────────────────────────────────────────────┘
```

### Interaction flow
1. Triggered by expanding the card (Surface 7). Renders inline beneath the card, not
   a modal — matches `UnfinishedItem.tsx` → `UnfinishedItemDetail` pattern exactly
   (no portal, no overlay).
2. All fields are read-only text/links. The PR link opens in a new tab
   (`target="_blank" rel="noreferrer"`, matching `GitHubPRsSection.tsx`'s `PRCard`
   link convention).
3. `allow_auto_merge` is fetched best-effort server-side (plan.md Story 4.1.4) and
   rendered only in this expanded view, never at the glance level (plan.md's explicit
   placement decision) — keeps the glance-level card uncluttered.
4. Escape key or re-clicking the card header collapses the detail (same as
   `UnfinishedItem.tsx`). On collapse, keyboard focus returns to that card's own toggle
   control (the `role="button"` header) — never dropped to `<body>` — per AC 29.

### Error/edge cases
- **`allow_auto_merge` fetch fails**: render `Repo auto-merge: unknown` and do not
  block or hide the rest of the detail panel (plan.md AC: "shows 'unknown' and does
  not block the rest of the detail" — this is the exit path, not an error dead end).
- **PR was merged/item resolved between list-render and expand-click** (race with a
  background poll): see Surface 12.
- **`context` field is empty/missing** (defensive): fall back to a generic
  reason-derived sentence (e.g., "No additional context recorded") rather than
  rendering a blank "Why:" line.

---

## Surface 10: Snooze flow

```
Step 1 — hover reveals control           Step 2 — click opens duration choice
┌───────────────────────────┐            ┌───────────────────────────┐
│ 🔴 Rework cap hit  [Snooze]│  click →   │ Snooze until:             │
│                            │            │  ( ) 1 hour               │
└───────────────────────────┘            │  (•) 1 day                │
                                          │  ( ) 3 days                │
                                          │  [Cancel]   [Confirm]      │
                                          └───────────────────────────┘

Step 3 — confirmed: item leaves the active list, toast confirms
┌─────────────────────────────────────────────┐
│  ✓ Snoozed "fix: diff auto-repair loop"       │
│    until tomorrow, 09:15                     │
└─────────────────────────────────────────────┘
(ephemeral toast, matches NotificationToast auto-dismiss pattern; item is NOT
permanently gone — it reappears automatically once snoozed_until passes and the
condition is still open)
```

### Interaction flow
1. User reaches the "Snooze" control → on a hover-capable pointer it appears on
   hover/focus (hover-reveal, matches `UnfinishedItem.tsx`'s dismiss/snooze buttons);
   on touch / no-hover it is the always-visible kebab/overflow button (≥ 44×44px, see
   Surface 7). Either entry point opens the same duration picker below.
2. Click opens a small duration picker (radio group, keyboard-navigable,
   `role="group"` + `aria-label="Snooze duration"`) — avoids a silent single-click
   "snooze forever" mistake; explicit AC: **"user can complete a snooze in ≤ 2
   clicks"** (open picker, confirm) matching the simplicity of the existing
   1-click `UnfinishedItem` snooze while still requiring an explicit duration since
   this feature's snooze has real consequences (suppresses a "PR unmerged for days"
   signal).
3. On confirm, calls `SnoozeStuckItem(item_id, reason, until)`; on success, the hook
   refetches and the item drops out of the active list (per plan.md AC); a brief
   ephemeral confirmation toast appears (reusing `NotificationToast` conventions,
   `aria-live="polite"`, auto-dismissing).
4. `Cancel` or clicking outside the picker closes it with no request sent — required
   exit path if the user changes their mind mid-flow.

### Error/edge cases
- **`SnoozeStuckItem` RPC fails**: picker stays open (or reopens) with an inline
  error message ("Couldn't snooze — try again") and a **Retry** action — do not
  silently close the picker as if it succeeded (that would be a false-positive dead
  end: the user believes they suppressed the signal but it's still live).
- **Item resolves on its own between opening the picker and confirming**: on confirm,
  if the server reports the row no longer exists/is already resolved, show "Already
  resolved — no longer stuck" instead of a generic error, and close the picker (the
  user's goal — "stop showing me this" — is already satisfied).

---

## Surface 11: Live background update while viewing (cross-cutting)

Applies to Surfaces 2, 7, 8 collectively.

```
Before poll tick:                         After poll tick (reason changed):
┌─────────────────────────────┐           ┌─────────────────────────────┐
│ 🟡 Abandoned review  ...     │  ──►      │ 🔴 Rework cap hit  ...       │
│ (position unchanged)         │  same     │ (position unchanged,        │
│                              │  card     │  chip swapped in place)     │
└─────────────────────────────┘           └─────────────────────────────┘
Count region: aria-live="polite" → screen readers hear "5 stuck" → "5 stuck"
(or the new count) WITHOUT interrupting whatever the user is doing — never role="alert"
for a routine poll update.
```

### Interaction flow
1. The hook polls on an interval (baseline: matching the 60s reconcile cadence).
2. On a data change, existing cards are diffed by `(item_id, reason)` identity and
   updated in place (chip swap, duration re-render) rather than the whole list being
   unmounted/remounted — avoids jarring reflow (research/ux.md §4).
3. The section's count summary region (`"5 stuck"`) carries `aria-live="polite"` so a
   screen-reader user is informed of the change without an interruption
   (`role="alert"` is reserved for actionable/urgent signals only, matching
   `NotificationToast.tsx`'s existing convention — a routine background refresh is
   never urgent).

### Error/edge cases
- See Surface 12 for the specific case of an item disappearing while expanded.
- A newly-appeared stuck item does **not** trigger a jarring toast interruption by
  itself in this view (the existing ephemeral `NotificationToast` already handles the
  "just became stuck" moment elsewhere, per the toast→panel duality in
  `research/ux.md`); this section's job is calm, persistent, always-there visibility,
  not another interruption source.

---

## Surface 12: Item resolves while expanded (edge-case transition)

```
User has this expanded when the underlying condition resolves (e.g., PR #148 merges):

┌─ StuckItem card (expanded) ───────────────────────────────────────────┐
│ 🟢 PR ready to merge     fix: benchmark job CI               stuck 3d │
│ PR #148 · repo/stapler-squad                               [Snooze ▾]│
├────────────────────────────────────────────────────────────────────────┤
│  ✓ This item was just resolved — PR #148 was merged.                  │
│    It will be removed from this list shortly.                         │
└────────────────────────────────────────────────────────────────────────┘
        (brief fade-out transition after ~2-3s, or immediately on next
         explicit user action — matches NotificationToast's
         visible/exiting CSS transition classes as precedent)
```

### Interaction flow
1. Background poll detects the row is now resolved (no longer in
   `FindOpenStuckStates`'s result) while the user has this specific card expanded.
2. Rather than yanking the card out from under an open detail panel mid-glance (jarring,
   research/ux.md §4), the card transitions to a brief "resolved" confirmation state
   in place, then fades out.
3. If the user is not actively looking at this card (collapsed), it can simply drop
   out of the list on the next render with no transition needed — the fade/hold
   behavior specifically protects the *expanded* case.

### Error/edge cases
- If the user clicks "Snooze" on a card that has just resolved server-side (race), the
  RPC should no-op gracefully server-side and the client should just let the fade-out
  proceed — no error surfaced for what is, from the user's perspective, a
  non-problem (the outcome they wanted — stop seeing it — already happened).

---

## UX Acceptance Criteria

### Task completion
1. User can determine "is anything stuck?" in **0 clicks** (nav badge visible on any
   page) and confirm details in **≤ 2 clicks** (click nav badge/tab → land on
   `/unfinished` with the section already visible; expand a card for full detail = 1
   more click, 2 total from anywhere in the app).
2. User can narrow to a single stuck-reason class in **1 click** (filter chip).
3. User can snooze a stuck item in **≤ 2 clicks** (open duration picker, confirm) from
   the list view — no navigation required.
4. User can reach the source PR from a `pr_ready_unmerged` item's detail in **1 click**
   after expanding (2 total: expand, then PR link).
5. User can clear an active reason-filter that yields zero results in **1 click**
   ("Clear filter" — Surface 4).

### Error states
6. When the stuck-items list fails to load/refresh, the section shows **"Couldn't
   refresh stuck items (last updated Nm ago)"** (or, on first load with no prior data,
   **"Couldn't check for stuck items right now"**) and offers a **Retry** button
   (Surface 6). The previously-successful list, if any, remains visible and is never
   silently blanked.
7. When a per-item GitHub check fails, that item shows a distinct **"Couldn't check PR
   status"** chip (⚪) with a visible "last checked Nm ago (check failing)" timestamp —
   never silently shown as, or upgraded to, the 🟢 "PR ready to merge" state on stale
   data (Surface 8).
8. When `allow_auto_merge` cannot be fetched, the detail view shows **"Repo auto-merge:
   unknown"** and all other detail fields still render normally (Surface 9).
9. When `SnoozeStuckItem` fails, the duration picker stays open (or reopens) with
   **"Couldn't snooze — try again"** and a Retry action; the item is not removed from
   the list on a failed snooze (Surface 10).

### No dead ends
10. Every error/degraded state above (6, 7, 8, 9) has a visible next action available
    to the user (Retry, Clear filter, or "detail still usable despite one field
    failing") — none of them require the user to reload the page or leave `/unfinished`
    to recover.
11. The filtered-empty state (Surface 4) always offers "Clear filter" — a filter
    selection can never trap the user in a permanently empty view with no way back to
    "All."
12. An item that resolves while its detail panel is open transitions visibly (Surface
    12) rather than disappearing with no explanation, so the user is never left
    wondering whether the disappearance was a bug.

### Accessibility
13. All interactive elements (filter chips, cards, snooze button/picker, Retry button,
    "Clear filter") are reachable and operable via keyboard alone: Tab to focus, Enter/
    Space to activate, Escape to collapse an expanded card or close the snooze picker.
14. The nav badge's `aria-label` states the full phrase (e.g., `"5 items stuck"`), never
    just the bare numeral, matching `UnfinishedNavBadge`/`ReviewQueueNavBadge`
    convention.
15. Filter chips use `aria-pressed` and are wrapped in a `role="group"` with an
    `aria-label` (e.g., `"Filter stuck items by reason"`).
16. Cards use `role="button"`, `aria-expanded`, and keyboard Enter/Space/Escape,
    verbatim-matching `UnfinishedItem.tsx`'s existing implementation.
17. The section's live count summary uses `aria-live="polite"` for routine background
    updates (poll-driven count changes); `role="alert"` is never used for a routine
    reconciliation update — reserved only for something the existing
    `NotificationToast` already treats as actionable/urgent.
18. Every stuck-reason chip pairs a color class with a visible text label (e.g., "PR
    ready to merge", not just a green dot) — verified by checking that removing all
    color (grayscale) still leaves the state fully legible from text alone.
19. Color contrast for all new chip text/background pairs is ≥ 4.5:1 (WCAG AA),
    consistent with the existing Axe Core CI gate on `web-app/src/` changes.
20. Long item titles truncate with a `title=` tooltip rather than breaking card layout
    or pushing the reason chip/duration off-screen at any supported viewport width.

### Touch, cross-reference, actionability, and first-load clarity

21. **Snooze is reachable on touch / no-hover pointers.** On a device without a
    hover-capable pointer (`hover: none` / `pointer: coarse`), the Snooze action is
    exposed via a persistently-visible affordance (kebab/overflow icon-button or an
    always-shown small icon-button) with a tap target of **≥ 44×44px** — it is never
    gated solely behind hover (which never fires on touch). On hover-capable pointers
    the hover-reveal remains as progressive enhancement (Surfaces 7, 10).
22. **Same item across multiple reason groups is cross-referenced.** When one `item_id`
    appears as a card in 2+ reason groups within the current view (multi-reason
    fan-out), each such card shows a small **"also stuck for N other reason(s)"**
    indicator so the user does not miscount items or assume acting on one card resolves
    the item entirely. The indicator is computed client-side from the fetched list and
    is suppressed when a filter narrows the item to a single visible card (Surface 2).
23. **`pr_ready_unmerged` states its one action explicitly.** The `pr_ready_unmerged`
    detail view shows an explicit line — **"This PR is ready — merge it on GitHub when
    you're ready."** — alongside the PR link, so the available next step is as explicit
    as the `pr_status_unknown` detail's "no action available" messaging, rather than
    only implied by the auto-merge-disabled note + link (Surfaces 8, 9).
24. **First-load nav badge never shows a misleading zero.** Before the first successful
    fetch completes with no prior last-known count cached, the `StuckNavBadge` shows a
    neutral loading affordance (no count / subtle pulse-skeleton, or nothing) — never a
    stale "0" or a confident empty state that could be misread as "confirmed zero stuck
    items." It settles to the real count or the genuine hidden-at-zero state only after
    the first response resolves (Surface 1).
25. **`pr_status_unknown` states "no action available" as literal on-screen copy.** The
    `pr_status_unknown` detail view renders the literal string **"Couldn't check this
    PR's status — no action available."** (not merely a muted chip that implies it), so
    the absence of a next step is explicit UI text of the same concreteness as AC 23's
    `pr_ready_unmerged` line (Surface 8).

### Responsive / narrow-viewport layout

26. **Filter-chip row wraps, never overflows.** At narrow (phone) viewport widths the
    6-chip reason-filter row wraps onto multiple lines (`display: flex; flexWrap: "wrap"`,
    matching `UnfinishedTab.css.ts` lines 19 & 88) — it is never squeezed into an
    unreadable single line, clipped by `overflow: hidden`, or allowed to push a chip
    off-screen. Each chip retains its full text + count label when wrapped (never
    icon-only). Horizontal scroll of the row is an acceptable alternative but wrap is the
    default (Surface 2).
27. **Card glance-line content stacks/truncates gracefully.** At narrow widths the card
    glance line uses the existing `UnfinishedItem.css.ts` convention: header is
    `display: flex; flexWrap: "wrap"` so pieces wrap rather than overflow; the reason chip
    and duration are `flexShrink: 0` fixed anchors that never clip; the item title is the
    single flexible element that truncates (`flexGrow: 1; overflow: hidden;
    textOverflow: "ellipsis"; whiteSpace: "nowrap"` + `title=` tooltip). The card body
    never scrolls horizontally and the duration is never pushed off-screen (Surface 7).

### Additional accessibility

28. **Reason-group dividers are semantic headings.** Each reason-group divider (e.g. "PR
    ready to merge (1)", "Abandoned review (2)") is a real heading element — an `<h3>` (or
    `role="heading" aria-level="3"`) — not a styled `<div>`/`<hr>` divider, so a
    screen-reader user can navigate group-to-group with heading navigation rather than
    tabbing through every card. The group heading text includes the reason label and its
    count (Surface 2).
29. **Focus returns to the card's own toggle on collapse.** When an expanded card is
    collapsed via Escape (or a re-click that collapses it), keyboard focus returns to that
    same card's toggle control (`role="button"` header) — it is never dropped to `<body>`
    or lost — consistent with standard disclosure-widget accessibility patterns and
    `UnfinishedItem.tsx`'s Escape handling (Surfaces 7, 9).
30. **Multiple cards may be expanded at once (non-exclusive accordion).** Expanding one
    card never auto-collapses another; each card owns an independent `aria-expanded` state
    and there is no single "currently open" index. This supports side-by-side comparison of
    two stuck items during triage (Surface 2, interaction flow step 3).

---

## Traceability to requirements.md and plan.md

| Requirement / decision | Surface(s) |
|---|---|
| 4 stuck-reason classes visible with correct reason | 2, 7 (chip table) |
| Duration-since-stuck visible at glance | 2, 7 |
| PR-ready-unmerged detail (mergeability, link) | 9 |
| Work-session count vs. cap | 7 (glance), 9 (rework_cap detail) |
| Review-cycle count | 7 (glance), 9 (bouncing detail) |
| `allow_auto_merge` read-only, detail-only, best-effort | 9 |
| Reconciler GitHub-check failure → distinct reason class | 8 |
| Durable "since when" (restart-survives) | 7, 9 (`first_detected_at` sourced) |
| Snooze = visibility control, not remediation | 10 |
| No one-click remediation actions | Confirmed absent from all surfaces — no "retry
push"/"re-trigger review"/"merge now" controls anywhere in this design |
| Restart-durability (UI implication) | 6, 11 — duration/timestamps always sourced from
persisted fields, never process-uptime |
