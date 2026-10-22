# UX Research: files-viewer-revamp

Phase 2, UX dimension. Confidence labels: VERIFIED = read in this repo; PRIOR-ART = from my knowledge of the named products, not re-fetched this session (no web lookups were run); PROPOSAL = my design recommendation.

## 0. Repo baseline (VERIFIED)

- `project_plans/files-tab-ux-review/ux-review.md` already lists the gaps this project closes: no jump-to-line, no hunk navigation, no content search, vim `H`/`L` bound to selection (wrong semantics), no mode indicator, no vim discoverability, no focus restoration on tab switch, ARIA on the CodeMirror host unverified.
- `FileTree.tsx:337-340,907-909` already uses `role="tree"` / `role="treeitem"` with `aria-level`, `aria-selected`, `aria-expanded`. The tree ARIA base exists; tabs, combobox and results-list roles do not.
- `project_plans/mobile-ux-improvements/research/findings-pitfalls.md:10-17`: `dvh` does not track the iOS keyboard; use `visualViewport` (resize and scroll events, debounced, ~300-500 ms lag); `position: fixed` is wrong under the keyboard on iOS 17; iOS PWA is worse. `ViewportProvider.tsx` and `globals.css` already use `visualViewport`, so the editor should reuse that provider rather than add a second listener.
- The terminal already has a mobile supplemental key overlay (Esc, Tab, Ctrl+C, arrows, 44px keys). Reuse its component and styling for the editor extra-key bar.

## 1. Comparable patterns

