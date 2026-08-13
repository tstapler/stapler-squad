# Files Tab — UX Research & Review

**Scope**: Evaluate the Files tab (CodeMirror 6 migration, commit `e6ea0de3b`) as a
daily-driver code-reading/review tool. Functional work (theming, sizing, back button, vim
keybindings, tab-switch state preservation) is already implemented; this is the deeper
review the user asked for on top of that — is it *good enough*, not just "does it work."

**Files reviewed**: `web-app/src/components/sessions/FileContentViewer.tsx`,
`FilesTab.tsx`, `FilesTab.css.ts`, `FileContentViewer.css.ts`, `FileTree.tsx`,
`SessionDetailView.tsx`, `web-app/src/lib/utils/parseDiff.ts`,
`web-app/src/components/files/LocalFileBrowser.tsx` (separate standalone `/files` page —
not part of the session Files tab, no CodeMirror or vim bindings there; noted for contrast
only).

---

## 1. Navigation efficiency

**Session list → reading a specific file's diff**, best case:

1. Click session card (1 click)
2. Click "Files" tab (1 click)
3. Click a changed file in the tree, or `⌘P` quick-open + type + Enter (1 click, or 1
   shortcut + typing)

That's competitive — 2 clicks minimum once a session is open, and cross-linking from the
VCS panel (`onNavigateToFile` in `SessionDetailView.tsx:849-852`) skips the tree entirely
for diff-driven review.

**Gaps versus a real editor**, all confirmed absent from `FileContentViewer.tsx`:

- **No jump-to-line.** CodeMirror ships `gotoLine` in `@codemirror/search` but it isn't
  wired into `vimKeymap` or the toolbar — no `:123`-style or `Ctrl-G` entry point.
- **No jump-to-symbol / outline.** No `@codemirror/lang-*`'s built-in symbol support is
  exposed; there's no per-file outline or breadcrumb-of-scope.
- **No go-to-definition / find-references.** Expected — this isn't an LSP-backed editor —
  but worth naming explicitly since it's the single biggest gap versus "daily-driver for
  reading code."
- **Search-within-file exists** (`/`, `n`, `N` wired to `openSearchPanel`/`findNext`/
  `findPrevious`, `FileContentViewer.tsx:330-332`) but it's the default CodeMirror search
  panel, not vim's `/` semantics (no incremental highlight-as-you-type distinct from
  CodeMirror's own, no `*`/`#` word-under-cursor search).
- **No cross-file search results list.** `FileTree`'s search (`searchFiles`, filename-based)
  finds files by name, not by content — there's no "search across all files, jump to hits"
  flow the way VS Code's global search provides.

## 2. Diff/review workflow

The gutter-marker approach (`buildGutterMarks` in `parseDiff.ts`, rendered via
`ChangeGutterMarker` in `FileContentViewer.tsx:217-239`) paints a colored strip
(add/delete/modify) next to changed lines in the **current full-file view**. This is a
*decoration*, not a review tool:

- **No hunk navigation.** There is no next-hunk/prev-hunk command (confirmed: no
  `nextHunk`/`prevHunk`/"jump to change" symbol anywhere in `web-app/src/components/
  sessions/`). A reviewer must scroll manually to find each change; on a large file with
  scattered edits this is materially slower than GitHub's PR viewer (`n`/`p` or the
  "Jump to" dropdown) or any IDE diff gutter (click-to-jump minimap markers).
- **No side-by-side or inline diff rendering** in this view — it's the post-change file
  with markers, not a diff. To see *what changed* (old vs. new text) a reviewer has to
  separately open the "Diff" tab (`SessionDetailView.tsx:840-844`, `DiffViewer`) and lose
  the CodeMirror navigation/vim context. The two views are not connected — no shared
  scroll position, no "open this hunk in Files tab at this line."
- **No "mark file as reviewed" / progress tracking.** Confirmed absent via repo-wide grep
  (only hit was the unrelated `ReviewQueuePanel`, which reviews agent actions, not diffs).
  A PR-review-shaped workflow — "I've looked at 4 of 12 changed files" — has no state to
  hang off of.
