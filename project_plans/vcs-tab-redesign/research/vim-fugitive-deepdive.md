# vim-fugitive deep dive — what translates to a read-mostly VCS status panel

Research for `vcs-tab-redesign`, SDD Phase 2. Sources: [tpope/vim-fugitive](https://github.com/tpope/vim-fugitive), [fugitive.txt](https://raw.githubusercontent.com/tpope/vim-fugitive/master/doc/fugitive.txt), [Tightly Integrating Git into Vim (jakobgm.com)](https://jakobgm.com/posts/vim/git-integration/), [Practical introduction to fugitive (jeancharles.quillet.org)](https://jeancharles.quillet.org/posts/2022-03-02-Practical-introduction-to-fugitive.html), [Vimcasts: exploring repo history with Fugitive](http://vimcasts.org/episodes/fugitive-vim-exploring-the-history-of-a-git-repository/).

Current component for reference: `web-app/src/components/shared/VcsWidget.tsx` — already splits into `VcsWidgetHeader`, `VcsWidgetGithubRow` (PR/mergeability), `VcsWidgetFileList`, `VcsWidgetCommitList` as separate sub-widgets with an `aggregateStatLine` in compact mode.

---

## 1. The `:Git status` buffer as an interactive object

Fugitive's core move is translating Git's object model into Vim's buffer model 1:1 — the index, the work tree, individual blobs, and commits are each represented as an addressable buffer, and the status buffer is a live, cursor-addressable view over the repo, not a report. `:Git` (no args) opens this summary buffer showing untracked/unstaged/staged files plus unpushed/unpulled commit summaries in one screen.

Every action targets **whatever the cursor is currently on** — a file line, a hunk header, or a commit line — via a single keystroke, with no separate "select, then find the button" step:

| Key | Action |
|---|---|
| `-` | Stage or unstage the file/hunk under the cursor (toggle) |
| `s` / `u` | Stage / unstage explicitly |
| `U` | Unstage everything |
| `X` | Discard the change under the cursor (`checkout`/`clean`) |
| `=` | Toggle an inline diff of the file under the cursor, expanded in place |
| `cc` | Open a commit-message buffer and commit |
| `ca` / `ce` | Amend (with/without editing message) |
| `dd` / `dv` / `dh` | Open a full diff split (unified / vertical / horizontal) of the file under cursor |
| `[c` / `]c` | Jump to previous/next hunk, auto-expanding inline diffs as you go |
| `gs` / `gu` / `gp` / `gP` | Jump cursor to the Staged / Unstaged / Unpushed / Unpulled section |

The `=` (inline diff) map is the most telling: the diff isn't a separate view you navigate to — it **unfolds directly underneath the file line you're on**, in the same buffer, then folds back up. Nothing else on screen moves or gets replaced.

**Recommendation:** the interaction is out of scope (read-mostly, no staging), but the *layout primitive* is not: an expand-in-place row, not a modal/drawer/separate route. Each file row in `VcsWidgetFileList` should support an inline expand (chevron or tap) that unfolds the diff hunk directly below that row in the same scroll — not a navigation to a separate diff view. `onViewDiff`/`onNavigateToFile` can stay as an "open full diff" escape hatch, but the default first click should expand in place, mirroring `=`. The `gs`/`gu`/`gp`/`gP` section-jump maps confirm status/diff/commits/PR-checks are conceptually **one buffer with named regions**, not four separate widgets — see §5 for how this maps to layout.

## 2. `:Gdiffsplit`/`:Gvdiffsplit` — inline 3-way merge resolution

During a merge conflict, `:Gdiffsplit!` opens a 3-way diff: the common ancestor, "ours," and "theirs" as separate windows, with the work-tree version (what you're editing) always placed to the right or bottom depending on available width — so the file you're resolving is always in a fixed, predictable position relative to the two source versions. Resolution then uses vanilla Vim diff maps repurposed by Fugitive: `d2o`/`d3o` pull a hunk from the "ours"/"theirs" ancestor into the work tree; `dp`/`do` (diff put/obtain) push or pull a hunk between the currently focused diff window and its neighbor. Nothing here requires leaving the diff view or opening a separate "resolve conflict" mode — the conflict *is* the diff, and resolving it is the same "operate at cursor position" model as the status buffer, just applied to three synchronized windows instead of one buffer.

**Recommendation:** conflict resolution is explicitly out of scope. What translates is the **fixed spatial layout convention** — always put the same kind of thing in the same place. For our panel: CI check status, PR mergeability, and review state should always render in the same relative position/order across sessions so a user pattern-matches location instead of reading labels every time (e.g., mergeability pill always top-left of the header, checks always directly below it, never reordered based on which checks are present/absent).

## 3. `:Git blame` — chunked interactive blame

`:Git blame` opens a scroll-bound vertical split: blame annotations on the left, the file on the right, cursor-synced so scrolling one scrolls the other. It's "chunked" — contiguous lines from the same commit are grouped into one visual chunk rather than repeating the commit hash on every line, and the interaction is keyed off which chunk the cursor is on:

| Key | Action |
|---|---|
| `<CR>` | Close blame, jump to the commit that introduced the line under the cursor |
| `o` / `O` / `p` | Open that commit in a horizontal split / new tab / preview window |
| `-` | Re-blame as of that commit (step further back in history) |
| `A`/`C`/`D` | Resize columns to fit author/commit/date |

The key property: blame isn't a static annotation dump, it's a **navigable index into history keyed by cursor position** — you point at a line, you get that line's commit, and you can descend further (`-` reblames at the parent, letting you walk a line's history backward one keystroke at a time).

**Recommendation:** doesn't translate directly (we're not building a file/line viewer), but the **chunking-by-commit** principle does: our commit list should visually group commits by author/recency the way blame groups contiguous lines by commit, rather than repeating identical metadata (avatar, relative time) on every single row when several commits share it. And "drill down without leaving the view" translates to: tapping a commit in `VcsWidgetCommitList` expands its file list / CI status inline, rather than routing to a separate commit-detail page.