| Product | What works | What to copy | What to avoid |
|---|---|---|---|
| VS Code desktop | Search view: query box with `Aa`/`ab`/`.*` toggles, include/exclude glob fields, results grouped by file with match count, collapsible, click opens at line with highlight. Tabs with preview (italic) tab that is replaced by the next single-click; dirty dot on tab; `Ctrl+P` quick-open, `Ctrl+G` goto line, `Ctrl+Shift+O` symbols, `Alt+Left/Right` navigation history. | Grouped results with per-file counts; preview tab; toggles; history stack independent of tab order. | Replace-in-files (out of scope). Split-editor groups on phone. |
| VS Code web / GitHub.dev | Same shell, degraded on touch: tiny tab targets, command palette is the real mobile entry (`F1`). Search is a client-side/index query with streamed results. | Command-palette-as-primary on small screens; streamed results. | Its phone layout is a shrunken desktop; tab strip and activity bar targets are under 44px. |
| Vim / ranger | Modal keys, `/` incremental search, `Ctrl-o`/`Ctrl-i` jumplist, `:N` goto line, `gf`, `:grep` + quickfix list (`]q`/`[q`), `m{a-z}` marks. Quickfix is the mental model for a results panel: a persistent list you step through with one key while the buffer stays focused. | Next/prev result keys that keep editor focus; jumplist back/forward; `:N`. | Operator grammar beyond what CodeMirror vim already provides; do not hand-roll more motions (see section 2). |
| Textastic / Koder (iOS) | Extra-key row above the keyboard (Tab, brackets, arrows, undo/redo, Esc) is the single most important phone-editing affordance. Sidebar slides over content; tabs scroll horizontally; find-in-files is a full-screen modal. | Key row with Tab, Esc, arrows, `{ } [ ] ( ) ; : / \`, undo/redo; full-screen search. | Modal-only search that loses the results when you open a hit (forces re-search). |
| Working Copy (iOS) | Opens a file in a pushed screen (nav-stack back arrow), search results as a pushed list where back returns to the list with scroll preserved; explicit "uncommitted changes" indicator; read-only by default with explicit Edit toggle. | Nav-stack semantics (results -> file -> back to results with state kept); read-only-by-default plus explicit edit toggle (maps to `files_editing` flag and prevents accidental edits on a touch screen). | None notable. |
| Termux | Terminal with an extra-keys row (Esc, Ctrl, Alt, Tab, arrows) where Ctrl/Alt are sticky one-shot modifiers. Volume-key shortcuts. | Sticky one-shot Ctrl (tap Ctrl, then tap a letter) is the right answer for Ctrl-d/u/o/r on a soft keyboard. | Modifier-lock without visual state. |

Flows that work (cross-product synthesis, PRIOR-ART):
1. **Results panel**: grouped by file, match line shown with the hit highlighted, count in header, keyboard step (next/prev) that does not leave the editor, results panel persists after opening a hit.
2. **Tabs**: preview tab, dirty indicator, overflow scroll with active tab auto-scrolled into view, close-others on long-press/context menu, and a separate back/forward jumplist.
3. **Phone editing**: read-only default, explicit enter-edit, extra-key row pinned above the keyboard, one-shot sticky modifiers, no hover-dependent UI.

## 2. Mental models for "Vim-or-VS-Code-like browsing"

The owner has two overlapping models; the UI should satisfy both without forcing a choice per action:

- **VS Code model** (mouse/palette): tree on the left, tabs on top, `Ctrl+P` find file, `Ctrl+Shift+F` find in files, click a hit to land on the line, `Ctrl+S` saves, dot on dirty tab.
- **Vim model** (keyboard/buffers): buffers not tabs, `/` and `n`/`N` in-file, `:N` goto line, `Ctrl-o`/`Ctrl-i` jumplist, `:w`, quickfix-style stepping through results, `gf`/`gd`-like "go to thing under cursor" (we have no LSP, so `gf` on a path-like token is the achievable subset).

Expectations to honor:
- Back/forward = the jumplist (cursor positions across files), not tab order. Bind `Alt+Left/Right` (browser/OS-safe on desktop) and `Ctrl-o`/`Ctrl-i` when vim mode is on.
- `Ctrl+P`, `Ctrl+Shift+F`, `Ctrl+G`/`:N`, `Ctrl+S`/`:w` must all work; `Ctrl+Shift+F` and `Ctrl+P` collide with browser shortcuts (print / find), so `preventDefault` only when focus is inside the Files tab, and offer `/`-style alternatives when the editor is not focused.
- Vim mode should be the real thing (CodeMirror vim extension) rather than the current partial keymap; the existing partial keymap's `H`/`L` bug is the sort of false-friend that breaks trust (ux-review section 3). Show a mode indicator (`-- NORMAL --`, `-- INSERT --`) in a status line on both form factors.
- Default: vim mode off for fresh users is the safer default, but the owner is a vim user; make it a persisted per-device setting (localStorage) with a visible toggle in the editor status bar. Flagged as an owner decision (requirements open question), PROPOSAL: default ON on desktop where a hardware keyboard is detected, OFF on touch-primary devices; the owner can override either way.

## 3. Accessibility (WCAG 2.2 AA)

Checklist mapped to this feature (PRIOR-ART for the standards; VERIFIED where noted):

| Area | Requirement | Application |
|---|---|---|
| Tree | WAI-ARIA tree pattern: Up/Down move, Right expands/enters, Left collapses/parent, Home/End, type-ahead, `Enter` activates, roving tabindex or `aria-activedescendant`. | Already has roles (VERIFIED); verify roving tabindex and that `j/k/h/l` aliases do not conflict with type-ahead (type-ahead and single-letter vim aliases collide; keep vim aliases only when the tree has focus AND vim mode is on). |
| Tabs | `role="tablist"`/`tab`/`tabpanel`, `aria-selected`, `aria-controls`; Left/Right between tabs, Home/End, `Delete` closes (closable tabs), automatic activation. Dirty state announced: include "unsaved" in the accessible name, not only a dot. | New. Tabpanel keeps the editor mounted (state preservation already designed). |
| Quick-open / goto / command palette | `role="combobox"` on the input with `aria-expanded`, `aria-controls` -> `role="listbox"`, `aria-activedescendant` for the highlighted `role="option"`; Esc closes and restores focus to the trigger; result count announced via polite live region. | Existing quick-open should be audited against this (UNVERIFIED which parts it meets). |
| Search results | Use a tree or listbox: file groups as `role="group"` with option rows; announce "N results in M files" via live region; do not steal focus from the query input while typing. Matched substring uses `<mark>` plus text, not color alone (1.4.1). | New. |
| Editor | CodeMirror sets `role="textbox"` + `aria-multiline` (INFERRED, ux-review said unconfirmed); add `aria-label="File: <path>"` and `aria-readonly`; keep Tab usable: **Esc then Tab** to leave the editor (CodeMirror's `indentWithTab` is a keyboard trap risk, WCAG 2.1.2) and document it in a help overlay. Vim mode live indicator should be `role="status"`. | Set `EditorView.contentAttributes`. Do not enable `indentWithTab` unconditionally; make it a toggle or use Esc-then-Tab escape. |
| Focus management | On open-from-results, focus moves to the editor at the line; on close tab, focus the neighbor tab; on dialog (conflict diff, discard prompt) trap focus and return it on close; on tab switch back to Files, restore last focus (gap noted in ux-review section 6). | Needed in every flow below. |
| Targets | 2.5.8 minimum target 24x24 CSS px; the requirements demand 44px, which is stricter (Apple HIG). Applies to tab close buttons (commonly tiny): make the tab tall 44px and the close hit-area 44x44 via padding even if the glyph is 16px. | Use 44px for all primary controls; 32px acceptable on desktop pointer devices only when spacing keeps 24px target clear. |
| Reflow / zoom | 1.4.10 reflow at 320px width, 1.4.4 200% text resize: no horizontal page scroll (the code area may scroll). | Playwright mobile viewport 320x568 and 390x844 checks. |
| Dragging | 2.5.7: any drag (resize tree, reorder tabs) needs a non-drag alternative (buttons/keyboard). | The existing `TreeResizeHandle` needs arrow-key resize (`role="separator"` + `aria-valuenow`). |
| Contrast | 1.4.3 text 4.5:1, 1.4.11 non-text 3:1 for diff/gutter/dirty indicators; do not use red/green alone for conflict diff add/remove (use +/- gutter glyphs). | Contrast unverified in the existing theme (ux-review flagged it). |
| Status messages | 4.1.3: "Saved", "Save blocked: file changed", "Search truncated at N" via `aria-live="polite"` (errors `assertive`). | Single toast/status region in the Files tab. |
| Motion | `prefers-reduced-motion` disables bottom-sheet slide animations. | Applies to the sheet and pane transitions. |
| Accessible authentication / consistent help (3.2.6) | Help/shortcut overlay in the same place on every view (`?` and a toolbar button). | PROPOSAL. |

## 4. Error states and edge cases

Each needs a defined UX, not a generic toast. Severity ordered by data-loss risk.

1. **Save conflict (file changed on disk since load)**. Block the save. Modal (desktop) / full-screen sheet (mobile) titled "File changed on disk". Show a unified diff of "your edits vs. current disk" (not disk vs. original, because the user decides what to keep). Actions: **Overwrite** (destructive style, confirm text names the path), **Reload from disk** (discards edits; offer "copy my version to clipboard" first), **Keep editing** (default/primary, dismisses). Merge: PROPOSAL cut as a later stage; a read-only three-way view is expensive on a phone. Provide a "show only changed hunks" collapse and next/prev hunk. Never auto-resolve. Log line per requirements.
2. **File changed on disk while open and clean** (the agent edits it). Quiet auto-refresh is only safe if the user has not moved the cursor mid-read; PROPOSAL: show a non-modal banner "Changed on disk (agent) - Reload" and keep content stable; auto-reload only when no unsaved edits and the user triggers it, or when the viewer is read-only and opted into "follow changes". With unsaved edits: banner turns to "Changed on disk - review diff". Banner must be `role="status"`, not a focus thief.
3. **Unsaved changes on navigation**: closing a dirty tab, switching sessions, session pause, browser `beforeunload`, mobile pane back. Prompt with Save / Discard / Cancel (Save is default). For session pause/stop initiated elsewhere (by agent or backlog automation) a blocking prompt is wrong; instead, persist an unsaved draft per (session, path) in localStorage and offer "Restore draft" on next open, and warn the pause was not blocked. Server worktree deletion on pause makes the base file disappear: restoring a draft must handle a missing target (offer "Save as new file" is out of scope; offer copy to clipboard).
4. **Truncated files (>1 MB) and rejected (>10 MB)**: read-only with an explicit banner "Showing first 1 MB of N MB"; editing disabled (saving a truncated buffer would destroy data - this is a hard rule, not a UX nicety). Offer Download and "Open in new tab" (exists). Search hits beyond the truncation point cannot jump to the line: show "Line N is beyond the loaded portion".
5. **Binary / non-UTF-8 / CRLF**: binary shows a placeholder (type, size, Download), never an editor. Non-UTF-8 text: read-only with encoding notice. CRLF/trailing newline: preserve exactly on save; show `CRLF` / `LF` in the status bar. Search skips binary by default with a "N binary files skipped" note.
6. **Search edge cases**: invalid regex -> inline error under the field (not a toast), keep prior results; zero results -> empty state with the active toggles listed ("No matches for `foo` (case-sensitive, regex)") and a one-tap "clear filters"; truncated at cap -> "Showing first 500 of many - refine your query"; slow search -> streaming/progress with Cancel; query on a worktree that vanished (paused session) -> "Session paused; worktree unavailable".
7. **Offline / reconnect**: the PWA on a phone loses connectivity regularly. Detect via failed RPC plus `navigator.onLine`; show a persistent status chip "Offline - edits kept locally". Dirty buffers stay in memory and draft storage; Save is disabled with an explanation (not silently queued, because a queued write against a changed file is the unsafe case); on reconnect re-check the disk version and route through the conflict flow if it changed. Reads: cached open tabs remain viewable.
8. **Permission / path errors**: write rejected due to path confinement or read-only mount -> inline banner with the server's reason class (no raw error strings with paths of other users); keep the buffer.
9. **Save in flight**: disable Save, show spinner; a second save is queued/ignored; if the response is lost, the next save must re-check the token (idempotent "unknown outcome" -> reload state before allowing further saves).
10. **Large file on phone**: skip syntax highlighting above a size/line threshold and disable minimap/wrapping features; show a one-line notice. Very long single lines (minified) -> force wrapping off with horizontal scroll; PROPOSAL: threshold values to be set from phone measurements in implementation.
11. **Feature flag off mid-session**: editor drops to read-only without losing the buffer; a banner explains.

## 5. Jobs-to-be-done

- **Functional**: (a) "When an agent changes my code, I want to find where something is used/defined across the worktree so I can verify the change." (b) "When I notice a one-line mistake away from my desk, I want to fix and save it without opening a terminal." (c) "When I follow a trail across files, I want to go back to where I was." (d) "When I read a diff-adjacent file, I want to hop between hunks and the related files without losing my place."
- **Emotional**: confidence that nothing was overwritten (the agent and I both write here); calm instead of dread when saving; feeling in control of agent output while mobile; familiar muscle memory (vim/VS Code) instead of re-learning.
- **Social**: demonstrably reviewed work (reviewing agent output before it ships/merges; supports "mark reviewed" idea from the earlier UX review); being able to answer a teammate's or the agent's question from a phone ("where is X handled?") without excusing "I'll check at my desk".
- Implication: the highest-value path is read-only search -> jump -> back (jobs a, c, d). Editing (job b) is the highest-risk and lowest-frequency job, which supports the requirements' staging (editing last, flag-gated) and read-only-by-default with an explicit Edit toggle.

## 6. Proposed layouts

### 6.1 Breakpoints and model
- Mobile (< 768px, matches the repo's existing 768px media query) and desktop (>= 768px). Tablet portrait counts as desktop layout with a collapsible tree. Use the container width of the Files tab, not the window, where practical, since the tab lives inside a session detail view with other chrome.
- One state model for both: `{openTabs[], activeTab, jumplist[], searchQuery/results, paneState}`; layouts are different views of it. This keeps e2e coverage symmetric.

### 6.2 Mobile layout (PROPOSAL)

Navigation-stack model with three full-screen panes, not a cramped split, matching Working Copy/Textastic: **Files (tree)** -> **Editor** -> (overlay) **Search**.

- **Top bar (44px)**: back arrow (to tree or previous jumplist entry; long-press = back history list), file name + dirty dot (truncate left, keep tail), overflow `...` (Wrap, Vim mode, Download, Open in new tab, Edit/Read-only). Tab switcher button showing open-tab count `[3]` (like mobile Safari/Chrome): opens a full-height **sheet of open tabs** (cards/list with close buttons, 44px rows), instead of a horizontal tab strip which has tiny targets.
- **Bottom action bar (thumb zone, 48px)**: Files | Search | Quick-open | Go-to-line (`:`) | Back/Forward. Primary navigation is reachable one-handed, which the success metric requires. It hides while the soft keyboard is open (the extra-key bar replaces it).
- **Search**: opens as a full-screen pane (not a bottom sheet), because results lists need height. Query field at top with `Aa`/`.*`/glob toggles in a collapsible filter row; results grouped by file (sticky file header, count, collapse). Tapping a hit pushes the Editor pane at the line, with the matched text highlighted and a **floating results chip** "3/27 ^ v" for next/prev hit that stays until dismissed (the quickfix equivalent); back returns to the results with scroll preserved. A bottom sheet is appropriate only for **secondary** content: the symbol outline, tab list, conflict actions, and the keybinding help (half-height, drag-to-dismiss, with a visible close button and Esc/back dismissal, focus-trapped).
- **Editor read mode**: no soft keyboard by default (CodeMirror set to read-only with `inputmode="none"`/not focused on tap) so tapping to read does not pop the keyboard; a floating **Edit** button (44px, bottom-right above the action bar) enters edit mode if `files_editing` is on. This avoids the common mobile-viewer annoyance of the keyboard covering code on every tap.
- **Editor edit mode**: soft keyboard up; the **extra-key bar** is pinned directly above the keyboard using `visualViewport` (reuse `ViewportProvider`, per pitfalls doc: do not use `position: fixed; bottom:0` alone). Layout: horizontally scrollable single row of 44x44 keys: `Esc | Tab | Ctrl* | Alt* | ^ v < > | { } [ ] ( ) | ; : " ' / \ | = - _ | undo redo | Save`. `Ctrl*`/`Alt*` are sticky one-shot modifiers (Termux model) with a visible pressed/locked state (double-tap = lock). In vim mode the bar's `Esc` is the most-pressed key (soft keyboards have no Esc), so it is first and visually emphasized; plus a configurable second row for vim (`: / h j k l`) shown only in normal mode. Save key shows dirty state and is disabled when clean.
- **Gestures** (all with a button equivalent for accessibility, 2.5.1/2.5.7): edge swipe right = back (do not hijack the iOS system edge-swipe in Safari: use the in-app back arrow as primary and treat the swipe as an enhancement only within the PWA, flagged as risky); swipe left/right on the editor header or tab sheet cards to switch tabs / close a tab card; long-press a file in the tree for actions (copy path, open in new tab); pull-to-refresh on the tree is NOT used (conflicts with browser pull-to-refresh and with scrolling); pinch-zoom of text is replaced by a font-size stepper in the overflow menu. Double-tap word select and CodeMirror's native touch selection are left untouched.
- **Keyboard-safe layout**: root pane height from `visualViewport.height`; caret-into-view on focus via CodeMirror `scrollIntoView` after the debounced viewport resize (300-500 ms lag, per pitfalls doc); `env(safe-area-inset-bottom)` padding on the bottom bar. iOS PWA mode needs a real-device check (listed blocker in requirements).
- **Landscape phone**: viewport is short; hide the top bar title and bottom action bar while editing, keep only the extra-key row.

