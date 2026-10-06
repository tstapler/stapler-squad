# UX Design: Kanban Board View

**Date**: 2026-08-06
**Phase**: SDD Phase 3 (design artifact, pre-implementation)
**Inputs**: [`requirements.md`](../requirements.md), [`research/ux.md`](../research/ux.md), [`implementation/plan.md`](../implementation/plan.md), [ADR-002](../decisions/ADR-002-drag-to-complete-uses-archive-not-delete.md), [ADR-003](../decisions/ADR-003-board-column-resolution-and-precedence.md)

This document specifies the twelve user-facing surfaces of the board view, the interaction
flow through each, what the user sees when each fails, and 34 human-testable UX acceptance
criteria. It also records four defects and one contradiction found in the plan while
designing against it, and an explicit verdict on the column-order question.

Interaction decisions already locked in by the plan are treated as fixed and are **not**
re-litigated here: two-stage delivery (read-only board + "Move to…" first, drag second),
`PointerSensor`-only drag with the menu as the universal path, pessimistic commit,
`COLUMN_RENDER_CAP = 50` with an overflow footer, and archive-not-delete on Complete.

---

## 0. Findings that change the plan

Five items surfaced while designing against the plan. Each is stated with its evidence and
the specific plan story it lands in. They are ordered by severity.

### F-1 (BLOCKER) — `#bulk-feedback-live` does not exist while the board is mounted

**VERIFIED.** The live region the plan routes every board announcement through is rendered
inside `SessionList`'s own JSX:

`/home/tstapler/Programming/stapler-squad/web-app/src/components/sessions/SessionList.tsx:1167`
```tsx
<div id="bulk-feedback-live" role="status" aria-live="polite" aria-atomic="true" aria-label="Action feedback" style={{ position: "absolute", width: 1, ... }}>{bulkFeedback ?? ""}</div>
```

`rg 'aria-live' --include='*.tsx'` over `web-app/src` confirms this is the only node with
that id, and it has no render site outside `SessionList`.

Story 2.2.1's own acceptance criterion establishes that `SessionListPaneBody` renders
`<SessionList>` **or** `<SessionBoard>` by ternary — in board mode `SessionList` is
unmounted. Therefore, in board mode:

- `document.getElementById("bulk-feedback-live")` returns `null`, so Task 2.3.3a's primary
  path is dead.
- Task 2.3.3a's fallback — "else through `showFeedback` from `useSessionSelection`" — does
  not rescue it. `showFeedback` sets state; the *rendering* of that state is the same
  unmounted node. The announcement is written to a variable nobody displays.
- **Every move announcement on the board is silently dropped**, which is the exact failure
  mode Story 2.3.3 exists to prevent, and the same class of silent-failure the plan
  structurally guards against everywhere else via `BoardMoveOutcome`.
- Story 2.3.3's first acceptance criterion (`document.getElementById("bulk-feedback-live")`
  has `textContent === "Moved 'fix-login-bug' to Paused"`) will either fail, or pass only in
  a test harness that happens to mount `SessionList` alongside the board — a green test
  asserting a broken production behaviour.

**Required fix (this design assumes it):** hoist the two persistent live regions
(`#bulk-feedback-live`, `#empty-state-live`) out of `SessionList` and into
`SessionListPaneBody`, **outside** the list/board ternary, and pass the feedback string down.
This is strictly better than the status quo for a second reason: a node that survives the
*view switch itself* is what lets the toggle announce "Board view, 4 columns" (UX-AC-03) and
what keeps a move announcement audible if a live push re-renders the board mid-announcement
— the precise property the comment at `SessionList.tsx:1166` says the node exists to provide.

Lands in: **Story 2.2.1** (hoist) as a prerequisite of **Story 2.3.3**.

### F-2 (MAJOR) — The Complete column's header count and its overflow footer contradict each other

Story 2.1.2 AC-3 specifies that 63 sessions in Complete render 50 cards plus a footer reading
`"+13 more — switch to list view"`. Story 3.2.1 AC-2 specifies the header count is "over
rendered cards so it never contradicts what the user can see."

Applied together those give `Complete (50)` with `+13 more` beneath it — the header
under-reports by exactly the cap. That inverts Success Metric #1, whose entire content is
"identify… how many sessions are in each" without scrolling.

**Resolution specified here:** the header count is **post-filter column membership** (63),
never the render-capped count. The overflow footer reports `membership − COLUMN_RENDER_CAP`.
Story 3.2.1's "over rendered cards" wording is about *filtered vs. total* (search must change
the count) and should be restated as "over the filtered membership, not the unfiltered
session list" so it stops colliding with the cap. See UX-AC-08 and UX-AC-09.

Lands in: **Story 2.1.2** and **Story 3.2.1**.

### F-3 (MAJOR) — Archive-to-Complete has an undo available and the plan doesn't use it

ADR-002's central safety argument is reversibility: "`archiveSession` has a documented
inverse, `unarchiveSession`… A drag gesture is far easier to trigger accidentally than a
click-through-a-confirm-modal, so the action it performs must be undoable."

That argument is currently satisfied only at the API layer. At the interaction layer, Story
2.3.3's Complete-move toast offers a **"Show archived"** action — which helps the user *find*
the session, not *reverse* the action. Reversing it still costs: enable Show Archived →
locate the card → open its overflow menu → unarchive → disable Show Archived.

The primitive already exists and is already used elsewhere in this component tree:

`/home/tstapler/Programming/stapler-squad/web-app/src/lib/contexts/NotificationContext.tsx:69`
```ts
showUndoToast: (message: string, onUndo: () => void, durationMs?: number) => string;
```

`SessionList.tsx:367` already destructures `showUndoToast` from `useNotifications()`, and
Task 1.2.2c is already widening `SessionServiceContextValue` with `unarchiveSession`. The
undo is therefore ~5 lines and closes ADR-002's own argument at the layer where the user
lives.

**Specified here:** the Complete-move toast is an undo toast with a 10 s window (longer than
the 5 s default because the card has *vanished*, so the user needs longer to notice), whose
`onUndo` calls `unarchiveSession(id)`. "Show archived" becomes a secondary affordance on the
same toast, not the primary one. See S10 and UX-AC-24/25.

Lands in: **Story 2.3.3** (Task 2.3.3b).

### F-4 (MAJOR) — The board has no mobile layout, and horizontal scroll defeats its own premise

The plan's only mobile provisions are "drag disabled on coarse pointers" and "the menu is the
universal move path." Neither addresses *layout*. `SessionBoard.css.ts` (Task 2.1.2a) is
specified as a horizontal flex with `overflowX: "auto"` and a fixed column `minWidth`.

At the repo's established 768px breakpoint (`SessionList.css.ts` uses `(max-width: 768px)`
throughout) and below, a 4-column board with any usable column width shows **one column at a
time**. The feature's entire premise and its first Success Metric — "identify, *without
scrolling*, how many sessions are in each of Running / Needs Review / Paused / Complete" — is
unmet on mobile by construction. Per the standing mobile+desktop UX requirement, this is a
gap, not an acceptable v1 cut.

