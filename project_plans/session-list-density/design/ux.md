# UX Design: session-list-density

Final design artifact for implementation. Built from `requirements.md`, the deep
research pass (`research/ux.md`), and `implementation/plan.md`/ADR-001 — this
document does not re-derive the truncation heuristic or the accessibility
findings; it wireframes the five interactive surfaces those documents already
scoped and turns them into testable UX acceptance criteria.

**Job the row/card does** (per `research/ux.md` §5): let the user decide, in
under a second, "resume this session or skip it." Everything below is ordered
by that job: status/recency first, category+slug second, opaque IDs last and
off the primary line.

---

## Surface 1: List-view row, normal width (~280px sidebar)

### Wireframe

```
┌─────────────────────────────────────────────────────┐
│ ☐ ● backlog/triage                              ···  │  ← line 1: checkbox, status dot,
│     ~/.stapler-squad/workspaces/…/triage-dc2c9…       │     name (wraps if needed), overflow
│     ⏱ 14m ago                                         │  ← line 2 (wrap of line 1 if long)
│                                                       │  ← line 3: elapsed (own line, no grid col)
└─────────────────────────────────────────────────────┘
  ^                                              ^
  reserveCheckbox (24px)                  rowOverflowButton
                                           (≥44×44 on pointer:coarse)
```

Agent glyph and memory badge are **not** rendered by default (Story 1.2.1) —
they existed as extra grid cells to the right of the name in the baseline;
that space is now reclaimed by the name/path wrap.

### Interaction flow

