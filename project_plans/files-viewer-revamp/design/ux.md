# UX Design: files-viewer-revamp

Phase 3 UX design. Inputs: `requirements.md`, `research/ux.md`, `implementation/plan.md` (UI stories 1.3.x, 1.4.x, 1.5.x, 2.2.x-2.4.x), ADR-030..034 (referenced, not re-read in full); extended on 2026-10-16 with the ADR-036 large-file and delta-save states (sections 8.12, 8.13, 9.4 and the `U-LARGE-*`, `U-PATCH-*`, `U-OBS-5` criteria). Where this document refines `research/ux.md`, the refinement is stated. Owner decisions already fixed: 3-way merge is in scope; vim is on by default only when a hardware keyboard is detected **at Edit entry** (never flipped live mid-edit), and on a touch device only by explicit per-device opt-in (the toggle is persisted per device and always wins; see section 8.8); tab state is localStorage per (session, device); create/rename/delete are out of scope.

## 0. Conventions

- **Layout rule (triad round 3; one rule, pure function `chooseFilesLayout`)**: the **mobile layout** (nav-stack panes, top and bottom bars, extra-key bar, the 7.1a edit ladder) is chosen when the Files-tab container is narrower than 768px **or** the device is phone-class: `(pointer: coarse)` matches **and** `min(screen.width, screen.height) < 600` CSS px. Otherwise the **desktop layout** is chosen. `screen` is used, not the window or visual viewport, so the answer is stable across rotation and when the soft keyboard shrinks the viewport (a tablet at 1180x820 with the keyboard up stays desktop; a phone at 844x390 stays mobile). Consequences: a landscape phone (`L-390` 844x390) keeps the mobile layout and the mobile editing ladder, which is what 7.1a, U-MOB-2/7/14 test; a tablet (820x1180 or 1180x820, short side >= 600) gets the desktop layout (7.3); a window dragged across 768px on a desktop swaps layouts, a phone never does. The layout never remounts the editor or drops the keyboard when it changes. Both layouts render the same state model (`OpenBuffers`, jumplist, search state, `PaneState`). The tab cap uses its own `pointer: coarse` or < 768px test (section 5), unchanged.
- **Touch targets**: 44x44 CSS px on `pointer: coarse` for every primary control; on `pointer: fine` at least 24x24 clear target.
- **Status messaging**: one `aria-live` region owned by the Files tab. Success and info use `polite`; errors use `assertive`. Banners use `role="status"` except a priority-1 (blocking) banner, which uses `role="alert"` (section 8.2); no banner moves focus. **Transient messages and toasts (triad round 2)**: every confirmation, success and Undo toast stays at least **8 s**, pauses while hovered or focused, has a visible dismiss control, and is also announced in the live region (so dismissing it loses nothing); errors persist until dismissed or superseded. Under `prefers-reduced-motion` toasts and sheets appear and disappear without slide or fade.
- **Dialogs**: focus trapped, focus restored to the invoking control on close, `Esc` (desktop) and browser/system back (mobile) dismiss to the safest option (never to a destructive one). **Initial focus is the least destructive option, and Enter on it never saves, discards or overwrites** (Discard dialog: Cancel; Restore-draft dialog: Not now; Conflict dialog: Keep editing).
- **Color is never the only cue**: diff uses `+`/`-` gutter glyphs, search hits use `<mark>`, dirty uses a dot plus the word "unsaved" in the accessible name.
- **Exit-path rule**: every error state below lists an exit path in its table. Section 17 audits this.

## 1. Surface inventory

| # | Surface | Interactive | Mobile | Desktop | Section |
|---|---|---|---|---|---|
| S1 | Search panel and results | yes | full-screen pane | Explorer\|Search side panel | 3 |
| S2 | Quickfix chip (next/prev hit) | yes | floating chip | status bar segment + F4 | 4 |
| S3 | Tab strip | yes | replaced by S4 | top of center column | 5 |
| S4 | Tab switcher sheet | yes | `[N]` button -> sheet | n/a | 6 |
| S5 | Layout shell (top bar, bottom action bar, pane stack, back/forward) | yes | nav-stack | toolbar + breadcrumb | 7 |
| S6 | Editor (read mode, edit mode, banners; large-file windowed states and delta-save states, ADR-036) | yes | read-by-default + Edit FAB | Edit toggle | 8, 8.12, 8.13 |
| S7 | Conflict dialog | yes | full-screen sheet | modal | 9 |
| S8 | 3-way merge view | yes | unified, per-hunk rows | unified or side-by-side | 10 |
| S9 | Discard and Restore-draft dialogs | yes | sheet/dialog | modal | 11 |
| S10 | Extra-key bar | yes | pinned above keyboard | not shown | 12 |
| S11 | Status bar | yes (chips are buttons) | compact, in overflow + chips | 24-28px bar | 13 |
| S12 | Quick-open / command palette / go-to-line | yes | opened from bottom bar | Ctrl+P, `>`, Ctrl+G | 14 |
| S13 | Outline panel | yes | sheet | dropdown from breadcrumb | 15 |
| S14 | Shortcut help overlay | yes | button in overflow | `?` and toolbar button | 15 |
| S15 | Non-interactive: feature flags, slog lines | no | n/a | n/a | 16 |
| S16 | Accessible view (non-virtual read alternative for assistive technology) | yes | overflow toggle + skip link | toolbar toggle + skip link | 8.10 |

16 surfaces: 15 interactive (full treatment), 1 non-interactive (condensed).

## 2. Global flows and state model

```
PaneState (mobile):   files  <->  editor  <->  search        (browser back pops the stack)
EditorMode:           Read  <->  Edit            (Edit exists only if files:editing && ReadOnlyReason=None)
Buffer arm:          Loading | Failed | Loaded          (section 8.5; Loading and Failed show their own pending/failure states and have no Edit or Save)
Loaded source:       Full | Windowed (ADR-036: files over 1 MiB when files:range-read is on; sections 8.12, 8.13; Windowed adds the states gap-loading, gap-failed, indexing, view-only T2)
Loaded status:       clean | dirty | stale(clean, disk changed) | conflict | offline-dirty
```

Primary journeys, with the interaction-count budget the requirements set (<= 3):