## 4. `:Git log` integration — one buffer type, reused everywhere

`:Gclog` (and `:Git log`) loads log output into a **quickfix list**, the same navigable list structure Vim already uses for compiler errors and search results — Fugitive doesn't invent a bespoke "log viewer" UI, it reuses the editor's existing generic "list of locations you can jump between" primitive. Hitting Enter on a log entry opens that commit as a buffer showing message, author, and diff — the *same* commit-buffer type used everywhere else Fugitive shows a commit (status buffer's unpushed section, blame's `<CR>`, etc.). There's exactly one representation of "a commit" in the whole plugin, reused by reference from every other view.

**Recommendation:** the informational payoff here is **one canonical shape per entity, reused everywhere**, not a special "log page" schema different from the "status page" schema. Concretely: define one `CommitSummary` shape (sha, message, author, relative time, CI/check rollup) and render it identically whether it appears in the compact aggregate view, the full commit list, or a future PR-conversation timeline — don't let the commit list and a hypothetical "recent activity" feed drift into two different visual treatments of the same data.

## 5. THE KEY QUESTION — why does this read as "incredibly useful" to a power user?

Four properties, each with a concrete translation for a **read-mostly, mobile+desktop web status panel** (no keystroke-driven editing in scope):

### a. Single source-of-truth buffer — no context-switching between panels
Fugitive's status buffer *is* the diff view *is* the commit browser *is* the entry point to blame — one buffer, one scroll position, one mental model, sections addressable in place (`gs`/`gu`/`gp`/`gP`). The user never asks "which window has the thing I need" because there's only one window.

