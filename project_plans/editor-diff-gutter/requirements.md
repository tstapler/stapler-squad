# Requirements: Editor Diff Gutter Indicators & Background Highlighting

## Problem Statement

The Files tab editor shows file contents but lacks visual indicators for git diff changes. Users viewing a modified file cannot immediately see which lines were added, modified, or deleted. The screenshot shows a plain editor with no gutter bars or line backgrounds for changed lines.

The goal is to show **VS Code-style diff decorations** in the CodeMirror-based editor:
- A colored gutter bar (4px strip) on the left margin for each changed line
- A colored semi-transparent background highlight on each changed line

## Scope

This work is **frontend-only**. The backend already provides diff content via `useSessionVcsContext()` and it is already wired through `FilesTab → FileContentViewer → ReadSurface → CodeEditor → CmEngine`. The gap is in the rendering layer:

1. **Line background highlighting is completely absent** — `CmEngine.createBuffer()` sets up the `gutter()` extension for markers but never registers a `Decoration.line()`-based ViewPlugin or StateField for row background colors.
2. **WindowedFileView does not receive gutterMarks** — the windowed code path in `FileContentViewer` passes no `gutterMarks` to `WindowedFileView`, so large-file views show no gutter indicators at all.

## Out of Scope

- Changing the backend VCS diff API
- The Diff tab (which already shows traditional side-by-side/unified diff via `DiffRenderer`)
- The Accessible view (screen reader text) — gutterMarks do not affect it

## Acceptance Criteria

1. When a file has uncommitted changes, changed lines show a colored gutter bar (add = green, modify = amber/yellow, delete indicator = red at the insertion point) in the CodeMirror gutter.
2. When a file has uncommitted changes, each changed line has a semi-transparent background highlight that matches the gutter bar color (green for add, amber for modify, red/deleted indicator line).
3. Background highlight colors are low-opacity so that syntax highlighting remains readable on top of the background tint.
4. Gutter indicators and background highlights work in both regular file view and windowed (large-file) view.
5. When no diff is active (no uncommitted changes for the current file), no gutter bars or backgrounds are shown.
6. All six theme variants (default dark, default light, retro, synthwave, solarized, hacker) render the diff colors without clashing with their syntax highlighting palette.
7. Existing gutter-marks unit tests (`parseDiff.test.ts`, `buildGutterMarks`) continue to pass.
8. A new unit test verifies that `CmEngine.createBuffer()` applies line-background decorations for "add", "modify", and "delete" gutter marks.

## Key Files

| File | Role |
|---|---|
| `web-app/src/components/sessions/files/engine/CmEngine.ts` | Add `Decoration.line()` ViewPlugin for background highlights |
| `web-app/src/components/sessions/CodeEditor.css.ts` | Add CSS classes for line background colors (semi-transparent) |
| `web-app/src/styles/theme.css.ts` | Add `gitAddedBg`, `gitModifiedBg`, `gitDeletedBg` semi-transparent tokens per theme |
| `web-app/src/components/sessions/FileContentViewer.tsx` | Pass `gutterMarks` to `WindowedFileView` |
| `web-app/src/lib/utils/parseDiff.ts` | No change needed (already complete) |
| `web-app/src/components/sessions/files/engine/EditorEngine.ts` | No change expected |
