# UX Design: Backlog Event-Driven Updates

Phase 3 design artifact, `backlog-event-driven-updates`. Extends `research/ux.md`
(Phase 2) into concrete wireframes, interaction flows, and testable acceptance
criteria. Grounded in the actual plan (`implementation/plan.md` Phase 5/6 epics,
Domain Glossary) and current component code — not generic mockups.

Do not re-derive the research; every design decision below cites the research
recommendation it implements. If a decision here appears to contradict
`research/ux.md`, the research wins — flag it, don't silently diverge.

---

## Surfaces designed (7)

1. `/backlog` list rows
2. `/backlog/board` Kanban cards
3. `BacklogItemDetail` side panel
4. `BacklogItemPanel` (inside `SessionDetail`)
5. Connection-state indicator (`ConnectionIndicator`, Epic 6.2)
6. Edit-mode-buffered-update banner (`InlineNotice`, Epic 5.3.2 / 6.4)
7. Filtered-list exit transition (Epic 6.3)

---

## 1. `/backlog` list rows

### Current state
Fetches once on mount (`page.tsx`), renders a static list of `BacklogItemCard`.
No re-fetch, no live update, no connection awareness.

### Wireframe — BEFORE (fetch-once, stale until manual nav)

```
┌─ /backlog ────────────────────────────────────────────────┐
│ Filter: [ in_progress ▾ ]                                 │
│                                                             │
│ ┌─────────────────────────────────────────────────────┐   │
│ │ Fix retry loop in triage runner            P2        │   │
│ │ 3/5 done                                [View Session]│  │
│ └─────────────────────────────────────────────────────┘   │
│ ┌─────────────────────────────────────────────────────┐   │
│ │ Add dark-mode token to theme.css.ts         P3        │  │
│ │ 2/2 done                                [View Session]│  │
│ └─────────────────────────────────────────────────────┘   │
│                                                             │
│  (item silently moved to "review" 4 minutes ago server-    │
│   side — nothing on screen reflects it. No indication      │
│   this list could even be stale.)                          │
└─────────────────────────────────────────────────────────────┘
```

### Wireframe — AFTER (live, connection-aware)

```
┌─ /backlog ─────────────────────────────────────  ● Live ───┐
│ Filter: [ in_progress ▾ ]                                  │
│                                                             │
│ ┌═════════════════════════════════════════════════════┐   │  ← .justChanged flash
│ │▓Fix retry loop in triage runner            P2       ▓│   │    (background tint,
│ │▓3/5 done            ✗ FAIL          [View Review]   ▓│   │     ~250ms, fading)
│ └═════════════════════════════════════════════════════┘   │
│ ┌─────────────────────────────────────────────────────┐   │
│ │ Add dark-mode token to theme.css.ts         P3       │   │
│ │ 2/2 done                                [View Session]│  │
│ └─────────────────────────────────────────────────────┘   │
└──────────────────────────────────────────────────────────────┘
```

### Interaction flow (automatic, no user action)
1. `useWatchBacklogItems()` streams `BacklogItemEvent`s; page subscribes via a
   selector over `backlogItemsSlice`, unfiltered (all items visible to this
   operator), then filters client-side same as today.
2. A `BacklogItemStatusChangedEvent` arrives for `"item-1"` (`in_progress` →
   `review`), carrying the full updated `BacklogItem`.
3. `upsertBacklogItem` reducer applies it in place, keyed by `id` — no list
   remount, no `key` churn.
4. `BacklogItemCard` for `"item-1"` re-renders with new `status`/action-button
   label/verdict badge (already-existing conditional render, per research §2)
   **and** gets the `.justChanged` class for ~250ms (Epic 6.1), then it clears.
5. Because the item still matches the `in_progress` filter's *old* value only
   until this event — if the new status still passes the active filter, it
   just updates in place (this row). If it now fails the filter (e.g. filter
   is literally `status: in_progress`), see Surface 7 (exit transition).
