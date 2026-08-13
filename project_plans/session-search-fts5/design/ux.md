# UX Design: "Find Related Past Work" in Triage

Builds directly on `research/ux.md` (placement, component-reuse, accessibility findings)
and `implementation/plan.md` Epic 2.2 (Story 2.2.1/2.2.2, `TriageRelatedWorkSection.tsx`).
This doc does not re-derive those findings — it turns them into wireframes, flows, and
testable acceptance criteria for the surfaces that actually ship in Phase 2.

**Scope confirmed against the plan**: v1 has no inline scroll-mode UI and no "load more"
pagination (fixed `limit=5`, Task 2.2.1b). The "N more matches in this session" affordance
(Task 2.2.1c) renders as **static text**, not an interactive expansion — the plan does not
wire an `onClick` for it, and `research/ux.md` Open Question 4 explicitly leaves Scroll
mode's actual surface undecided. This doc treats the whole `SessionHitCard` as the single
click target (rendered as a real `<a href target="_blank">` per §7/Task 2.2.3a, superseding
Task 2.2.1c's original `<button>` draft) and recommends a specific behavior for it in §7,
flagged as a design decision this doc is making — not one the plan had already settled.

---

## Surface inventory

1. Search input (pre-populated, editable)
2. Loading state (fresh query only)
3. Result list / `SessionHitCard`
4. Empty state (zero matches)
5. Error state (search failed)
6. Edge case: blank/whitespace item title
7. Click-through from a result card (view more context)
8. Read-only mode (section omitted)

---

## 1. Search input

```
┌─ Find related past work ──────────────────────────────────────────────┐
│                                                                        │
│  ┌──────────────────────────────────────────────────────────────┐ 🔍 │
│  │ Add dark mode toggle to settings page                         │    │
│  └──────────────────────────────────────────────────────────────┘    │
│                                                                        │
```

- On mount, the input value is pre-filled with `itemTitle` (Task 2.2.1b) — no user
  action required to get the first result set.
- No auto-focus (`research/ux.md` §4 "Focus Management" — stealing focus on a
  background-populated box is a surprise interruption; Apply/Skip remain the panel's
  focus entry points).
- Input is a plain `<input type="search">`, not a combobox — the triage variant drops
  `HistorySearchInput`'s recent-searches dropdown (`research/ux.md` §2), so there is no
  `aria-expanded`/`aria-controls` pair to wire.
- `aria-label="Search past sessions for {itemTitle}"` (falls back to `"this item"` if
  title is somehow blank — see §6).
- `data-testid="triage-related-work-input"`.
- Editing re-fires the same debounced search (300ms, `useDebounce`) with the fixed
  option bundle (`groupBySession`, `includeContext`, `excludeAutomationSessions`,
  `project: repoPath`, `limit: 5`) still attached — the user only ever changes the
  query text, never the mode flags.

**Interaction flow**

1. Panel mounts → input shows `itemTitle` → debounced auto-search fires (≤300ms).
2. User optionally edits the text → 300ms after the last keystroke, a new search fires.
3. Every keystroke-triggered search reuses the same fixed options; in-flight requests
   are aborted on each new keystroke (existing `AbortController` pattern from
   `useHistoryFullTextSearch`, reused per `research/ux.md` §2).

---

## 2. Loading state

```
┌─ Find related past work ──────────────────────────────────────────────┐
│  ┌──────────────────────────────────────────────────────────────┐ ⟳ │
│  │ Add dark mode toggle to settings page                         │    │
│  └──────────────────────────────────────────────────────────────┘    │
│                                                                        │
│   Searching…                                                         │
└────────────────────────────────────────────────────────────────────┘
```

- Spinner replaces the search icon inside the input, matching `HistorySearchInput`
  (`HistorySearchInput.tsx:154-163`).
- Full "Searching…" block shown **only** when `loading && results.length === 0`
  (Story 2.2.2, third bullet) — a fresh query blanks-and-loads, but there is no
  "load more" case in v1 (fixed `limit=5`), so this is effectively "every load."
- Text lives inside the ancestor `<section aria-live="polite">` on
  `TriageReviewPanel.tsx` (`research/ux.md` §4) — no nested `aria-live` added here.

---

## 3. Result list / `SessionHitCard`

```
┌─ Find related past work ──────────────────────────────────────────────┐
│  ┌──────────────────────────────────────────────────────────────┐ 🔍 │
│  │ Add dark mode toggle to settings page                         │    │
│  └──────────────────────────────────────────────────────────────┘    │
│                                                                        │
│  ┌────────────────────────────────────────────────────────────────┐  │
│  │ Dark mode + theme persistence               Jul 14, 2026       │  │  ← <li><a href>
│  │ +2 more matches in this session                                │  │
│  │ "...toggle stores the preference in localStorage and applies…" │  │
│  │ [reserved: outcome badge — not in v1, see plan Unresolved Qs]  │  │
│  └────────────────────────────────────────────────────────────────┘  │
│  ┌────────────────────────────────────────────────────────────────┐  │
│  │ Settings page redesign                       Feb 02, 2026      │  │
│  │ "...added a toggle row but didn't wire persistence yet…"        │  │
│  └────────────────────────────────────────────────────────────────┘  │
│                                                                        │
└────────────────────────────────────────────────────────────────────┘
```

- Semantic list: `<ul data-testid="triage-related-work-results">` /
  `<li>` wrapping each card (`research/ux.md` §4 "Result list semantics" — a bare `<div>`
  wrapper, as `HistorySearchResults.tsx` uses today, is a gap this new component should
  not copy forward).
- Each card is a real `<a href="/history?sessionId=...&messageIndex=..." target="_blank" rel="noopener noreferrer" data-testid="triage-related-work-hit-{sessionId}">`
  (Task 2.2.3a, superseding Task 2.2.1c's original `<button>` draft), not a `role="button" div`
  — matches the panel's own convention of using real interactive elements (Apply/Skip/Refine/
  dismiss are all real buttons; this is a real link since it navigates) and fixes the
  keyboard-activation gap `research/ux.md` §4 flags in `HistorySearchResults`' existing card,
  plus gets native middle-click/ctrl-click "open in new tab" for free.
- Field order (top to bottom, per `research/ux.md` §3 priority ranking): session
  title → date + match count → (reserved outcome-badge slot, empty in v1) → top snippet
  (first of `snippets[0]`, truncated ~200 chars) → project path (de-emphasized, omitted
  from the wireframe above for space but rendered smallest/lowest-contrast-safe text).
- "+N more matches in this session" is **plain text**, not a link/button — v1 does not
  wire a separate expand action for it (see scope note above and §7). It reads as
  informational context, not an unfulfilled promise: activating anywhere on the card
  (including over this text) navigates to the session's full history, where those
  additional matches are visible by browsing that session — it is not a second,
  separately-clickable target that silently does nothing.
- Max 5 cards (`limit=5`, fixed) — no pagination/"load more" control in v1.

**Interaction flow**

1. Search resolves with ≥1 session-deduped hit.
2. Cards render top-to-bottom by score (already sorted server-side).
3. User can Tab through cards in document order; each is independently activatable
   (Enter, native `<a href>` semantics, plus middle-click/ctrl-click) — see §7 for what
   activation does.

---

## 4. Empty state (zero matches)

```
┌─ Find related past work ──────────────────────────────────────────────┐
│  ┌──────────────────────────────────────────────────────────────┐ 🔍 │
│  │ Add dark mode toggle to settings page                         │    │
│  └──────────────────────────────────────────────────────────────┘    │
│                                                                        │
│   No related past sessions found — this looks like new territory.    │
│                                                                        │
└────────────────────────────────────────────────────────────────────┘
```

- Copy is deliberately reassuring, not generic (`research/ux.md` §5.1 / Story 2.2.2) —
  a null result is a *positive* triage signal ("genuinely novel"), distinct from an error.
- `data-testid="triage-related-work-empty"`.
- Shown only for a **completed, non-empty** query with `results.length === 0` — not the
  same as the blank-title case (§6), which never fires a query at all.
- Exit path: the input remains editable in place — the user can immediately try a
  different/shorter query without navigating away.

---

## 5. Error state (search failed)

```
┌─ Find related past work ──────────────────────────────────────────────┐
│  ┌──────────────────────────────────────────────────────────────┐ 🔍 │
│  │ Add dark mode toggle to settings page                         │    │
│  └──────────────────────────────────────────────────────────────┘    │
│                                                                        │
│  ⚠ Search failed — [Retry]                                            │
│                                                                        │
└────────────────────────────────────────────────────────────────────┘
```

- `<div role="alert" data-testid="triage-related-work-error">Search failed —
  <button>Retry</button></div>` (Story 2.2.2).
- Scoped to the search box, not a panel-wide banner — deliberately does **not** reuse
  `TriageErrorBanner`'s "Reload item"/"Skip without applying" actions
  (`research/ux.md` §5.3): those are about the triage *result*, not this search, and
  showing them here would let a user think a failed background search invalidates the
  triage suggestion itself.
- `Retry` re-invokes `search()` with the current `debouncedQuery` and the same fixed
  option bundle — one click, no re-typing required.
- Exit path: Retry is the primary action; the input also remains editable, so a user can
  change the query instead of retrying verbatim.

---

## 6. Edge case: blank/whitespace item title

```
┌─ Find related past work ──────────────────────────────────────────────┐
│  ┌──────────────────────────────────────────────────────────────┐ 🔍 │
│  │                                                                 │    │
│  └──────────────────────────────────────────────────────────────┘    │
│                                                                        │
└────────────────────────────────────────────────────────────────────┘
```

- Per `research/ux.md` §5.4, a saved `BacklogItem.title` is non-optional, so this is a
  defensive guard rather than an expected production state (the panel only renders once
  `triageStatus === "completed"`, by which point triage has already run against real
  title text). Still specified, mirroring `useHistoryFullTextSearch`'s own
  empty-query short-circuit (Task 2.2.1b: `if (!debouncedQuery.trim()) { clearSearch();
  return; }`).
- No RPC call fires. Input renders empty and **unfocused** (not a loading state, not an
  error state — just inert until the user types).
- `aria-label` falls back to `"Search past sessions for this item"` when `itemTitle` is
  empty (§1).

---

## 7. Click-through from a result card — design decision

`research/ux.md` Open Question 4 leaves this open ("modal? dedicated route? inline
expansion? — not decided by this doc"), and the implementation plan renders each
`SessionHitCard` as a `<button>` without specifying its `onClick`. This design closes
that gap for v1:

**Decision: clicking a card navigates to the session's existing detail/history page in a
new tab, not an inline expansion — implemented as a real `<a href target="_blank"
rel="noopener noreferrer">`, not a `<button>` with `window.open()`.** (Amended
2026-08-02 during Phase 4 triad review: a `<button onClick={() => window.open(...)}>`
loses native middle-click/ctrl-click "open in new tab" and screen-reader new-window
announcement — a real anchor element gets both for free, at no cost, since the target
URL is already known synchronously at render time.)

Rationale, consistent with the reasoning already used in `research/ux.md` and
`implementation/plan.md`:
- `research/ux.md` §1 itself calls inline expansion "likely too cramped" inside a triage
  panel that already stacks Summary + AC diff + task list.
- A new tab (not same-tab navigation) preserves the triage panel's state — the user is
  mid-review of a triage suggestion; losing that context to view prior art would be a
  worse trade than the extra tab.
- No new route, modal, or scroll-mode UI needs to ship in Phase 2 to satisfy this — the
  session detail page already exists as a navigation target (per `plan.md`'s Epic 1.4
  anchor-paging work, which exists precisely so a session page can center on a specific
  message). If the detail page does not yet support a `?anchor=<messageIndex>` deep link
  at implementation time, the fallback is "open the session at its default view" — still
  a valid exit, just not scrolled to the exact hit.

**Interaction flow**

1. User clicks/activates a `SessionHitCard`.
2. New browser tab opens at the session's detail/history page, anchored on the hit's
   `messageIndex` if the route supports it (fallback: session's default view).
3. Triage panel tab is untouched — its search box, results, and any in-progress
   Apply/Skip/Refine state remain exactly as they were.

**Flagged for Phase 3 implementation to confirm**: whether the session detail route
accepts an anchor/message-index query param today. If not, ship the fallback (open at
default view) rather than blocking this on a new deep-link route — matches the plan's
own "additive, don't rabbit-hole" posture.

---

## 8. Read-only mode

- The entire `TriageRelatedWorkSection` (search box + results, all states above) is
  **omitted from the DOM**, not rendered disabled/frozen (`research/ux.md` §1, matching
  the existing Actions-block precedent at `TriageReviewPanel.tsx:266`).
- No wireframe needed — there is no surface to render. This is itself the tested
  behavior (Story 2.2.1, fourth bullet).

---

## UX Acceptance Criteria

Each is independently checkable by a human clicking through the panel.

**Task completion**

1. A user can see related past work for a backlog item in **0 extra clicks** — it is
   visible (search fired, results rendered) immediately on triage-panel load, given a
   non-empty item title.
2. A user can revise the search to different terms in **1 step** (type in the
   pre-filled box) — no separate "edit query" mode or button to find first.
3. A user can retry a failed search in **1 click** (`Retry` button inside the inline
   error alert).
4. A user can view a hit's full surrounding conversation by activating the result card —
   opens in a new tab in **1 click/keypress**, without losing the triage panel's state.

**Error / edge-case handling**

5. Zero-match state shows the exact copy "No related past sessions found — this looks
   like new territory." (`data-testid="triage-related-work-empty"`) — not a generic
   "no results" message.
6. Search-failure state shows "Search failed — [Retry]" inside a `role="alert"` element
   (`data-testid="triage-related-work-error"`) — and does **not** show
   `TriageErrorBanner`'s "Reload item"/"Skip without applying" actions, which belong to
   the triage-apply flow, not the search.
7. A blank/whitespace item title results in an empty, unfocused input and **no** RPC
   call — never a spinner, never an error, never the "no matches" copy.
8. **No dead ends**: every one of the four data states (loading, error, empty, populated)
   leaves the search input editable and the panel's own Apply/Skip/Refine controls
   reachable — none of the new component's states can trap focus or block the rest of
   the panel.

**Accessibility**

9. Keyboard: the search input is reachable via Tab and directly typeable; every result
   card is reachable via Tab (document order) and activatable via Enter, because each
   card is a real `<a href>` element (amended from an earlier `<button>` draft — see §7;
   not a `role="button" div`, which lacks native key/click handling) — this also gives
   middle-click/ctrl-click "open in new tab" for free, which a `<button>` cannot.
10. Screen reader: the input exposes `aria-label="Search past sessions for {itemTitle}"`
    (or the `"this item"` fallback); the result list uses `<ul>/<li>` semantics so
    assistive tech announces item count; result-count and state-text changes are
    announced via the existing ancestor `<section aria-live="polite">` on
    `TriageReviewPanel.tsx` — no second, nested `aria-live` region is added (avoids
    duplicate announcements).
11. Color contrast: all new text — snippet body, "+N more matches" line, the reassuring
    empty-state copy, and the `role="alert"` error text — meets **4.5:1** contrast
    against its background in both light and dark theme, using existing `vars.color.*`
    tokens (per `.claude/rules/css-architecture.md`; no new hardcoded colors introduced
    by this component).
12. Focus is never programmatically moved into the input or into the results list on
    mount or on auto-search completion — the panel's existing focus entry points
    (Apply/Skip) are not disturbed by a background-populated search box.

**Read-only mode**

13. With `readOnly=true`, no trace of the search box or results renders in the DOM —
    verified by absence of `data-testid="triage-related-work-input"` — matching the
    Actions-block precedent exactly, not a disabled/frozen variant.