**Specified here (S11):** a sticky **column summary strip** above the lanes on `< 768px` — the
four column labels and counts in one always-visible row, each tapping to scroll-snap its lane
into view. This preserves the at-a-glance metric on mobile at the cost of one small
component, and doubles as wayfinding for the horizontal scroll (Nielsen #1). No new
interaction model, no vertical-stack redesign, no touch drag.

Lands in: **new story under Epic 2.1**, sized alongside Story 2.1.2.

### F-5 (MINOR) — The overflow footer drops the user's query on exit

`"+13 more — switch to list view"` switches the pane to list mode. The user's goal at that
moment is "see the rest of *this column*," and the list they land on is not scoped to that
column's membership — they must re-derive the query by hand. That is an exit path that
technically escapes the state while discarding the user's context.

Because the board and list share one `useSessionFilters` instance-by-key, the footer can set
the matching filter on its way out at near-zero cost: `paused` → status filter Paused;
`running` → status filter Active; `complete` → `showArchived: true`. `needsReview` has no
exact list-side equivalent — `filterNeedsApproval` keys off
`SubStatus.NEEDS_APPROVAL || INPUT_REQUIRED` (Story 1.1.1 AC-2) while `needsReviewIds` is
`reviewQueue ∪ approvals` (ADR-003 §1), so it is an approximation. Apply it anyway and label
the footer honestly for that one column (`"+N more — show in list"`), rather than either
silently applying a different filter or leaving all four columns context-less.

Lands in: **Story 2.1.2** (Task 2.1.2b).

---

## 1. Column order verdict

> **Question (plan.md Unresolved Questions #1):** ADR-003 §3 orders columns
> Needs Review → Running → Paused → Complete. `requirements.md:38` lists
> Running → Needs Review → Paused → Complete. Confirm or veto.

### Verdict: **Endorse ADR-003's order. Do not block Story 2.1.2 on it.**

Independent reasoning, not a restatement of the ADR's own case:

**1. The consistency-with-Trello objection does not survive inspection, and it is the only
serious objection.** The strongest argument against ADR-003 is Nielsen #4 (Consistency and
standards) and Krug's "use conventional patterns": every reference tool named in
`requirements.md:58` (Trello, Vibe-Kanban, Agent-Kanban, Dorothy) and every tool in
`research/ux.md` §1 (Linear, GitHub Projects) orders columns as a **temporal pipeline** —
work flows rightward toward Done. Under a pipeline reading, "Needs Review" occurs *after*
"Running," so putting it leftmost is backwards.

That objection fails because **the requirements' own order is not a pipeline either.**
`Running → Needs Review → Paused → Complete` places Paused — a suspension state orthogonal to
progress, reachable from and returnable to any point — between review and completion. No
session traverses those four columns left to right. Neither candidate order is a pipeline, so
neither can claim the convention. Once the pipeline framing is off the table, both orders are
attention orderings, and the tiebreaker is legitimately "which ordering best serves the
scan" — which ADR-003 wins on the F-pattern grounds it argues.

**2. Two arguments in ADR-003's favour that ADR-003 does not make.**

- *Narrow-viewport guarantee.* The board scrolls horizontally (`overflowX: "auto"`). The
  leftmost column is the **only** column guaranteed visible without scrolling on a narrow
  pane, a split pane, or a phone. Whatever occupies position 1 is the thing the user is
  guaranteed to see. That should be the column that can block them, not the one that by
  definition needs no action.
- *Fitts's Law on the dominant drag.* Under Phase 3, the legal moves are Running→Paused,
  Running→Complete, Paused→Running, and →Complete. Running→Paused is the most common. In
  ADR-003's order the layout is `[NR][Running][Paused][Complete]`, so Running is **adjacent**
  to Paused and one hop from Complete. In the requirements' order,
  `[Running][NR][Paused][Complete]`, Running is two columns from Paused with the
  never-a-drop-target Needs Review column sitting between them as an obstacle. ADR-003's
  order is measurably better drag ergonomics for the dominant gesture.

**3. One real cost, with a mandatory mitigation.** The common, good case is zero sessions
needing review. ADR-003 puts a frequently-empty column in the highest-value screen position,
so the first thing the eye lands on is often an empty box. Left as a neutral blank, that
inverts the ordering's benefit.

Mitigated, it becomes the feature's best moment: an **affirmative all-clear state** in the
leftmost column is a persistent, instantly-readable "nothing needs you," which is precisely
the emotional job `research/ux.md` §5 identifies (anxiety about autonomous agents blocking
unnoticed) and a direct application of Nielsen #1. This is a design *requirement* of adopting
the order, not a nicety — see S3 and UX-AC-06.

**4. Process recommendation: unblock Story 2.1.2.** The plan gates Story 2.1.2 on this
question while simultaneously recording that "*Cost of reversal: reorder the `BOARD_COLUMNS`
array… No other file changes.*" Blocking the critical path (`1.1.3 → 1.2.1 → 2.1.2 → …`) on a
preference question whose reversal is a one-line array edit is disproportionate. Build on
ADR-003's order, surface it in the first review, and reorder the array if vetoed.

Restated recommendation for plan.md:

```
- [ ] Column order per ADR-003 §3 (Needs Review first). Endorsed on independent UX grounds in
      design/ux.md §1. Confirm before merge — does NOT block Story 2.1.2.
      Cost of reversal: reorder BOARD_COLUMNS in web-app/src/lib/board/boardColumns.ts.
```

---

## 2. Surface inventory

| # | Surface | Stage | Section |
|---|---------|-------|---------|
| S1 | List/Board toggle + `b` shortcut | 2 | §3.1 |
| S2 | Board shell, 4 columns, populated | 2 | §3.2 |
| S3 | Column empty state (incl. all-clear) | 2 | §3.3 |
| S4 | Column overflow beyond `COLUMN_RENDER_CAP` | 2 | §3.4 |
| S5 | `SessionBoardCard` anatomy + busy state | 2 | §3.5 |
| S6 | Drag interaction + drop confirmation | 3 | §3.6 |
| S7 | "Move to…" menu (keyboard / touch path) | 2 | §3.7 |
| S8 | Swimlane-axis switcher + read-only mode | 3 | §3.8 |
| S9 | Failure & rejection toasts | 2 | §3.9 |
| S10 | Archive-and-disappear on Complete | 2 | §3.10 |
| S11 | Mobile board layout (< 768px) | 2 | §3.11 |
| S12 | Screen-reader announcement surface | 2 | §3.12 |

---

## 3. Surfaces

### 3.1 S1 — List/Board toggle and the `b` shortcut

Per Task 2.2.1c the toggle sits in the same screen position in both modes (via `SessionList`'s
existing `extraHeaderActions` prop in list mode, and in `SessionBoard`'s own header in board
mode) — so the control does not move under the user when it is used. That is the single most
important property of this surface.

```
LIST MODE                                                    BOARD MODE
┌──────────────────────────────────────────┐   ┌──────────────────────────────────────────┐
│ Sessions (18 of 24)                      │   │ Sessions (18 of 24)                      │
│                    ┌────────┬─────────┐  │   │                    ┌────────┬─────────┐  │
│                    │▣ List  │ ▢ Board │  │   │                    │▢ List  │ ▣ Board │  │
│                    └────────┴─────────┘  │   │                    └────────┴─────────┘  │
│                      ↑ same x-position, both modes ↑                                     │
├──────────────────────────────────────────┤   ├──────────────────────────────────────────┤
│ ▸ fix-login-bug        [Running]         │   │ ┌──────────┐┌──────────┐┌────────┐┌─────┐│
│ ▸ add-metrics          [Paused]          │   │ │Needs Rev.││ Running  ││ Paused ││Compl││
│ ▸ refactor-auth        [Needs Review]    │   │ └──────────┘└──────────┘└────────┘└─────┘│
└──────────────────────────────────────────┘   └──────────────────────────────────────────┘

  role="radiogroup" aria-label="Session view mode"
  ├─ role="radio" aria-checked="true"  "List view"   title="List view (b)"
  └─ role="radio" aria-checked="false" "Board view"  title="Board view (b)"
```

**Flow**

1. User clicks "Board view", or presses `b` with the pane focused and no text input focused.
2. System writes `"board"` to `pane-<id>.stapler-squad-dashboard-view-mode`, unmounts
   `SessionList`, mounts `SessionBoard`.
3. System announces `"Board view — 4 columns"` through the hoisted live region (F-1). The
   announcement is what makes the `b` shortcut safe: a keyboard user who hits `b` by accident
   is told what happened rather than having their whole view silently replaced.
4. Search text, filters, sort, and selection survive the switch, because both views call
   `useSessionFilters` / `useSessionSelection` with the same `storageKeyPrefix`.

**Errors / edge cases**

| Case | Behaviour |
|---|---|
| `localStorage` unavailable / throws | Fall through to `"list"` for this session. No error surface — the persisted preference is a convenience, not the user's data. Matches existing `loadFromStorage` behaviour. |
| Persisted value is not `"list"`/`"board"` (hand-edited, older build) | Treated as `"list"`. Never renders an empty pane. |
| `b` pressed with focus in the search input | Character is typed. View unchanged. Guard is the existing `INPUT`/`TEXTAREA`/`isContentEditable` shape at `SessionList.tsx:791-792`. |
| `Cmd+B` / `Ctrl+B` | Inert — browser shortcut proceeds. |
| Pane too narrow to fit the toggle's text labels | Labels collapse to icons; `aria-label` and `title` retain the full text. Never remove the accessible name. |

**Discoverability note (Krug).** The `b` binding is invisible unless it is on the control that
performs the same action. `title="Board view (b)"` on both radios is the whole cost of making
it discoverable — `rg -n 'key === "b"'` returned no existing binding, so there is no
collision.

---

### 3.2 S2 — Board shell, four columns, populated

```
┌──────────────────────────────────────────────────────────────────────────────────────────┐
│ Sessions (18 of 24)   [🔍 login              ✕]  Lane by: [Status ▾]   ▢ List  ▣ Board    │
├──────────────────────────────────────────────────────────────────────────────────────────┤
│ ┌───────────────────┐┌───────────────────┐┌───────────────────┐┌───────────────────┐     │
│ │ Needs Review (3)  ││ Running (9)       ││ Paused (4)        ││ Complete (2)      │  →  │
│ │ ═══════════════   ││ ───────────────   ││ ───────────────   ││ ───────────────   │     │
│ ├───────────────────┤├───────────────────┤├───────────────────┤├───────────────────┤     │
│ │┌─────────────────┐││┌─────────────────┐││┌─────────────────┐││┌─────────────────┐│     │
│ ││⠿ fix-login-bug  ││││⠿ add-metrics    ││││⠿ refactor-auth  ││││  bump-deps      ││     │
│ ││  ◆ Approval  ⋯  ││││  ● Processing ⋯ ││││  ⏸ Paused    ⋯  ││││  ■ Stopped   ⋯  ││     │
│ │└─────────────────┘││└─────────────────┘││└─────────────────┘││└─────────────────┘│     │
│ │┌─────────────────┐││┌─────────────────┐││┌─────────────────┐││┌─────────────────┐│     │
│ ││⠿ tests-failing  ││││⠿ docs-pass      ││││⠿ old-spike      ││││  archived-thing ││     │
│ ││  ▲ Tests fail⋯  ││││  ● Processing ⋯ ││││  ⏸ Hibernated⋯  ││││  ▣ Archived  ⋯  ││     │
│ │└─────────────────┘││└─────────────────┘││└─────────────────┘││└─────────────────┘│     │
│ │┌─────────────────┐││        ⋮          ││        ⋮          ││                   │     │
│ ││⠿ ci-escalation  │││                   ││                   ││                   │     │
│ ││  ◆ Approval  ⋯  │││ (scrolls inside)  ││                   ││                   │     │
│ │└─────────────────┘││                   ││                   ││                   │     │
│ └───────────────────┘└───────────────────┘└───────────────────┘└───────────────────┘     │
└──────────────────────────────────────────────────────────────────────────────────────────┘
   ⠿ = grip handle (fine pointer only)     ⋯ = "Move to…" control (all pointers)
   Header is OUTSIDE the scrolling element — count stays visible while its column scrolls.
```

**Structure**

- Board root: horizontal flex, `overflowX: auto`, `data-testid="session-board"`.
- Column: vertical flex, fixed `minWidth`. Header `flexShrink: 0`; body `flex: 1` +
  `overflowY: auto`, `role="list"`, `data-testid="board-column-body-<key>"`.
- Header: `data-testid="board-column-header-<key>"`, accessible text `"<Label> (<count>)"`.
- The Needs Review header carries a heavier rule (`═══`) than the other three — a non-colour
  cue that it is the attention column, satisfying WCAG 1.4.1 without relying on the status
  hues that `research/ux.md` §3 flags as too close in some themes.

**Flow**

1. User switches to board. All four lanes render from one `useMemo` over
   `sortedSessions × needsReviewIds`.
2. Header counts answer Success Metric #1 with no scrolling and no interaction.
3. A live `WatchSessions` / `WatchReviewQueue` push that changes a card's resolved column
   fades it out of the origin lane (200 ms) and flashes it into the destination (250 ms) —
   so a peer's or an agent's change is *noticed*, not silently swapped.

**Errors / edge cases**

| Case | Behaviour |
|---|---|
| A session's status is a value no rule enumerates | Lands in Running via `resolveBoardColumn`'s catch-all rule 4. **Never invisible** — this is the BUG-037 class the totality requirement exists to prevent. |
| Sum of the four counts ≠ filtered session count | Structurally impossible; asserted end-to-end by Story 2.1.3 AC-1 and by UX-AC-05. |
| `prefers-reduced-motion: reduce` | Both durations collapse to 0 ms; the card simply appears in its new lane. |
| Rapid pause→resume→pause (flap) | Flap-protection pass from `BacklogBoard.tsx:168-284` suppresses the thrash; the card settles once. |
| All four columns empty (no sessions at all) | Four empty states, not a single global blank. A global "no sessions" message would remove the columns the user just switched to see. |

---

### 3.3 S3 — Column empty state, and the all-clear case

Two distinct empty states, deliberately not the same component. Making them identical is the
mistake that turns ADR-003's column order from an asset into a liability (§1, point 3).

```
NEEDS REVIEW — EMPTY (the good case)        ANY OTHER COLUMN — EMPTY (neutral)
┌───────────────────┐                       ┌───────────────────┐
│ Needs Review (0)  │                       │ Paused (0)        │
│ ═══════════════   │                       │ ───────────────   │
├───────────────────┤                       ├───────────────────┤
│                   │                       │                   │
│        ✓          │                       │        ⏸          │
│                   │                       │                   │
│    All clear      │                       │ No paused sessions│
│  Nothing needs    │                       │                   │
│   your review     │                       │                   │
│                   │                       │                   │
│  ·  ·  ·  ·  ·    │  ← dashed drop-zone   │  ·  ·  ·  ·  ·    │
└───────────────────┘     outline, min-height└───────────────────┘
   Full column width preserved. Never collapses to zero.
```

**Rules**

1. An empty column keeps the **same width** as a populated one (Story 2.1.2 AC-2). A
   collapsed column is a smaller drag target than a full one, which is exactly backwards.
2. An empty column keeps a **minimum body height** with a dashed drop-zone outline, so it
   reads as "a place things can go," not "a broken box." The same minimum applies to a column
   holding a single card, so one card does not leave a void that reads as empty
   (`research/ux.md` §4).
3. Needs Review empty renders the **affirmative** variant: a check glyph and "All clear —
   nothing needs your review." Every other column renders the neutral variant with its
   `emptyLabel` from `BOARD_COLUMNS`.
4. Empty ≠ loading. On first paint before sessions arrive, columns show skeleton cards
   (`BacklogBoard.tsx`'s `SkeletonCard` is the model), never the empty state. Showing "All
   clear" while data is still in flight is an actively false statement about system status.

**Errors / edge cases**

| Case | Behaviour |
|---|---|
| Sessions loaded but every column empty because of an active search | Each column shows its empty state **plus** the board header shows `"18 of 24"` → `"0 of 24"`, and the existing `#empty-state-live` region announces "No sessions found". The user must be able to tell "nothing matches my filter" from "nothing exists." |
| Data fetch fails outright | Columns render the existing connection/error affordance used by the list view, not four empty states. Four "All clear"/"No X" panes during an outage is a false all-clear. |

---

### 3.4 S4 — Column overflow beyond `COLUMN_RENDER_CAP`

```
┌───────────────────┐
│ Complete (63)     │  ← header count = FULL post-filter membership (F-2)
│ ───────────────   │
├───────────────────┤
│┌─────────────────┐│
││  bump-deps      ││
│└─────────────────┘│
│        ⋮          │   50 cards rendered
│┌─────────────────┐│
││  old-migration  ││
│└─────────────────┘│
├───────────────────┤
│ +13 more —        │  ← data-testid="board-column-overflow-complete"
│ show in list  →   │     button; carries the column's filter with it (F-5)
└───────────────────┘
```

**Flow**

1. Column membership exceeds 50. The first 50 render (sorted by the shared `sortedSessions`
   order, so the cap is deterministic and matches what the list would show first).
2. Footer replaces the remainder: `"+<membership − 50> more — show in list"`.
3. Activating the footer switches the pane to list mode **and applies the column's
   corresponding filter**, so the user lands on the same set rather than an unfiltered list.

| Column | Filter applied on exit |
|---|---|
| `running` | status filter → Active |
| `paused` | status filter → Paused |
| `complete` | `showArchived: true` |
| `needsReview` | `filterNeedsApproval: true` — an **approximation**; see F-5 |

**Errors / edge cases**

| Case | Behaviour |
|---|---|
| Membership is exactly 50 | 50 cards, **no** footer. `"+0 more"` must never render. |
| Membership drops back below 50 while the board is open | Footer disappears; header count updates. No animation on the footer itself. |
| A hidden (beyond-cap) session changes column | It simply appears/disappears from the counts. No enter/exit animation fires for a card that was never rendered — an animation for an invisible card is a wasted timer and a phantom `data-entering` node. |
| User drags a card into a column already at 50 | Move proceeds normally; the card may land beyond the cap and be represented only in the count and footer. The success announcement therefore says where it went **and** that it is beyond the visible set: `"Moved 'x' to Complete — beyond the 50 shown"`. Silently having a successful move produce no visible change is a dead end (UX-AC-10). |

---

### 3.5 S5 — `SessionBoardCard` anatomy and busy state

Deliberately **not** `SessionCard`. The board card carries five things; everything else is one
click away in the session itself.

```
DEFAULT (fine pointer, hover)              BUSY (move in flight)
┌─────────────────────────────────┐        ┌─────────────────────────────────┐
│ ⠿  fix-login-bug            ⋯   │        │    fix-login-bug                │
│    ▸ claude   ◆ Approval        │        │    ▸ claude   ⟳ Pausing…        │
└─────────────────────────────────┘        └─────────────────────────────────┘
  │  │            │             │            aria-busy="true"
  │  │            │             └ "Move to…" control (always present)          
  │  │            └ ONE attention badge max (review reason OR error)
  │  └ program indicator + status/sub-status chip (reuses StatusBadge/SubStatusChip)
  └ grip handle: opacity 0 → 1 on hover/focus-within; cursor: grab
     (absent entirely on coarse pointers and on a grouping axis)

SELECT MODE                                DRAG SOURCE (while dragging)
┌─────────────────────────────────┐        ┌─────────────────────────────────┐
│ ☑ ⠿ fix-login-bug           ⋯   │        │ ░░ fix-login-bug            ░   │  40% opacity
│      ▸ claude   ◆ Approval      │        │ ░░ ▸ claude   ◆ Approval        │  data-drag-source
└─────────────────────────────────┘        └─────────────────────────────────┘
```

**Rules**

- **Uniform height.** Titles clamp at two lines (`WebkitLineClamp: 2`). Two cards with wildly
  different title lengths report the same `offsetHeight` (Story 2.1.1 AC-2). Ragged card
  heights destroy the column-scanning the board exists for.
- **Click targets are unambiguous.** Card body → open session. Grip → drag only. Move control
  → menu only. Checkbox (select mode) → selection only. `research/ux.md` §1's whole-card-drag
  option is rejected precisely because this card has four distinct targets.
- **Busy state is explicit, not just dimmed.** `aria-busy="true"`, `data-pending="<verb>"`,
  **and a visible verb** (`⟳ Pausing…`) replacing the status chip. This matters more under the
  pessimistic model than it would under an optimistic one: the card does not move, so without
  a verb the user's only feedback for the entire round-trip is a slight opacity change — which
  reads as "my drag failed," the opposite of the truth (Nielsen #1).
- While busy, the card is non-interactive: grip, move control, and body click are all inert.
  A second move cannot be queued on top of an in-flight one.

**Errors / edge cases**

| Case | Behaviour |
|---|---|
| Session has both a review reason and an error sub-status | Show **one** badge, highest-severity first (error > tests-failing > approval > input-required). Two badges at card width is the density regression the condensed card exists to avoid. |
| Title is empty or whitespace | Fall back to the session id, never render a card with no accessible name. |
| Move in flight when the session is deleted by a peer | Card unmounts; the in-flight outcome still resolves and still announces (`"session-gone"`), because the announcement target is the board, not the card. |

---

### 3.6 S6 — Drag interaction and drop confirmation (Stage 3)

Drag is a **shortcut for the S7 menu path**, not a second code path: same `BoardMoveIntent`,
same `useBoardMove`, same outcomes. Everything below is about making the consequence legible
*before* the drop commits — the mitigation `research/ux.md` §2 calls for against the
Trello-metaphor misread.

```
STEP 1 — press grip, move 8px           STEP 2 — hover a LEGAL target
┌──────────┐┌──────────┐┌────────┐      ┌──────────┐┌──────────┐┌──────────────────┐
│Needs Rev.││ Running  ││ Paused │      │Needs Rev.││ Running  ││ Drop to pause    │ ← header
│   (3)    ││   (9)    ││  (4)   │      │   (3)    ││   (9)    ││ session          │   text
├──────────┤├──────────┤├────────┤      ├──────────┤├──────────┤├━━━━━━━━━━━━━━━━━━┤   changes
│          ││░░░░░░░░░░││        │      │          ││░░░░░░░░░░││                  │
│          ││░ ghost  ░││        │      │          ││░ source ░││   ┌────────────┐ │
│          ││░ 40%    ░││        │      │          ││░  40%   ░││   │ ▓▓▓▓▓▓▓▓▓▓ │ │ ← solid
└──────────┘└──────────┘└────────┘      └──────────┘└──────────┘└───│ ▓ insert ▓ │─┘   border
        ┌──────────────┐                                            │ ▓▓▓▓▓▓▓▓▓▓ │     + tint
        │▓ fix-login  ▓│ ← DragOverlay, portaled to document.body   └────────────┘
        │▓ ◆ Approval ▓│    zIndex.dragOverlay = 1090
        └──────────────┘

STEP 3a — hover an ILLEGAL target        STEP 3b — drop on a legal target
┌ ─ ─ ─ ─ ─ ─ ─ ─ ─┐                     card shows ⟳ Pausing… in its ORIGIN column
│ Can't move here  │ ← header text       (pessimistic — it does not move yet)
├ ─ ─ ─ ─ ─ ─ ─ ─ ─┤   + DASHED border   RPC resolves → card animates to Paused
│      ⃠            │   + not-allowed     announcement + focus follow (S12)
└ ─ ─ ─ ─ ─ ─ ─ ─ ─┘   cursor
   Three simultaneous non-colour cues: text, border style, cursor (WCAG 1.4.1)
```

**Flow**

1. Press on the grip. Nothing happens until 8 px of movement (`activationConstraint`), so a
   press-and-release is still a click.
2. `onDragStart` snapshots the card's resolved source column and sets `data-drag-source` on
   the origin card (40 % opacity — the user can see *where it came from*, which is what makes
   an accidental drag recoverable by simply dropping outside).
3. Overlay follows the pointer, portaled to `document.body` at `zIndex.dragOverlay = 1090`.
4. On drag-over, **every** column computes its own legality via `resolveMoveVerb` — one rule,
   shared with the menu. Legal columns state the verb in their header; illegal columns state
   "Can't move here" and switch to a dashed border.
5. On drop over a legal column: source column is re-validated against a fresh
   `resolveBoardColumn`; on match, `move(intent)` fires and the card enters its busy state
   **in its origin column**.
6. On RPC success, the store update moves the card, the enter/exit animation plays, the move
   is announced, and focus follows.

**Errors / edge cases**

| Case | User sees |
|---|---|
| Drop on an illegal column | Nothing fires. Card returns to normal opacity. No toast — the rejection was already communicated continuously during the hover, and a toast for a refusal the user was warned about three ways is noise. |
| Drop outside any column | Silent cancel. This is the escape hatch for a misread drag (Nielsen #3, user control and freedom) and must never commit anything. |
| `Escape` during drag | Same as drop-outside: silent cancel, focus returns to the grip. |
| Session changed column mid-drag (live push) | Drop is rejected with `"stale-source-column"` → `"That session moved before the drop landed. Nothing changed."` Announced + toast. The card is **not** moved. |
| Session deleted mid-drag | Rejected `"session-gone"`, no RPC. |
| RPC fails | Card leaves busy state in its origin column; failure toast with one Retry (S9). |
| Native pane-swap drag hijacks the gesture (split panes) | Must not occur — `draggable={false}` on the board card root if Task 3.1.3c confirms the collision. Symptom to watch for: the pane layout changes instead of the card moving. |
| Drag started, then the axis is switched to a grouping strategy | Drag cancels; the board is read-only on that axis. |

---

### 3.7 S7 — "Move to…" menu (keyboard and touch path)

This ships **first** (Stage 2) and is the only move path on touch. It is not a fallback; drag
is the enhancement layered on top of it.

```
CLOSED (running card)                    OPEN — menu portaled to document.body
┌─────────────────────────────────┐      ┌─────────────────────────────────┐
│ ⠿  fix-login-bug            [⋯] │      │ ⠿  fix-login-bug            [⋯] │
│    ▸ claude   ● Processing      │      │    ▸ claude   ● Processing      │
└─────────────────────────────────┘      └─────────────────────────────────┘
   aria-haspopup="menu"                    ┌───────────────────────────┐
   aria-label="Move fix-login-bug"         │ Move to…                  │
                                           ├───────────────────────────┤
DISABLED (complete card)                   │ ⏸  Paused    — pause      │
┌─────────────────────────────────┐        │ ▣  Complete  — archive    │
│    bump-deps                [⋯] │        └───────────────────────────┘
│    ■ Stopped                    │          Only LEGAL targets listed.
└─────────────────────────────────┘          Each item names the VERB, so the
   aria-disabled="true"                      consequence is stated before commit
   title="No moves available from Complete"  — same information the drag header
                                             gives, in the non-drag path.
```

**Rules**

- Menu lists **only** targets for which `resolveMoveVerb` is non-`null`. Needs Review is never
  listed (server-derived); the current column is never listed (no-op); Complete offers
  nothing.
- Each item shows **column name + verb** (`"Paused — pause"`, `"Complete — archive"`). This
  is the menu's equivalent of the drag header's "Drop to archive session," and it is what
  keeps ADR-002's semantics visible to a user who never drags.
- When there are no targets the control is **disabled, not removed** (Nielsen #6, recognition
  over recall — a control that disappears makes the user wonder whether they misremembered).
  `title` states why.
- Portaled to `document.body`, or the column's `overflowY: auto` clips it.
- Keyboard: `Enter`/`Space` opens and focuses item 1; `↑`/`↓` cycle; `Enter` commits; `Esc`
  closes and returns focus to the control; `Tab` closes.
- Touch: control is ≥ 44 × 44 px and always visible (not hover-revealed) on coarse pointers.

**Errors / edge cases**

| Case | Behaviour |
|---|---|
| Session's legal targets change while the menu is open (live push) | Menu re-renders its items. If the chosen item is no longer legal at activation time, `useBoardMove` returns `"illegal-transition"` and it is announced — the menu never fires a stale RPC. |
| Menu opened on a card that is already busy | Control is inert while `aria-busy="true"`. |
| Menu opened on a grouping axis | `aria-disabled="true"`, `title="Moves are only available on the Status axis"` — see S8. |
| Board scrolled while the menu is open | Menu closes rather than detaching from its card. |

---

### 3.8 S8 — Swimlane-axis switcher and read-only mode

ADR-003 §4 makes the board read-only on any non-Status axis. The design risk is a **mode
error** (Nielsen #4): the user switched the axis to see their fleet grouped by tag, and their
grip handles silently vanished.

```
STATUS AXIS (default — movable)
┌──────────────────────────────────────────────────────────────────────────┐
│ Lane by: [ Status ▾ ]                                                    │
├──────────────────────────────────────────────────────────────────────────┤
│ ┌──────────────┐┌──────────────┐┌──────────────┐┌──────────────┐         │
│ │Needs Rev. (3)││ Running (9)  ││ Paused (4)   ││ Complete (2) │         │
│ │┌────────────┐││┌────────────┐││              ││              │         │
│ ││⠿ fix-login ││││⠿ add-metric││              ││              │         │
│ │└────────────┘││└────────────┘││              ││              │         │
└──────────────────────────────────────────────────────────────────────────┘

GROUPING AXIS (read-only)
┌──────────────────────────────────────────────────────────────────────────┐
│ Lane by: [ Tag ▾ ]   🔒 Read-only — moves are available on the Status axis│
├──────────────────────────────────────────────────────────────────────────┤
│ ┌──────────────┐┌──────────────┐┌──────────────┐                         │
│ │ backend (4)  ││ frontend (2) ││ urgent (3)   │                         │
│ │┌────────────┐││┌────────────┐││┌────────────┐│                         │
│ ││  fix-login │││  new-ui     │││  fix-login  ││ ← same card, two lanes: │
│ ││  ● Proc [⋯]│││  ⏸ Pause[⋯]│││  ● Proc [⋯] ││   why moves can't work  │
│ │└────────────┘││└────────────┘││└────────────┘│                         │
│  no grip handles; every [⋯] is aria-disabled                             │
└──────────────────────────────────────────────────────────────────────────┘
```

**Flow**

1. User selects a `GroupingStrategy` from the axis selector.
2. Board re-lanes via `groupSessions(sortedSessions, strategy)` — unchanged, shared with the
   list.
3. The read-only hint appears **immediately adjacent to the selector that caused it** — not
   in a corner — and the change is announced once:
   `"Tag lanes — read-only, moves are available on the Status axis."`
4. Grip handles are not rendered. Move controls remain **present and disabled** with
   `title="Moves are only available on the Status axis"`, so the reason is available at the
   point of the attempted action, not only in a banner the user has already scrolled past.
5. Selecting "Status" restores the four columns, the grips (on a fine pointer), and the move
   controls.

**Errors / edge cases**

| Case | Behaviour |
|---|---|
| Strategy produces zero lanes (e.g. Tag with no tagged sessions) | One "No tags to group by — switch axis or add tags" state, with the axis selector still reachable. Never a blank board with no way back. |
| Strategy produces very many lanes (Tag, Branch, Path) | Board scrolls horizontally as normal; `COLUMN_RENDER_CAP` applies per lane. The sticky mobile strip (S11) also applies. |
| Board axis and list grouping strategy diverge | Intended. They persist under distinct keys (`…board-swimlane-axis` vs `…grouping-strategy`) so switching views never silently re-groups the other. |
| User drags anyway (muscle memory, fine pointer) | No grip exists, so no drag starts. Nothing to announce — the affordance's absence plus the adjacent hint is the message. |

---

### 3.9 S9 — Failure and rejection toasts

Four rejection reasons and one failure kind. Every one produces a toast **and** an
announcement; `BoardMoveOutcome` being an exhaustive union is what makes "nothing happened and
the user wasn't told" unrepresentable.

```
FAILED (RPC returned an error)                REJECTED (pre-flight, no RPC fired)
┌──────────────────────────────────────┐      ┌──────────────────────────────────────┐
│ ⚠  Couldn't move 'fix-login-bug':    │      │ ℹ  That session moved before the drop│
│    session already paused            │      │    landed. Nothing changed.          │
│                                      │      │                                  [✕] │
│                       [ Retry ]  [✕] │      └──────────────────────────────────────┘
└──────────────────────────────────────┘        No Retry — retrying a stale-state
  Retry re-issues the SAME intent ONCE.         rejection would repeat the same
  Never auto-retries.                           refusal. The exit is the dismiss.
```

| Outcome | Toast copy | Action | Log |
|---|---|---|---|
| `rejected: illegal-transition` | "Can't move 'X' to Complete → Running." | dismiss | `console.warn` |
| `rejected: stale-source-column` | "That session moved before the drop landed. Nothing changed." | dismiss | `console.warn` |
| `rejected: session-gone` | "'X' no longer exists." | dismiss | `console.warn` |
| `rejected: readonly-axis` | "Moves are only available on the Status axis." | "Switch to Status" | `console.warn` |
| `failed: <msg>` | "Couldn't move 'X': `<msg>`" | **Retry** (once) | `console.error` |

**Rules**

- The failure toast surfaces the **RPC's own message**, not a generic one. "session already
  paused" tells the user their board was stale; "couldn't move session" tells them nothing
  (Nielsen #9 — plain language, specific diagnosis).
- Exactly one Retry, never automatic. Pause/resume/archive are not idempotent from the user's
  point of view and a silent retry loop on a stale board is how a user ends up with a state
  they did not ask for.
- `readonly-axis` is the one rejection with a constructive action, because it has one: the
  user's goal is achievable, just on another axis.
- Toasts dedupe by session id via `showActionToast`'s `key` semantics, so a user retrying the
  same card three times gets one toast, not a stack.

**Errors / edge cases**

| Case | Behaviour |
|---|---|
| Retry also fails | A second toast with the new message. Retry is offered again — but the user has now seen two explicit failures, which is information, not a loop. |
| Session vanishes between failure and Retry | Retry resolves to `"session-gone"` and says so. Retry never fires an RPC for a session that is not there. |
| Ten failures at once (bulk action against a dead server) | Toasts dedupe by key; the live region announces a summary rather than ten sequential messages, which would take ~40 s to read aloud. |

---

### 3.10 S10 — Archive-and-disappear on a Complete move

The sharpest edge in the feature. With `showArchived` off — the default — a card moved to
Complete **leaves the board entirely** (ADR-002 Consequences). A successful action whose only
visible effect is that something disappeared is indistinguishable from a bug.

```
BEFORE                                    AFTER (showArchived = OFF)
┌──────────┐┌──────────┐                  ┌──────────┐┌──────────┐
│ Running  ││ Complete │                  │ Running  ││ Complete │
│   (9)    ││   (2)    │                  │   (8)    ││   (2)    │ ← count did NOT go up
├──────────┤├──────────┤                  ├──────────┤├──────────┤
│ fix-login││ bump-deps│   drag/menu →    │ add-metr ││ bump-deps│
│ add-metr ││ old-mig  │                  │ docs-pass││ old-mig  │
└──────────┘└──────────┘                  ├──────────┤└──────────┘
                                          │ 1 archived session      │ ← optional footer
                                          │ hidden — Show archived  │   (recommended)
                                          └─────────────────────────┘
┌───────────────────────────────────────────────────────────┐
│ ▣ Archived 'fix-login-bug' — hidden by the Archived filter │  10s window
│                          [ Undo ]  [ Show archived ]  [✕]  │
└───────────────────────────────────────────────────────────┘
        ↑ F-3: Undo calls unarchiveSession — the reversal ADR-002's
          safety argument depends on, made reachable in one click.
```

**Flow**

1. User drags to Complete, or picks "Complete — archive" from the menu.
2. Pre-commit, the consequence is already stated: the drag header reads "Drop to archive
   session"; the menu item reads "Complete — archive". The word *archive* — not "complete",
   not "done" — appears before the user commits. This is the ADR-002 semantics reaching the
   user.
3. `archiveSession` resolves; the card leaves the filtered set and unmounts.
4. **Undo toast** (10 s, longer than the 5 s default because the object of the action is no
   longer on screen): `"Archived 'fix-login-bug' — hidden by the Archived filter"` with
   **Undo** (primary, `unarchiveSession`) and **Show archived** (secondary).
5. Announcement carries the same text (S12).
6. Focus does **not** go to `document.body` — see S12 and UX-AC-30.

**Open design question for the requirements author (sharpened)**

The plan asks "is disappearing acceptable, or should `showArchived` auto-enable?" The sharper
framing: **a Done column that hides done things is self-defeating.** With `showArchived` off,
the Complete column contains only sessions that are `STOPPED` but not archived — so the
column the user just dropped into can never show the result of that drop. Three options:

| Option | Trade-off |
|---|---|
| **(a)** As specified + Undo toast + a `"N archived hidden — Show archived"` column footer | Cheapest. Preserves the shared filter exactly (`requirements.md:18`). The footer makes the hidden state *visible* without diverging behaviour. **Recommended minimum.** |
| **(b)** Complete column exempts itself from `showArchived` — it *is* the archive view | Best kanban semantics; the drop always has a visible result. Cost: board and list legitimately disagree on that column's contents, which a reviewer will read as the divergence `requirements.md:18` prohibits. Needs an explicit ruling. |
| **(c)** Auto-enable `showArchived` after a Complete move | Rejected. A move silently changing a *shared, persisted filter* that also governs the list view is a side effect the user did not ask for and will not connect to their drag. |

This design assumes **(a)**. (b) is the better end state if the author is willing to rule that
the Complete column is definitionally the archive view.

**Errors / edge cases**

| Case | Behaviour |
|---|---|
| `showArchived` is **on** | Card lands visibly in Complete; the standard "Moved 'X' to Complete" announcement applies and the undo toast is still offered (the gesture is still easy to trigger accidentally). |
| Undo pressed after the session was deleted by a peer | `unarchiveSession` fails; the failure toast states why. Undo never silently no-ops. |
| Undo window expires | The session is still recoverable through Show archived → unarchive. The undo is a convenience, not the only path — no dead end. |
| Card was beyond `COLUMN_RENDER_CAP` in Complete | Announcement adds "— beyond the 50 shown" (UX-AC-10). |

---

### 3.11 S11 — Mobile board layout (< 768px)

New surface; addresses F-4. No touch drag, no new interaction model — one component that
restores the at-a-glance metric on a viewport too narrow to show four lanes.

```
┌─────────────────────────────┐
│ Sessions (18 of 24)     [⋯] │
│ [🔍 login              ✕]   │
│ ▢ List        ▣ Board       │
├─────────────────────────────┤
│ ┌───┬───┬───┬───┐           │ ← STICKY column summary strip.
│ │ ◆ │ ● │ ⏸ │ ▣ │           │   Always visible; horizontal scroll
│ │ 3 │ 9 │ 4 │ 2 │           │   of the lanes never hides it.
│ │NR │Run│Pau│Cmp│           │   Tap → scroll-snap that lane in.
│ └━━━┴───┴───┴───┘           │   Active lane underlined (━).
├─────────────────────────────┤
│ ┌─────────────────────────┐ │
│ │ Needs Review (3)        │ │ ← one lane at a time, scroll-snapped
│ ├─────────────────────────┤ │
│ │┌───────────────────────┐│ │
│ ││ fix-login-bug     [⋯] ││ │ ← no grip handle (coarse pointer)
│ ││ ▸ claude  ◆ Approval  ││ │    [⋯] is ≥44×44px and always visible
│ │└───────────────────────┘│ │
│ │┌───────────────────────┐│ │
│ ││ tests-failing     [⋯] ││ │
│ │└───────────────────────┘│ │
│ └─────────────────────────┘ │
│         ← swipe →           │
└─────────────────────────────┘
```

**Rules**

1. The summary strip is the mobile answer to Success Metric #1 — all four counts, no
   scrolling, at all times. Without it the metric is unmet below 768px.
2. Lanes scroll-snap so a swipe lands on a lane boundary, never half-way between two.
3. Strip buttons are real buttons: `role="tab"`-style semantics, `aria-label="Needs Review, 3
   sessions"`, ≥ 44 × 44 px.
4. No grip handles at all (coarse pointer). The `[⋯]` control is always visible — not
   hover-revealed, since there is no hover — and meets the 44 px target.
5. The strip is **additive**: on ≥ 768px it is not rendered, and the desktop board is
   unchanged.

**Errors / edge cases**

| Case | Behaviour |
|---|---|
| Grouping axis with 12 lanes on mobile | Strip scrolls horizontally itself; the active lane is always scrolled into view within the strip. |
| Landscape phone / small tablet between 480–768px | Two lanes may fit; the strip still renders and still snaps. It is a wayfinding aid at any width where all lanes do not fit. |
| Software keyboard open (search focused) | Strip stays sticky below the header; lanes shrink. Counts never scroll away — that is the one thing that must survive. |

---

### 3.12 S12 — Screen-reader announcement surface

Depends on F-1 being fixed: the live region must be hoisted to `SessionListPaneBody`, outside
the list/board ternary, or none of this reaches a user.

```
<SessionListPaneBody>                       ← live regions live HERE, not in SessionList
  ├── <SessionViewToggle mode="board" />
  ├── {mode === "board" ? <SessionBoard/> : <SessionList/>}   ← this unmounts on toggle
  ├── <div id="bulk-feedback-live" role="status" aria-live="polite" aria-atomic="true">
  └── <div id="empty-state-live"   role="status" aria-live="polite" aria-atomic="true">
        ↑ survive the view switch, the column re-render, and the card unmount
```

**Announcement catalogue** — every one of these is a state change a sighted user perceives
visually and a screen-reader user would otherwise not perceive at all:

| Event | Announcement |
|---|---|
| View toggled to board | "Board view — 4 columns" |
| View toggled to list | "List view" |
| Move applied | "Moved 'fix-login-bug' to Paused" |
| Move applied, beyond cap | "Moved 'fix-login-bug' to Complete — beyond the 50 shown" |
| Move applied, archived + hidden | "Archived 'fix-login-bug' — hidden by the Archived filter" |
| Move rejected | The `MOVE_REJECTION_MESSAGE` for that reason |
| Move failed | "Couldn't move 'fix-login-bug': session already paused" |
| Axis switched to grouping | "Tag lanes — read-only, moves are available on the Status axis" |
| Axis switched to status | "Status columns — moves available" |
| Column counts change from a live push | *Not announced.* Announcing every peer/agent-driven count change would make the region unusable. Visual transition only. |

**Focus management** — the rule the plan's Story 2.3.3 states, plus the case it misses:

| Outcome | Focus lands on |
|---|---|
| Applied, card still rendered | The moved card **in its destination column** |
| Applied, card unmounted (archived + hidden, or beyond cap) | The **origin column's header** — *this case is not covered by Story 2.3.3c and would send focus to `document.body`*, the exact regression its own criterion forbids |
| Rejected / failed | The card in its **origin** column |
| Drag cancelled (`Escape`, drop outside) | The **grip handle** the drag started from |

**Errors / edge cases**

| Case | Behaviour |
|---|---|
| Two announcements within ~1 s | The region is `aria-atomic="true"`; the later replaces the earlier. Announcements are written as complete standalone sentences so a replaced one loses nothing structural. |
| Board unmounts mid-announcement (user hits `b`) | Region survives — that is precisely why it is hoisted. |
| A second live region introduced for the board | **Prohibited.** Two `aria-live` regions racing is a known screen-reader failure mode; `research/ux.md` §0 and the plan's Pattern Decisions both rule against it. |

---

## 4. UX acceptance criteria

Each is testable by a human with a browser, a keyboard, and (where noted) a screen reader.
IDs are stable for reference from `implementation/validation.md` and e2e specs.

### View toggle and navigation

| ID | Criterion |
|---|---|
| **UX-AC-01** | A user in list view reaches the board in **1 step** (one click on "Board view", or one `b` keypress) and back in 1 step. |
| **UX-AC-02** | The toggle control occupies the **same screen position** in both modes — its bounding box does not move when used. |
| **UX-AC-03** | Toggling views announces the new mode through the live region ("Board view — 4 columns" / "List view"). Verified with a screen reader, or by reading `#bulk-feedback-live`'s `textContent` after the toggle. |
| **UX-AC-04** | Both toggle buttons expose the keyboard shortcut in their `title` (`"Board view (b)"`), so `b` is discoverable without documentation. |

### Board layout and counts

| ID | Criterion |
|---|---|
| **UX-AC-05** | With N sessions passing the current filters, the four column header counts **sum to exactly N**. No session appears twice; none is missing. |
| **UX-AC-06** | With zero sessions needing review, the Needs Review column shows an **affirmative all-clear state** ("All clear — nothing needs your review"), visually distinct from the neutral empty state of the other three columns. |
| **UX-AC-07** | An empty column keeps the **same width** as a populated one and shows a dashed drop-zone outline with a minimum height. It never collapses, and never looks identical to a loading or broken column. |
| **UX-AC-08** | With 63 sessions in Complete, the header reads **`Complete (63)`** — the full post-filter membership, never the render-capped 50. |
| **UX-AC-09** | The same column renders 50 cards and an overflow footer reading `+13 more — show in list`. With exactly 50 in the column, **no footer renders** (never `+0 more`). |
| **UX-AC-10** | Moving a card into a column that is already at the cap still announces where it went **and** that it is beyond the visible set. A successful move never produces zero perceptible feedback. |
| **UX-AC-11** | Activating an overflow footer switches to list view **with the matching filter already applied**, so the user lands on the same set rather than an unfiltered list. |
| **UX-AC-12** | Scrolling inside any column keeps that column's header (label + count) visible. |
| **UX-AC-13** | While sessions are still loading, columns show skeleton cards — **never** the empty or all-clear state. |

### Cards

| ID | Criterion |
|---|---|
| **UX-AC-14** | Two cards whose titles differ in length by 60+ characters render at the **same height**. |
| **UX-AC-15** | Clicking the card body opens the session; clicking the grip, the `[⋯]` control, or the select checkbox does **not**. Each of the four targets does exactly one thing. |
| **UX-AC-16** | A card with a move in flight shows a **visible verb** ("Pausing…"), carries `aria-busy="true"`, and is fully non-interactive. Opacity change alone is not sufficient. |
| **UX-AC-17** | A card never shows more than **one** attention badge, even when the session has both a review reason and an error sub-status. |

### Move path — menu (keyboard and touch)

| ID | Criterion |
|---|---|
| **UX-AC-18** | Any legal move is completable in **≤ 3 keyboard steps** from a focused card: `Enter` (open) → `↓`×n (select) → `Enter` (commit). No pointer required at any point. |
| **UX-AC-19** | The menu lists **only** legal targets, and each item names its verb ("Complete — archive"), so the consequence is stated before commit. |
| **UX-AC-20** | A card with no legal targets shows the move control **disabled with an explanatory `title`**, never removed. |
| **UX-AC-21** | The menu opens without being clipped by its column's scroll container (portaled to `document.body`), verified by opening it on the last card of a scrolled column. |
| **UX-AC-22** | On a touch device the move control is **always visible** (not hover-revealed) and measures ≥ 44 × 44 px. |

### Move outcomes — errors and dead ends

| ID | Criterion |
|---|---|
| **UX-AC-23** | A failed move shows a toast with **the RPC's own message** ("Couldn't move 'fix-login-bug': session already paused") and exactly one **Retry** action. It never auto-retries. |
| **UX-AC-24** | A move to Complete shows an **Undo** action that restores the session (`unarchiveSession`), within a ≥ 10 s window. |
| **UX-AC-25** | With `showArchived` off, a Complete move announces the disappearance explicitly ("Archived 'X' — hidden by the Archived filter"). The card never vanishes silently. |
| **UX-AC-26** | **No dead ends.** Every one of the five outcome states — `illegal-transition`, `stale-source-column`, `session-gone`, `readonly-axis`, `failed` — presents at least one exit: a Retry, a constructive action, or a dismiss that returns the user to a working board with focus intact. Verified by triggering each of the five. |
| **UX-AC-27** | No move outcome is silent. Every one produces both a toast and a live-region announcement. |

### Drag (Stage 3)

| ID | Criterion |
|---|---|
| **UX-AC-28** | Hovering a **legal** drop target changes that column's header to the verb ("Drop to archive session") **before** the drop commits. |
| **UX-AC-29** | Hovering an **illegal** target conveys rejection through **at least two non-colour cues** — header text ("Can't move here") and border style (dashed vs. solid) — satisfying WCAG 1.4.1. |
| **UX-AC-30** | Dropping outside any column, or pressing `Escape` mid-drag, cancels silently with **no state change**, and focus returns to the grip handle. |
| **UX-AC-31** | Starting a card drag in a split-pane layout **never swaps panes**. |

### Accessibility

| ID | Criterion |
|---|---|
| **UX-AC-32** | The live region is present in the DOM **while the board is mounted** — `document.getElementById("bulk-feedback-live")` is non-null in board mode with `SessionList` unmounted. (This is F-1; it fails against the plan as currently written.) |
| **UX-AC-33** | Focus is **never** left on `document.body` after any move outcome — including the archived-and-hidden case, where it lands on the origin column's header. Verified by tabbing immediately after each of: applied-visible, applied-hidden, rejected, failed. |
| **UX-AC-34** | All board text and its status/attention chips meet **≥ 4.5:1** contrast against their backgrounds in **all six themes** (`light`, `dark`, `matrix`, `cyberpunk77`, `wh40k`, `clean` — `web-app/src/styles/theme.css.ts`), and no state (drop-legality, selection, busy, attention) is conveyed by colour alone. The theme file already carries three inline records of previously-shipped AA failures at `theme.css.ts:277`, `:389`, `:501`, so per-theme verification is required rather than assumed. |

---

## 5. Traceability

| Requirement / decision | Surfaces | Criteria |
|---|---|---|
| Success Metric #1 — counts at a glance, no scrolling | S2, S3, S4, S11 | UX-AC-05, 08, 09, 12 |
| Success Metric #2 — no divergence from list | S1, S4 | UX-AC-11 |
| Success Metric #3 — view persists per workspace | S1 | UX-AC-01, 02 |
| `requirements.md:37` — toggle + `b` shortcut | S1 | UX-AC-01, 04 |
| `requirements.md:52` — touch rabbit hole | S7, S11 | UX-AC-18, 22 |
| `requirements.md:54` — drag failure/rollback UX | S9 | UX-AC-23, 26, 27 |
| ADR-002 — archive not delete | S10 | UX-AC-24, 25 |
| ADR-003 §2 — total column resolution | S2 | UX-AC-05 |
| ADR-003 §3 — column order | §1 verdict, S2, S3 | UX-AC-06 |
| ADR-003 §4 — read-only grouping axis | S8 | UX-AC-20, 26 |
| `research/ux.md` §3 — WCAG keyboard operability | S7, S12 | UX-AC-18, 32, 33 |
| `research/ux.md` §3 — colour is not the only signal | S6, S2 | UX-AC-29, 34 |
| Mobile + desktop UX requirement | S11 | UX-AC-22, and S11's rules |
