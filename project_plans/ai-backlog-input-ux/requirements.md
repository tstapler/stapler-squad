# Requirements: AI Backlog Input UX

**Project:** `ai-backlog-input-ux`  
**Item ID:** 57e83cfe-378b-4d97-b21a-d4b057b71406  
**Date:** 2026-10-11

## Problem Statement

The AI create-backlog-item flow in the omnibar has three concrete defects and a discoverability gap:

1. **Text discard**: Clicking the "create backlog item" button at the bottom of the omnibar result list unconditionally calls `setInput("backlog: ")`, which replaces whatever the user had already typed (Omnibar.tsx:2035).

2. **No attachment surface in the compose step**: The image attachment UI exists in `BacklogItemForm` (which is reached only after LLM parsing completes), but not in the initial capture widget. Users want to attach screenshots/images _before_ submitting for parsing.

3. **Blocking submission**: After the user clicks the AI-create button, the omnibar shows a "Parsing…" spinner while the `parseBacklogItemIntent` RPC runs, then the review form. Creating the item blocks the screen. Users want to submit and keep working, with a notification when the item is ready.

4. **Discoverability gap**: Entering AI mode currently requires typing the `backlog:` prefix into the single-line omnibar input. A dedicated, visible control is needed.

## Out of Scope

- Changes to the `BacklogItemForm` review/edit experience (already satisfactory).
- Changes to `ParseBacklogItemIntent` or `CreateBacklogItem` RPCs (already exist and work).
- New backend endpoints (attachment upload at `POST /api/v1/upload-backlog-attachment` already exists).
- The backlog board or item detail page.

## Acceptance Criteria

### AC-1: Text preservation
When the user has typed text into the omnibar input and then clicks "Create backlog item" (or the equivalent new widget), that text is carried over as the initial content of the compose widget; nothing is silently discarded.

### AC-2: Dedicated paragraph-style compose widget
A visually distinct multi-line compose area (not the single-line omnibar `<input>`) appears when the user enters AI backlog mode. It has:
- A multi-line `<textarea>` for the item description.
- An explicit "Attach image" control that uses the existing `/api/v1/upload-backlog-attachment` endpoint and inserts a markdown image reference.
- A "Create" submit button and a "Cancel" button.

### AC-3: Background processing
After the user clicks "Create", the omnibar closes immediately. The `parseBacklogItemIntent` + `createBacklogItem` calls run in the background (fire-and-forget from the UI's perspective). When creation completes, a notification appears with the item title and a deep link to `/backlog?item=<id>`. On failure, a notification with an error message appears.

### AC-4: Visible mode button
A clearly labelled button (e.g. "📝 Create backlog item") is visible at the bottom of the omnibar result list (existing `onCreateBacklog` slot) that switches the omnibar into the dedicated compose widget defined in AC-2.

### AC-5: No regression — existing `backlog:` prefix still works
Typing `backlog: <message>` in the omnibar input still opens the existing `BacklogItemIntentReview` flow unchanged, so power-user muscle memory is preserved.

### AC-6: Mobile-compatible
The compose widget works on narrow viewports (touch targets ≥ 44 px, textarea resizes to fit its container, image attachment uses file picker input).

### AC-7: Accessibility baseline
The new compose widget passes axe-core WCAG AA (no new errors introduced); submit button is reachable by keyboard; textarea has a visible label.