1. User scans the sidebar. Status dot + category prefix (`backlog/`, `pr-424-`)
   + human slug (`triage`, `compute-nop`) are always fully visible — never
   wrapped away, never truncated (per `ux.md` §2's priority order).
2. If the wrapped name/path still needs more than the available width, the
   opaque segment (workspace hash, worktree UUID suffix) collapses to `…` via
   `truncateWorkspacePath`; the visible text still fits without an ellipsis
   mid-word cutoff.
3. User clicks anywhere on the row (not the checkbox/overflow button) →
   navigates to the session detail view. (Existing behavior, unchanged by
   this feature — included here only to confirm the redesign doesn't
   introduce a new click target ambiguity: the row's click target excludes
   the checkbox and the overflow button, both of which stop propagation.)
4. User clicks `···` → overflow menu opens (existing behavior). Touch-target
   fix (Epic 3.3) makes this reliably tappable at ≥44×44px.
5. User wants agent/memory data → two paths:
   a. Tab to the (now-hidden-by-default) columns — not reachable if not
      visible. Real path is (b).
   b. Open Columns picker (Surface 4) and re-enable `agent`/`memory` as full
      columns, **or** if already visible, Tab/hover onto the icon/badge to
      get the Tooltip disclosure (Surface 5).

### Error / edge cases

| Case | Handling |
|---|---|
| Path with no natural break point (single 80-char opaque token, no `/`) | `truncateWorkspacePath` collapses the trailing hex run before render (`research/ux.md` §1/§4); `overflow-wrap: anywhere` is the CSS safety net if truncation somehow fails (JS error/unexpected data shape) — text breaks mid-character rather than blowing out the grid track. No horizontal scrollbar in either case. |
| Two sessions with identical display name | Not resolved by truncation (truncation only affects the *opaque suffix*, and identical display names by definition share their semantic prefix too). **Flagged, not silently handled**: nothing in the plan disambiguates two rows with the same `session.title`. The existing row already differentiates by path/branch on line 2, and by the row's own click-through to a URL-unique detail view, so a user who clicks the wrong one lands somewhere identifiably different — that's the exit path, not a fix at the list level. This repo's plan does not add a VS-Code-style "append parent folder on collision" disambiguator; recommend a fast-follow if this proves confusing in practice, but it isn't a dead end today. |
| Agent/memory data unavailable (no live process — session paused) | Existing "—" placeholder behavior is unchanged; the column is hidden by default regardless, so an unavailable value is simply absent rather than shown as "—" for the ~80% of rows without a live process (this *is* the point of Story 1.2.1). When the user explicitly re-enables the column via the picker, "—" still renders for a paused session — that's honest information ("no data"), not an error. |
| Row height with 2–3 wrapped lines inside a virtualized list | Handled by `measureElement` + a raised `estimateSize` (64px, Story 2.1.3) — not a user-visible error state, but flagged as the one place a genuine implementation risk (`architecture.md`/`plan.md` Unresolved Questions) exists: if `estimateSize` is badly wrong, the user sees scroll-position jank on fast scroll. No error message applies; it's a smoothness/tuning risk, called out here so it isn't lost between documents. |
| `SessionList` mounted inside the resizable pane-split cockpit layout (`web-app/src/components/pane/PaneSplitRenderer.tsx`, `pane.viewKind === "session-list"`) rather than the fixed ~280px sidebar | The "~280px sidebar" framing above describes the `app`/`backlog` layout's fixed column, not the only place this row renders. In the cockpit layout, pane width is a continuous, user-dragged value (`ResizeHandle`/`RESIZE_PANE`), so `ROW_PATH_MAX_LEN` and the 200px breakpoint must also look reasonable across that range, not just at 280px. Not a distinct visual design — same wireframe, same single budget, same breakpoint — but plan.md's Task 2.1.1c manual check explicitly covers this second mounting context (per pre-mortem.md P1 #1) rather than assuming the fixed-sidebar check generalizes. |
| Rows-visible-per-viewport after wrapping to 2–4 lines | Not a failure mode this design resolves on its own — wrapping trades single-line ellipsis for readability at the cost of showing fewer sessions per screen (per pre-mortem.md P1 #3). Plan.md adds a before/after rows-visible-per-viewport check (Task 2.1.3c) rather than assuming the density tradeoff is acceptable by construction; this document's wireframes are otherwise unchanged by that check's outcome. |

---

## Surface 2: List-view row, narrow container (< 200px)

### Wireframe

```
┌───────────────────────────┐
│ ☐ ● backlog/triage    ···  │
│     ~/.../triage-dc2c9…    │  ← ROW_PATH_MAX_LEN (72 chars — same constant, same
│     ⏱ 14m                  │     value, as Surface 1; wraps onto more lines here)
└───────────────────────────┘
```

Same structure as Surface 1. The truncation budget does **not** change at
this width — `ROW_PATH_MAX_LEN = 72` is one constant used at every container
width (per `plan.md` Story 2.1.2's Resolution Note; the originally-planned
normal/narrow budget pair was dropped). Below the `(max-width: 200px)`
container-query breakpoint, only purely-visual CSS shifts apply: a slightly
smaller font size, and second-line chips (elapsed + future badges) wrapping
onto their own line rather than clipping. Since Story 2.1.1 already switches
the path from ellipsis/`nowrap` to `overflow-wrap: anywhere` wrapping, the
same-budget string simply wraps onto more lines here instead of overflowing
or getting clipped. No new interaction — this is a `@container` CSS variant
of Surface 1, not a distinct flow.

### Interaction flow

Identical to Surface 1. There is no second, narrower-budget render:
`SessionRow` renders the path `<span>` exactly once, via
`truncateWorkspacePath(session.existingDir, ROW_PATH_MAX_LEN)` — the same
call, same constant, at every container width. The `@container` query drives
only visual CSS (font-size, `flex-wrap` on the chip line); it never toggles
between two differently-truncated strings. There is no JS resize-detection
step for the user to perceive, and no loading/flash state, since there is
only one render to begin with.

### Error / edge cases

| Case | Handling |
|---|---|
| Container width oscillates near the 200px breakpoint (e.g. user resizes a split-pane sidebar) | Pure CSS container query — no JS re-render, no layout thrash beyond the browser's own reflow. Not a race condition: there is only one path-text render, so there is nothing to toggle between. |
| Content still doesn't fit at the narrowest supported width (200px, per acceptance criteria) | `overflow-wrap: anywhere` remains the safety net at every width; no horizontal scrollbar requirement is a hard acceptance criterion (Story 2.1.2) at 200/280/400px. Below 200px is out of scope — not tested, and should be flagged if a future container ever goes narrower (e.g. an extremely narrow split view, or the resizable pane-split cockpit layout dragged past that point — see Surface 1's cockpit-context edge case). |

---

## Surface 3: Board-view card, both widths

### Wireframe (normal card width)

```
┌──────────────────────────────────────┐
│ ⠿  ● backlog/triage             ···   │  ← BoardCard chrome: drag handle (⠿, ~44px)
│                                        │     + SessionCard content
│    Path: ~/.stapler-squad/…/triage-dc2c…      │
│    Active: ~/.stapler-squad/…/worktree-18d8…  │
│    Repo:  ~/repos/…/stapler-squad             │
│    ⏱ 14m ago                           │
└──────────────────────────────────────┘
```

### Wireframe (narrow board column, ≤ 260px)

```
┌───────────────────────┐
│ ⠿ ● backlog/triage  ···│
│                        │
│  Path: ~/…/triage-dc2c…│  ← wraps rather than clips;
│  Active: ~/…/wt-18d8…  │     CARD_PATH_MAX_LEN unchanged (96),
│  Repo: ~/repo/…-squad  │     but the container itself is narrower
│  ⏱ 14m ago             │     so more visual wrapping occurs
└───────────────────────┘
```

Per `architecture.md`/plan's confirmation, `BoardCard` is a thin (72-line)
wrapper adding only the drag handle + move menu — this surface's design is
"fix `SessionCard`, `BoardCard` inherits it," not a parallel design.

### Interaction flow

1. User scans a Board column. Three path fields (`existingDir`, `activeDir`,
   `clonedRepoPath`) each show `truncateWorkspacePath(value, 96)` — cards get
   a larger budget than rows (96 vs. 72) because they have more horizontal
   room (per requirements.md's own note that Board parity isn't assumed to be
   identical to List).
2. Drag-and-drop (existing `BoardCard` behavior) is unaffected — the redesign
   only touches text rendering inside `SessionCard`, not `BoardCard`'s drag
   handle/move-menu chrome.
3. User opens `···` on the card → same overflow menu as List view (shared
   component), same 44×44px touch-target fix applied to the card's own
   `overflowButton` (Story 3.3.1, which additionally fixes a *width* gap the
   row's fix didn't have — the card had `minHeight: 44px` already but was
   missing `minWidth: 44px`).

### Error / edge cases

| Case | Handling |
|---|---|
| Board column narrowed below the card's usable minimum (drag handle chrome eating into content width) | Acceptance criterion: at a 260px total column width (accounting for the ~44px drag-handle chrome), the inner `SessionCard` content area must stay ≥200px wide with no unwrapped clipping (Story 2.2.2). If a column is narrowed further than that, this is an unhandled edge case — **flagged, not resolved** in the plan; recommend enforcing a minimum Board-column width in a fast-follow if this proves reachable in practice (drag-resize of Board columns isn't itself in scope here). |
| `activeDir` missing a `title` attribute (asymmetry with `existingDir`/`clonedRepoPath`, called out in Task 2.2.1b) | Plan explicitly requires adding one to match the sibling fields' pattern — this closes what would otherwise be an accessible-name gap unique to one of the three path fields. Confirmed as an acceptance criterion in Story 2.2.1, not left implicit. |
| Same identical-display-name ambiguity as Surface 1 | Same answer: not resolved at the card level either; the click-through to a unique detail view remains the exit path. |

---

## Surface 4: Columns picker (re-showing agent/memory)

### Wireframe

```
┌─ Columns ▾ ──────────┐
│ ☑ Elapsed             │
│ ☑ Diff                │
│ ☑ Branch              │
│ ☐ Agent      ← newly default-off
│ ☐ Memory     ← newly default-off
└───────────────────────┘
```

`ColumnPicker.tsx` requires **no code change** (Story 1.2.1) — it already
reads `COLUMN_DEFS` generically, so flipping `defaultVisible: false` on two
entries is invisible to this component; it simply starts both checkboxes
unchecked.

### Interaction flow

1. User opens the Columns picker (existing trigger — unchanged location/
   affordance).
2. User sees `Agent` and `Memory` present in the list but unchecked (this is
   the "reachable via the existing Columns picker" escape hatch the
   requirements doc requires — the data is demoted, not deleted).
3. User checks `Agent` → `visibleColumns` includes `"agent"` → the agent-icon
   `<span>` reappears in every row's grid as a normal column, identical to
   today's pre-redesign behavior (not the tooltip-disclosure path — this is
   the "make it a real column again" path).
