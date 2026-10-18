# Implementation Plan: Terminal Drag-and-Drop Upload

## Overview

Add OS-level drag-and-drop file upload to the live terminal area in `TerminalOutput.tsx`.
Files dropped on the terminal are uploaded via the existing `POST /api/v1/upload-image`
endpoint and their paths are inserted into the terminal input, exactly matching the
existing toolbar file-picker behavior. No backend changes required.

## Scope

**In:** `TerminalOutput.tsx`, `TerminalOutput.css.ts`, one new Jest test file,
one new E2E spec, `docs/features.md`.

**Out:** Backend, proto, backlog form, Omnibar/creation panel, pane reorder code.

---

## Task Breakdown

### Task 1 — Extract `uploadFileList` helper (refactor, ~2h)

**File:** `web-app/src/components/sessions/TerminalOutput.tsx`

Extract the upload loop from `handleFilesUpload` (lines ~1678–1730) into a
`useCallback`-wrapped helper `uploadFileList(files: File[])` that:
- Accepts a `File[]` (not a `FileList`) so it can be called from both the
  `onChange` handler and the new drop handler
- Captures the same state setters (`setUploadingCount`, `setUploadError`,
  `setUploadSuccess`, `uploadErrorTimerRef`, `uploadSuccessTimerRef`)
- Calls `handleTerminalData(path + " ")` per success
- Has the same isConnected-fallback (copy-to-clipboard) behavior

Rename `handleFilesUpload` → `handleFilesPickerChange` and make it call
`uploadFileList(Array.from(fileList))`.

This is a pure refactor with no behavior change; it makes Task 3 trivial.

### Task 2 — Add drag state + `isDragOver` (frontend state, ~1h)

**File:** `web-app/src/components/sessions/TerminalOutput.tsx`

Add:
```tsx
const [isDragOver, setIsDragOver] = useState(false);
const dragCounterRef = useRef(0);
```

Add the guard helper:
```tsx
const canAcceptDrop = !readOnly && hasFinePointer;
```

Note: `hasFinePointer` is already available from `useViewport()` in `TerminalOutput`.

### Task 3 — Add drag event handlers (frontend, ~1h)

**File:** `web-app/src/components/sessions/TerminalOutput.tsx`

Add four `useCallback` handlers:

```tsx
const handleDragEnter = useCallback((e: React.DragEvent<HTMLDivElement>) => {
  if (!canAcceptDrop) return;
  if (!e.dataTransfer.types.includes('Files')) return;
  e.preventDefault();
  e.stopPropagation();
  dragCounterRef.current++;
  if (dragCounterRef.current === 1) setIsDragOver(true);
}, [canAcceptDrop]);

const handleDragOver = useCallback((e: React.DragEvent<HTMLDivElement>) => {
  if (!canAcceptDrop) return;
  if (!e.dataTransfer.types.includes('Files')) return;
  e.preventDefault();
  e.stopPropagation();
  e.dataTransfer.dropEffect = 'copy';
}, [canAcceptDrop]);

const handleDragLeave = useCallback((e: React.DragEvent<HTMLDivElement>) => {
  if (!canAcceptDrop) return;
  e.preventDefault();
  e.stopPropagation();
  dragCounterRef.current--;
  if (dragCounterRef.current === 0) setIsDragOver(false);
}, [canAcceptDrop]);

const handleDrop = useCallback((e: React.DragEvent<HTMLDivElement>) => {
  e.preventDefault();
  e.stopPropagation();
  dragCounterRef.current = 0;
  setIsDragOver(false);
  if (!canAcceptDrop) return;
  const files = Array.from(e.dataTransfer.files);
  if (files.length === 0) return;
  uploadFileList(files);
}, [canAcceptDrop, uploadFileList]);
```

Wire to the terminal container div:
```tsx
<div
  className={styles.terminal}
  ref={terminalContainerRef}
  onDragEnter={handleDragEnter}
  onDragOver={handleDragOver}
  onDragLeave={handleDragLeave}
  onDrop={handleDrop}
  data-testid="terminal-drop-zone"
>
```

### Task 4 — Drop-zone overlay (frontend UI, ~1h)

**File:** `web-app/src/components/sessions/TerminalOutput.tsx`

Add inside `<div className={styles.terminal}>`, just before the xterm
`<XtermTerminal>` mount point:

```tsx
{isDragOver && canAcceptDrop && (
  <div
    className={styles.dropZoneOverlay}
    aria-hidden="true"
    data-testid="drop-zone-overlay"
  >
    <span className={styles.dropZoneLabel}>Drop to attach</span>
  </div>
)}
```

### Task 5 — CSS styles (frontend styling, ~30m)

**File:** `web-app/src/components/sessions/TerminalOutput.css.ts`