### 6.3 Desktop layout (PROPOSAL)

VS Code-like, three regions inside the Files tab:
- **Left**: resizable side panel with a two-mode header, **Explorer | Search** (segmented tabs, `Ctrl+Shift+F` switches to Search; the tree and the results each keep state). The existing resizable tree handle stays; add keyboard resize. Search panel: query row with toggles (case, word, regex), a collapsible "files to include / exclude" row, results grouped by file with counts; arrow keys walk results, `Enter` opens (preview tab), `Ctrl+Enter` pins, `Esc` returns focus to the editor. The editor shows the hit highlighted and keeps the results panel visible so `F4`/`Shift+F4` (or `]q`/`[q` in vim) steps to the next/prev hit.
- **Center**: tab strip (scroll with chevrons, drag-reorder with a keyboard alternative, preview tab in italics, dirty dot, middle-click and `Ctrl+W` close, context menu: close others / copy path / reveal in tree), breadcrumb row (path segments clickable; optional outline dropdown), then the editor. Optional split (editor right) is the last-priority item; if cut, nothing else depends on it.
- **Bottom status bar (24-28px)**: `-- NORMAL --` vim mode, `Ln, Col`, encoding, `LF/CRLF`, language, read-only/dirty state, save status, and offline/changed-on-disk chips. This doubles as the visible discoverability for vim (ux-review recommendation 2).
- **Back/forward**: toolbar arrows and `Alt+Left/Right`, `Ctrl-o/Ctrl-i` in vim mode; history entries restore scroll and cursor per file.
- **Command palette** (`Ctrl+Shift+P` or `>` in quick-open): one place to discover `Go to line`, `Toggle vim mode`, `Toggle wrap`, `Save`, `Reload from disk`, `Show shortcuts`. Cheap, and gives phone users the same actions through the Quick-open button.
- **Large screens/tablet landscape**: same layout, tree collapsible with `Ctrl+B`.

