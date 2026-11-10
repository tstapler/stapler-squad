# ADR-006: Hidden textarea is the primary input source; EditContext only as a later sub-flagged enhancement

## Status
Accepted (2026-11-06)

## Context
EditContext ships in Chrome 121+ per web-features, but caniuse lists Chrome Android inconsistently, and Firefox/Safari lack it (`research/stack.md` 2). `getTargetRanges()` is empty for textareas, so the layer diffs against its own mirrored state instead.

## Decision
Hidden `<textarea>` mounted as a SIBLING of, and outside, the element passed to the renderer (for ghostty: the element given to `open()`, which carries the blanket `beforeinput` `preventDefault` and keydown handlers; ghostty's own hidden textarea is made `inputmode="none"`, `tabindex="-1"` and blurred on touch). Appending it inside that element would let its `beforeinput` bubble into ghostty's handler. The layer owns tap-to-focus (`controller.bindTapTarget`), bound to the VISIBLE terminal surface (never the 1px `pointer-events:none` textarea), focusing synchronously in the gesture on `pointerup`/`touchend`/`click` (deduplicated; physical-phone check Task 0.1.3b), so a tap on either renderer leaves the layer textarea as `document.activeElement`. The layer attaches when the terminal is visible/docked (not on focus) so `bindTapTarget` exists for the very first tap; focus only moves between already-attached layers. An `InputSource` seam inside the adapter allows an EditContext source later, behind sub-flag `mobile_input_edit_context` (default off) and `'EditContext' in window`, only if the device corpus shows `beforeinput` gaps. Never mutate the textarea value or selection while a composition is live (ProseMirror lesson); reset it only at commit boundaries.

## Alternatives rejected
contenteditable (ghostty already makes its container contenteditable; more IME quirks); EditContext primary (no iOS/Firefox, unverified on Gboard).

## Consequences
Story 6.1 is conditional. The adapter keeps a single `InputSource` interface so adding it does not touch the reducer.