Add two new vanilla-extract rules:

```ts
export const dropZoneOverlay = style({
  position: 'absolute',
  inset: 0,
  zIndex: 20,  // above terminal canvas (1), below toast overlays (30+)
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  backgroundColor: 'rgba(0, 0, 0, 0.45)',
  pointerEvents: 'none',   // don't intercept mouse events — xterm handles those
  borderRadius: 'inherit',
});

export const dropZoneLabel = style({
  color: '#fff',
  fontSize: '1rem',
  fontWeight: 600,
  letterSpacing: '0.02em',
  padding: '8px 16px',
  borderRadius: '6px',
  border: '2px dashed rgba(255,255,255,0.7)',
  background: 'rgba(0,0,0,0.3)',
  userSelect: 'none',
});
```

Verify z-index values against existing overlays in the file (`loadingOverlay`,
`reconnectingBanner`, `resizingOverlay`) to confirm no conflicts.

### Task 6 — Jest unit tests (~2h)

**File:** `web-app/src/components/sessions/__tests__/TerminalDnD.test.tsx` (new)

Test cases:
1. `dragenter` with `files` type sets `isDragOver=true` after first enter
2. `dragenter` twice / `dragleave` once does NOT clear `isDragOver` (counter guard)
3. `dragleave` after counter reaches 0 clears `isDragOver`
4. `drop` clears `isDragOver` and calls `uploadFileList` with the dropped files
5. `dragenter` with no `Files` type (pane drag) does NOT set `isDragOver`
6. `dragenter` when `readOnly=true` does NOT set `isDragOver`
7. `dragenter` when `hasFinePointer=false` (mobile) does NOT set `isDragOver`

Use `@testing-library/react` + `fireEvent.dragEnter/dragLeave/drop` with mock
`dataTransfer.types` and `dataTransfer.files`.

### Task 7 — E2E test (~2h)

**File:** `tests/e2e/terminal-dnd-upload.spec.ts` (new)

Annotation: `// @feature terminal-image-upload`

Test cases:
1. **Drop image file** — simulate a PNG drop on `[data-testid="terminal-drop-zone"]`;
   assert `[data-testid="drop-zone-overlay"]` appeared during drag, disappears after
   drop, and an upload success toast appears.
2. **Drop non-image file** — simulate a `.txt` file drop; assert path injected.
3. **Multiple files** — simulate dropping 2 files; assert two separate path insertions.

Use Playwright's `page.dispatchEvent` with a synthetic `DataTransfer` carrying
`File` objects (constructed in `page.evaluate`).

No `waitForTimeout` — use `expect(locator).toBeVisible()` / `.not.toBeVisible()`.

### Task 8 — Feature docs (~30m)

**File:** `docs/features.md`

Add an entry under the Terminal section:

> **Drag-and-drop file upload** — Drag any file (image, text, archive) from your OS
> file manager onto the terminal area of a live session. The file is uploaded to the
> session's `uploads/` directory and its path is inserted into the terminal input.
> Desktop only (requires a mouse/pointer device).

---

## Dependency Order

```
Task 1 (extract uploadFileList)
  └── Task 3 (drag handlers — calls uploadFileList)
        ├── Task 2 (state — isDragOver, dragCounterRef)
        ├── Task 4 (overlay render)
        ├── Task 5 (CSS)
        └── Task 6 (unit tests)
Task 7 (E2E) — depends on Tasks 3–5 being in the build
Task 8 (docs) — independent
```

Tasks 1–5 can be done in one PR commit. Tasks 6–8 can be added in the same PR.

## Estimated Cost

| Task | Category | Estimate |
|---|---|---|
| 1. Extract uploadFileList | frontend | 2h |
| 2. Drag state | frontend | 1h |
| 3. Drag handlers | frontend | 1h |
| 4. Drop overlay | frontend | 1h |
| 5. CSS styles | frontend | 30m |
| 6. Jest unit tests | test | 2h |
| 7. E2E test | test | 2h |
| 8. Docs | docs | 30m |
| **Total** | | **~10h** |

## Pre-Ship Checklist

- [ ] `make quick-check` passes (build + test + lint)
- [ ] `make ready-duplication-gate-web` passes (no new jscpd duplication)
- [ ] `dragCounterRef` counter resets to 0 in `handleDrop` (not just `setIsDragOver(false)`)
- [ ] `data-testid="terminal-drop-zone"` on terminal container
- [ ] `data-testid="drop-zone-overlay"` on overlay div
- [ ] `readOnly=true` sessions: no overlay, no upload
- [ ] Mobile/touch (`!hasFinePointer`): no overlay, no drop handlers active
- [ ] Pane drag (`dataTransfer.types` lacks `'Files'`): no overlay
- [ ] Feature entry in `docs/features.md`