### 6.4 State persistence (answers requirements' open question, PROPOSAL)
Open tabs, active tab, per-file scroll/cursor, tree width, vim toggle, and search toggles: **localStorage per (session id, device)**, because tab layouts are device-specific (phone and desktop need different sets) and need no server work. Unsaved drafts: localStorage keyed by (session, path, base token). Do not persist file contents of non-dirty tabs. Cross-device sync is not required (single user, social/functional jobs do not depend on it). Wrap all storage access in try/catch.

## 7. Recommendations summary

1. Treat search -> jump -> back with persistent results (quickfix model) as the flagship flow; ship it first and read-only.
2. Mobile = nav-stack panes + bottom action bar + tab-switcher sheet + extra-key bar above the keyboard with sticky modifiers; bottom sheet only for secondary content. Read-only by default with explicit Edit.
3. Desktop = Explorer|Search side panel, tab strip with preview tabs, status bar with vim mode and file state, command palette.
4. Use the real CodeMirror vim extension (fixes the `H`/`L` false friend), show mode indicator, make Esc-then-Tab the keyboard escape from the editor.
5. Editing safety UX is non-negotiable: truncated/binary = read-only; conflict = block + diff + explicit choice; clean file changed on disk = banner not auto-replace; offline = disabled save + local draft.
6. Accessibility work items: tablist/tabpanel, combobox+listbox for palettes, results listbox with live region count, focus restoration, 44px targets including tab close, non-drag alternatives, contrast and non-color diff cues, 320px reflow e2e.

## 8. Open questions for planning (not decided here)
- Vim mode default per device (owner choice; proposal in section 2).
- Whether merge (three-way) in the conflict dialog is in scope or deferred; proposal is overwrite / reload / keep editing now, merge later.
- Real-device iOS PWA verification of the keyboard bar and edge-swipe behavior (already a listed wall-clock blocker).
- Which accessibility items can be automated (axe in the existing UX-analysis CI) versus manual screen-reader checks.