- **Deletion-only hunks are approximated.** `buildGutterMarks`'s own comment documents that
  a pure deletion (no matching add) is marked on the line *after* the deletion point
  (`hunk.newStart`) as a heuristic "removed above" indicator — there is no way to see the
  actual deleted text without switching to the Diff tab.

Net: the gutter marker is a "something changed here" signal, not a review workflow. It's
strictly less capable than GitHub's file-viewer (which at minimum offers inline diff,
hunk-by-hunk expand/collapse, and viewed-file checkboxes).

## 3. Vim keybinding completeness/correctness

The implemented keymap (`FileContentViewer.tsx:294-333`) covers: `h j k l`, `0`/`$`, `gg`/`G`,
`Ctrl-d`/`Ctrl-u` (page), `Shift-h/l/j/k` and `Shift-0`/`Shift-$` (bound to
`selectCharLeft`/`selectLineDown`/etc.), `Shift-Ctrl-d/u`, `y` (yank-to-clipboard on active
selection), `/`, `n`, `N`.

Two problems, one of them a correctness bug that will actively surprise vim users rather
than just feel incomplete:

- **`Shift-h`/`Shift-l` are bound to the wrong vim semantics.** In real vim, `H`/`L`/`M`
  move the cursor to the top/bottom/middle of the *visible window* (screen-relative
  motion) — they are not "select left/right." Here they're wired to
  `selectCharLeft`/`selectCharRight` (`FileContentViewer.tsx:314-315`), i.e. the pattern is
  "hold Shift to extend a character-wise selection," borrowed from ordinary text-editor
  shift-arrow conventions, not from vim. A vim user pressing `H` expecting to jump to the
  top of the screen will instead silently extend a selection one character — a "false
  friend" that's worse than simply not implementing the key, because it does something
  wrong rather than nothing.
- **No visual mode (`v`/`V`), no operator+motion grammar** (`dd`, `yy`, `d$`, `ciw`, etc.) —
  everything is single-keystroke, there's no verb-then-motion composition at all. This is a
  reasonable scope cut for a read-only viewer (no `d`/`c` make sense without edits) but `v`
  to start a real visual selection that then accepts motions (`v` then `j/k/l/h` extends,
  matching actual vim) would have been achievable and is closer to muscle memory than the
  current Shift-prefixed scheme.
- **Missing common read-only navigation**: word motions (`w`/`b`/`e`), paragraph motions
  (`{`/`}`), character search (`f`/`t`/`;`/`,`), percent bracket-match (`%`), marks
  (`m`/backtick), jump list (`Ctrl-o`/`Ctrl-i`), and `.` repeat. None of these require
  editing capability, so their absence isn't scope-justified the way `d`/`c`/`i` are.
- **No mode indicator.** There is no on-screen "-- NORMAL --" or any UI element showing vim
  mode is active; this compounds the discoverability problem in section 5.

Verdict: it's a thin, partially-*incorrect* subset. A vim user will find the `hjkl`/`gg`/`G`
core reassuring but will hit the `H`/`L` mismatch quickly and conclude the keymap "sort of
gets in the way" rather than helps — worse for trust than shipping fewer bindings.

## 4. Performance/scalability

- **File tree**: built on `react-arborist` (`FileTree.tsx:4`), which virtualizes rows —
  this scales to large trees without a custom effort.