4. Setting persists per existing app behavior (out of scope to this feature
   to change persistence semantics).

### Error / edge cases

| Case | Handling |
|---|---|
| User doesn't discover the picker (never opens it) | This is the acknowledged tradeoff in the requirements doc: single-user personal tool, the picker is an existing, already-discovered affordance (not new UI), and ADR-001's row-level `aria-label` fold-in (Surface 5) is the redundant path that doesn't require discovering the picker at all. Two independent paths to the same data — no dead end. |
| User re-enables both columns on a narrow container | Not specially handled — re-enabled columns render as normal grid cells same as before this feature, subject to the same narrow-width wrapping/container-query rules as any other visible column. No new failure mode introduced. |

---

## Surface 5: Tooltip/disclosure for agent-program and memory data (ADR-001)

### Wireframe (row, agent column visible, keyboard focus)

```
┌───────────────────────────────────────┐
│ ☐ ● backlog/triage   [🤖]  512MB  ···  │
│                        ▲                │
│                 focus ring (Tab lands here)
│                        │
│                 ┌──────────────┐
│                 │ claude        │  ← role="tooltip", shown on focus OR hover
│                 └──────────────┘
└───────────────────────────────────────┘
```

### Interaction flow

1. **Mouse user**: hovers the agent icon or memory badge → Radix `Tooltip`
   shows (unchanged visual behavior from baseline, but now backed by a real
   `role="tooltip"` element instead of a bare `title` attribute).
