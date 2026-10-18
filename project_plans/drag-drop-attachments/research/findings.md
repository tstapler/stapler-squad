# Research Findings: Terminal Drag-and-Drop Upload

## 1. Existing Upload Infrastructure (Verified by Code Inspection)

### 1.1 Backend — `SessionImageUploadHandler`

File: `server/services/session_image_upload_handler.go`

- Route: `POST /api/v1/upload-image` (registered in `server/server.go` ~line 439)
- Request: `multipart/form-data` with fields `session_id` (string) and `file` (binary)
- Accepts: any MIME type (not restricted to images)
- Size limit: 10 MB per file (`sessionUploadMaxBytes = 10 * 1024 * 1024`)
- Output directory: `<session_path>/uploads/` (created with `0o750`)
- File name: `<timestamp_ms>-*-<sanitized_original_name>` via `os.CreateTemp`
- File permissions: `0o600`
- Response: `{"path":"<absolute_path>","filename":"<basename>"}`
- Security: `sanitizeFilename` strips path components, limits to 100 chars
- **No backend changes are needed for this feature.**

### 1.2 Frontend — `handleFilesUpload` in `TerminalOutput.tsx`

Lines ~1660–1731:

```
for each file in FileList:
  const formData = new FormData()
  formData.append("session_id", sessionId)
  formData.append("file", file)
  const resp = await fetch(`${baseUrl}/v1/upload-image`, { method: "POST", body: formData })
  if (resp.ok): handleTerminalData(data.path + " ")
  else: setUploadError(msg)
```

This loop is the exact path drag-dropped files should follow. It should be extracted
to a shared helper (`uploadFileList`) callable from both the toolbar pickers and the
drop handler.

### 1.3 Upload State Already Present

`TerminalOutput` already manages:
- `uploadingCount: number` — count in-flight files
- `uploadError: string | null` — shown briefly as a toast
- `uploadSuccess: string | null` — shown briefly as a toast
- `uploadErrorTimerRef` / `uploadSuccessTimerRef` — auto-dismiss timers

The drag-and-drop handler can share all of these — no new state needed.

## 2. Drag-and-Drop API Reference

### 2.1 HTML5 `DragEvent` sequence

```
dragenter  → fires when cursor enters the drop target
dragover   → fires continuously while cursor is over it (must preventDefault() to allow drop)
dragleave  → fires when cursor leaves (fires for EACH child element — see §2.2)
drop       → fires on release; event.dataTransfer.files contains the FileList
```

All four need `preventDefault()` + `stopPropagation()` in their handlers to prevent
the browser from opening the dropped file in a new tab (the browser default for file
drops on web pages).

### 2.2 Spurious `dragleave` on child elements (the primary gotcha)

The terminal container (`<div className={styles.terminal}>`) contains xterm.js's own
DOM tree (canvas elements, rows div, etc.). When the drag cursor moves over any
child element, `dragleave` fires on the parent and then `dragenter` fires again. This
causes the overlay to flicker without a guard.

**Recommended solution:** reference-count the enter/leave pairs.

```tsx
const dragCounterRef = useRef(0);

const handleDragEnter = (e: React.DragEvent) => {
  e.preventDefault();
  e.stopPropagation();
  dragCounterRef.current++;
  if (dragCounterRef.current === 1) setIsDragOver(true);
};

const handleDragLeave = (e: React.DragEvent) => {
  e.preventDefault();
  e.stopPropagation();
  dragCounterRef.current--;
  if (dragCounterRef.current === 0) setIsDragOver(false);
};

const handleDrop = (e: React.DragEvent) => {
  e.preventDefault();
  e.stopPropagation();
  dragCounterRef.current = 0;
  setIsDragOver(false);
  uploadFileList(Array.from(e.dataTransfer.files));
};
```

Reset `dragCounterRef` to 0 in `handleDrop` because `dragleave` never fires after
`drop`.

Alternative (checking `relatedTarget`): works but is less reliable in xterm's
shadow-DOM-like canvas tree because `relatedTarget` can be null for canvas children.
The counter approach is the standard recommendation and is used by react-dropzone,
react-dnd, and similar libraries.

### 2.3 Checking for file content in `dragenter` / `dragover`

`e.dataTransfer.types` includes `'Files'` when the drag payload contains files.
Filtering on this prevents activating the overlay for pane-reorder drags (which use
a custom MIME type, not `'Files'`).

```tsx
const hasFiles = (e: React.DragEvent) => e.dataTransfer.types.includes('Files');
```

## 3. Component Architecture

