# Requirements: Better File Tree UX

**Date**: 2026-05-15  
**Project**: stapler-squad — file browser (FilesTab + FileTree + FileContentViewer)  
**User goal**: Use the file browser like a text editor — reliable, fast navigation across many files without friction.

---

## Context

The current file browser is a split-pane layout:
- **Left pane** (30%, min 200 px, max 480 px, hardcoded): `FileTree` — lazy-loaded directory tree with search, git status badges, vim-style keyboard nav
- **Right pane** (flex 1): `FileContentViewer` — Shiki syntax-highlighted content with breadcrumb

Pain points reported by the user:
1. No way to resize the left tree panel
2. No way to collapse to full-screen content or re-expand (especially on mobile)
3. File names get cut off — hard to know what you're clicking on
4. Scroll position in the tree is lost when switching files
5. No breadcrumb / path indicator in the tree (only in the content pane header)
6. No recently-opened list for quick re-access

---

## Requirements

### R1 — Resizable tree panel
- User can drag a handle between tree and content panes to set custom width
- Minimum tree width: 160 px. Maximum: 50% of viewport width
- Width persists across page reloads via localStorage (`filestab.treeWidth`)
- A collapse button (← arrow icon) fully collapses the tree to 0 px (icon-strip or hidden)
- An expand button (→ arrow icon) restores the last width or defaults to 260 px
- Collapsed state also persists in localStorage

### R2 — Mobile-friendly layout
- On viewports < 768 px, the layout switches to single-pane:
  - Default view: file tree (full width)
  - After selecting a file: content pane slides in over the tree (full width)
  - A back button (← "Files") in the content pane header returns to the tree
- The tree is never partially visible on mobile — it's either fully shown or fully hidden

### R3 — File name truncation fix
- File names in the tree must not silently clip mid-name
- Strategy: truncate in the middle of the name (`foo…bar.tsx`) so both the beginning and extension are visible
- Full path shown in a tooltip (`title` attribute) on hover
- If the tree pane is wider, names expand naturally (no artificial max)

### R4 — Preserve tree scroll position
- When the user opens a file and later returns focus to the tree, the tree's scroll position is unchanged
- The selected file row is scrolled into view only when it is not already visible (no jump if already visible)

### R5 — Tree highlights & auto-reveals current file
- The currently open file is highlighted in the tree with the existing `selected` style
- If the selected file is in a collapsed directory, that directory auto-expands and the tree scrolls the row into view
- This replaces the current behavior where the tree does not track the active file after initial selection

### R6 — Recently opened files panel
- A "Recent" section appears at the top of the file tree pane (above the directory tree)
- It shows up to 8 most-recently opened files in this session (in-memory, not persisted)
- Each entry shows the file icon, basename, and parent directory name
- Clicking an entry opens the file and scrolls the tree to it
- The section is hidden when no files have been opened yet

### R7 — Keyboard navigation enhancements
- `/` or `Ctrl+F` focuses the search box (already wired — verify it works from content pane focus)
- `Escape` in the search box clears the search and returns focus to the tree
- `Ctrl+P` (or `Cmd+P`) opens a quick-open palette that searches file names across the entire tree (reuses `SearchFiles` RPC)
- Arrow keys Up/Down in the quick-open palette navigate results; Enter opens; Escape closes

---

## Out of scope
- Editing files (read-only viewer)
- Multi-pane / tabs for files
- File diff view
- Drag-and-drop file reordering

---

## Acceptance criteria

| ID | Criterion |
|----|-----------|
| AC-1 | Drag the resize handle to 400 px; reload page; tree is still 400 px wide |
| AC-2 | Click collapse; tree disappears; click expand (or the ← icon); tree returns to previous width |
| AC-3 | On a 375 px viewport, selecting a file shows the content full-width with a back button |
| AC-4 | File name `very-long-component-name.tsx` is displayed as `very-long-comp…name.tsx` and full path is in tooltip |
| AC-5 | Scroll tree to bottom; open a file in the middle of the tree; tree scroll position stays at bottom |
| AC-6 | Navigate to a file via the address bar / VCS panel; tree auto-expands ancestor dirs and scrolls file into view |
| AC-7 | Open 3 files; the Recent section shows all 3, most recent first |
| AC-8 | `Ctrl+P` opens quick-open palette; typing "store" filters to files with "store" in their name |