2. **Keyboard user**: Tabs to the agent-icon/memory-badge span (now
   `tabIndex={0}`, previously not in the tab order at all) → the same Radix
   tooltip shows on focus. This is the fix: previously `title` never fired
   without a mouse, so this data was **unreachable** by keyboard, full stop.
3. **Screen-reader user, columns hidden (new default)**: never needs to find
   the tooltip at all — the row's `aria-label` already concatenates
   `agent: claude, memory: 200 MB` (when present) into the row's accessible
   name, so the data is announced on landing on the row, with zero extra
   interaction, independent of `visibleColumns`.
4. **Touch-only sighted user**: per ADR-001's explicit, named consequence —
   Radix tooltips are hover/focus-triggered, not tap-triggered, on touch
   devices. This is **not fixed** by this feature. The escape hatch is
   Surface 4 (Columns picker) — re-enable the column as a permanent, always-
   visible grid cell, which needs no tap-disclosure at all.

### Error / edge cases

| Case | Handling |
|---|---|
| Touch-only user, columns hidden, never opens the picker | **Named gap, not resolved.** ADR-001 accepts this as proportionate for a complexity-2, single-user tool rather than building bespoke tap-tooltip logic. Flagging here per this document's instructions: this is the one interaction path in the whole feature with a real, acknowledged dead end (no non-mouse, non-keyboard, no-picker path to agent/memory data) — Task 3.2.1d requires a manual check of whether tap happens to work on iOS Safari/Chrome Android regardless (Radix's real-world tap behavior is asserted, not verified, per the plan's own Unresolved Questions), which could close this gap for free if it turns out tap already triggers focus-adjacent behavior. |
| `session.program` empty/missing | Existing conditional (`session.program ? ... : ""`) already guards the `aria-label` append and the visible icon's presence — an absent program renders neither the icon nor a stray tooltip trigger; no broken/empty tooltip appears. |
| `memMB` is 0 or unavailable | Same guard pattern (`memMB > 0`) — memory badge and its tooltip/aria-label fragment are simply omitted, not shown as "0 MB" or an error state. |

---

## UX Acceptance Criteria

Testable by a human, organized by the categories requested.

### Task completion