| Journey | Desktop | Mobile (390x844) |
|---|---|---|
| Find string, land on line | Ctrl+Shift+F, type, Enter (3) | tap Search in bottom bar, type, tap hit (3) |
| Step through hits | F4 / Shift+F4 (1 each) | tap chip chevron (1 each) |
| Return to the previous location | Alt+[ (1) | tap Back in bottom bar (1) |
| Return to the search results after opening a hit | Ctrl+Shift+F, which re-focuses the query field with the results intact (1; the panel never closed; Esc is not used in this direction because inside the editor Esc belongs to the editor, 8.4) | tap `<` in the top bar or system Back (1) |
| Edit and save | Edit toggle (or already in Edit), type, Ctrl+S | Edit FAB, type, Save key on extra-key bar |

## 3. S1 Search panel and results

### 3.1 Desktop wireframe

```
+--------------------------------------------------------------------------+
| [Explorer] [Search*]                                      (left panel)   |
| +----------------------------------+                                     |
| | > foo_bar                    [x] |  query (role=searchbox)             |
| | [Aa] [ab] [.*]   [v filters]     |  toggles are aria-pressed buttons   |
| |  include: **/*.go                |  (collapsed by default)             |
| |  exclude: vendor/**              |                                     |
| |  [ ] include ignored files       |                                     |
| +----------------------------------+                                     |
| | 27 results in 6 files   [Cancel] |  aria-live=polite header            |
| | v pkg/a.go                    (4)|  group header = treeitem level 1    |
| |     12  func [foo_bar](x int)    |  hit row = treeitem level 2, <mark> |
| |     88  return [foo_bar]()       |                                     |
| | > pkg/b.go                    (9)|                                     |
| | ...                              |                                     |
| | Skipped 3 binary files           |                                     |
| +----------------------------------+                                     |
+--------------------------------------------------------------------------+
```

### 3.2 Mobile wireframe (full-screen pane)

```
+--------------------------------+
| <  Search               [...]  |  44px top bar; < returns to previous pane
| [ foo_bar                  x ] |  44px query field, inputmode=search
| [Aa] [ab] [.*]  [Filters v]    |  44px toggle row; Filters expands include/exclude
| 27 results in 6 files          |
| v pkg/a.go                (4)  |  sticky group header, 44px
|   12  func [foo_bar](x int)    |  row >= 44px, two-line wrap allowed
|   88  return [foo_bar]()       |
| > pkg/b.go                (9)  |
|                                |
+--------------------------------+
| Files|Search|Open|Go|< >       |  bottom bar (hidden while keyboard is open)
+--------------------------------+
```

### 3.3 Interaction flow

| Step | User | System |
|---|---|---|
| 1 | Opens Search (Ctrl+Shift+F / bottom bar) | Focus lands in query field; prior query and results restored (tree state also retained). Selected text in the editor pre-fills the query (desktop). |
| 2 | Types >= 2 chars | 300 ms debounce; one streaming request; header shows "Searching..." with Cancel; groups appear as batches arrive. A 1-char query sends nothing and shows "Type at least 2 characters". |
| 3 | Edits query mid-flight | Prior stream aborted; late batches ignored; list keeps prior results dimmed until first new batch arrives. |
| 4 | Arrow Down (desktop) or scrolls (mobile) | Focus walks rows with `aria-activedescendant` on the tree; focus never leaves the query input on desktop while typing. Semantics in 3.5. |
| 5 | Enter / tap on a hit | Opens (or reuses) a preview tab at the line, match highlighted, cursor on match; results panel stays; focus moves to editor. Ctrl+Enter pins the tab (desktop). |
| 6 | Esc (desktop, focus in the query field or results; 8.4 table) / system back or top-bar `<` (mobile) | Desktop: focus returns to the editor (Ctrl+Shift+F goes the other way). Mobile: pops the editor pane back to the results pane, scroll, query and group collapse state preserved (the Back model is section 7.4). |
| 7 | Collapse a group header | Rows hidden; count stays; state kept across tab switches. |

### 3.4 Error and edge states (S1)

| State | What the user sees | Exit path |
|---|---|---|
| Idle (panel open, no query yet, or query cleared) | A helper block, not a blank pane: "Search file contents in <session name>", "Type at least 2 characters", one line explaining the toggles (`Aa` case, `ab` whole word, `.*` regex) and that ignored and binary files are skipped. If the previous search's results were retained they are shown instead, dimmed with "Results for `foo` - edit to search again". No request is sent. | Type; Esc / `<` closes the panel. |
| Invalid regex | Inline error under the field: "Invalid regular expression: <reason>". Prior results stay. No toast. | Fix the pattern, or toggle `.*` off. |
| Zero results | "No matches for `foo` (case-sensitive, regex)" listing active toggles, plus **Clear filters** button. | Clear filters button; edit query. |
| Truncated at cap | "Showing first 1000 - refine your query". | Refine query or add include/exclude globs (field gets focus via "Add filters" link). |
| Slow search (> 1 s) | Progress text with elapsed count and **Cancel**; partial results stay usable. | Cancel, or keep working with partial results. |
| Queued (waiting behind 2 other walks) | Header: "Waiting for other searches (1 ahead)..." with **Cancel**; prior results stay visible, dimmed; announced once via aria-live. Becomes "Searching..." on the first batch. | **Cancel** (returns to idle) or keep waiting; editing the query replaces the queued search. |
| Cancelled | "Search cancelled - 312 results so far". | Re-run by editing the query or pressing Enter. |
| Binary skipped | "Skipped N binary files" note at the list end. | Informational; "Include ignored files" toggle does not affect it. |
| Worktree unavailable (paused/archived session) | Empty state: "Session paused; worktree unavailable". The server refuses the search (`FailedPrecondition WorktreeMissing`); the repo-root fallback is never searched, so results can never come from a different checkout than the session's. | **Resume session** link (existing resume action) or switch to another session tab. |
| RPC failure / network | Inline error "Search failed - check your connection" with **Retry**. Prior results stay. | Retry button; offline chip handles the rest (section 13). |
| Search disabled by flag | Search entry points absent (not disabled). | n/a |
| 5,000+ rows | List is virtualized; header still reports full count. | n/a |
| Hit lands in a file whose open buffer has unsaved changes | Hits are computed from **disk**, not from the buffer. Rows for files with a dirty open buffer carry the marker "buffer has unsaved changes; line may differ" (text plus an icon, in the row's accessible name). Activating such a hit opens the file's existing tab and jumps to the hit's line; if the text at that line no longer contains the matched text, the jump falls back to the nearest line in the buffer that does (searching outward from the hit line, +/- 200 lines), selects it, and announces "Moved to line N (buffer has unsaved changes)"; if none is found it clamps to the hit line and announces "Match not found in your unsaved version". The buffer is never modified or re-read from disk. The same marker and fallback apply to next/prev stepping. | Continue stepping; Save or discard first if line accuracy matters (marker links to Save). |

### 3.5 Results semantics, first tap and idle behavior (triad round 2)

- **Pattern**: the results are a `role="tree"` (not a listbox, which cannot hold collapsible groups). A file group header is a `treeitem` with `aria-level="1"`, `aria-expanded`, `aria-setsize` (number of groups) and `aria-posinset`; a hit row is a `treeitem` with `aria-level="2"` and its own `aria-setsize`/`aria-posinset` within the group. Because the list is virtualized (rows not mounted are absent from the DOM), **every row carries explicit `aria-setsize`, `aria-posinset` and `aria-level`**, and the tree has `aria-label="Search results"` and `aria-busy` while streaming. On desktop the query input owns `aria-controls` = the tree and `aria-activedescendant` = the active treeitem id (focus stays in the input). On touch the rows themselves are focusable (`tabindex="-1"` roving, tapped or swiped to by the screen reader).
- **Accessible names**: group header "pkg/a.go, 4 matches, expanded"; hit row "pkg/a.go line 12, func foo_bar(x int), match 1 of 4, result 3 of 27" (plus "buffer has unsaved changes; line may differ" when applicable). The `<mark>` stays inside the row text so it is not announced as a separate item. Rows are at least 44px on `pointer: coarse` with a two-line wrap allowed.
- **First tap with the keyboard open**: on a phone the query field usually has focus and the soft keyboard is up when results arrive. A tap that lands on a result row must open that row with **one** tap. The row captures its identity on `pointerdown`; if the query input is focused the handler blurs it (the keyboard dismisses) and activates the captured row on `pointerup`, never re-hit-testing after the viewport relayout. A pointer that moves more than 10 px before release is a scroll, not an activation (and a scroll dismisses the keyboard without activating). The same rule applies to the group header and the quickfix chip.
- **Idle state** is defined in the 3.4 table; the empty list is never rendered without an explanation.

## 4. S2 Quickfix chip

```
Mobile (floating, above bottom bar, right):      Desktop (status bar segment):
        +----------------+                       ... | Hit 3/27  [^] [v] [x] | ...
        | 3/27   ^   v  x|
        +----------------+
```

- Appears after opening any hit; stays until dismissed (`x`), a new search starts, or the results are cleared.
- `^`/`v` (and F4 / Shift+F4, `]q`/`[q` in vim normal mode) open prev/next hit without leaving the editor. Wraps at the ends with a polite announcement "Wrapped to first hit".
- Mobile chip targets are 44x44 each; the chip never overlaps the extra-key bar (it is hidden while the keyboard is up; the status bar chip is not shown on mobile).
- Error states: next hit lands in a file that no longer exists -> chip shows "Hit 4 unavailable (file removed)" and auto-advances on next press; hit beyond the loaded portion of a truncated file -> banner described in section 8. Exit: dismiss with `x` or Esc when chip focused.

## 5. S3 Tab strip (desktop)

```
+-----------------------------------------------------------------------------+
| < >  | a.go | *b.go (unsaved)* | _c.go_ (preview, italic) | d.go |  ...  [v] |
+-----------------------------------------------------------------------------+
| pkg > util > b.go                                  [Outline v]   [Edit]     |
+-----------------------------------------------------------------------------+
```

- `role="tablist"`; each tab `role="tab"`, `aria-selected`, `aria-controls`; panel keeps the editor mounted (scroll and cursor preserved per buffer).
- Single-click in tree/results opens a **preview** tab (italic, replaced by the next single-click); edit, double-click, Ctrl+Enter, or save pins it.
- Dirty: dot plus accessible name "b.go, unsaved".
- Close: `x` with a 44x44 hit area on coarse pointers, middle-click, **Alt+W** (not Ctrl+W: browsers reserve it and pages cannot cancel it; chord policy in 14.1), Delete when tab focused. Left/Right/Home/End move and activate.
- Overflow: scrolls horizontally; active tab auto-scrolls into view; trailing `[v]` lists all tabs.
- Context menu (right-click / Shift+F10 / long-press): Close, Close others, Copy path, Reveal in tree, **Pin / Unpin** (the label follows the tab's state). A pinned tab shows a pin glyph and its accessible name ends ", pinned" (a preview tab ", preview"), so the state is not italic-only.
- Drag reorder has a keyboard alternative: Alt+Shift+Left/Right moves the focused tab.

| State | What the user sees | Exit path |
|---|---|---|
| Close dirty tab | Discard dialog (S9). | Save / Discard / Cancel. |
| File deleted from disk while tab open | Tab gets a strikethrough-free "deleted" marker via text ("b.go, deleted on disk") and the editor banner "File no longer exists" (section 8). | Close tab, or (if dirty) copy content / keep editing. |
| Tab limit (10 on mobile / `pointer: coarse` or < 768px, 30 on desktop) | **Pinned and dirty tabs are never closed automatically.** Opening one past the cap evicts only the least-recently-used **clean, unpinned (preview)** tab, announces it via `aria-live="polite"` and shows a toast "Closed <file> to stay within N tabs" with **Undo** (at least 8 s, paused while hovered or focused, dismissible; Undo reopens it with its scroll and cursor, evicting the next eligible tab or, if none, refusing as below). Eviction never happens while the tab is mid-load or has a pending save. If no tab is eligible (all pinned or dirty), the open is **refused** with an inline message "Too many tabs open - close one" and the tab list opens (mobile: the S4 sheet; desktop: the `[v]` overflow list) so a tab can be chosen to close; the refused file is not lost: it stays selected in the tree or results. Unpinning is by the tab context menu (Unpin) on desktop and by the row's actions menu in the mobile sheet (section 6). **The LRU rule and the refusal are one rule**: the cap counts every open tab; the only automatic closure is a clean preview, and when none exists the open is refused, never worked around by closing a pinned or dirty tab. The refusal message offers **Unpin and close the oldest pinned tab** only as an explicit choice inside the tab list (one extra tap), never as an automatic action. | Undo; reopen from Recent files; close a tab from the opened list; Close others. |
| Persisted tab refers to missing file on reload | Tab shown dimmed with "not found"; selecting it shows the missing-file state. | Close tab (x), or Close all missing via context menu. |
| localStorage unavailable | Tabs work in memory only; one-time status "Tab layout will not be remembered". | Dismiss; continue. |

## 6. S4 Mobile tab switcher sheet

```
+--------------------------------+
|  Open files (3)            [x] |  sheet header; x is 44x44
|  +--------------------------+  |
|  | b.go  *unsaved*  [..][x] |  |  row >= 44px; [..] = actions, x = close, each 44x44
|  | pkg/util/b.go            |  |  path subtitle (tail kept)
|  +--------------------------+  |
|  | [pin] a.go (pinned)  [..][x] |  pin glyph + text "pinned" in the name
|  | c.go  (preview)  [..][x] |  |
|  [Close others]                |
+--------------------------------+
```

- Opened by the `[N]` button in the top bar. Tapping a row switches and dismisses. Swipe left on a row closes it (button equivalent always present). **Pin and unpin on touch**: each row has a 44x44 actions button `[..]` (and long-press opens the same menu) with **Pin / Unpin**, **Close**, **Close others**, **Copy path**; pinned rows carry a pin glyph and the word "pinned" in the accessible name (preview rows "preview"), never italic or colour alone. Double-tapping a row's title also toggles preview to pinned. When the cap refuses an open (ux 5), the sheet opens with the message pinned to its top and the eligible rows (`[..]` > Close) highlighted.
- Focus trapped; Esc / system back / `x` dismisses; `prefers-reduced-motion` removes the slide animation.
- Errors: closing a dirty row shows the Discard alert dialog above the sheet (the one permitted stacking, section 9.1a; the sheet stays open underneath). Empty list shows "No open files - open one from Files" with a **Go to Files** button (exit).

## 7. S5 Layout shell and navigation

### 7.1 Mobile (nav-stack)

```
files pane           editor pane                    search pane
+-----------+        +--------------------------+   +------------------+
| Files [3] |        | <  b.go*  [3]  [...]     |   | <  Search        |
| tree      |        |                          |   | query / results  |
|           |  tap   | code (read mode)         |   |                  |
|           | -----> |                   [Edit] |   |                  |
|           |        |            [3/27 ^ v x]  |   |                  |
+-----------+        | Files|Search|Open|Go|<  > |   +------------------+
| action bar|        +--------------------------+
+-----------+
```

- Top bar 44px, left to right: back arrow `<` (pane back; section 7.4), filename with dirty dot (left-truncated to keep tail), `[N]` tab count, **Save** (shown whenever the active buffer is dirty, in Read or Edit mode, with or without the soft keyboard; in Edit mode it is shown even when clean, `aria-disabled` with the reason; icon plus dirty dot, accessible name "Save, unsaved changes"; spinner while saving; it is the same single save path as every other trigger), **Stop editing** (Edit mode only; a 44x44 icon button, accessible name "Stop editing" or "Stop editing (unsaved)" when dirty, never "Done"; it sits immediately right of Save, so the two never reorder), overflow `[...]` (Save, Stop editing/Edit, Wrap, Vim, Keyboard: Auto/Touch/Hardware, Accessible view, Font A-/A+, Download, Open in new tab, Shortcuts). Save and Stop editing are therefore reachable with the keyboard dismissed; the extra-key `Save` key is a convenience, not the only mobile path. At 320px the fixed targets are back 44 + `[N]` 44 + Save 44 + Stop 44 + overflow 44 = 220px, leaving 100px for the filename (minimum 72), so everything fits; when the filename would fall below 72px (a wider system font), `[N]` moves into the overflow menu first (Save and Stop editing never move).
- **Disabled Save always says why**: a disabled Save keeps focus and `aria-disabled`, and its reason is both visible and programmatic: the reason string ("No changes", "Saving...", "Offline - edits kept locally", "Server unreachable - edits kept locally", "Read-only: Truncated", "Session paused") is shown as the top banner row (priority 5/6 banners) or the status chip where those exist, is the target of the button's `aria-describedby`, and is announced politely when the disabled Save is tapped or Enter is pressed on it (nothing silent).
- Bottom action bar 48px plus `env(safe-area-inset-bottom)`: Files | Search | Quick-open | Go-to-line | Back/Forward (the jumplist, section 7.4). Hidden while the keyboard is open; the extra-key bar replaces it. **Narrow rule (< 360px)**: six 44px targets do not fit, so the bar shows icons only (labels become `aria-label`/tooltips) and collapses to **Files | Search | Quick-open | Back | More**; **More** opens a small popover holding Go-to-line and Forward (Back stays on the bar because "return to where I was" is a primary journey; the top-bar arrow also pops the pane). At >= 360px all five groups show with Back/Forward as one split control. Checked at 320x568 by U-MOB-9.
- **Floating controls stack** (right edge, above the bottom bar, Read mode only): the quickfix chip sits directly above the bar (8px gap); the Edit FAB sits 8px above the chip when the chip is shown, and drops to the 8px-above-bar position when the chip is dismissed. Both are 44px minimum and never overlap each other, the bar or the extra-key bar (both hidden while the keyboard is up; in Edit mode the FAB is replaced by the keyboard-bound controls). The editor reserves `padding-bottom` equal to the stack height (up to 104px) so the last lines can scroll clear of the floating controls. At 320x568 with the chip and FAB both shown the stack is 96px, leaving > 400px for code.
- Browser back mirrors the in-app pane back arrow and does not leave the session; at the root pane it follows normal browser behavior. The three back controls and their exact scopes are specified once in section 7.4.
- Edge-swipe back is an enhancement inside the installed PWA only; the arrow is always primary.
- Landscape and short-viewport editing: see 7.1a. The old rule "top and bottom bars hidden, Save at the end of the scrolling key row" is replaced: Save and Stop editing are never inside a scrolling row.

### 7.1a Edit-mode layout budget, minimum editor height and fallback ladder (triad round 2)

**Named viewports** (so the criteria are testable): `XS-568` = 320x568 portrait; `S-844` = 390x844 portrait; `L-390` = 844x390 landscape (a phone: it gets the mobile layout and this ladder by the section 0 layout rule, not the desktop layout its 844px width alone would suggest); `XS-L` = 568x320 landscape. **Keyboard assumption** for the budget: soft keyboard 260 CSS px tall portrait, 200 landscape (a Phase 0 spike measurement on the owner's phone replaces this; Story 0.1.5). Visual viewport with the keyboard up: XS-568 = 308px, S-844 = 584px, L-390 = 190px, XS-L = 120px.

**Minimum editor height**: target **120px** (about 5 lines at 16px / 1.4); hard floor 64px with the hint in step 5. The layout is computed by one pure function `editLayoutBudget(viewport, keyboard, inputProfile, flags)` that returns which rows are shown; `inputProfile` is the **resolved** `InputProfile` (8.8, including the manual override) plus the explicit "Show key bar" choice, because the key row, the vim row and the Save/Stop cluster exist only while the key bar is shown (see "Profile changes mid-edit" below).

Rows in Edit mode with the keyboard up, in priority order (portrait): top bar 44 (Save, Stop editing, overflow) > one banner row 44 (blocking banner text with its action buttons, or the "N notices" chip; 44 because the row carries 44px action targets, U2 of triad round 3) > editor (flex, min 120) > key bar main row 44 > vim row 44 (vim on) > status bar 24 (hidden on mobile in Edit; its chips live in Overflow > File info).

| Viewport (keyboard up) | Rows shown | Editor height |
|---|---|---|
| S-844, vim off | top 44, banner 44, key 44 | 584 - 132 = 452 |
| XS-568, vim on, 2 banners | top 44, banner row 44 (1 line + "+1 more"), key 44, vim row 44 | 308 - 176 = 132 (>= 120) |
| XS-568, vim on, keyboard 320px tall (viewport 248) | step 1 fires | see ladder |
| L-390, vim off | step 3 fires | see ladder |

**Fallback ladder** (apply in order until the editor region is >= 120px; each step is reversible when room returns):
1. Merge the vim row into the main key row (the keys `:` `/` `h` `j` `k` `l` scroll with the rest; Esc stays first).
2. Collapse the banner area to one 44px row. With a priority-1 (blocking) banner the row is: truncated one-line text (tap expands it to the full text in place, and the full text is always the row's accessible name), then **Copy my version** and **Retry Save** as two 44x44 icon buttons with text accessible names, in that order, so the data-loss escape is always one tap away even at 320px (320 - 88 for the two buttons leaves 232px for the text). Without a blocking banner the row collapses to an "N notices" chip inside the top bar (visual 24px, hit area 44x44, so the top bar stays 44px). A blocking banner is never collapsed away and its two actions are never moved into a menu (U-EDIT-15, U-MOB-14).
3. **Auto-hide the top bar** (only while the key bar is shown, see "Profile changes mid-edit") while the keyboard is up and the editor is still < 120px, and move **Save and Stop editing into a fixed, non-scrolling cluster at the right end of the key bar** (two 44x44 buttons, 88px, outside the horizontally scrolling key list; the list scrolls in the remaining width). The reveal handle (44px tall, visible, at the top edge) still opens the overflow menu. Dismissing the keyboard brings the top bar back, and the cluster leaves with the key bar, so **there is no state with a dirty buffer in which neither a Save nor a Stop editing control is visible** (U-MOB-11, U-MOB-14).
4. Hide the status bar and the quickfix chip.
5. If the region is still < 120px (L-390 or XS-L with a tall keyboard), keep the cluster and the Esc/Tab/arrow keys only, show a polite one-time status "Little room - rotate to portrait or dismiss the keyboard for more space", and accept the 64px floor; nothing else is removed (Save and Stop never are).

**Profile changes mid-edit.** The cluster of step 3 lives on the key bar, so step 3 is available only while the key bar is shown. When the resolved profile is `HardwareKeyboard` (and "Show key bar" is off) there is no key bar, no vim row and no cluster: `editLayoutBudget` skips steps 1 and 3, the top bar is never auto-hidden, and Save and Stop editing stay in the top bar. If detection or the override flips the profile to `Hardware` while the top bar is auto-hidden, the key bar and the top bar change in **one layout pass** (the top bar returns in the same render that removes the key bar, so there is no frame and no state with a dirty buffer in which neither Save nor Stop editing is visible), the one-time status of 8.8 shows, and the editor region is recomputed (a hardware keyboard usually means no soft keyboard, so the full height is normally available). The reverse flip (override to `Touch`, or "Show key bar") re-runs the ladder with the key bar present and may hide the top bar again once the cluster has mounted. Tested by the matrix of Task 2.4.2c with `inputProfile` as a dimension and by a jest step-through of Touch -> Hardware -> Touch with the top bar hidden (U-MOB-17).

### 7.2 Desktop

Left panel Explorer|Search (resizable; arrow-key resize via `role="separator"` + `aria-valuenow`; Ctrl+B collapses), center tab strip + breadcrumb + editor, bottom status bar. Toolbar back/forward arrows (**Alt+[ / Alt+]**; Ctrl-o/Ctrl-i with vim). Alt+Left/Right are the browser's history shortcuts and are deliberately not intercepted (14.1).

| State | What the user sees | Exit path |
|---|---|---|
| No file open | Empty center: "Open a file" with **Quick open (Ctrl+P)** and **Search (Ctrl+Shift+F)** buttons. | Either button. |
| Jumplist empty | Back/Forward disabled with `aria-disabled` and tooltip "No earlier location". | n/a |
| Jumplist target file gone | Skip entry, status "Skipped missing file b.go". | Continue pressing Back. |
| Router race on pane push | Layout falls back to in-memory pane state without URL change. | Controls still work. |
| Session paused/stopped while viewing | Banner "Session paused - files are read from disk and may be unavailable"; dirty buffers keep drafts (section 11). | Resume session link; copy content. |
| Layout rule result changes (container crosses 768px, or a desktop window is resized; a phone rotating does not change it, section 0) | State preserved; layout swaps without remounting the editor; keyboard-open mobile editing stays in the editor pane. | n/a |
| Archived session with a dirty buffer | Archiving does not delete the open buffer. The draft is written synchronously (same path as a paused session), banner (priority 1 if a save is attempted) "Session archived - saving is disabled. Your unsaved draft is kept locally."; Save is `aria-disabled` with that reason and the server refuses a write (`HiddenSession` or `WorktreeMissing`, whichever the Task 0.1.3b recon records); **Save never un-archives or restarts the session**. | **Copy my version**, Keep editing, restore the session from the session list (the existing explicit action) and then Save. |

### 7.3 Tablet (desktop layout, coarse pointer, soft keyboard)

The desktop layout and the touch input profile are independent axes. A tablet is a device whose container is >= 768px **and** whose short screen side is >= 600px (section 0), for example 820x1180 or 1180x820 with `pointer: coarse`; a landscape phone (844x390) is not a tablet, it keeps the mobile layout. A tablet gets the **desktop layout** (tab strip, Explorer|Search panel, breadcrumb toolbar, status bar) with **touch sizing and the touch input profile**:

| Control | Phone (mobile layout, incl. landscape) | Tablet, soft keyboard (desktop layout, coarse, Touch profile) | Tablet + hardware keyboard / desktop (Hardware profile) |
|---|---|---|---|
| Layout shell | nav-stack panes | desktop split layout | desktop split layout |
| Target size | 44px | 44px (keyed on `pointer: coarse`, not width) | 24px minimum |
| Tab navigation | tab sheet `[N]` | tab strip (close x 44x44, long-press menu with Pin/Unpin) | tab strip |
| Save | top bar | breadcrumb-row Save, always visible while dirty (keyboard dismissed reachable) | same |
| Stop editing | top bar | breadcrumb-row button | same |
| Edit entry | floating Edit FAB | breadcrumb-row Edit button (no FAB, no bottom bar) | same |
| Quick open, Go to line, Search | bottom bar | breadcrumb-row toolbar buttons **Quick open**, **Go to line** and **Search** (44px; there is no bottom bar and a soft keyboard has no Ctrl chords, so without them these three actions would be unreachable by touch) | chords Ctrl+P, Ctrl+G, Ctrl+Shift+F plus the same toolbar buttons (24px) |
| Extra-key bar | shown, scrolls if needed | shown, pinned to `visualViewport` bottom; all keys fit without scrolling at >= 768px; vim row only with explicit vim opt-in | hidden |
| Vim default | off | off (opt-in) | on at Edit entry (8.8) |
| Status bar | compact chips in Overflow | desktop status bar with 44px chips | 24-28px bar |
| Back controls | 7.4 | toolbar Back/Forward + pane model not used | same |

The extra-key bar is keyed on the resolved `InputProfile`, never on viewport width. Rotating a tablet across 768px swaps the layout without remounting the editor or dropping the keyboard.

### 7.4 One Back model: three controls, three scopes (triad round 2)

| Control | Where | What it moves | Never does |
|---|---|---|---|
| **Jumplist Back/Forward** | desktop toolbar and `Alt+[` / `Alt+]`; mobile bottom-bar Back/Forward split control; vim `Ctrl-o` / `Ctrl-i` | the `NavEntry` stack: previous/next **location** (file + line + scroll), across files | change the mobile pane or the browser history length beyond its own entries |
| **Pane back** | mobile top-bar `<` | pops `PaneState` (editor -> previous pane: `files` or `search`), restoring that pane exactly (scroll, query, expanded groups) | change the jumplist |
| **Browser / system back** | browser Back, Android gesture/button, PWA edge swipe | pops the pushed pane history entry exactly as pane back does; at the root pane falls through to normal browser behavior (leaves the Files tab route only from the root pane) | leave the session while a pane entry is on the stack |

Rules: (1) opening a hit from the search pane **pushes the editor pane over the search pane** (history entry) and records one jumplist entry for the location the user left; (2) therefore, after opening a hit, `<` / system Back returns to the **search results** (scroll, query, toggles, collapsed groups preserved) while the bottom-bar Back returns to the **previous location** (which may be another file) and does not show results; the two are intentionally different and both labelled (tooltip/aria-label "Previous location" vs "Back to results" when the previous pane is search); (3) stepping through hits with the chip or F4 updates the current location without pushing a pane or a new jumplist entry (only the first hit of a search records the origin); (4) Back/Forward at the ends are `aria-disabled` with "No earlier location"/"No later location"; (5) desktop has no panes: **Ctrl+Shift+F** returns focus from the editor to the query field of the results panel (which never closed; Esc is not used in this direction because in the editor Esc belongs to the editor, 8.4), Esc in the panel returns focus to the editor, and `Alt+[` is the only history control; (6) **Android system Back with the soft keyboard up**: on Android the first Back press dismisses the keyboard and is consumed by the browser (the page gets no `popstate`; UNVERIFIED on the owner's device, checked by a line in the Task 2.5.4a script), so the app never treats a keyboard dismissal as navigation: the pane does not change and Edit mode, the draft and the buffer are untouched; the second Back pops the pane as in the table. The app does not push or pop history entries in response to the viewport resize. In the installed iOS PWA there is no system Back; the top-bar `<` is the only pane control.

## 8. S6 Editor

### 8.1 Modes and entry

- **Read mode** (default): CodeMirror read-only. On touch, content is not focusable (`inputmode="none"`), so tapping to read does not raise the keyboard.
- **Edit mode**: requires `files:editing` and `ReadOnlyReason=None`. Desktop: **Edit** button in the breadcrumb row (or `i` / click when already focused with vim). Mobile: floating 44px **Edit** button (bottom-right, above the bottom bar). Entering Edit on mobile focuses the editor at the last tap position (when the last interaction was a tap in the code) and otherwise at the first visible line, raises the keyboard plus extra-key bar, and announces "Editing b.go, line N" politely; with a screen reader active the focus target is the line last read in the Accessible view (8.10), never an off-screen position.
- **Entering Edit pins the tab (triad round 3)**: a preview tab is promoted to pinned at the moment its buffer enters Edit mode, not on the first keystroke, so a clean buffer in Edit can never be the tab that the next single-click or the tab cap replaces (the cap evicts only a clean preview, section 5); the invariant "a dirty buffer is always pinned" still holds because Edit entry precedes any edit. Stop editing does not unpin it (Unpin is the user's explicit action).
- **Edit mode scope**: Edit mode is **per buffer** (each tab keeps its own Read/Edit mode), not per session or global. A newly opened or reloaded buffer starts in Read; switching tabs preserves each tab's mode; **Stop editing** affects only the active buffer; a restored draft opens in Edit; turning `files:editing` off drops every buffer to Read (draft kept). Mode is not persisted across a page reload. A buffer in Edit that is evicted/closed goes through the Discard dialog if dirty (it is pinned, so it is never evicted).
- **Edit when the file cannot be edited**: with `files:editing` on, a file with any `ReadOnlyReason` (Truncated, TooLarge, Binary, NonUtf8, MixedEol, Symlink, WorktreeMissing, HiddenSession, GitInternal) shows the Edit control **disabled, never absent**, with the reason as visible text on or beside it (desktop: the button label "Edit" plus the chip "Read-only: Truncated"; mobile: the FAB is rendered `aria-disabled` and tapping it opens a small popover with the reason and the available action, for example **Download**). With `files:editing` off the Edit control is absent (flag-off is not a per-file reason).
- Leaving Edit: the button reads **Stop editing** (desktop and mobile; never "Done", which reads as "saved"). It returns to Read mode and keeps the draft; when the buffer is dirty the label becomes **Stop editing (unsaved)** and a polite message "Stopped editing; N unsaved changes kept. Save to write them." is announced; the dirty dot and the Save affordance stay visible in Read mode. Exit does not save and never prompts.
- **Typing in Read mode (desktop)**: when `files:editing` is on and the file is editable, a printable key, Enter, Backspace or Delete pressed with focus in the read-only editor (not a vim motion in Normal mode, not a shortcut chord) shows an inline hint anchored to the breadcrumb row, `role="status"`, no focus move: "Read-only. Press E or click Edit to start editing" with an **Edit** button. **`E`** (a single key, only while that hint is showing or the editor is focused in Read mode with vim off) enters Edit mode and replays nothing (the keystroke that triggered the hint is not inserted). The hint shows at most once per tab per 10 s and auto-hides after 4 s; when `files:editing` is off or the file has a `ReadOnlyReason` the hint instead shows the reason chip text ("Read-only: Truncated"). With vim on, `i`/`a`/`o` already enter Edit (existing rule), so the hint is suppressed for vim keys.

```
Desktop edit mode:
| pkg > b.go                    [Outline v] [Save] [Stop editing] |
| 1  package pkg                                                  |
| 2  ...                                                          |
| NORMAL | Ln 2, Col 5 | UTF-8 | LF | Go | Modified | Saved 10:42 |
```

### 8.2 Banners (stack at top of editor; `role="status"` except a priority-1 banner, which is `role="alert"`; never steal focus; at most 2 visible, rest collapse to "+N more")

**Priority order (highest first; the stack sorts by this, then by newest)**: 1 Save rejected / Save outcome unknown / **Draft not stored while dirty** (all blocking) > 2 Deleted > 3 Flag off mid-session > 4 Stale with unsaved edits > 5 Offline > 6 Truncated / Rejected / Binary / Non-UTF-8 (permanent read-only reasons) > 7 Beyond loaded portion > 8 Stale (clean) > 9 Large-file degraded / Directory-mode / Syntax unavailable (informational). **Rule: a blocking banner (priority 1) is never collapsed into "+N more" and is never hidden behind an informational banner**; if more than 2 banners compete, the lowest-priority ones collapse first, and "+N more" is a button that opens the full list. A priority-1 banner uses `role="alert"` (assertive, announced once) while the others stay `role="status"`; it has no **Dismiss**: it persists until a save succeeds or the buffer is no longer dirty (for a draft-not-stored banner: or until a backup write succeeds), and its actions are **Copy my version** and **Retry Save** (**Retry backup** for the draft-not-stored variant; Keep editing simply leaves it showing above the still-editable buffer). When the stack is collapsed to one row on a short screen (7.1a step 2) these two actions stay on that row as 44x44 buttons. Informational banners dismiss individually and re-appear only on their trigger.

| Banner | Text | Actions (exit path) |
|---|---|---|
| Stale (clean buffer, disk changed) | "Changed on disk - Reload" | **Reload** (keeps cursor line and scroll), **Dismiss** (re-appears only on next change). |
| Stale with unsaved edits | "Changed on disk - review diff" | **Review** (opens S7), Dismiss. |
| Truncated | "Read-only: showing first 1 MB of 3.2 MB" | **Download**, **Open in new tab**; editing impossible by design. |
| Rejected (> 10 MB) | "File too large to display (12.4 MB)" in place of editor | **Download**, **Open in new tab**, Close tab. |
| Binary | Placeholder: type, size | **Download**, Close tab. |
| Non-UTF-8 text | "Read-only: unsupported encoding (<name>)" | Download, Close tab. |
| Flag off mid-session | "Editing was disabled; your unsaved changes are kept" | **Copy my version**, Dismiss; draft retained. |
| Beyond loaded portion | "Line 40000 is beyond the loaded portion" (view clamps to last line) | **Download**, Dismiss. |
| Large-file degraded (coarse pointer) | "Large file: syntax highlighting off" | Dismiss. |
| File deleted | "File no longer exists on disk" | Close tab; if dirty: **Copy my version**, Keep editing (Save disabled with reason). |
| Offline / server unreachable | Banner (priority 5) "You are offline - edits kept locally" or "Server unreachable - edits kept locally" (detection in 8.11; also the status-bar chip on desktop) with **Retry now** | Auto-clears on the next successful RPC or `online` event; Save disabled meanwhile with this text as its visible reason. |
| Session archived, dirty buffer | "Session archived - saving is disabled. Your unsaved draft is kept locally." (priority 1 once a save is attempted, else 5) | **Copy my version**; restore the session explicitly. |
| Directory-mode session (no dedicated worktree) | Status-bar chip "Editing the main checkout" (informational; no dedicated worktree) | Dismiss; owner can veto in ADR-031 |
| Draft not stored while dirty (priority 1) | "Draft not stored: <too large for backup | storage full | storage unavailable>. Your edits exist only in this tab." Shown whenever the buffer is dirty and neither the localStorage draft (<= 256 KB) nor the IndexedDB backup (<= 1 MB, 11.3) has been written successfully; it replaces the lower-priority "Not backed up" chip text. iOS may discard a backgrounded PWA, so this is a data-loss warning, not a notice. | **Save** (top bar, emphasised), **Copy my version**, **Retry backup**; clears when a backup write lands or the buffer is saved. |
| Save rejected (path confinement / read-only mount / permission / any `ReadOnlyReason`) | "Could not save: <reason class>" (no raw paths of other users) | **Copy my version**, Keep editing, retry Save. |
| Save outcome unknown | "Save status unknown - checking..." then resolves to Saved or conflict | Automatic re-stat; Retry. |
| Syntax load failure | Plain-text fallback, no banner needed beyond status "Syntax unavailable" | n/a |

### 8.3 Save flow

| Step | User | System |
|---|---|---|
| 1 | Ctrl+S / `:w` / Save key / Save button | Single save path; button shows spinner, a second trigger is ignored. |
| 2 | (success) | "Saved" announced (polite); draft cleared; undo history kept; dirty dot clears; VCS status refreshed. |
| 3 | (conflict) | Opens S7. |
| 4 | (offline) | Save disabled before any request; nothing queued. |
| 5 | (reconnect, then Save) | `StatFile` first; changed token routes to S7. |

### 8.4 Keyboard and focus (desktop)

- Editor has `aria-label="File: <path>"`, `aria-readonly` in Read mode.
- **No keyboard trap, and one Escape rule per context (triad round 3; replaces the earlier text that gave Esc opposite meanings in sections 2, 3.3 and 7.4).** Esc is overloaded, so what it does depends only on where focus is and whether vim is on; the table is the single source and every other section refers to it:

| Focus is in... | Single Esc | Esc, Esc (two presses within 500 ms) |
|---|---|---|
| A dialog, sheet, popover, palette or the shortcut overlay | Closes that top layer to its safest option (section 0) | Same as single; the second press hits whatever is underneath only after the layer has closed, so **a double Esc never closes a layer and then acts on the editor** (the second press is swallowed while a layer was closing) |
| The search query field or results (desktop) | Focus returns to the editor; the panel stays open | n/a |
| A focused quickfix chip | Dismisses it | n/a |
| The editor, vim off | Passed to CodeMirror (closes completion or its own panels); no navigation. **Esc then Tab** moves focus out of the editor (WCAG 2.1.2; Tab is not bound) | Nothing special |
| The editor, vim on | Vim's own: returns to Normal, cancels an operator or the command line. **No navigation.** | **Leaves the editor** only if all of: vim was already in Normal mode with no pending operator, count or register when the first Esc arrived; the first Esc made the status bar show the hint chip "Esc again to leave the editor" (visible 1.5 s, polite announcement); the second Esc arrives while that chip is visible; and the per-device setting "Esc, Esc leaves the editor" (Overflow and the shortcut overlay, default on) is on. From Insert, Visual or the command line the first Esc is the usual return to Normal and shows no chip, so leaving takes up to three presses |
| The status-bar **Leave editor** control (reached by Esc, Esc or by pointer) | **Returns focus to the editor exactly as it was** (cursor, scroll, vim Normal mode); Enter or Space on the control does the same; Tab moves on through the status bar | n/a |

  *Why a hint chip and an undo*: a habitual double Esc (clearing a search highlight, "make sure I am in Normal mode") must not strand the user. Leaving is therefore never destructive and costs one Esc to undo, it announces itself (the chip) before it happens, and users who double-tap Esc by habit can turn the setting off; with it off the keyboard routes are `:leave` and the other routes below.
  *Routes out of the editor with vim on* (Tab is a vim key in Normal mode, so **Tab order does not reach the status bar from inside the editor** and the earlier claim that the control is "reachable by Tab order from the editor" was wrong): (1) Esc, Esc as above; (2) the ex command **`:leave`** (also `:lea`), registered through the same handler as the status-bar control and available in every setting state; (3) pointer or touch on the **Leave editor** control; (4) screen-reader or virtual-cursor navigation to the control; (5) Esc then Tab applies only with vim off. These are listed in the shortcut overlay and in a one-time in-editor status when vim is first turned on ("Press Esc twice, or type :leave, to leave the editor"), so the exit is advertised (WCAG 2.1.2 allows a non-standard exit method only when the user is told). No modifier chord is used: Ctrl+Shift+M was removed because Chrome binds it to the profile menu and Firefox to Responsive Design Mode (14.1). **Guarantee**: the Esc, Esc handler is a capture-phase keydown handler on the editor DOM registered after the vim extension and re-registered on every vim reconfigure, so neither vim's own handling nor a user `:map`/`:nmap <Esc>` can consume or remap it, and `:leave` is defined with `Vim.defineEx` after the user's rc; it works in Normal, Insert, Visual and command-line modes (AC in plan Story 2.4.1; the test runs `:nmap <Esc> x` and `:imap <Esc> x`).
  The direction editor -> results panel on desktop is **Ctrl+Shift+F** (7.4 rule 5), not Esc.
- A preview tab is promoted to pinned when its buffer enters Edit mode (8.1) and is never replaced by opening another file (plan Story 1.2.2); the dirty-implies-pinned invariant follows from it.
- Font size stepper (A-/A+) in overflow on mobile; >= 16px on coarse pointers to avoid iOS zoom-on-focus.

### 8.5 Loading and pending states

Rule: any wait longer than 150 ms shows a visible, announced pending state in the place the content will appear; any failure shows a message with an exit. Nothing renders as a blank pane. Skeletons honor `prefers-reduced-motion` (no shimmer).

| Moment | Pending state (after 150 ms) | Failure state | Exit path |
|---|---|---|---|
| Open a file (`GetFileContent` in flight) | Tab appears immediately with the file name and a spinner; editor area shows a line-skeleton and "Opening b.go..." (`aria-busy="true"` on the panel, polite announce once). The previous tab's content is not shown under the new name. | Same tab shows "Could not open b.go: <class>" (not found / too large / network / worktree unavailable) and the editor stays unmounted for that buffer (the `Buffer` is `Failed`). | **Retry**, **Close tab**; not found also offers "Reveal in tree". |
| `GetFileContent` fails for a file that is already open in Edit (reload after conflict, restore) | Spinner on the Reload/Restore control only; the editor content is untouched. | Banner (priority 1 if the buffer is dirty, else 8): "Could not reload: <class>"; the buffer stays exactly as it was, dirty state preserved. | **Retry**, **Copy my version**, Keep editing. |
| Tree load / refresh | Tree area shows 8 skeleton rows and "Loading files..." on first load; on refresh the old tree stays and a small spinner shows in the toolbar. | "Could not load files" with the error class in place of the tree; last good tree is kept on refresh failures. | **Retry**; Quick-open still works from cache. |
| Tab restore on mount (localStorage -> buffers -> `StatFile` per visible tab) | Tab strip renders the persisted tabs immediately (dimmed, spinner dot) while only the **active** tab is fetched; others fetch on activation. Announce once: "Restoring N tabs". | Per tab: "not found" (existing row in section 5) or "Could not restore: <class>" with the tab still closable. | Select retries; Close tab; Close all missing. |
| Outline load (parse in a worker/idle callback) | Outline opens with "Reading symbols..." (spinner) when parsing exceeds 150 ms. | "No symbols found for this file type" (existing) or "Could not read symbols" if the parser throws. | **Close**; Go-to-line link. |
| `StatFile` pending (freshness check, pre-save re-stat, post-offline Save) | Silent for the freshness poll (no UI churn). For an explicit Save/Reload it shows the control spinner plus the message "Checking the file on disk..." (polite). | "Could not check the file: <class>"; Save stays enabled for retry, nothing is written. | **Retry**; Keep editing. |
| Draft check on open (localStorage read, `Draft` token compare) | Synchronous in practice; if it exceeds 150 ms the Restore prompt is deferred and the editor opens editable without it, then the prompt appears when ready (never blocks typing). | Storage unreadable: nothing is offered, one-time status "Drafts unavailable"; editing continues. | n/a |
| Merge/diff library lazy load | Dialog opens at once with the plain unified diff text and "Loading merge view..." on the Merge button. | Existing "Merge view unavailable - try again" row (section 9.3). | Retry. |
| Save in flight | Existing: Save control spinner, second trigger ignored. | Existing Save outcomes. | Existing. |

### 8.6 Success feedback

Successes are `polite` messages in the status-bar message region (auto-clear after **8 s**, section 13) plus an on-screen cue for users who cannot see the status bar (mobile shows a transient toast above the bottom bar or extra-key bar, `role="status"`, never over the Save control). **Toast timing**: at least 8 s, paused while hovered or focused, a visible dismiss control, no motion under `prefers-reduced-motion`; a toast offering **Undo** (tab eviction undone, and any future Undo) stays until dismissed or 12 s after the last hover/focus, whichever is later, and the Undo remains available from the tab list's Recent files after the toast is gone:
- **Saved**: "Saved b.go" (existing), dirty dot clears.
- **Merge saved**: "Merged and saved b.go" (the "Review merged result" step, when required, happens before this save).
- **Reload from disk**: "Reloaded b.go from disk" with the cursor line preserved.
- **Draft restored**: "Restored your unsaved draft of b.go" and the buffer is dirty with the Save affordance showing.
- **Overwrite**: "Overwrote b.go on disk with your version".
- **Evicted tab undone**: "Reopened c.go".
Errors keep their existing persistent behavior.

### 8.7 Concurrent agent write journey

The owner watches an agent work in the same worktree while reading or editing. States and what the user sees, in order:

| Step | Trigger | UI | Timing |
|---|---|---|---|
| 1 Detect | The active buffer is re-stat'd on window focus, on `visibilitychange` to visible, and by a low-frequency poll **every 10 s while the Files tab is visible and focused** (active tab only; paused when hidden or offline; one request in flight; back-off to 30 s after 3 failures). Other open tabs are checked on activation. | none | A changed `sha256` segment is the only change signal; mtime-only changes are adopted silently (Story 1.3.4). |
| 2 Debounce | A change must be seen on **two consecutive checks or persist 1 s** before the banner shows, so a file being rewritten in bursts (formatter then agent) shows one banner, not a flicker. The banner text is updated in place if the token changes again while it is open. | none | 1 s |
| 3 Inform (clean buffer) | Banner priority 8 "Changed on disk - Reload"; if the session is in a running state the suffix reads "(session is running; this may be the agent)", otherwise "(source unknown)". **The actor is never asserted**: `agent_likely` is inferred only from the session's existing running status; there is no per-file attribution. | `role="status"`, no focus move | Banner stays until Reload/Dismiss; re-appears only on a newer change. |
| 4 Inform (dirty buffer) | Banner priority 4 "Changed on disk - review diff"; editing continues uninterrupted; saving later routes to the conflict dialog. | same | |
| 5 Act | Reload (clean) keeps cursor line and scroll; Review opens S7 with your edits vs. the current disk text. | | |
| 6 Agent writes after the user saved | The saved buffer is clean with the new token, so step 1-3 apply: the user is told the agent changed their saved edit (the previously deferred pre-mortem #7 case is covered for the **active** tab by this poll; non-active tabs on activation). | | |
| 7 File deleted/renamed by the agent | "File no longer exists on disk" (priority 2). | | |

### 8.8 Input profile, hardware keyboard and vim defaults

`InputProfile` is resolved in this fixed precedence (first match wins): (1) the **persisted per-device manual override** (Overflow > "Keyboard: Auto / Touch / Hardware"; always wins, including over any detection and over a later `pointer: fine` change); (2) **auto-detect**, deliberately conservative; (3) otherwise **`Touch`** (the default).

**Auto-detect signals (triad round 2; the first design, "first non-229 non-modifier keydown", was dropped because soft keyboards emit Enter, Backspace, Space and Tab with a normal `keyCode`, so it false-positives within seconds of normal phone typing).** Only `isTrusted` events count (the extra-key bar's synthetic events never do). Auto-detect resolves to `HardwareKeyboard` when **any** of the following holds, and to `Touch` otherwise:
- **S1** the *primary* pointer is fine and hover-capable: `matchMedia('(pointer: fine) and (hover: hover)')` matches (`any-pointer` is not used alone: a phone with a stylus or a connected mouse would flip). A touchscreen laptop with a mouse is therefore Hardware; a phone or tablet is not.
- **S2** a trusted `keydown` with `ctrlKey` or `metaKey` held together with a non-modifier key (soft keyboards cannot produce chords).
- **S3** a trusted `keydown` of a key that soft keyboards do not have, with a non-empty physical `event.code`: `Escape`, `F1`-`F12`, `Insert`, `Home`/`End`/`PageUp`/`PageDown`, or an arrow key; or **three** trusted letter keydowns whose `event.code` is `Key<L>` with `event.key` equal to that letter and `keyCode !== 229`, `isComposing === false`. `Enter`, `Backspace`, `Space`, `Tab`, digits and punctuation **never** count on their own.
- **S4** (supporting only, never alone) when the editor is focused by a tap on a coarse-pointer device and `visualViewport.height` has not shrunk by at least 100px within 500 ms, the soft keyboard did not open; this counts only together with one trusted letter keydown matching S3's letter rule.

On detection mid-session (a Bluetooth keyboard on a phone): the key bar hides (and, if 7.1a had auto-hidden the top bar, the top bar returns with Save and Stop editing in the same layout pass) and a one-time status "Hardware keyboard detected - key bar hidden. Change in Overflow > Keyboard" shows; the flip is **not persisted** (only the manual choice is), so a disconnected keyboard does not leave the device stuck, and a later touch interaction does not flip it back (the override is the escape). Detection state resets on reload.

**Vim default is evaluated only at Edit entry.** When a buffer enters Edit mode the resolved profile at that instant decides the default (on for `HardwareKeyboard`, off for `Touch`) unless a **persisted explicit vim toggle** exists, which always wins; vim is never enabled automatically on a touch-resolved device. **Vim never flips on or off live while a buffer is in Edit mode**: if detection changes the profile during an edit, only the key bar hides or shows, and a one-time chip offers "Hardware keyboard detected - turn vim on? [Turn on] [Not now]" (nothing changes without the tap). The extra-key bar is shown only when the resolved profile is `Touch` (or the user chose "Show key bar"). The Phase 0 spike (Story 0.1.5) captures real `keydown` field sets from the owner's phone with and without a Bluetooth keyboard and the unit fixtures encode them (ADR-033).

### 8.9 Additional states (triad round 2)

| State | What the user sees | Exit path |
|---|---|---|
| Zero-byte file | Opens (not an error): the empty editor with a visible "Empty file" placeholder (not part of the document; `aria-label` says "empty file"), status `0 B`, `LF` shown as "LF (none detected)", Edit enabled like any text file. Typing makes the buffer dirty; saving writes the text with LF and no BOM; saving back to empty writes zero bytes and a no-op save of an empty file is byte-identical. The token hash is the sha256 of zero bytes (not the `-` of an unreadable file). | Normal Edit/Save. |
| A stale (disk changed again) change is detected while the conflict dialog or merge view is open | The freshness poll keeps running against the dialog's `theirs` token. A change seen twice (or persisting 1 s) refreshes the diff/merge **in place** with a polite "Disk changed again - diff updated" line (merge keeps the resolutions made for regions that did not change; section 10); the banner stack underneath is suppressed while the dialog is open and shows nothing on close if the dialog already reflected the newest token. Focus never moves. | Same four actions; the refresh is not a decision. |
| Archived session with a dirty buffer | Section 7.2 table. | Copy my version; restore the session explicitly. |
| Server unreachable (browser reports online) | Section 8.11. | Retry now; keep editing. |

### 8.10 S16 Accessible view (non-virtual read path for assistive technology; triad round 2)

**Why**: CodeMirror 6 virtualizes: only the lines near the viewport exist in the DOM, so a screen reader's explore-by-touch or virtual cursor cannot reach the rest of a file, and "focus at the last tap position" has no meaning for a user who has not tapped. Whether CM6's Read mode is sufficient with VoiceOver and TalkBack is not assumed; it is measured in the Phase 0 spike (Story 0.1.6) and the Accessible view is built regardless as the guaranteed path.

**What it is**: a plain, semantic, fully rendered representation of the active buffer, toggled per device from Overflow > "Accessible view" (also in the command palette, as a skip link "Switch to accessible view" that is the first focusable element inside the editor region, and persisted per device). It replaces the CodeMirror view in the same panel (the buffer, tabs, search and jumplist keep working); it is **read-only**. Structure: a `region` labelled "b.go, accessible view" containing a pager and a `<pre tabindex="0">` per page of up to **500 lines**, each line rendered as real text prefixed with its line number ("12  func foo") so a screen reader reads the number with the line. The pager is "Lines 501-1000 of 3200" with Previous page / Next page buttons (44px), a "Go to line" input, and `aria-live="polite"` page announcements. Pages (not the whole file) keep a 1 MB file to at most 500 lines of DOM.
- **Search hits and Go to line** open the page containing the line and move focus to that line's element (a `<span id>` with `tabindex=-1` inside the `<pre>`), announcing "Line 12".
- **A dirty buffer is shown as the draft, never as disk (triad round 3).** The Accessible view renders the buffer's current `EditorState` text, so it shows the user's unsaved edits, not the last saved version, and line numbers refer to that draft. When the buffer is dirty: the region's accessible name becomes "b.go, accessible view, unsaved changes"; the pager row shows a visible "Unsaved changes" label (text plus the dirty dot) and a **Save** button (44px, the same single save path as every other trigger; its disabled reasons follow 7.1), and a conflict, stale banner or offline banner appears in the normal banner stack above the region, so a screen-reader user can save without leaving the view and is told when the text is not what is on disk. Search hits computed from disk use the same "buffer has unsaved changes; line may differ" rule as 3.4. Edits themselves still require Edit.
- **Spike pass criteria (copied from plan Story 0.1.6 so this document is self-contained; the plan owns the wording)**: *R1 (Accessible view, required)*: with VoiceOver (iOS) and TalkBack (Android), swiping next from the first line of a page reads 50 consecutive lines in order with no skipped or repeated line and each announcement begins with the line number; the Next page button is reachable by swipe; Go to line 1,234 lands on the page containing it and reads "Line 1234" first. *R2 (Accessible view, required)*: heading/landmark or rotor navigation reaches the pager and the code region; the skip link "Switch to accessible view" is the first element reached inside the editor region by swipe from the file name. *R3 (CM6 Read mode, informational)*: swiping reaches line 1, a line in the viewport and then stops at the last mounted line; the record states whether off-screen lines are ever reachable (expected: no). *R4 (CM6 Edit, informational)*: typing echoes each character, moving the caret announces the character or line, IME composition does not duplicate text, and Delete/Backspace announce the removed character. R1 and R2 must pass or the Accessible view is redesigned before it is built.
- **Edit** is not available inside the Accessible view: activating Edit switches back to the CodeMirror view in Edit mode at the line last read (8.1) and announces the change; Edit with a screen reader is best-effort and its result is recorded by the manual script (Task 2.5.4b) and the spike (0.1.6), with failures filed.
- **Windowed (large) files (ADR-036, Task 1.6.5c)**: for a buffer served in windows the pager reads "Lines 1-500 of about 4.2 million" until the line count is exact, each page is one range read, a page that is loading announces "Loading lines 501-1000" politely and a failed page announces "Couldn't load lines 501-1000. Retry" with a 44px Retry; the view never depends on the editor engine or on which lines are mounted, which is why it stays the guaranteed screen-reader path for T1 and T2 files (U-LARGE-9).
- **Not a fallback for sighted users** and not shown by default; there is no auto-detection of a screen reader (browsers do not expose it reliably). The overflow item and skip link are reachable by keyboard and by explore-by-touch before any code is read.

### 8.11 Offline and unreachable detection (triad round 2)

`navigator.onLine` is only a hint: it is true on a captive-portal Wi-Fi, a dropped Tailscale tunnel, or a stopped server, and the previous design would have let Save fail instead of being disabled. The reachability state is the union of three signals: (a) `navigator.onLine === false` (state **Offline**, text "You are offline - edits kept locally"); (b) **any RPC network failure** (fetch `TypeError`, Connect `unavailable`/`deadline_exceeded`, TLS or connection reset) from a Files-tab call, counted once; and (c) a **heartbeat**: the 10 s visible-tab `StatFile` of the active buffer (section 8.7; it runs for dirty buffers too) failing twice in a row. Two consecutive failures from (b)/(c), or one failed explicit Save/`StatFile`, set **Server unreachable** (text "Server unreachable - edits kept locally"). Both states disable Save (reason visible, 7.1), persist the draft, queue nothing, and clear on the next successful RPC or `online` event; **Retry now** (popover on the chip and the banner action) runs `StatFile` immediately. The poll backs off to 30 s after 3 failures as before and a successful poll restores 10 s. Recovery always `StatFile`s before the next Save is allowed (section 8.3 step 5).

### 8.12 Large files: windowed read, gaps, indexing and view-only states (ADR-036 Phase B; Stories 1.6.4-1.6.6)

**When it applies**: `files:range-read` is on, the server advertises line reads for this listener, and the file is over 1 MiB. Tier T0 (up to 1 MiB) opens exactly as before. **T1** (over 1 MiB up to the hash cap, proposed 64 MiB) opens windowed: one range read paints the first window (or the window around the requested line), the rest of the document is represented by placeholders that load as the viewport approaches them, and the scrollbar is proportional to the whole file. **T2** (over the hash cap, up to the read ceiling, proposed 1 GiB) is windowed and always view-only. With the flag off, an old server, or a demoted transport, the base-plan behavior applies (a file over 1 MiB opens truncated and read-only with its reason chip) and nothing here is shown except the fallback chip. Line numbers in the gutter are always the true absolute line numbers; the status bar shows `Ln 1,234,567 of about 4.2 million` until the index finishes and the exact count afterwards.

| State | What the user sees | Announced | Exit path |
|---|---|---|---|
| Opening a large file | The 8.5 open state (tab with file name and spinner, "Opening b.log...", `aria-busy`), then the first window; the rest of the pane is placeholders, never a blank pane | Polite once: "Opened b.log, about 4.2 million lines" | Cancel the open by closing the tab |
| Gap loading | A placeholder block "Loading lines 4,001-4,400..." whose height is proportional to the gap; it cannot be typed into, partly selected for deletion or edited; reduced-motion users see no shimmer | Polite, once per burst of loads and only after 500 ms: "Loading lines..." | Keep scrolling; loads are abortable |
| Gap load failed | "Couldn't load lines 4,001-4,400. Retry" with a 44px Retry; the rest of the file stays usable | Polite: "Couldn't load lines 4,001-4,400" | **Retry**; scroll elsewhere; Reload |
| Far jump (Go to line, search hit, deep link beyond the loaded window) | After 300 ms an inline "Going to line 4,100,000..." with Cancel (44px); the target window loads, the line is centered within one line height and flashed; Go to line beyond the end clamps and says "File has about 4.2 million lines; went to the end" | Polite: "Line 4,100,000" on landing | **Cancel** returns to the previous position (the jumplist entry is created only on landing) |
| Far Go to line before the index exists | "Waiting for the file index..." with Cancel; the jump completes automatically when `analysis=ready`; a search-hit jump does not wait (it uses the hit's byte offset) | Polite once | Cancel |
| Indexing | A status-bar chip "Indexing file..." (spinner plus text, never color alone); the line count reads "about N lines"; **Edit is `aria-disabled` with the visible reason "Preparing file for editing"** | Polite on completion: "File ready to edit" | Wait; reading is unaffected |
| T2 view-only | Reason chip "Read-only: file too large to edit"; Edit `aria-disabled` with that reason; Download stays available | none | Download |
| Very long line (over 256 KiB) | The region renders in wrapped read-only windows with the reason chip "Read-only: very long line"; editing is off for the whole file | none | Download |
| Binary data found late | The Binary state of 8.1 replaces text paging with "This file contains binary data after line N"; Download | Polite once | Download |
| Changed on disk while open, or a torn read | A torn window is retried once silently and is never shown as current; a confirmed change raises the stale banner of 8.7 ("Changed on disk - Reload"); **Reload re-anchors the same line** and drops the chunk cache | Polite, as 8.7 | Reload, Keep reading |
| Transport demoted (three consecutive transport-class failures) | A quiet status chip "Using simpler file mode" (text) whose popover says "Large files open read-only and truncated in this mode. Reloading the page tries again." No dialog, no error toast | Polite once | Reload; dismissing the chip does not hide the file's own reason chip |
| Memory pressure / scrolled away | Clean chunks far from the viewport return to placeholders without moving the viewport by more than one line height and without losing selection or undo history; **a region with unsaved edits is never evicted** | none | n/a |
| Editing a T1 file (`files:edit-large` on) | On Edit entry a dismissible status banner "Large file: Save sends only your changes. Lines you can't see yet are loaded as you scroll." (once per device); typing into a placeholder is refused with a polite "Load these lines to edit them"; the Save and dirty rules of 8.3 are unchanged | Polite banner | Dismiss; Stop editing |

**Touch and memory**: on `pointer: coarse` windows are smaller and the chunk cache is 8 MiB (64 MiB on desktop); dragging the scrollbar shows a tooltip with the approximate line ("Line ~1,200,000") that is visual only (no live announcements while dragging); prefetch is one window ahead and behind and never opens a stream. **Accessibility**: the guaranteed read path is the Accessible view (8.10), which reads pages through the transport; the placeholders live in CodeMirror's virtualized DOM and are not relied on by assistive technology.

### 8.13 Delta saves: verification, rejection and recovery states (ADR-036 Phase C; Story 2.2.5)

A save sends only the edits (a patch) when the server advertises it and `files:delta-write` is on; otherwise it sends the whole file as in 8.3. The user-visible flow of 8.3 does not change (single Save path, one request in flight, "Saved" announced, dirty clears); the UI never shows payload sizes, offsets or transport names. The states below exist because a patch is verified before anything is written, and a wrong patch is refused, not applied.

| State | What the user sees | Exit path |
|---|---|---|
| Normal patch save | Identical to 8.3 | n/a |
| Checking (periodic reconcile every 20 patches and on blur) | A brief status chip "Checking file..." that never blocks typing and announces nothing unless the check fails | n/a |
| Patch rejected (offset or result mismatch) | Nothing was written and the buffer stays dirty. Blocking banner: "Couldn't verify your edit, so nothing was saved. Your changes are still here." T0 actions: **Save whole file**, **Copy my version**, Keep editing. T1 actions (no whole-file save exists for large files): **Copy my edits as patch**, **Reload (keep my edits)**, Keep editing | Any action; Esc keeps editing |
| Diverged (reconcile found the file and the client disagree) | Banner: T0 "Edits can no longer be verified. This file will be saved whole from now on." (deltas stop for the buffer, the next Save uses the whole file); T1 "Edits can no longer be verified. Reload to continue; your edits are kept." | Reload (keep my edits), Copy my version |
| Change too large for a patch (over 2,000 edits or 1 MiB inserted) | T0: polite status "Saving the whole file" and the save proceeds as a whole-file save; T1: Save is disabled with the visible reason "Change too large to save as a patch" and the actions **Undo** (to the last saved state) and **Copy my edits as patch**; nothing is ever split across requests | Undo, Copy |
| Preparing to edit (T1, analysis pending) | See 8.12 Indexing: Edit disabled with "Preparing file for editing" | Wait |
| Unknown outcome on a T1 save (response lost) | Banner: "Can't tell if the last save landed. Reload to check; your edits are kept as a draft." No automatic retry (a retry could double-apply) | **Reload (keep my edits)**, Copy my edits as patch |
| Offline with a ChangeSet draft | Banner (8.11 wording): "You are offline - N changes kept locally"; the Restore prompt reads "Restore N changes to b.log?" (default **Not now**) | Retry now; Restore / Not now |
| `files:delta-write` or `files:edit-large` turned off mid-session | T0: editing continues and Save sends the whole file (a quiet "Saving the whole file" the first time); T1: the buffer drops to Read, banner "Editing large files is turned off. Your changes are kept as a draft." | Reload, Copy my edits as patch |
| Stored draft no longer matches the file | "Draft doesn't match the file" and, for T1, the same-position rebase (9.4) | Copy my edits as patch; Discard draft |

## 9. S7 Conflict dialog

### 9.1 Wireframes

```
Desktop modal (min 720px, max 90vw)             Mobile full-screen sheet
+--------------------------------------+        +----------------------------+
| File changed on disk                 |        | <  File changed on disk    |
| src/a.go was modified since you      |        | src/a.go was modified ...  |
| opened it.                           |        | Hunk 1 of 3        [^] [v] |
|  Your edits vs. current disk         |        | + added in mine            |
|  - line removed on disk              |        | - removed in mine          |
|  + line in your version              |        |   context                  |
| [Show only changed hunks v] [^][v]   |        | [Keep editing] (primary)   |
| [Keep editing]  [Merge...]           |        | [Merge...]                 |
| [Reload from disk] [Overwrite...]    |        | [Reload from disk]         |
+--------------------------------------+        | [Overwrite...]             |
                                                +----------------------------+
```

- The server sends the current disk version as `FullText` (LF-normalized) or `Unavailable{reason}`. For `Unavailable` (disk file grew past 1 MB, became binary, latin-1 or mixed-EOL, or was replaced by a symlink) the dialog shows the reason instead of a diff and offers only **Keep editing**, **Copy my version** and **Reload from disk**; **Merge...** and **Overwrite...** are shown disabled with the reason (the client cannot overwrite a file it cannot show). Deleted files keep the existing deleted variant.
- Diff is "your edits vs. current disk" using the dynamic-imported unified merge view; `+`/`-` glyphs in the gutter; changed-hunks-only collapse; hunk prev/next (buttons; F7/Shift+F7 desktop).
- Actions, in order of safety: **Keep editing** (primary, default focus), **Merge...** (opens S8), **Reload from disk**, **Overwrite...** (destructive style, last).

### 9.1a Overlay layering (mobile and desktop)

Maximum depth is **two**: one full-screen sheet (tab switcher S4, conflict S7, merge S8, outline, file info) plus at most one small alert dialog (Discard, Reload copy-first, Overwrite confirm, Discard merge choices). Rules: (1) S7 -> S8 -> back to S7 is a **navigation inside the same sheet** (the sheet content swaps; the sheet is not stacked), and the S7 Reload "Copy my version first?" and Overwrite confirm steps are inline steps of the sheet, not additional dialogs; (2) an alert dialog opens over a sheet but a sheet never opens over a sheet or over an alert dialog (the tab switcher closes before any sheet-opening action, and a dirty-close Discard dialog is the only thing that may sit over S4); (3) Esc/system back closes the top layer only (from S8 it never closes the sheet: it returns to S7, see the Back rule in section 10); (4) focus is trapped in the top layer and returns to the previous layer's invoker on close.

### 9.2 Action flows

| Action | Flow | Exit path |
|---|---|---|
| Keep editing | Dialog closes; buffer stays dirty and marked "conflict: disk changed"; Save re-opens the dialog with a fresh token if disk changed again. | Always available. |
| Merge... | Enabled only when the disk version is `FullText` and the merge base is available. Opens S8 with base = version the buffer opened from, mine = draft, theirs = current disk. Disabled with a reason otherwise ("Disk version cannot be shown" or "original version not available after reload"). | Back to dialog from S8. |
| Reload from disk | Step 1: "Copy my version first?" with **Copy my version** and **Reload without copying** and **Cancel**. Step 2: buffer reloads, `baseToken` updated, draft cleared. | Cancel returns to dialog. |
| Overwrite... | Disabled when the disk version is `Unavailable`. Otherwise the confirm step names the path: "Overwrite src/a.go on disk with your version? The other changes will be lost." **Overwrite src/a.go** / **Cancel**. On confirm, re-sent with the disk token. | Cancel returns to dialog. |

### 9.3 Error and edge states (S7)

| State | What the user sees | Exit path |
|---|---|---|
| Disk changed again during Overwrite | "File changed again while you were deciding" - dialog refreshes diff against the newest disk version. | Same four actions. |
| File deleted on disk | Dialog variant "File was deleted on disk": no diff; actions **Keep editing**, **Copy my version**, **Close tab**. Overwrite is not offered (recreation is out of scope). | Keep editing / Close tab. |
| Diff too large (> 5,000 changed lines) | "Diff is large - showing a summary only": counts of added and removed lines, both sizes, and the first 200 changed lines. **Overwrite... and Merge... are disabled** with the visible reason "Show the full diff to enable this" (Overwrite must never be offered over a diff the user has not been able to see, 9.1). A **Show full diff** button renders the whole diff in chunks (a warning "This can be slow on this device" on a coarse pointer) and, once it is shown, enables both. Reload from disk and Keep editing stay available throughout. | Show full diff, Keep editing, Reload. |
| Merge library fails to load (dynamic import error) | Inline "Merge view unavailable - try again" with **Retry**; plain diff text still shown. | Retry, or any other action. |
| Offline when choosing Overwrite | Overwrite disabled with reason "Offline" or "Server unreachable" (8.11); others work. | Keep editing. |
| Disk changed again while the dialog is open (detected by the poll, not only at Save) | Diff refreshes in place with a polite "Disk changed again - diff updated"; the stale banner underneath is suppressed (8.9); in the merge view resolutions for unchanged regions are kept. | Same four actions. |
| Dialog dismissed via Esc / back | Equivalent to **Keep editing**; never to Overwrite or Reload. | n/a |
| Copy to clipboard denied | "Copy blocked by the browser" with a selectable text area holding the content. | Select manually, then continue. |

### 9.4 T1 conflict: a file too large to compare (ADR-036; Story 2.3.3)

For a file over 1 MiB the server cannot return its content with a `Conflict` (`theirs = Unavailable{TooLarge}`), so there is no diff and no merge. The dialog opens as the variant **"File changed on disk (too large to compare)"**: **Merge... and Overwrite... are disabled** with the visible reason "File is too large to compare"; the available actions are **Keep editing** (default focus), **Reload (keep my edits)** (loads the new file and keeps the pending edits as a draft), **Rebase if lines unchanged** and **Copy my edits as patch**. Rebase asks the server to verify that every edit's removed text and its whole touched lines are still identical at the same line numbers; if so it saves and announces "Saved"; if an agent inserted lines above or anything differs it fails closed with "The file changed around your edits - Reload and re-apply" and writes nothing (it never guesses where an edit belongs). Esc and system back are Keep editing, as in 9.3. For files up to 1 MiB (T0) the dialog is the one in 9.1 with a diff and Merge; the pending edits are never transformed, the documents are merged.

## 10. S8 Three-way merge view

```
Mobile (unified, per-hunk rows)                 Desktop (result pane editable)
+-----------------------------------+           +------------------------------------------+
| <  Merge: src/a.go     2 conflicts|           | Merge: src/a.go     2 conflicts left      |
| (auto-merged text)                |           | Base | Mine | Theirs   [ Result (edit) ]  |
| <<<< Conflict 1 of 2              |           | ...                                      |
| mine:   y                         |           | [Use Mine] [Use Theirs] [Use Both]       |
| theirs: z                         |           |                                          |
| [Mine] [Theirs] [Both]  >=44px    |           | [Back to diff]       [Save merged]       |
| ...                               |           +------------------------------------------+
| [Back to diff]  [Save merged]     |
+-----------------------------------+
```

- Clean auto-merge: opens with zero conflicts, **Save merged** enabled, result visible for review (never auto-saved).
- Conflicts: each region needs Mine / Theirs / Both (or manual edit of the result); **Save merged** stays disabled until none remain, with the reason "2 conflicts left" shown beside it (not just a disabled style).
- Save uses the current disk token; on success buffer content = merged text and the merge base becomes the saved text.
- Hunk controls are labeled ("Use mine for conflict 1 of 2") for screen readers; conflicts announce count changes politely.
- **Phone kill switch**: the unified per-hunk merge view is the riskiest phone surface, so it has its own live-settable global flag `files:merge-phone` (default on) and a runtime degrade. When the flag is off, or the Files-tab container is < 360px wide, or the merge library fails to load, **Merge...** on a phone is replaced by the 2-way conflict flow (diff + Keep editing / Reload / Overwrite; the Merge button shows the disabled reason "Merge is not available on this screen"). Desktop merge is unaffected by this flag. The owner dogfoods the phone merge with the scripted walkthrough (Task 2.5.4a) before the global `files:editing` flip.

### 10.1 Phone merge: wireframes, states, review step (triad round 2)

Phone merge is a three-screen flow inside the one conflict sheet (9.1a: the sheet content swaps, nothing stacks): **Overview** -> **Resolve** (one card at a time) -> **Review**. Targets are 44px minimum; every card has a text label, so state is never colour-only.

```
Overview (S-844)                 Resolve (hunk card)               Review (nearTheirs step)
+-----------------------------+  +-----------------------------+  +-----------------------------+
| <  Merge: src/a.go          |  | <  Conflict 1 of 2   [^] [v]|  | <  Review merged result     |
| 1 auto-merged area to review|  | 2 unchanged lines (base)    |  | Area 1 of 3 (near disk edit)|
| 2 conflicts left            |  |   func a() {                |  | ctx   func a() {            |
| [Resolve conflict 1]        |  | Yours:                      |  | you   x := 1                |
| [Undo last choice] (off)    |  |   y := 2                    |  | disk  x := 1  // from disk  |
| [Back to diff]              |  | Disk:                       |  | ctx   }                     |
| Save merged: off (2 left)   |  |   y := 3                    |  | [Previous] [Next]           |
+-----------------------------+  | [Yours] [Disk] [Both]       |  | [ ] I reviewed all 3 areas  |
                                 | 2 unchanged lines (base)    |  | [Back to resolve] [Save     |
                                 +-----------------------------+  |  merged] (off until ticked) |
                                                                  +-----------------------------+
```

- **Base context**: every hunk card shows two unchanged lines above and below (from the merge base), collapsed into an expander "N unchanged lines" when more; the card header names the area ("Conflict 1 of 2", "Area 2 of 3: from disk").
- **Choose, re-pick, undo**: choosing Yours / Disk / Both collapses the card to a summary line ("Conflict 1: kept yours") with a **Change** button that reopens it (re-pick is always possible before Save). The Overview has **Undo last choice** (an unlimited in-memory undo stack for the dialog's lifetime; polite announcement "Conflict 1 reopened") and each summary line has Change. Manual editing of the result is desktop only; on a phone the result is edited only by the three choices (Both inserts yours then disk).
- **nearTheirs review step**: when any auto-merged area is within one line of a disk change, Review appears after the last conflict is resolved (or immediately when there are no conflicts). It is a stepper over only the flagged areas ("Area k of N", Previous/Next 44px), each showing its base context and the merged lines labelled "from you" / "from disk"; the checkbox "I reviewed all N areas" is disabled until every flagged area has been displayed once, and **Save merged** stays disabled with the visible reason "Review N areas first" until it is ticked. A merge with no flagged area skips the step (explicit Save only). Review never saves.
- **Disk changed during the flow**: the section-10 table row applies (merge recomputed, "Disk changed - merge updated, N conflicts"); a conflict whose underlying region changed returns to unresolved with the label "changed again".
- **Below 360px (or `files:merge-phone` off, or the library fails to load)**: the Merge action is disabled with "Merge is not available on this screen" and the 2-way flow is used. If the container *becomes* narrower than 360px while a merge is in progress (split-screen, font scaling), the merge pane is replaced by "Merge needs a wider screen. Your choices are kept." with **Back to diff**; widening restores the pane with every choice intact; nothing is lost and nothing is saved.

| State | What the user sees | Exit path |
|---|---|---|
| Disk changed again before Save merged | Merge recomputed against newest disk; "Disk changed - merge updated, N conflicts"; prior resolutions kept where the region is unchanged. | Continue or Back to diff. |
| Base unavailable (draft restored after the file changed; no base is stored) | "Merge unavailable: original version not available after reload." Merge... is disabled in S7 with that explanation; the dialog is a 2-way draft-vs-disk diff. A draft restored while the file is unchanged keeps Merge available (base = disk text at restore time). | Overwrite / Reload / Keep editing / Copy my version remain. |
| Binary or truncated file | Merge never offered (editing already blocked). | n/a |
| Huge file (> 1 MB) | Merge unavailable message; same exits as base-unavailable. | Same. |
| Back to diff (button, and the top-bar `<`) | Returns to S7 **keeping** every resolution made so far (kept in memory until the dialog closes; choosing Merge... again restores them). The sheet stays open. | Always available. |
| Esc / system back from S8 | **Never closes the sheet.** With no resolutions made it behaves exactly like Back to diff (returns to S7). With resolutions it first asks "Discard merge choices?" (alert dialog over the sheet, 9.1a): **Keep merging** (default, stays in S8) or **Discard**, which clears the resolutions and **returns to S7** (not out of the sheet). Esc from S7 is then Keep editing (9.3). | Both choices. |

## 11. S9 Discard and Restore-draft dialogs

```
Discard changes?                               Restore unsaved draft?
"b.go has unsaved changes."                    "A draft of src/b.go from 2h ago (40 KB) was found."
[Cancel] (default focus)                       [Not now] (default focus)
[Discard] (destructive style)  [Save] (primary) [Discard draft] [Restore draft] (primary)
```

**Default is the safest option, not the most helpful one**: initial focus and the Esc/back target are **Cancel** (Discard dialog) and **Not now** (Restore dialog); Enter on the default does nothing destructive and writes nothing. **Save** is the visually primary button but is never the focused default, because Enter on a default Save would write to disk (possibly into a conflict, an agent's newer version, or a directory-mode checkout) from a dialog the user opened to close a tab. Button order left to right: Cancel, Discard, Save (Discard and Save are separated by at least 16px on desktop and stacked full-width on mobile).

### 11.1 Discard dialog triggers

Closing a dirty tab, closing "Close others" that includes dirty tabs (one dialog listing them; section 9.1a layering: an alert dialog over the tab sheet is the one permitted stacking; semantics in 11.1a), `beforeunload` (browser-native prompt). **Switching sessions is not a dialog trigger (decided in triad round 2; this section previously listed it, contradicting the plan's `SessionBufferHold`)**: the parent switches sessions (including automatically, for example from the review queue) and `FilesTab` has no veto point, so a blocking dialog is both impossible to guarantee and wrong for automation. Instead dirty buffers are written synchronously to the in-memory `SessionBufferHold` (<= 1 MB each, <= 5 entries) and, under the cap, to the draft store; a toast "Unsaved changes in b.go kept - reopen this session to restore" (>= 8 s, dismissible) shows in the new view; returning to the session offers the Restore prompt first with the held text. When the hold evicts its oldest entry a toast says which file lost its in-memory copy. Mobile pane back from editor with a dirty buffer is **not** prompted (navigation within the Files tab keeps the buffer).

| State | What the user sees | Exit path |
|---|---|---|
| Save chosen but conflicts | Conflict dialog opens; tab stays open. | S7 actions. |
| Save chosen but offline | Dialog text changes to "Offline - cannot save"; Save disabled. | **Discard** (with draft kept as backup note "draft kept locally for 24 h"), or Cancel. |
| Session paused/stopped externally while dirty | No blocking dialog (automation must not be blocked). Banner "Session paused. Unsaved draft kept locally." Draft stored. | Restore on reopen (below). |
| Draft cannot be stored (localStorage over 256 KB or full, and the IndexedDB backup of 11.3 failed or is unavailable) | The priority-1 banner "Draft not stored: <reason>. Your edits exist only in this tab." (8.2) with **Save**, **Copy my version** and **Retry backup**. Higher priority than the old informational "Draft not stored (too large)" text because an iOS PWA discards a backgrounded page and a dirty buffer without any backup then loses everything. | Save, Copy, Retry backup. |

### 11.1a Close others with several dirty tabs (triad round 3)

One Discard dialog lists every dirty tab among those that would close, with the default focus on **Cancel**. Its buttons: **Cancel**, **Discard all** (destructive style; drafts of the discarded tabs are still kept for 24 h as in 11.2), and **Save all**. **Save all** runs a **sequential** loop in tab order, one save at a time, each through the single save path with the buffer's own `sessionId`, never in parallel, with a progress line "Saving 2 of 4 - b.go" (polite) and a **Stop** button. Rules: (1) a tab that saves successfully is closed as part of the Close others action; (2) the first `Conflict`, `Rejected` or `Unknown` outcome **pauses** the loop: that tab is activated, its normal conflict dialog or banner opens, the remaining tabs are untouched and stay open and dirty, and a toast "Saved 2 of 4; b.go needs attention" (>= 8 s) says where it stopped; the user resolves that tab the usual way and Close others is not resumed automatically (running it again offers the dialog with the tabs that are still dirty); (3) **Stop** (or Esc, which is Cancel's target) takes effect after the in-flight save returns: no further saves start, saved tabs stay saved and closed, and nothing is discarded; (4) offline or unreachable disables **Save all** with the reason (8.11) while Cancel and Discard all still work; (5) the tab being saved cannot be closed by another trigger until its save returns. Analytics: `files.edit.save{via=save_all}` per save and one `files.tab.close{method=close_others}`.

### 11.2 Restore-draft dialog

| State | What the user sees | Exit path |
|---|---|---|
| Draft token equals disk token | Restore puts the draft into an Edit buffer; the merge base is the current disk text (equal to the original base), so Merge stays available. | Done. |
| Draft token differs from disk | "File changed since this draft" - Restore opens S7 (conflict) rather than overwriting, as a 2-way flow (Merge disabled: no base is stored). | S7 actions except Merge. |
| Target file missing | "File no longer exists" - **Copy draft**, **Discard draft**. | Either. |
| Draft older than 24 h | Not offered; silent. | n/a |
| Not now | Draft kept until expiry; offered again next open. | Always available. |

### 11.3 Where drafts live: two tiers (triad round 3)

A dirty buffer is protected by up to two copies, because iOS discards a backgrounded PWA and a dirty buffer that exists only in memory is lost when that happens: (a) **localStorage tier**, <= 256 KB, 24 h, written synchronously on the debounce and on `visibilitychange`/`pagehide` (synchronous, so it survives the page being frozen); (b) **IndexedDB tier** (new), for dirty buffers from 256 KB up to the 1 MB edit cap, 24 h, at most 5 entries, written asynchronously on a 2 s debounce and also started on `visibilitychange`/`pagehide` (best effort: an IndexedDB transaction begun in `pagehide` usually completes but iOS can kill the page first, so the debounced write is what normally protects the user). Both tiers use the same key (session, path, baseToken) and the same Restore prompt. Why IndexedDB for the larger tier and not localStorage: localStorage is synchronous on the main thread, has a small shared quota (about 5 MB of UTF-16 per origin, so two 1 MB buffers can exhaust it and break the tab layout storage too) and a 1 MB string write blocks typing; IndexedDB is asynchronous and has a far larger quota. The same secrets-at-rest exposure as the localStorage drafts applies (origin-scoped plaintext), so the owner's veto of bounded drafts (ADR-032, plan Unresolved Questions) covers both tiers; with a veto neither tier is written and the priority-1 "Draft not stored" banner of 8.2 is the standing state for dirty buffers. While a larger buffer's backup is pending the status chip reads "Backing up...", after a successful write it clears, and if IndexedDB is unavailable (private mode, quota, blocked) the banner appears at once.

**ChangeSet drafts (ADR-036; Task 2.2.5d)**: the draft written to either tier is the composed list of edits since the last save, not a copy of the text, so its size follows how much you changed, not how big the file is: a 900 KB file with a few edits fits the small tier, and a 50 MB file with a few edits can be backed up at all. The tier is chosen by the serialized size (the same 256 KB and 1 MB limits, 24 h, five entries), the Restore prompt and the "Draft not stored" banner are unchanged, and the prompt names the edits ("Restore N changes to b.log?"). A draft that does not fit the file it is restored onto fails closed ("Draft doesn't match the file") and offers **Copy my edits as patch**; engines that cannot produce edits fall back to the text snapshot.

## 12. S10 Extra-key bar (mobile edit mode only)

```
+------------------------------------------------------------+
| [Esc] [Tab] [Ctrl] [Alt] [^][v][<][>] { } [ ] ( ) ; : " ' / \ = - _ [undo][redo]  ||  [Save*] [Stop] |  keys scroll; the cluster after || is fixed (7.1a step 3)
+------------------------------------------------------------+
|                    system soft keyboard                    |
+------------------------------------------------------------+
vim row (only when vim is ON, which on a touch device means the explicit opt-in from Overflow > Vim; never auto-enabled): normal mode adds a second row: [ : ] [ / ] [ h ] [ j ] [ k ] [ l ]
```

- Pinned at `visualViewport.offsetTop + height`, reusing `ViewportProvider`; keys 44x44; reuses the terminal supplemental-key styling. The row scrolls horizontally; the **Save and Stop editing cluster is fixed at the right end and never scrolls** (it is always visible, and it is the Save/Stop placement when the top bar is auto-hidden, 7.1a). The bar is shown for the `Touch` profile on tablets as well as phones (7.3).
- `Ctrl`/`Alt` are sticky one-shot modifiers: single tap arms (visible pressed state plus text "Ctrl on" in the live region), next key consumes it, double-tap locks (distinct lock indicator), tap again unlocks.
- With vim on (touch devices: only after the explicit per-device opt-in, section 8.8), `Esc` is first and visually emphasized. `Save` shows a dirty marker and is disabled (with `aria-disabled` and label "Save, no changes") when clean; spinner while saving.
- Keys queue until `compositionend` when an IME composition is active.
- Keys do not take focus from the editor (`mousedown`/`touchstart` preventDefault) so the keyboard stays up.

| State | What the user sees | Exit path |
|---|---|---|
| Keyboard dismissed by the user | Bar slides away with the keyboard (no slide under reduced motion); Edit mode stays (draft kept). The top bar returns if 7.1a had hidden it, so **Save** and **Stop editing** (section 7.1) are visible without the keyboard. | Tap the editor to reopen keyboard; **Stop editing** in the top bar returns to Read mode; Save in the top bar. |
| `visualViewport` unsupported | Bar falls back to sticky bottom of the editor pane (no keyboard-follow). | Functional but unpinned; no blocking error. |
| Viewport lags (300-500 ms) | Bar position settles after debounce; no jump flicker (transition disabled for reduced-motion). | n/a |
| Hardware keyboard attached (including Bluetooth) | Precedence and detection in section 8.8: the persisted manual override wins; otherwise bar hidden when the resolved `InputProfile=HardwareKeyboard`; overflow offers "Show key bar" and "Keyboard: Auto / Touch / Hardware". If the top bar had been auto-hidden by 7.1a step 3, it returns in the same layout pass that hides the bar (Save and Stop editing move back to the top bar; never a state with neither visible). | Toggle / override. |
| Modifier armed and forgotten | Auto-disarms after 5 s with polite announcement "Ctrl off". Lock does not auto-disarm. | Tap to unlock. |
| Landscape / short viewport | Layout follows 7.1a: banner row and vim row fold first, then the top bar auto-hides and Save/Stop live in this bar's fixed cluster; the bottom bar is always hidden while the keyboard is up. | Reveal handle (overflow), dismiss the keyboard to restore the top bar. |

## 13. S11 Status bar

```
Desktop (24-28px):
 NORMAL | Ln 12, Col 4 | UTF-8 | LF | Go | [Read-only: Truncated] | * Modified | Saved 10:42 | [Offline] | Hit 3/27 ^ v
Mobile (compact, above bottom bar, one line, chips scroll; details also in overflow > File info):
 Ln 12, Col 4 | LF | Go | * Modified
```

- Mode segment (`-- NORMAL --`, `-- INSERT --`, `-- VISUAL --`) is a button that toggles vim (desktop); on mobile vim is toggled from overflow. It is exposed as `role="status"` with **`aria-live="off"`** so ordinary mode changes (every Esc and `i`) are not announced. A polite announcement is made only on an **explicit** change: toggling vim on or off ("Vim mode on, Normal"), and when the user focuses the segment (reads the current mode on demand). The ex command line and `-- INSERT --` entry are not announced.
- Chips are buttons where they have an action: `LF`/`CRLF` informational only (never editable: round-trip preserved), read-only reason chip opens an explanation popover, offline chip opens a popover "Edits are kept locally; saving resumes when you are back online. **Retry now**".
- Large-file segments (ADR-036, 8.12): `Ln 1,234,567 of about 4.2 million` (exact after indexing), the chip "Indexing file...", the read-only reason chips "Read-only: file too large to edit" and "Read-only: very long line", and the quiet chip "Using simpler file mode" when the transport has been demoted; each is text (never color alone), the info chips open an explanation popover, and on a narrow phone they collapse behind "..." (File info sheet) with the line count always visible.
- Message region (`aria-live`): Saved, Save blocked, Search truncated, etc. Messages auto-clear after **8 s** (paused while hovered or focused, see section 0) except errors, which persist until dismissed or superseded.

| State | What the user sees | Exit path |
|---|---|---|
| Info popover open | Popover with explanation. | Esc / tap outside. |
| Message flood | Only the latest error persists; others coalesce. | Dismiss `x`. |
| Mobile viewport narrow (320px) | Chips collapse behind "..." (File info sheet). | Sheet closes with back/x. |

## 14. S12 Quick-open, command palette, go-to-line

```
+----------------------------------------+
| > go to line__                    (combobox, aria-expanded)
+----------------------------------------+
| Go to line                    Ctrl+G   | <- role=option, aria-activedescendant
| Toggle vim mode                        |
| Toggle word wrap                       |
| Save                          Ctrl+S   |
| Reload from disk                       |
| Show shortcuts                    ?    |
+----------------------------------------+
 "6 results" in polite live region
```

- Ctrl+P opens file search (existing); typing `>` switches to command mode; `:` prefix or Ctrl+G is go-to-line (`:250` or just `250`); `@` prefix opens outline for the current file.
- Mobile: bottom-bar **Quick-open** opens full-width palette at the top of the viewport (not a bottom sheet, so the keyboard does not cover results); **Go-to-line** opens the same palette pre-filled with `:` and numeric inputmode.
- Enter runs; Esc closes and restores focus to the previous element; prior input is not retained.

| State | What the user sees | Exit path |
|---|---|---|
| No matches | "No files match `xyz`" (file mode) / "No commands match" (command mode). | Edit input; Esc. |
| Go-to-line out of range | Clamps: `9999` goes to last line with status "Clamped to line 300"; `0` or non-numeric shows inline "Enter a line number 1-300". | Fix input; Esc. |
| Command unavailable in this state (e.g. Save when clean, Reload when no buffer) | Listed but `aria-disabled` with reason suffix "(no changes)"; Enter announces the reason. | Pick another; Esc. |
| File list load failure | "Could not load file list" with **Retry**. | Retry; Esc. |
| Browser shortcut collision (Ctrl+P / Ctrl+Shift+F) | Handled only when focus is inside the Files tab; elsewhere the browser default is untouched. A toolbar/bottom-bar button always exists as the alternative. | Use the button. |

### 14.1 Chord policy: no browser-reserved shortcuts (triad round 2)

Browsers deliver some accelerators to the page and let it cancel them; others are reserved and never reach (or cannot be cancelled by) page script. The first design used chords in the second group. Rule: **a Files shortcut must be a chord the page both receives and may `preventDefault`, and only while focus is inside the Files tab; every shortcut has a visible button or menu item as the primary route.**

| Action | Chord | Status |
|---|---|---|
| Quick open | Ctrl/Cmd+P | existing, page-cancelable (UNVERIFIED per browser, per-browser check below) |
| Search | Ctrl/Cmd+Shift+F | page-cancelable (UNVERIFIED per browser) |
| Go to line | Ctrl/Cmd+G | page-cancelable (find-next in browsers; UNVERIFIED) |
| Save | Ctrl/Cmd+S | page-cancelable |
| Jumplist back / forward | **Alt+[ / Alt+]** (was Alt+Left/Right, the browser's history keys) | chosen; matched by `event.code` (`BracketLeft`/`BracketRight`) so macOS Option does not type a glyph |
| Close tab | **Alt+W** (was Ctrl+W, reserved and uncancelable); middle-click, `x`, Delete on a focused tab | chosen; matched by `event.code` `KeyW` |
| Vim keyboard-trap escape | **Esc, Esc** and the status-bar Leave editor control (was Ctrl+Shift+M: Chrome profile menu, Firefox Responsive Design Mode) | chosen; no modifier chord |
| Next / prev hit | F4 / Shift+F4 | page-cancelable |
| Next / prev diff hunk | F7 / Shift+F7 | UNVERIFIED (Firefox and Chrome use F7 for caret browsing); the hunk buttons are the primary route |
| Collapse panel | Ctrl+B | existing behavior, kept; Firefox binds it to the Bookmarks sidebar (UNVERIFIED if cancelable); the collapse button is the primary route |
| Move tab | Alt+Shift+Left/Right and the tab context menu "Move left / right" | chosen |

**Non-US layouts and AltGr (triad round 3).** Alt+[ , Alt+] and Alt+W are matched by the physical `event.code` (`BracketLeft`, `BracketRight`, `KeyW`), so they sit on the same physical keys on every layout. Two consequences are specified: (1) on layouts where **AltGr** is reported as Ctrl+Alt (Windows German, Polish, Czech and others; AltGr+8 types `[`), the keydown has `altKey` and `ctrlKey` both true and `getModifierState('AltGraph')` true; the matcher requires `altKey && !ctrlKey && !metaKey && !getModifierState('AltGraph')`, so a character typed with AltGr is never swallowed as a Files chord (tested with a synthetic AltGr+8 event and with `Alt+BracketLeft`); (2) the shortcut overlay, the tooltips and the palette show **the label of the physical key on the user's layout**, resolved with `navigator.keyboard.getLayoutMap()` where it exists (Chromium: for example `Alt+Ü` on a German layout, where the key at the US `[` position is `Ü`), and where the API is missing (Firefox, Safari) the US label plus the words "(the key at the US [ position)" so the text is never a chord that does not exist on that keyboard. The toolbar and bottom-bar buttons remain the primary route on every layout.

**Never registered** (denylist, asserted by a test): Ctrl/Cmd+W, Ctrl/Cmd+Shift+W, Ctrl/Cmd+T, Ctrl/Cmd+Shift+T, Ctrl/Cmd+N, Ctrl/Cmd+Shift+N, Ctrl+Tab, Ctrl+Shift+Tab, Ctrl+PageUp/PageDown, Ctrl/Cmd+1..9, Alt+Left, Alt+Right, Alt+Home, Ctrl/Cmd+L, Ctrl/Cmd+K, Ctrl/Cmd+E, Ctrl/Cmd+D, Ctrl/Cmd+R, F5, F6, F11, F12, Ctrl/Cmd+Shift+I/J/C/M/Delete, Ctrl/Cmd+Q.

**Verification**: (1) a Jest test asserts no registered chord is on the denylist and every documented chord resolves to an action; (2) a Playwright spec runs on desktop Chromium, Firefox and WebKit and asserts each chord reaches its handler and is cancelled once. Playwright injects key events below the browser accelerator layer, so it **cannot** prove real delivery; (3) the owner's manual per-browser check presses each chord in Chrome, Firefox and Safari and records delivered / swallowed in the checklist, and any chord a browser swallows gets its menu or button as the documented route for that browser.

## 15. S13 Outline panel and S14 Shortcut overlay

### 15.1 Outline (S13)

```
Desktop dropdown from breadcrumb          Mobile sheet
| Outline            [filter___] |         | Outline (12)            [x] |
| A            func     5        |         | A     func     5            |
| B            type     20       |         | B     type     20           |
```

- Listbox with type-ahead filter; selecting jumps to the symbol line and pushes a jumplist entry. Esc / `x` / back closes and restores focus.
- Languages without a node map or a parse failure: "No symbols found for this file type". Files above the highlight-off threshold: "Outline unavailable for large files". Both show a **Close** exit; the Go-to-line action is offered as an alternative link.

### 15.2 Shortcut overlay (S14)

- `?` or the "Shortcuts" toolbar/overflow button, same place on every Files view. Lists Ctrl+P, Ctrl+Shift+F, Ctrl+G, Alt+[ / Alt+] and Alt+W (with the layout-resolved physical-key labels of 14.1), Ctrl+S, F4/Shift+F4, Ctrl+B, Esc-then-Tab (vim off), and with vim on Esc Esc, `:leave` and Leave editor (8.4), plus the "Esc, Esc leaves the editor" setting, vim items when on. Grouped; searchable. Mobile shows tap equivalents instead of key chords for touch-only rows.
- Dismiss with Esc, `x`, or back; focus restored. Errors: none beyond failing to render (falls back to a static list).

## 16. S15 Non-interactive surfaces (condensed)

**Feature flags**: `files:editing` gates WriteFile and every Edit control; `files:viewer-v2` (or ungated) governs search and navigation. Live-settable through the existing feature-flag panel (no env vars).

**Structured log sample (info level, no content or query text)**:

```
{"level":"INFO","msg":"file write","session":"s-1","path":"src/a.go","bytes":1204,"outcome":"conflict_rejected"}
{"level":"INFO","msg":"content search","session":"s-1","query_len":7,"duration_ms":412,"results":27,"truncated":false}
```

**Analytics events** (user analytics system, ADR-035): the viewer emits typed `files.*` events through `useFilesAnalytics()` and the server records `files.server.*` outcomes; labels are enums, booleans and buckets only, never paths, file names, file contents or query text. The user sees no UI for this; there is no consent prompt because the data stays in the local `analytics.db` (the platform's offline, no-PII stance). Acceptance criteria are in section 18 (`U-OBS-1..4`).

## 17. Exit-path audit

Every flow above was checked against the rule "no dead ends". Result:

| Flow | Exit path present | Notes |
|---|---|---|
| S1 invalid regex, zero results, truncated, slow, cancelled, RPC failure, paused session | yes | Each table row names the exit. |
| S1 binary skipped, root fallback | informational | No action needed; not error states. |
| S2 quickfix | yes | `x`, Esc. |
| S3 tab states | yes | |
| S4 sheet | yes | `x`, back, Go to Files. |
| S5 states | yes | |
| S6 banners | yes | Every blocking banner offers Download, Close tab, or Copy my version. |
| S7 / S8 | yes | Esc and back map to Keep editing / Keep merging. |
| S9 | yes | |
| S10 | yes | Stop editing button, top-bar Save, tap-to-reopen keyboard. |
| S11, S12, S13, S14 | yes | |
| S6 large-file states (8.12): gap loading, gap failed, far jump, indexing, T2 view-only, torn/stale, transport demoted | yes | Cancel, Retry, Reload, Download; every state names its exit. |
| S6 delta-save states (8.13) and S7 T1 conflict (9.4) | yes | Save whole file (T0), Copy my edits as patch, Reload (keep my edits), Keep editing; rebase fails closed with Reload. |
| Flows lacking a complete exit path | **none identified** | Two residual risks, both mitigated but untestable until implementation: (a) the vim-on keyboard escape from the editor (Esc is consumed by vim) depends on the Esc, Esc handler and the status-bar control being reliable (no modifier chord is involved any more); (b) the iOS PWA keyboard/viewport behavior needs the owner's real-device check (already a listed blocker). |

## 18. UX acceptance criteria

Each is testable by a human or by Playwright at 1280x800 (desktop) and 390x844 / 320x568 (mobile). IDs are stable for the validation phase.

### Search and navigation
- **U-SEARCH-1**: A user can go from the Files tab to a search hit open at the correct line in <= 3 interactions (open Search, type, activate hit) on desktop and mobile.
- **U-SEARCH-2**: Typing 1 character sends no request and shows "Type at least 2 characters"; typing `foo` quickly sends exactly one request.
- **U-SEARCH-3**: Invalid regex shows an inline message under the field (not a toast), prior results remain visible, and toggling `.*` off clears the error.
- **U-SEARCH-4**: Zero results shows "No matches for `<q>` (<active toggles>)" and a Clear filters button that restores a broader search in one tap.
- **U-SEARCH-5**: A truncated result set shows "Showing first 1000 - refine your query" and an action that focuses the include/exclude fields.
- **U-SEARCH-6**: A search over a paused or archived session whose worktree is gone is refused by the server (never run against the repo-root fallback) and shows "Session paused; worktree unavailable" with a Resume action; no results from another checkout are ever shown.
- **U-SEARCH-12**: A search waiting for a free walk slot shows "Waiting for other searches (N ahead)..." with a Cancel control, distinct from "Searching..."; cancelling while queued ends it with no results change.
- **U-SEARCH-7**: A running search can be cancelled with one control, and partial results remain usable with a "cancelled" message.
- **U-SEARCH-8**: The results header announces "N results in M files" via `aria-live="polite"`; every match is wrapped in `<mark>`; typing in the query field never loses focus when results arrive or a banner appears.
- **U-SEARCH-9**: Opening a hit leaves the results panel/pane intact; on mobile, Back returns to the results with scroll position preserved; on desktop, Esc in the query field or results returns focus to the editor and Ctrl+Shift+F returns focus from the editor to the query field (8.4 table; Esc is never the editor-to-panel key).
- **U-SEARCH-10**: Next/prev hit (F4/Shift+F4, `]q`/`[q` in vim, chip chevrons on mobile) steps through hits without leaving the editor and wraps with an announcement.
- **U-SEARCH-11**: A hit beyond the loaded portion of a truncated file shows "Line N is beyond the loaded portion" and clamps to the last line.
- **U-NAV-1**: Alt+[ / Alt+] (and the mobile bottom-bar Back/Forward, vim Ctrl-o/Ctrl-i) restore the previous file, cursor line, and scroll; each buffer keeps its own scroll and cursor when switched away and back.
- **U-NAV-2**: Go-to-line clamps out-of-range input and shows "Clamped to line N"; non-numeric input shows an inline hint.
- **U-NAV-3**: Ctrl+P and Ctrl+Shift+F call `preventDefault` only when focus is inside the Files tab; elsewhere the browser default works; a visible button exists for every shortcut-only action.
- **U-NAV-4**: The outline lists symbols with line numbers, selecting one jumps there and adds a jumplist entry; unsupported or large files show the specific unavailable message and a Close control.
- **U-NAV-5**: Opening `?file=...&line=N` opens the file at that line as a pinned tab.
- **U-SEARCH-13**: A search hit in a file whose open buffer is dirty is marked "buffer has unsaved changes; line may differ" (in the row's accessible name too); activating it opens the existing tab and, when the matched text is not at the hit line of the buffer, jumps to the nearest line (within 200 lines) that contains it and announces "Moved to line N (buffer has unsaved changes)", else announces "Match not found in your unsaved version"; the buffer content is never altered by the jump.

- **U-SEARCH-14**: With the soft keyboard open (stubbed `visualViewport` 844 -> 500) and focus in the query field, a single tap on a result row opens that row exactly once (identity captured on pointerdown, keyboard dismissed, no second tap, no mis-hit after relayout); a pointer that moves more than 10px is a scroll and opens nothing.
- **U-SEARCH-15**: The results container is `role="tree"` (`aria-label`, `aria-busy` while streaming); every mounted group header and hit row carries `aria-level`, `aria-setsize` and `aria-posinset` (valid with virtualization), group headers `aria-expanded`; the hit row's accessible name is "<path> line N, <text>, match k of m, result r of t"; axe reports no ARIA violation on the list at 5,000 rows.
- **U-SEARCH-16**: With the panel open and no query, a helper explanation is shown (never a blank pane); with retained results they are shown dimmed with "edit to search again".
- **U-NAV-6**: The three back controls follow section 7.4: after opening a hit from results, top-bar `<` and system Back return to the results with scroll, query, toggles and collapsed groups preserved, while the bottom-bar Back returns to the previous location; neither changes the other's stack; Back is `aria-disabled` with "No earlier location" at the ends.
- **U-NAV-7**: Stepping through hits (chip, F4) does not push pane entries and records the origin jumplist entry only for the first hit of a search.

### Tabs
- **U-TAB-1**: Tabs expose tablist/tab/tabpanel roles; Left/Right/Home/End move and activate; Delete closes and focuses the neighbor.
- **U-TAB-2**: A dirty tab's accessible name includes "unsaved" in addition to the dot; the dot has >= 3:1 contrast.
- **U-TAB-3**: Tab close hit-areas measure >= 44x44 on coarse pointers (>= 24x24 on fine); the mobile tab sheet rows are >= 44px.
- **U-TAB-4**: With 20 tabs, activating the 20th scrolls it into view; a reorder is possible by keyboard (Alt+Shift+Arrow) as well as drag.
- **U-TAB-5**: A single-click open creates an italic preview tab replaced by the next single-click; edit or double-click pins it.
- **U-TAB-6**: Persisted tabs for files that no longer exist are shown as "not found" and can be closed in one action; if storage is unavailable, a one-time notice appears and tabs work in memory.
- **U-TAB-8**: Pinned and dirty tabs are never closed by the tab limit. Opening past the cap evicts only the least-recently-used clean preview tab, announces it and offers Undo for at least 8 s, paused while hovered or focused (Undo restores scroll and cursor); when no tab is eligible the open is refused with "Too many tabs open - close one" and the tab list opens; no tab is ever closed without a visible announcement.
- **U-TAB-7**: Closing a dirty tab opens the Discard dialog with **Cancel** as the default focus and the Esc/back target (Enter on it changes nothing; Save is primary-styled but never the focused default); Cancel leaves everything unchanged; focus returns to the tab afterward.
- **U-TAB-11**: Close others with several dirty tabs opens one Discard dialog listing them (default focus Cancel) with Cancel, Discard all and Save all; Save all saves sequentially in tab order with a progress line and a Stop button, closes each tab that saved, pauses at the first Conflict, Rejected or Unknown outcome by activating that tab and opening its normal dialog or banner (the remaining tabs stay open and dirty, a toast names where it stopped), never runs saves in parallel, and Stop takes effect after the in-flight save without discarding anything (11.1a).

- **U-TAB-9**: On touch, a tab can be pinned and unpinned from the sheet row's actions menu or long-press; pinned and preview rows are distinguished by a glyph and the words "pinned"/"preview" in the accessible name; the strip shows the same state on desktop.
- **U-TAB-10**: The cap rule is one rule: only a clean preview is ever closed automatically; with none eligible the open is refused with the message, the sheet or list opens with the eligible rows' Close actions highlighted, and "Unpin and close the oldest pinned tab" is available only as an explicit choice in the list.

### Mobile layout
- **U-MOB-1**: At 390x844 and 320x568 there is no horizontal page scroll (code area excepted), all primary controls are >= 44px, and the bottom bar respects `safe-area-inset-bottom`.
- **U-MOB-2**: On the named viewports of section 7.1a (`XS-568`, `S-844`, `L-390`, `XS-L`; all four get the mobile layout by the section 0 layout rule, including the 844px-wide landscape phone), opening the soft keyboard in edit mode (stubbed `visualViewport` heights 308, 584, 190, 120) does not hide the caret line, covers no primary control other than the bottom bar (which hides), the extra-key bar sits directly above the keyboard (within 2px of `visualViewport` bottom), and the editor region is >= 120px wherever the ladder can achieve it (hard floor 64px with the one-time hint).
- **U-MOB-3**: In Read mode tapping code does not raise the keyboard and does not change `visualViewport.height`.
- **U-MOB-4**: Browser back from editor pane returns to the previous pane and never leaves the session; every swipe gesture has a visible button equivalent. On Android with the soft keyboard up, the first system Back dismisses the keyboard without changing the pane, Edit mode or the buffer, and the second pops the pane (7.4 rule 6; Playwright stubs the viewport resize and asserts no pane or history change, and the owner confirms the real behavior in the Task 2.5.4a script).
- **U-MOB-5**: A 600 KB file on a coarse pointer opens with highlighting, bracket matching, and fold gutter off and a one-line notice; a single 4,000-char line scrolls horizontally without freezing.
- **U-MOB-6**: The font stepper changes size in 1px steps, minimum 16px on coarse pointers, and the choice persists per device.
- **U-MOB-7**: Landscape phone editing (844x390 and 568x320, mobile layout by the section 0 rule) hides the bottom bar and, only when the editor would otherwise be < 120px, the top bar; Save and Stop editing then live in the key bar's fixed cluster, and a visible 44px reveal handle still reaches the overflow menu (section 7.1a).
- **U-MOB-9**: At 320x568 the bottom bar shows Files | Search | Quick-open | Back | More with every target >= 44px and no horizontal overflow; Go-to-line and Forward are reachable in <= 2 taps through More; at >= 360px all five groups show.
- **U-MOB-10**: With the quickfix chip and the Edit FAB both shown, they stack on the right edge (chip adjacent to the bar, FAB 8px above it) without overlapping each other, the bottom bar or any code line that cannot be scrolled clear (editor bottom padding equals the stack height); dismissing the chip moves the FAB down.
- **U-MOB-11**: Save is reachable in one tap with the soft keyboard dismissed: the top-bar Save appears whenever the active buffer is dirty (Read or Edit mode), shows a dirty marker and a spinner while saving, and uses the single save path. While the top bar is auto-hidden to protect the minimum editor height (7.1a step 3) the identical Save sits in the key bar's fixed, non-scrolling cluster; no viewport/keyboard/vim combination leaves a dirty buffer without a visible Save.
- **U-MOB-12**: Sheets and dialogs never stack deeper than one sheet plus one alert dialog; S7 -> S8 swaps content within one sheet; Esc/back closes only the top layer.
- **U-MOB-13**: With the `files:merge-phone` flag off, or a container narrower than 360px, Merge... on a phone falls back to the 2-way conflict flow with a stated reason; desktop merge is unaffected.
- **U-MOB-8**: Sheets and dialogs trap focus, dismiss with back/Esc/`x`, and disable slide animation under `prefers-reduced-motion`.

- **U-MOB-14**: At each named viewport and keyboard state (7.1a) with vim on and off and with and without a priority-1 banner, exactly one Save (>= 44px) and one Stop editing (>= 44px) are visible whenever a buffer is in Edit mode and dirty, in the top bar or in the key bar's fixed cluster, never inside the scrolling key list; with a priority-1 banner the collapsed banner row still shows **Copy my version** and **Retry Save** as 44x44 buttons at 320px (7.1a step 2); the editor region is >= 120px or the one-time "Little room" hint is shown with the floor at 64px.
- **U-MOB-15**: The phone merge follows 10.1: Overview, one-card Resolve with two base context lines, Change on every resolved card plus Undo last choice, a Review stepper over the `nearTheirs` areas that gates Save merged with the visible reason "Review N areas first", and a narrower-than-360px container keeps all choices and shows "Merge needs a wider screen. Your choices are kept.".
- **U-MOB-16**: The layout rule of section 0 holds: a container < 768px, or a coarse-pointer device whose `min(screen.width, screen.height)` < 600, gets the mobile layout; 844x390 coarse (landscape phone) gets the mobile layout and the 7.1a ladder; 820x1180 and 1180x820 coarse get the desktop layout, and opening the soft keyboard on the 1180x820 tablet (viewport height 470) does not switch layouts; rotating a phone never changes the layout; a layout change never remounts the editor or drops the keyboard (pure-function table test plus a Playwright check at 844x390 and 1180x820).
- **U-MOB-17**: With the top bar auto-hidden (7.1a step 3), a profile flip to `HardwareKeyboard` (detection or override) restores the top bar with Save and Stop editing in the same layout pass that hides the key bar, so there is no render in which a dirty Edit buffer has neither control visible; the reverse flip re-runs the ladder (step-through jest test over Touch -> Hardware -> Touch with `inputProfile` as an `editLayoutBudget` input).
- **U-TABLET-1**: At 820x1180 and 1180x820 with `pointer: coarse` and a stubbed soft keyboard, the desktop layout renders (tab strip, side panel, no FAB, no bottom bar), controls are >= 44px, Save and Stop editing are in the breadcrumb row, the breadcrumb-row toolbar has **Quick open**, **Go to line** and **Search** buttons (>= 44px; the Touch profile has no Ctrl chords and no bottom bar, so these are the only touch route to them), the extra-key bar is pinned above the keyboard with all keys visible, and with a detected or overridden hardware keyboard the bar is hidden and the toolbar buttons shrink to the 24px desktop size.

### Editing and saving
- **U-EDIT-1**: When `files:editing` is off no Edit control or Save appears; when on and the file is writable, Edit is reachable in <= 1 interaction from the open file on both form factors.
- **U-EDIT-2**: Truncated, rejected, binary, and non-UTF-8 files never enter Edit mode; the reason chip/banner states why and offers Download.
- **U-EDIT-3**: Ctrl+S, `:w`, the Save button, and the mobile Save key all use one save path; a second press during a save sends no second request and shows a spinner.
- **U-EDIT-4**: A successful save announces "Saved", clears the dirty dot, preserves undo history, and keeps CRLF/BOM exactly (verified by round-trip diff).
- **U-EDIT-5**: Flipping `files:editing` off while a buffer is dirty makes the editor read-only, keeps the draft, and shows "Editing was disabled; your unsaved changes are kept" with Copy my version.
- **U-EDIT-6**: Offline: Save is disabled with the chip "Offline - edits kept locally", nothing is queued; on reconnect the next Save re-checks the disk version first.
- **U-EDIT-7**: A save with unknown outcome shows "Save status unknown - checking..." and resolves to Saved or the conflict dialog without a second blind write.
- **U-EDIT-8**: With vim on, Esc, Esc leaves the editor only when vim was already in Normal mode with nothing pending, the hint chip "Esc again to leave the editor" was showing from the first Esc, the second Esc came within 500 ms, and the per-device setting is on; `:leave` and the status-bar "Leave editor" control leave in every vim mode and the setting state; none can be remapped by `:map`/`:nmap`/`:imap <Esc>`; no modifier chord is involved (14.1); a single Esc, and Esc followed by another key, only behave as vim; Esc or Enter on the focused Leave editor control returns focus to the editor with cursor and Normal mode intact.
- **U-EDIT-9**: A preview tab is pinned when its buffer enters Edit mode (before any keystroke), so a clean buffer in Edit is never replaced by opening another file and never evicted by the tab cap; a dirty tab is always pinned.
- **U-EDIT-11**: A clean open file that changes on disk shows "Changed on disk - Reload" without moving focus or content; Reload keeps cursor line and scroll.
- **U-EDIT-12**: A rejected save (permission/confinement) shows a reason class without leaking another user's path and keeps the buffer editable.
- **U-EDIT-13**: Typing a printable key in Read mode on an editable desktop file shows the inline hint "Read-only. Press E or click Edit to start editing" (once per 10 s, auto-hide 4 s, `role="status"`, no focus move); `E` enters Edit mode without inserting the triggering character; on a non-editable file the hint shows the reason chip text instead.
- **U-EDIT-14**: The control that leaves Edit mode is labeled "Stop editing" ("Stop editing (unsaved)" when dirty), never "Done"; leaving with a dirty buffer announces that N changes are kept unsaved and keeps the dirty dot and Save affordance visible.
- **U-EDIT-15**: A blocking banner (Save rejected, Save outcome unknown) is never collapsed into "+N more" and never hidden behind an informational banner; banners sort by the documented priority; a blocking banner has no Dismiss and is announced assertively once.
- **U-EDIT-16**: A file that changes on disk is detected within 10 s while the tab is visible (poll) or immediately on focus/visibility; a change seen twice or persisting 1 s shows exactly one banner (updated in place on further changes); the banner never asserts who changed the file (suffix "this may be the agent" only while the session is running, else "source unknown").
- **U-EDIT-10**: Esc then Tab leaves the editor when vim is off (no keyboard trap); with vim on, Tab is a vim key and does not reach the status bar from inside the editor, and the documented exits are Esc, Esc (8.4), `:leave` and the pointer/screen-reader route to Leave editor, advertised by the shortcut overlay and a one-time status when vim is turned on.

- **U-EDIT-17**: Edit mode is per buffer: switching tabs preserves each tab's mode, Stop editing affects only the active buffer, a new or reloaded buffer starts in Read, entering Edit pins the tab, and flag-off drops all buffers to Read keeping drafts.
- **U-EDIT-18**: With `files:editing` on, a file with any `ReadOnlyReason` shows Edit disabled with the reason as visible text (mobile FAB tap opens a popover with the reason and Download); with the flag off the control is absent.
- **U-EDIT-19**: A zero-byte file opens, shows an "Empty file" placeholder, is editable, and a no-op save writes zero bytes; saved text uses LF and no BOM.
- **U-EDIT-20**: With `navigator.onLine === true` but two consecutive failed RPC/heartbeat checks, Save is disabled and the banner reads "Server unreachable - edits kept locally" with Retry now; a disabled Save always exposes its reason visibly and via `aria-describedby`, and tapping or pressing Enter on it announces the reason.
- **U-EDIT-21**: A Save attempt in an archived session shows "Session archived - saving is disabled", keeps the draft, offers Copy my version, and never revives, un-archives or restarts the session.
- **U-EDIT-22**: With a dirty buffer, the Accessible view shows the draft text (not disk), names itself "..., accessible view, unsaved changes", shows a visible "Unsaved changes" label and a 44px Save button on the single save path, and shows conflict, stale and offline banners above the region; a clean buffer shows neither.
- **U-EDIT-23**: The Esc contexts of the 8.4 table hold: a dialog or sheet closes on a single Esc and a double Esc never then acts on the editor; Esc in the search panel returns to the editor; Esc in the vim-off editor is passed to CodeMirror; the double-Esc leave is armed only by the visible chip and can be switched off per device.
- **U-OFFLINE-1**: Recovery from either state runs `StatFile` before the next Save is enabled; a token change routes to the conflict dialog.

### Conflict and merge
- **U-CONF-1**: A conflicting save is blocked, never writes, and opens "File changed on disk" with a unified diff of mine vs. disk; +/- glyphs make additions and removals distinguishable without color.
- **U-CONF-2**: Keep editing is the default focus and the target of Esc/back; Overwrite requires a second confirmation that names the file path.
- **U-CONF-3**: Reload from disk first offers Copy my version; Cancel at any step returns to the dialog with the draft intact.
- **U-CONF-4**: If the file changes again or is deleted during the decision, the dialog refreshes (or shows the deleted variant without an Overwrite option) instead of acting on stale data.
- **U-CONF-5**: At 390px the dialog is a full-screen sheet with hunk prev/next controls >= 44px; focus is trapped and restored.
- **U-CONF-6**: A clean 3-way merge opens with zero conflicts and requires an explicit Save merged; nothing is saved automatically.
- **U-CONF-7**: With conflicts, Save merged is disabled and shows "N conflicts left"; each conflict offers Mine / Theirs / Both (>= 44px on mobile) with labels naming the conflict number.
- **U-CONF-8**: When merge is unavailable (base missing, or the disk version is `Unavailable`: > 1 MB, binary, non-UTF-8, mixed EOL), the dialog says why; if the disk version is `Unavailable`, Overwrite is also disabled and only Keep editing, Copy my version and Reload remain, otherwise the other three actions remain available.
- **U-CONF-9**: Leaving the merge view (Esc, system back, top bar `<`) with resolutions made asks "Discard merge choices?" with Keep merging as the default; Discard clears the choices and returns to the conflict view S7 (never closes the sheet); with no resolutions the same keys return to S7 directly; the Back to diff button returns to S7 keeping the choices.

- **U-CONF-10**: A change to the file detected while the conflict dialog or merge view is open refreshes the diff/merge in place with a polite message, keeps unchanged-region resolutions, suppresses the duplicate stale banner, and never moves focus.
- **U-CONF-11**: A diff over 5,000 changed lines shows a summary only (counts, sizes, first 200 changed lines) with Overwrite... and Merge... disabled and the visible reason "Show the full diff to enable this"; Show full diff renders the whole diff (with a slow-device warning on a coarse pointer) and then enables them; Reload and Keep editing are available throughout; no text says a download is available.

### Drafts and discard
- **U-DRAFT-1**: A dirty buffer in a session paused externally is never blocked by a dialog; a draft (<= 256 KB, <= 24 h) is stored and "Restore draft?" is offered when reopened.
- **U-DRAFT-2**: Restoring a draft whose token differs from disk opens the conflict dialog (2-way, Merge disabled with a reason) instead of overwriting; if the file is missing, Copy draft and Discard draft are offered.
- **U-DRAFT-3**: A dirty buffer whose draft cannot be stored (over 256 KB with no IndexedDB backup, or storage full or unavailable) shows the priority-1 banner "Draft not stored: <reason>. Your edits exist only in this tab." with Save, Copy my version and Retry backup; it is never collapsed into "+N more" (8.2, 11.3).
- **U-DRAFT-4**: `beforeunload` is requested only while at least one buffer is dirty.

- **U-DRAFT-5**: Switching sessions never opens a blocking dialog: dirty buffers go to the in-memory hold (and the draft store under 256 KB), a toast (>= 8 s, dismissible) says they were kept, and returning offers the Restore prompt first; the Restore dialog's default focus is Not now.
- **U-DRAFT-6**: A dirty buffer between 256 KB and 1 MB is backed up to IndexedDB (debounced 2 s and started on `visibilitychange`/`pagehide`), shows "Backing up..." until the write lands, is offered by the same Restore prompt (Not now default) within 24 h, is limited to 5 entries, and falls back to the U-DRAFT-3 banner when IndexedDB is unavailable.

### Extra-key bar, status bar, palette
- **U-KEY-1**: The bar's keys are 44x44, the row scrolls horizontally, Esc is first (emphasized with vim on), and tapping a key never dismisses the soft keyboard.
- **U-KEY-2**: Tap Ctrl then `d` dispatches Ctrl+D once and disarms; double-tap locks with a visibly distinct state; an armed modifier auto-disarms after 5 s with an announcement.
- **U-KEY-3**: Save on the bar is disabled with the label "Save, no changes" when clean and shows a dirty marker otherwise.
- **U-INPUT-1**: Input profile resolves as manual override > auto-detect > Touch (default). Auto-detect flips to hardware only on a strong signal (8.8: primary fine pointer; a trusted Ctrl/Meta chord; Escape, F-keys, arrows or paging keys; or three trusted `Key<L>` letters with a matching `code`), never on Enter, Backspace, Space, Tab, digits, punctuation, `keyCode` 229 or synthetic events; a flip is session-only with a one-time status. A persisted manual choice (including "Touch" with a Bluetooth keyboard attached) always wins. Vim defaults are read only at Edit entry and never flip live mid-edit; vim is never auto-enabled on a touch-resolved device and an explicit persisted vim toggle always wins.
- **U-KEY-4**: During IME composition, bar keys queue until composition ends.
- **U-STATUS-1**: The status bar shows mode (with `role="status"`), Ln/Col, encoding, EOL, language, read-only reason, dirty/save state, and offline chip; each actionable chip is a keyboard-reachable button with an explanatory popover.
- **U-STATE-1**: Every wait > 150 ms in section 8.5 (open file, tree load, tab restore, outline load, explicit `StatFile`, merge library) shows its pending state; every failure there shows a message with the listed exit; a failed reload of an open file leaves the buffer and its dirty state unchanged.
- **U-STATE-2**: Merge saved, Reload, Draft restored, Overwrite and Undo-eviction each announce a polite success message (section 8.6); on mobile the transient toast (>= 8 s, dismissible, no motion under reduced-motion) does not cover the Save control.
- **U-STATUS-3**: The vim mode segment has `aria-live="off"`: Esc/`i` mode changes produce no live-region announcement; toggling vim on/off and focusing the segment announce the current mode politely.
- **U-STATUS-2**: Errors in the message region persist until dismissed; successes clear after 8 s (paused while hovered or focused); no two simultaneous messages overlap.
- **U-PAL-1**: The palette uses combobox/listbox/option with `aria-activedescendant`, announces the result count politely, and Esc restores focus to the prior element.
- **U-PAL-2**: Unavailable commands are listed disabled with a reason suffix; Enter announces the reason rather than doing nothing silently.
- **U-HELP-1**: The shortcut overlay opens from `?` and a visible button in the same place in every Files view, lists every Files shortcut, and closes with Esc/`x`/back.

- **U-KEY-5**: The Save and Stop editing cluster is fixed at the right end of the key bar and never scrolls out of view; the key list scrolls in the remaining width.
- **U-CHORD-1**: No registered Files shortcut is on the reserved denylist (14.1); each documented chord reaches its handler and is cancelled once in desktop Chromium, Firefox and WebKit; the owner's per-browser manual check is recorded; every shortcut has a visible control.
- **U-CHORD-2**: Alt+[ , Alt+] and Alt+W are matched by physical code but never when the event is an AltGr composition (`ctrlKey` with `altKey`, or `AltGraph` active): AltGr+8 on a German layout types `[` and navigates nowhere; the overlay and tooltips show the layout-resolved physical-key label where `getLayoutMap()` exists and the US label with an explanation otherwise.
- **U-TOAST-1**: Every confirmation, success and Undo toast stays >= 8 s, pauses on hover/focus, has a dismiss control, is also announced in the live region, and has no slide/fade under `prefers-reduced-motion`; a blocking banner is `role="alert"` and all other banners `role="status"`.

### Cross-cutting accessibility
- **U-A11Y-1**: All Files-tab flows are completable by keyboard alone on desktop (no mouse-only action, no keyboard trap); tab order is tree/search -> tabs -> editor -> status bar.
- **U-A11Y-2**: Text contrast >= 4.5:1 and non-text indicators (dirty dot, pin glyph, diff glyphs, focus ring, toggles pressed state) >= 3:1 in both light and dark themes. **Automated**: axe covers text contrast only, so a Playwright helper (`contrastRatio`) computes the contrast of each named indicator (found by `data-testid`) against its computed background in both `prefers-color-scheme` values and fails below 3:1; the manual spot check remains for judgment cases.
- **U-A11Y-3**: axe reports no WCAG AA violations on the Files tab with search results, a conflict dialog, and a merge view open.
- **U-A11Y-4**: The Explorer|Search panel resize handle is keyboard-operable: `role="separator"`, `aria-orientation="vertical"`, `aria-valuenow`/`aria-valuemin`/`aria-valuemax`, focusable (`tabIndex=0`), Left/Right arrows resize in 16 px steps (Shift = 64 px), Home/End jump to min/max; no drag-only function remains (split view is deferred, so there is no second handle). Implemented in Story 1.3.6.
- **U-A11Y-5**: At 200% text zoom and 320px width, content reflows with no loss of function. **Automated**: Playwright at 320x568 with root `font-size: 200%` asserts `scrollWidth <= innerWidth` (code area excepted), every primary control's bounding box lies within the viewport horizontally or is reachable by scrolling its own container, and no two primary controls' boxes overlap; the manual check covers screen-reader zoom.
- **U-A11Y-7**: A manual screen-reader script (iOS VoiceOver and Android TalkBack on the owner's phone, plus NVDA or VoiceOver on desktop) passes for: opening a file, reading code in Read mode, entering Edit mode, typing, saving, the conflict dialog, the merge view, the tab sheet and search results. CodeMirror's touch editing is the high-risk part (its content element and IME composition); the script is Task 2.5.4b, runs the concrete pass criteria of 8.10 / Story 0.1.6 against the **Accessible view** (the guaranteed read path) and against CM Read mode (informational), and its result is recorded in the manual checklist.
- **U-A11Y-8**: On mobile the DOM and focus order is top bar (back, filename, tabs, Save, Stop editing, overflow) -> banners -> pane content -> floating controls -> key bar -> bottom bar; sheets and dialogs trap focus; verified by a Playwright `Tab` sequence read by ARIA role and name at 390x844 and 320x568, and by the swipe-order step of the manual script.
- **U-A11Y-9**: The Accessible view (8.10) renders every line of the active page as real text with its line number, in pages of at most 500 lines with a pager and Go to line; search hits and Go to line focus the target line; it is reachable by a skip link and an Overflow item; axe reports no violations; Edit switches back to CodeMirror and announces it; with a dirty buffer it shows the draft with the unsaved label and Save (U-EDIT-22).
- **U-A11Y-6**: Every dialog, sheet, and popover returns focus to its invoker on close.

### Large files and delta saves (ADR-036; sections 8.12, 8.13, 9.4)
- **U-LARGE-1**: A file over 1 MiB (flag on, server advertises line reads) opens windowed: the tab and its pending state appear at once (8.5), the first window paints from one range read, the status bar reads "Ln N of about M" until the line count is exact, and a reload restores the same anchor line from persisted tab state without storing any file text.
- **U-LARGE-2**: An unloaded gap is a placeholder reading "Loading lines A-B..." with a height proportional to the gap; it cannot be typed into, partly deleted or edited, announces loading politely at most once per burst after 500 ms, and a failed load shows "Couldn't load lines A-B. Retry" with a 44 px Retry while the rest of the file stays usable.
- **U-LARGE-3**: Go to line, a search hit or a deep link far beyond the loaded window shows "Going to line N..." with a 44 px Cancel after 300 ms, lands with the line within one line height of center and announces "Line N", clamps beyond the end with a stated message, and before the index exists a far Go to line queues with "Waiting for the file index..." and Cancel while a search-hit jump does not wait.
- **U-LARGE-4**: While analysis is pending an "Indexing file..." chip (text plus spinner) shows, the count reads "about N lines", and Edit is `aria-disabled` with the visible reason "Preparing file for editing"; completion announces "File ready to edit" and enables Edit.
- **U-LARGE-5**: A file over the hash cap is view-only: Edit is `aria-disabled` with the visible reason "Read-only: file too large to edit", the reason chip opens an explanation, and Download remains available.
- **U-LARGE-6**: Entering Edit on a T1 file shows the dismissible "Large file: Save sends only your changes" status banner once per device; typing into a placeholder is refused with a polite "Load these lines to edit them"; the Save, dirty and discard rules are those of small files.
- **U-LARGE-7**: Scrolling away evicts clean chunks to placeholders without moving the viewport by more than one line height and without losing the selection or undo history; a region with unsaved edits is never evicted.
- **U-LARGE-8**: A file that changes on disk while open raises the stale banner of U-EDIT-11 (never claiming an actor); Reload re-anchors the same line and drops cached chunks; a torn window is retried once silently and never shown as current.
- **U-LARGE-9**: The Accessible view pages T1 and T2 files through the transport ("Lines 1-500 of about N"), announces loading and failed pages politely with a 44 px Retry, and does not depend on which lines the editor has mounted.
- **U-LARGE-10**: A line over 256 KiB renders in wrapped read-only windows with the reason "Read-only: very long line", and binary data found after the first window switches to the Binary state with Download.
- **U-LARGE-11**: When the transport demotes after three consecutive transport-class failures, a quiet text chip "Using simpler file mode" appears with an explanation popover and a Reload exit, the file behaves as in the base plan (truncated, read-only), no dialog or error toast appears, and application errors such as a conflict never trigger it.
- **U-PATCH-1**: A patch save looks like any save: one request in flight (a second Ctrl+S sends nothing and shows the spinner), "Saved" is announced, the dirty dot clears; the UI never shows payload sizes, offsets or transport names.
- **U-PATCH-2**: A rejected patch (offset or result mismatch) writes nothing and keeps the buffer dirty behind a blocking banner; for a file up to 1 MiB it offers Save whole file, Copy my version and Keep editing, for a larger file Copy my edits as patch, Reload (keep my edits) and Keep editing (no whole-file save exists for large files).
- **U-PATCH-3**: A diverged buffer shows "Edits can no longer be verified" and stops deltas for it: up to 1 MiB the next Save sends the whole file, above it Save is blocked until Reload (keeping the edits as a draft).
- **U-PATCH-4**: A change too large for a patch saves the whole file with a polite "Saving the whole file" for files up to 1 MiB; above 1 MiB Save is disabled with the visible reason "Change too large to save as a patch" and Undo and Copy my edits as patch are offered; nothing is split across requests.
- **U-PATCH-5**: A T1 conflict opens "File changed on disk (too large to compare)" with Merge and Overwrite disabled and a visible reason, Keep editing as default focus, Reload (keep my edits) and Rebase if lines unchanged; a successful rebase saves and announces "Saved", a failed one says "The file changed around your edits - Reload and re-apply" and writes nothing; Esc and back are Keep editing.
- **U-PATCH-6**: Offline with a ChangeSet draft shows "You are offline - N changes kept locally"; the Restore prompt reads "Restore N changes to <file>?" with default focus Not now; a draft that does not match the file fails closed with "Draft doesn't match the file" and offers Copy my edits as patch.
- **U-PATCH-7**: The periodic reconcile shows "Checking file..." without blocking typing and announces nothing unless it fails, in which case the U-PATCH-3 state appears.
- **U-PATCH-8**: Turning `files:delta-write` or `files:edit-large` off mid-session keeps small-file editing (whole-file saves) and drops a large-file buffer to Read with the banner "Editing large files is turned off. Your changes are kept as a draft." and loses nothing.
- **U-PATCH-9**: A large-file save whose response was lost shows "Can't tell if the last save landed. Reload to check; your edits are kept as a draft." with Reload (keep my edits) and Copy my edits as patch, and never retries automatically.
- **U-PATCH-10**: Edit on a large file never accepts typing before analysis is ready (U-LARGE-4), and entering Edit on a large file whose last verification diverged shows the U-PATCH-3 state instead of continuing silently.

### Observability (non-interactive)
- **U-OBS-1**: A save logs session, path, bytes, and outcome (`ok|conflict_rejected|denied|disabled|error`, including `conflict_rejected`) with no file contents.
- **U-OBS-2**: A search logs query length, duration, result count, and truncation with no query text at info level.
- **U-OBS-4**: Using search, tabs, edit, save, conflict, merge and the mobile key bar emits the ADR-035 `files.*` events with only enum, boolean and bucket label values; a canary file path, file name and query text typed or opened during the flow never appear in any tracked event (name, labels, component).
- **U-OBS-3**: Flipping `files:editing` takes effect on open sessions within one flag refresh without a reload and without losing buffers.
- **U-OBS-5**: Windowed reads, patch saves, transport fallbacks and rejected patches emit only the ADR-035 transport events with closed-enum, boolean and bucket labels (`transport`, `fallback_reason`, `payload`, `ops`, `tier`, `patch_reason`); a canary path, file name, line number, offset and text typed or opened during those flows never appears in any tracked event, and every patch also produces exactly one server `files.server.patch` outcome and one slog line without content.

## 19. Open UX risks carried to implementation

1. iOS PWA keyboard, `visualViewport`, and edge-swipe behavior: owner real-device check (existing blocker).
2. Thresholds for degraded rendering (600 KB, 4,000-char line) are placeholders until measured on a phone.
3. Esc handling with vim on makes the keyboard-trap escape depend on a dedicated gesture; resolved by the 8.4 context table and a plan AC (Story 2.4.1): an armed Esc, Esc handler (hint chip, capture phase, registered after vim) that a vim `:nmap <Esc>` cannot override, `:leave`, and the status-bar Leave editor control, tested in all vim modes. Remaining risks: real-device iOS behavior; a habitual double Esc leaving the editor (mitigated, not removed: it needs vim already in Normal with nothing pending and the visible chip, it is undone by one Esc or Enter, and a per-device setting turns it off); and the Tab-order claim that was wrong with vim on (corrected in 8.4).
4. Whether the existing quick-open already meets combobox/listbox ARIA is unverified; Task 1.3.5b audits it before U-PAL-1 can be asserted.
5. Screen-reader access to CodeMirror on touch is unproven (CM6 virtualization, IME). Mitigation: the Phase 0 spike (Story 0.1.6) measures VoiceOver/TalkBack against concrete pass criteria, and the Accessible view (8.10, Story 1.5.4) is built regardless so reading never depends on CM6's DOM; screen-reader *editing* stays best-effort until the manual script (Task 2.5.4b) says otherwise.
6. Hardware-keyboard auto-detect is conservative by design (default Touch); a Bluetooth keyboard user may need the manual override once per device. The Phase 0 capture (Story 0.1.5) supplies real event shapes for the fixtures.
7. Browser-reserved chords cannot be tested in CI (14.1); the owner's per-browser manual check is the only proof of delivery.
8. **Known limit: no 3-way merge below 360px (stated explicitly, triad round 3).** The phone merge needs a container of at least 360px (three 44px targets plus text per card, section 10.1). On a 320px phone, in split-screen or with a large system font the **Merge... action is unavailable** and the 2-way conflict flow (Keep editing / Reload / Overwrite / Copy my version) is the only resolution path; nothing is lost, but a user who needs a true merge must widen the window or use a larger device. This is accepted, not a defect, and is recorded in requirements.md Constraints.
9. Android system Back with the soft keyboard up, the AltGr/layout-label behavior and the real-device draft-backup timing on iOS (an IndexedDB write started in `pagehide`) are UNVERIFIED until the owner's phone and desktop checks (Tasks 2.5.4a and 2.5.2); CI uses stubs for all three.
10. **Scroll and gap interplay in CodeMirror on iOS (ADR-036, UNVERIFIED until the extended S-T3 gate, Task 0.1.4d).** The windowed document relies on placeholders loading above the viewport without a visible jump; if the gap-line model fails on the owner's phone the window-swap model (one editable window) or the contingent custom engine replaces it, and 8.12/8.13 keep the same states with "Edit this window" wording for the window-swap case.
11. **Large-file states depend on constants that are proposals** (the 64 MiB hash cap, 1 GiB read ceiling, 256 KiB long-line threshold, 500 ms and 300 ms announcement thresholds); the copy that quotes limits reads them from the capabilities response, never from literals, and the thresholds are adjustable in one place each.
12. **No whole-file save exists for files over 1 MiB** (stated plainly, ADR-036): a patch the server refuses can only be recovered by Reload (keeping the edits as a draft), Copy my edits as patch or Undo, never by a "save everything" action; this is accepted because sending a whole large file is the failure mode the delta path removes.