6. If `is_snapshot: true` (i.e. this data arrived as part of initial
   connect/reconnect catch-up, not a live event), **no flash** — only genuine
   real-time deltas flash (Domain Glossary: `is_snapshot` "drives whether this
   flash/highlights on the frontend").

### Error / edge cases
- **Stream disconnect**: `ConnectionIndicator` (Surface 5) shows
  "Reconnecting…"; list keeps last-known state, does not blank or spinner-out.
- **Reconnect**: hook forces a full re-fetch/resnapshot of currently-filtered
  items (research §4) rather than resuming from "now," reconciling any gap.
- **Extended disconnect** (past threshold): degrade to 5s poll fallback
  (research §4) — `ConnectionIndicator` should distinguish this
  ("Reconnecting…" vs. a longer-outage state) so the user isn't told "Live"
  while actually polling. See Surface 5 for the exact states.
- **Out-of-order events**: `upsertBacklogItem` applies last-write-wins by
  `seq`/`updatedAt`, never by arrival order (research §4, Domain Glossary
  `upsertBacklogItem`) — a stale event that arrives late must not visually
  flicker the row backward.

---

## 2. `/backlog/board` Kanban cards

### Current state
`BacklogBoard.tsx:105` — `items.filter((i) => i.status === column.status)`,
purely derived from a `items` prop fetched once by the parent page. Same card
component (`BacklogItemCard`) as the list.

### Wireframe — column-to-column live move

```
BEFORE:                                    AFTER (event arrives):
┌─ Ready ──┐┌─ In Progress ─┐┌─ Review ─┐  ┌─ Ready ──┐┌─ In Progress ─┐┌─ Review ─┐
│          ││ ┌───────────┐ ││          │  │          ││               ││┌────────┐│
│          ││ │Fix retry  │ ││          │  │          ││               │││Fix retry││ ← re-
│          ││ │loop  P2   │ ││          │  │          ││               │││loop  P2 ││   appears
│          ││ └───────────┘ ││          │  │          ││ (fades out    │││ ✗FAIL  ▓││   here,
│          ││               ││          │  │          ││  ~200ms, then ││└────────┘│   flashed
│          ││               ││          │  │          ││  removed)     ││          │
└──────────┘└───────────────┘└──────────┘  └──────────┘└───────────────┘└──────────┘
```

### Interaction flow
1. Board subscribes to the *same* `useWatchBacklogItems` stream/slice as the
   list (Domain Glossary: "consumed identically by all 4 views") — no
   board-specific fetch.
2. A status-changed event moves the item's column membership purely via the
   `i.status === column.status` filter re-evaluating on the updated item —
   this is structurally identical to Surface 7's "filtered-list exit
   transition," just with a same-page re-entry into a sibling column instead
   of leaving the page entirely.
3. Origin column: card plays the exit transition (Epic 6.3) — fade/slide,
   ~200ms — instead of instantly vanishing (Trello "ghost placeholder" model,
   research §1/§4), so it reads as "moved to Review" not "deleted."
4. Destination column: card mounts already carrying its `.justChanged` flash
   (Epic 6.1) so the arrival is visually tied to the same event as the
   departure — same event, same visual "pulse" vocabulary on both ends.
5. `ConnectionIndicator` shown once per board (not per column) — a single
   ambient affordance, not N redundant ones (research §5, "prove it's
   trustworthy without duplicating chrome").

### Error / edge cases
- Same disconnect/reconnect/fallback-poll behavior as Surface 1 (shared hook).
- **Column-count mismatch during a gap**: on reconnect resnapshot, a column
  may gain/lose several cards at once (multiple transitions happened while
  disconnected) — do **not** attempt to animate every one of those as an
  individual transition; only live (`is_snapshot: false`) single-item events
  get the flash/exit treatment. A resnapshot re-render is a silent, instant
  correction (same "is_snapshot suppresses flash" rule as Surface 1).
- **Drag-and-drop interaction** (if the board supports manual drag — confirm
  during implementation): an incoming live event for a card mid-drag must not
  interrupt the drag gesture. If the board has no drag-and-drop today, this is
  moot; flag it as a check, not an assumption.

---

## 3. `BacklogItemDetail` side panel

### Current state
`BacklogItemDetail.tsx:244-249` — `shouldPoll` computed from
`triageStatus === "running" || (status === "review" && (!gateVerdict ||
gateVerdict === "PENDING")) || status === "pr_pending"`, gated by `&&
!editMode`, driving a 5s `setInterval` calling `load()`. Zero coverage while
`in_progress` — the single most anxiety-prone state per research §5.

### Wireframe — BEFORE / AFTER, viewing state (not editing)

```
BEFORE (in_progress, no polling — frozen until manual close/reopen):
┌─ Backlog Item: Fix retry loop in triage runner ──────────┐
│ Status: In Progress                                       │
│ Session: staplersquad_fix-retry-loop                      │
│                                                             │
│  (agent finishes, status flips to "review" server-side —   │
│   panel shows "In Progress" indefinitely until user closes  │
│   and reopens it)                                          │
└─────────────────────────────────────────────────────────────┘

AFTER (live, in_progress → review, verdict lands moments later):
┌─ Backlog Item: Fix retry loop in triage runner ─  ● Live ─┐
│ Status: Review                          ▓ (flash, ~250ms) │
│ Session: staplersquad_fix-retry-loop                       │
│                                                             │
│ ┌─ Gate Verdict ──────────────────────────────────────┐    │
│ │ role="status" aria-live="polite" aria-atomic="true"  │    │
│ │  ✗ FAILED                                            │    │
│ │  2 of 5 criteria did not pass                        │    │
│ │  [readOnly — no action buttons, this session ended]  │    │
│ └───────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

### Interaction flow (viewing, not editing)
1. Panel subscribes by `itemId` directly (Domain Glossary: `useWatchBacklogItems`
   scoped selector), **independent of any list/board filter state** — this is
   the decoupling research §4 calls out as "easy to get backwards." Confirmed
   in plan Story 5.3.1: *"the detail panel still shows the item's current
   (done) state — it does not disappear or freeze"* even if a list filter
   elsewhere would have dropped it.
2. `shouldPoll`/its `setInterval` (line 244-249) is deleted outright — no
   fallback timer for this panel specifically; it relies entirely on the
   stream (plus the shared connection-level poll-fallback in Surface 5 during
   extended outages).
3. Status label updates in place (text swap, same DOM node) with a brief
   background flash on the status row, mirroring the card's `.justChanged`
   treatment but scoped to the status field, not the whole panel (avoid an
   entire-panel flash that would visually "shout" for every incidental field
   update).
4. `GateVerdictBox` in `readOnly` mode is fed the live verdict directly
   (`aria-atomic="true"` added per Epic 6.2) — this is a **single discrete
   announcement point**, exactly what `aria-live="polite"` is designed for
   (research §3), as opposed to announcing every list item's change.

### Interaction flow (editing — `editMode === true`)
See Surface 6 (Edit-Mode Buffered Update Banner) for the full flow; summary
here: incoming events for the open item are **buffered**, not applied, while
`editMode` is true; a non-blocking `InlineNotice` appears; "Reload" (or
exiting edit mode) applies the buffered update.

### Error / edge cases
- **Item deleted/archived while panel open**: `BacklogItemArchivedEvent`/
  `BacklogItemRemovedEvent` arrives for the open item — panel must show a
  clear terminal state ("This item was archived/removed") rather than
  continuing to render stale action buttons that now 404 on click. Not
  explicitly covered in research — flagged here as a gap the plan should
  confirm has a handler (likely: disable action buttons, show a small
  `InlineNotice`-style banner, same visual family as the buffering banner).
- **Panel open for item id that never existed / already gone on initial
  connect**: standard fetch-fails-404 path, unchanged from today.
- **Reconnect while panel open**: resnapshot applies via the same
  `upsertBacklogItem`/staleness-guard path as Surface 1; no special panel
  logic needed beyond "don't apply mid-edit" (Surface 6).

---

## 4. `BacklogItemPanel` (inside `SessionDetail`)

### Current state
Fourth consumer, found during Phase 2 research resolution — shown inside
`SessionDetail`/`SessionDetailView` for a session's linked backlog item.
Confirm current data-fetching approach during implementation (plan Task
5.4.1a is explicitly a discovery task — path/mechanism not yet nailed down).

### Wireframe — embedded panel inside a session view

```
┌─ Session: staplersquad_fix-retry-loop ───────────────────────┐
│ [Terminal] [Diff] [Logs]                                     │
│ ┌─ Linked Backlog Item ──────────────────  ● Live ─────────┐ │
│ │ Fix retry loop in triage runner                          │ │
│ │ Status: Review              ▓(flash)                     │ │
│ │ ✗ FAILED — 2 of 5 criteria did not pass                  │ │
│ └────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────┘
```

### Interaction flow
1. Same `useWatchBacklogItems` hook, scoped to the single linked item id —
   identical subscription shape to `BacklogItemDetail` (Surface 3), just
   rendered in a smaller embedded card rather than a full side panel.
2. Status transition + verdict arriving "shortly after" (plan Story 5.4.1
   acceptance criterion) both reflect **without the session detail page
   itself reloading** — this panel's live update must not force a re-render
   of the surrounding `SessionDetail`/terminal view (a remount here would be
   far more disruptive than elsewhere, since it risks interrupting an
   actively-open terminal/log stream on the same page).
3. No connection indicator duplication needed if `SessionDetail` already has
   its own live-connection affordance for session state (`WatchSessions`) —
   confirm during implementation whether to reuse that existing indicator or
   add a second small one; do not silently ship two competing "Live" badges
   on the same page fighting for attention.

### Error / edge cases
- Same disconnect/reconnect story as Surface 3, scoped to one item.
- **This session's linked item is archived/removed while the panel is open**:
  a `BacklogItemArchivedEvent`/`BacklogItemRemovedEvent` arrives for the
  linked item — panel must disable/hide its action buttons and show a clear
  terminal state ("This item was archived/removed elsewhere") rather than
  continuing to render stale action buttons that now silently fail or 404 on
  click. This is the same handling as Surface 3's equivalent edge case
  (UX AC #13) — plan Task 5.4.1c implements it identically to Task 5.3.1c.
- **This session's linked item is unlinked (not archived/removed, just
  disassociated from the session)**: panel should collapse gracefully or show
  "No linked backlog item" rather than an empty flash of a now-nonexistent
  card — a distinct case from the above (the item itself still exists).

---

## 5. Connection-state indicator (`ConnectionIndicator`)

### Current state
Does not exist. New small component per Epic 6.2 Task 6.2.1b, mounted in
list/board/detail (Task 6.2.1c) — and per Surface 4's note, possibly reused
or coordinated with in `BacklogItemPanel`.

### Wireframe — three states

```
Connected (steady state):        Reconnecting (transient):      Degraded (extended outage):
┌───────────────┐                ┌───────────────────┐          ┌─────────────────────────┐
│ ● Live         │                │ ◐ Reconnecting…   │          │ ○ Polling (every 5s)     │
└───────────────┘                └───────────────────┘          └─────────────────────────┘
  green/success dot                 amber/pulsing dot              muted dot, explicit label
  role="status"                     role="status"                  role="status"
  aria-live="polite"                aria-live="polite"              aria-live="polite"
  "Live updates connected"          "Reconnecting to live          "Live updates unavailable —
  (visually-hidden full text)        updates…"                       refreshing every 5 seconds"
```

### Interaction flow (automatic)
1. Mounted once per view (list, board, detail — and `BacklogItemPanel` if it
   doesn't reuse an existing session-level indicator per Surface 4).
2. Reflects `useWatchBacklogItems`'s `connectionState` directly — no derived
   or guessed state; the hook is the single source of truth (Domain
   Glossary).
3. State transitions:
   - `connected` → `reconnecting`: on stream drop (network blip, server
     restart, tab wake-from-sleep). Shows immediately, no debounce needed for
     entering this state (research §5: silence reads as "safe," so a fast,
     honest "something changed" signal here is correct even though it's
     mildly more chatter than a debounced version).
   - `reconnecting` → `connected`: on successful resubscribe *and* resnapshot
     completing (not just socket-open — must include the reconciliation
     re-fetch from Surface 1's error-case flow).
   - `reconnecting` → `degraded/polling` after a defined threshold (mirrors
     research §4's "if disconnected longer than some threshold, degrade to
     the existing 5s-poll fallback" — exact threshold value is an
     implementation detail, not a UX decision, but must be user-visible via a
     distinct third state, not silently folded into "Reconnecting…" forever).
   - `degraded/polling` → `connected`: once the stream re-establishes: no
     silent flip back — momentarily show `connected`'s entry (research §5
     "prove it's accurate ambiently," not just "be accurate").

### Error / edge cases
- **Rapid connect/disconnect flapping** (e.g. flaky wifi): avoid a visibly
  flickering indicator — debounce only the `reconnecting → connected`
  transition (a few hundred ms hold), never the `connected → reconnecting`
  one (must announce trouble immediately, per above).
  - This surface itself IS the answer to "what does the user see when the
    stream disconnects" — every other surface points back here rather than
    inventing its own disconnect messaging.
- **`prefers-reduced-motion`**: the "reconnecting" pulsing dot must have a
  static (non-animated) fallback — a distinct color/icon still communicates
  state without motion.

---

## 6. Edit-mode-buffered-update banner (`InlineNotice`)

### Current state
No buffering exists today — polling is simply suspended `&& !editMode`
(`BacklogItemDetail.tsx:245`), so an incoming background change while editing
is invisible until the poll resumes post-edit and silently overwrites nothing
(because it wasn't polling). The event-driven version changes this: events
keep arriving regardless of `editMode`; they must now be explicitly buffered,
not merely ignored, because arrival is now continuous rather than
interval-gated.

Plan confirms `InlineError`'s styling is **not** reusable as-is — `InlineError`
is `role="alert" aria-live="assertive"` (confirmed by reading
`InlineError.tsx:105-106`), which is exactly the wrong politeness level for a
non-blocking, informational buffered-update notice (research §3: "assertive…
wildly inappropriate" for routine state). A new sibling `InlineNotice`
component is warranted per Task 5.3.2b's own caveat ("if `InlineError` can't
be reused via a prop/variant") — it cannot, without weakening `InlineError`'s
correct assertive behavior for actual errors.

### Wireframe — BEFORE (editing, event silently invisible today) / AFTER (buffered + banner)

```
BEFORE (today — polling just doesn't run while editing; no event system exists yet):
┌─ Edit: Fix retry loop in triage runner ───────────────────┐
│ Title: [Fix the retry loop_______________]                │
│ Description: [...editing...]                              │
│                                    [Cancel]  [Save]        │
└─────────────────────────────────────────────────────────────┘

AFTER (event-driven — description changed server-side mid-edit):
┌─ Edit: Fix retry loop in triage runner ───────────────────┐
│ Title: [Fix the retry loop_______________]  ← untouched   │
│ Description: [...user's in-progress edit, untouched...]   │
│                                                             │
│ ┌───────────────────────────────────────────────────────┐ │
│ │ ⓘ This item changed elsewhere.        [Reload]  [×]    │ │  ← InlineNotice
│ │   role="status" aria-live="polite"                     │ │     non-blocking,
│ └───────────────────────────────────────────────────────┘ │     informational
│                                    [Cancel]  [Save]        │     styling — NOT
└─────────────────────────────────────────────────────────────┘     the red/alert family
```

### Interaction flow
1. `editMode` is true; a `BacklogItemUpdatedEvent`/`StatusChangedEvent`/etc.
   arrives for the open item.
2. Reducer/hook layer still receives and stores the update (in the Redux
   slice, so it's not lost) — but the **detail component's rendering layer**
   suppresses applying it to visible form fields while `editMode` is true
   (plan Task 5.3.2a: "buffered-update state + suppress apply during
   `editMode`").
3. `InlineNotice` banner appears once (not re-triggered/re-stacked by
   subsequent buffered events while still editing — a second buffered event
   updates the *content* the banner will apply on reload, not a second
   banner instance).
4. Two ways out, both apply the buffered update and clear the banner:
   - User clicks **Reload**: form is replaced with the latest server state
     (any unsaved edits are discarded — this is the explicit, user-initiated
     trade the button names).
   - User exits edit mode (Cancel, or completes Save): buffered update
     applies automatically since `editMode` flips back to `false` and the
     suppression condition no longer holds.
5. If the user clicks **Save** instead of Reload while a buffered update is
   pending: this is a real conflict (user's edit vs. server's concurrent
   change) not explicitly resolved in research or plan — flagged below as an
   open question for implementation, not silently assumed.

### Error / edge cases
- **Multiple buffered events while editing**: only the banner's underlying
  "pending reload" data is refreshed each time (last-write-wins by
  `seq`/`updatedAt`, same rule as Surface 1); the banner itself does not
  stack or re-animate per event — one persistent notice, not N.
- **Save-while-buffered conflict** (open question, flag for plan/implementation
  to resolve explicitly, do not leave implicit): does Save silently overwrite
  the server's concurrent change, or should Save be blocked/warned when a
  buffered update exists? Recommend: warn, don't silently overwrite — surface
  the same `InlineNotice` copy adjusted to "Saving will overwrite a change
  made elsewhere — Reload first?" as a confirm-style variant, rather than
  letting the user's Save silently clobber the reconciler's concurrent write
  (this mirrors research §4's "silent data loss in either direction is the
  failure mode to design against").
- **Item deleted/archived while buffered**: the buffered "reload" target no
  longer exists — Reload should show the terminal-state handling described in
  Surface 3's edge cases, not error out with a raw 404.

---

## 7. Filtered-list exit transition

### Current state
`/backlog` and `/backlog/board` derive their visible set by filtering the
full `items` array client-side (`BacklogBoard.tsx:105`). Today this filter
only ever re-evaluates on a full re-fetch (page nav), so "item drops out of
view" doesn't yet happen live at all — this is new behavior entirely enabled
by the live stream, not a pre-existing bug being fixed.

### Wireframe — BEFORE/AFTER for a single list filtered to `in_progress`

```
BEFORE (event arrives, naive instant removal — the "did it get deleted?" problem):
┌─ Filter: in_progress ─────────┐        ┌─ Filter: in_progress ─────────┐
│ Fix retry loop in triage...   │  ───▶  │ (item just... gone)           │
│ Add dark-mode token...        │        │ Add dark-mode token...        │
└────────────────────────────────┘        └────────────────────────────────┘

AFTER (exit transition — reads as "moved," not "vanished"):
┌─ Filter: in_progress ─────────┐        ┌─ Filter: in_progress ─────────┐
│ Fix retry loop in triage...   │        │▒Fix retry loop in triage...  ▒│ ← fading out
│ ▓(status flips to review, ▓   │  ───▶  │▒  ~200ms fade+slight scale  ▒│    (~200ms)
│ ▓ flash fires first)      ▓   │        │ Add dark-mode token...        │
│ Add dark-mode token...        │        └────────────────────────────────┘
└────────────────────────────────┘        (then unmounts; item is gone from DOM)
```

### Interaction flow
1. Event arrives; `upsertBacklogItem` updates the item in the shared slice.
2. The `.justChanged` flash (Epic 6.1) fires first — same visual language as
   an in-place update — because from the item's own perspective, nothing
   about it "leaving" is special; it just changed fields.
3. The *filtered view* then re-evaluates: the item no longer matches, so
   rather than removing it from the DOM in the same tick as the flash, it
   plays a short (~200ms) fade/slide-out (Epic 6.3) and *then* unmounts.
4. This is purely a list/board-level behavior — the detail panel (Surface 3)
   and `BacklogItemPanel` (Surface 4) never apply this transition, because
   they're not filtered views; they keep showing the item regardless
   (confirmed decoupling, research §4).
5. On the Kanban board specifically (Surface 2), this exit transition is
   paired with the item's *entry* into the destination column, so the same
   event visually reads as one continuous "moved from X to Y," not two
   independent, uncorrelated animations.

### Error / edge cases
- **Reduced motion**: instant removal (no fade), consistent with the
  `prefers-reduced-motion` guard used everywhere else in this design
  (research §3).
- **Bulk resnapshot on reconnect** (Surface 1/5's degraded-then-recovered
  case): if several items simultaneously stop matching the filter because a
  resnapshot corrected multiple stale rows at once, do **not** play N
  simultaneous exit transitions — resnapshot-driven removals apply instantly
  (same `is_snapshot` suppression rule used for the flash), reserving the
  transition for genuinely live, one-at-a-time departures.
- **Item re-enters the same filtered view moments later** (e.g. flaps
  `review` → `in_progress` → `review` from a fast reconciler retry): avoid a
  double fade-out/fade-in flicker — if a pending exit transition's target
  item re-matches the filter before the transition completes, cancel the
  exit and let it settle back to a normal (flashed) in-place row rather than
  finishing the unmount and immediately remounting.

---

## UX Acceptance Criteria

Organized by surface; each is independently testable by a human without
reading source code.

### General / cross-surface
1. A status transition, verdict, or session-attach event is visible in every
   open view showing that item within the same latency envelope
   `WatchSessions` already delivers — sub-second to a few seconds — with no
   manual refresh, no full-page reload, and no scroll-position jump anywhere
   on the page.
2. No view shows two contradictory states for the same item at the same
   time — e.g. the list showing `in_progress` while the open detail panel for
   the same item shows `review` for more than the brief in-flight propagation
   window.
3. `is_snapshot` (reconnect catch-up) events never trigger a flash, exit
   transition, or screen-reader announcement — only genuinely live deltas do.
   A human tester can verify this by forcing a reconnect (kill/restore
   network) and confirming the resulting resync is visually silent (no flash
   storm across every visible card).
4. Every animated treatment (flash, exit transition, connection-indicator
   pulse) has a working, verified `prefers-reduced-motion: reduce` fallback
   that still communicates the same information via an instant state change,
   not by omitting the information.
5. No treatment relies on color alone — verdict badges, status labels, and
   the connection indicator all carry a text label a colorblind or
   screen-reader user can consume identically to a sighted user.

### `/backlog` list rows (Surface 1)
6. An item's status badge/label updates within ~2 seconds of a server-side
   transition, with a background flash that fades within ~1 second, without
   a visible page reload or scroll-position jump.
7. A row never remounts (verified via React DevTools or a stable `key`/DOM
   node identity check) on a live update — keyboard focus on an in-list
   control is preserved across an update to that same row.

### `/backlog/board` Kanban cards (Surface 2)
8. A card that changes column membership visibly plays an exit transition in
   its origin column and appears (with the "just changed" flash) in its
   destination column, both driven by the same underlying event — a human
   tester watching the board during a live transition perceives it as "moved"
   rather than "one card disappeared, an unrelated card appeared."
9. Only one connection indicator is visible per board view (not one per
   column).

### `BacklogItemDetail` side panel (Surface 3)
10. An item open in `BacklogItemDetail` with `status: in_progress` (today's
    zero-polling blind spot) updates live to `review` (or any other status)
    without any interval-based refresh — verified by confirming no
    network/poll request fires on a timer while the panel is idle and
    unedited.
11. The detail panel for an item continues showing that item's current state
    even after the item stops matching whatever list/board filter is active
    elsewhere in the app — it never blanks, freezes, or shows a stale status
    just because a filter elsewhere no longer includes it.
12. A screen reader user with `BacklogItemDetail` open hears exactly one
    announcement when the open item's status or verdict changes — not one
    per list item elsewhere on the page, and not a duplicate announcement for
    the same change.
13. An item that is archived or removed while its detail panel is open shows
    a clear terminal message (not stale, now-broken action buttons) —
    verified by archiving/removing the open item from another tab and
    confirming the panel updates to a "this item was archived/removed" state
    rather than continuing to render live action buttons.

### `BacklogItemPanel` inside `SessionDetail` (Surface 4)
14. A status transition and a verdict landing shortly after both appear in
    the embedded `BacklogItemPanel` without the surrounding `SessionDetail`
    page (terminal, logs, diff view) reloading or losing its own scroll/tab
    state.
15. If `SessionDetail` already shows a session-level "Live" indicator, the
    backlog panel does not add a second, redundant "Live" badge competing for
    attention on the same page.
16. An item whose linked `BacklogItemPanel` is open and that is archived or
    removed elsewhere shows a clear terminal message (not stale, now-broken
    action buttons) — verified by archiving/removing the linked item from
    another tab and confirming the panel updates to a "this item was
    archived/removed" state rather than continuing to render live action
    buttons, mirroring criterion #13's guarantee for `BacklogItemDetail`.

### Connection-state indicator (Surface 5)
17. The indicator shows exactly one of three distinguishable states at all
    times a live view is mounted: connected ("Live"), reconnecting
    ("Reconnecting…"), or degraded/polling — never blank, never a fourth
    ambiguous state.
18. On a forced disconnect (e.g. killing network in devtools), the indicator
    flips to "Reconnecting…" within roughly a second — it does not keep
    showing "Live" while the underlying stream is actually down.
19. During a flaky/flapping connection, the indicator does not visibly
    flicker on every brief reconnect — the `connected` state is debounced
    enough to avoid rapid on/off flashing, while the initial
    `connected → reconnecting` transition on genuine loss is never delayed.
20. Every view that mounts `useWatchBacklogItems` (list, board, detail, and
    `BacklogItemPanel` unless it reuses an existing session-level indicator)
    shows this indicator — no live view is silently missing the affordance.

### Edit-mode buffered update banner (Surface 6)
21. Editing an item's title with unsaved changes, while a live update to a
    different field (e.g. description) arrives for the same item, leaves the
    in-progress title edit completely untouched — verified by typing into
    the title field, triggering a background change from another tab, and
    confirming the typed text is unaffected.
22. The `InlineNotice` banner appears using non-error, informational styling
    (not the same red/alert visual family as `InlineError`'s
    `role="alert"` variant) and does not block interaction with the rest of
    the form (fields remain editable, Save/Cancel remain clickable) while the
    banner is visible.
23. Clicking "Reload" (or exiting edit mode via Cancel/Save) applies the
    buffered update and clears the banner in the same action — no leftover
    banner after the buffered data has been applied.
24. A second buffered event arriving while the banner is already showing
    updates what will be applied on Reload without spawning a second,
    stacked banner instance.
25. No dead end: every buffered-update state has a working "Reload" path back
    to current server state; the user is never left staring at a banner with
    no available action.
26. Clicking Save while a buffered update is pending does not silently
    overwrite the concurrent server-side change — a confirm-style variant of
    the notice ("Saving will overwrite a change made elsewhere — Reload
    first?") appears first, with explicit "Save Anyway" and "Reload" actions,
    and the save is not submitted until the user picks one.

### Filtered-list exit transition (Surface 7)
27. When an item's fields change such that it no longer matches the active
    filter, it visibly fades/slides out over roughly 200ms before being
    removed from the DOM — a human tester does not perceive it as an abrupt
    disappearance.
28. Under `prefers-reduced-motion: reduce`, the same item is removed
    instantly with no animation, but the underlying data change (status
    label, etc.) is still visible for at least one render frame before
    removal so the "why did it leave" cause is still inferable from a
    screenshot/recording if needed.
29. A bulk resnapshot after reconnect (multiple items no longer matching the
    filter at once) does not play the individual exit transition N times —
    it corrects instantly, reserving the animated transition for one-at-a-time
    live departures.
30. An item that briefly leaves and re-enters the active filter's match set
    within the transition window does not flicker (fade out then immediately
    fade back in) — the design cancels the pending exit and settles back to a
    normal in-place (flashed) row.

### Accessibility (cross-cutting, WCAG 4.1.3 AA)
31. All live-region roles use `aria-live="polite"` for routine state changes;
    `aria-live="assertive"` is never used for a routine status/verdict
    change (reserved for genuine errors, per `InlineError`'s existing usage).
32. Any live region whose text is replaced wholesale (status label, verdict
    summary) carries `aria-atomic="true"` so assistive tech reads the full
    updated phrase, not a partial diff.
33. The list/board item collection itself is never wrapped in one giant
    `aria-live` region — screen reader users do not hear a queued
    announcement per item during a burst of reconciler activity.
34. Every interactive element introduced by this feature (Reload button,
    dismiss on `InlineNotice`, connection indicator if it's ever
    interactive) is reachable and operable via keyboard alone, with a visible
    focus indicator.
35. Text/icon contrast for all new/modified elements (status flash overlay,
    connection indicator states, `InlineNotice`) meets ≥ 4.5:1 against its
    background in both light and dark themes (per `.claude/rules/css-architecture.md`
    token usage — no hardcoded colors).

---

## Open questions for implementation to resolve explicitly

These were gaps identified during this design pass; all four have since been
resolved (Product Triad Review UX fix pass, 2026-07-21) — resolutions
recorded here rather than leaving the questions open indefinitely:

1. ~~**Save-while-buffered conflict** (Surface 6): should Save silently
   overwrite a concurrent server-side change, or warn first?~~ **Resolved:
   warn, don't silently overwrite.** Implemented as plan Task 5.3.2c: Save
   while a buffered update is pending shows a confirm-style notice ("Saving
   will overwrite a change made elsewhere — Reload first?") with explicit
   "Save Anyway"/"Reload" actions; see UX AC #26.
2. ~~**Terminal state for archived/deleted-while-open** (Surfaces 3 and 4):
   no existing component/copy covers this.~~ **Resolved:** both surfaces
   disable/hide action buttons and show an `InlineNotice`-family terminal
   banner ("This item was archived/removed elsewhere") on receipt of
   `BacklogItemArchivedEvent`/`BacklogItemRemovedEvent` for the open item —
   implemented as plan Tasks 5.3.1c (`BacklogItemDetail`) and 5.4.1c
   (`BacklogItemPanel`); see UX AC #13 and #16.
3. ~~**`BacklogItemPanel`'s connection indicator**: reuse `SessionDetail`'s
   existing session-stream indicator (if one exists) vs. mount a second
   `ConnectionIndicator`.~~ **Resolved: mount its own.** Confirmed via grep
   (2026-07-21) that `SessionDetail.tsx`, `SessionDetailView.tsx`, and
   `SessionDetailBar.tsx` contain no existing session-level "Live"/connection
   indicator to reuse — `BacklogItemPanel` gets its own `ConnectionIndicator`
   like every other consumer (plan Task 6.2.1c).
4. ~~**Kanban board drag-and-drop interaction** (Surface 2): confirm whether
   `BacklogBoard` supports manual drag today.~~ **Resolved: no drag-and-drop
   exists today.** Confirmed via grep (2026-07-21) that
   `BacklogBoard.tsx` has zero drag/drop-related code — no
   interaction-precedence concern exists for this project (plan Task 5.2.1a).
   A future project adding drag-and-drop to the board must revisit this.