- **Editor content**: CodeMirror 6 (`basicSetup` from the `codemirror` package) does its own
  viewport-based line rendering internally, so raw line count in a single file shouldn't
  choke the DOM the way a naive `<pre>`-per-line approach would (contrast with
  `LocalFileBrowser.tsx`'s standalone `/files` page, which renders large text files as one
  giant `<pre>{textContent}</pre>` with no virtualization at all — a real scalability gap,
  though that's a different, unrelated component from the session Files tab).
- **Explicit ceiling**: the session Files tab truncates file content to 1&nbsp;MB
  server-side (`data.isTruncated` warning, `FileContentViewer.tsx:598-601`) — reasonable as
  a hard stop, but there's no progressive/streaming load, so a 900&nbsp;KB minified file or
  generated lockfile still round-trips as one large JSON payload and one full CodeMirror
  parse/highlight pass before anything renders — no "here's the first screen, rest is
  loading" affordance.
- **Language extension loading is lazy per-open** (`loadCodemirrorLang`, dynamic
  `import()` per language, `FileContentViewer.tsx:366-413`) — good for initial bundle size,
  but means every *first* open of a given language in a session pays an extra async
  round-trip before syntax highlighting appears (file renders unhighlighted, then
  re-renders once the language chunk resolves) — a minor flash-of-unstyled-code, not a
  scalability problem per se.
- **No evidence of a large-file or large-repo stress test** anywhere in the reviewed test
  files (`FileContentViewer.test.tsx` wasn't inspected line-by-line here, but nothing in
  the component itself defends against e.g. a file with hundreds of thousands of lines
  beyond the 1&nbsp;MB byte cap, which for a minified/generated file can still be tens of
  thousands of very long lines).

## 5. Discoverability

- **Back button** (`mobileBackButton`, `FilesTab.tsx:260-265`, "← Files") only renders in
  the mobile pane layout (`mobilePaneHidden`/`mobilePaneVisible` classes gated on
  `mobilePane` state) — on desktop widths there is no back-button affordance at all; the
  tree pane is simply always visible side-by-side, so "back" isn't meaningful there. Fine
  as a design choice, but worth confirming the "back button" the original ask cared about
  was specifically the mobile/narrow-viewport case — if a desktop back-to-tree gesture was
  also expected, it doesn't exist.
- **Vim keybindings have zero on-screen hint in the file content viewer.** The only
  keyboard hint text anywhere is `FileTree.tsx:869`'s `title` attribute ("Navigate with
  j/k/h/l • Enter to open • gg/G for top/bottom") on the tree `<div>` — a hover tooltip on
  a container, not a visible label, and it only covers tree navigation, not the CodeMirror
  pane's separate vim keymap. `FileContentViewer.tsx:470`'s empty-state hint
  ("Press ⌘P… to quick-open") disappears the moment a file is open, which is exactly when a
  user would want to discover `hjkl`/`/`/`n`/`gg` support. **A first-time user has no way
  to discover vim mode exists** short of trying `hjkl` speculatively or reading source —
  there's no help overlay, `?`-triggered cheat sheet, or status-bar mode indicator.
- Toolbar affordances (wrap toggle, download, open-in-new-tab, sort, collapse-all, refresh)
  are all icon/label buttons with `title` tooltips and `aria-label`s — reasonably
  discoverable by hovering, consistent with the rest of the toolbar's pattern.

## 6. Accessibility

- **Keyboard-trap risk is low but not zero.** `FileTree.tsx:721`'s `handleTreeKeyDown`
  explicitly bails on any Meta/Ctrl/Alt-modified key (`if (e.metaKey || e.ctrlKey ||
  e.altKey) return;`), so browser/OS shortcuts aren't swallowed there. The CodeMirror
  `vimKeymap`, however, intercepts bare `h/j/k/l/g/G/y/n` etc. globally within the editor's
  focus scope with no visible way to "exit" a mode (there is no mode to exit — it's
  always-on single-keystroke bindings) — practically this means a sighted mouse user who
  clicks into the editor and then tries to type `g` to trigger the browser's normal "find
  in page starting with g" or similar text-search behavior will instead move the cursor.
  Low severity since the editor is read-only and Tab/Escape aren't rebound, but the
  combination of "no mode indicator" + "single letters are live commands" is the classic
  precondition for an accessibility-adjacent surprise (a screen-reader or switch-access
  user typing text into what looks like a text area gets navigation instead).
- **No ARIA role annotation on the CodeMirror host** (`codeMirrorEditor` div,
  `FileContentViewer.tsx:363`) beyond whatever CodeMirror's own `contentDOM` sets
  internally (CodeMirror 6 does set `role="textbox"`/`aria-multiline` on its content DOM by
  default) — this wasn't independently re-verified here, so treat "CodeMirror handles it"
  as **inferred, not confirmed** for this specific `basicSetup` configuration.
- **Focus management across tab switches is unaddressed.** `SessionDetailView.tsx:856-869`
  keeps the Files tab's DOM mounted and toggles `display: none`/`aria-hidden` — good for
  state preservation (the explicit ask), but there's no focus restoration logic: switching
  from another tab back to Files does not appear to return focus to the previously focused
  tree node or editor position, which matters for both vim-keyboard users and screen-reader
  users tracking focus.
- **Color contrast**: chrome and syntax colors are sourced from theme tokens
  (`vars.color.terminal*`, `FileContentViewer.tsx:241-289`) rather than hardcoded, so theme
  switches repaint correctly — this is good practice, but no explicit contrast-ratio check
  (e.g. against WCAG AA) was performed as part of this review; flagging as unverified.

## 7. Concrete prioritized recommendations

Ordered by (impact ÷ effort), highest leverage first:

1. **Fix `Shift-h`/`Shift-l` semantics or rename the affordance.** Either rebind them to
   real vim window-relative motion (top/bottom of visible screen) or drop the vim framing
   for that pair and keep plain Shift-arrow-style selection under a different,
   non-vim-branded key. Cheapest fix, removes the single most likely trust-breaking
   surprise for the target power-user audience. *(Low effort, high impact — it's a coding
   fix in one keymap array, `FileContentViewer.tsx:314-319`.)*
2. **Add a discoverable vim/keybinding hint.** A small `?`-triggered cheat sheet or a
   persistent one-line hint in the toolbar ("hjkl · gg/G · / search · ⌘P open") closes the
   discoverability gap for both vim keys and the mobile back button at near-zero
   engineering cost.
3. **Wire up hunk navigation** (next/prev changed hunk, e.g. `]c`/`[c` — vim's own
   diff-navigation convention, or simpler `Alt-↓`/`Alt-↑`) using the gutter-mark map that
   already exists (`buildGutterMarks`) — the data is already computed, this is UI/keymap
   work, not new diff-parsing work. This is the single highest-leverage change for the
   stated "daily driver for reading and reviewing code" goal, since right now review is
   scroll-and-hope.
4. **Add jump-to-line** (`Ctrl-G` or `:123`) — `@codemirror/search`'s `gotoLine` is already
   a dependency-away, not a new library.
5. **Connect the Diff tab and Files tab.** Clicking a hunk in `DiffViewer` should deep-link
   into the Files tab at that file+line (mirroring the existing `VcsPanel` →
   `onNavigateToFile` → `filesSelectedPath` cross-link in `SessionDetailView.tsx:849-852`,
   which already proves the plumbing pattern works) instead of requiring a manual re-find.
6. **Add "mark file reviewed" state**, even client-side/session-scoped to start (a
   checkbox per file in the tree, persisted the same way `filesSelectedPath` is persisted
   via URL hash) — the biggest structural gap versus GitHub's PR reviewer for actual
   review workflows.
7. **Add basic word motions** (`w`/`b`/`e`) and `%` bracket-match — cheap, CodeMirror
   likely already exposes equivalent commands (`cursorGroupLeft`/`cursorGroupRight` are
   close analogs used elsewhere in `@codemirror/commands`), and these are the vim motions
   used most after `hjkl` for code reading specifically.
8. **Progressive render for large files** — show the first viewport's worth of content
   before the full 1&nbsp;MB payload/parse completes, or at minimum a loading-progress
   affordance distinct from the current binary loading-spinner-then-full-render.
9. **Verify/hardcode ARIA role + focus restoration** on tab-switch back into Files (store
   and restore last-focused tree node id / editor cursor position) — accessibility gap
   that's currently unverified rather than confirmed-fine.
10. **Cross-file content search** (grep-and-jump across the whole tree, not just filename
    match) — largest single addition on this list, ordered last because it's materially
    more backend + UI work than everything above it.
