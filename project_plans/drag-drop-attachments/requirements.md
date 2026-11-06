# Requirements: Terminal Drag-and-Drop File/Image Upload

## Problem Statement

Every file-attachment path in Stapler Squad today requires an explicit UI gesture —
clipboard paste or tapping a toolbar button to open a file picker. There is no way to
drag a file from the OS file manager (or another browser tab) directly onto a live
agent session's terminal and have it attached. This friction is disproportionate because
every other modern tool (editors, chat apps, email clients) accepts file drops, and
the full upload infrastructure (`POST /api/v1/upload-image` + `SessionImageUploadHandler`)
already handles any file type once the browser delivers the bytes.

## Stakeholder

Tyler Stapler (sole user / developer)

## Current State

| Component | Location | Role |
|---|---|---|
| Session upload handler | `server/services/session_image_upload_handler.go` | Accepts multipart `POST /api/v1/upload-image`; any file ≤10 MB; saves to `<session_path>/uploads/`; returns `{path, filename}` |
| Terminal file-picker upload | `web-app/src/components/sessions/TerminalOutput.tsx` `handleFilesUpload` (line ~1660) | Iterates a `FileList`, uploads each via the above endpoint, calls `handleTerminalData(path + " ")` per success |
| Hidden file inputs | Same file, lines ~280–282 | `cameraInputRef`, `galleryInputRef`, `filesInputRef` — trigger `handleFilesUpload` |
| Paste handler | Same file, `handlePaste` (line ~1599) | Reads clipboard; uploads image blobs via `${baseUrl}/upload/file` (the base64 endpoint) |
| Pane drag-and-drop | `web-app/src/components/pane/PaneSplitRenderer.tsx` lines 339–350 | `dataTransfer` used only for pane reorder — not file drops |
| `readOnly` prop | `TerminalOutput.tsx` | Suppresses all input, keyboard toolbar, paste, and upload |
| Mobile detection | `useViewport()` hook in `TerminalOutput.tsx` | `isMobile`, `hasFinePointer` — gate desktop-only features |

No `onDrop`, `onDragEnter`, `onDragOver`, or `onDragLeave` handlers exist anywhere in
`web-app/src/components/sessions/`. The only `DragEvent` / `dataTransfer` usage is
the pane-reorder code, which is unrelated to file drops.

## Functional Requirements

### FR-1: Drop target on the terminal area
The terminal container element (rendered at `<div className={styles.terminal}>`,
line ~2246 in `TerminalOutput.tsx`) must accept OS-level file drops:
- `onDragEnter` / `onDragOver` — `preventDefault()` + `stopPropagation()` to allow drop;
  set drop-active visual state
- `onDragLeave` — clear drop-active visual state (guard against child-element leave
  events firing spuriously — use a counter or `relatedTarget` check)
- `onDrop` — `preventDefault()`; extract `event.dataTransfer.files`; call the existing
  `handleFilesUpload` logic (extract to a shared `uploadFileList(files: FileList | File[])` helper)

### FR-2: Reuse existing upload logic
The file upload path (`FormData` → `POST /api/v1/upload-image` → `handleTerminalData(path)`)
must be the same for both toolbar-picker uploads and drag-and-drop uploads. Extract the
core loop from `handleFilesUpload` into a standalone `uploadFileList` function or hook
so both call sites share it without duplication.

### FR-3: Visual drop-zone affordance
While files are being dragged over the terminal, a visible overlay must appear:
- Semi-transparent backdrop covering the terminal area
- Text label: "Drop file(s) to attach" (or equivalent)
- Styled consistently with the existing upload-success/error toast design
- Dismisses immediately when the drag exits or the drop fires

### FR-4: Non-image files
Dropped non-image files (text, PDF, archives, code) must be saved and their path
inserted into the terminal input exactly as image drops are — the backend already
supports any MIME type, so no backend changes are needed.

### FR-5: Desktop-only
Drag-and-drop is a desktop-only enhancement (requires a fine pointer / mouse).
When `isMobile && !hasFinePointer` (no `compactToolbar`-equivalent guard needed —
`hasFinePointer` already captures the distinction), the drag handlers must be absent
(or no-ops) and the overlay must never render. The existing paste flow and toolbar
file-picker on mobile are unaffected.

### FR-6: readOnly suppression
When `readOnly === true`, the drop handlers must be no-ops (same policy as the paste
and upload buttons). A read-only session must silently ignore drops.

### FR-7: Error and success feedback
Dropped files use the same `uploadError` / `uploadSuccess` / `uploadingCount` state
already present in `TerminalOutput`. No separate state variables are needed.

## Non-Functional Requirements

### NFR-1: No backend changes required
The existing `POST /api/v1/upload-image` endpoint accepts `multipart/form-data` with
`session_id` + `file` fields and handles any MIME type. No new route, handler, or
schema change is needed.

### NFR-2: No regressions to pane drag-and-drop
The pane-reorder `dataTransfer` logic in `PaneSplitRenderer.tsx` must be unaffected.

### NFR-3: Guard against recursive drag-leave
Child elements inside the terminal container fire `dragLeave` when the cursor moves
over them. The implementation must debounce or count enter/leave pairs to prevent the
overlay from flickering as the cursor moves over xterm's canvas children.

### NFR-4: CSS via vanilla-extract
Drop-zone overlay styles must follow the project's vanilla-extract (`*.css.ts`) pattern,
not inline styles. See `docs/reference/css-architecture.md`.

### NFR-5: No `waitForTimeout` in E2E tests
Any new E2E test must follow `e2e-test-conventions`: `data-testid` locators,
`expect(locator).toBeVisible()` waits, no `waitForTimeout`.

## Out of Scope

- Drag-and-drop on the backlog item creation form (separate feature path)
- Drag-and-drop on the Omnibar / creation panel (different component)
- Progress bars or chunked uploads
- File manager / persistent file library
- Drag-and-drop from the file tree pane *within* the app (only OS-level drops are in scope)
- macOS Finder "Spring loading" / hover-to-navigate

## Acceptance Criteria

1. Dragging a file (any type) from the OS file manager over the live session terminal
   shows a visible drop-zone overlay while the cursor is over the terminal area.
2. Releasing the drag uploads the file via `POST /api/v1/upload-image` and inserts the
   returned path into the terminal input.
3. Dropping an image file routes through the same backend endpoint and path as the
   toolbar image picker — no separate image-specific code path.
4. Dropping multiple files at once uploads them sequentially (same behavior as
   multi-select via the file picker).
5. The drop-zone overlay disappears immediately when the cursor leaves the terminal
   area or the drop fires, with no flickering caused by hover over child elements.
6. On a mobile/touch-only device (`!hasFinePointer`), no drop-zone overlay appears
   and no drag handlers are active.
7. When `readOnly === true`, dropped files are silently ignored.
8. `make ci` passes after the change (lint, tests, no duplication gate regression).
9. Feature is documented in `docs/features.md`.

## Technical Context

| File | Relevance |
|---|---|
| `web-app/src/components/sessions/TerminalOutput.tsx` | Primary change site — add drag handlers + overlay |
| `web-app/src/components/sessions/TerminalOutput.css.ts` | Add `dropOverlay` + `dropActive` styles |
| `server/services/session_image_upload_handler.go` | No changes needed |
| `server/server.go` line ~439 | Route registration — no changes needed |
| `web-app/src/components/pane/PaneSplitRenderer.tsx` | Reference only — pane DnD must not regress |
| `docs/features.md` | Add drag-and-drop entry |