### 3.1 Where to attach handlers

The `<div className={styles.terminal} ref={terminalContainerRef}>` at line ~2246
is the correct element. It wraps the entire xterm canvas area (plus loading overlays),
and it is already the layout boundary for the terminal viewport. Attaching handlers
here, rather than to a child, gives the broadest possible drop target without
interfering with the toolbar above.

### 3.2 Overlay implementation

A new `<div>` rendered inside `styles.terminal` with `position: absolute; inset: 0`
when `isDragOver` is true. The vanilla-extract `TerminalOutput.css.ts` file already
defines the overlay pattern (e.g. `loadingOverlay`, `reconnectingBanner`) — the new
`dropZoneOverlay` follows the same pattern.

```tsx
{isDragOver && !readOnly && hasFinePointer && (
  <div className={styles.dropZoneOverlay} aria-hidden="true">
    <span>Drop to attach</span>
  </div>
)}
```

### 3.3 Gate conditions (from requirements)

```tsx
const canAcceptDrop = !readOnly && hasFinePointer;
```

When `!canAcceptDrop`, the event handlers are either omitted from the JSX or short-circuit
immediately (short-circuit is simpler since the flag can change at runtime).

## 4. Interaction with Pane Drag-and-Drop

`PaneSplitRenderer.tsx` (lines 339–350) uses `e.dataTransfer.getData('text/plain')` to
detect pane-reorder drags — it sets a `text/plain` payload (pane ID), not `'Files'`.
The `hasFiles` check in §2.3 ensures the terminal drop zone only activates for real OS
file drops, never for pane drags. These two drag systems are disjoint.

## 5. Similar Patterns in the Codebase

The `BacklogItemForm.tsx` and `BacklogComposeWidget.tsx` both reference `dataTransfer`
for attachment uploads (those are the backlog form file uploads). Reviewing them:

- `BacklogItemForm.tsx` uses `dataTransfer` for paste/drop on the form's attachment zone
- This confirms the pattern is already in use elsewhere; the terminal implementation can
  follow the same shape

## 6. CSS Architecture Reference

`docs/reference/css-architecture.md` mandates vanilla-extract for all styles. The
`TerminalOutput.css.ts` file already defines overlay classes (`loadingOverlay`,
`reconnectingBanner`, `hardFailedBanner`, `unavailableOverlay`, `resizingOverlay`).
The new `dropZoneOverlay` follows the same pattern:
- `position: absolute; inset: 0; zIndex: <above terminal, below toasts>`
- semi-transparent background (matching the existing overlay scheme)
- centered label text

## 7. Testing Strategy

### Unit / Jest
- Extract `uploadFileList` to a testable function (not a `useCallback` that captures
  refs) — pass `sessionId`, `baseUrl`, `handleTerminalData`, and the status-setter
  callbacks as parameters. This makes it mockable without rendering `TerminalOutput`.
- Test `dragCounterRef` logic with simulated `dragenter`/`dragLeave`/`drop` sequences.

### E2E (Playwright)
- Use `page.dispatchEvent` + `createDataTransfer` to simulate a file drop
- Playwright's `page.dragAndDrop` works for element-to-element; for OS-level file drop,
  use `page.evaluate` + `DataTransfer` constructor (Chromium supports this in tests).
- `data-testid="terminal-drop-zone"` on the terminal wrapper enables locator targeting.
- Assert `data-testid="drop-zone-overlay"` appears on `dragenter` and disappears on `drop`.

## 8. Scope Confirming No Backend Work Needed

The backend already:
- Accepts any MIME type (not image-only)
- Saves to `<session_path>/uploads/` (accessible to the agent process by absolute path)
- Returns `{path, filename}` JSON
- Enforces 10 MB limit with proper HTTP 413 response
- Uses `sanitizeFilename` for path traversal defense

No new endpoint, no new route, no proto change.

## 9. Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| `dragleave` flicker on xterm child elements | High | Medium | `dragCounterRef` pattern (§2.2) |
| Pane-reorder drag triggers file overlay | Low | Low | `hasFiles` type check (§2.3) |
| Mobile regression (touch drag triggers overlay) | Low | Medium | `hasFinePointer` guard (FR-5) |
| `readOnly` session shows overlay | Low | Low | `canAcceptDrop` guard (§3.3) |
| jscpd duplication gate: extracting uploadFileList | Low | Low | Extract as shared utility, not copy |
| Drop while session disconnected | Medium | Low | Existing `isConnected` copy-to-clipboard fallback in `handleFilesUpload` already handles this |