1. A user can identify a session's category and human-readable slug (e.g.
   `backlog/triage`, `pr-424-compute-nop`) **without hovering or clicking
   anything** — it's on line 1 of the row/card at every container width from
   200px to full sidebar width.
2. A user can re-enable the `agent`/`memory` columns in **≤ 2 clicks**: open
   Columns picker (1 click) → check the box (1 click).
3. A user can reach the full, untruncated path for any row/card in **≤ 1
   interaction**: hover or focus the path text (Tooltip/`title` shows the
   full string) — no picker, no menu, no second screen required.
4. A keyboard-only user can reach agent-program and memory data via Tab in
   **≤ 1 Tab stop** from the row (the icon/badge itself), when the columns
   are visible; via **0 additional interactions** (row-level `aria-label`)
   when hidden.

### Error states

5. There is no failure mode where a truncated string appears with **no**
   full-value companion — every `truncateWorkspacePath` call site has a
   co-located full-value `title`/`aria-label` (Story 3.1.1 acceptance
   criteria; verified by grep-audit, Task 3.1.1a).
6. A path with a single unbroken 80+-character opaque token (no natural
   break) never causes horizontal overflow or a clipped/cut-off character —
   `overflow-wrap: anywhere` is present as a CSS-level guarantee independent
   of the JS truncation succeeding.
7. If `session.program` or memory data is unavailable, the UI shows nothing
   (omitted) rather than a placeholder error string — distinguishing "no
   data" from "error loading data" is out of scope (there is no async load
   for this data; it's synchronous from the already-fetched session object).

### No dead ends

8. Every error/edge case identified above has a stated exit path **except
   one, explicitly flagged**: touch-only sighted users with `agent`/`memory`
   columns hidden and who never open the Columns picker have no way to reach
   that data via tap alone (ADR-001's named, accepted consequence). All other
   flows (identical display names, narrow-width overflow, missing
   program/memory, board-column narrowing) resolve to an existing, working UI
   element (detail-view click-through, CSS safety net, Columns picker,
   omitted display).
9. The overflow menu (`···`) is always dismissible (existing behavior,
   unchanged) and never the only path to an action — every menu action is
   also reachable via the row/card's own click-through where applicable.

### Accessibility

10. Every interactive element introduced or modified by this feature
    (`rowOverflowButton`, `SessionCard`'s `overflowButton`, the agent-icon and
    memory-badge spans) is keyboard-reachable via Tab and has a visible focus
    indicator (inherited from the existing shared button/`Tooltip` focus-ring
    styles — no new focus style is introduced, so this criterion is "confirm
    unchanged," not "build new").
11. The agent-icon and memory-badge tooltips use `role="tooltip"` +
    `aria-describedby` (via Radix), not a bare `title` attribute — verified
    by DOM inspection after Story 3.2.1.
12. Every row's `aria-label` is the row's full accessible name, including the
    untruncated path and (when present) `agent:`/`memory:` fragments —
    verified by the regression test added in Task 3.1.1b.
13. The "···" overflow buttons meet a 44×44px touch target on
    `(pointer: coarse)` for both `SessionRow` and `SessionCard` — verified by
    the new Playwright assertions in `tests/e2e/touch-targets.spec.ts`
    (Story 3.3.1).
14. Color contrast for all text introduced by this feature (name, path,
    elapsed second line, tooltip content) is inherited from existing design
    tokens (`vars.color.textMuted`, etc.) already used elsewhere in the
    component — no new color is introduced, so no new contrast check is
    required beyond confirming the existing tokens meet ≥ 4.5:1 (already true
    of the baseline, per the existing Axe Core CI gate on `web-app/src/`
    PRs — this feature doesn't touch that gate's pass/fail status).

---

## Open flag for implementation

One dead end is knowingly shipped, not silently dropped: **touch-only sighted
users cannot reach demoted agent/memory data without discovering the Columns
picker** (see Surface 5, Criterion 8). This is ADR-001's explicit, accepted
tradeoff for a complexity-2 single-user tool, and Task 3.2.1d's manual tap
check may close it for free if Radix's tooltip happens to respond to tap on
the project's real target browsers. If that manual check comes back negative,
this gap should be logged as a fast-follow rather than assumed fixed.
