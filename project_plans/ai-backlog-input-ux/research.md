# Research: AI Backlog Input UX

**Date:** 2026-10-11

## Current Code Paths

### Entry point: OmnibarResultList "create backlog" button

`web-app/src/components/sessions/OmnibarResultList.tsx` receives an `onCreateBacklog?: () => void` callback. The parent (`Omnibar.tsx:2034`) wires it as:

```tsx
onCreateBacklog={() => {
  setInput("backlog: ");          // ← discards existing text (AC-1 bug)
  setResultHighlightIndex(-1);
  inputRef.current?.focus();
}}
```

The `backlog: ` prefix triggers `InputType.ChatBacklogItem` detection → `setBacklogReviewText(detection.parsedValue)` → renders `BacklogItemIntentReview` in place of the omnibar body.

### BacklogItemIntentReview flow

`web-app/src/components/backlog/BacklogItemIntentReview.tsx`:
1. Mount → calls `parseBacklogItemIntent(initialText)` (blocking RPC, ~2–5 s).
2. Shows `<ParsingIndicator>` ("Parsing your message…") while waiting.
3. On result: populates `BacklogItemForm` with `initialValues`.
4. User edits → clicks "Create" → `createBacklogItem(data)` → `onDone(item)` navigates to `/backlog?item=<id>`.

The `BacklogItemIntentReview` layer is synchronous/blocking from the UX perspective — it holds the omnibar modal open through both the parse and create RPCs.

### BacklogItemForm — attachment already exists

`web-app/src/components/backlog/BacklogItemForm.tsx` already uploads via `POST /api/v1/upload-backlog-attachment` and inserts `![filename](url)` markdown. The `MAX_ATTACHMENT_BYTES` constant is 10 MB. This logic is self-contained in `uploadAttachment` callback. It can be extracted or reused.

### Notification system

`NotificationContext.tsx` exposes `addNotification(notification)`. Callers pass `{ sessionId, sessionName, message, notificationType, onView }`. This already renders toasts and stores history. There is also `showUndoToast` and `appendEphemeral` for lighter toasts. This is the right channel for the "item created" completion notification.

### Keyboard entry path — still valid for AC-5

`web-app/src/lib/omnibar/detector.ts` detects `InputType.ChatBacklogItem` when input starts with `backlog:`. This existing path must remain unchanged.

## Design Decision: Where Does the Compose Widget Live?

**Option A — Inside the omnibar modal**: Replace the result list view with a new `BacklogComposeWidget` panel when the button is clicked. Closes on submit/cancel.

**Option B — A separate drawer/modal**: A second modal on top of the omnibar.

**Option C — Inline in the result list**: Expand the result list item in place.

**Chosen: Option A.** It fits the existing pattern where `backlogReviewText !== null` replaces the omnibar body (Omnibar.tsx:1700–1722). Instead of waiting for a parse to complete, we show the compose widget immediately with the existing input text pre-filled. The compose widget does _not_ call `parseBacklogItemIntent` synchronously; it fires it in the background on submit.

## Background Processing Design

On submit from `BacklogComposeWidget`:
1. Close the omnibar immediately (`onClose()`).
2. Fire an async chain: `parseBacklogItemIntent(rawText)` → merge with any user edits → `createBacklogItem(...)` — or skip parse and use raw text directly if parsing times out / fails.
3. On success: `addNotification({ message: "Backlog item created: <title>", onView: () => router.push('/backlog?item=<id>') })`.
4. On failure: `addNotification({ message: "Failed to create backlog item: <error>", ... })`.

The `useNotificationCommands` hook (from `NotificationContext`) provides `addNotification`. The compose widget needs access to it via the hook, same pattern used in other components.

A simpler variant: skip `parseBacklogItemIntent` in the background path entirely and call `createBacklogItemFromChat(rawText)` directly (already exists in `useBacklogService`). This collapses parse + create into one RPC, reducing round trips and simplifying error handling. **Use `createBacklogItemFromChat` as the background path.**

## Omnibar Mode State Change

Omnibar.tsx uses `backlogReviewText: string | null` state to signal "in review mode". For the new compose widget, we can add a similar `backlogComposeText: string | null` state (initialized from the current `input` value when the button is clicked, not from `"backlog: "` replacement). The compose widget renders in the same early-return slot as review mode.

Both states cannot be active simultaneously (compose precedes review; review only happens when the legacy `backlog:` prefix path is used).

## Attachment Re-use Strategy

Extract the upload logic from `BacklogItemForm.uploadAttachment` into a shared hook `useBacklogAttachment()` in `web-app/src/lib/hooks/`, or copy the minimal upload snippet directly into `BacklogComposeWidget`. Given the widget is small, a lightweight inline copy with a shared constant for the endpoint URL is acceptable. If a third consumer appears, extract to a hook then.

## Risks

1. **Background parse failure**: `createBacklogItemFromChat` already handles parse failure server-side (falls back to raw text). Client-side, the error notification path covers it.
2. **Large attachments**: `MAX_ATTACHMENT_BYTES = 10 MB` cap enforced client-side and server-side.
3. **jscpd duplication gate**: The attachment upload block is ~15 lines. If copied verbatim, `jscpd` at minLines=20 won't flag it, but keep an eye on the threshold.
4. **OmnibarResultList aria wiring**: `getResultListItemCount` and `getHighlightedItemId` in `OmnibarResultList.tsx` need a corresponding count adjustment if the "create backlog" button index changes (currently it's `createBacklogIndex = createNewIndex + 1`). No structural change needed — the button stays in the same list position.