**Recommendation:** collapse the VCS tab into **one scrollable "everything" view** with named, anchor-jumpable sections (branch/commits, unstaged/staged-equivalent file changes, CI checks, PR mergeability, reviewer feedback) instead of a pill bar that routes to separate sub-panels. If a compact "jump to CI" / "jump to reviews" affordance is wanted, implement it as an in-page anchor scroll (mirroring `gp`/`gP`), not a route or tab change. The current `VcsWidget` split into `VcsWidgetHeader`/`VcsWidgetGithubRow`/`VcsWidgetFileList`/`VcsWidgetCommitList` is already close to this — the redesign's job is making sure they read as **one flowing status document**, not four independently-scrolling or independently-collapsed widgets a user has to hunt across to answer "is this PR ready to merge."

### b. Direct manipulation via cursor position + keystroke — no "select, then find the control elsewhere" step
Every fugitive action operates on whatever's under the cursor; there's no separate toolbar you look away to. The control and the object of the control are colocated.

**Recommendation:** even without editing, this argues for **inline affordances on the row itself** rather than a global action bar: the expand-diff chevron lives on the file row (see §1), the "view full diff"/"open in GitHub" link lives on the commit row, the "why is this blocking" explanation lives directly under the failing check it explains — not routed to a shared details panel elsewhere on the page. Nothing the user needs to interpret one row should live in a different DOM region than that row.

### c. Tight feedback loop — the buffer updates in place, immediately
Staging a hunk removes it from "Unstaged" and adds it to "Staged" in the same buffer instantly; there's no "save, then reload, then re-navigate to see the result."

**Recommendation:** applies to live refresh, not user actions: when `onRefresh`/live polling updates VCS state (new commit landed, check finished, review posted), the affected section should update **in place** — same scroll position, same expand/collapse state — not a full-panel re-render or a jarring re-fetch flash. This is the strongest argument for keeping expand/collapse and scroll-anchor state in stable, keyed local state rather than re-deriving it from a fresh data object on every poll.

### d. Information density — a lot shown in a small space, no decorative chrome
The status buffer packs branch, ahead/behind counts, per-file change type, and commit subjects into plain text with almost no visual decoration — density comes from *terse, consistent formatting*, not from cramming (e.g. a single `M path/to/file.go` line encodes status letter + path with zero wasted pixels).

**Recommendation:** favor compact, label-free encodings over verbose cards: a single-letter/icon status glyph + path (already how `VcsWidgetFileList` likely renders, worth confirming), commit rows as one line (short sha, subject, relative time, author avatar) rather than a multi-line card, CI checks as a compact glyph row rather than one card per check. Reserve visual weight (color, size, whitespace) for the two or three signals that actually gate "is this ready" — mergeability and failing checks — the way Fugitive reserves color for diff +/- and section headers, not for every line.

## 6. Explicit translate / don't-translate summary

**Does NOT translate (out of scope — editing surface):**
- Keystroke-driven staging/unstaging/committing (`s`, `u`, `cc`, `ca`, `X`) — no staging concept exists in a read-mostly panel.
- 3-way merge conflict resolution (`d2o`/`d3o`/`dp`/`do`) — no conflict-editing UI in scope.
- Blame's line-level history walking (`-` to reblame, `<CR>` to jump to introducing commit) — we're not building a file/blame viewer.
- Modal-buffer keyboard navigation itself (`gs`/`gu`/`[c`/`]c` as literal keybindings) — mobile has no keyboard; these inform layout/anchors, not actual key handling. A desktop keyboard-shortcut layer *could* borrow the letters as a stretch goal but is not core to this redesign.

**DOES translate (the actual value, independent of the editing):**
- One buffer, no context-switching: a single flowing scroll view with in-page anchors instead of a pill-plus-separate-panel split.
- Cursor/row-colocated controls: inline expand and inline "why," not a shared details panel elsewhere on the page.
- In-place live updates that preserve scroll and expand state, not full remounts on refresh.
- Terse, consistent, chrome-free density: single-line rows with glyph-encoded status, color reserved for the signals that actually gate mergeability.
- One canonical shape per entity (commit, check, file) reused identically everywhere it appears, rather than per-section bespoke formatting.
