# Validation: Terminal Drag-and-Drop Upload

## Adversarial Review

### Challenge 1: Is the `dragCounterRef` pattern correct when xterm's canvas is replaced by WebGL?

**Claim in plan:** The counter approach handles spurious `dragleave` from xterm child elements.

**Adversarial concern:** xterm.js in WebGL mode (`WebglAddon`) renders via a `<canvas>` element.
Canvas elements can have `pointer-events: none` applied by xterm, which would mean
`dragenter`/`dragleave` never fires on the canvas children at all — making the counter
redundant, or (worse) misfiring if pointer-events behavior differs between canvas and HTML
children.

**Resolution:** `dragenter` and `dragleave` fire based on DOM element boundaries, not pointer-events
CSS. `pointer-events: none` affects mouse click/hover events, not drag-and-drop events, because DnD
uses the separate `DragEvent` system which follows the DOM layout, not the pointer-events tree.
So xterm's canvas children *do* fire `dragleave`/`dragenter` as the cursor moves across them,
regardless of `pointer-events: none`. The counter approach is still correct. ✅

### Challenge 2: Will `e.dataTransfer.types.includes('Files')` prevent interference with the pane-reorder drag?

**Claim in plan:** Filter on `dataTransfer.types.includes('Files')` prevents activating the drop
zone for pane-drag.

**Adversarial concern:** What MIME type does the pane-reorder drag use?

**Verification:** `PaneSplitRenderer.tsx` lines 339–350 — `e.dataTransfer.setData('text/plain', paneId)`.
So pane drags have `types = ['text/plain']`, not `'Files'`. The filter works. ✅

### Challenge 3: Does the approach conflict with the `image-file-upload` project (multi-file upload refactor)?

**Claim in plan:** Build on top of `handleFilesUpload` / the existing `POST /api/v1/upload-image` endpoint.

**Adversarial concern:** The `image-file-upload` plan (`project_plans/image-file-upload/requirements.md`)
proposes renaming the route from `/api/upload/image` to `/api/upload/file` and renaming
`AttachedImage` → `AttachedFile`. Does that conflict?

**Resolution:** The `image-file-upload` plan targets the **Omnibar creation panel**
(`OmnibarCreationPanel.tsx`, separate from `TerminalOutput.tsx`) and the backlog-attachment
path (`/api/upload/image` vs `/api/v1/upload-image`). These are distinct routes. The terminal
upload (`/api/v1/upload-image` → `session_image_upload_handler.go`) is *not* in scope of the
`image-file-upload` plan, which explicitly lists "Drag-and-drop upload" as out-of-scope.
No conflict. If both land in the same sprint, they touch different files and routes. ✅

### Challenge 4: Is the `hasFinePointer` guard actually populated correctly at drag time?

**Claim in plan:** `const { isMobile, hasFinePointer } = useViewport()` gates drag features.

**Adversarial concern:** `useViewport` evaluates pointer media queries at mount time. A device
connected to a mouse after mount would not update `hasFinePointer`.

**Resolution:** `useViewport` uses a `matchMedia` listener — checking the project's
`ViewportProvider.tsx` is warranted. Even if it only evaluates at mount, this is acceptable:
drag-and-drop is a desktop-first enhancement. A phone that gains a mouse after mount is an
extreme edge case; the existing toolbar upload buttons work regardless. The read-only guard is
the security-critical one (not the mobile guard). ✅ (acceptable trade-off)

### Challenge 5: Does `pointer-events: none` on `dropZoneOverlay` cause the `dragleave` to fire unexpectedly?

**Claim in plan:** `pointer-events: none` on the overlay to not intercept mouse events.

**Adversarial concern:** When the overlay becomes visible (on `dragenter`), it covers the
terminal. If `pointer-events: none` is NOT set, the overlay becomes the drag target and
`dragenter`/`dragleave` would fire again on the overlay itself, making the counter
double-increment and break cleanup.

**Resolution:** `pointer-events: none` is explicitly specified in Task 5's CSS (`pointerEvents: 'none'`).
This means the overlay does not become a separate drag event source — drag events continue
to bubble through it to the terminal container div. The counter stays consistent. ✅

### Challenge 6: What happens if a user drops a file while the upload of a previous drop is in-flight?

**Claim in plan:** Reuses `uploadingCount` etc.

**Adversarial concern:** The existing `handleFilesUpload` loop runs sequentially and uses
`setUploadingCount(total - i)` as a progress counter. If a second drop fires before the
first completes, `uploadingCount` state could become confused (two concurrent calls both
calling `setUploadingCount(0)` at the end).

**Resolution:** The existing `handleFilesUpload` already has this same concurrent-drop
risk via multiple toolbar-click actions (unlikely but not prevented). The fix is the same
for both: either serialize via a queue ref, or simply let the "N uploading" display
temporarily show the larger count and reset to 0 when either finishes. For the V1 of this
feature, accept the same behavior as the existing toolbar path — the outcome is that the
files are uploaded and paths inserted correctly either way, just the progress toast may
jump. Track as a known limitation, not a blocker. ✅ (acceptable for V1)

### Challenge 7: Is `data-testid="terminal-drop-zone"` on the terminal container div safe?

**Claim in plan:** Add `data-testid="terminal-drop-zone"` to the terminal container.

**Adversarial concern:** Existing E2E tests targeting the terminal element by className or
other attribute might be affected.

**Resolution:** Adding a new `data-testid` attribute is purely additive — it does not
remove or modify existing attributes, class names, or DOM structure. No existing test will
break from this addition. ✅

---

## Open Questions

1. **`ViewportProvider` reactivity:** Confirm that `useViewport`'s `hasFinePointer` updates
   reactively on subsequent pointer-media changes (not a blocker, but worth a one-line check
   of `ViewportProvider.tsx` before ship).

2. **z-index values:** Verify the `dropZoneOverlay` z-index (proposed: 20) doesn't conflict
   with existing overlays in `TerminalOutput.css.ts`. Run `grep -n 'zIndex' TerminalOutput.css.ts`
   before finalising the value.

3. **`image-file-upload` sprint coordination:** If both features land in the same sprint,
   Task 1 (extract `uploadFileList`) may produce a minor merge conflict with `image-file-upload`'s
   changes to `OmnibarCreationPanel.tsx`. These are different files so the risk is low, but
   awareness is warranted.

---

## Validation Summary

| Area | Status | Notes |
|---|---|---|
| `dragCounterRef` with WebGL canvas | ✅ Valid | DragEvent fires independently of pointer-events CSS |
| Pane-drag non-interference | ✅ Valid | pane drag uses `text/plain`, not `'Files'` |
| Route conflict with `image-file-upload` plan | ✅ Valid | Different routes and files |
| `hasFinePointer` guard reactivity | ✅ Acceptable | Edge case; acceptable for V1 |
| Overlay `pointer-events: none` | ✅ Valid | Prevents overlay from disrupting drag counter |
| Concurrent drop race on `uploadingCount` | ✅ Acceptable | Same limitation as existing toolbar path |
| `data-testid` addition safety | ✅ Valid | Purely additive |

**Plan is approved for implementation.** No blocking concerns identified.
